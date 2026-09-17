package meshgen

import (
	"math"

	"github.com/derekmwright/glyphengine/renderer"
)

// Builder accumulates flat-shaded geometry. Every face is given its outward
// direction and the builder works out the winding, rather than the caller
// getting it right six times per box and wrong once.
//
// Faces do not share vertices even where they meet, which is the point: a
// shared vertex has one normal, and these shapes want a hard edge at every
// seam.
type Builder struct {
	Verts []renderer.Vertex
	Idx   []uint32
}

// Scale multiplies every vertex position by f, about the origin. Normals are
// left alone, which is correct for a uniform scale and would not be for a
// non-uniform one — hence the single factor.
func (b *Builder) Scale(f float32) {
	for i := range b.Verts {
		b.Verts[i].Pos[0] *= f
		b.Verts[i].Pos[1] *= f
		b.Verts[i].Pos[2] *= f
	}
}

// Tri appends a triangle. outward decides which side faces the viewer; the
// winding is flipped to match if it has to be.
func (b *Builder) Tri(p0, p1, p2, outward [3]float32, col [3]float32) {
	n := normalize(outward)

	// The renderer wants the cross product opposite the normal; see this
	// package's doc comment.
	if dot3(cross3(sub3(p1, p0), sub3(p2, p0)), n) > 0 {
		p1, p2 = p2, p1
	}

	base := uint32(len(b.Verts))
	b.Verts = append(b.Verts,
		renderer.Vertex{Pos: p0, Color: col, Normal: n, UV: [2]float32{0, 0}},
		renderer.Vertex{Pos: p1, Color: col, Normal: n, UV: [2]float32{1, 0}},
		renderer.Vertex{Pos: p2, Color: col, Normal: n, UV: [2]float32{1, 1}},
	)
	b.Idx = append(b.Idx, base, base+1, base+2)
}

// Quad appends a planar quad given its corners in ring order.
func (b *Builder) Quad(p0, p1, p2, p3, outward [3]float32, col [3]float32) {
	b.Tri(p0, p1, p2, outward, col)
	b.Tri(p0, p2, p3, outward, col)
}

// Box appends an axis-aligned box centred on (cx, cy, cz).
func (b *Builder) Box(cx, cy, cz, sx, sy, sz float32, col [3]float32) {
	hx, hy, hz := sx/2, sy/2, sz/2
	x0, x1 := cx-hx, cx+hx
	y0, y1 := cy-hy, cy+hy
	z0, z1 := cz-hz, cz+hz

	p := func(x, y, z float32) [3]float32 { return [3]float32{x, y, z} }

	b.Quad(p(x0, y1, z0), p(x1, y1, z0), p(x1, y1, z1), p(x0, y1, z1), [3]float32{0, 1, 0}, col)
	b.Quad(p(x0, y0, z0), p(x1, y0, z0), p(x1, y0, z1), p(x0, y0, z1), [3]float32{0, -1, 0}, shade(col, 0.55))
	b.Quad(p(x0, y0, z0), p(x1, y0, z0), p(x1, y1, z0), p(x0, y1, z0), [3]float32{0, 0, -1}, shade(col, 0.82))
	b.Quad(p(x0, y0, z1), p(x1, y0, z1), p(x1, y1, z1), p(x0, y1, z1), [3]float32{0, 0, 1}, shade(col, 0.82))
	b.Quad(p(x0, y0, z0), p(x0, y0, z1), p(x0, y1, z1), p(x0, y1, z0), [3]float32{-1, 0, 0}, shade(col, 0.9))
	b.Quad(p(x1, y0, z0), p(x1, y0, z1), p(x1, y1, z1), p(x1, y1, z0), [3]float32{1, 0, 0}, shade(col, 0.9))
}

// TiltedPanel appends a thin slab rotated about the X axis, for solar panels
// facing the sky at an angle.
func (b *Builder) TiltedPanel(cx, cy, cz, w, d, tilt float32, col [3]float32) {
	const thick = 0.035
	sin, cos := sin32(tilt), cos32(tilt)

	// Corners of the panel plane, tilted about X: the Z extent picks up a Y
	// component as it rotates.
	corner := func(sx, sz float32) [3]float32 {
		lz := sz * d / 2
		return [3]float32{cx + sx*w/2, cy + lz*sin, cz + lz*cos}
	}
	n := [3]float32{0, cos, -sin}

	a, bb, c, d2 := corner(-1, -1), corner(1, -1), corner(1, 1), corner(-1, 1)
	off := [3]float32{n[0] * thick, n[1] * thick, n[2] * thick}
	lower := func(p [3]float32) [3]float32 {
		return [3]float32{p[0] - off[0], p[1] - off[1], p[2] - off[2]}
	}

	// The panel's own depth direction, in its tilted plane. The end faces
	// point along this, NOT along n: n is perpendicular to it, so using n
	// here gives faces that are edge-on to their own normal and light as
	// black slivers.
	depth := [3]float32{0, sin, cos}

	b.Quad(a, bb, c, d2, n, col)
	b.Quad(lower(a), lower(bb), lower(c), lower(d2), [3]float32{-n[0], -n[1], -n[2]}, shade(col, 0.5))
	b.Quad(a, bb, lower(bb), lower(a), [3]float32{-depth[0], -depth[1], -depth[2]}, shade(col, 0.7))
	b.Quad(d2, c, lower(c), lower(d2), depth, shade(col, 0.7))
	b.Quad(a, d2, lower(d2), lower(a), [3]float32{-1, 0, 0}, shade(col, 0.65))
	b.Quad(bb, c, lower(c), lower(bb), [3]float32{1, 0, 0}, shade(col, 0.65))
}

