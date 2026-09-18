// Command terrainatlas packs the terrain detail patterns into one texture and
// writes a manifest saying which cell each one landed in.
//
//	go run ./cmd/terrainatlas -src assets/terrain-src -out assets/terrain-detail.png
//
// The patterns are near-white greyscale, because the lit shader multiplies
// them by the vertex colour a tile already carries:
//
//	vec3 baseColor = fragColor * texSample.rgb;
//
// So a coloured pattern would give colour times colour — muddy, and darker
// than either. White is "leave this tile the colour it already is", and the
// detail is how far below white the pattern dips. Measured on the current set
// that is 0.86 at the deepest and 0.99 on average, which is what "lightly
// texturing" turns out to mean in numbers.
//
// # The gutter is the contract
//
// A cell is 512 pixels holding a 480-pixel pattern, leaving a 16-pixel white
// margin all the way round. That margin is not spare room, it is what stops
// one pattern bleeding into the tile next door: the mesher insets its reads to
// 16.5/1024 and spans 479/1024, and at mip levels above zero a sample near a
// cell edge averages in whatever is beyond it. Without the margin that is the
// neighbouring pattern, and it shows up as a seam on every hexagon — visible
// only at distance, which is exactly where this camera sits.
//
// White is the safe thing to bleed in, because every texel here is a
// multiplier: white means "leave this tile the colour it already is".
//
// So this tool composes rather than scales to fit. A source that is already
// the content size is placed untouched; one that is not is resampled to the
// content size and then placed. Either way the margin survives, and
// TestTerrainAtlasMatchesTheMesher checks the result against the numbers
// internal/meshgen actually reads with.
//
// # Which pattern goes where does not matter
//
// The four patterns are not one-per-terrain. internal/meshgen picks among them
// per *tile*, by a hash of the tile's position, so that neighbouring tiles of
// the same terrain do not repeat — and rotates each by one of six turns that
// preserve the hexagon. Terrain identity stays in the vertex colour.
//
// That is why there is no manifest here and no name-to-cell mapping: nothing
// downstream asks which cell holds which pattern. Adding or replacing a
// pattern changes the variety, not the meaning, and the only thing that has to
// stay true is the count the mesher hashes into.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
)

func main() {
	src := flag.String("src", "assets/terrain-src", "directory of greyscale detail patterns")
	out := flag.String("out", "assets/terrain-detail.png", "atlas to write")
	cell := flag.Int("cell", 512, "cell size in pixels, including the gutter")
	gutter := flag.Int("gutter", 16, "white margin inside each cell, in pixels")
	flag.Parse()

	content := *cell - 2**gutter
	if content <= 0 {
		log.Fatalf("a %d-pixel gutter leaves nothing inside a %d-pixel cell", *gutter, *cell)
	}

	paths, err := filepath.Glob(filepath.Join(*src, "*.png"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		log.Fatalf("no PNGs in %s", *src)
	}

	// A square-ish grid, so the atlas stays close to a power of two on both
	// sides whatever the pattern count.
	cols := 1
	for cols*cols < len(paths) {
		cols++
	}
	rows := (len(paths) + cols - 1) / cols

	atlas := image.NewNRGBA(image.Rect(0, 0, cols**cell, rows**cell))

	// White, not transparent. An empty cell has to mean "change nothing",
	// because every texel of this atlas is a multiplier — and a transparent
	// one sampled at a mip boundary would read as black and punch a hole in
	// the terrain.
	draw.Draw(atlas, atlas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for i, path := range paths {
		img, err := readPNG(path)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}

		name := strings.TrimSuffix(filepath.Base(path), ".png")

		// Composed into the middle of the cell, never stretched to fill it:
		// the margin that is left is the whole point. A source already at the
		// content size is copied across untouched.
		cx, cy := (i%cols)**cell+*gutter, (i/cols)**cell+*gutter
		dst := image.Rect(cx, cy, cx+content, cy+content)
		if img.Bounds().Dx() == content && img.Bounds().Dy() == content {
			draw.Draw(atlas, dst, img, img.Bounds().Min, draw.Src)
		} else {
			xdraw.CatmullRom.Scale(atlas, dst, img, img.Bounds(), draw.Src, nil)
		}

		lo, hi := extremes(img)
		fit := "placed"
		if img.Bounds().Dx() != content || img.Bounds().Dy() != content {
			fit = fmt.Sprintf("resampled to %d", content)
		}
		fmt.Printf("cell %d  %-24s %4dx%-4d  range %.3f-%.3f  %s\n",
			i, name, img.Bounds().Dx(), img.Bounds().Dy(), lo, hi, fit)

		if hi < 0.90 {
			log.Fatalf("%s is too dark to be a multiplier: brightest texel is %.3f. "+
				"These are multiplied by the tile colour, so they have to sit near white.", name, hi)
		}
	}

	if err := writePNG(*out, atlas); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nwrote %s: %dx%d, %d of %d cells in a %dx%d grid, "+
		"%d-pixel content in %d-pixel cells\n",
		*out, atlas.Bounds().Dx(), atlas.Bounds().Dy(), len(paths), cols*rows,
		cols, rows, content, *cell)
}

// extremes returns the darkest and brightest luminance in an image, which is
// what decides whether it can be used as a multiplier at all.
func extremes(img image.Image) (lo, hi float64) {
	b := img.Bounds()
	lo, hi = 1, 0
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			v := float64(r+g+bl) / 3 / 65535
			lo = min(lo, v)
			hi = max(hi, v)
		}
	}
	return lo, hi
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
