package game

import (
	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
)

// The demo colony: a few structures put down around the landing site, for a
// screenshot that shows the game doing something.
//
// It exists because this project is verified by capture — `-frames N
// -screenshot out.png` with the camera and cursor pinned — and a fresh world
// has two buildings in it. Checking that a battery bank's charge strips light
// up, or that a greenhouse glows at dusk, meant playing far enough to build
// one and then hoping to catch it, which is not a check anyone can repeat.
//
// It is deliberately not a save file. A save pins the map as well, so every
// capture would be of the same terrain forever and a worldgen change would
// invalidate the lot. This places structures on whatever ground is actually
// there, so `-demo -seed N` composes with every other flag.
//
// Nothing here is reachable from play. It is a flag, and the colony it builds
// is free — Found rather than Place — because a capture should not also be a
// test of whether the opening stockpile stretches this far.
func (g *Game) foundDemoColony(e *glyph.Engine) {
	site := g.Map.StartSite()

	// What to put down, in the order it is tried. Power first so the rest of
	// it is lit and running by the time the frame is captured.
	wanted := []colony.Kind{
		colony.SolarArray, colony.SolarArray, colony.SolarArray,
		colony.Battery, colony.Battery,
		colony.Greenhouse, colony.Habitat,
		colony.Mine, colony.Extractor, colony.Condenser, colony.Geothermal,

		// The two coastal structures go last because they are the ones most
		// likely to find no site: a map with no shoreline near the landing
		// site simply captures without them.
		colony.Methane, colony.Synthesizer,
	}

	// Rings outward from the landing site. Each structure takes the first tile
	// that will have it, so the ones with siting rules — a mine wants ore, a
	// plant wants a vent — land wherever the map happens to offer one, and
	// simply do not appear when it offers none.
	for _, k := range wanted {
		// Six rings covers ore, ice and vents, which worldgen scatters near
		// the landing site. Coastline is the exception: every map is an
		// island, so there is always a shore, but it can be most of a
		// continent away and a capture that silently omits two structures is
		// worse than one that reaches for them.
		radius := 6
		if colony.Of(k).Needs == colony.NeedsCoast {
			radius = 16
		}
		if at, ok := g.nearestSiteFor(k, site, radius); ok {
			if err := g.Colony.Found(g.Map, k, at); err == nil {
				emit(g, StructurePlaced{Kind: k, At: at})
			}
		}
	}

	// A part-charged bank, because both extremes are uninformative: a flat one
	// looks broken and a full one cannot show which way the strips fill.
	g.Colony.Charge = 0.62 * colony.Of(colony.Battery).PowerStore *
		float64(g.Colony.Count(colony.Battery))

	// Enough colonists to be eating, so the food row is not a flat line.
	g.Colony.Colonists = 4

	// A part-built vespite stock and a mine standing at its top tier, so a
	// capture shows both the resource row and the upgrade art rather than a
	// colony that has never used either.
	g.Colony.Vespite = 0.4 * colony.Of(colony.Synthesizer).VespiteStore *
		float64(g.Colony.Count(colony.Synthesizer))
	for _, b := range g.Colony.Buildings {
		if b.Kind == colony.Mine {
			g.Colony.SetTier(b.At, colony.MaxTier)
		}
	}
}

// nearestSiteFor finds the closest free tile to origin that will take a
// structure, searching out to radius rings.
//
// It asks CanFound rather than CanPlace because this places by founding. The
// demo is a capture, not a colony: it wants a vent and an ice sheet in shot
// even when they are further from the landing site than a working colony
// could staff, and it should not be gated on an opening stockpile either.
func (g *Game) nearestSiteFor(k colony.Kind, origin hex.Axial, radius int) (hex.Axial, bool) {
	for r := 1; r <= radius; r++ {
		for _, at := range ring(origin, r) {
			if g.Colony.CanFound(g.Map, k, at) == nil {
				return at, true
			}
		}
	}
	return hex.Axial{}, false
}

// ring returns the tiles exactly r steps from centre, walking the hexagon
// rather than scanning a box and filtering — the standard traversal, and the
// reason the direction order in package hex is worth getting right.
func ring(centre hex.Axial, r int) []hex.Axial {
	if r <= 0 {
		return []hex.Axial{centre}
	}

	// Start r steps along one direction, then walk r steps along each of the
	// six in turn, which closes the loop exactly.
	at := centre
	for range r {
		at = at.Neighbor(4)
	}

	out := make([]hex.Axial, 0, 6*r)
	for d := range 6 {
		for range r {
			out = append(out, at)
			at = at.Neighbor(d)
		}
	}
	return out
}
