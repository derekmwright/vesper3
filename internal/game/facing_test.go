package game

import (
	"math"
	"testing"

	"github.com/derekmwright/worldbuild/internal/colony"
	"github.com/derekmwright/worldbuild/internal/hex"
	"github.com/derekmwright/worldbuild/internal/world"
)

// Facings are sixths of a turn because that is the grid's symmetry. Any other
// step leaves a structure sitting crooked on its hexagon, which is the whole
// reason the old per-tile hash used sixths too.
func TestFacingYawCoversTheHexagon(t *testing.T) {
	seen := map[float32]bool{}
	for f := uint8(0); f < colony.Facings; f++ {
		y := facingYaw(f)
		if y < 0 || y >= 2*math.Pi {
			t.Errorf("facing %d gives yaw %.3f, outside one turn", f, y)
		}
		if seen[y] {
			t.Errorf("facing %d repeats an earlier yaw %.3f", f, y)
		}
		seen[y] = true
	}
	if len(seen) != colony.Facings {
		t.Errorf("%d distinct headings, want %d", len(seen), colony.Facings)
	}

	// A sixth of a turn each, and a full turn back to the start.
	step := facingYaw(1)
	if math.Abs(float64(step)-math.Pi/3) > 1e-5 {
		t.Errorf("step is %.4f rad, want %.4f (60 degrees)", step, math.Pi/3)
	}
	if facingYaw(colony.Facings) != facingYaw(0) {
		t.Error("a full turn does not return to the first heading")
	}
}

// Rotation has to survive placement and a reload, or a player who lined a row
// up finds it scrambled next session.
func TestFacingIsStoredWithTheBuilding(t *testing.T) {
	m, err := world.NewMap(12, 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})

	c := colony.New()
	c.Iron = 10000
	c.Crystal = 10000

	at := world.FromOffset(4, 4)
	if err := c.PlaceFacing(m, colony.Habitat, at, 4); err != nil {
		t.Fatal(err)
	}

	b, ok := c.At(at)
	if !ok {
		t.Fatal("nothing placed")
	}
	if b.Facing != 4 {
		t.Errorf("facing %d, want 4", b.Facing)
	}

	// Across a save, which keeps only the exported fields.
	loaded := &colony.Colony{Buildings: c.Buildings}
	loaded.Reindex()
	if lb, ok := loaded.At(at); !ok || lb.Facing != 4 {
		t.Errorf("facing came back as %d after a round trip", lb.Facing)
	}
}

// Out-of-range facings wrap rather than producing a seventh heading nothing
// can draw.
func TestFacingWraps(t *testing.T) {
	m, err := world.NewMap(12, 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 2
	})

	c := colony.New()
	c.Iron = 10000
	c.Crystal = 10000
	at := world.FromOffset(4, 4)
	if err := c.PlaceFacing(m, colony.Habitat, at, colony.Facings+2); err != nil {
		t.Fatal(err)
	}
	b, _ := c.At(at)
	if b.Facing >= colony.Facings {
		t.Errorf("facing %d was not wrapped into 0..%d", b.Facing, colony.Facings-1)
	}
	if b.Facing != 2 {
		t.Errorf("facing %d, want 2", b.Facing)
	}
}
