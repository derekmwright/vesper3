package main

import (
	"fmt"
	"math"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/world"
)

// simulate runs a build order through whole day/night cycles with no player
// input and reports whether the colony is still standing.
func simulate(label string, cycles int, build func(*colony.Colony, *world.Map)) {
	m, err := world.NewMap(24, 24, 7)
	if err != nil {
		panic(err)
	}
	for i := range m.Tiles {
		m.Tiles[i].Terrain = world.Regolith
		m.Tiles[i].Elevation = world.SeaLevel + 2
	}
	m.At(world.FromOffset(8, 8)).Terrain = world.Ice
	m.At(world.FromOffset(9, 8)).Terrain = world.Ice
	m.At(world.FromOffset(10, 8)).Terrain = world.Vent
	for x := range 6 {
		m.At(world.FromOffset(4+x, 12)).Terrain = world.Dunes
	}
	m.At(world.FromOffset(4, 14)).Terrain = world.Crystal

	// A methane sea within reach of the home cluster, so the coast-sited plant
	// is a build order this can actually run.
	for x := range 9 {
		t := m.At(world.FromOffset(4+x, 4))
		t.Terrain = world.Sea
		t.Elevation = world.SeaLevel - 1
	}

	c := colony.New()
	build(c, m)

	const dayLen, dt = 240.0, 0.1
	steps := int(float64(cycles) * dayLen / dt)

	// The first cycle is the colony populating from nothing, so minima are
	// taken after it. Otherwise every run reports a low of zero people and
	// the measurement says nothing at all.
	settle := int(dayLen / dt)

	minFood, minWater := math.Inf(1), math.Inf(1)
	peak, trough := 0.0, math.Inf(1)
	everFell := false

	for i := range steps {
		el := math.Sin(2 * math.Pi * float64(i) * dt / dayLen)
		c.Tick(dt, math.Min(math.Max(el*1.6, 0), 1))

		if i < settle {
			continue
		}
		minFood = math.Min(minFood, c.Food)
		minWater = math.Min(minWater, c.Water)
		peak = math.Max(peak, c.Colonists)
		trough = math.Min(trough, c.Colonists)
		if c.Colonists < peak-0.5 {
			everFell = true
		}
	}

	// Enough of a buffer to absorb one bad night, in the units the colony
	// starts with: about a tenth of the 80 it lands holding.
	const slack = 8.0

	verdict := "stable"
	switch {
	case c.Colonists < 1:
		// The lose condition, and it is terminal by design rather than by
		// accident. Nobody left means nothing staffed, and coolant is drawn
		// before life support - so a geothermal plant takes every drop the
		// extractors make, the tank never refills, and the population
		// oscillates against zero however much is torn down. Verified by
		// tearing a collapsed colony back to one habitat and watching it stay
		// dead. A colony that reaches this has lost, not stalled.
		verdict = "DEAD"
	case everFell:
		verdict = "LOSING PEOPLE"
	case minWater <= 0.01 || minFood <= 0.01:
		verdict = "RAN DRY"

	// Surviving on nothing in the tank is not the same as surviving. A colony
	// whose low-water mark is a rounding error has no reserve for the next
	// thing it builds, and calling that "stable" is the same mistake the
	// resource bars made before they were given a maximum.
	case minWater < slack || minFood < slack:
		verdict = "NO RESERVE"
	case c.Colonists < c.Readout.Housing-0.5:
		verdict = "under-housed"
	}

	fmt.Printf("  %-34s %-14s %2.0f/%2.0f people (low %2.0f)  food %5.1f (low %5.1f)  water %5.1f (low %5.1f)  vespite %5.1f  staffing %.0f%%\n",
		label, verdict, c.Colonists, c.Readout.Housing, trough,
		c.Food, minFood, c.Water, minWater, c.Vespite, c.Readout.Staffing*100)
}

// put places a structure for free but not for nothing: founding skips the
// reach rule, so this asserts it separately. A build order the player could
// not lay out is a build order whose numbers mean nothing.
func put(c *colony.Colony, m *world.Map, k colony.Kind, col, row int) {
	at := world.FromOffset(col, row)
	if colony.Of(k).Housing == 0 && !c.InReach(at) {
		panic(fmt.Sprintf("%v at %d,%d is out of reach of any habitat", k, col, row))
	}
	if err := c.Found(m, k, at); err != nil {
		panic(fmt.Sprintf("%v at %d,%d: %v", k, col, row, err))
	}
}

