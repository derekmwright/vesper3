package game

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// Right-drag orbits and right-click demolishes, so the two have to be told
// apart by how far the pointer moved. Getting the threshold wrong is not a
// visible bug — it is demolish quietly failing about half the time, which
// reads as an unreliable game rather than as a binding.
func TestRightPressIsAClickUntilThePointerMoves(t *testing.T) {
	cases := []struct {
		name    string
		moves   [][2]float64
		wantHit bool // true = still a click
	}{
		{"no movement", nil, true},
		{"a pixel of wobble", [][2]float64{{1, 0}}, true},
		{"a hand releasing", [][2]float64{{1, 1}, {1, 0}}, true},
		{"a deliberate drag", [][2]float64{{20, 4}}, false},
		{"a slow drag adds up", [][2]float64{{2, 0}, {2, 0}, {2, 0}}, false},
		{"movement is direction-blind", [][2]float64{{-20, 0}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCamera(mgl32.Vec3{})
			c.BeginDrag()
			for _, m := range tc.moves {
				c.ApplyDrag(m[0], m[1])
			}
			if got := !c.Dragged(); got != tc.wantHit {
				t.Errorf("click=%v after %v, want %v", got, tc.moves, tc.wantHit)
			}
		})
	}
}

// A new press has to forget the last one, or one orbit disables demolish for
// the rest of the session.
func TestBeginDragForgetsThePreviousDrag(t *testing.T) {
	c := NewCamera(mgl32.Vec3{})

	c.BeginDrag()
	c.ApplyDrag(50, 50)
	if !c.Dragged() {
		t.Fatal("a 50 pixel drag did not register")
	}

	c.BeginDrag()
	if c.Dragged() {
		t.Error("a fresh press still counts as dragged")
	}
}

// The two axes deliberately go opposite ways, and neither is arbitrary:
// dragging right slides the ground right like a map, and dragging down tips
// the camera toward looking straight down like every other strategy game.
// Both are invisible in a screenshot and obvious in the hand, so they are
// pinned here.
func TestDragDirections(t *testing.T) {
	c := NewCamera(mgl32.Vec3{})
	before := c.Yaw
	c.BeginDrag()
	c.ApplyDrag(10, 0)
	if c.Yaw >= before {
		t.Errorf("dragging right moved yaw from %.3f to %.3f, want a decrease", before, c.Yaw)
	}

	c2 := NewCamera(mgl32.Vec3{})
	beforePitch := c2.Pitch
	c2.BeginDrag()
	c2.ApplyDrag(0, 10)
	if c2.Pitch <= beforePitch {
		t.Errorf("dragging down moved pitch from %.3f to %.3f, want an increase", beforePitch, c2.Pitch)
	}
}

// Orbiting must not be able to push the camera under the ground or past
// straight down, however far the pointer is dragged.
func TestDragStaysWithinThePitchLimits(t *testing.T) {
	c := NewCamera(mgl32.Vec3{})
	c.BeginDrag()
	for range 200 {
		c.ApplyDrag(0, 50)
	}
	if c.Pitch > maxPitch+1e-5 {
		t.Errorf("pitch %.3f above the %.3f limit", c.Pitch, maxPitch)
	}

	c.BeginDrag()
	for range 200 {
		c.ApplyDrag(0, -50)
	}
	if c.Pitch < minPitch-1e-5 {
		t.Errorf("pitch %.3f below the %.3f limit", c.Pitch, minPitch)
	}
	if math.IsNaN(float64(c.Yaw)) {
		t.Error("yaw went NaN")
	}
}
