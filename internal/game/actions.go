package game

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/derekmwright/vesper3/internal/meshgen"
	"github.com/derekmwright/vesper3/internal/world"
)

// Terraforming costs ore per step and is bounded so the coastline never
// moves: the water surface is built once from the heightmap at load, and
// letting a player dig below the waterline would leave the sea hanging over
// dry ground with nothing to tell it otherwise.
const (
	terraformCost   = 6
	terraformFloor  = world.SeaLevel + 1
	terraformCeling = world.MaxElevation
)

// hotkeys maps the number row onto the hotbar, in the order colony.Buildable
// lists them.
var hotkeys = [...]input.Key{
	input.Key1, input.Key2, input.Key3, input.Key4,
	input.Key5, input.Key6, input.Key7, input.Key8,
	input.Key9,
}

// intent is what the player has told the game they want to do: which tool is
// in hand, what it is aimed at, and which way round it will be put down.
//
// It is the input half of this package, kept apart from ui below it — that one
// is about the interface's own state, like which panel is under the pointer.
// The distinction matters because these two are written by different things:
// intent is written by the keyboard and the world pick, ui by the layout.
type intent struct {
	mode     Mode
	selected colony.Kind

	// facing is which way the next structure will be built, in sixths of a
	// turn. It persists across selections: a player laying out a row wants the
	// next one the same way round.
	facing uint8

	// hover is the tile under the pointer, and hovering says whether the ray
	// hit anything at all.
	hover    hex.Axial
	hovering bool
}

func newIntent() intent {
	return intent{selected: colony.Habitat, mode: ModeBuild}
}

// toolChanged publishes the player's current tool along with the one-line
// reason it changed.
func (g *Game) toolChanged(format string, args ...any) {
	emit(g, ToolChanged{
		Mode:     g.intent.mode,
		Selected: g.intent.selected,
		Facing:   g.intent.facing,
		Why:      fmt.Sprintf(format, args...),
	})
}

func (g *Game) handleKeys(e *glyph.Engine) {
	in := e.Input()

	for i, key := range hotkeys {
		if i >= len(colony.Buildable) {
			break
		}
		if in.KeyPressed(key) {
			g.selectKind(e, i)
		}
	}

	switch {
	case in.KeyPressed(input.KeyB):
		g.intent.mode = ModeBuild
		g.toolChanged("Build mode")
	case in.KeyPressed(input.KeyX):
		g.intent.mode = ModeDemolish
		g.toolChanged("Demolish mode: left-click removes a structure")
	case in.KeyPressed(input.KeyT):
		g.intent.mode = ModeTerraform
		g.toolChanged("Terraform mode: left-click raises, right-click lowers (%d iron)", terraformCost)
	}

	// Shift and the wheel turn the structure about to be placed, a sixth of a
	// turn at a time — the grid's own symmetry, so it always sits square on
	// its hexagon.
	//
	// The scroll is consumed HERE, before Camera.Update runs, which is what
	// stops the same wheel notch also zooming. Both read the same one-shot
	// value, so whichever asks first gets it and the other sees nothing; the
	// order in Game.Update is the whole mechanism.
	if in.KeyDown(input.KeyLeftShift) || in.KeyDown(input.KeyRightShift) {
		if _, sy := in.ConsumeScroll(); sy != 0 {
			step := 1
			if sy < 0 {
				step = -1
			}
			g.intent.facing = uint8((int(g.intent.facing) + step + colony.Facings) % colony.Facings)
			g.toolChanged("Facing %d of %d", g.intent.facing+1, colony.Facings)
		}
	}

	// Escape opens the in-game menu. It used to quit outright, which is a
	// thing to do to a player exactly once. The demolition prompt consumes it
	// before this runs; see Update.
	if in.KeyPressed(input.KeyEscape) {
		g.pause()
		return
	}

	if in.KeyPressed(input.KeyF3) {
		g.ui.showDebug = !g.ui.showDebug
	}

	// Live interface scale, because the right value depends on a display this
	// code cannot see.
	if in.KeyPressed(input.KeyLeftBracket) {
		g.setUIScale(e, -1)
	}
	if in.KeyPressed(input.KeyRightBracket) {
		g.setUIScale(e, +1)
	}

	if in.KeyPressed(input.KeyC) {
		minX, minZ, maxX, maxZ := g.Map.Bounds()
		g.cam.FrameAll(minX, minZ, maxX, maxZ)
		emit(g, Noticed{Text: "Framed the whole continent"})
	}

	if in.KeyPressed(input.KeyF5) {
		if err := g.Save(); err != nil {
			g.refuse("Save failed: %v", err)
		} else {
			emit(g, GameSaved{Path: g.cfg.SavePath})
		}
	}
	if in.KeyPressed(input.KeyF9) {
		if err := g.Load(e); err != nil {
			g.refuse("Load failed: %v", err)
		} else {
			emit(g, GameLoaded{Path: g.cfg.SavePath})
		}
	}
}

