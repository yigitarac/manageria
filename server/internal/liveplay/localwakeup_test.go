package liveplay

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// T-046 opt-in per-runner local event wakeup tests (isolated prototype evidence).
//
// The persisted ordering/snapshot contract is fixed in the session evidence
// BEFORE this code (vault/07-Session-Logs/2026-10-12-t046-local-wakeup-evidence.md).
// These tests exercise the runtime wiring: event-time locality, per-runner gating
// of the local sample/local intent layer, fail-closed invalid opt-ins, persistence
// validation and complete JSON Resume. Existing T-040–T-045 fixtures are untouched.

// --- helpers ---------------------------------------------------------------

// t046JSONResume marshals a Sim snapshot, unmarshals it and resumes.
func t046JSONResume(t *testing.T, sim *Sim) *Sim {
	t.Helper()
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(blob, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	rs, err := Resume(snap)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	return rs
}

// t046SamplesTail returns the straight run's sample suffix aligned to resumed.
func t046SamplesTail(straight, resumed *Sim) []Sample {
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 {
		return nil
	}
	return all[head:]
}

// t046AssertRuntimeTailsMid steps straight and resumed in lockstep, applying the
// same mid hook (between-step dynamics such as a second release) to BOTH before
// each step, and asserting the COMPLETE serializable Sim snapshot (ball, runners
// incl. flags/pending, contact/touch records, event log+seq, immunity, RNG,
// scheduler fields) is identical at every step, plus the derived sample tail.
func t046AssertRuntimeTailsMid(t *testing.T, straight, resumed *Sim, horizon int, label string, mid func(*Sim, int)) {
	t.Helper()
	for straight.ClockMS() < horizon {
		if resumed.ClockMS() != straight.ClockMS() {
			t.Fatalf("%s: clocks diverged before step %d vs %d", label, straight.ClockMS(), resumed.ClockMS())
		}
		if mid != nil {
			mid(straight, straight.ClockMS())
			mid(resumed, resumed.ClockMS())
		}
		straight.Step()
		resumed.Step()
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("%s @%d: complete snapshot diverged after resume", label, straight.ClockMS())
		}
	}
	if !reflect.DeepEqual(t046SamplesTail(straight, resumed), resumed.Samples()) {
		t.Fatalf("%s: resumed sample tail differs from the straight run", label)
	}
}

// t046AssertRuntimeTails is the no-mid variant.
func t046AssertRuntimeTails(t *testing.T, straight, resumed *Sim, horizon int, label string) {
	t.Helper()
	t046AssertRuntimeTailsMid(t, straight, resumed, horizon, label, nil)
}

// --- 1. event-time locality geometry ---------------------------------------

// t046GeometrySim builds a one-runner sim whose captured PRE-step position is
// preX; the caller then sees its END-step position mutated to endX, so the two
// differ (a moving runner).
func t046GeometrySim(t *testing.T, radius, preX, endX float64) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := InertialRunner(preX, 0.5, preX, 0.5, DefaultMotionParams())
	if err := r.EnableLocalWakeup(radius); err != nil {
		t.Fatalf("EnableLocalWakeup: %v", err)
	}
	sim.SetRunners([]Runner{r})
	sim.capturePreStep()
	sim.runners[0].X = endX
	return sim
}

// TestT046EventTimeLocalityGeometry pins the event-instant rule: a RELEASE is a
// between-step event and uses the runner's CURRENT position; every in-step event
// (control/deflect/stop/out) interpolates at the event fraction, so a contact at
// fraction 0 uses the PRE-step position and a fraction-1 event uses the END-step
// position.
func TestT046EventTimeLocalityGeometry(t *testing.T) {
	sim := t046GeometrySim(t, 0.03, 0.5, 0.5005) // end-pre = 0.0525 m; radius 0.03
	preX, endX := 0.5, 0.5005
	check := func(kind PassEventKind, bx, by, frac float64, want bool) {
		sim.runners[0].WakePending = false
		sim.noteLocalWake(kind, bx, by, frac)
		if got := sim.runners[0].WakePending; got != want {
			t.Fatalf("%s frac=%.2f ball=(%.4f,%.2f): pending=%v want %v", kind, frac, bx, by, got, want)
		}
	}
	check(PassEventControl, preX, 0.5, 0, true)
	check(PassEventControl, endX, 0.5, 0, false)
	check(PassEventControl, endX, 0.5, 1, true)
	check(PassEventControl, preX, 0.5, 1, false)
	check(PassEventRelease, endX, 0.5, 0, true)
	check(PassEventRelease, preX, 0.5, 0, false)
	check(PassEventOut, endX, 0.5, 1, true)
	check(PassEventStop, preX, 0.5, 1, false)
	check(PassEventPath, preX, 0.5, 0, false)
	check(PassEventPath, endX, 0.5, 0, false)
	t.Logf("event-time locality: control frac0 uses PRE; control/stop/out frac1 uses END; release uses CURRENT; path inert")
}

// TestT046InteriorFractionalLocality proves an INTERIOR fractional event uses the
// interpolated runner position, distinguishing it from both the pre and end
// positions.
func TestT046InteriorFractionalLocality(t *testing.T) {
	sim := t046GeometrySim(t, 0.10, 0.5, 0.504) // end-pre = 0.42 m
	check := func(frac float64, want bool) {
		sim.runners[0].WakePending = false
		sim.noteLocalWake(PassEventDeflect, 0.502, 0.5, frac)
		if got := sim.runners[0].WakePending; got != want {
			t.Fatalf("frac=%.2f: pending=%v want %v", frac, got, want)
		}
	}
	check(0, false)
	check(1, false)
	check(0.5, true)
	t.Logf("interior frac 0.5 interpolated position is local while frac 0 and frac 1 are remote")
}

