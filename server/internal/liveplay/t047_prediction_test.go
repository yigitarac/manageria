package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// T-047 observation-based interception prediction STUDY (test-only, isolated
// package `liveplay`). See vault/07-Session-Logs/2026-10-13-t047-interception-
// study-evidence.md for the PRESPEC (and the repair1 prespec additions, §4)
// written BEFORE these changes.
//
// This file adds NO runtime/non-test source. It contains:
//   - a pure, observation-only bounded interception estimator (audited inputs,
//     finite/bounded guards, explicitly OPTIMISTIC reachability);
//   - a test-only controller with an INDEPENDENT re-implementation of the T-044
//     chase, a FRESH-ONLY prediction variant and a separate harmfully-stale
//     WITHIN-AGE variant;
//   - a harness that reproduces the T-046 PER-RUNNER local gating (LocalWakeup /
//     pending / shared native semantics), samples the REAL T-046 observation
//     after the reaction gate, then installs the estimate for the eligible local
//     runner BEFORE the real physics Step integrates it (never post-step).
//
// All numbers are explicit assumptions, not measured truths or outcome knobs.

const (
	// t047HorizonMS is the maximum prediction horizon (ms).
	t047HorizonMS = 500
	// t047MaxAgeMS is the maximum observation age that may be extrapolated (ms).
	t047MaxAgeMS = 100
	// t047CoordLimit bounds a physically meaningful normalized coordinate; an
	// observation/runner outside [-limit, limit] is treated as invalid input.
	t047CoordLimit = 10.0
	// t047EpsA is the tolerance under which the quadratic is treated as linear
	// (|ball velocity| == runner top speed), for numerical stability.
	t047EpsA = 1e-9
)

// t047Mode selects the controller policy.
type t047Mode uint8

const (
	// t047ModeChase is the independent re-implementation of the T-044 chase.
	t047ModeChase t047Mode = iota
	// t047ModePredictFresh predicts ONLY a truly fresh record and rejects any
	// age > 0 (falls back to T-044).
	t047ModePredictFresh
	// t047ModePredictWithinAge extrapolates a record up to MaxAgeMS; this is the
	// deliberately HARMFUL stale hypothesis, kept separate from fresh-only.
	t047ModePredictWithinAge
)

// String returns a stable label for evidence.
func (m t047Mode) String() string {
	switch m {
	case t047ModeChase:
		return "chase"
	case t047ModePredictFresh:
		return "predict-fresh"
	case t047ModePredictWithinAge:
		return "predict-within-age"
	default:
		return "unknown"
	}
}

// Window labels the age policy so evidence cannot confuse the two variants.
func (m t047Mode) Window() string {
	switch m {
	case t047ModePredictFresh:
		return "fresh-only(age==0)"
	case t047ModePredictWithinAge:
		return "within-age(0..100ms,harmful-stale-hypothesis)"
	default:
		return "t044-chase"
	}
}

// Estimator result reasons.
const (
	t047ReasonOK             = "ok"
	t047ReasonAlreadyReached = "already-in-reach"
	t047ReasonUnknown        = "unknown"
	t047ReasonNonFree        = "non-free-mode"
	t047ReasonInvalid        = "invalid-record"
	t047ReasonOverage        = "overage"
	t047ReasonFreshRejected  = "stale-fresh-rejected"
	t047ReasonNoRoot         = "no-root"
	t047ReasonBeyondHorizon  = "beyond-horizon"
	t047ReasonChase          = "chase"
)

// t047EstimateInput is the AUDITED estimator input: only the runner's own
// observation, own state/limits, the current clock and the controller config.
type t047EstimateInput struct {
	NowMS      int
	Seen       bool
	ObsClockMS int
	ObsX       float64
	ObsY       float64
	ObsVXM     float64
	ObsVYM     float64
	ObsMode    BallMode
	ObsRadiusM float64

	RX          float64
	RY          float64
	TopSpeedMPS float64
	ReachM      float64

	HorizonMS int
	MaxAgeMS  int
	ApplyAge  bool
	FreshOnly bool
}

