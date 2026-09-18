package colony

import (
	"errors"
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

func TestPlaceChargesOreAndRecordsTheBuilding(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()

	before := c.Iron
	at := world.FromOffset(4, 4)
	if err := c.Place(m, Habitat, at); err != nil {
		t.Fatalf("Place: %v", err)
	}

	if got, want := c.Iron, before-Of(Habitat).IronCost; got != want {
		t.Errorf("ore is %.1f, want %.1f", got, want)
	}
	b, ok := c.At(at)
	if !ok {
		t.Fatal("nothing recorded on the tile that was just built on")
	}
	if b.Kind != Habitat {
		t.Errorf("tile holds %v, want Habitat", b.Kind)
	}
}

func TestPlaceRejectsTheWaysItShould(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(2, 2)).Terrain = world.Sea
	m.At(world.FromOffset(3, 3)).Terrain = world.Dunes
	m.At(world.FromOffset(5, 5)).Terrain = world.Ice
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	occupied := world.FromOffset(4, 4)
	if err := c.Place(m, Habitat, occupied); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		kind Kind
		at   hex.Axial
		want error
	}{
		{"off the map", Habitat, hex.Axial{Q: -50, R: -50}, ErrOffMap},
		{"already built on", SolarArray, occupied, ErrOccupied},
		{"habitat on sea", Habitat, world.FromOffset(2, 2), ErrTerrain},
		{"mine on plain regolith", Mine, world.FromOffset(7, 7), ErrTerrain},
		{"extractor off ice", Extractor, world.FromOffset(3, 3), ErrTerrain},
		{"geothermal off a vent", Geothermal, world.FromOffset(3, 3), ErrTerrain},
		{"habitat on a vent", Habitat, world.FromOffset(6, 6), ErrTerrain},
		{"no such kind", Kind(200), world.FromOffset(8, 8), ErrUnknownKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.CanPlace(m, tc.kind, tc.at); !errors.Is(err, tc.want) {
				t.Errorf("CanPlace = %v, want %v", err, tc.want)
			}
		})
	}

	// And the ones that should be allowed.
	allowed := []struct {
		kind Kind
		at   hex.Axial
	}{
		{Mine, world.FromOffset(3, 3)},
		{Extractor, world.FromOffset(5, 5)},
		{Geothermal, world.FromOffset(6, 6)},
	}
	for _, a := range allowed {
		if err := c.CanPlace(m, a.kind, a.at); err != nil {
			t.Errorf("CanPlace(%v) = %v, want nil", a.kind, err)
		}
	}
}

// Found is how the landing site arrives: free, but still subject to every
// siting rule. A Found that skipped the terrain check would let the opening
// habitat come down on the methane sea.
func TestFoundIsFreeButStillObeysSiting(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(2, 2)).Terrain = world.Sea
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 0 // cannot afford anything at all

	at := world.FromOffset(4, 4)
	if err := c.Found(m, Habitat, at); err != nil {
		t.Fatalf("Found with no ore: %v", err)
	}
	if c.Iron != 0 {
		t.Errorf("Found charged %.1f ore", -c.Iron)
	}
	if b, ok := c.At(at); !ok || b.Kind != Habitat {
		t.Error("Found did not record the building")
	}

	// And the rules that are not about money still hold.
	if err := c.Found(m, Habitat, world.FromOffset(2, 2)); !errors.Is(err, ErrTerrain) {
		t.Errorf("Found on sea = %v, want ErrTerrain", err)
	}
	if err := c.Found(m, Habitat, at); !errors.Is(err, ErrOccupied) {
		t.Errorf("Found on an occupied tile = %v, want ErrOccupied", err)
	}
	if err := c.Found(m, Mine, world.FromOffset(5, 5)); !errors.Is(err, ErrTerrain) {
		t.Errorf("Found a mine off ore = %v, want ErrTerrain", err)
	}
	if err := c.Found(m, Habitat, hex.Axial{Q: -50, R: -50}); !errors.Is(err, ErrOffMap) {
		t.Errorf("Found off the map = %v, want ErrOffMap", err)
	}
}

