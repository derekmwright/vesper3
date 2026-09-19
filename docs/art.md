# Art and typography

*Part of the [Vesper III](../README.md) documentation: the icon atlas, the panel bezel, the terrain detail atlas, the structure models, and the typeface.*

```
assets/icons.png       the structure icon atlas the hotbar samples
assets/icons-src/      the icons as separate files, for regenerating the atlas
assets/panel.png       the nine-slice bezel behind every panel
assets/logo.png        the title card badge
assets/fonts/Exo2.ttf  the interface typeface
cmd/iconatlas          packs icons-src into icons.png
cmd/trimart            crops generated art to its opaque content and resizes it
```

`cmd/trimart` exists because generated art arrives with the subject floating in
a transparent margin. That is harmless for an icon and wrong for a nine-slice:
the slice takes its corner regions from the image corners, so a frame that
stops short of the edge gets corners made of empty space and edges that stretch
the margin along with the bezel. It also reports the border thickness at each
edge midpoint, which is what `frameInset` has to be set from.

The panel bezel is drawn in the UI pipeline's *texture* mode, not its panel
mode, even though it is a nine-slice. Panel mode paints every transparent texel
as `tint * 0.2` at 70% alpha, and a frame quad is mostly transparent — the
artwork band is about a third of the corner region — so it drew a light grey
ring all the way around the inside of every panel, between the metal and the
interior (measured `#656768` against a `#2E363C` interior). Texture mode
multiplies by the texel, so transparent texels contribute nothing and only the
frame is drawn; the fill comes from a backing quad this code controls, which is
what makes the backdrop a colour rather than a fixed wash.

The backing runs the **full** panel rectangle, under the bezel rather than
inset inside it. Inset by even a couple of units it left a strip along every
edge covered by neither the backing nor the artwork — the bezel carries a small
transparent margin of its own — and the map through that strip read as a second
grey border.

Both are optional at runtime: a missing bezel falls back to the flat panels the
layout was built against, and a missing logo leaves a type-only title card.

The icons were generated with an image model and packed by `cmd/iconatlas`
into a single 512x640 texture, because the whole interface is otherwise one
draw call and a second texture would mean a second. Two sets share it: the
first ten cells are the hotbar in `colony.Buildable` order, and the seven after
them are the resources the status panel labels its rows with.

The cell a source lands in is the number on the front of its filename, so the
ordering is visible in a directory listing:

```
01-habitat.png .. 10-synthesizer.png   the hotbar
11-water.png .. 17-vespite.png        the resource rows
```

Those numbers are read as numbers, not sorted as text. Sorting by name worked
while there were eight sources and would have quietly put `10-` before `2-` on
the ninth — the kind of break that shows up as a wrong picture rather than as
an error. Two files claiming one cell is refused outright, because one would
overwrite the other and the atlas would still look plausible.

A test checks the atlas divides into the grid the UV maths assumes and that
there is art for every cell the HUD refers to. Both matter: regenerating with a
different `-cols` would leave every slot showing the right name against the
wrong picture, and adding a structure without adding an icon would push the
resource cells along by one.

To change an icon, replace the file in `assets/icons-src` and re-run:

```
go run ./cmd/iconatlas -src assets/icons-src -out assets/icons.png
```

Icons are optional at runtime. A build whose atlas fails to load logs it and
falls back to text-only slots rather than refusing to start.

