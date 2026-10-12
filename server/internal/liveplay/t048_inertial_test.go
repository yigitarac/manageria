package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// T-048 inertial reachability and contact-evidence STUDY (test-only, isolated
// package `liveplay`). See vault/07-Session-Logs/2026-10-13-t048-inertial-
// feasibility-evidence.md for the PRESPEC (§1–§3) written BEFORE these changes.
//
// This file adds NO runtime/non-test source. It contains:
//   - a pure, observation+own-body-only INERTIAL FEASIBILITY candidate that rolls
//     a PRIVATE VALUE copy of a runner toward an observed-velocity target with the
//     REAL integrateInertial integrator, under a bounded candidate scan and step
//     budget (explicitly model-relative: fixed target + constant observed ball
//     velocity; NOT a theorem and NOT a contact guarantee);
//   - a test-only controller with THREE modes: the independent T-044 chase, the
//     T-047 optimistic fresh-only top-speed estimate (unchanged horizon), and the
//     new inertial rollout;
//   - a harness that reproduces the T-046 PER-RUNNER local gating, samples the
//     REAL T-046 observation after the reaction gate, then installs the policy
//     target BEFORE the real physics Step integrates it (never post-step).
//
// All numbers are explicit assumptions, not measured truths or outcome knobs.

const (
	// t048HorizonMS is the maximum inertial rollout horizon (ms).
	t048HorizonMS = 1500
	// t048TopSpeedHorizonMS is the T-047 top-speed horizon, kept UNCHANGED so the
	// T-047 estimate is reproduced exactly in the topspeed comparison mode.
	t048TopSpeedHorizonMS = 500
	// t048MaxAgeMS is the maximum observation age that may be used; 0 = fresh-only.
	t048MaxAgeMS = 0
	// t048CandidateStrideMS is the coarse intercept-time candidate grid.
	t048CandidateStrideMS = 100
	// t048MaxSteps is the hard per-decision rollout integrate-call budget. The
	// analytic full-scan worst case for the default horizon/stride is
	// (stride/substep) * N(N+1)/2 = 5 * 120 = 600 calls.
	t048MaxSteps = 900
	// t048MaxHorizonMS bounds the horizon so a finite oversized input cannot make
	// the candidate scan loop huge unchecked; a larger horizon is rejected.
	t048MaxHorizonMS = 6000
	// t048MaxStepLimit bounds the declared step budget so a finite oversized
	// budget cannot integrate huge unchecked.
	t048MaxStepLimit = 50000
	// t048CoordLimit bounds a physically meaningful normalized coordinate.
	t048CoordLimit = 10.0
)

// Inertial candidate outcome reasons (added to the reused T-047 reasons). The
// boundary reason rejects a predicted point OUTSIDE the pitch rather than
// returning a clamped point as if it were reachable.
const (
	t048ReasonReached    = "inertial-reached"
	t048ReasonNotReached = "inertial-not-reached"
	t048ReasonBudget     = "inertial-step-budget"
	t048ReasonBoundary   = "inertial-target-out-of-pitch"
)

// t048InertialInput is the AUDITED estimator input: only the runner's own
// observation, own state/limits (incl. own velocity + motion params), the current
// clock and the controller config.
type t048InertialInput struct {
	NowMS      int
	Seen       bool
	ObsClockMS int
	ObsX       float64
	ObsY       float64
	ObsVXM     float64
	ObsVYM     float64
	ObsMode    BallMode
	ObsRadiusM float64

	RX, RY           float64
	RVXM, RVYM       float64
	TopSpeedMPS      float64
	AccelMPS2        float64
	DecelMPS2        float64
	LateralAccelMPS2 float64
	ReachM           float64

	HorizonMS int
	MaxAgeMS  int
	MaxSteps  int
	FreshOnly bool
}

