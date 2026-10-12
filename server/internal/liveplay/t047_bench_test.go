package liveplay

import "testing"

// T-047 prediction 22-body benchmarks and guards (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the scheduler/record
// buffers are rewound in place, so each number is the real microstep cost of the
// live path on a controlled always-active scene. This is a synthetic always-active
// prototype workload, NOT a 90-minute 11v11 game.
//
// GEOMETRY (repair1): the old far-touchline scene gave ZERO reachable estimates
// under the 500 ms horizon (ball faster than the runner and moving away => no
// root). The new scene puts 22 bodies in a narrow off-lane band AHEAD of a fast
// (30 m/s) ball so that the optimistic envelope finds a short intercept for every
// body while the ball physically passes ~2 m clear (no contact) and outruns them.
//
// The two points share the SAME 22 LocalWakeup sensing bodies and the SAME
// scheduler (100 ms) and differ ONLY in the controller mode (independent T-044
// chase vs bounded within-age prediction), isolating the prediction cost.

// t047BenchSlot lays out 22 bodies in two off-lane rows ahead of the ball.
func t047BenchSlot(i int) (x, y float64) {
	return 0.24 + float64(i%11)*0.006, 0.47 + float64(i/11)*0.06
}

// t047BenchRunners builds 22 inertial LocalWakeup sensing bodies driven ONLY by
// the test controller.
func t047BenchRunners() []Runner {
	rs := make([]Runner, 22)
	for i := range rs {
		x, y := t047BenchSlot(i)
		rs[i] = t047Sensor(x, y, t044BenchRadiusM, 0)
	}
	return rs
}

// t047RearmBench resets a fresh contest window INCLUDING the scheduler state,
// the ball, every runner and its pending/observation record.
func t047RearmBench(sim *Sim) {
	t042RearmBench(sim)
	sim.ball = Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: 30.0}
	sim.contactMS, sim.contactRunner = -1, -1
	sim.releaseRunner = -1
	sim.immunityUntil = 0
	sim.lastPathMS = sim.clock
	sim.controlError = 0
	sim.firstTouchMS, sim.firstTouchRunner = -1, -1
	for i := range sim.runners {
		x, y := t047BenchSlot(i)
		r := &sim.runners[i]
		r.X, r.Y = x, y
		r.VXM, r.VYM = 0, 0
		r.IntentX, r.IntentY = x, y
		r.ReactAtMS = 0
		r.CooldownUntilMS = 0
		r.WakePending = false
		if o := r.Perception; o != nil {
			*o = BallObservation{RadiusM: o.RadiusM}
		}
	}
	sim.samples = sim.samples[:0]
	sim.events = sim.events[:0]
}

// t047ResetCtl resets the controller in place (no allocation).
func t047ResetCtl(c *t047Controller, mode t047Mode) {
	c.Mode, c.HorizonMS, c.MaxAgeMS = mode, t047HorizonMS, t047MaxAgeMS
	c.LastDecisionMS, c.Predictions, c.Fallbacks = -1, 0, 0
	c.LastEv = c.LastEv[:0]
}

func benchmarkT047(b *testing.B, mode t047Mode) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t047BenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.localGate = make([]bool, len(sim.runners))
	ctl := t047NewController(mode)
	ctl.LastEv = make([]t047RunnerEv, 0, len(sim.runners))
	t047RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t047RearmBench(sim)
		t047ResetCtl(ctl, mode)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			t047Step(sim, ctl)
		}
	}
}

// BenchmarkT047Intent22ChaseOff: the independent T-044 chase controller (the
// prediction-off baseline) on the shared 22-body LocalWakeup sensing fixture.
func BenchmarkT047Intent22ChaseOff(b *testing.B) { benchmarkT047(b, t047ModeChase) }

// BenchmarkT047Intent22Predicted: the SAME fixture with the bounded within-age
// prediction controller, so the pair isolates the prediction cost.
func BenchmarkT047Intent22Predicted(b *testing.B) { benchmarkT047(b, t047ModePredictWithinAge) }

// TestT047Active22StaysActive guards the windows: ball rolling, no contact, all
// 22 bodies move EVERY substep, exactly lattice publications, and (for the
// predicted modes) the prediction actually EXECUTED with a successful+fallback
// split reported per 100-step window.
func TestT047Active22StaysActive(t *testing.T) {
	for _, mode := range []t047Mode{t047ModeChase, t047ModePredictWithinAge} {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		sim.SetRunners(t047BenchRunners())
		sim.localGate = make([]bool, len(sim.runners))
		ctl := t047NewController(mode)
		ctl.LastEv = make([]t047RunnerEv, 0, len(sim.runners))
		for w := 0; w < 3; w++ {
			t047RearmBench(sim)
			t047ResetCtl(ctl, mode)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			pubs := 0
			for k := 0; k < t040Window; k++ {
				if t047Step(sim, ctl) {
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
			if mode != t047ModeChase && ctl.Predictions == 0 {
				t.Fatalf("%s window=%d: prediction never executed", mode, w)
			}
			t.Logf("%s (%s) window=%d: %d harness publications (100 steps); predictions=%d fallbacks=%d",
				mode, mode.Window(), w, pubs, ctl.Predictions, ctl.Fallbacks)
		}
	}
}

// TestT047PredictedNoAllocations proves the predicted controller path allocates
// nothing per step (the estimator uses only scalars; scratch is reused).
func TestT047PredictedNoAllocations(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t047BenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.localGate = make([]bool, len(sim.runners))
	ctl := t047NewController(t047ModePredictWithinAge)
	ctl.LastEv = make([]t047RunnerEv, 0, len(sim.runners))
	allocs := testing.AllocsPerRun(20, func() {
		t047RearmBench(sim)
		t047ResetCtl(ctl, t047ModePredictWithinAge)
		for k := 0; k < 50; k++ {
			t047Step(sim, ctl)
		}
	})
	if allocs != 0 {
		t.Fatalf("predicted controller allocated %.2f allocs/run, want 0", allocs)
	}
	t.Logf("predicted controller: 0 allocs/run")
}
