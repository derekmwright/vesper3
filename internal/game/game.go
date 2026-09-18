// Package game wires the world, the colony and the engine together: it owns
// the entities, the camera, and everything the player does with the mouse.
//
// The split it maintains is the one the engine's own guide asks for. World
// generation, grid maths and the economy live in packages that import no
// engine code and are tested without a GPU; this package is the only one that
// knows an Engine exists.
package game

import (
	"fmt"
	"io/fs"
	"log"

	"github.com/go-gl/mathgl/mgl32"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/event"
	"github.com/derekmwright/vesper3/internal/world"
)

// meshPart is one drawable piece of a structure: a mesh and the surface it is
// drawn with. Procedural structures are a single part carrying their colour in
// vertex data; modelled ones are one part per glTF primitive.
type meshPart struct {
	// Name is the glTF material's name, and it is the handle the simulation
	// finds a primitive by: the battery's charge strips, the condenser's fin
	// band, the greenhouse's grow lights. See internal/artcheck.
	Name string

	Mesh           *renderer.Mesh
	Color          [3]float32
	Metallic       float32
	Roughness      float32
	Texture        *renderer.Texture
	Material       *renderer.Material
	DoubleSided    bool
	CondenserPulse bool // separate fin-band primitive, animated by the lighting pass

	// Scale seats the part on a tile. Procedural geometry is authored against
	// the grid and needs none; the Blender models are authored to a 0.80-metre
	// footprint radius, which is wider than the 0.87 inradius leaves room for
	// once a building is standing next to its neighbours, so they come down a
	// little. See modelScale.
	Scale float32
}

// Mode is what a click does.
type Mode int

const (
	ModeBuild Mode = iota
	ModeDemolish
	ModeTerraform
)

func (m Mode) String() string {
	switch m {
	case ModeDemolish:
		return "DEMOLISH"
	case ModeTerraform:
		return "TERRAFORM"
	default:
		return "BUILD"
	}
}

// Config is what main passes in.
type Config struct {
	Seed       int64
	Cols, Rows int
	SavePath   string

	// CamDist, CamPitch and CamYaw override the opening camera. They exist so
	// a screenshot can be reproduced exactly — the engine's own examples are
	// all captured with -screenshot rather than by hand, and a view that
	// depends on where the mouse happened to be is not a capture anyone can
	// re-take. Zero means use the defaults.
	CamDist, CamPitch, CamYaw float32

	// Assets is the filesystem the game loads art from. main owns the embed,
	// so this package never hardcodes where the files live and a test or a mod
	// folder can supply a different one.
	Assets fs.FS

	// NoSplash skips the title card, which is what a screenshot run wants.
	NoSplash bool

	// Demo puts a sample colony down around the landing site. It is a capture
	// aid, not a game mode — see demo.go.
	Demo bool

	// TimeOfDay starts the clock somewhere other than mid-morning, and
	// DayLength overrides how long a full cycle takes. A DayLength below zero
	// freezes the clock, which is what a capture of a particular hour needs:
	// dusk is a moving target otherwise.
	TimeOfDay float32
	DayLength float32

	// UIScale fixes the interface scale in design-units-per-pixel. Zero picks
	// one from the window height.
	UIScale float32

	// Cursor pins the pick ray to a point on screen, in fractions of the
	// window, instead of following the mouse. Negative means follow the mouse.
	//
	// The camera flags above are not enough on their own: half of what a
	// capture is meant to show — the tile marker, the placement preview,
	// whether the ground under it can be built on — depends on where the
	// pointer is, and in a headless run that is wherever the operating system
	// happened to leave it. A capture that cannot be re-taken is not evidence.
	CursorX, CursorY float32
}

// Game wires the simulation to the engine. It owns no rules of its own: the
// map and the colony hold the truth, and everything here is either a mirror of
// them on the GPU or a record of what the player is doing with the mouse.
//
// Each field below is a named duty rather than a loose pile of state, because
// this type sits at the junction of five things that change for different
// reasons — the world, the renderer, the lights, the interface, and the
// player's intent — and a struct that lists all of them flat stops saying
// which is which.
type Game struct {
	cfg Config

	// The simulation. Neither of these knows the engine exists.
	Map    *world.Map
	Colony *colony.Colony

	cam *Camera

	// bus is how the systems below hear about what the player did without the
	// code that did it having to know they exist; see events.go.
	bus *event.Bus

	// scene is what the renderer has been told; see scene.go.
	scene scene

	// lights is the dusk lamp pass; see lights.go.
	lights lighting

	// intent is what the player is about to do; see actions.go.
	intent intent

	// ui is the interface's own state, and hud is what draws it; see hud.go.
	ui  uiState
	hud *hud

	// splash is the title card; see splash.go.
	splash splash

	// daylight is sampled in Update and consumed in FixedUpdate, which may
	// run zero or several times per frame.
	daylight float64

	elapsed float32
}