// t047Estimate is the bounded observed-velocity interception estimate.
//
// It returns a target (clamped to the pitch), the intercept time tau in seconds,
// ok and a reason label. It is a PURE function of `in` and reads nothing else
// (no global ball/owner, no true ball deceleration, no future state).
//
// LIMIT: tau is a LOWER-BOUND, OPTIMISTIC reachability estimate. The reachable
// envelope is `TopSpeedMPS*tau + ReachM` with an INSTANT heading change and full
// top speed from rest, so it ignores the runner's current velocity, its
// acceleration ramp and its turning limit. A positive result is therefore NOT a
// reachability proof and NOT a guaranteed contact.
func t047Estimate(in t047EstimateInput) (tx, ty, tauS float64, ok bool, reason string) {
	if !in.Seen {
		return 0, 0, 0, false, t047ReasonUnknown
	}
	if in.ObsMode != BallRolling && in.ObsMode != BallLoose {
		return 0, 0, 0, false, t047ReasonNonFree
	}
	if !isFinite(in.ObsRadiusM) || in.ObsRadiusM <= 0 {
		return 0, 0, 0, false, t047ReasonInvalid
	}
	if !isFinite(in.ObsX) || !isFinite(in.ObsY) || !isFinite(in.ObsVXM) || !isFinite(in.ObsVYM) ||
		!isFinite(in.RX) || !isFinite(in.RY) || !isFinite(in.TopSpeedMPS) || !isFinite(in.ReachM) {
		return 0, 0, 0, false, t047ReasonInvalid
	}
	// Bound to a physically meaningful pitch envelope; a huge unphysical input
	// falls back rather than producing a garbage target.
	if math.Abs(in.ObsX) > t047CoordLimit || math.Abs(in.ObsY) > t047CoordLimit ||
		math.Abs(in.RX) > t047CoordLimit || math.Abs(in.RY) > t047CoordLimit {
		return 0, 0, 0, false, t047ReasonInvalid
	}
	if in.TopSpeedMPS <= 0 || in.ReachM < 0 || in.HorizonMS <= 0 || in.MaxAgeMS < 0 {
		return 0, 0, 0, false, t047ReasonInvalid
	}
	age := in.NowMS - in.ObsClockMS
	if age < 0 {
		return 0, 0, 0, false, t047ReasonInvalid
	}
	if in.FreshOnly && age > 0 {
		return 0, 0, 0, false, t047ReasonFreshRejected
	}
	if age > in.MaxAgeMS {
		return 0, 0, 0, false, t047ReasonOverage
	}
	ageS := 0.0
	if in.ApplyAge {
		ageS = float64(age) / 1000.0
	}

	// Ball start (with the declared age term) and relative position in metres.
	bx := in.ObsX + (in.ObsVXM/PitchLengthM)*ageS
	by := in.ObsY + (in.ObsVYM/PitchWidthM)*ageS
	d0x := (bx - in.RX) * PitchLengthM
	d0y := (by - in.RY) * PitchWidthM
	if d0x*d0x+d0y*d0y <= in.ReachM*in.ReachM {
		return clamp01(bx), clamp01(by), 0, true, t047ReasonAlreadyReached
	}

	// Solve |D0 + U*tau|^2 = (v*tau + reach)^2 for the earliest tau >= 0.
	ux, uy := in.ObsVXM, in.ObsVYM
	v, r := in.TopSpeedMPS, in.ReachM
	a := ux*ux + uy*uy - v*v
	b := 2 * (d0x*ux + d0y*uy - v*r)
	c := d0x*d0x + d0y*d0y - r*r
	if !isFinite(a) || !isFinite(b) || !isFinite(c) {
		return 0, 0, 0, false, t047ReasonNoRoot
	}

	tau := math.Inf(1)
	if math.Abs(a) < t047EpsA {
		// Equal speeds: the quadratic degenerates to a linear equation.
		if math.Abs(b) < t047EpsA {
			return 0, 0, 0, false, t047ReasonNoRoot
		}
		if t := -c / b; t >= 0 {
			tau = t
		}
	} else {
		disc := b*b - 4*a*c
		if !isFinite(disc) || disc < 0 {
			return 0, 0, 0, false, t047ReasonNoRoot
		}
		sq := math.Sqrt(disc)
		if t1 := (-b - sq) / (2 * a); t1 >= 0 && t1 < tau {
			tau = t1
		}
		if t2 := (-b + sq) / (2 * a); t2 >= 0 && t2 < tau {
			tau = t2
		}
	}
	if math.IsInf(tau, 1) || math.IsNaN(tau) {
		return 0, 0, 0, false, t047ReasonNoRoot
	}
	if tau > float64(in.HorizonMS)/1000.0 {
		return 0, 0, 0, false, t047ReasonBeyondHorizon
	}

	// Target = the ball at the assumed (age + tau) time, clamped to the pitch.
	tTot := ageS + tau
	tx = clamp01(in.ObsX + (in.ObsVXM/PitchLengthM)*tTot)
	ty = clamp01(in.ObsY + (in.ObsVYM/PitchWidthM)*tTot)
	if !isFinite(tx) || !isFinite(ty) {
		return 0, 0, 0, false, t047ReasonNoRoot
	}
	return tx, ty, tau, true, t047ReasonOK
}