// TestT046MovingFractionZeroContactLocal is the physics-level regression: a real
// fraction-0 CONTROL on a MOVING runner must be classified local at the pre-step
// position, even though the runner's end-step position is outside the radius.
func TestT046MovingFractionZeroContactLocal(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
	r.VXM = 8.5
	if err := r.EnableLocalWakeup(0.03); err != nil {
		t.Fatalf("EnableLocalWakeup: %v", err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.5, Y: 0.5})
	sim.Step()
	var ctrl PassEvent
	found := false
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl {
			ctrl, found = e, true
			break
		}
	}
	if !found {
		t.Fatalf("fixture produced no control event: %v", sim.Events())
	}
	if ctrl.Frac != 0 {
		t.Fatalf("control Frac=%v, want 0 (contact at the step start)", ctrl.Frac)
	}
	if !sim.runners[0].WakePending {
		t.Fatal("a fraction-0 contact on a moving runner was classified remote (end-step position used)")
	}
	t.Logf("moving fraction-0 control: pre-step contact is local (pending set), end-step position is outside radius")
}

// --- 2. enable / disable / persisted state ---------------------------------

// TestT046DisableAndEnablePendingState proves disabling clears BOTH the flag and
// the pending (so it can never leave an impossible state Resume rejects), that
// disabling keeps the sensing record, that re-enabling resets a stale pending, and
// that a pending without the flag is rejected on import.
func TestT046DisableAndEnablePendingState(t *testing.T) {
	r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	if err := r.EnableLocalWakeup(4.0); err != nil {
		t.Fatalf("EnableLocalWakeup: %v", err)
	}
	r.WakePending = true
	r.DisableLocalWakeup()
	if r.LocalWakeup || r.WakePending {
		t.Fatalf("disable left flag=%v pending=%v", r.LocalWakeup, r.WakePending)
	}
	if r.Perception == nil {
		t.Fatal("disable removed the sensing record")
	}
	base := func(r Runner, cfg Config) Snapshot {
		return Snapshot{Cfg: cfg, ClockMS: 40, RNG: [4]uint64{1, 2, 3, 4}, Runners: []Runner{r}}
	}
	if _, err := Resume(base(r, ScheduledLivePassConfig(100))); err != nil {
		t.Fatalf("disabled runner rejected by Resume: %v", err)
	}
	bad := r
	bad.WakePending = true
	if _, err := Resume(base(bad, ScheduledLivePassConfig(100))); !errors.Is(err, ErrLocalWakeupPending) {
		t.Fatalf("pending without flag: err=%v, want ErrLocalWakeupPending", err)
	}
	r.WakePending = true
	if err := r.EnableLocalWakeup(4.0); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if r.WakePending {
		t.Fatal("enable did not reset the stale pending")
	}
	t.Logf("disable clears flag+pending but keeps sensing; enable resets pending; pending-without-flag rejected")
}

// TestT046InvalidEnablingFailsClosed proves a raw LocalWakeup flag with an invalid
// (missing) enabling never runs the local layer AND never falls back to the shared
// native oracle — even with LocalPursuit and Pursue both set — and that a broken
// pending is cleared.
func TestT046InvalidEnablingFailsClosed(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := InertialRunner(0.5, 0.5, 0.95, 0.95, DefaultMotionParams())
	r.LocalPursuit = true
	r.Pursue = true
	r.LocalWakeup = true
	r.WakePending = true
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.9, Y: 0.5, VXM: 5.0})
	sim.Step()
	got := sim.runners[0]
	if got.IntentX != 0.95 || got.IntentY != 0.95 {
		t.Fatalf("invalid enabling leaked a target: intent=(%v,%v)", got.IntentX, got.IntentY)
	}
	if got.WakePending {
		t.Fatal("broken pending was not cleared")
	}
	if _, ok := sim.Perception(0); ok {
		t.Fatal("invalid enabling produced an observation")
	}
	if sim.LocalWakeupPublishMS(0) != -1 {
		t.Fatalf("invalid enabling scheduled locally at %d", sim.LocalWakeupPublishMS(0))
	}
	t.Logf("invalid raw opt-in fails closed: no oracle, no target, no sample, broken pending cleared")
}

// --- 3. remote release noninterference (mandatory first step) ---------------

// TestT046RemoteReleaseNoLocalWake ports T-045's mandatory scene to the runtime: a
// moving Unknown local runner with and without a remote off-lattice ReleasePass at
// 60 ms while the ball stays outside its radius. The local record, the observed
// intent and the per-runner local scheduling clock are identical every step, and
// no local wakeup is produced.
func TestT046RemoteReleaseNoLocalWake(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(sim *Sim) {
		r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
		r.VXM = 5.0
		r.ReachM = 0.05
		if err := r.EnableLocalPursuit(2.0); err != nil {
			panic(err)
		}
		if err := r.EnableLocalWakeup(2.0); err != nil {
			panic(err)
		}
		sim.SetRunners([]Runner{r})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.9})
	}
	with := mustSim(t, cfg, 1)
	without := mustSim(t, cfg, 1)
	build(with)
	build(without)
	for with.ClockMS() < 300 {
		if with.ClockMS() == 60 {
			with.ReleasePass(PassParams{StartX: 0.05, StartY: 0.5, TargetX: 0.30, TargetY: 0.5, SpeedMPS: 12.0, Passer: -1})
		}
		with.Step()
		without.Step()
		ow, _ := with.Perception(0)
		owo, _ := without.Perception(0)
		if ow != owo {
			t.Fatalf("@%d: observation differs with/without remote release: %+v vs %+v", with.ClockMS(), ow, owo)
		}
		if with.runners[0].IntentX != without.runners[0].IntentX || with.runners[0].IntentY != without.runners[0].IntentY {
			t.Fatalf("@%d: intent differs with/without remote release", with.ClockMS())
		}
		if with.runners[0].WakePending || without.runners[0].WakePending {
			t.Fatalf("@%d: a remote release set a local pending", with.ClockMS())
		}
		if with.LocalWakeupPublishMS(0) != without.LocalWakeupPublishMS(0) {
			t.Fatalf("@%d: local scheduling clocks differ (%d vs %d)", with.ClockMS(), with.LocalWakeupPublishMS(0), without.LocalWakeupPublishMS(0))
		}
	}
	t.Logf("remote release @60ms: record, intent and per-runner scheduling clocks identical every step; no local wake")
}

// --- 4. near/far union rejection + reaction drop ---------------------------

