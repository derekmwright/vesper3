package game

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/renderer/lightcluster"
	"github.com/derekmwright/glyphengine/ui"
)

// The per-frame interface pass: what gets drawn, in what order, and what is
// dropped when the window is too small for all of it.
//
// Everything here reads game state and emits geometry. Nothing here decides
// anything — the layout choices live in drawColumn, and the widgets they are
// made of live in hud.go.

// drawHUD rebuilds the whole interface for this frame.
//
// Immediate mode: every quad and every string is rebuilt from the colony's
// current state, so there is no retained widget tree that can disagree with
// the simulation. The cost is a few hundred vertices of upload per frame.
func (g *Game) drawHUD(e *glyph.Engine) {
	if g.hud == nil {
		return
	}
	h := g.hud
	h.verts, h.idx, h.lines = h.verts[:0], h.idx[:0], h.lines[:0]
	h.iconVerts, h.iconIdx = h.iconVerts[:0], h.iconIdx[:0]
	h.frameVerts, h.frameIdx = h.frameVerts[:0], h.frameIdx[:0]
	h.debugVerts, h.debugIdx = h.debugVerts[:0], h.debugIdx[:0]
	for _, layer := range h.buttons {
		if layer != nil {
			layer.verts, layer.idx = layer.verts[:0], layer.idx[:0]
		}
	}

	w, ph := e.Renderer().Extent()
	sw, sh := float32(w), float32(ph)

	h.scale = g.uiScale(sh)

	// Design-space dimensions: the layout below is written as if the window
	// were this size, and h.scale turns it into pixels on the way out.
	dw, dh := sw/h.scale, sh/h.scale

	switch {
	case g.splash.up:
		g.drawSplash(h, dw, dh)

	case g.screen == screenMenu:
		// Nothing. The main menu's world is a backdrop, not a colony: a
		// resource panel reporting 0/0 colonists and an advisory telling
		// nobody to build a habitat would be furniture from a game that is
		// not being played.
		//
		// The pause menu keeps its panel, because there the numbers are real
		// and checking them is half the reason to pause.

	default:
		// The readout and the resource column both live in the top-left, and
		// they cannot share it. A backdrop is not enough on its own: this
		// game's own text draws in a later channel than any UI overlay, so a
		// sheet laid over the panel covers its background and none of its
		// words. The panel steps aside instead - every figure on it is in the
		// readout anyway, in more decimal places.
		if !g.ui.showDebug {
			g.drawColumn(h, dw, dh)
		}
		g.drawHotbar(h, dw, dh)
		g.drawTooltip(h, dw, dh)
		g.drawConfirm(h, dw, dh)
	}

	// The menu goes over everything, including the prompt: while it is open it
	// is the only thing listening, so it had better be the only thing that
	// looks like it is.
	//
	// Except the title card, which outranks it. The game opens on screenMenu
	// with the main menu already built, so for the length of the splash this
	// drew New Colony, Load Colony and Exit straight over the logo - a menu
	// that was not listening yet, on top of a card that was.
	if g.menuVisible() {
		g.menu.draw(h, dw, dh)
	}

	if g.ui.showDebug {
		g.drawDebugLines(e, h)
	}

	if len(h.verts) > hudMaxQuads*4 && !h.overflowed {
		// Past this the engine truncates the upload without complaint, so say
		// so once rather than letting widgets vanish silently.
		h.overflowed = true
		emit(g, Noticed{Text: fmt.Sprintf("HUD exceeded %d quads and is being truncated", hudMaxQuads)})
	}

	proj := mgl32.Ortho(0, sw, 0, sh, -1, 1)

	// The card fades out as one piece, so its opacity multiplies every layer.
	alpha := float32(1)
	panelAlpha := float32(panelOpacity)
	if g.splash.up {
		alpha = g.splashAlpha()
		panelAlpha = alpha
	}

	// Order is draw order. The flat mesh goes first because it carries the
	// solid backing each panel sits on; the bezel then frames it, and the
	// textured layers go on top of both.
	var overlays []renderer.UIRenderObject

	e.Renderer().UpdateMeshData(h.mesh, h.verts, h.idx)
	overlays = append(overlays, renderer.UIRenderObject{
		RenderObject: renderer.RenderObject{Mesh: h.mesh, MVP: proj},
		Opacity:      panelAlpha,
	})

	// Icons are a second object so they can carry the atlas and the texture
	// mode the flat geometry must not have. Appended after the panels, because
	// the list is drawn in order and an icon under its own slot is invisible.
	if h.iconMesh != nil && len(h.iconIdx) > 0 {
		e.Renderer().UpdateMeshData(h.iconMesh, h.iconVerts, h.iconIdx)
		overlays = append(overlays, renderer.UIRenderObject{
			RenderObject: renderer.RenderObject{
				Mesh:    h.iconMesh,
				MVP:     proj,
				Texture: h.icons,
			},
			Opacity:     alpha,
			TextureMode: true,
		})
	}

	// One draw per button state actually used this frame. Four textures cannot
	// share a draw, but a menu only ever shows two or three states at once and
	// an unused state submits nothing.
	for _, layer := range h.buttons {
		if layer == nil || len(layer.idx) == 0 {
			continue
		}
		e.Renderer().UpdateMeshData(layer.mesh, layer.verts, layer.idx)
		overlays = append(overlays, renderer.UIRenderObject{
			RenderObject: renderer.RenderObject{
				Mesh:    layer.mesh,
				MVP:     proj,
				Texture: layer.slice.Texture,
			},
			Opacity: alpha,

			// Texture mode with a white tint, as the asset README asks: the
			// artwork carries its own colour and panel mode would repaint the
			// face it was drawn with.
			TextureMode: true,
		})
	}

	if h.frameMesh != nil && len(h.frameIdx) > 0 {
		e.Renderer().UpdateMeshData(h.frameMesh, h.frameVerts, h.frameIdx)
		overlays = append(overlays, renderer.UIRenderObject{
			RenderObject: renderer.RenderObject{
				Mesh:    h.frameMesh,
				MVP:     proj,
				Texture: h.frame.Texture,
			},
			Opacity: panelAlpha,

			// Straight texture mode, NOT panel mode, even though this is a
			// nine-slice.
			//
			// Panel mode paints every transparent texel as tint*0.2 at 70%
			// alpha. A frame quad is mostly transparent — the artwork band is
			// about a third of the corner region — so that wash came out as a
			// light grey ring all the way around the inside of every bezel,
			// between the metal and the interior. Texture mode multiplies by
			// the texel instead, so transparent texels contribute nothing and
			// the only thing drawn is the frame itself.
			TextureMode: true,
		})
	}

	if h.logoMesh != nil && len(h.logoIdx) > 0 {
		e.Renderer().UpdateMeshData(h.logoMesh, h.logoVerts, h.logoIdx)
		overlays = append(overlays, renderer.UIRenderObject{
			RenderObject: renderer.RenderObject{
				Mesh:    h.logoMesh,
				MVP:     proj,
				Texture: h.logo,
			},
			Opacity:     alpha,
			TextureMode: true,
		})
		h.logoVerts, h.logoIdx = h.logoVerts[:0], h.logoIdx[:0]
	}

	// Last, and so on top of everything else the interface drew. The engine's
	// debug text draws in a channel of its own above all overlays, so this
	// lands between the two: over the panel it used to be illegible against,
	// under the numbers it exists to make readable.
	if h.debugMesh != nil && len(h.debugIdx) > 0 {
		e.Renderer().UpdateMeshData(h.debugMesh, h.debugVerts, h.debugIdx)
		overlays = append(overlays, renderer.UIRenderObject{
			RenderObject: renderer.RenderObject{Mesh: h.debugMesh, MVP: proj},
			Opacity:      debugBackdropOpacity,
		})
	}

	e.SetUIOverlays(overlays)

	h.text.SetText(e.Renderer(), h.lines, sw, sh)
	e.SetMSDFOverlays([]renderer.RenderObject{h.text.RenderObject(sw, sh, 48)})
}