// New builds a game that has not touched the engine yet. Init does that.
func New(cfg Config) *Game {
	if cfg.Cols <= 0 {
		cfg.Cols = 48
	}
	if cfg.Rows <= 0 {
		cfg.Rows = 48
	}
	return &Game{
		cfg:    cfg,
		bus:    event.New(),
		scene:  newScene(),
		lights: newLighting(),
		intent: newIntent(),
		ui:     newUI(),
	}
}

// Init generates the planet and puts everything on the GPU.
func (g *Game) Init(e *glyph.Engine) error {
	m, err := world.NewMap(g.cfg.Cols, g.cfg.Rows, g.cfg.Seed)
	if err != nil {
		return fmt.Errorf("create map: %w", err)
	}
	world.Generate(m)
	g.Map = m
	g.Colony = colony.New()

	if err := g.initTerrain(e); err != nil {
		return err
	}
	if err := g.initStructures(e); err != nil {
		return err
	}
	if err := g.initWater(e); err != nil {
		return err
	}
	if err := g.initHUD(e); err != nil {
		return err
	}
	if err := g.initSplash(e); err != nil {
		return err
	}

	// Subscriptions go up before anything can publish, so the landing site
	// below is spawned by the same path every later structure uses.
	g.subscribe(e)

	e.Renderer().InitParticles(steamMaxInstances)
	g.initEnvironment(e)
	g.initCamera()
	g.foundLandingSite(e)
	if g.cfg.Demo {
		g.foundDemoColony(e)
	}

	log.Printf("Vesper III: seed %d, %dx%d tiles, colony at %v",
		m.Seed, m.Cols, m.Rows, m.StartSite())
	log.Println("1-7 pick a structure, left-click builds, right-click demolishes.")
	log.Println("WASD pans, Q/E turns, R/F tilts, wheel zooms, T terraforms, F5/F9 save and load.")

	return nil
}

// initEnvironment sets the sky and air of an alien world: a four-minute day so
// the solar mechanic is felt within a session rather than watched for, and
// thicker fog than Earth to sell a dusty atmosphere.
func (g *Game) initEnvironment(e *glyph.Engine) {
	// Four minutes a day, so the solar mechanic is felt within a session
	// rather than waited for.
	length := float32(240)
	if g.cfg.DayLength != 0 {
		length = g.cfg.DayLength
	}
	if length > 0 {
		e.SetDayCycleSpeed(1 / length)
	} else {
		e.SetDayCycleSpeed(0) // frozen
	}

	tod := float32(0.30) // mid-morning: the sun is up and casting long shadows
	if g.cfg.TimeOfDay > 0 {
		tod = g.cfg.TimeOfDay
	}
	e.SetTimeOfDay(tod)

	// Fog is atmospheric perspective here, not weather. A strategy camera
	// looks across the better part of a hundred units, and the engine's own
	// note that 0.008 "fades over a few hundred units" is written for a
	// first-person view: at this range it washed the whole continent grey and
	// flattened every biome into the same haze. Measured on seed 20260916 at
	// the default zoom, 0.011 put roughly two thirds of the frame in fog;
	// 0.0025 leaves the far shore hazy and the near ground its own colour,
	// which is the only job it has.
	e.SetFogDensity(0.0025)
}

func (g *Game) initCamera() {
	site := g.Map.StartSite()
	x, z := g.Map.Center(site)

	g.cam = NewCamera(mgl32.Vec3{x, g.Map.SurfaceY(site), z})

	if g.cfg.CamDist > 0 {
		g.cam.Distance = g.cfg.CamDist
	}
	if g.cfg.CamPitch > 0 {
		g.cam.Pitch = g.cfg.CamPitch
	}
	if g.cfg.CamYaw != 0 {
		g.cam.Yaw = g.cfg.CamYaw
	}

	minX, minZ, maxX, maxZ := g.Map.Bounds()
	g.cam.SetBounds(minX, minZ, maxX, maxZ)
}

