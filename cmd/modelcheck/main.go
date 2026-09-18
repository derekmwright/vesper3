// Command modelcheck verifies a structure model carries the materials the
// game drives from the simulation.
//
//	go run ./cmd/modelcheck assets/models/8-battery.glb
//	go run ./cmd/modelcheck assets/models/*.glb
//
// The structure a file describes is taken from its name, which is the same
// "N-slug.glb" the loader resolves.
//
// It exists because the art is a contract nothing else enforces. The game
// finds a battery's charge strips, a condenser's fin band and a greenhouse's
// grow lamps by matching the base colour their primitive arrived with, and a
// model exported without them loads perfectly and simply never lights up. This
// is what turns that into a message at build time.
//
// tools/models/build.sh runs it on every model it writes, before that model is
// allowed to replace the one already there.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/derekmwright/vesper3/internal/artcheck"
	"github.com/derekmwright/vesper3/internal/colony"
)

func main() {
	paths := os.Args[1:]
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: modelcheck <model.glb>...")
		os.Exit(2)
	}

	bad := 0
	for _, path := range paths {
		kind, ok := kindFor(path)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s: cannot tell which structure this is from the filename\n", path)
			bad++
			continue
		}

		problems := artcheck.Verify(path, kind)
		if len(problems) == 0 {
			mats, _ := artcheck.Materials(path)
			fmt.Printf("ok    %-34s %s  %d materials\n", filepath.Base(path), kind, len(mats))
			continue
		}
		bad++
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "FAIL  %v\n", p)
		}
	}

	if bad > 0 {
		fmt.Fprintf(os.Stderr, "\n%d model(s) do not carry what the game reads from them.\n", bad)
		os.Exit(1)
	}
}

// kindFor maps "8-battery.glb" back to the structure it is for, by matching
// the slug half against colony.Buildable rather than against a second list
// that could disagree with the loader's.
func kindFor(path string) (colony.Kind, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dash := strings.IndexByte(name, '-')
	if dash < 0 {
		return colony.None, false
	}
	slug := name[dash+1:]

	for _, k := range colony.Buildable {
		if strings.EqualFold(slugFor(k), slug) {
			return k, true
		}
	}
	return colony.None, false
}

// slugFor is the filename half of a structure's model path. It mirrors the
// game's own modelSlug; the two are held together by modelcheck being run on
// the files the game actually loads, which would fail to resolve a kind if
// they drifted.
func slugFor(k colony.Kind) string {
	switch k {
	case colony.Habitat:
		return "habitat"
	case colony.SolarArray:
		return "solar"
	case colony.Mine:
		return "mine"
	case colony.Extractor:
		return "extractor"
	case colony.Condenser:
		return "condenser"
	case colony.Greenhouse:
		return "greenhouse"
	case colony.Geothermal:
		return "geothermal"
	case colony.Battery:
		return "battery"
	case colony.Methane:
		return "methane"
	case colony.Synthesizer:
		return "synthesizer"
	}
	return ""
}
