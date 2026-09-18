package game

import (
	"fmt"
	"log"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
)

// The button artwork: four nine-slice textures, one per state.
//
// Everything here follows the contract the assets shipped with, in
// assets/ui/buttons/README.md and buttons.json, rather than being inferred
// from the images. Where a number below looks arbitrary it came from there.
//
// Two of those numbers matter more than the rest:
//
//   - The inset is 24 pixels on all four sides of a 96-pixel-tall source, so a
//     button drawn shorter than 48 units has its top and bottom corner regions
//     overlapping and the artwork folds in on itself. The documented minimum
//     is 56, which is what buttonMinH holds and what a test checks every
//     button in the game against.
//
//   - They are drawn in *texture* mode with a white tint, which the asset
//     README asks for in as many words: "Draw as ordinary textured UI with
//     white tint to retain the artwork, rather than a panel shader that
//     replaces the center fill." Panel mode would repaint the authored face
//     with a flat wash, which is exactly the fight documented over the bezel
//     in hud.go.
type buttonState uint8

const (
	buttonNormal buttonState = iota
	buttonHover
	buttonPressed
	buttonDisabled

	buttonStateCount
)

// Geometry and files, from buttons.json.
const (
	buttonTexW  = 256
	buttonTexH  = 96
	buttonInset = 24

	// buttonMinH is the shortest a button may be drawn. Below twice the inset
	// the corners overlap; the asset README recommends 56 and that is what is
	// enforced, not the 48 where it merely starts to fail.
	buttonMinH = 56
	buttonMinW = 72

	// One texel to one design unit, so the corners are the size they were
	// authored at.
	buttonTexelScale = 1

	// Nine quads a button, and a menu is a handful of them.
	hudMaxButtonQuads = 72
)

func buttonFile(s buttonState) string {
	switch s {
	case buttonHover:
		return "assets/ui/buttons/button-hover.png"
	case buttonPressed:
		return "assets/ui/buttons/button-pressed.png"
	case buttonDisabled:
		return "assets/ui/buttons/button-disabled.png"
	}
	return "assets/ui/buttons/button-normal.png"
}

// Label colours, from buttons.json. The pressed label also drops two units,
// which is buttonLabelDrop below — the art reverses its bevel, and a label
// that stayed put would float off the face it is printed on.
var buttonLabelColor = [buttonStateCount][3]float32{
	buttonNormal:   {0.839, 0.898, 0.929}, // #d6e5ed
	buttonHover:    {0.945, 0.984, 1.000}, // #f1fbff
	buttonPressed:  {0.800, 0.871, 0.910}, // #ccdee8
	buttonDisabled: {0.443, 0.514, 0.561}, // #71838f
}

const buttonLabelDrop = 2

// buttonLayer is one state's geometry for the frame: its own texture means its
// own draw, so each state accumulates separately and only the ones actually
// used are submitted.
type buttonLayer struct {
	slice *renderer.NineSlice
	mesh  *renderer.Mesh
	verts []renderer.Vertex
	idx   []uint16
}

// initButtons loads the four states. Optional, like every other piece of art
// here: without them buttons fall back to the flat rectangles the layout was
// built against, and the game is entirely playable.
func (g *Game) initButtons(e *glyph.Engine) error {
	if g.cfg.Assets == nil {
		return nil
	}
	r := e.Renderer()

	for s := buttonState(0); s < buttonStateCount; s++ {
		tex, err := r.LoadTexture(g.cfg.Assets, buttonFile(s))
		if err != nil {
			log.Printf("button art unavailable (%v); drawing flat buttons", err)
			g.hud.buttons = [buttonStateCount]*buttonLayer{}
			return nil
		}

		mesh, err := r.CreateDynamicIndexedMesh(hudMaxButtonQuads*4, hudMaxButtonQuads*6)
		if err != nil {
			return fmt.Errorf("button mesh: %w", err)
		}

		slice := renderer.NewNineSlice(tex, buttonTexW, buttonInset)
		slice.TexH = buttonTexH

		g.hud.buttons[s] = &buttonLayer{slice: slice, mesh: mesh}
	}
	return nil
}

// button draws one, in design units.
//
// Unlike hud.panel this keeps all nine quads. The bezel drops its centre
// because panel mode would wash it; a button's centre is the authored metal
// face and is the whole point of the artwork.
func (h *hud) button(s buttonState, x, y, w, ht float32) {
	layer := h.buttons[s]
	if layer == nil {
		// No art: a filled rectangle in the state's rough colour, so the
		// layout still reads.
		fill := colSlot
		switch s {
		case buttonHover:
			fill = colSlotHover
		case buttonPressed:
			fill = colSlotPick
		}
		h.quad(x, y, w, ht, fill)
		h.quad(x, y, w, 2, colPanelEdge)
		return
	}

	k := h.scale
	verts, _ := layer.slice.GenerateQuads(
		x*k, y*k, w*k, ht*k,
		buttonTexelScale*k,
		[3]float32{1, 1, 1}, // white: the artwork carries its own colour
	)

	for q := 0; q+4 <= len(verts); q += 4 {
		base := uint16(len(layer.verts))
		layer.verts = append(layer.verts, verts[q:q+4]...)
		layer.idx = append(layer.idx, base, base+1, base+2, base+2, base+3, base)
	}
}

// buttonStateFor resolves the four states in the priority the asset contract
// sets out: disabled beats pressed beats hover beats normal.
//
// held is the pointer being down on this button since it went down on it —
// not merely down while over it, which would light up a button the player
// pressed somewhere else and dragged onto.
func buttonStateFor(enabled, hot, held bool) buttonState {
	switch {
	case !enabled:
		return buttonDisabled
	case held:
		return buttonPressed
	case hot:
		return buttonHover
	}
	return buttonNormal
}