// drawColumn stacks the readout, the advisories and the tile inspector down
// the left edge, dropping what will not fit.
//
// They were three separately anchored panels — two to the top, one to the
// bottom — which is fine at 1x and overlaps at 2x, because raising the scale
// shrinks the design-space window that all three are being placed in. Stacking
// them means the only thing that can run short is the bottom of the list, and
// the checks below decide what gets dropped rather than letting panels draw
// over each other.
func (g *Game) drawColumn(h *hud, dw, dh float32) {
	// Everything above the hotbar and its mode label is fair game.
	bottom := dh - hotbarHeight(dw) - hotbarDY - textSub*lineBox - 14

	// Compact when the full panel would leave no room for the advisories,
	// which are the thing worth keeping.
	compact := panelY+statusH+10+alertRow+12 > bottom

	y := g.drawStatusPanel(h, panelY, compact)

	// Advisories come before the inspector in the queue for what is left.
	// They are the only thing on screen that says what to do about a problem;
	// the inspector describes a tile the cursor is already sitting on, and the
	// tooltip and the cursor colour cover most of what it says.
	rows := int((bottom - (y + 10)) / alertRow)
	if y = g.drawAlerts(h, y+10, min(rows, 3)); y > bottom {
		return
	}

	if y+10+inspectorH <= bottom {
		g.drawInspector(h, y+10)
	}
}

