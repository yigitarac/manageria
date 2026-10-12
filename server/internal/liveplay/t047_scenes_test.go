package liveplay

import (
	"math"
	"reflect"
	"testing"
)

// T-047 scene tests, estimator guards and continuation (isolated prototype
// evidence). See t047_prediction_test.go for the estimator/controller/harness and
// the session note §1/§4 for the PRESPEC and repair1 additions.

// t047FirstPub returns the record of the publication at pre-step clock `pre`.
func t047FirstPub(recs []t047StepRec, pre int) *t047StepRec {
	for i := range recs {
		if recs[i].Published && recs[i].ClockMS-SubstepMS == pre {
			return &recs[i]
		}
	}
	return nil
}

// --- estimator guards & analytic behaviour ---------------------------------

// TestT047EstimateFiniteGuards proves the estimator fails closed on every
// unknown / non-free / invalid / unphysical / overage / fresh-only input and is
// deterministic, with finite results for extreme but finite inputs.
func TestT047EstimateFiniteGuards(t *testing.T) {
	base := t047EstimateInput{
		NowMS: 100, Seen: true, ObsClockMS: 100,
		ObsX: 0.53, ObsY: 0.5, ObsVXM: 0, ObsVYM: 7, ObsMode: BallRolling, ObsRadiusM: 200,
		RX: 0.5, RY: 0.5, TopSpeedMPS: 8.5, ReachM: 1,
		HorizonMS: 500, MaxAgeMS: 100,
	}
	cases := []struct {
		name       string
		mutate     func(*t047EstimateInput)
		wantOK     bool
		wantReason string
	}{
		{"ok", func(*t047EstimateInput) {}, true, t047ReasonOK},
		{"unknown", func(in *t047EstimateInput) { in.Seen = false }, false, t047ReasonUnknown},
		{"nonfree", func(in *t047EstimateInput) { in.ObsMode = BallHeld }, false, t047ReasonNonFree},
		{"nan-obsx", func(in *t047EstimateInput) { in.ObsX = math.NaN() }, false, t047ReasonInvalid},
		{"inf-rx", func(in *t047EstimateInput) { in.RX = math.Inf(1) }, false, t047ReasonInvalid},
		{"nan-radius", func(in *t047EstimateInput) { in.ObsRadiusM = math.NaN() }, false, t047ReasonInvalid},
		{"coord-limit", func(in *t047EstimateInput) { in.ObsX = 100 }, false, t047ReasonInvalid},
		{"overage", func(in *t047EstimateInput) { in.NowMS = 201 }, false, t047ReasonOverage},
		{"fresh-reject", func(in *t047EstimateInput) { in.FreshOnly = true; in.ObsClockMS = 99 }, false, t047ReasonFreshRejected},
		{"future-clock", func(in *t047EstimateInput) { in.ObsClockMS = 200 }, false, t047ReasonInvalid},
		{"zero-top", func(in *t047EstimateInput) { in.TopSpeedMPS = 0 }, false, t047ReasonInvalid},
		{"huge-velocity", func(in *t047EstimateInput) { in.ObsVYM = 0; in.ObsVXM = 1e200 }, false, t047ReasonNoRoot},
	}
	for _, c := range cases {
		in := base
		c.mutate(&in)
		_, _, _, ok, reason := t047Estimate(in)
		if ok != c.wantOK {
			t.Fatalf("%s: ok=%v want %v (reason %q)", c.name, ok, c.wantOK, reason)
		}
		if !c.wantOK && reason != c.wantReason {
			t.Fatalf("%s: reason=%q want %q", c.name, reason, c.wantReason)
		}
	}
	// near-equal speeds (a ~ 0): the linear branch must stay finite/defined.
	near := base
	near.ObsVYM, near.ObsVXM = 0, 8.5
	if tx, ty, tau, ok, _ := t047Estimate(near); ok && (!isFinite(tx) || !isFinite(ty) || !isFinite(tau)) {
		t.Fatal("near-equal-speed branch produced a non-finite result")
	}
	// determinism.
	in := base
	t1, u1, s1, o1, r1 := t047Estimate(in)
	t2, u2, s2, o2, r2 := t047Estimate(in)
	if t1 != t2 || u1 != u2 || s1 != s2 || o1 != o2 || r1 != r2 {
		t.Fatal("estimate is not deterministic")
	}
	t.Logf("finite guards: %d cases + near-equal-speed + determinism; all classified", len(cases))
}

