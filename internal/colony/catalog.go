// Package colony is the game rules: what can be built, where it is allowed,
// and what the colony produces and consumes while it stands there.
//
// It knows about the map and the grid, and nothing about the engine, the
// renderer, or entities. That boundary is deliberate — the whole economy is
// testable without a GPU, and every rule below is exercised that way.
package colony

import (
	"fmt"

	"github.com/derekmwright/vesper3/internal/world"
)

// Kind identifies a type of structure.
type Kind uint8

const (
	None Kind = iota
	Habitat
	SolarArray
	Mine
	Extractor
	Greenhouse
	Geothermal

	// Condenser is appended rather than slotted next to Extractor, where it
	// belongs on the hotbar, because a Kind is written into save files as its
	// number. Inserting one in the middle would turn every saved greenhouse
	// into a geothermal plant. Buildable below decides the display order, and
	// that is the one that is free to change.
	Condenser

	// Battery is appended for the same reason Condenser was: a Kind is its
	// number in a save file, so new ones go on the end and Buildable decides
	// where they appear.
	Battery

	kindCount
)

// Spec is the full definition of a structure: what it costs, where it is
// allowed to stand, and its rates in units per second at full supply.
type Spec struct {
	Name string
	Desc string

	// IronCost and CrystalCost are charged on placement; demolishing refunds
	// RefundFraction of both.
	//
	// Iron is the material everything is made of and crystal is what the
	// things that hold or convert energy need, so most of the catalog costs
	// iron alone and the two that do not are the two a colony reaches for
	// once it has stopped merely surviving.
	IronCost    float64
	CrystalCost float64

	// Color is the structure's paint. Buildings are lit geometry with a
	// per-entity tint rather than textures, so this is the whole look.
	Color [3]float32

	// Needs gates placement on what the terrain provides. A structure with no
	// need can go on any buildable tile.
	Needs Requirement

	// Power is generated (Out) or drawn (In). A colony that generates less
	// than it draws runs everything at the ratio between them, so the first
	// brownout is a design pressure rather than a failure state.
	PowerOut float64
	PowerIn  float64

	// SolarDependent scales PowerOut by how high the sun is. It is what makes
	// the day/night cycle a mechanic instead of a lighting effect.
	SolarDependent bool

	// MineOut is the rate a mine works at. What comes up is decided by the
	// ground, not by the building — see world.Ore — so there is one rate here
	// rather than one field per ore.
	MineOut float64

	WaterOut float64
	WaterIn  float64

	// FoodOut is grown; FoodIn is eaten. A habitat's FoodIn is the draw when
	// every bed is filled, and the real draw scales with how full it is, so
	// an empty habitat eats nothing.
	FoodOut float64
	FoodIn  float64

	// Housing is how many colonists can live here.
	Housing float64

	// Jobs is how many colonists it takes to run this, at full output.
	//
	// Labour is the third input alongside power and water, and it is what
	// makes a habitat something other than a cost centre: before this a
	// habitat housed people who drank, ate and did nothing, so the only
	// reason to build one was that the game kept offering more colonists.
	// Now every mine and greenhouse wants staff, and staff want somewhere to
	// live, which wants food, which wants a greenhouse, which wants staff.
	Jobs float64

	// Storage. A stock the colony has nowhere to put is a stock it loses, so
	// these are what a resource bar is drawn against — a bar needs a maximum
	// before "full" means anything.
	//
	// PowerStore is in power-seconds: a store of 600 covers a draw of 20 for
	// thirty seconds. The rest are in plain units of their resource.
	PowerStore   float64
	WaterStore   float64
	FoodStore    float64
	IronStore    float64
	CrystalStore float64
}

// Requirement is what a structure needs from the ground under it.
type Requirement uint8

const (
	NeedsNothing Requirement = iota
	NeedsOre
	NeedsFrozen
	NeedsGeothermal
)

// RefundFraction is how much of a structure's materials demolishing returns. Less than half would
// punish experimenting, and experimenting is the game.
const RefundFraction = 0.6