// A founded structure has to be indistinguishable from a bought one
// afterwards, including the terrain yield it was sited for.
func TestFoundBuildingsBehaveLikeBuiltOnes(t *testing.T) {
	m := flatMap(t, world.Crystal)
	c := New()
	c.Iron = 0

	at := world.FromOffset(4, 4)
	if err := c.Found(m, Mine, at); err != nil {
		t.Fatal(err)
	}

	b, _ := c.At(at)
	if want := Yield(Mine, world.Crystal); b.Yield != want {
		t.Errorf("yield %.2f, want %.2f", b.Yield, want)
	}
	if got := c.Count(Mine); got != 1 {
		t.Errorf("Count(Mine) = %d, want 1", got)
	}

	// It refunds like anything else, which is how a player relocates the
	// landing site's solar array.
	if _, ok := c.Demolish(at); !ok {
		t.Fatal("a founded building could not be demolished")
	}
	if want := Of(Mine).IronCost * RefundFraction; math.Abs(c.Iron-want) > 1e-6 {
		t.Errorf("refund %.2f, want %.2f", c.Iron, want)
	}
}

func TestPlaceFailsWhenTooPoorAndChangesNothing(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron = 5

	at := world.FromOffset(4, 4)
	err := c.Place(m, Habitat, at)
	if !errors.Is(err, ErrTooPoor) {
		t.Fatalf("Place = %v, want ErrTooPoor", err)
	}
	if c.Iron != 5 {
		t.Errorf("ore moved to %.1f on a failed placement", c.Iron)
	}
	if len(c.Buildings) != 0 {
		t.Errorf("%d buildings recorded on a failed placement", len(c.Buildings))
	}
}

// Demolition uses a swap-remove, so the index entry for the building that
// moves has to be repaired. A stale index shows up as a tile that cannot be
// built on and has nothing on it.
func TestDemolishKeepsTheIndexHonest(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron = 10000
	c.Crystal = 10000

	var placed []hex.Axial
	for i := 0; i < 6; i++ {
		at := world.FromOffset(2+i, 3)
		if err := c.Place(m, Habitat, at); err != nil {
			t.Fatal(err)
		}
		placed = append(placed, at)
	}

	// Remove from the front, which is the case that moves the last element.
	for _, victim := range []int{0, 2, 1} {
		at := placed[victim]
		if _, ok := c.Demolish(at); !ok {
			t.Fatalf("Demolish(%v) found nothing", at)
		}
		placed[victim] = hex.Axial{Q: -999, R: -999}

		for _, still := range placed {
			if still.Q == -999 {
				continue
			}
			b, ok := c.At(still)
			if !ok {
				t.Fatalf("after demolishing %v, %v reports empty", at, still)
			}
			if b.At != still {
				t.Fatalf("index for %v returns a building at %v", still, b.At)
			}
		}
	}

	if len(c.Buildings) != 3 {
		t.Errorf("%d buildings left, want 3", len(c.Buildings))
	}
}

func TestDemolishRefundsPartOfTheCost(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()

	at := world.FromOffset(4, 4)
	if err := c.Place(m, Habitat, at); err != nil {
		t.Fatal(err)
	}
	afterBuild := c.Iron

	kind, ok := c.Demolish(at)
	if !ok || kind != Habitat {
		t.Fatalf("Demolish = %v, %v; want Habitat, true", kind, ok)
	}
	want := afterBuild + Of(Habitat).IronCost*RefundFraction
	if math.Abs(float64(c.Iron-want)) > 1e-4 {
		t.Errorf("ore is %.2f after refund, want %.2f", c.Iron, want)
	}

	if _, ok := c.Demolish(at); ok {
		t.Error("demolishing an empty tile reported success")
	}
}

// Solar is the reason night is a mechanic. If it kept producing after dark
// the whole geothermal branch would be pointless.
func TestSolarStopsAtNightAndGeothermalDoesNot(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	solar := New()
	anchor(t, solar, m, world.FromOffset(4, 4))
	if err := solar.Place(m, SolarArray, world.FromOffset(4, 4)); err != nil {
		t.Fatal(err)
	}
	solar.Tick(1, 0)
	if got := solar.Readout.PowerSupply; got != 0 {
		t.Errorf("solar supplies %.1f at night, want 0", got)
	}
	solar.Tick(1, 1)
	if got := solar.Readout.PowerSupply; got != Of(SolarArray).PowerOut {
		t.Errorf("solar supplies %.1f at noon, want %.1f", got, Of(SolarArray).PowerOut)
	}

	geo := New()
	geo.Iron = 1000
	anchor(t, geo, m, world.FromOffset(6, 6))
	if err := geo.Place(m, Geothermal, world.FromOffset(6, 6)); err != nil {
		t.Fatal(err)
	}
	geo.Tick(1, 0)
	if got := geo.Readout.PowerSupply; got != Of(Geothermal).PowerOut {
		t.Errorf("geothermal supplies %.1f at night, want %.1f", got, Of(Geothermal).PowerOut)
	}
}

