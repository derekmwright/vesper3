package game

import (
	"testing"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// The preview is the real structure mesh with its material stripped, so over a
// tile that already has something on it the two interpenetrate - a red habitat
// dome growing out of a solar array, which reads as two buildings in one place
// rather than as a refusal. The red tile marker says "not here" on its own.
func TestThePreviewStaysOffOccupiedTiles(t *testing.T) {
	m, err := world.NewMap(16, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})

	c := colony.New()
	g := &Game{Map: m, Colony: c}
	g.intent = newIntent()
	g.intent.hovering = true

	at := world.FromOffset(5, 5)
	g.intent.hover = at
	if !g.ghostVisible() {
		t.Error("no preview on empty ground, where it is the useful half")
	}

	if err := c.Found(m, colony.Habitat, at); err != nil {
		t.Fatal(err)
	}
	if g.ghostVisible() {
		t.Error("preview drawn over a building, which is what makes it read as two")
	}

	// A refusal on empty ground still shows it: the mesh is how a player sees
	// what they were about to place.
	g.intent.hover = at.Neighbor(0)
	if !g.ghostVisible() {
		t.Error("preview hidden on an empty neighbour")
	}

	// And never outside build mode.
	g.intent.mode = ModeDemolish
	if g.ghostVisible() {
		t.Error("preview drawn in demolish mode")
	}
	g.intent.mode = ModeBuild
	g.intent.hovering = false
	if g.ghostVisible() {
		t.Error("preview drawn while pointing at the sky")
	}
}