// t047Reach returns the runner's own control reach (its ReachM, else the default).
func t047Reach(r *Runner) float64 {
	if r.ReachM > 0 {
		return r.ReachM
	}
	return ControlRadiusM
}

// t047RunnerEv is the per-runner decision evidence of one publication.
type t047RunnerEv struct {
	Index      int
	Eligible   bool
	Predicted  bool
	Reason     string
	TauS       float64
	TX, TY     float64
	RX, RY     float64
	ObsSeen    bool
	ObsClockMS int
	ObsX       float64
	ObsY       float64
	ObsVXM     float64
	ObsVYM     float64
	ObsMode    BallMode
}

// t047Controller is the test-only controller. Its serialized state is the config
// + counters + last decision clock; `LastEv` is transient evidence (never
// serialized) and it draws NO RNG.
type t047Controller struct {
	Mode           t047Mode `json:"mode"`
	HorizonMS      int      `json:"horizonMs"`
	MaxAgeMS       int      `json:"maxAgeMs"`
	LastDecisionMS int      `json:"lastDecisionMs"`
	Predictions    int      `json:"predictions"`
	Fallbacks      int      `json:"fallbacks"`

	LastEv []t047RunnerEv `json:"-"`
}

// t047NewController returns a controller with the prespecified horizon/age.
func t047NewController(mode t047Mode) *t047Controller {
	return &t047Controller{
		Mode:           mode,
		HorizonMS:      t047HorizonMS,
		MaxAgeMS:       t047MaxAgeMS,
		LastDecisionMS: -1,
	}
}

// inputFor builds the audited estimator input from a runner and its OWN record.
func (c *t047Controller) inputFor(r *Runner, o *BallObservation, clock int) t047EstimateInput {
	return t047EstimateInput{
		NowMS: clock, RX: r.X, RY: r.Y,
		TopSpeedMPS: r.Params().TopSpeedMPS, ReachM: t047Reach(r),
		HorizonMS: c.HorizonMS, MaxAgeMS: c.MaxAgeMS,
		ApplyAge:  c.Mode == t047ModePredictWithinAge,
		FreshOnly: c.Mode == t047ModePredictFresh,
		Seen:      o.Seen, ObsClockMS: o.ClockMS,
		ObsX: o.X, ObsY: o.Y, ObsVXM: o.VXM, ObsVYM: o.VYM,
		ObsMode: o.Mode, ObsRadiusM: o.RadiusM,
	}
}

// t047FallbackTarget maps an estimator failure to the EXACT T-044 fallback: an
// invalid record retains the prior destination (no inadvertent brake), every
// other case uses the T-044 policy (unknown/non-free brake, stale retain).
func t047FallbackTarget(r *Runner, o *BallObservation, clock int, reason string) (float64, float64) {
	if reason == t047ReasonInvalid {
		return r.IntentX, r.IntentY
	}
	return t047ChaseTarget(r, o, clock)
}

