package colony

import (
	"math"
	"strings"
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// The whole reason Flow keeps both halves: a net rate cannot distinguish these
// two colonies, and they need opposite fixes.
func TestFlowSeparatesProductionFromConsumption(t *testing.T) {
	m := flatMap(t, world.Ice)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	// One habitat drawing water, no production.
	dry := New()
	dry.Iron = 10000
	dry.Crystal = 10000
	dry.Colonists = 100 // staffed; this test is about flows, not labour
	mustPlace(t, dry, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, dry, m, Habitat, world.FromOffset(4, 4))
	dry.Tick(1, 1)

	// Same net draw, but with an extractor running and more habitats.
	supplied := New()
	supplied.Iron = 10000
	supplied.Crystal = 10000
	supplied.Colonists = 100 // staffed; this test is about flows, not labour
	mustPlace(t, supplied, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, supplied, m, Extractor, world.FromOffset(3, 3))
	for i := 0; i < 6; i++ {
		mustPlace(t, supplied, m, Habitat, world.FromOffset(2+i, 8))
	}
	supplied.Tick(1, 1)

	if got := dry.Readout.Water.Produced; got != 0 {
		t.Errorf("colony with no extractor produced %.3f water", got)
	}
	if got := supplied.Readout.Water.Produced; got <= 0 {
		t.Errorf("colony with an extractor produced %.3f water", got)
	}
	if dry.Readout.Water.Consumed <= 0 {
		t.Error("a habitat drew no water")
	}
}

// Produced has to be what happened, not what was intended: a greenhouse with
// no water must report zero, or the readout tells the player their food supply
// is fine while the stock falls.
func TestProducedReportsWhatActuallyHappened(t *testing.T) {
	m := flatMap(t, world.Lichen)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	// Staffed: labour gates production, and this test is about something else.
	c.Colonists = 100
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))

	c.Water = 1e6
	c.Tick(1, 1)
	wet := c.Readout.Food.Produced
	if wet <= 0 {
		t.Fatalf("a watered greenhouse produced %.3f", wet)
	}

	c.Water = 0
	c.Tick(1, 1)
	if dry := c.Readout.Food.Produced; dry != 0 {
		t.Errorf("a dry greenhouse reported producing %.4f", dry)
	}
}

// Power satisfaction scales production, and the readout has to show the scaled
// figure — otherwise a brownout looks like a healthy colony whose stock is
// mysteriously not rising.
func TestReadoutShowsBrownoutScaledProduction(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	// Staffed: labour gates production, and this test is about something else.
	c.Colonists = 100
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 5))

	c.Tick(1, 0.25) // solar at a quarter: 3.5 against a demand of 6

	full := Of(Mine).MineOut * Yield(Mine, world.Dunes)
	if got := c.Readout.Iron.Produced; got >= full {
		t.Errorf("brownout production %.4f, want less than the full %.4f", got, full)
	}
	if got := c.Readout.Iron.Produced; math.Abs(got-full*c.Readout.Satisfaction) > 1e-9 {
		t.Errorf("production %.4f is not full rate times satisfaction %.4f", got, c.Readout.Satisfaction)
	}
}

func TestCountsTrackWhatIsStanding(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
	mustPlace(t, c, m, Mine, world.FromOffset(5, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(6, 4))
	c.Tick(1, 1)

	if got := c.Readout.Counts[Mine]; got != 2 {
		t.Errorf("Counts[Mine] = %d, want 2", got)
	}
	if got := c.Readout.Counts[SolarArray]; got != 1 {
		t.Errorf("Counts[SolarArray] = %d, want 1", got)
	}
	if got := c.Readout.Counts[Habitat]; got != 0 {
		t.Errorf("Counts[Habitat] = %d, want 0", got)
	}
}

func TestSecondsLeftOnlyCountsDown(t *testing.T) {
	if got := SecondsLeft(100, Flow{Produced: 2, Consumed: 1}); got != -1 {
		t.Errorf("a rising stock reported %.1f seconds left", got)
	}
	if got := SecondsLeft(100, Flow{Produced: 1, Consumed: 1}); got != -1 {
		t.Errorf("a level stock reported %.1f seconds left", got)
	}
	if got := SecondsLeft(100, Flow{Consumed: 2}); math.Abs(got-50) > 1e-9 {
		t.Errorf("SecondsLeft = %.3f, want 50", got)
	}
}

// The advisory is the feature. A colony draining its water has to say so, say
// when, and name the building that fixes it.
func TestDrainingWaterRaisesAnActionableAlert(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))

	c.Water = 5 // about a minute at a habitat's draw
	c.Tick(1, 1)

	alerts := c.Alerts()
	var found *Alert
	for i := range alerts {
		if strings.Contains(alerts[i].Text, "Water") {
			found = &alerts[i]
		}
	}
	if found == nil {
		t.Fatalf("no water alert; got %+v", alerts)
	}
	if found.Level != LevelCritical {
		t.Errorf("water alert level %v, want critical with %.0f left", found.Level, c.Water)
	}
	if found.Fix == "" {
		t.Error("water alert has no fix")
	}
	if !strings.Contains(found.Fix, "Condenser") && !strings.Contains(found.Fix, "Extractor") {
		t.Errorf("fix %q names no building that makes water", found.Fix)
	}
}

