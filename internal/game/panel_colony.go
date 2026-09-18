package game

import (
	"fmt"

	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/world"
)

// The colony readout: the resource panel, the advisory strip under it, and the
// tile inspector under that.
//
// This is the file that turns colony.Readout into something a person can act
// on. It subscribes to nothing and caches nothing — the panel is rebuilt from
// the colony every frame, because a resource figure is state rather than an
// event and a copy of it could only ever be wrong. See package event for why
// that distinction is drawn where it is.

// Panel geometry, in pixels.
//
// TextLine.Y is the TOP of a line box, not its baseline: MSDFText places each
// glyph from Y downward by the font's ascender (renderer/msdftext.go). Laying
// rows out as if Y were a baseline is what drew the first draft's bars through
// the middle of their own labels, so every offset here is measured from the
// top and every panel height is derived from its parts rather than guessed.
const (
	panelX = 16
	panelY = 16
	panelW = 376

	textHead = 16.0
	textMain = 13.0
	textSub  = 11.0

	// A line of text occupies about 1.15 times its scale, ascender to
	// descender.
	lineBox = 1.15

	barH = 7

	// One resource row: label, then the bar, then the making/using line.
	rowBarDY = textMain*lineBox + 5
	rowSubDY = rowBarDY + barH + 6
	rowH     = rowSubDY + textSub*lineBox + 10

	// Right edges of the two numeric columns. Figures are right-aligned to
	// these so the decimal points line up down the panel.
	colStockR = panelX + 214
	colRateR  = panelX + panelW - 14

	// Compact rows drop the making/using sub-line, keeping the figure, the net
	// rate and the bar. That is what gets shed first when the window is too
	// short for everything, because the sub-line is elaboration and the
	// advisory strip below it is instruction.
	rowCompactH = rowBarDY + barH + 9

	headerH = textHead*lineBox + 14

	// Six resource rows — power, water, food, iron, crystal, vespite — and
	// then the colonist line, which is a plain figure rather than a flow.
	statusRows     = 6
	statusH        = headerH + statusRows*rowH + textMain*lineBox + 16
	statusCompactH = headerH + statusRows*rowCompactH + textMain*lineBox + 16

	alertsY  = panelY + statusH + 10
	alertRow = textMain*lineBox + textSub*lineBox + 12

	// Text in the alert strip starts 20px in and needs a margin on the right.
	alertTextW = panelW - 20 - 12
)

// drawStatusPanel is the resource readout: what there is, what is making it,
// what is using it, and how long it lasts.
func (g *Game) drawStatusPanel(h *hud, top float32, compact bool) float32 {
	c := g.Colony
	r := c.Readout

	height, step := float32(statusH), float32(rowH)
	if compact {
		height, step = statusCompactH, rowCompactH
	}
	h.panel(panelX, top, panelW, height)

	h.label(panelX+14, top+10, textHead, colAccent, "VESPER III")
	h.rightLabel(colRateR, top+14, textSub, colDim, "%s", dayPhase(r.Daylight))

	y := top + headerH

	// Power first: it scales every other rate, so it is what explains a number
	// further down the panel.
	g.drawPowerRow(h, y, r, compact)
	y += step

	g.drawFlowRow(h, y, iconWater, "WATER", c.Water, r.Cap.Water, r.Water, r.Spilled.Water, compact)
	y += step

	g.drawFlowRow(h, y, iconFood, "FOOD", c.Food, r.Cap.Food, r.Food, r.Spilled.Food, compact)
	y += step

	// Iron and crystal come out of the same building on different ground, so
	// they are drawn the same way and read as a pair.
	g.drawMineRow(h, y, iconIron, "IRON", c.Iron, r.Cap.Iron, r.Iron, r.Spilled.Iron, r.IronMines, compact)
	y += step

	g.drawMineRow(h, y, iconCrystal, "CRYSTAL", c.Crystal, r.Cap.Crystal, r.Crystal, r.Spilled.Crystal, r.CrystalMines, compact)
	y += step

	// Vespite last of the stocks, because it is the only one that is not
	// about staying alive. Its row is drawn even with no synthesizer standing
	// — a resource the player has never seen is one they will never plan for,
	// and an empty row that says what makes it is the cheapest way to say so.
	if r.Cap.Vespite > 0 {
		g.drawFlowRow(h, y, iconVespite, "VESPITE", c.Vespite, r.Cap.Vespite,
			r.Vespite, r.Spilled.Vespite, compact)
	} else {
		h.rowLabel(y, iconVespite, "VESPITE")
		h.rightLabel(colStockR, y, textMain, colDim, "no synthesizer")
	}
	y += step

	// Population, as a plain line: it has a ceiling rather than a flow.
	popCol := colInk
	if r.Housing > 0 && c.Colonists >= r.Housing-0.01 {
		popCol = colWarn
	}
	h.rowLabel(y, iconColonists, "COLONISTS")
	h.rightLabel(colStockR, y, textMain, popCol, "%.0f / %.0f", c.Colonists, r.Housing)

	// Staffing, not the structure count: how many of the jobs the colony has
	// created are actually being done is the thing that scales every other
	// number on this panel.
	jobCol := colDim
	switch {
	case r.Jobs > 0 && r.Staffing < 0.5:
		jobCol = colCritical
	case r.Jobs > 0 && r.Staffing < 0.999:
		jobCol = colWarn
	}
	h.rightLabel(colRateR, y+1, textSub, jobCol, "%.0f of %.0f jobs", min(c.Colonists, r.Jobs), r.Jobs)

	return top + height
}