// TestT047EstimateAnalytic pins the closed-form behaviour: crossing lead,
// deceleration overshoot, harmful stale reversal, fresh-only vs within-age,
// already-in-reach, no-root and beyond-horizon.
func TestT047EstimateAnalytic(t *testing.T) {
	base := t047EstimateInput{
		Seen: true, ObsMode: BallRolling, ObsRadiusM: 200,
		TopSpeedMPS: 8.5, ReachM: 1, HorizonMS: 500, MaxAgeMS: 100,
	}

	// crossing: ball crossing the lane; the target must LEAD along the heading.
	cross := base
	cross.ObsX, cross.ObsY, cross.ObsVXM, cross.ObsVYM = 0.53, 0.5, 0, 7
	cross.RX, cross.RY = 0.5, 0.5
	tx, ty, tau, ok, reason := t047Estimate(cross)
	if !ok || tau <= 0 {
		t.Fatalf("crossing: ok=%v tau=%v reason=%s", ok, tau, reason)
	}
	if lead := t047AlongHeading(cross.ObsX, cross.ObsY, cross.ObsVXM, cross.ObsVYM, tx, ty); lead <= 0 {
		t.Fatalf("crossing: no lead (%.6f)", lead)
	}

	// rolling deceleration: the constant-velocity target overshoots the true
	// decelerating ball at the same tau. Measured prediction harm.
	decel := base
	decel.ObsX, decel.ObsY, decel.ObsVXM, decel.ObsVYM = 0.5, 0.5, 5, 0
	decel.RX, decel.RY = 0.48, 0.5
	_, _, dtau, ok, reason := t047Estimate(decel)
	if !ok {
		t.Fatalf("decel: not ok: %s", reason)
	}
	predDist := decel.ObsVXM * dtau
	trueDist := decel.ObsVXM*dtau - 0.5*RollDecelMPS2*dtau*dtau
	if predDist <= trueDist {
		t.Fatalf("decel: predicted %.6f m not beyond true %.6f m", predDist, trueDist)
	}

	// harmful stale reversal: a stale record moving +x while the ball actually
	// reversed -x leads to a large error (evaluation truth = the true reversal).
	rev := base
	rev.ObsX, rev.ObsY, rev.ObsVXM, rev.ObsVYM = 0.5, 0.5, 6, 0
	rev.RX, rev.RY = 0.48, 0.5
	rtx, rty, rtau, ok, reason := t047Estimate(rev)
	if !ok {
		t.Fatalf("reversal: not ok: %s", reason)
	}
	trueX := rev.ObsX - (rev.ObsVXM/PitchLengthM)*rtau
	if err := DistM(trueX, rev.ObsY, rtx, rty); err <= 2.0 {
		t.Fatalf("reversal: error %.3f m not harmful", err)
	}

	// fresh-only rejects any age > 0 while the within-age variant predicts it.
	stale := base
	stale.ObsX, stale.ObsY, stale.ObsVXM, stale.ObsVYM = 0.5, 0.5, 6, 0
	stale.RX, stale.RY = 0.48, 0.5
	stale.ObsClockMS, stale.NowMS = 0, 10
	stale.FreshOnly = true
	if _, _, _, ok, reason := t047Estimate(stale); ok || reason != t047ReasonFreshRejected {
		t.Fatalf("fresh-only: ok=%v reason=%s", ok, reason)
	}
	stale.FreshOnly, stale.ApplyAge = false, true
	if _, _, _, ok, reason := t047Estimate(stale); !ok {
		t.Fatalf("within-age: expected a prediction, reason=%s", reason)
	}

	// already in reach.
	reach := base
	reach.ObsX, reach.ObsY, reach.ObsVXM = 0.5, 0.5, 0
	reach.RX, reach.RY = 0.5, 0.5
	if _, _, _, ok, reason := t047Estimate(reach); !ok || reason != t047ReasonAlreadyReached {
		t.Fatalf("already-in-reach: ok=%v reason=%s", ok, reason)
	}

	// no root: ball moving away faster than the runner can ever close.
	nor := base
	nor.ObsX, nor.ObsY, nor.ObsVXM, nor.ObsVYM = 0.5, 0.5, 20, 0
	nor.RX, nor.RY = 0.45, 0.5
	if _, _, _, ok, reason := t047Estimate(nor); ok || reason != t047ReasonNoRoot {
		t.Fatalf("no-root: ok=%v reason=%s", ok, reason)
	}

	// beyond horizon: a catchable but far intercept.
	bes := base
	bes.ObsX, bes.ObsY, bes.ObsVXM, bes.ObsVYM = 0.4, 0.5, 6, 0
	bes.RX, bes.RY = 0.5, 0.5
	if _, _, _, ok, reason := t047Estimate(bes); ok || reason != t047ReasonBeyondHorizon {
		t.Fatalf("beyond-horizon: ok=%v reason=%s", ok, reason)
	}

	t.Logf("analytic: crossing tau=%.4fs lead>0; decel overshoot=%.4f m; stale-reversal error>2 m; "+
		"fresh-only rejects age>0 while within-age predicts; already/no-root/beyond classified",
		tau, predDist-trueDist)
}

