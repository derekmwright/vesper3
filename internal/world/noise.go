package world

import (
	"math"
	"math/rand/v2"
)

// perlin is a classic 2D gradient noise source. It is seeded from a PCG
// stream rather than a time or a map iteration, so the same seed gives the
// same planet on every machine and every run — which is the only reason a
// seed is worth showing the player at all.
type perlin struct {
	perm [512]int
}

func newPerlin(seed int64) *perlin {
	p := &perlin{}
	var base [256]int
	for i := range base {
		base[i] = i
	}
	rng := rand.New(rand.NewPCG(uint64(seed), 0x9E3779B97F4A7C15))
	rng.Shuffle(len(base), func(i, j int) { base[i], base[j] = base[j], base[i] })
	for i := 0; i < 512; i++ {
		p.perm[i] = base[i&255]
	}
	return p
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func lerp(a, b, t float64) float64 { return a + t*(b-a) }

// grad projects onto one of eight unit-ish gradients chosen by the hash.
func grad(h int, x, y float64) float64 {
	switch h & 7 {
	case 0:
		return x + y
	case 1:
		return -x + y
	case 2:
		return x - y
	case 3:
		return -x - y
	case 4:
		return x
	case 5:
		return -x
	case 6:
		return y
	default:
		return -y
	}
}

// at returns noise in roughly [-1, 1].
func (p *perlin) at(x, y float64) float64 {
	xi, yi := int(math.Floor(x)), int(math.Floor(y))
	xf, yf := x-float64(xi), y-float64(yi)
	xi &= 255
	yi &= 255

	u, v := fade(xf), fade(yf)

	aa := p.perm[p.perm[xi]+yi]
	ab := p.perm[p.perm[xi]+yi+1]
	ba := p.perm[p.perm[xi+1]+yi]
	bb := p.perm[p.perm[xi+1]+yi+1]

	x1 := lerp(grad(aa, xf, yf), grad(ba, xf-1, yf), u)
	x2 := lerp(grad(ab, xf, yf-1), grad(bb, xf-1, yf-1), u)
	return lerp(x1, x2, v)
}

// fbm sums octaves at doubling frequency and halving amplitude, normalised so
// the result stays in about [-1, 1] however many octaves are asked for.
func (p *perlin) fbm(x, y float64, octaves int, lacunarity, gain float64) float64 {
	var sum, amp, norm float64 = 0, 1, 0
	freq := 1.0
	for i := 0; i < octaves; i++ {
		sum += amp * p.at(x*freq, y*freq)
		norm += amp
		amp *= gain
		freq *= lacunarity
	}
	if norm == 0 {
		return 0
	}
	return sum / norm
}

// ridged folds the noise at zero and inverts it, turning smooth hills into
// the sharp spines a volcanic world wants.
func (p *perlin) ridged(x, y float64, octaves int) float64 {
	var sum, amp, norm float64 = 0, 1, 0
	freq := 1.0
	for i := 0; i < octaves; i++ {
		n := 1 - math.Abs(p.at(x*freq, y*freq))
		sum += amp * n * n
		norm += amp
		amp *= 0.5
		freq *= 2
	}
	if norm == 0 {
		return 0
	}
	return sum/norm*2 - 1
}
