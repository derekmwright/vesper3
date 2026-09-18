package game

import (
	"math"
	"sort"

	"github.com/derekmwright/glyphengine/renderer"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/vesper3/internal/artcheck"
	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
)

// What a structure looks like while it is working.
//
// This is the other half of the lighting pass, and it is a different question
// from the one lights.go answers. That file asks "is it dark" and lights the
// colony accordingly. This one asks "is this building doing its job" and
// answers on the building itself: a condenser's fin band sweeps while it is
// drawing water, a battery bank's cells fill as it charges, a furnace glows
// with how hard it is running, a greenhouse shows its grow lamps.
//
// They are separate because they change for different reasons. Dusk is one
// number for the whole colony, sampled from the sun. Activity is per building,
// read from what that building is doing this tick, and stays on at noon.

// Condenser art controls. Geometry values match the fin-band primitive authored
// in 5-condenser.glb; the other values can be adjusted without rebuilding art.
const (
	condenserPulsePeriod     = 3.2 // seconds for one bottom-to-top sweep
	condenserPulseTravel     = float32((1.95 - .895) * .8 / .91)
	condenserPulseBase       = float32(.895 * .8 / .91)
	condenserPulseTrail      = 3    // one leading strip and two fading copies
	condenserPulseTrailLag   = .042 // fraction of a cycle between copies
	condenserPulseBrightness = float32(3.0)
	condenserLightRange      = float32(3.2)
	condenserLightIntensity  = float32(.80)
)

var condenserPulseColor = mgl32.Vec3{.025, .95, .62}

// isCondenserPulse reports the fin band that sweeps while a condenser is
// drawing water. Neither the painted body nor the amber lamp can accidentally
// become one; see internal/artcheck for the names.
func isCondenserPulse(name string) bool { return artcheck.IsCondenserPulse(name) }

func condenserPulsePart(parts []meshPart) int {
	for i, p := range parts {
		if p.CondenserPulse {
			return i
		}
	}
	return -1
}

func configureCondenserPulse(e *glyph.Engine, ent glyph.Entity) {
	e.C.Emissive.Set(ent, &glyph.Emissive{})
	e.C.NoCastShadow.Set(ent, &glyph.NoCastShadow{})
	e.C.DoubleSided.Set(ent, &glyph.DoubleSided{})
	e.C.Translucent.Set(ent, &glyph.Translucent{Alpha: 0})
	e.C.Hidden.Set(ent, &glyph.Hidden{})
	// Runtime emission uses the simple emissive/translucent path. PBR surfaces
	// would route elsewhere and ignore alpha in this engine version.
	e.C.MaterialRef.Remove(ent)
}

func (g *Game) addCondenserPulseTrail(e *glyph.Engine, ents []glyph.Entity) []glyph.Entity {
	idx := condenserPulsePart(g.scene.structParts[colony.Condenser])
	if idx < 0 || idx >= len(ents) {
		return ents
	}
	lead := ents[idx]
	transform, hasTransform := e.C.Transform.Get(lead)
	mesh, hasMesh := e.C.MeshRef.Get(lead)
	if !hasTransform || !hasMesh {
		return ents
	}
	for i := 1; i < condenserPulseTrail; i++ {
		ent := e.Spawn()
		t, m := *transform, *mesh
		e.C.Transform.Set(ent, &t)
		e.C.MeshRef.Set(ent, &m)
		e.C.Color.Set(ent, &glyph.Color{})
		configureCondenserPulse(e, ent)
		ents = append(ents, ent)
	}
	return ents
}

func condenserSweep(elapsed float32, at hex.Axial, trail int) (float32, float32) {
	// Nearby condensers have their own cycle; every fin on one condenser agrees.
	h := uint32(at.Q)*0x9E3779B1 ^ uint32(at.R)*0x85EBCA77
	offset := float64(h&1023) / 1024
	p := math.Mod(float64(elapsed)/condenserPulsePeriod+offset-float64(trail)*condenserPulseTrailLag, 1)
	if p < 0 {
		p++
	}
	phase := float32(p)
	envelope := smoothstep32(0, .09, phase) * (1 - smoothstep32(.88, 1, phase))
	return phase, envelope
}

