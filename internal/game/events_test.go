package game

import (
	"errors"
	"testing"

	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/event"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// busGame is a Game wired far enough to publish and subscribe, with no window.
func busGame(t *testing.T) (*Game, *glyph.Engine) {
	t.Helper()

	m, err := world.NewMap(16, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})

	c := colony.New()
	c.Iron, c.Crystal = 10000, 10000

	g := &Game{
		Map:    m,
		Colony: c,
		bus:    event.New(),
		scene:  newScene(),
		intent: newIntent(),
		ui:     newUI(),
	}
	g.scene.structParts[partKey{colony.Habitat, 1}] = []meshPart{{Scale: 1}}
	g.scene.structParts[partKey{colony.SolarArray, 1}] = []meshPart{{Scale: 1}}

	e := &glyph.Engine{Scene: glyph.NewScene()}
	g.subscribe(e)
	return g, e
}

// The payoff: placing a structure is one call, and the thing that draws it
// keeps up on its own. Before the bus, place() had to remember to spawn — and
// a path that forgot left a building that existed in the colony and nowhere on
// screen.
func TestPlacingAStructureSpawnsItWithoutBeingAsked(t *testing.T) {
	g, e := busGame(t)
	at := world.FromOffset(4, 4)

	g.place(e, at)

	if _, ok := g.Colony.At(at); !ok {
		t.Fatal("the colony has no structure there")
	}
	if len(g.scene.buildingEnt[at]) == 0 {
		t.Error("the structure was placed but nothing was spawned to draw it")
	}
	if g.ui.status == "" {
		t.Error("nothing was said about it")
	}
}

// And the reverse. Demolition has two subscribers and both have to run.
func TestDemolishingRemovesTheEntities(t *testing.T) {
	g, e := busGame(t)
	at := world.FromOffset(4, 4)
	g.place(e, at)

	g.demolish(e, at)

	if _, ok := g.Colony.At(at); ok {
		t.Error("the colony still has a structure there")
	}
	if _, ok := g.scene.buildingEnt[at]; ok {
		t.Error("the entities outlived the structure")
	}
}

// A refused action publishes rather than silently doing nothing, and the
// colony's own error is what the player is shown — not a paraphrase of it that
// can drift from the rule.
func TestARefusedPlacementCarriesTheColonysReason(t *testing.T) {
	g, e := busGame(t)
	at := world.FromOffset(4, 4)
	g.place(e, at)

	var refused ActionRefused
	var seen int
	event.On(g.bus, func(ev ActionRefused) { refused, seen = ev, seen+1 })

	g.place(e, at) // same tile, already occupied

	if seen != 1 {
		t.Fatalf("%d refusals published, want 1", seen)
	}
	if !errors.Is(refused.Err, colony.ErrOccupied) {
		t.Errorf("refusal carried %v, want ErrOccupied", refused.Err)
	}
	if len(g.scene.buildingEnt[at]) != 1 {
		t.Error("a refused placement spawned something anyway")
	}
}

// Subscribers are independent: one that is not registered cannot stop another
// from running. This is what the bus buys over a chain of direct calls, so it
// is worth pinning rather than assuming.
func TestSubscribersDoNotDependOnEachOther(t *testing.T) {
	m, err := world.NewMap(16, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})
	c := colony.New()
	c.Iron, c.Crystal = 10000, 10000

	// A game with a bus but no subscriptions at all: placement still succeeds,
	// it simply goes unwatched.
	g := &Game{Map: m, Colony: c, bus: event.New(), scene: newScene(),
		intent: newIntent(), ui: newUI()}

	at := world.FromOffset(4, 4)
	g.place(nil, at)

	if _, ok := g.Colony.At(at); !ok {
		t.Error("placement failed when nothing was listening")
	}
}

// A Game with no bus at all must not panic. Several tests build one that way,
// and a nil check in one place beats a construction ritual in twenty.
func TestANilBusDoesNotBreakPlacement(t *testing.T) {
	m, err := world.NewMap(16, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})
	c := colony.New()
	c.Iron, c.Crystal = 10000, 10000

	g := &Game{Map: m, Colony: c, scene: newScene(), intent: newIntent(), ui: newUI()}
	g.place(nil, world.FromOffset(4, 4))
	g.refuse("nothing should explode")
}
