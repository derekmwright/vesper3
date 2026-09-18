package game

import (
	"fmt"
	"io/fs"
	"log"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/meshgen"
	"github.com/derekmwright/vesper3/internal/world"
)

// heightmapSamplesPerTile is how finely the map is resampled into the
// rectangular heightmap the water surface is cut from. Two samples per tile
// is enough for the shoreline to follow the hexes; more only makes the water
// mesh bigger.
const heightmapSamplesPerTile = 2

// oceanMargin is how far past the last tile the sea keeps going, in world
// units.
//
// Without it the water stops at the map's bounding rectangle, and from any
// height the continent reads as a tray of hexagons sitting in a void with a
// visible straight edge. The margin is sized against the camera's maximum
// zoom so the edge stays over the horizon at every distance the player can
// reach. Off-map samples report height 0, which is below sea level, so the
// margin floods on its own with no special case in the mesher.
const oceanMargin = maxDistance * 2.2

// initTerrain uploads one dynamic mesh per chunk and spawns an entity for it.
//
// Dynamic rather than static because terraforming rewrites a chunk in place.
// The alternative — destroy the mesh and create a new one — churns a GPU
// allocation on every click of a key the player may well hold down.
func (g *Game) initTerrain(e *glyph.Engine) error {
	r := e.Renderer()
	// One shared atlas keeps the existing one-draw-per-chunk layout. Missing
	// optional art preserves the original vertex-colored terrain.
	var detail *renderer.Texture
	if g.cfg.Assets != nil {
		var err error
		detail, err = r.LoadTexture(g.cfg.Assets, "assets/terrain-detail.png")
		if err != nil {
			log.Printf("terrain detail unavailable (%v); using plain terrain", err)
		}
	}
	maxV, maxI := meshgen.ChunkCapacity()

	nx, ny := meshgen.ChunkGrid(g.Map)
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			id := meshgen.ChunkID{CX: cx, CY: cy}

			mesh, err := r.CreateDynamicIndexedMesh(maxV, maxI)
			if err != nil {
				return fmt.Errorf("create chunk mesh %v: %w", id, err)
			}
			g.scene.chunkMesh[id] = mesh

			ox, oz := meshgen.Origin(id)
			ent := e.Spawn()
			e.C.Transform.Set(ent, &glyph.Transform{
				Position: mgl32.Vec3{ox, 0, oz},
				Scale:    mgl32.Vec3{1, 1, 1},
			})
			e.C.MeshRef.Set(ent, &glyph.MeshRef{Mesh: mesh, Roughness: 0.92})
			if detail != nil {
				e.C.MaterialRef.Set(ent, &glyph.MaterialRef{Texture: detail})
			}
			// Terrain is world geometry that does not move, and tagging it so
			// keeps it out of the per-tick spatial grid rebuild.
			e.C.Static.Set(ent, &glyph.Static{})
			g.scene.chunkEnt[id] = ent

			g.uploadChunk(e, id)
		}
	}
	e.RebuildStatics()
	return nil
}

// uploadChunk rebuilds one chunk's geometry and sends it to the GPU.
func (g *Game) uploadChunk(e *glyph.Engine, id meshgen.ChunkID) {
	mesh, ok := g.scene.chunkMesh[id]
	if !ok {
		return
	}

	g.scene.scratchV, g.scene.scratchI = meshgen.BuildChunk(g.Map, id, g.scene.scratchV, g.scene.scratchI)

	if err := e.Renderer().UpdateMeshData(mesh, g.scene.scratchV, g.scene.scratchI); err != nil {
		// Not fatal: one stale chunk is a visual bug, not a reason to take
		// the colony down.
		emit(g, Noticed{Text: fmt.Sprintf("chunk %v failed to update: %v", id, err)})
		return
	}

	// A dynamic mesh has no bounds of its own, so without this every chunk
	// would be drawn in every cascade of every frame. Recomputed here because
	// terraforming changes the height the bound has to cover.
	setBounds(mesh, g.scene.scratchV)
}

