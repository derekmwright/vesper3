package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/world"
)

// saveVersion guards against loading a file written by a build whose map or
// economy meant something different. Bumping it is cheaper than debugging a
// colony that loads with plausible but wrong numbers.
// Version 2 split ore into iron and crystal, renamed biomass to food, and
// moved the mine's product onto the building. A version 1 file would load with
// every stock at zero and every mine producing nothing, which is exactly the
// "plausible but wrong numbers" case this guard exists for.
const saveVersion = 2

// savedGame is the whole game state on disk.
//
// The tiles are stored rather than regenerated from the seed. Regenerating
// would be smaller, but terraforming edits the map, and a save that quietly
// undid the player's excavation on load would be worse than a larger file.
type savedGame struct {
	Version int            `json:"version"`
	Seed    int64          `json:"seed"`
	Cols    int            `json:"cols"`
	Rows    int            `json:"rows"`
	Tiles   []world.Tile   `json:"tiles"`
	Colony  *colony.Colony `json:"colony"`
}

// encode serialises the current state. Split out from Save so the round trip
// can be tested without a window, which is where the interesting failures are
// — a dropped field looks fine until someone reloads a colony.
func (g *Game) encode() ([]byte, error) {
	return json.Marshal(savedGame{
		Version: saveVersion,
		Seed:    g.Map.Seed,
		Cols:    g.Map.Cols,
		Rows:    g.Map.Rows,
		Tiles:   g.Map.Tiles,
		Colony:  g.Colony,
	})
}

// decodeSave parses and validates a save against the dimensions of the
// session that is loading it.
func decodeSave(blob []byte, cols, rows int) (*savedGame, error) {
	var s savedGame
	if err := json.Unmarshal(blob, &s); err != nil {
		return nil, fmt.Errorf("decode save: %w", err)
	}
	if s.Version != saveVersion {
		return nil, fmt.Errorf("save is version %d, this build reads version %d", s.Version, saveVersion)
	}
	if s.Cols != cols || s.Rows != rows {
		// The chunk meshes were allocated for the current dimensions, so a
		// differently sized map would need them rebuilt from scratch. Refuse
		// clearly instead of half-loading.
		return nil, fmt.Errorf("save is %dx%d, this session is %dx%d; restart with -cols %d -rows %d",
			s.Cols, s.Rows, cols, rows, s.Cols, s.Rows)
	}
	if len(s.Tiles) != s.Cols*s.Rows {
		return nil, fmt.Errorf("save holds %d tiles, want %d", len(s.Tiles), s.Cols*s.Rows)
	}
	if s.Colony == nil {
		return nil, fmt.Errorf("save has no colony")
	}
	return &s, nil
}

// Save writes the game to the configured path.
func (g *Game) Save() error {
	path := g.savePath()

	blob, err := g.encode()
	if err != nil {
		return fmt.Errorf("encode save: %w", err)
	}

	// Write to a temporary file and rename over the target, so a crash
	// halfway through cannot destroy a good save.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return fmt.Errorf("write save: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace save: %w", err)
	}
	return nil
}

// Load replaces the current game with the one on disk and rebuilds everything
// on the GPU to match.
func (g *Game) Load(e *glyph.Engine) error {
	blob, err := os.ReadFile(g.savePath())
	if err != nil {
		return fmt.Errorf("read save: %w", err)
	}

	s, err := decodeSave(blob, g.Map.Cols, g.Map.Rows)
	if err != nil {
		return err
	}

	g.Map.Seed = s.Seed
	copy(g.Map.Tiles, s.Tiles)

	g.Colony = s.Colony
	g.Colony.Reindex()

	// Despawn the structures of the outgoing colony before the incoming one
	// spawns its own, or the two sets are drawn on top of each other.
	g.scene.clearBuildings(e)
	// Rebuilt directly rather than by publishing a StructurePlaced for each
	// one, and the distinction is worth being deliberate about: loading is not
	// a sequence of placements. Nothing was built, nothing was paid for, and
	// announcing forty structures going up would be false. An event is a fact
	// about something that happened; restoring a saved world is the world
	// being replaced, which the load itself already reports.
	for _, b := range g.Colony.Buildings {
		g.spawnBuilding(e, b.Kind, b.At)
	}

	g.rebuildAllChunks(e)
	e.RebuildStatics()

	return nil
}

// savePath resolves the configured save location, defaulting next to the
// working directory so a player can find it.
func (g *Game) savePath() string {
	if g.cfg.SavePath != "" {
		return g.cfg.SavePath
	}
	return filepath.Join(".", "colony.save.json")
}
