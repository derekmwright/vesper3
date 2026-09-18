// Command worldbuild is a colony builder on a hex grid, in true 3D, on a
// planet that is not Earth.
//
// The camera looks down on the world the way a strategy game does, but the
// world under it is real geometry: hexagonal columns with cliff faces, cast
// shadows, a day/night cycle that decides whether the solar arrays are
// producing, and a methane sea with waves in it.
//
//	go run .                      # a new planet from a random seed
//	go run . -seed 12345          # the same planet every time
//	go run . -cols 64 -rows 64    # a bigger continent
//	go run . -frames 120 -screenshot shot.png   # render and exit, for CI
//
// Built on github.com/derekmwright/glyphengine.
package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"runtime"

	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/vesper3/internal/game"
)

// assetsFS carries the game art into the binary, so a built worldbuild is one
// file with no directory to ship beside it.
//
// The patterns are explicit rather than a bare "assets", because that embedded
// the whole tree: the icon *sources* the atlas is generated from, five
// megabytes of them, rode along in every build and were never opened. go:embed
// skips paths beginning with "_" or ".", which is why the model backups do not
// need listing here — and is also the only thing that was keeping them out, so
// a backup directory named without the underscore would have shipped too.
//
// Anything added under assets/ from now on has to be named here to reach the
// binary. That is the intended trade: a missing asset is a loud failure at
// startup, a silently embedded one is a download nobody notices.
//
//go:embed assets/icons.png assets/panel.png assets/logo.png assets/terrain-detail.png
//go:embed assets/ui/buttons/*.png
//go:embed assets/fonts
//go:embed assets/models/*.glb
var assetsFS embed.FS

// version is stamped in at build time by the Taskfile:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)"
//
// It is a plain var rather than a constant because -X can only write to one.
// A build made without it says "dev", which is the honest answer for a binary
// nobody can trace back to a commit.
var version = "dev"

func init() {
	// GLFW has to be called from the thread that initialised it.
	runtime.LockOSThread()
}

func main() {
	var (
		seed       = flag.Int64("seed", 0, "world seed; 0 picks one at random")
		cols       = flag.Int("cols", 48, "map width in tiles")
		rows       = flag.Int("rows", 48, "map height in tiles")
		save       = flag.String("save", "colony.save.json", "save file path")
		width      = flag.Int("width", 1600, "window width")
		height     = flag.Int("height", 900, "window height")
		fullscreen = flag.Bool("fullscreen", false, "run fullscreen on the primary monitor")
		frames     = flag.Int("frames", 0, "render N frames then exit (0 runs until closed)")
		shot       = flag.String("screenshot", "", "write a PNG of the last frame to this path")
		validate   = flag.Bool("validate", false, "enable the Vulkan validation layer")

		camDist   = flag.Float64("camdist", 0, "opening camera distance (0 uses the default)")
		camPitch  = flag.Float64("campitch", 0, "opening camera pitch in radians (0 uses the default)")
		camYaw    = flag.Float64("camyaw", 0, "opening camera yaw in radians")
		cursorX   = flag.Float64("cursorx", -1, "pin the pick ray to this fraction across the window (<0 follows the mouse)")
		cursorY   = flag.Float64("cursory", -1, "pin the pick ray to this fraction down the window (<0 follows the mouse)")
		showVer   = flag.Bool("version", false, "print the build version and exit")
		noSplash  = flag.Bool("nosplash", false, "skip the title card")
		demo      = flag.Bool("demo", false, "put a sample colony down at the landing site, for screenshots")
		timeOfDay = flag.Float64("timeofday", 0, "start the clock here: 0.25 sunrise, 0.5 noon, 0.75 sunset")
		dayLength = flag.Float64("daylen", 0, "seconds per day; negative freezes the clock")
		uiScale   = flag.Float64("uiscale", 0, "interface scale; 0 picks one from the window height. [ and ] adjust it live")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("Vesper III", version)
		return
	}

	if *seed == 0 {
		*seed = int64(rand.Uint64()>>1) + 1
	}

	g := game.New(game.Config{
		Seed:      *seed,
		Cols:      *cols,
		Rows:      *rows,
		SavePath:  *save,
		Demo:      *demo,
		Assets:    assetsFS,
		CamDist:   float32(*camDist),
		CamPitch:  float32(*camPitch),
		CamYaw:    float32(*camYaw),
		NoSplash:  *noSplash,
		TimeOfDay: float32(*timeOfDay),
		DayLength: float32(*dayLength),
		UIScale:   float32(*uiScale),
		CursorX:   float32(*cursorX),
		CursorY:   float32(*cursorY),
	})

	opts := []glyph.Option{
		glyph.WithTitle("WorldBuild - Vesper III"),
		glyph.WithWindowSize(*width, *height),
		glyph.WithMSAA(4),
		// Escape is not handed to the engine as a quit key: it also backs out
		// of the demolish prompt, and the engine's binding fires first — so
		// cancelling a confirmation would have closed the game. The game
		// handles it in handleKeys, where it can see whether anything is open.
		// The engine defaults to a 500-unit far plane. A continent seen from
		// the top of the camera's zoom is further away than that, and the
		// horizon would otherwise be cut off mid-map.
		glyph.WithProjection(52, 0.1, 1200),
		glyph.WithValidation(*validate),
	}
	if *fullscreen {
		opts = append(opts, glyph.WithFullscreen())
	}
	if *frames > 0 {
		opts = append(opts, glyph.WithMaxFrames(*frames))
	}
	if *shot != "" {
		opts = append(opts, glyph.WithScreenshot(*shot))
	}

	e, err := glyph.New(g, opts...)
	if err != nil {
		log.Fatalf("create engine: %v", err)
	}
	defer e.Destroy()

	e.Run()
}
