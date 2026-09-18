package colony

import (
	"math"
	"testing"

	"github.com/derekmwright/vesper3/internal/world"
)

// What happens to colonists when the colony stops looking after them.
//
// Thirst used to do nothing. A colony could run its tank dry and lose nobody,
// because water reached the colonists only as an input to the greenhouse that
// fed them — so the punishment for losing water was a slow starvation two
// steps downstream, and a colony with a full larder and an empty tank was
// fine indefinitely.

// housed returns a colony with one full habitat, a grid, and whatever stores
// the caller wants to take away.
func housed(t *testing.T) (*Colony, *world.Map) {
	t.Helper()
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Habitat, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))
	c.Colonists = Of(Habitat).Housing
	return c, m
}

// The headline: an empty tank costs colonists, on its own.
func TestColonistsLeaveWithoutWater(t *testing.T) {
	c, _ := housed(t)
	c.Water = 0
	c.Food = 1000 // a full larder, so only thirst is in play

	before := c.Colonists
	for range 20 {
		c.Tick(0.1, 1)
	}

	if c.Colonists >= before {
		t.Errorf("colonists went from %.2f to %.2f with an empty tank", before, c.Colonists)
	}
	if c.Readout.Watered >= 0.999 {
		t.Errorf("Watered is %.3f with no water", c.Readout.Watered)
	}
	if c.Readout.Fed < 0.999 {
		t.Errorf("Fed is %.3f with a full larder; thirst should not read as hunger", c.Readout.Fed)
	}
}

// And an empty larder does, as it always did.
func TestColonistsLeaveWithoutFood(t *testing.T) {
	c, _ := housed(t)
	c.Food = 0

	before := c.Colonists
	for range 20 {
		c.Tick(0.1, 1)
	}

	if c.Colonists >= before {
		t.Errorf("colonists went from %.2f to %.2f with an empty larder", before, c.Colonists)
	}
	if c.Readout.Fed >= 0.999 {
		t.Errorf("Fed is %.3f with no food", c.Readout.Fed)
	}
}

// Leaving is proportional to how short the colony is, not a cliff. A colony
// two percent short should lose someone eventually; one with nothing at all
// should lose them fast. The old rule ran both at the same speed.
func TestColonistsLeaveInProportionToTheShortfall(t *testing.T) {
	loss := func(food float64) float64 {
		c, _ := housed(t)
		// Enough for a fraction of one tick's demand.
		c.Food = food
		before := c.Colonists
		c.Tick(1, 1)
		return before - c.Colonists
	}

	want := Of(Habitat).FoodIn // one second of full demand

	none := loss(0)
	half := loss(want / 2)

	if none <= 0 {
		t.Fatal("an empty larder cost nothing")
	}
	if half <= 0 {
		t.Fatal("a half-empty larder cost nothing")
	}
	if half >= none {
		t.Errorf("half fed lost %.4f, empty lost %.4f; half should be gentler", half, none)
	}
	if math.Abs(half-none/2) > 1e-9 {
		t.Errorf("half fed lost %.4f, want half of %.4f", half, none)
	}
}

// A colony that is looked after grows instead.
func TestAProvisionedColonyGrows(t *testing.T) {
	c, _ := housed(t)
	c.Colonists = 1 // room to grow into

	before := c.Colonists
	c.Tick(1, 1)

	if c.Colonists <= before {
		t.Errorf("a fed and watered colony went from %.2f to %.2f", before, c.Colonists)
	}
	if c.Readout.Fed < 0.999 || c.Readout.Watered < 0.999 {
		t.Errorf("fed %.3f watered %.3f in a supplied colony", c.Readout.Fed, c.Readout.Watered)
	}
}

// Life support is not on the grid. People drink in a blackout for the same
// reason they eat in one, and the old code had it both ways: food was exempt
// and the habitat's water was not.
func TestLifeSupportIsNotScaledByPower(t *testing.T) {
	c, _ := housed(t)
	c.Tick(1, 0) // night, no storage: the grid is dead

	if c.Readout.Satisfaction != 0 {
		t.Fatalf("fixture is not blacked out: satisfaction %.3f", c.Readout.Satisfaction)
	}

	want := Of(Habitat).WaterIn
	if math.Abs(c.Readout.Water.Consumed-want) > 1e-9 {
		t.Errorf("habitat drew %.3f water/s in a blackout, want its full %.3f",
			c.Readout.Water.Consumed, want)
	}
	if want := Of(Habitat).FoodIn; math.Abs(c.Readout.Food.Consumed-want) > 1e-9 {
		t.Errorf("habitat ate %.3f/s in a blackout, want %.3f", c.Readout.Food.Consumed, want)
	}
}

// An industrial draw still is on the grid, which is the distinction that makes
// the exemption above mean something.
func TestIndustrialWaterIsStillScaledByPower(t *testing.T) {
	c, m := stocked(t, world.Regolith)
	mustPlace(t, c, m, Greenhouse, world.FromOffset(4, 4))
	mustPlace(t, c, m, SolarArray, world.FromOffset(5, 4))

	before := c.Water
	c.Tick(1, 0) // night: nothing runs

	if c.Water != before {
		t.Errorf("a greenhouse drank %.3f during a blackout", before-c.Water)
	}
}

// Thirst and hunger are reported separately, because the fix for one is not
// the fix for the other and an advisory that says only "colonists leaving"
// sends the player to check every row on the panel.
func TestTheReadoutSaysWhichKindOfShortfall(t *testing.T) {
	thirsty, _ := housed(t)
	thirsty.Water, thirsty.Food = 0, 1000
	thirsty.Tick(1, 1)

	hungry, _ := housed(t)
	hungry.Food = 0
	hungry.Tick(1, 1)

	if thirsty.Readout.Watered >= 0.999 || thirsty.Readout.Fed < 0.999 {
		t.Errorf("thirsty colony reads fed %.2f watered %.2f",
			thirsty.Readout.Fed, thirsty.Readout.Watered)
	}
	if hungry.Readout.Fed >= 0.999 {
		t.Errorf("hungry colony reads fed %.2f", hungry.Readout.Fed)
	}

	// And the advisory names the cause rather than the symptom.
	var text string
	for _, a := range thirsty.Alerts() {
		if a.Level == LevelCritical {
			text = a.Text
			break
		}
	}
	if text == "" {
		t.Fatal("a colony losing people raised no critical alert")
	}
	if !contains(text, "water") {
		t.Errorf("a thirsty colony's alert says %q, which does not mention water", text)
	}
}

// An empty colony has nobody to lose, and must not report people leaving.
func TestAnEmptyColonyIsNotStarving(t *testing.T) {
	c, _ := housed(t)
	c.Colonists = 0
	c.Water, c.Food = 0, 0
	c.Tick(1, 1)

	if c.Colonists < 0 {
		t.Errorf("colonists went negative: %.3f", c.Colonists)
	}
	for _, a := range c.Alerts() {
		if a.Level == LevelCritical && contains(a.Text, "leaving") {
			t.Errorf("an empty colony reported %q", a.Text)
		}
	}
}
