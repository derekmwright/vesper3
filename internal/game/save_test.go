package game

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/world"
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

	// A mine needs a habitat within reach, which every real save has and this
	// fixture did not.
	home := world.FromOffset(7, 6)
	g.Map.At(home).Terrain = world.Regolith
	if err := g.Colony.Found(g.Map, colony.Habitat, home); err != nil {
		t.Fatal(err)
	}

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

// A tiered building and a vespite stock have to come back, and a save written
// before either existed has to keep loading.
func TestSaveRoundTripsTiersAndVespite(t *testing.T) {
	g := savableGame(t)

	home := world.FromOffset(7, 6)
	g.Map.At(home).Terrain = world.Regolith
	if err := g.Colony.Found(g.Map, colony.Habitat, home); err != nil {
		t.Fatal(err)
	}
	site := world.FromOffset(6, 6)
	g.Map.At(site).Terrain = world.Dunes
	if err := g.Colony.Place(g.Map, colony.Mine, site); err != nil {
		t.Fatal(err)
	}

	g.Colony.Iron, g.Colony.Vespite = 100000, 500
	if err := g.Colony.Upgrade(site); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if err := g.Colony.Upgrade(site); err != nil {
		t.Fatalf("second upgrade: %v", err)
	}
	wantVespite := g.Colony.Vespite

	blob, err := g.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeSave(blob, g.Map.Cols, g.Map.Rows)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got.Colony.Reindex()

	b, ok := got.Colony.At(site)
	if !ok {
		t.Fatal("the mine did not come back")
	}
	if b.Tier() != colony.MaxTier {
		t.Errorf("mine reloaded at tier %d, want %d", b.Tier(), colony.MaxTier)
	}
	if got.Colony.Vespite != wantVespite {
		t.Errorf("vespite reloaded as %.2f, want %.2f", got.Colony.Vespite, wantVespite)
	}
}

// The reason saveVersion did not need bumping: a file from before tiers has
// no tier field at all, and the building it describes has to load as tier 1
// rather than as a building that scales by nothing.
func TestASaveWithoutTiersLoadsAsTierOne(t *testing.T) {
	g := savableGame(t)
	home := world.FromOffset(7, 6)
	g.Map.At(home).Terrain = world.Regolith
	if err := g.Colony.Found(g.Map, colony.Habitat, home); err != nil {
		t.Fatal(err)
	}
	// Set both fields to something a zero value could not be mistaken for,
	// so that stripping them has to actually do something. Deleting a key
	// that was never there would make this test pass without testing
	// anything, which is exactly how a renamed field would slip through.
	g.Colony.Iron, g.Colony.Vespite = 100000, 77
	if err := g.Colony.Upgrade(home); err != nil {
		t.Fatal(err)
	}

	blob, err := g.encode()
	if err != nil {
		t.Fatal(err)
	}

	// Strip the fields a pre-tier build would never have written.
	var raw map[string]any
	if err := json.Unmarshal(blob, &raw); err != nil {
		t.Fatal(err)
	}
	col := raw["colony"].(map[string]any)
	if _, ok := col["Vespite"]; !ok {
		t.Fatal(`no "Vespite" key in the save: this test is stripping nothing`)
	}
	delete(col, "Vespite")
	for _, b := range col["Buildings"].([]any) {
		bm := b.(map[string]any)
		if _, ok := bm["TierLevel"]; !ok {
			t.Fatal(`no "TierLevel" key in a saved building`)
		}
		delete(bm, "TierLevel")
	}
	stripped, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeSave(stripped, g.Map.Cols, g.Map.Rows)
	if err != nil {
		t.Fatalf("a save without tiers would not load: %v", err)
	}
	got.Colony.Reindex()
	b, ok := got.Colony.At(home)
	if !ok {
		t.Fatal("the habitat did not come back")
	}
	if b.Tier() != 1 {
		t.Errorf("an untiered building loaded at tier %d, want 1", b.Tier())
	}
	if got.Colony.Vespite != 0 {
		t.Errorf("vespite loaded as %.2f, want 0", got.Colony.Vespite)
	}
}
