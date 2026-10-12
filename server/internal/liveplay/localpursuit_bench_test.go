package liveplay

import "testing"

// T-044 local pursuit 22-body benchmark (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record/scheduler
// buffers are rewound, so each number is the real microstep cost of the live path
// on a controlled always-active scene: a rolling free ball, all 22 bodies
// genuinely chasing it, the swept contact scan, path events and one sample. This
// is a synthetic always-active prototype workload, NOT a 90-minute 11v11 game.
//
// The two points share the SAME scheduler (100 ms) and the SAME sensing records
// (200 m), and differ ONLY in which intent path updates the chasing bodies:
//   - DisabledLocal: the native oracle (Pursue) publishes at the lattice;
//   - EnabledLocal : the runner opts into T-044 local pursuit and the native
//     oracle skips it; the local publisher chases the fresh local observation.
//
// The local layer's added work is one extra pass over 22 runners on a due step
// (reading each runner's own record); the sampling record already runs for both
// points, so the delta isolates the policy pass, not the recording.

// t044BenchRadiusM is large enough that every one of the 22 far-touchline chasing
// bodies is inside the sensing circle of the ball at the pitch lane.
const t044BenchRadiusM = 200.0

// t044BenchRunners builds the shared fixture: 22 chasing bodies, all opting into
// sensing, with the native oracle (false) or the local policy (true).
func t044BenchRunners(local bool) []Runner {
	rs := liveContestBenchRunners()
	for i := range rs {
		rs[i].Pursue = !local
		rs[i].LocalPursuit = local
		if err := rs[i].EnableSensing(t044BenchRadiusM); err != nil {
			panic(err)
		}
	}
	return rs
}

// t044RearmBench resets a fresh contest window INCLUDING the scheduler state and
// every perception record, so each timed window starts clean on the lattice.
// LocalPursuit flags persist on the runner structs (re-arm never clears them).
func t044RearmBench(sim *Sim) {
	t043RearmBench(sim)
}

func benchmarkT044Local(b *testing.B, local bool) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t044BenchRunners(local))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t044RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t044RearmBench(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT044LocalPursuit22DisabledLocal: same scheduler/sensing fixture, native
// oracle intent path (LocalPursuit off). The comparison baseline.
func BenchmarkT044LocalPursuit22DisabledLocal(b *testing.B) { benchmarkT044Local(b, false) }

// BenchmarkT044LocalPursuit22EnabledLocal: same scheduler/sensing fixture, all 22
// runners on the T-044 local policy (native oracle skipped).
func BenchmarkT044LocalPursuit22EnabledLocal(b *testing.B) { benchmarkT044Local(b, true) }

// TestT044LocalPursuitBenchStaysActive guards the benchmark windows: the ball
// stays rolling, all 22 bodies move every substep, the scheduler publishes on the
// lattice, every opted-in record is refreshed at the last publication, and the
// enabled-local intents actually chase the pre-step observation (a real target
// refresh, not a degenerate no-op).
func TestT044LocalPursuitBenchStaysActive(t *testing.T) {
	for _, local := range []bool{false, true} {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		sim.SetRunners(t044BenchRunners(local))
		if local && !sim.runners[0].LocalPursuitEnabled() {
			t.Fatal("enabled-local fixture runner is not active")
		}
		for w := 0; w < 3; w++ {
			t044RearmBench(sim)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			pubs := 0
			last := sim.DecisionPublishCount()
			for k := 0; k < t040Window; k++ {
				preBall := sim.ball
				sim.Step()
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("local=%v window=%d: ball mode=%s at step %d", local, w, sim.Ball().Mode, k)
				}
				cur := sim.Runners()
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("local=%v window=%d: runner %d did not move at step %d", local, w, i, k)
					}
				}
				prev = cur
				if c := sim.DecisionPublishCount(); c > last {
					last = c
					pubs++
					if local {
						// Every publication must refresh every local target to the
						// PRE-step observed ball position.
						for i := range cur {
							if cur[i].IntentX != preBall.X || cur[i].IntentY != preBall.Y {
								t.Fatalf("local=%v window=%d: runner %d intent (%v,%v) != observed (%v,%v)",
									local, w, i, cur[i].IntentX, cur[i].IntentY, preBall.X, preBall.Y)
							}
						}
					}
				}
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("local=%v window=%d: clock advanced %d ms, want %d", local, w, got, t040Window*sim.Config().SubstepMS)
			}
			want := t040Window * sim.Config().SubstepMS / sim.Config().IntentCadenceMS
			if pubs != want {
				t.Fatalf("local=%v window=%d: %d publications, want %d lattice ticks", local, w, pubs, want)
			}
			wantClock := clock0 + t040Window*sim.Config().SubstepMS - sim.Config().IntentCadenceMS
			refreshed := 0
			for i := 0; i < 22; i++ {
				o, ok := sim.Perception(i)
				if !ok {
					t.Fatalf("local=%v window=%d: runner %d has no perception record", local, w, i)
				}
				if o.Seen && o.ClockMS == wantClock {
					refreshed++
				}
			}
			if refreshed != 22 {
				t.Fatalf("local=%v window=%d: only %d/22 records refreshed at %d ms", local, w, refreshed, wantClock)
			}
		}
	}
}
