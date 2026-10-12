package liveplay

import (
	"encoding/json"
	"math"
	"testing"
)

// T-041 multi-scene intent-cadence and transition study (isolated prototype
// evidence). This file measures what changes when a player's movement INTENT is
// republished on a coarse cadence (20/40/100/200 ms) while the physics substep
// stays fixed at the accepted 20 ms. It is a STUDY, not a scheduler: no default
// is switched, no production file is touched and the Sim Snapshot gains no
// scheduling field. The recommended persisted contract is recorded in the
// session evidence, not implemented here.
//
// Reference: cadence = one substep (20 ms) with the IDENTICAL 20 ms integration,
// expressed through an external controller that reproduces the engine's own
// per-substep pursuit publication exactly (proved bit-identical in
// TestT041CadenceReferenceMatchesNative). Coarser cadences publish intents only
// on their boundary, so between boundaries every player keeps integrating
// toward its last published intent. All players integrate EVERY substep; the
// cadence only changes WHEN intents are refreshed.
//
// Two scheduling policies are studied separately:
//   - periodic perception: republish on a fixed substep cadence;
//   - event-triggered invalidation: the same cadence PLUS an out-of-band
//     republish on the substep after any state-changing ball event
//     (control/deflect/stop/out). It never bypasses the reaction gate: a runner
//     is only republished once clock >= ReactAtMS. Cooldowns are enforced by the
//     swept contact resolver and are never bypassed by scheduling.
//
// Every scene below contains an ACTUAL contact. Error is classified by phase:
// the largest aligned player error is labelled pre-event or post-event relative
// to the reference's first state-changing event, because a post-contact stale
// publication (a player still heading to a pre-deflection ball position) inflates
// the full-run total without any movement being skipped.

// Prespecified comparison tolerances, fixed BEFORE the measurements below were
// taken. "Safe" means the coarse cadence keeps the same causal outcome (first
// event class+runner, ordered contact sequence, terminal mode/owner) AND stays
// inside a deliberately roomy position envelope. They are not tightened to the
// observed values and are not universal realism/equivalence bounds.
const (
	t041FullRunBallMaxM   = 1.00 // m, full-run aligned ball discrepancy envelope
	t041FullRunPlayerMaxM = 2.00 // m, full-run aligned player discrepancy envelope
)

// t041CadenceMode selects how the study controller refreshes intents.
type t041CadenceMode int

const (
	// t041Periodic republishes intents only on fixed substep boundaries.
	t041Periodic t041CadenceMode = iota
	// t041EventTriggered republishes on the fixed cadence AND on the substep
	// after any new state-changing ball event (still reaction-gated).
	t041EventTriggered
)

func (m t041CadenceMode) String() string {
	if m == t041EventTriggered {
		return "event"
	}
	return "periodic"
}

// t041StateChanging reports whether an event changes the free-ball state and so
// invalidates a perception/decision (control, deflect, stop, out). Release is the
// initial condition and path is a pure position trace, so neither triggers.
func t041StateChanging(k PassEventKind) bool {
	switch k {
	case PassEventControl, PassEventDeflect, PassEventStop, PassEventOut:
		return true
	default:
		return false
	}
}

// t041CadenceState is the complete EXTERNAL scheduling state of the study
// controller. It is deliberately NOT part of Sim.Snapshot: the prototype has no
// persisted cadence field (TestT041ExternalCadenceStateIsHarnessOnly), so this is
// harness state, not a Sim.Resume contract.
type t041CadenceState struct {
	Step           int             `json:"step"`
	LastEventCount int             `json:"lastEventCount"`
	Period         int             `json:"period"`
	Mode           t041CadenceMode `json:"mode"`
}

// t041Controller drives intent publication for a scene. The physics stays inside
// Sim; this only decides whether this substep's intents are refreshed.
type t041Controller struct {
	t041CadenceState
	perceive     []bool
	reactAt      []int
	firstPublish []int
	publishes    int
}

func t041NewController(period int, mode t041CadenceMode, perceive []bool) *t041Controller {
	return &t041Controller{
		t041CadenceState: t041CadenceState{Period: period, Mode: mode},
		perceive:         perceive,
	}
}

