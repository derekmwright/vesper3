# How it is put together

*Part of the [Vesper III](../README.md) documentation: the package split, the event bus, the geometry, and the dusk lighting.*

```
internal/hex       flat-top axial coordinates, layout, neighbours, rounding
internal/world     terrain, tiles, the map, procedural generation
internal/colony    what can be built, where, and the economy that results
internal/meshgen   hex chunks and structure geometry -> renderer vertices
internal/event     a small synchronous publish/subscribe bus
internal/game      the only package that knows an Engine exists
```

That split is the engine's own rule 8 applied to a game: `hex`, `world`,
`colony`, `event` and most of `meshgen` import no engine code and are tested
without a GPU. Game components live in the game's own structs on the engine's
`ecs.World` rather than being added to `Scene.C`.

Inside `internal/game`, one file per duty:

```
game.go          Game, Config, the frame loop
events.go        the event vocabulary and every subscription, in one place
scene.go         the GPU's copy of the world: meshes and entities
terrain.go       chunk meshes, the cursor, the placement ghost
actions.go       input -> intent -> the colony
camera.go        the RTS camera
pick.go          screen ray -> tile, without a physics engine
lights.go        the colony going dark, and lighting itself
activity.go      what a structure looks like while it is working
hud.go           the widget toolkit: text, quads, panels, bars
hud_frame.go     what gets drawn each frame, and what is dropped when it will not fit
panel_colony.go  the resource readout, advisories and tile inspector
hotbar.go        the build bar
tooltip.go       the hovered structure's card
confirm.go       the one modal: demolition
splash.go        the title card
save.go          serialisation
```

## Game holds five things, not thirty

`Game` sits where the world, the renderer, the lights, the interface and the
player's intent all meet, which is exactly the type that grows into a pile of
loose fields nobody can read. It holds each of those as a named group instead:

```go
type Game struct {
    cfg    Config
    Map    *world.Map      // the truth
    Colony *colony.Colony  // the truth
    cam    *Camera
    bus    *event.Bus
    scene  scene           // what the renderer has been told
    lights lighting        // the dusk pass
    intent intent          // what the player is about to do
    ui     uiState         // what the interface itself is doing
    hud    *hud
    splash splash
    ...
}
```

Each group is declared in the file that drives it, so `g.intent.facing` can be
found by looking where facing is changed. The test for whether a field is in the
right group is what changes it: `intent` is written by the keyboard and the
world pick, `ui` by the layout, `scene` by placement and demolition.

## Events for things that happen, polling for things that are true

`internal/event` is a synchronous, type-keyed bus: `event.On(bus, func(ev T))`
to subscribe, `event.Emit(bus, ev)` to publish. Delivery is in registration
order, on the caller's goroutine, and complete before `Emit` returns — a game
loop wants that, and an async bus would mean an event raised on the frame a
building went up might be handled on the next one.

What it is for is the discipline, not the plumbing:

- **"A structure was placed" is an event.** It happens at an instant, it is
  gone once handled, and nobody can ask about it later.
- **"The colony has 250 iron" is state.** It is true continuously, anyone can
  read it whenever they like, and publishing it would create a second copy that
  can disagree with the first.

So the resource panel subscribes to *nothing*. It reads `Colony.Readout` every
frame and draws what it finds. Routing that through a bus would buy nothing and
add a cache to invalidate.

Placement is where the bus earns its keep. It used to read: place it in the
colony, spawn its entities, write a status line — three unrelated concerns in
one function, in an order nobody could derive from the rules, and a path that
forgot the middle one left a building that existed and was never drawn. Now:

```go
func (g *Game) place(_ *glyph.Engine, a hex.Axial) {
    k, facing := g.intent.selected, g.intent.facing
    if err := g.Colony.PlaceFacing(g.Map, k, a, facing); err != nil {
        emit(g, ActionRefused{Err: err})
        return
    }
    emit(g, StructurePlaced{Kind: k, At: a, Facing: facing})
}
```

The colony is the authority on whether the structure exists. Once it says so,
this publishes the fact and stops. Drawing it and announcing it are two other
systems' business, and both are registered in `subscribe` — which is the one
function to read to find out what reacts to what.

One deliberate non-use: **loading a save does not publish placements.** Nothing
was built and nothing was paid for, so announcing forty structures going up
would be false. The load rebuilds the scene directly and reports itself. An
event is a fact about something that happened, and restoring a saved world is
the world being replaced.

A few decisions worth knowing about if you change something:

**The grid is flat-top axial, and corner order is load-bearing.** The edge
between corner *i* and corner *i+1* is the edge shared with neighbour *i*, which
is what lets the cliff mesher walk corners and neighbours with one index. A test
pins it. Using the pointy-top corner phase on this grid silently shears every
cliff half a tile off its edge.

