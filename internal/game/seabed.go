package game

import (
	"fmt"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/derekmwright/vesper3/internal/world"
)

// The sea has a bottom now, and it is there to stop the sun shining through it.
//
// The water surface runs well past the last tile — see oceanMargin — so from
// any camera the sea reaches the horizon. What it does not have behind it is
// anything at all: terrain chunks exist only over the map, so past the
// coastline the sea is a translucent sheet over empty space. At sunrise and
// sunset the disc sits low enough to be behind that sheet rather than above
// it, and it is drawn depth-tested against a scene with nothing in the way, so
// it comes through the water as a bright smear under the surface.
//
// A plane under everything is the whole fix. It never needs to be seen, only
// to be hit first.

const (
	// seabedDrop is how far under the lowest ground the floor sits.
	//
	// Elevation clamps at 0, so the lowest terrain cap and the foot of the
	// lowest cliff are both at y=0. Any gap clears them; this one is large
	// enough that a float comparison at the horizon cannot put the two
	// surfaces in the wrong order and small enough to stay well under the
	// water it hides behind.
	seabedDrop = 0.5

	// seabedOversize scales the floor past the water's own extent.
	//
	// The floor only has to cover what the water covers: the sea already runs
	// beyond the horizon, so anything past that is hidden by the same surface
	// this sits under. The margin is for the corners — the water is cut to a
	// rectangle around the map and the floor is a square around its centre, so
	// without slack the diagonal would fall short.
	seabedOversize = 1.6
)

// seabedColor is the colour the methane fades to at depth. It is the water's
// own DeepColor and the floor's tint, shared rather than written twice: where
// the floor shows through it has to be the colour the water was already
// heading towards, or the sea gains a visible bottom edge.
var seabedColor = [3]float32{0.01, 0.05, 0.07}

// seabedPlane is where the floor goes and how big it is, given the map it has
// to sit under. Separated from the spawning so the arithmetic can be checked
// without a renderer.
func seabedPlane(m *world.Map) (cx, cz, y, size float32) {
	minX, minZ, maxX, maxZ := m.Bounds()
	cx, cz = (minX+maxX)/2, (minZ+maxZ)/2

	// The water is cut from the map's bounding rectangle grown by oceanMargin
	// on every side, so that rectangle plus the margin is what has to be
	// covered. The longer side decides a square.
	spanX := (maxX - minX) + 2*oceanMargin
	spanZ := (maxZ - minZ) + 2*oceanMargin
	size = max(spanX, spanZ) * seabedOversize

	return cx, cz, world.SurfaceYAt(0) - seabedDrop, size
}

// initSeabed lays the floor under the sea.
func (g *Game) initSeabed(e *glyph.Engine) error {
	cx, cz, y, size := seabedPlane(g.Map)

	mesh, err := e.Renderer().CreatePlane(size, size)
	if err != nil {
		return fmt.Errorf("create seabed mesh: %w", err)
	}

	ent := e.Spawn()
	e.C.Transform.Set(ent, &glyph.Transform{
		Position: mgl32.Vec3{cx, y, cz},
		Scale:    mgl32.Vec3{1, 1, 1},
	})
	e.C.MeshRef.Set(ent, &glyph.MeshRef{Mesh: mesh, Roughness: 1})

	// The colour the water fades to at depth, so the places it does show
	// through — the deepest water, and past the last tile — read as more sea
	// rather than as a lid someone put under it.
	e.C.Color.Set(ent, &glyph.Color{R: seabedColor[0], G: seabedColor[1], B: seabedColor[2]})

	// It is under everything and lit by nothing that matters, so it neither
	// casts nor moves.
	e.C.NoCastShadow.Set(ent, &glyph.NoCastShadow{})
	e.C.Static.Set(ent, &glyph.Static{})

	return nil
}