// rowIcon is how big a resource icon is drawn on a panel row, and rowTextX is
// where the label starts once one is allowed for. The power and colonist rows
// have no icon and still indent to rowTextX, because a column of labels that
// only lines up on four rows out of six reads as a mistake.
const (
	rowIcon  = 17
	rowTextX = 14 + rowIcon + 6
)

// rowLabel draws a resource row's icon and name together, so the two cannot
// drift apart from row to row.
func (h *hud) rowLabel(y float32, cell int, name string) {
	// The icon sits a touch above the text baseline box: the artwork is
	// centred in a square cell and the label is not, so aligning the boxes
	// leaves the icon looking low.
	h.icon(panelX+14, y-2, rowIcon, cell)
	h.label(panelX+rowTextX, y, textMain, colDim, "%s", name)
}

// drawPowerRow shows supply against demand. Power is the resource with no
// stock — generated and consumed in the same instant — so it gets a
// satisfaction reading rather than a countdown.
func (g *Game) drawPowerRow(h *hud, y float32, r colony.Readout, compact bool) {
	label, col := "ok", colGood
	switch {
	case r.PowerDemand == 0:
		label, col = "idle", colDim
	case r.Satisfaction < 0.5:
		label, col = "BLACKOUT", colCritical
	case r.Satisfaction < 0.999:
		label, col = "BROWNOUT", colWarn
	}

	h.rowLabel(y, iconPower, "POWER")
	h.rightLabel(colStockR, y, textMain, colInk, "%.0f / %.0f", r.PowerSupply, r.PowerDemand)
	h.rightLabel(colRateR, y, textMain, col, "%s", label)

	h.supplyBar(panelX+14, y+rowBarDY, panelW-28, barH,
		float32(r.PowerSupply), float32(r.PowerDemand))

	if compact {
		// The reserve is the one number worth keeping when there is no room
		// for the sub-line: it is what decides whether the next hour is a
		// problem.
		if r.Cap.Power > 0 {
			h.rightLabel(colStockR-52, y+1, textSub, bankColor(r), "%.0f", r.Stored)
		}
		return
	}

	sub := fmt.Sprintf("making %.0f   using %.0f", r.PowerSupply, r.PowerDemand)
	h.label(panelX+14, y+rowSubDY, textSub, colDim, "%s", sub)

	if r.Cap.Power > 0 {
		// The bank, on the right of the same line: how much is in it, and
		// which way it is going. The sign is the whole message — a colony
		// charging at dusk is fine and one draining at dusk is on a clock.
		rate := r.ChargeRate
		if rate > -colony.RateEpsilon && rate < colony.RateEpsilon {
			rate = 0 // "-0/s" is not a direction
		}
		bank := fmt.Sprintf("bank %.0f/%.0f  %+.0f/s", r.Stored, r.Cap.Power, rate)
		if r.ChargeRate < -0.001 && r.Stored > 0 {
			bank += "  " + colony.Duration(r.Stored/-r.ChargeRate)
		}
		h.rightLabel(colRateR, y+rowSubDY, textSub, bankColor(r), "%s", bank)
	}
}

// bankColor is green while the reserve is filling and amber while it drains,
// so the direction reads before the number does.
func bankColor(r colony.Readout) [3]float32 {
	switch {
	case r.ChargeRate > 0.001:
		return colGood
	case r.ChargeRate < -0.001:
		if r.Stored <= 0.001 {
			return colCritical
		}
		return colWarn
	default:
		return colDim
	}
}

