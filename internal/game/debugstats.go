package game

import (
	"fmt"
	"runtime"
	"sort"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
)

// The F3 readout's runtime half: what the process is doing, as opposed to what
// the colony is doing.
//
// It is here rather than in the resource readout because it answers a
// different question. The colony lines say whether the game is going well;
// these say whether the program is, and the two go wrong independently — a
// colony can be thriving while the heap climbs a megabyte a second.

// runtimeSampleEvery is how often the Go runtime is asked how it is doing.
//
// Not every frame, because runtime.ReadMemStats stops the world. At sixty
// frames a second that is sixty stop-the-world pauses a second to draw a
// number that changes slowly, which would make the diagnostic a cause of the
// stutter it exists to diagnose. Twice a second is faster than anyone reads
// and slow enough to cost nothing.
const runtimeSampleEvery = 0.5

// runtimeStats is the last sample taken, held so every frame between samples
// can draw the same numbers rather than no numbers.
type runtimeStats struct {
	mem        runtime.MemStats
	goroutines int

	at    float32 // elapsed time of the last sample
	taken bool
}

// sample refreshes the readout if it is stale. now is Game.elapsed.
func (r *runtimeStats) sample(now float32) {
	if r.taken && now-r.at < runtimeSampleEvery {
		return
	}
	runtime.ReadMemStats(&r.mem)
	r.goroutines = runtime.NumGoroutine()
	r.at, r.taken = now, true
}

// mib formats bytes as mebibytes, which is the unit every other tool reports a
// Go heap in.
func mib(b uint64) float64 { return float64(b) / (1 << 20) }

// runtimeLines is the process half of the readout: memory, goroutines, the
// entity count, and where the frame went on the GPU.
func (g *Game) runtimeLines(e *glyph.Engine) []string {
	g.ui.runtime.sample(g.elapsed)
	m := &g.ui.runtime.mem

	// The most recent GC pause. PauseNs is a 256-entry ring indexed by
	// collection number, so the last one written is at NumGC-1 modulo the
	// ring — the +255 is that minus one without underflowing at NumGC 0.
	var lastPause float64
	if m.NumGC > 0 {
		lastPause = float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6
	}

	lines := []string{
		fmt.Sprintf("heap   %6.1fM live  %6.1fM from os   stack %5.2fM   gc %d, last %.2fms",
			mib(m.HeapAlloc), mib(m.HeapSys), mib(m.StackInuse), m.NumGC, lastPause),
		fmt.Sprintf("proc   %6.1fM total from os   goroutines %d   next gc at %.1fM",
			mib(m.Sys), g.ui.runtime.goroutines, mib(m.NextGC)),
	}

	// Entities are the engine's count, not a count of this game's buildings:
	// one structure is several entities, and the terrain chunks, the cursor
	// and the placement ghost are entities too. The number worth watching is
	// whether it climbs while nothing is being built.
	entities := 0
	if e.Scene != nil && e.Scene.World() != nil {
		entities = e.Scene.World().EntityCount()
	}
	lines = append(lines, fmt.Sprintf("scene  %d entities   frame %d", entities, e.FrameCount()))

	if gpu := e.MeanGPUTimings(); gpu.Valid {
		lines = append(lines, fmt.Sprintf("gpu    %.2fms total%s", gpu.Total, topPasses(gpu, 4)))
	}
	return lines
}

// topPasses names the heaviest passes of the frame rather than all thirteen.
//
// All of them is a wall of near-zero numbers that has to be read to find the
// one that is not, and the question this readout answers is "what is the frame
// being spent on", which is the top of the list every time.
func topPasses(gpu renderer.GPUTimings, n int) string {
	type pass struct {
		p  renderer.Pass
		ms float32
	}
	ranked := make([]pass, 0, len(gpu.Pass))
	for i, ms := range gpu.Pass {
		if ms > 0 {
			ranked = append(ranked, pass{renderer.Pass(i), ms})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].ms > ranked[j].ms })

	out := ""
	for i, p := range ranked {
		if i == n {
			break
		}
		out += fmt.Sprintf("   %s %.2f", p.p, p.ms)
	}
	return out
}
