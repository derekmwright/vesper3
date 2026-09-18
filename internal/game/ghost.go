package game

import (
	"log"

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
// # One mesh, not one per primitive
//
// A glTF exporter splits a mesh by material, so a structure arrives as several
// primitives. The preview used to be one translucent entity per primitive,
// because renderer.LoadGLTF returned GPU handles and no vertex data and there
// was nothing to merge on the CPU side. That cost a draw per primitive and,
// worse, made a structure blend against *itself*: where two of its own parts
// overlapped, the preview went denser.
//
// glyphengine#19 retains the decoded geometry, so the parts are concatenated
// into one mesh at load. One entity, one draw, and a preview with a single
// consistent alpha — which is what a placement preview wants, because the
// varying density read as depth in the model rather than as the flat statement
// of intent it is meant to be.

const ghostAlpha = 0.45

var (
	ghostOKColor  = [3]float32{0.35, 1.00, 0.55}
	ghostBadColor = [3]float32{1.00, 0.32, 0.28}
)

// buildGhostMesh prepares the preview geometry for one structure.
//
// A modelled structure's primitives are concatenated into a single mesh:
// same vertices, same winding, indices rebased as each primitive is appended.
// Anything without a model falls back to meshgen.Ghost, which builds the
// procedural shapes in flat white - those genuinely cannot be reused, because
// meshgen.Structure paints itself in vertex data and tinting it green would
// give the product of two colours.
func (g *Game) buildGhostMesh(e *glyph.Engine, k colony.Kind, geom []renderer.ModelMesh) error {
	r := e.Renderer()

	if len(geom) > 0 {
		verts, idx, ok := mergeGeometry(geom)
		if ok {
			mesh, err := r.CreateIndexedMesh32(verts, idx)
			if err != nil {
				return err
			}
			g.scene.ghostParts[k] = []meshPart{{Mesh: mesh, Scale: modelScale, Roughness: 1}}
			return nil
		}
		// A skinned primitive decodes to a different vertex layout and carries
		// no Verts, so there is nothing to merge. Nothing here is skinned
		// today; the fallback is so that changing that does not silently
		// produce an empty preview.
		log.Printf("%v has geometry that cannot be merged; using the procedural ghost", k)
	}

	gb := meshgen.Ghost(k)
	mesh, err := r.CreateIndexedMesh32(gb.Verts, gb.Idx)
	if err != nil {
		return err
	}
	g.scene.ghostParts[k] = []meshPart{{Mesh: mesh, Scale: 1, Roughness: 0.6}}
	return nil
}

// mergeGeometry concatenates a model's primitives into one vertex and index
// buffer, rebasing each primitive's indices as it goes. It reports false if any
// primitive has no geometry to contribute.
func mergeGeometry(geom []renderer.ModelMesh) ([]renderer.Vertex, []uint32, bool) {
	var verts []renderer.Vertex
	var idx []uint32
	for _, mm := range geom {
		if len(mm.Verts) == 0 || len(mm.Idx) == 0 {
			return nil, nil, false
		}
		base := uint32(len(verts))
		verts = append(verts, mm.Verts...)
		for _, i := range mm.Idx {
			idx = append(idx, base+i)
		}
	}
	return verts, idx, len(verts) > 0
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
