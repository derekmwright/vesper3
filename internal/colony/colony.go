package colony

import (
	"errors"
	"fmt"

	"github.com/derekmwright/worldbuild/internal/hex"
	"github.com/derekmwright/worldbuild/internal/world"
)

// Starting stores. Enough to put down a power source, a mine and a habitat
// and still have room for one mistake.
//
// The crystal is the exception: thirty is one battery bank, or three
// geothermal plants, and not both. It is deliberately enough to make the
// choice interesting and not enough to avoid making it.
const (
	StartingIron    = 250
	StartingCrystal = 30
	StartingWater   = 80
	StartingFood    = 80
)

// Colonist appetite and behaviour, per second.
const (
	FoodPerColonist = 0.020
	GrowthRate      = 0.25
	StarveRate      = 0.15
)

// Building is one placed structure.
type Building struct {
	Kind Kind
	At   hex.Axial

	// Yield is the terrain multiplier, resolved when it was placed.
	Yield float64

	// Ore is what the ground under a mine yields, resolved at placement for
	// the same reason Yield is: the tick has no map to ask, and retuning the
	// terrain table should not quietly convert a standing iron mine into a
	// crystal one.
	Ore world.Ore

	// Facing is which way it points, as a sixth of a turn: 0 to 5. Sixths
	// because that is the grid's own symmetry — any other step would leave a
	// structure sitting crooked on its hexagon.
	//
	// It has no effect on the economy and is here anyway, because it is part
	// of what the player placed and has to come back when they reload.
	Facing uint8
}

// Facings is how many distinct headings a structure can have.
const Facings = 6

// Colony is the whole player-side game state: the ledger and what is standing
// on the map. It is plain data with no engine types in it, so it serialises to
// a save file as it is and ticks in a test without a window.
type Colony struct {
	// Iron builds things; Crystal builds the things that hold or convert
	// energy. Both come out of the same building sited on different ground.
	Iron      float64
	Crystal   float64
	Water     float64
	Food      float64
	Colonists float64

	// Charge is the energy in the colony's batteries, in power-seconds. It is
	// a stock like the others and saves with them, so a colony reloads with
	// the reserve it went to sleep with.
	Charge float64

	Buildings []Building

	// index maps a tile to its slot in Buildings. Rebuilt by Reindex after a
	// load, because a map does not survive the round trip through JSON in a
	// useful form.
	index map[hex.Axial]int

	// Readout is the last tick's derived numbers, for the HUD. Nothing reads
	// it back as input.
	Readout Readout
}

// Flow is one resource's traffic for a tick, in units per second.
//
// Both halves are kept rather than just the difference, because the net rate
// alone cannot answer the question a player actually has. "Water -0.10/s" does
// not say whether that is one extractor against three habitats or no
// extractors against one: the first needs another extractor, the second needs
// the habitat switched off, and the number is the same either way.
type Flow struct {
	// Produced and Consumed are what actually happened after every scaling —
	// power satisfaction, water shortfall, the lot — not what the buildings
	// would manage if they were fully supplied.
	Produced float64
	Consumed float64
}

// Net is the rate the stock is moving at.
func (f Flow) Net() float64 { return f.Produced - f.Consumed }

// Readout is what the last tick worked out, for display.
type Readout struct {
	PowerSupply float64
	PowerDemand float64

	// Satisfaction is the fraction of demanded power that was available, and
	// therefore the rate everything ran at. 1 means a healthy grid.
	Satisfaction float64

	Housing float64

	// Stored and Capacity are the battery bank. ChargeRate is positive while
	// charging and negative while drawing down, so the panel can say which is
	// happening without inferring it.
	Stored     float64
	Capacity   float64
	ChargeRate float64

	Iron    Flow
	Crystal Flow
	Water   Flow
	Food    Flow

	// IronMines and CrystalMines split Counts[Mine] by what the ground under
	// each one yields, which is the only way the panel can draw a mine's
	// output against what the mines that make that ore could manage.
	IronMines    int
	CrystalMines int

	// Coolant is the fraction of the geothermal plants' water draw that was
	// met. It is resolved before the grid, because a plant with a dry loop
	// cannot generate, and the rest of the colony's water depends on the grid.
	Coolant float64

	// Daylight is the solar factor the tick ran with, 0 at night.
	Daylight float64

	// Counts is how many of each kind are standing, indexed by Kind.
	Counts [kindCount]int
}

// RateEpsilon is the smallest net rate treated as real movement, in units per
// second.
//
// It exists because a balanced ledger does not come out exactly balanced in
// floating point: a colony producing 0.19 water a second and drinking 0.19 a
// second leaves a net of about -1e-17, which is not a leak but reads as one.
// Dividing a stock by it produced countdowns in the tens of billions of hours,
// which is the bug this constant closes.
//
// A tenth of a thousandth a second is about a third of a unit an hour. Nothing
// in the catalog moves that slowly, so nothing real is being rounded away.
const RateEpsilon = 1e-4

