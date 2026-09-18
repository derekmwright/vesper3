package world

import (
	"math"
	"sort"

	"github.com/derekmwright/vesper3/internal/hex"
)

// Generate fills a map with a continent surrounded by methane sea.
//
// The shape comes from three noise fields and one falloff. Elevation is fbm
// blended toward ridged noise at altitude, so lowlands roll and highlands
// form spines rather than domes; a radial falloff pulls the edges under the
// sea, which is what makes the map an island instead of a rectangle that
// stops. Moisture and a rare vent field then decide what the ground is made
// of.
//
// Everything is driven by m.Seed, so the same seed is the same planet.
func Generate(m *Map) {
	elevNoise := newPerlin(m.Seed)
	ridgeNoise := newPerlin(m.Seed ^ 0x5DEECE66D)
	moistNoise := newPerlin(m.Seed + 977)
	ventNoise := newPerlin(m.Seed * 31)
	coastNoise := newPerlin(m.Seed - 4451)

	// Noise is sampled in tile space rather than world space so the terrain
	// does not stretch along Z with the hex stagger.
	const (
		elevScale  = 0.055
		moistScale = 0.085
		ventScale  = 0.21
		coastScale = 0.09

		// Raw fbm clusters around its midpoint: five octaves of it put almost
		// every tile between 0.35 and 0.65, which generated a continent that
		// was all one biome at all one height. Stretching both fields about
		// their midpoint is what produces peaks, inlets, and ground that is
		// not everywhere the same. Measured over seeds 1-12 on a 48x48 map,
		// elevation contrast 1.85 moves tiles at or above elevation 10 from
		// 0% of the land to about 12%, and moisture contrast 2.1 moves Lichen
		// and Dunes together from under 4% to about 30%.
		elevContrast  = 1.85
		moistContrast = 2.1
	)

	for row := 0; row < m.Rows; row++ {
		for col := 0; col < m.Cols; col++ {
			x, y := float64(col), float64(row)

			base := elevNoise.fbm(x*elevScale, y*elevScale, 5, 2.0, 0.5)*0.5 + 0.5
			ridge := ridgeNoise.ridged(x*elevScale*0.8, y*elevScale*0.8, 4)*0.5 + 0.5

			// Blend toward ridges only where the base is already high, so the
			// spines sit on the mountains instead of cutting through plains.
			h := base
			if t := smoothstep(0.55, 0.9, base); t > 0 {
				h = base*(1-t) + ridge*t
			}

			h = contrast(h, elevContrast)

			// A falloff on the raw distance gives a circular island with a
			// suspiciously smooth shore. Perturbing the distance instead
			// pushes the coastline in and out by a few tiles and costs one
			// more noise sample.
			wobble := coastNoise.fbm(x*coastScale, y*coastScale, 2, 2.0, 0.5) * 0.11
			h *= islandFalloff(col, row, m.Cols, m.Rows, wobble)

			elev := int(math.Round(h * MaxElevation))
			elev = clampInt(elev, 0, MaxElevation)

			moisture := moistNoise.fbm(x*moistScale, y*moistScale, 3, 2.0, 0.5)*0.5 + 0.5
			moisture = contrast(moisture, moistContrast)
			vent := ventNoise.fbm(x*ventScale, y*ventScale, 2, 2.0, 0.5)*0.5 + 0.5

			t := m.AtOffset(col, row)
			t.Elevation = int8(elev)
			t.Terrain = classify(elev, moisture, vent)
		}
	}

	ensureIce(m)
}

// MinIceTiles is how much ice a map is guaranteed, however its noise came out.
//
// Ice is the only ground an Ice Extractor can stand on, and water is the
// resource a colony fails on first. Leaving that to chance meant leaving
// whether the map was playable to chance.
const MinIceTiles = 10

