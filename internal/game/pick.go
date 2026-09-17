package game

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// Picking steps along the ray this far at a time. A tile is two units across
// and 1.73 deep, so a third of a unit samples every tile the ray crosses
// several times even at a grazing angle, and the refinement below recovers
// the exact crossing from whichever step straddles it.
const (
	pickStep    = 0.33
	pickRefine  = 12 // bisection rounds; 12 puts the crossing inside a millimetre
	pickMaxDist = 600
)

// Pick returns the tile a ray enters the ground in.
//
// This does not use the engine's Scene.Raycast, and deliberately so. That
// works against colliders, and giving 2,304 tiles a collider apiece to answer
// one query per frame would cost a broadphase rebuild and a pile of entities
// for something the map can answer directly. The terrain is a height per
// tile, so the question "where does this ray go underground" is a march and a
// bisection, with no scene structures involved at all.
//
// A ray that hits a cliff face returns the tile at the top of the cliff,
// which is the tile the player is pointing at.
func Pick(m *world.Map, origin, dir mgl32.Vec3, maxDist float32) (hex.Axial, bool) {
	if m == nil {
		return hex.Axial{}, false
	}
	if maxDist <= 0 || maxDist > pickMaxDist {
		maxDist = pickMaxDist
	}
	if d := dir.Len(); d == 0 {
		return hex.Axial{}, false
	} else if d < 0.999 || d > 1.001 {
		dir = dir.Normalize()
	}

	ceiling := world.SurfaceYAt(world.MaxElevation)
	floor := float32(-1)

	// Skip the part of the ray that is above every possible tile: it cannot
	// hit anything, and at a shallow angle it is most of the ray.
	t0 := float32(0)
	if origin.Y() > ceiling {
		if dir.Y() >= 0 {
			return hex.Axial{}, false // pointed at the sky from above the world
		}
		t0 = (ceiling - origin.Y()) / dir.Y()
	}
	if t0 > maxDist {
		return hex.Axial{}, false
	}

	// And stop once it has passed below the lowest ground.
	t1 := maxDist
	if dir.Y() < 0 {
		if below := (floor - origin.Y()) / dir.Y(); below < t1 {
			t1 = below
		}
	}

	inside := func(t float32) (hex.Axial, bool) {
		p := origin.Add(dir.Mul(t))
		a := world.Layout.At(p.X(), p.Z())
		if !m.Contains(a) {
			return a, false
		}
		return a, p.Y() <= m.SurfaceY(a)
	}

	// The camera can sit inside a mountain after a terraform; starting
	// underground should report the tile it is in rather than nothing.
	if a, hit := inside(t0); hit {
		return a, true
	}

	prev := t0
	for t := t0 + pickStep; t <= t1; t += pickStep {
		if _, hit := inside(t); !hit {
			prev = t
			continue
		}

		// Straddled the surface between prev and t. Bisect for the first t
		// that is underground, then report the tile there: refining matters
		// because the step that found the hit can be most of a tile past the
		// edge the player actually clicked.
		lo, hi := prev, t
		for i := 0; i < pickRefine; i++ {
			mid := (lo + hi) / 2
			if _, hit := inside(mid); hit {
				hi = mid
			} else {
				lo = mid
			}
		}
		a, hit := inside(hi)
		return a, hit
	}

	return hex.Axial{}, false
}