func runSims() {
	fmt.Println("\n== CAN IT SURVIVE? (whole cycles, no player input after the build) ==")

	simulate("landed, nothing built", 3, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.SolarArray, 7, 6)
	})

	simulate("+ greenhouse + extractor", 5, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.SolarArray, 7, 7)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Extractor, 8, 8)
	})

	simulate("... + 2 batteries", 5, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.SolarArray, 7, 7)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Battery, 6, 7)
		put(c, m, colony.Battery, 6, 8)
	})

	simulate("... + geothermal instead", 5, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Geothermal, 10, 8)
	})

	simulate("mid: 2 hab 2 gh 2 ext geo 2 mine", 6, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.Habitat, 6, 7)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Greenhouse, 5, 7)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Extractor, 9, 8)
		put(c, m, colony.Geothermal, 10, 8)

		// The dunes are six tiles from home, so mining them means planting an
		// outpost there first. That is the reach rule's whole cost: an extra
		// habitat's worth of food, water and power before the first ore.
		put(c, m, colony.Habitat, 4, 11)
		put(c, m, colony.Mine, 4, 12)
		put(c, m, colony.Mine, 5, 12)
	})

	simulate("mid, but 3 habitats", 6, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.Habitat, 6, 7)
		put(c, m, colony.Habitat, 6, 8)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Greenhouse, 5, 7)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Extractor, 9, 8)
		put(c, m, colony.Geothermal, 10, 8)
		put(c, m, colony.Habitat, 4, 11)
		put(c, m, colony.Mine, 4, 12)
		put(c, m, colony.Mine, 5, 12)
	})

	// What the reach rule actually charges for: the same industry, with and
	// without the outpost that the distance to it now requires.
	simulate("home cluster only, no mining", 6, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.Habitat, 6, 7)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Greenhouse, 5, 7)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Extractor, 9, 8)
		put(c, m, colony.Geothermal, 10, 8)
	})

	simulate("+ mining outpost (hab + 2 mines)", 6, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.Habitat, 6, 7)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Greenhouse, 5, 7)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Extractor, 9, 8)
		put(c, m, colony.Geothermal, 10, 8)
		put(c, m, colony.Habitat, 4, 11)
		put(c, m, colony.Mine, 4, 12)
		put(c, m, colony.Mine, 5, 12)
	})

	// And whether the outpost can feed itself locally instead of leaning on
	// the home greenhouses, which is the choice the rule is meant to create.
	simulate("+ outpost with its own greenhouse", 6, func(c *colony.Colony, m *world.Map) {
		put(c, m, colony.Habitat, 6, 6)
		put(c, m, colony.Habitat, 6, 7)
		put(c, m, colony.SolarArray, 7, 6)
		put(c, m, colony.Greenhouse, 5, 6)
		put(c, m, colony.Greenhouse, 5, 7)
		put(c, m, colony.Extractor, 8, 8)
		put(c, m, colony.Extractor, 9, 8)
		put(c, m, colony.Geothermal, 10, 8)
		put(c, m, colony.Habitat, 4, 11)
		put(c, m, colony.Greenhouse, 5, 11)
		put(c, m, colony.Mine, 4, 12)
		put(c, m, colony.Mine, 5, 12)
	})
}

// runWhatIf re-runs the failing builds against proposed retunes, so a
// recommendation is evidence rather than arithmetic.
func runWhatIf() {
	fmt.Println("\n== WHAT IF ==")

	mid := func(habs, exts int) func(*colony.Colony, *world.Map) {
		return func(c *colony.Colony, m *world.Map) {
			for i := range habs {
				put(c, m, colony.Habitat, 6, 6+i)
			}
			put(c, m, colony.SolarArray, 7, 6)
			put(c, m, colony.Greenhouse, 5, 6)
			put(c, m, colony.Greenhouse, 5, 7)
			if exts > 0 {
				put(c, m, colony.Extractor, 8, 8)
			}
			if exts > 1 {
				put(c, m, colony.Extractor, 9, 8)
			}
			if exts > 2 {
				put(c, m, colony.Condenser, 7, 9)
			}
			put(c, m, colony.Geothermal, 10, 8)
			put(c, m, colony.Habitat, 4, 11)
			put(c, m, colony.Mine, 4, 12)
			put(c, m, colony.Mine, 5, 12)
		}
	}

	simulate("3 habitats, 2 extractors (as is)", 6, mid(3, 2))

	// The progression build, and the reason vespite is gated on crystal: the
	// crystal flats are nowhere near the landing site, so synthesising
	// anything at all means a second outpost with its own habitat feeding a
	// crystal mine, on top of the methane plant the water budget already
	// wanted. Three clusters, joined by nothing but the decision to have them.
	progression := func(c *colony.Colony, m *world.Map) {
		mid(3, 2)(c, m)
		put(c, m, colony.Methane, 6, 5)
		put(c, m, colony.Habitat, 4, 13)
		put(c, m, colony.Mine, 4, 14)
		put(c, m, colony.Synthesizer, 7, 5)
	}
	simulate("... + crystal outpost + synthesizer", 10, progression)

	// The same build with the water and the staff the two new outposts
	// actually cost. A synthesizer is three more jobs and a crystal mine is
	// three more on top, which is most of a habitat's worth of people before
	// either produces anything.
	simulate("... + a second methane plant", 10, func(c *colony.Colony, m *world.Map) {
		progression(c, m)
		put(c, m, colony.Methane, 5, 5)
	})
	simulate("... and a fourth habitat", 10, func(c *colony.Colony, m *world.Map) {
		progression(c, m)
		put(c, m, colony.Methane, 5, 5)
		put(c, m, colony.Habitat, 7, 7)
	})
	simulate("... swap an extractor for methane", 6, func(c *colony.Colony, m *world.Map) {
		mid(3, 1)(c, m)
		put(c, m, colony.Methane, 6, 5)
	})
	simulate("... methane on top of both", 6, func(c *colony.Colony, m *world.Map) {
		mid(3, 2)(c, m)
		put(c, m, colony.Methane, 6, 5)
	})
	simulate("3 habitats, + a condenser", 6, mid(3, 3))
	simulate("2 habitats, 2 extractors", 6, mid(2, 2))
}
