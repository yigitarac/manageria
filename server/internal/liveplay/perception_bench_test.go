package liveplay

import "testing"

// T-043 local sampled perception 22-body benchmark (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record buffers
// are rewound, so each number is the real microstep cost of the live path on a
// controlled always-active scene: a rolling free ball, all 22 bodies genuinely
// chasing it, the swept contact scan, path events and one sample. This is a
// synthetic always-active prototype workload, NOT a 90-minute 11v11 game.
//
// Three points isolate the cost:
//   - Disabled     : LivePass, scheduler off, no perception (the native path).
//   - SchedulerOnly: scheduler armed at 100 ms, still no perception.
//   - Sampling     : scheduler armed at 100 ms AND all 22 runners opted in with a
//     200 m radius, so every lattice publication refreshes all 22 records.
//
// The sampling sweep mutates each record IN PLACE (no per-sample allocation), so
// the sampling point should stay at 0 allocs/op.

// t043BenchRadiusM is large enough that every one of the 22 far-touchline chasing
// bodies is inside the sensing circle of the ball at the pitch lane.
const t043BenchRadiusM = 200.0

func t043BenchRunners() []Runner {
	rs := liveContestBenchRunners()
	for i := range rs {
		if err := rs[i].EnableSensing(t043BenchRadiusM); err != nil {
			panic(err)
		}
	}
	return rs
}

// t043RearmBench resets a fresh contest window INCLUDING the scheduler state and
// every perception record, so each timed window starts clean on the lattice.
func t043RearmBench(sim *Sim) {
	t042RearmBench(sim)
	for i := range sim.runners {
		if o := sim.runners[i].Perception; o != nil {
			*o = BallObservation{RadiusM: o.RadiusM}
		}
	}
}

func benchmarkT043(b *testing.B, cfg Config, runners []Runner) {
	sim, err := NewSim(cfg, 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(runners)
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t043RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t043RearmBench(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT043Perception22Disabled: native live path, scheduler disabled, no
// perception records. The baseline on this host.
func BenchmarkT043Perception22Disabled(b *testing.B) {
	benchmarkT043(b, LivePassConfig(), liveContestBenchRunners())
}

// BenchmarkT043Perception22SchedulerOnly: scheduler armed at 100 ms, no perception
// records (isolates the scheduling cost from the sampling cost).
func BenchmarkT043Perception22SchedulerOnly(b *testing.B) {
	benchmarkT043(b, ScheduledLivePassConfig(100), liveContestBenchRunners())
}

// BenchmarkT043Perception22Sampling: scheduler armed at 100 ms and all 22 runners
// opted in, so every lattice publication refreshes all 22 local records.
func BenchmarkT043Perception22Sampling(b *testing.B) {
	benchmarkT043(b, ScheduledLivePassConfig(100), t043BenchRunners())
}

// TestT043PerceptionBenchStaysActive guards every sampling benchmark window: the
// ball stays rolling, all 22 bodies move every substep, the scheduler publishes on
// the lattice, and every opted-in record is actually refreshed at the last
// publication.
func TestT043PerceptionBenchStaysActive(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t043BenchRunners())
	for w := 0; w < 3; w++ {
		t043RearmBench(sim)
		clock0 := sim.ClockMS()
		prev := sim.Runners()
		pubs := 0
		last := sim.DecisionPublishCount()
		for k := 0; k < t040Window; k++ {
			sim.Step()
			if sim.Ball().Mode != BallRolling {
				t.Fatalf("window=%d: ball mode=%s at step %d", w, sim.Ball().Mode, k)
			}
			if len(sim.Runners()) != 22 {
				t.Fatalf("window=%d: runner count %d", w, len(sim.Runners()))
			}
			cur := sim.Runners()
			for i := range cur {
				if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
					t.Fatalf("window=%d: runner %d did not move at step %d", w, i, k)
				}
			}
			prev = cur
			if c := sim.DecisionPublishCount(); c > last {
				last = c
				pubs++
			}
		}
		want := t040Window * sim.Config().SubstepMS / sim.Config().IntentCadenceMS
		if pubs != want {
			t.Fatalf("window=%d: %d publications, want %d lattice ticks", w, pubs, want)
		}
		// The last lattice publication inside the window is clock0 + 1900 ms; every
		// opted-in record must have been refreshed there.
		wantClock := clock0 + t040Window*sim.Config().SubstepMS - sim.Config().IntentCadenceMS
		refreshed := 0
		for i := 0; i < 22; i++ {
			o, ok := sim.Perception(i)
			if !ok {
				t.Fatalf("window=%d: runner %d has no perception record", w, i)
			}
			if o.Seen && o.ClockMS == wantClock && o.Freshness(sim.ClockMS()) == ObservationStale {
				refreshed++
			}
		}
		if refreshed != 22 {
			t.Fatalf("window=%d: only %d/22 records refreshed at %d ms", w, refreshed, wantClock)
		}
	}
}
