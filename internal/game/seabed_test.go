package game

import (
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// The floor exists to be hit before the sun is, so what matters is that it is
// under everything and wider than the water it hides behind. Neither is
// obvious from the numbers, and both break silently: too high and it pokes
// through the sea bed, too small and the sun comes back at the corners.
func TestTheSeabedIsUnderEverythingAndWiderThanTheSea(t *testing.T) {
	m, err := world.NewMap(48, 48, 7)
	if err != nil {
		t.Fatal(err)
	}
	world.Generate(m)

	cx, cz, y, size := seabedPlane(m)

	// Under the lowest ground there can be. Elevation clamps at zero, so a
	// cap and the foot of a cliff both bottom out at SurfaceYAt(0).
	if floor := world.SurfaceYAt(0); y >= floor {
		t.Errorf("the seabed sits at %.2f, at or above the lowest ground at %.2f", y, floor)
	}

	// And under the water, which is the surface it hides behind.
	if y >= world.SeaY() {
		t.Errorf("the seabed at %.2f is not under the sea surface at %.2f", y, world.SeaY())
	}

	// Wide enough to cover the water, corners included. The water is the map
	// rectangle grown by oceanMargin; the floor is a square, so the far corner
	// of that rectangle is the case that has to reach.
	minX, minZ, maxX, maxZ := m.Bounds()
	half := size / 2
	for _, c := range [4][2]float32{
		{minX - oceanMargin, minZ - oceanMargin},
		{maxX + oceanMargin, minZ - oceanMargin},
		{minX - oceanMargin, maxZ + oceanMargin},
		{maxX + oceanMargin, maxZ + oceanMargin},
	} {
		if dx := float32(math.Abs(float64(c[0] - cx))); dx > half {
			t.Errorf("water corner x=%.1f is %.1f from the centre, past the floor's %.1f", c[0], dx, half)
		}
		if dz := float32(math.Abs(float64(c[1] - cz))); dz > half {
			t.Errorf("water corner z=%.1f is %.1f from the centre, past the floor's %.1f", c[1], dz, half)
		}
	}

	// Centred on the map rather than on the origin: the map starts at tile
	// (0,0) and runs positive, so a plane at the origin would hang off it.
	wantX, wantZ := (minX+maxX)/2, (minZ+maxZ)/2
	if cx != wantX || cz != wantZ {
		t.Errorf("seabed centred at (%.1f, %.1f), map centre is (%.1f, %.1f)", cx, cz, wantX, wantZ)
	}
}
