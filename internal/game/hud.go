package game

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"

	"golang.org/x/image/font/gofont/gomono"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/msdf"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/ui"
	"github.com/derekmwright/vesper3/internal/colony"
)

// hudMaxQuads bounds the dynamic mesh the whole interface is built into. The
// engine's UpdateMeshData clamps silently past the capacity a mesh was created
// with, so drawHUD checks what it actually built rather than trusting this.
const hudMaxQuads = 512

// The interface palette, written the way the colours are picked: as they
// should look on screen.
//
// That is only true as of glyphengine#13. The overlay shaders used to pass a
// game's colour straight through to a B8G8R8A8_SRGB swapchain, which meant the
// hardware encoded it on the way out and declared it linear on the way in — so
// a backdrop written as 0.05 meaning "nearly black" arrived at about 0.24, a
// mid slate. This file used to carry an srgb() helper that converted every
// constant below into linear to compensate.
//
// ui.frag and msdf.frag now decode with srgbToLinear themselves. The helper is
// gone, and it had to go in the same change as the engine bump: leaving it in
// would apply the conversion twice and take the whole interface near-black.

// Palette, in display space. A HUD that has to be read at a glance while the
// eye is on the map gets its meaning from colour before it gets it from text,
// so these are shared by the bars, the numbers and the alert strip rather than
// being picked per widget.
var (
	colInk      = [3]float32{0.945, 0.955, 0.968}
	colDim      = [3]float32{0.786, 0.815, 0.849}
	colGood     = [3]float32{0.680, 0.931, 0.748}
	colWarn     = [3]float32{0.982, 0.876, 0.566}
	colCritical = [3]float32{1.000, 0.680, 0.634}
	colAccent   = [3]float32{0.810, 0.896, 0.978}

	// Surfaces. Dark navy and close to black: the panels sit over a sunlit
	// map, and anything lighter costs contrast against the text on top of
	// them, which is the only reason the panels exist.
	colPanel     = [3]float32{0.050, 0.060, 0.090}
	colPanelEdge = [3]float32{0.350, 0.450, 0.580}
	colBarBack   = [3]float32{0.160, 0.180, 0.220}
	colSlot      = [3]float32{0.075, 0.095, 0.130}
	colSlotPick  = [3]float32{0.200, 0.300, 0.400}
	colSlotHover = [3]float32{0.130, 0.170, 0.230}
)

// panelOpacity lets a little of the map through the interface, which keeps it
// feeling laid over the world rather than bolted in front of it. It is only
// affordable because the backdrop under it is nearly black.
const panelOpacity = 0.86

// The icons are packed into one texture by cmd/iconatlas. One atlas rather
// than one texture per icon, because a texture switch is a draw call and the
// whole interface is otherwise a single one.
//
// The first len(colony.Buildable) cells are the hotbar, in Buildable order.
// The rest are the resources, named below. A test holds the grid to the number
// of cells actually needed, so adding a structure without adding a row here
// fails rather than silently reusing someone else's art.
const (
	atlasPath = "assets/icons.png"
	atlasCols = 4

	// Five rows rather than four since the synthesizer and vespite arrived:
	// ten structures and seven resource rows is seventeen cells, and sixteen
	// was exactly full. The grid is declared to the packer rather than
	// inferred from the source count, so a missing source file cannot quietly
	// shrink the atlas out from under these indices.
	atlasRows = 5
)

// Resource cells, after the structures.
//
// These are indices into the atlas rather than a property of colony.Kind,
// because they are not about structures at all — and package colony has no
// business knowing an atlas exists.
//
// Vars rather than constants because where they start depends on how many
// structures there are. Deriving it is the only way adding a structure cannot
// silently shift every resource icon one cell to the left.
var (
	iconWater   = len(colony.Buildable)
	iconIron    = iconWater + 1
	iconCrystal = iconWater + 2
	iconFood    = iconWater + 3
	iconPower   = iconWater + 4

	// Colonists are not a resource — no bar, no rate, a ceiling rather than a
	// flow — but the row still sits in the same column and reads better with
	// the same treatment as the ones above it.
	iconColonists = iconWater + 5

	// Vespite sits last because it is the newest and because it is the only
	// row that is not about staying alive.
	iconVespite = iconWater + 6

	// iconCount is how many cells the atlas actually has to hold.
	iconCount = iconWater + 7
)

