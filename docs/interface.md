# The interface

*Part of the [Vesper III](../README.md) documentation: input, the menus, the title card, and the scale of the HUD.*

## Pointing and clicking

Right-drag orbits the camera and right-click demolishes, which is a conflict:
the same button does both. They are told apart by whether the pointer moved —
a release within five pixels of the press is a click, anything further was an
orbit. The slop matters. At zero, a hand releasing a button moves enough to
swallow about half of all intended clicks, and the player concludes demolish is
unreliable rather than that their mouse wobbled. `camera_test.go` pins it.

Demolition also asks first, because it is the only irreversible action in the
game and the only one a stray drag can trigger. The prompt says what comes back
and what it cost, and takes Enter or Escape as readily as the mouse.

Escape is deliberately **not** handed to the engine as `WithQuitKey`. The
engine's binding fires before the game sees the key, so cancelling a
confirmation would have quit instead. The game handles it, where it can see
what is open: the confirmation takes it first, then the pause menu, and only
the Exit item in that menu actually closes the window. Escape used to quit
outright, which is a thing you do to a player exactly once.

`Shift` and the wheel turn the structure about to be placed, a sixth of a turn
at a time — the grid's own symmetry, so it always sits square on its hexagon.
The heading is stored with the building and comes back on reload.

This replaced a per-tile hash that rotated each structure to one of the same six
orientations automatically, to stop a row of habitats looking stamped. That read
as a colony nobody surveyed, because the structures are not hex-symmetric.
Variety was worth having; deciding it for the player was not.

The scroll wheel does two jobs, and the one that does not happen is as
deliberate as the one that does: `handleKeys` consumes the scroll before
`Camera.Update` runs, so a shift-wheel notch turns the building **instead of**
zooming. Both read the same one-shot value, so whichever asks first gets it —
the call order in `Game.Update` is the entire mechanism.

## The front end

```
splash  ->  main menu  ->  playing  <->  paused
```

A main menu with New Colony, Load Colony and Exit; Escape during a game opens
the same widget with Resume, Save, Load and Exit to Menu. Both are one `menu`
type — a title and a column of choices, driven by the keyboard and the pointer
at the same time, sharing one highlight so the two can never disagree about
what Enter would take.

The widget holds no actions. `update` returns the id of what was chosen and
`session.go` decides what that means, which is what lets the whole thing be
tested without constructing an `Engine`.

### There is no scene swap, and there does not need to be one

Worth being explicit, because "main menu" usually implies loading a different
scene. glyphengine has no such concept: an `Engine` owns exactly one `*Scene`,
created once, and the `Game` interface it drives is `Init` plus `Update` for
the lifetime of the process. There is no `SetScene`, no scene stack, nothing to
unload.

So this is a state flag inside one scene:

- **One set of GPU resources for the whole process.** The font, the icon atlas,
  the structure meshes, the water surface and the chunk meshes are all built in
  `Init` and never rebuilt. None of them depend on which world is loaded.
- **`screen` gates input and simulation.** `Update` returns early when a menu
  is open, so the camera, the pick ray and the hotbar never see the frame;
  `FixedUpdate` returns early too, so a paused colony does not quietly drink its
  water while the player reads the menu.
- **The world is replaced in place.** `startWorld` regenerates the map, clears
  the building entities, re-uploads every chunk and refreshes the heightmap the
  water shades against. That is the same operation terraforming already does on
  one tile, at the scale of all of them.

Which is not a workaround. Chunk meshes are dynamic and sized for a fixed grid
precisely so they can be rewritten; tearing them down to build identical ones
would be work for its own sake. The one real constraint is that the grid is
fixed at startup — which is also why loading a save from a differently sized
world is refused outright rather than half-applied.

**But two parts of it were, and got filed.** The distinction is the whole job
of a pathfinder: "this game did not need scene management" is a conclusion
about this game, and the question that matters is whether the next adopter
hits a wall here.