// A brownout must throttle production rather than stopping it dead or being
// ignored, because the whole power-balance mechanic rests on that curve.
func TestBrownoutScalesProductionByTheSupplyRatio(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	// Staffed: labour gates production, and this test is about something else.
	c.Colonists = 100

	// Under the iron ceiling, or the mine's output spills instead of landing
	// in the stock this test measures the delta of.
	c.Iron = 100
	c.Crystal = 100

	anchor(t, c, m, world.FromOffset(4, 4))
	if err := c.Place(m, Mine, world.FromOffset(4, 4)); err != nil {
		t.Fatal(err)
	}

	// No power at all: demand 6, supply 0.
	start := c.Iron
	c.Tick(1, 0)
	if c.Iron != start {
		t.Errorf("an unpowered mine produced %.4f ore", c.Iron-start)
	}
	if got := c.Readout.Satisfaction; got != 0 {
		t.Errorf("satisfaction %.2f with no supply, want 0", got)
	}

	// Half the power it needs: a solar array at a quarter daylight gives 3.5
	// against a demand of 6.
	if err := c.Place(m, SolarArray, world.FromOffset(5, 5)); err != nil {
		t.Fatal(err)
	}
	start = c.Iron
	c.Tick(1, 0.25)

	// The anchoring habitat draws from the same grid, so the demand this
	// supply is rationed against is both of them.
	demand := Of(Mine).PowerIn + Of(Habitat).PowerIn
	wantSat := (Of(SolarArray).PowerOut * 0.25) / demand
	if got := c.Readout.Satisfaction; math.Abs(float64(got-wantSat)) > 1e-4 {
		t.Errorf("satisfaction %.4f, want %.4f", got, wantSat)
	}

	yield := Yield(Mine, world.Dunes)
	wantOre := Of(Mine).MineOut * yield * wantSat
	if got := c.Iron - start; math.Abs(float64(got-wantOre)) > 1e-4 {
		t.Errorf("mined %.4f ore, want %.4f", got, wantOre)
	}
}