func (g *Game) handleClicks(e *glyph.Engine) {
	in := e.Input()

	left := in.MousePressed(input.MouseButtonLeft)

	// Right fires on release rather than on press, and only when the pointer
	// did not move: the same button orbits the camera while it is held, and a
	// drag that happened to start over a building must not also demolish it.
	right := g.cam.WasRightClick(in)
	if !left && !right {
		return
	}

	// A click on a hotbar slot selects it and goes no further.
	if g.ui.hot >= 0 && left {
		g.selectKind(e, g.ui.hot)
		return
	}
	// Anywhere else over the interface swallows the click entirely, rather
	// than letting it through to the tile behind the panel.
	if g.ui.blocked || !g.intent.hovering {
		return
	}

	shift := in.KeyDown(input.KeyLeftShift) || in.KeyDown(input.KeyRightShift)

	switch g.intent.mode {
	case ModeTerraform:
		if left {
			g.terraform(e, g.intent.hover, +1)
		}
		if right {
			g.terraform(e, g.intent.hover, -1)
		}

	case ModeDemolish:
		if left || right {
			g.askDemolish(g.intent.hover)
		}

	default: // ModeBuild
		if left {
			g.place(e, g.intent.hover)
		}
		// Shift-right demolishes without leaving build mode. Plain right is
		// the camera's — it orbits while held — and demolition is the one
		// action here that cannot be undone, so it asks for the modifier and
		// then asks again.
		if right && shift {
			g.askDemolish(g.intent.hover)
		}
	}
}

// place builds the selected structure on a tile.
//
// Note what is not here: nothing spawns an entity and nothing writes a status
// line. The colony is the authority on whether the structure exists, and once
// it says so this publishes the fact. Drawing it and announcing it are two
// other systems' business — see subscribe in events.go.
func (g *Game) place(_ *glyph.Engine, a hex.Axial) {
	k, facing := g.intent.selected, g.intent.facing
	if err := g.Colony.PlaceFacing(g.Map, k, a, facing); err != nil {
		emit(g, ActionRefused{Err: err})
		return
	}
	emit(g, StructurePlaced{Kind: k, At: a, Facing: facing})
}

// demolish removes whatever is on a tile.
func (g *Game) demolish(_ *glyph.Engine, a hex.Axial) {
	kind, ok := g.Colony.Demolish(a)
	if !ok {
		g.refuse("Nothing to demolish there")
		return
	}
	iron, crystal := colony.Of(kind).Refund()
	emit(g, StructureDemolished{Kind: kind, At: a, Iron: iron, Crystal: crystal})
}

// terraform raises or lowers a tile by one step.
func (g *Game) terraform(_ *glyph.Engine, a hex.Axial, delta int) {
	tile := g.Map.At(a)
	if tile == nil {
		return
	}
	if _, built := g.Colony.At(a); built {
		g.refuse("Cannot reshape ground under a structure")
		return
	}
	if tile.Terrain == world.Sea {
		g.refuse("The methane sea cannot be reshaped")
		return
	}

	next := int(tile.Elevation) + delta
	if next < terraformFloor {
		g.refuse("Already at the waterline")
		return
	}
	if next > terraformCeling {
		g.refuse("Already at maximum elevation")
		return
	}
	if g.Colony.Iron < terraformCost {
		g.refuse("Terraforming costs %d iron", terraformCost)
		return
	}

	g.Colony.Iron -= terraformCost
	tile.Elevation = int8(next)
	emit(g, TileReshaped{At: a, Delta: delta, Cost: terraformCost})
}

