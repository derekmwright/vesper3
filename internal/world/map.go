package world

import (
	"fmt"

	"github.com/derekmwright/vesper3/internal/hex"
)

// The vertical and horizontal scale of the world. TileSize is a hexagon
// circumradius in world units; StepHeight is how much one elevation step
// raises the ground.
//
// The ratio between them is a look, not an accident: at 0.34 against a
// circumradius of 1 a cliff is a clear step without turning the map into a
// stack of towers, and a ten-step mountain still fits under a camera that can
// see the whole colony.
const (
	TileSize   = 1.0
	StepHeight = 0.34

	// SeaLevel is the highest elevation still under methane, and MaxElevation
	// caps the peaks. Terraforming clamps into (SeaLevel, MaxElevation] so the
	// coastline never moves: the water surface is built once at load and would
	// otherwise go stale the first time a player dug a channel.
	SeaLevel     = 3
	MaxElevation = 14
)

// Layout is the single grid every part of the game shares.
var Layout = hex.Layout{Size: TileSize}

// SurfaceYAt is the world Y of the top face of a tile at a given elevation.
func SurfaceYAt(elevation int) float32 { return float32(elevation) * StepHeight }

// SeaY is the world Y of the methane surface, half a step above the highest
// submerged ground so the shallows read as shallow.
func SeaY() float32 { return SurfaceYAt(SeaLevel) + StepHeight*0.5 }

// Tile is one hexagon of ground.
type Tile struct {
	Terrain   Terrain
	Elevation int8
}

// Map is a finite patch of the planet, stored as a rectangle of columns and
// rows in even-q offset coordinates and addressed by axial coordinates.
//
// Offset storage keeps the array rectangular and dense. An axial rhombus
// would either waste half the array or shear the map into a parallelogram on
// screen. Callers never see the offset form: it lives between Axial and the
// slice index and nowhere else.
type Map struct {
	Cols, Rows int
	Seed       int64
	Tiles      []Tile
}

// NewMap allocates an empty map. Generate fills one in.
func NewMap(cols, rows int, seed int64) (*Map, error) {
	if cols < 2 || rows < 2 {
		return nil, fmt.Errorf("world: map must be at least 2x2, got %dx%d", cols, rows)
	}
	return &Map{
		Cols:  cols,
		Rows:  rows,
		Seed:  seed,
		Tiles: make([]Tile, cols*rows),
	}, nil
}

// Offset converts an axial coordinate to the even-q column and row used for
// storage.
func Offset(a hex.Axial) (col, row int) {
	col = int(a.Q)
	row = int(a.R) + (col+(col&1))/2
	return
}

// FromOffset inverts Offset.
func FromOffset(col, row int) hex.Axial {
	return hex.Axial{Q: int32(col), R: int32(row - (col+(col&1))/2)}
}

// Contains reports whether a coordinate is inside the map.
func (m *Map) Contains(a hex.Axial) bool {
	col, row := Offset(a)
	return col >= 0 && col < m.Cols && row >= 0 && row < m.Rows
}

// At returns a pointer to a tile, or nil when the coordinate is off the map.
// The pointer is live: write through it, then remesh the chunk that holds it.
func (m *Map) At(a hex.Axial) *Tile {
	col, row := Offset(a)
	if col < 0 || col >= m.Cols || row < 0 || row >= m.Rows {
		return nil
	}
	return &m.Tiles[row*m.Cols+col]
}

// AtOffset returns the tile at a storage position, which is what the mesher
// and the generator iterate over.
func (m *Map) AtOffset(col, row int) *Tile {
	if col < 0 || col >= m.Cols || row < 0 || row >= m.Rows {
		return nil
	}
	return &m.Tiles[row*m.Cols+col]
}

// Elevation returns a tile elevation. Off-map coordinates report the sea
// floor, which makes the mesher build a wall around the map edge instead of
// leaving a hole for the camera to see in through.
func (m *Map) Elevation(a hex.Axial) int {
	t := m.At(a)
	if t == nil {
		return 0
	}
	return int(t.Elevation)
}

// SurfaceY is the world Y of a tile walkable top. Sea tiles report the sea
// floor; the methane surface above them is a separate mesh.
func (m *Map) SurfaceY(a hex.Axial) float32 {
	return SurfaceYAt(m.Elevation(a))
}

// IsSea reports whether a tile is under methane.
func (m *Map) IsSea(a hex.Axial) bool {
	t := m.At(a)
	return t != nil && t.Terrain == Sea
}

// Center is the world-space XZ of a tile center.
func (m *Map) Center(a hex.Axial) (x, z float32) { return Layout.Center(a) }

// Bounds returns the world-space rectangle the map covers, padded by one tile
// so geometry at the edge still falls inside it.
func (m *Map) Bounds() (minX, minZ, maxX, maxZ float32) {
	minX, minZ = Layout.Center(FromOffset(0, 0))
	maxX, maxZ = minX, minZ
	// Row 0 and the last row bound Z, but which column reaches furthest
	// depends on the even-q stagger, so sweep the four edges rather than
	// guessing the extreme corner.
	for col := 0; col < m.Cols; col++ {
		for _, row := range [2]int{0, m.Rows - 1} {
			x, z := Layout.Center(FromOffset(col, row))
			minX, maxX = min(minX, x), max(maxX, x)
			minZ, maxZ = min(minZ, z), max(maxZ, z)
		}
	}
	pad := Layout.Width()
	return minX - pad, minZ - pad, maxX + pad, maxZ + pad
}

// Each visits every tile on the map with its axial coordinate.
func (m *Map) Each(fn func(a hex.Axial, t *Tile)) {
	for row := 0; row < m.Rows; row++ {
		for col := 0; col < m.Cols; col++ {
			fn(FromOffset(col, row), &m.Tiles[row*m.Cols+col])
		}
	}
}