// t048InertialRollout is the bounded own-body feasibility candidate.
//
// It predicts the ball with the CONSTANT OBSERVED velocity (no rolling
// deceleration is applied) and, for each coarse candidate time tau, rolls a
// PRIVATE VALUE copy of the runner toward that FIXED predicted point using the
// real integrateInertial integrator. It returns the first feasible target
// (clamped to the pitch), the candidate tau in seconds, whether a candidate was
// feasible, the integrate-call cost and a reason.
//
// It is a PURE function of `in`. LIMIT: model-relative, NOT a theorem and NOT a
// guarantee of contact: it assumes a fixed target and constant observed ball
// velocity, and the runner's real motion is only approximated by the rollout.
func t048InertialRollout(in t048InertialInput) (tx, ty, tauS float64, reached bool, steps int, reason string) {
	if !in.Seen {
		return 0, 0, 0, false, 0, t047ReasonUnknown
	}
	if in.ObsMode != BallRolling && in.ObsMode != BallLoose {
		return 0, 0, 0, false, 0, t047ReasonNonFree
	}
	if !isFinite(in.ObsRadiusM) || in.ObsRadiusM <= 0 {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	if !isFinite(in.ObsX) || !isFinite(in.ObsY) || !isFinite(in.ObsVXM) || !isFinite(in.ObsVYM) ||
		!isFinite(in.RX) || !isFinite(in.RY) || !isFinite(in.RVXM) || !isFinite(in.RVYM) ||
		!isFinite(in.TopSpeedMPS) || !isFinite(in.AccelMPS2) || !isFinite(in.DecelMPS2) ||
		!isFinite(in.LateralAccelMPS2) || !isFinite(in.ReachM) {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	if math.Abs(in.ObsX) > t048CoordLimit || math.Abs(in.ObsY) > t048CoordLimit ||
		math.Abs(in.RX) > t048CoordLimit || math.Abs(in.RY) > t048CoordLimit {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	if in.TopSpeedMPS <= 0 || in.AccelMPS2 <= 0 || in.DecelMPS2 <= 0 || in.LateralAccelMPS2 <= 0 ||
		in.ReachM < 0 || in.MaxAgeMS < 0 {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	// The rollout integrates whole SubstepMS steps and searches whole
	// CandidateStrideMS candidates, so a horizon that is not an exact multiple of
	// the candidate stride would report a tau it did NOT integrate. Reject such a
	// mismatch rather than claim an exact forecast. Oversized finite horizon/budget
	// are also rejected so the scan cannot loop/integrate huge unchecked.
	if in.HorizonMS <= 0 || in.HorizonMS%t048CandidateStrideMS != 0 || in.HorizonMS > t048MaxHorizonMS {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	if in.MaxSteps <= 0 || in.MaxSteps > t048MaxStepLimit {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	age := in.NowMS - in.ObsClockMS
	if age < 0 {
		return 0, 0, 0, false, 0, t047ReasonInvalid
	}
	if in.FreshOnly && age > 0 {
		return 0, 0, 0, false, 0, t047ReasonFreshRejected
	}
	if age > in.MaxAgeMS {
		return 0, 0, 0, false, 0, t047ReasonOverage
	}

	dt := float64(SubstepMS) / 1000.0
	nCand := in.HorizonMS / t048CandidateStrideMS
	if nCand < 1 {
		nCand = 1
	}
	used := 0
	lastX, lastY := clamp01(in.ObsX), clamp01(in.ObsY)
	for j := 1; j <= nCand; j++ {
		tauMS := j * t048CandidateStrideMS
		if tauMS > in.HorizonMS {
			tauMS = in.HorizonMS
		}
		tau := float64(tauMS) / 1000.0
		bx := in.ObsX + (in.ObsVXM/PitchLengthM)*tau
		by := in.ObsY + (in.ObsVYM/PitchWidthM)*tau
		lastX, lastY = clamp01(bx), clamp01(by)
		if !isFinite(bx) || !isFinite(by) {
			return 0, 0, 0, false, used, t047ReasonNoRoot
		}
		// A predicted point OUTSIDE the pitch cannot be a legitimate target: a
		// clamped point returned as if reachable would be false. Reject it as a
		// boundary failure (the search cannot continue outside either).
		if bx < 0 || bx > 1 || by < 0 || by > 1 {
			return clamp01(bx), clamp01(by), 0, false, used, t048ReasonBoundary
		}
		nSteps := tauMS / SubstepMS
		if nSteps < 1 {
			nSteps = 1
		}
		if used+nSteps > in.MaxSteps {
			return lastX, lastY, 0, false, used, t048ReasonBudget
		}
		// A PRIVATE VALUE copy of a runner: the real integrator, no alias to any
		// live Perception record or sim state, no truth ball/decay/future contact.
		r := Runner{Motion: MotionInertial, X: in.RX, Y: in.RY, VXM: in.RVXM, VYM: in.RVYM}
		r.SetMotionParams(MotionParams{
			TopSpeedMPS:      in.TopSpeedMPS,
			AccelMPS2:        in.AccelMPS2,
			DecelMPS2:        in.DecelMPS2,
			LateralAccelMPS2: in.LateralAccelMPS2,
		})
		for k := 0; k < nSteps; k++ {
			r.IntentX, r.IntentY = bx, by
			r.integrateInertial(dt)
			used++
		}
		if !isFinite(r.X) || !isFinite(r.Y) {
			return 0, 0, 0, false, used, t047ReasonInvalid
		}
		if DistM(r.X, r.Y, bx, by) <= in.ReachM {
			return bx, by, tau, true, used, t048ReasonReached
		}
	}
	return lastX, lastY, 0, false, used, t048ReasonNotReached
}

// t048Mode selects the controller policy.
type t048Mode uint8

const (
	// t048ModeChase is the independent re-implementation of the T-044 chase.
	t048ModeChase t048Mode = iota
	// t048ModeTopSpeed is the T-047 optimistic fresh-only top-speed estimate.
	t048ModeTopSpeed
	// t048ModeInertial is the new own-body rollout feasibility candidate.
	t048ModeInertial
)

// String returns a stable label for evidence.
func (m t048Mode) String() string {
	switch m {
	case t048ModeChase:
		return "chase"
	case t048ModeTopSpeed:
		return "topspeed"
	case t048ModeInertial:
		return "inertial"
	default:
		return "unknown"
	}
}

// t048RunnerEv is the per-runner decision evidence of one publication.
type t048RunnerEv struct {
	Index          int
	Eligible       bool
	Predicted      bool
	Reason         string
	TauS           float64
	TX, TY         float64
	RX, RY         float64
	Seen           bool
	ObsClockMS     int
	ObsX, ObsY     float64
	ObsVXM, ObsVYM float64
	ObsMode        BallMode
	RolloutSteps   int
}

// t048Controller is the test-only three-mode controller. Its serialized state is
// the config + counters + last decision clock; `LastEv` is transient evidence
// (never serialized) and it draws NO RNG.
type t048Controller struct {
	Mode              t048Mode `json:"mode"`
	HorizonMS         int      `json:"horizonMs"`
	TopSpeedHorizonMS int      `json:"topSpeedHorizonMs"`
	MaxAgeMS          int      `json:"maxAgeMs"`
	MaxSteps          int      `json:"maxSteps"`
	LastDecisionMS    int      `json:"lastDecisionMs"`
	Predictions       int      `json:"predictions"`
	Fallbacks         int      `json:"fallbacks"`
	RolloutRuns       int      `json:"rolloutRuns"`
	RolloutSteps      int      `json:"rolloutSteps"`
	RolloutMaxSteps   int      `json:"rolloutMaxSteps"`

	LastEv []t048RunnerEv `json:"-"`
}

// t048NewController returns a controller with the prespecified assumptions.
func t048NewController(mode t048Mode) *t048Controller {
	return &t048Controller{
		Mode:              mode,
		HorizonMS:         t048HorizonMS,
		TopSpeedHorizonMS: t048TopSpeedHorizonMS,
		MaxAgeMS:          t048MaxAgeMS,
		MaxSteps:          t048MaxSteps,
		LastDecisionMS:    -1,
	}
}

// inputFor builds the audited inertial-rollout input from a runner and its OWN
// record.
func (c *t048Controller) inputFor(r *Runner, o *BallObservation, clock int) t048InertialInput {
	p := r.Params()
	return t048InertialInput{
		NowMS: clock, RX: r.X, RY: r.Y, RVXM: r.VXM, RVYM: r.VYM,
		TopSpeedMPS: p.TopSpeedMPS, AccelMPS2: p.AccelMPS2,
		DecelMPS2: p.DecelMPS2, LateralAccelMPS2: p.LateralAccelMPS2,
		ReachM: t047Reach(r), HorizonMS: c.HorizonMS, MaxAgeMS: c.MaxAgeMS,
		MaxSteps: c.MaxSteps, FreshOnly: true,
		Seen: o.Seen, ObsClockMS: o.ClockMS, ObsX: o.X, ObsY: o.Y,
		ObsVXM: o.VXM, ObsVYM: o.VYM, ObsMode: o.Mode, ObsRadiusM: o.RadiusM,
	}
}

// topSpeedInput builds the T-047 audited input (unchanged horizon/age).
func (c *t048Controller) topSpeedInput(r *Runner, o *BallObservation, clock int) t047EstimateInput {
	return t047EstimateInput{
		NowMS: clock, RX: r.X, RY: r.Y,
		TopSpeedMPS: r.Params().TopSpeedMPS, ReachM: t047Reach(r),
		HorizonMS: c.TopSpeedHorizonMS, MaxAgeMS: c.MaxAgeMS, FreshOnly: true,
		Seen: o.Seen, ObsClockMS: o.ClockMS, ObsX: o.X, ObsY: o.Y,
		ObsVXM: o.VXM, ObsVYM: o.VYM, ObsMode: o.Mode, ObsRadiusM: o.RadiusM,
	}
}

// policy computes the destination and the outcome for one eligible runner.
func (c *t048Controller) policy(r *Runner, o *BallObservation, clock int) (tx, ty float64, predicted bool, reason string, tau float64, steps int) {
	switch c.Mode {
	case t048ModeChase:
		x, y := t047ChaseTarget(r, o, clock)
		return x, y, false, t047ReasonChase, 0, 0
	case t048ModeTopSpeed:
		ptx, pty, ptau, ok, preason := t047Estimate(c.topSpeedInput(r, o, clock))
		if ok {
			return ptx, pty, true, preason, ptau, 0
		}
		x, y := t047FallbackTarget(r, o, clock, preason)
		return x, y, false, preason, ptau, 0
	default: // t048ModeInertial
		itx, ity, itau, reached, used, ireason := t048InertialRollout(c.inputFor(r, o, clock))
		c.RolloutRuns++
		c.RolloutSteps += used
		if used > c.RolloutMaxSteps {
			c.RolloutMaxSteps = used
		}
		if reached {
			return itx, ity, true, ireason, itau, used
		}
		x, y := t047FallbackTarget(r, o, clock, ireason)
		return x, y, false, ireason, itau, used
	}
}

// applyRunner runs the controller for ONE eligible local runner and records the
// evidence. It fails closed on a missing/invalid sensing record, a non-inertial
// runner or a pre-reaction clock.
func (c *t048Controller) applyRunner(sim *Sim, i, clock int) t048RunnerEv {
	r := &sim.runners[i]
	o := r.Perception
	ev := t048RunnerEv{Index: i, RX: r.X, RY: r.Y}
	if o != nil {
		ev.Seen, ev.ObsClockMS = o.Seen, o.ClockMS
		ev.ObsX, ev.ObsY, ev.ObsVXM, ev.ObsVYM = o.X, o.Y, o.VXM, o.VYM
		ev.ObsMode = o.Mode
	}
	if !o.SensingEnabled() || r.Motion != MotionInertial || clock < r.ReactAtMS {
		return ev
	}
	ev.Eligible = true
	tx, ty, predicted, reason, tau, steps := c.policy(r, o, clock)
	r.IntentX, r.IntentY = tx, ty
	ev.Predicted, ev.Reason, ev.TauS, ev.TX, ev.TY, ev.RolloutSteps = predicted, reason, tau, tx, ty, steps
	c.LastDecisionMS = clock
	if predicted {
		c.Predictions++
	} else {
		c.Fallbacks++
	}
	return ev
}

// applySim runs the controller for the gated LocalWakeup runners only.
func (c *t048Controller) applySim(sim *Sim, gate []bool, clock int) {
	c.LastEv = c.LastEv[:0]
	for i := range sim.runners {
		if gate != nil && !gate[i] {
			continue
		}
		if !sim.runners[i].LocalWakeup {
			continue // the controller drives ONLY opted-in local runners
		}
		c.LastEv = append(c.LastEv, c.applyRunner(sim, i, clock))
	}
}

// t048Step reproduces the T-046 PER-RUNNER local gating INDEPENDENTLY (identical
// gating to the accepted t047Step), samples the REAL T-046 observation after the
// reaction gate, runs the shared T-044 local layer for unflagged runners, and
// installs the controller target for the eligible LocalWakeup runners BEFORE the
// real physics Step integrates it. Returns whether a publication occurred.
func t048Step(sim *Sim, ctl *t048Controller) bool {
	clock := sim.clock
	latticeDue := clock >= sim.nextDecisionMS
	if latticeDue {
		sim.nextDecisionMS = (clock/sim.cfg.IntentCadenceMS + 1) * sim.cfg.IntentCadenceMS
	}
	sharedDue := latticeDue || sim.decisionDirty
	if !sharedDue && !sim.anyLocalPending() {
		sim.decisionDirty = false
		sim.Step()
		return false
	}
	if sharedDue {
		sim.publishPursuitIntents() // native oracle (shared)
	}
	n := len(sim.runners)
	if cap(sim.localGate) < n {
		sim.localGate = make([]bool, n)
	}
	gate := sim.localGate[:n]
	for i := range sim.runners {
		r := &sim.runners[i]
		if !r.LocalWakeup {
			gate[i] = sharedDue // exact T-042 shared semantics
			r.WakePending = false
			continue
		}
		if !r.Perception.SensingEnabled() {
			gate[i] = false // fail closed; drop an unhonourable pending
			r.WakePending = false
			continue
		}
		run := latticeDue || r.WakePending
		gate[i] = run
		if run {
			r.WakePending = false
		}
	}
	sim.localGate = gate
	sim.samplePerceptionGate(clock, gate)           // REAL sample, reaction-gated
	sim.publishLocalPursuitIntentsGate(clock, gate) // shared T-044 local layer
	if ctl != nil {
		ctl.applySim(sim, gate, clock) // T-048 policy for eligible LocalWakeup runners
	}
	sim.decisionDirty = false
	sim.Step()
	return true
}

// t048Run steps one scene and records the full per-step evidence (same record
// type as the accepted T-047 harness, so paired metrics are directly comparable).
func t048Run(t *testing.T, cfg Config, runners []Runner, ctl *t048Controller, horizon int, setup func(*Sim), mid func(*Sim, int)) ([]t047StepRec, *Sim) {
	t.Helper()
	sim := mustSim(t, cfg, 7)
	sim.SetRunners(runners)
	if setup != nil {
		setup(sim)
	}
	recs := make([]t047StepRec, 0, horizon/SubstepMS)
	for sim.ClockMS() < horizon {
		if mid != nil {
			mid(sim, sim.ClockMS())
		}
		preBall := sim.ball
		owner := -1
		if preBall.Mode == BallHeld {
			owner = sim.contactRunner
		}
		evStart := len(sim.events)
		pub := t048Step(sim, ctl)
		rec := t047StepRec{ClockMS: sim.ClockMS(), Published: pub, Ball: preBall, Owner: owner}
		rec.Runners = make([]t047RunnerObs, len(sim.runners))
		for i := range sim.runners {
			r := sim.runners[i]
			ro := t047RunnerObs{X: r.X, Y: r.Y, VXM: r.VXM, VYM: r.VYM, IX: r.IntentX, IY: r.IntentY}
			if r.Perception != nil {
				o := *r.Perception
				ro.Seen, ro.ObsClockMS = o.Seen, o.ClockMS
				ro.ObsX, ro.ObsY, ro.ObsVXM, ro.ObsVYM = o.X, o.Y, o.VXM, o.VYM
				ro.ObsMode, ro.ObsRadiusM = o.Mode, o.RadiusM
			}
			rec.Runners[i] = ro
		}
		if pub {
			for _, e := range ctl.LastEv {
				if e.Index >= 0 && e.Index < len(rec.Runners) {
					rec.Runners[e.Index].Eligible = e.Eligible
					rec.Runners[e.Index].Predicted = e.Predicted
					rec.Runners[e.Index].Reason = e.Reason
					rec.Runners[e.Index].TauS = e.TauS
					rec.Runners[e.Index].TX, rec.Runners[e.Index].TY = e.TX, e.TY
					rec.Runners[e.Index].PreX, rec.Runners[e.Index].PreY = e.RX, e.RY
				}
			}
		}
		rec.Events = append(rec.Events, sim.events[evStart:]...)
		recs = append(recs, rec)
	}
	return recs, sim
}

// t048Persisted is the complete JSON continuation unit for the T-048 controller.
type t048Persisted struct {
	Sim Snapshot       `json:"sim"`
	Ctl t048Controller `json:"controller"`
}

// t048MarshalResume marshals {Sim snapshot, controller} and resumes both.
func t048MarshalResume(t *testing.T, sim *Sim, ctl *t048Controller) (*Sim, *t048Controller) {
	t.Helper()
	blob, err := json.Marshal(t048Persisted{Sim: sim.Snapshot(), Ctl: *ctl})
	if err != nil {
		t.Fatalf("marshal t048 state: %v", err)
	}
	var p t048Persisted
	if err := json.Unmarshal(blob, &p); err != nil {
		t.Fatalf("unmarshal t048 state: %v", err)
	}
	rs, err := Resume(p.Sim)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	c := p.Ctl
	return rs, &c
}

// t048CtlEqual compares the SERIALIZED controller state (ignores transient LastEv).
func t048CtlEqual(a, b *t048Controller) bool {
	aa, bb := *a, *b
	aa.LastEv, bb.LastEv = nil, nil
	return reflect.DeepEqual(aa, bb)
}

// t048PreBallAt returns the recorded PRE-step ball at pre-clock `pre`.
func t048PreBallAt(recs []t047StepRec, pre int) (Ball, bool) {
	for i := range recs {
		if recs[i].ClockMS-SubstepMS == pre {
			return recs[i].Ball, true
		}
	}
	return Ball{}, false
}

// t048AssertObservationsTruthful proves every Seen record is a REAL earlier
// pre-step sighting (no future clock, no fabricated position/mode).
func t048AssertObservationsTruthful(t *testing.T, recs []t047StepRec) {
	t.Helper()
	for _, rec := range recs {
		pre := rec.ClockMS - SubstepMS
		for i, ro := range rec.Runners {
			if !ro.Seen {
				continue
			}
			if ro.ObsClockMS > pre {
				t.Fatalf("future observation: clock %d > pre-step %d (runner %d)", ro.ObsClockMS, pre, i)
			}
			b, ok := t048PreBallAt(recs, ro.ObsClockMS)
			if !ok {
				t.Fatalf("no recorded pre-step ball at observed clock %d", ro.ObsClockMS)
			}
			if b.X != ro.ObsX || b.Y != ro.ObsY || b.Mode != ro.ObsMode {
				t.Fatalf("fabricated observation (runner %d @%d): record (%.9f,%.9f,%s) != real pre-step ball (%.9f,%.9f,%s)",
					i, ro.ObsClockMS, ro.ObsX, ro.ObsY, ro.ObsMode, b.X, b.Y, b.Mode)
			}
		}
	}
}

// --- exact first step: optimistic tau vs the own-body rollout ---------------

// TestT048CrossingTauVsInertialRollout reproduces the T-047 crossing optimistic
// lower-bound tau and compares it with the actual OWN inertial rollout: the
// optimistic estimate must not exceed the inertial reach time (or the rollout
// concludes not-reached).
func TestT048CrossingTauVsInertialRollout(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}
	setup := func(s *Sim) { s.SetBall(ball) }

	topCtl := t048NewController(t048ModeTopSpeed)
	recs, _ := t048Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, topCtl, 20, setup, nil)
	pub := t047FirstPub(recs, 0)
	if pub == nil {
		t.Fatal("crossing: no publication at clock 0")
	}
	r0 := pub.Runners[0]
	if !r0.Eligible || !r0.Seen {
		t.Fatalf("crossing: not eligible/seen (%+v)", r0)
	}
	optIn := t047EstimateInput{
		NowMS: 0, RX: r0.PreX, RY: r0.PreY,
		TopSpeedMPS: DefaultMotionParams().TopSpeedMPS, ReachM: ControlRadiusM,
		HorizonMS: t048TopSpeedHorizonMS, MaxAgeMS: t048MaxAgeMS, FreshOnly: true,
		Seen: r0.Seen, ObsClockMS: r0.ObsClockMS, ObsX: r0.ObsX, ObsY: r0.ObsY,
		ObsVXM: r0.ObsVXM, ObsVYM: r0.ObsVYM, ObsMode: r0.ObsMode, ObsRadiusM: r0.ObsRadiusM,
	}
	_, _, tauOpt, okOpt, optReason := t047Estimate(optIn)
	if !okOpt {
		t.Fatalf("crossing: optimistic estimate not ok (%s)", optReason)
	}
	if r0.TauS != tauOpt {
		t.Fatalf("crossing: recorded topspeed tau %.6f != recomputed %.6f", r0.TauS, tauOpt)
	}

	p := DefaultMotionParams()
	inrIn := t048InertialInput{
		NowMS: 0, RX: r0.PreX, RY: r0.PreY, RVXM: 0, RVYM: 0,
		TopSpeedMPS: p.TopSpeedMPS, AccelMPS2: p.AccelMPS2, DecelMPS2: p.DecelMPS2,
		LateralAccelMPS2: p.LateralAccelMPS2, ReachM: ControlRadiusM,
		HorizonMS: t048HorizonMS, MaxAgeMS: t048MaxAgeMS, MaxSteps: t048MaxSteps, FreshOnly: true,
		Seen: r0.Seen, ObsClockMS: r0.ObsClockMS, ObsX: r0.ObsX, ObsY: r0.ObsY,
		ObsVXM: r0.ObsVXM, ObsVYM: r0.ObsVYM, ObsMode: r0.ObsMode, ObsRadiusM: r0.ObsRadiusM,
	}
	_, _, tauInr, reached, steps, inrReason := t048InertialRollout(inrIn)
	if steps <= 0 || steps > t048MaxSteps {
		t.Fatalf("crossing: rollout steps %d outside (0,%d]", steps, t048MaxSteps)
	}
	if reached && tauInr < tauOpt-1e-9 {
		t.Fatalf("crossing: inertial tau %.4f s beat the optimistic lower bound %.4f s", tauInr, tauOpt)
	}
	t.Logf("crossing first step: optimistic tau=%.4f s (LIMITED: instantaneous top speed); "+
		"inertial rollout reached=%v tau=%.4f s reason=%s steps=%d (model-relative, not a contact guarantee)",
		tauOpt, reached, tauInr, inrReason, steps)
}

// --- estimator guards & pure-input audit ------------------------------------

// TestT048InertialRolloutFiniteGuards proves the rollout fails closed on every
// unknown / non-free / invalid / unphysical / overage / fresh-only / budget input
// and is deterministic with finite results for extreme-but-finite inputs.
func TestT048InertialRolloutFiniteGuards(t *testing.T) {
	base := t048InertialInput{
		NowMS: 100, Seen: true, ObsClockMS: 100,
		ObsX: 0.53, ObsY: 0.5, ObsVXM: 0, ObsVYM: 7, ObsMode: BallRolling, ObsRadiusM: 200,
		RX: 0.5, RY: 0.5, TopSpeedMPS: 8.5, AccelMPS2: 4, DecelMPS2: 4.5, LateralAccelMPS2: 3,
		ReachM: 1, HorizonMS: t048HorizonMS, MaxAgeMS: t048MaxAgeMS, MaxSteps: t048MaxSteps, FreshOnly: true,
	}
	cases := []struct {
		name       string
		mutate     func(*t048InertialInput)
		wantReach  bool
		wantReason string
	}{
		{"ok", func(*t048InertialInput) {}, false, ""},
		{"unknown", func(in *t048InertialInput) { in.Seen = false }, false, t047ReasonUnknown},
		{"nonfree", func(in *t048InertialInput) { in.ObsMode = BallHeld }, false, t047ReasonNonFree},
		{"nan-obsx", func(in *t048InertialInput) { in.ObsX = math.NaN() }, false, t047ReasonInvalid},
		{"inf-rx", func(in *t048InertialInput) { in.RX = math.Inf(1) }, false, t047ReasonInvalid},
		{"nan-radius", func(in *t048InertialInput) { in.ObsRadiusM = math.NaN() }, false, t047ReasonInvalid},
		{"coord-limit", func(in *t048InertialInput) { in.ObsX = 100 }, false, t047ReasonInvalid},
		{"overage", func(in *t048InertialInput) { in.NowMS = 101 }, false, t047ReasonFreshRejected},
		{"fresh-reject", func(in *t048InertialInput) { in.ObsClockMS = 99 }, false, t047ReasonFreshRejected},
		{"future-clock", func(in *t048InertialInput) { in.ObsClockMS = 200 }, false, t047ReasonInvalid},
		{"zero-top", func(in *t048InertialInput) { in.TopSpeedMPS = 0 }, false, t047ReasonInvalid},
		{"neg-accel", func(in *t048InertialInput) { in.AccelMPS2 = -1 }, false, t047ReasonInvalid},
		{"zero-horizon", func(in *t048InertialInput) { in.HorizonMS = 0 }, false, t047ReasonInvalid},
		{"zero-budget", func(in *t048InertialInput) { in.MaxSteps = 0 }, false, t047ReasonInvalid},
		{"boundary", func(in *t048InertialInput) { in.ObsX, in.ObsVXM, in.ObsVYM = 0.99, 40, 0 }, false, t048ReasonBoundary},
	}
	for _, c := range cases {
		in := base
		c.mutate(&in)
		_, _, _, reached, _, reason := t048InertialRollout(in)
		if reached != c.wantReach {
			t.Fatalf("%s: reached=%v want %v (reason %q)", c.name, reached, c.wantReach, reason)
		}
		if c.wantReason != "" && reason != c.wantReason {
			t.Fatalf("%s: reason=%q want %q", c.name, reason, c.wantReason)
		}
	}
	// Extreme-but-finite velocity: the predicted point leaves the pitch, so the
	// result is a bounded boundary failure, never NaN/Inf and never a panic.
	big := base
	big.ObsVXM, big.ObsVYM = 1e200, 0
	if tx, ty, tau, reached, steps, reason := t048InertialRollout(big); reached || reason != t048ReasonBoundary ||
		!isFinite(tx) || !isFinite(ty) || !isFinite(tau) || steps < 0 {
		t.Fatalf("huge velocity: reached=%v reason=%q finite=(%v,%v,%v)", reached, reason, isFinite(tx), isFinite(ty), isFinite(tau))
	}
	// Non-grid horizon (not an exact multiple of the candidate stride) and
	// oversized finite horizon/budget are REJECTED, not silently truncated; the
	// returned tau can never be a time the rollout did not integrate.
	for _, bad := range []int{137, 140, t048MaxHorizonMS + t048CandidateStrideMS} {
		in := base
		in.HorizonMS = bad
		if _, _, _, reached, steps, reason := t048InertialRollout(in); reached || steps != 0 || reason != t047ReasonInvalid {
			t.Fatalf("non-grid/oversized horizon %d: reached=%v steps=%d reason=%q", bad, reached, steps, reason)
		}
	}
	over := base
	over.MaxSteps = t048MaxStepLimit + 1
	if _, _, _, reached, _, reason := t048InertialRollout(over); reached || reason != t047ReasonInvalid {
		t.Fatalf("oversized budget: reached=%v reason=%q", reached, reason)
	}
	if t048CandidateStrideMS%SubstepMS != 0 || t048HorizonMS%t048CandidateStrideMS != 0 {
		t.Fatalf("rollout grid not aligned: stride=%d substep=%d horizon=%d", t048CandidateStrideMS, SubstepMS, t048HorizonMS)
	}
	// Step-budget exhaustion (thin budget) is reported, not exceeded.
	tiny := base
	tiny.MaxSteps = 5
	if _, _, _, reached, steps, reason := t048InertialRollout(tiny); reached || reason != t048ReasonBudget || steps > 5 {
		t.Fatalf("budget: reached=%v reason=%q steps=%d", reached, reason, steps)
	}
	// Any returned tau is an exact grid time (no fractional time mismatch).
	if _, _, tau, reached, _, _ := t048InertialRollout(base); reached {
		if ms := tau * 1000; math.Abs(ms-math.Round(ms)) > 1e-9 || int(math.Round(ms))%SubstepMS != 0 {
			t.Fatalf("tau %.9f s is not a whole-substep grid time", tau)
		}
	}
	// Determinism.
	a1, b1, t1, o1, s1, r1 := t048InertialRollout(base)
	a2, b2, t2, o2, s2, r2 := t048InertialRollout(base)
	if a1 != a2 || b1 != b2 || t1 != t2 || o1 != o2 || s1 != s2 || r1 != r2 {
		t.Fatal("rollout is not deterministic")
	}
	t.Logf("finite guards: %d cases + huge-velocity + budget + determinism; all classified", len(cases))
}

// TestT048EstimatorInputAudit proves the input contains ONLY the runner's own
// observation/state (no ball, no owner, no true decel) and that the rollout is a
// pure function of that input, with no mutation/alias of the source runner.
func TestT048EstimatorInputAudit(t *testing.T) {
	r := t047Sensor(0.5, 0.5, 200, 0)
	o := &BallObservation{RadiusM: 200, Seen: true, ClockMS: 0, X: 0.53, Y: 0.5, VXM: 0, VYM: 7, Mode: BallRolling, Owner: -1}
	c := t048NewController(t048ModeInertial)
	got := c.inputFor(&r, o, 0)
	p := DefaultMotionParams()
	want := t048InertialInput{
		NowMS: 0, RX: 0.5, RY: 0.5, RVXM: 0, RVYM: 0,
		TopSpeedMPS: p.TopSpeedMPS, AccelMPS2: p.AccelMPS2, DecelMPS2: p.DecelMPS2,
		LateralAccelMPS2: p.LateralAccelMPS2, ReachM: ControlRadiusM,
		HorizonMS: t048HorizonMS, MaxAgeMS: t048MaxAgeMS, MaxSteps: t048MaxSteps, FreshOnly: true,
		Seen: true, ObsClockMS: 0, ObsX: 0.53, ObsY: 0.5, ObsVXM: 0, ObsVYM: 7,
		ObsMode: BallRolling, ObsRadiusM: 200,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inertial input:\n got %+v\nwant %+v", got, want)
	}
	// Purity / no hidden source: a sim with a wildly different ball must not
	// change a rollout recomputed from the SAME audited input.
	sim := mustSim(t, ScheduledLivePassConfig(100), 7)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.05, Y: 0.95, VXM: -40, VYM: 30})
	a1, b1, t1, o1, s1, r1 := t048InertialRollout(got)
	a2, b2, t2, o2, s2, r2 := t048InertialRollout(got)
	if a1 != a2 || b1 != b2 || t1 != t2 || o1 != o2 || s1 != s2 || r1 != r2 {
		t.Fatal("rollout changed without an input change")
	}
	// No mutation/alias: mutating the runner AFTER building the input must not
	// change the input or its rollout result (the input is a value copy).
	ba, bb, bt, bo, bs, br := t048InertialRollout(got)
	r.X, r.VXM = 99, 99
	aa, ab, at, ao, as, ar := t048InertialRollout(got)
	if aa != ba || ab != bb || at != bt || ao != bo || as != bs || ar != br {
		t.Fatal("rollout result depended on the source runner after input was built")
	}
	t.Logf("input audit: struct == manual build; rollout pure (no ball/owner/decel); no source alias; ok=%v", o1)
}

// --- ordering: sample-then-estimate ----------------------------------------

// TestT048OrderingSampleThenEstimate proves the inertial controller uses the
// CURRENT sampled observation and its pre-step own state, installs the target
// before integration, and is called only for the eligible local runner.
func TestT048OrderingSampleThenEstimate(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.58, Y: 0.50, VXM: -4.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	ctl := t048NewController(t048ModeInertial)
	const radius = 175.0 // distinct from the bench 200 so a hardcode cannot pass
	recs, sim := t048Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, radius, 0)}, ctl, 20, setup, nil)
	if len(recs) != 1 || !recs[0].Published {
		t.Fatalf("ordering: expected one publication, got %d", len(recs))
	}
	rec := recs[0]
	r0 := rec.Runners[0]
	actualRadius := sim.runners[0].Perception.RadiusM
	if r0.ObsRadiusM != actualRadius || r0.ObsRadiusM != radius || r0.ObsRadiusM <= 0 {
		t.Fatalf("ordering: recorded radius %.1f != actual %.1f (want %.1f)", r0.ObsRadiusM, actualRadius, radius)
	}
	if r0.ObsClockMS != 0 {
		t.Fatalf("ordering: observation clock %d != publication pre-step 0", r0.ObsClockMS)
	}
	if r0.ObsX != rec.Ball.X || r0.ObsY != rec.Ball.Y {
		t.Fatalf("ordering: observation (%v,%v) != pre-step ball (%v,%v)", r0.ObsX, r0.ObsY, rec.Ball.X, rec.Ball.Y)
	}
	in := ctl.inputFor(&sim.runners[0], sim.runners[0].Perception, 0)
	in.RX, in.RY, in.RVXM, in.RVYM = r0.PreX, r0.PreY, 0, 0
	in.Seen, in.ObsClockMS = r0.Seen, r0.ObsClockMS
	in.ObsX, in.ObsY, in.ObsVXM, in.ObsVYM = r0.ObsX, r0.ObsY, r0.ObsVXM, r0.ObsVYM
	in.ObsMode, in.ObsRadiusM = r0.ObsMode, r0.ObsRadiusM
	tx, ty, _, ok, steps, _ := t048InertialRollout(in)
	if !ok {
		t.Fatal("ordering: recomputed rollout not feasible")
	}
	if steps <= 0 {
		t.Fatal("ordering: rollout did not execute")
	}
	if tx != r0.TX || ty != r0.TY {
		t.Fatalf("ordering: controller target (%v,%v) != recomputed (%v,%v)", r0.TX, r0.TY, tx, ty)
	}
	if r0.IX != r0.TX || r0.IY != r0.TY {
		t.Fatalf("ordering: installed intent (%v,%v) != target (%v,%v)", r0.IX, r0.IY, r0.TX, r0.TY)
	}
	if len(ctl.LastEv) != 1 || ctl.LastEv[0].Index != 0 {
		t.Fatalf("ordering: controller called for %d runners, want only eligible local runner 0", len(ctl.LastEv))
	}
	t.Logf("ordering: sampled observation@0 == pre-step ball; inertial target installed before integration; only eligible runner called; steps=%d", steps)
}