func (g *Game) updateCondenserPulses(e *glyph.Engine) {
	parts := g.scene.structParts[colony.Condenser]
	idx := condenserPulsePart(parts)
	if idx < 0 {
		return
	}
	power := float32(0)
	if g.Colony.Readout.PowerDemand > 0 {
		power = clampF(float32(g.Colony.Readout.Satisfaction), 0, 1)
	}
	for at, ents := range g.scene.buildingEnt {
		b, ok := g.Colony.At(at)
		if !ok || b.Kind != colony.Condenser || len(ents) < len(parts)+condenserPulseTrail-1 {
			continue
		}
		for trail := 0; trail < condenserPulseTrail; trail++ {
			partIndex := idx
			if trail > 0 {
				partIndex = len(parts) + trail - 1
			}
			ent := ents[partIndex]
			phase, envelope := condenserSweep(g.elapsed, at, trail)
			weight := float32(1)
			if trail == 1 {
				weight = .42
			} else if trail == 2 {
				weight = .15
			}
			alpha := envelope * weight * power
			if alpha <= .001 {
				e.C.Hidden.Set(ent, &glyph.Hidden{})
				continue
			}
			e.C.Hidden.Remove(ent)
			if transform, ok := e.C.Transform.Get(ent); ok {
				transform.Position[1] = g.Map.SurfaceY(at) + phase*condenserPulseTravel*transform.Scale[1]
			}
			if translucent, ok := e.C.Translucent.Get(ent); ok {
				translucent.Alpha = alpha
			}
			if color, ok := e.C.Color.Get(ent); ok {
				brightness := condenserPulseBrightness * (.8 + .2*g.lights.level)
				c := condenserPulseColor.Mul(brightness)
				color.R, color.G, color.B = c[0], c[1], c[2]
			}
		}
	}
}

// Activity art controls: quiet, building-specific rhythms instead of flashing.
var growLightColor = mgl32.Vec3{.18, 1, .46}

var furnaceColor = mgl32.Vec3{1, .31, .045}

// activityMarker reports which simulation-driven part a material identifies.
//
// The names live in internal/artcheck rather than here, because they are a
// contract with the art rather than with the renderer - and cmd/modelcheck has
// to read them without linking Vulkan to do it.
//
// This matched base colours with a 0.002 tolerance until glyphengine#21 made
// renderer.ModelMesh keep the material name. The tolerance was the fragile
// part: invisible in the art, not greppable, and a re-export a thousandth off
// loaded fine and silently stopped lighting up.
func activityMarker(kind colony.Kind, name string) int {
	return artcheck.Marker(kind, name)
}

func activityWave(elapsed float32, at hex.Axial, period float64) float32 {
	offset := float64((uint32(at.Q)*73^uint32(at.R)*157)&255) / 256
	return float32(.5 + .5*math.Sin(2*math.Pi*(float64(elapsed)/period+offset)))
}

func (g *Game) batteryLevel() float32 {
	if g.Colony.Readout.Cap.Power <= 0 {
		return 0
	}
	return clampF(float32(g.Colony.Readout.Stored/g.Colony.Readout.Cap.Power), 0, 1)
}