// The panel bezel texture and how it slices.
//
// frameInset is the corner block in texels, measured off the artwork: too
// small and stretching the edges eats into the corner decoration, too large
// and the corners of a small panel overlap each other.
const (
	framePath    = "assets/panel.png"
	frameTexSize = 256
	frameInset   = 42

	// Nine quads per panel, and the interface draws a handful of panels.
	hudMaxFrameQuads = 128
)

// hud owns the GPU resources the interface is drawn with.
type hud struct {
	font *renderer.Font
	text *renderer.MSDFText
	mesh *renderer.Mesh

	verts []renderer.Vertex
	idx   []uint16
	lines []renderer.TextLine

	// Icons are a second mesh and a second draw: they sample the atlas, and
	// the flat panel geometry does not, so they cannot share one.
	icons     *renderer.Texture
	iconMesh  *renderer.Mesh
	iconVerts []renderer.Vertex
	iconIdx   []uint16

	// The debug readout's backdrop is its own draw for the same reason the
	// menu veil is not: it has to land on top of every panel, icon and bezel
	// rather than under them, and draw order is layer order. One quad.
	debugMesh  *renderer.Mesh
	debugVerts []renderer.Vertex
	debugIdx   []uint16

	// The splash logo is its own texture and so its own draw again.
	logo      *renderer.Texture
	logoMesh  *renderer.Mesh
	logoVerts []renderer.Vertex
	logoIdx   []uint16

	// Button artwork, one nine-slice per state. Each is its own texture and so
	// its own draw, which is why they accumulate separately; see button.go.
	buttons [buttonStateCount]*buttonLayer

	// The panel bezel is a nine-slice, drawn in the UI pipeline's panel mode
	// rather than its texture mode: that mode reads the texture's alpha to
	// tell frame from fill, giving the translucent interior for free.
	frame      *renderer.NineSlice
	frameMesh  *renderer.Mesh
	frameVerts []renderer.Vertex
	frameIdx   []uint16

	// scale converts design units to pixels. Everything in this file is laid
	// out in design units — the geometry constants, every x and y, every text
	// size — and multiplied by this exactly once, where the quad or the line
	// is emitted. Scaling at the boundary rather than at each call site is
	// what keeps the layout arithmetic readable when the whole interface is
	// being drawn at 2x on a high-density display.
	scale float32

	// overflowed records that a build exceeded hudMaxQuads, so the failure
	// surfaces as a message rather than as quietly missing widgets.
	overflowed bool
}

// initHUD builds the font and the mesh the interface needs.
//
// The atlas is generated at startup rather than shipped as a pre-baked PNG and
// JSON, which is what renderer.LoadFont expects: the TTF is one file, it is the
// thing with a licence attached, and generating from it means the atlas can
// never be stale with respect to the font it came from.
//
// Exo 2 is a proportional face, so the numbers on this panel change width as
// they change value. That is handled by right-aligning every numeric column
// (see rightLabel) rather than by picking a monospace font — alignment is what
// actually makes a column of figures readable, and it is worth having whatever
// the face is.
// uiState is the interface's own state, as distinct from the world's: what the
// pointer is over, what the player is being told, and what they have asked to
// see. Nothing here is part of the simulation, and none of it is saved.
// The name carries the "State" suffix only because the engine's own ui package
// is imported here; the field on Game is just g.ui.
type uiState struct {
	// hot is the hotbar slot under the pointer, or -1. blocked is true when
	// the pointer is over any panel at all, which suppresses world
	// interaction — a click on the hotbar must not also land on the tile
	// behind it.
	hot     int
	blocked bool

	// scaleAdj is the live interface scale override, from the bracket keys.
	scaleAdj float32

	// showDebug toggles the raw text readout over the panel, on F3.
	showDebug bool

	// runtime is the last sample of the Go runtime, kept between frames
	// because taking one stops the world; see debugstats.go.
	runtime runtimeStats

	// frames is the rolling frame-time window behind the tail figures; see
	// debugstats.go for why a mean FPS is not enough.
	frames frames

	// lightDebug is the engine's clustered-lighting view: off, a heatmap of
	// per-cell light counts, or the brute-force reference path. Cycled with
	// F4; see actions.go.
	lightDebug glyph.LightDebugMode

	// confirm is the open question, if there is one. Demolition is the only
	// irreversible action in the game, so it is the only one that asks.
	confirm *confirmPrompt

	// status is the transient line under the panel, and statusUntil is the
	// elapsed time it stops being shown at.
	status      string
	statusUntil float32
}