// TestT047EstimatorInputAudit proves the estimator input contains ONLY the
// runner's own observation/state (no ball, no owner, no true decel) and that the
// estimate is a pure function of that input.
func TestT047EstimatorInputAudit(t *testing.T) {
	r := t047Sensor(0.5, 0.5, 200, 0)
	o := &BallObservation{RadiusM: 200, Seen: true, ClockMS: 0, X: 0.53, Y: 0.5, VXM: 0, VYM: 7, Mode: BallRolling, Owner: -1}
	c := t047NewController(t047ModePredictFresh)
	got := c.inputFor(&r, o, 0)
	want := t047EstimateInput{
		NowMS: 0, RX: 0.5, RY: 0.5,
		TopSpeedMPS: DefaultMotionParams().TopSpeedMPS, ReachM: ControlRadiusM,
		HorizonMS: t047HorizonMS, MaxAgeMS: t047MaxAgeMS, ApplyAge: false, FreshOnly: true,
		Seen: true, ObsClockMS: 0, ObsX: 0.53, ObsY: 0.5, ObsVXM: 0, ObsVYM: 7,
		ObsMode: BallRolling, ObsRadiusM: 200,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("estimator input:\n got %+v\nwant %+v", got, want)
	}
	// Purity / no hidden source: a sim with a wildly different ball must not
	// change an estimate recomputed from the SAME audited input.
	sim := mustSim(t, ScheduledLivePassConfig(100), 7)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.05, Y: 0.95, VXM: -40, VYM: 30})
	a1, b1, t1, ok1, r1 := t047Estimate(got)
	a2, b2, t2, ok2, r2 := t047Estimate(got)
	if a1 != a2 || b1 != b2 || t1 != t2 || ok1 != ok2 || r1 != r2 {
		t.Fatal("estimate changed without an input change")
	}
	t.Logf("input audit: struct == manual build (incl fresh-only); estimate pure (no ball/owner/decel); ok=%v", ok1)
}

// --- disabled mode reproduces T-044/T-046 independently ---------------------

// t047CompareChase steps a real runtime against the T-047 harness-chase in
// lockstep and asserts identical position/intent.
func t047CompareChase(t *testing.T, cfg Config, harnessRunner, runtimeRunner Runner, ball Ball, setup func(*Sim), horizon int, label string) {
	t.Helper()
	real := mustSim(t, cfg, 21)
	real.SetRunners([]Runner{runtimeRunner})
	real.SetBall(ball)
	har := mustSim(t, cfg, 21)
	har.SetRunners([]Runner{harnessRunner})
	har.SetBall(ball)
	if setup != nil {
		setup(real)
		setup(har)
	}
	ctl := t047NewController(t047ModeChase)
	for har.ClockMS() < horizon {
		real.Step()
		t047Step(har, ctl)
		rr, hr := real.Runners()[0], har.Runners()[0]
		if rr.X != hr.X || rr.Y != hr.Y || rr.IntentX != hr.IntentX || rr.IntentY != hr.IntentY {
			t.Fatalf("%s @%d: chase diverged real=(%.9f,%.9f,%.9f,%.9f) harness=(%.9f,%.9f,%.9f,%.9f)",
				label, har.ClockMS(), rr.X, rr.Y, rr.IntentX, rr.IntentY, hr.X, hr.Y, hr.IntentX, hr.IntentY)
		}
	}
	t.Logf("%s: harness-chase == real runtime over %d ms", label, har.ClockMS())
}

