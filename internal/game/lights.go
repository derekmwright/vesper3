package game

import (
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/vesper3/internal/artcheck"
	"github.com/derekmwright/vesper3/internal/colony"
)

// Colony lighting.
//
// Two things happen together as the sun goes down: a warm point light appears
// at each structure, and the structure's own amber accent starts to glow. The
// light is what falls on the ground around it; the glow is what makes the
// building itself read as occupied rather than as a shape with a lamp next to
// it.
//
// Both are scaled by the same number, and that number includes the power
// grid — so a colony that cannot cover its own demand after dark simply does
// not light up. That is the whole solar-versus-geothermal lesson made visible
// on the map instead of only in the readout: build nothing but solar arrays
// and night falls on a colony that goes completely dark.
const (
	// The sun elevations the lamps fade between. They start coming on while
	// the sun is still just above the horizon, because that is when a real
	// settlement switches its lights on, and reach full a little after it
	// sets.
	lampDuskStart = 0.18
	lampDuskEnd   = -0.04

	// lampRange is how far a structure's light reaches, in world units. A tile
	// is two across, so this pools light over the building and the tiles
	// around it without washing the whole colony into one blob.
	//
	// It was 4.0 while the engine's ceiling was 32 lights and every lamp had
	// to earn its place. Clustered lighting raised that to 1024 and the cost
	// was measured rather than feared: at 9.0 a demo colony puts seven lights
	// screen-wide out of a thousand, and the worst cell holds twelve of a
	// hundred and twenty-eight. The tuning was never near the budget.
	lampRange = 9.0

	// lampIntensity scales each lamp. It is low because these accumulate:
	// point lights are unshadowed and additive, so a dozen structures at the
	// magnitude the engine's lighting example uses for a handful blew the
	// ground around the colony out to flat white and turned the buildings
	// into silhouettes against it. One lamp should light its own tile and
	// tint its neighbours, and the sum of a colony should still be night.
	//
	// That ceiling is real and this sits under it rather than at it: 0.45 with
	// a 12-unit range flattens the ground to a pale sheet and the night stops
	// being night. 0.30 at 9.0 is the widest, brightest pool that still falls
	// off to dark ground inside the frame.
	//
	// Provisional. The plan is a lamp on a pole casting an actual spot, at
	// which point this light stops being what illuminates a building and
	// becomes the scattered glow around one - and a glow doing a key light's
	// job is exactly what this number is currently set to.
	lampIntensity = 0.30

	// lampGlowFloor is how bright an accent is when the lamps first come on,
	// so the transition starts from a visible ember rather than from nothing.
	lampGlowFloor = 0.55
	lampGlowGain  = 1.45
)

// lampWarm is the colour of colony lighting: sodium-ish, and deliberately
// nothing like the blue-white daylight, so a lit colony reads as warm against
// cold ground.
var lampWarm = mgl32.Vec3{1.00, 0.68, 0.34}

// lighting is the dusk lamp pass's state.
//
// part and base are resolved once at load and never change: which mesh part of
// a structure is its accent, and what colour that part is in daylight. The
// rest is per-frame, and points and scratch are retained buffers rather than
// values — this runs every frame over every building, and allocating two
// slices a frame to light a colony would be the one place this game generates
// garbage.
type lighting struct {
	part map[partKey]int
	base map[partKey][3]float32

	// level is the current lamp brightness, 0 to 1, kept for the debug
	// readout.
	level float32
	on    bool

	points  []glyph.PointLight
	scratch []lampCandidate
}

func newLighting() lighting {
	return lighting{
		part: make(map[partKey]int),
		base: make(map[partKey][3]float32),
	}
}

// lampLevel is how brightly the colony is lit, from 0 to 1.
//
// It is the product of dusk and power, which is the point: either one at zero
// means darkness. A blackout at noon costs nothing visible because the lamps
// were not on anyway; a blackout at midnight puts the colony out.
func (g *Game) lampLevel(sunElevation float32) float32 {
	dusk := smoothstep32(lampDuskStart, lampDuskEnd, sunElevation)
	if dusk <= 0 {
		return 0
	}

	// A grid with nothing drawing on it has nothing running lights either,
	// and satisfaction reports 1 for an idle colony — so demand, not supply,
	// is what says there is a colony here at all.
	//
	// It has to be demand rather than supply: a solar colony running off its
	// battery bank after dark generates nothing and is still lit, which is the
	// entire point of having bought the batteries. Testing supply here blacked
	// out exactly the colony that had solved the problem.
	if g.Colony.Readout.PowerDemand <= 0 {
		return 0
	}

	// Power satisfaction is the fraction of demand the grid actually met —
	// generation plus whatever the bank covered — so a brownout dims the
	// colony by exactly the amount it is short.
	power := float32(g.Colony.Readout.Satisfaction)
	return dusk * clampF(power, 0, 1)
}