// A mine is the same building everywhere; the ground decides what it is for.
// This is the rule the whole hotbar hint, tooltip and HUD split rests on, so
// it is asserted against colony.Mines rather than against the catalog.
func TestWhatAMineProducesIsDecidedByTheGroundUnderIt(t *testing.T) {
	cases := []struct {
		terrain  world.Terrain
		wantOre  world.Ore
		wantRate float64
	}{
		{world.Dunes, world.OreIron, Of(Mine).MineOut},
		{world.Crystal, world.OreCrystal, Of(Mine).MineOut * 0.4},

		// Ground that yields nothing: a mine cannot be placed at all, so the
		// rate is moot and the zero is what stops a stray one producing.
		{world.Basalt, world.OreNone, 0},
		{world.Regolith, world.OreNone, 0},
		{world.Lichen, world.OreNone, 0},
	}
	for _, tc := range cases {
		t.Run(tc.terrain.String(), func(t *testing.T) {
			ore, rate := Mines(Mine, tc.terrain)
			if ore != tc.wantOre {
				t.Errorf("yields %v, want %v", ore, tc.wantOre)
			}
			if math.Abs(rate-tc.wantRate) > 1e-9 {
				t.Errorf("rate %.3f, want %.3f", rate, tc.wantRate)
			}

			// Placement and production have to agree: anything mineable is
			// buildable on and anything else is refused.
			m := flatMap(t, tc.terrain)
			c := New()
			c.Iron, c.Crystal = 10000, 10000
			anchor(t, c, m, world.FromOffset(4, 4))
			err := c.Place(m, Mine, world.FromOffset(4, 4))
			if tc.wantOre == world.OreNone {
				if !errors.Is(err, ErrTerrain) {
					t.Errorf("Place = %v, want ErrTerrain", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := c.At(world.FromOffset(4, 4))
			if b.Ore != tc.wantOre {
				t.Errorf("placed mine records %v, want %v", b.Ore, tc.wantOre)
			}
		})
	}
}

// Nothing but a mine yields an ore, whatever it is standing on. Without this
// a stray MineOut on another spec would quietly print money.
func TestOnlyMinesYield(t *testing.T) {
	for _, k := range Buildable {
		if k == Mine {
			continue
		}
		for _, ground := range []world.Terrain{world.Dunes, world.Crystal} {
			if ore, rate := Mines(k, ground); ore != world.OreNone || rate != 0 {
				t.Errorf("%s on %s yields %.2f %v", k, ground, rate, ore)
			}
		}
	}
}

// A habitat's food draw is written in the catalog and the growth rules are
// written against FoodPerColonist. If those two drift, a full colony either
// starves on a working greenhouse or eats nothing at all.
func TestHabitatFoodMatchesWhatItsColonistsEat(t *testing.T) {
	spec := Of(Habitat)
	if want := spec.Housing * FoodPerColonist; math.Abs(spec.FoodIn-want) > 1e-9 {
		t.Errorf("Habitat.FoodIn %.4f, want %.4f for %.0f beds", spec.FoodIn, want, spec.Housing)
	}
}

// The growth loop: housing plus food brings colonists in, and losing the food
// drives them out again.
func TestColonistsArriveWithFoodAndLeaveWithoutIt(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))

	for i := 0; i < 600; i++ {
		c.Tick(0.1, 1)
	}

	housing := Of(Habitat).Housing
	if c.Colonists < housing-0.01 {
		t.Errorf("colonists reached %.2f of %.0f housing with food available", c.Colonists, housing)
	}
	if c.Colonists > housing+1e-4 {
		t.Errorf("colonists overshot housing: %.3f > %.0f", c.Colonists, housing)
	}

	// Cut the food off.
	c.Food = 0
	peak := c.Colonists
	for i := 0; i < 100; i++ {
		c.Tick(0.1, 1)
	}
	if c.Colonists >= peak {
		t.Errorf("colonists held at %.2f with no biomass", c.Colonists)
	}
	if c.Colonists < 0 {
		t.Errorf("colonists went negative: %.3f", c.Colonists)
	}
}

// Demolishing the housing out from under a population has to shed colonists
// rather than leaving them floating above the new capacity forever.
func TestColonistsFallBackToRemainingHousing(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))
	mustPlace(t, c, m, Habitat, world.FromOffset(5, 4))

	c.Food = 1e6
	for i := 0; i < 1000; i++ {
		c.Tick(0.1, 1)
	}
	if c.Colonists < 2*Of(Habitat).Housing-0.01 {
		t.Fatalf("only %.2f colonists before the test starts", c.Colonists)
	}

	c.Demolish(world.FromOffset(5, 4))
	for i := 0; i < 1000; i++ {
		c.Tick(0.1, 1)
	}
	if want := Of(Habitat).Housing; math.Abs(float64(c.Colonists-want)) > 0.01 {
		t.Errorf("colonists settled at %.3f, want %.0f", c.Colonists, want)
	}
}

// A greenhouse with no water must not conjure biomass, and the stores must
// never go negative however long it runs dry.
func TestStoresNeverGoNegative(t *testing.T) {
	m := flatMap(t, world.Lichen)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))
	mustPlace(t, c, m, Habitat, world.FromOffset(5, 5))

	c.Water = 0
	c.Food = 0
	before := c.Food

	for i := 0; i < 500; i++ {
		c.Tick(0.1, 1)
		if c.Iron < 0 || c.Water < 0 || c.Food < 0 || c.Colonists < 0 {
			t.Fatalf("negative store at step %d: ore %.3f water %.3f biomass %.3f colonists %.3f",
				i, c.Iron, c.Water, c.Food, c.Colonists)
		}
	}
	if c.Food > before+1e-6 {
		t.Errorf("a dry greenhouse produced %.4f biomass", c.Food-before)
	}
}

