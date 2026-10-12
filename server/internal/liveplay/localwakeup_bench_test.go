package liveplay

import "testing"

// T-046 local event wakeup benchmarks and guards (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the scheduler/record
// buffers are rewound in place, so each number is the real microstep cost of the
// live path on a controlled always-active scene: a rolling free ball, 22 inertial
// bodies genuinely chasing it, the swept contact scan, path events and one sample.
// This is a synthetic always-active prototype workload, NOT a 90-minute 11v11 game.
//
// Three points isolate the local-wakeup cost:
//   - Disabled: LivePass, scheduler OFF (the native per-substep path).
//   - Shared  : scheduler armed at 100 ms, no local-wakeup opt-in (the pre-T-046
//     shared fast path, which must allocate no diagnostics).
//   - Local   : scheduler armed at 100 ms and all 22 runners opted into LocalWakeup
//     (per-runner gating + the local layer).

// t046BenchRunners builds the shared fixture: 22 chasing bodies. With local=false
// they use the native oracle (Pursue); with local=true they opt into the local
// policy AND per-runner local wakeup.
func t046BenchRunners(local bool) []Runner {
	rs := liveContestBenchRunners()
	if local {
		for i := range rs {
			rs[i].Pursue = false
			rs[i].LocalPursuit = true
			if err := rs[i].EnableLocalWakeup(t044BenchRadiusM); err != nil {
				panic(err)
			}
		}
	}
	return rs
}

// t046LocalBenchRunners builds a paired fixture for the T-046 incremental-cost
// measurement: 22 chasing bodies ALL on the local pursuit policy with the SAME
// sensing radius and the SAME scheduler. They differ ONLY in the per-runner local
// wakeup opt-in (wakeup=false keeps the exact pre-T-046 shared scheduling), so the
// pair isolates the T-046 wakeup-gating cost from the policy/sensing cost.
func t046LocalBenchRunners(wakeup bool) []Runner {
	rs := liveContestBenchRunners()
	for i := range rs {
		rs[i].Pursue = false
		rs[i].LocalPursuit = true
		if wakeup {
			if err := rs[i].EnableLocalWakeup(t044BenchRadiusM); err != nil {
				panic(err)
			}
		} else if err := rs[i].EnableSensing(t044BenchRadiusM); err != nil {
			panic(err)
		}
	}
	return rs
}

// t046RearmBench resets a fresh contest window INCLUDING the scheduler state, the
// local-wakeup evidence counters and every runner's pending flag. The diagnostic
// slices (localGate/localPubMS) are rewound IN PLACE, never re-allocated, so a
// re-armed window is steady-state.
func t046RearmBench(sim *Sim) {
	t042RearmBench(sim)
	sim.localWakePublications = 0
	for i := range sim.localPubMS {
		sim.localPubMS[i] = -1
	}
	for i := range sim.runners {
		sim.runners[i].WakePending = false
	}
}

