package colony

import (
	"errors"
	"fmt"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
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

// What the lander itself can hold, before a single building is placed.
//
// Every stock has a ceiling, and production past it is lost.
//
// The reason that matters most is not the interface. It is that an uncapped
// economy pays a player for leaving the game running: walk away for ten
// minutes and come back to enough iron that the next hour of decisions has
// already been made for you. A ceiling means time alone earns nothing — what
// earns is building somewhere to put it, which is a decision, which is the
// game.
//
// The interface follows from that rather than the other way round. A bar
// needs a maximum before "full" can mean anything: without one the only
// honest things to draw are a rate or a countdown, and both read as "how much
// have I got" to everyone who has ever seen a bar.
//
// These clear the starting stores with room to spare. A colony that began
// overflowing on its first tick would be teaching the mechanic by punishing
// something the player did not do.
const (
	BaseIronStore    = 400
	BaseCrystalStore = 120
	BaseWaterStore   = 140
	BaseFoodStore    = 140
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

	// TierLevel is 1, 2 or 3, and 0 in a save written before tiers existed.
	// Read it through Tier, which resolves the zero; see tier.go.
	TierLevel uint8
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

	// Vespite is the only stock with nothing to spend it on but upgrades. It
	// is deliberately not a build material: a resource that did both would be
	// spent on whichever was cheaper that minute, and the point of it is to
	// be a decision the colony saves up for.
	Vespite float64

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

	// Jobs is how many colonists the standing structures want, and Staffing
	// is the fraction of that the colony can actually supply. It scales
	// production exactly as power satisfaction does.
	Jobs     float64
	Staffing float64

	// Life support: the fraction of what the colonists needed that they got.
	// Both are kept rather than only the worse of them, because "they are
	// leaving" is a different sentence from "they are leaving because the
	// tank is empty", and only the second one is worth reading.
	Fed     float64
	Watered float64

	// Cap is how much of each stock the colony can hold. Every bar on the
	// panel is drawn against it.
	Cap Capacities

	// Spilled is what was produced past the ceiling and lost, per second. It
	// is the number that turns a full bar from a fact into an instruction.
	Spilled Capacities

	// Stored and ChargeRate are the battery bank; its ceiling is Cap.Power.
	// ChargeRate is positive while charging and negative while drawing down,
	// so the panel can say which is happening without inferring it.
	Stored     float64
	ChargeRate float64

	Iron    Flow
	Crystal Flow
	Water   Flow
	Food    Flow
	Vespite Flow

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

// MaxCountdown is the longest runway worth reporting, in seconds. Four days
// and change.
//
// Past this a countdown is not information. A drain a hair over RateEpsilon
// against a full store is a real number - 140 units at 0.0001 a second really
// is two hundred hours - and it is still nothing a player will act on, or
// should be invited to worry about. Duration caps its *rendering* at ">99h",
// which keeps it off the screen but leaves every caller handling a value it
// cannot use.
const MaxCountdown = 100 * 3600

// SecondsLeft is how long a stock lasts at a flow's current net rate, or -1
// when it is not falling fast enough to matter. It is the number that turns a
// negative rate into a decision: a shortfall with an hour of buffer is not the
// same problem as the same shortfall with ninety seconds.
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
	if left := stock / -net; left <= MaxCountdown {
		return left
	}
	return -1
}

// Rate rounds a flow's net movement for display, so a ledger that balances to
// within floating-point noise prints as flat rather than as "-0.00/s".
func (f Flow) Rate() float64 {
	if net := f.Net(); net <= -RateEpsilon || net >= RateEpsilon {
		return net
	}
	return 0
}

// Capacities is a figure per stock: how much can be held, or how much was
// lost for want of somewhere to put it.
//
// One struct rather than five fields on Readout twice over, because every
// consumer wants them together — the panel draws five bars, the advisory
// strip checks five ceilings — and because a sixth resource should be one line
// here rather than a hunt through the tick.
type Capacities struct {
	Water   float64
	Food    float64
	Iron    float64
	Crystal float64
	Power   float64
	Vespite float64
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
// BuildRadius is how far from a habitat anything else may be built, in tiles.
//
// Labour was colony-wide and abstract: a mine on the far side of the continent
// drew on the same pool of staff as one next door, and the map had no say in
// where a colony went. This is the rule that gives the workforce a place to be.
// Someone has to walk to that mine.
//
// It makes the habitat the anchor of expansion rather than a supply of bodies.
// Reaching a distant ice sheet, a thermal vent or a stretch of coast is no
// longer a matter of clicking on it — it means planting an outpost first and
// feeding it, which is a decision with a cost rather than a free choice of
// tile. Colonies come out as clusters joined by intent instead of sprawl.
//
// Four is a footprint of thirty-seven tiles per habitat: room to lay out a
// working cluster around one, not enough to cover a continent from the landing
// site.
const BuildRadius = 4

var (
	ErrOffMap      = errors.New("outside the map")
	ErrOccupied    = errors.New("tile already built on")
	ErrTerrain     = errors.New("wrong ground")
	ErrTooPoor     = errors.New("not enough materials")
	ErrUnknownKind = errors.New("no such structure")
	ErrNoHabitat   = errors.New("out of reach")
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
func (c *Colony) fitsGround(m *world.Map, k Kind, a hex.Axial) error {
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
	if !spec.Needs.metBy(m, a) {
		return fmt.Errorf("%w: %s needs %s", ErrTerrain, spec.Name, spec.Needs)
	}
	return nil
}

// fits is fitsGround plus the rule that someone has to be able to get there.
func (c *Colony) fits(m *world.Map, k Kind, a hex.Axial) error {
	if err := c.fitsGround(m, k, a); err != nil {
		return err
	}
	spec := Of(k)

	// Everything but a habitat has to be within walking distance of one.
	// Habitats are exempt because they are what creates the reach: a rule that
	// required one near a habitat could never be satisfied for the first one,
	// and would leave a colony unable to expand past its landing site.
	if spec.Housing == 0 && !c.InReach(a) {
		return fmt.Errorf("%w: %s needs a Habitat within %d tiles",
			ErrNoHabitat, spec.Name, BuildRadius)
	}
	return nil
}

// InReach reports whether a tile is close enough to a habitat to be staffed.
//
// Exported because "can anyone get there" is a question worth asking outside
// the placement check itself — an overlay shading the reachable ground, or a
// balance run that wants its build orders to be ones a player could actually
// make.
func (c *Colony) InReach(a hex.Axial) bool {
	for _, b := range c.Buildings {
		if Of(b.Kind).Housing > 0 && hex.Distance(b.At, a) <= BuildRadius {
			return true
		}
	}
	return false
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

// CanFound reports whether Found would succeed: the ground rules, and nothing
// else. No cost, because founding does not charge, and no reach, because
// founding is what puts the first habitat down.
//
// It exists so that callers which place by founding can also *search* by
// founding. Asking CanPlace where a structure may go and then founding it
// there is two different standards, and the gap shows up as a site rejected
// by the search that Found would have accepted.
func (c *Colony) CanFound(m *world.Map, k Kind, a hex.Axial) error {
	return c.fitsGround(m, k, a)
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
	// Ground rules still apply — the lander does not set down on the sea —
	// but not the habitat reach rule. Reach is about the colony walking to
	// work, and nothing founded was walked to: the landing site arrives with
	// its first habitat, and there is no habitat to be near before that.
	if err := c.fitsGround(m, k, a); err != nil {
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
		vespiteOut, crystalIn        float64

		// Water drawn to keep people alive rather than to run a process.
		// Rationed separately from the industrial draw above, and on the same
		// terms as food: not scaled by the grid, because a blackout does not
		// stop anyone being thirsty.
		lifeWaterIn float64
	)

	// The lander's own holds, before anything is built on top of them.
	r.Cap = Capacities{
		Iron:    BaseIronStore,
		Crystal: BaseCrystalStore,
		Water:   BaseWaterStore,
		Food:    BaseFoodStore,
	}
	for _, b := range c.Buildings {
		// Everything below reads the tier-scaled spec, so a tier-3 mine is
		// simply a mine with bigger numbers and the rest of the tick does not
		// have to know tiers exist. Jobs is the one field scaling leaves
		// alone — see tier.go for why that is the whole mechanic.
		s := Of(b.Kind).scaled(TierScale(b.Tier()))

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
		r.Jobs += s.Jobs
		r.Counts[b.Kind]++

		r.Cap.Power += s.PowerStore
		r.Cap.Water += s.WaterStore
		r.Cap.Food += s.FoodStore
		r.Cap.Vespite += s.VespiteStore

		// A mine's product is the ground's, not the building's.
		switch b.Ore {
		case world.OreIron:
			ironOut += s.MineOut * b.Yield
			r.IronMines++
			r.Cap.Iron += s.IronStore
		case world.OreCrystal:
			crystalOut += s.MineOut * b.Yield
			r.CrystalMines++
			r.Cap.Crystal += s.CrystalStore
		}

		waterOut += s.WaterOut * b.Yield
		foodOut += s.FoodOut * b.Yield
		foodIn += s.FoodIn

		// Synthesis. Not scaled by Yield: what the ground offers a lattice
		// grower is the sea beside it, which siting already decided, and a
		// terrain multiplier on top would be the same rule charged twice.
		vespiteOut += s.VespiteOut
		crystalIn += s.CrystalIn

		// Three kinds of thirst, rationed at three different points. A
		// generator's coolant comes off the top, because the grid cannot be
		// resolved without it. Life support is exempt from the grid entirely.
		// Everything else queues behind it.
		//
		// Housing is what marks life support, rather than naming the habitat:
		// anything people live in draws water for them.
		switch {
		case s.PowerOut > 0:
			coolantIn += s.WaterIn
		case s.Housing > 0:
			lifeWaterIn += s.WaterIn
		default:
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

	// Storage demolished out from under a full colony cannot leave more in a
	// stock than there is now room for. The battery has always done this; the
	// other four stocks gained ceilings and needed the same, and a test
	// demolishing a full habitat is what noticed they had not got it.
	//
	// Silent rather than reported as a spill: Spilled is a rate, and this is a
	// single loss at the instant a building came down, which the player just
	// asked for and watched happen.
	c.Charge = min(c.Charge, r.Cap.Power)
	c.Iron = min(c.Iron, r.Cap.Iron)
	c.Crystal = min(c.Crystal, r.Cap.Crystal)
	c.Water = min(c.Water, r.Cap.Water)
	c.Food = min(c.Food, r.Cap.Food)
	c.Vespite = min(c.Vespite, r.Cap.Vespite)

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
			room := r.Cap.Power - c.Charge
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
	} else if r.Cap.Power > 0 {
		// No demand at all: anything generated goes to the cells.
		taken := min(r.PowerSupply*dt, r.Cap.Power-c.Charge)
		c.Charge += taken
		r.ChargeRate = taken / dt
	}

	c.Charge = max(c.Charge, 0)
	r.Stored = c.Charge
	r.Satisfaction = sat

	// 3. Labour. One ratio for the colony, the same shape as the grid above
	// it: a colony short of people runs everything slower rather than picking
	// which mine to switch off.
	//
	// Colonists are the input that cannot be bought. Power is a building away
	// and water is a building away, but staff arrive only as fast as housing
	// and food allow, which is what makes expanding a decision rather than a
	// formality.
	r.Staffing = 1
	if r.Jobs > 0 {
		r.Staffing = min(c.Colonists/r.Jobs, 1)
	}

	// 4. What the rest of the colony draws. Scaled by the grid and by the
	// staff, because a mine that is not turning is not pumping either — and
	// because a colony that has gone dark, or emptied out, should not drain
	// its tank while it is down.
	//
	// The staffing half was missing and it made losing people unrecoverable.
	// Production was scaled by staff and consumption was not, so a greenhouse
	// nobody worked still drank a quarter of a unit a second and grew nothing.
	// A colony that dipped below its own water supply lost colonists, which
	// cost it the extractors that would have refilled the tank, while the
	// greenhouses kept drinking — and there was no floor to it. Six cycles
	// from a working colony to nobody left.
	waterRatio := float64(1)
	if want := waterIn * sat * r.Staffing * dt; want > 0 {
		got := min(want, c.Water)
		c.Water -= got
		waterRatio = got / want
		r.Water.Consumed += got / dt
	}

	// Crystal is drawn here rather than in the production step below, for the
	// same reason water is: it is a feedstock, and a feedstock has to be taken
	// out of the stock that exists before this tick's mining is added to it.
	// Drawing it afterwards would let a synthesizer run on ore that had not
	// come up yet, and a colony with no crystal mine at all would never notice
	// it had run out.
	crystalRatio := float64(1)
	if want := crystalIn * sat * r.Staffing * dt; want > 0 {
		got := min(want, c.Crystal)
		c.Crystal -= got
		crystalRatio = got / want
		r.Crystal.Consumed += got / dt
	}

	// 5. Production. Everything below records what actually happened, divided
	// back out by dt into a per-second rate. Recording the achieved amount
	// rather than the intended one is the whole point: a greenhouse that ran
	// dry has to show as producing nothing, not as producing what it would
	// have.
	//
	// Ore and food need power and water both. The extractors need only power,
	// since what they draw on is the ice or the air rather than the tank.
	works := sat * waterRatio * r.Staffing
	mined, cut := ironOut*works, crystalOut*works
	drawn := waterOut * sat * r.Staffing
	grown := foodOut * works

	// Into the stores, and no further. What will not fit is lost, and saying
	// how much is what makes a full bar an instruction rather than a fact.
	c.Iron, r.Spilled.Iron = fill(c.Iron, mined*dt, r.Cap.Iron)
	c.Crystal, r.Spilled.Crystal = fill(c.Crystal, cut*dt, r.Cap.Crystal)
	c.Water, r.Spilled.Water = fill(c.Water, drawn*dt, r.Cap.Water)
	c.Food, r.Spilled.Food = fill(c.Food, grown*dt, r.Cap.Food)

	// Synthesis needs everything the colony has: the grid, the tank, the crew
	// and the crystal. It is the last thing to keep running and the first to
	// stop, which is what makes it the measure of a colony that has stopped
	// merely surviving.
	synth := vespiteOut * works * crystalRatio
	c.Vespite, r.Spilled.Vespite = fill(c.Vespite, synth*dt, r.Cap.Vespite)
	r.Spilled.Vespite /= dt
	r.Vespite.Produced = synth

	r.Spilled.Iron /= dt
	r.Spilled.Crystal /= dt
	r.Spilled.Water /= dt
	r.Spilled.Food /= dt

	// The rate recorded is what was made, not what was kept: a mine at a full
	// stockpile is still running, and the panel says so on the spill line
	// rather than by pretending the mine stopped.
	r.Iron.Produced = mined
	r.Crystal.Produced = cut
	r.Water.Produced = drawn
	r.Food.Produced = grown

	// 6. Habitats feed the people living in them. The draw scales with
	// occupancy so a habitat raised ahead of the colonists who will fill it
	// does not eat on their behalf — and because the colony-wide ratio makes
	// the total come out at exactly FoodPerColonist a head.
	//
	// It is not scaled by the grid: people eat in a blackout.
	occupancy := float64(0)
	if r.Housing > 0 {
		occupancy = min(c.Colonists/r.Housing, 1)
	}
	r.Fed, r.Watered = 1, 1
	if want := foodIn * occupancy * dt; want > 0 {
		got := min(want, c.Food)
		c.Food -= got
		r.Fed = got / want
		r.Food.Consumed = got / dt
	}
	if want := lifeWaterIn * occupancy * dt; want > 0 {
		got := min(want, c.Water)
		c.Water -= got
		r.Watered = got / want
		r.Water.Consumed += got / dt
	}

	// Colonists arrive to fill housing while they are looked after, and leave
	// when they are not. Thirst counts as much as hunger: before this a
	// colony could run its tank dry and lose nobody, because water reached
	// the colonists only as an input to the greenhouse that fed them.
	//
	// Leaving is proportional to the shortfall rather than a flat rate past a
	// threshold. A colony 2% short of food should lose someone eventually,
	// not at the same speed as one with an empty larder — and the old cliff
	// at 0.999 meant a rounding error could empty a colony as fast as a
	// famine.
	support := min(r.Fed, r.Watered)
	switch {
	case support < 1:
		c.Colonists -= StarveRate * (1 - support) * dt
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

// fill adds to a stock up to its ceiling and reports what would not fit.
//
// A ceiling of zero is treated as no ceiling rather than as a stock that can
// hold nothing, so a Readout built by hand in a test does not silently throw
// away everything it produces.
func fill(held, added, capacity float64) (now, spilled float64) {
	if added <= 0 {
		return held, 0
	}
	if capacity <= 0 {
		return held + added, 0
	}
	room := capacity - held
	if room <= 0 {
		return held, added
	}
	if added <= room {
		return held + added, 0
	}
	return capacity, added - room
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
	case NeedsCoast:
		return "ground on the coast"
	default:
		return "solid ground"
	}
}
