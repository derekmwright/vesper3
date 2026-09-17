package meshgen

import (
	"testing"

	"github.com/derekmwright/vesper3/internal/colony"
)

// Builder.Tri flips the winding when the caller hands it corners the other
// way round, which is the only reason the prop definitions can be written
// without thinking about it. This checks that it actually does.
func TestBuilderFlipsWindingToMatchTheOutwardFace(t *testing.T) {
	up := [3]float32{0, 1, 0}
	p0 := [3]float32{0, 0, 0}
	p1 := [3]float32{1, 0, 0}
	p2 := [3]float32{0, 0, 1}

	// The same triangle, given both ways round, must come out identical.
	var a, b Builder
	a.Tri(p0, p1, p2, up, [3]float32{1, 1, 1})
	b.Tri(p0, p2, p1, up, [3]float32{1, 1, 1})

	for _, bld := range []*Builder{&a, &b} {
		v := bld.Verts
		cr := cross(sub(v[1].Pos, v[0].Pos), sub(v[2].Pos, v[0].Pos))
		if dot(normalize(cr), up) > -0.5 {
			t.Errorf("triangle winds with its normal, not against it")
		}
	}
}

// Every structure has to be solid geometry facing outward, for the same
// reason the terrain does: a backwards face is invisible, not broken.
func TestEveryStructureIsWoundOutward(t *testing.T) {
	for _, k := range colony.Buildable {
		t.Run(k.String(), func(t *testing.T) {
			b := Structure(k)
			if len(b.Idx) == 0 {
				t.Fatal("no geometry")
			}
			if len(b.Idx)%3 != 0 {
				t.Fatalf("%d indices, not a whole number of triangles", len(b.Idx))
			}

			var minY, maxR, maxH float32 = 1e9, 0, 0
			for i := 0; i < len(b.Idx); i += 3 {
				v0, v1, v2 := b.Verts[b.Idx[i]], b.Verts[b.Idx[i+1]], b.Verts[b.Idx[i+2]]
				cr := cross(sub(v1.Pos, v0.Pos), sub(v2.Pos, v0.Pos))
				if norm(cr) < 1e-9 {
					t.Fatalf("degenerate triangle %d", i/3)
				}
				if d := dot(normalize(cr), v0.Normal); d > -0.3 {
					t.Fatalf("triangle %d winds with its normal (dot %.3f)", i/3, d)
				}
			}
			for _, v := range b.Verts {
				minY = min(minY, v.Pos[1])
				maxR = max(maxR, sqrt32(v.Pos[0]*v.Pos[0]+v.Pos[2]*v.Pos[2]))
				maxH = max(maxH, v.Pos[1])
			}

			// Structures are placed by setting their origin on a tile's top
			// face, so geometry below y=0 would sink into the ground.
			if minY < -1e-4 {
				t.Errorf("geometry reaches y=%.3f, below the tile surface", minY)
			}
			// And it has to stay on its own tile: a hexagon's inradius is
			// about 0.87 at the grid's tile size.
			if maxR > 0.87 {
				t.Errorf("footprint radius %.3f overhangs the tile (inradius 0.87)", maxR)
			}
			// Logged because structureScale is tuned against it: a structure
			// well under the bound reads as a pebble on its tile.
			t.Logf("footprint radius %.3f of the 0.87 inradius, height %.3f", maxR, maxH)
		})
	}
}

func TestUnknownKindStillGetsGeometry(t *testing.T) {
	b := Structure(colony.Kind(200))
	if len(b.Idx) == 0 {
		t.Error("an unknown kind produced no geometry, so a bad save would be invisible")
	}
}

func TestCursorRingIsFlatAndHexagonal(t *testing.T) {
	b := Cursor(1.0)
	if len(b.Idx) != 6*2*3 {
		t.Errorf("%d indices, want %d for six quads", len(b.Idx), 6*2*3)
	}
	for _, v := range b.Verts {
		if v.Pos[1] != 0 {
			t.Errorf("cursor vertex off the plane at y=%.3f", v.Pos[1])
		}
		if v.Normal != [3]float32{0, 1, 0} {
			t.Errorf("cursor normal %v, want +Y", v.Normal)
		}
	}
}

// The marker has to be tile-sized. It is the only thing telling the player
// which hexagon a click will land on, and one that came out a third of a tile
// across would look like a decoration rather than a cursor.
func TestCursorRingMatchesTheTileItMarks(t *testing.T) {
	const size = 2.5
	b := Cursor(size)

	var gotInner, gotOuter float32 = 1e9, 0
	for _, v := range b.Verts {
		r := sqrt32(v.Pos[0]*v.Pos[0] + v.Pos[2]*v.Pos[2])
		gotInner = min(gotInner, r)
		gotOuter = max(gotOuter, r)
	}

	if want := float32(size * CursorInnerRadius); absF(gotInner-want) > 1e-4 {
		t.Errorf("inner radius %.4f, want %.4f", gotInner, want)
	}
	if want := float32(size * CursorOuterRadius); absF(gotOuter-want) > 1e-4 {
		t.Errorf("outer radius %.4f, want %.4f", gotOuter, want)
	}

	// It must cover most of the tile without spilling over the corners.
	if gotOuter >= size {
		t.Errorf("outer radius %.3f reaches the tile corners at %.3f", gotOuter, size)
	}
	if gotOuter < size*0.8 {
		t.Errorf("outer radius %.3f is well inside the tile (%.3f); the marker will read as too small",
			gotOuter, size)
	}
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
