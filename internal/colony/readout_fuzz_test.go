package colony

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/derekmwright/worldbuild/internal/world"
)

// The readout is the only thing a player ever sees of the economy, so it is
// the one place a number that is merely *arithmetically* correct can still be
// a bug. "empty in 31224955555h16m" was true — a stock of 0.0004 falling at
// 1e-17 a second really does last that long — and it was nonsense on screen.
//
// These tests drive random colonies through random days and assert that every
// derived number stays inside the range a person could read. They are the
// backstop for the class of defect rather than for the one instance of it.

// randomColony builds a colony of random structures on random ground, with
// random stores. Nothing here is tuned to be sensible: half these colonies are
// blacked out, dry, starving, or all three, because that is where the
// arithmetic gets strange.
func randomColony(rng *rand.Rand, t *testing.T) *Colony {
	t.Helper()

	m, err := world.NewMap(24, 24, rng.Int63())
	if err != nil {
		t.Fatal(err)
	}
	// Random ground under every tile, so mines land on all three cases:
	// ferrous, crystal, and ground that refuses them.
	grounds := []world.Terrain{
		world.Regolith, world.Dunes, world.Lichen, world.Basalt,
		world.Crystal, world.Ice, world.Vent,
	}
	for i := range m.Tiles {
		m.Tiles[i].Terrain = grounds[rng.Intn(len(grounds))]
		m.Tiles[i].Elevation = int8(world.SeaLevel + 1 + rng.Intn(4))
	}

	c := New()
	c.Iron = rng.Float64() * 5000
	c.Crystal = rng.Float64() * 5000
	c.Water = rng.Float64() * rng.Float64() * 200 // biased towards empty
	c.Food = rng.Float64() * rng.Float64() * 200
	c.Charge = rng.Float64() * 2000

	for range rng.Intn(40) {
		k := Buildable[rng.Intn(len(Buildable))]
		a := world.FromOffset(rng.Intn(24), rng.Intn(24))
		// Found rather than Place: siting rules still apply, but the colony is
		// not required to be able to afford its own fixture.
		_ = c.Found(m, k, a)
	}
	c.Colonists = rng.Float64() * 40
	return c
}

// checkReadout is the invariant every displayed number has to satisfy,
// whatever the colony is doing.
func checkReadout(t *testing.T, c *Colony, when string) {
	t.Helper()
	r := c.Readout

	finite := func(name string, v float64) {
		t.Helper()
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("%s: %s is %v", when, name, v)
		}
	}
	nonNeg := func(name string, v float64) {
		t.Helper()
		finite(name, v)
		if v < 0 {
			t.Errorf("%s: %s is %.6f, want >= 0", when, name, v)
		}
	}

	for name, stock := range map[string]float64{
		"Iron": c.Iron, "Crystal": c.Crystal, "Water": c.Water,
		"Food": c.Food, "Colonists": c.Colonists, "Charge": c.Charge,
	} {
		nonNeg(name, stock)
	}

	// A stock can never hold more than there are cells for it.
	if c.Charge > r.Capacity+1e-6 {
		t.Errorf("%s: charge %.3f exceeds capacity %.3f", when, c.Charge, r.Capacity)
	}

	// Ratios are ratios.
	for name, v := range map[string]float64{
		"Satisfaction": r.Satisfaction, "Coolant": r.Coolant,
	} {
		finite(name, v)
		if v < 0 || v > 1 {
			t.Errorf("%s: %s is %.6f, want 0..1", when, name, v)
		}
	}

	for name, f := range map[string]Flow{
		"Iron": r.Iron, "Crystal": r.Crystal, "Water": r.Water, "Food": r.Food,
	} {
		nonNeg(name+".Produced", f.Produced)
		nonNeg(name+".Consumed", f.Consumed)
	}

	// The headline: every countdown a player can be shown has to be a length
	// of time, not an artifact of dividing by nearly zero.
	for name, pair := range map[string]struct {
		stock float64
		flow  Flow
	}{
		"Water": {c.Water, r.Water},
		"Food":  {c.Food, r.Food},
	} {
		left := SecondsLeft(pair.stock, pair.flow)
		finite(name+" SecondsLeft", left)
		if left >= 0 && left > 100*3600 {
			t.Errorf("%s: %s runs out in %.0f seconds (%s) - that is not a countdown",
				when, name, left, Duration(left))
		}
		if d := Duration(left); strings.Contains(d, "e+") || len(d) > 8 {
			t.Errorf("%s: %s countdown renders as %q", when, name, d)
		}
	}
}

// The property: no sequence of ticks produces a number the panel cannot show.
func TestReadoutStaysReadable(t *testing.T) {
	rng := rand.New(rand.NewSource(20260917))

	for run := range 300 {
		c := randomColony(rng, t)

		// A day and a night, at the tick the game actually uses.
		for step := range 200 {
			daylight := math.Max(0, math.Sin(float64(step)/32))
			c.Tick(0.05, daylight)
			checkReadout(t, c, "run "+itoa(run)+" step "+itoa(step))
			if t.Failed() {
				return
			}
		}
	}
}

// The specific shape that produced the bad countdown: production and
// consumption that cancel to within floating-point noise. It is worth pinning
// on its own, because a fuzzer only finds it by luck and it is the case a
// working colony sits in for most of a session.
func TestABalancedLedgerReadsAsFlat(t *testing.T) {
	// Accumulated at runtime rather than written as a literal: the compiler
	// folds 0.1+0.09 to exactly 0.19 and the test would pass without ever
	// reaching the guard. Nineteen hundredths added one at a time do not.
	produced := 0.0
	for range 19 {
		produced += 0.01
	}
	f := Flow{Produced: produced, Consumed: 0.19}
	if f.Net() == 0 {
		t.Fatal("fixture no longer has the rounding error it is testing")
	}

	if got := f.Rate(); got != 0 {
		t.Errorf("Rate is %v, want exactly 0 for a balanced ledger", got)
	}
	if got := SecondsLeft(0.0004, f); got != -1 {
		t.Errorf("SecondsLeft is %v (%s), want -1: the stock is not falling",
			got, Duration(got))
	}
}

// A stock that is already empty has nothing to count down.
func TestAnEmptyStockCountsDownToNothing(t *testing.T) {
	f := Flow{Produced: 0, Consumed: 0.2}
	if got := SecondsLeft(0, f); got != 0 {
		t.Errorf("SecondsLeft on an empty store is %v, want 0", got)
	}
	if got := Duration(0); got != "0s" {
		t.Errorf("Duration(0) = %q", got)
	}
}

// Duration has to stay short enough to fit the panel whatever it is handed.
func TestDurationIsAlwaysShort(t *testing.T) {
	for _, secs := range []float64{0, 1, 59, 60, 61, 3599, 3600, 86400, 1e9, 1e15, math.MaxFloat64} {
		d := Duration(secs)
		if len(d) > 8 {
			t.Errorf("Duration(%g) = %q, too long for the row", secs, d)
		}
		if strings.ContainsAny(d, "e+") && d != ">99h" {
			t.Errorf("Duration(%g) = %q, which is not a length of time", secs, d)
		}
	}
}

// itoa without pulling strconv into the readable part of the test above.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
