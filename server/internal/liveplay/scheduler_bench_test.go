package liveplay

import "testing"

// T-042 opt-in scheduler 22-body benchmark (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record buffers
// are rewound, so the number is the real microstep cost of the SCHEDULED live
// path on a controlled always-active scene: a rolling free ball, all 22 bodies
// genuinely chasing it, the swept contact scan, path events and one sample. This
// is a synthetic always-active prototype workload, NOT a 90-minute 11v11 game.
//
// The scheduler's only added work is (a) an O(1) per-substep due test and (b) on
// a due step, one publishPursuitIntents pass. Cadence changes WHEN intents
// refresh, not the integration cost, which still runs every substep for all 22
// bodies.

// t042RearmBench resets a fresh contest window INCLUDING the scheduler state, so
// each timed window starts on the lattice at the re-armed clock.
func t042RearmBench(sim *Sim) {
	benchRearmLiveContest(sim)
	sim.nextDecisionMS = 0
	sim.decisionDirty = false
	sim.decisionPublishes = 0
	sim.lastDecisionMS = -1
}

func benchmarkT042Scheduler22(b *testing.B, cadenceMS int) {
	sim, err := NewSim(ScheduledLivePassConfig(cadenceMS), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveContestBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t042RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t042RearmBench(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT042Scheduler22Cadence20 republishes intents every substep (the
// equivalence anchor cadence).
func BenchmarkT042Scheduler22Cadence20(b *testing.B) { benchmarkT042Scheduler22(b, SubstepMS) }

// BenchmarkT042Scheduler22Cadence100: 100 ms cadence.
func BenchmarkT042Scheduler22Cadence100(b *testing.B) { benchmarkT042Scheduler22(b, 100) }

// BenchmarkT042Scheduler22Cadence200: 200 ms cadence.
func BenchmarkT042Scheduler22Cadence200(b *testing.B) { benchmarkT042Scheduler22(b, 200) }

// TestT042SchedulerBenchStaysActive guards every benchmark window: the ball stays
// rolling, no contact fires, all 22 bodies move every substep, the clock advances
// exactly one substep per Step, and the scheduler publishes exactly on the
// lattice (count = window / cadence). It also proves the path stays active rather
// than degenerating into a reset/dead path.
func TestT042SchedulerBenchStaysActive(t *testing.T) {
	for _, cadence := range []int{SubstepMS, 100, 200} {
		sim := mustSim(t, ScheduledLivePassConfig(cadence), 1)
		sim.SetRunners(liveContestBenchRunners())
		for w := 0; w < 3; w++ {
			t042RearmBench(sim)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			pubs := 0
			last := sim.DecisionPublishCount()
			for k := 0; k < t040Window; k++ {
				sim.Step()
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("cadence=%d window=%d: ball mode=%s at step %d", cadence, w, sim.Ball().Mode, k)
				}
				if len(sim.Runners()) != 22 {
					t.Fatalf("cadence=%d window=%d: runner count %d", cadence, w, len(sim.Runners()))
				}
				if ms, _ := sim.FirstContact(); ms >= 0 {
					t.Fatalf("cadence=%d window=%d: contact at step %d", cadence, w, k)
				}
				cur := sim.Runners()
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("cadence=%d window=%d: runner %d did not move at step %d", cadence, w, i, k)
					}
				}
				prev = cur
				if c := sim.DecisionPublishCount(); c > last {
					last = c
					pubs++
				}
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("cadence=%d window=%d: clock advanced %d ms, want %d", cadence, w, got, t040Window*sim.Config().SubstepMS)
			}
			want := t040Window * sim.Config().SubstepMS / cadence
			if pubs != want {
				t.Fatalf("cadence=%d window=%d: %d publications, want %d lattice ticks", cadence, w, pubs, want)
			}
			// The free travel must stay on the recorded path (path events), so the
			// window measures the real active rolling path, not a degenerate one.
			if paths := countKind(sim, PassEventPath); paths < 10 {
				t.Fatalf("cadence=%d window=%d: only %d path events in a 2 s rolling window", cadence, w, paths)
			}
		}
	}
}
