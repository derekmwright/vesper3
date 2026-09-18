package colony

import (
	"errors"
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// coastMap is flat ground with a sea along one edge, for the two structures
// that want a shoreline.
func coastMap(t *testing.T, ground world.Terrain) *world.Map {
	t.Helper()
	m := flatMap(t, ground)
	for col := range 12 {
		tile := m.At(world.FromOffset(col, 0))
		tile.Terrain = world.Sea
		tile.Elevation = world.SeaLevel - 1
	}
	return m
}

// powered founds enough solar to run whatever the test is about, so that a
// measurement of production is not secretly a measurement of a brownout.
func powered(t *testing.T, c *Colony, m *world.Map, at hex.Axial, arrays int) {
	t.Helper()
	placed := 0
	for r := 1; r <= 4 && placed < arrays; r++ {
		for d := range 6 {
			step := at
			for range r {
				step = step.Neighbor(d)
			}
			if c.Found(m, SolarArray, step) == nil {
				if placed++; placed == arrays {
					return
				}
			}
		}
	}
	if placed < arrays {
		t.Fatalf("only found room for %d of %d arrays", placed, arrays)
	}
}

// The one sentence the whole tier system rests on: a tier-3 building is two
// buildings' worth of plant run by one building's worth of staff. If Jobs ever
// scales with it, upgrading stops being the answer to a map that will not let
// a colony sprawl and becomes an expensive way to do the same thing.
func TestATierScalesEveryRateButStaff(t *testing.T) {
	for _, k := range Buildable {
		base := Of(k)
		top := base.scaled(TierScale(MaxTier))

		if top.Jobs != base.Jobs {
			t.Errorf("%s: tier 3 wants %.0f staff, tier 1 wants %.0f",
				base.Name, top.Jobs, base.Jobs)
		}
		if top.IronCost != base.IronCost || top.CrystalCost != base.CrystalCost {
			t.Errorf("%s: scaling moved the build price", base.Name)
		}
		if top.Needs != base.Needs {
			t.Errorf("%s: scaling moved the siting rule", base.Name)
		}

		// Everything that is a rate or a capacity doubles at the top tier.
		rates := []struct {
			name       string
			one, three float64
		}{
			{"power out", base.PowerOut, top.PowerOut},
			{"power in", base.PowerIn, top.PowerIn},
			{"mine out", base.MineOut, top.MineOut},
			{"water out", base.WaterOut, top.WaterOut},
			{"water in", base.WaterIn, top.WaterIn},
			{"food out", base.FoodOut, top.FoodOut},
			{"food in", base.FoodIn, top.FoodIn},
			{"vespite out", base.VespiteOut, top.VespiteOut},
			{"crystal in", base.CrystalIn, top.CrystalIn},
			{"housing", base.Housing, top.Housing},
			{"power store", base.PowerStore, top.PowerStore},
			{"water store", base.WaterStore, top.WaterStore},
			{"food store", base.FoodStore, top.FoodStore},
			{"iron store", base.IronStore, top.IronStore},
			{"crystal store", base.CrystalStore, top.CrystalStore},
			{"vespite store", base.VespiteStore, top.VespiteStore},
		}
		for _, r := range rates {
			if want := r.one * 2; math.Abs(r.three-want) > 1e-9 {
				t.Errorf("%s: tier 3 %s is %.3f, want %.3f", base.Name, r.name, r.three, want)
			}
		}
	}
}

// A save written before tiers existed has none, and every building in it has
// to keep working at the rate it had.
func TestAnUnsetTierIsTheFirstOne(t *testing.T) {
	var b Building
	if got := b.Tier(); got != 1 {
		t.Errorf("an unset tier reads as %d, want 1", got)
	}
	for tier, want := range map[uint8]float64{0: 1, 1: 1, 2: 1.5, 3: 2} {
		if got := TierScale(tier); got != want {
			t.Errorf("tier %d scales by %.2f, want %.2f", tier, got, want)
		}
	}
}

// Upgrading is not demolish-and-rebuild. A rebuild would refund the original,
// re-run the siting rules and re-roll the ore under a mine, and an upgrade is
// none of those things.
func TestUpgradingKeepsTheBuildingItUpgraded(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	c.Iron, c.Crystal, c.Vespite = 10000, 10000, 100
	c.Colonists = 100

	at := world.FromOffset(5, 5)
	if err := c.Found(m, Habitat, at.Neighbor(3)); err != nil {
		t.Fatal(err)
	}
	if err := c.Place(m, Mine, at); err != nil {
		t.Fatal(err)
	}
	powered(t, c, m, at, 2)
	before, _ := c.At(at)

	iron, vespite := c.Iron, c.Vespite
	cost, err := c.CanUpgrade(at)
	if err != nil {
		t.Fatalf("CanUpgrade: %v", err)
	}
	if err := c.Upgrade(at); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	after, ok := c.At(at)
	if !ok {
		t.Fatal("the building went away")
	}
	if after.Tier() != 2 {
		t.Errorf("tier %d after one upgrade, want 2", after.Tier())
	}
	if after.At != before.At || after.Facing != before.Facing ||
		after.Ore != before.Ore || after.Yield != before.Yield {
		t.Errorf("upgrade changed identity: %+v -> %+v", before, after)
	}
	if got := iron - c.Iron; math.Abs(got-cost.Iron) > 1e-9 {
		t.Errorf("charged %.2f iron, want %.2f", got, cost.Iron)
	}
	if got := vespite - c.Vespite; math.Abs(got-cost.Vespite) > 1e-9 {
		t.Errorf("charged %.2f vespite, want %.2f", got, cost.Vespite)
	}

	// And the colony now runs it at the bigger numbers.
	c.Tick(1, 1)
	if c.Readout.Satisfaction < 1 {
		t.Fatalf("brownout at %.2f: this test cannot measure output", c.Readout.Satisfaction)
	}
	want := Of(Mine).MineOut * TierScale(2) * before.Yield
	if got := c.Readout.Iron.Produced; math.Abs(got-want) > 1e-4 {
		t.Errorf("a tier-2 mine produced %.4f, want %.4f", got, want)
	}
}

func TestUpgradingStopsAtTheTopAndSaysSo(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron, c.Vespite = 100000, 10000

	at := world.FromOffset(5, 5)
	if err := c.Found(m, Habitat, at); err != nil {
		t.Fatal(err)
	}
	for tier := 2; tier <= MaxTier; tier++ {
		if err := c.Upgrade(at); err != nil {
			t.Fatalf("upgrade to tier %d: %v", tier, err)
		}
	}
	if err := c.Upgrade(at); !errors.Is(err, ErrTopTier) {
		t.Errorf("a fourth upgrade returned %v, want ErrTopTier", err)
	}
	if b, _ := c.At(at); b.Tier() != MaxTier {
		t.Errorf("tier %d, want %d", b.Tier(), MaxTier)
	}
}

func TestUpgradingNeedsVespiteAndSaysWhich(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron = 100000

	at := world.FromOffset(5, 5)
	if err := c.Found(m, Habitat, at); err != nil {
		t.Fatal(err)
	}

	cost, _ := CostToReach(Habitat, 2)
	c.Vespite = cost.Vespite - 1
	err := c.Upgrade(at)
	if !errors.Is(err, ErrTooPoor) {
		t.Fatalf("upgraded without the vespite: %v", err)
	}
	if !contains(err.Error(), "vespite") {
		t.Errorf("error %q does not name what is short", err)
	}
	if b, _ := c.At(at); b.Tier() != 1 {
		t.Error("a refused upgrade still moved the tier")
	}
}

func TestUpgradingNothingIsNotAnUpgrade(t *testing.T) {
	c := New()
	if err := c.Upgrade(world.FromOffset(3, 3)); !errors.Is(err, ErrNothingThere) {
		t.Errorf("upgraded bare ground: %v", err)
	}
}

// The premise, mechanically: vespite comes from a coast and from crystal, and
// there is nowhere else to get it.
func TestVespiteComesOnlyFromASynthesizerOnACoast(t *testing.T) {
	m := coastMap(t, world.Regolith)
	c := New()
	c.Iron, c.Crystal = 10000, 10000

	if err := c.Found(m, Habitat, world.FromOffset(5, 5)); err != nil {
		t.Fatal(err)
	}
	if err := c.Place(m, Synthesizer, world.FromOffset(5, 6)); !errors.Is(err, ErrTerrain) {
		t.Errorf("a synthesizer well inland: %v, want ErrTerrain", err)
	}

	// And on the shore it goes down.
	if err := c.Found(m, Habitat, world.FromOffset(4, 2)); err != nil {
		t.Fatal(err)
	}
	if err := c.Place(m, Synthesizer, world.FromOffset(4, 1)); err != nil {
		t.Errorf("a synthesizer on the shore: %v", err)
	}

	// Nothing else in the catalog makes any.
	for _, k := range Buildable {
		if k != Synthesizer && Of(k).VespiteOut != 0 {
			t.Errorf("%s synthesises vespite", Of(k).Name)
		}
	}
}

func TestSynthesisRunsOnCrystalAndStopsWithoutIt(t *testing.T) {
	m := coastMap(t, world.Regolith)
	c := New()
	c.Iron, c.Crystal = 10000, 10000
	c.Colonists = 100

	home := world.FromOffset(4, 2)
	if err := c.Found(m, Habitat, home); err != nil {
		t.Fatal(err)
	}
	if err := c.Place(m, Synthesizer, world.FromOffset(4, 1)); err != nil {
		t.Fatalf("on the shore: %v", err)
	}
	powered(t, c, m, home, 2)

	spec := Of(Synthesizer)
	c.Tick(1, 1)
	if c.Readout.Satisfaction < 1 {
		t.Fatalf("brownout at %.2f: this test cannot measure synthesis", c.Readout.Satisfaction)
	}
	if got := c.Readout.Vespite.Produced; math.Abs(got-spec.VespiteOut) > 1e-6 {
		t.Errorf("synthesised %.4f/s, want %.4f", got, spec.VespiteOut)
	}
	if got := c.Readout.Crystal.Consumed; math.Abs(got-spec.CrystalIn) > 1e-6 {
		t.Errorf("drew %.4f crystal/s, want %.4f", got, spec.CrystalIn)
	}

	// Out of crystal, and it stops rather than growing lattice out of nothing.
	c.Crystal = 0
	c.Tick(1, 1)
	if got := c.Readout.Vespite.Produced; got != 0 {
		t.Errorf("synthesised %.4f/s with no crystal, want 0", got)
	}
}

func TestVespiteIsHeldByTheSynthesizersThatMakeIt(t *testing.T) {
	m := coastMap(t, world.Regolith)
	c := New()
	c.Iron, c.Crystal = 10000, 10000
	c.Colonists = 100

	home := world.FromOffset(4, 2)
	if err := c.Found(m, Habitat, home); err != nil {
		t.Fatal(err)
	}
	c.Tick(1, 1)
	if got := c.Readout.Cap.Vespite; got != 0 {
		t.Errorf("a colony with no synthesizer can hold %.0f vespite", got)
	}

	if err := c.Place(m, Synthesizer, world.FromOffset(4, 1)); err != nil {
		t.Fatal(err)
	}
	powered(t, c, m, home, 2)
	c.Tick(1, 1)
	if got, want := c.Readout.Cap.Vespite, Of(Synthesizer).VespiteStore; got != want {
		t.Errorf("capacity %.0f, want %.0f", got, want)
	}

	// Past the ceiling it spills, and says so, exactly like every other stock.
	c.Vespite = c.Readout.Cap.Vespite
	c.Tick(1, 1)
	if c.Vespite > c.Readout.Cap.Vespite {
		t.Errorf("held %.2f past a ceiling of %.2f", c.Vespite, c.Readout.Cap.Vespite)
	}
	if c.Readout.Spilled.Vespite <= 0 {
		t.Error("a full synthesizer reported no spill")
	}
}

// The upgrade price is derived from the build price so the table generalises,
// which only holds if it really is derived.
func TestUpgradeCostsFollowTheBuildPrice(t *testing.T) {
	for _, k := range Buildable {
		spec := Of(k)
		two, ok := CostToReach(k, 2)
		if !ok {
			t.Fatalf("%s has no tier 2", spec.Name)
		}
		three, ok := CostToReach(k, MaxTier)
		if !ok {
			t.Fatalf("%s has no tier 3", spec.Name)
		}
		if three.Iron <= two.Iron || three.Vespite <= two.Vespite {
			t.Errorf("%s: tier 3 is not dearer than tier 2", spec.Name)
		}
		if math.Abs(two.Iron-1.5*spec.IronCost) > 1e-9 {
			t.Errorf("%s: tier 2 iron %.1f is not derived from its %.0f build price",
				spec.Name, two.Iron, spec.IronCost)
		}
		if _, ok := CostToReach(k, MaxTier+1); ok {
			t.Errorf("%s has a price for a tier above the top", spec.Name)
		}
	}
}
