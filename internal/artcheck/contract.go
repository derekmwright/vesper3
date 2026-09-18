package artcheck

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/derekmwright/vesper3/internal/colony"
)

// The markers: the material names a structure uses to say "this primitive is
// driven by the simulation".
//
// A marker is a name now. It used to be a base colour — (0.015, 0.55, 0.85)
// meant the battery's second charge strip — matched with a tolerance of 0.002,
// because a colour makes a float32 round trip through the glTF exporter and an
// exact comparison would fail on a file that is correct.
//
// That tolerance was the whole problem. It was invisible in the art: nothing in
// a modelling tool says 0.55 is load-bearing, it looks like a colour someone
// picked. It was not greppable. It failed silently and late — a model
// re-exported with its indicator merged into the body, or a thousandth off,
// loaded without complaint and never lit up. And every driven part in the game
// needed a globally distinct colour, because the RGB cube was the only
// namespace there was.
//
// renderer.ModelMesh keeps the material name as of glyphengine#21, so the
// handle is a string. Charge_Runtime_2 turns up in the model, in the exporter
// and in the game, means one thing, and cannot be reached by accident.
const (
	// LampName is the warm accent every structure lights at dusk.
	LampName = "Amber runtime lamp"

	// CondenserPulseName is the fin band that sweeps while a condenser draws.
	CondenserPulseName = "CondenserPulse_Runtime"

	// GrowLightName is the greenhouse's lamp bank.
	GrowLightName = "GrowLight_Runtime"

	// chargePrefix names the battery's strips and its status lamp:
	// Charge_Runtime_1 through _4 fill from the bottom, and _5_Status is the
	// pilot lamp on the crown.
	chargePrefix = "Charge_Runtime_"
)

// The battery's driven parts, in marker order: four fill strips from the
// bottom up, then the pilot lamp above them.
//
// The two are read differently and that is the point of separating them. The
// strips are a level — how much is stored. The lamp is a state — charging,
// draining, full, or flat — which a level cannot show: a bank sitting at 60%
// looks identical whether it is filling or emptying, and which of those is
// happening is the thing a player needs at dusk.
const (
	BatteryStrips = 4
	BatteryStatus = 5
)

// Marker reports which simulation-driven part a material name identifies, or 0
// for a material that is just paint.
//
// The battery returns 1 to 4 for its fill strips, bottom first, and
// BatteryStatus for the pilot lamp. The greenhouse returns 1: it has one lamp
// bank.
func Marker(kind colony.Kind, name string) int {
	switch kind {
	case colony.Greenhouse:
		if name == GrowLightName {
			return 1
		}
	case colony.Battery:
		rest, ok := strings.CutPrefix(name, chargePrefix)
		if !ok {
			return 0
		}
		// "5_Status" carries a suffix the strips do not, so the number is
		// whatever leads.
		if i := strings.IndexByte(rest, '_'); i >= 0 {
			rest = rest[:i]
		}
		n, err := strconv.Atoi(rest)
		if err != nil || n < 1 || n > BatteryStatus {
			return 0
		}
		return n
	}
	return 0
}

// IsCondenserPulse reports the fin band that sweeps while a condenser runs.
func IsCondenserPulse(name string) bool { return name == CondenserPulseName }

// IsLamp reports the part a structure's dusk lamp is driven from.
//
// This replaced a heuristic that picked whichever material was warmest —
// reddest relative to blue, weighted by brightness. It worked, and degraded in
// the worst possible way: a model with nothing warm in it simply never lit,
// leaving an unexplained dark patch in a colony at night. A name either matches
// or it does not.
func IsLamp(name string) bool { return name == LampName }

// Verify checks a model file against everything the game expects to find in
// it, and returns one error per breach.
//
// This is the whole defence for a contract that is otherwise invisible. The
// game matches these names at load and silently does nothing when they are
// absent — so a re-export that merges the battery's strips into its body
// produces a bank that never lights, months later, in a build nobody connects
// to the export.
func Verify(path string, kind colony.Kind) []error {
	mats, err := Materials(path)
	if err != nil {
		return []error{err}
	}

	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if len(mats) < 2 {
		fail("%s exported as %d material(s): its markers have been merged into the body",
			path, len(mats))
		return problems
	}

	switch kind {
	case colony.Battery:
		seen := map[int]string{}
		for _, m := range mats {
			n := Marker(kind, m.Name)
			if n == 0 {
				continue
			}
			if prev, taken := seen[n]; taken {
				fail("%s: %q and %q both read as charge marker %d", path, prev, m.Name, n)
			}
			seen[n] = m.Name
		}
		for n := 1; n <= BatteryStrips; n++ {
			if seen[n] == "" {
				fail("%s: no %s%d, so the bank cannot show a partial charge",
					path, chargePrefix, n)
			}
		}
		if seen[BatteryStatus] == "" {
			fail("%s: no %s%d_Status, so the bank cannot show whether it is filling or draining",
				path, chargePrefix, BatteryStatus)
		}

	case colony.Condenser:
		if !anyMaterial(mats, func(m Material) bool { return IsCondenserPulse(m.Name) }) {
			fail("%s: no %q, so the condenser will not show that it is running",
				path, CondenserPulseName)
		}

	case colony.Greenhouse:
		if !anyMaterial(mats, func(m Material) bool { return Marker(kind, m.Name) != 0 }) {
			fail("%s: no %q", path, GrowLightName)
		}
	}

	// The battery is the one structure with no dusk lamp, and deliberately:
	// its charge strips are its night-time presence, and an amber lamp beside
	// them would compete with the colour carrying the reading.
	lamp := anyMaterial(mats, func(m Material) bool { return IsLamp(m.Name) })
	switch {
	case kind == colony.Battery && lamp:
		fail("%s: has a %q, which will be lit at dusk and fight the charge strips",
			path, LampName)
	case kind != colony.Battery && !lamp:
		fail("%s: no %q, so it will stay dark after sunset", path, LampName)
	}

	return problems
}

func anyMaterial(mats []Material, pred func(Material) bool) bool {
	for _, m := range mats {
		if pred(m) {
			return true
		}
	}
	return false
}