func newUI() uiState { return uiState{hot: -1} }

func (g *Game) initHUD(e *glyph.Engine) error {
	r := e.Renderer()

	atlas, err := msdf.Generate(g.fontBytes(), msdf.Options{})
	if err != nil {
		return fmt.Errorf("generate HUD font: %w", err)
	}
	meta, err := json.Marshal(atlas.Meta)
	if err != nil {
		return fmt.Errorf("HUD font metadata: %w", err)
	}
	font, err := renderer.NewFontFromAtlas(r, atlas.Image, meta)
	if err != nil {
		return fmt.Errorf("HUD font: %w", err)
	}
	text, err := renderer.NewMSDFText(r, font)
	if err != nil {
		return fmt.Errorf("HUD text: %w", err)
	}
	mesh, err := r.CreateDynamicIndexedMesh(hudMaxQuads*4, hudMaxQuads*6)
	if err != nil {
		return fmt.Errorf("HUD mesh: %w", err)
	}

	h := &hud{font: font, text: text, mesh: mesh}

	// The panel bezel. Optional, like the icons: without it panels fall back
	// to the flat rectangles the layout was built against.
	if g.cfg.Assets != nil {
		if tex, err := r.LoadTexture(g.cfg.Assets, framePath); err != nil {
			log.Printf("panel bezel unavailable (%v); drawing flat panels", err)
		} else {
			frameMesh, err := r.CreateDynamicIndexedMesh(hudMaxFrameQuads*4, hudMaxFrameQuads*6)
			if err != nil {
				return fmt.Errorf("panel mesh: %w", err)
			}
			h.frame = renderer.NewNineSlice(tex, frameTexSize, frameInset)
			h.frameMesh = frameMesh
		}
	}

	// Icons are optional. A build without the atlas should lose the pictures
	// and keep the interface, not fail to start: every slot is labelled in
	// text as well, so the game is fully playable without them.
	if g.cfg.Assets != nil {
		if tex, err := r.LoadTexture(g.cfg.Assets, atlasPath); err != nil {
			log.Printf("structure icons unavailable (%v); falling back to text-only slots", err)
		} else {
			iconMesh, err := r.CreateDynamicIndexedMesh(hudMaxIcons*4, hudMaxIcons*6)
			if err != nil {
				return fmt.Errorf("icon mesh: %w", err)
			}
			h.icons, h.iconMesh = tex, iconMesh
		}
	}

	debugMesh, err := r.CreateDynamicIndexedMesh(4, 6)
	if err != nil {
		return fmt.Errorf("debug backdrop mesh: %w", err)
	}
	h.debugMesh = debugMesh

	g.hud = h
	return nil
}

// fontPath is the interface typeface, bundled under the SIL Open Font Licence
// (assets/fonts/OFL.txt).
const fontPath = "assets/fonts/Exo2.ttf"

