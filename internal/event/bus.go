// Package event is a small synchronous publish/subscribe bus.
//
// It exists to answer one question: when something happens in the game, who
// needs to know? Before this, whoever caused the thing also had to remember
// everyone it affected. Placing a structure meant calling Colony.Place, then
// spawning its entities, then writing a status line — three unrelated
// concerns, in one function, in an order nobody could derive from the rules.
// Forgetting the middle one left a building that existed but was not drawn.
//
// With a bus, the code that places a structure says "a structure was placed"
// and stops. The renderer and the interface each said, once, at startup, that
// they wanted to hear about that.
//
// # What belongs on a bus
//
// Events, not state. The distinction is the whole discipline:
//
//   - "A structure was demolished" is an event. It happens at an instant, it
//     is gone once handled, and nobody can ask about it later.
//   - "The colony has 250 iron" is state. It is true continuously, anyone can
//     read it at any time, and publishing it would create a second copy that
//     can disagree with the first.
//
// This game's resource panel does not subscribe to anything. It reads
// Colony.Readout every frame and draws what it finds, because that is state.
// Routing it through a bus would buy nothing and cost a cache to invalidate.
// Use a bus for the things that *happen*.
//
// # Design
//
// Delivery is synchronous, ordered, and on the caller's goroutine: when Emit
// returns, every handler has run. A game loop wants that. An asynchronous bus
// would mean an event placed on the frame a building went up might be handled
// on the next one, which is a whole category of bug for no benefit at this
// scale.
//
// There is no Unsubscribe, deliberately. Everything here subscribes during
// startup and lives as long as the process, so a handle to remove a
// subscription would be a lifetime to get wrong for a case that does not
// arise. If a game needs one — per-level handlers, say — the fix is to give
// each level its own Bus and drop it wholesale, rather than to unpick
// individual registrations.
package event

import "reflect"

// Bus routes events to handlers by their type.
//
// The zero Bus is not usable; call New. A Bus is not safe for concurrent use,
// which is the correct trade for a game loop that runs on one goroutine: the
// alternative is a mutex on the hot path to protect against a case that never
// happens.
type Bus struct {
	handlers map[reflect.Type][]func(any)

	// depth guards against a handler that emits the event it is handling.
	// That is an infinite loop, and without this it is one that ends in a
	// stack overflow with a backtrace thousands of frames deep — which says
	// nothing about which handler did it.
	depth int
}

// maxDepth is how many nested Emits are allowed before the bus concludes it is
// in a cycle. Legitimate chains are two or three deep: something happens, a
// handler reacts by causing something else.
const maxDepth = 16

// New returns an empty Bus.
func New() *Bus {
	return &Bus{handlers: make(map[reflect.Type][]func(any))}
}

// On registers a handler for one type of event.
//
// Handlers run in the order they were registered. Register at startup, before
// anything can be emitted, and the order is then something a reader can see in
// one place rather than something that depends on when each system woke up.
func On[T any](b *Bus, handle func(T)) {
	k := reflect.TypeFor[T]()
	b.handlers[k] = append(b.handlers[k], func(v any) { handle(v.(T)) })
}

// Emit delivers an event to every handler registered for its type, in order,
// before returning. An event nobody subscribed to is not an error — it is the
// normal case for a system that has not been written yet.
func Emit[T any](b *Bus, ev T) {
	if b == nil {
		// A Game built without a bus — as several tests are — should not have
		// to construct one to exercise a code path that happens to publish.
		return
	}

	if b.depth >= maxDepth {
		panic("event: handlers are emitting in a cycle; " +
			"a handler for one event type must not emit that same type")
	}
	b.depth++
	defer func() { b.depth-- }()

	// Ranging takes a snapshot of the slice header, so a handler that
	// registers another handler during delivery does not affect this pass —
	// its new handler starts receiving from the next event. That is the
	// behaviour worth having: a subscription list that grows while it is
	// being walked is the kind of thing that works until it does not.
	for _, handle := range b.handlers[reflect.TypeFor[T]()] {
		handle(ev)
	}
}