// spawnBuilding creates the entity for a placed structure.
func (g *Game) spawnBuilding(e *glyph.Engine, k colony.Kind, a hex.Axial) {
	x, z := g.Map.Center(a)
	y := g.Map.SurfaceY(a)

	// The facing comes from what was actually placed, so a reload puts every
	// structure back the way it was built rather than the way the cursor
	// happens to be pointing now.
	yaw := float32(0)
	if b, ok := g.Colony.At(a); ok {
		yaw = facingYaw(b.Facing)
	}

	ents := make([]glyph.Entity, 0, len(g.scene.structParts[k]))

	for _, part := range g.scene.structParts[k] {
		scale := part.Scale
		if scale <= 0 {
			scale = 1
		}

		ent := e.Spawn()
		e.C.Transform.Set(ent, &glyph.Transform{
			Position: mgl32.Vec3{x, y, z},
			// The heading the player chose, shared by every part so they do
			// not come apart.
			//
			// This was once a per-tile hash, rotating each structure to one
			// of the six orientations the grid allows so a row of habitats
			// would not look stamped. That read as a colony nobody surveyed,
			// because the structures are not hex-symmetric. Variety is worth
			// having; deciding it for the player was not.
			Rotation: mgl32.Vec3{0, yaw, 0},
			Scale:    mgl32.Vec3{scale, scale, scale},
		})
		e.C.MeshRef.Set(ent, &glyph.MeshRef{
			Mesh:      part.Mesh,
			Metallic:  part.Metallic,
			Roughness: part.Roughness,
		})
		if part.Material != nil || part.Texture != nil {
			e.C.MaterialRef.Set(ent, &glyph.MaterialRef{
				PBR:     part.Material,
				Texture: part.Texture,
			})
		}
		if part.DoubleSided {
			e.C.DoubleSided.Set(ent, &glyph.DoubleSided{})
		}
		e.C.Color.Set(ent, &glyph.Color{R: part.Color[0], G: part.Color[1], B: part.Color[2]})
		if part.CondenserPulse {
			configureCondenserPulse(e, ent)
		} else {
			e.C.Static.Set(ent, &glyph.Static{})
		}
		ents = append(ents, ent)
	}

	if k == colony.Condenser {
		ents = g.addCondenserPulseTrail(e, ents)
	}
	g.scene.buildingEnt[a] = ents
}

// updateCursor moves the tile marker and the placement ghost onto whatever
// the mouse is over.
func (g *Game) updateCursor(e *glyph.Engine) {
	if !g.intent.hovering {
		e.C.Hidden.Set(g.scene.cursorEnt, &glyph.Hidden{})
		g.hideGhost(e)
		return
	}

	x, z := g.Map.Center(g.intent.hover)
	y := g.Map.SurfaceY(g.intent.hover)
	if g.Map.IsSea(g.intent.hover) {
		y = world.SeaY()
	}

	// Lifted clear of the ground it marks: coplanar geometry z-fights, and
	// under reverse-Z the flicker is worse at distance, which is exactly
	// where a strategy camera sits.
	const lift = 0.02

	e.C.Hidden.Remove(g.scene.cursorEnt)
	if t, ok := e.C.Transform.Get(g.scene.cursorEnt); ok {
		t.Position = mgl32.Vec3{x, y + lift, z}
	}

	ok := g.canActHere()
	col := ghostOKColor
	if !ok {
		col = ghostBadColor
	}
	if c, has := e.C.Color.Get(g.scene.cursorEnt); has {
		c.R, c.G, c.B = col[0], col[1], col[2]
	}

	// The ghost only makes sense while building.
	if g.intent.mode != ModeBuild {
		g.hideGhost(e)
		return
	}
	g.showGhost(e, mgl32.Vec3{x, y, z}, facingYaw(g.intent.facing), col)
}

