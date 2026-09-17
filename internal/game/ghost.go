package game

import (
	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/meshgen"
)

// The placement preview.
//
// It is the real structure's geometry, drawn as a translucent single-colour
// solid: green where it can go, red where it cannot. It used to be a crude
// procedural stand-in — a dome was a hemisphere, a mine was a box — which said
// where a building would sit but nothing about what would be sitting there.
// Lining up a row of them meant imagining the difference.
//
// # Why it is not simply the structure, drawn see-through
//
// The lit shader multiplies vertex colour by the per-entity tint, and a
// modelled structure carries its material colour on that same tint. Reusing a
// structure's parts as they stand and overriding the tint to green gives
// green — but only because the mesh's own vertices are white and the colour
// was hoisted out of the glTF material. The texture is the part that has to go:
// a textured primitive drawn green would come back as the product of green and
// the texture, which is mud. So a ghost part is the structure's mesh with its
// material dropped, which is the whole of the difference.
//
// # Why several entities
//
// A glTF exporter splits a mesh by material, so a structure arrives as one
// primitive per material and the ghost is one translucent entity per primitive.
// Merging them into a single mesh would be better — one draw, and no blending
// between a structure's own parts — but renderer.LoadGLTF returns GPU handles
// and no vertex data, so there is nothing to merge on the CPU side.
// (glyphengine#19.)
//
// The blending that results is left visible rather than fought: where parts
// overlap, the preview is denser. That reads as a hologram of something with
// structure inside it, which is nearer what a placement preview is for than a
// flat cutout would be.

const ghostAlpha = 0.45

var (
	ghostOKColor  = [3]float32{0.35, 1.00, 0.55}
	ghostBadColor = [3]float32{1.00, 0.32, 0.28}
)

// ghostParts turns a structure's drawable parts into preview parts: same
// geometry and scale, no material, no colour of their own.
//
// The procedural fallback cannot be reused this way and takes a different
// path — meshgen.Structure paints itself in vertex data, so tinting it green
// would give the product of two colours. meshgen.Ghost is the same shapes in
// flat white, and exists for exactly this.
func ghostParts(structure []meshPart, ghostMesh *renderer.Mesh) []meshPart {
	if ghostMesh != nil {
		return []meshPart{{Mesh: ghostMesh, Scale: 1, Roughness: 0.6}}
	}

	out := make([]meshPart, 0, len(structure))
	for _, p := range structure {
		out = append(out, meshPart{
			Mesh:  p.Mesh,
			Scale: p.Scale,
			// Flat and matte: a preview that catches a specular highlight
			// reads as a surface that is already there.
			Roughness:   1,
			Metallic:    0,
			DoubleSided: p.DoubleSided,
		})
	}
	return out
}

// rebuildGhost replaces the preview entities with ones for the selected
// structure.
//
// Respawning rather than re-pointing a fixed entity, because the part count
// differs per structure — a habitat is two primitives and a condenser is five.
// It runs when the selection changes, which is a keypress, not a frame.
func (g *Game) rebuildGhost(e *glyph.Engine) {
	for _, ent := range g.scene.ghostEnts {
		e.Despawn(ent)
	}
	g.scene.ghostEnts = g.scene.ghostEnts[:0]

	for _, part := range g.scene.ghostParts[g.intent.selected] {
		scale := part.Scale
		if scale <= 0 {
			scale = 1
		}

		ent := e.Spawn()
		e.C.Transform.Set(ent, &glyph.Transform{Scale: mgl32.Vec3{scale, scale, scale}})
		e.C.MeshRef.Set(ent, &glyph.MeshRef{
			Mesh:      part.Mesh,
			Metallic:  part.Metallic,
			Roughness: part.Roughness,
		})
		e.C.Color.Set(ent, &glyph.Color{
			R: ghostOKColor[0], G: ghostOKColor[1], B: ghostOKColor[2],
		})
		if part.DoubleSided {
			e.C.DoubleSided.Set(ent, &glyph.DoubleSided{})
		}

		// A real translucent preview: it keeps the scene's lighting and fog
		// and reads as a proposal rather than as a lamp, which is what the
		// full-bright stand-in it replaced always looked like.
		//
		// No NoCastShadow — the engine exempts translucent draws from the
		// shadow pass itself, and a preview that threw a solid shadow would
		// read as a building that already exists.
		e.C.Translucent.Set(ent, &glyph.Translucent{Alpha: ghostAlpha})
		e.C.Hidden.Set(ent, &glyph.Hidden{})

		g.scene.ghostEnts = append(g.scene.ghostEnts, ent)
	}
}

// hideGhost takes the preview off screen.
func (g *Game) hideGhost(e *glyph.Engine) {
	for _, ent := range g.scene.ghostEnts {
		e.C.Hidden.Set(ent, &glyph.Hidden{})
	}
}

// showGhost puts every part of the preview on a tile, facing a heading, in one
// colour. The parts move together or the structure comes apart.
func (g *Game) showGhost(e *glyph.Engine, pos mgl32.Vec3, yaw float32, col [3]float32) {
	for _, ent := range g.scene.ghostEnts {
		e.C.Hidden.Remove(ent)
		if t, has := e.C.Transform.Get(ent); has {
			t.Position = pos
			t.Rotation = mgl32.Vec3{0, yaw, 0}
		}
		if c, has := e.C.Color.Get(ent); has {
			c.R, c.G, c.B = col[0], col[1], col[2]
		}
	}
}

// buildGhostMeshes prepares a preview for every buildable structure. Modelled
// ones reuse their own geometry; anything falling back to procedural shapes
// gets the flat-white version from meshgen.
func (g *Game) buildGhostMeshes(e *glyph.Engine, k colony.Kind, modelled bool) error {
	if modelled {
		g.scene.ghostParts[k] = ghostParts(g.scene.structParts[k], nil)
		return nil
	}

	gb := meshgen.Ghost(k)
	mesh, err := e.Renderer().CreateIndexedMesh32(gb.Verts, gb.Idx)
	if err != nil {
		return err
	}
	g.scene.ghostParts[k] = ghostParts(nil, mesh)
	return nil
}