// updateLights places the colony's lamps for this frame.
func (g *Game) updateLights(e *glyph.Engine, sunElevation float32) {
	level := g.lampLevel(sunElevation)
	g.lights.level = level

	g.updateLampGlow(e, level)
	g.updateCondenserPulses(e)
	g.updateBuildingActivity(e)

	if level <= 0.001 {
		// Nothing lit: hand the scene an empty set rather than 32 black
		// lights, which would cost the fragment shader a loop for no effect.
		e.Scene.SetPointLights(nil)
		return
	}

	g.gatherLamps(level, g.cam.Eye())

	// The engine takes at most renderer.MaxLights and truncates the rest, so
	// choose which ones survive rather than letting slice order decide: the
	// nearest are the ones whose light the player can actually see.
	//
	// That ceiling was 32 when this was written, which a mid-game colony hit
	// and a late one sailed past - so this was the difference between a lit
	// colony and a lit corner of one. Clustered lighting raised it to 1024,
	// which no colony this game can build will reach, so the sort now costs
	// nothing and does nothing. It stays because "nothing" is a fact about
	// today's budget rather than a rule: the engine is free to lower it again,
	// and dropping the far lights is still the right answer if it does.
	if len(g.lights.scratch) > maxLamps {
		sort.Slice(g.lights.scratch, func(i, j int) bool {
			return g.lights.scratch[i].dist < g.lights.scratch[j].dist
		})
		g.lights.scratch = g.lights.scratch[:maxLamps]
	}

	g.lights.points = g.lights.points[:0]
	for _, c := range g.lights.scratch {
		g.lights.points = append(g.lights.points, c.light)
	}
	e.Scene.SetPointLights(g.lights.points)
}

// maxLamps is the engine's light budget, read from the engine rather than
// copied from it.
//
// It was the literal 32 for as long as that was true, and stayed 32 for a
// while after it was not: the engine went to 1024 and this game went on
// rationing lamps to a ceiling three per cent of the real one, with nothing
// to notice because a hardcoded number cannot go stale loudly. Referencing
// the constant is what makes the next change to it arrive here for free.
const maxLamps = renderer.MaxLights

type lampCandidate struct {
	light glyph.PointLight
	dist  float32
}

// gatherLamps fills the candidate list with every light the colony is casting
// this frame, in no particular order.
//
// Split out of updateLights so it can be exercised without a renderer: it
// needs a map, a colony and a camera position, and none of those need a GPU.
// What it decides - how many lights a building contributes, and where they sit
// - is the part worth pinning.
func (g *Game) gatherLamps(level float32, eye mgl32.Vec3) {
	g.lights.scratch = g.lights.scratch[:0]

	for _, b := range g.Colony.Buildings {
		x, z := g.Map.Center(b.At)
		y := g.Map.SurfaceY(b.At)
		light := glyph.PointLight{
			Pos:   mgl32.Vec3{x, y + .55, z},
			Range: lampRange,
			Color: lampWarm.Mul(level * lampIntensity),
		}
		if b.Kind == colony.Condenser && condenserPulsePart(g.scene.partsFor(b.Kind, b.Tier())) >= 0 {
			phase, envelope := condenserSweep(g.elapsed, b.At, 0)
			light.Pos[1] = y + (condenserPulseBase+phase*condenserPulseTravel)*modelScale
			light.Range = condenserLightRange
			light.Color = condenserPulseColor.Mul(level * condenserLightIntensity * (.65 + .35*envelope))
		}

		if tint, gain, radius, ok := g.activitySpill(b.Kind, b.At); ok {
			light.Color = tint.Mul(level * gain)
			light.Range = radius
		}

		dist := eye.Sub(mgl32.Vec3{x, y, z}).LenSqr()
		g.lights.scratch = append(g.lights.scratch, lampCandidate{light: light, dist: dist})

		// The flare is a second light rather than a replacement for the first.
		//
		// It used to overwrite it - same variable, one light per building -
		// so for the two seconds a stack burned, the plant's deck lamp moved
		// up to the stack lip and its range fell from lampRange to a metre and
		// a half. The building went dark exactly when it was most obviously
		// doing something, which is the opposite of what the flare is for.
		//
		// One light per building was the rule when the engine held 32 of them
		// and every one had to earn its place. It holds 1024 now, and a colony
		// at fourteen lights can afford a second one on the handful of plants
		// that happen to be burning.
		if b.Kind == colony.Methane && g.methaneHasStack(b.Tier()) {
			if flare := methaneFlare(g.elapsed, b.At); flare > .001 {
				g.lights.scratch = append(g.lights.scratch, lampCandidate{
					light: glyph.PointLight{
						Pos:   g.methaneStackPosition(b.At, b.Facing).Add(mgl32.Vec3{0, flareLightLift, 0}),
						Range: flareLightRange,
						Color: flareLightColor.Mul(level * (flareLightFloor + flareLightGain*flare)),
					},
					dist: dist,
				})
			}
		}
	}
}