// ensureIce guarantees a map has somewhere to get water, by freezing its
// highest ground until there is enough.
//
// classify makes ice above MaxElevation-2, which is a cliff rather than a
// gradient: a map whose tallest peak reaches 11 has no ice at all, and one
// with a plateau at 12 has a hundred tiles of it. Measured over forty seeds,
// *eighteen had none* and twenty-three had none within reach of the landing
// site. The aggregate distribution test passed the whole time, because a few
// ice-rich maps carried the average for all the maps that had nothing.
//
// The fix is to state the intent instead of hoping for it: ice is at altitude,
// so the highest ground is ice. Deterministic from the seed like everything
// else here, and it takes that ground from basalt, which is scenery.
func ensureIce(m *Map) {
	type tile struct {
		idx  int
		elev int8
	}
	var candidates []tile
	ice := 0

	for i := range m.Tiles {
		switch m.Tiles[i].Terrain {
		case Ice:
			ice++
		case Sea, Vent:
			// A vent is the only other thing worth keeping at altitude, and
			// the sea is not ground.
		default:
			candidates = append(candidates, tile{i, m.Tiles[i].Elevation})
		}
	}
	if ice >= MinIceTiles {
		return
	}

	// Highest first, and by index where the height ties, so the result does
	// not depend on map iteration order.
	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].elev != candidates[b].elev {
			return candidates[a].elev > candidates[b].elev
		}
		return candidates[a].idx < candidates[b].idx
	})

	for _, c := range candidates {
		if ice >= MinIceTiles {
			return
		}
		m.Tiles[c.idx].Terrain = Ice
		ice++
	}
}

// classify turns a tile's elevation and its two climate samples into ground.
// Order matters: the tests that gate on elevation run before the ones that
// gate on climate, so a peak is ice whatever the moisture says.
func classify(elev int, moisture, vent float64) Terrain {
	switch {
	case elev <= SeaLevel:
		return Sea
	case elev >= MaxElevation-2:
		return Ice
	case vent > 0.735 && elev < MaxElevation-4:
		return Vent
	case elev >= 10:
		return Basalt
	case moisture > 0.66:
		return Lichen
	case moisture < 0.34:
		return Dunes
	case vent > 0.655:
		return Crystal
	default:
		return Regolith
	}
}

// islandFalloff scales elevation down toward the edges of the map so the
// continent ends in water rather than at the array bound. It returns 1 in the
// middle and 0 at the corners.
func islandFalloff(col, row, cols, rows int, wobble float64) float64 {
	nx := float64(col)/float64(cols-1)*2 - 1
	ny := float64(row)/float64(rows-1)*2 - 1

	// Euclidean distance would make a circular island in a rectangular map,
	// wasting the corners. Blending it with the square distance keeps the
	// coastline irregular and uses more of the array.
	d := math.Hypot(nx, ny) / math.Sqrt2
	sq := math.Max(math.Abs(nx), math.Abs(ny))
	d = d*0.65 + sq*0.35 + wobble

	return 1 - smoothstep(0.42, 0.98, d)
}

// contrast stretches a value in [0,1] about its midpoint and clamps it back
// into range. Amount 1 is a no-op; above 1 pushes toward the extremes.
func contrast(v, amount float64) float64 {
	v = (v-0.5)*amount + 0.5
	return math.Max(0, math.Min(1, v))
}

func smoothstep(edge0, edge1, x float64) float64 {
	if edge1 == edge0 {
		return 0
	}
	t := (x - edge0) / (edge1 - edge0)
	t = math.Max(0, math.Min(1, t))
	return t * t * (3 - 2*t)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StartSite picks somewhere to drop the first colonists: the buildable tile
// whose immediate neighbourhood is the flattest and most buildable, breaking
// ties toward the middle of the map.
//
// A generated map is allowed to be hostile, but it is not allowed to be
// unplayable, and "every tile within reach is a cliff" is unplayable. This is
// the one place the generator is asked for a guarantee rather than a shape.
func (m *Map) StartSite() hex.Axial {
	best := FromOffset(m.Cols/2, m.Rows/2)
	bestScore := math.Inf(-1)

	for row := 0; row < m.Rows; row++ {
		for col := 0; col < m.Cols; col++ {
			a := FromOffset(col, row)
			t := m.At(a)
			if t == nil || !t.Terrain.Info().Buildable {
				continue
			}

			score := 0.0
			for _, n := range hex.Area(a, 2) {
				nt := m.At(n)
				if nt == nil {
					score -= 3 // the map edge is a bad neighbour
					continue
				}
				if nt.Terrain.Info().Buildable {
					score += 2
				}
				drop := int(nt.Elevation) - int(t.Elevation)
				score -= math.Abs(float64(drop)) * 1.5
			}

			// Prefer the middle, gently: enough to break ties, not enough to
			// override good ground.
			cx, cy := float64(m.Cols-1)/2, float64(m.Rows-1)/2
			dist := math.Hypot(float64(col)-cx, float64(row)-cy)
			score -= dist * 0.35

			if score > bestScore {
				bestScore, best = score, a
			}
		}
	}
	return best
}