func benchmarkT046(b *testing.B, cfg Config, runners []Runner, rearm func(*Sim)) {
	sim, err := NewSim(cfg, 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(runners)
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	rearm(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rearm(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT046Wakeup22Disabled: native live path, scheduler disabled.
func BenchmarkT046Wakeup22Disabled(b *testing.B) {
	benchmarkT046(b, LivePassConfig(), liveContestBenchRunners(), t042RearmBench)
}

// BenchmarkT046Wakeup22Shared: scheduler armed, no local-wakeup opt-in (fast path).
func BenchmarkT046Wakeup22Shared(b *testing.B) {
	benchmarkT046(b, ScheduledLivePassConfig(100), liveContestBenchRunners(), t042RearmBench)
}

// BenchmarkT046Wakeup22Local: scheduler armed, all 22 runners opted into
// per-runner local wakeup + local pursuit.
func BenchmarkT046Wakeup22Local(b *testing.B) {
	benchmarkT046(b, ScheduledLivePassConfig(100), t046BenchRunners(true), t046RearmBench)
}

// BenchmarkT046LocalPursuit22WakeupOff: local pursuit + sensing, NO local wakeup
// (the pre-T-046 shared scheduling). One half of the paired incremental-cost pair.
func BenchmarkT046LocalPursuit22WakeupOff(b *testing.B) {
	benchmarkT046(b, ScheduledLivePassConfig(100), t046LocalBenchRunners(false), t046RearmBench)
}

// BenchmarkT046LocalPursuit22WakeupOn: the SAME local pursuit + sensing + scheduler
// with the per-runner local wakeup opt-in. The pair isolates the T-046 cost.
func BenchmarkT046LocalPursuit22WakeupOn(b *testing.B) {
	benchmarkT046(b, ScheduledLivePassConfig(100), t046LocalBenchRunners(true), t046RearmBench)
}

// TestT046Active22StaysActive guards every window: the ball stays rolling, no
// contact fires, all 22 bodies move EVERY substep, the clock advances one substep
// per Step, and for the scheduled modes the scheduler publishes exactly on the
// lattice (no off-lattice wakeups on this event-free scene).
func TestT046Active22StaysActive(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		runners func() []Runner
		rearm   func(*Sim)
	}{
		{"disabled", LivePassConfig(), liveContestBenchRunners, t042RearmBench},
		{"shared", ScheduledLivePassConfig(100), liveContestBenchRunners, t042RearmBench},
		{"local", ScheduledLivePassConfig(100), func() []Runner { return t046BenchRunners(true) }, t046RearmBench},
		{"localpursuit-wakeup-off", ScheduledLivePassConfig(100), func() []Runner { return t046LocalBenchRunners(false) }, t046RearmBench},
		{"localpursuit-wakeup-on", ScheduledLivePassConfig(100), func() []Runner { return t046LocalBenchRunners(true) }, t046RearmBench},
	}
	for _, c := range cases {
		sim := mustSim(t, c.cfg, 1)
		sim.SetRunners(c.runners())
		for w := 0; w < 3; w++ {
			c.rearm(sim)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			pubs := 0
			last := sim.DecisionPublishCount()
			for k := 0; k < t040Window; k++ {
				sim.Step()
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("%s window=%d: ball mode=%s at step %d", c.name, w, sim.Ball().Mode, k)
				}
				if ms, _ := sim.FirstContact(); ms >= 0 {
					t.Fatalf("%s window=%d: contact at step %d", c.name, w, k)
				}
				cur := sim.Runners()
				if len(cur) != 22 {
					t.Fatalf("%s window=%d: runner count %d", c.name, w, len(cur))
				}
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("%s window=%d: runner %d did not move at step %d", c.name, w, i, k)
					}
				}
				prev = cur
				if cnt := sim.DecisionPublishCount(); cnt > last {
					last = cnt
					pubs++
				}
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("%s window=%d: clock advanced %d ms, want %d", c.name, w, got, t040Window*sim.Config().SubstepMS)
			}
			if c.cfg.IntentCadenceMS > 0 {
				want := t040Window * sim.Config().SubstepMS / sim.Config().IntentCadenceMS
				if pubs != want {
					t.Fatalf("%s window=%d: %d publications, want %d lattice ticks", c.name, w, pubs, want)
				}
			}
		}
	}
}

// TestT046DisabledNoAllocations proves the disabled path allocates nothing.
func TestT046DisabledNoAllocations(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(liveContestBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	allocs := testing.AllocsPerRun(20, func() {
		benchRearmLiveContest(sim)
		for k := 0; k < 50; k++ {
			sim.Step()
		}
	})
	if allocs != 0 {
		t.Fatalf("disabled local scheduling allocated %.2f allocs/run, want 0", allocs)
	}
	t.Logf("disabled local scheduling: 0 allocs/run")
}

// TestT046NoOptInNoAllocations proves the shared fast path (scheduler armed, no
// local-wakeup opt-in) allocates no diagnostics and allocates nothing per step.
func TestT046NoOptInNoAllocations(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(liveContestBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	allocs := testing.AllocsPerRun(20, func() {
		t042RearmBench(sim)
		for k := 0; k < 50; k++ {
			sim.Step()
		}
	})
	if allocs != 0 {
		t.Fatalf("shared (no opt-in) allocated %.2f allocs/run, want 0", allocs)
	}
	if sim.localGate != nil || sim.localPubMS != nil {
		t.Fatal("shared fast path allocated local diagnostics")
	}
	t.Logf("scheduler armed, no opt-in: 0 allocs/run and no local diagnostics")
}
