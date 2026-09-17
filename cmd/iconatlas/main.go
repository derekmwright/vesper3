// Command iconatlas packs the per-structure icon PNGs into the single texture
// the HUD samples.
//
// One texture rather than one per icon, because the interface is drawn as one
// mesh in one draw call and a second texture would mean a second.
//
// The atlas is a plain grid and the cell a source lands in is the number on the
// front of its filename, so the ordering is visible in a directory listing:
//
//	01-habitat.png .. 08-battery.png   the hotbar, in colony.Buildable order
//	09-water.png .. 12-food.png        the resource rows on the status panel
//
//	go run ./cmd/iconatlas -src assets/icons-src -out assets/icons.png
//
// The numbers are read as numbers rather than sorted as text. Sorting by name
// worked while there were eight sources and would have quietly put "10-" before
// "2-" on the ninth, which is the kind of break that shows up as a wrong icon
// rather than as an error.
//
// The grid is padded to -cols by -rows whatever the source count, so a cell
// with no art is transparent and drawing it draws nothing. That is what lets
// the HUD refer to a cell before its icon exists.
//
// The sources are committed alongside the atlas so this is reproducible rather
// than a one-time paste.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

func main() {
	src := flag.String("src", "assets/icons-src", "directory of square source PNGs")
	out := flag.String("out", "assets/icons.png", "atlas to write")
	cell := flag.Int("cell", 128, "cell size in pixels")
	cols := flag.Int("cols", 4, "cells per row")
	rows := flag.Int("rows", 0, "rows to pad the grid to; 0 fits the sources")
	flag.Parse()

	cells, err := sourceFiles(*src)
	if err != nil {
		log.Fatal(err)
	}
	if len(cells) == 0 {
		log.Fatalf("no numbered PNGs in %s", *src)
	}

	// One past the highest cell claimed, not the number of files: a gap in the
	// numbering is a hole in the atlas, not a shift of everything after it.
	used := 0
	for n := range cells {
		if n+1 > used {
			used = n + 1
		}
	}
	gridRows := (used + *cols - 1) / *cols
	if *rows > gridRows {
		gridRows = *rows
	}
	atlas := image.NewNRGBA(image.Rect(0, 0, *cols**cell, gridRows**cell))

	for i := 0; i < used; i++ {
		path, ok := cells[i]
		if !ok {
			fmt.Printf("cell %d  %-28s (empty)\n", i, "-")
			continue
		}
		img, err := readPNG(path)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}

		cx := (i % *cols) * *cell
		cy := (i / *cols) * *cell
		dst := fitRect(img.Bounds(), *cell).Add(image.Pt(cx, cy))

		// CatmullRom rather than a box filter: these are downscaled by about
		// ten to one, and a cheaper kernel turns the thin bright edges the
		// icons are drawn with into aliasing that is very visible at 48px.
		xdraw.CatmullRom.Scale(atlas, dst, img, img.Bounds(), draw.Over, nil)

		fmt.Printf("cell %d  %-28s %dx%d -> %v\n",
			i, filepath.Base(path), img.Bounds().Dx(), img.Bounds().Dy(), dst)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, atlas); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nwrote %s: %dx%d, %d of %d cells filled in a %dx%d grid\n",
		*out, atlas.Bounds().Dx(), atlas.Bounds().Dy(),
		len(cells), *cols*gridRows, *cols, gridRows)
}

// sourceFiles maps each PNG in dir to the atlas cell its filename claims.
//
// The name must start with a number and a dash — "09-water.png" is cell 8,
// counting from one so the files read the way the hotbar keys do. A file
// without a number is a mistake worth stopping for rather than silently
// dropping, and two files claiming one cell is worse: one would overwrite the
// other and the atlas would still look plausible.
func sourceFiles(dir string) (map[int]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	out := make(map[int]string)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(filepath.Ext(name), ".png") {
			continue
		}

		dash := strings.IndexByte(name, '-')
		if dash <= 0 {
			return nil, fmt.Errorf("%s: name must start with a cell number, as in 01-habitat.png", name)
		}
		n, err := strconv.Atoi(name[:dash])
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a cell number", name, name[:dash])
		}
		if n < 1 {
			return nil, fmt.Errorf("%s: cells are numbered from 1", name)
		}
		if prev, taken := out[n-1]; taken {
			return nil, fmt.Errorf("%s and %s both claim cell %d", filepath.Base(prev), name, n)
		}
		out[n-1] = filepath.Join(dir, name)
	}
	return out, nil
}

// fitRect returns the largest centred rectangle of the given cell size that
// preserves the source aspect ratio. Sources are meant to be square, but a
// generated image that came back slightly off would otherwise be stretched.
func fitRect(b image.Rectangle, cell int) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	if w == h {
		return image.Rect(0, 0, cell, cell)
	}
	if w > h {
		nh := h * cell / w
		off := (cell - nh) / 2
		return image.Rect(0, off, cell, off+nh)
	}
	nw := w * cell / h
	off := (cell - nw) / 2
	return image.Rect(off, 0, off+nw, cell)
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