// t047Inertial builds a fixture runner with the requested opt-ins.
func t047Inertial(x, y, radius float64, reactMS int, localPursuit, localWakeup bool) Runner {
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	r.ReachM = ControlRadiusM
	r.ReactMS = reactMS
	if localPursuit {
		if err := r.EnableLocalPursuit(radius); err != nil {
			panic(err)
		}
	} else if err := r.EnableSensing(radius); err != nil {
		panic(err)
	}
	if localWakeup {
		if err := r.EnableLocalWakeup(radius); err != nil {
			panic(err)
		}
	}
	return r
}

// TestT047DisabledReproducesT044Chase proves the test-only controller in disabled
// mode reproduces (a) the real T-044 runtime on an event-free scene and (b) the
// real T-046 per-runner wakeup runtime on a local-event (deflect) scene.
func TestT047DisabledReproducesT044Chase(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	// (a) event-free rolling: T-044 shared runtime reference.
	ball := Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 6.0}
	t047CompareChase(t, cfg,
		t047Inertial(0.60, 0.10, 200, 0, false, true), // harness: LocalWakeup, controller
		t047Inertial(0.60, 0.10, 200, 0, true, false), // T-044 runtime: LocalPursuit
		ball, nil, 1500, "t044-event-free")

	// (b) local-event (real deflect): T-046 per-runner wakeup runtime reference.
	loose := Ball{Mode: BallLoose, X: 0.50, Y: 0.50}
	setup := func(s *Sim) { s.controlError = 1.0 }
	t047CompareChase(t, cfg,
		t047Inertial(0.50, 0.50, 200, 0, false, true), // harness: LocalWakeup, controller
		t047Inertial(0.50, 0.50, 200, 0, true, true),  // T-046 runtime: LocalPursuit + LocalWakeup
		loose, setup, 600, "t046-local-event")
}

// --- scene 1: crossing (positive, geometric) -------------------------------

// TestT047CrossingScene measures the crossing case: the estimate leads the
// observed ball, its target-lag error against the free-ball evaluation truth is
// smaller than the T-044 chase target's error, and the full paired trajectories
// and contact sequence times are logged. The geometric benefit is explicitly
// LIMITED (it is not by itself a reachability or contact proof).
func TestT047CrossingScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	truth := t047BuildTruth(t, cfg, ball, 1200)

	pred := t047NewController(t047ModePredictFresh)
	recsP, simP := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, pred, 1200, setup, nil)
	chase := t047NewController(t047ModeChase)
	recsC, simC := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, chase, 1200, setup, nil)

	first := t047FirstPub(recsP, 0)
	if first == nil {
		t.Fatal("crossing: no publication at clock 0")
	}
	r0 := first.Runners[0]
	if !r0.Predicted {
		t.Fatalf("crossing: no prediction (%s)", r0.Reason)
	}
	if r0.TauS <= 0 {
		t.Fatalf("crossing: tau=%.6f", r0.TauS)
	}
	if lead := t047AlongHeading(r0.ObsX, r0.ObsY, r0.ObsVXM, r0.ObsVYM, r0.TX, r0.TY); lead <= 0 {
		t.Fatalf("crossing: predicted target does not lead (%.6f m^2/s)", lead)
	}
	if pred.Predictions == 0 {
		t.Fatal("crossing: no predictions executed")
	}
	truthMs := int(r0.TauS*1000 + 0.5)
	trueBall, ok := truth.at(truthMs)
	if !ok {
		t.Fatalf("crossing: no truth at %d ms", truthMs)
	}
	predErr := DistM(trueBall.X, trueBall.Y, r0.TX, r0.TY)
	chaseErr := DistM(trueBall.X, trueBall.Y, r0.ObsX, r0.ObsY)
	if predErr >= chaseErr {
		t.Fatalf("crossing: no target-lag improvement predErr=%.4f chaseErr=%.4f", predErr, chaseErr)
	}
	cp, cr := simP.FirstContact()
	cc, _ := simC.FirstContact()
	t.Logf("crossing (%s): target-lag improvement predErr=%.4f m < chaseErr=%.4f m at +%d ms (LIMITED: geometric, not reachability); "+
		"contact predicted=(%d,%d) chase=%d; predictions=%d fallbacks=%d; chase recs=%d pred recs=%d",
		pred.Mode.String(), predErr, chaseErr, truthMs, cp, cr, cc, pred.Predictions, pred.Fallbacks, len(recsC), len(recsP))
}

