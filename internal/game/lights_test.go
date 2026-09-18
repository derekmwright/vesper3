package game

import (
	"math"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/derekmwright/vesper3/internal/artcheck"
	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

func TestCondenserSweepLoopsAndFadesAtTheSeam(t *testing.T) {
	for trail := 0; trail < condenserPulseTrail; trail++ {
		phase, alpha := condenserSweep(.73, hex.Axial{Q: 2, R: -1}, trail)
		nextPhase, nextAlpha := condenserSweep(.73+condenserPulsePeriod, hex.Axial{Q: 2, R: -1}, trail)
		if math.Abs(float64(phase-nextPhase)) > 1e-6 || math.Abs(float64(alpha-nextAlpha)) > 1e-6 {
			t.Fatal("sweep is not periodic")
		}
	}
	_, bottom := condenserSweep(0, hex.Axial{}, 0)
	_, top := condenserSweep(condenserPulsePeriod-.001, hex.Axial{}, 0)
	phase, middle := condenserSweep(condenserPulsePeriod/2, hex.Axial{}, 0)
	if bottom != 0 || top > .001 || middle != 1 || math.Abs(float64(phase-.5)) > 1e-6 {
		t.Fatalf("loop seam is not faded: bottom=%g top=%g middle=%g", bottom, top, middle)
	}
	for step := 0; step < 100; step++ {
		for trail := 0; trail < condenserPulseTrail; trail++ {
			p, a := condenserSweep(float32(step)*.11, hex.Axial{Q: -3, R: 7}, trail)
			if p < 0 || p >= 1 || a < 0 || a > 1 {
				t.Fatal("pulse escaped fin bounds or alpha range")
			}
		}
	}
}

func TestCondenserPulseSpawnsMovesAndObeysPower(t *testing.T) {
	m, err := world.NewMap(2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := colony.New()
	at := hex.Axial{}
	c.Buildings = []colony.Building{{Kind: colony.Condenser, At: at, Yield: 1}}
	c.Reindex()
	c.Readout.PowerDemand = 4
	c.Readout.Satisfaction = 1
	g := &Game{Map: m, Colony: c, scene: scene{structParts: map[colony.Kind][]meshPart{colony.Condenser: {
		{Name: "painted surfaces", Color: white, Scale: modelScale},
		{Name: artcheck.LampName, Color: [3]float32{1, .43, .026}, Scale: modelScale},
		{Name: artcheck.CondenserPulseName, Color: [3]float32{.015, .65, .35}, Scale: modelScale, CondenserPulse: true},
	}}, buildingEnt: make(map[hex.Axial][]glyph.Entity)}, elapsed: condenserPulsePeriod / 2}
	e := &glyph.Engine{Scene: glyph.NewScene()}
	g.spawnBuilding(e, colony.Condenser, at)
	ents := g.scene.buildingEnt[at]
	if len(ents) != 5 {
		t.Fatalf("got %d entities, want body, amber and three sweep strips", len(ents))
	}
	g.updateCondenserPulses(e)
	var previousY float32 = 100
	for _, ent := range ents[2:] {
		if _, ok := e.C.Emissive.Get(ent); !ok {
			t.Fatal("pulse is not emissive")
		}
		if _, ok := e.C.NoCastShadow.Get(ent); !ok {
			t.Fatal("pulse casts shadows")
		}
		if _, ok := e.C.Static.Get(ent); ok {
			t.Fatal("animated strip was registered as static")
		}
		if _, ok := e.C.Hidden.Get(ent); ok {
			t.Fatal("powered mid-cycle pulse is hidden")
		}
		tr, _ := e.C.Transform.Get(ent)
		if tr.Position[1] >= previousY {
			t.Fatal("trail does not follow below the leading edge")
		}
		previousY = tr.Position[1]
	}
	lead, _ := e.C.Transform.Get(ents[2])
	wantY := condenserPulseTravel * .5 * modelScale
	if math.Abs(float64(lead.Position[1]-wantY)) > 1e-6 {
		t.Fatalf("lead Y=%g want %g", lead.Position[1], wantY)
	}
	c.Readout.Satisfaction = .5
	g.updateCondenserPulses(e)
	alpha, _ := e.C.Translucent.Get(ents[2])
	if alpha.Alpha != .5 {
		t.Fatalf("brownout alpha=%g, want .5", alpha.Alpha)
	}
	c.Readout.Satisfaction = 0
	g.updateCondenserPulses(e)
	for _, ent := range ents[2:] {
		if _, ok := e.C.Hidden.Get(ent); !ok {
			t.Fatal("pulse remains visible in blackout")
		}
	}
	// Trails are owned by the same list demolition/load already despawn.
	for _, ent := range ents {
		e.Despawn(ent)
	}
	for _, ent := range ents {
		if _, ok := e.C.Transform.Get(ent); ok {
			t.Fatal("orphaned pulse after building despawn")
		}
	}
}

func TestPulseMarkerDoesNotCaptureBodyOrAmber(t *testing.T) {
	if !isCondenserPulse(artcheck.CondenserPulseName) || isCondenserPulse("painted surfaces") || isCondenserPulse(artcheck.LampName) {
		t.Fatal("pulse material classification is wrong")
	}
}

func TestCondenserNightLightUsesTealAndDimsWithPower(t *testing.T) {
	m, err := world.NewMap(2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := colony.New()
	c.Buildings = []colony.Building{{Kind: colony.Condenser, At: hex.Axial{}, Yield: 1}}
	c.Reindex()
	c.Readout.PowerDemand = 4
	c.Readout.Satisfaction = 1
	g := &Game{
		Map: m, Colony: c, cam: NewCamera(mgl32.Vec3{}),
		scene:   scene{structParts: map[colony.Kind][]meshPart{colony.Condenser: {{CondenserPulse: true}}}},
		elapsed: condenserPulsePeriod / 2,
	}
	e := &glyph.Engine{Scene: glyph.NewScene()}
	g.updateLights(e, -.3)
	if len(g.lights.points) != 1 {
		t.Fatalf("got %d lights, want 1", len(g.lights.points))
	}
	full := g.lights.points[0]
	if full.Color[1] <= full.Color[2] || full.Color[2] <= full.Color[0] || full.Color[1] < .5 {
		t.Fatal("condenser spill light is not bright teal")
	}
	if full.Range != condenserLightRange {
		t.Fatal("condenser light range changed")
	}
	c.Readout.Satisfaction = .5
	g.updateLights(e, -.3)
	if math.Abs(float64(g.lights.points[0].Color[1]/full.Color[1]-.5)) > 1e-6 {
		t.Fatal("night light does not dim with grid power")
	}
	c.Readout.Satisfaction = 0
	g.updateLights(e, -.3)
	if g.lights.level != 0 {
		t.Fatal("blackout keeps the light enabled")
	}
}

// lampLevel multiplies dusk by the power grid, and the second half is the part
// worth pinning: it is what makes a blackout visible on the map instead of
// only in the readout.
func TestLampLevelNeedsBothDarknessAndPower(t *testing.T) {
	cases := []struct {
		name         string
		sun          float32
		demand       float64
		satisfaction float64
		wantLit      bool
	}{
		{"noon, powered", 0.9, 30, 1, false},
		{"noon, blackout", 0.9, 30, 0, false},
		{"night, powered", -0.3, 30, 1, true},
		{"night, blackout", -0.3, 30, 0, false},
		{"night, brownout", -0.3, 30, 0.5, true},
		{"dusk, powered", 0.05, 30, 1, true},

		// An idle grid reports satisfaction 1 because nothing is drawing
		// anything, which would otherwise light an empty map.
		{"night, nothing built", -0.3, 0, 1, false},

		// The case that matters since storage: a solar colony after dark
		// generates nothing at all and is running entirely off its bank. It
		// is lit, and testing supply instead of demand blacked out precisely
		// the colony that had solved the problem.
		{"night, running on the battery bank", -0.3, 12, 1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Game{Colony: colony.New()}
			g.Colony.Readout.PowerDemand = tc.demand
			g.Colony.Readout.Satisfaction = tc.satisfaction

			got := g.lampLevel(tc.sun)
			if lit := got > 0.001; lit != tc.wantLit {
				t.Errorf("lampLevel(%.2f) = %.3f (lit=%v), want lit=%v",
					tc.sun, got, lit, tc.wantLit)
			}
			if got < 0 || got > 1 {
				t.Errorf("lampLevel = %.3f, outside 0..1", got)
			}
		})
	}
}

// A brownout should dim the colony by the amount it is short, not switch it
// off — the grid model throttles rather than failing, and the lights should
// say the same thing.
func TestBrownoutDimsRatherThanExtinguishes(t *testing.T) {
	level := func(sat float64) float32 {
		g := &Game{Colony: colony.New()}
		g.Colony.Readout.PowerDemand = 30
		g.Colony.Readout.Satisfaction = sat
		return g.lampLevel(-0.3)
	}

	full, half, none := level(1), level(0.5), level(0)

	if !(full > half && half > none) {
		t.Errorf("not monotonic in satisfaction: full %.3f, half %.3f, none %.3f", full, half, none)
	}
	if none != 0 {
		t.Errorf("zero satisfaction still lit at %.3f", none)
	}
	if math.Abs(float64(half/full)-0.5) > 1e-5 {
		t.Errorf("half power gave %.3f of full brightness, want 0.5", half/full)
	}
}

// The lamps have to come up before it is fully dark, or they snap on at a
// moment the player reads as arbitrary.
func TestLampsRampThroughDusk(t *testing.T) {
	level := func(sun float32) float32 {
		g := &Game{Colony: colony.New()}
		g.Colony.Readout.PowerDemand = 30
		g.Colony.Readout.Satisfaction = 1
		return g.lampLevel(sun)
	}

	if got := level(0.4); got != 0 {
		t.Errorf("lamps at %.3f with the sun well up", got)
	}
	if got := level(lampDuskStart + 0.01); got != 0 {
		t.Errorf("lamps at %.3f before the ramp starts", got)
	}
	if got := level(lampDuskEnd - 0.01); got < 0.999 {
		t.Errorf("lamps only %.3f after the ramp ends", got)
	}

	// Monotonically rising as the sun goes down.
	prev := float32(0)
	for sun := float32(lampDuskStart); sun >= lampDuskEnd; sun -= 0.01 {
		got := level(sun)
		if got < prev-1e-6 {
			t.Fatalf("lamps dipped from %.4f to %.4f at sun %.3f", prev, got, sun)
		}
		prev = got
	}
}

// The accent that glows is chosen by material name, so a remodelled structure
// keeps working as long as it keeps the lamp material - whatever colour the
// artist gives it, and wherever it lands in the primitive order.
func TestFindLampPartPicksTheNamedLamp(t *testing.T) {
	parts := []meshPart{
		{Name: "painted surfaces", Color: [3]float32{0.30, 0.34, 0.40}},
		{Name: artcheck.LampName, Color: [3]float32{0.90, 0.62, 0.20}},
		{Name: "pad", Color: [3]float32{0.10, 0.11, 0.13}},
	}
	idx, base, ok := findLampPart(parts)
	if !ok {
		t.Fatal("no lamp found")
	}
	if idx != 1 {
		t.Errorf("chose part %d, want 1", idx)
	}
	if base != parts[1].Color {
		t.Errorf("base colour %v, want %v", base, parts[1].Color)
	}
}

// The name is the whole rule, so a part that merely looks like a lamp is not
// one. Under the colour heuristic this model would have lit its warmest part;
// that is exactly the guessing the name replaced.
func TestFindLampPartIgnoresAWarmPartThatIsNotTheLamp(t *testing.T) {
	parts := []meshPart{
		{Name: "painted surfaces", Color: [3]float32{0.30, 0.34, 0.40}},
		{Name: "rust", Color: [3]float32{0.90, 0.62, 0.20}}, // warm, but not the lamp
	}
	if _, _, ok := findLampPart(parts); ok {
		t.Error("lit a part that only looked like a lamp")
	}
}

// A model with no lamp material reports so, rather than nominating something.
func TestFindLampPartRejectsAModelWithNoLamp(t *testing.T) {
	parts := []meshPart{
		{Name: "painted surfaces", Color: [3]float32{0.30, 0.34, 0.40}},
		{Name: "pad", Color: [3]float32{0.10, 0.11, 0.13}},
	}
	if _, _, ok := findLampPart(parts); ok {
		t.Error("found a lamp in a model that has none")
	}
}

func TestActivityGaugeTracksStoredCharge(t *testing.T) {
	g := &Game{Colony: colony.New()}
	g.Colony.Readout.Capacity = 100
	g.Colony.Readout.Stored = 50
	for i := 1; i <= 4; i++ {
		_, gain := g.batteryAppearance(hex.Axial{}, i)
		if (gain > 0) != (i <= 2) {
			t.Fatalf("50%% charge: segment %d gain %g", i, gain)
		}
	}
	g.Colony.Readout.Stored = 0
	_, gain := g.batteryAppearance(hex.Axial{}, 1)
	if gain != 0 {
		t.Fatal("empty gauge still lit")
	}
	g.Colony.Readout.Stored = 200
	if g.batteryLevel() != 1 {
		t.Fatal("charge is not clamped")
	}
	g.Colony.Readout.Capacity = 0
	if g.batteryLevel() != 0 {
		t.Fatal("zero capacity gauge")
	}
}

func TestActivityFixturesRespondToPowerAndCoolant(t *testing.T) {
	m, err := world.NewMap(2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := colony.New()
	at := hex.Axial{}
	c.Buildings = []colony.Building{{Kind: colony.Greenhouse, At: at, Yield: 1}}
	c.Reindex()
	c.Readout.PowerDemand = 4
	c.Readout.Satisfaction = 1
	g := &Game{Map: m, Colony: c, scene: scene{structParts: map[colony.Kind][]meshPart{colony.Greenhouse: {{Name: "painted surfaces", Color: white, Scale: modelScale}, {Name: artcheck.GrowLightName, Color: [3]float32{.015, .8, .25}, Scale: modelScale}}}, buildingEnt: make(map[hex.Axial][]glyph.Entity)}}
	e := &glyph.Engine{Scene: glyph.NewScene()}
	g.spawnBuilding(e, colony.Greenhouse, at)
	ent := g.scene.buildingEnt[at][1]
	g.updateBuildingActivity(e)
	if _, ok := e.C.Emissive.Get(ent); !ok {
		t.Fatal("powered grow fixture is dark")
	}
	col, _ := e.C.Color.Get(ent)
	full := col.G
	c.Readout.Satisfaction = .5
	g.updateBuildingActivity(e)
	if math.Abs(float64((col.G-.035)/(full-.035)-.5)) > 1e-5 {
		t.Fatal("grow fixture does not dim with brownout")
	}
	c.Readout.Satisfaction = 0
	g.updateBuildingActivity(e)
	if _, ok := e.C.Emissive.Get(ent); ok {
		t.Fatal("grow fixture on during blackout")
	}
	c.Readout.Coolant = 1
	if g.furnaceActivity(at) <= .8 {
		t.Fatal("cooled furnace is off on a blacked-out grid")
	}
	c.Readout.Coolant = 0
	if g.furnaceActivity(at) != 0 {
		t.Fatal("dry furnace glows")
	}
	for _, kind := range []colony.Kind{colony.Greenhouse, colony.Battery} {
		if activityMarker(kind, "painted surfaces") != 0 || activityMarker(kind, artcheck.LampName) != 0 {
			t.Fatal("marker captured body or amber")
		}
	}
}

func steamTestGame(t *testing.T) *Game {
	t.Helper()
	m, err := world.NewMap(12, 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := colony.New()
	c.Buildings = []colony.Building{{Kind: colony.Geothermal, At: hex.Axial{}, Yield: 1}}
	c.Reindex()
	c.Readout.Coolant = 1
	return &Game{Map: m, Colony: c, scene: newScene(), cam: NewCamera(mgl32.Vec3{}), daylight: 1}
}

func TestSteamTracksStackFacingAndElevation(t *testing.T) {
	g := steamTestGame(t)
	at := hex.Axial{}
	g.Map.Tiles[0].Elevation = 7
	base := g.steamStackPosition(at, 0)
	for facing := uint8(0); facing < 6; facing++ {
		got := g.steamStackPosition(at, facing)
		x, z := g.Map.Center(at)
		local := mgl32.HomogRotate3DY(facingYaw(facing)).Mul4x1(mgl32.Vec4{0, steamStackHeight * modelScale, steamStackOffsetZ * modelScale, 1})
		want := mgl32.Vec3{x + local[0], g.Map.SurfaceY(at) + local[1] + .012, z + local[2]}
		if got.Sub(want).Len() > 1e-5 {
			t.Fatalf("facing %d: %v want %v", facing, got, want)
		}
	}
	if base[1] <= g.Map.SurfaceY(at)+1 {
		t.Fatal("steam is below the chimney")
	}
}

func TestSteamStopsEmittingAndClearsOnDemolition(t *testing.T) {
	g := steamTestGame(t)
	at := hex.Axial{}
	g.stepSteam(.01)
	if len(g.scene.steam.emitters[at].puffs) != 1 {
		t.Fatal("cooled generator did not emit")
	}
	if got := g.stepSteam(.2); len(got) != steamLobes {
		t.Fatalf("got %d lobes", len(got))
	}
	puff := g.scene.steam.emitters[at].puffs[0]
	visible := appendSteamPuff(nil, puff, 1)
	for _, p := range visible {
		if p.Y <= puff.origin[1] {
			t.Fatal("steam is not rising")
		}
	}
	g.Colony.Readout.Coolant = 0
	for i := 0; i < 40; i++ {
		g.stepSteam(.1)
	}
	if len(g.scene.steam.emitters[at].puffs) != 0 || len(g.scene.steam.instances) != 0 {
		t.Fatal("dry plant continues venting")
	}
	g.Colony.Readout.Coolant = 1
	g.stepSteam(steamPeriod)
	g.scene.buildingEnt[at] = nil
	g.scene.despawnBuilding(&glyph.Engine{Scene: glyph.NewScene()}, at)
	if _, ok := g.scene.steam.emitters[at]; ok {
		t.Fatal("demolition leaked steam emitter")
	}
	g.Colony.Buildings = nil
	g.Colony.Reindex()
	if len(g.stepSteam(.1)) != 0 {
		t.Fatal("removed building left rendered steam")
	}
}

func TestSteamHasBoundedBudgetAndFades(t *testing.T) {
	g := steamTestGame(t)
	g.Colony.Buildings = nil
	for q := 0; q < 12; q++ {
		for row := 0; row < 12; row++ {
			at := world.FromOffset(q, row)
			g.Colony.Buildings = append(g.Colony.Buildings, colony.Building{Kind: colony.Geothermal, At: at, Yield: 1})
		}
	}
	g.Colony.Reindex()
	for i := 0; i < 100; i++ {
		if len(g.stepSteam(.1)) > steamMaxInstances {
			t.Fatal("GPU particle budget exceeded")
		}
	}
	if len(g.scene.steam.emitters) != steamMaxStacks {
		t.Fatal("stack cap not enforced")
	}
	g.Colony.Buildings = nil
	g.Colony.Reindex()
	g.stepSteam(.1)
	if len(g.scene.steam.emitters) != 0 {
		t.Fatal("removed stacks leaked")
	}
	if len(appendSteamPuff(nil, steamPuff{age: 0, strength: 1}, 1)) != 0 || len(appendSteamPuff(nil, steamPuff{age: steamLifetime, strength: 1}, 1)) != 0 {
		t.Fatal("puff pops at its lifetime boundary")
	}
	day := appendSteamPuff(nil, steamPuff{age: 1, strength: 1}, 1)
	night := appendSteamPuff(nil, steamPuff{age: 1, strength: 1}, 0)
	if len(day) == 0 || night[0].R >= day[0].R {
		t.Fatal("steam is not subdued at night")
	}
}

func TestBatteryStatusAndIdleAnimation(t *testing.T) {
	g := &Game{Colony: colony.New()}
	at := hex.Axial{}
	g.Colony.Readout.Capacity = 100
	g.Colony.Readout.Stored = 100
	_, first := g.batteryAppearance(at, 1)
	g.elapsed = 1.2
	_, second := g.batteryAppearance(at, 1)
	if math.Abs(float64(first-second)) < .1 {
		t.Fatal("charged idle battery has no visible breathing")
	}
	g.Colony.Readout.Stored = 0
	g.Colony.Readout.PowerDemand = 4
	g.Colony.Readout.Satisfaction = 1
	_, status := g.batteryAppearance(at, 5)
	if status <= 0 {
		t.Fatal("powered empty bank has no status heartbeat")
	}
	for i := 1; i <= 4; i++ {
		_, gain := g.batteryAppearance(at, i)
		if gain != 0 {
			t.Fatal("status light falsely filled an empty gauge")
		}
	}
	g.Colony.Readout.Satisfaction = 0
	_, status = g.batteryAppearance(at, 5)
	if status != 0 {
		t.Fatal("unpowered empty bank heartbeat remains on")
	}
	if activityMarker(colony.Battery, "Charge_Runtime_5_Status") != 5 {
		t.Fatal("status marker is not wired")
	}
}

func TestBatteryChaseReversesWithEnergyFlow(t *testing.T) {
	g := &Game{Colony: colony.New()}
	g.Colony.Readout.Capacity = 100
	g.Colony.Readout.Stored = 100
	g.elapsed = 1.24
	at := hex.Axial{}
	g.Colony.Readout.ChargeRate = 2
	tint, charging := g.batteryAppearance(at, 2)
	if tint[1] <= tint[2] {
		t.Fatal("charging gauge is not mint")
	}
	g.Colony.Readout.ChargeRate = -2
	g.elapsed = 1.24
	tint, discharging := g.batteryAppearance(at, 2)
	if tint[2] <= tint[1] {
		t.Fatal("discharge gauge is not cyan")
	}
	if charging-discharging < .7 {
		t.Fatal("charge and discharge do not reverse the segment chase")
	}
	lo, hi := float32(100), float32(0)
	for i := 0; i < 24; i++ {
		g.elapsed = float32(i) * .1
		_, gain := g.batteryAppearance(at, 2)
		if gain < lo {
			lo = gain
		}
		if gain > hi {
			hi = gain
		}
	}
	if hi-lo < .7 {
		t.Fatal("active chase is too subtle")
	}
}
