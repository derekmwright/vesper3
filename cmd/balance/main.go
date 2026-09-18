// Command balance prints the economy's derived numbers: the ratios between
// buildings that the catalog only implies.
package main

import (
	"fmt"
	"math"

	"github.com/derekmwright/vesper3/internal/colony"
)

func of(k colony.Kind) colony.Spec { return colony.Of(k) }

// daylightAverage integrates the solar factor over one full cycle.
// daylightFrom is clamp(elevation*1.6, 0, 1); elevation is modelled as a sine.
func daylightAverage() (avg, dayFraction float64) {
	const steps = 20000
	sum, lit := 0.0, 0
	for i := range steps {
		el := math.Sin(2 * math.Pi * float64(i) / steps)
		d := math.Min(math.Max(el*1.6, 0), 1)
		sum += d
		if d > 0 {
			lit++
		}
	}
	return sum / steps, float64(lit) / steps
}

func main() {
	hab, mine := of(colony.Habitat), of(colony.Mine)
	gh, ext := of(colony.Greenhouse), of(colony.Extractor)
	con, geo := of(colony.Condenser), of(colony.Geothermal)
	sol, bat := of(colony.SolarArray), of(colony.Battery)

	fmt.Println("== DAY / NIGHT ==")
	avg, dayFrac := daylightAverage()
	const dayLen = 240.0
	fmt.Printf("  cycle %.0fs   lit for %.0f%% of it (%.0fs)   dark %.0fs\n",
		dayLen, dayFrac*100, dayFrac*dayLen, (1-dayFrac)*dayLen)
	fmt.Printf("  solar averages %.2f of nameplate over a cycle\n", avg)
	fmt.Printf("  so a %.0f-power array really delivers %.1f on average\n\n",
		sol.PowerOut, sol.PowerOut*avg)

	fmt.Println("== CROSSING THE NIGHT ==")
	night := (1 - dayFrac) * dayLen
	for _, draw := range []float64{10, 25, 50} {
		need := draw * night
		fmt.Printf("  a %.0f-power draw needs %.0f power-seconds -> %.1f batteries (%.0f iron, %.0f crystal)\n",
			draw, need, need/bat.PowerStore,
			math.Ceil(need/bat.PowerStore)*bat.IronCost,
			math.Ceil(need/bat.PowerStore)*bat.CrystalCost)
	}
	fmt.Printf("  one geothermal covers %.0f power all night for %.0f iron + %.0f crystal\n\n",
		geo.PowerOut, geo.IronCost, geo.CrystalCost)

	fmt.Println("== FOOD ==")
	fmt.Printf("  greenhouse grows %.2f/s; a full habitat eats %.2f/s\n", gh.FoodOut, hab.FoodIn)
	fmt.Printf("  one greenhouse feeds %.1f habitats = %.0f colonists\n",
		gh.FoodOut/hab.FoodIn, gh.FoodOut/hab.FoodIn*hab.Housing)
	fmt.Printf("  those habitats supply %.0f workers; the greenhouse wants %.0f\n\n",
		gh.FoodOut/hab.FoodIn*hab.Housing, gh.Jobs)

	fmt.Println("== WATER ==")
	fmt.Printf("  extractor %.2f/s (%.0f power, %.0f staff) | condenser %.2f/s (%.0f power, %.0f staff)\n",
		ext.WaterOut, ext.PowerIn, ext.Jobs, con.WaterOut, con.PowerIn, con.Jobs)
	fmt.Printf("  draws: habitat %.2f  greenhouse %.2f  mine %.2f  geothermal %.2f\n",
		hab.WaterIn, gh.WaterIn, mine.WaterIn, geo.WaterIn)
	perGreen := gh.FoodOut / hab.FoodIn
	cluster := perGreen*hab.WaterIn + gh.WaterIn
	fmt.Printf("  one greenhouse + the %.1f habitats it feeds draw %.2f/s -> %.2f extractors\n\n",
		perGreen, cluster, cluster/ext.WaterOut)

	fmt.Println("== LABOUR ==")
	fmt.Printf("  one habitat houses %.0f and employs %.0f\n", hab.Housing, hab.Jobs)
	for _, k := range colony.Buildable {
		s := of(k)
		if s.Jobs == 0 {
			continue
		}
		fmt.Printf("  %-22s %.0f staff -> %.2f habitats each\n", s.Name, s.Jobs, s.Jobs/hab.Housing)
	}
	fmt.Printf("  colonists arrive at %.2f/s: a habitat fills in %.0fs (%.0f%% of a day)\n\n",
		colony.GrowthRate, hab.Housing/colony.GrowthRate,
		hab.Housing/colony.GrowthRate/dayLen*100)

	fmt.Println("== STORAGE: TIME TO FILL FROM EMPTY ==")
	fmt.Printf("  iron    base %4d  +%.0f/mine   one mine %.2f/s -> %.0fs to fill base\n",
		colony.BaseIronStore, mine.IronStore, mine.MineOut, colony.BaseIronStore/mine.MineOut)
	fmt.Printf("  crystal base %4d  +%.0f/mine   one mine %.2f/s -> %.0fs\n",
		colony.BaseCrystalStore, mine.CrystalStore, mine.MineOut*0.4, colony.BaseCrystalStore/(mine.MineOut*0.4))
	fmt.Printf("  water   base %4d  +%.0f/habitat  one extractor %.2f/s -> %.0fs\n",
		colony.BaseWaterStore, hab.WaterStore, ext.WaterOut, colony.BaseWaterStore/ext.WaterOut)
	fmt.Printf("  food    base %4d  +%.0f/habitat  one greenhouse %.2f/s -> %.0fs\n\n",
		colony.BaseFoodStore, hab.FoodStore, gh.FoodOut, colony.BaseFoodStore/gh.FoodOut)

	fmt.Println("== THE OPENING (habitat + solar, as landed) ==")
	fmt.Printf("  start: %d iron, %d crystal, %d water, %d food\n",
		colony.StartingIron, colony.StartingCrystal, colony.StartingWater, colony.StartingFood)
	fmt.Printf("  8 colonists drink %.2f/s -> %.0fs of water\n", hab.WaterIn, colony.StartingWater/hab.WaterIn)
	fmt.Printf("  8 colonists eat   %.2f/s -> %.0fs of food\n", hab.FoodIn, colony.StartingFood/hab.FoodIn)
	fmt.Printf("  that is %.1f and %.1f day/night cycles\n",
		colony.StartingWater/hab.WaterIn/dayLen, colony.StartingFood/hab.FoodIn/dayLen)

	runVespite(dayLen)

	runSims()
	runWhatIf()
}