// TestT046NearFarPerRunnerIsolation ports T-045's near/far union rejection: a
// release visible ONLY to the near runner wakes only the near runner.
func TestT046NearFarPerRunnerIsolation(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	near := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	if err := near.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	far := InertialRunner(0.3, 0.5, 0.3, 0.5, DefaultMotionParams())
	if err := far.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{near, far})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.9})
	for sim.ClockMS() < 60 {
		sim.Step()
	}
	sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.52, TargetX: 0.52, TargetY: 0.52, SpeedMPS: 2.0, Passer: -1})
	if !containsInt(t042EventClocks(sim.Events(), PassEventRelease), 60) {
		t.Fatal("release fixture did not emit a release at 60")
	}
	sim.Step()
	if sim.LocalWakeupPublishMS(0) != 60 {
		t.Fatalf("near local scheduling at %d, want 60", sim.LocalWakeupPublishMS(0))
	}
	if sim.LocalWakeupPublishMS(1) != 0 {
		t.Fatalf("far local scheduling at %d, want 0 (lattice only)", sim.LocalWakeupPublishMS(1))
	}
	if sim.runners[1].WakePending {
		t.Fatal("far runner was woken by a near-only event")
	}
	if sim.runners[0].WakePending {
		t.Fatal("near pending was not consumed at its publication")
	}
	t.Logf("near/far (sensing-only): near scheduled @60, far @0 only")
}

// TestT046NearFarLocalPursuitVariant adds MOVING local-pursuit watchers: the
// near-only event wakes the near runner (target updates to the observed ball) and
// does NOT wake the far runner (target unchanged). The near runner's per-step
// position/intent/observation history is identical to a shared-mode run of the same
// scene (the shared oracle), proving the far runner's opt-in does not perturb the
// near runner's decisions.
func TestT046NearFarLocalPursuitVariant(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	scene := func(sim *Sim, perRunner bool) {
		near := InertialRunner(0.5, 0.5, 0.95, 0.50, DefaultMotionParams())
		near.VXM = 5.0
		near.ReachM = 0.05
		if err := near.EnableLocalPursuit(2.0); err != nil {
			panic(err)
		}
		if perRunner {
			if err := near.EnableLocalWakeup(2.0); err != nil {
				panic(err)
			}
		}
		far := InertialRunner(0.3, 0.5, 0.95, 0.50, DefaultMotionParams())
		far.VXM = 5.0
		far.ReachM = 0.05
		if err := far.EnableLocalPursuit(2.0); err != nil {
			panic(err)
		}
		if perRunner {
			if err := far.EnableLocalWakeup(2.0); err != nil {
				panic(err)
			}
		}
		sim.SetRunners([]Runner{near, far})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.9})
	}
	mid := func(s *Sim, clock int) {
		if clock == 60 {
			s.ReleasePass(PassParams{StartX: 0.51, StartY: 0.52, TargetX: 0.52, TargetY: 0.52, SpeedMPS: 1.0, Passer: -1})
		}
	}

	per := mustSim(t, cfg, 1)
	scene(per, true)
	lastLattice := -1
	for per.ClockMS() < 300 {
		start := per.ClockMS()
		if start%cfg.IntentCadenceMS == 0 {
			lastLattice = start
		}
		if start == 60 {
			mid(per, 60)
			if per.runners[1].WakePending {
				t.Fatal("far local-pursuit runner got a pending immediately after the near-only release")
			}
		}
		per.Step()
		// The far runner must be scheduled ONLY on the periodic lattice, every step.
		if per.LocalWakeupPublishMS(1) != lastLattice {
			t.Fatalf("@%d: far local timestamp %d != periodic lattice %d", per.ClockMS(), per.LocalWakeupPublishMS(1), lastLattice)
		}
		if start == 60 {
			// Immediately after the pre-step-60 Step (ending at clock 80): the near
			// runner was scheduled at the visible event, the far runner was not.
			if per.LocalWakeupPublishMS(0) != 60 {
				t.Fatalf("near local-pursuit runner not scheduled at 60: %d", per.LocalWakeupPublishMS(0))
			}
			if per.LocalWakeupPublishMS(1) != 0 {
				t.Fatalf("far local-pursuit runner scheduled at the near-only event 60: %d", per.LocalWakeupPublishMS(1))
			}
			if per.runners[0].WakePending {
				t.Fatal("near pending was not consumed at its 60ms publication")
			}
		}
	}

	// Near runner's full per-step history equals the shared oracle's near history.
	perN := mustSim(t, cfg, 1)
	scene(perN, true)
	sharedN := mustSim(t, cfg, 1)
	scene(sharedN, false)
	for perN.ClockMS() < 300 {
		if perN.ClockMS() == 60 {
			mid(perN, 60)
			mid(sharedN, 60)
		}
		perN.Step()
		sharedN.Step()
		rn, rs := perN.runners[0], sharedN.runners[0]
		if rn.X != rs.X || rn.Y != rs.Y || rn.IntentX != rs.IntentX || rn.IntentY != rs.IntentY {
			t.Fatalf("@%d: near runner history differs from the shared oracle: per=(%.6f,%.6f,%.6f) shared=(%.6f,%.6f,%.6f)",
				perN.ClockMS(), rn.X, rn.Y, rn.IntentX, rs.X, rs.Y, rs.IntentX)
		}
		on, _ := perN.Perception(0)
		os, _ := sharedN.Perception(0)
		if on != os {
			t.Fatalf("@%d: near observation differs from the shared oracle", perN.ClockMS())
		}
	}
	t.Logf("near/far local-pursuit: far never scheduled off-lattice; near history identical to the shared oracle")
}

