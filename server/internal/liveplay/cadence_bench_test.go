package liveplay

import "testing"

// T-041 active 22-body cadence-overlay cost (isolated prototype evidence).
//
// These windows measure the cost the cadence layer ADDS on top of the accepted
// 20 ms integration: a per-substep overlay decision plus, on cadence boundaries,
// a republish of all 22 perceivers' intents. The ball is a rolling free ball and
// all 22 bodies are genuinely chasing it (real microsteps), so the window stays
// on the active path. Reset happens OUTSIDE the timed region (b.StopTimer), and
// the per-substep cost is ns/op divided by the 100-substep window.
//
// Scene limitation: a controlled always-active 22-body prototype scene, NOT a
// 90-minute 11v11 game (no team AI, decisions, shots, keepers, aerials, fouls or
// restarts). Because no decision/AI cost exists yet, the overlay only measures
// intent publication; the cadence's real saving is a future decision-layer cost
// that this prototype cannot exhibit.

func t041BenchRunners() []Runner {
	p := DefaultMotionParams()
	rs := make([]Runner, 22)
	for i := range rs {
		x, y := liveContestBenchSlot(i)
		r := PursuingRunner(x, y, p, 0)
		r.Pursue = false // the external controller owns publication in this study
		rs[i] = r
	}
	return rs
}

func t041BenchPerceive() []bool {
	perceive := make([]bool, 22)
	for i := range perceive {
		perceive[i] = true
	}
	return perceive
}

func benchmarkT041CadenceOverlay(b *testing.B, period int, mode t041CadenceMode) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t041BenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	perceive := t041BenchPerceive()
	benchRearmLiveContest(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		benchRearmLiveContest(sim)
		ctrl := t041NewController(period, mode, perceive)
		ctrl.bind(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			ctrl.beforeStep(sim)
			sim.Step()
		}
	}
}

// BenchmarkT041CadenceOverlay22Period1: republish every substep (20 ms cadence).
func BenchmarkT041CadenceOverlay22Period1(b *testing.B) {
	benchmarkT041CadenceOverlay(b, 1, t041Periodic)
}

// BenchmarkT041CadenceOverlay22Period10: republish every 10th substep (200 ms).
func BenchmarkT041CadenceOverlay22Period10(b *testing.B) {
	benchmarkT041CadenceOverlay(b, 10, t041Periodic)
}

// BenchmarkT041CadenceOverlay22Event10: 200 ms cadence plus event-triggered
// invalidation.
func BenchmarkT041CadenceOverlay22Event10(b *testing.B) {
	benchmarkT041CadenceOverlay(b, 10, t041EventTriggered)
}

// TestT041CadenceBenchStaysActive guards the overlay windows: the ball stays
// rolling, no contact fires inside a window, all 22 bodies move every substep and
// the clock advances exactly one substep per step — so the benchmark never
// degenerates to a reset/dead path.
func TestT041CadenceBenchStaysActive(t *testing.T) {
	for _, tc := range []struct {
		period int
		mode   t041CadenceMode
	}{
		{1, t041Periodic},
		{10, t041Periodic},
		{10, t041EventTriggered},
	} {
		sim := mustSim(t, LivePassConfig(), 1)
		sim.SetRunners(t041BenchRunners())
		for w := 0; w < 3; w++ {
			benchRearmLiveContest(sim)
			ctrl := t041NewController(tc.period, tc.mode, t041BenchPerceive())
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			for k := 0; k < t040Window; k++ {
				ctrl.beforeStep(sim)
				sim.Step()
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("period=%d mode=%s: ball mode=%s at step %d", tc.period, tc.mode, sim.Ball().Mode, k)
				}
				if len(sim.Runners()) != 22 {
					t.Fatalf("period=%d mode=%s: runner count %d", tc.period, tc.mode, len(sim.Runners()))
				}
				if ms, _ := sim.FirstContact(); ms >= 0 {
					t.Fatalf("period=%d mode=%s: contact at step %d", tc.period, tc.mode, k)
				}
				cur := sim.Runners()
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("period=%d mode=%s: runner %d did not move at step %d", tc.period, tc.mode, i, k)
					}
				}
				prev = cur
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("period=%d mode=%s: clock advanced %d ms, want %d", tc.period, tc.mode, got, t040Window*sim.Config().SubstepMS)
			}
		}
	}
}