// A healthy colony must stay quiet, or the alert panel becomes noise the
// player learns to ignore.
func TestAHealthyColonySaysNothing(t *testing.T) {
	m := flatMap(t, world.Ice)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 400
	c.Crystal = 100
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Extractor, world.FromOffset(3, 3))
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))
	mustPlace(t, c, m, Habitat, world.FromOffset(5, 5))
	mustPlace(t, c, m, Habitat, world.FromOffset(5, 6))
	mustPlace(t, c, m, Habitat, world.FromOffset(5, 7))

	// Healthy means healthy under every rule the colony has, and two of them
	// are newer than this test: the jobs above have to be staffed, and the
	// stores have to have room left. Filling water and food to 1e5 used to be
	// the definition of well supplied and is now a colony throwing away
	// everything it makes.
	// Enough to staff the seven jobs, with beds to spare: the colony grows
	// during the fifty ticks below, and filling its housing raises an
	// advisory of its own.
	c.Colonists = 7
	c.Water, c.Food = 60, 60
	for i := 0; i < 50; i++ {
		c.Tick(0.1, 1)
	}

	if alerts := c.Alerts(); len(alerts) != 0 {
		t.Errorf("a well-supplied colony raised %d alerts: %+v", len(alerts), alerts)
	}
}

// A stock that is falling but has hours of buffer is not an emergency.
func TestSlowDrainDoesNotRaiseAnAlert(t *testing.T) {
	m := flatMap(t, world.Regolith)
	m.At(world.FromOffset(6, 6)).Terrain = world.Vent

	c := New()
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Geothermal, world.FromOffset(6, 6))
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))

	c.Water = 1e5 // draining, but not this century
	c.Tick(1, 1)

	for _, a := range c.Alerts() {
		if strings.Contains(a.Text, "Water") {
			t.Errorf("a stock with %.0f seconds left raised %q",
				SecondsLeft(c.Water, c.Readout.Water), a.Text)
		}
	}
}

// Recommending a solar array at midnight is advice that does nothing until
// morning, so the fix reads the clock. After dark it also reads the battery
// bank, because "build a Battery Bank" is useless to someone whose bank just
// ran flat and essential to someone who has none.
func TestPowerAdviceFollowsTheTimeOfDayAndTheReserve(t *testing.T) {
	m := flatMap(t, world.Dunes)

	newColony := func() *Colony {
		c := New()
		c.Iron = 10000
		c.Crystal = 10000
		mustPlace(t, c, m, Mine, world.FromOffset(4, 4))
		return c
	}

	t.Run("dark with no storage names the battery", func(t *testing.T) {
		c := newColony()
		c.Tick(1, 0)
		if got := powerAdvice(t, c.Alerts()); !strings.Contains(got, "Battery") {
			t.Errorf("advice %q does not mention a battery", got)
		}
	})

	t.Run("dark with a flat bank says it is flat", func(t *testing.T) {
		c := newColony()
		mustPlace(t, c, m, Battery, world.FromOffset(5, 5))
		c.Charge = 0
		c.Tick(1, 0)

		got := powerAdvice(t, c.Alerts())
		if !strings.Contains(got, "flat") {
			t.Errorf("advice %q does not say the reserve is flat", got)
		}
	})

	// There is deliberately no "partly charged but still short" case: the bank
	// covers the whole shortfall while it has anything in it, so a colony that
	// is short after dark has a flat bank or no bank. This pins that, because
	// it is the reason powerFix has two branches and not three.
	t.Run("a charged bank means no brownout at all", func(t *testing.T) {
		c := newColony()
		mustPlace(t, c, m, Battery, world.FromOffset(5, 5))
		c.Charge = 300
		c.Tick(1, 0)

		if c.Readout.Satisfaction < 0.999 {
			t.Errorf("satisfaction %.3f with a charged bank covering the draw", c.Readout.Satisfaction)
		}
		for _, a := range c.Alerts() {
			if strings.Contains(a.Text, "BROWNOUT") || strings.Contains(a.Text, "BLACKOUT") {
				t.Errorf("charged bank still raised %q", a.Text)
			}
		}
	})

	t.Run("daylight names solar", func(t *testing.T) {
		c := newColony()
		mustPlace(t, c, m, SolarArray, world.FromOffset(5, 5))
		mustPlace(t, c, m, Mine, world.FromOffset(6, 6))
		mustPlace(t, c, m, Mine, world.FromOffset(7, 7))
		c.Tick(1, 1)

		if got := powerAdvice(t, c.Alerts()); !strings.Contains(got, "Solar") {
			t.Errorf("daytime advice %q does not mention solar", got)
		}
	})
}

