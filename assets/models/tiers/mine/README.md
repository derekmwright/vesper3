# Vesper III — mine tiers

Three visual tiers for one mine building. Tier 1 is an exact copy of the current source/runtime mine. Tier 2 and Tier 3 preserve that foundation, derrick, winch hut, body geometry and UV textures and add progressively larger production/storage equipment.

| Tier | Visual changes | Triangles | Material primitives |
| --- | --- | ---: | ---: |
| 1 | Original shaft mine | 5890 | 2 |
| 2 | Powered drill collar, hoist motor/canopy, receiving hopper and conveyor, drive cabinet | 9,342 | 3 |
| 3 | Taller automated hoist head, guided drill feed, twin sealed silos with feed pipes | 12,790 | 3 |

## Asset contract

- GLB / glTF 2.0, Y-up, ground-centered, existing radius 0.8 and placement scale 0.88. No footprint rescaling between tiers.
- Runtime GLBs contain 512 px textures. Authored GLBs retain the original 2048 px body maps plus 1024 px upgrade maps.
- One shared `Amber runtime lamp` primitive per model preserves the game's dusk lamp marker. Silo indicator bars are part of that lamp material, decorative rather than live storage telemetry.
- Retained original body plus one textured upgrade body and one lamp: three primitives for each new tier.
- No additional lights or animation clips are embedded. Moving machinery and live fill indicators would require game-side work.
- Validation: Blender reimport and front/rear renders, finite geometry and radius/ground assertions, embedded image/UV/normal checks, and the project's `cmd/modelcheck` on each runtime tier under its canonical `3-mine.glb` filename.

## Game installation and integration

Current `assets/models/3-mine.glb` and its source remain untouched. Upgrade runtime files belong under `assets/models/tiers/mine/tier2/3-mine.glb` and `tier3/3-mine.glb`; source GLBs and packed Blender files mirror those folders under `assets/models-src`.

The current root-only embed and model bake globs do not pick up these nested variants. When implementing upgrades, include `assets/models/tiers/mine` in go:embed, load/cache parts by `(kind, tier)`, and swap the parts when the building upgrades. Preserve its position, facing, inventory and simulation identity; rebuild its lamp-part reference when replacing entities. Extend bake/check tasks to visit each tier directory. Production rate, capacity, upgrade costs and saved tier state belong in the game; no economy values or upgrade behavior are changed here.

## Reproduction

Use Blender 5.0 with `--background --factory-startup --python build_mine_tiers.py`. The script consumes the Tier 1 copy under models-src and the included helpers. Run the game's texscale utility with this package's models-src and models directories, size 512. Run `render_previews.py` with Blender for the studio images. Each .blend contains packed textures.

Preview images show exported runtime assets in Blender studio lighting, not an in-game upgrade implementation.
