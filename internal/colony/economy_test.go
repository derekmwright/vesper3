package colony

import (
	"math"
	"testing"

	"github.com/derekmwright/worldbuild/internal/world"
)

// stocked returns a colony on the named ground with enough of both ores to
// build whatever a test wants, so a fixture never fails on price.
func stocked(t *testing.T, ground world.Terrain) (*Colony, *world.Map) {
	t.Helper()
	m := flatMap(t, ground)
	c := New()
	c.Iron, c.Crystal = 100000, 100000
	return c, m
}

// Every structure that eats has to show up in the ledger as eating. This is
// the whole complaint the economy pass answered: a greenhouse that grew food
// nothing consumed, and a habitat that drank water and nothing else.
func TestEveryDrawIsSpentAndEveryOutputIsMade(t *testing.T) {
	c, m := stocked(t, world.Dunes)

	// A colony with one of everything that matters, powered well past what it
	// needs so nothing below is masked by a brownout.
	mustPlace(t, c, m, Habitat, world.FromOffset(2, 2))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(3, 2))
	mustPlace(t, c, m, Mine, world.FromOffset(4, 2))
	for i := range 4 {
		mustPlace(t, c, m, SolarArray, world.FromOffset(2+i, 4))
	}
	c.Colonists = Of(Habitat).Housing // full, so the habitat eats at its rate

	c.Tick(1, 1)
	r := c.Readout

	if r.Satisfaction < 0.999 {
		t.Fatalf("fixture is browning out at %.2f", r.Satisfaction)
	}

	// Water: the habitat, the greenhouse and the mine all draw, and nothing
	// makes any.
	wantWater := Of(Habitat).WaterIn + Of(Greenhouse).WaterIn + Of(Mine).WaterIn
	if math.Abs(r.Water.Consumed-wantWater) > 1e-9 {
		t.Errorf("water use %.3f/s, want %.3f/s", r.Water.Consumed, wantWater)
	}
	if r.Water.Produced != 0 {
		t.Errorf("water made %.3f/s with no extractor", r.Water.Produced)
	}

	// Food: the greenhouse grows it and the habitat eats it.
	if want := Of(Greenhouse).FoodOut; math.Abs(r.Food.Produced-want) > 1e-9 {
		t.Errorf("food grown %.3f/s, want %.3f/s", r.Food.Produced, want)
	}
	if want := Of(Habitat).FoodIn; math.Abs(r.Food.Consumed-want) > 1e-9 {
		t.Errorf("food eaten %.3f/s, want %.3f/s", r.Food.Consumed, want)
	}

	// Iron from the dunes, and no crystal from ground that has none.
	if want := Of(Mine).MineOut; math.Abs(r.Iron.Produced-want) > 1e-9 {
		t.Errorf("iron %.3f/s, want %.3f/s", r.Iron.Produced, want)
	}
	if r.Crystal.Produced != 0 {
		t.Errorf("crystal %.3f/s out of ferrous dunes", r.Crystal.Produced)
	}
}

// A mine needs power and water both, so cutting either stops it. Before this
// the water half did not exist and a mine ran on electricity alone.
func TestAMineStopsWhenEitherInputIsCut(t *testing.T) {
	build := func() (*Colony, *world.Map) {
		c, m := stocked(t, world.Dunes)
		mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
		mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))
		return c, m
	}

	// Both available.
	full, _ := build()
	full.Tick(1, 1)
	if full.Readout.Iron.Produced <= 0 {
		t.Fatal("a supplied mine produced nothing")
	}

	// No power.
	dark, _ := build()
	dark.Tick(1, 0)
	if got := dark.Readout.Iron.Produced; got != 0 {
		t.Errorf("mined %.3f/s in a blackout", got)
	}

	// No water.
	dry, _ := build()
	dry.Water = 0
	dry.Tick(1, 1)
	if got := dry.Readout.Iron.Produced; got != 0 {
		t.Errorf("mined %.3f/s with a dry tank", got)
	}
}

// A greenhouse with no water grows nothing, which was already true, and is
// asserted here because food is now the resource a colony actually lives on.
func TestAGreenhouseWithNoWaterGrowsNothing(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))
	c.Water = 0

	c.Tick(1, 1)
	if got := c.Readout.Food.Produced; got != 0 {
		t.Errorf("grew %.3f food/s out of nothing", got)
	}
}

// The geothermal plant's water is the price of night power being
// unconditional. A plant with a dry loop winds down instead of running free.
func TestGeothermalNeedsCoolantToGenerate(t *testing.T) {
	build := func() (*Colony, *world.Map) {
		c, m := stocked(t, world.Regolith)
		vent := world.FromOffset(4, 4)
		m.At(vent).Terrain = world.Vent
		if err := c.Found(m, Geothermal, vent); err != nil {
			t.Fatal(err)
		}
		mustPlace(t, c, m, Habitat, world.FromOffset(6, 6))
		return c, m
	}

	wet, _ := build()
	wet.Tick(1, 0) // night: the plant is the only supply
	if want := Of(Geothermal).PowerOut; math.Abs(wet.Readout.PowerSupply-want) > 1e-9 {
		t.Errorf("supply %.2f with coolant, want %.2f", wet.Readout.PowerSupply, want)
	}
	if wet.Readout.Coolant < 0.999 {
		t.Errorf("coolant %.3f with a full tank", wet.Readout.Coolant)
	}

	dry, _ := build()
	dry.Water = 0
	dry.Tick(1, 0)
	if dry.Readout.PowerSupply != 0 {
		t.Errorf("supply %.2f from a plant with no coolant", dry.Readout.PowerSupply)
	}
	if dry.Readout.Coolant != 0 {
		t.Errorf("coolant %.3f with an empty tank", dry.Readout.Coolant)
	}
}

