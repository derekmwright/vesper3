package game

import (
	"strings"
	"testing"

	"github.com/derekmwright/worldbuild/internal/colony"
	"github.com/derekmwright/worldbuild/internal/hex"
	"github.com/derekmwright/worldbuild/internal/world"
)

// A save has to come back carrying everything the player earned and
// everything they changed. A silently dropped field looks fine until someone
// reloads and finds their mountain flattened or their ore gone.
func TestSaveRoundTripsMapAndColony(t *testing.T) {
	g := savableGame(t)

	// Terraform a tile so the saved map differs from what the seed generates,
	// which is the case regenerating-from-seed would get wrong.
	dug := world.FromOffset(5, 5)
	g.Map.At(dug).Elevation = 11
	g.Map.At(dug).Terrain = world.Crystal

	site := world.FromOffset(6, 6)
	g.Map.At(site).Terrain = world.Dunes
	if err := g.Colony.Place(g.Map, colony.Mine, site); err != nil {
		t.Fatal(err)
	}
	g.Colony.Iron = 137.5
	g.Colony.Water = 42.25
	g.Colony.Food = 61
	g.Colony.Colonists = 5.5

	blob, err := g.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := decodeSave(blob, g.Map.Cols, g.Map.Rows)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Seed != g.Map.Seed {
		t.Errorf("seed %d, want %d", got.Seed, g.Map.Seed)
	}
	if len(got.Tiles) != len(g.Map.Tiles) {
		t.Fatalf("%d tiles, want %d", len(got.Tiles), len(g.Map.Tiles))
	}
	for i := range g.Map.Tiles {
		if got.Tiles[i] != g.Map.Tiles[i] {
			t.Fatalf("tile %d is %+v, want %+v", i, got.Tiles[i], g.Map.Tiles[i])
		}
	}

	c := got.Colony
	if c.Iron != 137.5 || c.Water != 42.25 || c.Food != 61 || c.Colonists != 5.5 {
		t.Errorf("stores came back as iron %.2f water %.2f food %.2f colonists %.2f",
			c.Iron, c.Water, c.Food, c.Colonists)
	}

	c.Reindex()
	b, ok := c.At(site)
	if !ok {
		t.Fatal("the mine did not survive the round trip")
	}
	if b.Kind != colony.Mine {
		t.Errorf("building is %v, want Mine", b.Kind)
	}
	// Yield is resolved at placement and stored, so it has to persist rather
	// than be recomputed against a possibly retuned table.
	if want := colony.Yield(colony.Mine, world.Basalt); b.Yield != want {
		t.Errorf("yield %.2f, want %.2f", b.Yield, want)
	}
}

func TestDecodeSaveRejectsBadFiles(t *testing.T) {
	g := savableGame(t)
	good, err := g.encode()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		blob       []byte
		cols, rows int
		wantErr    string
	}{
		{"not json", []byte("{definitely not"), g.Map.Cols, g.Map.Rows, "decode save"},
		{"wrong version", []byte(`{"version":99,"cols":16,"rows":16}`), 16, 16, "version"},
		{"wrong size", good, g.Map.Cols + 8, g.Map.Rows, "restart with"},
		{"no colony", []byte(`{"version":2,"cols":2,"rows":2,"tiles":[{},{},{},{}]}`), 2, 2, "no colony"},
		{"short tile array", []byte(`{"version":2,"cols":16,"rows":16,"tiles":[{}],"colony":{}}`), 16, 16, "tiles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeSave(tc.blob, tc.cols, tc.rows)
			if err == nil {
				t.Fatal("decodeSave accepted it")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// A good save must survive being decoded against the session that wrote it,
// which is the case the rejection table above must not catch by accident.
func TestDecodeSaveAcceptsItsOwnOutput(t *testing.T) {
	g := savableGame(t)
	blob, err := g.encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSave(blob, g.Map.Cols, g.Map.Rows); err != nil {
		t.Fatalf("a freshly written save was rejected: %v", err)
	}
}

func savableGame(t *testing.T) *Game {
	t.Helper()
	m, err := world.NewMap(16, 16, 4242)
	if err != nil {
		t.Fatal(err)
	}
	world.Generate(m)
	m.Each(func(_ hex.Axial, tile *world.Tile) {
		if tile.Terrain == world.Sea {
			return
		}
	})

	g := New(Config{Seed: 4242, Cols: 16, Rows: 16})
	g.Map = m
	g.Colony = colony.New()
	return g
}