// --- no RNG draws -----------------------------------------------------------

// TestT048NoRNGDraws proves the estimator/controller draw no RNG.
func TestT048NoRNGDraws(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 7)
	sim.SetRunners([]Runner{t047Sensor(0.50, 0.50, 200, 0)})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.58, Y: 0.50, VXM: -4.0})
	ctl := t048NewController(t048ModeInertial)
	before := sim.RNGState()
	for sim.ClockMS() < 600 {
		t048Step(sim, ctl)
	}
	if after := sim.RNGState(); after != before {
		t.Fatalf("RNG state changed: %v -> %v", before, after)
	}
	t.Logf("no RNG draws: state unchanged after %d ms of inertial rollout", sim.ClockMS())
}

// --- deterministic replay ---------------------------------------------------

// TestT048DeterministicReplay proves two identical inertial runs are bit-identical.
func TestT048DeterministicReplay(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	ball := Ball{Mode: BallRolling, X: 0.58, Y: 0.50, VXM: -4.0}
	setup := func(s *Sim) { s.SetBall(ball) }
	run := func() ([]t047StepRec, *Sim) {
		return t048Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, 200, 0)}, t048NewController(t048ModeInertial), 400, setup, nil)
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

// --- complete JSON continuation ---------------------------------------------

// t048AssertContinuation splits at each clock, resumes the complete {Sim,
// controller} JSON state and asserts full snapshot + serialized controller +
// sample tails every subsequent step.
func t048AssertContinuation(t *testing.T, name string, cfg Config, runners []Runner, mode t048Mode, horizon int, setup func(*Sim), mid func(*Sim, int), splits []int) {
	t.Helper()
	for _, split := range splits {
		straight := mustSim(t, cfg, 7)
		straight.SetRunners(runners)
		if setup != nil {
			setup(straight)
		}
		sc := t048NewController(mode)
		for straight.ClockMS() < split {
			if mid != nil {
				mid(straight, straight.ClockMS())
			}
			t048Step(straight, sc)
		}
		resumed, rc := t048MarshalResume(t, straight, sc)
		for straight.ClockMS() < horizon {
			if mid != nil {
				mid(straight, straight.ClockMS())
				mid(resumed, resumed.ClockMS())
			}
			t048Step(straight, sc)
			t048Step(resumed, rc)
			if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
				t.Fatalf("%s split=%d @%d: sim snapshot diverged", name, split, straight.ClockMS())
			}
			if !t048CtlEqual(sc, rc) {
				t.Fatalf("%s split=%d @%d: controller state diverged", name, split, straight.ClockMS())
			}
		}
		if !reflect.DeepEqual(t046SamplesTail(straight, resumed), resumed.Samples()) {
			t.Fatalf("%s split=%d: resumed sample tail differs", name, split)
		}
	}
}

