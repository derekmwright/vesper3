package artcheck

import (
	"fmt"
	"math"

	"github.com/derekmwright/vesper3/internal/colony"
)

// The markers, and the rule for reading them.
//
// A marker is a base colour reserved to mean "this primitive is driven by the
// simulation". They are dark, saturated and nothing like a paint colour, so a
// modeller cannot reach one by accident: the battery's strips are all
// (0.015, g, 0.85), the greenhouse's lamps (0.015, 0.8, 0.25), the condenser's
// fin band (0.015, 0.65, 0.35).
//
// The tolerance is what a glTF round trip costs. Colours go out as float32 and
// come back through the exporter's own conversion, so an exact comparison
// would fail on a file that is correct.
const markerTolerance = 0.002

func near(a, b float32) bool { return math.Abs(float64(a-b)) < markerTolerance }

// Marker reports which simulation-driven part a colour names, or 0 for a
// colour that is just paint.
//
// The battery returns 1 to 4 for its fill strips, bottom first, so a bank at
// half charge can light the bottom two — and BatteryStatus for the pilot lamp
// on the crown, which shows what the bank is doing rather than how full it is.
// The greenhouse returns 1: it has one lamp bank.
func Marker(kind colony.Kind, c [3]float32) int {
	switch kind {
	case colony.Greenhouse:
		if near(c[0], .015) && near(c[1], .8) && near(c[2], .25) {
			return 1
		}
	case colony.Battery:
		if near(c[0], .015) && near(c[2], .85) {
			for i := range BatteryStatus {
				if near(c[1], .45+.1*float32(i)) {
					return i + 1
				}
			}
		}
	}
	return 0
}

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

// IsCondenserPulse reports the fin band that sweeps while a condenser runs.
func IsCondenserPulse(c [3]float32) bool {
	return near(c[0], .015) && near(c[1], .65) && near(c[2], .35)
}

// Warmest returns the index of the colour a dusk lamp should be driven from:
// the warmest one, scored as how much more red than blue it is, weighted by
// brightness so a dark brown does not beat a bright amber.
//
// A heuristic rather than a reserved colour, because "the warm accent" is a
// thing every model has anyway and reserving a colour for it would mean
// retouching all eight. The cost is that it degrades quietly — a model with
// nothing warm in it simply never lights — which is why it is asserted too.
func Warmest(colors [][3]float32) (int, bool) {
	best, bestScore := -1, float32(0)
	for i, c := range colors {
		if warmth := (c[0] - c[2]) * c[0]; warmth > bestScore {
			best, bestScore = i, warmth
		}
	}
	if best < 0 || bestScore < 0.02 {
		return 0, false
	}
	return best, true
}

// Verify checks a model file against everything the game expects to find in
// it, and returns one error per breach.
//
// This is the whole defence for a contract that is otherwise invisible. The
// game matches these colours at load and silently does nothing when they are
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

	colors := make([][3]float32, len(mats))
	for i, m := range mats {
		colors[i] = m.BaseColor
	}

	switch kind {
	case colony.Battery:
		seen := map[int]string{}
		for _, m := range mats {
			n := Marker(kind, m.BaseColor)
			if n == 0 {
				continue
			}
			if prev, taken := seen[n]; taken {
				fail("%s: %q and %q both read as charge strip %d", path, prev, m.Name, n)
			}
			seen[n] = m.Name
		}
		for n := 1; n <= BatteryStrips; n++ {
			if seen[n] == "" {
				fail("%s: nothing reads as charge strip %d of %d, so the bank cannot show a partial charge",
					path, n, BatteryStrips)
			}
		}
		if seen[BatteryStatus] == "" {
			fail("%s: nothing reads as the status lamp, so the bank cannot show whether it is filling or draining",
				path)
		}

	case colony.Condenser:
		found := false
		for _, m := range mats {
			found = found || IsCondenserPulse(m.BaseColor)
		}
		if !found {
			fail("%s: nothing reads as the fin band, so the condenser will not show that it is running", path)
		}

	case colony.Greenhouse:
		found := false
		for _, m := range mats {
			found = found || Marker(kind, m.BaseColor) != 0
		}
		if !found {
			fail("%s: nothing reads as a grow lamp", path)
		}
	}

	// The battery is the one structure with no warm accent: its charge strips
	// are its night-time presence, and an amber lamp beside them would compete
	// with the colour carrying the reading.
	_, warm := Warmest(colors)
	switch {
	case kind == colony.Battery && warm:
		fail("%s: has a warm accent, which will be lit at dusk and fight the charge strips", path)
	case kind != colony.Battery && !warm:
		fail("%s: nothing warm to light at dusk, so it will stay dark after sunset", path)
	}

	return problems
}