The typeface is [Exo 2](https://fonts.google.com/specimen/Exo+2), bundled under
the SIL Open Font Licence — the licence travels with it in
`assets/fonts/OFL.txt`, and a test fails if it ever stops doing so. The MSDF
atlas is generated from the TTF at startup rather than shipped pre-baked, so it
cannot go stale against the font it came from. Exo 2 is proportional, so every
figure in the panel is right-aligned to a column edge: left-aligned, a number
moves its own last digit each time the value changes, which turns a readout
into a flicker.

## Texturing the hexagons

```
assets/terrain-src/*.png    four greyscale detail patterns, 480x480
assets/terrain-detail.png   the atlas the game embeds
```

The tiles are flat-shaded colour, and the colour is the biome. The detail atlas
adds surface without touching that, because the lit shader multiplies:

```glsl
vec3 baseColor = fragColor * texSample.rgb;
```

So every texel is a **multiplier**, and white means "leave this tile the colour
it already is". The patterns sit between 0.86 and 1.0 — which is what "lightly
textured" turns out to mean in numbers. A coloured pattern here would give
colour times colour: muddy, and darker than either. `cmd/terrainatlas` refuses
a source that is too dark to be a multiplier.

Which pattern a tile gets is **not** decided by its terrain. It is a hash of
the tile's position into one of four patterns and one of six rotations, so
neighbouring tiles of the same biome do not repeat. Six rotations because those
are the ones that map a hexagon onto itself; any other angle would show as a
pattern sitting crooked on its tile. The hash is stable, so terraforming a tile
or reloading a save does not reshuffle the map's own texture.

Walls and submerged caps sample a white corner deliberately — detail belongs on
the surfaces the light falls on, and a cliff face that took it would read as
dirt.

### The gutter

A cell is 512 pixels holding a 480-pixel pattern, leaving a 16-pixel white
margin. That margin is the whole reason the atlas works: at mip levels above
zero a sample near a cell edge averages in whatever is beyond it, and without
the margin that is the neighbouring pattern. It shows up as a faint seam around
every hexagon — visible only at distance, which is exactly where this camera
sits.

It is also the easiest thing in the project to destroy by accident. Rebaking
with a packer that scales each source *to fill* its cell produces a file that
is the right size, loads perfectly, and is wrong. `TestEveryCellKeepsItsWhiteGutter`
measures the bounding box of everything that is not white and fails if it
reaches into the margin.

That test had to be written twice. The first version probed the margin for
white and passed on a deliberately broken atlas — these patterns are round
blobs that fade to white at their own edges, so a stretched cell *still* reads
white in its corners. Probing could not tell. The extent is the thing that
actually has to fit.

```
task terrain      # repack and check
```

## Structure models

```
assets/models-src/*.glb  the authored models, as they come out of the DCC tool
assets/models/*.glb      what the game embeds: the same models, textures baked down
```

Structures are loaded from glTF when a model exists for them and fall back to
the procedural geometry in `internal/meshgen` when it does not, so the set can
be replaced one at a time rather than all at once — and a build with no
`assets/models` at all still runs.

A model's footprint radius has to stay under 0.80 metres. A hexagon of
circumradius 1 has an inradius of 0.87, so a model at 1:1 reaches almost to its
own tile edge and a row of them reads as one continuous mass; `modelScale`
brings them to about where the procedural shapes sit, which is what makes the
modelled and procedural sets look like one game.

A glTF exporter splits a mesh by material, so a three-material building arrives
as three primitives. Those become three entities sharing one transform, which
is why `buildingEnt` maps a tile to a *slice*.

### The bake

Models are authored with 2048x2048 base-colour and surface maps. That is the
right thing to keep — a source that has been thrown away cannot be re-cut, and
the next screen is always bigger — and the wrong thing to ship. A structure
covers about a hundred pixels at the camera distance this game is played at, so
eight pairs of 2048 maps were spending 55 MB to describe detail nobody sees,
and took the release binary to 72 MB.

So `assets/models-src` is the source and `assets/models` is the bake:

```
task models:bake      # or: go run ./cmd/texscale -src assets/models-src -out assets/models -size 512
```

`cmd/texscale` rewrites the glTF with its textures capped, copying geometry
through untouched and preserving every field it does not understand — a glTF
carries extensions, and a rewrite that silently dropped one would be worse than
not rewriting at all. 55 MB of textures becomes 7.4 MB and the release binary
goes from 72 MB to 15 MB. At three times magnification the two are
indistinguishable.

### The art contract

Three structures are animated by *material*. The game finds the battery bank's
four charge strips and its status lamp, the condenser's fin band, and the
greenhouse's grow lights by **material name** — `Charge_Runtime_2`,
`CondenserPulse_Runtime`, `GrowLight_Runtime` — and drives that primitive's
emission from the simulation.

That makes the art load-bearing, and it fails *silently*. A model re-exported
with its indicator merged into the body loads perfectly and simply never lights
up.

```
task models:check     # or: go run ./cmd/modelcheck assets/models-src/*.glb
```

This is not a hypothetical failure. An earlier version of this project kept the
models under a set of procedural Blender scripts, and those scripts went stale:
they produced two materials where the shipped battery had six. Running them
regenerated all eight models, replaced every textured one with an untextured
one, and reported success. The models were recovered by scanning a stale binary
for glTF headers.

#### It used to be colours, and that is the more useful story

`renderer.ModelMesh` did not keep the material name, so appearance was the only
handle there was and a marker was a reserved base colour — `(0.015, 0.55, 0.85)`
meant the battery's second charge strip. Matched with a tolerance of 0.002,
because a colour makes a float32 round trip through the exporter and an exact
comparison fails on a file that is correct.

The tolerance was the whole problem:

- **Invisible in the art.** Nothing in a modelling tool says 0.55 is
  load-bearing. It looks like a colour someone picked.
- **Not greppable.** `Charge_Runtime_2` turns up in the model, the exporter and
  the game. `0.55` turns up everywhere.
- **One namespace.** Every driven part in the game needed a globally distinct
  colour, because the RGB cube was all there was.
- **Silent and late.** A re-export a thousandth off loaded fine and stopped
  lighting up weeks later.

The dusk lamp was worse: there was no reserved colour for it at all, just a
heuristic picking whichever part was warmest. It worked, and degraded in the
worst way available — a model with nothing warm in it never lit, and the symptom
was an unexplained dark patch in a colony at night.

[glyphengine#21](https://github.com/derekmwright/glyphengine/issues/21) was
filed with that as the evidence, and `ModelMesh` keeps the name now. The
matching is string equality; the tolerance is gone; `internal/artcheck` is
shorter than the version that replaced it.

`internal/artcheck` holds the names and the rule for reading them, in a package
with no engine dependency so a command-line tool can use it. The tests in
`internal/game/models_test.go` assert the shipped art through the game's own
matchers rather than restating the names, so renaming a marker renames the test
with it — and `task check` runs the lot.

