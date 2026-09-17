package meshgen

import (
	"testing"

	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/worldbuild/internal/hex"
	"github.com/derekmwright/worldbuild/internal/world"
)

// Backface culling is a silent failure: get the winding wrong and the terrain
// does not glitch, it disappears, and the only symptom is an empty frame. So
// this recomputes the cross product of every triangle the mesher emits and
// checks it against the normal the mesher wrote on the same vertices.
//
// To see it fail, swap any two indices in appendCap or appendWalls.
func TestEveryTriangleWindsAgainstItsNormal(t *testing.T) {
	m := testMap(t)
	nx, ny := ChunkGrid(m)

	checked := 0
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			verts, idx := BuildChunk(m, ChunkID{cx, cy}, nil, nil)

			if len(idx)%3 != 0 {
				t.Fatalf("chunk %d,%d: %d indices, not a whole number of triangles", cx, cy, len(idx))
			}

			for i := 0; i < len(idx); i += 3 {
				a, b, c := verts[idx[i]], verts[idx[i+1]], verts[idx[i+2]]

				e1 := sub(b.Pos, a.Pos)
				e2 := sub(c.Pos, a.Pos)
				cr := cross(e1, e2)

				// Degenerate triangles have no winding to check, and none
				// should be emitted in the first place.
				if norm(cr) < 1e-9 {
					t.Fatalf("chunk %d,%d: degenerate triangle at index %d", cx, cy, i)
				}

				if d := dot(normalize(cr), a.Normal); d > -0.5 {
					t.Fatalf("chunk %d,%d triangle %d: cross . normal = %.3f, want <= -0.5 "+
						"(cross must oppose the normal, see the package doc)", cx, cy, i/3, d)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no triangles were checked, so this test proved nothing")
	}
	t.Logf("checked %d triangles", checked)
}

// The engine's dynamic mesh path takes uint16 indices only, so a chunk that
// overflows 65535 vertices would wrap silently into garbage geometry.
func TestChunksFitInUint16Indices(t *testing.T) {
	m := testMap(t)
	nx, ny := ChunkGrid(m)

	worst := 0
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			verts, idx := BuildChunk(m, ChunkID{cx, cy}, nil, nil)
			if len(verts) > 65535 {
				t.Fatalf("chunk %d,%d has %d vertices, past the uint16 index limit", cx, cy, len(verts))
			}
			for _, ix := range idx {
				if int(ix) >= len(verts) {
					t.Fatalf("chunk %d,%d: index %d out of range for %d vertices", cx, cy, ix, len(verts))
				}
			}
			worst = max(worst, len(verts))
		}
	}
	// Headroom matters: this is the number that decides whether ChunkCols and
	// ChunkRows can grow.
	t.Logf("worst chunk: %d vertices (%.0f%% of the uint16 limit)", worst, float64(worst)/65535*100)
}

// A wall between two tiles at the same height is invisible and still costs
// four vertices and two triangles. On mostly flat ground that is most of the
// mesh, so this is worth pinning.
func TestNoWallsBetweenTilesAtTheSameHeight(t *testing.T) {
	// 32x32 puts chunk 1,1 in the interior. On a 16x16 map it would be a
	// corner chunk, and the map edge counts as ground at elevation 0, so it
	// would correctly grow a wall along two sides and fail this test for a
	// reason that has nothing to do with flatness.
	m, err := world.NewMap(32, 32, 1)
	if err != nil {
		t.Fatal(err)
	}
	// A flat plateau, entirely above the sea floor.
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = 6
	})

	verts, _ := BuildChunk(m, ChunkID{1, 1}, nil, nil)

	// Cap only: 13 vertices per tile, 64 tiles in an interior chunk.
	const wantCap = 13 * ChunkCols * ChunkRows
	if len(verts) != wantCap {
		t.Errorf("flat chunk has %d vertices, want %d (cap only, no walls)", len(verts), wantCap)
	}

	// Drop one tile and its six neighbours must grow a wall apiece.
	m.At(world.FromOffset(12, 12)).Elevation = 3
	verts2, _ := BuildChunk(m, ChunkID{1, 1}, nil, nil)
	if len(verts2) <= len(verts) {
		t.Errorf("lowering a tile did not add wall vertices: %d then %d", len(verts), len(verts2))
	}
}

