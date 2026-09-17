package game

import (
	"testing"

	"github.com/derekmwright/worldbuild/internal/artcheck"
	"github.com/derekmwright/worldbuild/internal/colony"
	"github.com/derekmwright/worldbuild/internal/hex"
)

// The battery bank shows two different things, and the split is the point.
//
// The four fill strips are a level: how much is stored. The pilot lamp on the
// crown is a state: what is happening to it. A bank sitting at 60% looks
// identical whether it is filling in the last of the daylight or draining into
// the night, and at dusk that difference is the whole question.

func batteryGame(charge, capacity, rate float64) *Game {
	g := &Game{Colony: colony.New()}
	g.Colony.Readout.Stored = charge
	g.Colony.Readout.Capacity = capacity
	g.Colony.Readout.ChargeRate = rate
	return g
}

// Strips fill from the bottom. Half a bank lights the bottom half, and it is
// the bottom half — a bank that filled downwards would read as draining.
func TestChargeStripsFillFromTheBottom(t *testing.T) {
	g := batteryGame(600, 1200, 0) // half

	var lit []int
	for n := 1; n <= artcheck.BatteryStrips; n++ {
		if _, gain := g.batteryAppearance(hex.Axial{}, n); gain > 0 {
			lit = append(lit, n)
		}
	}

	if len(lit) != 2 {
		t.Fatalf("half a bank lit strips %v, want two of them", lit)
	}
	if lit[0] != 1 || lit[1] != 2 {
		t.Errorf("half a bank lit strips %v, want the bottom two", lit)
	}
}

// Every level between empty and full lights a different number of strips, or
// the indicator is not reporting anything.
func TestStripCountTracksTheCharge(t *testing.T) {
	cases := []struct {
		charge float64
		want   int
	}{
		{0, 0},
		{0.2, 1},
		{0.45, 2},
		{0.7, 3},
		{1, 4},
	}
	for _, tc := range cases {
		g := batteryGame(tc.charge*1200, 1200, 0)
		lit := 0
		for n := 1; n <= artcheck.BatteryStrips; n++ {
			if _, gain := g.batteryAppearance(hex.Axial{}, n); gain > 0 {
				lit++
			}
		}
		if lit != tc.want {
			t.Errorf("%.0f%% charge lit %d strips, want %d", tc.charge*100, lit, tc.want)
		}
	}
}

// The lamp distinguishes the states a level cannot.
func TestStatusLampSeparatesFillingFromDraining(t *testing.T) {
	const cap = 1200

	filling, _ := batteryGame(cap*.6, cap, 4).batteryStatus(hex.Axial{})
	draining, _ := batteryGame(cap*.6, cap, -4).batteryStatus(hex.Axial{})
	holding, _ := batteryGame(cap*.6, cap, 0).batteryStatus(hex.Axial{})

	if filling == draining {
		t.Error("a bank filling and a bank draining show the same lamp; " +
			"that is the one thing the strips cannot already say")
	}
	if holding == filling || holding == draining {
		t.Error("a bank holding steady is not distinguishable from one that is moving")
	}
}

// Full and flat are their own states, so a bank at rest still says which end
// of the range it is resting at.
func TestStatusLampCallsFullAndFlat(t *testing.T) {
	const cap = 1200

	full, fullGain := batteryGame(cap, cap, 0).batteryStatus(hex.Axial{})
	flat, flatGain := batteryGame(0, cap, 0).batteryStatus(hex.Axial{})

	if full == flat {
		t.Error("a full bank and a flat one show the same lamp")
	}
	if fullGain <= 0 || flatGain <= 0 {
		t.Errorf("a lamp at rest is unlit: full %.2f flat %.2f", fullGain, flatGain)
	}
}

// A colony with no batteries has no lamp to drive, and asking must not divide
// by a capacity of zero.
func TestStatusLampIsDarkWithoutABank(t *testing.T) {
	_, gain := batteryGame(0, 0, 0).batteryStatus(hex.Axial{})
	if gain != 0 {
		t.Errorf("a colony with no capacity lit its status lamp at %.2f", gain)
	}
}

// Floating-point noise in the ledger must not make the lamp flicker between
// filling and draining. It uses the economy's own epsilon for that.
func TestStatusLampIgnoresRoundingNoise(t *testing.T) {
	const cap = 1200
	still, _ := batteryGame(cap*.6, cap, 0).batteryStatus(hex.Axial{})
	noisy, _ := batteryGame(cap*.6, cap, colony.RateEpsilon/10).batteryStatus(hex.Axial{})

	if still != noisy {
		t.Error("a rate below RateEpsilon changed the lamp; it will flicker on a balanced grid")
	}
}
