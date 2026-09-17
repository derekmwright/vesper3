package meshgen

import "math"

func sqrt32(v float32) float32 { return float32(math.Sqrt(float64(v))) }

func normalize(v [3]float32) [3]float32 {
	l := v[0]*v[0] + v[1]*v[1] + v[2]*v[2]
	if l == 0 {
		return [3]float32{0, 1, 0}
	}
	inv := 1 / sqrt32(l)
	return [3]float32{v[0] * inv, v[1] * inv, v[2] * inv}
}