**Winding is derived from the engine, not reasoned about.** The renderer culls
with `FrontFace: Clockwise` under a Y-flipped projection, so the rule every face
follows is *the triangle's cross product points opposite its surface normal* —
taken from the engine's own `CreatePlane`. Getting it backwards does not glitch
or warn, it makes geometry vanish. `TestEveryTriangleWindsAgainstItsNormal`
recomputes it for every triangle of a real map, and `Builder.Tri` flips the
winding for you so the structure definitions never have to think about it.

**Terrain is chunked and dynamic.** 8×8 tiles per chunk, one draw call each,
uploaded through `CreateDynamicIndexedMesh` so terraforming rewrites a chunk in
place instead of churning a GPU allocation per keypress. `UpdateMeshData`
silently *truncates* past the capacity it was created with, so
`meshgen.ChunkCapacity` sizes the buffer and two tests hold the mesher to it —
one for the worst-case chunk, one that measures what a single fully walled tile
actually costs, because the first can never reach the bound.

**Picking does not use the physics engine.** Giving 2,304 tiles a collider
apiece to answer one query per frame would be a lot of machinery for a question
the map can answer directly, so `game.Pick` marches the screen ray and bisects
the crossing. A ray that hits a cliff face returns the tile on top of it.

**The ledger is float64.** Rendering is float32 because the GPU is; the economy
is not, because at a five-figure stockpile float32 quietly stops counting small
per-tick increments.

**The placement ghost is the real structure with its material stripped off.**
The lit shader multiplies vertex colour by the per-entity tint, and a modelled
structure carries its material colour on that same tint — so a ghost is the
structure's own meshes with the texture and material dropped, drawn translucent
under a green or red `Color`. One mesh serves both states. It used to be a crude
procedural stand-in (a dome was a hemisphere, a mine was a box), which said
where a building would go but nothing about what would be going there.

A glTF exporter splits a mesh by material, so a structure arrives as several
primitives. They are concatenated into a single mesh at load — same vertices,
same winding, indices rebased as each is appended — so the preview is one
entity and one draw with a single consistent alpha.

It was one translucent entity *per primitive* until
[glyphengine#19](https://github.com/derekmwright/glyphengine/issues/19), because
`LoadGLTF` returned GPU handles and discarded the geometry it had just decoded,
leaving nothing to merge on the CPU side. That cost a draw per primitive and,
worse, made a structure blend against itself: where two of its own parts
overlapped the preview went denser. That was rationalised at the time as reading
like a hologram. It read like a bug, and it was one.

Anything with no model still falls back to `meshgen.Ghost`, which builds the
procedural shapes in flat white — the procedural meshes paint themselves in
vertex data, so those genuinely cannot be tinted.

## Lights at dusk

As the sun goes down a warm point light appears at every structure and the
structure's own amber accent starts to glow — the light is what falls on the
ground around it, the glow is what makes the building read as occupied rather
than as a shape with a lamp beside it. The accent that lights up is chosen by
colour rather than by index, so a remodelled structure keeps working as long as
it keeps one warm part.

Both are scaled by the same number, and **that number includes the power
grid**:

```
lamps = dusk(sunElevation) * powerSatisfaction
```

Satisfaction counts the battery bank, which is the point: a solar colony after
dark generates nothing at all and stays lit because it bought storage. An
earlier version tested power *supply* here and blacked out precisely the colony
that had solved the problem — the test suite carries that case now.

Either factor at zero means darkness. A blackout at noon costs nothing visible
because the lamps were not on anyway; a blackout at midnight puts the colony
out. A brownout dims it by exactly the fraction the grid is short, because that
is what satisfaction already means everywhere else.

Which makes the solar-versus-geothermal lesson something you can see on the map
instead of only reading in the panel: build nothing but solar arrays and night
falls on a colony that goes completely dark. Both cases are pinned by tests,
since the coupling is a rule rather than an effect.

Lamps are deliberately dim — 0.16 of the magnitude the engine's lighting
example uses. They accumulate, being unshadowed and additive, and at example
strength a dozen structures blew the ground out to flat white and turned the
buildings into silhouettes against it. One lamp should light its own tile and
tint its neighbours, and the sum of a colony should still be night. The engine
takes at most 32 point lights and truncates the rest, so the nearest to the
camera are the ones kept.

`-timeofday` and `-daylen` exist for looking at this: `-timeofday 0.8 -daylen -1`
freezes the clock just after sunset, which is the only way to capture a
particular hour twice.