func TestBlackoutOutranksBrownout(t *testing.T) {
	m := flatMap(t, world.Dunes)
	c := New()
	// Staffed: labour gates production, and this test is about something else.
	c.Colonists = 100
	c.Iron = 10000
	c.Crystal = 10000
	mustPlace(t, c, m, Mine, world.FromOffset(4, 4))

	c.Tick(1, 0)
	if got := Worst(c.Alerts()); got != LevelCritical {
		t.Errorf("no power at all is level %v, want critical", got)
	}

	// Enough solar to cover most but not all of the demand.
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 5))
	c.Tick(1, 0.35) // 4.9 against 6
	if sat := c.Readout.Satisfaction; sat < 0.5 || sat >= 1 {
		t.Fatalf("satisfaction %.2f is not in the brownout band", sat)
	}
	if got := Worst(c.Alerts()); got != LevelWarn {
		t.Errorf("a brownout is level %v, want warn", got)
	}
}

func TestNoHousingIsReported(t *testing.T) {
	c := New()
	c.Tick(1, 1)

	var found bool
	for _, a := range c.Alerts() {
		if strings.Contains(a.Text, "No housing") {
			found = true
		}
	}
	if !found {
		t.Errorf("an empty colony did not report having no housing: %+v", c.Alerts())
	}
}

func TestDurationReadsAsTime(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{-1, "-"},
		{12, "12s"},
		{59, "59s"},
		{60, "1:00"},
		{252, "4:12"},
		{3600, "1h00m"},
		{5400, "1h30m"},
	}
	for _, tc := range cases {
		if got := Duration(tc.in); got != tc.want {
			t.Errorf("Duration(%.0f) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The condenser exists so water is obtainable without ice. If it ever needs
// special ground, a colony that landed away from an ice sheet is unwinnable
// again and nothing would say so.
func TestCondenserWorksOnOrdinaryGround(t *testing.T) {
	m := flatMap(t, world.Regolith)
	c := New()
	c.Iron = 10000
	c.Crystal = 10000

	if err := c.CanPlace(m, Condenser, world.FromOffset(4, 4)); err != nil {
		t.Fatalf("condenser on regolith: %v", err)
	}
	if got := Of(Condenser).WaterOut; got <= 0 {
		t.Errorf("condenser produces %.2f water", got)
	}
	// And it must stay the worse option, or the extractor is pointless.
	if Of(Condenser).WaterOut >= Of(Extractor).WaterOut {
		t.Error("the condenser matches or beats the extractor, so ice is worth nothing")
	}
	if Of(Condenser).PowerIn <= Of(Extractor).PowerIn {
		t.Error("the condenser is not more expensive to run than the extractor")
	}
}

func powerAdvice(t *testing.T, alerts []Alert) string {
	t.Helper()
	for _, a := range alerts {
		if strings.Contains(a.Text, "BROWNOUT") || strings.Contains(a.Text, "BLACKOUT") {
			return a.Fix
		}
	}
	t.Fatalf("no power alert in %+v", alerts)
	return ""
}