// bind sizes the per-runner records once from the sim. It is called before a
// timed window so the controller does not allocate during integration.
func (c *t041Controller) bind(sim *Sim) {
	if c.reactAt != nil {
		return
	}
	n := len(sim.runners)
	c.reactAt = make([]int, n)
	c.firstPublish = make([]int, n)
	for i := range sim.runners {
		c.reactAt[i] = sim.runners[i].ReactAtMS
		c.firstPublish[i] = -1
	}
}

// beforeStep runs before every Step, at the substep's start clock — the same
// point at which the engine republishes pursuit intents. It reads the sim's
// fields directly (in-package test harness) so a measured window never allocates.
func (c *t041Controller) beforeStep(sim *Sim) {
	c.bind(sim)
	publish := c.Step%c.Period == 0
	if !publish && c.Mode == t041EventTriggered {
		for i := c.LastEventCount; i < len(sim.events); i++ {
			if t041StateChanging(sim.events[i].Kind) {
				publish = true
				break
			}
		}
	}
	if publish {
		c.publishIntents(sim)
	}
	c.LastEventCount = len(sim.events)
	c.Step++
}

// publishIntents reproduces the engine's per-substep pursuit semantics: while the
// ball is free (Rolling/Loose) every designated perceiver that has reacted aims
// at the ball's current position. A perceiver whose reaction has not arrived is
// skipped, so the cadence can never bypass the reaction gate.
func (c *t041Controller) publishIntents(sim *Sim) {
	b := &sim.ball
	if b.Mode != BallRolling && b.Mode != BallLoose {
		return
	}
	for i := range sim.runners {
		if i >= len(c.perceive) || !c.perceive[i] || sim.runners[i].Motion != MotionInertial {
			continue
		}
		if sim.clock < sim.runners[i].ReactAtMS {
			continue
		}
		if c.firstPublish[i] < 0 {
			c.firstPublish[i] = sim.clock
		}
		sim.runners[i].IntentX, sim.runners[i].IntentY = b.X, b.Y
	}
	c.publishes++
}

// t041Scene is one contact-containing fixture.
type t041Scene struct {
	name      string
	horizon   int
	perceive  []bool // which runners the cadence controller refreshes
	runners   func() []Runner
	setupBall func(*Sim)
}

func t041Scenes() []t041Scene {
	p := DefaultMotionParams()
	return []t041Scene{
		{
			name:     "moving-contact",
			horizon:  2500,
			perceive: []bool{true, true, true, true},
			runners: func() []Runner {
				return []Runner{
					PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
					PursuingRunner(0.50, 0.50+0.8/PitchWidthM, p, 0),
					PursuingRunner(0.50, 0.44, p, 0),
					PursuingRunner(0.45, 0.50, p, 0),
				}
			},
			setupBall: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
			},
		},
		{
			name:     "crossing-lane",
			horizon:  5000,
			perceive: []bool{true, true},
			runners: func() []Runner {
				// Two flank pursuers cross the ball's lane from opposite sides.
				// The ball is slower than the runners' top speed so a genuine
				// crossing interception happens (not an outrun).
				return []Runner{
					PursuingRunner(0.50, 0.38, p, 0),
					PursuingRunner(0.58, 0.64, p, 0),
				}
			},
			setupBall: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 5.0})
			},
		},
		{
			name:     "reaction-delay",
			horizon:  4000,
			perceive: []bool{true},
			runners: func() []Runner {
				return []Runner{PursuingRunner(0.60, 0.50, p, 300)}
			},
			setupBall: func(sim *Sim) {
				sim.ReleasePass(PassParams{StartX: 0.58, StartY: 0.50, TargetX: 0.72, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
			},
		},
		{
			name:     "stationary-recovery",
			horizon:  3000,
			perceive: []bool{true, true},
			runners: func() []Runner {
				return []Runner{
					PursuingRunner(0.60-5.0/PitchLengthM, 0.50, p, 0),
					PursuingRunner(0.60-7.0/PitchLengthM, 0.50+2.0/PitchWidthM, p, 0),
				}
			},
			setupBall: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
			},
		},
		{
			name:     "multi-deflection",
			horizon:  4000,
			perceive: []bool{true, true},
			runners: func() []Runner {
				return []Runner{
					PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
					PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
				}
			},
			setupBall: func(sim *Sim) {
				sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
			},
		},
		{
			name:     "held-control",
			horizon:  3000,
			perceive: []bool{true, true},
			runners: func() []Runner {
				return []Runner{
					PursuingRunner(0.62, 0.50, p, 0),
					PursuingRunner(0.78, 0.42, p, 0),
				}
			},
			setupBall: func(sim *Sim) {
				sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
			},
		},
		{
			name:     "boundary-contact",
			horizon:  2500,
			perceive: []bool{true},
			runners: func() []Runner {
				return []Runner{PursuingRunner(0.90, 0.50, p, 0)}
			},
			setupBall: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallRolling, X: 0.80, Y: 0.50, VXM: 6.0})
			},
		},
	}
}

