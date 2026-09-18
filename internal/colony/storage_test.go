package colony

import (
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// Storage and labour.
//
// Every stock has a ceiling, production past it is lost, and every structure
// wants staff. The three together are what turn a resource bar into something
// a bar can honestly be: a bar needs a maximum before "full" means anything.

// The headline: a store that is full stops accepting, and says how much is
// going over the side.
func TestAFullStoreSpillsAndReportsIt(t *testing.T) {
	c, m := stocked(t, world.Dunes)
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	for i := range 3 {
		mustPlace(t, c, m, SolarArray, world.FromOffset(5+i, 4))
	}

	// One tick to learn the ceiling, then sit on it.
	c.Tick(0.1, 1)
	cap := c.Readout.Cap.Iron
	if cap <= 0 {
		t.Fatal("no iron capacity")
	}
	c.Iron = cap

	c.Tick(1, 1)

	if c.Iron > cap+1e-9 {
		t.Errorf("iron went to %.2f past a ceiling of %.2f", c.Iron, cap)
	}
	if c.Readout.Spilled.Iron <= 0 {
		t.Error("a mine at a full stockpile reported no spill")
	}

	// And the mine is still reported as running. It is: the ore is being cut
	// and then thrown away, which is a different problem from a stopped mine
	// and wants a different fix.
	if c.Readout.Iron.Produced <= 0 {
		t.Error("a mine at a full store reported as producing nothing; it is still running")
	}
	if math.Abs(c.Readout.Spilled.Iron-c.Readout.Iron.Produced) > 1e-9 {
		t.Errorf("spilled %.4f of %.4f produced; with no room all of it should go",
			c.Readout.Spilled.Iron, c.Readout.Iron.Produced)
	}
}

// Room for some of it takes some of it. The partial case is the one an
// off-by-one in fill would get wrong.
func TestAPartlyFullStoreTakesWhatFits(t *testing.T) {
	cases := []struct {
		held, added, capacity float64
		wantNow, wantSpilled  float64
	}{
		{0, 5, 10, 5, 0},   // room to spare
		{8, 5, 10, 10, 3},  // partial
		{10, 5, 10, 10, 5}, // none
		{0, 5, 0, 5, 0},    // no ceiling declared: no ceiling applied
		{3, 0, 10, 3, 0},   // nothing produced
		{12, 5, 10, 12, 5}, // already over, from a demolished store
	}
	for _, c := range cases {
		now, spilled := fill(c.held, c.added, c.capacity)
		if math.Abs(now-c.wantNow) > 1e-9 || math.Abs(spilled-c.wantSpilled) > 1e-9 {
			t.Errorf("fill(%.0f, %.0f, cap %.0f) = %.2f, %.2f; want %.2f, %.2f",
				c.held, c.added, c.capacity, now, spilled, c.wantNow, c.wantSpilled)
		}
	}
}

// Building storage raises the ceiling, which is the whole reason to build it.
func TestHabitatsRaiseTheFoodAndWaterCeiling(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	c.Tick(0.1, 1)
	base := c.Readout.Cap

	if base.Food != BaseFoodStore || base.Water != BaseWaterStore {
		t.Fatalf("an empty colony has food %.0f water %.0f, want the lander's %d and %d",
			base.Food, base.Water, BaseFoodStore, BaseWaterStore)
	}

	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))
	c.Tick(0.1, 1)

	spec := Of(Habitat)
	if got := c.Readout.Cap.Food - base.Food; math.Abs(got-spec.FoodStore) > 1e-9 {
		t.Errorf("a habitat added %.0f food capacity, want %.0f", got, spec.FoodStore)
	}
	if got := c.Readout.Cap.Water - base.Water; math.Abs(got-spec.WaterStore) > 1e-9 {
		t.Errorf("a habitat added %.0f water capacity, want %.0f", got, spec.WaterStore)
	}
}

// A mine's stockpile follows the ground under it, exactly as its output does.
// A crystal mine must not raise the iron ceiling.
func TestAMineStockpilesWhateverItCuts(t *testing.T) {
	for _, tc := range []struct {
		ground   world.Terrain
		wantIron bool
	}{
		{world.Dunes, true},
		{world.Crystal, false},
	} {
		c, m := stocked(t, tc.ground)
		c.Tick(0.1, 1)
		base := c.Readout.Cap

		mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
		c.Tick(0.1, 1)

		gotIron := c.Readout.Cap.Iron > base.Iron
		gotCrystal := c.Readout.Cap.Crystal > base.Crystal

		if gotIron != tc.wantIron || gotCrystal == tc.wantIron {
			t.Errorf("a mine on %s raised iron=%v crystal=%v, want iron=%v",
				tc.ground, gotIron, gotCrystal, tc.wantIron)
		}
	}
}

