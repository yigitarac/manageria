package liveplay

import (
	"reflect"
	"testing"
)

// T-053 transition-heavy active22 cost and persistence study (isolated prototype
// evidence, TEST-ONLY).
//
// Prespec (seeds/geometry/order/windows/deadlines/snapshot+cost protocol):
//   vault/07-Session-Logs/2026-10-16-t053-transition-cost-evidence.md
//
// Synthetic deterministic clips, not full matches. No runtime change; S0
// reaction/cooldown/immunity and participant/RNG order are preserved.

const (
	t053ControlSeed = 101
	t053DeflectSeed = 202
	t053WindowSteps = 100 // 100 substeps = 2000 ms
	t053SecondRelMS = 1000
)

// t053SensingObserver is a moving SENSING-ONLY observer with an explicit tiny
// reach (non-participant fixture parameter).
func t053SensingObserver(x, y float64) Runner {
	r := InertialRunner(x, y, x+0.5, y, DefaultMotionParams())
	r.ReachM = 0.01
	if err := r.EnableSensing(200.0); err != nil {
		panic(err)
	}
	return r
}

// t053LocalObserver is a moving local-pursuit + local-wakeup observer with a tiny
// reach (it chases the free ball, moves every step, and yields sidecar records).
func t053LocalObserver(x, y float64) Runner {
	r := t050Local(x, y, x+0.5, y, 200.0, 0, true)
	r.ReachM = 0.01
	return r
}

// t053Runners builds the 22-body fixture: index 0 participant, indices 1-2
// observers, indices 3-21 off-lane movers.
//
// "near"/"far" are GEOMETRIC positions ONLY: both observers have a 200 m
// (whole-pitch) sensing radius, so this is NOT a distant-observer-unaware claim.
func t053Runners(control bool) []Runner {
	p := DefaultMotionParams()
	rs := make([]Runner, 22)
	rs[0] = InertialRunner(0.50, 0.50, 0.30, 0.50, p) // participant moves toward the ball
	if control {
		rs[1] = t053SensingObserver(0.30, 0.53)
		rs[2] = t053SensingObserver(0.10, 0.30)
	} else {
		rs[1] = t053LocalObserver(0.30, 0.53)
		rs[2] = t053LocalObserver(0.10, 0.30)
	}
	for i := 3; i < 22; i++ {
		row := i % 2
		x := 0.05 + float64(i%11)*0.085
		y := 0.08 + float64(row)*0.84
		iy := 0.02
		if row == 1 {
			iy = 0.98
		}
		rs[i] = InertialRunner(x, y, x+0.5, iy, p)
	}
	return rs
}

// t053Fill (re)builds the fixture on a sim (used inside StopTimer in benchmarks).
func t053Fill(sim *Sim, control bool) {
	sim.SetRunners(t053Runners(control))
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
}

// t053Build builds a fresh fixture sim.
func t053Build(t *testing.T, seed uint64, control bool) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	t053Fill(sim, control)
	return sim
}

// t053SecondReleaseParams is the control window's real mid-window second release.
func t053SecondReleaseParams() PassParams {
	return PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1}
}

// t053MidSecondRelease applies the second release at the declared clock.
func t053MidSecondRelease(s *Sim, clock int) {
	if clock == t053SecondRelMS {
		s.ReleasePass(t053SecondReleaseParams())
	}
}

// t053StepToWithMid steps to endMS, applying mid before each step.
func t053StepToWithMid(sim *Sim, end int, mid func(*Sim, int)) {
	for sim.ClockMS() < end {
		if mid != nil {
			mid(sim, sim.ClockMS())
		}
		sim.Step()
	}
}

// t053StepWithMid performs ONE declared window substep on a prepared sim (the
// control window's real mid second release lands on substep index 50). This is the
// SINGLE shared window step used by both the guard and the benchmark, so they
// cannot drift.
func t053StepWithMid(sim *Sim, control bool, k int) {
	if control && k == 50 {
		sim.ReleasePass(t053SecondReleaseParams())
	}
	sim.Step()
}

