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

	// Losing the last colonist is the end of the game, and it is said on its
	// own. Everything below would still be true - the grid is short, the
	// coolant loop is dry, the tank is empty - but they are symptoms of an
	// empty colony rather than things to go and fix, and a wall of advice
	// under a headstone reads as a colony that could still be saved.
	//
	// It cannot. Nothing is staffed, so nothing is produced, so nobody stays:
	// see cmd/balance, which tears a collapsed colony back to a single
	// habitat and watches it stay dead.
	if c.Settled && c.Colonists < 1 {
		return []Alert{{
			Level: LevelCritical,
			Text:  "COLONY LOST - no one left",
			Fix:   "nothing can be staffed; Esc to load or start again",
		}}
	}

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
			Fix:   powerFix(short, r.Daylight, r.Cap.Power),
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

	// People leaving is the worst thing that can be happening, so it is said
	// first and it names the cause. "Colonists are leaving" on its own sends a
	// player to check five rows; "no water" sends them to build an extractor.
	if support := min(r.Fed, r.Watered); support < 0.999 && c.Colonists > 0 {
		cause := "no food"
		switch {
		case r.Fed >= 0.999:
			cause = "no water"
		case r.Watered < 0.999:
			cause = "no food or water"
		}
		out = append(out, Alert{
			Level: LevelCritical,
			Text:  fmt.Sprintf("Colonists leaving - %s", cause),
			Fix:   lifeFix(r.Fed, r.Watered),
		})
	}

	// Labour, before the stock lines below it: a short-staffed colony is
	// producing less of everything, so it explains numbers a player would
	// otherwise try to fix one at a time.
	if r.Jobs > 0 && r.Staffing < 0.999 {
		short := r.Jobs - c.Colonists
		lvl := LevelWarn
		if r.Staffing < 0.5 {
			lvl = LevelCritical
		}
		out = append(out, Alert{
			Level: lvl,
			Text:  fmt.Sprintf("Short %.0f staff - everything at %.0f%%", short, r.Staffing*100),
			Fix:   "build a Habitat, and the food to keep it",
		})
	}

	// A full store is not a problem in itself, it is production being thrown
	// away, and nothing else on the panel says so: the rate keeps reading
	// healthy because the mine really is still running.
	out = append(out, spillAlert("Iron", r.Spilled.Iron, "more Mines will not help until there is room")...)
	out = append(out, spillAlert("Crystal", r.Spilled.Crystal, "more Mines will not help until there is room")...)
	out = append(out, spillAlert("Water", r.Spilled.Water, "build a Habitat, or stop an Extractor")...)
	out = append(out, spillAlert("Food", r.Spilled.Food, "build a Habitat to store it, or feed more colonists")...)

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

// lifeFix names the building that answers whichever half is short.
func lifeFix(fed, watered float64) string {
	switch {
	case fed < 0.999 && watered < 0.999:
		return "water and food both: an Extractor and a Greenhouse"
	case watered < 0.999:
		return "build an Ice Extractor, or a Condenser anywhere"
	}
	return "build a Greenhouse - lichen yields 1.5x"
}

// spillAlert reports production being lost for want of somewhere to put it.
//
// Separate from stockAlert because it is the opposite failure and reads
// nothing like it: the stock is not falling, the rate is not negative, and
// every number on the row looks healthy. The only evidence is that some of
// what was made did not arrive.
func spillAlert(name string, rate float64, fix string) []Alert {
	if rate < RateEpsilon {
		return nil
	}
	return []Alert{{
		Level: LevelInfo,
		Text:  fmt.Sprintf("%s store full - losing %.2f/s", name, rate),
		Fix:   fix,
	}}
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
