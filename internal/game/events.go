package game

import (
	"fmt"

	glyph "github.com/derekmwright/glyphengine"

	"github.com/derekmwright/worldbuild/internal/colony"
	"github.com/derekmwright/worldbuild/internal/event"
	"github.com/derekmwright/worldbuild/internal/hex"
)

// The game's event vocabulary: the things that happen, as opposed to the
// things that are true.
//
// Every type here is a fact in the past tense about something that already
// happened. None of them carry state a subscriber could have read for itself —
// a StructurePlaced does not include the colony's iron balance, because the
// colony knows its own balance and anyone who needs it can ask.
//
// See package event for the reasoning, and subscribe below for the wiring.

// StructurePlaced is published after a structure has been added to the colony
// and paid for. The colony is already the authority on it by the time this
// goes out; subscribers are catching up, not approving.
type StructurePlaced struct {
	Kind   colony.Kind
	At     hex.Axial
	Facing uint8
}

// StructureDemolished carries what was reclaimed, because the structure is
// gone by the time anyone hears about it and the refund cannot be looked up
// from a building that no longer exists.
type StructureDemolished struct {
	Kind          colony.Kind
	At            hex.Axial
	Iron, Crystal float64
}

// TileReshaped is one terraforming step that actually happened.
type TileReshaped struct {
	At    hex.Axial
	Delta int
	Cost  float64
}

// ActionRefused is the player asking for something the rules do not allow.
//
// It is one type rather than one per rule because every subscriber treats them
// alike — the interface says so, and nothing else cares. Err is set when a
// rule in package colony refused it and nil when the refusal is this package's
// own, which is the difference between "the catalog says no" and "there is
// nothing there".
type ActionRefused struct {
	Reason string
	Err    error
}

// ToolChanged is the player picking a different structure, mode, or heading.
type ToolChanged struct {
	Mode     Mode
	Selected colony.Kind
	Facing   uint8

	// Why is what changed, so the interface can say the useful half rather
	// than restating all three every time.
	Why string
}

// GameSaved and GameLoaded bracket the one operation that touches the disk.
type (
	GameSaved  struct{ Path string }
	GameLoaded struct{ Path string }
)

// Noticed is a plain message with no domain meaning: a diagnostic, a limit
// being hit, something the player should see once.
//
// It is the escape hatch, and it is worth being honest that it is one. An
// event type per string would be ceremony without structure — nothing can
// subscribe to "the HUD ran out of quads" and do anything about it but say so.
// When a Noticed turns out to have a second subscriber, that is the signal it
// wanted to be a real event all along.
type Noticed struct{ Text string }

// subscribe wires the systems to the events, in one place and in the order
// they run.
//
// This is the function to read to find out what reacts to what. Registration
// order is delivery order, and the scene goes first on purpose: by the time
// the interface says "Habitat built", the habitat is on screen.
func (g *Game) subscribe(e *glyph.Engine) {
	// The renderer's mirror. This is the subscription that earns the bus: the
	// code that places a structure no longer has to remember to draw it, and
	// the two cannot fall out of step because there is only one path.
	event.On(g.bus, func(ev StructurePlaced) {
		g.spawnBuilding(e, ev.Kind, ev.At)
	})
	event.On(g.bus, func(ev StructureDemolished) {
		g.scene.despawnBuilding(e, ev.At)
	})
	event.On(g.bus, func(ev TileReshaped) {
		g.refreshAround(e, ev.At)
	})

	// The interface. Everything below only turns an event into a line of text,
	// which is why it can be read as a list of what the game says and when.
	event.On(g.bus, func(ev StructurePlaced) {
		g.setStatus("%s built", colony.Of(ev.Kind).Name)
	})
	event.On(g.bus, func(ev StructureDemolished) {
		g.setStatus("%s demolished, %s recovered",
			colony.Of(ev.Kind).Name, colony.Materials(ev.Iron, ev.Crystal))
	})
	event.On(g.bus, func(ev TileReshaped) {
		verb := "Raised"
		if ev.Delta < 0 {
			verb = "Lowered"
		}
		g.setStatus("%s a tile for %.0f iron", verb, ev.Cost)
	})
	event.On(g.bus, func(ev ActionRefused) {
		if ev.Err != nil {
			g.setStatus("%v", ev.Err)
			return
		}
		g.setStatus("%s", ev.Reason)
	})
	event.On(g.bus, func(ev ToolChanged) {
		g.setStatus("%s", ev.Why)
	})
	event.On(g.bus, func(ev GameSaved) {
		g.setStatus("Saved to %s", ev.Path)
	})
	event.On(g.bus, func(ev GameLoaded) {
		g.setStatus("Loaded %s", ev.Path)
	})
	event.On(g.bus, func(ev Noticed) {
		g.setStatus("%s", ev.Text)
	})
}

// emit is shorthand for publishing on this game's bus. It exists so call sites
// read as g.emit(StructurePlaced{...}) rather than carrying the package and
// the bus around with them.
func emit[T any](g *Game, ev T) { event.Emit(g.bus, ev) }

// refuse publishes a refusal whose reason is this package's own rule rather
// than one from the catalog.
func (g *Game) refuse(format string, args ...any) {
	emit(g, ActionRefused{Reason: fmt.Sprintf(format, args...)})
}
