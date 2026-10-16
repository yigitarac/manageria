package liveplay

import "testing"

// T-051 active-22 benchmarks and guard (isolated prototype evidence, TEST-ONLY).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record/scheduler
// buffers are rewound in place, so each number is the real microstep cost on a
// controlled always-active scene: a rolling free ball and 22 inertial sensing
// bodies genuinely moving every substep. Synthetic always-active workload, NOT a
// 90-minute 11v11 game; the <20 ms budget stays open.
//
// Three separated points:
//   - Engine22     : engine only (runtime baseline).
//   - Controller22 : engine + 22 test-only awareness controllers observing each
//     runner's sampled record once per substep (the ADDED controller cost).
//   - Diagnostics22: engine + the optional T-049 provenance recorder (diagnostic
//     cost, measured separately; TOTAL window cost, delta computed against the
//     engine baseline by the reader).

// BenchmarkT051Engine22: engine-only active-22 baseline (same fixture as T-050).
func BenchmarkT051Engine22(b *testing.B) {
	benchmarkT050Active22(b, func() []Runner { return t046BenchRunners(true) }, t046RearmBench)
}

// BenchmarkT051Controller22: the same active-22 scene plus 22 test-only awareness
// controllers, each observing its runner's sampled record every substep.
func BenchmarkT051Controller22(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t046BenchRunners(true))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	ctls := make([]t051Controller, len(sim.runners))
	t046RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t046RearmBench(sim)
		for j := range ctls {
			ctls[j] = newT051Controller()
		}
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
			for j := range ctls {
				if o, ok := sim.Perception(j); ok {
					ctls[j].Observe(o, t051AwarenessDelayMS)
				}
			}
		}
	}
}

// BenchmarkT051Diagnostics22: the active-22 scene with the optional T-049
// provenance recorder attached. TOTAL window cost; the diagnostic-only delta is
// this number minus BenchmarkT051Engine22 and is kept separate.
func BenchmarkT051Diagnostics22(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t046BenchRunners(true))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t046RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rec, err := NewProvenanceRecorder(8192)
		if err != nil {
			b.Fatal(err)
		}
		sim.SetProvenanceRecorder(rec)
		t046RearmBench(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
	b.StopTimer()
	sim.SetProvenanceRecorder(nil)
}

// TestT051Active22StaysActive guards the controller-attached window: the ball
// stays rolling, no contact fires, all 22 bodies move every substep, and the
// controllers never alter the engine.
func TestT051Active22StaysActive(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t046BenchRunners(true))
	ctls := make([]t051Controller, len(sim.runners))
	for w := 0; w < 3; w++ {
		t046RearmBench(sim)
		for j := range ctls {
			ctls[j] = newT051Controller()
		}
		clock0 := sim.ClockMS()
		prev := sim.Runners()
		for k := 0; k < t040Window; k++ {
			sim.Step()
			for j := range ctls {
				if o, ok := sim.Perception(j); ok {
					ctls[j].Observe(o, t051AwarenessDelayMS)
				}
			}
			if sim.Ball().Mode != BallRolling {
				t.Fatalf("window=%d: ball mode=%s at step %d", w, sim.Ball().Mode, k)
			}
			if ms, _ := sim.FirstContact(); ms >= 0 {
				t.Fatalf("window=%d: contact at step %d", w, k)
			}
			cur := sim.Runners()
			for i := range cur {
				if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
					t.Fatalf("window=%d: runner %d did not move at step %d", w, i, k)
				}
			}
			prev = cur
		}
		if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
			t.Fatalf("window=%d: clock advanced %d ms, want %d", w, got, t040Window*sim.Config().SubstepMS)
		}
	}
	t.Logf("active22: 22 sensing bodies all move every substep with controllers attached; no contact")
}