// SecondsLeft is how long a stock lasts at a flow's current net rate, or -1
// when it is not falling. It is the number that turns a negative rate into a
// decision: a shortfall with an hour of buffer is not the same problem as the
// same shortfall with ninety seconds.
//
// A stock that is already at zero returns zero rather than a countdown: there
// is nothing left to count down.
func SecondsLeft(stock float64, f Flow) float64 {
	net := f.Net()
	if net > -RateEpsilon {
		return -1
	}
	if stock <= 0 {
		return 0
	}
	return stock / -net
}

// Rate rounds a flow's net movement for display, so a ledger that balances to
// within floating-point noise prints as flat rather than as "-0.00/s".
func (f Flow) Rate() float64 {
	if net := f.Net(); net <= -RateEpsilon || net >= RateEpsilon {
		return net
	}
	return 0
}

// New returns a colony with starting stores and nothing built.
func New() *Colony {
	return &Colony{
		Iron:      StartingIron,
		Crystal:   StartingCrystal,
		Water:     StartingWater,
		Food:      StartingFood,
		Colonists: 0,
		index:     make(map[hex.Axial]int),
	}
}

// Errors placement can return. They are values rather than strings so the HUD
// can react to them and a test can assert on them.
var (
	ErrOffMap      = errors.New("outside the map")
	ErrOccupied    = errors.New("tile already built on")
	ErrTerrain     = errors.New("wrong ground")
	ErrTooPoor     = errors.New("not enough materials")
	ErrUnknownKind = errors.New("no such structure")
)

// At returns the building on a tile, if there is one.
func (c *Colony) At(a hex.Axial) (Building, bool) {
	c.ensureIndex()
	i, ok := c.index[a]
	if !ok {
		return Building{}, false
	}
	return c.Buildings[i], true
}

// fits reports whether a structure could stand on a tile, ignoring what it
// costs. Split from CanPlace so Found can reuse the siting rules without the
// price check, rather than restating them and drifting apart from it.
func (c *Colony) fits(m *world.Map, k Kind, a hex.Axial) error {
	if k == None || k >= kindCount {
		return ErrUnknownKind
	}
	tile := m.At(a)
	if tile == nil {
		return ErrOffMap
	}
	c.ensureIndex()
	if _, taken := c.index[a]; taken {
		return ErrOccupied
	}
	spec := Of(k)
	if !spec.Needs.meets(tile.Terrain) {
		return fmt.Errorf("%w: %s needs %s", ErrTerrain, spec.Name, spec.Needs)
	}
	return nil
}

// CanPlace reports why a structure cannot go on a tile, or nil if it can.
//
// It is the single source of truth for placement: the ghost preview, the
// click handler and the tests all call this, so a rule cannot be enforced in
// one and forgotten in another.
func (c *Colony) CanPlace(m *world.Map, k Kind, a hex.Axial) error {
	if err := c.fits(m, k, a); err != nil {
		return err
	}
	spec := Of(k)
	if c.Iron < spec.IronCost {
		return fmt.Errorf("%w: %s costs %.0f iron, have %.0f", ErrTooPoor, spec.Name, spec.IronCost, c.Iron)
	}
	if c.Crystal < spec.CrystalCost {
		return fmt.Errorf("%w: %s costs %.0f crystal, have %.0f", ErrTooPoor, spec.Name, spec.CrystalCost, c.Crystal)
	}
	return nil
}

// Place builds a structure facing the default direction.
func (c *Colony) Place(m *world.Map, k Kind, a hex.Axial) error {
	return c.PlaceFacing(m, k, a, 0)
}

// PlaceFacing builds a structure, charging its cost. It returns the same
// errors as CanPlace and changes nothing when it fails.
func (c *Colony) PlaceFacing(m *world.Map, k Kind, a hex.Axial, facing uint8) error {
	if err := c.CanPlace(m, k, a); err != nil {
		return err
	}
	spec := Of(k)
	c.Iron -= spec.IronCost
	c.Crystal -= spec.CrystalCost
	c.add(m, k, a, facing)
	return nil
}

// Found places a structure without charging for it, for the landing site: the
// first habitat and its power come down with the colonists rather than being
// built by them. Siting rules still apply — the lander does not set down on
// the sea.
func (c *Colony) Found(m *world.Map, k Kind, a hex.Axial) error {
	if err := c.fits(m, k, a); err != nil {
		return err
	}
	c.add(m, k, a, 0)
	return nil
}