// TestT046PendingDroppedBeforeReaction proves a LOCALLY visible event before a
// runner's reaction deadline produces a local SCHEDULING ATTEMPT but the reaction
// gate drops the sample AND the local intent; the pending is consumed regardless,
// and the first decision AFTER the reaction executes.
func TestT046PendingDroppedBeforeReaction(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams()) // placeholder = own pos
	r.ReactMS = 300
	r.ReachM = 0.05
	if err := r.EnableLocalPursuit(2.0); err != nil {
		t.Fatal(err)
	}
	if err := r.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.9})
	for sim.ClockMS() < 60 {
		sim.Step()
	}
	// Visible release just inside the circle, slow so it stays in range through the
	// reaction deadline (ReactAtMS := 60 + 300 = 360; first lattice after is 400).
	sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.505, TargetX: 0.52, TargetY: 0.505, SpeedMPS: 0.5, Passer: -1})
	if !sim.runners[0].WakePending {
		t.Fatal("visible release set no pending")
	}
	sim.Step() // pre-step 60
	if sim.runners[0].WakePending {
		t.Fatal("pending was not consumed (drop-before-reaction)")
	}
	if sim.LocalWakeupPublishMS(0) != 60 {
		t.Fatalf("scheduling attempt at %d, want 60", sim.LocalWakeupPublishMS(0))
	}
	if o, _ := sim.Perception(0); o.Seen {
		t.Fatalf("runner sensed before its reaction: %+v", o)
	}
	for sim.ClockMS() < 400 { // through lattice 300 (300 < 360 => dropped)
		sim.Step()
		if sim.runners[0].IntentX != 0.5 || sim.runners[0].IntentY != 0.5 {
			t.Fatalf("@%d: intent changed before the reaction: (%v,%v)", sim.ClockMS(), sim.runners[0].IntentX, sim.runners[0].IntentY)
		}
	}
	sim.Step() // pre-step 400: first decision after the reaction
	if sim.runners[0].IntentX == 0.5 && sim.runners[0].IntentY == 0.5 {
		t.Fatal("intent did not update at the first decision after the reaction (400)")
	}
	t.Logf("visible event before reaction: attempt @60 dropped (no sample, no intent change) until the 400 decision")
}

// --- 5. coalescing, stop/out, stale second release -------------------------

// TestT046CoalescedDeflectOutSingleWake reuses the proven T-044 coalescing fixture.
func TestT046CoalescedDeflectOutSingleWake(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	w := InertialRunner(0.93, 0.5, 0.95, 0.95, DefaultMotionParams())
	if err := w.EnableLocalPursuit(20.0); err != nil {
		t.Fatal(err)
	}
	if err := w.EnableLocalWakeup(20.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{w, {X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
	sim.controlError = 1.0
	sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
	sim.Step()
	if countKind(sim, PassEventDeflect) != 1 || countKind(sim, PassEventOut) != 1 {
		t.Fatalf("coalesced fixture produced def=%d out=%d, want 1/1", countKind(sim, PassEventDeflect), countKind(sim, PassEventOut))
	}
	if !sim.runners[0].WakePending {
		t.Fatal("coalesced events left no pending")
	}
	base := sim.LocalWakeupPublishCount()
	sim.Step()
	if got := sim.LocalWakeupPublishCount() - base; got != 1 {
		t.Fatalf("coalesced events caused %d local scheduling attempts, want exactly 1", got)
	}
	if sim.LocalWakeupPublishMS(0) != 20 {
		t.Fatalf("coalesced local scheduling at %d, want 20", sim.LocalWakeupPublishMS(0))
	}
	if o, _ := sim.Perception(0); o.Mode != BallOut {
		t.Fatalf("watcher observed mode %s, want out", o.Mode)
	}
	t.Logf("coalesced deflect+out: exactly ONE local scheduling attempt at 20ms; watcher observed the Out")
}

// TestT046VisibleStopWakes proves a visible STOP sets a pending and produces one
// local scheduling attempt at the next pre-step.
func TestT046VisibleStopWakes(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	r.ReachM = 0.05
	if err := r.EnableLocalWakeup(30.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.51, Y: 0.70, VXM: 0.12})
	for sim.ClockMS() < 3000 && countKind(sim, PassEventStop) == 0 {
		sim.Step()
	}
	if countKind(sim, PassEventStop) == 0 {
		t.Fatal("fixture produced no stop event")
	}
	if !sim.runners[0].WakePending {
		t.Fatal("visible stop left no pending")
	}
	base := sim.LocalWakeupPublishCount()
	sim.Step()
	if got := sim.LocalWakeupPublishCount() - base; got != 1 {
		t.Fatalf("visible stop caused %d local scheduling attempts, want 1", got)
	}
	t.Logf("visible STOP: pending set, exactly one local scheduling attempt at the next pre-step")
}

// TestT046StaleRemoteSecondReleaseNoWake builds a REAL first ReleasePass (in range)
// and a REAL second off-lattice ReleasePass at 820 (remote). It asserts exactly two
// release events, that the second release sets NO pending immediately and adds NO
// local scheduling attempt, that the record stays stale (ball actually outside the
// radius at 820) and the prior intent is unchanged, and the tail is stable.
func TestT046StaleRemoteSecondReleaseNoWake(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
	r.VXM = 2.0
	r.ReachM = 0.05
	if err := r.EnableLocalPursuit(2.0); err != nil {
		t.Fatal(err)
	}
	if err := r.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{r})
	// REAL first release at 0, near the runner and in range.
	sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	for sim.ClockMS() < 820 {
		sim.Step()
	}
	if rel := t042EventClocks(sim.Events(), PassEventRelease); len(rel) != 1 || rel[0] != 0 {
		t.Fatalf("first-release fixture: releases=%v, want [0]", rel)
	}
	oBefore, _ := sim.Perception(0)
	if !oBefore.Seen || oBefore.Freshness(820) != ObservationStale {
		t.Fatalf("record not stale at 820: %+v", oBefore)
	}
	if d := DistM(sim.ball.X, sim.ball.Y, sim.runners[0].X, sim.runners[0].Y); d <= 2.0 {
		t.Fatalf("ball not actually outside the radius at 820: %.3f m", d)
	}
	pubBefore := sim.LocalWakeupPublishMS(0)
	countBefore := sim.LocalWakeupPublishCount()
	intentBefore := sim.runners[0].IntentX

	// REAL remote second release at the off-lattice clock 820.
	sim.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
	if sim.runners[0].WakePending {
		t.Fatal("remote second release set a pending immediately")
	}
	sim.Step() // pre-step 820
	if sim.runners[0].WakePending {
		t.Fatal("remote second release left a pending after the next pre-step")
	}
	if sim.LocalWakeupPublishMS(0) != pubBefore {
		t.Fatalf("remote release moved the local scheduling clock: %d -> %d", pubBefore, sim.LocalWakeupPublishMS(0))
	}
	if sim.LocalWakeupPublishCount() != countBefore {
		t.Fatalf("remote release added local scheduling attempts: %d -> %d", countBefore, sim.LocalWakeupPublishCount())
	}
	for sim.ClockMS() < 1000 {
		sim.Step()
	}
	if rel := t042EventClocks(sim.Events(), PassEventRelease); len(rel) != 2 || rel[1] != 820 {
		t.Fatalf("want exactly 2 releases incl 820, got %v", rel)
	}
	oAfter, _ := sim.Perception(0)
	if !reflect.DeepEqual(oBefore, oAfter) {
		t.Fatalf("remote release changed the stale observation:\n before=%+v\n after =%+v", oBefore, oAfter)
	}
	if sim.runners[0].IntentX != intentBefore {
		t.Fatalf("remote release changed the prior intent: %v -> %v", intentBefore, sim.runners[0].IntentX)
	}
	t.Logf("real first release @0; remote second @820: 2 releases, no pending, no attempt, stale record + intent unchanged")
}

// --- 6. sensing-only native mix (layers isolated) --------------------------

// TestT046NativePursueSharedWithLocalSensing proves the native publisher and the
// local sample are SEPARATE layers.
func TestT046NativePursueSharedWithLocalSensing(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	r := PursuingRunner(0.2, 0.5, DefaultMotionParams(), 0)
	if err := r.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.215, Y: 0.5, VXM: 2.0})
	sim.Step()
	if r := sim.runners[0]; r.IntentX != 0.215 {
		t.Fatalf("native intent at 0 = %v, want the true ball 0.215", r.IntentX)
	}
	if o, _ := sim.Perception(0); !o.Seen {
		t.Fatal("expected an in-range local sample at the lattice 0")
	}
	for sim.ClockMS() < 60 {
		sim.Step()
	}
	sim.ReleasePass(PassParams{StartX: 0.90, StartY: 0.5, TargetX: 0.95, TargetY: 0.5, SpeedMPS: 3.0, Passer: -1})
	sim.Step()
	if sim.runners[0].IntentX != 0.90 {
		t.Fatalf("native intent at 60 = %v, want the true released ball 0.90 (shared native layer)", sim.runners[0].IntentX)
	}
	if sim.runners[0].WakePending {
		t.Fatal("remote event set a local pending on the sensing-only local runner")
	}
	if o, _ := sim.Perception(0); o.ClockMS == 60 {
		t.Fatal("local sample ran on an invisible off-lattice remote event")
	}
	t.Logf("native Pursue stays shared (target 0.90 at the remote 60ms event) while the local sample stays lattice-gated")
}

