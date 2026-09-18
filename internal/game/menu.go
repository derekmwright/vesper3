package game

import (
	"github.com/derekmwright/glyphengine/input"
)

// The front end: the menu before a game and the one that interrupts it.
//
// Both are the same widget. A menu is a title and a column of choices, driven
// by the keyboard and the mouse at the same time — arrows or W/S to move,
// Enter or Space to take, Escape to back out where backing out means anything,
// and the pointer doing whatever the pointer does. Neither input mode is the
// primary one; a player reaching for whichever is under their hand should not
// discover that this menu wanted the other.
//
// The widget holds no actions. choose returns the id of what was picked and
// the caller decides what that means, which keeps the thing testable without
// constructing a Game or an Engine to hang closures off.

// screen is which of the three things the frame is currently doing.
//
// It is a small state machine rather than a pile of booleans because the three
// are genuinely exclusive and the transitions are the feature: a paused game
// still draws its world behind the menu, and a main menu draws a world nobody
// is playing.
type screen uint8

const (
	// screenPlaying is the game. The world takes input.
	screenPlaying screen = iota

	// screenMenu is the main menu, drawn over a generated world that is
	// running but unplayed — the sun still moves, the sea still has waves in
	// it. A static backdrop would be cheaper and would look like a photograph
	// of the game rather than the game.
	screenMenu

	// screenPaused is the in-game menu. The world is still there and still
	// drawn; it simply stops advancing and stops listening.
	screenPaused
)

// menuAction identifies a choice. Values are compared, never stored, so they
// are free to be reordered.
type menuAction uint8

const (
	actionNone menuAction = iota
	actionNewGame
	actionLoadGame
	actionQuit
	actionResume
	actionSaveGame
	actionExitToMenu
)

// menuItem is one line of a menu.
type menuItem struct {
	Action menuAction
	Label  string

	// Note is the grey line under a label: what the choice does, or when
	// Enabled is false, why it cannot be taken. A disabled choice that simply
	// greys out leaves the player guessing; one that says "no save file yet"
	// has answered them.
	Note string

	Enabled bool
}

// menu is a title and a column of choices.
type menu struct {
	Title    string
	Subtitle string
	Items    []menuItem

	// hot is the highlighted row. The keyboard moves it, the pointer sets it,
	// and both draw the same highlight — so the two never disagree about what
	// Enter would take.
	hot int

	// held is the row the pointer went down on, or -1. A button shows its
	// pressed artwork only while the press that started on it is still on it,
	// so dragging off cancels — which is also when the release does nothing.
	held int
}

// Menu geometry, in design units.
const (
	menuW      = 340
	menuPad    = 22
	menuRowGap = 8
	menuTitleH = 34
	menuNoteDY = 26

	// A row is a button, so it inherits the artwork's minimum: below twice the
	// 24-unit nine-slice inset the corner regions overlap and the metal folds
	// in on itself. buttonMinH is the documented floor; this clears it.
	menuRowH = 58
)

// newMenu returns a menu with the first enabled item highlighted, because
// opening on a choice that cannot be taken invites a keypress that does
// nothing.
func newMenu(title, subtitle string, items []menuItem) *menu {
	m := &menu{Title: title, Subtitle: subtitle, Items: items, held: -1}
	m.hot = m.firstEnabled()
	return m
}

func (m *menu) firstEnabled() int {
	for i, it := range m.Items {
		if it.Enabled {
			return i
		}
	}
	return 0
}

// height is how tall the menu's panel is, in design units.
func (m *menu) height() float32 {
	rows := float32(len(m.Items))
	return menuPad + menuTitleH + textSub*lineBox + 10 +
		rows*menuRowH + (rows-1)*menuRowGap + menuPad
}

// rowRect is where one row sits, so drawing and hit-testing cannot disagree.
func (m *menu) rowRect(dw, dh float32, i int) rect {
	x := (dw - menuW) / 2
	y := (dh-m.height())/2 + menuPad + menuTitleH + textSub*lineBox + 10
	y += float32(i) * (menuRowH + menuRowGap)
	return rect{x + menuPad, y, menuW - 2*menuPad, menuRowH}
}

// move steps the highlight, skipping anything that cannot be chosen and
// wrapping at both ends. A menu whose first item is disabled should still let
// Up reach the last one.
func (m *menu) move(delta int) {
	n := len(m.Items)
	if n == 0 {
		return
	}
	for range n {
		m.hot = (m.hot + delta + n) % n
		if m.Items[m.hot].Enabled {
			return
		}
	}
}