// setBounds fits a bounding sphere around freshly built geometry so the
// renderer can frustum-cull it.
func setBounds(mesh *renderer.Mesh, verts []renderer.Vertex) {
	if len(verts) == 0 {
		mesh.BoundRadius = 0
		return
	}
	lo, hi := verts[0].Pos, verts[0].Pos
	for _, v := range verts[1:] {
		for i := 0; i < 3; i++ {
			lo[i] = min32(lo[i], v.Pos[i])
			hi[i] = max32(hi[i], v.Pos[i])
		}
	}
	center := [3]float32{(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2}

	var r2 float32
	for _, v := range verts {
		dx, dy, dz := v.Pos[0]-center[0], v.Pos[1]-center[1], v.Pos[2]-center[2]
		r2 = max32(r2, dx*dx+dy*dy+dz*dz)
	}
	mesh.BoundCenter = center
	mesh.BoundRadius = sqrt32(r2)
}

// refreshAround re-uploads every chunk a tile's geometry can appear in.
//
// That is more than the tile's own chunk: a cliff wall belongs to the higher
// of the two tiles that share the edge, so raising a tile can add or remove a
// wall on a neighbour that lives in the chunk next door.
func (g *Game) refreshAround(e *glyph.Engine, a hex.Axial) {
	seen := make(map[meshgen.ChunkID]bool, 4)

	touch := func(at hex.Axial) {
		if id, ok := meshgen.ChunkOf(g.Map, at); ok && !seen[id] {
			seen[id] = true
			g.uploadChunk(e, id)
		}
	}

	touch(a)
	for d := 0; d < 6; d++ {
		touch(a.Neighbor(d))
	}
}

// initStructures builds one mesh per structure kind, plus the tile cursor and
// the placement ghost.
func (g *Game) initStructures(e *glyph.Engine) error {
	r := e.Renderer()

	for i, k := range colony.Buildable {
		// A modelled structure if one has been built for it, the procedural
		// shape otherwise. Both end up as the same list of parts, so nothing
		// downstream needs to know which it got.
		var modelGeometry []renderer.ModelMesh
		if parts, geom, ok := g.loadModel(e, i, k); ok {
			modelGeometry = geom
			g.scene.structParts[k] = parts

			// Which part lights up after dark, resolved now rather than per
			// frame.
			if idx, base, found := findLampPart(parts); found {
				g.lights.part[k] = idx
				g.lights.base[k] = base
			}
		} else {
			b := meshgen.Structure(k)
			mesh, err := r.CreateIndexedMesh32(b.Verts, b.Idx)
			if err != nil {
				return fmt.Errorf("create %v mesh: %w", k, err)
			}
			g.scene.structParts[k] = []meshPart{{
				Mesh: mesh, Color: white, Metallic: 0.18, Roughness: 0.55, Scale: 1,
			}}
		}

		// The preview reuses the structure's own geometry where there is a
		// model for it; see ghost.go for why that needs the material dropped
		// rather than merely the tint overridden.
		if err := g.buildGhostMesh(e, k, modelGeometry); err != nil {
			return fmt.Errorf("create %v ghost mesh: %w", k, err)
		}
	}

	cur := meshgen.Cursor(world.TileSize)
	cursorMesh, err := r.CreateIndexedMesh32(cur.Verts, cur.Idx)
	if err != nil {
		return fmt.Errorf("create cursor mesh: %w", err)
	}
	g.scene.cursorMesh = cursorMesh

	g.scene.cursorEnt = e.Spawn()
	e.C.Transform.Set(g.scene.cursorEnt, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	e.C.MeshRef.Set(g.scene.cursorEnt, &glyph.MeshRef{Mesh: cursorMesh})
	e.C.Color.Set(g.scene.cursorEnt, &glyph.Color{R: 1, G: 1, B: 1})
	// Full-bright and casting nothing: a cursor that took lighting would be
	// invisible in a shadow, and one that cast a shadow would look like a
	// physical ring lying on the ground.
	e.C.Emissive.Set(g.scene.cursorEnt, &glyph.Emissive{})
	e.C.NoCastShadow.Set(g.scene.cursorEnt, &glyph.NoCastShadow{})
	e.C.Hidden.Set(g.scene.cursorEnt, &glyph.Hidden{})

	g.rebuildGhost(e)

	return nil
}

// How the placement preview is painted. The mesh under it is white (see
// meshgen.Ghost) because the lit shader multiplies vertex colour by the

// initWater lays the methane sea over the map.
//
// The engine's water pipeline needs a rectangular heightmap to cut the
// surface from, which the hex map is not, so it gets resampled into one. That
// heightmap is also handed to the Scene, where it answers downward raycasts
// in constant time.
func (g *Game) initWater(e *glyph.Engine) error {
	hm, err := g.heightmap()
	if err != nil {
		return err
	}
	e.SetTerrain(hm)

	opts := glyph.DefaultWaterOptions(world.SeaY())
	// Liquid methane, not water: darker, greener, and less transparent than
	// the engine's lake defaults.
	opts.ShallowColor = [3]float32{0.14, 0.31, 0.30}
	opts.DeepColor = [3]float32{0.01, 0.05, 0.07}
	opts.AbsorptionDepth = 3.2

	// The engine's rule, from the WaterOptions doc: the shortest wave
	// component is WaveLength*0.26, and it needs two grid cells to exist at
	// all. The ocean margin makes this surface about 385 units across, so at
	// Resolution 288 a cell is 1.34 units and WaveLength has to clear 10.3 or
	// the finest component is faded out and the sea goes glassy.
	opts.Resolution = 288
	opts.WaveLength = 12
	opts.WaveAmplitude = 0.12

	mesh, err := e.CreateWaterMesh(hm, opts)
	if err != nil {
		return fmt.Errorf("create water mesh: %w", err)
	}

	ent := e.Spawn()
	e.C.Transform.Set(ent, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	e.C.MeshRef.Set(ent, &glyph.MeshRef{Mesh: mesh, Roughness: 0.08})
	e.C.Water.Set(ent, &glyph.Water{Options: opts})
	e.C.NoCastShadow.Set(ent, &glyph.NoCastShadow{})

	return nil
}

// heightmap resamples the hex map onto the rectangular grid the water surface
// and the raycast fast path need.
// refreshTerrainHeights re-sends the heightmap the water pass reads for depth
// and refraction. A new world changes every tile at once, so without this the
// sea would be shaded against the continent that used to be there.
//
// Cheap: it recomputes a float grid and hands it over. No mesh is rebuilt, and
// the water surface itself does not move because sea level has not.
func (g *Game) refreshTerrainHeights(e *glyph.Engine) {
	hm, err := g.heightmap()
	if err != nil {
		log.Printf("could not rebuild the heightmap: %v", err)
		return
	}
	e.SetTerrain(hm)
}

func (g *Game) heightmap() (*glyph.Heightmap, error) {
	minX, minZ, maxX, maxZ := g.Map.Bounds()
	minX, minZ = minX-oceanMargin, minZ-oceanMargin
	maxX, maxZ = maxX+oceanMargin, maxZ+oceanMargin
	worldW, worldD := maxX-minX, maxZ-minZ

	gridW := int(worldW/world.TileSize*heightmapSamplesPerTile) + 1
	gridH := int(worldD/world.TileSize*heightmapSamplesPerTile) + 1

	heights := make([]float32, gridW*gridH)
	for gz := 0; gz < gridH; gz++ {
		for gx := 0; gx < gridW; gx++ {
			x := minX + worldW*float32(gx)/float32(gridW-1)
			z := minZ + worldD*float32(gz)/float32(gridH-1)

			a := world.Layout.At(x, z)
			h := float32(0)
			if g.Map.Contains(a) {
				h = g.Map.SurfaceY(a)
			}
			heights[gz*gridW+gx] = h
		}
	}

	return glyph.NewHeightmap(gridW, gridH, worldW, worldD, minX, minZ, heights)
}

// white is the tint for geometry whose colour is already in the mesh.
var white = [3]float32{1, 1, 1}

// modelScale seats a Blender model on a tile.
//
// The build scripts cap the footprint radius at 0.80 metres, and a hexagon of
// circumradius 1 has an inradius of 0.87 — so a model at 1:1 reaches almost to
// its own tile edge and a row of them reads as one continuous mass. The
// procedural shapes sit at about 0.72 of the circumradius, and this brings the
// models to the same place, which is what makes the two sets look like they
// belong to one game.
const modelScale = 0.88

// modelPath is where a structure's glTF lives, if it has one. The number
// prefix matches the icon sources so the two stay in step.
func modelPath(i int, k colony.Kind) string {
	return fmt.Sprintf("assets/models/%d-%s.glb", i+1, modelSlug(k))
}

// loadModel loads a structure's glTF, reporting false when there is not one.
//
// Models are optional per structure: the ones that have been built are used,
// and the rest keep their procedural geometry, so the set can be replaced one
// at a time rather than all at once.
// loadModel returns a structure's drawable parts and the raw geometry they
// were decoded from. The geometry is what the placement preview is merged out
// of; see ghost.go.
func (g *Game) loadModel(e *glyph.Engine, i int, k colony.Kind) ([]meshPart, []renderer.ModelMesh, bool) {
	if g.cfg.Assets == nil {
		return nil, nil, false
	}
	path := modelPath(i, k)
	if _, err := fs.Stat(g.cfg.Assets, path); err != nil {
		return nil, nil, false
	}

	model, err := e.Renderer().LoadGLTF(g.cfg.Assets, path)
	if err != nil {
		log.Printf("%s failed to load (%v); using procedural geometry", path, err)
		return nil, nil, false
	}

	// One part per glTF primitive: the exporter splits a mesh by material, so
	// a three-material building arrives as three primitives that have to be
	// drawn with three different colours.
	parts := make([]meshPart, 0, len(model.Meshes))
	for _, mm := range model.Meshes {
		colour := mm.BaseColor
		if colour == ([3]float32{}) {
			colour = white
		}
		parts = append(parts, meshPart{
			Name:           mm.Name,
			Mesh:           mm.Mesh,
			Color:          colour,
			Metallic:       mm.Metallic,
			Roughness:      mm.Roughness,
			Texture:        mm.Texture,
			Material:       mm.Material,
			DoubleSided:    mm.DoubleSided,
			CondenserPulse: k == colony.Condenser && isCondenserPulse(mm.Name),
			Scale:          modelScale,
		})
	}
	if len(parts) == 0 {
		return nil, nil, false
	}
	log.Printf("%s loaded: %d primitive(s)", path, len(parts))
	return parts, model.Meshes, true
}

// modelSlug is the filename stem for a structure.
func modelSlug(k colony.Kind) string {
	switch k {
	case colony.Habitat:
		return "habitat"
	case colony.SolarArray:
		return "solar"
	case colony.Mine:
		return "mine"
	case colony.Extractor:
		return "extractor"
	case colony.Condenser:
		return "condenser"
	case colony.Greenhouse:
		return "greenhouse"
	case colony.Geothermal:
		return "geothermal"
	case colony.Battery:
		return "battery"
	}
	return "unknown"
}
