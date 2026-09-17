package colony

import (
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// solarColony is a colony that runs entirely on sunlight, with as much storage
// as asked for: the arrangement storage exists to make viable.
func solarColony(t *testing.T, solar, mines, batteries int) (*Colony, *world.Map) {
	t.Helper()
	m := flatMap(t, world.Dunes)

	c := New()
	c.Iron = 100000
	c.Crystal = 100000

	col := 0
	place := func(k Kind, n int) {
		for range n {
			mustPlace(t, c, m, k, world.FromOffset(col%11, 2+col/11))
			col++
		}
	}
	place(SolarArray, solar)
	place(Mine, mines)
	place(Battery, batteries)
	return c, m
}

// The headline: a solar colony with enough storage keeps running after dark.
// Without storage the same colony blacks out, which is the state that sent the
// player looking for this building.
func TestStorageCarriesASolarColonyThroughTheNight(t *testing.T) {
	withBank, _ := solarColony(t, 2, 1, 2)
	withBank.Charge = withBank.Readout.Capacity // start the night charged
	withBank.Tick(0.1, 1)                       // one daylight tick to fill in Capacity
	withBank.Charge = withBank.Readout.Capacity

	noBank, _ := solarColony(t, 2, 1, 0)

	// One minute after dark.
	for range 600 {
		withBank.Tick(0.1, 0)
		noBank.Tick(0.1, 0)
	}

	if withBank.Readout.Satisfaction < 0.999 {
		t.Errorf("a banked colony ran at %.2f after a minute of darkness", withBank.Readout.Satisfaction)
	}
	if noBank.Readout.Satisfaction != 0 {
		t.Errorf("an unbanked solar colony ran at %.2f after dark, want 0", noBank.Readout.Satisfaction)
	}
	if withBank.Charge >= withBank.Readout.Capacity {
		t.Error("the bank carried the night without spending anything")
	}
}

// Surplus during the day has to go somewhere, or there is nothing to spend
// after dark.
func TestSurplusChargesTheBank(t *testing.T) {
	c, _ := solarColony(t, 3, 1, 1) // 42 power against a 6 draw

	if c.Charge != 0 {
		t.Fatalf("started with %.1f charge", c.Charge)
	}
	c.Tick(1, 1)

	if c.Charge <= 0 {
		t.Fatal("a colony with surplus banked nothing")
	}
	if c.Readout.ChargeRate <= 0 {
		t.Errorf("ChargeRate %.2f while charging, want positive", c.Readout.ChargeRate)
	}

	// It charges at the surplus, not faster.
	surplus := c.Readout.PowerSupply - c.Readout.PowerDemand
	if c.Readout.ChargeRate > surplus+1e-9 {
		t.Errorf("charged at %.2f from a surplus of %.2f", c.Readout.ChargeRate, surplus)
	}
}

// The bank cannot hold more than the cells that are standing.
func TestChargeIsCappedByCapacity(t *testing.T) {
	c, _ := solarColony(t, 4, 0, 1)

	for range 10000 {
		c.Tick(0.1, 1)
	}
	cap := c.Readout.Capacity
	if cap <= 0 {
		t.Fatal("no capacity from a battery")
	}
	if c.Charge > cap+1e-6 {
		t.Errorf("charge %.1f exceeds capacity %.1f", c.Charge, cap)
	}
	if math.Abs(c.Charge-cap) > 1e-6 {
		t.Errorf("charge settled at %.1f, want the full %.1f", c.Charge, cap)
	}
}

// Demolishing the cells cannot leave energy stored in cells that are gone.
func TestDemolishingBatteriesSpillsTheirCharge(t *testing.T) {
	c, _ := solarColony(t, 4, 0, 2)
	for range 2000 {
		c.Tick(0.1, 1)
	}
	full := c.Charge
	if full <= 0 {
		t.Fatal("bank never charged")
	}

	// Remove one of the two.
	var removed bool
	for _, b := range append([]Building(nil), c.Buildings...) {
		if b.Kind == Battery {
			c.Demolish(b.At)
			removed = true
			break
		}
	}
	if !removed {
		t.Fatal("no battery to demolish")
	}

	c.Tick(0.1, 1)
	if c.Charge > c.Readout.Capacity+1e-6 {
		t.Errorf("charge %.1f survives in %.1f of capacity", c.Charge, c.Readout.Capacity)
	}
}

// Discharge covers the shortfall exactly, and reports itself as negative so
// the panel can tell charging from draining without inferring it.
func TestDischargeCoversTheShortfallAndReportsIt(t *testing.T) {
	c, _ := solarColony(t, 1, 2, 1) // 12 power of draw, none at night
	c.Charge = 500

	before := c.Charge
	c.Tick(1, 0)

	if c.Readout.Satisfaction < 0.999 {
		t.Errorf("satisfaction %.3f while the bank had charge", c.Readout.Satisfaction)
	}
	if c.Readout.ChargeRate >= 0 {
		t.Errorf("ChargeRate %.2f while discharging, want negative", c.Readout.ChargeRate)
	}

	spent := before - c.Charge
	if math.Abs(spent-c.Readout.PowerDemand) > 1e-6 {
		t.Errorf("spent %.2f to cover a demand of %.2f", spent, c.Readout.PowerDemand)
	}
}

// A bank with less than the shortfall covers what it can and the rest is a
// brownout, rather than the whole colony stopping.
func TestAnAlmostFlatBankBrownsOutRatherThanBlacksOut(t *testing.T) {
	c, _ := solarColony(t, 1, 2, 1) // 12 draw
	c.Charge = 6                    // half a second of it

	c.Tick(1, 0)

	sat := c.Readout.Satisfaction
	if sat <= 0 || sat >= 1 {
		t.Errorf("satisfaction %.3f, want a partial brownout", sat)
	}
	if c.Charge != 0 {
		t.Errorf("charge %.3f left after covering what it could", c.Charge)
	}
}

// A colony with no batteries must behave exactly as it did before storage
// existed, or adding the building changed every game that does not use it.
func TestNoBatteriesMeansTheOldBehaviour(t *testing.T) {
	c, _ := solarColony(t, 1, 1, 0)

	c.Tick(1, 0) // night: 0 supply against 6 demand
	if c.Readout.Satisfaction != 0 {
		t.Errorf("satisfaction %.3f with no supply and no bank", c.Readout.Satisfaction)
	}
	if c.Readout.Capacity != 0 || c.Readout.Stored != 0 {
		t.Errorf("capacity %.1f stored %.1f without a battery", c.Readout.Capacity, c.Readout.Stored)
	}
	if c.Charge != 0 {
		t.Errorf("charge %.3f without a battery", c.Charge)
	}
}

// Charge is a stock like the others, so it has to survive a save.
func TestChargeIsPartOfTheSavedState(t *testing.T) {
	c, _ := solarColony(t, 3, 0, 1)
	for range 100 {
		c.Tick(0.1, 1)
	}
	if c.Charge <= 0 {
		t.Fatal("nothing banked to save")
	}

	// What a decoded save looks like: exported fields only.
	loaded := &Colony{
		Iron:      c.Iron,
		Crystal:   c.Crystal,
		Water:     c.Water,
		Food:      c.Food,
		Colonists: c.Colonists,
		Charge:    c.Charge,
		Buildings: c.Buildings,
	}
	loaded.Reindex()

	if loaded.Charge != c.Charge {
		t.Errorf("charge %.3f across the round trip, want %.3f", loaded.Charge, c.Charge)
	}
}
