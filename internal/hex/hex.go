// Package hex is the coordinate system the whole game is built on: flat-top
// hexagons addressed by axial coordinates.
//
// Flat-top means a hexagon has vertical left and right edges and points at
// due east and due west. Columns of hexes line up along X and interlock along
// Z, which is what makes a road or a wall read as straight on screen.
//
// Axial (Q,R) is cube coordinates with the third axis dropped, since
// S = -Q-R is always recoverable. Two integers per tile, honest arithmetic,
// and distance is a one-liner. See redblobgames.com/grids/hexagons for the
// derivations; the conventions here match that page's "flat-top, axial".
package hex

import "math"

// Sqrt3 is the ratio between a flat-top hexagon's height and its
// circumradius. It appears in every world-space conversion below.
const Sqrt3 = 1.7320508

// Axial is a tile address. Q selects the column, R the position within it.
type Axial struct {
	Q, R int32
}

// S is the implicit third cube axis.
func (a Axial) S() int32 { return -a.Q - a.R }

// Add returns the coordinate offset by b.
func (a Axial) Add(b Axial) Axial { return Axial{a.Q + b.Q, a.R + b.R} }

// Sub returns the vector from b to a.
func (a Axial) Sub(b Axial) Axial { return Axial{a.Q - b.Q, a.R - b.R} }

// Directions are the six neighbours in angular order, starting with the one
// at 30 degrees in the XZ plane and proceeding the same way Layout.Corner
// does. That shared order is load-bearing: the edge between corner i and
// corner i+1 is exactly the edge shared with Neighbor(i), which is what lets
// the cliff mesher walk corners and neighbours with one index.
var Directions = [6]Axial{
	{1, 0},
	{0, 1},
	{-1, 1},
	{-1, 0},
	{0, -1},
	{1, -1},
}

// Neighbor returns the adjacent tile in direction d, which is taken modulo 6.
func (a Axial) Neighbor(d int) Axial {
	return a.Add(Directions[((d%6)+6)%6])
}

// Distance is the number of steps between two tiles, moving one tile at a
// time through shared edges.
func Distance(a, b Axial) int {
	d := a.Sub(b)
	q, r, s := abs32(d.Q), abs32(d.R), abs32(d.S())
	return int((q + r + s) / 2)
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// Ring returns the tiles exactly radius steps from center, walking the ring
// once. Radius 0 is the center itself.
func Ring(center Axial, radius int) []Axial {
	if radius <= 0 {
		return []Axial{center}
	}
	out := make([]Axial, 0, 6*radius)
	// Start radius steps to the south-west, then walk each of the six sides.
	cur := center
	for i := 0; i < radius; i++ {
		cur = cur.Neighbor(4)
	}
	for side := 0; side < 6; side++ {
		for step := 0; step < radius; step++ {
			out = append(out, cur)
			cur = cur.Neighbor(side)
		}
	}
	return out
}

// Area returns every tile within radius steps of center, center included.
func Area(center Axial, radius int) []Axial {
	if radius < 0 {
		return nil
	}
	out := make([]Axial, 0, 3*radius*(radius+1)+1)
	for q := -radius; q <= radius; q++ {
		lo, hi := max(-radius, -q-radius), min(radius, -q+radius)
		for r := lo; r <= hi; r++ {
			out = append(out, Axial{center.Q + int32(q), center.R + int32(r)})
		}
	}
	return out
}

// Layout converts between tile addresses and world space. Size is the
// circumradius: the distance from a hexagon's center to any of its corners,
// which is also the length of every edge.
type Layout struct {
	Size float32
}

// Width is a tile's full extent along X, corner to corner.
func (l Layout) Width() float32 { return 2 * l.Size }

// Height is a tile's full extent along Z, flat edge to flat edge.
func (l Layout) Height() float32 { return Sqrt3 * l.Size }

// Center returns the world-space XZ center of a tile. Y is the game's
// business, not the grid's.
func (l Layout) Center(a Axial) (x, z float32) {
	x = l.Size * 1.5 * float32(a.Q)
	z = l.Size * Sqrt3 * (float32(a.R) + float32(a.Q)/2)
	return
}

// Corner returns the offset from a tile's center to corner i, for i in 0..5.
//
// A flat-top hexagon has a corner due east, so corner i sits at i*60 degrees.
// Edge midpoints then fall at 30, 90, 150... degrees, which is precisely where
// the six neighbouring centers are — the invariant Directions documents. The
// pointy-top layout is the one that needs a 30 degree phase here; using it on
// a flat-top grid puts corners where edges belong and silently shears every
// cliff wall half a tile off its edge.
func (l Layout) Corner(i int) (dx, dz float32) {
	ang := float64(((i%6)+6)%6) * (math.Pi / 3)
	return l.Size * float32(math.Cos(ang)), l.Size * float32(math.Sin(ang))
}

// Corners returns all six corner offsets in one allocation, in the order
// Corner defines.
func (l Layout) Corners() [6][2]float32 {
	var out [6][2]float32
	for i := range out {
		dx, dz := l.Corner(i)
		out[i] = [2]float32{dx, dz}
	}
	return out
}

// At returns the tile containing a world-space XZ point.
func (l Layout) At(x, z float32) Axial {
	qf := (2.0 / 3.0 * x) / l.Size
	rf := (-1.0/3.0*x + Sqrt3/3.0*z) / l.Size
	return round(qf, rf)
}

// round snaps fractional axial coordinates to the nearest tile. It rounds in
// cube space and repairs the axis that moved furthest, which is what keeps
// the result inside the hexagon the point actually fell in.
func round(qf, rf float32) Axial {
	sf := -qf - rf
	q := float64(math.Round(float64(qf)))
	r := float64(math.Round(float64(rf)))
	s := float64(math.Round(float64(sf)))

	dq := math.Abs(q - float64(qf))
	dr := math.Abs(r - float64(rf))
	ds := math.Abs(s - float64(sf))

	switch {
	case dq > dr && dq > ds:
		q = -r - s
	case dr > ds:
		r = -q - s
	}
	return Axial{int32(q), int32(r)}
}
