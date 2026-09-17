package game

import (
	"fmt"

	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/worldbuild/internal/colony"
	"github.com/derekmwright/worldbuild/internal/world"
)

// uiScale converts design units to pixels.
//
// An explicit setting wins; otherwise it is derived from the window height,
// because that is what actually decides whether 13-unit text is readable. The
// design was drawn against a 900-pixel-tall window, so a 2160-tall one gets
// 2x and a 4K display does not render the interface at the size of a postage
// stamp.
func (g *Game) uiScale(screenH float32) float32 {
	if g.cfg.UIScale > 0 {
		return g.cfg.UIScale
	}
	if g.ui.scaleAdj > 0 {
		return g.ui.scaleAdj
	}

	const designH = 900
	k := screenH / designH
	switch {
	case k < 1:
		// Never below 1: the layout stops fitting its own text before it
		// stops fitting the window.
		return 1
	case k >= 3:
		return 3
	case k >= 2:
		return 2
	case k >= 1.5:
		return 1.5
	}
	return 1
}

// Tooltip geometry, in design units.
const (
	tipW       = 300
	tipPad     = 12
	tipLineDY  = 15
	tipGap     = 10
	tipMinRowH = textSub * lineBox
)

// hotbarSlotAt returns the index of the hotbar slot under a point in design
// units, or -1.
//
// This is also what stops a click on the interface from also placing a
// building: the bar is 64 units of the bottom of the screen and the world is
// directly behind it, so without a hit test every click on a slot was also a
// click on whatever tile happened to be under it.
func (g *Game) hotbarSlotAt(dw, dh, x, y float32) int {
	// Asks slotRect rather than recomputing the layout, so a hit test cannot
	// drift from where the slot was actually drawn — which would show as a
	// tooltip for the wrong building, or a click that selects the neighbour.
	for i := range colony.Buildable {
		sx, sy, sw, sh := slotRect(dw, dh, i)
		if x >= sx && x <= sx+sw && y >= sy && y <= sy+sh {
			return i
		}
	}
	return -1
}

// overHUD reports whether a point in design units is over any panel, so world
// interaction can be suppressed there.
func (g *Game) overHUD(dw, dh, x, y float32) bool {
	if g.hotbarSlotAt(dw, dh, x, y) >= 0 {
		return true
	}
	// The left column: one strip from the top of the status panel down to
	// wherever the stack ended. Treating it as one rectangle is right because
	// the panels are stacked with small gaps, and a click landing in a gap
	// should not fall through to the map either.
	if x >= panelX && x <= panelX+panelW && y >= panelY && y <= g.columnBottom(dw, dh) {
		return true
	}
	return false
}

// columnBottom is how far down the left column reached this frame.
func (g *Game) columnBottom(dw, dh float32) float32 {
	return min32(dh-hotbarHeight(dw)-hotbarDY-10, panelY+statusH+10+3*alertRow+10+inspectorH)
}