// policy computes the (x,y) destination and returns the estimator outcome.
func (c *t047Controller) policy(r *Runner, o *BallObservation, clock int) (tx, ty float64, predicted bool, reason string, tau float64) {
	if c.Mode == t047ModeChase {
		x, y := t047ChaseTarget(r, o, clock)
		return x, y, false, t047ReasonChase, 0
	}
	tx, ty, tau, ok, reason := t047Estimate(c.inputFor(r, o, clock))
	if ok {
		return tx, ty, true, reason, tau
	}
	x, y := t047FallbackTarget(r, o, clock, reason)
	return x, y, false, reason, tau
}

// applyRunner runs the controller for ONE eligible local runner and records the
// evidence. It fails closed on a missing/invalid sensing record, a non-inertial
// runner or a pre-reaction clock.
func (c *t047Controller) applyRunner(sim *Sim, i, clock int) t047RunnerEv {
	r := &sim.runners[i]
	o := r.Perception
	ev := t047RunnerEv{Index: i, RX: r.X, RY: r.Y}
	if o != nil {
		ev.ObsSeen, ev.ObsClockMS = o.Seen, o.ClockMS
		ev.ObsX, ev.ObsY, ev.ObsVXM, ev.ObsVYM = o.X, o.Y, o.VXM, o.VYM
		ev.ObsMode = o.Mode
	}
	if !o.SensingEnabled() || r.Motion != MotionInertial || clock < r.ReactAtMS {
		return ev
	}
	ev.Eligible = true
	tx, ty, predicted, reason, tau := c.policy(r, o, clock)
	r.IntentX, r.IntentY = tx, ty
	ev.Predicted, ev.Reason, ev.TauS, ev.TX, ev.TY = predicted, reason, tau, tx, ty
	c.LastDecisionMS = clock
	if predicted {
		c.Predictions++
	} else {
		c.Fallbacks++
	}
	return ev
}

// applySim runs the controller for the gated LocalWakeup runners only, recording
// per-runner evidence.
func (c *t047Controller) applySim(sim *Sim, gate []bool, clock int) {
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

// t047ChaseTarget is the INDEPENDENT re-implementation of the T-044 policy: it
// reads only the runner's own observation + own position.
func t047ChaseTarget(r *Runner, o *BallObservation, clock int) (float64, float64) {
	switch o.Freshness(clock) {
	case ObservationUnknown:
		return r.X, r.Y
	case ObservationStale:
		return r.IntentX, r.IntentY // retain the prior destination
	default:
		switch o.Mode {
		case BallRolling, BallLoose:
			return o.X, o.Y
		default:
			return r.X, r.Y
		}
	}
}

// t047Step reproduces the T-046 PER-RUNNER local gating INDEPENDENTLY (not the
// T-042 shared invalidation), samples the REAL T-046 observation after the
// reaction gate, runs the shared T-044 local layer for unflagged runners, and
// installs the controller estimate for the eligible LocalWakeup runners BEFORE
// the real physics Step integrates it. The built-in scheduler is left inert for
// the step (lattice advanced, dirty cleared, pendings consumed) so no runtime
// publisher overwrites the intents. Returns whether a publication occurred.
//
// Gating (identical to the runtime T-046 publishLocalSamples):
//   - unflagged runner: local layer runs iff sharedDue (lattice OR dirty);
//   - LocalWakeup runner with an invalid record: never runs (pending dropped);
//   - LocalWakeup runner: runs iff latticeDue OR its OWN WakePending.
func t047Step(sim *Sim, ctl *t047Controller) bool {
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
		ctl.applySim(sim, gate, clock) // T-047 estimate for eligible LocalWakeup runners
	}
	sim.decisionDirty = false
	sim.Step()
	return true
}

// t047Sensor builds an inertial sensing LocalWakeup runner driven by the T-047
// controller. ReactAtMS is only set by a real ReleasePass; a SetBall scene has an
// inert reaction gate (ReactAtMS 0).
func t047Sensor(x, y, radius float64, reactMS int) Runner {
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	r.ReachM = ControlRadiusM
	if err := r.EnableLocalWakeup(radius); err != nil {
		panic(err)
	}
	r.ReactMS = reactMS
	return r
}

