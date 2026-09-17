package game

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/derekmwright/glyphengine/input"
)

// Camera is the strategy-game camera: a point on the ground that the player
// drives around, with the eye orbiting above it.
//
// The engine ships an orbit camera (glyphengine.Camera) and this is not it.
// That one follows a character and clamps its distance to 20 units, which is
// about four tiles across at this grid's scale — an aerial view of a colony
// needs ten times that. What the engine actually provides is SetCamera(eye,
// center, up), and everything below is the arithmetic between a keyboard and
// those three vectors.
type Camera struct {
	// Focus is the ground point the camera looks at and the player moves.
	Focus mgl32.Vec3

	// Yaw turns around Focus; Pitch is the angle above the ground, where a
	// larger pitch looks more directly down.
	Yaw   float32
	Pitch float32

	// Distance is how far the eye sits from Focus.
	Distance float32

	// Bounds keep Focus over the map. Without them the player pans into empty
	// space and has no landmark to find their way back by.
	MinX, MinZ, MaxX, MaxZ float32

	// dragged is how far the pointer has moved since the right button went
	// down, in pixels. Right-drag orbits and right-click demolishes, so the
	// two have to be told apart, and the only thing that separates them is
	// whether the pointer moved before the button came back up.
	dragged float32
}

// Camera limits and rates. Pan speed is proportional to distance, which is
// what makes one keypress feel the same whether the player is inspecting a
// habitat or looking at the whole continent.
const (
	minDistance = 6
	maxDistance = 70

	minPitch = 0.22 // radians, about 13 degrees: almost side-on
	maxPitch = 1.45 // about 83 degrees: almost straight down

	panPerSecond    = 0.75 // multiplied by Distance
	fastPanFactor   = 2.5
	rotatePerSec    = 1.8
	pitchPerSec     = 1.1
	zoomPerNotch    = 0.12 // fraction of current distance
	dragSensitivity = 0.006
)

// NewCamera returns a camera looking at a point from a comfortable survey
// height.
func NewCamera(focus mgl32.Vec3) *Camera {
	return &Camera{
		Focus:    focus,
		Yaw:      0.6,
		Pitch:    0.85,
		Distance: 34,
	}
}

// SetBounds limits how far Focus can travel, in world XZ.
func (c *Camera) SetBounds(minX, minZ, maxX, maxZ float32) {
	c.MinX, c.MinZ, c.MaxX, c.MaxZ = minX, minZ, maxX, maxZ
}

// Update reads one frame of input. It must be called exactly once per frame
// from Game.Update: mouse delta and scroll are consumed, so calling it from a
// fixed-timestep callback would read them several times on one frame and not
// at all on the next.
func (c *Camera) Update(in *input.Input, dt float32) {
	forward, right := c.GroundAxes()

	speed := c.Distance * panPerSecond * dt
	if in.KeyDown(input.KeyLeftShift) || in.KeyDown(input.KeyRightShift) {
		speed *= fastPanFactor
	}

	var move mgl32.Vec3
	if in.KeyDown(input.KeyW) || in.KeyDown(input.KeyUp) {
		move = move.Add(forward)
	}
	if in.KeyDown(input.KeyS) || in.KeyDown(input.KeyDown) {
		move = move.Sub(forward)
	}
	if in.KeyDown(input.KeyD) || in.KeyDown(input.KeyRight) {
		move = move.Add(right)
	}
	if in.KeyDown(input.KeyA) || in.KeyDown(input.KeyLeft) {
		move = move.Sub(right)
	}
	if move.Len() > 0 {
		c.Focus = c.Focus.Add(move.Normalize().Mul(speed))
	}

	if in.KeyDown(input.KeyQ) {
		c.Yaw -= rotatePerSec * dt
	}
	if in.KeyDown(input.KeyE) {
		c.Yaw += rotatePerSec * dt
	}
	if in.KeyDown(input.KeyR) {
		c.Pitch += pitchPerSec * dt
	}
	if in.KeyDown(input.KeyF) {
		c.Pitch -= pitchPerSec * dt
	}

	// Right-drag orbits, and middle-drag does the same for anyone whose hand
	// already expects it. Left stays reserved for building. See ApplyDrag for
	// which way each axis goes.
	if in.MousePressed(input.MouseButtonRight) {
		c.BeginDrag()
	}
	if in.MouseDown(input.MouseButtonRight) || in.MouseDown(input.MouseButtonMiddle) {
		dx, dy := in.MouseDelta()
		c.ApplyDrag(dx, dy)
	}

	if _, sy := in.ConsumeScroll(); sy != 0 {
		c.Distance *= 1 - float32(sy)*zoomPerNotch
	}

	c.clamp()
}

