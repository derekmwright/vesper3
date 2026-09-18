package game

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/derekmwright/vesper3/internal/artcheck"
	"github.com/derekmwright/vesper3/internal/colony"
)

// The art contract.
//
// Three structures are animated by material: the game finds the battery's four
// charge strips, the condenser's fin band and the greenhouse's grow lamps by
// matching the base colour their primitive arrived with, and drives that
// primitive's emission from the simulation. There is no other handle —
// renderer.ModelMesh does not carry the material name.
//
// So the export is load-bearing, and nothing about it fails loudly. A model
// re-exported with its indicator merged into the body, or with a colour a
// thousandth off, loads without complaint and simply stops lighting up. These
// tests are what makes that a build failure instead.
//
// They assert through the game's own matchers rather than restating the
// colours, so retuning a marker retunes the test with it.

// modelFor returns the .glb for a structure, or "" if it has none — a
// structure with no model falls back to procedural geometry, which is not what
// these tests are about.
//
// The path comes from modelPath, the same function the loader uses, so a test
// cannot pass by checking a file the game never opens.
func modelFor(t *testing.T, k colony.Kind) string {
	t.Helper()
	for i, b := range colony.Buildable {
		if b != k {
			continue
		}
		path := filepath.Join("../..", modelPath(i, k))
		if _, err := os.Stat(path); err != nil {
			return ""
		}
		return path
	}
	return ""
}

// partsOf reads a model's materials as the meshPart list the game would build
// from it, so the matchers below are fed exactly what they see at runtime.
func partsOf(t *testing.T, k colony.Kind) ([]meshPart, bool) {
	t.Helper()
	path := modelFor(t, k)
	if path == "" {
		return nil, false
	}
	mats, err := artcheck.Materials(path)
	if err != nil {
		t.Fatalf("%v: %v", k, err)
	}
	parts := make([]meshPart, 0, len(mats))
	for _, m := range mats {
		parts = append(parts, meshPart{Name: m.Name, Color: m.BaseColor})
	}
	return parts, true
}

// The battery bank shows its charge on four strips that fill from the bottom.
// All four have to be separate materials the game can tell apart, or a bank at
// half charge either lights everything or nothing.
func TestBatteryModelCarriesItsFourChargeStrips(t *testing.T) {
	parts, ok := partsOf(t, colony.Battery)
	if !ok {
		t.Skip("no battery model built")
	}

	seen := map[int]bool{}
	for _, p := range parts {
		if n := activityMarker(colony.Battery, p.Name); n != 0 {
			if seen[n] {
				t.Errorf("two materials both read as charge strip %d", n)
			}
			seen[n] = true
		}
	}

	for n := 1; n <= artcheck.BatteryStrips; n++ {
		if !seen[n] {
			t.Errorf("no material in the battery model reads as charge strip %d; "+
				"the bank cannot show a partial charge without it", n)
		}
	}

	// The pilot lamp on the crown, which is a state rather than a level.
	if !seen[artcheck.BatteryStatus] {
		t.Error("no material reads as the status lamp; the bank cannot show " +
			"whether it is filling or draining")
	}
}

// The condenser's fin band sweeps while it is drawing water.
func TestCondenserModelCarriesItsPulseBand(t *testing.T) {
	parts, ok := partsOf(t, colony.Condenser)
	if !ok {
		t.Skip("no condenser model built")
	}
	for _, p := range parts {
		if isCondenserPulse(p.Name) {
			return
		}
	}
	t.Error("no material in the condenser model reads as the pulse band")
}

// The greenhouse's grow lamps come on with the power.
func TestGreenhouseModelCarriesItsGrowLamps(t *testing.T) {
	parts, ok := partsOf(t, colony.Greenhouse)
	if !ok {
		t.Skip("no greenhouse model built")
	}
	for _, p := range parts {
		if activityMarker(colony.Greenhouse, p.Name) != 0 {
			return
		}
	}
	t.Error("no material in the greenhouse model reads as a grow lamp")
}

