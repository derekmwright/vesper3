package game

import (
	"github.com/derekmwright/vesper3/internal/colony"
	"github.com/derekmwright/vesper3/internal/hex"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestMethaneFlareHasLongQuietInterval(t *testing.T) {
	for _, at := range []hex.Axial{{}, {Q: 4, R: 7}, {Q: -5, R: 2}} {
		lit := 0
		for i := 0; i < 10000; i++ {
			v := methaneFlare(float32(i)*.01, at)
			if v < 0 || v > 1.001 {
				t.Fatal(v)
			}
			if v > .01 {
				lit++
			}
		}
		if lit < 800 || lit > 2200 {
			t.Fatalf("unexpected duty cycle %d", lit)
		}
	}
}

func TestMethaneFlareUsesRotatedStackAndStopsOnRemoval(t *testing.T) {
	g := steamTestGame(t)
	at := hex.Axial{}
	g.Colony.Buildings = []colony.Building{{Kind: colony.Methane, At: at, Yield: 1}}
	g.Colony.Reindex()
	g.scene.structParts[partKey{colony.Methane, 1}] = []meshPart{{Name: "Amber runtime lamp"}}
	g.elapsed = .8
	for f := uint8(0); f < 6; f++ {
		p := g.methaneStackPosition(at, f)
		x, z := g.Map.Center(at)
		horizontal := mgl32.Vec3{p[0] - x, 0, p[2] - z}.Len()
		want := mgl32.Vec3{methaneStackLocal[0], 0, methaneStackLocal[2]}.Len() * modelScale
		if abs32(horizontal-want) > 1e-5 || p[1] <= g.Map.SurfaceY(at)+.9 {
			t.Fatal("bad stack transform", f, p)
		}
	}
	if n := len(g.stepBuildingParticles(.016)); n != flareParticles {
		t.Fatalf("particles %d", n)
	}
	g.Colony.Buildings = nil
	g.Colony.Reindex()
	if len(g.stepBuildingParticles(.016)) != 0 {
		t.Fatal("flame survived demolition")
	}
}

func TestMethaneFlareKeepsSteamAndBudget(t *testing.T) {
	g := steamTestGame(t)
	g.scene.structParts[partKey{colony.Methane, 1}] = []meshPart{{Name: "Amber runtime lamp"}}
	for i := 0; i < 1000; i++ {
		g.Colony.Buildings = append(g.Colony.Buildings, colony.Building{Kind: colony.Methane, At: hex.Axial{Q: int32(i % 12), R: int32(i / 12)}, Yield: 1})
	}
	g.Colony.Reindex()
	g.elapsed = .8
	g.stepBuildingParticles(.01)
	particles := g.stepBuildingParticles(.2)
	if len(g.scene.steam.instances) == 0 {
		t.Fatal("steam disappeared")
	}
	if len(particles) <= len(g.scene.steam.instances) || len(particles) > steamMaxInstances+flareMaxInstances {
		t.Fatal("bad combined budget", len(particles))
	}
	g.elapsed = 8
	// An unknown future tier mesh must not get the old stack socket.
	g.scene.structParts[partKey{colony.Methane, 2}] = []meshPart{{Name: "new geometry"}}
	if g.methaneHasStack(2) {
		t.Fatal("guessed a future stack")
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