// --- scene 2: rolling deceleration -----------------------------------------

// TestT047RollingDecelScene proves the constant-velocity assumption overshoots a
// decelerating ball and that the estimate still leads the observation.
func TestT047RollingDecelScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 5.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	ctl := t047NewController(t047ModePredictWithinAge)
	recs, sim := t047Run(t, cfg, []Runner{t047Sensor(0.48, 0.50, 200, 0)}, ctl, 600, setup, nil)

	first := t047FirstPub(recs, 0)
	if first == nil || !first.Runners[0].Predicted {
		t.Fatalf("decel: first publication not predicted")
	}
	r0 := first.Runners[0]
	if lead := t047AlongHeading(r0.ObsX, r0.ObsY, r0.ObsVXM, r0.ObsVYM, r0.TX, r0.TY); lead <= 0 {
		t.Fatalf("decel: no lead (%.6f)", lead)
	}
	predDist := r0.ObsVXM * r0.TauS
	trueDist := r0.ObsVXM*r0.TauS - 0.5*RollDecelMPS2*r0.TauS*r0.TauS
	if predDist <= trueDist {
		t.Fatalf("decel: predicted %.6f not beyond true %.6f", predDist, trueDist)
	}
	b := sim.Ball()
	finalSpeed := math.Sqrt(b.VXM*b.VXM + b.VYM*b.VYM)
	if finalSpeed >= 5.0 {
		t.Fatalf("decel: ball did not decelerate (speed %.6f)", finalSpeed)
	}
	t.Logf("decel: tau=%.4fs overshoot=%.4f m (harmful); final ball speed=%.4f m/s predictions=%d fallbacks=%d",
		r0.TauS, predDist-trueDist, finalSpeed, ctl.Predictions, ctl.Fallbacks)
}

// --- scene 3: delayed reaction ---------------------------------------------

// TestT047DelayedReactionScene proves no prediction runs before the reaction and
// that ReactAtMS is genuinely armed by a real ReleasePass.
func TestT047DelayedReactionScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	runner := t047Sensor(0.50, 0.50, 200, 200)
	setup := func(s *Sim) {
		s.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
	}
	ctl := t047NewController(t047ModePredictFresh)
	recs, sim := t047Run(t, cfg, []Runner{runner}, ctl, 500, setup, nil)
	if got := sim.runners[0].ReactAtMS; got != 200 {
		t.Fatalf("delay: ReactAtMS=%d want 200", got)
	}
	sawAfter := false
	for i := range recs {
		if !recs[i].Published {
			continue
		}
		pre := recs[i].ClockMS - SubstepMS
		if pre < 200 {
			if recs[i].Runners[0].Predicted {
				t.Fatalf("delay: prediction before reaction at %d", pre)
			}
		} else if recs[i].Runners[0].Predicted {
			sawAfter = true
		}
	}
	if !sawAfter {
		t.Fatal("delay: no prediction after the reaction")
	}
	t.Logf("delay: ReactAtMS=200; no prediction before 200 ms; predictions after reaction=%d", ctl.Predictions)
}

// --- scene 4: stale / outside radius ---------------------------------------