// --- 7. pending-only publication must not disturb unflagged runners --------

// TestT046PendingOnlyDoesNotPublishUnflagged proves a pending-only publication
// (off-lattice, sharedDirty=false) serves ONLY the opted-in runner carrying the
// pending: an unflagged local-pursuit runner, a sensing-only runner and a native
// pursuer are all left untouched (shared semantics restricted to sharedDue).
func TestT046PendingOnlyDoesNotPublishUnflagged(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	// Off-lane bodies (y=0.2) with tiny reach and a ball on the y=0.5 lane: no
	// contact/out/stop can fire, so the scene is physically EVENT-FREE (no real
	// shared decisionDirty is generated).
	const lane = 0.2
	// runner 0: opted-in local wakeup (carries the pending).
	r0 := InertialRunner(0.5, lane, 0.5, lane, DefaultMotionParams())
	if err := r0.EnableLocalWakeup(200.0); err != nil {
		t.Fatal(err)
	}
	// runner 1: unflagged, MOVING unknown local-pursuit + sensing.
	r1 := InertialRunner(0.2, lane, 0.95, lane, DefaultMotionParams())
	r1.VXM = 3.0
	r1.ReachM = 0.05
	if err := r1.EnableLocalPursuit(200.0); err != nil {
		t.Fatal(err)
	}
	// runner 2: unflagged sensing-only.
	r2 := InertialRunner(0.3, lane, 0.3, lane, DefaultMotionParams())
	r2.ReachM = 0.05
	if err := r2.EnableSensing(200.0); err != nil {
		t.Fatal(err)
	}
	// runner 3: native pursuer.
	r3 := PursuingRunner(0.4, lane, DefaultMotionParams(), 0)
	r3.ReachM = 0.05
	sim.SetRunners([]Runner{r0, r1, r2, r3})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.6, Y: 0.5, VXM: 3.0})
	for sim.ClockMS() < 120 { // pre-steps 0..100 (lattice 0 and 100 both run)
		sim.Step()
	}
	if sim.NextDecisionMS() != 200 {
		t.Fatalf("next decision %d, want 200 (off-lattice at 120)", sim.NextDecisionMS())
	}
	if sim.decisionDirty {
		t.Fatal("the physically event-free setup generated a real shared decisionDirty before forcing the pending")
	}
	o1, _ := sim.Perception(1)
	o2, _ := sim.Perception(2)
	i1x, i1y := sim.runners[1].IntentX, sim.runners[1].IntentY
	i3x, i3y := sim.runners[3].IntentX, sim.runners[3].IntentY
	// Force the incoherent boundary: pending on the opted runner, dirty false.
	sim.runners[0].WakePending = true
	sim.decisionDirty = false
	// Complete JSON Resume of the pending-only snapshot BEFORE the 120 pre-step.
	resumed := t046JSONResume(t, sim)
	if !resumed.runners[0].WakePending {
		t.Fatal("JSON round-trip lost the pending-only flag")
	}
	sim.Step() // pre-step 120: pending-only publication
	resumed.Step()
	if sim.runners[0].WakePending {
		t.Fatal("opted-in pending was not consumed at 120")
	}
	if sim.LocalWakeupPublishMS(0) != 120 {
		t.Fatalf("opted-in runner not scheduled at 120: %d", sim.LocalWakeupPublishMS(0))
	}
	if o, _ := sim.Perception(1); o != o1 {
		t.Fatalf("unflagged local-pursuit observation changed on a pending-only publication: %+v vs %+v", o, o1)
	}
	if o, _ := sim.Perception(2); o != o2 {
		t.Fatalf("unflagged sensing observation changed on a pending-only publication: %+v vs %+v", o, o2)
	}
	if sim.runners[1].IntentX != i1x || sim.runners[1].IntentY != i1y {
		t.Fatal("unflagged local-pursuit intent changed on a pending-only publication")
	}
	if sim.runners[3].IntentX != i3x || sim.runners[3].IntentY != i3y {
		t.Fatal("native pursuer intent changed on a pending-only publication")
	}
	// The live fixture and the JSON-resumed fixture continue identically.
	t046AssertRuntimeTails(t, sim, resumed, 500, "pending-only")
	t.Logf("pending-only publication at 120 served only the opted runner; unflagged local/sensing/native untouched; JSON-resumed tail identical")
}

