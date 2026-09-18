package game

import (
	"os"
	"testing"
)

// The menu widget is driven by the keyboard and the mouse at once, and the two
// share one highlight. These test the widget through its geometry and its
// selection rules rather than through an Engine, which is the reason update
// takes a pointer position already converted to design units.

func testMenu(enabled ...bool) *menu {
	items := make([]menuItem, len(enabled))
	for i, en := range enabled {
		items[i] = menuItem{Action: menuAction(i + 1), Label: "item", Enabled: en}
	}
	return newMenu("T", "", items)
}

// A menu opens on something that can actually be taken. Opening on a disabled
// row invites a keypress that does nothing and reads as a broken menu.
func TestMenuOpensOnAnEnabledItem(t *testing.T) {
	m := testMenu(false, false, true)
	if m.hot != 2 {
		t.Errorf("opened on row %d, want 2 — the first two cannot be chosen", m.hot)
	}
}

// Moving skips what cannot be chosen, in both directions.
func TestMenuMoveSkipsDisabledItems(t *testing.T) {
	m := testMenu(true, false, true)

	m.move(+1)
	if m.hot != 2 {
		t.Errorf("down from 0 landed on %d, want 2 (1 is disabled)", m.hot)
	}
	m.move(-1)
	if m.hot != 0 {
		t.Errorf("up from 2 landed on %d, want 0", m.hot)
	}
}

// And wraps at both ends, so a menu is a loop rather than a dead end.
func TestMenuMoveWraps(t *testing.T) {
	m := testMenu(true, true)

	m.move(-1)
	if m.hot != 1 {
		t.Errorf("up from the first row landed on %d, want the last", m.hot)
	}
	m.move(+1)
	if m.hot != 0 {
		t.Errorf("down from the last row landed on %d, want the first", m.hot)
	}
}

// A menu where nothing can be chosen must not spin forever looking for
// something that is not there.
func TestMenuMoveTerminatesWithNothingEnabled(t *testing.T) {
	m := testMenu(false, false)
	m.move(+1) // must return
	m.move(-1)
}

// Taking a disabled row yields nothing, whether it is reached by pointer or by
// a highlight that should never have been on it.
func TestMenuWillNotTakeADisabledItem(t *testing.T) {
	m := testMenu(true, false)
	if got := m.take(1); got != actionNone {
		t.Errorf("took a disabled row and got action %d", got)
	}
	if got := m.take(7); got != actionNone {
		t.Errorf("took a row that does not exist and got action %d", got)
	}
}

// Rows do not overlap and sit inside the panel, because the same rowRect is
// used to draw them and to decide what the pointer is over. If those disagreed
// a player would click one row and get another.
func TestMenuRowsTileThePanelWithoutOverlapping(t *testing.T) {
	m := testMenu(true, true, true, true)
	const dw, dh = 800, 600

	panelTop := (dh - m.height()) / 2
	panelBottom := panelTop + m.height()

	var prev rect
	for i := range m.Items {
		r := m.rowRect(dw, dh, i)

		if r.Y < panelTop || r.Y+r.H > panelBottom {
			t.Errorf("row %d spans %.1f-%.1f, outside the panel %.1f-%.1f",
				i, r.Y, r.Y+r.H, panelTop, panelBottom)
		}

		// And clear of the bezel, horizontally. The panel's corner artwork
		// occupies frameInset texels at frameTexelScale, so a row that started
		// inside that would have its own clipped corners drawn over the
		// panel's.
		bezel := float32(frameInset) * frameTexelScale
		panelLeft := float32(dw-menuW) / 2
		if r.X < panelLeft+bezel {
			t.Errorf("row %d starts at %.1f, inside the %.1f-unit bezel at %.1f",
				i, r.X, bezel, panelLeft)
		}
		if r.X+r.W > panelLeft+menuW-bezel {
			t.Errorf("row %d ends at %.1f, inside the bezel on the right",
				i, r.X+r.W)
		}
		if i > 0 && r.Y < prev.Y+prev.H {
			t.Errorf("row %d starts at %.1f, inside row %d which ends at %.1f",
				i, r.Y, i-1, prev.Y+prev.H)
		}
		prev = r
	}
}

