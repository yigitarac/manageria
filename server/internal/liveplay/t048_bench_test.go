package liveplay

import "testing"

// T-048 three-mode 22-body benchmarks and guards (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the scheduler/record
// buffers are rewound in place, so each number is the real microstep cost of the
// live path on a controlled always-active scene. It is a synthetic always-active
// prototype workload, NOT a 90-minute 11v11 game.
//
// All three points share the SAME 22 LocalWakeup sensing bodies, the SAME
// scheduler (100 ms) and the SAME geometry (the T-047 off-lane, fast-ball scene
// reused verbatim), and differ ONLY in the controller mode (independent T-044
// chase vs the T-047 top-speed estimate vs the inertial own-body rollout), so the
// pair isolates the policy cost. All 22 bodies still integrate and move EVERY
// substep on every point.
//
// WORKLOAD LABEL: on this fixture the ball (30 m/s) outruns every body, so the
// inertial candidate produces ZERO successful predictions and the timed cost is
// the full-scan REJECTION workload (every fresh free record runs a full 600-step
// scan and then falls back to chase). That is the intended, honestly-labelled
// measurement here — it is NOT a benefit claim and the horizon is NOT tuned to
// force a positive prediction. The separate positive (feasible) decision cost is
// measured by BenchmarkT048InertialDecisionFeasible.

// t048ResetCtl resets the controller in place (no allocation).
func t048ResetCtl(c *t048Controller, mode t048Mode) {
	c.Mode, c.HorizonMS, c.TopSpeedHorizonMS, c.MaxAgeMS, c.MaxSteps = mode, t048HorizonMS, t048TopSpeedHorizonMS, t048MaxAgeMS, t048MaxSteps
	c.LastDecisionMS, c.Predictions, c.Fallbacks = -1, 0, 0
	c.RolloutRuns, c.RolloutSteps, c.RolloutMaxSteps = 0, 0, 0
	c.LastEv = c.LastEv[:0]
}

func benchmarkT048(b *testing.B, mode t048Mode) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t047BenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.localGate = make([]bool, len(sim.runners))
	ctl := t048NewController(mode)
	ctl.LastEv = make([]t048RunnerEv, 0, len(sim.runners))
	t047RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t047RearmBench(sim)
		t048ResetCtl(ctl, mode)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			t048Step(sim, ctl)
		}
	}
}

// BenchmarkT048Intent22ChaseOff: the independent T-044 chase (prediction-off baseline).
func BenchmarkT048Intent22ChaseOff(b *testing.B) { benchmarkT048(b, t048ModeChase) }

// BenchmarkT048Intent22TopSpeed: the SAME fixture with the T-047 top-speed estimate.
func BenchmarkT048Intent22TopSpeed(b *testing.B) { benchmarkT048(b, t048ModeTopSpeed) }

// BenchmarkT048Intent22Inertial: the SAME fixture with the new own-body rollout.
func BenchmarkT048Intent22Inertial(b *testing.B) { benchmarkT048(b, t048ModeInertial) }