func (c *Colony) add(m *world.Map, k Kind, a hex.Axial, facing uint8) {
	terrain := m.At(a).Terrain
	ore, _ := Mines(k, terrain)
	c.Buildings = append(c.Buildings, Building{
		Kind:   k,
		At:     a,
		Yield:  Yield(k, terrain),
		Ore:    ore,
		Facing: facing % Facings,
	})
	c.index[a] = len(c.Buildings) - 1
}

// Demolish removes whatever is on a tile and refunds part of its cost. It
// reports what was there.
func (c *Colony) Demolish(a hex.Axial) (Kind, bool) {
	c.ensureIndex()
	i, ok := c.index[a]
	if !ok {
		return None, false
	}
	b := c.Buildings[i]
	spec := Of(b.Kind)
	c.Iron += spec.IronCost * RefundFraction
	c.Crystal += spec.CrystalCost * RefundFraction

	// Swap-remove, then repair the index entry for whatever moved into the
	// hole. Rebuilding the whole index here would be O(n) per demolition for
	// no reason.
	last := len(c.Buildings) - 1
	c.Buildings[i] = c.Buildings[last]
	c.Buildings = c.Buildings[:last]
	delete(c.index, a)
	if i != last {
		c.index[c.Buildings[i].At] = i
	}
	return b.Kind, true
}

// Count returns how many of a kind are standing.
func (c *Colony) Count(k Kind) int {
	n := 0
	for _, b := range c.Buildings {
		if b.Kind == k {
			n++
		}
	}
	return n
}