// drawFlowRow is the two-sided readout the whole panel exists for: stock,
// production, consumption, and — when it is falling — how long is left.
func (g *Game) drawFlowRow(h *hud, y float32, cell int, name string, stock, capacity float64, f colony.Flow, spill float64, compact bool) {
	h.rowLabel(y, cell, name)

	// Flow.Rate rather than Flow.Net: a ledger that balances to within
	// floating-point noise has to print as flat, not as "-0.00/s".
	net := f.Rate()
	netCol := colDim
	switch {
	case net > 0:
		netCol = colGood
	case net < 0:
		netCol = colWarn
	}
	h.rightLabel(colRateR, y, textMain, netCol, "%+.2f/s", net)

	left := colony.SecondsLeft(stock, f)

	// The figure is the stock against what there is room for, because that is
	// the pair the bar under it draws.
	stockCol := colInk
	switch {
	case stock <= 0 && f.Consumed > 0:
		stockCol = colCritical
	case left >= 0 && left <= 90:
		stockCol = colCritical
	case left >= 0 && left <= 300:
		stockCol = colWarn
	}
	h.rightLabel(colStockR, y, textMain, stockCol, "%.0f / %.0f", stock, capacity)

	h.storeBar(panelX+14, y+rowBarDY, panelW-28, barH, stock, capacity, left)

	if compact {
		if left >= 0 {
			col := colWarn
			if left <= 90 {
				col = colCritical
			}
			h.rightLabel(colStockR-52, y+1, textSub, col, "%s", colony.Duration(left))
		}
		return
	}

	// The sub-line is where "do I need another extractor" is actually
	// answered: the two halves side by side, and the runway if it is
	// shrinking — or what is being thrown away if it is not.
	sub := fmt.Sprintf("making %.2f   using %.2f", f.Produced, f.Consumed)
	subCol := colDim
	switch {
	case spill >= colony.RateEpsilon:
		sub += fmt.Sprintf("   full, losing %.2f", spill)
		subCol = colAccent
	case left >= 0:
		sub += "   empty in " + colony.Duration(left)
		subCol = colWarn
		if left <= 90 {
			subCol = colCritical
		}
	}
	h.label(panelX+14, y+rowSubDY, textSub, subCol, "%s", sub)
}

// drawMineRow is one-sided: an ore is spent in lumps when something is built,
// not drawn continuously, so a consumption rate would read as zero and a
// countdown would be meaningless.
//
// ground is a terrain that yields this ore, used only to work out what one
// mine on it manages — the bar needs a full-rate figure to draw against and
// asking colony.Yield is better than restating the multiplier here.
func (g *Game) drawMineRow(h *hud, y float32, cell int, label string, stock, capacity float64, f colony.Flow, spill float64, mines int, compact bool) {
	h.rowLabel(y, cell, label)

	made := f.Produced
	if made < colony.RateEpsilon {
		made = 0
	}
	netCol := colDim
	if made > 0 {
		netCol = colGood
	}
	h.rightLabel(colRateR, y, textMain, netCol, "%+.2f/s", made)

	full := capacity > 0 && stock >= capacity-1e-6
	stockCol := colInk
	if full {
		stockCol = colAccent
	}
	h.rightLabel(colStockR, y, textMain, stockCol, "%.0f / %.0f", stock, capacity)

	// An ore is only spent in lumps when something is built, so it has no
	// countdown to draw — the bar is purely how much room is left.
	h.storeBar(panelX+14, y+rowBarDY, panelW-28, barH, stock, capacity, -1)

	if !compact {
		note := fmt.Sprintf("%d mine(s)   spent on building, not drawn", mines)
		switch {
		case spill >= colony.RateEpsilon:
			note = fmt.Sprintf("%d mine(s)   full, losing %.2f/s", mines, spill)
		case mines == 0:
			note = "no mine   nothing is being cut"
		}
		col := colDim
		if spill >= colony.RateEpsilon {
			col = colAccent
		}
		h.label(panelX+14, y+rowSubDY, textSub, col, "%s", note)
	}
}

// drawAlerts is the advisory strip: what is wrong, and what to build about it.
// It returns the y it ended at, and draws nothing if maxShown is zero.
func (g *Game) drawAlerts(h *hud, top float32, maxShown int) float32 {
	if maxShown <= 0 {
		return top
	}
	alerts := g.Colony.Alerts()

	if len(alerts) == 0 {
		height := float32(textMain*lineBox + 18)
		h.panel(panelX, top, panelW, height)
		h.label(panelX+14, top+9, textMain, colGood, "Colony stable")
		return top + height
	}

	shown := min(len(alerts), maxShown)
	height := float32(shown)*alertRow + 12
	h.panel(panelX, top, panelW, height)

	for i := 0; i < shown; i++ {
		a := alerts[i]
		ay := top + 9 + float32(i)*alertRow
		col := alertColor(a.Level)

		// A colour chip on the left, so severity registers before the text is
		// read.
		h.quad(panelX+8, ay+1, 4, alertRow-12, col)
		h.clipped(panelX+20, ay, textMain, alertTextW, col, "%s", a.Text)
		h.clipped(panelX+20, ay+textMain*lineBox+2, textSub, alertTextW, colDim, "%s", a.Fix)
	}

	if extra := len(alerts) - shown; extra > 0 {
		h.rightLabel(colRateR, top+height-textSub*lineBox-3, textSub, colDim, "+%d more", extra)
	}
	return top + height
}