// Update runs once per frame. Input is read here and only here; see the
// engine's rule 9.
func (g *Game) Update(e *glyph.Engine, dt float32) {
	g.elapsed += dt

	// The title card owns the frame while it is up: no input reaches the
	// camera or the world behind it.
	g.splash.up = g.splashActive(e, dt)
	if g.splash.up {
		g.daylight = daylightFrom(e.Environment().SunElevation)
		g.drawHUD(e)
		return
	}

	// A prompt owns the frame: no camera, no world, no hotbar until it is
	// answered.
	if g.ui.confirm != nil {
		w, ph := e.Renderer().Extent()
		scale := g.uiScale(float32(ph))
		g.handleConfirm(e, float32(w)/scale, float32(ph)/scale)
		g.daylight = daylightFrom(e.Environment().SunElevation)
		g.drawHUD(e)
		return
	}

	g.handleKeys(e)
	g.cam.Update(e.Input(), dt)
	g.followGround()
	e.SetCamera(g.cam.ViewVectors())

	// The interface gets first refusal on the pointer: a click on the hotbar
	// must not also land on the tile behind it.
	g.updateUIHover(e)
	g.updateHover(e)
	g.handleClicks(e)
	g.updateCursor(e)

	// Sampled here so FixedUpdate, which can run zero or several times a
	// frame, always sees one consistent value.
	sun := e.Environment().SunElevation
	g.daylight = daylightFrom(sun)
	g.updateLights(e, sun)
	e.Renderer().UpdateParticleInstances(g.stepSteam(dt))

	g.drawHUD(e)
}

// FixedUpdate advances the economy on the simulation tick.
func (g *Game) FixedUpdate(_ *glyph.Engine, dt float32) {
	g.Colony.Tick(float64(dt), g.daylight)
}

// followGround keeps the camera's focus on the surface, so panning across a
// mountain range does not bury the view inside it.
func (g *Game) followGround() {
	a := world.Layout.At(g.cam.Focus.X(), g.cam.Focus.Z())
	target := g.Map.SurfaceY(a)
	if !g.Map.Contains(a) {
		target = world.SeaY()
	}
	// Eased rather than snapped: a hard follow makes every cliff edge a jolt.
	g.cam.Focus[1] += (target - g.cam.Focus[1]) * 0.15
}

// daylightFrom turns the sun's elevation into a solar output factor. The
// slope is steeper than the elevation itself so panels reach full output in
// mid-morning rather than only at local noon, and it is clamped at zero so
// nothing is generated after dark.
func daylightFrom(sunElevation float32) float64 {
	d := float64(sunElevation) * 1.6
	if d < 0 {
		return 0
	}
	if d > 1 {
		return 1
	}
	return d
}

// pointer is where the game should treat the mouse as being: the real cursor,
// or the pinned position when one was given for a reproducible capture.
//
// Both the world pick and the interface hit test go through this. They used to
// disagree — only the world honoured the override — which meant a screenshot
// could show a highlighted tile and never the tooltip belonging to it.
func (g *Game) pointer(e *glyph.Engine) (x, y float64) {
	if g.cfg.CursorX >= 0 && g.cfg.CursorY >= 0 {
		w, h := e.Renderer().Extent()
		return float64(g.cfg.CursorX) * float64(w), float64(g.cfg.CursorY) * float64(h)
	}
	return e.Input().MousePos()
}

// updateHover works out which tile the mouse is over.
func (g *Game) updateHover(e *glyph.Engine) {
	mx, my := g.pointer(e)
	origin, dir := e.ScreenRay(mx, my)
	g.intent.hover, g.intent.hovering = Pick(g.Map, origin, dir, 0)
}

// Shutdown releases the resources initHUD created. The engine calls it before
// tearing the renderer down, which is the only point at which destroying them
// is safe.
func (g *Game) Shutdown(e *glyph.Engine) {
	if g.hud == nil {
		return
	}
	if g.hud.text != nil {
		g.hud.text.Destroy(e.Renderer())
	}
	if g.hud.mesh != nil {
		e.Renderer().DestroyMesh(g.hud.mesh)
	}
}

// setStatus shows a line in the HUD for a few seconds.
func (g *Game) setStatus(format string, args ...any) {
	g.ui.status = fmt.Sprintf(format, args...)
	g.ui.statusUntil = g.elapsed + 3.0
}