// TestT048JSONContinuation covers a feasible approach, the crossing fallback, a
// real fractional deflection and the seen-then-stale boundary across split clocks.
func TestT048JSONContinuation(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	t048AssertContinuation(t, "approach", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 0)}, t048ModeInertial, 800,
		func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.58, Y: 0.50, VXM: -4.0}) },
		nil, []int{0, 20, 40, 100, 200, 400})

	t048AssertContinuation(t, "crossing", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 0)}, t048ModeInertial, 600,
		func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}) },
		nil, []int{0, 20, 100, 300})

	t048AssertContinuation(t, "deflection", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 0)}, t048ModeTopSpeed, 300,
		func(s *Sim) { s.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50}); s.controlError = 1.0 },
		nil, []int{0, 20, 40, 100, 200})

	t048AssertContinuation(t, "seen-then-stale", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 3.0, 0)}, t048ModeInertial, 400,
		func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.50 + 2.0/PitchLengthM, Y: 0.50, VXM: 6.0}) },
		nil, []int{80, 100, 120, 200, 300})

	// A GENUINELY armed reaction (ReactAtMS=200 set by the real ReleasePass) with
	// splits due-before/at/after the reaction deadline.
	t048AssertContinuation(t, "delay-reaction", cfg,
		[]Runner{t047Sensor(0.50, 0.50, 200, 200)}, t048ModeInertial, 600,
		func(s *Sim) {
			s.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
		},
		nil, []int{180, 200, 220})

	t.Logf("JSON continuation: 5 scenes x multiple splits value-identical (snapshot+controller+samples)")
}