// Fertile ground is the reason to build a greenhouse on lichen instead of
// next to the habitat.
func TestFertileGroundRaisesGreenhouseOutput(t *testing.T) {
	plain := greenhouseRun(t, world.Regolith)
	fertile := greenhouseRun(t, world.Lichen)

	if fertile <= plain {
		t.Fatalf("lichen produced %.3f, regolith %.3f; fertile should be higher", fertile, plain)
	}
	if ratio := fertile / plain; math.Abs(float64(ratio-1.5)) > 0.01 {
		t.Errorf("fertile bonus is %.3fx, want 1.5x", ratio)
	}
}

// Tick with a zero or negative delta must be a no-op, because the frame loop
// can hand one over on the first frame.
func TestTickIgnoresNonPositiveDeltas(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))

	before := *c
	c.Tick(0, 1)
	c.Tick(-1, 1)
	if c.Iron != before.Iron || c.Water != before.Water || c.Food != before.Food {
		t.Error("a zero or negative tick changed the stores")
	}
}

// A colony loaded from a save has no index until it is rebuilt, and every
// lookup has to keep working across that gap.
func TestReindexRestoresLookupsAfterALoad(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	for i := 0; i < 4; i++ {
		mustPlace(t, c, m, Habitat, world.FromOffset(2+i, 3))
	}

	// What a decoded save looks like: exported fields only.
	loaded := &Colony{
		Iron:      c.Iron,
		Crystal:   c.Crystal,
		Water:     c.Water,
		Food:      c.Food,
		Colonists: c.Colonists,
		Buildings: c.Buildings,
	}
	loaded.Reindex()

	for i := 0; i < 4; i++ {
		at := world.FromOffset(2+i, 3)
		if _, ok := loaded.At(at); !ok {
			t.Errorf("tile %v lost its building across the load", at)
		}
		if err := loaded.CanPlace(m, Habitat, at); !errors.Is(err, ErrOccupied) {
			t.Errorf("CanPlace on a loaded tile = %v, want ErrOccupied", err)
		}
	}
}

// The same guard, for the path where nothing called Reindex at all.
func TestLookupsWorkOnAZeroValueColony(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := &Colony{Iron: 1000}
	at := world.FromOffset(4, 4)

	if _, ok := c.At(at); ok {
		t.Error("an empty colony reported a building")
	}
	if err := c.Place(m, Habitat, at); err != nil {
		t.Fatalf("Place on a zero-value colony: %v", err)
	}
	if _, ok := c.At(at); !ok {
		t.Error("the building placed on a zero-value colony is not there")
	}
}

func greenhouseRun(t *testing.T, terrain world.Terrain) float64 {
	t.Helper()
	m := flatMap(t, terrain)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))

	// Staffed, and started empty of food so the ten seconds below are all
	// growth rather than growth against a ceiling. The water is capped to
	// what there is room for: 1e6 would spill on the first tick and the
	// greenhouse would then be measuring the tank rather than the ground.
	c.Colonists = 100
	c.Water = BaseWaterStore
	c.Food = 0
	for i := 0; i < 100; i++ {
		c.Tick(0.1, 1)
	}
	return c.Food
}

// anchor founds a habitat beside a tile so that something may be built there.
//
// Tests about terrain, cost or production call it to satisfy the reach rule
// and get on with the rule they are actually about. It founds rather than
// places so the anchor itself is free and unconstrained — the habitat is
// scaffolding for the test, not part of what it measures.
func anchor(t *testing.T, c *Colony, m *world.Map, a hex.Axial) {
	t.Helper()

	// Three tiles out rather than one: inside BuildRadius with room to spare,
	// and far enough that the anchor is not sitting on a tile the test wanted
	// to build its second structure on.
	var last error
	for d := range 6 {
		at := a
		for range 3 {
			at = at.Neighbor(d)
		}
		if last = c.Found(m, Habitat, at); last == nil {
			return
		}
	}
	t.Fatalf("no room to anchor a habitat near %v: %v", a, last)
}

// mustPlace puts a structure in a fixture. It founds rather than places: a
// fixture is describing a colony that exists, not exercising the rules for
// building one. Those have their own tests, which call Place and CanPlace
// directly so that a change to the fixtures cannot quietly weaken them.
func mustPlace(t *testing.T, c *Colony, m *world.Map, k Kind, a hex.Axial) {
	t.Helper()
	if err := c.Found(m, k, a); err != nil {
		t.Fatalf("Found(%v at %v): %v", k, a, err)
	}
}