// lampPart and lampBase resolve a structure's accent for a given tier, falling
// back to its tier-1 art the same way partsFor does — a building drawn with
// tier-1 geometry has to be lit with tier-1 lamp indices, or the glow lands on
// whichever primitive happens to sit at that index.
func (l *lighting) lampPart(k colony.Kind, tier uint8) (int, bool) {
	if idx, ok := l.part[partKey{k, tier}]; ok {
		return idx, true
	}
	idx, ok := l.part[partKey{k, 1}]
	return idx, ok
}

func (l *lighting) lampBase(k colony.Kind, tier uint8) ([3]float32, bool) {
	if base, ok := l.base[partKey{k, tier}]; ok {
		return base, true
	}
	base, ok := l.base[partKey{k, 1}]
	return base, ok
}

// updateLampGlow makes each structure's amber accent light up.
//
// The glow is the accent part of the model drawn full-bright, which is what
// the Emissive tag does. The tag is added and removed on the transition rather
// than every frame — it is a component write per structure, and the brightness
// ramp is carried by the tint, which has to be written anyway.
func (g *Game) updateLampGlow(e *glyph.Engine, level float32) {
	lit := level > 0.02
	brightness := lampGlowFloor + lampGlowGain*level

	for at, ents := range g.scene.buildingEnt {
		b, ok := g.Colony.At(at)
		if !ok {
			continue
		}
		idx, has := g.lights.lampPart(b.Kind, b.Tier())
		if !has || idx >= len(ents) {
			continue
		}
		ent := ents[idx]

		switch {
		case lit && !g.lights.on:
			e.C.Emissive.Set(ent, &glyph.Emissive{})
		case !lit && g.lights.on:
			e.C.Emissive.Remove(ent)
		}

		if !lit {
			// Back to the material's own colour for daylight.
			if base, okBase := g.lights.lampBase(b.Kind, b.Tier()); okBase {
				if c, okCol := e.C.Color.Get(ent); okCol {
					c.R, c.G, c.B = base[0], base[1], base[2]
				}
			}
			continue
		}

		if base, okBase := g.lights.lampBase(b.Kind, b.Tier()); okBase {
			if c, okCol := e.C.Color.Get(ent); okCol {
				// Toward the lamp colour as it brightens, so a lit accent is
				// warmer than the metal it sits in rather than just lighter.
				c.R = base[0]*(1-level)*brightness + lampWarm[0]*level*brightness
				c.G = base[1]*(1-level)*brightness + lampWarm[1]*level*brightness
				c.B = base[2]*(1-level)*brightness + lampWarm[2]*level*brightness
			}
		}
	}

	g.lights.on = lit
}

// findLampPart picks the part a structure's dusk lamp is driven from.
//
// A name match, as of glyphengine#21. It used to be a heuristic - whichever
// part was warmest, reddest against blue and weighted by brightness - because
// the loader did not keep material names and appearance was the only handle
// there was. That worked, and degraded in the worst way available: a model
// with nothing warm in it simply never lit, and the symptom was an unexplained
// dark patch in a colony at night rather than anything that looked like a
// failure.
func findLampPart(parts []meshPart) (int, [3]float32, bool) {
	for i, p := range parts {
		if artcheck.IsLamp(p.Name) {
			return i, p.Color, true
		}
	}
	return 0, [3]float32{}, false
}

// smoothstep32 ramps between two edges, which may be given in either order so
// a falling quantity like sun elevation reads naturally.
func smoothstep32(edge0, edge1, x float32) float32 {
	if edge0 == edge1 {
		return 0
	}
	t := clampF((x-edge0)/(edge1-edge0), 0, 1)
	return t * t * (3 - 2*t)
}