// --- 8. persistence & complete JSON Resume ---------------------------------

// TestT046DisabledKeysUnchanged proves a default runner serialises no new key.
func TestT046DisabledKeysUnchanged(t *testing.T) {
	r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
	blob, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal runner: %v", err)
	}
	if strings.Contains(string(blob), "localWakeup") || strings.Contains(string(blob), "wakePending") {
		t.Fatalf("disabled runner serialised a local-wakeup key: %s", blob)
	}
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners([]Runner{r})
	sblob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(sblob), "localWakeup") || strings.Contains(string(sblob), "wakePending") {
		t.Fatalf("disabled snapshot serialised a local-wakeup key: %s", sblob)
	}
	t.Logf("disabled runner/snapshot serialise no localWakeup/wakePending key")
}

// TestT046PersistedPendingSchedules proves a legitimately persisted pending
// schedules a publication even when decisionDirty is false, and a real JSON import
// reproduces it.
func TestT046PersistedPendingSchedules(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
	if err := r.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.6, Y: 0.5, VXM: 2.0})
	sim.Step()
	sim.runners[0].WakePending = true
	sim.decisionDirty = false
	base := sim.DecisionPublishCount()
	sim.Step()
	if sim.DecisionPublishCount() != base+1 {
		t.Fatalf("persisted pending did not schedule a publication")
	}
	if sim.runners[0].WakePending {
		t.Fatal("persisted pending was not consumed")
	}

	baseSim := mustSim(t, cfg, 1)
	br := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
	if err := br.EnableLocalWakeup(2.0); err != nil {
		t.Fatal(err)
	}
	baseSim.SetRunners([]Runner{br})
	baseSim.SetBall(Ball{Mode: BallRolling, X: 0.6, Y: 0.5, VXM: 2.0})
	snap := baseSim.Snapshot()
	snap.ClockMS = 120
	snap.NextDecisionMS = 200
	snap.DecisionDirty = false
	snap.Runners[0].WakePending = true
	blob, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal import: %v", err)
	}
	var back Snapshot
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal import: %v", err)
	}
	rs, err := Resume(back)
	if err != nil {
		t.Fatalf("Resume import: %v", err)
	}
	if !rs.runners[0].WakePending {
		t.Fatal("JSON import lost the pending flag")
	}
	baseCount := rs.DecisionPublishCount()
	rs.Step()
	if rs.DecisionPublishCount() != baseCount+1 {
		t.Fatal("imported pending did not schedule a publication")
	}
	if rs.runners[0].WakePending {
		t.Fatal("imported pending was not consumed")
	}
	t.Logf("persisted pending schedules without decisionDirty; JSON import resumes and schedules it")
}

