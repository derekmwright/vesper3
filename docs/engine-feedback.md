# What this fed back into the engine

*Part of the [Vesper III](../README.md) documentation.*

This is the part that makes it a showcase rather than a sample. Building a
whole game on a young engine finds things, and every one of them was filed with
the measurement that found it rather than as an opinion.

**Fixed upstream, and in use here:**

| | |
|---|---|
| [#3](https://github.com/derekmwright/glyphengine/issues/3) | no generic mesh instancing |
| [#4](https://github.com/derekmwright/glyphengine/issues/4) | no `ShaderSet` passthrough on `glyph.New` |
| [#5](https://github.com/derekmwright/glyphengine/issues/5) | no blended pass, so a placement preview had to be an opaque full-bright solid. The ghost is a real translucent object now. |
| [#6](https://github.com/derekmwright/glyphengine/issues/6) | overlays were composited into the HDR scene target *before* the water, bloom and tonemap passes, so water refracted the HUD and erased most of it. Found by looking at a wide shot of this map and wondering why the text was wavy. |
| [#13](https://github.com/derekmwright/glyphengine/issues/13) | overlay colours were linear while documented as sRGB, so a HUD backdrop written as 0.05 arrived at 0.24. See the note above the palette in `hud.go` — the fix required deleting this game's compensating helper *in the same commit* as the engine bump, or the conversion applies twice. |
| [#14](https://github.com/derekmwright/glyphengine/issues/14) | panel mode hardcoded its interior fill, so a game could not choose its own panel colour |
| [#20](https://github.com/derekmwright/glyphengine/issues/20) | macOS: the instance and device missed the two portability opt-ins MoltenVK requires. Without them `vkEnumeratePhysicalDevices` returns zero devices on a Mac and it surfaces as "no GPU found" on a machine with a perfectly good one. |
| [#19](https://github.com/derekmwright/glyphengine/issues/19) | `LoadGLTF` discarded the geometry it had just decoded, so a model could only ever be a draw call. The placement ghost had to be one translucent entity per primitive, which cost a draw each and made a structure blend against *itself*. It is one merged mesh now. |
| [#21](https://github.com/derekmwright/glyphengine/issues/21) | `ModelMesh` dropped the glTF material name, leaving base colour as the only way to identify a primitive — matched with a float tolerance that was invisible in the art, not greppable, and silent when it broke. `internal/artcheck` matches names now. |
| [#23](https://github.com/derekmwright/glyphengine/issues/23) | a machine with no Vulkan driver got a null proc address rather than a message saying so. That is the default state of every Mac. |
| [#24](https://github.com/derekmwright/glyphengine/issues/24) | `MousePos` was in screen points while `ScreenRay` divided by framebuffer pixels. They agree on a 1:1 display and are out by 2x on a Retina one — or on Windows at 200% scaling, which is where it was actually observed. |
| [#35](https://github.com/derekmwright/glyphengine/issues/35) | the simulation could not be paused: `Scene.Tick` ran whatever the game did. This game paused by returning early from `Update`, which stopped the colony and none of the engine — the sun kept crossing the sky behind the pause menu. `SetTimeScale(0)` is the fix and the engine's own comment names the trap: a game that pauses by returning early gets away with it only if it has no physics and no skinned meshes. |
| [#36](https://github.com/derekmwright/glyphengine/issues/36) | `ui.UIManager` had no focus traversal, so a menu could not be driven by the keyboard |
| [#37](https://github.com/derekmwright/glyphengine/issues/37) | no cone-shaped spotlights, which is what a lamp on a pole over a doorway needs |
| [#41](https://github.com/derekmwright/glyphengine/issues/41) | `ExtractFrustum` used OpenGL's clip volume rather than Vulkan's, so the far plane was never tested. Filed from reading the source; the engine author measured it and confirmed. |

**Open, and why they matter here:**

| | |
|---|---|
| [#12](https://github.com/derekmwright/glyphengine/issues/12) | the sky palette is baked into `atmosphere.inc`, which `applyFog` shares. Changing it through `WithShaders` means vendoring 430 lines of engine lighting into this repo to edit six constants. This is why the sky over an alien planet is still Earth's. |
| [#46](https://github.com/derekmwright/glyphengine/issues/46) | `renderer.Model` exposes only `Meshes`, so the glTF nodes the loader already walks are dropped and a named empty cannot be read. A game that wants to put an effect *at* a point on a model has to hand-measure the offset out of Blender — `flare.go` carries exactly that constant for the methane stack, and it breaks silently the moment the model is re-exported. |
| [#47](https://github.com/derekmwright/glyphengine/issues/47) | light shafts are anchored to the sun disc and are screen-space, so they are gone at night. Since #37 a game can aim a spotlight anywhere, but it cannot make the beam visible — which is most of the reason to point one at anything after dark. |
| [#48](https://github.com/derekmwright/glyphengine/issues/48) | `MSDFText.SetText` builds its geometry into two slices grown from nil on every call. Measured at **73% of everything this process allocates** — about 390 KB a frame, which took the collector from idle to a GC every fifteen frames. Nothing leaks; it is pure churn, and it is the one thing between this engine and a zero-garbage frame. |

Instancing is deliberately **not** adopted yet. A colony reaches a few dozen
structures in normal play, so batching would save a few dozen draw calls
against a frame that is already comfortably inside budget — and the issue
asking for it says to measure with `task bench` before building on it, which
applies just as much to using it. Worth revisiting if colonies get into the
thousands.

## On filing engine issues from a game

Every issue above says what was measured, on what, and what it cost. Two of
them include a correction the engine author made to my diagnosis after
measuring — one where I predicted a Windows no-op and was wrong — and that is
the point rather than an embarrassment. An issue that reports a number can be
argued with. An issue that reports a feeling cannot.

Three of them said plainly that the finding was read from source rather than
from a failing run, because there was no Mac to test on. Saying so is what makes
the others trustworthy — and one of those three, #24, turned out to reproduce on
Windows at 200% display scaling, which is how it got confirmed before a Mac was
ever involved.

The traffic goes both ways. The fix for #20 carries a correction to the report:
it predicted the portability extension would be absent on Windows so that
nothing would change there, and that is wrong — it is a *loader* extension and a
current Windows loader advertises it, so the flag is set there too. Harmless,
and measured rather than assumed.

#35 and #36 came out of building the front end. What produced them, and what
deliberately was *not* filed alongside them, is in
[the interface page](interface.md#there-is-no-scene-swap-and-there-does-not-need-to-be-one).

The three most recent came out of instrumenting rather than playing, which is
worth saying because it is a different way of finding things. #48 in particular
was found by asking a question with no bug attached to it — a readout showed the
process holding 43 MB against 8 MB live, the gap turned out to be a harmless
high-water mark from start-up, and the profile taken to prove it was harmless is
what found the allocation. The thing that was flagged was fine. The thing beside
it was not.
