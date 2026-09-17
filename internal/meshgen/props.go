package meshgen

import (
	"github.com/derekmwright/vesper3/internal/colony"
)

// Structure sizes are relative to a tile's circumradius of 1. A hexagon's
// inradius is about 0.87, which is the real ceiling on a footprint: past it a
// structure crosses its own tile edge and onto a neighbour.
const (
	padRadius  = 0.62
	padHeight  = 0.07
	accentDark = 0.62
	accentLite = 1.25

	// Shapes are authored against a unit tile and then scaled up to fill it.
	// At 1.0 a habitat covered about a third of its hexagon and read as a
	// pebble dropped on the ground rather than as a building; the grid, not
	// the structure, was what the eye followed. 1.25 brings the widest
	// structure to a radius of about 0.79 against the hexagon's inradius of
	// 0.87, so they fill their tile without touching the six around them.
	structureScale = 1.25
)

// Structure returns the geometry for a kind of building, in tile-local space
// with its base at y=0. The game creates one GPU mesh per kind and reuses it
// for every instance.
//
// Each of these is a handful of primitives rather than an imported model:
// they have to read at a glance from a camera fifty units up, and silhouette
// is what does that, not detail.
func Structure(k colony.Kind) *Builder {
	return structure(k, colony.Of(k).Color)
}

// Ghost returns the same shapes in flat white, for the placement preview.
//
// It has to be a separate mesh rather than the same one under a green tint,
// because the lit shader multiplies vertex colour by the per-entity tint
// (shaders/lit.vert). Tinting the painted mesh green would give a muddy
// product of two colours; tinting a white one gives the green the preview
// actually wants.
func Ghost(k colony.Kind) *Builder {
	return structure(k, [3]float32{1, 1, 1})
}

func structure(k colony.Kind, col [3]float32) *Builder {
	dark := shade(col, accentDark)
	lite := shade(col, accentLite)

	b := &Builder{}

	switch k {
	case colony.Habitat:
		// A pressurised dome on a low pad, with an airlock stub facing +X so
		// the shape has a front.
		b.Cylinder(0, 0, 0, padRadius, padHeight, 12, dark)
		b.Dome(0, padHeight, 0, 0.44, 4, 14, col)
		b.Box(0.46, padHeight+0.11, 0, 0.30, 0.22, 0.26, lite)

	case colony.SolarArray:
		// Three panels on short legs. The tilt is toward -Z, which is roughly
		// where the sun tracks, and it is what makes the array read as an
		// array rather than as a slab.
		b.Cylinder(0, 0, 0, padRadius, padHeight*0.6, 6, dark)
		for i, z := range []float32{-0.34, 0, 0.34} {
			// Legs are centred at half their height so they stand on the
			// tile surface rather than sinking below it.
			b.Box(-0.22, 0.09, z, 0.05, 0.18, 0.05, dark)
			b.Box(0.22, 0.09, z, 0.05, 0.18, 0.05, dark)
			panel := col
			if i == 1 {
				panel = lite
			}
			b.TiltedPanel(0, padHeight+0.20, z, 0.84, 0.28, -0.42, panel)
		}

	case colony.Mine:
		// A shed and a derrick. The derrick is the tall thing, so it is what
		// identifies the tile from across the map.
		b.Cylinder(0, 0, 0, padRadius, padHeight, 6, dark)
		b.Box(-0.26, padHeight+0.11, 0.18, 0.44, 0.22, 0.40, col)
		b.Cone(0.16, padHeight, -0.06, 0.28, 0.78, 4, lite)
		b.Box(0.16, padHeight+0.80, -0.06, 0.10, 0.10, 0.10, dark)

	case colony.Extractor:
		// A tank, a cap, and an intake pipe running off the pad.
		b.Cylinder(0, 0, 0, padRadius, padHeight, 10, dark)
		b.Cylinder(-0.10, padHeight, 0, 0.34, 0.46, 12, col)
		b.Dome(-0.10, padHeight+0.46, 0, 0.34, 3, 12, lite)
		b.Box(0.34, padHeight+0.12, 0, 0.42, 0.13, 0.13, dark)
		b.Cylinder(0.50, padHeight, 0, 0.09, 0.26, 6, dark)

	case colony.Greenhouse:
		// A gabled glasshouse: two panels meeting at a ridge, with end walls
		// so it is not see-through from the side.
		b.Cylinder(0, 0, 0, padRadius, padHeight*0.7, 6, dark)
		b.Box(0, padHeight, 0, 1.04, 0.10, 0.70, dark)
		b.TiltedPanel(0, padHeight+0.22, -0.17, 1.00, 0.40, 0.72, col)
		b.TiltedPanel(0, padHeight+0.22, 0.17, 1.00, 0.40, -0.72, col)
		b.Box(-0.50, padHeight+0.16, 0, 0.06, 0.24, 0.66, lite)
		b.Box(0.50, padHeight+0.16, 0, 0.06, 0.24, 0.66, lite)

	case colony.Condenser:
		// A chilled column with radiator fins around it. The fins are what
		// separate it from the extractor's smooth tank at a glance, which
		// matters because the two do the same job and a player needs to see
		// which one they built.
		b.Cylinder(0, 0, 0, padRadius, padHeight, 8, dark)
		b.Cylinder(0, padHeight, 0, 0.26, 0.54, 10, col)
		for i, ang := range []float64{0, 1.047, 2.094, 3.142, 4.189, 5.236} {
			fin := lite
			if i%2 == 1 {
				fin = col
			}
			s, c2 := sin64(ang), cos64(ang)
			b.Box(c2*0.40, padHeight+0.22, s*0.40, 0.30, 0.34, 0.06, fin)
		}
		b.Dome(0, padHeight+0.54, 0, 0.26, 3, 10, lite)

	case colony.Geothermal:
		// A turbine hall under a stack. The stack is the tallest thing a
		// colony builds, which is the point: it marks the vent.
		b.Cylinder(0, 0, 0, padRadius, padHeight, 8, dark)
		b.Cylinder(0, padHeight, 0, 0.46, 0.26, 10, col)
		b.Cylinder(-0.02, padHeight+0.26, 0, 0.17, 0.62, 10, lite)
		b.Cone(-0.02, padHeight+0.88, 0, 0.21, 0.16, 10, dark)
		b.Box(0.44, padHeight+0.14, 0.22, 0.26, 0.20, 0.26, dark)

	default:
		// An unknown kind still gets a body, so a bad save shows a crate
		// rather than an invisible building.
		b.Box(0, 0.25, 0, 0.6, 0.5, 0.6, col)
	}

	b.Scale(structureScale)
	return b
}

// Cursor returns the hovering tile marker: a hex ring that sits just above
// the ground, drawn full-bright so it stays readable in shadow and at night.
//
// The band is wide rather than a hairline. At the far end of the camera's
// zoom a tile is only a few dozen pixels across, and a ring of a tenth of a
// tile is a shimmer rather than a marker.
func Cursor(size float32) *Builder {
	b := &Builder{}
	b.HexRing(0, size*CursorInnerRadius, size*CursorOuterRadius, [3]float32{1, 1, 1})
	return b
}

// The cursor ring's extent, as fractions of a tile's circumradius. The outer
// edge stops just short of the tile's own corners so the marker reads as
// sitting on the tile rather than bleeding onto the six around it.
const (
	CursorInnerRadius = 0.66
	CursorOuterRadius = 0.96
)