// t041Capture runs one scene. When native is true the engine's own per-substep
// pursuit publication drives movement (Pursue=true for the scene's perceivers);
// otherwise the external cadence controller drives it (Pursue=false). The cadence
// period is ignored when native.
func t041Capture(t *testing.T, sc t041Scene, period int, mode t041CadenceMode, native bool) (*liveTrajectory, *t041Controller) {
	t.Helper()
	setup := func(sim *Sim) {
		rs := sc.runners()
		for i := range rs {
			rs[i].Pursue = native && sc.perceive[i]
		}
		sim.SetRunners(rs)
		sc.setupBall(sim)
	}
	var ctrl *t041Controller
	var pre func(*Sim)
	if !native {
		ctrl = t041NewController(period, mode, sc.perceive)
		pre = ctrl.beforeStep
	}
	tr := captureLiveTrajWith(t, LivePassConfig(), 1, setup, sc.horizon, pre)
	return tr, ctrl
}

// TestT041CadenceReferenceMatchesNative anchors the whole study: a 20 ms cadence
// of one substep, driven through the external controller, is bit-identical to the
// engine's own per-substep pursuit publication on every scene. It also proves
// same-seed reproducibility of the harness. If this fails, the cadence layer is
// not faithful and no cadence conclusion is valid.
func TestT041CadenceReferenceMatchesNative(t *testing.T) {
	for _, sc := range t041Scenes() {
		nat, _ := t041Capture(t, sc, 0, t041Periodic, true)
		ref, _ := t041Capture(t, sc, 1, t041Periodic, false)
		assertSameTrajectory(t, nat, ref)
		ref2, _ := t041Capture(t, sc, 1, t041Periodic, false)
		assertSameTrajectory(t, ref, ref2)
		t.Logf("scene=%s: cadence-20ms controller == engine per-substep publish (bit-identical, reproducible)", sc.name)
	}
}

// t041Verdict is the classification of one scene/cadence/mode comparison.
type t041Verdict struct {
	cadenceMS        int
	mode             string
	firstKindMatch   bool
	firstRunnerMatch bool
	firstEvent       string
	firstTimeErrMS   float64
	refEvents        int
	candEvents       int
	orderDivergent   int
	eventTimeErrMS   float64
	eventBallErrM    float64
	ballMax          float64
	playerMax        float64
	prefixPlayerMax  float64
	modeMismatch     int
	ownerMismatch    int
	terminalMatch    bool
	maxErrClock      int
	maxErrPhase      string
	safe             bool
	failures         []string
}

// t041MaxPlayerErrClock returns the largest aligned per-runner position error and
// the clock at which it occurred.
func t041MaxPlayerErrClock(ref, cand *liveTrajectory) (float64, int) {
	maxErr, clock := 0.0, -1
	for _, cf := range cand.frames {
		if cf.clock == 0 {
			continue
		}
		rf, ok := frameAtClock(ref, cf.clock)
		if !ok {
			continue
		}
		for i := range cf.xs {
			if d := DistM(cf.xs[i], cf.ys[i], rf.xs[i], rf.ys[i]); d > maxErr {
				maxErr, clock = d, cf.clock
			}
		}
	}
	return maxErr, clock
}

// t041ErrPhase labels a clock as pre-event or post-event relative to the
// reference's first state-changing event, so a large post-contact stale-intent
// divergence is not read as a broken integrator.
func t041ErrPhase(ref *liveTrajectory, clock int) string {
	if clock < 0 {
		return "none"
	}
	first := math.Inf(1)
	for _, e := range ref.events {
		if t041StateChanging(e.Kind) {
			if et := eventExactTimeMS(e, ref.dt); et < first {
				first = et
			}
		}
	}
	if math.IsInf(first, 1) {
		return "no-event"
	}
	if float64(clock) <= first {
		return "pre-event"
	}
	return "post-event"
}