// batteryStatus drives the pilot lamp on the bank's crown. It answers a
// different question from the fill strips below it: not how much is stored,
// but what is happening to it.
//
// The strips cannot say that. A bank sitting at 60% looks the same whether it
// is filling in the last of the daylight or draining into the night, and which
// of those it is doing is the thing worth knowing at dusk — it is the
// difference between a colony that will get through and one that will not.
func (g *Game) batteryStatus(at hex.Axial) (mgl32.Vec3, float32) {
	r := g.Colony.Readout
	charge := g.batteryLevel()

	// A slow breath while something is happening, steady when it is not. The
	// rate threshold is the economy's own, so a bank the ledger calls flat
	// does not blink from rounding noise.
	breathe := func(period float64) float32 {
		return .72 + .28*activityWave(g.elapsed, at, period)
	}

	switch {
	case r.Cap.Power <= 0:
		return batteryFlatColor, 0

	case r.ChargeRate > colony.RateEpsilon:
		// Filling: green, and quicker the closer it is to done.
		return batteryChargingColor, 1.5 * breathe(3.2)

	case r.ChargeRate < -colony.RateEpsilon:
		// Draining: amber, and urgent once there is little left. This is the
		// one state the lamp exists for.
		urgency := 1 - charge
		return batteryDrainingColor, (1.2 + .9*urgency) * breathe(1.4-.7*float64(urgency))

	case charge >= .999:
		// Full and holding: steady, no pulse. Nothing to watch.
		return batteryFullColor, 1.35

	case charge <= .001:
		return batteryFlatColor, .9 * breathe(.8)
	}

	// Holding a partial charge with nothing moving: present but quiet.
	return batteryIdleColor, .8
}

// The pilot lamp's four states. They are deliberately the colours the rest of
// the interface already uses for the same meanings — green for healthy, amber
// for a clock running, red for out — so the lamp reads without being learned.
var (
	batteryChargingColor = mgl32.Vec3{.08, 1, .45}
	batteryDrainingColor = mgl32.Vec3{1, .62, .12}
	batteryFullColor     = mgl32.Vec3{.35, 1, .95}
	batteryIdleColor     = mgl32.Vec3{.06, .7, 1}
	batteryFlatColor     = mgl32.Vec3{1, .3, .12}
)

func (g *Game) batteryAppearance(at hex.Axial, segment int) (mgl32.Vec3, float32) {
	charge := g.batteryLevel()
	tint := mgl32.Vec3{.06, .7, 1}
	if g.Colony.Readout.ChargeRate > .001 {
		tint = mgl32.Vec3{.08, 1, .5}
	}
	if charge <= .2 {
		tint = mgl32.Vec3{1, .37, .045}
	}

	// The fifth marker is a status LED, not another charge segment. It can
	// indicate a powered empty bank without suggesting stored energy.
	if segment == 5 {
		powered := g.Colony.Readout.Cap.Power > 0 && (charge > 0 || g.Colony.Readout.PowerSupply > .001 || (g.Colony.Readout.PowerDemand > 0 && g.Colony.Readout.Satisfaction > .001))
		if !powered {
			return tint, 0
		}
		wave := activityWave(g.elapsed, at, 2.4)
		return tint, .35 + .85*wave*wave*wave
	}
	amount := clampF(charge*artcheck.BatteryStrips-float32(segment-1), 0, 1)
	gain := amount * 2.0
	if amount > 0 {
		if math.Abs(g.Colony.Readout.ChargeRate) > .001 {
			// A clear chase on the filled segments reverses when discharging.
			direction := float32(1)
			if g.Colony.Readout.ChargeRate < 0 {
				direction = -1
			}
			wave := activityWave(g.elapsed-direction*float32(segment)*.32, at, 2.4)
			gain *= .55 + .45*wave*wave
		} else {
			// Charged, idle banks breathe gently rather than looking switched off.
			gain *= .80 + .20*activityWave(g.elapsed, at, 4.8)
		}
	}

	return tint, gain
}

func (g *Game) furnaceActivity(at hex.Axial) float32 {
	// Geothermal is a generator: coolant availability, not downstream grid
	// satisfaction, determines whether the furnace itself is running.
	coolant := clampF(float32(g.Colony.Readout.Coolant), 0, 1)
	return coolant * (.86 + .09*activityWave(g.elapsed, at, 3.7) + .05*activityWave(g.elapsed, at, 1.3))
}

func setActivityGlow(e *glyph.Engine, ent glyph.Entity, tint mgl32.Vec3, gain float32) {
	e.C.MaterialRef.Remove(ent)
	e.C.NoCastShadow.Set(ent, &glyph.NoCastShadow{})
	e.C.DoubleSided.Set(ent, &glyph.DoubleSided{})
	if gain > .001 {
		e.C.Emissive.Set(ent, &glyph.Emissive{})
	} else {
		e.C.Emissive.Remove(ent)
	}
	// Unlit glass remains as a dark fixture rather than disappearing.
	c := tint.Mul(.035 + gain)
	if color, ok := e.C.Color.Get(ent); ok {
		color.R, color.G, color.B = c[0], c[1], c[2]
	}
}

