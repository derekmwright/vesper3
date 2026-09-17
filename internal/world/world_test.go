package world

import (
	"testing"

	"github.com/derekmwright/worldbuild/internal/hex"
)

func TestOffsetRoundTrip(t *testing.T) {
	for col := 0; col < 40; col++ {
		for row := 0; row < 40; row++ {
			a := FromOffset(col, row)
			gotCol, gotRow := Offset(a)
			if gotCol != col || gotRow != row {
				t.Fatalf("(%d,%d) -> %v -> (%d,%d)", col, row, a, gotCol, gotRow)
			}
		}
	}
}

// Storage must not alias: two different offsets that mapped to the same axial
// coordinate would silently share a tile.
func TestOffsetIsInjective(t *testing.T) {
	seen := make(map[hex.Axial][2]int)
	for col := 0; col < 32; col++ {
		for row := 0; row < 32; row++ {
			a := FromOffset(col, row)
			if prev, dup := seen[a]; dup {
				t.Fatalf("%v is both (%d,%d) and (%d,%d)", a, prev[0], prev[1], col, row)
			}
			seen[a] = [2]int{col, row}
		}
	}
}

// Neighbouring tiles in the grid must be neighbours on the map, or the mesher
// builds walls against the wrong tiles.
func TestAdjacentOffsetsAreHexNeighbours(t *testing.T) {
	m := newTestMap(t)
	for row := 1; row < m.Rows-1; row++ {
		for col := 1; col < m.Cols-1; col++ {
			a := FromOffset(col, row)
			count := 0
			for d := 0; d < 6; d++ {
				if m.Contains(a.Neighbor(d)) {
					count++
				}
			}
			if count != 6 {
				t.Fatalf("interior tile (%d,%d) has %d on-map neighbours, want 6", col, row, count)
			}
		}
	}
}

// The same seed has to give the same planet, or a saved game cannot be
// reopened and a bug cannot be reproduced from a seed.
func TestGenerateIsDeterministic(t *testing.T) {
	a := newTestMap(t)
	b := newTestMap(t)
	for i := range a.Tiles {
		if a.Tiles[i] != b.Tiles[i] {
			t.Fatalf("tile %d differs: %+v vs %+v", i, a.Tiles[i], b.Tiles[i])
		}
	}
}

func TestDifferentSeedsGiveDifferentPlanets(t *testing.T) {
	a, err := NewMap(48, 48, 1)
	if err != nil {
		t.Fatal(err)
	}
	Generate(a)
	b, err := NewMap(48, 48, 2)
	if err != nil {
		t.Fatal(err)
	}
	Generate(b)

	same := 0
	for i := range a.Tiles {
		if a.Tiles[i] == b.Tiles[i] {
			same++
		}
	}
	// Some agreement is expected — both maps are mostly sea at the edges.
	if same == len(a.Tiles) {
		t.Fatal("seeds 1 and 2 produced identical maps")
	}
}

// A map that is all water, or all mountain, is generated but not playable.
// This is the guard on the generator's tuning, and it is the test that fails
// when someone changes a noise constant by feel.
func TestGeneratedMapsAreHabitable(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		m, err := NewMap(48, 48, seed)
		if err != nil {
			t.Fatal(err)
		}
		Generate(m)

		var land, sea, buildable int
		m.Each(func(_ hex.Axial, tile *Tile) {
			if tile.Terrain == Sea {
				sea++
			} else {
				land++
			}
			if tile.Terrain.Info().Buildable {
				buildable++
			}
		})

		total := len(m.Tiles)
		landFrac := float64(land) / float64(total)
		if landFrac < 0.20 || landFrac > 0.75 {
			t.Errorf("seed %d: land is %.1f%% of the map, want 20-75%%", seed, landFrac*100)
		}
		if sea == 0 {
			t.Errorf("seed %d: no sea at all", seed)
		}
		if float64(buildable)/float64(total) < 0.15 {
			t.Errorf("seed %d: only %.1f%% buildable", seed, float64(buildable)/float64(total)*100)
		}
	}
}

// The edge of the array must be under water, or the continent is cut off
// square and the player can see the map end.
func TestMapEdgesAreSea(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		m, err := NewMap(48, 48, seed)
		if err != nil {
			t.Fatal(err)
		}
		Generate(m)

		for col := 0; col < m.Cols; col++ {
			for _, row := range []int{0, m.Rows - 1} {
				if got := m.AtOffset(col, row).Terrain; got != Sea {
					t.Errorf("seed %d: edge tile (%d,%d) is %v, want Sea", seed, col, row, got)
				}
			}
		}
		for row := 0; row < m.Rows; row++ {
			for _, col := range []int{0, m.Cols - 1} {
				if got := m.AtOffset(col, row).Terrain; got != Sea {
					t.Errorf("seed %d: edge tile (%d,%d) is %v, want Sea", seed, col, row, got)
				}
			}
		}
	}
}

func TestStartSiteIsBuildableAndFlat(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		m, err := NewMap(48, 48, seed)
		if err != nil {
			t.Fatal(err)
		}
		Generate(m)

		site := m.StartSite()
		tile := m.At(site)
		if tile == nil {
			t.Fatalf("seed %d: start site %v is off the map", seed, site)
		}
		if !tile.Terrain.Info().Buildable {
			t.Errorf("seed %d: start site is %v, which is not buildable", seed, tile.Terrain)
		}

		// There has to be somewhere to expand into.
		room := 0
		for _, n := range hex.Area(site, 2) {
			if nt := m.At(n); nt != nil && nt.Terrain.Info().Buildable {
				room++
			}
		}
		if room < 8 {
			t.Errorf("seed %d: only %d buildable tiles within 2 of the start site", seed, room)
		}
	}
}

func TestNewMapRejectsDegenerateSizes(t *testing.T) {
	if _, err := NewMap(1, 40, 0); err == nil {
		t.Error("NewMap(1,40) succeeded, want an error")
	}
	if _, err := NewMap(40, 0, 0); err == nil {
		t.Error("NewMap(40,0) succeeded, want an error")
	}
}

func newTestMap(t *testing.T) *Map {
	t.Helper()
	m, err := NewMap(48, 48, 20260916)
	if err != nil {
		t.Fatal(err)
	}
	Generate(m)
	return m
}