var catalog = [kindCount]Spec{
	Habitat: {
		Name:     "Habitat",
		Desc:     "Houses 8 colonists on water and food",
		IronCost: 40,
		Color:    [3]float32{0.82, 0.80, 0.74},
		PowerIn:  4,
		WaterIn:  0.10,

		// Eight beds at FoodPerColonist each. The test in colony_test holds
		// these two together so the catalog cannot drift from the constant
		// the growth and starvation rules are written against.
		FoodIn:  8 * FoodPerColonist,
		Housing: 8,
		Needs:   NeedsNothing,

		// A habitat is where the larder and the tank are, so it carries the
		// stores for the people living in it. That ties storage to housing,
		// which ties it to labour: a colony that expands to staff its mines
		// gains the room to keep what they dig.
		//
		// It employs nobody. Living somewhere is not a job.
		WaterStore: 60,
		FoodStore:  40,
	},
	SolarArray: {
		// No staff: a panel that needs someone standing next to it is not a
		// solar panel. Same for the battery bank below.
		Name:           "Solar Array",
		Desc:           "14 power, daylight only",
		IronCost:       25,
		Color:          [3]float32{0.22, 0.28, 0.46},
		PowerOut:       14,
		SolarDependent: true,
	},
	Mine: {
		Name: "Mine",
		Desc: "Iron from ferrous dunes, crystal from a silicate flat",

		// One building, two jobs, decided by where it stands. Crystal ground
		// works the rate down to 0.4 of this (see Yield) because crystal is
		// the scarcer half of the pair and gates the batteries and the
		// geothermal plants rather than the housing.
		IronCost: 30,
		Color:    [3]float32{0.46, 0.40, 0.34},
		PowerIn:  6,
		WaterIn:  0.12, // cutting slurry and dust suppression
		MineOut:  0.55,
		Needs:    NeedsOre,

		// The largest employer in the game, which is the point: ore is what
		// everything else is built out of, so the colony's first staffing
		// problem should be the thing it needs most.
		Jobs: 3,

		// A stockpile at the pithead. Both are declared because a mine does
		// not know which it will be until it is sited, and Spec is resolved
		// before the ground is; the tick credits whichever the ground yields.
		IronStore:    80,
		CrystalStore: 40,
	},
	Extractor: {
		Name:       "Ice Extractor",
		Desc:       "Water from an ice sheet",
		IronCost:   30,
		Color:      [3]float32{0.50, 0.66, 0.74},
		PowerIn:    5,
		WaterOut:   0.45,
		Needs:      NeedsFrozen,
		Jobs:       2,
		WaterStore: 50, // the holding tank it draws into
	},
	Greenhouse: {
		Name:      "Greenhouse",
		Desc:      "Food from water; lichen ground yields more",
		IronCost:  35,
		Color:     [3]float32{0.30, 0.56, 0.34},
		PowerIn:   4,
		WaterIn:   0.25,
		FoodOut:   0.40,
		Jobs:      2,
		FoodStore: 60, // what it can keep before the next harvest
	},
	Geothermal: {
		Name: "Geothermal Plant",
		Desc: "26 power day and night, on a vent, for water",

		// The water is the price of night power being unconditional. A plant
		// is a steam cycle and the cycle leaks, so it wants makeup water
		// forever — which means the colony that solved darkness has to solve
		// water first, and a plant with a dry tank winds down rather than
		// running on nothing.
		IronCost:    60,
		CrystalCost: 10,
		Color:       [3]float32{0.56, 0.30, 0.22},
		PowerOut:    26,
		WaterIn:     0.15,
		Needs:       NeedsGeothermal,

		// A steam cycle wants watching. It is the one generator with staff.
		Jobs:       3,
		WaterStore: 40, // the makeup reservoir
	},
	Battery: {
		Name: "Battery Bank",
		Desc: "Stores 1500 power-seconds; charges on surplus, covers the night",

		// The always-available answer to darkness, priced against the
		// location-gated one: a geothermal plant is 60 ore and needs a vent,
		// and several batteries cost more than that but can be built
		// anywhere. Same shape as the extractor and the condenser, which is
		// deliberate — the game asks the same question twice, about water and
		// about power, and a player who learned it once should recognise it.
		IronCost: 50,

		// The crystal sink. Storage is the thing worth walking to a silicate
		// flat for, and pricing it in the ore that only one terrain yields is
		// what turns "I should build batteries" into "I need a crystal mine
		// first".
		CrystalCost: 20,
		Color:       [3]float32{0.34, 0.46, 0.40},
		// 1500, not 600. A night is 120 seconds, so 600 carried a five-power
		// draw across it — while one geothermal plant covers twenty-six all
		// night for 60 iron and 10 crystal. Crossing a 25-power night on the
		// old number took five banks: 250 iron and 100 crystal, against a
		// crystal ceiling of 120. The "always-available answer to darkness"
		// was four times the price of the location-gated one and could not be
		// stockpiled for.
		//
		// At 1500 one bank carries 12.5 power through the night and two carry
		// 25 for 100 iron and 40 crystal. Geothermal is still cheaper, which
		// is right — it is the one that needs a vent.
		PowerStore: 1500,
		Needs:      NeedsNothing,
	},
	Condenser: {
		Name: "Atmospheric Condenser",
		Desc: "Water anywhere, slowly and at a price in power",

		// Water was only obtainable from ice, which is about one tile in
		// eighty and sits at altitude. A colony that landed without an ice
		// sheet in reach could not keep a habitat running at all, and nothing
		// on screen said so — the run was lost at worldgen. This is the
		// fallback that makes the water problem always solvable: worse than an
		// extractor on every axis, so it never makes ice not worth walking to.
		// Iron only, deliberately. It is the guarantee that water is always
		// solvable, and a guarantee that can itself be gated behind finding a
		// crystal flat is not one.
		IronCost:   35,
		Color:      [3]float32{0.42, 0.54, 0.62},
		PowerIn:    7,
		WaterOut:   0.18,
		Needs:      NeedsNothing,
		Jobs:       1,
		WaterStore: 30,
	},
}