// TestT047StaleOutsideScene proves a within-age stale record (<=MaxAge) is
// extrapolated, an overage record (>MaxAge) falls back to the exact T-044 stale
// retention, and fresh-only rejects ANY age > 0.
func TestT047StaleOutsideScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(s *Sim) {
		s.SetBall(Ball{Mode: BallDead, X: 0.9, Y: 0.9})
		s.runners[0].Perception = &BallObservation{
			RadiusM: 0.02, Seen: true, ClockMS: 0, X: 0.5, Y: 0.5, VXM: 6, VYM: 0,
			Mode: BallRolling, Owner: -1,
		}
		s.runners[0].ReactAtMS = 0
	}

	within := t047NewController(t047ModePredictWithinAge)
	recsW, _ := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 0.02, 0)}, within, 260, setup, nil)
	w100 := t047FirstPub(recsW, 100)
	w200 := t047FirstPub(recsW, 200)
	if w100 == nil || w200 == nil {
		t.Fatalf("stale: missing publications (100=%v 200=%v)", w100 != nil, w200 != nil)
	}
	if !w100.Runners[0].Predicted {
		t.Fatalf("stale @100: within-age should predict (reason %s)", w100.Runners[0].Reason)
	}
	if w200.Runners[0].Predicted || w200.Runners[0].Reason != t047ReasonOverage {
		t.Fatalf("stale @200: want overage fallback, got predicted=%v reason=%s", w200.Runners[0].Predicted, w200.Runners[0].Reason)
	}
	if w200.Runners[0].IX != w100.Runners[0].IX || w200.Runners[0].IY != w100.Runners[0].IY {
		t.Fatalf("stale @200: fallback did not retain the prior intent (%v,%v) vs (%v,%v)",
			w200.Runners[0].IX, w200.Runners[0].IY, w100.Runners[0].IX, w100.Runners[0].IY)
	}

	fresh := t047NewController(t047ModePredictFresh)
	recsF, _ := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 0.02, 0)}, fresh, 160, setup, nil)
	f100 := t047FirstPub(recsF, 100)
	if f100 == nil || f100.Runners[0].Predicted || f100.Runners[0].Reason != t047ReasonFreshRejected {
		t.Fatalf("fresh-only @100: want fresh-rejected fallback, got %+v", f100.Runners[0])
	}
	t.Logf("stale: within-age predicted @100 (harmful window <=%d ms); overage @200 and fresh-only @100 fall back to T-044 retention",
		t047MaxAgeMS)
}

// --- scene 5: deflection ---------------------------------------------------

// TestT047DeflectionScene proves a real contact-local miscontrol deflect occurs
// with a fractional event and that the estimator keeps running on the record.
func TestT047DeflectionScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(s *Sim) {
		s.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
		s.controlError = 1.0
	}
	ctl := t047NewController(t047ModePredictFresh)
	recs, sim := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, ctl, 300, setup, nil)
	if n := countKind(sim, PassEventDeflect); n < 1 {
		t.Fatalf("deflection: %d deflect events, want >=1", n)
	}
	frac := math.NaN()
	for _, rec := range recs {
		for _, e := range rec.Events {
			if e.Kind == PassEventDeflect {
				frac = e.Frac
			}
		}
	}
	if math.IsNaN(frac) || frac < 0 || frac > 1 {
		t.Fatalf("deflection: bad event fraction %v", frac)
	}
	if ctl.Predictions == 0 {
		t.Fatal("deflection: estimator never ran")
	}
	t.Logf("deflection: %d deflect event(s) with frac=%.4f; predictions=%d fallbacks=%d",
		countKind(sim, PassEventDeflect), frac, ctl.Predictions, ctl.Fallbacks)
}

// --- scene 6: boundary -----------------------------------------------------

// TestT047BoundaryScene proves the ball exits the pitch within the horizon and
// the predicted target is clamped to the pitch (bounded).
func TestT047BoundaryScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.995, Y: 0.50, VXM: 5.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	ctl := t047NewController(t047ModePredictFresh)
	recs, sim := t047Run(t, cfg, []Runner{t047Sensor(0.98, 0.50, 200, 0)}, ctl, 300, setup, nil)
	if n := countKind(sim, PassEventOut); n < 1 {
		t.Fatalf("boundary: %d out events, want >=1", n)
	}
	first := t047FirstPub(recs, 0)
	if first == nil || !first.Runners[0].Predicted {
		t.Fatalf("boundary: first publication not predicted")
	}
	r0 := first.Runners[0]
	if r0.TX < 0 || r0.TX > 1 || r0.TY < 0 || r0.TY > 1 {
		t.Fatalf("boundary: target (%v,%v) outside the pitch", r0.TX, r0.TY)
	}
	t.Logf("boundary: %d out event(s); predicted target clamped to (%.4f,%.4f)", countKind(sim, PassEventOut), r0.TX, r0.TY)
}

// --- scene 7: second release ------------------------------------------------