func t041Classify(sc t041Scene, n int, mode t041CadenceMode, ref, cand *liveTrajectory) t041Verdict {
	v := t041Verdict{cadenceMS: n * SubstepMS, mode: mode.String(), maxErrClock: -1}
	refFirst, refOK := firstLiveEvent(ref)
	candFirst, candOK := firstLiveEvent(cand)
	switch {
	case refOK && candOK:
		v.firstEvent = refFirst.Kind.String() + "/" + candFirst.Kind.String()
		v.firstKindMatch = refFirst.Kind == candFirst.Kind
		v.firstRunnerMatch = refFirst.Runner == candFirst.Runner
		v.firstTimeErrMS = math.Abs(eventExactTimeMS(candFirst, cand.dt) - eventExactTimeMS(refFirst, ref.dt))
	case refOK == candOK:
		v.firstEvent = "none/none"
		v.firstKindMatch = true
		v.firstRunnerMatch = true
	default:
		v.firstEvent = "event-presence-mismatch"
	}
	seq := compareEventSequences(ref, cand)
	v.refEvents, v.candEvents, v.orderDivergent = seq.refCount, seq.candCount, seq.divergent
	v.eventTimeErrMS, v.eventBallErrM = seq.maxTimeErrMS, seq.maxBallErrM
	full := alignedFullRun(ref, cand)
	v.ballMax, v.playerMax = full.ballMax, full.playerMax
	v.modeMismatch, v.ownerMismatch = full.modeMismatch, full.ownerMismatch
	_, _, v.prefixPlayerMax, _, _, _ = alignedPrefixError(ref, cand)
	rLast := ref.frames[len(ref.frames)-1]
	cLast := cand.frames[len(cand.frames)-1]
	v.terminalMatch = rLast.mode == cLast.mode && (rLast.mode != BallHeld || rLast.owner == cLast.owner)
	_, v.maxErrClock = t041MaxPlayerErrClock(ref, cand)
	v.maxErrPhase = t041ErrPhase(ref, v.maxErrClock)

	if !v.firstKindMatch {
		v.failures = append(v.failures, "first-event-class")
	}
	if !v.firstRunnerMatch {
		v.failures = append(v.failures, "first-event-runner")
	}
	if v.orderDivergent != 0 {
		v.failures = append(v.failures, "event-order")
	}
	if !v.terminalMatch {
		v.failures = append(v.failures, "terminal-mode-owner")
	}
	if v.ballMax > t041FullRunBallMaxM {
		v.failures = append(v.failures, "ball-envelope")
	}
	if v.playerMax > t041FullRunPlayerMaxM {
		v.failures = append(v.failures, "player-envelope")
	}
	v.safe = len(v.failures) == 0
	return v
}

// TestT041MultiSceneCadence is the core comparison: 20/40/100/200 ms intent
// publication (periodic and event-triggered) against the 20 ms publication
// reference on every contact-containing scene, all at the identical 20 ms
// integration. Each candidate is captured twice (reproducibility); reaction and
// cooldown gates are asserted never bypassed; the causal classification is
// reported. Unsafe cadences are labelled, not hidden.
func TestT041MultiSceneCadence(t *testing.T) {
	for _, sc := range t041Scenes() {
		ref, _ := t041Capture(t, sc, 1, t041Periodic, false)
		refFirst, refOK := firstLiveEvent(ref)
		if !refOK {
			t.Fatalf("scene %s reference (20ms cadence) contains no contact; T-041 scenes must be contact-containing", sc.name)
		}
		refTime, refKind, refRunner := liveEventFields(refFirst, refOK, ref.dt)
		t.Logf("scene=%s | REFERENCE cadence=20ms first=%s@%.2fms r%d events=%d terminal=%s",
			sc.name, refKind, refTime, refRunner, len(contactEventRows(ref)), ref.ballMode)

		for _, n := range []int{2, 5, 10} { // 40 / 100 / 200 ms
			for _, mode := range []t041CadenceMode{t041Periodic, t041EventTriggered} {
				cand, ctrl := t041Capture(t, sc, n, mode, false)
				repro, _ := t041Capture(t, sc, n, mode, false)
				assertSameTrajectory(t, cand, repro)

				v := t041Classify(sc, n, mode, ref, cand)
				t041AssertReactionHonoured(t, sc, ctrl, cand)
				t041AssertCooldownHonoured(t, sc, cand)

				t.Logf("  cadence=%3dms mode=%-8s PREFIXplayer=%.4fm | FULL ball=%.4fm player=%.4fm modeMis=%d ownerMis=%d maxErr@%dms(%s) | events ref=%d cand=%d orderDiv=%d maxTimeErr=%.2fms maxBallErr=%.4fm firstTimeErr=%.2fms | first=%s classMatch=%v runnerMatch=%v terminal=%v SAFE=%v %v",
					v.cadenceMS, v.mode, v.prefixPlayerMax, v.ballMax, v.playerMax, v.modeMismatch, v.ownerMismatch,
					v.maxErrClock, v.maxErrPhase, v.refEvents, v.candEvents, v.orderDivergent, v.eventTimeErrMS, v.eventBallErrM, v.firstTimeErrMS,
					v.firstEvent, v.firstKindMatch, v.firstRunnerMatch, v.terminalMatch, v.safe, v.failures)
			}
		}
	}
}

