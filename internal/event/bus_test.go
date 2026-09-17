package event

import "testing"

type placed struct{ Name string }
type removed struct{ Name string }

func TestHandlersOnlyHearTheirOwnType(t *testing.T) {
	b := New()

	var gotPlaced, gotRemoved []string
	On(b, func(e placed) { gotPlaced = append(gotPlaced, e.Name) })
	On(b, func(e removed) { gotRemoved = append(gotRemoved, e.Name) })

	Emit(b, placed{"habitat"})
	Emit(b, removed{"mine"})
	Emit(b, placed{"solar"})

	if len(gotPlaced) != 2 || gotPlaced[0] != "habitat" || gotPlaced[1] != "solar" {
		t.Errorf("placed handler got %v", gotPlaced)
	}
	if len(gotRemoved) != 1 || gotRemoved[0] != "mine" {
		t.Errorf("removed handler got %v", gotRemoved)
	}
}

// Several systems reacting to one event is the reason the bus exists, and they
// run in the order they registered — which is what makes the wiring readable
// in one place instead of being an emergent property of startup.
func TestEveryHandlerRunsInRegistrationOrder(t *testing.T) {
	b := New()

	var order []string
	On(b, func(placed) { order = append(order, "scene") })
	On(b, func(placed) { order = append(order, "status") })
	On(b, func(placed) { order = append(order, "audio") })

	Emit(b, placed{"habitat"})

	want := []string{"scene", "status", "audio"}
	if len(order) != len(want) {
		t.Fatalf("ran %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("ran %v, want %v", order, want)
		}
	}
}

// Delivery finishes before Emit returns. A game loop depends on this: the
// entity for a structure has to exist by the time the frame that placed it
// goes on to draw.
func TestDeliveryIsSynchronous(t *testing.T) {
	b := New()
	done := false
	On(b, func(placed) { done = true })

	Emit(b, placed{"habitat"})
	if !done {
		t.Error("Emit returned before its handler ran")
	}
}

// An event nobody wants is the normal state of a system that has not been
// written yet, not an error.
func TestEmittingWithNoHandlersIsFine(t *testing.T) {
	b := New()
	Emit(b, placed{"habitat"}) // must not panic
}

// A nil Bus swallows events, so a test can build a Game without one.
func TestANilBusIsInert(t *testing.T) {
	var b *Bus
	Emit(b, placed{"habitat"}) // must not panic
}

// Chains are legitimate: something happens, a handler reacts by causing
// something else. Only a handler emitting its *own* type is a cycle.
func TestAHandlerMayEmitADifferentEvent(t *testing.T) {
	b := New()

	var saw string
	On(b, func(e placed) { Emit(b, removed{e.Name + "-old"}) })
	On(b, func(e removed) { saw = e.Name })

	Emit(b, placed{"habitat"})
	if saw != "habitat-old" {
		t.Errorf("chained handler saw %q", saw)
	}
}

// The guard is worth more than the stack overflow it replaces: a panic naming
// the mistake beats four thousand identical frames.
func TestACycleIsReportedRatherThanOverflowingTheStack(t *testing.T) {
	b := New()
	On(b, func(e placed) { Emit(b, e) })

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a self-emitting handler did not panic")
		}
		msg, _ := r.(string)
		if msg == "" {
			t.Fatalf("panicked with %v, want a message naming the problem", r)
		}
	}()
	Emit(b, placed{"habitat"})
}

// Registering during delivery must not disturb the pass that is running.
func TestSubscribingDuringDeliveryTakesEffectNextTime(t *testing.T) {
	b := New()

	late := 0
	On(b, func(placed) {
		On(b, func(placed) { late++ })
	})

	Emit(b, placed{"one"})
	if late != 0 {
		t.Errorf("a handler registered mid-delivery ran %d times, want 0", late)
	}
	Emit(b, placed{"two"})
	if late != 1 {
		t.Errorf("the late handler ran %d times on the next event, want 1", late)
	}
}