// --- required genuine seen-then-stale fixture -------------------------------

// TestT048SeenThenStale builds a REAL in-range rolling sighting and then lets the
// ball naturally leave the sensing circle: the record must stay Seen at the last
// in-range clock and the pre-step ball/runner distance must exceed the radius at
// a later due clock. (T-047's outside row was never-seen UNKNOWN — this is the
// required actual stale fixture.)
func TestT048SeenThenStale(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const radius = 3.0
	ctl := t048NewController(t048ModeInertial)
	// The ball starts OUTSIDE the 1.0 m control reach but INSIDE the 3.0 m sensing
	// circle (2.0 m ahead), rolling away, so the runner genuinely SEES a free
	// rolling ball without collecting it, then the ball naturally leaves the radius.
	recs, sim := t048Run(t, cfg, []Runner{t047Sensor(0.50, 0.50, radius, 0)}, ctl, 400,
		func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.50 + 2.0/PitchLengthM, Y: 0.50, VXM: 6.0}) }, nil)
	if ms, _ := sim.FirstContact(); ms >= 0 {
		t.Fatalf("seen-then-stale: an initial control at %d ms (the ball must start outside reach)", ms)
	}

	lastIn := -1
	firstOut := -1
	outObsClock := -1
	for _, rec := range recs {
		if !rec.Published {
			continue
		}
		pre := rec.ClockMS - SubstepMS
		ro := rec.Runners[0]
		d := DistM(rec.Ball.X, rec.Ball.Y, ro.PreX, ro.PreY)
		if ro.Seen && d <= radius {
			lastIn = pre
		}
		if ro.Seen && d > radius && firstOut < 0 {
			firstOut = pre
			outObsClock = ro.ObsClockMS
		}
	}
	if lastIn < 0 {
		t.Fatal("seen-then-stale: the ball was never actually sighted in range")
	}
	if firstOut < 0 {
		t.Fatal("seen-then-stale: the ball never left the sensing circle")
	}
	if firstOut <= lastIn {
		t.Fatalf("seen-then-stale: out-of-range %d not after last in-range %d", firstOut, lastIn)
	}
	if outObsClock != lastIn {
		t.Fatalf("seen-then-stale: stale record clock %d != last in-range clock %d (not refreshed, not cleared)", outObsClock, lastIn)
	}
	if outObsClock >= firstOut {
		t.Fatalf("seen-then-stale: record clock %d is not before the out due %d", outObsClock, firstOut)
	}
	t048AssertObservationsTruthful(t, recs)
	t.Logf("seen-then-stale: real sighting in [%d,%d]; record stays Seen clock=%d; pre-step distance>%.1fm at due %d (truthful, not a fabricated future obs)",
		recs[0].ClockMS-SubstepMS, lastIn, outObsClock, radius, firstOut)
}