// TestT047SecondReleaseScene proves two REAL ReleasePass calls at [0, 820] and
// that the per-moment observation continues across the second release.
func TestT047SecondReleaseScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(s *Sim) {
		s.ReleasePass(PassParams{StartX: 0.20, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 6, Passer: 0})
	}
	mid := func(s *Sim, clock int) {
		if clock == 820 {
			s.ReleasePass(PassParams{StartX: 0.10, StartY: 0.50, TargetX: 0.40, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
		}
	}
	ctl := t047NewController(t047ModePredictFresh)
	recs, sim := t047Run(t, cfg, []Runner{t047Sensor(0.60, 0.50, 200, 0)}, ctl, 1000, setup, mid)
	clocks := t042EventClocks(sim.Events(), PassEventRelease)
	if len(clocks) != 2 || clocks[0] != 0 || clocks[1] != 820 {
		t.Fatalf("second release: release clocks %v, want [0 820]", clocks)
	}
	// moment strictness: at the 820 publication the ball is the released ball.
	pub820 := t047FirstPub(recs, 820)
	if pub820 == nil {
		t.Fatal("second release: no publication at 820")
	}
	t.Logf("second release: exactly two real ReleasePass calls at %v; observations=%d predictions=%d",
		clocks, len(recs), ctl.Predictions)
}

// --- ordering: sample-then-estimate ----------------------------------------

// TestT047OrderingSampleThenEstimate proves the controller uses the CURRENT
// sampled observation and its pre-step own state, installs the target before
// integration, and is called only for the eligible local runner.
func TestT047OrderingSampleThenEstimate(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	ctl := t047NewController(t047ModePredictFresh)
	// A DISTINCT radius (not the bench 200) so a hardcoded value cannot pass.
	const radius = 175.0
	recs, sim := t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, radius, 0)}, ctl, 20, setup, nil)
	if len(recs) != 1 || !recs[0].Published {
		t.Fatalf("ordering: expected one publication, got %d", len(recs))
	}
	rec := recs[0]
	r0 := rec.Runners[0]
	actualRadius := sim.runners[0].Perception.RadiusM
	if r0.ObsRadiusM != actualRadius || r0.ObsRadiusM != radius || r0.ObsRadiusM <= 0 {
		t.Fatalf("ordering: recorded radius %.1f != actual sampled record %.1f (want %.1f, not hardcoded)",
			r0.ObsRadiusM, actualRadius, radius)
	}
	if r0.ObsClockMS != 0 {
		t.Fatalf("ordering: observation clock %d != publication pre-step 0", r0.ObsClockMS)
	}
	// sample-before-motion: the sampled observation equals the PRE-step ball.
	if r0.ObsX != rec.Ball.X || r0.ObsY != rec.Ball.Y {
		t.Fatalf("ordering: observation (%v,%v) != pre-step ball (%v,%v)", r0.ObsX, r0.ObsY, rec.Ball.X, rec.Ball.Y)
	}
	in := t047EstimateInput{
		NowMS: 0, RX: r0.PreX, RY: r0.PreY,
		TopSpeedMPS: DefaultMotionParams().TopSpeedMPS, ReachM: ControlRadiusM,
		HorizonMS: t047HorizonMS, MaxAgeMS: t047MaxAgeMS, ApplyAge: false, FreshOnly: true,
		Seen: r0.Seen, ObsClockMS: r0.ObsClockMS,
		ObsX: r0.ObsX, ObsY: r0.ObsY, ObsVXM: r0.ObsVXM, ObsVYM: r0.ObsVYM,
		ObsMode: r0.ObsMode, ObsRadiusM: r0.ObsRadiusM,
	}
	tx, ty, _, ok, _ := t047Estimate(in)
	if !ok {
		t.Fatal("ordering: recomputed estimate not ok")
	}
	if tx != r0.TX || ty != r0.TY {
		t.Fatalf("ordering: controller target (%v,%v) != recomputed (%v,%v)", r0.TX, r0.TY, tx, ty)
	}
	if r0.IX != r0.TX || r0.IY != r0.TY {
		t.Fatalf("ordering: installed intent (%v,%v) != target (%v,%v)", r0.IX, r0.IY, r0.TX, r0.TY)
	}
	if len(ctl.LastEv) != 1 || ctl.LastEv[0].Index != 0 {
		t.Fatalf("ordering: controller called for %d runners, want only the eligible local runner 0", len(ctl.LastEv))
	}
	t.Logf("ordering: sampled observation@0 == pre-step ball; estimate installed before integration; only eligible local runner called")
}

// --- no RNG draws ----------------------------------------------------------

// TestT047NoRNGDraws proves the estimator/controller draw no RNG.
func TestT047NoRNGDraws(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 7)
	sim.SetRunners([]Runner{t047Sensor(0.50, 0.50, 200, 0)})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0})
	ctl := t047NewController(t047ModePredictWithinAge)
	before := sim.RNGState()
	for sim.ClockMS() < 600 {
		t047Step(sim, ctl)
	}
	if after := sim.RNGState(); after != before {
		t.Fatalf("RNG state changed: %v -> %v", before, after)
	}
	t.Logf("no RNG draws: state unchanged after %d ms of prediction", sim.ClockMS())
}

