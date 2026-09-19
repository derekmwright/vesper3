// Package meshgen turns the tile map into GPU geometry.
//
// The map is meshed in chunks rather than per tile. One entity per hexagon
// would be one draw call per hexagon, and a 48x48 map is 2304 of them before
// a single building exists; a chunk is one draw call for 64 tiles, and
// terraforming one tile costs a rebuild of only the chunk that holds it.
//
// # Winding
//
// The renderer culls with FrontFace: Clockwise under a Y-flipped projection
// (renderer/pipeline.go), which is easier to get right by copying a face the
// engine already draws than by reasoning about it. Its CreatePlane builds an
// upward-facing quad whose (v1-v0) x (v2-v0) points along -Y, so the rule
// every face here follows is:
//
//	the triangle cross product points OPPOSITE its surface normal.
//
// Get it backwards and the geometry does not error, warn, or flicker. It
// silently vanishes, because every face is being culled as a backface.
package meshgen

import (
	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// Chunk dimensions in tiles. 8x8 keeps a chunk under 2,400 vertices, which
// matters twice: uint16 indices stay valid (the engine dynamic mesh path only
// accepts those), and a terraform rebuild stays small enough to do
// synchronously on the frame the player clicked.
const (
	ChunkCols = 8
	ChunkRows = 8
)

// The most geometry one tile can contribute: a cap of 13 vertices and 18
// triangles, plus six cliff walls of 4 vertices and 2 triangles each.
//
// These are not decoration. Chunks are uploaded through the engine's dynamic
// mesh path, and UpdateMeshData silently TRUNCATES anything past the capacity
// the buffer was created with — a chunk that overflowed would come back with
// its last few tiles quietly missing and nothing logged. ChunkCapacity sizes
// the buffer from these, and a test holds the mesher to them.
const (
	MaxVertsPerTile   = 13 + 6*4
	MaxIndicesPerTile = 18*3 + 6*6
)

// ChunkCapacity is how large a dynamic mesh has to be to hold any chunk.
func ChunkCapacity() (maxVerts, maxIndices int) {
	tiles := ChunkCols * ChunkRows
	return tiles * MaxVertsPerTile, tiles * MaxIndicesPerTile
}

// Vertices are tinted by these factors to make the grid legible without a
// texture: a hexagon outer rim is darker than its middle, so tile edges read
// on flat ground, and cliff faces are darker still and shade downward.
const (
	rimInset      = 0.88 // where the darker rim starts, as a fraction of the circumradius
	rimShade      = 0.74
	wallShade     = 0.62
	wallFootShade = 0.40 // at the bottom of a cliff, for a cheap contact shadow
	jitterAmount  = 0.05
)

// ChunkID addresses a chunk by its position in the chunk grid.
type ChunkID struct{ CX, CY int }

// ChunkGrid returns how many chunks cover the map on each axis.
func ChunkGrid(m *world.Map) (nx, ny int) {
	return (m.Cols + ChunkCols - 1) / ChunkCols, (m.Rows + ChunkRows - 1) / ChunkRows
}

// ChunkOf returns the chunk holding a tile, and whether the tile is on the
// map at all.
func ChunkOf(m *world.Map, a hex.Axial) (ChunkID, bool) {
	if !m.Contains(a) {
		return ChunkID{}, false
	}
	col, row := world.Offset(a)
	return ChunkID{CX: col / ChunkCols, CY: row / ChunkRows}, true
}

// Origin is the world-space XZ that a chunk's vertices are relative to.
//
// Chunk geometry is built in chunk-local space and placed with the entity's
// Transform. Building it in world space instead would put every chunk's
// bounding sphere around the world origin, and the renderer's frustum cull
// would then treat every chunk as always visible.
func Origin(id ChunkID) (x, z float32) {
	return world.Layout.Center(world.FromOffset(id.CX*ChunkCols, id.CY*ChunkRows))
}

// BuildChunk generates the geometry for one chunk. It reads tiles outside the
// chunk to decide cliff walls, so a chunk rebuilt after a neighbouring tile
// changed comes out correct.
//
// Passing the previous slices back in avoids reallocating on every terraform;
// pass nil on the first call.
func BuildChunk(m *world.Map, id ChunkID, verts []renderer.Vertex, idx []uint16) ([]renderer.Vertex, []uint16) {
	verts, idx = verts[:0], idx[:0]

	ox, oz := Origin(id)
	corners := world.Layout.Corners()

	col0, row0 := id.CX*ChunkCols, id.CY*ChunkRows
	for row := row0; row < row0+ChunkRows && row < m.Rows; row++ {
		for col := col0; col < col0+ChunkCols && col < m.Cols; col++ {
			a := world.FromOffset(col, row)
			tile := m.AtOffset(col, row)

			cx, cz := world.Layout.Center(a)
			cx, cz = cx-ox, cz-oz
			top := world.SurfaceYAt(int(tile.Elevation))

			base := tile.Terrain.Info().Color
			base = scaleColor(base, 1+jitter(col, row)*jitterAmount)

			capStart := len(verts)
			verts, idx = appendCap(verts, idx, cx, cz, top, base, corners)
			variant, rotation := terrainDetailChoice(m.Seed, col, row)
			for i := capStart; i < len(verts); i++ {
				if tile.Terrain == world.Sea {
					verts[i].UV = terrainNeutralUV
				} else {
					verts[i].UV = terrainDetailUV(verts[i].UV, variant, rotation)
				}
			}
			verts, idx = appendWalls(verts, idx, m, a, cx, cz, top, base, corners)
		}
	}
	return verts, idx
}

// isSea reports whether a tile exists and is under water. A tile off the edge
// of the map is not sea: the map's rim keeps its wall.
func isSea(m *world.Map, a hex.Axial) bool {
	t := m.At(a)
	return t != nil && t.Terrain == world.Sea
}

// appendCap writes a tile's top face: a fan over an inner disc plus a darker
// rim, which is what draws the grid on ground that is otherwise flat and one
// colour.
func appendCap(verts []renderer.Vertex, idx []uint16, cx, cz, y float32, col [3]float32, corners [6][2]float32) ([]renderer.Vertex, []uint16) {
	up := [3]float32{0, 1, 0}
	rim := scaleColor(col, rimShade)

	center := uint16(len(verts))
	verts = append(verts, renderer.Vertex{
		Pos: [3]float32{cx, y, cz}, Color: col, Normal: up, UV: [2]float32{0.5, 0.5},
	})

	inner := uint16(len(verts))
	for _, c := range corners {
		verts = append(verts, renderer.Vertex{
			Pos:    [3]float32{cx + c[0]*rimInset, y, cz + c[1]*rimInset},
			Color:  col,
			Normal: up,
			UV:     [2]float32{0.5 + c[0]*0.5*rimInset, 0.5 + c[1]*0.5*rimInset},
		})
	}

	outer := uint16(len(verts))
	for _, c := range corners {
		verts = append(verts, renderer.Vertex{
			Pos:    [3]float32{cx + c[0], y, cz + c[1]},
			Color:  rim,
			Normal: up,
			UV:     [2]float32{0.5 + c[0]*0.5, 0.5 + c[1]*0.5},
		})
	}

	for i := uint16(0); i < 6; i++ {
		next := (i + 1) % 6
		// Corner order runs anticlockwise in the XZ plane, which makes these
		// wind opposite +Y, the convention this file's doc comment sets out.
		idx = append(idx, center, inner+i, inner+next)
		idx = append(idx, inner+i, outer+i, outer+next)
		idx = append(idx, inner+i, outer+next, inner+next)
	}
	return verts, idx
}

// appendWalls writes a cliff face on each side where the neighbouring tile
// sits lower. Sides facing a tile at the same height or higher are skipped:
// they would be interior faces, invisible and paid for anyway.
func appendWalls(verts []renderer.Vertex, idx []uint16, m *world.Map, a hex.Axial, cx, cz, top float32, col [3]float32, corners [6][2]float32) ([]renderer.Vertex, []uint16) {
	wall := scaleColor(col, wallShade)
	foot := scaleColor(col, wallFootShade)

	// A tile that is under water does not need a cliff face against another
	// tile that is also under water.
	//
	// Every tile gets walls wherever the ground drops, sea floor included, and
	// the sea floor has the same varied elevation the land does. Those faces
	// are shaded darker than the caps they hang off, so through translucent
	// water at a shallow angle they read as dark patches scattered over the
	// shelf - geometry the player can neither reach nor build on, drawn only
	// to be seen as noise.
	//
	// The coastline keeps its cliff: that drop is land against sea, so only
	// one side is submerged and the test below is false.
	submerged := isSea(m, a)

	for d := 0; d < 6; d++ {
		n := a.Neighbor(d)
		if submerged && isSea(m, n) {
			continue
		}

		below := world.SurfaceYAt(m.Elevation(n))
		if below >= top {
			continue
		}

		// Corners d and d+1 bound the edge shared with Directions[d]; the hex
		// package pins that invariant with a test.
		ax, az := cx+corners[d][0], cz+corners[d][1]
		bx, bz := cx+corners[(d+1)%6][0], cz+corners[(d+1)%6][1]

		// Outward normal of an edge on an anticlockwise polygon.
		dx, dz := bx-ax, bz-az
		out := normalize([3]float32{dz, 0, -dx})

		v := uint16(len(verts))
		verts = append(verts,
			renderer.Vertex{Pos: [3]float32{ax, top, az}, Color: wall, Normal: out, UV: terrainNeutralUV},
			renderer.Vertex{Pos: [3]float32{ax, below, az}, Color: foot, Normal: out, UV: terrainNeutralUV},
			renderer.Vertex{Pos: [3]float32{bx, below, bz}, Color: foot, Normal: out, UV: terrainNeutralUV},
			renderer.Vertex{Pos: [3]float32{bx, top, bz}, Color: wall, Normal: out, UV: terrainNeutralUV},
		)
		// aTop, aFoot, bFoot then aTop, bFoot, bTop: both wind opposite the
		// outward normal.
		idx = append(idx, v+0, v+1, v+2, v+0, v+2, v+3)
	}
	return verts, idx
}

func scaleColor(c [3]float32, f float32) [3]float32 {
	return [3]float32{clamp01(c[0] * f), clamp01(c[1] * f), clamp01(c[2] * f)}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// jitter returns a stable value in [-1, 1] for a tile, so neighbouring tiles
// of the same terrain are not exactly the same colour. Hashed rather than
// random: the map has to look identical every time it is drawn.
func jitter(col, row int) float32 {
	h := uint32(col)*0x9E3779B1 ^ uint32(row)*0x85EBCA77
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	return float32(int32(h%2001)-1000) / 1000
}

// The atlas contains four 512-pixel cells in a 2x2 grid, with a 16-pixel white
// gutter around each 480-pixel map. Walls and submerged caps sample the white
// corner so only the top surfaces receive detail. The PNG is neutral grayscale:
// the terrain's vertex colors remain its biome identity.
var terrainNeutralUV = [2]float32{.5 / 1024, .5 / 1024}

func terrainDetailChoice(seed int64, col, row int) (variant, rotation int) {
	// Stable across chunk rebuilds, terraforming and save/load; independent of
	// iteration order and the simulation's random-number stream.
	h := uint64(seed) ^ uint64(uint32(col))*0x9e3779b97f4a7c15 ^ uint64(uint32(row))*0xbf58476d1ce4e5b9
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	h ^= h >> 31
	return int(h & 3), int((h >> 8) % 6)
}

func terrainDetailUV(uv [2]float32, variant, rotation int) [2]float32 {
	// Six rotations preserve the hexagon. Pixel-center bounds plus the gutter
	// prevent neighboring atlas entries leaking into close and mid-range caps.
	cosine := [6]float32{1, .5, -.5, -1, -.5, .5}
	sine := [6]float32{0, .8660254, .8660254, 0, -.8660254, -.8660254}
	x, y := uv[0]-.5, uv[1]-.5
	u := .5 + x*cosine[rotation] - y*sine[rotation]
	v := .5 + x*sine[rotation] + y*cosine[rotation]
	return [2]float32{(float32(variant%2)*512 + 16.5 + u*479) / 1024, (float32(variant/2)*512 + 16.5 + v*479) / 1024}
}