// t041AssertReactionHonoured proves the cadence never bypasses a reaction: no
// perceiver is published an intent before its reaction deadline, and no runner
// touches the ball before it.
func t041AssertReactionHonoured(t *testing.T, sc t041Scene, ctrl *t041Controller, tr *liveTrajectory) {
	t.Helper()
	if ctrl == nil {
		return
	}
	for i, react := range ctrl.reactAt {
		if react <= 0 {
			continue
		}
		if fp := ctrl.firstPublish[i]; fp >= 0 && fp < react {
			t.Fatalf("scene %s: runner %d published intent at %dms before its %dms reaction", sc.name, i, fp, react)
		}
	}
	for _, e := range tr.events {
		if e.Kind != PassEventControl && e.Kind != PassEventDeflect {
			continue
		}
		if e.Runner < 0 || e.Runner >= len(ctrl.reactAt) {
			continue
		}
		if react := ctrl.reactAt[e.Runner]; react > 0 {
			if ex := eventExactTimeMS(e, tr.dt); ex < float64(react)-0.6 {
				t.Fatalf("scene %s: runner %d touched at %.2fms before its %dms reaction", sc.name, e.Runner, ex, react)
			}
		}
	}
}

// t041AssertCooldownHonoured proves a runner's re-control of a deflected ball
// still respects DeflectionCooldownMS under every cadence (the swept resolver
// gates it, not the scheduler).
func t041AssertCooldownHonoured(t *testing.T, sc t041Scene, tr *liveTrajectory) {
	t.Helper()
	last := map[int]float64{}
	for _, e := range tr.events {
		if e.Kind != PassEventDeflect && e.Kind != PassEventControl {
			continue
		}
		if e.Runner < 0 {
			continue
		}
		ex := eventExactTimeMS(e, tr.dt)
		if prev, ok := last[e.Runner]; ok {
			if gap := ex - prev; gap < DeflectionCooldownMS-0.6 {
				t.Fatalf("scene %s: runner %d re-touched after %.2fms (< cooldown %dms)", sc.name, e.Runner, gap, DeflectionCooldownMS)
			}
		}
		last[e.Runner] = ex
	}
}

// TestT041PostContactStalePublication classifies the cadence error MECHANISM on
// the multi-deflection scene: it reports the pre-event vs post-event split and
// proves event-triggered invalidation adds publications on top of the periodic
// cadence (its strict superset) without moving any player on the scheduler's
// behalf. The post-event total is inflated by stale intent, not skipped movement.
func TestT041PostContactStalePublication(t *testing.T) {
	var sc t041Scene
	for _, s := range t041Scenes() {
		if s.name == "multi-deflection" {
			sc = s
		}
	}
	if sc.name == "" {
		t.Fatal("multi-deflection scene not found")
	}
	ref, _ := t041Capture(t, sc, 1, t041Periodic, false)
	first, ok := firstLiveEvent(ref)
	if !ok {
		t.Fatal("reference multi-deflection has no contact")
	}
	firstMS := eventExactTimeMS(first, ref.dt)
	for _, n := range []int{2, 5, 10} {
		periodic, pctrl := t041Capture(t, sc, n, t041Periodic, false)
		event, ectrl := t041Capture(t, sc, n, t041EventTriggered, false)
		_, _, prefixPlayer, _, _, _ := alignedPrefixError(ref, periodic)
		maxErr, clock := t041MaxPlayerErrClock(ref, periodic)
		phase := t041ErrPhase(ref, clock)
		t.Logf("multi-deflection cadence=%3dms: pre-event player max=%.4fm | overall player max=%.4fm @%dms phase=%s | publishes periodic=%d event=%d (first ref event @%.2fms)",
			n*SubstepMS, prefixPlayer, maxErr, clock, phase, pctrl.publishes, ectrl.publishes, firstMS)
		if ectrl.publishes < pctrl.publishes {
			t.Fatalf("event-triggered publishes %d < periodic %d at cadence=%dms", ectrl.publishes, pctrl.publishes, n*SubstepMS)
		}
		assertSameTrajectory(t, event, mustCaptureAgain(t, sc, n, t041EventTriggered))
	}
}