// canActHere reports whether the current mode's action would succeed on the
// hovered tile. It drives the cursor colour, and it asks the same functions
// the click handler does rather than reimplementing their rules.
func (g *Game) canActHere() bool {
	if !g.intent.hovering {
		return false
	}
	switch g.intent.mode {
	case ModeDemolish:
		_, built := g.Colony.At(g.intent.hover)
		return built
	case ModeTerraform:
		tile := g.Map.At(g.intent.hover)
		if tile == nil || tile.Terrain == world.Sea {
			return false
		}
		if _, built := g.Colony.At(g.intent.hover); built {
			return false
		}
		return g.Colony.Iron >= terraformCost
	default:
		return g.Colony.CanPlace(g.Map, g.intent.selected, g.intent.hover) == nil
	}
}

// foundLandingSite puts the first habitat and its power on the map for free.
//
// An empty planet and a pile of ore is a worse opening than a lander already
// on the ground: with a habitat standing, colonists start arriving on the
// first tick, which immediately poses the question the game is about — where
// does their food come from. The structures are placed, not bought, so the
// starting ore is still entirely the player's to spend.
func (g *Game) foundLandingSite(_ *glyph.Engine) {
	site := g.Map.StartSite()

	if err := g.Colony.Found(g.Map, colony.Habitat, site); err == nil {
		emit(g, StructurePlaced{Kind: colony.Habitat, At: site})
	}

	// Power next door, on whichever neighbour will take it. StartSite already
	// guarantees buildable ground around it, so this normally takes the first
	// try; the loop is here for the map where it does not.
	for d := 0; d < 6; d++ {
		at := site.Neighbor(d)
		if err := g.Colony.Found(g.Map, colony.SolarArray, at); err == nil {
			emit(g, StructurePlaced{Kind: colony.SolarArray, At: at})
			break
		}
	}

	// Point the camera at what was just built rather than at the bare tile.
	x, z := g.Map.Center(site)
	g.cam.Focus = mgl32.Vec3{x, g.Map.SurfaceY(site), z}
}

// rebuildAllChunks re-uploads every chunk, which is what loading a save
// needs.
func (g *Game) rebuildAllChunks(e *glyph.Engine) {
	nx, ny := meshgen.ChunkGrid(g.Map)
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			g.uploadChunk(e, meshgen.ChunkID{CX: cx, CY: cy})
		}
	}
}

// uiScaleSteps are the scales the bracket keys cycle through.
var uiScaleSteps = []float32{1, 1.25, 1.5, 2, 2.5, 3, 4}

// setUIScale steps the interface scale up or down and reports the new value.
func (g *Game) setUIScale(e *glyph.Engine, delta int) {
	current := float32(1)
	if g.hud != nil && g.hud.scale > 0 {
		current = g.hud.scale
	}

	// Find where the current scale sits in the ladder, then step from there.
	idx := 0
	for i, s := range uiScaleSteps {
		if s <= current+0.01 {
			idx = i
		}
	}
	idx = max(0, min(len(uiScaleSteps)-1, idx+delta))

	g.ui.scaleAdj = uiScaleSteps[idx]
	g.cfg.UIScale = 0 // a live adjustment takes over from the flag
	emit(g, Noticed{Text: fmt.Sprintf("Interface scale %.2fx", g.ui.scaleAdj)})
}

// selectKind picks the i-th buildable structure and switches to build mode.
func (g *Game) selectKind(e *glyph.Engine, i int) {
	if i < 0 || i >= len(colony.Buildable) {
		return
	}
	g.intent.selected = colony.Buildable[i]
	g.intent.mode = ModeBuild
	g.rebuildGhost(e)

	spec := colony.Of(g.intent.selected)
	g.toolChanged("%s: %s (%s)", spec.Name, spec.Desc, spec.CostText())
}

// facingYaw turns a facing index into radians.
func facingYaw(facing uint8) float32 {
	return float32(facing%colony.Facings) * (2 * math.Pi / colony.Facings)
}
