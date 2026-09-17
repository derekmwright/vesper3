package colony

import "fmt"

// Level is how urgent an advisory is. It drives colour on screen and the order
// they are shown in.
type Level uint8

const (
	LevelGood Level = iota
	LevelInfo
	LevelWarn
	LevelCritical
)

// Alert is one thing the player should know, and what to do about it.
//
// The Fix matters as much as the Text. "Water -0.10/s" is a measurement;
// "water runs out in 4:12 — build an Atmospheric Condenser" is a decision. A
// builder that only reports measurements makes the player derive the same
// three sentences every time, and derive them wrong while they are learning
// what the buildings do.
type Alert struct {
	Level Level
	Text  string
	Fix   string
}

// Alert thresholds, in seconds of remaining stock.
const (
	criticalSeconds = 90
	warnSeconds     = 300
)

// Alerts reports what is wrong with the colony, worst first, and what would
// fix it. An empty result means nothing needs attention, which the caller can
// render as such.
func (c *Colony) Alerts() []Alert {
	r := c.Readout
	var out []Alert

	// Power first: it scales everything else, so a shortfall here is the
	// cause of numbers the player would otherwise try to fix individually.
	if r.PowerDemand > 0 && r.Satisfaction < 0.999 {
		short := r.PowerDemand - r.PowerSupply
		lvl, what := LevelWarn, "BROWNOUT"
		if r.Satisfaction < 0.5 {
			lvl, what = LevelCritical, "BLACKOUT"
		}
		out = append(out, Alert{
			Level: lvl,
			Text:  fmt.Sprintf("%s - %.0f%% power, output reduced", what, r.Satisfaction*100),
			Fix:   powerFix(short, r.Daylight, r.Capacity),
		})
	}

	// A dry coolant loop is upstream of the brownout above, not a second
	// symptom of it: the plants wind down because the tank is empty, and
	// building another generator would not help.
	if r.Counts[Geothermal] > 0 && r.Coolant < 0.999 {
		out = append(out, Alert{
			Level: LevelCritical,
			Text:  fmt.Sprintf("Geothermal at %.0f%% - coolant loop dry", r.Coolant*100),
			Fix:   "water first: an Ice Extractor, or a Condenser anywhere",
		})
	}

	// A reserve that is draining is the warning that comes before the
	// blackout, and the only one a player can act on while the lights are
	// still on.
	if r.ChargeRate < -0.001 && r.Stored > 0 {
		left := r.Stored / -r.ChargeRate
		if left <= warnSeconds {
			lvl := LevelWarn
			if left <= criticalSeconds {
				lvl = LevelCritical
			}
			out = append(out, Alert{
				Level: lvl,
				Text:  fmt.Sprintf("Reserve out in %s", Duration(left)),
				Fix:   "build more Battery Banks, or night power",
			})
		}
	}

	out = append(out, stockAlert("Water", c.Water, r.Water,
		"build an Ice Extractor, or a Condenser anywhere")...)
	out = append(out, stockAlert("Food", c.Food, r.Food,
		"build a Greenhouse - lichen yields 1.5x")...)

	// Housing is not a shortage, it is a ceiling: the colony has stopped
	// growing and nothing else says so.
	if r.Housing > 0 && c.Colonists >= r.Housing-0.01 {
		out = append(out, Alert{
			Level: LevelInfo,
			Text:  fmt.Sprintf("Housing full - %.0f in %.0f beds", c.Colonists, r.Housing),
			Fix:   "build a Habitat to keep growing",
		})
	}
	if r.Housing == 0 {
		out = append(out, Alert{
			Level: LevelWarn,
			Text:  "No housing - nobody lives here",
			Fix:   "build a Habitat",
		})
	}

	// Idle capacity is worth saying once the basics are covered: a mine with
	// no power is a common way to spend 30 ore on nothing.
	if r.Counts[Mine] > 0 && r.Iron.Produced == 0 && r.Crystal.Produced == 0 && r.PowerDemand > 0 {
		out = append(out, Alert{
			Level: LevelWarn,
			Text:  "Mines are stopped",
			Fix:   "no power at all - build a generator",
		})
	}

	// Crystal has one source and no alternative, so a colony without a
	// crystal mine is not short of a resource yet — it is short of the only
	// way to ever get one, and nothing else on screen says so.
	if r.CrystalMines == 0 && c.Crystal < Of(Battery).CrystalCost {
		out = append(out, Alert{
			Level: LevelInfo,
			Text:  fmt.Sprintf("Crystal %.0f and no crystal mine", c.Crystal),
			Fix:   "put a Mine on a Crystal Flat - batteries need it",
		})
	}

	return out
}

// stockAlert reports a store that is draining, escalating as it empties. A
// store that is merely low but filling is not a problem and says nothing.
func stockAlert(name string, stock float64, f Flow, fix string) []Alert {
	left := SecondsLeft(stock, f)
	if left < 0 {
		return nil
	}

	lvl := LevelInfo
	switch {
	case left <= criticalSeconds:
		lvl = LevelCritical
	case left <= warnSeconds:
		lvl = LevelWarn
	default:
		// Draining, but with more than five minutes of buffer. Worth showing
		// in the readout, not worth an advisory.
		return nil
	}

	return []Alert{{
		Level: lvl,
		Text: fmt.Sprintf("%s out in %s (make %.2f use %.2f)",
			name, Duration(left), f.Produced, f.Consumed),
		Fix: fix,
	}}
}

// powerFix names the generator that would actually help, which depends on the
// time of day: recommending solar at midnight is advice that does nothing
// until morning.
// powerFix names the answer that would actually help now.
//
// After dark that depends on whether the colony has storage at all: a player
// with no batteries needs to know they exist, and one whose bank just ran flat
// needs to know it was not enough rather than being told to build the thing
// they already built.
func powerFix(short, daylight, capacity float64) string {
	solar := Of(SolarArray)

	if daylight < 0.25 {
		// Only two cases reach here, not three. A bank with charge left in it
		// covers the whole shortfall — discharge is limited by what is stored,
		// not by a rate — so there is no "partly charged and still short"
		// state to advise about: if the grid is short after dark, either the
		// bank is flat or there is no bank.
		if capacity > 0 {
			return "reserve is flat - build more Battery Banks"
		}
		return "dark: a Battery Bank banks the day's surplus"
	}
	n := countNeeded(short, solar.PowerOut*daylight)
	return fmt.Sprintf("build %d more Solar Array(s), or a Geothermal", n)
}

func countNeeded(short, each float64) int {
	if each <= 0 {
		return 1
	}
	n := int(short/each) + 1
	if n < 1 {
		return 1
	}
	return n
}

// Duration formats seconds as m:ss, or as seconds alone under a minute. The
// HUD shows this next to a falling stock, where "4:12" reads as time and
// "252s" reads as a measurement.
func Duration(seconds float64) string {
	if seconds < 0 {
		return "-"
	}
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}

	// Anything past this is not a number a player acts on, and printing it in
	// full is how "31224955555h16m" ended up on screen. The cap is a backstop
	// under RateEpsilon rather than a substitute for it: a countdown this long
	// is already meaningless, whatever produced it.
	const capSeconds = 99*3600 + 59*60
	if seconds > capSeconds {
		return ">99h"
	}

	m := int(seconds) / 60
	s := int(seconds) % 60
	if m >= 60 {
		return fmt.Sprintf("%dh%02dm", m/60, m%60)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Worst returns the highest level among some alerts, for colouring a summary.
func Worst(alerts []Alert) Level {
	worst := LevelGood
	for _, a := range alerts {
		if a.Level > worst {
			worst = a.Level
		}
	}
	return worst
}
