package meshgen

import (
	"testing"

	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
)

// The engine's UpdateMeshData clamps to the buffer it was given and returns
// no error, so a chunk that outgrew ChunkCapacity would come back with its
// last tiles silently missing. Nothing else in this project would notice, so
// this is the check that has to.
//
// The worst case is not a generated map: it is ground where every tile is
// higher than all six of its neighbours, which is the only arrangement that
// makes every tile pay for six cliff walls.
func TestWorstCaseChunkFitsInCapacity(t *testing.T) {
	m, err := world.NewMap(32, 32, 1)
	if err != nil {
		t.Fatal(err)
	}

	// A checkerboard of peaks and pits. Offset parity alternates in both
	// axes, so no tile shares a height with any neighbour.
	m.Each(func(a hex.Axial, tile *world.Tile) {
		col, row := world.Offset(a)
		tile.Terrain = world.Regolith
		if (col+row)%2 == 0 {
			tile.Elevation = world.MaxElevation
		} else {
			tile.Elevation = world.SeaLevel + 1
		}
	})

	maxV, maxI := ChunkCapacity()
	nx, ny := ChunkGrid(m)

	worstV, worstI := 0, 0
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			verts, idx := BuildChunk(m, ChunkID{cx, cy}, nil, nil)
			worstV = max(worstV, len(verts))
			worstI = max(worstI, len(idx))

			if len(verts) > maxV {
				t.Fatalf("chunk %d,%d built %d vertices, capacity is %d", cx, cy, len(verts), maxV)
			}
			if len(idx) > maxI {
				t.Fatalf("chunk %d,%d built %d indices, capacity is %d", cx, cy, len(idx), maxI)
			}
		}
	}

	t.Logf("worst case: %d/%d vertices, %d/%d indices", worstV, maxV, worstI, maxI)
}

// The capacity above is per-tile times tiles-per-chunk, and it is only sound
// if the per-tile figure is what a tile can actually cost. No arrangement
// makes every tile a local maximum — a hex grid is 3-colourable, not 2 — so
// the chunk-level test can never reach the bound, and would pass just as
// happily if the bound were twice too large.
//
// This measures a single tile instead, by differencing a flat chunk against
// one with exactly one raised tile in it.
func TestPerTileBoundIsWhatATileActuallyCosts(t *testing.T) {
	m, err := world.NewMap(32, 32, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		tile.Terrain = world.Regolith
		tile.Elevation = world.SeaLevel + 1
	})

	// Chunk 1,1 is interior, so the map edge contributes no walls of its own.
	const chunkTiles = ChunkCols * ChunkRows
	flatV, flatI := BuildChunk(m, ChunkID{1, 1}, nil, nil)

	capVerts := len(flatV) / chunkTiles
	capIdx := len(flatI) / chunkTiles
	if len(flatV) != capVerts*chunkTiles || len(flatI) != capIdx*chunkTiles {
		t.Fatalf("flat chunk is not uniform: %d verts, %d idx over %d tiles",
			len(flatV), len(flatI), chunkTiles)
	}

	// Raise one interior tile above all six neighbours: the most walls a
	// single tile can ever have.
	m.At(world.FromOffset(12, 12)).Elevation = world.MaxElevation
	peakV, peakI := BuildChunk(m, ChunkID{1, 1}, nil, nil)

	gotVerts := len(peakV) - len(flatV) + capVerts
	gotIdx := len(peakI) - len(flatI) + capIdx

	if gotVerts != MaxVertsPerTile {
		t.Errorf("a fully walled tile costs %d vertices, MaxVertsPerTile says %d",
			gotVerts, MaxVertsPerTile)
	}
	if gotIdx != MaxIndicesPerTile {
		t.Errorf("a fully walled tile costs %d indices, MaxIndicesPerTile says %d",
			gotIdx, MaxIndicesPerTile)
	}
}