func mustCaptureAgain(t *testing.T, sc t041Scene, n int, mode t041CadenceMode) *liveTrajectory {
	t.Helper()
	tr, _ := t041Capture(t, sc, n, mode, false)
	return tr
}

// TestT041CadenceIntegratesEveryPlayerEveryStep proves the cadence never changes
// integration. Part 1: with fixed intents that the cadence never refreshes, every
// cadence overlay is bit-identical to the 20 ms reference. Part 2: at the
// coarsest cadence every one of 22 bodies integrates every substep (no frozen
// frame, no skipped movement).
func TestT041CadenceIntegratesEveryPlayerEveryStep(t *testing.T) {
	sc := t041Scene{
		name:      "fixed-intent-22",
		horizon:   2000,
		perceive:  make([]bool, 22), // no perceivers: the cadence never republishes
		runners:   func() []Runner { return inertial22() },
		setupBall: func(sim *Sim) { sim.SetBall(Ball{Mode: BallDead, X: 0.10, Y: 0.50}) },
	}
	base, _ := t041Capture(t, sc, 1, t041Periodic, false)
	for _, n := range []int{2, 5, 10} {
		for _, mode := range []t041CadenceMode{t041Periodic, t041EventTriggered} {
			cand, _ := t041Capture(t, sc, n, mode, false)
			assertSameTrajectory(t, base, cand)
		}
	}
	t.Log("fixed intents: 20/40/100/200ms cadence overlays are bit-identical (integration runs every substep; cadence only refreshes intents)")

	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(inertial22())
	ctrl := t041NewController(10, t041Periodic, make([]bool, 22))
	prev := sim.Runners()
	steps := 0
	for sim.ClockMS() < 2000 {
		ctrl.beforeStep(sim)
		sim.StepQuiet()
		cur := sim.Runners()
		for i := range cur {
			if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
				t.Fatalf("cadence-10 substep %d: runner %d did not move", steps, i)
			}
		}
		prev = cur
		steps++
	}
	want := 2000 / SubstepMS
	if steps != want {
		t.Fatalf("cadence-10 ran %d substeps, want %d", steps, want)
	}
	t.Logf("cadence-10 runtime guard: %d/%d substeps integrated all 22 bodies", steps, want)
}

// TestT041ExternalCadenceStateIsHarnessOnly documents that the studied cadence is
// EXTERNAL harness state: the Sim Snapshot gains no scheduling field and the
// optional JSON round-trip below is explicitly NOT a Sim.Resume contract — a real
// cadence model would need a new persisted scheduling field and its own Resume
// fixture (the T-042 recommendation).
func TestT041ExternalCadenceStateIsHarnessOnly(t *testing.T) {
	sc := t041Scenes()[0]
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(sc.runners())
	sc.setupBall(sim)
	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	for _, bad := range []string{"cadence", "intentCadence", "nextDecisionMs", "publishEvery", "lastEventCount", "schedulingPeriod"} {
		if _, ok := m[bad]; ok {
			t.Fatalf("unexpected persisted scheduling field %q in Sim.Snapshot", bad)
		}
	}

	state := t041CadenceState{Step: 42, LastEventCount: 3, Period: 5, Mode: t041EventTriggered}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal cadence state: %v", err)
	}
	var back t041CadenceState
	if err := json.Unmarshal(stateRaw, &back); err != nil {
		t.Fatalf("unmarshal cadence state: %v", err)
	}
	if back != state {
		t.Fatalf("external cadence state did not round-trip: %+v != %+v", back, state)
	}
	t.Log("Sim.Snapshot has no scheduling field; the external cadence state round-trips but is harness-only (not Sim.Resume)")
}
