package game

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/world"
)

// Starting, abandoning and restarting a colony.
//
// A new game replaces the world in place rather than rebuilding the renderer's
// side of it. The chunk meshes were allocated for a fixed grid and terraforming
// already rewrites them every time a tile moves, so regenerating the map is the
// same operation at a larger scale: new tiles, upload every chunk, respawn what
// is standing. Loading a save has always worked this way; this is that path
// with a generated map instead of a decoded one.
//
// Nothing here recreates a GPU resource, which is what keeps it cheap enough to
// sit behind a menu item. The one thing that cannot change is the map's
// dimensions, for the same reason a save from a differently sized world is
// refused rather than half-loaded.

// startWorld replaces the current world with a freshly generated one.
func (g *Game) startWorld(e *glyph.Engine, seed int64) error {
	// A world always starts running, whatever the clock was doing before it.
	// Leaving a paused game for the main menu would otherwise hand the menu a
	// frozen backdrop, and the backdrop moving is the whole point of it being
	// a world rather than a picture of one.
	e.SetTimeScale(1)

	m, err := world.NewMap(g.cfg.Cols, g.cfg.Rows, seed)
	if err != nil {
		return fmt.Errorf("create map: %w", err)
	}
	world.Generate(m)

	g.Map = m
	g.Colony = colony.New()

	// The outgoing colony's entities go before the incoming one spawns its
	// own, or both sets are drawn on top of each other.
	g.scene.clearBuildings(e)
	g.rebuildAllChunks(e)
	g.refreshTerrainHeights(e)

	g.initCamera()
	g.foundLandingSite(e)
	if g.cfg.Demo {
		g.foundDemoColony(e)
	}
	e.RebuildStatics()

	// A fresh colony has nothing selected from the last one and no stale
	// advisory sitting under the panel.
	g.intent = newIntent()
	g.ui.status, g.ui.statusUntil = "", 0
	g.rebuildGhost(e)

	log.Printf("Vesper III: seed %d, %dx%d tiles, colony at %v",
		m.Seed, m.Cols, m.Rows, m.StartSite())
	return nil
}

// newGame starts a colony on a planet nobody has seen.
//
// The seed is random rather than the one -seed pinned, because -seed fixes the
// world the process *opens* on — for a screenshot, or to compare a change
// against the same terrain. Once a player has asked for a new colony, giving
// them the same planet again would be the opposite of what they asked.
func (g *Game) newGame(e *glyph.Engine) {
	if err := g.startWorld(e, rand.Int64()); err != nil {
		g.refuse("Could not start a new colony: %v", err)
		return
	}
	g.screen = screenPlaying
}

// exitToMenu abandons the current colony and returns to the main menu, over a
// world generated for the occasion.
//
// It generates rather than keeping the abandoned one, so the menu is never a
// picture of the colony that was just given up on — and so New Colony from
// there is a different planet again.
func (g *Game) exitToMenu(e *glyph.Engine) {
	if err := g.startWorld(e, rand.Int64()); err != nil {
		log.Printf("could not generate a backdrop: %v", err)
	}
	g.screen = screenMenu
	g.menu = g.mainMenu()
	g.frameBackdrop()
}

// menuOrbitRate is how fast the main menu's camera turns, in radians a second.
// A full turn takes a little over three minutes: enough that the view is
// visibly alive, slow enough that it is not the thing you are looking at.
const menuOrbitRate = 0.03

// frameBackdrop points the camera at the whole continent, so the main menu
// always opens on the planet rather than on whatever the last camera position
// happened to be looking at — which, after a game, is a patch of ground five
// tiles wide, and on a fresh launch is the landing site at playing distance.
func (g *Game) frameBackdrop() {
	minX, minZ, maxX, maxZ := g.Map.Bounds()
	g.cam.FrameAll(minX, minZ, maxX, maxZ)
}

// haveSave reports whether there is a save file to load, which is what decides
// whether the menus offer it.
func (g *Game) haveSave() bool {
	info, err := os.Stat(g.savePath())
	return err == nil && !info.IsDir() && info.Size() > 0
}

// handleMenu runs whichever menu is open and acts on what it returns.
//
// It is the only place a menu action turns into a change, which is what keeps
// the widget in menu.go free of the game.
func (g *Game) handleMenu(e *glyph.Engine) {
	if g.menu == nil {
		g.menu = g.mainMenu()
	}

	w, ph := e.Renderer().Extent()
	scale := g.uiScale(float32(ph))
	dw, dh := float32(w)/scale, float32(ph)/scale

	mx, my := g.pointer(e)
	px, py := float32(mx)/scale, float32(my)/scale

	// Escape closes the pause menu and does nothing on the main one, where
	// there is nothing behind it to go back to.
	if g.screen == screenPaused && e.Input().KeyPressed(input.KeyEscape) {
		g.resume(e)
		return
	}

	switch g.menu.update(e.Input(), px, py, dw, dh) {
	case actionNewGame:
		g.newGame(e)

	case actionLoadGame:
		if err := g.Load(e); err != nil {
			g.refuse("Load failed: %v", err)
			return
		}
		emit(g, GameLoaded{Path: g.cfg.SavePath})
		g.screen = screenPlaying

	case actionSaveGame:
		if err := g.Save(); err != nil {
			g.refuse("Save failed: %v", err)
			return
		}
		emit(g, GameSaved{Path: g.cfg.SavePath})
		// The menu is rebuilt so Load stops saying "no save file yet" the
		// instant there is one.
		g.menu = g.pauseMenu()

	case actionResume:
		g.resume(e)

	case actionExitToMenu:
		g.exitToMenu(e)

	case actionQuit:
		e.Close()
	}
}

// pause opens the in-game menu.
// clock is the part of the engine that pausing needs.
//
// An interface rather than *glyph.Engine because pause and resume are the
// whole state machine for the in-game menu and are worth testing, and a
// renderer cannot be stood up in a unit test. Narrowing it to the one method
// also makes the dependency legible: pausing stops a clock, and that is all it
// does to the engine.
type clock interface {
	SetTimeScale(float32)
}

func (g *Game) pause(e clock) {
	g.screen = screenPaused
	g.menu = g.pauseMenu()

	// Stop the engine's clock too, not just this game's.
	//
	// Returning early from Update already stopped the colony, and for a game
	// with no physics and no skinned meshes that looked like enough. It was
	// not: the sun kept crossing the sky behind the menu, so pausing at dusk
	// to read the panel and coming back found the lamps on and the solar
	// arrays dead. The engine's own note on SetTimeScale is blunt about it -
	// a game that pauses by returning early gets away with it by luck.
	e.SetTimeScale(0)
}

// resume closes it.
func (g *Game) resume(e clock) {
	g.screen = screenPlaying
	g.menu = nil
	e.SetTimeScale(1)
}