// runVespite is the progression arithmetic: how long a colony works for one
// upgrade, and what it has to keep mining to do it.
//
// Vespite is the only resource with a target rather than a rate to sustain, so
// the useful figures are all durations: a number of day/night cycles is
// something a player can feel, where "0.01 per second" is not.
func runVespite(dayLen float64) {
	syn := colony.Of(colony.Synthesizer)
	mine := colony.Of(colony.Mine)
	crystalRate := mine.MineOut * 0.4

	fmt.Println()
	fmt.Println("== VESPITE AND TIERS ==")
	fmt.Printf("  synthesizer %.3f/s from %.2f crystal/s, %.0f power, %.0f staff, holds %.0f\n",
		syn.VespiteOut, syn.CrystalIn, syn.PowerIn, syn.Jobs, syn.VespiteStore)
	fmt.Printf("  one crystal mine at %.2f/s feeds %.1f synthesizers\n",
		crystalRate, crystalRate/syn.CrystalIn)
	fmt.Printf("  filling one synthesizer from empty: %.0fs (%.1f cycles)\n",
		syn.VespiteStore/syn.VespiteOut, syn.VespiteStore/syn.VespiteOut/dayLen)
	fmt.Println()

	fmt.Println("  what one upgrade costs, and how long one synthesizer works for it:")
	for _, k := range []colony.Kind{colony.Mine, colony.Habitat, colony.Greenhouse, colony.Geothermal} {
		spec := colony.Of(k)
		line := ""
		for tier := uint8(2); tier <= colony.MaxTier; tier++ {
			cost, ok := colony.CostToReach(k, tier)
			if !ok {
				continue
			}
			cycles := cost.Vespite / syn.VespiteOut / dayLen
			line += fmt.Sprintf("   T%d %3.0fv +%3.0fi (%4.1f cycles)", tier, cost.Vespite, cost.Iron, cycles)
		}
		fmt.Printf("    %-18s%s\n", spec.Name, line)
	}

	// What the whole catalog costs to take to the top: the length of the game
	// if a player decides to finish it rather than merely survive it.
	var totalV, totalI float64
	for _, k := range colony.Buildable {
		for tier := uint8(2); tier <= colony.MaxTier; tier++ {
			if cost, ok := colony.CostToReach(k, tier); ok {
				totalV += cost.Vespite
				totalI += cost.Iron
			}
		}
	}
	fmt.Println()
	fmt.Printf("  one of everything at tier 3: %.0f vespite, %.0f iron\n", totalV, totalI)
	fmt.Printf("  at one synthesizer that is %.0f cycles of synthesis alone\n",
		totalV/syn.VespiteOut/dayLen)
}