func (g *Game) updateBuildingActivity(e *glyph.Engine) {
	power := float32(0)
	if g.Colony.Readout.PowerDemand > 0 {
		power = clampF(float32(g.Colony.Readout.Satisfaction), 0, 1)
	}
	for at, ents := range g.scene.buildingEnt {
		b, ok := g.Colony.At(at)
		if !ok {
			continue
		}
		if b.Kind == colony.Geothermal {
			if idx, found := g.lights.part[b.Kind]; found && idx < len(ents) {
				setActivityGlow(e, ents[idx], furnaceColor, g.furnaceActivity(at)*(1.1+.8*g.lights.level))
			}
			continue
		}
		if b.Kind != colony.Greenhouse && b.Kind != colony.Battery {
			continue
		}
		for i, part := range g.scene.structParts[b.Kind] {
			if i >= len(ents) {
				break
			}
			marker := activityMarker(b.Kind, part.Name)
			if marker == 0 {
				continue
			}
			if b.Kind == colony.Battery {
				tint, gain := g.batteryAppearance(at, marker)
				if marker == artcheck.BatteryStatus {
					tint, gain = g.batteryStatus(at)
				}
				setActivityGlow(e, ents[i], tint, gain)
			} else {
				gain := power * (.65 + .85*g.lights.level) * (.94 + .06*activityWave(g.elapsed, at, 7))
				setActivityGlow(e, ents[i], growLightColor, gain)
			}
		}
	}
}

func (g *Game) activitySpill(kind colony.Kind, at hex.Axial) (mgl32.Vec3, float32, float32, bool) {
	switch kind {
	case colony.Greenhouse:
		return growLightColor, .32 * (.94 + .06*activityWave(g.elapsed, at, 7)), 2.8, true
	case colony.Geothermal:
		return furnaceColor, .48 * g.furnaceActivity(at), 3.0, true
	case colony.Battery:
		tint, _ := g.batteryAppearance(at, 1)
		return tint, .18 * g.batteryLevel(), 2.0, true
	}
	return mgl32.Vec3{}, 0, 0, false
}

// Steam uses the engine's soft billboard particle pass. Keep the plume small:
// three overlapping lobes per puff, bounded to the nearest 64 active stacks.
const (
	steamPeriod        = float32(.95)
	steamLifetime      = float32(2.8)
	steamMaxStacks     = 64
	steamPuffsPerStack = 5
	steamLobes         = 3
	steamMaxInstances  = steamMaxStacks * steamPuffsPerStack * steamLobes
	// Measured from the authored GLB, in glTF coordinates before modelScale.
	steamStackHeight  = float32(1.470488)
	steamStackOffsetZ = float32(-.2188034)
)

type steamPuff struct {
	origin               mgl32.Vec3
	age, strength, phase float32
}
type steamEmitter struct {
	puffs    []steamPuff
	cooldown float32
	serial   uint32
	seen     uint64
}
type steamCandidate struct {
	at       hex.Axial
	facing   uint8
	distance float32
}
type steamSystem struct {
	emitters   map[hex.Axial]*steamEmitter
	candidates []steamCandidate
	instances  []renderer.ParticleInstance
	frame      uint64
}

func (g *Game) steamStackPosition(at hex.Axial, facing uint8) mgl32.Vec3 {
	x, z := g.Map.Center(at)
	yaw := facingYaw(facing)
	offset := steamStackOffsetZ * modelScale
	return mgl32.Vec3{x + float32(math.Sin(float64(yaw)))*offset, g.Map.SurfaceY(at) + steamStackHeight*modelScale + .012, z + float32(math.Cos(float64(yaw)))*offset}
}