// t047RunnerObs is one runner's recorded state/observation of a step.
type t047RunnerObs struct {
	PreX, PreY     float64
	X, Y, VXM, VYM float64
	IX, IY         float64
	Seen           bool
	ObsClockMS     int
	ObsX, ObsY     float64
	ObsVXM, ObsVYM float64
	ObsMode        BallMode
	ObsRadiusM     float64
	Eligible       bool
	Predicted      bool
	Reason         string
	TauS           float64
	TX, TY         float64
}

// t047StepRec is one recorded step: the PRE-step ball, every runner's state and
// observation, and the contact/deflection event fractions emitted in the step.
type t047StepRec struct {
	ClockMS   int
	Published bool
	Ball      Ball // PRE-step ball (sample-before-motion)
	Owner     int  // PRE-step held owner, -1 if unheld
	Runners   []t047RunnerObs
	Events    []PassEvent
}

// t047Run steps one scene and records the full per-step evidence.
func t047Run(t *testing.T, cfg Config, runners []Runner, ctl *t047Controller, horizon int, setup func(*Sim), mid func(*Sim, int)) ([]t047StepRec, *Sim) {
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
		pub := t047Step(sim, ctl)
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

// t047CtlEqual compares the SERIALIZED controller state (ignores transient LastEv).
func t047CtlEqual(a, b *t047Controller) bool {
	aa, bb := *a, *b
	aa.LastEv, bb.LastEv = nil, nil
	return reflect.DeepEqual(aa, bb)
}

// t047Truth is a free-ball (no-runner) trajectory used ONLY as evaluation truth.
type t047Truth struct {
	ball []Ball
}

// t047BuildTruth records the free-ball trajectory of a scene. The ball's motion
// is independent of runner intents, so this is the true ball for error scoring.
func t047BuildTruth(t *testing.T, cfg Config, ball Ball, horizonMS int) *t047Truth {
	t.Helper()
	sim := mustSim(t, cfg, 101)
	sim.SetBall(ball)
	tr := &t047Truth{ball: make([]Ball, 0, horizonMS/SubstepMS+1)}
	tr.ball = append(tr.ball, sim.Ball())
	for sim.ClockMS() < horizonMS {
		sim.StepQuiet()
		tr.ball = append(tr.ball, sim.Ball())
	}
	return tr
}

// at returns the truth ball at the nearest substep to ms.
func (tr *t047Truth) at(ms int) (Ball, bool) {
	if ms < 0 {
		return Ball{}, false
	}
	idx := (ms + SubstepMS/2) / SubstepMS
	if idx < 0 || idx >= len(tr.ball) {
		return Ball{}, false
	}
	return tr.ball[idx], true
}

// t047Persisted is the complete JSON continuation unit: the Sim snapshot plus the
// controller state (the gating is derived deterministically from both, so no
// further harness state is needed).
type t047Persisted struct {
	Sim Snapshot       `json:"sim"`
	Ctl t047Controller `json:"controller"`
}

// t047MarshalResume marshals {Sim snapshot, controller} and resumes both.
func t047MarshalResume(t *testing.T, sim *Sim, ctl *t047Controller) (*Sim, *t047Controller) {
	t.Helper()
	blob, err := json.Marshal(t047Persisted{Sim: sim.Snapshot(), Ctl: *ctl})
	if err != nil {
		t.Fatalf("marshal t047 state: %v", err)
	}
	var p t047Persisted
	if err := json.Unmarshal(blob, &p); err != nil {
		t.Fatalf("unmarshal t047 state: %v", err)
	}
	rs, err := Resume(p.Sim)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	c := p.Ctl
	return rs, &c
}

// t047AlongHeading returns the dot product (m^2/s) of (target - origin) with the
// observed ball velocity; positive means the target LEADS along the heading.
func t047AlongHeading(ox, oy, vx, vy, tx, ty float64) float64 {
	return ((tx-ox)*PitchLengthM)*vx + ((ty-oy)*PitchWidthM)*vy
}