// TestT046JSONResumeScenes proves complete JSON Resume across the required local
// scenes: an UNKNOWN local-pursuit+wakeup runner (reaction 130, splits 120/140/200),
// a SEEN-but-outside-range STALE runner (asserted at the split), a real mid-tail
// second release applied symmetrically, and the existing coalesced pending boundary.
// The comparator checks the COMPLETE snapshot (incl. RNG and events) every step and
// the samples tail.
func TestT046JSONResumeScenes(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	// (a) Unknown runner, ball never in range, reaction ARMED at 130 (no
	// ReleasePass here, so ReactAtMS is set explicitly); splits before/at/after the
	// reaction deadline and the lattice.
	for _, split := range []int{120, 140, 180, 200, 220} {
		straight := mustSim(t, cfg, 1)
		r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
		r.ReactMS = 130
		r.ReactAtMS = 130 // explicitly arm the reaction gate (no ReleasePass in this branch)
		r.ReachM = 0.05
		if err := r.EnableLocalPursuit(2.0); err != nil {
			t.Fatal(err)
		}
		if err := r.EnableLocalWakeup(2.0); err != nil {
			t.Fatal(err)
		}
		straight.SetRunners([]Runner{r})
		straight.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.9})
		for straight.ClockMS() < split {
			straight.Step()
		}
		if straight.runners[0].ReactAtMS != 130 {
			t.Fatalf("unknown scene split %d: ReactAtMS %d, want 130 (gate must be armed)", split, straight.runners[0].ReactAtMS)
		}
		// The reaction gate blocks while clock < ReactAtMS and has passed afterwards.
		blocking := straight.ClockMS() < straight.runners[0].ReactAtMS
		if wantBlocking := split < 130; blocking != wantBlocking {
			t.Fatalf("unknown scene split %d: reaction gate blocking=%v, want %v", split, blocking, wantBlocking)
		}
		if o, _ := straight.Perception(0); o.Seen {
			t.Fatalf("unknown scene split %d: record seen", split)
		}
		rs := t046JSONResume(t, straight)
		t046AssertRuntimeTails(t, straight, rs, split+300, "unknown")
	}

	// (b) Seen-then-outside-range stale runner, asserted at the split.
	{
		straight := mustSim(t, cfg, 1)
		r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
		r.ReachM = 0.05
		if err := r.EnableLocalPursuit(2.0); err != nil {
			t.Fatal(err)
		}
		if err := r.EnableLocalWakeup(2.0); err != nil {
			t.Fatal(err)
		}
		straight.SetRunners([]Runner{r})
		straight.SetBall(Ball{Mode: BallLoose, X: 0.51, Y: 0.5})
		for straight.ClockMS() < 400 {
			straight.Step()
		}
		straight.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		for straight.ClockMS() < 700 {
			straight.Step()
		}
		o, _ := straight.Perception(0)
		if !o.Seen || o.Freshness(700) != ObservationStale {
			t.Fatalf("stale scene not stale: %+v", o)
		}
		if d := DistM(straight.ball.X, straight.ball.Y, straight.runners[0].X, straight.runners[0].Y); d <= 2.0 {
			t.Fatalf("stale scene ball inside radius: %.3f m", d)
		}
		rs := t046JSONResume(t, straight)
		t046AssertRuntimeTails(t, straight, rs, 1100, "stale")
	}

	// (c) REAL first release at 0 (in range) plus a REAL mid-tail second release at
	// 820, both applied symmetrically to the straight and resumed tails.
	{
		straight := mustSim(t, cfg, 1)
		r := InertialRunner(0.5, 0.5, 0.95, 0.5, DefaultMotionParams())
		r.VXM = 2.0
		r.ReachM = 0.05
		if err := r.EnableLocalPursuit(2.0); err != nil {
			t.Fatal(err)
		}
		if err := r.EnableLocalWakeup(2.0); err != nil {
			t.Fatal(err)
		}
		straight.SetRunners([]Runner{r})
		// REAL first release at 0, near the runner and in range.
		straight.ReleasePass(PassParams{StartX: 0.51, StartY: 0.5, TargetX: 0.60, TargetY: 0.5, SpeedMPS: 12.0, Passer: -1})
		for straight.ClockMS() < 300 {
			straight.Step()
		}
		if rel := t042EventClocks(straight.Events(), PassEventRelease); len(rel) != 1 || rel[0] != 0 {
			t.Fatalf("mid-tail first release fixture: releases=%v, want [0]", rel)
		}
		rs := t046JSONResume(t, straight)
		mid := func(s *Sim, clock int) {
			if clock == 820 {
				s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.5, TargetX: 0.30, TargetY: 0.5, SpeedMPS: 12.0, Passer: -1})
			}
		}
		t046AssertRuntimeTailsMid(t, straight, rs, 1200, "mid-release", mid)
		if rel := t042EventClocks(straight.Events(), PassEventRelease); len(rel) != 2 || rel[0] != 0 || rel[1] != 820 {
			t.Fatalf("want releases [0 820], got %v", rel)
		}
	}

	// (d) Existing coalesced pending boundary.
	{
		straight := mustSim(t, cfg, 1)
		w := InertialRunner(0.93, 0.5, 0.95, 0.95, DefaultMotionParams())
		if err := w.EnableLocalPursuit(20.0); err != nil {
			t.Fatal(err)
		}
		if err := w.EnableLocalWakeup(20.0); err != nil {
			t.Fatal(err)
		}
		straight.SetRunners([]Runner{w, {X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
		straight.controlError = 1.0
		straight.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
		straight.Step()
		if !straight.runners[0].WakePending {
			t.Fatal("coalesced fixture did not produce a pending")
		}
		rs := t046JSONResume(t, straight)
		if !rs.runners[0].WakePending {
			t.Fatal("pending lost in JSON round-trip")
		}
		t046AssertRuntimeTails(t, straight, rs, 400, "pending-deflect")
	}
	t.Logf("complete JSON Resume: unknown (reaction armed @130; splits 120/140/180/200/220), stale, real [0,820] mid-tail releases and coalesced pending all value-identical")
}

// TestT046ResumeContextValidation checks the contextual import guards.
func TestT046ResumeContextValidation(t *testing.T) {
	base := func(cfg Config, r Runner) Snapshot {
		return Snapshot{Cfg: cfg, ClockMS: 40, RNG: [4]uint64{1, 2, 3, 4}, Runners: []Runner{r}}
	}
	valid := func() Runner {
		r := InertialRunner(0.6, 0.5, 0.6, 0.5, DefaultMotionParams())
		if err := r.EnableLocalWakeup(4.0); err != nil {
			t.Fatalf("EnableLocalWakeup: %v", err)
		}
		return r
	}
	pendingNoFlag := valid()
	pendingNoFlag.LocalWakeup = false
	pendingNoFlag.WakePending = true
	if _, err := Resume(base(ScheduledLivePassConfig(100), pendingNoFlag)); !errors.Is(err, ErrLocalWakeupPending) {
		t.Fatalf("pending-no-flag err=%v, want ErrLocalWakeupPending", err)
	}
	noRec := InertialRunner(0.6, 0.5, 0.6, 0.5, DefaultMotionParams())
	noRec.LocalWakeup = true
	if _, err := Resume(base(ScheduledLivePassConfig(100), noRec)); !errors.Is(err, ErrLocalWakeupNoPerception) {
		t.Fatalf("no-perception err=%v, want ErrLocalWakeupNoPerception", err)
	}
	if _, err := Resume(base(LivePassConfig(), valid())); !errors.Is(err, ErrLocalWakeupNoScheduler) {
		t.Fatalf("disabled-scheduler err=%v, want ErrLocalWakeupNoScheduler", err)
	}
	if _, err := Resume(base(ScheduledLivePassConfig(100), valid())); err != nil {
		t.Fatalf("valid local-wakeup runner rejected: %v", err)
	}
	clean := InertialRunner(0.6, 0.5, 0.6, 0.5, DefaultMotionParams())
	if _, err := Resume(base(LivePassConfig(), clean)); err != nil {
		t.Fatalf("clean unflagged runner rejected: %v", err)
	}
	t.Logf("contextual import guards: impossible pending, missing perception, disabled scheduler all rejected; valid accepted")
}

// --- 9. default/shared independent reference -------------------------------

// TestT046DefaultSharedReference proves the default (no opt-in) path is unchanged
// against an INDEPENDENT contract oracle: the publication clocks match T-042's
// hand-computed invalidating-event/lattice set, every native pursuer's published
// target is the true pre-step ball, no local diagnostics are allocated and no
// pending/opt-in exists.
func TestT046DefaultSharedReference(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	p := DefaultMotionParams()
	sensing := InertialRunner(0.10, 0.10, 0.10, 0.10, p)
	if err := sensing.EnableSensing(200.0); err != nil {
		t.Fatal(err)
	}
	sim.SetRunners([]Runner{
		PursuingRunner(0.60, 0.30, p, 0),
		PursuingRunner(0.70, 0.70, p, 0),
		sensing,
	})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0, VYM: 0.2})
	last := sim.DecisionPublishCount()
	var actual []int
	for sim.ClockMS() < 2000 {
		preBall := sim.ball
		start := sim.ClockMS()
		sim.Step()
		if c := sim.DecisionPublishCount(); c > last {
			last = c
			actual = append(actual, start)
			for i := range sim.runners {
				r := sim.runners[i]
				if r.Pursue && r.Motion == MotionInertial && !r.LocalPursuit {
					if r.IntentX != preBall.X || r.IntentY != preBall.Y {
						t.Fatalf("@%d: native runner %d intent (%v,%v) != independent pre-step ball (%v,%v)",
							start, i, r.IntentX, r.IntentY, preBall.X, preBall.Y)
					}
				}
			}
		}
	}
	want := t042ExpectedPubs(cfg, sim.Events(), 1980)
	if !t042IntsEqual(actual, want) {
		t.Fatalf("publication clocks %v != independent contract oracle %v", actual, want)
	}
	if sim.localGate != nil || sim.localPubMS != nil {
		t.Fatal("local diagnostics were allocated with no opt-in runner")
	}
	if sim.LocalWakeupPublishCount() != 0 {
		t.Fatalf("local scheduling attempts without an opt-in: %d", sim.LocalWakeupPublishCount())
	}
	for i := range sim.runners {
		if sim.runners[i].WakePending {
			t.Fatalf("runner %d has a pending without an opt-in", i)
		}
	}
	t.Logf("default path matches the independent T-042 contract oracle (%d publications); no local diagnostics, no pending", len(actual))
}

