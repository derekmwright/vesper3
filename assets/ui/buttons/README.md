# Vesper III button assets

Four unlabeled 256 x 96 RGBA textures matching panel.png: slate metal, clipped corners, pale edging and small amber inserts. Normal is raised; hover brightens the face and edge; pressed reverses the bevel for a recessed appearance; disabled mutes contrast and the amber accents.

## Nine-slice

Uniform 24 px source inset. At scale 1, corners occupy 24 screen pixels. Recommended height 56-96 px, width >=72 px. Keep labels at least 24 px from the left/right edges. PNGs include opaque faces and transparent exterior corners, with straight alpha; use linear filtering and clamp sampling. Draw as ordinary textured UI with white tint to retain the artwork, rather than a panel shader that replaces the center fill. SVG sources and the deterministic Python/Pillow generator are included.

Glyphengine definition for each state:

```go
slice := renderer.NewNineSlice(texture, 256, 24)
slice.TexH = 96
// slice.GenerateQuads(x, y, w, h, 1, [3]float32{1, 1, 1})
```

Load each texture once. Switch texture/state without changing the layout rectangle. Priority: disabled > pointer held inside > hover > normal. On release outside, cancel the action. Disabled buttons should not accept input. Active here means hovered, not toggled. Draw labels separately using buttons.json colors; shift the pressed label downward 2 px without moving the hit box. Keyboard focus can reuse hover; keyboard activation can use pressed.

These are asset deliverables; existing game UI handlers and embedding are not changed. Add assets/ui/buttons/*.png to the go:embed file list when integrating. preview.png includes text overlays solely for demonstration.