// Chunks are built in chunk-local space so their bounding spheres are tight
// and the renderer can cull them. Geometry that drifted into world space
// would still draw, just never be culled, so nothing else would catch it.
func TestChunkGeometryIsLocalToItsChunk(t *testing.T) {
	m := testMap(t)
	nx, ny := ChunkGrid(m)

	// A chunk spans 8 columns and 8 rows, and the widest a local coordinate
	// can get is that span plus one tile of overhang.
	limit := float32(ChunkCols)*world.Layout.Width()*0.75 + float32(ChunkRows)*world.Layout.Height() + world.Layout.Width()

	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			verts, _ := BuildChunk(m, ChunkID{cx, cy}, nil, nil)
			for _, v := range verts {
				if abs32(v.Pos[0]) > limit || abs32(v.Pos[2]) > limit {
					t.Fatalf("chunk %d,%d: vertex at (%.2f, %.2f) exceeds local bound %.2f",
						cx, cy, v.Pos[0], v.Pos[2], limit)
				}
			}
		}
	}
}

// Every tile has to end up in exactly one chunk, or it is either meshed twice
// or not at all.
func TestChunkOfCoversTheMapExactlyOnce(t *testing.T) {
	m := testMap(t)
	nx, ny := ChunkGrid(m)

	seen := map[ChunkID]int{}
	m.Each(func(a hex.Axial, _ *world.Tile) {
		id, ok := ChunkOf(m, a)
		if !ok {
			t.Fatalf("tile %v is on the map but has no chunk", a)
		}
		if id.CX < 0 || id.CX >= nx || id.CY < 0 || id.CY >= ny {
			t.Fatalf("tile %v maps to chunk %v, outside the %dx%d chunk grid", a, id, nx, ny)
		}
		seen[id]++
	})
	if len(seen) != nx*ny {
		t.Errorf("%d chunks hold tiles, want %d", len(seen), nx*ny)
	}
}

// The scratch-slice form has to produce the same mesh as a fresh build, or
// terraforming would slowly corrupt whichever chunk the player edits.
func TestReusingScratchSlicesGivesTheSameMesh(t *testing.T) {
	m := testMap(t)

	fresh, freshIdx := BuildChunk(m, ChunkID{2, 2}, nil, nil)

	scratchV, scratchI := BuildChunk(m, ChunkID{0, 0}, nil, nil)
	scratchV, scratchI = BuildChunk(m, ChunkID{5, 4}, scratchV, scratchI)
	scratchV, scratchI = BuildChunk(m, ChunkID{2, 2}, scratchV, scratchI)

	if len(scratchV) != len(fresh) || len(scratchI) != len(freshIdx) {
		t.Fatalf("reused: %d verts %d idx, fresh: %d verts %d idx",
			len(scratchV), len(scratchI), len(fresh), len(freshIdx))
	}
	for i := range fresh {
		if scratchV[i] != fresh[i] {
			t.Fatalf("vertex %d differs after reuse", i)
		}
	}
	for i := range freshIdx {
		if scratchI[i] != freshIdx[i] {
			t.Fatalf("index %d differs after reuse", i)
		}
	}
}

func testMap(t *testing.T) *world.Map {
	t.Helper()
	m, err := world.NewMap(48, 48, 20260916)
	if err != nil {
		t.Fatal(err)
	}
	world.Generate(m)
	return m
}

func sub(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func dot(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func norm(v [3]float32) float32 { return sqrt32(dot(v, v)) }

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

var _ = renderer.Vertex{}
