package game

import (
	"fmt"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"
	"github.com/derekmwright/glyphengine/renderer"
)

// The title card's timing, in seconds.
//
// It is a title card rather than a loading screen, and the distinction is
// worth being straight about: Game.Init loads the whole world synchronously
// before the first frame is drawn, so by the time anything can be shown there
// is nothing left to wait for. A progress bar here would be a decoration
// counting to a number that is already reached. What it is for is the moment
// between launching and playing — and it is skippable, because the second time
// you see it you do not want it.
const (
	splashHold = 2.4
	splashFade = 0.7
)

const logoPath = "assets/logo.png"

// splash is the title card's state: how long it has been up, whether it has
// finished, and whether it is owning the current frame.
//
// up is separate from done because the card swallows input while it is
// showing, and Update needs to know that before it has decided whether this is
// the frame that ends it.
type splash struct {
	t    float32
	done bool
	up   bool
}

// initSplash loads the title art. A missing logo is not an error: the card
// falls back to type, which is most of what it is anyway.
func (g *Game) initSplash(e *glyph.Engine) error {
	if g.cfg.Assets == nil || g.cfg.NoSplash {
		g.splash.done = true
		return nil
	}

	r := e.Renderer()
	tex, err := r.LoadTexture(g.cfg.Assets, logoPath)
	if err != nil {
		// Type-only card. Worth a line in the log, not worth refusing to run.
		fmt.Printf("splash logo unavailable (%v); showing the title in type only\n", err)
	} else {
		g.hud.logo = tex
		mesh, err := r.CreateDynamicIndexedMesh(8, 12)
		if err != nil {
			return fmt.Errorf("splash mesh: %w", err)
		}
		g.hud.logoMesh = mesh
	}
	return nil
}

// splashActive reports whether the title card is still up, and advances its
// clock. It also consumes the input that dismisses it, so the click that
// skips the card does not also land on the world behind it.
func (g *Game) splashActive(e *glyph.Engine, dt float32) bool {
	if g.splash.done {
		return false
	}

	g.splash.t += dt

	in := e.Input()
	skipped := in.MousePressed(input.MouseButtonLeft) ||
		in.MousePressed(input.MouseButtonRight) ||
		in.KeyPressed(input.KeySpace) ||
		in.KeyPressed(input.KeyEnter)

	if skipped && g.splash.t > 0.25 {
		// The quarter-second grace stops a click that was in flight at launch
		// from dismissing a card nobody saw.
		g.splash.done = true
		return false
	}
	if g.splash.t >= splashHold+splashFade {
		g.splash.done = true
		return false
	}
	return true
}

// splashAlpha is the card's opacity for the current moment.
func (g *Game) splashAlpha() float32 {
	if g.splash.t <= splashHold {
		return 1
	}
	return max32(0, 1-(g.splash.t-splashHold)/splashFade)
}

// drawSplash builds the title card.
func (g *Game) drawSplash(h *hud, dw, dh float32) {
	// A solid ground so the world does not read through the card.
	h.quad(0, 0, dw, dh, colSplashBack)

	cx := dw / 2

	// The logo sits above the type, sized against the shorter dimension so a
	// wide window does not blow it up past the screen.
	logoSize := min32(dw, dh) * 0.34
	logoY := dh*0.5 - logoSize - 18
	if h.logo != nil {
		h.logoQuad(cx-logoSize/2, logoY, logoSize)
	}

	titleY := dh*0.5 - 6
	h.centred(cx, titleY, 46, colInk, "VESPER III")
	h.centred(cx, titleY+52, 15, colAccent, "A COLONY ON A HEXAGONAL WORLD")

	h.centred(cx, dh*0.5+108, 13, colDim, "seed %d   %dx%d tiles", g.Map.Seed, g.Map.Cols, g.Map.Rows)

	// Only offer the prompt once the grace period has passed, so it never
	// invites a click that will be ignored.
	if g.splash.t > 0.25 && g.splash.t <= splashHold {
		h.centred(cx, dh-72, 13, colDim, "click or press space to begin")
	}
}

// centred queues a line of text centred on x.
func (h *hud) centred(cx, y, size float32, col [3]float32, format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	h.label(cx-h.measure(s, size)/2, y, size, col, "%s", s)
}

// logoQuad appends the splash logo, which samples its own texture and so
// needs its own mesh and its own draw.
func (h *hud) logoQuad(x, y, size float32) {
	k := h.scale
	x, y, size = x*k, y*k, size*k

	col := [3]float32{1, 1, 1}
	h.logoVerts = append(h.logoVerts[:0],
		renderer.Vertex{Pos: [3]float32{x, y, 0}, Color: col, UV: [2]float32{0, 0}},
		renderer.Vertex{Pos: [3]float32{x + size, y, 0}, Color: col, UV: [2]float32{1, 0}},
		renderer.Vertex{Pos: [3]float32{x + size, y + size, 0}, Color: col, UV: [2]float32{1, 1}},
		renderer.Vertex{Pos: [3]float32{x, y + size, 0}, Color: col, UV: [2]float32{0, 1}},
	)
	h.logoIdx = append(h.logoIdx[:0], 0, 1, 2, 2, 3, 0)
}

// colSplashBack is the card's ground, written in display space like the rest
// of the palette. See srgb.
var colSplashBack = srgb(0.030, 0.035, 0.050)