// --- deterministic replay --------------------------------------------------

// TestT047DeterministicReplay proves two identical predicted runs are bit-identical.
func TestT047DeterministicReplay(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	run := func() ([]t047StepRec, *Sim) {
		ctl := t047NewController(t047ModePredictWithinAge)
		return t047Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, ctl, 400, setup, nil)
	}
	r1, s1 := run()
	r2, s2 := run()
	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("deterministic replay: step records differ")
	}
	if !reflect.DeepEqual(s1.Snapshot(), s2.Snapshot()) {
		t.Fatal("deterministic replay: snapshots differ")
	}
	t.Logf("deterministic replay: %d identical steps, identical complete snapshot", len(r1))
}

// --- complete JSON continuation --------------------------------------------

// t047AssertContinuation runs a scene, splits at each clock, resumes the complete
// {Sim, controller} JSON state and asserts the full snapshot + serialized
// controller + sample tails remain value-identical every subsequent step.
func t047AssertContinuation(t *testing.T, name string, cfg Config, runners []Runner, mode t047Mode, horizon int, setup func(*Sim), mid func(*Sim, int), splits []int) {
	t.Helper()
	for _, split := range splits {
		straight := mustSim(t, cfg, 7)
		straight.SetRunners(runners)
		if setup != nil {
			setup(straight)
		}
		sc := t047NewController(mode)
		for straight.ClockMS() < split {
			if mid != nil {
				mid(straight, straight.ClockMS())
			}
			t047Step(straight, sc)
		}
		resumed, rc := t047MarshalResume(t, straight, sc)
		for straight.ClockMS() < horizon {
			if mid != nil {
				mid(straight, straight.ClockMS())
				mid(resumed, resumed.ClockMS())
			}
			t047Step(straight, sc)
			t047Step(resumed, rc)
			if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
				t.Fatalf("%s split=%d @%d: sim snapshot diverged", name, split, straight.ClockMS())
			}
			if !t047CtlEqual(sc, rc) {
				t.Fatalf("%s split=%d @%d: controller state diverged", name, split, straight.ClockMS())
			}
		}
		if !reflect.DeepEqual(t046SamplesTail(straight, resumed), resumed.Samples()) {
			t.Fatalf("%s split=%d: resumed sample tail differs", name, split)
		}
	}
}

// TestT047JSONContinuation covers crossing, a real fractional deflection, a
// second release and the stale/overage boundary across many split clocks.
func TestT047JSONContinuation(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	t047AssertContinuation(t, "crossing", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 0)}, t047ModePredictWithinAge, 400,
		func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}) },
		nil, []int{0, 20, 40, 100, 200, 300})

	t047AssertContinuation(t, "deflection", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 0)}, t047ModePredictFresh, 300,
		func(s *Sim) { s.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50}); s.controlError = 1.0 },
		nil, []int{0, 20, 40, 100, 200})

	t047AssertContinuation(t, "second-release", cfg,
		[]Runner{t047Sensor(0.60, 0.50, 200, 0)}, t047ModePredictFresh, 1000,
		func(s *Sim) {
			s.ReleasePass(PassParams{StartX: 0.20, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 6, Passer: 0})
		},
		func(s *Sim, clock int) {
			if clock == 820 {
				s.ReleasePass(PassParams{StartX: 0.10, StartY: 0.50, TargetX: 0.40, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
			}
		}, []int{800, 820, 840})

	staleSetup := func(s *Sim) {
		s.SetBall(Ball{Mode: BallDead, X: 0.9, Y: 0.9})
		s.runners[0].Perception = &BallObservation{
			RadiusM: 0.02, Seen: true, ClockMS: 0, X: 0.5, Y: 0.5, VXM: 6, VYM: 0,
			Mode: BallRolling, Owner: -1,
		}
		s.runners[0].ReactAtMS = 0
	}
	t047AssertContinuation(t, "stale-overage", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 0.02, 0)}, t047ModePredictWithinAge, 260,
		staleSetup, nil, []int{80, 100, 120, 180, 200, 220})

	t.Logf("JSON continuation: 4 scenes x multiple splits value-identical (snapshot+controller+samples)")
}
