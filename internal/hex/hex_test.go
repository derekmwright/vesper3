package hex

import (
	"math"
	"testing"
)

// The grid is worthless if a world position does not land back on the tile it
// came from, so this walks a patch of tiles and probes each one's center plus
// four points well inside it.
func TestAtInvertsCenter(t *testing.T) {
	l := Layout{Size: 1.3}
	for q := int32(-12); q <= 12; q++ {
		for r := int32(-12); r <= 12; r++ {
			want := Axial{q, r}
			cx, cz := l.Center(want)
			probes := [][2]float32{
				{cx, cz},
				{cx + 0.4*l.Size, cz},
				{cx - 0.4*l.Size, cz},
				{cx, cz + 0.4*l.Size},
				{cx, cz - 0.4*l.Size},
			}
			for _, p := range probes {
				if got := l.At(p[0], p[1]); got != want {
					t.Fatalf("At(%.3f, %.3f) = %v, want %v", p[0], p[1], got, want)
				}
			}
		}
	}
}

// Every point in a rectangle must belong to the tile whose center is nearest
// it. That is the definition of the grid, and it is the property a mis-phased
// corner or a bad rounding repair breaks.
func TestAtPicksTheNearestCenter(t *testing.T) {
	l := Layout{Size: 1.0}
	const step = 0.077
	for x := float32(-6); x < 6; x += step {
		for z := float32(-6); z < 6; z += step {
			got := l.At(x, z)
			gx, gz := l.Center(got)
			best := (x-gx)*(x-gx) + (z-gz)*(z-gz)

			// Nothing adjacent to the answer may be closer.
			for d := 0; d < 6; d++ {
				n := got.Neighbor(d)
				nx, nz := l.Center(n)
				if d2 := (x-nx)*(x-nx) + (z-nz)*(z-nz); d2 < best-1e-5 {
					t.Fatalf("At(%.3f,%.3f)=%v, but neighbour %v is nearer (%.5f < %.5f)",
						x, z, got, n, d2, best)
				}
			}
		}
	}
}

// The mesher walks corner i and corner i+1 to build the wall facing
// Neighbor(i). If those two orders ever drift apart, cliffs get built on the
// wrong edges, so pin them together here.
func TestCornerEdgeFacesMatchingNeighbor(t *testing.T) {
	l := Layout{Size: 1.0}
	for i := 0; i < 6; i++ {
		ax, az := l.Corner(i)
		bx, bz := l.Corner((i + 1) % 6)
		midX, midZ := (ax+bx)/2, (az+bz)/2

		nx, nz := l.Center(Directions[i])

		// The edge midpoint must lie on the segment to the neighbour's center,
		// at exactly half the distance.
		wantX, wantZ := nx/2, nz/2
		if math.Abs(float64(midX-wantX)) > 1e-4 || math.Abs(float64(midZ-wantZ)) > 1e-4 {
			t.Errorf("edge %d midpoint (%.4f,%.4f), want (%.4f,%.4f) toward %v",
				i, midX, midZ, wantX, wantZ, Directions[i])
		}
	}
}

// Corners must be one circumradius out and evenly spaced, or the hexagons do
// not tile and seams open between them.
func TestCornersFormARegularHexagon(t *testing.T) {
	l := Layout{Size: 2.5}
	for i, c := range l.Corners() {
		got := math.Hypot(float64(c[0]), float64(c[1]))
		if math.Abs(got-float64(l.Size)) > 1e-4 {
			t.Errorf("corner %d radius %.5f, want %.5f", i, got, l.Size)
		}
	}
	// Adjacent corners are one edge apart, and a regular hexagon's edge equals
	// its circumradius.
	cs := l.Corners()
	for i := 0; i < 6; i++ {
		n := cs[(i+1)%6]
		d := math.Hypot(float64(cs[i][0]-n[0]), float64(cs[i][1]-n[1]))
		if math.Abs(d-float64(l.Size)) > 1e-4 {
			t.Errorf("edge %d length %.5f, want %.5f", i, d, l.Size)
		}
	}
}

func TestNeighborsAreOneStepAway(t *testing.T) {
	c := Axial{3, -2}
	for d := 0; d < 6; d++ {
		if got := Distance(c, c.Neighbor(d)); got != 1 {
			t.Errorf("Distance to neighbour %d = %d, want 1", d, got)
		}
	}
	if got := Distance(Axial{0, 0}, Axial{3, -1}); got != 3 {
		t.Errorf("Distance = %d, want 3", got)
	}
	if got := Distance(Axial{-2, 5}, Axial{-2, 5}); got != 0 {
		t.Errorf("Distance to self = %d, want 0", got)
	}
}

// Ring's walk depends on Directions being in angular order; a shuffled table
// still returns six times radius tiles, but not the right ones.
func TestRingHoldsExactlyTheTilesAtThatDistance(t *testing.T) {
	center := Axial{2, 1}
	for radius := 1; radius <= 5; radius++ {
		ring := Ring(center, radius)
		if len(ring) != 6*radius {
			t.Fatalf("radius %d: %d tiles, want %d", radius, len(ring), 6*radius)
		}
		seen := make(map[Axial]bool, len(ring))
		for _, h := range ring {
			if d := Distance(center, h); d != radius {
				t.Errorf("radius %d: %v is at distance %d", radius, h, d)
			}
			if seen[h] {
				t.Errorf("radius %d: %v appears twice", radius, h)
			}
			seen[h] = true
		}
	}
	if got := Ring(center, 0); len(got) != 1 || got[0] != center {
		t.Errorf("Ring(c,0) = %v, want [%v]", got, center)
	}
}

func TestAreaCountsTheCenteredHexagon(t *testing.T) {
	for radius := 0; radius <= 4; radius++ {
		want := 3*radius*(radius+1) + 1
		got := Area(Axial{-1, 4}, radius)
		if len(got) != want {
			t.Fatalf("radius %d: %d tiles, want %d", radius, len(got), want)
		}
		for _, h := range got {
			if d := Distance(Axial{-1, 4}, h); d > radius {
				t.Errorf("%v is at distance %d, outside radius %d", h, d, radius)
			}
		}
	}
}

func TestSIsTheThirdCubeAxis(t *testing.T) {
	a := Axial{4, -7}
	if a.Q+a.R+a.S() != 0 {
		t.Errorf("Q+R+S = %d, want 0", a.Q+a.R+a.S())
	}
}