// GroundAxes returns the camera's forward and right directions flattened onto
// the ground, which is what panning follows: pressing W walks the view up the
// screen regardless of how far the camera is tilted.
func (c *Camera) GroundAxes() (forward, right mgl32.Vec3) {
	sin, cos := sin32(c.Yaw), cos32(c.Yaw)
	forward = mgl32.Vec3{-sin, 0, -cos}
	right = mgl32.Vec3{cos, 0, -sin}
	return
}

// Eye is the camera position in world space.
func (c *Camera) Eye() mgl32.Vec3 {
	cp := cos32(c.Pitch)
	return c.Focus.Add(mgl32.Vec3{
		sin32(c.Yaw) * cp,
		sin32(c.Pitch),
		cos32(c.Yaw) * cp,
	}.Mul(c.Distance))
}

// ViewVectors returns the triple Engine.SetCamera takes.
func (c *Camera) ViewVectors() (eye, center, up mgl32.Vec3) {
	return c.Eye(), c.Focus, mgl32.Vec3{0, 1, 0}
}

// FrameAll pulls back far enough to see a box of the given width and depth.
// It is what the key that resets the view uses.
func (c *Camera) FrameAll(minX, minZ, maxX, maxZ float32) {
	c.Focus = mgl32.Vec3{(minX + maxX) / 2, c.Focus.Y(), (minZ + maxZ) / 2}
	extent := max32(maxX-minX, maxZ-minZ)
	c.Distance = min32(extent*0.9, maxDistance)
	c.Pitch = 1.05
	c.clamp()
}

func (c *Camera) clamp() {
	c.Distance = clampF(c.Distance, minDistance, maxDistance)
	c.Pitch = clampF(c.Pitch, minPitch, maxPitch)

	// Keep yaw in one turn so it stays readable in a debug line and never
	// drifts far enough to lose float precision in a long session.
	const twoPi = 2 * math.Pi
	c.Yaw = float32(math.Mod(float64(c.Yaw), twoPi))

	if c.MaxX > c.MinX {
		c.Focus[0] = clampF(c.Focus[0], c.MinX, c.MaxX)
	}
	if c.MaxZ > c.MinZ {
		c.Focus[2] = clampF(c.Focus[2], c.MinZ, c.MaxZ)
	}
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sin32(v float32) float32 { return float32(math.Sin(float64(v))) }
func cos32(v float32) float32 { return float32(math.Cos(float64(v))) }

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func sqrt32(v float32) float32 { return float32(math.Sqrt(float64(v))) }

func pow32(v, e float32) float32 { return float32(math.Pow(float64(v), float64(e))) }

// clickSlop is how far the pointer may travel between pressing and releasing
// the right button and still count as a click rather than an orbit.
//
// A few pixels, because a hand releasing a button moves: at zero, half of all
// intended clicks would be swallowed as tiny drags, and the player would
// conclude that demolish is unreliable rather than that their mouse wobbled.
const clickSlop = 5

// BeginDrag starts measuring a press. ApplyDrag orbits and accumulates how far
// the pointer has travelled since.
//
// Both are exported, and separate from Update, so the click-versus-drag split
// can be tested without a window — an Input needs a live GLFW context, and
// this is exactly the logic that would silently stop demolish working.
func (c *Camera) BeginDrag() { c.dragged = 0 }

func (c *Camera) ApplyDrag(dx, dy float64) {
	// Yaw follows the pointer: drag right and the ground goes right, the way
	// a map does. Pitch is inverted against it — drag down and the camera
	// tips toward looking straight down — which is the convention nearly
	// every strategy game uses and does not have to agree with yaw, because
	// the two are doing different things: one slides the world, the other
	// changes how far over your shoulder you are leaning.
	c.Yaw -= float32(dx) * dragSensitivity
	c.Pitch += float32(dy) * dragSensitivity
	c.dragged += float32(abs64(dx) + abs64(dy))
	c.clamp()
}

// Dragged reports whether the pointer has moved far enough since the press to
// count as an orbit rather than a click.
func (c *Camera) Dragged() bool { return c.dragged >= clickSlop }

// WasRightClick reports a right button release that did not orbit the camera,
// which is what the game should treat as a click.
func (c *Camera) WasRightClick(in *input.Input) bool {
	return in.MouseReleased(input.MouseButtonRight) && !c.Dragged()
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