// update runs one frame of the menu and reports what was chosen.
//
// It takes the pointer already converted to design units, so the widget never
// has to know about the interface scale or about where the real cursor is when
// a capture has pinned it somewhere else.
func (m *menu) update(in *input.Input, px, py, dw, dh float32) menuAction {
	switch {
	case in.KeyPressed(input.KeyUp), in.KeyPressed(input.KeyW):
		m.move(-1)
	case in.KeyPressed(input.KeyDown), in.KeyPressed(input.KeyS):
		m.move(+1)
	}

	// The pointer sets the highlight rather than keeping its own, so a player
	// who moves the mouse and then presses Enter gets what is under the mouse.
	for i := range m.Items {
		if m.rowRect(dw, dh, i).contains(px, py) && m.Items[i].Enabled {
			m.hot = i
		}
	}

	if in.KeyPressed(input.KeyEnter) || in.KeyPressed(input.KeySpace) {
		return m.take(m.hot)
	}

	// Press, then release on the same row. Acting on the press would be
	// simpler and would mean a menu that cannot be backed out of once the
	// button is down — the asset README asks for the opposite, and so does
	// every other button anyone has used.
	if in.MousePressed(input.MouseButtonLeft) {
		m.held = -1
		for i := range m.Items {
			if m.rowRect(dw, dh, i).contains(px, py) && m.Items[i].Enabled {
				m.held = i
			}
		}
	}
	if in.MouseReleased(input.MouseButtonLeft) {
		held := m.held
		m.held = -1
		if held >= 0 && m.rowRect(dw, dh, held).contains(px, py) {
			return m.take(held)
		}
	}
	return actionNone
}

func (m *menu) take(i int) menuAction {
	if i < 0 || i >= len(m.Items) || !m.Items[i].Enabled {
		return actionNone
	}
	return m.Items[i].Action
}

// draw paints the menu over whatever is already on screen.
func (m *menu) draw(h *hud, dw, dh float32) {
	// Dim the world behind it. The same veil the demolition prompt uses, for
	// the same reason: the menu has to be obviously the only thing listening.
	h.quad(0, 0, dw, dh, colModalVeil)

	height := m.height()
	x := (dw - menuW) / 2
	y := (dh - height) / 2
	h.panel(x, y, menuW, height)

	h.centred(dw/2, y+menuPad, textHead, colAccent, "%s", m.Title)
	if m.Subtitle != "" {
		h.centred(dw/2, y+menuPad+menuTitleH, textSub, colDim, "%s", m.Subtitle)
	}

	for i, it := range m.Items {
		r := m.rowRect(dw, dh, i)

		state := buttonStateFor(it.Enabled, i == m.hot, m.held == i)
		h.button(state, r.X, r.Y, r.W, r.H)

		// The pressed art reverses its bevel, so its label drops with it. The
		// hit box does not move: rowRect is the same either way.
		drop := float32(0)
		if state == buttonPressed {
			drop = buttonLabelDrop
		}

		col := buttonLabelColor[state]
		h.centred(r.X+r.W/2, r.Y+13+drop, textMain, col, "%s", it.Label)
		if it.Note != "" {
			h.centred(r.X+r.W/2, r.Y+menuNoteDY+9+drop, textSub, col, "%s", it.Note)
		}
	}
}

// ---------------------------------------------------------------- the menus

// mainMenu is what the game opens on.
func (g *Game) mainMenu() *menu {
	return newMenu("VESPER III", "a colony on a planet that is not Earth", []menuItem{
		{Action: actionNewGame, Label: "New Colony", Enabled: true,
			Note: "a new planet, from a new seed"},
		g.loadItem(),
		{Action: actionQuit, Label: "Exit", Enabled: true},
	})
}

// pauseMenu is what Escape opens during a game.
func (g *Game) pauseMenu() *menu {
	return newMenu("PAUSED", "", []menuItem{
		{Action: actionResume, Label: "Resume", Enabled: true, Note: "or press Escape"},
		{Action: actionSaveGame, Label: "Save Colony", Enabled: true, Note: g.cfg.SavePath},
		g.loadItem(),
		{Action: actionExitToMenu, Label: "Exit to Menu", Enabled: true,
			Note: "unsaved progress is lost"},
	})
}

// loadItem is shared by both menus, and is the reason menuItem has a Note:
// "Load Colony" greyed out with nothing beside it reads as a bug, and the one
// thing the player needs to know is that there is no save yet.
func (g *Game) loadItem() menuItem {
	if !g.haveSave() {
		return menuItem{Action: actionLoadGame, Label: "Load Colony",
			Note: "no save file yet", Enabled: false}
	}
	return menuItem{Action: actionLoadGame, Label: "Load Colony",
		Note: g.cfg.SavePath, Enabled: true}
}