// Demolishing storage cannot leave a stock above what is left to hold it.
func TestDemolishingStorageSpillsWhatNoLongerFits(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	at := world.FromOffset(4, 4)
	mustPlace(t, c, m, Habitat, at)
	c.Tick(0.1, 1)

	c.Food = c.Readout.Cap.Food // brim full
	full := c.Food

	if _, ok := c.Demolish(at); !ok {
		t.Fatal("nothing demolished")
	}
	c.Tick(0.1, 1)

	if c.Food > c.Readout.Cap.Food+1e-6 {
		t.Errorf("%.1f food survives in %.1f of capacity", c.Food, c.Readout.Cap.Food)
	}
	if c.Food >= full {
		t.Error("demolishing the larder cost nothing")
	}
}

// ---------------------------------------------------------------- labour

// Nothing runs without staff. This is the change that gives a habitat a
// purpose beyond housing people who eat.
func TestNothingRunsWithoutStaff(t *testing.T) {
	c, m := stocked(t, world.Dunes)
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))

	c.Colonists = 0
	c.Tick(1, 1)

	if c.Readout.Staffing != 0 {
		t.Errorf("staffing %.2f with nobody living here", c.Readout.Staffing)
	}
	if got := c.Readout.Iron.Produced; got != 0 {
		t.Errorf("an unstaffed mine cut %.3f/s", got)
	}
}

// Half the staff is half the output, the same way half the power is.
func TestStaffingScalesProductionProportionally(t *testing.T) {
	build := func(colonists float64) *Colony {
		c, m := stocked(t, world.Dunes)
		mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
		for i := range 2 {
			mustPlace(t, c, m, SolarArray, world.FromOffset(5+i, 4))
		}
		c.Colonists = colonists
		c.Tick(1, 1)
		return c
	}

	jobs := Of(Mine).Jobs
	full := build(jobs)
	half := build(jobs / 2)

	if full.Readout.Staffing < 0.999 {
		t.Fatalf("a fully staffed mine reports %.3f", full.Readout.Staffing)
	}
	if math.Abs(half.Readout.Staffing-0.5) > 1e-9 {
		t.Errorf("half the staff reports %.3f, want 0.5", half.Readout.Staffing)
	}

	want := full.Readout.Iron.Produced / 2
	if math.Abs(half.Readout.Iron.Produced-want) > 1e-9 {
		t.Errorf("half-staffed produced %.4f, want %.4f", half.Readout.Iron.Produced, want)
	}
}

// Extra people are not extra output. Staffing caps at one, or a colony would
// mine faster by breeding.
func TestSurplusStaffDoesNotRaiseOutput(t *testing.T) {
	build := func(colonists float64) float64 {
		c, m := stocked(t, world.Dunes)
		mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
		mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))
		c.Colonists = colonists
		c.Tick(1, 1)
		return c.Readout.Iron.Produced
	}

	exact := build(Of(Mine).Jobs)
	plenty := build(Of(Mine).Jobs * 20)
	if math.Abs(exact-plenty) > 1e-9 {
		t.Errorf("twenty times the staff produced %.4f against %.4f", plenty, exact)
	}
}

// A colony with nothing to do is fully staffed by definition, rather than
// dividing by a job count of zero.
func TestAColonyWithNoJobsIsFullyStaffed(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4)) // housing, no jobs
	c.Colonists = 0

	c.Tick(1, 1)

	if c.Readout.Jobs != 0 {
		t.Errorf("a habitat declared %.0f jobs", c.Readout.Jobs)
	}
	if c.Readout.Staffing != 1 {
		t.Errorf("staffing %.2f with no jobs to fill, want 1", c.Readout.Staffing)
	}
}

// The loop the whole change exists to close: more structures want more staff,
// staff want housing, housing wants food. A colony that builds only mines
// grinds to a halt.
func TestBuildingWithoutHousingStarvesTheWorkforce(t *testing.T) {
	c, m := stocked(t, world.Dunes)
	mustPlace(t, c, m, Habitat, world.FromOffset(2, 2))
	for i := range 4 {
		mustPlace(t, c, m, SolarArray, world.FromOffset(2+i, 6))
	}
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))

	c.Colonists = Of(Habitat).Housing // one full habitat
	c.Tick(1, 1)
	before := c.Readout.Staffing

	// Three more mines, no more housing.
	for i := range 3 {
		mustPlace(t, c, m, Mine, world.FromOffset(5+i, 5))
	}
	c.Tick(1, 1)

	if c.Readout.Staffing >= before {
		t.Errorf("staffing went from %.2f to %.2f after tripling the jobs",
			before, c.Readout.Staffing)
	}
	if c.Readout.Jobs <= Of(Mine).Jobs {
		t.Errorf("four mines declared %.0f jobs", c.Readout.Jobs)
	}
}
