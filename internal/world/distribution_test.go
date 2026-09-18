package world

import (
	"fmt"
	"strings"
	"testing"
)

// Every terrain has to actually occur, and occur in roughly the proportion it
// was tuned to. A building that needs Vent under it is unbuildable if the
// generator stops producing vents, and nothing else in the suite would notice:
// the map would still be an island, still be habitable, still have a start
// site. This is the test that fails when a noise constant is changed by feel.
//
// The bounds are wide on purpose. They are there to catch a terrain
// disappearing or swamping the map, not to freeze the exact tuning.
func TestTerrainDistribution(t *testing.T) {
	const (
		seeds = 12
		size  = 48
	)

	counts := map[Terrain]int{}
	land := 0
	ventedMaps := 0
	crystalMaps := 0
	iceMaps := 0

	for seed := int64(1); seed <= seeds; seed++ {
		m, err := NewMap(size, size, seed)
		if err != nil {
			t.Fatal(err)
		}
		Generate(m)

		vents, crystals, ices := 0, 0, 0
		for _, tile := range m.Tiles {
			counts[tile.Terrain]++
			if tile.Terrain != Sea {
				land++
			}
			switch tile.Terrain {
			case Vent:
				vents++
			case Crystal:
				crystals++
			case Ice:
				ices++
			}
		}
		if vents > 0 {
			ventedMaps++
		}
		if crystals > 0 {
			crystalMaps++
		}
		if ices > 0 {
			iceMaps++
		}
	}

	// Geothermal is the only night-proof power source, so a map without a
	// vent is a map with a much harder mid-game. A few are acceptable; a
	// majority are not.
	if ventedMaps < seeds*3/4 {
		t.Errorf("only %d of %d maps have a thermal vent", ventedMaps, seeds)
	}

	// Ice is the same requirement and was the harder failure. Every map has
	// to have some, because it is the only ground an Ice Extractor can stand
	// on and water is what a colony runs out of first.
	//
	// The aggregate bound below passed for a long time while this did not:
	// measured over forty seeds, eighteen maps had no ice at all and
	// twenty-three had none within reach of the landing site. A handful of
	// ice-rich maps carried the average for all the ones with nothing, which
	// is exactly the failure a per-map check catches and a distribution does
	// not.
	if iceMaps != seeds {
		t.Errorf("only %d of %d maps have ice; an extractor needs somewhere to stand", iceMaps, seeds)
	}

	// Crystal is the harder requirement: it is the only source of the ore
	// batteries and geothermal plants are priced in, and unlike power there
	// is no second way to get it. A map with no crystal flat is a map whose
	// colony can never store energy, so every map has to have one.
	if crystalMaps != seeds {
		t.Errorf("only %d of %d maps have a crystal flat; crystal has no alternative source", crystalMaps, seeds)
	}

	// Fractions of land, not of the whole map, so a change in sea level does
	// not move every bound at once.
	want := map[Terrain][2]float64{
		Regolith: {30, 70},
		Dunes:    {8, 32},
		Lichen:   {8, 32},
		Basalt:   {1.5, 15},
		Crystal:  {0.5, 8},
		Ice:      {0.2, 6},
		Vent:     {0.1, 2},
	}
	for terr, bounds := range want {
		pct := float64(counts[terr]) / float64(land) * 100
		if pct < bounds[0] || pct > bounds[1] {
			t.Errorf("%v is %.2f%% of land, want %.1f-%.1f%%", terr, pct, bounds[0], bounds[1])
		}
	}
}

// A picture of one map, for when a number above moves and the question is
// what it looks like. Logged only, and only under -v.
func TestPreviewMap(t *testing.T) {
	m, err := NewMap(48, 48, 20260916)
	if err != nil {
		t.Fatal(err)
	}
	Generate(m)

	glyphs := map[Terrain]rune{
		Sea: '.', Regolith: 'r', Dunes: 'd', Lichen: 'L',
		Basalt: 'B', Crystal: 'C', Ice: 'I', Vent: 'V',
	}
	site := m.StartSite()
	sc, sr := Offset(site)

	var b strings.Builder
	for row := 0; row < m.Rows; row++ {
		for col := 0; col < m.Cols; col++ {
			if col == sc && row == sr {
				b.WriteRune('@')
				continue
			}
			b.WriteRune(glyphs[m.AtOffset(col, row).Terrain])
		}
		b.WriteByte('\n')
	}

	var elevHist [MaxElevation + 1]int
	for _, tile := range m.Tiles {
		elevHist[tile.Elevation]++
	}
	for e, n := range elevHist {
		b.WriteString(fmt.Sprintf("elev %2d: %4d\n", e, n))
	}
	t.Log("\n" + b.String())
}
