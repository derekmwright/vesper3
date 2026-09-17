package game

import (
	"image"
	_ "image/png"
	"os"
	"strings"
	"testing"
)

// The HUD computes icon UVs from atlasCols and atlasRows alone. If the atlas
// is ever regenerated with a different grid — cmd/iconatlas takes -cols on the
// command line — every icon silently samples the wrong cell, and the symptom
// is a hotbar showing the right names against the wrong pictures. Nothing else
// would catch that.
func TestIconAtlasMatchesTheGridTheHUDAssumes(t *testing.T) {
	f, err := os.Open("../../" + atlasPath)
	if err != nil {
		t.Skipf("no atlas built: %v", err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode %s: %v", atlasPath, err)
	}

	if cfg.Width%atlasCols != 0 || cfg.Height%atlasRows != 0 {
		t.Fatalf("atlas is %dx%d, which does not divide into a %dx%d grid",
			cfg.Width, cfg.Height, atlasCols, atlasRows)
	}

	cellW := cfg.Width / atlasCols
	cellH := cfg.Height / atlasRows
	if cellW != cellH {
		t.Errorf("atlas cells are %dx%d, not square; icons will be stretched", cellW, cellH)
	}

	// Every structure and every resource needs a cell. The grid has to hold
	// all of them: a structure added without a row in the atlas would sample
	// whatever cell the resources moved into, which reads as the wrong icon
	// rather than as an error.
	if iconCount > atlasCols*atlasRows {
		t.Errorf("%d icons needed but only %d atlas cells", iconCount, atlasCols*atlasRows)
	}

	// And the art has to actually be there. A transparent cell draws nothing,
	// which on a resource row looks like a layout bug rather than missing art.
	if _, err := os.Stat("../../assets/icons-src"); err == nil {
		if n := countAtlasSources(t); n < iconCount {
			t.Errorf("%d numbered source icons for %d cells; run cmd/iconatlas after adding art", n, iconCount)
		}
	}
}

// The font is bundled under the OFL, which requires the licence to travel with
// it. Shipping the TTF without OFL.txt would be a licensing bug, and it is the
// kind that is only ever noticed by someone else.
func TestBundledFontShipsItsLicence(t *testing.T) {
	if _, err := os.Stat("../../" + fontPath); err != nil {
		t.Skipf("no bundled font: %v", err)
	}
	licence, err := os.ReadFile("../../assets/fonts/OFL.txt")
	if err != nil {
		t.Fatalf("font is bundled without its licence: %v", err)
	}
	if !strings.Contains(string(licence), "SIL OPEN FONT LICENSE") {
		t.Error("OFL.txt does not look like the Open Font Licence")
	}
}

// A fixed-width advance, which is what the HUD's font actually is.
func mono(w float32) func(rune) float32 {
	return func(rune) float32 { return w }
}

// Alert text and placement errors are written for clarity, not to a character
// budget, and the longest of them ran off the panel and across the map. This
// is the guard on that.
func TestFitTruncatesToTheWidthGiven(t *testing.T) {
	const cw = 10

	cases := []struct {
		name string
		in   string
		maxW float32
		want string
	}{
		{"fits exactly", "abcde", 50, "abcde"},
		{"fits with room", "abc", 100, "abc"},
		{"empty", "", 50, ""},
		{"one over", "abcdef", 50, "ab..."},
		{"far too long", strings.Repeat("x", 100), 50, "xx..."},
		{"no room for anything", "abcdef", 30, "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fitText(tc.in, tc.maxW, mono(cw))
			if got != tc.want {
				t.Errorf("fitText(%q, %.0f) = %q, want %q", tc.in, tc.maxW, got, tc.want)
			}
		})
	}
}

// Whatever it returns has to actually fit, which is the property the panel
// depends on and the one a hand-written table can get wrong.
func TestFitOutputAlwaysFits(t *testing.T) {
	const cw = 7
	advance := mono(cw)

	inputs := []string{
		"",
		"a",
		"Water runs out in 4:12 - using 0.55/s, making 0.45/s",
		"BLACKOUT - 28% power, everything is running slow",
		"build 3 more Solar Array(s), or a Geothermal Plant to cover the night too",
		strings.Repeat("long ", 40),
	}

	for _, in := range inputs {
		for _, maxW := range []float32{0, 5, 21, 50, 140, 320, 10000} {
			got := fitText(in, maxW, advance)

			var w float32
			for _, r := range got {
				w += advance(r)
			}
			// A string that could not be cut any further is allowed to be the
			// bare ellipsis even when that overflows; there is nothing shorter
			// to return.
			if w > maxW && got != "..." {
				t.Errorf("fitText(%q, %.0f) = %q, which is %.0f wide", in, maxW, got, w)
			}
			if len([]rune(got)) > len([]rune(in)) && !strings.HasSuffix(got, "...") {
				t.Errorf("fitText(%q) grew to %q", in, got)
			}
		}
	}
}

// Proportional fonts are the case the fixed-width tests above cannot catch: a
// run of wide glyphs has to cut sooner than a run of narrow ones.
func TestFitRespectsPerGlyphWidths(t *testing.T) {
	advance := func(r rune) float32 {
		if r == 'W' {
			return 20
		}
		return 5
	}

	narrow := fitText("iiiiiiiiii", 50, advance)
	wide := fitText("WWWWWWWWWW", 50, advance)

	if len(narrow) <= len(wide) {
		t.Errorf("narrow %q did not outlast wide %q at the same width", narrow, wide)
	}
}

// Every string the panel shows verbatim has to fit the space it is given, at
// the scale it is drawn at. This checks the copy rather than the mechanism:
// clipping is a safety net, and an advisory that is always ellipsised is a
// bad advisory even though it no longer overflows.
func TestAlertCopyFitsWithoutClipping(t *testing.T) {
	// Go Mono advances 0.6 em. The two alert lines are drawn at different
	// scales, so they get different budgets.
	const advanceEm = 0.6
	// Assigned to variables first: as constant expressions these truncations
	// would be compile errors rather than floors.
	avail := float64(alertTextW)
	headBudget := int(avail / (float64(textMain) * advanceEm))
	fixBudget := int(avail / (float64(textSub) * advanceEm))

	// The longest strings colony.Alerts can produce, at their worst-case
	// substitutions. If one is added there and not here, the panel is where it
	// shows up — as an ellipsis in the middle of the advice.
	headlines := []string{
		"BLACKOUT - 100% power, output reduced",
		"Food out in 12:34 (make 0.16 use 0.55)",
		"Housing full - 128 in 128 beds",
		"No housing - nobody lives here",
		"Mines are stopped",
	}
	fixes := []string{
		"build 12 more Solar Array(s), or a Geothermal",
		"dark: build 12 Geothermal, or wait for dawn",
		"build an Ice Extractor, or a Condenser anywhere",
		"build a Greenhouse - lichen yields 1.5x",
		"build a Habitat to keep growing",
		"no power at all - build a generator",
	}

	for _, s := range headlines {
		if len(s) > headBudget {
			t.Errorf("headline is %d chars, %d fit:\n  %s", len(s), headBudget, s)
		}
	}
	for _, s := range fixes {
		if len(s) > fixBudget {
			t.Errorf("fix is %d chars, %d fit:\n  %s", len(s), fixBudget, s)
		}
	}
}

// countAtlasSources counts the numbered PNGs cmd/iconatlas packs.
func countAtlasSources(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("../../assets/icons-src")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
			n++
		}
	}
	return n
}
