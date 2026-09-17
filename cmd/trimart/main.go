// Command trimart crops a PNG to its opaque content and resizes it.
//
// Generated art arrives with the subject floating in a transparent margin,
// which is harmless for an icon and wrong for a nine-slice: the slice takes
// its corner regions from the image corners, so a frame that stops short of
// the edge gets corners made of empty space and edges that stretch the margin
// along with the bezel.
//
//	go run ./cmd/trimart -in raw.png -out assets/panel.png -size 256 -square
//
// It also reports the border thickness at the midpoint of each edge, which is
// what the nine-slice inset has to be set from.
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

	xdraw "golang.org/x/image/draw"
)

func main() {
	in := flag.String("in", "", "source PNG")
	out := flag.String("out", "", "destination PNG")
	size := flag.Int("size", 0, "resize the result to this many pixels square; 0 keeps it")
	square := flag.Bool("square", false, "pad the crop to a square before resizing")
	threshold := flag.Int("threshold", 16, "alpha at or below this counts as empty")
	flag.Parse()

	if *in == "" || *out == "" {
		log.Fatal("need -in and -out")
	}

	src, err := readPNG(*in)
	if err != nil {
		log.Fatal(err)
	}

	box := opaqueBounds(src, uint32(*threshold))
	if box.Empty() {
		log.Fatalf("%s is entirely transparent", *in)
	}
	fmt.Printf("source %v, content %v\n", src.Bounds(), box)

	if *square {
		box = squareOut(box, src.Bounds())
		fmt.Printf("squared to %v\n", box)
	}

	cropped := image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(cropped, cropped.Bounds(), src, box.Min, draw.Src)

	result := image.Image(cropped)
	if *size > 0 {
		scaled := image.NewNRGBA(image.Rect(0, 0, *size, *size))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), cropped, cropped.Bounds(), draw.Src, nil)
		result = scaled
	}

	reportBorder(result, uint32(*threshold))

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, result); err != nil {
		log.Fatal(err)
	}

	b := result.Bounds()
	fmt.Printf("wrote %s: %dx%d\n", *out, b.Dx(), b.Dy())
}

// opaqueBounds is the tightest rectangle containing every pixel above the
// alpha threshold.
func opaqueBounds(img image.Image, threshold uint32) image.Rectangle {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 > threshold {
				minX = min(minX, x)
				minY = min(minY, y)
				maxX = max(maxX, x+1)
				maxY = max(maxY, y+1)
			}
		}
	}
	if minX >= maxX || minY >= maxY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

// squareOut grows a rectangle to a square about its centre, clamped to the
// image. A nine-slice wants equal insets on both axes, which needs a square.
func squareOut(box, limit image.Rectangle) image.Rectangle {
	side := max(box.Dx(), box.Dy())
	cx, cy := (box.Min.X+box.Max.X)/2, (box.Min.Y+box.Max.Y)/2

	out := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	if out.Min.X < limit.Min.X {
		out = out.Add(image.Pt(limit.Min.X-out.Min.X, 0))
	}
	if out.Min.Y < limit.Min.Y {
		out = out.Add(image.Pt(0, limit.Min.Y-out.Min.Y))
	}
	if out.Max.X > limit.Max.X {
		out = out.Sub(image.Pt(out.Max.X-limit.Max.X, 0))
	}
	if out.Max.Y > limit.Max.Y {
		out = out.Sub(image.Pt(0, out.Max.Y-limit.Max.Y))
	}
	return out.Intersect(limit)
}

// reportBorder measures how far the opaque frame runs in from each edge at
// its midpoint, which is what the nine-slice inset is chosen from: too small
// and the stretched edge eats the corner decoration, too large and the corners
// overlap on a small panel.
func reportBorder(img image.Image, threshold uint32) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	run := func(name string, steps int, at func(i int) (int, int)) {
		thickness := 0
		for i := range steps {
			x, y := at(i)
			if _, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA(); a>>8 > threshold {
				thickness++
				continue
			}
			break
		}
		fmt.Printf("  %-6s border %d px (%.0f%% of the side)\n",
			name, thickness, float64(thickness)/float64(steps)*100)
	}

	run("left", w/2, func(i int) (int, int) { return i, h / 2 })
	run("right", w/2, func(i int) (int, int) { return w - 1 - i, h / 2 })
	run("top", h/2, func(i int) (int, int) { return w / 2, i })
	run("bottom", h/2, func(i int) (int, int) { return w / 2, h - 1 - i })
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
