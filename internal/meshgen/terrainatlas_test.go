package meshgen

import (
	"image"
	"image/png"
	"math"
	"os"
	"testing"
)

// The terrain detail atlas is a contract between an image and some arithmetic,
// and neither half can see the other.
//
// terrainDetailUV reads a cell as 16.5/1024 in from its corner, spanning
// 479/1024. Those numbers describe a 2x2 grid of 512-pixel cells each holding a
// 480-pixel pattern with a 16-pixel white margin. Nothing enforces that: a
// rebake that stretched the patterns to fill their cells would still produce a
// 1024x1024 PNG that loads perfectly, and the only symptom would be a faint
// seam around every hexagon at mip levels above zero — visible at distance,
// which is where this camera lives.
//
// So the image is measured against the arithmetic here. This is the test that
// catches a packer changed without the mesher, or the other way round.

const atlasPath = "../../assets/terrain-detail.png"

func loadAtlas(t *testing.T) image.Image {
	t.Helper()
	f, err := os.Open(atlasPath)
	if err != nil {
		t.Skipf("no terrain atlas built: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", atlasPath, err)
	}
	return img
}

func lum(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return float64(r+g+b) / 3 / 65535
}

// The grid the UV maths assumes.
func TestTerrainAtlasHasTheGridTheMesherAssumes(t *testing.T) {
	img := loadAtlas(t)
	b := img.Bounds()

	const want = 1024
	if b.Dx() != want || b.Dy() != want {
		t.Fatalf("atlas is %dx%d, want %dx%d: terrainDetailUV divides by 1024",
			b.Dx(), b.Dy(), want, want)
	}

	// Four variants, because terrainDetailChoice returns h&3.
	variants := 4
	for v := range variants {
		cx, cy := (v%2)*512, (v/2)*512
		if cx+512 > b.Dx() || cy+512 > b.Dy() {
			t.Errorf("variant %d would sample outside the atlas", v)
		}
	}
}

// The gutter: a 16-pixel white margin inside every cell, which is what stops
// one pattern bleeding into the tile next door under mipmapping.
//
// Measured as the bounding box of everything that is not white, rather than by
// probing the margin. Probing cannot tell: these patterns are drawn as round
// blobs that fade to white at their own edges, so a cell whose pattern had been
// stretched to fill it would still read white in the corners and pass. The
// extent is the thing that actually has to fit.
func TestEveryCellKeepsItsWhiteGutter(t *testing.T) {
	img := loadAtlas(t)

	const (
		cell   = 512
		gutter = 16
		white  = 0.999
	)

	for v := range 4 {
		ox, oy := (v%2)*cell, (v/2)*cell

		minX, minY := cell, cell
		maxX, maxY := -1, -1
		for y := range cell {
			for x := range cell {
				if lum(img, ox+x, oy+y) >= white {
					continue
				}
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
		if maxX < 0 {
			t.Errorf("cell %d is entirely white: its pattern is missing", v)
			continue
		}

		if minX < gutter || minY < gutter || maxX >= cell-gutter || maxY >= cell-gutter {
			t.Errorf("cell %d has ink from (%d,%d) to (%d,%d), outside the %d-pixel "+
				"margin: the pattern has been stretched into its gutter and will bleed "+
				"into the next cell", v, minX, minY, maxX, maxY, gutter)
		}
	}
}

// Every texel is a multiplier against the tile's own colour, so none of them
// may be dark. A pattern that dipped to zero would punch a black hexagon out
// of the terrain.
func TestTheAtlasIsAMultiplierNotAPaint(t *testing.T) {
	img := loadAtlas(t)
	b := img.Bounds()

	lo := 1.0
	var loX, loY int
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if v := lum(img, x, y); v < lo {
				lo, loX, loY = v, x, y
			}
		}
	}

	// 0.7 is well below anything in the current set (0.863) and well above
	// the point where detail stops being "light".
	if lo < 0.7 {
		t.Errorf("darkest texel is %.3f at (%d,%d); the atlas multiplies the tile "+
			"colour, so this would show as a stain rather than as texture", lo, loX, loY)
	}
	if lo > 0.999 {
		t.Error("the atlas is uniformly white: it would have no visible effect at all")
	}
}

// The sample the walls and submerged caps use has to be white, or every cliff
// face in the game picks up whatever is in that corner.
func TestTheNeutralSampleIsWhite(t *testing.T) {
	img := loadAtlas(t)
	b := img.Bounds()

	x := int(float64(terrainNeutralUV[0]) * float64(b.Dx()))
	y := int(float64(terrainNeutralUV[1]) * float64(b.Dy()))
	if got := lum(img, x, y); got < 0.999 {
		t.Errorf("terrainNeutralUV samples %.4f at (%d,%d), want white: "+
			"cliff faces are meant to take no detail", got, x, y)
	}
}

// Rotation has to preserve the hexagon, so the six turns must be multiples of
// sixty degrees and must come back to the start.
func TestDetailRotationsPreserveTheHexagon(t *testing.T) {
	// A point on the unit circle, rotated by each of the six turns, has to
	// land on a sixth-of-a-turn boundary.
	for rot := range 6 {
		got := terrainDetailUV([2]float32{1, 0.5}, 0, rot)

		// Undo the atlas placement to recover the rotated unit coordinate.
		u := (float64(got[0])*1024 - 16.5) / 479
		v := (float64(got[1])*1024 - 16.5) / 479
		dx, dy := u-0.5, v-0.5

		angle := math.Atan2(dy, dx)
		sixth := angle / (math.Pi / 3)
		if d := math.Abs(sixth - math.Round(sixth)); d > 1e-4 {
			t.Errorf("rotation %d turns by %.4f of a sixth, which does not map the "+
				"hexagon onto itself", rot, sixth)
		}

		// And the radius is unchanged, or the pattern would be sheared.
		if r := math.Hypot(dx, dy); math.Abs(r-0.5) > 1e-4 {
			t.Errorf("rotation %d changed the radius to %.5f, want 0.5", rot, r)
		}
	}
}