// flatMap is a small map of one terrain at a constant, above-sea elevation,
// so a test can say what it means without generating a planet.
func flatMap(t *testing.T, terrain world.Terrain) *world.Map {
	t.Helper()
	return flatMapOf(t, terrain, 12)
}

// flatMapOf is flatMap with room to spread out, for tests about distance.
func flatMapOf(t *testing.T, terrain world.Terrain, size int) *world.Map {
	t.Helper()
	m, err := world.NewMap(size, size, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = terrain
		tile.Elevation = world.SeaLevel + 2
	})
	return m
}

// walk steps n tiles in one direction, for placing something a known distance
// away from somewhere else.
func walk(a hex.Axial, dir, n int) hex.Axial {
	for range n {
		a = a.Neighbor(dir)
	}
	return a
}

// The reach rule is what gives the workforce somewhere to be. Without it a
// mine on the far side of the continent drew on the same staff as one next
// door, and the map had no say in where a colony went.
func TestNothingIsBuiltOutOfReachOfAHabitat(t *testing.T) {
	m := flatMapOf(t, world.Dunes, 24)
	home := world.FromOffset(5, 5)

	for n := range BuildRadius + 3 {
		c := New()
		c.Iron, c.Crystal = 10000, 10000
		if err := c.Found(m, Habitat, home); err != nil {
			t.Fatal(err)
		}

		at := walk(home, 0, n+1)
		if m.At(at) == nil {
			t.Fatalf("distance %d walks off the map", n+1)
		}

		err := c.Place(m, Mine, at)
		switch {
		case n+1 <= BuildRadius && err != nil:
			t.Errorf("mine %d tiles out: %v, want it allowed", n+1, err)
		case n+1 > BuildRadius && !errors.Is(err, ErrNoHabitat):
			t.Errorf("mine %d tiles out: %v, want ErrNoHabitat", n+1, err)
		}
	}
}

// Habitats are exempt, and have to be: the rule would otherwise be unsatisfiable
// for the first one, and a colony could never expand past its landing site.
func TestAHabitatIsItsOwnPermission(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron, c.Crystal = 10000, 10000

	// Nothing built at all: the first habitat still goes down.
	first := world.FromOffset(2, 2)
	if err := c.Place(m, Habitat, first); err != nil {
		t.Fatalf("the first habitat: %v", err)
	}

	// And the next one may be planted beyond the reach of that one, which is
	// how a colony walks across the map towards ice or a vent.
	far := walk(first, 0, BuildRadius+2)
	if err := c.Place(m, Habitat, far); err != nil {
		t.Fatalf("an outpost %d tiles out: %v", BuildRadius+2, err)
	}

	// Which then carries its own reach with it.
	if err := c.Place(m, Condenser, far.Neighbor(1)); err != nil {
		t.Errorf("beside the new outpost: %v", err)
	}
}

// Founding is placement by fiat and skips the reach rule, because the landing
// site arrives before there is any habitat to be near.
func TestFoundingIgnoresReachButNotTheGround(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()

	if err := c.Found(m, SolarArray, world.FromOffset(4, 4)); err != nil {
		t.Errorf("founding an array with no habitat: %v", err)
	}
	// The ground rules are not skipped with it.
	if err := c.Found(m, Geothermal, world.FromOffset(6, 6)); !errors.Is(err, ErrTerrain) {
		t.Errorf("founded a geothermal plant off a vent: %v", err)
	}
}

// The rule is about building, not about standing. Demolishing the habitat an
// outpost was built around leaves what is already there running — it is a
// siting decision, and unbuilding one thing should not silently unbuild
// another.
func TestLosingTheHabitatDoesNotStrandWhatIsBuilt(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	c.Iron, c.Crystal = 10000, 10000
	c.Colonists = 100

	home := world.FromOffset(5, 5)
	mine := walk(home, 0, BuildRadius)
	if err := c.Place(m, Habitat, home); err != nil {
		t.Fatal(err)
	}
	if err := c.Place(m, Mine, mine); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Demolish(home); !ok {
		t.Fatal("the habitat would not demolish")
	}

	if _, ok := c.At(mine); !ok {
		t.Fatal("the mine went with the habitat")
	}
	if err := c.Place(m, Mine, mine.Neighbor(1)); !errors.Is(err, ErrNoHabitat) {
		t.Errorf("built beside a stranded mine: %v, want ErrNoHabitat", err)
	}
}