// stepSteam is independent of the renderer so lifecycle and placement can be
// checked headlessly. Already emitted puffs finish drifting after coolant stops.
func (g *Game) stepSteam(dt float32) []renderer.ParticleInstance {
	s := &g.scene.steam
	if s.emitters == nil {
		s.emitters = make(map[hex.Axial]*steamEmitter)
	}
	dt = max(dt, 0)
	s.frame++
	s.candidates = s.candidates[:0]
	eye := g.cam.Eye()
	for _, b := range g.Colony.Buildings {
		if b.Kind != colony.Geothermal {
			continue
		}
		x, z := g.Map.Center(b.At)
		s.candidates = append(s.candidates, steamCandidate{b.At, b.Facing, eye.Sub(mgl32.Vec3{x, g.Map.SurfaceY(b.At), z}).LenSqr()})
	}
	sort.Slice(s.candidates, func(i, j int) bool {
		a, b := s.candidates[i], s.candidates[j]
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if a.at.Q != b.at.Q {
			return a.at.Q < b.at.Q
		}
		return a.at.R < b.at.R
	})
	if len(s.candidates) > steamMaxStacks {
		s.candidates = s.candidates[:steamMaxStacks]
	}
	coolant := clampF(float32(g.Colony.Readout.Coolant), 0, 1)
	s.instances = s.instances[:0]
	for _, candidate := range s.candidates {
		at := candidate.at
		em := s.emitters[at]
		if em == nil {
			h := (uint32(at.Q)*73 ^ uint32(at.R)*157) & 255
			em = &steamEmitter{cooldown: float32(h) / 256 * steamPeriod, puffs: make([]steamPuff, 0, steamPuffsPerStack)}
			s.emitters[at] = em
		}
		em.seen = s.frame
		live := em.puffs[:0]
		for _, p := range em.puffs {
			p.age += dt
			if p.age < steamLifetime {
				live = append(live, p)
			}
		}
		em.puffs = live
		if coolant > .01 && dt > 0 {
			em.cooldown -= dt
			if em.cooldown <= 0 && len(em.puffs) < steamPuffsPerStack {
				em.serial++
				phase := float32((em.serial*97+uint32(at.Q)*31+uint32(at.R)*71)&255) / 256 * 2 * math.Pi
				em.puffs = append(em.puffs, steamPuff{origin: g.steamStackPosition(at, candidate.facing), strength: coolant, phase: phase})
				// No catch-up burst after a stalled frame.
				em.cooldown = steamPeriod / max(coolant, .15)
			}
		}
		for _, p := range em.puffs {
			s.instances = appendSteamPuff(s.instances, p, float32(g.daylight))
		}
	}
	for at, em := range s.emitters {
		if em.seen != s.frame {
			delete(s.emitters, at)
		}
	}
	return s.instances
}

func appendSteamPuff(dst []renderer.ParticleInstance, p steamPuff, daylight float32) []renderer.ParticleInstance {
	age := p.age
	fade := smoothstep32(0, .18, age) * (1 - smoothstep32(1.1, steamLifetime, age))
	alpha := .24 * fade * p.strength
	if alpha <= .001 {
		return dst
	}
	size := .19 + .16*age
	center := p.origin.Add(mgl32.Vec3{.085*age + .022*float32(math.Sin(float64(p.phase+age))), .32*age + .018*age*age, -.035 * age})
	// The particle pass is additive: use low, daylight-aware values to read as
	// pale vapor, not a white lamp, especially after dark.
	brightness := .22 + .48*clampF(daylight, 0, 1)
	for lobe := 0; lobe < steamLobes; lobe++ {
		a := p.phase + float32(lobe)*2*math.Pi/steamLobes
		radius := size * .23
		pos := center.Add(mgl32.Vec3{float32(math.Cos(float64(a))) * radius, float32(lobe-1) * size * .12, float32(math.Sin(float64(a))) * radius})
		dst = append(dst, renderer.ParticleInstance{X: pos[0], Y: pos[1], Z: pos[2], Size: size * (.82 + .08*float32(lobe)), R: brightness * .87, G: brightness * .94, B: brightness, A: alpha})
	}
	return dst
}