// fontBytes returns the bundled typeface, or Go Mono if it cannot be read.
//
// The fallback is the engine's own debug font, which is always in the module
// graph. A missing asset should cost the game its typography, not its ability
// to start.
func (g *Game) fontBytes() []byte {
	if g.cfg.Assets == nil {
		return gomono.TTF
	}
	b, err := fs.ReadFile(g.cfg.Assets, fontPath)
	if err != nil {
		log.Printf("HUD font %s unavailable (%v); falling back to Go Mono", fontPath, err)
		return gomono.TTF
	}
	return b
}

// hudMaxIcons bounds the icon mesh: one per hotbar slot plus a few spare.
const hudMaxIcons = 32

// icon appends a textured quad sampling atlas cell n.
func (h *hud) icon(x, y, size float32, n int) {
	if h.icons == nil || n < 0 || n >= atlasCols*atlasRows {
		return
	}

	u0 := float32(n%atlasCols) / atlasCols
	v0 := float32(n/atlasCols) / atlasRows
	u1 := u0 + 1.0/atlasCols
	v1 := v0 + 1.0/atlasRows

	// White vertex colour: the icon pipeline multiplies texture by vertex
	// colour, so anything else would tint the art.
	const white = 1
	col := [3]float32{white, white, white}

	k := h.scale
	x, y, size = x*k, y*k, size*k

	base := uint16(len(h.iconVerts))
	h.iconVerts = append(h.iconVerts,
		renderer.Vertex{Pos: [3]float32{x, y, 0}, Color: col, UV: [2]float32{u0, v0}},
		renderer.Vertex{Pos: [3]float32{x + size, y, 0}, Color: col, UV: [2]float32{u1, v0}},
		renderer.Vertex{Pos: [3]float32{x + size, y + size, 0}, Color: col, UV: [2]float32{u1, v1}},
		renderer.Vertex{Pos: [3]float32{x, y + size, 0}, Color: col, UV: [2]float32{u0, v1}},
	)
	h.iconIdx = append(h.iconIdx, base, base+1, base+2, base+2, base+3, base)
}

// quad appends one rectangle to the HUD mesh, in design units.
func (h *hud) quad(x, y, w, ht float32, col [3]float32) {
	k := h.scale
	h.verts, h.idx = ui.AppendQuad(h.verts, h.idx, x*k, y*k, w*k, ht*k, col)
}

// panel draws the background behind a group of widgets.
//
// With the bezel texture loaded this is a nine-slice: the UI pipeline's panel
// mode uses the texture's alpha to separate frame from fill, so the centre
// comes out as a translucent dark wash and the border keeps the artwork's own
// colours. Without it, the flat rectangle the interface was built on.
func (h *hud) panel(x, y, w, ht float32) {
	if h.frame == nil {
		h.quad(x, y, w, ht, colPanel)
		h.quad(x, y, w, 2, colPanelEdge)
		return
	}

	// The backing runs the full rectangle, under the bezel rather than inside
	// it. Inset by even a couple of units it left a strip along every edge
	// where neither the backing nor the bezel artwork covered — the bezel
	// carries a small transparent margin of its own — and the map showing
	// through that strip read as a grey border around every panel.
	h.quad(x, y, w, ht, colPanel)

	k := h.scale
	verts, _ := h.frame.GenerateQuads(
		x*k, y*k, w*k, ht*k,
		frameTexelScale*k,
		colFrame,
	)

	// Keep the eight frame quads and drop the centre one.
	//
	// Panel mode paints a quad whose texels are transparent as tint*0.2 at 70%
	// alpha — a fixed wash the game cannot choose. Over a sunlit map that came
	// out a mid slate however dark the backing underneath it was, which is
	// exactly the "lighter is harder to read" problem. Dropping the centre
	// leaves the bezel supplying the frame and the backing quad supplying the
	// fill, which is a colour this file controls.
	for q := 0; q+4 <= len(verts); q += 4 {
		if interiorQuad(verts[q : q+4]) {
			continue
		}
		base := uint16(len(h.frameVerts))
		h.frameVerts = append(h.frameVerts, verts[q:q+4]...)
		h.frameIdx = append(h.frameIdx,
			base, base+1, base+2, base+2, base+3, base)
	}
}