// TestT048Active22StaysActive guards the windows for all three modes: ball
// rolling, no contact, all 22 bodies move EVERY substep, exactly the lattice
// publications, and the inertial candidate actually EXECUTED with its rollout
// step cost reported per window.
func TestT048Active22StaysActive(t *testing.T) {
	for _, mode := range []t048Mode{t048ModeChase, t048ModeTopSpeed, t048ModeInertial} {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		sim.SetRunners(t047BenchRunners())
		sim.localGate = make([]bool, len(sim.runners))
		ctl := t048NewController(mode)
		ctl.LastEv = make([]t048RunnerEv, 0, len(sim.runners))
		for w := 0; w < 3; w++ {
			t047RearmBench(sim)
			t048ResetCtl(ctl, mode)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			pubs := 0
			for k := 0; k < t040Window; k++ {
				if t048Step(sim, ctl) {
					pubs++
				}
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("%s window=%d: ball mode=%s at step %d", mode, w, sim.Ball().Mode, k)
				}
				if ms, _ := sim.FirstContact(); ms >= 0 {
					t.Fatalf("%s window=%d: contact at step %d", mode, w, k)
				}
				cur := sim.Runners()
				if len(cur) != 22 {
					t.Fatalf("%s window=%d: runner count %d", mode, w, len(cur))
				}
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("%s window=%d: runner %d did not move at step %d", mode, w, i, k)
					}
				}
				prev = cur
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("%s window=%d: clock advanced %d ms", mode, w, got)
			}
			want := t040Window * sim.Config().SubstepMS / sim.Config().IntentCadenceMS
			if pubs != want {
				t.Fatalf("%s window=%d: %d harness publications, want %d lattice ticks", mode, w, pubs, want)
			}
			if mode == t048ModeInertial {
				if ctl.RolloutRuns == 0 || ctl.RolloutSteps == 0 {
					t.Fatalf("%s window=%d: inertial candidate never executed (runs=%d steps=%d)", mode, w, ctl.RolloutRuns, ctl.RolloutSteps)
				}
				// STRICT full-scan rejection WORKLOAD guard. The 22-body fixture's
				// 30 m/s ball outruns every body, so the candidate produces no
				// successful prediction and the timed cost is the bounded candidate
				// scan itself: one rollout peaks at the full 600 integrate calls
				// (15 candidates x 5 substeps), and the deterministic per-window
				// total is 233200 calls (was 264000 before the repair1 boundary
				// rejection makes a predicted point past x=1 short-circuit a later
				// publication earlier). This is a WORKLOAD, not a benefit.
				if ctl.RolloutMaxSteps != 600 {
					t.Fatalf("%s window=%d: rollout max steps %d != 600 (full 15-candidate scan)", mode, w, ctl.RolloutMaxSteps)
				}
				if ctl.RolloutSteps != 233200 {
					t.Fatalf("%s window=%d: rollout steps %d != 233200 (deterministic full-scan rejection workload)", mode, w, ctl.RolloutSteps)
				}
			}
			t.Logf("%s window=%d: %d harness publications (100 steps); predictions=%d fallbacks=%d rolloutRuns=%d rolloutSteps=%d rolloutMaxSteps=%d",
				mode, w, pubs, ctl.Predictions, ctl.Fallbacks, ctl.RolloutRuns, ctl.RolloutSteps, ctl.RolloutMaxSteps)
		}
	}
}

// BenchmarkT048InertialDecisionFeasible measures the POSITIVE (feasible) candidate
// decision cost in isolation (one decision on the approach scene), complementing
// the 22-body full-scan-rejection workload benchmark above. It is NOT tuned to
// force benefit and makes no earlier-control claim.
func BenchmarkT048InertialDecisionFeasible(b *testing.B) {
	in := t048InertialInput{
		NowMS: 0, RX: 0.50, RY: 0.50, RVXM: 0, RVYM: 0,
		TopSpeedMPS: PlayerTopSpeedMPS, AccelMPS2: PlayerAccelMaxMPS2,
		DecelMPS2: PlayerDecelMPS2, LateralAccelMPS2: PlayerLateralAccelMaxMPS2,
		ReachM: ControlRadiusM, HorizonMS: t048HorizonMS, MaxAgeMS: t048MaxAgeMS,
		MaxSteps: t048MaxSteps, FreshOnly: true,
		Seen: true, ObsClockMS: 0, ObsX: 0.58, ObsY: 0.50, ObsVXM: -4.0, ObsMode: BallRolling, ObsRadiusM: 200,
	}
	if _, _, _, reached, steps, reason := t048InertialRollout(in); !reached || steps <= 0 || reason != t048ReasonReached {
		b.Fatalf("feasible decision not reached: reached=%v steps=%d reason=%q", reached, steps, reason)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t048InertialRollout(in)
	}
}

// TestT048ControllerNoAllocations proves the inertial controller path allocates
// nothing per step (the rollout uses only scalars and a stack value copy).
func TestT048ControllerNoAllocations(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t047BenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.localGate = make([]bool, len(sim.runners))
	ctl := t048NewController(t048ModeInertial)
	ctl.LastEv = make([]t048RunnerEv, 0, len(sim.runners))
	allocs := testing.AllocsPerRun(20, func() {
		t047RearmBench(sim)
		t048ResetCtl(ctl, t048ModeInertial)
		for k := 0; k < 50; k++ {
			t048Step(sim, ctl)
		}
	})
	if allocs != 0 {
		t.Fatalf("inertial controller allocated %.2f allocs/run, want 0", allocs)
	}
	t.Logf("inertial controller: 0 allocs/run")
}