// t053AdvanceWindow runs the declared 100-substep window via the shared per-step
// helper and returns the number of held substeps. Shared by guard and benchmark.
func t053AdvanceWindow(sim *Sim, control bool, onStep func(*Sim, int)) int {
	held := 0
	for k := 0; k < t053WindowSteps; k++ {
		t053StepWithMid(sim, control, k)
		if sim.Ball().Mode == BallHeld {
			held++
		}
		if onStep != nil {
			onStep(sim, k)
		}
	}
	return held
}

// t053Perceptions returns all 22 observation records for equality checks.
func t053Perceptions(t *testing.T, sim *Sim) []BallObservation {
	t.Helper()
	out := make([]BallObservation, 22)
	for i := range out {
		o, _ := sim.Perception(i)
		out[i] = o
	}
	return out
}

// t053AssertSplitState asserts the ACTUAL fixture precondition at a JSON split.
func t053AssertSplitState(t *testing.T, sim *Sim, control bool, split int) {
	t.Helper()
	if sim.ClockMS() != split {
		t.Fatalf("split %d reached clock %d", split, sim.ClockMS())
	}
	if control {
		touches := t052Touches(sim)
		switch split {
		case 20: // contact state
			if len(touches) == 0 || touches[0].Kind != PassEventControl {
				t.Fatalf("control split 20: expected a contact, got %+v", touches)
			}
		case 200: // held state
			if sim.Ball().Mode != BallHeld {
				t.Fatalf("control split 200: ball mode=%s, want held", sim.Ball().Mode)
			}
		case 1020: // post-second-release state
			rel := t051ReleaseClocks(sim.Events())
			if len(rel) != 2 || rel[1] != t053SecondRelMS {
				t.Fatalf("control split 1020: releases %v, want [0 %d]", rel, t053SecondRelMS)
			}
		}
		return
	}
	switch split {
	case 20: // before contact
		if len(t052Touches(sim)) != 0 {
			t.Fatalf("deflect split 20: unexpected touch before contact")
		}
	case 1100: // DURING the cooldown (deadline 1176 > 1100), NOT post-cooldown
		if len(t052Touches(sim)) == 0 {
			t.Fatalf("deflect split 1100: expected a deflect")
		}
		if sim.runners[0].CooldownUntilMS <= sim.ClockMS() {
			t.Fatalf("deflect split 1100: cooldown %d not still active at %d", sim.runners[0].CooldownUntilMS, sim.ClockMS())
		}
	}
}

// t053AssertAllMoved verifies every body moved > 1e-9 m this substep.
func t053AssertAllMoved(t *testing.T, label string, prev, cur []Runner, step int) {
	t.Helper()
	for i := range cur {
		if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
			t.Fatalf("%s: body %d did not move at step %d", label, i, step)
		}
	}
}

// --- guards ------------------------------------------------------------------

// TestT053ControlWindowActive pins the control window: 22/22 move every substep,
// a genuine interior control, held integration and a real mid-window second
// release.
func TestT053ControlWindowActive(t *testing.T) {
	sim := t053Build(t, t053ControlSeed, true)
	prev := sim.Runners()
	heldSteps := t053AdvanceWindow(sim, true, func(s *Sim, k int) {
		cur := s.Runners()
		t053AssertAllMoved(t, "control", prev, cur, k)
		prev = cur
	})
	if heldSteps == 0 {
		t.Fatal("no held substep (control not produced)")
	}
	touches := t052Touches(sim)
	if len(touches) == 0 || touches[0].Kind != PassEventControl || touches[0].Runner != 0 {
		t.Fatalf("first touch not an interior control by runner0: %+v", touches)
	}
	if !(touches[0].Frac > 0 && touches[0].Frac < 1) {
		t.Fatalf("control frac %v not interior", touches[0].Frac)
	}
	rel := t051ReleaseClocks(sim.Events())
	if len(rel) != 2 || rel[0] != 0 || rel[1] != t053SecondRelMS {
		t.Fatalf("release clocks %v, want [0 %d]", rel, t053SecondRelMS)
	}
	if o, _ := sim.Perception(1); !o.Seen {
		t.Fatalf("near observer never saw the ball: %+v", o)
	}
	if o, _ := sim.Perception(2); !o.Seen {
		t.Fatalf("far observer never saw the ball: %+v", o)
	}
	t.Logf("T053 control window: 22/22 moved every step for %d steps; %d held substeps; first control runner0 frac=%.3f exact=%d; releases %v; observers geometric near/far with a 200 m whole-pitch view",
		t053WindowSteps, heldSteps, touches[0].Frac, t052ExactTime(touches[0]), rel)
}

