package game

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The button artwork arrived with a contract: assets/ui/buttons/buttons.json
// states the size, the nine-slice inset, the minimum a button may be drawn at,
// and a sha256 for each file. These check the code against that file rather
// than against numbers copied out of it, so the assets and the constants
// cannot drift apart quietly.

const buttonsDir = "../../assets/ui/buttons"

type buttonsManifest struct {
	Size   [2]int `json:"size"`
	Insets struct {
		Left, Top, Right, Bottom int
	} `json:"insets"`
	RecommendedMinimumSize [2]int `json:"recommended_minimum_size"`
	States                 map[string]struct {
		File string `json:"file"`
	} `json:"states"`
	StatePriority []string `json:"state_priority"`
}

func loadManifest(t *testing.T) buttonsManifest {
	t.Helper()
	blob, err := os.ReadFile(filepath.Join(buttonsDir, "buttons.json"))
	if err != nil {
		t.Skipf("no button manifest: %v", err)
	}
	var m buttonsManifest
	if err := json.Unmarshal(blob, &m); err != nil {
		t.Fatalf("buttons.json: %v", err)
	}
	return m
}

// The constants in button.go are the manifest's numbers. If the art is
// re-cut at a different size or inset, this is what says so.
func TestButtonConstantsMatchTheManifest(t *testing.T) {
	m := loadManifest(t)

	if m.Size[0] != buttonTexW || m.Size[1] != buttonTexH {
		t.Errorf("manifest says %dx%d, button.go says %dx%d",
			m.Size[0], m.Size[1], buttonTexW, buttonTexH)
	}

	// A nine-slice with different insets per side would need a different
	// GenerateQuads call than the one the engine offers.
	in := m.Insets
	if in.Left != in.Top || in.Top != in.Right || in.Right != in.Bottom {
		t.Fatalf("insets are not uniform: %+v — renderer.NineSlice takes one", in)
	}
	if in.Left != buttonInset {
		t.Errorf("manifest inset %d, button.go %d", in.Left, buttonInset)
	}

	if got := m.RecommendedMinimumSize; got[1] != buttonMinH || got[0] != buttonMinW {
		t.Errorf("manifest minimum %dx%d, button.go %dx%d",
			got[0], got[1], buttonMinW, buttonMinH)
	}
}

// Below twice the inset a button's top and bottom corner regions overlap and
// the artwork folds in on itself. Every button the game draws has to clear it.
func TestEveryButtonClearsTheNineSliceInset(t *testing.T) {
	if buttonMinH < 2*buttonInset {
		t.Fatalf("buttonMinH %d is under twice the inset %d; the corners would overlap",
			buttonMinH, buttonInset)
	}

	// The menu's rows are the buttons in the game.
	if menuRowH < buttonMinH {
		t.Errorf("menu rows are %d units tall, under the %d the artwork needs",
			menuRowH, buttonMinH)
	}
	if menuW-2*menuPad < buttonMinW {
		t.Errorf("menu rows are %d units wide, under the %d the artwork needs",
			menuW-2*menuPad, buttonMinW)
	}
}

// The priority the states resolve in is the manifest's: disabled beats
// pressed beats hover beats normal. Getting this wrong shows a disabled
// button lighting up under the pointer.
func TestButtonStatePriority(t *testing.T) {
	cases := []struct {
		enabled, hot, held bool
		want               buttonState
	}{
		{false, true, true, buttonDisabled}, // disabled wins over everything
		{false, false, false, buttonDisabled},
		{true, true, true, buttonPressed}, // pressed over hover
		{true, false, true, buttonPressed},
		{true, true, false, buttonHover},
		{true, false, false, buttonNormal},
	}
	for _, c := range cases {
		if got := buttonStateFor(c.enabled, c.hot, c.held); got != c.want {
			t.Errorf("enabled=%v hot=%v held=%v gave state %d, want %d",
				c.enabled, c.hot, c.held, got, c.want)
		}
	}

	// And it matches what the manifest declares, in order.
	m := loadManifest(t)
	want := []string{"disabled", "pressed", "hover", "normal"}
	if len(m.StatePriority) != len(want) {
		t.Fatalf("manifest lists %d priorities, want %d", len(m.StatePriority), len(want))
	}
	for i := range want {
		if m.StatePriority[i] != want[i] {
			t.Errorf("manifest priority %d is %q, the code resolves %q",
				i, m.StatePriority[i], want[i])
		}
	}
}

