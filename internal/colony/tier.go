package colony

import (
	"errors"
	"fmt"

	"github.com/derekmwright/vesper3/internal/hex"
)

// Tiers are the answer to a map that will not let a colony sprawl.
//
// BuildRadius means tiles near a habitat are finite, and Jobs means every new
// structure wants people who want food and water. Between them, the cheapest
// way to produce more had become "build another one somewhere", and that runs
// out. Upgrading is the other direction: the same footprint, the same crew,
// more plant.
//
// So the rule is one sentence. A tier-3 building is two buildings' worth of
// machinery run by one building's worth of staff — every rate and every
// capacity scales, and Jobs does not.
//
// That makes the upgrade worth its cost without making it strictly better on
// every axis. A bigger mine draws proportionally more power and water, so
// tiering up does not quietly solve the grid or the tank; it buys back the
// one input a colony cannot simply build more of.
const MaxTier = 3

// TierScale is the multiplier a building's rates and stores run at.
//
// Tier 0 means "never set" — a building loaded from a save written before
// tiers existed — and reads as tier 1 rather than as nothing at all.
func TierScale(tier uint8) float64 {
	switch tier {
	case MaxTier:
		return 2.0
	case 2:
		return 1.5
	default:
		return 1.0
	}
}

// Tier is a building's tier, reading an unset one as the first.
func (b Building) Tier() uint8 {
	if b.TierLevel == 0 {
		return 1
	}
	return b.TierLevel
}

// UpgradeCost is what moving one building up one tier takes.
type UpgradeCost struct {
	Vespite float64
	Iron    float64
}

// CostToReach is the price of arriving at a tier from the one below it.
//
// The iron half is a multiple of what the building cost to put down, so the
// table generalises to every structure instead of naming nine of them, and a
// retune of any building's price carries into its upgrades. The Vespite half
// is flat: Vespite is the gate, and a gate that scales with the thing being
// gated is not a gate, it is a discount for starting small.
func CostToReach(k Kind, tier uint8) (UpgradeCost, bool) {
	base := Of(k).IronCost
	switch tier {
	case 2:
		return UpgradeCost{Vespite: 8, Iron: 1.5 * base}, true
	case MaxTier:
		return UpgradeCost{Vespite: 20, Iron: 3 * base}, true
	}
	return UpgradeCost{}, false
}

// Errors an upgrade can return.
var (
	ErrNothingThere = errors.New("nothing built there")
	ErrTopTier      = errors.New("already at the top tier")
)

// CanUpgrade reports whether Upgrade would succeed, and what it would cost.
func (c *Colony) CanUpgrade(a hex.Axial) (UpgradeCost, error) {
	b, ok := c.At(a)
	if !ok {
		return UpgradeCost{}, ErrNothingThere
	}
	next := b.Tier() + 1
	cost, ok := CostToReach(b.Kind, next)
	if !ok {
		return UpgradeCost{}, fmt.Errorf("%w: %s is tier %d",
			ErrTopTier, Of(b.Kind).Name, b.Tier())
	}
	if c.Vespite < cost.Vespite {
		return cost, fmt.Errorf("%w: tier %d needs %.0f vespite, have %.0f",
			ErrTooPoor, next, cost.Vespite, c.Vespite)
	}
	if c.Iron < cost.Iron {
		return cost, fmt.Errorf("%w: tier %d needs %.0f iron, have %.0f",
			ErrTooPoor, next, cost.Iron, c.Iron)
	}
	return cost, nil
}

// Upgrade moves a standing building up one tier, charging for it.
//
// The building keeps its identity: same tile, same facing, same ore, same slot
// in Buildings. Nothing is demolished and rebuilt, because a rebuild would
// refund, re-site and re-roll the ore under a mine, and an upgrade is none of
// those things.
func (c *Colony) Upgrade(a hex.Axial) error {
	cost, err := c.CanUpgrade(a)
	if err != nil {
		return err
	}
	i, ok := c.index[a]
	if !ok {
		return ErrNothingThere
	}
	c.Vespite -= cost.Vespite
	c.Iron -= cost.Iron
	c.Buildings[i].TierLevel = c.Buildings[i].Tier() + 1
	return nil
}

// SetTier forces a building to a tier without charging for it.
//
// For the demo colony and for tests: the same reason Found exists beside
// Place. Nothing in play calls it, because in play a tier is something the
// colony paid vespite for.
func (c *Colony) SetTier(a hex.Axial, tier uint8) {
	i, ok := c.index[a]
	if !ok || tier < 1 || tier > MaxTier {
		return
	}
	c.Buildings[i].TierLevel = tier
}

// scaled returns the spec a building of this tier actually runs at.
//
// Every rate and every capacity is multiplied. Jobs is not, which is the
// whole mechanic. Costs, colour and siting rules are not either: what a
// structure needs from the ground does not change because it got bigger, and
// the build price is what CostToReach is derived from.
func (s Spec) scaled(f float64) Spec {
	if f == 1 {
		return s
	}
	s.PowerOut *= f
	s.PowerIn *= f
	s.MineOut *= f
	s.WaterOut *= f
	s.WaterIn *= f
	s.FoodOut *= f
	s.FoodIn *= f
	s.VespiteOut *= f
	s.CrystalIn *= f
	s.Housing *= f

	s.PowerStore *= f
	s.WaterStore *= f
	s.FoodStore *= f
	s.IronStore *= f
	s.CrystalStore *= f
	s.VespiteStore *= f
	return s
}
