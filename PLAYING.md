# Vesper III

A colony on a planet that is not Earth. You land with a habitat and a solar
array; everything after that is yours to work out.

Run the executable. There is nothing to install and nothing to unpack — every
piece of art, the font and all eight structure models are inside the one file.

**You need:** a graphics card and driver supporting Vulkan 1.1. Anything from
about 2016 onwards has one. If it refuses to start, updating the graphics
driver is almost always the fix.

## Controls

| | |
|---|---|
| `W` `A` `S` `D` | pan |
| Right-drag | orbit. Drag down to tip the camera toward looking straight down |
| Wheel | zoom |
| `Q` `E` | turn · `R` `F` tilt |
| `1`–`8` | pick a structure |
| Left click | build |
| `Shift` + wheel | turn the structure before you place it, a sixth at a time |
| `Shift` + right click | demolish, with a confirmation |
| `T` | terraform mode — left click raises, right click lowers |
| `X` | demolish mode · `B` back to build |
| `F5` `F9` | save and load |
| `F3` | the raw numbers |
| `[` `]` | interface scale |
| `Esc` | quit |

## What to do

Colonists arrive to fill the habitat, and they drink and eat, which is the
first problem.

- **Water** comes from an **Ice Extractor** on an ice sheet, or slowly from an
  **Atmospheric Condenser** anywhere. Habitats, greenhouses and mines all draw
  on it.
- **Food** comes from a **Greenhouse**, which turns water and power into it.
  Lichen ground grows half again as much.
- **Iron** builds everything, and comes from a **Mine** on ferrous dunes.
  **Crystal** comes from the same building on a crystal flat, and is what
  battery banks and geothermal plants are priced in. Regolith and basalt yield
  nothing — where you put a mine is the decision.
- **Power** is the binding constraint, and it is not pass or fail: a colony
  that generates less than it draws runs *everything* at the ratio between the
  two. Solar stops dead at sunset. A **Battery Bank** charges by day and covers
  the night; a **Geothermal Plant** runs day and night but needs a thermal vent
  and a steady supply of water for its coolant loop.

The panel down the left says what is happening. The strip under it says what to
do about it — that one is worth reading, because it names the building that
fixes the problem rather than just reporting the number.

Watch the colony at dusk. The lights come on, and they go out again if the
power does.

## Licences

The game is built on [glyphengine](https://github.com/derekmwright/glyphengine).
The interface is set in Exo 2, used under the SIL Open Font Licence — the full
text ships alongside as `LICENSE-Exo2.txt`.