// TestT053DeflectWindowActive pins the deflect window: 22/22 move every substep,
// a genuine interior deflect with the exact cooldown, and nonempty actual sidecar.
func TestT053DeflectWindowActive(t *testing.T) {
	sim := t053Build(t, t053DeflectSeed, false)
	rec := mustRecorder(t, 8192)
	sim.SetProvenanceRecorder(rec)
	prev := sim.Runners()
	t053AdvanceWindow(sim, false, func(s *Sim, k int) {
		cur := s.Runners()
		t053AssertAllMoved(t, "deflect", prev, cur, k)
		prev = cur
	})
	touches := t052Touches(sim)
	if len(touches) == 0 || touches[0].Kind != PassEventDeflect || touches[0].Runner != 0 {
		t.Fatalf("first touch not an interior deflect by runner0: %+v", touches)
	}
	if !(touches[0].Frac > 0 && touches[0].Frac < 1) {
		t.Fatalf("deflect frac %v not interior", touches[0].Frac)
	}
	want := eventTimeMS(touches[0].ClockMS-SubstepMS, SubstepMS, touches[0].Frac) + DeflectionCooldownMS
	if sim.runners[0].CooldownUntilMS != want {
		t.Fatalf("cooldown %d, want %d", sim.runners[0].CooldownUntilMS, want)
	}
	if rec.Len() == 0 {
		t.Fatal("no sidecar records in the deflect window")
	}
	applied := 0
	for _, r := range rec.Records() {
		if r.Applied && r.Policy == PolicyChase {
			applied++
		}
	}
	if applied == 0 {
		t.Fatalf("no applied chase sidecar records (%d records)", rec.Len())
	}
	t.Logf("T053 deflect window: 22/22 moved every step; first deflect runner0 frac=%.3f exact=%d cooldown=%d; %d sidecar records (%d applied chase)",
		touches[0].Frac, t052ExactTime(touches[0]), want, rec.Len(), applied)
}

// --- repeated fresh windows + purity (with the mid second release) -----------

// TestT053RepeatedFreshWindowsDeterministic prepares THREE fresh windows per type
// (same seed + t053Fill) and asserts identical initial/final snapshots, events,
// perceptions and transitions, with all22 moving every step in each.
func TestT053RepeatedFreshWindowsDeterministic(t *testing.T) {
	for _, control := range []bool{true, false} {
		label := "deflect"
		seed := uint64(t053DeflectSeed)
		if control {
			label = "control"
			seed = t053ControlSeed
		}
		var inits, finals []Snapshot
		var evs, touches [][]PassEvent
		var percs [][]BallObservation
		for w := 0; w < 3; w++ {
			sim := mustSim(t, ScheduledLivePassConfig(100), seed)
			t053Fill(sim, control)
			inits = append(inits, sim.Snapshot())
			prev := sim.Runners()
			t053AdvanceWindow(sim, control, func(s *Sim, k int) {
				cur := s.Runners()
				t053AssertAllMoved(t, label, prev, cur, k)
				prev = cur
			})
			finals = append(finals, sim.Snapshot())
			evs = append(evs, sim.Events())
			percs = append(percs, t053Perceptions(t, sim))
			touches = append(touches, t052Touches(sim))
		}
		for w := 1; w < 3; w++ {
			if !reflect.DeepEqual(inits[0], inits[w]) {
				t.Fatalf("%s window %d: initial snapshot differs", label, w)
			}
			if !reflect.DeepEqual(finals[0], finals[w]) {
				t.Fatalf("%s window %d: final snapshot differs", label, w)
			}
			if !reflect.DeepEqual(evs[0], evs[w]) {
				t.Fatalf("%s window %d: events differ", label, w)
			}
			if !reflect.DeepEqual(percs[0], percs[w]) {
				t.Fatalf("%s window %d: perceptions differ", label, w)
			}
			if !reflect.DeepEqual(touches[0], touches[w]) {
				t.Fatalf("%s window %d: transitions differ", label, w)
			}
		}
		if len(touches[0]) == 0 {
			t.Fatalf("%s: no transition", label)
		}
		if control && touches[0][0].Kind != PassEventControl {
			t.Fatalf("control: first touch kind %s", touches[0][0].Kind)
		}
		if !control && touches[0][0].Kind != PassEventDeflect {
			t.Fatalf("deflect: first touch kind %s", touches[0][0].Kind)
		}
		t.Logf("T053 %s: 3 fresh windows identical (initial+final snapshots, events, perceptions, %d touches)", label, len(touches[0]))
	}
}