// interiorQuad reports whether a nine-slice quad is the middle one, by its UVs
// rather than its position in the list: GenerateQuads drops degenerate quads,
// so the middle is not always at a fixed index.
func interiorQuad(q []renderer.Vertex) bool {
	const edge = 0.001
	for _, v := range q {
		if v.UV[0] <= edge || v.UV[0] >= 1-edge || v.UV[1] <= edge || v.UV[1] >= 1-edge {
			return false
		}
	}
	return true
}

// frameTexelScale is screen pixels per texture pixel for the bezel.
//
// The artwork's corner block is frameInset texels, so this sets how thick the
// border reads: at 0.34 a 42-texel corner lands at about 14 design units,
// which frames a panel without crowding the text inside it.
const frameTexelScale = 0.34

// colFrame tints the bezel. Texture mode multiplies it into the artwork, so
// white leaves the frame exactly as drawn.
var colFrame = [3]float32{1, 1, 1}

// label queues a line of text. y is the TOP of the line box; see the geometry
// block below.
func (h *hud) label(x, y, size float32, col [3]float32, format string, args ...any) {
	k := h.scale
	h.lines = append(h.lines, renderer.TextLine{
		Text:  fmt.Sprintf(format, args...),
		X:     x * k,
		Y:     y * k,
		Scale: size * k,
		Color: col,
	})
}

// measure returns how wide a string will be drawn.
func (h *hud) measure(s string, scale float32) float32 {
	if h.font == nil {
		return float32(len(s)) * scale * 0.5
	}
	var w float32
	for _, r := range s {
		if g, ok := h.font.Glyphs[r]; ok {
			w += g.Advance * scale
		} else {
			w += 0.5 * scale
		}
	}
	return w
}

// rightLabel queues a line whose RIGHT edge sits at xRight.
//
// Every figure in the panel is drawn through this. In a proportional face a
// left-aligned number moves its own last digit every time the value changes,
// which turns a readout into a flicker; right-aligned, the decimal point holds
// still and only the digits change.
func (h *hud) rightLabel(xRight, y, scale float32, col [3]float32, format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	h.label(xRight-h.measure(s, scale), y, scale, col, "%s", s)
}

// clipped queues a line truncated to fit maxW pixels.
//
// Strings that come from the colony — an alert, the reason a tile refuses a
// building — are written for clarity rather than to a character budget, and
// the longest of them ran off the side of the panel and across the map. This
// measures against the font rather than assuming a width, so it stays correct
// if the panel is resized or the font changes.
func (h *hud) clipped(x, y, scale, maxW float32, col [3]float32, format string, args ...any) {
	h.label(x, y, scale, col, "%s", h.fit(fmt.Sprintf(format, args...), scale, maxW))
}

// fit truncates a string to maxW pixels at the HUD's font and the given scale.
func (h *hud) fit(s string, scale, maxW float32) string {
	if h.font == nil {
		return s
	}
	return fitText(s, maxW, func(r rune) float32 {
		if g, ok := h.font.Glyphs[r]; ok {
			return g.Advance * scale
		}
		return 0.5 * scale
	})
}

// fitText is the measurement, separated from the font so it can be tested
// without a GPU. It returns s unchanged when it fits, and otherwise the
// longest prefix that leaves room for the ellipsis.
func fitText(s string, maxW float32, advance func(rune) float32) string {
	rs := []rune(s)

	var w float32
	for i, r := range rs {
		rw := advance(r)
		if w+rw <= maxW {
			w += rw
			continue
		}

		// Overflowed at i, where w is the width of everything before it. Back
		// off until the ellipsis fits too.
		ellipsis := advance('.') * 3
		cut := i
		for cut > 0 && w+ellipsis > maxW {
			cut--
			w -= advance(rs[cut])
		}
		return string(rs[:cut]) + "..."
	}
	return s
}

