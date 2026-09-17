package game

import (
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/meshgen"
)

// scene is the GPU's copy of the world: the meshes that were uploaded and the
// entities that draw them.
//
// It is kept apart from the simulation deliberately. world.Map and
// colony.Colony are the truth and neither knows the renderer exists; this is
// the mirror, and every field here is something that has to be corrected when
// the truth changes — a chunk rebuilt after terraforming, an entity spawned
// when a structure is placed, despawned when it is demolished. Anything that
// can be recomputed from the map or the colony does not belong here.
type scene struct {
	steam steamSystem
	// Chunk meshes are dynamic because terraforming rewrites them in place.
	chunkMesh map[meshgen.ChunkID]*renderer.Mesh
	chunkEnt  map[meshgen.ChunkID]glyph.Entity

	// Structure geometry is built once per kind and shared by every instance.
	// ghostParts is the same geometry stripped of its materials, for the
	// placement preview; see ghost.go.
	structParts map[colony.Kind][]meshPart
	ghostParts  map[colony.Kind][]meshPart
	cursorMesh  *renderer.Mesh

	// One structure can be several entities: a glTF exporter splits a mesh by
	// material, so a three-material building arrives as three primitives that
	// have to be drawn with three different colours.
	buildingEnt map[hex.Axial][]glyph.Entity

	cursorEnt glyph.Entity

	// The preview is one entity per primitive of the selected structure, so
	// it is respawned when the selection changes rather than re-pointed.
	ghostEnts []glyph.Entity

	// Scratch buffers for chunk rebuilds, kept so terraforming does not
	// allocate two slices per edited tile per frame.
	scratchV []renderer.Vertex
	scratchI []uint16
}

func newScene() scene {
	return scene{
		chunkMesh:   make(map[meshgen.ChunkID]*renderer.Mesh),
		chunkEnt:    make(map[meshgen.ChunkID]glyph.Entity),
		structParts: make(map[colony.Kind][]meshPart),
		ghostParts:  make(map[colony.Kind][]meshPart),
		buildingEnt: make(map[hex.Axial][]glyph.Entity),
	}
}

// despawnBuilding removes every entity standing on a tile and forgets it.
// Structures are one-to-many, so this is the only correct way to remove one —
// despawning the first entity and dropping the slice leaves the rest drawn
// with nothing referring to them.
func (s *scene) despawnBuilding(e *glyph.Engine, a hex.Axial) {
	ents, ok := s.buildingEnt[a]
	if !ok {
		return
	}
	for _, ent := range ents {
		e.Despawn(ent)
	}
	delete(s.buildingEnt, a)
	delete(s.steam.emitters, a)
}

// clearBuildings removes every standing structure, which is what loading a
// save has to do before spawning the one it read.
func (s *scene) clearBuildings(e *glyph.Engine) {
	for a := range s.buildingEnt {
		s.despawnBuilding(e, a)
	}
}
