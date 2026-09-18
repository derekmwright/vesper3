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

	verdict := "stable"
	switch {
	case c.Colonists < 1:
		verdict = "DEAD"
	case everFell:
		verdict = "LOSING PEOPLE"
	case minWater <= 0.01 || minFood <= 0.01:
		verdict = "RAN DRY"
	case c.Colonists < c.Readout.Housing-0.5:
		verdict = "under-housed"
	}

	fmt.Printf("  %-32s %-14s %2.0f/%2.0f people (low %2.0f)  food %4.0f (low %3.0f)  water %4.0f (low %3.0f)  staffing %.0f%%\n",
		label, verdict, c.Colonists, c.Readout.Housing, trough,
		c.Food, minFood, c.Water, minWater, c.Readout.Staffing*100)
}

func put(c *colony.Colony, m *world.Map, k colony.Kind, col, row int) {
	if err := c.Found(m, k, world.FromOffset(col, row)); err != nil {
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
			put(c, m, colony.Mine, 4, 12)
			put(c, m, colony.Mine, 5, 12)
		}
	}

	simulate("3 habitats, 2 extractors (as is)", 6, mid(3, 2))
	simulate("3 habitats, + a condenser", 6, mid(3, 3))
	simulate("2 habitats, 2 extractors", 6, mid(2, 2))
}