// t046ReferenceStep reproduces the PRE-T-046 shared publication INDEPENDENTLY of
// maybePublishIntents: it computes the lattice/dirty decision itself and calls the
// old publisher helpers manually, then keeps the built-in scheduler inert for the
// physics Step (lattice advanced, dirty cleared), so no semantic change is applied.
func t046ReferenceStep(sim *Sim) {
	clock := sim.clock
	due := clock >= sim.nextDecisionMS
	if due {
		sim.nextDecisionMS = (clock/sim.cfg.IntentCadenceMS + 1) * sim.cfg.IntentCadenceMS
	}
	dirty := sim.decisionDirty
	if due || dirty {
		sim.publishPursuitIntents()
		sim.samplePerception(clock)
		sim.publishLocalPursuitIntents(clock)
	}
	sim.decisionDirty = false
	sim.Step()
}

// t046CompareRuntimeRef steps the real runtime and the independent reference in
// lockstep (with the same optional mid hook), asserting the COMPLETE snapshot is
// identical every step and the sample streams match.
func t046CompareRuntimeRef(t *testing.T, real, ref *Sim, horizon int, mid func(*Sim, int), label string) {
	t.Helper()
	for real.ClockMS() < horizon {
		if ref.ClockMS() != real.ClockMS() {
			t.Fatalf("%s: clock %d vs %d", label, real.ClockMS(), ref.ClockMS())
		}
		if mid != nil {
			mid(real, real.ClockMS())
			mid(ref, ref.ClockMS())
		}
		real.Step()
		t046ReferenceStep(ref)
		if !reflect.DeepEqual(real.Snapshot(), ref.Snapshot()) {
			t.Fatalf("%s @%d: complete snapshot diverged between the runtime and the independent reference", label, real.ClockMS())
		}
	}
	if !reflect.DeepEqual(real.Samples(), ref.Samples()) {
		t.Fatalf("%s: sample streams differ", label)
	}
}

// t046RefLocal builds an UNFLAGGED inertial local-pursuit + sensing runner (the
// shared local layer the pre-T-046 reference must reproduce).
func t046RefLocal(x, y, radius float64) Runner {
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	r.ReachM = ControlRadiusM
	if err := r.EnableLocalPursuit(radius); err != nil {
		panic(err)
	}
	return r
}

// TestT046SharedReferenceScenes compares the runtime against an INDEPENDENT
// pre-T-046 shared local scheduler reference (manual old publisher helpers, built-in
// scheduler inert) across release/deflect/held/contact/reaction and a mid-tail
// second release, with full snapshots compared every step.
func TestT046SharedReferenceScenes(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()
	scenes := []struct {
		name    string
		setup   func(*Sim)
		mid     func(*Sim, int)
		horizon int
		want    PassEventKind
	}{
		{"release", func(sim *Sim) {
			sim.SetRunners([]Runner{t046RefLocal(0.60, 0.50, 100.0), InertialRunner(0.30, 0.50, 0.30, 0.50, p)})
			sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: 1})
		}, nil, 1500, PassEventControl},
		{"deflect", func(sim *Sim) {
			sim.SetRunners([]Runner{t046RefLocal(0.60, 0.50, 100.0)})
			sim.controlError = 1.0
			sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 12.0})
		}, nil, 1500, PassEventDeflect},
		{"held", func(sim *Sim) {
			sim.SetRunners([]Runner{t046RefLocal(0.55, 0.50, 100.0)})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.55, Y: 0.50})
		}, nil, 800, PassEventControl},
		{"contact", func(sim *Sim) {
			sim.SetRunners([]Runner{t046RefLocal(0.60, 0.50, 100.0), t046RefLocal(0.70, 0.50, 100.0)})
			sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 12.0})
		}, nil, 1500, PassEventControl},
		{"reaction", func(sim *Sim) {
			r := t046RefLocal(0.60, 0.50, 100.0)
			r.ReactMS = 200
			r.ReactAtMS = 0
			sim.SetRunners([]Runner{r})
			sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
		}, nil, 1500, PassEventControl},
		{"mid-release", func(sim *Sim) {
			sim.SetRunners([]Runner{t046RefLocal(0.60, 0.50, 100.0)})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.50})
		}, func(s *Sim, clock int) {
			if clock == 620 {
				s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.50, TargetX: 0.30, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
			}
		}, 1000, PassEventRelease},
	}
	for _, sc := range scenes {
		real := mustSim(t, cfg, 1)
		ref := mustSim(t, cfg, 1)
		sc.setup(real)
		sc.setup(ref)
		t046CompareRuntimeRef(t, real, ref, sc.horizon, sc.mid, sc.name)
		if countKind(real, sc.want) == 0 {
			t.Fatalf("%s: independent reference scene produced no %s event", sc.name, sc.want)
		}
	}
	t.Logf("runtime matches the independent pre-T-046 shared reference on %d scenes (full snapshots every step)", len(scenes))
}

// t042EventClocks returns the clocks of events of kind k.
func t042EventClocks(evs []PassEvent, k PassEventKind) []int {
	var out []int
	for _, e := range evs {
		if e.Kind == k {
			out = append(out, e.ClockMS)
		}
	}
	return out
}