// supplyBar draws production against consumption on one track.
//
// The track is the demand and the fill is the supply, so a bar that does not
// reach the end is a shortfall and the gap is its size. That is the reading
// the player needs, and it is the same shape for power, water and food: once
// the bar is understood for one resource it is understood for the rest.
func (h *hud) supplyBar(x, y, w, ht, produced, consumed float32) {
	h.quad(x, y, w, ht, colBarBack)
	if consumed <= 0 {
		// Nothing is drawing it: the whole track is surplus.
		if produced > 0 {
			h.quad(x, y, w, ht, colGood)
		}
		return
	}

	ratio := produced / consumed
	col := colGood
	switch {
	case ratio < 0.5:
		col = colCritical
	case ratio < 0.999:
		col = colWarn
	}
	h.quad(x, y, min32(ratio, 1)*w, ht, col)

	// Surplus rides above the track as a thin overflow, so "comfortably ahead"
	// and "exactly breaking even" do not look identical.
	if ratio > 1.02 {
		h.quad(x, y-3, min32((ratio-1)/2, 1)*w, 2, colAccent)
	}
}

// runwayFull is the horizon the stocked-resource gauge is drawn against, in
// seconds. Ten minutes in hand reads as full.
const runwayFull = 600

// runwayBar is the gauge for a resource that has a stock: how long that stock
// lasts, not how production compares to consumption.
//
// It used to be a supplyBar — production against consumption, which is the
// same pair of numbers the sub-line already spells out in words, and a picture
// that could contradict the figure beside it. A colony holding 157 food and
// losing it slowly drew a quarter-full red bar next to an obviously healthy
// number; a colony with a dry tank and a ledger that happened to balance drew
// a full green one. Neither was wrong about what it measured. Both were
// answering a question nobody asks of a bar.
//
// What a bar gets read as, on a row with a stock, is "how much trouble am I
// in". So that is what it shows. The numbers stay the flow; the bar is the
// runway.
func (h *hud) runwayBar(x, y, w, ht float32, left float64) {
	h.quad(x, y, w, ht, colBarBack)
	if left < 0 {
		// Not falling: nothing is running out, so the track is full.
		h.quad(x, y, w, ht, colGood)
		return
	}

	frac := float32(left / runwayFull)
	// A sliver rather than nothing at zero, so an empty stock still reads as a
	// bar that has run down rather than as a row with no bar at all.
	h.quad(x, y, max32(min32(frac, 1), 0.015)*w, ht, runwayColor(left))
}

// storeBar is the gauge for a stock with a ceiling: how full it is.
//
// This replaced a runway bar, which replaced a supply-against-demand bar, and
// the second change is the one that made the first unnecessary. A bar needs a
// maximum before "full" means anything — without one the only honest things to
// draw were a rate or a countdown, and both of those read as "how much have I
// got" to anyone who has ever seen a bar. Now every stock has a ceiling, so
// the bar can just be the obvious thing.
//
// left is the countdown from SecondsLeft, or negative when the stock is not
// falling. It only colours the bar: a store that is half full and emptying
// fast should not look like one that is half full and filling.
func (h *hud) storeBar(x, y, w, ht float32, stock, capacity float64, left float64) {
	h.quad(x, y, w, ht, colBarBack)
	if capacity <= 0 {
		return
	}

	frac := float32(min(stock/capacity, 1))

	col := colGood
	switch {
	case left >= 0 && left <= 90:
		col = colCritical
	case left >= 0 && left <= 300:
		col = colWarn
	case frac >= 0.999:
		// Full is worth its own colour: it is the one state where the thing
		// to do is not "make more".
		col = colAccent
	}

	if frac > 0 {
		h.quad(x, y, max32(frac, 0.015)*w, ht, col)
	}
}

// runwayColor matches the countdown's own thresholds, so the bar and the text
// under it cannot disagree about how bad it is.
func runwayColor(left float64) [3]float32 {
	switch {
	case left <= 90:
		return colCritical
	case left <= 300:
		return colWarn
	}
	return colGood
}
