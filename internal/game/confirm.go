package game

import (
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
)

// confirmPrompt is a question waiting for an answer.
//
// Demolition is the one action in the game that cannot be undone and that a
// player can trigger by accident — the same button orbits the camera, and a
// misjudged drag lands on a building. Everything else is either reversible or
// costs only ore.
type confirmPrompt struct {
	At   hex.Axial
	Kind colony.Kind

	// Refund is spelled out rather than recomputed at draw time, so the
	// number in the question is provably the number the player gets.
	RefundIron    float64
	RefundCrystal float64
}

// Modal geometry, in design units.
const (
	modalW    = 330
	modalPad  = 16
	modalBtnH = 30
	modalBtnW = 132
)

// askDemolish opens the prompt for a tile, if there is anything on it.
func (g *Game) askDemolish(a hex.Axial) {
	b, ok := g.Colony.At(a)
	if !ok {
		g.refuse("Nothing to demolish there")
		return
	}
	iron, crystal := colony.Of(b.Kind).Refund()
	g.ui.confirm = &confirmPrompt{
		At:            a,
		Kind:          b.Kind,
		RefundIron:    iron,
		RefundCrystal: crystal,
	}
}

// handleConfirm answers the prompt. It returns true while one is open, so the
// caller can stop the world from seeing this frame's input at all.
func (g *Game) handleConfirm(e *glyph.Engine, dw, dh float32) bool {
	if g.ui.confirm == nil {
		return false
	}
	in := e.Input()

	// Keyboard first: Enter or Y commits, Escape or N backs out. A modal that
	// can only be dismissed with the mouse is a modal that traps anyone whose
	// hand is on the keyboard.
	switch {
	case in.KeyPressed(input.KeyEnter), in.KeyPressed(input.KeyY):
		g.commitDemolish(e)
		return true
	case in.KeyPressed(input.KeyEscape), in.KeyPressed(input.KeyN):
		g.ui.confirm = nil
		emit(g, Noticed{Text: "Left standing"})
		return true
	}

	if in.MousePressed(input.MouseButtonLeft) {
		mx, my := g.pointer(e)
		x, y := float32(mx)/g.hud.scale, float32(my)/g.hud.scale

		yes, no := modalButtons(dw, dh)
		switch {
		case yes.contains(x, y):
			g.commitDemolish(e)
		case no.contains(x, y):
			g.ui.confirm = nil
			emit(g, Noticed{Text: "Left standing"})
		}
	}
	return true
}

func (g *Game) commitDemolish(e *glyph.Engine) {
	if g.ui.confirm == nil {
		return
	}
	at := g.ui.confirm.At
	g.ui.confirm = nil
	g.demolish(e, at)
}

// rect is a design-space rectangle with a hit test.
type rect struct{ X, Y, W, H float32 }

func (r rect) contains(x, y float32) bool {
	return x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}

// modalButtons returns where the two buttons sit, so drawing and hit-testing
// cannot disagree about it.
func modalButtons(dw, dh float32) (yes, no rect) {
	h := modalHeight()
	x := (dw - modalW) / 2
	y := (dh - h) / 2

	by := y + h - modalPad - modalBtnH
	gap := float32(modalW - 2*modalBtnW - 2*modalPad)
	return rect{x + modalPad, by, modalBtnW, modalBtnH},
		rect{x + modalPad + modalBtnW + gap, by, modalBtnW, modalBtnH}
}

func modalHeight() float32 {
	return modalPad + textHead*lineBox + 8 + 2*(textMain*lineBox+4) + 14 + modalBtnH + modalPad
}

// drawConfirm draws the prompt over everything else.
func (g *Game) drawConfirm(h *hud, dw, dh float32) {
	if g.ui.confirm == nil {
		return
	}
	c := g.ui.confirm
	spec := colony.Of(c.Kind)

	// Dim the whole frame so the question is obviously the only thing
	// accepting input.
	h.quad(0, 0, dw, dh, colModalVeil)

	mh := modalHeight()
	x := (dw - modalW) / 2
	y := (dh - mh) / 2
	h.panel(x, y, modalW, mh)

	ty := y + modalPad
	h.clipped(x+modalPad, ty, textHead, modalW-2*modalPad, colInk, "Demolish %s?", spec.Name)
	ty += textHead*lineBox + 8

	h.clipped(x+modalPad, ty, textMain, modalW-2*modalPad, colGood,
		"%s reclaimed", colony.Materials(c.RefundIron, c.RefundCrystal))
	ty += textMain*lineBox + 4
	h.clipped(x+modalPad, ty, textMain, modalW-2*modalPad, colDim,
		"of the %s it cost", spec.CostText())

	yes, no := modalButtons(dw, dh)

	// The key goes in the label rather than beside it: a 30-unit button has
	// room for one line of text, and a second one drawn under it landed on top
	// of the first.
	g.drawModalButton(h, yes, colCritical, "Demolish  (Enter)")
	g.drawModalButton(h, no, colSlotPick, "Keep  (Esc)")
}

func (g *Game) drawModalButton(h *hud, r rect, fill [3]float32, label string) {
	h.quad(r.X, r.Y, r.W, r.H, fill)
	h.quad(r.X, r.Y, r.W, 2, colPanelEdge)
	h.centred(r.X+r.W/2, r.Y+(r.H-textMain*lineBox)/2, textMain, colInk, "%s", label)
}

var colModalVeil = [3]float32{0.02, 0.025, 0.04}