// Every structure needs the lamp material findLampPart looks for. It used to
// be a heuristic over colours, which degraded quietly: a model with nothing
// warm in it simply never lit, and the colony got a dark patch nobody could
// explain. It is a name now, so this asserts the name is there.
func TestEveryModelHasSomethingToLightAtDusk(t *testing.T) {
	for _, k := range colony.Buildable {
		parts, ok := partsOf(t, k)
		if !ok {
			continue
		}

		// The battery is the exception, and deliberately: its charge strips
		// are its night-time presence, and an amber lamp beside them would
		// compete with the colour that carries the reading.
		if k == colony.Battery {
			if _, _, found := findLampPart(parts); found {
				t.Errorf("%v has a warm lamp part; it is meant to show charge instead", k)
			}
			continue
		}

		if _, _, found := findLampPart(parts); !found {
			t.Errorf("%v has no warm material, so it will stay dark after sunset", k)
		}
	}
}

// A structure arriving as one primitive means the exporter merged its
// materials, which is the failure that takes every marker above with it.
func TestModelsKeepTheirMaterialsSeparate(t *testing.T) {
	for _, k := range colony.Buildable {
		parts, ok := partsOf(t, k)
		if !ok {
			continue
		}
		if len(parts) < 2 {
			t.Errorf("%v exported as %d material(s); its markers have been merged into the body",
				k, len(parts))
			continue
		}

		// Nothing may be nameless: an unnamed material is one the game cannot
		// identify at all, whatever it looks like.
		for i, p := range parts {
			if p.Name == "" {
				t.Errorf("%v primitive %d has no material name", k, i)
			}
		}
	}
}

// tierModelFor is modelFor for an upgrade variant, returning "" when that tier
// has no art of its own — which is the normal case for everything but the mine
// and is not a failure.
func tierModelFor(t *testing.T, k colony.Kind, tier uint8) string {
	t.Helper()
	for i, b := range colony.Buildable {
		if b != k {
			continue
		}
		path := filepath.Join("../..", tierModelPath(i, k, tier))
		if _, err := os.Stat(path); err != nil {
			return ""
		}
		return path
	}
	return ""
}

// A tier model is loaded by the same path the game builds, lit by the same
// lamp matcher, and drawn by the same code. So it has to carry the same
// contract the tier-1 model does: something to light at dusk, and its
// materials kept apart.
//
// This is the check that would have caught the tier art being re-exported with
// its lamp merged into the body — the failure mode that cost this project its
// models once already, which is why every art contract here is a test.
func TestTierModelsKeepTheContractTheirFirstTierHas(t *testing.T) {
	found := 0
	for _, k := range colony.Buildable {
		for tier := uint8(2); tier <= colony.MaxTier; tier++ {
			path := tierModelFor(t, k, tier)
			if path == "" {
				continue
			}
			found++

			mats, err := artcheck.Materials(path)
			if err != nil {
				t.Errorf("%v tier %d: %v", k, tier, err)
				continue
			}
			if len(mats) < 2 {
				t.Errorf("%v tier %d has %d material(s): a merged export lights nothing",
					k, tier, len(mats))
			}

			parts := make([]meshPart, 0, len(mats))
			for _, m := range mats {
				parts = append(parts, meshPart{Name: m.Name, Color: m.BaseColor})
			}
			if _, _, ok := findLampPart(parts); !ok {
				t.Errorf("%v tier %d has nothing that lights at dusk", k, tier)
			}

			seen := map[string]bool{}
			for _, m := range mats {
				if seen[m.Name] {
					t.Errorf("%v tier %d repeats the material %q", k, tier, m.Name)
				}
				seen[m.Name] = true
			}
		}
	}
	if found == 0 {
		t.Skip("no tier art on disk yet")
	}
}

// The mine is the building tiers were implemented against, so its art is the
// one case this project can assert is actually present rather than skipping.
func TestTheMineHasArtForEveryTier(t *testing.T) {
	for tier := uint8(2); tier <= colony.MaxTier; tier++ {
		if tierModelFor(t, colony.Mine, tier) == "" {
			t.Errorf("no mine model for tier %d", tier)
		}
	}
}