// Cylinder appends a cylinder standing on (cx, cy, cz) with its base at cy.
func (b *Builder) Cylinder(cx, cy, cz, radius, height float32, segments int, col [3]float32) {
	if segments < 3 {
		segments = 3
	}
	top := cy + height
	side := shade(col, 0.88)

	for i := 0; i < segments; i++ {
		a0 := float64(i) / float64(segments) * 2 * math.Pi
		a1 := float64(i+1) / float64(segments) * 2 * math.Pi
		x0, z0 := cx+radius*cos64(a0), cz+radius*sin64(a0)
		x1, z1 := cx+radius*cos64(a1), cz+radius*sin64(a1)

		// One normal per quad rather than per vertex: these are small props
		// and the facets read as machined rather than as low-poly.
		mid := (a0 + a1) / 2
		n := [3]float32{cos64(mid), 0, sin64(mid)}

		b.Quad(
			[3]float32{x0, cy, z0}, [3]float32{x1, cy, z1},
			[3]float32{x1, top, z1}, [3]float32{x0, top, z0},
			n, side,
		)
		b.Tri(
			[3]float32{cx, top, cz},
			[3]float32{x0, top, z0},
			[3]float32{x1, top, z1},
			[3]float32{0, 1, 0}, col,
		)
	}
}

// Cone appends a cone standing on (cx, cy, cz), point up.
func (b *Builder) Cone(cx, cy, cz, radius, height float32, segments int, col [3]float32) {
	if segments < 3 {
		segments = 3
	}
	apex := [3]float32{cx, cy + height, cz}
	for i := 0; i < segments; i++ {
		a0 := float64(i) / float64(segments) * 2 * math.Pi
		a1 := float64(i+1) / float64(segments) * 2 * math.Pi
		p0 := [3]float32{cx + radius*cos64(a0), cy, cz + radius*sin64(a0)}
		p1 := [3]float32{cx + radius*cos64(a1), cy, cz + radius*sin64(a1)}

		mid := (a0 + a1) / 2
		// Tilt the normal up by the slope so the cone lights as a cone.
		slope := radius / max32(height, 1e-3)
		n := normalize([3]float32{cos64(mid), slope, sin64(mid)})
		b.Tri(apex, p0, p1, n, col)
	}
}

// Dome appends a hemisphere sitting on (cx, cy, cz).
func (b *Builder) Dome(cx, cy, cz, radius float32, rings, segments int, col [3]float32) {
	if rings < 2 {
		rings = 2
	}
	if segments < 3 {
		segments = 3
	}
	at := func(ring, seg int) [3]float32 {
		phi := float64(ring) / float64(rings) * (math.Pi / 2)
		theta := float64(seg) / float64(segments) * 2 * math.Pi
		r := radius * cos64(phi)
		return [3]float32{
			cx + r*cos64(theta),
			cy + radius*sin64(phi),
			cz + r*sin64(theta),
		}
	}

	for ring := 0; ring < rings; ring++ {
		for seg := 0; seg < segments; seg++ {
			p00 := at(ring, seg)
			p01 := at(ring, seg+1)
			p10 := at(ring+1, seg)
			p11 := at(ring+1, seg+1)

			out := normalize([3]float32{
				(p00[0]+p01[0]+p10[0]+p11[0])/4 - cx,
				(p00[1]+p01[1]+p10[1]+p11[1])/4 - cy,
				(p00[2]+p01[2]+p10[2]+p11[2])/4 - cz,
			})

			// The top ring collapses to a point, so it is a triangle.
			if ring == rings-1 {
				b.Tri(p00, p01, [3]float32{cx, cy + radius, cz}, out, col)
				continue
			}
			b.Quad(p00, p01, p11, p10, out, col)
		}
	}
}

// HexRing appends a flat hexagonal annulus lying in the XZ plane, used for
// the tile cursor. It matches the grid's corner phase, so it sits exactly on
// a tile rather than rotated 30 degrees off it.
func (b *Builder) HexRing(cy, inner, outer float32, col [3]float32) {
	up := [3]float32{0, 1, 0}
	for i := 0; i < 6; i++ {
		a0 := float64(i) * (math.Pi / 3)
		a1 := float64(i+1) * (math.Pi / 3)

		i0 := [3]float32{inner * cos64(a0), cy, inner * sin64(a0)}
		i1 := [3]float32{inner * cos64(a1), cy, inner * sin64(a1)}
		o0 := [3]float32{outer * cos64(a0), cy, outer * sin64(a0)}
		o1 := [3]float32{outer * cos64(a1), cy, outer * sin64(a1)}

		b.Quad(i0, o0, o1, i1, up, col)
	}
}

func shade(c [3]float32, f float32) [3]float32 { return scaleColor(c, f) }

func sub3(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross3(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func dot3(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cos64(a float64) float32 { return float32(math.Cos(a)) }
func sin64(a float64) float32 { return float32(math.Sin(a)) }

func cos32(a float32) float32 { return float32(math.Cos(float64(a))) }
func sin32(a float32) float32 { return float32(math.Sin(float64(a))) }

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