// --- real unseen deflection watcher -----------------------------------------

// TestT048UnseenDeflectionWatcher proves an observer that previously SAW the ball
// keeps only its truthful earlier sighting when ANOTHER runner miscontrols the
// ball OUTSIDE the observer radius: a real deflect event is emitted, the record is
// retained stale, the fresh-only policy falls back, and no observation is faked.
func TestT048UnseenDeflectionWatcher(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const radius = 2.5
	ctl := t048NewController(t048ModeInertial)
	setup := func(s *Sim) {
		s.SetBall(Ball{Mode: BallRolling, X: 0.40 + 1.5/PitchLengthM, Y: 0.50, VXM: 6.0})
		s.controlError = 1.0
	}
	recs, sim := t048Run(t, cfg,
		[]Runner{t047Sensor(0.40, 0.50, radius, 0), t047Sensor(0.46, 0.50, radius, 0)},
		ctl, 1000, setup, nil)

	// A real contact-local miscontrol by the OTHER runner (index 1).
	defClock, defRunner := -1, -1
	for _, e := range sim.Events() {
		if e.Kind == PassEventDeflect {
			defClock, defRunner = e.ClockMS, e.Runner
			break
		}
	}
	if defClock < 0 || defRunner != 1 {
		t.Fatalf("unseen-deflect: no real deflect by runner 1 (clock=%d runner=%d)", defClock, defRunner)
	}

	obsSeenAt := -1
	staleAt := -1
	for _, rec := range recs {
		if !rec.Published {
			continue
		}
		pre := rec.ClockMS - SubstepMS
		ro := rec.Runners[0]
		if ro.Seen && ro.ObsClockMS == pre {
			obsSeenAt = pre // refreshed (a real in-range sighting) at this due
		}
		d := DistM(rec.Ball.X, rec.Ball.Y, ro.PreX, ro.PreY)
		if ro.Seen && pre >= defClock && d > radius && staleAt < 0 {
			staleAt = pre
			if ro.ObsClockMS >= pre {
				t.Fatalf("unseen-deflect: observation not stale (%d >= %d)", ro.ObsClockMS, pre)
			}
			if ro.Predicted {
				t.Fatalf("unseen-deflect: stale record was predicted (fresh-only must fall back)")
			}
		}
	}
	if obsSeenAt < 0 {
		t.Fatal("unseen-deflect: observer never actually saw the ball")
	}
	if staleAt < 0 {
		t.Fatal("unseen-deflect: observer never went stale after the deflect")
	}
	t048AssertObservationsTruthful(t, recs)
	t.Logf("unseen-deflect: real deflect by runner %d @%dms; observer last saw @%dms; retained truthful stale, fresh-only fallback @%dms (distance>%.1fm; no fake observation)",
		defRunner, defClock, obsSeenAt, staleAt, radius)
}