// Half the coolant is half the plant, rather than all or nothing: a colony
// running its tank down should see the lights dim before they go out.
func TestCoolantScalesGenerationProportionally(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	vent := world.FromOffset(4, 4)
	m.At(vent).Terrain = world.Vent
	if err := c.Found(m, Geothermal, vent); err != nil {
		t.Fatal(err)
	}

	// Exactly half a second's worth of makeup water, against a one-second tick.
	c.Water = Of(Geothermal).WaterIn * 0.5
	c.Tick(1, 0)

	if got := c.Readout.Coolant; math.Abs(got-0.5) > 1e-9 {
		t.Errorf("coolant %.3f, want 0.5", got)
	}
	if want := Of(Geothermal).PowerOut * 0.5; math.Abs(c.Readout.PowerSupply-want) > 1e-9 {
		t.Errorf("supply %.2f, want %.2f", c.Readout.PowerSupply, want)
	}
	if c.Water != 0 {
		t.Errorf("%.4f water left after drinking what there was", c.Water)
	}
}

// Solar generation cannot depend on coolant, because there is no ordering that
// resolves a solar plant that drinks: its water would have to be rationed
// before the grid, and its output is already decided by the sun. The catalog
// is free to change; this is the invariant the tick's three-way generation
// split assumes.
func TestNoSolarGeneratorDrinks(t *testing.T) {
	for k := None + 1; k < kindCount; k++ {
		if s := Of(k); s.SolarDependent && s.WaterIn > 0 {
			t.Errorf("%s is solar-dependent and draws %.2f water/s", s.Name, s.WaterIn)
		}
	}
}

// Consumption is scaled by the grid, so a colony that has gone dark does not
// also empty its tank while it is down. That matters more now than it did:
// the water it would waste is the water its power plants need to restart.
func TestABlackoutDoesNotDrainTheTank(t *testing.T) {
	c, m := stocked(t, world.Dunes)
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(5, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(6, 4))

	before := c.Water
	c.Tick(1, 0) // night, no storage: nothing runs

	if c.Readout.Satisfaction != 0 {
		t.Fatalf("fixture is not blacked out: satisfaction %.3f", c.Readout.Satisfaction)
	}
	if c.Water != before {
		t.Errorf("tank went from %.3f to %.3f during a blackout", before, c.Water)
	}
}

// People eat whether or not the lights are on. The habitat's draw is the one
// thing the grid does not scale, and a colony that starves its way out of a
// brownout would be a different game.
func TestColonistsEatThroughABlackout(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))
	c.Colonists = Of(Habitat).Housing

	c.Tick(1, 0)
	if want := Of(Habitat).FoodIn; math.Abs(c.Readout.Food.Consumed-want) > 1e-9 {
		t.Errorf("ate %.3f/s in the dark, want %.3f/s", c.Readout.Food.Consumed, want)
	}
}

// A habitat raised ahead of the people who will fill it does not eat on their
// behalf, and a full one eats exactly what its colonists do.
func TestHabitatFoodScalesWithOccupancy(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))

	c.Colonists = 0
	c.Tick(1, 1)
	if got := c.Readout.Food.Consumed; got != 0 {
		t.Errorf("an empty habitat ate %.4f/s", got)
	}

	c.Colonists = 4 // half of eight
	c.Tick(1, 1)
	if want := 4 * FoodPerColonist; math.Abs(c.Readout.Food.Consumed-want) > 1e-9 {
		t.Errorf("four colonists ate %.4f/s, want %.4f/s", c.Readout.Food.Consumed, want)
	}
}

// The two ores are separate ledgers. A crystal mine must not top up the iron
// a colony builds out of, or the ground under a mine would stop mattering.
func TestTheTwoOresDoNotMix(t *testing.T) {
	c, m := stocked(t, world.Crystal)
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))

	iron, crystal := c.Iron, c.Crystal
	c.Tick(1, 1)

	if c.Iron != iron {
		t.Errorf("iron moved by %.4f on a crystal flat", c.Iron-iron)
	}
	if c.Crystal <= crystal {
		t.Errorf("crystal did not move: %.4f", c.Crystal-crystal)
	}
	if c.Readout.CrystalMines != 1 || c.Readout.IronMines != 0 {
		t.Errorf("counted %d iron and %d crystal mines, want 0 and 1",
			c.Readout.IronMines, c.Readout.CrystalMines)
	}
}

// Demolition returns both materials. A battery that refunded only its iron
// would make experimenting with storage a one-way door.
func TestDemolishRefundsBothMaterials(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	at := world.FromOffset(4, 4)
	mustPlace(t, c, m, Battery, at)

	iron, crystal := c.Iron, c.Crystal
	if _, ok := c.Demolish(at); !ok {
		t.Fatal("nothing demolished")
	}

	wantIron, wantCrystal := Of(Battery).Refund()
	if math.Abs(c.Iron-iron-wantIron) > 1e-9 {
		t.Errorf("iron back %.2f, want %.2f", c.Iron-iron, wantIron)
	}
	if math.Abs(c.Crystal-crystal-wantCrystal) > 1e-9 {
		t.Errorf("crystal back %.2f, want %.2f", c.Crystal-crystal, wantCrystal)
	}
}

// Crystal gates placement in its own right: iron alone does not buy a battery.
func TestCrystalIsItsOwnGate(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	c.Crystal = Of(Battery).CrystalCost - 1

	err := c.CanPlace(m, Battery, world.FromOffset(4, 4))
	if err == nil {
		t.Fatal("bought a battery without the crystal for it")
	}
	if got := err.Error(); !contains(got, "crystal") {
		t.Errorf("error %q does not say which material is short", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