// Tick advances the economy by dt seconds. daylight is 0 at night and 1 with
// the sun overhead; it only affects solar output.
//
// The order is the design, and every step depends only on the ones above it —
// which is why this is written as steps rather than as one loop:
//
//  1. Coolant. Power plants draw their makeup water straight off the tank,
//     before anything else, because the grid cannot be resolved until their
//     output is known and a plant with a dry loop cannot generate. Nothing
//     else can come first without making the resolution circular.
//  2. The grid. Supply against demand, the battery bank covering the gap, and
//     whatever is still short becomes the brownout that scales everything
//     below.
//  3. Water. What the rest of the colony draws, scaled by the grid and
//     clamped to the tank. The fraction met scales the outputs water is an
//     input to.
//  4. Production. Ore and food out; water out into the tank.
//  5. Food. Habitats feed the colonists living in them, and the fraction met
//     decides whether the colony grows or starves.
//
// Water and food produced this tick land in the store for the next one. At a
// tenth of a second that is a lag nobody can see, and it is what keeps every
// step above reading only numbers that are already settled.
func (c *Colony) Tick(dt, daylight float64) {
	if dt <= 0 {
		return
	}
	if daylight < 0 {
		daylight = 0
	}

	var r Readout
	r.Daylight = daylight

	// One pass over the buildings for every total the steps below work from.
	// Generation is split three ways by what scales it — the sun, the coolant
	// loop, or nothing — because those three are resolved at different points
	// and a single running total could not be scaled correctly afterwards.
	var (
		solarOut, cooledOut, firmOut float64
		ironOut, crystalOut          float64
		waterOut, foodOut            float64
		coolantIn, waterIn, foodIn   float64
	)
	for _, b := range c.Buildings {
		s := Of(b.Kind)

		switch {
		case s.SolarDependent:
			solarOut += s.PowerOut
		case s.PowerOut > 0 && s.WaterIn > 0:
			cooledOut += s.PowerOut
		default:
			firmOut += s.PowerOut
		}

		r.PowerDemand += s.PowerIn
		r.Housing += s.Housing
		r.Capacity += s.PowerStore
		r.Counts[b.Kind]++

		// A mine's product is the ground's, not the building's.
		switch b.Ore {
		case world.OreIron:
			ironOut += s.MineOut * b.Yield
			r.IronMines++
		case world.OreCrystal:
			crystalOut += s.MineOut * b.Yield
			r.CrystalMines++
		}

		waterOut += s.WaterOut * b.Yield
		foodOut += s.FoodOut * b.Yield
		foodIn += s.FoodIn

		// A generator's water is coolant and comes off the top; everything
		// else queues behind the grid.
		if s.PowerOut > 0 {
			coolantIn += s.WaterIn
		} else {
			waterIn += s.WaterIn
		}
	}

	// 1. Coolant. A plant that never drinks has a ratio of 1 and is untouched.
	coolant := float64(1)
	if want := coolantIn * dt; want > 0 {
		got := min(want, c.Water)
		c.Water -= got
		coolant = got / want
		r.Water.Consumed += got / dt
	}
	r.Coolant = coolant

	r.PowerSupply = solarOut*daylight + cooledOut*coolant + firmOut

	// Batteries demolished out from under a charged colony cannot leave more
	// energy stored than there are cells to hold it.
	c.Charge = min(c.Charge, r.Capacity)

	// 2. The grid, in order: generation first, then the battery bank covering
	// whatever generation missed, and only what is still short becomes a
	// brownout.
	//
	// Storage is the reason solar is a whole answer rather than half of one.
	// A colony running on sunlight banks its surplus by day and spends it
	// after dark, and the question the game asks becomes how much reserve to
	// build rather than whether to bother with solar at all.
	sat := float64(1)
	if r.PowerDemand > 0 {
		if r.PowerSupply >= r.PowerDemand {
			// Surplus: charge, up to what the cells can still hold.
			surplus := r.PowerSupply - r.PowerDemand
			room := r.Capacity - c.Charge
			taken := min(surplus*dt, room)
			c.Charge += taken
			r.ChargeRate = taken / dt
		} else {
			// Shortfall: discharge to cover it, as far as the reserve goes.
			deficit := r.PowerDemand - r.PowerSupply
			drawn := min(deficit*dt, c.Charge)
			c.Charge -= drawn
			r.ChargeRate = -drawn / dt

			sat = (r.PowerSupply + drawn/dt) / r.PowerDemand
		}
		if sat > 1 {
			sat = 1
		}
	} else if r.Capacity > 0 {
		// No demand at all: anything generated goes to the cells.
		taken := min(r.PowerSupply*dt, r.Capacity-c.Charge)
		c.Charge += taken
		r.ChargeRate = taken / dt
	}

	c.Charge = max(c.Charge, 0)
	r.Stored = c.Charge
	r.Satisfaction = sat

	// 3. What the rest of the colony draws. Scaled by the grid, because a
	// mine that is not turning is not pumping either — and because a colony
	// that has gone dark should not empty its tank while it is down.
	waterRatio := float64(1)
	if want := waterIn * sat * dt; want > 0 {
		got := min(want, c.Water)
		c.Water -= got
		waterRatio = got / want
		r.Water.Consumed += got / dt
	}

	// 4. Production. Everything below records what actually happened, divided
	// back out by dt into a per-second rate. Recording the achieved amount
	// rather than the intended one is the whole point: a greenhouse that ran
	// dry has to show as producing nothing, not as producing what it would
	// have.
	//
	// Ore and food need power and water both. The extractors need only power,
	// since what they draw on is the ice or the air rather than the tank.
	works := sat * waterRatio
	mined, cut := ironOut*works, crystalOut*works
	drawn := waterOut * sat
	grown := foodOut * works

	c.Iron += mined * dt
	c.Crystal += cut * dt
	c.Water += drawn * dt
	c.Food += grown * dt

	r.Iron.Produced = mined
	r.Crystal.Produced = cut
	r.Water.Produced = drawn
	r.Food.Produced = grown

	// 5. Habitats feed the people living in them. The draw scales with
	// occupancy so a habitat raised ahead of the colonists who will fill it
	// does not eat on their behalf — and because the colony-wide ratio makes
	// the total come out at exactly FoodPerColonist a head.
	//
	// It is not scaled by the grid: people eat in a blackout.
	occupancy := float64(0)
	if r.Housing > 0 {
		occupancy = min(c.Colonists/r.Housing, 1)
	}
	fed := float64(1)
	if want := foodIn * occupancy * dt; want > 0 {
		got := min(want, c.Food)
		c.Food -= got
		fed = got / want
		r.Food.Consumed = got / dt
	}

	// Colonists arrive to fill housing while there is food, and leave when
	// there is not.
	switch {
	case fed < 0.999:
		c.Colonists -= StarveRate * dt
	case c.Colonists < r.Housing:
		c.Colonists = min(c.Colonists+GrowthRate*dt, r.Housing)
	case c.Colonists > r.Housing:
		// Housing was demolished out from under them.
		c.Colonists = max(c.Colonists-GrowthRate*dt, r.Housing)
	}
	c.Colonists = max(c.Colonists, 0)

	// Stores are clamped at zero: floating point subtraction of a clamped
	// draw can still land a hair below it.
	c.Iron = max(c.Iron, 0)
	c.Crystal = max(c.Crystal, 0)
	c.Water = max(c.Water, 0)
	c.Food = max(c.Food, 0)

	c.Readout = r
}

// Reindex rebuilds the tile lookup. Call it after loading a colony from a
// save, where only the exported fields survived.
func (c *Colony) Reindex() {
	c.index = make(map[hex.Axial]int, len(c.Buildings))
	for i, b := range c.Buildings {
		c.index[b.At] = i
	}
}

func (c *Colony) ensureIndex() {
	if c.index == nil {
		c.Reindex()
	}
}

// String names a requirement in the terms the player sees on a failed
// placement.
func (r Requirement) String() string {
	switch r {
	case NeedsOre:
		return "ferrous or crystal ground"
	case NeedsFrozen:
		return "an ice sheet"
	case NeedsGeothermal:
		return "a thermal vent"
	default:
		return "solid ground"
	}
}
