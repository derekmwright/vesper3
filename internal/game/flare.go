package game

import (
	"math"
	"sort"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	flareMaxStacks    = 64
	flareParticles    = 14
	flareMaxInstances = flareMaxStacks * flareParticles
)

// Tier 1 authored stack lip: Blender (-.40, .53, 1.66), scaled .8/1.17
// and converted to glTF Y-up. This is a visual vent, not simulated fuel loss.
var methaneStackLocal = mgl32.Vec3{-.27350428, 1.1350428, -.36239317}

func (g *Game) methaneStackPosition(at hex.Axial, facing uint8) mgl32.Vec3 {
	x, z := g.Map.Center(at)
	local := mgl32.HomogRotate3DY(facingYaw(facing)).Mul4x1(mgl32.Vec4{methaneStackLocal[0] * modelScale, methaneStackLocal[1] * modelScale, methaneStackLocal[2] * modelScale, 1})
	return mgl32.Vec3{x + local[0], g.Map.SurfaceY(at) + local[1] + .008, z + local[2]}
}

func (g *Game) methaneHasStack(tier uint8) bool {
	// Unknown future tier art needs its own socket; fallback tier-1 art uses this one.
	if tier > 1 {
		if _, ok := g.scene.structParts[partKey{colony.Methane, tier}]; ok {
			return false
		}
	}
	_, _, found := findLampPart(g.scene.partsFor(colony.Methane, tier))
	return found
}

func methaneFlare(t float32, at hex.Axial) float32 {
	h := uint32(at.Q)*73856093 ^ uint32(at.R)*19349663
	period := 12.0 + float64(h%501)/100
	phase := math.Mod(math.Max(0, float64(t))+float64((h>>9)%1000)/1000*period, period)
	// Two seconds of release, with soft ignition and extinction, followed by a long rest.
	envelope := smoothstep32(0, .28, float32(phase)) * (1 - smoothstep32(1.3, 2.2, float32(phase)))
	flicker := .86 + .09*float32(math.Sin(float64(t)*27+float64(h%31))) + .05*float32(math.Sin(float64(t)*43))
	return envelope * flicker
}

// Stateless sampling avoids catch-up bursts, emitter leaks and consuming the
// simulation RNG. Methane generation is firm power in the current economy:
// it does not stop when grid satisfaction or staffing falls.
func (g *Game) stepBuildingParticles(dt float32) []renderer.ParticleInstance {
	dst := g.scene.buildingParticles[:0]
	dst = append(dst, g.stepSteam(dt)...)
	candidates := g.scene.flareCandidates[:0]
	eye := g.cam.Eye()
	for _, b := range g.Colony.Buildings {
		if b.Kind != colony.Methane || !g.methaneHasStack(b.Tier()) {
			continue
		}
		if methaneFlare(g.elapsed, b.At) <= .001 {
			continue
		}
		origin := g.methaneStackPosition(b.At, b.Facing)
		candidates = append(candidates, steamCandidate{b.At, b.Facing, eye.Sub(origin).LenSqr()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if a.at.Q != b.at.Q {
			return a.at.Q < b.at.Q
		}
		return a.at.R < b.at.R
	})
	if len(candidates) > flareMaxStacks {
		candidates = candidates[:flareMaxStacks]
	}
	for _, c := range candidates {
		origin := g.methaneStackPosition(c.at, c.facing)
		dst = appendMethaneFlame(dst, origin, g.elapsed, methaneFlare(g.elapsed, c.at))
	}
	g.scene.flareCandidates = candidates
	g.scene.buildingParticles = dst
	return dst
}

func appendMethaneFlame(dst []renderer.ParticleInstance, origin mgl32.Vec3, t, strength float32) []renderer.ParticleInstance {
	if strength <= .001 {
		return dst
	}
	for i := 0; i < flareParticles; i++ {
		u := float32(i) / float32(flareParticles-1)
		height := u * (.16 + .21*strength)
		sway := u * u * .035
		x := origin[0] + sway*float32(math.Sin(float64(t*19+u*7)))
		z := origin[2] + sway*.65*float32(math.Cos(float64(t*15+u*9)))
		// Blue ignition near the nozzle transitions to a narrow warm flame tip.
		r, g, b := float32(1), float32(.40+.20*(1-u)), float32(.06)
		if i < 3 {
			r, g, b = .14, .43, 1
		}
		size := (.065*(1-u) + .05) * (.8 + .2*strength)
		alpha := (.30 - .28*u) * strength
		dst = append(dst, renderer.ParticleInstance{X: x, Y: origin[1] + height + .016, Z: z, Size: size, R: r, G: g, B: b, A: alpha})
	}
	return dst
}
