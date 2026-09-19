# Vesper III

**A showcase game for [glyphengine](https://github.com/derekmwright/glyphengine).**

A colony builder on a hex grid, in true 3D, on a planet that is not Earth.

The camera looks down on the world the way a strategy game does, but what is
under it is real geometry: hexagonal columns with cliff faces, cast shadows, a
day/night cycle that decides whether the solar arrays are producing, and a
methane sea with waves in it.

## What this is for

glyphengine is a Go game engine on Vulkan. This is the first complete game
built on it from outside, and it exists to answer two questions that an
engine's own examples cannot:

**What does the engine look like in the hands of someone who did not write it?**
Every example in an engine repository is written by the person who knows which
call to reach for. This one was not. Where the engine is good, that shows up
here as code that is short and obvious. Where it is not, it shows up as a
workaround with a comment explaining what was in the way — and usually an issue
number beside it.

**What does a whole game need that a demo does not?** A rotating cube needs a
mesh and a matrix. A game needs a save format, an interface that stays readable
at four different window sizes, art that survives a re-export, a build that
someone else can run, and a way to reproduce a screenshot six weeks later. Most
of the interesting decisions in this repository are about those, not about
Vulkan.

It is a real game, not a technology demo. It is played rather than watched, the
economy closes, and the parts you would expect to be faked are not.

### If you are reading this to learn the engine

The code is commented for that. Comments here explain *why*, not what, and the
ones worth finding are the ones that start with a mistake — the winding
convention derived from the engine's own `CreatePlane` rather than reasoned
about, the mesh capacity that silently truncates, the sRGB conversion that was
right until the engine fixed the thing it was compensating for.

Suggested reading order:

| | |
|---|---|
| `internal/hex` | no engine dependency at all. Start here to see the shape of the project. |
| `internal/world`, `internal/colony` | the simulation. Also no engine dependency, and tested without a GPU. |
| `internal/meshgen` | geometry to `renderer.Vertex`. Where the engine's conventions start to matter. |
| `internal/game/game.go` | the frame loop, and the only type that knows an `Engine` exists. |
| `internal/game/events.go` | every subscription in the project, in one function. |
| `internal/game/hud.go` | the interface toolkit, and the sRGB story. |

The map of the whole tree, and the decisions behind it, is in
[docs/architecture.md](docs/architecture.md).

The captures live in `docs/`, beside the rest of the documentation. Every one
was taken by the game itself with `-screenshot`, and the flags to re-take it
are in the caption.

![A colony on the coast in daylight: domes, solar arrays, a greenhouse, a geothermal plant and a mine, with the resource panel and the ten-slot build bar around them. A red hexagon marks a tile that is already built on.](docs/hud.png)

*A colony in the afternoon. The red hexagon is the cursor refusing an occupied
tile — the placement preview itself stays off a tile that has something on it,
because the preview is the real structure mesh and drawing it there reads as
two buildings in one place rather than as a refusal. The panel reads left to
right as stock against capacity, then rate; the strip under it names the
building that fixes the problem rather than restating the number.*

![The same colony after dark: cyan grow lights in the greenhouse, amber deck lamps pooling on the ground, and a translucent green dome showing where the next habitat would go](docs/dusk.png)

*The same colony after sunset, and the reason the light budget is worth
measuring. The lights are not decoration — lamp brightness is the power grid's
satisfaction, so a colony that cannot cover its own demand after dark goes
dark. The battery banks show their charge on four strips and their state on the
lamp above them; the greenhouse's grow lights are on because it has the power
to run them. The green dome is the placement preview on ground that will take
it.*

![The continent of Vesper III from the top of the camera's zoom, hexagonal terrain in ochre, green and slate running to a methane sea](docs/vesper.png)

*The whole continent. Every tile is real geometry with cliff faces and cast
shadows, chunked 8x8 into one draw call each.*

![The title card: a hexagonal badge over a ringed planet, with a lit colony dome on the horizon](docs/splash.png)

*All four were captured by the game itself with `-screenshot`. `-camdist`,
`-campitch`, `-camyaw`, `-cursorx`, `-cursory` and `-timeofday` pin the camera,
the pick ray and the clock, so a capture can be re-taken exactly instead of
depending on where the mouse happened to be and what time the run reached.
`-demo` puts the colony there. The exact commands are in the Taskfile under
`task shot`.*

## Running it

```
task run                        # a new planet from a random seed
task run -- -seed 20260916      # the same planet every time
task run -- -cols 64 -rows 64   # a bigger continent
task run -- -demo               # a sample colony already standing, for screenshots
task run -- -debug              # open with the F3 readout already up

task            # everything there is to run, listed
task check      # formatting, vet, tests, and the art contract
task dist       # one executable you can hand to someone
```

Plain `go run .` works too; the [Taskfile](Taskfile.yml) is a collection of the
commands this was actually built with rather than a layer over them.

**To build:** Go 1.26+ and CGo with a C compiler, because GLFW and the Vulkan
wrapper are cgo. **To run:** a GPU and driver with Vulkan 1.1.

Windows and Linux are built and tested. macOS has everything it needs on the
engine side — the portability opt-ins
([#20](https://github.com/derekmwright/glyphengine/issues/20)), a real message
when no Vulkan driver is present
([#23](https://github.com/derekmwright/glyphengine/issues/23)), and mouse
coordinates that agree with the framebuffer
([#24](https://github.com/derekmwright/glyphengine/issues/24)) — but nobody has
run it on a Mac yet. `task dist:macos` builds the `.app` and bundles MoltenVK.
Reports welcome.

The engine is pinned to a commit in `go.mod` rather than a tag, because it is
v0.x and says outright that it breaks APIs without notice. Bumping it is a
deliberate act here, not a `go get -u`: the commit history records which engine
change each bump was for and what it required on this side.

### Distributing it

`task dist` produces one executable with every asset embedded — no folder
beside it, nothing to install or unpack — plus [the controls](PLAYING.md) and
the font licence:

```
dist/vesper.exe        16 MB
dist/README.md
dist/LICENSE-Exo2.txt
```

16 MB rather than 72: the models are authored with 2048x2048 maps and baked
down to 512 by `cmd/texscale` before they are embedded. See
[the bake](docs/art.md#the-bake).

## Controls

| | |
|---|---|
| `WASD` / arrows | pan (hold `Shift` to move faster) |
| `Q` `E` | turn |
| `R` `F` | tilt |
| wheel | zoom |
| right-drag, middle-drag | orbit (the drag moves the world, not the camera) |
| `C` | frame the whole continent |
| `1`–`0` | pick a structure |
| left click | build |
| `Shift` + wheel | turn the structure about to be placed |
| `Shift` + right click | demolish, with a confirmation |
| `X` | demolish mode |
| `T` | terraform mode — left click raises, right click lowers |
| `U` | upgrade mode — left click raises a structure a tier |
| `F5` / `F9` | save and load |
| `F3` | the diagnostic readout: colony figures, light binning, frame tails, memory. The resource panel steps aside while it is up |
| `F4` | cycle the light view: normal, cluster heatmap, brute-force reference |
| `[` `]` | interface scale |
| `Esc` | the menu: save, load, back to the title |

## The game

You land with a habitat and a solar array already on the ground. Colonists
start arriving to fill the habitat, and they eat, which is the first problem.

Ten structures. Every output has a consumer and every input has a source:

```
             power ──┬─▶ mine ──┬─▶ iron ──▶ everything
                     │          └─▶ crystal ──▶ batteries, geothermal
                     ├─▶ extractor / condenser ──▶ water
                     └─▶ greenhouse ──▶ food ──▶ habitat ──▶ colonists
water ──┬─▶ mine
        ├─▶ greenhouse
        ├─▶ habitat
        └─▶ geothermal ──▶ power
```

That last edge is the interesting one. A geothermal plant is a steam cycle and
the cycle leaks, so night power is bought with water, and a colony that solves
darkness has to have solved water first.

Power is the binding constraint and it is not pass/fail: a colony that
generates less than it draws runs *everything* at the ratio between the two, so
a brownout slows the mines rather than stopping them. Solar stops dead at
night, and the lamps that come on at dusk are scaled by the grid's
satisfaction — build nothing but solar arrays and night falls on a colony that
goes completely dark.

Everything but a habitat has to go down within four tiles of one, so reaching a
distant ice sheet or a thermal vent means planting an outpost first and feeding
it. Every store has a ceiling, so time alone earns nothing. Every structure
declares how many people it takes to run, so colonists are a resource like
power and water, and they leave if you stop looking after them. Lose the last
one and the game says so in one line and then leaves you alone, because the
wreck is worth reading.

The rest of the rules — two ores and one building, vespite and tiers, the
five-step resolution order, the panel that reports it all, and the wrong
versions most of these replaced — are in [docs/economy.md](docs/economy.md).

## What this fed back into the engine

This is the part that makes it a showcase rather than a sample. Building a
whole game on a young engine finds things, and every one of them was filed with
the measurement that found it rather than as an opinion: the placement preview
that had to be an opaque solid until the engine grew a blended pass, the water
that refracted the HUD, the mouse coordinates that were out by 2x at 200%
display scaling.

The full list is in [docs/engine-feedback.md](docs/engine-feedback.md) — what
each issue cost here, which are fixed upstream and in use, and why an issue
that reports a number can be argued with while an issue that reports a feeling
cannot.

## The long version

The depth is the point of this repository: the decisions, the measurements and
the mistakes are what someone learning the engine actually needs. It lives in
`docs/`, one page per subject.

| | |
|---|---|
| [The economy](docs/economy.md) | the structure catalog, two ores and one building, the build radius, vespite and tiers, the five-step resolution order, ceilings, staffing, losing, and how the panel reports it |
| [The interface](docs/interface.md) | pointing and clicking, the menus and why there is no scene swap, the title card, interface scale |
| [How it is put together](docs/architecture.md) | the package split, the event bus, winding, chunking, picking, the placement ghost, and the colony lighting itself at dusk |
| [Art and typography](docs/art.md) | the icon atlas, the panel bezel, texturing the hexagons, the structure models and their bake, the art contract, the typeface |
| [What this fed back into the engine](docs/engine-feedback.md) | every issue filed upstream, with the measurement that found it |

[PLAYING.md](PLAYING.md) is the player's sheet — it ships beside the
executable as the `README.md` in `dist/`.

## Tests

```
go test ./...
```

All of it runs without a GPU. The suite is mostly about the things that fail
silently: winding, chunk capacity, grid rounding, save round-trips, and the
generator's tuning — `TestTerrainDistribution` is what fails when a noise
constant gets changed by feel and thermal vents stop appearing, which no other
test would notice.
