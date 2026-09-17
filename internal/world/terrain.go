package world

// Terrain is what a tile is made of. It decides the tile colour, whether
// anything can be built on it, and which extractors will work there.
type Terrain uint8

const (
	Regolith Terrain = iota // dust and gravel: the default ground
	Dunes                   // wind-piled ferrous sand
	Lichen                  // the only native life worth the name
	Basalt                  // old lava rock, shot through with ore
	Crystal                 // silicate flats that grew rather than settled
	Ice                     // water ice, at altitude and at the poles
	Vent                    // a fissure venting heat from below
	Sea                     // liquid methane

	terrainCount
)

// Ore is what a mine sunk into this ground brings up. It is an enum rather
// than a flag because the two are not interchangeable: iron is what everything
// is built out of, and crystal is what the things that store or switch energy
// need. A tile that yields neither cannot be mined at all.
type Ore uint8

const (
	OreNone Ore = iota
	OreIron
	OreCrystal
)

// String names an ore in the terms the player sees.
func (o Ore) String() string {
	switch o {
	case OreIron:
		return "iron"
	case OreCrystal:
		return "crystal"
	}
	return "nothing"
}

// TerrainInfo is everything the game needs to know about a kind of ground.
type TerrainInfo struct {
	Name string

	// Color is the tile base albedo. Tops use it directly; cliff faces use a
	// darkened copy, which is what makes elevation read at a glance.
	Color [3]float32

	// Buildable is whether a structure can stand here at all. Sea cannot, and
	// a vent is too unstable for anything but the plant that taps it.
	Buildable bool

	// Ore, Frozen, Geothermal and Fertile gate the extractors. A mine wants
	// ground that yields an ore, a condenser wants Frozen, and so on; the
	// catalog in package colony reads these rather than switching on Terrain
	// itself.
	//
	// Ore also decides *what* a mine on this ground produces, which is what
	// makes siting a mine a decision rather than a formality: the same
	// building is an iron mine on ferrous dunes and a crystal mine on a
	// silicate flat, and is not worth building anywhere else.
	Ore        Ore
	Frozen     bool
	Geothermal bool
	Fertile    bool
}

var terrainTable = [terrainCount]TerrainInfo{
	Regolith: {
		Name:      "Regolith",
		Color:     [3]float32{0.52, 0.46, 0.38},
		Buildable: true,
	},
	Dunes: {
		Name:      "Ferrous Dunes",
		Color:     [3]float32{0.64, 0.38, 0.22},
		Buildable: true,
		Ore:       OreIron,
	},
	Lichen: {
		Name:      "Lichen Plain",
		Color:     [3]float32{0.28, 0.48, 0.35},
		Buildable: true,
		Fertile:   true,
	},
	Basalt: {
		// Basalt used to yield ore, and better ore than the dunes. That made
		// it the obvious place for a mine and the obvious place for
		// everything else too, since it is the most common high ground on the
		// map — so siting a mine was never a decision. It is building room
		// now, and nothing else.
		Name:      "Basalt Ridge",
		Color:     [3]float32{0.30, 0.29, 0.34},
		Buildable: true,
	},
	Crystal: {
		Name:      "Crystal Flat",
		Color:     [3]float32{0.62, 0.58, 0.78},
		Buildable: true,
		Ore:       OreCrystal,
	},
	Ice: {
		Name:      "Ice Sheet",
		Color:     [3]float32{0.80, 0.86, 0.92},
		Buildable: true,
		Frozen:    true,
	},
	Vent: {
		Name:       "Thermal Vent",
		Color:      [3]float32{0.42, 0.22, 0.17},
		Buildable:  false,
		Geothermal: true,
	},
	Sea: {
		Name:      "Methane Sea",
		Color:     [3]float32{0.09, 0.24, 0.28},
		Buildable: false,
	},
}

// TerrainCount is how many terrains there are, for callers that need to walk
// the set — the tooltip reads every one to find where a structure yields best,
// rather than restating the table and drifting from it.
func TerrainCount() Terrain { return terrainCount }

// Info describes a terrain. An unknown value returns the Regolith entry
// rather than panicking, because a corrupt save should show the player a dull
// tile, not take the process down.
func (t Terrain) Info() TerrainInfo {
	if t >= terrainCount {
		return terrainTable[Regolith]
	}
	return terrainTable[t]
}

// String makes Terrain printable in HUD lines and test failures.
func (t Terrain) String() string { return t.Info().Name }