- [#35](https://github.com/derekmwright/glyphengine/issues/35) — there is no
  way to pause the *engine's* simulation. `Scene.Tick` runs unconditionally,
  before `FixedUpdate`, so a game that returns early has stopped its own
  simulation and none of the engine's: physics, character controllers,
  interpolation, animation. This game is unaffected and filed it anyway,
  because it is unaffected by luck — its entire simulation happens to live in
  its own `FixedUpdate`. The first game on this engine with a rigid body and a
  pause menu finds a crate still sliding behind it, and nothing errors.
- [#36](https://github.com/derekmwright/glyphengine/issues/36) — `ui.UIManager`
  has no focus traversal. `Clickable` is mouse-only and `Focusable` exists for
  text entry, so there is no arrow-key or gamepad movement between widgets.
  Which is why the menus here are hand-rolled rather than built from `ui`: the
  one behaviour a main menu must have is the one the toolkit does not offer.

What was *not* filed matters too. The camera not being set on menu frames was
this game's bug, not the engine's — `Update` returned before `SetCamera` and
the engine had simply never been given a view. And "add scene management" is
not an issue, because nothing here needed it and a speculative request is worth
less than no request.

The main menu draws over a generated world with the camera framed on the whole
continent and turning slowly, about three minutes to the revolution. A still
image would be cheaper and would look like a photograph of the game instead of
the game. Leaving to the menu generates a *new* world rather than keeping the
abandoned one, so the backdrop is never the colony that was just given up on.

### The buttons came with a contract

The menu rows are nine-slice artwork — four states in `assets/ui/buttons`, and
the set arrived with `buttons.json` and `validation.json` beside it: size,
inset, minimum drawn size, per-state label colours, the order states resolve
in, and a sha256 for each PNG.

That is worth more than the pictures. `button_test.go` checks the *code*
against that manifest rather than against numbers copied out of it — the
constants in `button.go`, the state priority, which file each state loads, that
all four share a silhouette so a state change recolours a button instead of
moving it, and that the art still hashes to what was recorded. Art and code
cannot drift apart quietly, which is the same problem `internal/artcheck`
solves for the structure models and the same problem the terrain atlas's gutter
test solves.

One number in it is load-bearing: the inset is 24 on all four sides of a
96-tall source, so a button drawn shorter than 48 units has its top and bottom
corner regions overlapping and the metal folds in on itself. The documented
minimum is 56. The menu's rows were 46 when they were plain rectangles, and a
test now fails if any button in the game drops below the floor.

The README that shipped with the art also asked for texture mode with a white
tint rather than panel mode, "to retain the artwork rather than a panel shader
that replaces the center fill" — which is the same fight documented over the
bezel in `hud.go`, arrived at independently by someone drawing the art.

The resource panel is hidden on the main menu and kept on the pause menu. On
the main menu it would be furniture from a game nobody is playing — 0 of 0
colonists, an advisory telling nobody to build a habitat. On the pause menu the
numbers are real, and checking them is half the reason to pause.

`-nosplash` skips the whole front end and opens straight into a game, which is
what every capture command in this project wants.

## The title card

`-nosplash` skips it; a click or space dismisses it; it fades after a couple of
seconds. It is a title card and not a loading screen, and the difference is
worth being straight about: `Game.Init` loads the whole world synchronously
before the first frame is drawn, so by the time anything can be shown there is
nothing left to wait for. A progress bar here would be counting to a number
that is already reached.

One thing worth knowing if you change any interface colour: overlays are
composited into an sRGB swapchain, so the values a shader writes are LINEAR and
the hardware encodes them on the way out. A linear 0.05 lands at about 0.24 on
screen — a mid slate, not the near-black it reads as in source. Every colour in
`hud.go` therefore goes through `srgb()`, which takes the value as it should
look and converts it, so what the constant says is what the pixel is.

The engine documents the opposite ("a UI colour is an sRGB value that reaches
the display as written"), which is
[glyphengine#13](https://github.com/derekmwright/glyphengine/issues/13).

## Interface scale

The whole HUD is laid out in *design units* and multiplied by one scale factor
where each quad and each line of text is emitted, so the layout arithmetic
stays readable at any size. The scale comes from the window height by default —
the design is drawn against 900px, so a 2160px window gets 2x — and `[` and `]`
step it live. `-uiscale` fixes it.

Raising the scale shrinks the design-space window, which is what makes the
layout responsive rather than merely bigger:

- the left column stacks the readout, the advisories and the tile inspector,
  and drops from the bottom when there is no room — advisories outrank the
  inspector, because they are the only thing on screen that says what to *do*;
- the readout compacts, shedding the making/using sub-lines but keeping the
  bars, the net rates and any countdown;
- the build bar wraps onto a second row rather than squeezing slots below the
  width a structure name needs.

If you run at 2x or above, give it a tall window — at 900px the readout has to
compact to fit.