// Buildable lists the kinds a player can place, in the order they appear on
// the hotbar. It is the one place that order is defined, and it is free to
// differ from the Kind values, which saves depend on.
var Buildable = []Kind{
	Habitat, SolarArray, Mine,
	Extractor, Condenser, Greenhouse, Geothermal,
	Battery,
}

// Affordable reports whether a colony can pay for this structure. It is the
// price half of CanPlace, split out so the hotbar can grey a slot without
// needing a tile to ask about.
func (s Spec) Affordable(iron, crystal float64) bool {
	return iron >= s.IronCost && crystal >= s.CrystalCost
}

// CostText is what a structure costs, written once so the hotbar, the tooltip
// and the status line cannot disagree about whether a battery needs crystal.
func (s Spec) CostText() string { return Materials(s.IronCost, s.CrystalCost) }

// Refund is what demolishing a structure returns.
func (s Spec) Refund() (iron, crystal float64) {
	return s.IronCost * RefundFraction, s.CrystalCost * RefundFraction
}

// Materials writes an iron and crystal pair the way the HUD wants it. The
// crystal half is dropped when there is none, because "40 iron + 0 crystal"
// invites the reader to work out whether the zero matters.
func Materials(iron, crystal float64) string {
	if crystal > 0 {
		return fmt.Sprintf("%.0f iron + %.0f crystal", iron, crystal)
	}
	return fmt.Sprintf("%.0f iron", iron)
}

// Of returns a kind's specification. None and out-of-range values return the
// zero Spec, which costs nothing and produces nothing.
func Of(k Kind) Spec {
	if k == None || k >= kindCount {
		return Spec{Name: "None"}
	}
	return catalog[k]
}

// String makes Kind printable in the HUD and in test failures.
func (k Kind) String() string { return Of(k).Name }

// Yield is the multiplier a kind gets from the ground it stands on: slower
// going in a crystal flat, fertile ground under a greenhouse. It is resolved
// once at placement and stored, so retuning the table never silently
// rebalances a colony that is already standing.
func Yield(k Kind, t world.Terrain) float64 {
	switch k {
	case Mine:
		if t.Info().Ore == world.OreCrystal {
			// Crystal is cut, not scooped. The slower rate is what keeps a
			// crystal flat from also being the best iron substitute.
			return 0.4
		}
		return 1.0
	case Greenhouse:
		if t.Info().Fertile {
			return 1.5
		}
		return 1.0
	default:
		return 1.0
	}
}

// Mines reports what a mine on this ground brings up, and how fast. A kind
// that is not a mine, or ground that yields nothing, reports world.OreNone at
// a rate of zero.
//
// This is the one place the building and the ground are combined, so the
// economy, the HUD and the tooltip cannot disagree about what a mine on a
// given tile is for.
func Mines(k Kind, t world.Terrain) (world.Ore, float64) {
	if k != Mine {
		return world.OreNone, 0
	}
	ore := t.Info().Ore
	if ore == world.OreNone {
		return world.OreNone, 0
	}
	return ore, Of(k).MineOut * Yield(k, t)
}

// meets reports whether terrain satisfies a requirement.
func (r Requirement) meets(t world.Terrain) bool {
	info := t.Info()
	switch r {
	case NeedsOre:
		return info.Ore != world.OreNone
	case NeedsFrozen:
		return info.Frozen
	case NeedsGeothermal:
		return info.Geothermal
	default:
		// Everything else needs ground that is simply stable enough to build
		// on, which rules out sea and vents.
		return info.Buildable
	}
}
