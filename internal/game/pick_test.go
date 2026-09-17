package game

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// The case the whole cursor rests on: a ray straight down over a tile has to
// return that tile, for every tile on a real generated map.
func TestPickStraightDownFindsTheTileUnderneath(t *testing.T) {
	m := pickMap(t)

	tested := 0
	m.Each(func(a hex.Axial, tile *world.Tile) {
		x, z := world.Layout.Center(a)
		origin := mgl32.Vec3{x, 60, z}
		dir := mgl32.Vec3{0, -1, 0}

		got, ok := Pick(m, origin, dir, 200)
		if !ok {
			t.Fatalf("no hit straight down over %v", a)
		}
		if got != a {
			t.Fatalf("ray over %v hit %v", a, got)
		}
		tested++
	})
	if tested == 0 {
		t.Fatal("no tiles were tested")
	}
}

// Off-centre probes matter more than centres: they are where a rounding error
// in the layout shows up as a cursor that sits one tile off what the player
// is pointing at.
func TestPickHitsTheRightTileFromOffCentre(t *testing.T) {
	m := flatPickMap(t, 8)

	offsets := [][2]float32{
		{0, 0}, {0.35, 0}, {-0.35, 0}, {0, 0.35}, {0, -0.35}, {0.25, 0.25},
	}
	for col := 2; col < 10; col++ {
		for row := 2; row < 10; row++ {
			a := world.FromOffset(col, row)
			cx, cz := world.Layout.Center(a)
			for _, off := range offsets {
				origin := mgl32.Vec3{cx + off[0], 40, cz + off[1]}
				got, ok := Pick(m, origin, mgl32.Vec3{0, -1, 0}, 200)
				if !ok {
					t.Fatalf("no hit at %v offset %v", a, off)
				}
				if got != a {
					t.Errorf("offset %v over %v hit %v", off, a, got)
				}
			}
		}
	}
}

func TestPickMissesWhenPointedAtTheSky(t *testing.T) {
	m := pickMap(t)
	origin := mgl32.Vec3{0, 80, 0}

	if _, ok := Pick(m, origin, mgl32.Vec3{0, 1, 0}, 200); ok {
		t.Error("a ray pointed straight up hit the ground")
	}
	if _, ok := Pick(m, origin, mgl32.Vec3{0.3, 0.9, 0.3}.Normalize(), 200); ok {
		t.Error("a ray angled upward hit the ground")
	}
}

func TestPickMissesBesideTheMap(t *testing.T) {
	m := pickMap(t)
	// Well outside the array, pointing down into empty space.
	origin := mgl32.Vec3{-500, 40, -500}
	if _, ok := Pick(m, origin, mgl32.Vec3{0, -1, 0}, 200); ok {
		t.Error("a ray beside the map reported a hit")
	}
}

// Clicking the face of a cliff should select the tile on top of it, not the
// low tile the ray would eventually reach.
func TestPickOnACliffFaceReturnsTheHighTile(t *testing.T) {
	m := flatPickMap(t, 2)

	high := world.FromOffset(6, 6)
	m.At(high).Elevation = 10

	hx, hz := world.Layout.Center(high)
	lowNeighbour := high.Neighbor(3) // to the west
	lx, _ := world.Layout.Center(lowNeighbour)

	// Aim at the cliff's west face from low and to the west, at a shallow
	// angle so the ray strikes the wall rather than the top.
	target := mgl32.Vec3{hx - world.TileSize*0.9, world.SurfaceYAt(6), hz}
	origin := mgl32.Vec3{lx - 6, world.SurfaceYAt(9), hz}
	dir := target.Sub(origin).Normalize()

	got, ok := Pick(m, origin, dir, 200)
	if !ok {
		t.Fatal("no hit on the cliff face")
	}
	if got != high {
		t.Errorf("cliff face hit %v (elevation %d), want %v (elevation %d)",
			got, m.Elevation(got), high, m.Elevation(high))
	}
}

// A camera that has been driven inside a hill must still report a tile, or
// the cursor vanishes and the player cannot dig their way out.
func TestPickFromUndergroundReportsTheTileItIsIn(t *testing.T) {
	m := flatPickMap(t, 9)
	a := world.FromOffset(5, 5)
	x, z := world.Layout.Center(a)

	got, ok := Pick(m, mgl32.Vec3{x, world.SurfaceYAt(4), z}, mgl32.Vec3{0, -1, 0}, 200)
	if !ok {
		t.Fatal("a ray starting underground found nothing")
	}
	if got != a {
		t.Errorf("underground pick returned %v, want %v", got, a)
	}
}

// An unnormalised direction is easy to pass by accident, and silently
// scaling every distance by its length would make maxDist meaningless.
func TestPickNormalisesItsDirection(t *testing.T) {
	m := flatPickMap(t, 5)
	a := world.FromOffset(5, 5)
	x, z := world.Layout.Center(a)

	got, ok := Pick(m, mgl32.Vec3{x, 40, z}, mgl32.Vec3{0, -17, 0}, 200)
	if !ok || got != a {
		t.Errorf("Pick with an unnormalised dir = %v, %v; want %v, true", got, ok, a)
	}
}

func TestPickHandlesDegenerateInput(t *testing.T) {
	m := flatPickMap(t, 5)
	if _, ok := Pick(nil, mgl32.Vec3{}, mgl32.Vec3{0, -1, 0}, 100); ok {
		t.Error("a nil map reported a hit")
	}
	if _, ok := Pick(m, mgl32.Vec3{0, 40, 0}, mgl32.Vec3{}, 100); ok {
		t.Error("a zero-length direction reported a hit")
	}
}

func pickMap(t *testing.T) *world.Map {
	t.Helper()
	m, err := world.NewMap(24, 24, 7)
	if err != nil {
		t.Fatal(err)
	}
	world.Generate(m)
	return m
}

func flatPickMap(t *testing.T, elevation int8) *world.Map {
	t.Helper()
	m, err := world.NewMap(16, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = elevation
	})
	return m
}