// --- positive (feasible) candidate decision cost ----------------------------

// TestT048PositiveCandidateCost measures the POSITIVE feasible-candidate decision
// cost in isolation on the approach scene (complementing the 22-body
// full-scan-rejection WORKLOAD; it does NOT tune the horizon to force benefit).
func TestT048PositiveCandidateCost(t *testing.T) {
	in := t048InertialInput{
		NowMS: 0, RX: 0.50, RY: 0.50, RVXM: 0, RVYM: 0,
		TopSpeedMPS: PlayerTopSpeedMPS, AccelMPS2: PlayerAccelMaxMPS2,
		DecelMPS2: PlayerDecelMPS2, LateralAccelMPS2: PlayerLateralAccelMaxMPS2,
		ReachM: ControlRadiusM, HorizonMS: t048HorizonMS, MaxAgeMS: t048MaxAgeMS,
		MaxSteps: t048MaxSteps, FreshOnly: true,
		Seen: true, ObsClockMS: 0, ObsX: 0.58, ObsY: 0.50, ObsVXM: -4.0, ObsMode: BallRolling, ObsRadiusM: 200,
	}
	_, _, tau, reached, steps, reason := t048InertialRollout(in)
	if !reached || steps <= 0 || reason != t048ReasonReached {
		t.Fatalf("positive candidate not feasible: reached=%v steps=%d reason=%q", reached, steps, reason)
	}
	res := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			t048InertialRollout(in)
		}
	})
	t.Logf("positive candidate: reached=%v tau=%.4f s steps=%d ns/decision=%d (feasible decision cost, NOT a benefit claim)",
		reached, tau, steps, res.NsPerOp())
}

// --- three-mode scene comparison + truthfulness -----------------------------