// The pointer hits the row it is drawn over, at every row.
func TestPointerHitsTheRowItIsOver(t *testing.T) {
	m := testMenu(true, true, true)
	const dw, dh = 800, 600

	for i := range m.Items {
		r := m.rowRect(dw, dh, i)
		cx, cy := r.X+r.W/2, r.Y+r.H/2

		hit := -1
		for j := range m.Items {
			if m.rowRect(dw, dh, j).contains(cx, cy) {
				hit = j
			}
		}
		if hit != i {
			t.Errorf("the centre of row %d hit row %d", i, hit)
		}
	}
}

// Both menus offer Load only when there is something to load, and say why when
// there is not — a greyed row with no explanation reads as a bug.
func TestLoadIsOfferedOnlyWithASaveAndExplainsWhyNot(t *testing.T) {
	g := &Game{}
	g.cfg.SavePath = t.TempDir() + "/nothing-here.json"

	item := g.loadItem()
	if item.Enabled {
		t.Error("offered Load with no save file")
	}
	if item.Note == "" {
		t.Error("disabled Load with no explanation")
	}

	// Now write one.
	g.cfg.SavePath = t.TempDir() + "/colony.json"
	if err := os.WriteFile(g.cfg.SavePath, []byte(`{"version":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if item := g.loadItem(); !item.Enabled {
		t.Error("did not offer Load when a save exists")
	}
}

// An empty file is not a save. Offering it would mean a load that fails the
// moment it is taken.
func TestAnEmptySaveFileIsNotOffered(t *testing.T) {
	g := &Game{}
	g.cfg.SavePath = t.TempDir() + "/empty.json"
	if err := os.WriteFile(g.cfg.SavePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if g.haveSave() {
		t.Error("an empty file was treated as a save")
	}
}

// The main menu never offers Resume: there is nothing to go back to, and
// Escape on it must not drop the player into a world they have not started.
func TestTheMainMenuHasNoWayBackIntoNothing(t *testing.T) {
	g := &Game{}
	g.cfg.SavePath = t.TempDir() + "/none.json"

	for _, it := range g.mainMenu().Items {
		if it.Action == actionResume || it.Action == actionExitToMenu {
			t.Errorf("the main menu offers %q, which has no meaning before a game", it.Label)
		}
	}
}

// The pause menu offers a way out that is not "quit": leaving to the menu and
// resuming both have to be reachable, or Escape is a trap.
func TestThePauseMenuCanBeLeft(t *testing.T) {
	g := &Game{}
	g.cfg.SavePath = t.TempDir() + "/none.json"

	var resume, toMenu bool
	for _, it := range g.pauseMenu().Items {
		resume = resume || (it.Action == actionResume && it.Enabled)
		toMenu = toMenu || (it.Action == actionExitToMenu && it.Enabled)
	}
	if !resume {
		t.Error("no way to resume: Escape would be a trap")
	}
	if !toMenu {
		t.Error("no way back to the main menu")
	}
}

// pause and resume are the whole state machine for the in-game menu, and a
// resumed game must not leave a menu behind to be drawn over it.
func TestPauseAndResumeAreSymmetric(t *testing.T) {
	g := &Game{}
	g.cfg.SavePath = t.TempDir() + "/none.json"

	g.pause()
	if g.screen != screenPaused {
		t.Errorf("pause left the screen as %d", g.screen)
	}
	if g.menu == nil {
		t.Fatal("pause opened no menu")
	}

	g.resume()
	if g.screen != screenPlaying {
		t.Errorf("resume left the screen as %d", g.screen)
	}
	if g.menu != nil {
		t.Error("resume left a menu behind, which drawHUD would still paint")
	}
}
