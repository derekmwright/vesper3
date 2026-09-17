package game

import (
	"github.com/derekmwright/vesper3/internal/colony"
)

// The build bar along the bottom: what can be built, what it costs, and
// whether the ground under the cursor will take it.
//
// The geometry is worked out by slotRect and hotbarMetrics rather than by the
// drawing code, so the hit test in updateUIHover and the tooltip anchored
// above a slot cannot disagree with where the slot was actually drawn.

// Hotbar geometry.
const (
	slotWMax = 186
	// Below this a slot cannot show a structure name, which is the whole point
	// of the slot; the bar wraps to another row rather than truncating.
	slotWMin = 142
	slotH    = 64
	slotGap  = 6
	hotbarDY = 16  // from the bottom of the window
	modeW    = 108 // reserved for the mode label to the left of the bar
	iconMax  = 46
)

// hotbarMetrics sizes the bar to the window, wrapping it onto a second row
// when one will not fit.
//
// Seven slots at full width need about 1,450 design units once the mode label
// is allowed for. Raising the interface scale shrinks the design-space window,
// so at 2x on a 1600-pixel display there are only 800 units to place them in
// and a single row cannot hold them at a legible width. Squeezing them anyway
// is what truncated every name to "Habi..."; wrapping keeps the names.
func hotbarMetrics(dw float32) (slotW, iconSize float32, perRow, rows int) {
	n := len(colony.Buildable)

	for perRow = n; perRow >= 2; perRow-- {
		avail := dw - 2*panelX - (float32(perRow)-1)*slotGap
		if w := avail / float32(perRow); w >= slotWMin {
			slotW = min32(slotWMax, w)
			break
		}
	}
	if slotW == 0 {
		perRow, slotW = 2, slotWMin
	}

	rows = (n + perRow - 1) / perRow

	// The icon keeps its share of the slot rather than a fixed size, and drops
	// out entirely when what is left would not be legible.
	iconSize = min32(iconMax, slotW*0.28)
	if iconSize < 18 {
		iconSize = 0
	}
	return slotW, iconSize, perRow, rows
}

// hotbarHeight is how much vertical room the bar needs, including wrapping.
func hotbarHeight(dw float32) float32 {
	_, _, _, rows := hotbarMetrics(dw)
	return float32(rows)*slotH + float32(rows-1)*slotGap
}

// slotRect returns the design-space rectangle of hotbar slot i.
func slotRect(dw, dh float32, i int) (x, y, w, h float32) {
	slotW, _, perRow, rows := hotbarMetrics(dw)

	row := i / perRow
	col := i % perRow

	// The last row may be short, and a centred short row reads better than one
	// flush to the left.
	inRow := perRow
	if row == rows-1 {
		if rem := len(colony.Buildable) - row*perRow; rem > 0 {
			inRow = rem
		}
	}
	rowW := float32(inRow)*slotW + float32(inRow-1)*slotGap
	x0 := (dw - rowW) / 2

	top := dh - hotbarHeight(dw) - hotbarDY
	return x0 + float32(col)*(slotW+slotGap), top + float32(row)*(slotH+slotGap), slotW, slotH
}

// drawHotbar is the build bar along the bottom: what can be built, what it
// costs, and whether it is affordable right now.
func (g *Game) drawHotbar(h *hud, dw, dh float32) {
	_, iconSize, _, _ := hotbarMetrics(dw)

	for i, k := range colony.Buildable {
		x, y, slotW, _ := slotRect(dw, dh, i)
		spec := colony.Of(k)

		selected := k == g.intent.selected && g.intent.mode == ModeBuild
		back := colSlot
		switch {
		case selected:
			back = colSlotPick
		case i == g.ui.hot:
			back = colSlotHover
		}
		h.quad(x, y, slotW, slotH, back)
		h.quad(x, y, slotW, 2, colPanelEdge)

		nameCol, costCol := colInk, colDim
		if !spec.Affordable(g.Colony.Iron, g.Colony.Crystal) {
			nameCol, costCol = colDim, colCritical
		}
		if selected {
			nameCol = colAccent
		}

		h.label(x+9, y+10, textSub, colDim, "%d", i+1)

		// The icon sits where the eye lands first; the text is the fallback
		// when the atlas is missing, and the confirmation when it is not.
		textX := x + 24
		if h.icons != nil && iconSize > 0 {
			h.icon(x+18, y+(slotH-iconSize)/2, iconSize, i)
			textX = x + 22 + iconSize
		}

		right := x + slotW - 8
		h.clipped(textX, y+9, textMain, right-textX, nameCol, "%s", shortName(spec.Name))
		h.clipped(textX, y+28, textSub, right-textX, costCol, "%s", spec.CostText())
		if req := requirementHint(spec.Needs); req != "" {
			h.clipped(textX, y+45, textSub, right-textX, colDim, "%s", req)
		}
	}

	// Mode above the bar rather than beside it: beside it, the label was the
	// first thing a narrow window pushed into the slots.
	modeCol := colAccent
	switch g.intent.mode {
	case ModeDemolish:
		modeCol = colCritical
	case ModeTerraform:
		modeCol = colWarn
	}
	x0, y0, _, _ := slotRect(dw, dh, 0)
	h.label(x0, y0-textSub*lineBox-5, textSub, modeCol, "%s  (B / X / T)", g.intent.mode)
}

// shortName trims the catalog names that do not fit a hotbar slot. The catalog
// keeps the full name, so the inspector and the alerts still say all of it.
func shortName(name string) string {
	switch name {
	case "Atmospheric Condenser":
		return "Condenser"
	case "Geothermal Plant":
		return "Geothermal"
	case "Ice Extractor":
		return "Extractor"
	case "Solar Array":
		return "Solar"
	}
	return name
}

// requirementHint is the one-line version of a siting rule, for the hotbar.
func requirementHint(r colony.Requirement) string {
	switch r {
	case colony.NeedsOre:
		return "on ferrous or crystal"
	case colony.NeedsFrozen:
		return "on ice"
	case colony.NeedsGeothermal:
		return "on a vent"
	}
	return ""
}