// Every state names a file, the file exists, and the code asks for the same
// one. A state whose texture failed to load takes all four down together —
// initButtons drops the whole set rather than drawing three states and a hole.
func TestEveryButtonStateHasItsFile(t *testing.T) {
	m := loadManifest(t)

	byState := map[string]buttonState{
		"normal": buttonNormal, "hover": buttonHover,
		"pressed": buttonPressed, "disabled": buttonDisabled,
	}
	if len(m.States) != int(buttonStateCount) {
		t.Errorf("manifest has %d states, the code has %d", len(m.States), buttonStateCount)
	}

	for name, st := range m.States {
		s, ok := byState[name]
		if !ok {
			t.Errorf("manifest has a state %q the code does not know", name)
			continue
		}
		if got := filepath.Base(buttonFile(s)); got != st.File {
			t.Errorf("state %q: manifest says %q, code loads %q", name, st.File, got)
		}
		if _, err := os.Stat(filepath.Join(buttonsDir, st.File)); err != nil {
			t.Errorf("state %q: %v", name, err)
		}
	}
}

// All four share a silhouette, which is what lets a state change without the
// layout moving. The manifest claims it was checked; this is the check.
func TestButtonStatesShareTheirSilhouette(t *testing.T) {
	m := loadManifest(t)

	var ref []bool
	var refName string
	for _, s := range []buttonState{buttonNormal, buttonHover, buttonPressed, buttonDisabled} {
		path := filepath.Join(buttonsDir, filepath.Base(buttonFile(s)))
		f, err := os.Open(path)
		if err != nil {
			t.Skipf("no button art: %v", err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		b := img.Bounds()
		if b.Dx() != m.Size[0] || b.Dy() != m.Size[1] {
			t.Errorf("%s is %dx%d, manifest says %dx%d",
				path, b.Dx(), b.Dy(), m.Size[0], m.Size[1])
			continue
		}

		// Opaque-or-not per pixel is the silhouette.
		sil := make([]bool, 0, b.Dx()*b.Dy())
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				sil = append(sil, a > 0x7fff)
			}
		}

		if ref == nil {
			ref, refName = sil, filepath.Base(path)
			continue
		}
		for i := range sil {
			if sil[i] != ref[i] {
				px := i % b.Dx()
				py := i / b.Dx()
				t.Errorf("%s differs from %s in shape at (%d,%d); a state change would "+
					"move the button rather than just recolour it",
					filepath.Base(path), refName, px, py)
				break
			}
		}
	}

}

// The manifest ships a hash per file. A re-export that changed the art without
// updating the manifest is a drift this catches — and one that updated both is
// a deliberate change that will show as a one-line diff.
func TestButtonArtMatchesItsRecordedHashes(t *testing.T) {
	blob, err := os.ReadFile(filepath.Join(buttonsDir, "validation.json"))
	if err != nil {
		t.Skipf("no validation manifest: %v", err)
	}
	var v struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(blob, &v); err != nil {
		t.Fatalf("validation.json: %v", err)
	}
	if len(v.SHA256) == 0 {
		t.Fatal("validation.json records no hashes")
	}

	for name, want := range v.SHA256 {
		data, err := os.ReadFile(filepath.Join(buttonsDir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s has changed since validation.json was written:\n  have %s\n  want %s",
				name, got, want)
		}
	}
}