// drawTooltip explains the structure under the pointer.
//
// The hotbar has room for a name and a price, which is enough to pick a
// building you already know and not enough to learn one. Everything that
// decides whether a structure is the right answer right now — what it draws,
// what it makes, what ground it needs, where it does better — is here.
func (g *Game) drawTooltip(h *hud, dw, dh float32) {
	if g.ui.hot < 0 || g.ui.hot >= len(colony.Buildable) {
		return
	}
	k := colony.Buildable[g.ui.hot]
	spec := colony.Of(k)

	// Build the body first: the panel has to be as tall as its contents, and
	// the contents depend on the structure.
	type row struct {
		label string
		value string
		col   [3]float32
	}
	var rows []row

	add := func(label, value string, col [3]float32) {
		rows = append(rows, row{label, value, col})
	}

	add("Cost", spec.CostText(), costColor(spec.Affordable(g.Colony.Iron, g.Colony.Crystal)))
	add("Ground", requirementText(spec.Needs), colInk)

	if spec.PowerOut > 0 {
		note := fmt.Sprintf("%.0f", spec.PowerOut)
		if spec.SolarDependent {
			note += " in daylight, none at night"
		} else {
			note += " day and night"
		}
		add("Power", note, colGood)
	}
	if spec.PowerIn > 0 {
		add("Draws", fmt.Sprintf("%.0f power", spec.PowerIn), colWarn)
	}
	if spec.MineOut > 0 {
		// What a mine brings up is the ground's business, so the tooltip
		// answers for the tile under the cursor when there is one and states
		// the rule when there is not.
		note := fmt.Sprintf("%.2f iron/s on dunes, %.2f crystal/s on a flat",
			spec.MineOut*colony.Yield(k, world.Dunes),
			spec.MineOut*colony.Yield(k, world.Crystal))
		col := colGood
		if g.intent.hovering {
			if tile := g.Map.At(g.intent.hover); tile != nil {
				if ore, rate := colony.Mines(k, tile.Terrain); rate > 0 {
					note = fmt.Sprintf("%.2f %s/s here", rate, ore)
				} else {
					note, col = "nothing under this ground", colCritical
				}
			}
		}
		add("Mines", note, col)
	}
	if spec.WaterOut > 0 {
		add("Makes", fmt.Sprintf("%.2f water/s", spec.WaterOut), colGood)
	}
	if spec.FoodOut > 0 {
		add("Grows", fmt.Sprintf("%.2f food/s", spec.FoodOut), colGood)
	}
	if spec.WaterIn > 0 {
		add("Uses", fmt.Sprintf("%.2f water/s", spec.WaterIn), colWarn)
	}
	if spec.FoodIn > 0 {
		add("Eats", fmt.Sprintf("%.2f food/s when full", spec.FoodIn), colWarn)
	}
	if spec.PowerStore > 0 {
		add("Stores", fmt.Sprintf("%.0f power-seconds", spec.PowerStore), colGood)
	}
	if spec.Housing > 0 {
		add("Houses", fmt.Sprintf("%.0f colonists", spec.Housing), colGood)
	}
	if bonus := yieldNote(k); bonus != "" {
		add("Best on", bonus, colAccent)
	}

	// What the tile under the cursor would give it, which is the reason to
	// read a tooltip while pointing at somewhere in particular.
	var siteLine string
	var siteCol [3]float32
	if g.intent.hovering {
		if tile := g.Map.At(g.intent.hover); tile != nil {
			if err := g.Colony.CanPlace(g.Map, k, g.intent.hover); err != nil {
				siteLine, siteCol = fmt.Sprintf("%s: %v", tile.Terrain, err), colCritical
			} else {
				note := fmt.Sprintf("%s: can build here", tile.Terrain)
				if y := colony.Yield(k, tile.Terrain); y != 1 {
					note = fmt.Sprintf("%s: can build, x%.2f yield", tile.Terrain, y)
				}
				siteLine, siteCol = note, colGood
			}
		}
	}

	bodyH := tipPad + textMain*lineBox + 4 + textSub*lineBox + 6 +
		float32(len(rows))*tipLineDY + tipPad
	if siteLine != "" {
		bodyH += tipLineDY + 4
	}

	// Anchored above the slot it describes, clamped to the window so the last
	// slot's tooltip does not hang off the edge.
	sx, sy, _, _ := slotRect(dw, dh, g.ui.hot)
	x := min32(sx, dw-tipW-panelX)
	x = max32(x, panelX)
	y := sy - bodyH - tipGap

	h.panel(x, y, tipW, bodyH)

	ty := y + tipPad
	h.clipped(x+tipPad, ty, textMain, tipW-2*tipPad, colAccent, "%s", spec.Name)
	ty += textMain*lineBox + 4
	h.clipped(x+tipPad, ty, textSub, tipW-2*tipPad, colDim, "%s", spec.Desc)
	ty += textSub*lineBox + 6

	for _, r := range rows {
		h.label(x+tipPad, ty, textSub, colDim, "%s", r.label)
		h.clipped(x+tipPad+72, ty, textSub, tipW-2*tipPad-72, r.col, "%s", r.value)
		ty += tipLineDY
	}

	if siteLine != "" {
		ty += 4
		h.clipped(x+tipPad, ty, textSub, tipW-2*tipPad, siteCol, "%s", siteLine)
	}
}

func costColor(affordable bool) [3]float32 {
	if affordable {
		return colInk
	}
	return colCritical
}

// requirementText is the long form of a siting rule, for the tooltip.
func requirementText(r colony.Requirement) string {
	switch r {
	case colony.NeedsOre:
		return "ferrous dunes or a crystal flat"
	case colony.NeedsFrozen:
		return "an ice sheet"
	case colony.NeedsGeothermal:
		return "a thermal vent"
	}
	return "any solid ground"
}

// yieldNote names the terrain a structure does better on, when it has one.
// Read from colony.Yield rather than restated, so retuning the table cannot
// leave the tooltip lying.
func yieldNote(k colony.Kind) string {
	if k == colony.Mine {
		// A mine's two grounds yield different ores, so the larger multiplier
		// is not a better tile — it is a different resource. The Mines row
		// above says what each one gives.
		return ""
	}
	best, bestYield := world.Terrain(0), float64(1)
	for t := world.Terrain(0); t < world.TerrainCount(); t++ {
		if y := colony.Yield(k, t); y > bestYield {
			best, bestYield = t, y
		}
	}
	if bestYield <= 1 {
		return ""
	}
	return fmt.Sprintf("%s, x%.2f", best, bestYield)
}

// updateUIHover works out what the pointer is over in the interface, before
// the world gets a look at it.
func (g *Game) updateUIHover(e *glyph.Engine) {
	g.ui.hot = -1
	g.ui.blocked = false
	if g.hud == nil || g.hud.scale <= 0 {
		return
	}

	mx, my := g.pointer(e)
	w, ph := e.Renderer().Extent()
	dw, dh := float32(w)/g.hud.scale, float32(ph)/g.hud.scale
	x, y := float32(mx)/g.hud.scale, float32(my)/g.hud.scale

	g.ui.hot = g.hotbarSlotAt(dw, dh, x, y)
	g.ui.blocked = g.overHUD(dw, dh, x, y)
}
