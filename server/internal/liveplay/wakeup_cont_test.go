package liveplay

import (
	"math"
	"reflect"
	"testing"
)

// TestT045ObservationVelocityCaptured proves the observation record captures the
// ACTUAL ball velocity at each refresh: a sensing-only runner (no chase, no
// control) watches a BallRolling ball safely off-lane, and at every lattice refresh
// the observed VXM equals the true pre-step ball VXM. Frames also carry ball
// velocity and per-runner observations.
func TestT045ObservationVelocityCaptured(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	r := InertialRunner(0.50, 0.30, 0.50, 0.30, DefaultMotionParams())
	if err := r.EnableSensing(2.0); err != nil { // sensing-only: never chases or controls
		t.Fatalf("EnableSensing: %v", err)
	}
	h.sim.SetRunners([]Runner{r})
	// Off-lane by ~1.36 m (> the default 1 m control reach) and Rolling so it truly integrates.
	h.sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.32, VXM: 0.5})

	sawFresh := false
	tr := &t045Trace{mode: h.mode}
	for h.sim.ClockMS() < 400 {
		start := h.sim.ClockMS()
		preVXM := h.sim.ball.VXM
		preMode := h.sim.ball.Mode
		h.step()
		tr.record(h.sim)
		o := h.sim.runners[0].Perception
		if o != nil && o.Seen && o.ClockMS == start {
			sawFresh = true
			if o.Mode != BallRolling || preMode != BallRolling {
				t.Fatalf("@%d: sampled mode %s / pre mode %s, want rolling", start, o.Mode, preMode)
			}
			if o.VXM != preVXM {
				t.Fatalf("@%d: observed VXM %v != actual pre-step ball VXM %v", start, o.VXM, preVXM)
			}
		}
	}
	if !sawFresh {
		t.Fatal("no fresh observation was taken")
	}
	for _, f := range tr.frames {
		if len(f.obs) != 1 || len(f.hasObs) != 1 {
			t.Fatal("frame missing per-runner observation slots")
		}
	}
	t.Logf("observed ball VXM equals the actual pre-step VXM at every refresh; sensing-only runner, ball stays Rolling")
}

// TestT045ContinuationPendingTrueAfterDeflect is the pending=true JSON
// continuation: after a genuine, LOCAL, fractional DEFLECT the next pre-step has
// pending=true, which must survive a JSON snapshot and continue value-identically.
// It also audits the emitted event's fractional geometry by interpolating the
// touching runner's position at Frac.
func TestT045ContinuationPendingTrueAfterDeflect(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	h.sim.SetRunners([]Runner{t045Watcher(0.55, 0.50, 50.0, 0)})
	h.sim.controlError = 1.0 // force the miscontrol (test-only)
	h.sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	for h.sim.ClockMS() < 8000 && countKind(h.sim, PassEventDeflect) == 0 {
		h.step()
	}
	if countKind(h.sim, PassEventDeflect) == 0 {
		t.Fatal("no deflection occurred")
	}
	if !h.pendingLocal[0] {
		t.Fatalf("pendingLocal not set after the visible deflect: %v", h.pendingLocal)
	}
	var def PassEvent
	for _, e := range h.sim.Events() {
		if e.Kind == PassEventDeflect {
			def = e
			break
		}
	}
	if def.Frac <= 0 || def.Frac >= 1 {
		t.Fatalf("deflect Frac=%v not fractional", def.Frac)
	}
	rx := h.preX[0] + def.Frac*(h.endX[0]-h.preX[0])
	ry := h.preY[0] + def.Frac*(h.endY[0]-h.preY[0])
	if math.Abs(rx-def.RunnerX) > 1e-9 || math.Abs(ry-def.RunnerY) > 1e-9 {
		t.Fatalf("fractional runner interpolation (%.12f,%.12f) != emitted (%.12f,%.12f)", rx, ry, def.RunnerX, def.RunnerY)
	}

	split := h.sim.ClockMS()
	rh, st := t045ResumeFrom(t, h)
	if len(st.Pending) != 1 || !st.Pending[0] {
		t.Fatalf("resumed harness did not carry pending=true: %+v", st)
	}
	pS, pR := t045CompareTails(t, h, rh, split+400, "t045-deflect")
	if !reflect.DeepEqual(pS, pR) {
		t.Fatalf("publication tails differ after resume: %v vs %v", pS, pR)
	}
	t.Logf("continuation pending=true after local deflect @%dms (Frac=%.4f): full sim+harness identical every tail step; pub tail %v==%v", split, def.Frac, pS, pR)
}

// TestT045ContinuationAroundReactionWithVisibleStop covers the before/after
// REACTION continuation: a real visible STOP before a 300 ms reaction is dropped by
// the gate but leaves pending=true, and a resume at that boundary continues
// value-identically across the reaction deadline.
func TestT045ContinuationAroundReactionWithVisibleStop(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	// Off-lane slow ball (~13.6 m lateral) inside a 30 m circle so it emits a real
	// visible STOP without ever being controllable (ReachM 0.05).
	h.sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, 30.0, 300)})
	h.sim.SetBall(Ball{Mode: BallRolling, X: 0.51, Y: 0.70, VXM: 0.12})
	for h.sim.ClockMS() < 3000 && countKind(h.sim, PassEventStop) == 0 {
		h.step()
	}
	if countKind(h.sim, PassEventStop) == 0 {
		t.Fatal("no stop event")
	}
	stopClock := t045EventClockOfKind(h.sim.Events(), PassEventStop)
	if stopClock >= 300 {
		t.Fatalf("stop %d not before the 300 ms reaction", stopClock)
	}
	if !h.pendingLocal[0] {
		t.Fatalf("visible stop did not set pendingLocal: %v", h.pendingLocal)
	}
	rh, st := t045ResumeFrom(t, h)
	if len(st.Pending) != 1 || !st.Pending[0] {
		t.Fatalf("pending not carried at the stop boundary: %+v", st)
	}
	pS, pR := t045CompareTails(t, h, rh, 700, "t045-stop")
	if !reflect.DeepEqual(pS, pR) {
		t.Fatalf("tail differs across the reaction: %v vs %v", pS, pR)
	}
	t.Logf("continuation around reaction: visible STOP @%dms (before 300 ms reaction), pending=true carried; tails identical %v", stopClock, pS)
}

// TestT045ContinuationBeforeAtAfterDue covers JSON continuation snapshots just
// before, exactly at and just after a lattice due clock: the tail (complete Sim +
// harness state + publication records) is identical from each split.
func TestT045ContinuationBeforeAtAfterDue(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(sim *Sim) {
		r := InertialRunner(0.50, 0.30, 0.50, 0.30, DefaultMotionParams())
		if err := r.EnableSensing(2.0); err != nil { // sensing-only, no events
			panic(err)
		}
		sim.SetRunners([]Runner{r})
		sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.32, VXM: 0.5})
	}
	for _, split := range []int{180, 200, 220} {
		h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
		setup(h.sim)
		for h.sim.ClockMS() < split {
			h.step()
		}
		rh, _ := t045ResumeFrom(t, h)
		pS, pR := t045CompareTails(t, h, rh, split+300, "t045-due")
		if !reflect.DeepEqual(pS, pR) {
			t.Fatalf("split @%d: publication tails differ: %v vs %v", split, pS, pR)
		}
		t.Logf("split @%dms (before/at/after a due clock): tails identical %v", split, pS)
	}
}