// TestT053PurityWithMidSecondRelease asserts attach-vs-none leaves the engine
// bit-identical every substep INCLUDING the control window's mid second release
// (t052RecorderPurity does not apply a mid hook).
func TestT053PurityWithMidSecondRelease(t *testing.T) {
	for _, control := range []bool{true, false} {
		label := "deflect"
		seed := uint64(t053DeflectSeed)
		if control {
			label = "control"
			seed = t053ControlSeed
		}
		a := mustSim(t, ScheduledLivePassConfig(100), seed)
		t053Fill(a, control)
		a.SetProvenanceRecorder(mustRecorder(t, 8192))
		b := mustSim(t, ScheduledLivePassConfig(100), seed)
		t053Fill(b, control)
		for k := 0; k < t053WindowSteps; k++ {
			t053StepWithMid(a, control, k)
			t053StepWithMid(b, control, k)
			if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
				t.Fatalf("%s: recorder changed the engine at substep %d", label, k)
			}
		}
		t.Logf("T053 %s: attach-vs-none bit-identical every substep incl. the mid second release", label)
	}
}

// --- persistence + sidecar tails + purity -----------------------------------

// TestT053JSONTailsAndPurity pins complete engine JSON Resume across the actual
// contact/held/second-release/deflect-cooldown split states, bit-identical attach
// vs none, and a real sidecar record tail.
func TestT053JSONTailsAndPurity(t *testing.T) {
	for _, split := range []int{20, 200, 1020} { // contact, held, post-second-release
		straight := t053Build(t, t053ControlSeed, true)
		t053StepToWithMid(straight, split, t053MidSecondRelease)
		t053AssertSplitState(t, straight, true, split)
		resumed := t046JSONResume(t, straight)
		t046AssertRuntimeTailsMid(t, straight, resumed, 1900, "t053-control", t053MidSecondRelease)
		t.Logf("T053 control JSON tail split=%d -> 1900 identical every substep", split)
	}
	for _, split := range []int{20, 1100} { // before contact, DURING cooldown (deadline 1176)
		straight := t053Build(t, t053DeflectSeed, false)
		t053StepToWithMid(straight, split, nil)
		t053AssertSplitState(t, straight, false, split)
		resumed := t046JSONResume(t, straight)
		t046AssertRuntimeTailsMid(t, straight, resumed, 1900, "t053-deflect", nil)
		t.Logf("T053 deflect JSON tail split=%d -> 1900 identical every substep", split)
	}

	straight := t053Build(t, t053DeflectSeed, false)
	t053StepToWithMid(straight, 20, nil)
	resumed := t046JSONResume(t, straight)
	t052AssertTailWithSidecar(t, straight, resumed, 1900, true, nil, "t053-sidecar")
	t.Logf("T053 sidecar tail 20->1900 identical record-for-record after JSON resume")
}