func alertColor(l colony.Level) [3]float32 {
	switch l {
	case colony.LevelCritical:
		return colCritical
	case colony.LevelWarn:
		return colWarn
	case colony.LevelInfo:
		return colAccent
	default:
		return colGood
	}
}

// inspectorH is a title line plus two detail lines, with padding.
const (
	inspectorH = textMain*lineBox + 2*(textSub*lineBox) + 28
	inspTextW  = panelW - 14 - 12
)

// drawInspector reports the tile under the cursor, and when it cannot be built
// on, why. The reason is the useful half: "wrong ground" sends a player
// looking for the right ground, where a bare refusal sends them nowhere.
func (g *Game) drawInspector(h *hud, y float32) float32 {
	h.panel(panelX, y, panelW, inspectorH)

	top := y + 9
	line2 := top + textMain*lineBox + 5
	line3 := line2 + textSub*lineBox + 3

	if !g.intent.hovering {
		h.label(panelX+14, top, textMain, colDim, "Pointing at the sky")
		return y + inspectorH
	}

	tile := g.Map.At(g.intent.hover)
	if tile == nil {
		return y + inspectorH
	}

	h.label(panelX+14, top, textMain, colInk, "%s", tile.Terrain)
	if tile.Terrain == world.Sea {
		h.rightLabel(colRateR, top+1, textSub, colDim, "submerged")
	} else {
		h.rightLabel(colRateR, top+1, textSub, colDim, "elevation %d", tile.Elevation)
	}

	if b, built := g.Colony.At(g.intent.hover); built {
		spec := colony.Of(b.Kind)
		name := spec.Name
		if b.Tier() > 1 {
			name = fmt.Sprintf("%s  tier %d", name, b.Tier())
		}
		h.clipped(panelX+14, line2, textSub, inspTextW, colAccent, "%s", name)

		// In upgrade mode the second line is the price rather than the
		// description: the description is what the player read before they
		// built it, and what they want now is whether they can afford the
		// next one.
		if g.intent.mode == ModeUpgrade {
			switch cost, err := g.Colony.CanUpgrade(g.intent.hover); {
			case err != nil:
				h.clipped(panelX+14, line3, textSub, inspTextW, colCritical, "%v", err)
			default:
				h.clipped(panelX+14, line3, textSub, inspTextW, colGood,
					"tier %d for %.0f vespite + %.0f iron", b.Tier()+1, cost.Vespite, cost.Iron)
			}
			return y + inspectorH
		}

		note := spec.Desc
		if b.Yield != 1 {
			note = fmt.Sprintf("%s (x%.2f here)", note, b.Yield)
		}
		h.clipped(panelX+14, line3, textSub, inspTextW, colDim, "%s", note)
		return y + inspectorH
	}

	switch g.intent.mode {
	case ModeBuild:
		spec := colony.Of(g.intent.selected)
		if err := g.Colony.CanPlace(g.Map, g.intent.selected, g.intent.hover); err != nil {
			h.clipped(panelX+14, line2, textSub, inspTextW, colCritical, "cannot build %s", spec.Name)
			h.clipped(panelX+14, line3, textSub, inspTextW, colDim, "%v", err)
		} else {
			h.clipped(panelX+14, line2, textSub, inspTextW, colGood, "build %s here", spec.Name)
			h.clipped(panelX+14, line3, textSub, inspTextW, colDim,
				"facing %d/%d - shift+wheel turns it", g.intent.facing+1, colony.Facings)
		}
	case ModeTerraform:
		h.label(panelX+14, line2, textSub, colWarn, "terraform this tile")
		h.label(panelX+14, line3, textSub, colDim, "left raises, right lowers (%d ore)", terraformCost)
	case ModeDemolish:
		h.label(panelX+14, line2, textSub, colDim, "nothing here to demolish")
	case ModeUpgrade:
		h.label(panelX+14, line2, textSub, colDim, "nothing here to upgrade")
	}
	return y + inspectorH
}