// t048Scenes returns the prespecified scenes (see the session note §3).
func t048Scenes() []t047SceneDef {
	return []t047SceneDef{
		{
			name: "rest", horizon: 300, note: "Dead ball in range => non-free fallback (brake)",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallDead, X: 0.60, Y: 0.50}) },
		},
		{
			name: "turn-away", horizon: 800, note: "runner already moving +x, ball rolling away behind",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup: func(s *Sim) {
				s.runners[0].VXM = 4.0
				s.runners[0].IntentX, s.runners[0].IntentY = 1.0, 0.50
				s.SetBall(Ball{Mode: BallRolling, X: 0.40, Y: 0.50, VXM: -5.0})
			},
		},
		{
			name: "crossing", horizon: 1200, note: "T-047 crossing: ball +y 7 m/s",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}) },
		},
		{
			name: "approach", horizon: 1500, note: "ball rolling toward the runner (feasible)",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.58, Y: 0.50, VXM: -4.0}) },
		},
		{
			name: "delay", horizon: 600, note: "real ReleasePass, reaction 200 ms",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 200)},
			setup: func(s *Sim) {
				s.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
			},
		},
		{
			name: "decel", horizon: 600, note: "decelerating ball; constant-velocity overshoot",
			runners: []Runner{t047Sensor(0.48, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 5.0}) },
		},
		{
			name: "seen-then-stale", horizon: 400, note: "real in-range sighting then natural exit (outside reach, inside radius)",
			runners: []Runner{t047Sensor(0.50, 0.50, 3.0, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.50 + 2.0/PitchLengthM, Y: 0.50, VXM: 6.0}) },
		},
		{
			name: "deflection", horizon: 300, note: "real contact-local miscontrol (T-047 scene)",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup: func(s *Sim) {
				s.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
				s.controlError = 1.0
			},
		},
		{
			name: "stale-deflect", horizon: 400, note: "real deflect pushes the ball out of a small circle",
			runners: []Runner{t047Sensor(0.50, 0.50, 1.0, 0)},
			setup: func(s *Sim) {
				s.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 8.0})
				s.controlError = 1.0
			},
		},
		{
			name: "unseen-deflect", horizon: 1000, note: "observer saw the ball, then ANOTHER runner miscontrols it OUTSIDE the observer radius",
			runners: []Runner{t047Sensor(0.40, 0.50, 2.5, 0), t047Sensor(0.46, 0.50, 2.5, 0)},
			setup: func(s *Sim) {
				s.SetBall(Ball{Mode: BallRolling, X: 0.40 + 1.5/PitchLengthM, Y: 0.50, VXM: 6.0})
				s.controlError = 1.0
			},
		},
		{
			name: "boundary", horizon: 300, note: "ball exits the pitch",
			runners: []Runner{t047Sensor(0.98, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.995, Y: 0.50, VXM: 5.0}) },
		},
	}
}

// TestT048ThreeModeSceneComparison runs every prespecified scene under chase,
// topspeed and inertial with identical setup/mid, verifies observation
// truthfulness, and reports the full-run paired differences and the ORDERED actual
// contact/event sequences with exact fractional timestamps plus the inertial
// candidate's reached/not verdict and rollout cost. No claim that prediction wins.
func TestT048ThreeModeSceneComparison(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	scenes := t048Scenes()
	for _, sc := range scenes {
		chaseRecs, _ := t048Run(t, cfg, sc.runners, t048NewController(t048ModeChase), sc.horizon, sc.setup, sc.mid)
		topCtl := t048NewController(t048ModeTopSpeed)
		topRecs, _ := t048Run(t, cfg, sc.runners, topCtl, sc.horizon, sc.setup, sc.mid)
		inrCtl := t048NewController(t048ModeInertial)
		inrRecs, _ := t048Run(t, cfg, sc.runners, inrCtl, sc.horizon, sc.setup, sc.mid)

		if len(topRecs) != len(chaseRecs) || len(inrRecs) != len(chaseRecs) {
			t.Fatalf("%s: step counts differ chase=%d top=%d inertial=%d", sc.name, len(chaseRecs), len(topRecs), len(inrRecs))
		}
		if len(chaseRecs) == 0 || chaseRecs[len(chaseRecs)-1].ClockMS != sc.horizon {
			t.Fatalf("%s: run did not reach the full horizon %d", sc.name, sc.horizon)
		}
		t048AssertObservationsTruthful(t, chaseRecs)
		t048AssertObservationsTruthful(t, topRecs)
		t048AssertObservationsTruthful(t, inrRecs)

		mT := t047PairMetricsOf(chaseRecs, topRecs)
		mI := t047PairMetricsOf(chaseRecs, inrRecs)
		for _, m := range []t047PairMetrics{mT, mI} {
			if !isFinite(m.MaxPlayerPos) || !isFinite(m.MaxPlayerVel) || !isFinite(m.BallPos) || !isFinite(m.BallVel) || !isFinite(m.MaxTargetDelta) {
				t.Fatalf("%s: non-finite paired metric %s", sc.name, m)
			}
		}
		if inrCtl.RolloutRuns == 0 {
			t.Fatalf("%s: inertial candidate never invoked (runs=0)", sc.name)
		}
		// Per-scene step expectations (repair2). Every scene INVOKES the candidate,
		// but only a genuinely FREE FRESH record can integrate, so the step count
		// depends on the observed mode (a non-free/unknown/stale record may legitimately
		// short-circuit with zero integration before any step).
		switch sc.name {
		case "rest":
			// The observed ball is Dead => non-free => no integration, no prediction.
			if inrCtl.RolloutSteps != 0 || inrCtl.Predictions != 0 {
				t.Fatalf("rest: expected non-free zero-integration/zero-prediction, got steps=%d predictions=%d", inrCtl.RolloutSteps, inrCtl.Predictions)
			}
			sawNonFree := false
			for _, rec := range inrRecs {
				for _, ro := range rec.Runners {
					if !rec.Published || !ro.Eligible {
						continue
					}
					if ro.Predicted {
						t.Fatalf("rest: a non-free scene must not predict")
					}
					if ro.Reason == t047ReasonNonFree {
						sawNonFree = true
					}
				}
			}
			if !sawNonFree {
				t.Fatal("rest: no non-free reason was recorded")
			}
		case "crossing", "approach":
			// A genuinely free fresh record must integrate the bounded scan.
			if inrCtl.RolloutSteps == 0 {
				t.Fatalf("%s: a known free scene must integrate (steps>0)", sc.name)
			}
		}
		t.Logf("[%s | %s] horizon=%dms", sc.name, sc.note, sc.horizon)
		t.Logf("  chase contacts:    %v", t047ContactSeq(chaseRecs))
		t.Logf("  topspeed vs chase: %s", mT)
		t.Logf("  topspeed contacts: %v", mT.Contacts)
		t.Logf("  inertial vs chase: %s", mI)
		t.Logf("  inertial contacts: %v", mI.Contacts)
		t.Logf("  inertial candidate: runs=%d steps=%d maxSteps=%d predictions=%d fallbacks=%d",
			inrCtl.RolloutRuns, inrCtl.RolloutSteps, inrCtl.RolloutMaxSteps, inrCtl.Predictions, inrCtl.Fallbacks)
	}
	t.Logf("three-mode comparison: %d scenes x chase/topspeed/inertial over the full horizon; NO claim of an earlier/winning control", len(scenes))
}