// drawDebugLines is the raw readout, behind F3, for numbers the panel
// deliberately does not show.
// Debug readout geometry, in the pixels the engine lays its debug text out in
// (see Engine.Debugf: x 18, y 18 + line*24, scale 20).
//
// They are pixels rather than design units because the engine's text is: it is
// drawn in a channel of its own that never sees this game's interface scale,
// so a backdrop measured in design units would drift away from the text it is
// meant to sit behind the moment anyone pressed [ or ].
const (
	debugTextX    = 18.0
	debugTextY    = 18.0
	debugLineH    = 24.0
	debugTextSize = 20.0

	// Go Mono's advance is 1229/2048 of the em. The engine picked a monospaced
	// font so a changing value cannot shift the line after it, and the same
	// property is what makes a backdrop measurable from a character count.
	debugAdvance = debugTextSize * 1229 / 2048

	debugPad = 8.0

	// 0.86 is what every other panel uses, and the readout is a panel.
	debugBackdropOpacity = panelOpacity
)

// debugBackdropColor is near-black rather than the panel's blue-grey: this sits
// over the interface rather than over the world, and a tinted sheet on top of
// a tinted panel reads as a rendering mistake.
var debugBackdropColor = [3]float32{0.01, 0.012, 0.016}

func (g *Game) drawDebugLines(e *glyph.Engine, h *hud) {
	r := g.Colony.Readout

	// Collected rather than emitted one at a time, because the backdrop has to
	// be sized from the longest line and drawn before any of them.
	lines := []string{
		fmt.Sprintf("seed %d   tiles %dx%d   fps %.0f", g.Map.Seed, g.Map.Cols, g.Map.Rows, e.FPS()),
		fmt.Sprintf("daylight %.3f   satisfaction %.3f   lamps %.3f", r.Daylight, r.Satisfaction, g.lights.level),
		fmt.Sprintf("iron   %8.3f  make %.4f   crystal %8.3f  make %.4f",
			g.Colony.Iron, r.Iron.Produced, g.Colony.Crystal, r.Crystal.Produced),
		fmt.Sprintf("water  %8.3f  make %.4f  use %.4f  coolant %.3f",
			g.Colony.Water, r.Water.Produced, r.Water.Consumed, r.Coolant),
		fmt.Sprintf("food   %8.3f  make %.4f  use %.4f", g.Colony.Food, r.Food.Produced, r.Food.Consumed),
		fmt.Sprintf("cam    dist %.1f pitch %.2f yaw %.2f", g.cam.Distance, g.cam.Pitch, g.cam.Yaw),
	}

	// What the light binner did with what this game handed it. Worth a line
	// because every number in it is one this game can move: it submits the
	// lights, and their range and position decide how many cells each one
	// lands in.
	//
	// ScreenWideLights is the one to watch. Those are the lights every
	// fragment in a slice evaluates whatever the clustering did, so they are
	// the part of the old every-light loop that survived - and lamp range is
	// what makes a light screen-wide.
	ls := e.LightStats()
	lines = append(lines,
		fmt.Sprintf("lights sent %d  drawn %d  culled %d  dropped %d  of %d",
			ls.Submitted, ls.Uploaded, ls.Culled, ls.DroppedOverBudget, renderer.MaxLights),
		fmt.Sprintf("       screen-wide %d  unbounded %d  worst cell %d/%d  overflowed %d",
			ls.ScreenWideLights, ls.UnboundedLights,
			ls.MaxCellDemand, lightcluster.MaxLightsPerCell, ls.CellsOverflowed))

	lines = append(lines, g.runtimeLines(e)...)

	if g.intent.hovering {
		lines = append(lines, fmt.Sprintf("hover  %v", g.intent.hover))
	}
	if g.ui.status != "" && g.elapsed < g.ui.statusUntil {
		lines = append(lines, fmt.Sprintf("> %s", g.ui.status))
	}

	g.drawDebugBackdrop(h, lines)
	for _, l := range lines {
		e.Debugf("%s", l)
	}
}

// drawDebugBackdrop lays a dark sheet under the readout so it can be read over
// the resource panel it overlaps.
//
// The two have always collided - both start in the top-left corner - and white
// monospace over a lit panel was legible only by luck of what was behind it.
func (g *Game) drawDebugBackdrop(h *hud, lines []string) {
	if h.debugMesh == nil || len(lines) == 0 {
		return
	}

	widest := 0
	for _, l := range lines {
		if len(l) > widest {
			widest = len(l)
		}
	}

	// Pixels to design units: quad multiplies by h.scale on the way out, and
	// the numbers above are already in pixels.
	k := h.scale
	if k <= 0 {
		return
	}
	x := (debugTextX - debugPad) / k
	y := (debugTextY - debugPad) / k
	w := (float32(widest)*debugAdvance + 2*debugPad) / k
	ht := (float32(len(lines))*debugLineH + 2*debugPad) / k

	h.debugVerts, h.debugIdx = ui.AppendQuad(h.debugVerts, h.debugIdx,
		x*k, y*k, w*k, ht*k, debugBackdropColor)
}

func dayPhase(daylight float64) string {
	switch {
	case daylight <= 0:
		return "night"
	case daylight < 0.35:
		return "twilight"
	case daylight < 0.8:
		return "daylight"
	default:
		return "high sun"
	}
}
