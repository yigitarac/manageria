package liveplay

import (
	"math"
	"reflect"
	"testing"
)

// T-052 local control-transition and contest-awareness study (isolated prototype
// evidence, TEST-ONLY).
//
// The ordering contract, observed-vs-authoritative moment list and paired-scene
// matrix are fixed BEFORE this code in:
//   vault/07-Session-Logs/2026-10-16-t052-transition-awareness-evidence.md
//
// Every scene reads the authoritative event log for EVIDENCE only; no event
// outcome is a decision input. No runtime behaviour is changed. `reflect` is used
// for the paired recorder/non-recorder purity comparisons.

// t052InteriorPass returns a zero-jitter ground pass whose contact lands at an
// INTERIOR within-substep fraction (ball starts 1.02 m out, sweeps in within one
// 20 ms substep).
func t052InteriorPass(speed, controlError float64) PassParams {
	return PassParams{
		StartX: 0.50 - 1.02/PitchLengthM, StartY: 0.50,
		TargetX: 0.60, TargetY: 0.50,
		SpeedMPS: speed, Passer: -1, ControlError: controlError,
	}
}

// t052FirstKind returns the first event of the given kind.
func t052FirstKind(sim *Sim, kind PassEventKind) (PassEvent, bool) {
	for _, e := range sim.Events() {
		if e.Kind == kind {
			return e, true
		}
	}
	return PassEvent{}, false
}

// t052Touches returns the contact events (control + deflect) in order.
func t052Touches(sim *Sim) []PassEvent {
	var out []PassEvent
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl || e.Kind == PassEventDeflect {
			out = append(out, e)
		}
	}
	return out
}

// t052ExactTime returns the declared integer-ms time of an in-step event
// (ClockMS is the end-step clock; Frac is the within-substep fraction).
func t052ExactTime(e PassEvent) int {
	return eventTimeMS(e.ClockMS-SubstepMS, SubstepMS, e.Frac)
}

// t052Sensing returns a stationary sensing-only inertial runner.
func t052Sensing(t *testing.T, x, y, radiusM float64) Runner {
	t.Helper()
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	if err := r.EnableSensing(radiusM); err != nil {
		t.Fatalf("EnableSensing: %v", err)
	}
	return r
}

// t052StepTo advances until the clock reaches endMS.
func t052StepTo(sim *Sim, endMS int) {
	for sim.ClockMS() < endMS {
		sim.Step()
	}
}

// t052LogRunner logs the full own motion/intent/gates/observation of one runner.
func t052LogRunner(t *testing.T, label string, sim *Sim, i int) {
	t.Helper()
	r := sim.runners[i]
	o, _ := sim.Perception(i)
	t.Logf("%s t=%d r%d own(x=%.4f y=%.4f vx=%.3f vy=%.3f intent=(%.4f,%.4f) ReactAt=%d cooldown=%d) obs(seen=%v clock=%d x=%.4f y=%.4f mode=%s owner=%d) immunity=%d",
		label, sim.ClockMS(), i, r.X, r.Y, r.VXM, r.VYM, r.IntentX, r.IntentY, r.ReactAtMS, r.CooldownUntilMS,
		o.Seen, o.ClockMS, o.X, o.Y, o.Mode, o.Owner, sim.immunityUntil)
}

// mustRecorder builds a bounded recorder or fails.
func mustRecorder(t *testing.T, capacity int) *ProvenanceRecorder {
	t.Helper()
	rec, err := NewProvenanceRecorder(capacity)
	if err != nil {
		t.Fatalf("NewProvenanceRecorder: %v", err)
	}
	return rec
}

// t052RecorderPurity runs the same scene with and without a recorder and asserts
// the engine Snapshot is bit-identical every substep (the sidecar is pure).
func t052RecorderPurity(t *testing.T, cfg Config, seed uint64, build func(*Sim), steps int, label string) {
	t.Helper()
	withRec := mustSim(t, cfg, seed)
	build(withRec)
	withRec.SetProvenanceRecorder(mustRecorder(t, 1024))
	plain := mustSim(t, cfg, seed)
	build(plain)
	for i := 0; i < steps; i++ {
		withRec.Step()
		plain.Step()
		if !reflect.DeepEqual(withRec.Snapshot(), plain.Snapshot()) {
			t.Fatalf("%s: recorder changed the engine at step %d", label, i)
		}
	}
}

// --- A: interior clean control ---------------------------------------------

// TestT052InteriorControl pins a clean control at an interior fraction and its
// next pre-step echo, with the gates unchanged and the recorder pure.
func TestT052InteriorControl(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))

	sim.Step() // 0 -> 20
	ctrl, ok := t052FirstKind(sim, PassEventControl)
	if !ok {
		t.Fatalf("no control event: %v", sim.Events())
	}
	if !(ctrl.Frac > 0 && ctrl.Frac < 1) {
		t.Fatalf("control Frac=%v, want interior (0,1)", ctrl.Frac)
	}
	if d := DistM(ctrl.BallX, ctrl.BallY, ctrl.RunnerX, ctrl.RunnerY); math.Abs(d-ControlRadiusM) > 1e-6 {
		t.Fatalf("control ball/runner distance %.9f != reach %.3f", d, ControlRadiusM)
	}
	if ctrl.Runner != 0 {
		t.Fatalf("control runner=%d, want 0", ctrl.Runner)
	}
	r := sim.runners[0]
	if r.ReactAtMS != 0 || r.CooldownUntilMS != 0 || sim.immunityUntil != PasserImmunityMS {
		t.Fatalf("gates changed by control: ReactAt=%d cooldown=%d immunity=%d", r.ReactAtMS, r.CooldownUntilMS, sim.immunityUntil)
	}
	if ex := t052ExactTime(ctrl); ex <= 0 || ex > SubstepMS {
		t.Fatalf("control exact time %d outside (0,%d]", ex, SubstepMS)
	}
	t052LogRunner(t, "A post-control", sim, 0)
	sim.Step() // 20 -> 40: next dirty pre-step publication
	post, _ := sim.Perception(0)
	if !post.Seen || post.ClockMS != SubstepMS || post.Mode != BallHeld || post.Owner != 0 {
		t.Fatalf("next pre-step echo not Held/owner0: %+v", post)
	}
	t052RecorderPurity(t, cfg, 1, func(s *Sim) {
		s.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
		s.ReleasePass(t052InteriorPass(2.0, 0.0))
	}, 40, "A")
	t.Logf("A: interior control Frac=%.3f exactTime=%d; next pre-step Held owner0 @20; recorder pure", ctrl.Frac, t052ExactTime(ctrl))
}

// --- B: interior deflect + scheduling vs physical cooldown -----------------

// TestT052InteriorDeflectCooldown pins an interior deflect and shows the physical
// cooldown (not publication timing) governs the NEXT touch, which stays a deflect
// because ControlError=1 keeps miscontrolling.
func TestT052InteriorDeflectCooldown(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 2)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim.ReleasePass(t052InteriorPass(2.0, 1.0))
	sim.Step()
	def, ok := t052FirstKind(sim, PassEventDeflect)
	if !ok {
		t.Fatalf("no deflect: %v", sim.Events())
	}
	if !(def.Frac > 0 && def.Frac < 1) {
		t.Fatalf("deflect Frac=%v, want interior", def.Frac)
	}
	if def.OutVXM == 0 && def.OutVYM == 0 {
		t.Fatal("deflect has zero outgoing velocity")
	}
	cooldown := sim.runners[0].CooldownUntilMS
	if want := eventTimeMS(0, SubstepMS, def.Frac) + DeflectionCooldownMS; cooldown != want {
		t.Fatalf("cooldown=%d, want %d", cooldown, want)
	}

	// SCRIPTED mechanical fixture: place the ball within reach during the cooldown.
	// ControlError=1 stays 1, so the next eligible touch must ALSO be a deflect.
	sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50})
	t052StepTo(sim, cooldown+40)
	touches := t052Touches(sim)
	if len(touches) < 2 {
		t.Fatalf("no second touch after the cooldown: %v", sim.Events())
	}
	second := touches[1]
	if second.Kind != PassEventDeflect {
		t.Fatalf("second touch kind=%s, want deflect (ControlError=1 keeps miscontrolling)", second.Kind)
	}
	for _, e := range touches[1:] {
		if ex := t052ExactTime(e); ex < cooldown {
			t.Fatalf("touch at exact time %d before the cooldown %d", ex, cooldown)
		}
	}
	if ex := t052ExactTime(second); ex != cooldown {
		t.Fatalf("second touch exact time=%d, want the cooldown deadline %d", ex, cooldown)
	}
	t.Logf("B: interior deflect Frac=%.3f exactTime=%d cooldown=%d; SCRIPTED ball within reach -> next DEFLECT exactTime=%d == cooldown (physical gate, not publication timing)", def.Frac, t052ExactTime(def), cooldown, t052ExactTime(second))
}

// --- C: failed control then genuine subsequent eligibility ------------------

// TestT052MiscontrolRecoveryOtherParticipant reproduces the EXISTING T-039 seeded
// miscontrol->recovery fixture (pre-recorded seed 1, ControlError 0.5) and adds an
// ACTUAL second participant. The winner is MEASURED, never prescribed, and no
// controlError is mutated.
func TestT052MiscontrolRecoveryOtherParticipant(t *testing.T) {
	const seed = 1 // documented T-039 export fixture
	const controlError = 0.5
	release := func(sim *Sim) {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.72, TargetY: 0.50, SpeedMPS: 12.0, ControlError: controlError, Passer: -1})
	}
	run := func(runners []Runner) (*Sim, []PassEvent, []PassEvent) {
		sim := mustSim(t, LivePassConfig(), seed)
		sim.SetRunners(runners)
		release(sim)
		for i := 0; i < 400 && sim.Ball().Mode != BallHeld; i++ {
			sim.Step()
		}
		var def, ctl []PassEvent
		for _, e := range sim.Events() {
			switch e.Kind {
			case PassEventDeflect:
				def = append(def, e)
			case PassEventControl:
				ctl = append(ctl, e)
			}
		}
		return sim, def, ctl
	}

	// C1: documented single participant (reproduces the export fixture).
	_, def1, ctl1 := run([]Runner{PursuingRunner(0.56, 0.50, DefaultMotionParams(), 0)})
	if len(def1) == 0 || len(ctl1) == 0 {
		t.Fatalf("C1 documented fixture did not miscontrol then recover: deflects=%d controls=%d", len(def1), len(ctl1))
	}

	// C2: same scene + an ACTUAL second participant; measure the real winner.
	sim2, def2, ctl2 := run([]Runner{
		PursuingRunner(0.56, 0.50, DefaultMotionParams(), 0),
		PursuingRunner(0.56, 0.60, DefaultMotionParams(), 0),
	})
	if len(def2) == 0 || len(ctl2) == 0 {
		t.Fatalf("C2 two-participant scene did not miscontrol then recover: deflects=%d controls=%d", len(def2), len(ctl2))
	}
	rec := ctl2[len(ctl2)-1].Runner
	if rec != 0 && rec != 1 {
		t.Fatalf("recovery by runner %d, want one of the two participants", rec)
	}
	t.Logf("C: seed=%d ControlError=%.1f (pre-recorded); C1 deflect@%d exact=%d recovery@%d by runner %d; C2 (2 participants) deflects=%d recovery by runner %d (MEASURED, not prescribed)",
		seed, controlError, def1[0].ClockMS, t052ExactTime(def1[0]), ctl1[0].ClockMS, ctl1[0].Runner, len(def2), rec)
	if sim2.runners[rec].ReactAtMS != 0 {
		t.Fatalf("recovery participant had a nonzero reaction gate: %d", sim2.runners[rec].ReactAtMS)
	}
}

// --- D: held body movement every microstep ----------------------------------

// TestT052HeldBodyMovement pins that a holder keeps integrating and the ball
// follows owner+offset every microstep.
func TestT052HeldBodyMovement(t *testing.T) {
	cfg := LivePassConfig()
	sim := mustSim(t, cfg, 4)
	sim.SetRunners([]Runner{PursuingRunner(0.10, 0.50, DefaultMotionParams(), 0)})
	sim.ReleasePass(PassParams{StartX: 0.25, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for i := 0; i < 800; i++ {
		if ms, _ := sim.FirstContact(); ms >= 0 {
			break
		}
		sim.Step()
	}
	if ms, _ := sim.FirstContact(); ms < 0 {
		t.Fatal("holder never controlled the ball")
	}
	// SCRIPTED post-control far intent so the held body exercises integration.
	sim.SetIntent(0, 0.90, 0.50)
	t052LogRunner(t, "D held", sim, 0)
	prev := sim.runners[0]
	moved := 0
	for i := 0; i < 60; i++ {
		sim.Step()
		r := sim.runners[0]
		b := sim.Ball()
		if b.Mode != BallHeld || sim.contactRunner != 0 {
			t.Fatalf("ball not held by 0: mode=%s owner=%d", b.Mode, sim.contactRunner)
		}
		if d := DistM(prev.X, prev.Y, r.X, r.Y); d > 1e-9 {
			moved++
		}
		if math.Abs(b.X-(r.X+b.HoldDX)) > 1e-9 || math.Abs(b.Y-(r.Y+b.HoldDY)) > 1e-9 {
			t.Fatalf("held ball did not follow owner+offset")
		}
		prev = r
	}
	if moved != 60 {
		t.Fatalf("holder moved on %d/60 post-control substeps (want all)", moved)
	}
	t052RecorderPurity(t, cfg, 4, func(s *Sim) {
		s.SetRunners([]Runner{PursuingRunner(0.10, 0.50, DefaultMotionParams(), 0)})
		s.ReleasePass(PassParams{StartX: 0.25, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	}, 900, "D")
	t.Logf("D: held holder moved 60/60 substeps (scripted far intent); ball followed owner+offset; recorder pure")
}

// --- E: real second release --------------------------------------------------

// TestT052RealSecondRelease pins two real releases and the re-armed deadline.
func TestT052RealSecondRelease(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 5)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim.runners[0].ReactMS = 300
	sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	t052StepTo(sim, 600)
	sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	t052StepTo(sim, 700)
	clocks := t051ReleaseClocks(sim.Events())
	if len(clocks) != 2 || clocks[0] != 0 || clocks[1] != 600 {
		t.Fatalf("release clocks=%v, want [0 600]", clocks)
	}
	if sim.runners[0].ReactAtMS != 900 {
		t.Fatalf("ReactAtMS=%d, want 900", sim.runners[0].ReactAtMS)
	}
	t.Logf("E: two real releases [0 600]; ReactAtMS re-armed to 900")
}

// --- F: actual out ----------------------------------------------------------

// TestT052ActualOut pins a real pitch exit (no interception) and the next
// observation.
func TestT052ActualOut(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 6)
	sim.SetRunners([]Runner{t052Sensing(t, 0.95, 0.50, 200.0)})
	sim.ReleasePass(PassParams{StartX: 0.98, StartY: 0.50, TargetX: 1.05, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	t052StepTo(sim, 2000)
	if _, ok := t052FirstKind(sim, PassEventControl); ok {
		t.Fatalf("watcher intercepted the out ball: %v", sim.Events())
	}
	if sim.Ball().Mode != BallOut {
		t.Fatalf("ball mode=%s after exit, want out", sim.Ball().Mode)
	}
	out, ok := t052FirstKind(sim, PassEventOut)
	if !ok {
		t.Fatalf("no out event: %v", sim.Events())
	}
	if !(out.Frac > 0 && out.Frac <= 1) {
		t.Fatalf("out Frac=%v, want (0,1]", out.Frac)
	}
	o, _ := sim.Perception(0)
	if !o.Seen || o.Mode != BallOut {
		t.Fatalf("next observation not Out: %+v", o)
	}
	t.Logf("F: real out @%d exactTime=%d; no interception; observation Mode=out@%d", out.ClockMS, t052ExactTime(out), o.ClockMS)
}

// --- G: two-runner contest (deterministic winner) ---------------------------

// TestT052TwoRunnerContest pins that the winner is the physically earliest
// contestant and an identical-chord tie resolves to the lowest index.
func TestT052TwoRunnerContest(t *testing.T) {
	winner := func(invert bool) int {
		sim := mustSim(t, ScheduledLivePassConfig(100), 7)
		p := DefaultMotionParams()
		a := InertialRunner(0.45, 0.50, 0.50, 0.50, p)
		b := InertialRunner(0.60, 0.50, 0.50, 0.50, p)
		if invert {
			a, b = b, a
		}
		sim.SetRunners([]Runner{a, b})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
		for i := 0; i < 200; i++ {
			sim.Step()
			if _, r := sim.FirstContact(); r >= 0 {
				return r
			}
		}
		return -1
	}
	if got := winner(false); got != 0 {
		t.Fatalf("nearer contestant at index 0 did not win: %d", got)
	}
	if got := winner(true); got != 1 {
		t.Fatalf("nearer contestant at index 1 did not win: %d", got)
	}

	sim := mustSim(t, ScheduledLivePassConfig(100), 8)
	p := DefaultMotionParams()
	sim.SetRunners([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, p), InertialRunner(0.50, 0.50, 0.50, 0.50, p)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
	sim.Step()
	if _, r := sim.FirstContact(); r != 0 {
		t.Fatalf("identical-chord tie resolved to runner %d, want 0", r)
	}
	t.Logf("G: nearer contestant wins (0 then 1 on swap); identical-chord tie (SCRIPTED) -> lowest index 0")
}

// --- H: control-transition watchers (correct chronology) --------------------

// TestT052ControlTransitionWatchers pins that a physical toucher controls at an
// interior fraction while a near watcher is woken by the CONTROL transition (its
// release pending was already consumed at pre-step 0) and its pending is then
// consumed at pre-step 20 and dropped by the reaction gate; a far watcher is never
// woken and has no off-lattice sidecar record.
func TestT052ControlTransitionWatchers(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 9)
	toucher := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams()) // react 0
	nearX := 0.50 - 1.5/PitchLengthM                                         // 1.5 m from the toucher
	near := t050Local(nearX, 0.50, nearX, 0.50, 3.0, 300, true)
	far := t050Local(0.10, 0.10, 0.10, 0.10, 0.5, 300, true)
	if d := DistM(0.50, 0.50, nearX, 0.50); d < 1.0 || d >= 3.0 {
		t.Fatalf("near watcher displacement %.3f m not in [1,3)", d)
	}
	rec := mustRecorder(t, 256)
	sim.SetProvenanceRecorder(rec)
	sim.SetRunners([]Runner{toucher, near, far})
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))

	sim.Step() // 0 -> 20: interior control by the toucher; release pending consumed at pre-step 0
	ctrl, ok := t052FirstKind(sim, PassEventControl)
	if !ok || ctrl.Runner != 0 || !(ctrl.Frac > 0 && ctrl.Frac < 1) {
		t.Fatalf("expected an interior control by the toucher: %+v (events %v)", ctrl, sim.Events())
	}
	if !sim.runners[1].WakePending {
		t.Fatal("near watcher not woken by the control transition")
	}
	if sim.runners[2].WakePending {
		t.Fatal("far watcher woken by a near transition")
	}
	t052LogRunner(t, "H post-control", sim, 1)

	sim.Step() // 20 -> 40: pre-step 20 consumes the control pending, reaction-blocked
	if sim.runners[1].WakePending {
		t.Fatal("near pending not consumed at pre-step 20")
	}
	foundLocal := false
	for _, r := range rec.Records() {
		if r.Runner == 2 && r.ClockMS != 0 {
			t.Fatalf("far watcher has an off-lattice sidecar record: %+v", r)
		}
		if r.Runner == 1 && r.ClockMS == 20 && r.Reason == ReasonLocalEvent && r.GateOutcome == GateReactionBlocked {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Fatalf("no local-event reaction-blocked record at clock 20: %+v", rec.Records())
	}
	nearObs, _ := sim.Perception(1)
	if nearObs.Seen {
		t.Fatalf("near watcher observed before its reaction: %+v", nearObs)
	}
	t.Logf("H: toucher interior control @%d Frac=%.3f; near pending set by the control (release pending consumed @0), consumed+reaction-blocked @20 (local-event sidecar), no observation; far never woken/no off-lattice record", ctrl.ClockMS, ctrl.Frac)
}

// --- I: passer immunity vs scheduling ---------------------------------------

// TestT052PasserImmunity pins that the releasing passer cannot re-touch before
// immunityUntil (a physical gate).
func TestT052PasserImmunity(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 10)
	sim.SetRunners([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())})
	sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.55, TargetY: 0.50, SpeedMPS: 0.0, Passer: 0})
	t052StepTo(sim, 460)
	ms, r := sim.FirstContact()
	if ms != PasserImmunityMS || r != 0 {
		t.Fatalf("passer first contact=%d/%d, want %d/0", ms, r, PasserImmunityMS)
	}
	t.Logf("I: passer re-touch gated exactly to immunityUntil=%d (physical, not scheduling)", PasserImmunityMS)
}

// --- J: participant vs sidecar decision --------------------------------------

// TestT052ParticipantVsDecision pins that the sidecar describes only opted-in
// LOCAL runners' pre-step intents, not a plain participant's touch, with verbose
// pre/post state and actual provenance records.
func TestT052ParticipantVsDecision(t *testing.T) {
	// (1) Plain participant: no local runner -> ZERO sidecar records.
	plain := mustSim(t, ScheduledLivePassConfig(100), 11)
	plain.SetRunners([]Runner{PursuingRunner(0.10, 0.50, DefaultMotionParams(), 0)})
	plain.SetProvenanceRecorder(mustRecorder(t, 256))
	plain.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for i := 0; i < 800; i++ {
		if ms, _ := plain.FirstContact(); ms >= 0 {
			break
		}
		plain.Step()
	}
	if ms, _ := plain.FirstContact(); ms < 0 {
		t.Fatal("plain participant never controlled")
	}
	if n := plain.ProvenanceRecorder().Len(); n != 0 {
		t.Fatalf("sidecar recorded %d decisions for a plain participant", n)
	}

	// (2) Local-pursuit participant: records exist; last pre-contact record is a
	// chase, consistent with the participant's intent, at a pre-step clock.
	local := mustSim(t, ScheduledLivePassConfig(100), 12)
	local.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 200.0, 0, true)})
	recL := mustRecorder(t, 256)
	local.SetProvenanceRecorder(recL)
	local.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for i := 0; i < 800; i++ {
		if ms, _ := local.FirstContact(); ms >= 0 {
			break
		}
		local.Step()
	}
	ms, _ := local.FirstContact()
	if ms < 0 {
		t.Fatal("local participant never controlled")
	}
	if recL.Len() == 0 {
		t.Fatal("no sidecar records for a local-pursuit participant")
	}
	last := recL.Records()[recL.Len()-1]
	if !last.Applied || last.Policy != PolicyChase {
		t.Fatalf("last pre-contact record applied=%v policy=%s, want applied chase", last.Applied, last.Policy)
	}
	if last.ClockMS >= ms {
		t.Fatalf("record clock %d not a pre-step before contact %d", last.ClockMS, ms)
	}
	t052LogRunner(t, "J local pre-contact", local, 0)
	for i, r := range recL.Records() {
		if i >= 4 {
			break
		}
		t.Logf("J sidecar record[%d]: clock=%d reason=%s applied=%v policy=%s freshness=%s target=(%.4f,%.4f)", i, r.ClockMS, r.Reason, r.Applied, r.Policy, r.Freshness, r.TargetX, r.TargetY)
	}
	t.Logf("J: plain participant controlled with 0 sidecar records; local participant %d records (last chase@%d < contact @%d) -> sidecar = pre-step intent for LOCAL runners only", recL.Len(), last.ClockMS, ms)
}

// --- M: SCRIPTED dead observation -------------------------------------------

// TestT052ScriptedDeadObservation uses a SCRIPTED dead-ball SetBall, labelled
// because the current engine has no factual dead-ball mechanism other than out.
func TestT052ScriptedDeadObservation(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 21)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim.SetBall(Ball{Mode: BallDead, X: 0.51, Y: 0.50})
	sim.Step()
	o, _ := sim.Perception(0)
	if !o.Seen || o.Mode != BallDead {
		t.Fatalf("SCRIPTED dead observation not recorded: %+v", o)
	}
	t.Logf("M (SCRIPTED, no factual dead event in the engine): observation Mode=dead@%d", o.ClockMS)
}

// --- K: complete JSON continuations + sidecar tails -------------------------

// t052AssertTailWithSidecar steps straight and resumed in lockstep (fresh recorder
// on each), compares the complete engine Snapshot each substep and the sidecar
// record tails.
func t052AssertTailWithSidecar(t *testing.T, straight, resumed *Sim, horizon int, wantRecords bool, mid func(*Sim, int), label string) {
	t.Helper()
	rs := mustRecorder(t, 4096)
	rr := mustRecorder(t, 4096)
	straight.SetProvenanceRecorder(rs)
	resumed.SetProvenanceRecorder(rr)
	for straight.ClockMS() < horizon {
		if mid != nil {
			mid(straight, straight.ClockMS())
			mid(resumed, resumed.ClockMS())
		}
		straight.Step()
		resumed.Step()
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("%s @%d: engine diverged after resume", label, straight.ClockMS())
		}
	}
	if !reflect.DeepEqual(rs.Records(), rr.Records()) {
		t.Fatalf("%s: sidecar record tails differ", label)
	}
	if wantRecords && rs.Len() == 0 {
		t.Fatalf("%s: expected nonempty sidecar tail", label)
	}
}

// TestT052JSONContinuationTransitions pins complete engine JSON Resume across
// control/deflect/held/out/second-release transitions plus a real sidecar tail.
func TestT052JSONContinuationTransitions(t *testing.T) {
	scenes := []struct {
		name  string
		cfg   Config
		seed  uint64
		build func(*Sim)
	}{
		{"control", ScheduledLivePassConfig(100), 13, func(s *Sim) {
			s.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
			s.ReleasePass(t052InteriorPass(2.0, 0.0))
		}},
		{"deflect", ScheduledLivePassConfig(100), 14, func(s *Sim) {
			s.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
			s.ReleasePass(t052InteriorPass(2.0, 1.0))
		}},
		{"held", LivePassConfig(), 15, func(s *Sim) {
			s.SetRunners([]Runner{PursuingRunner(0.10, 0.50, DefaultMotionParams(), 0)})
			s.ReleasePass(PassParams{StartX: 0.25, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		}},
		{"out", ScheduledLivePassConfig(100), 16, func(s *Sim) {
			s.SetRunners([]Runner{t052Sensing(t, 0.95, 0.50, 200.0)})
			s.ReleasePass(PassParams{StartX: 0.98, StartY: 0.50, TargetX: 1.05, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		}},
		{"second-release", ScheduledLivePassConfig(100), 17, func(s *Sim) {
			s.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
			s.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		}},
	}
	for _, sc := range scenes {
		straight := mustSim(t, sc.cfg, sc.seed)
		sc.build(straight)
		split := 40
		if sc.name == "held" {
			split = 400 // past control
		}
		t052StepTo(straight, split)
		resumed := t046JSONResume(t, straight)
		mid := func(*Sim, int) {}
		if sc.name == "second-release" {
			mid = func(s *Sim, clock int) {
				if clock == 600 {
					s.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
				}
			}
		}
		horizon := 300
		if sc.name == "held" {
			horizon = 900
		}
		if sc.name == "second-release" {
			horizon = 800
		}
		t046AssertRuntimeTailsMid(t, straight, resumed, horizon, "t052-"+sc.name, mid)
		t.Logf("K: %s split=%d -> engine JSON tail to %d ms identical every substep", sc.name, split, horizon)
	}

	// Real sidecar tail: a local-pursuit participant with a mid-tail second release.
	straight := mustSim(t, ScheduledLivePassConfig(100), 18)
	straight.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 200.0, 0, true)})
	straight.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	t052StepTo(straight, 60)
	resumed := t046JSONResume(t, straight)
	mid := func(s *Sim, clock int) {
		if clock == 600 {
			s.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		}
	}
	t052AssertTailWithSidecar(t, straight, resumed, 800, true, mid, "t052-sidecar")
	t.Logf("K: real sidecar tail 60->800 ms identical record-for-record after JSON resume")
}

// --- L: active22 stays active ------------------------------------------------

// TestT052Active22StaysActive guards the benchmark window with the sidecar.
func TestT052Active22StaysActive(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t046BenchRunners(true))
	sim.SetProvenanceRecorder(mustRecorder(t, 8192))
	for w := 0; w < 2; w++ {
		t046RearmBench(sim)
		prev := sim.Runners()
		for k := 0; k < t040Window; k++ {
			sim.Step()
			if sim.Ball().Mode != BallRolling {
				t.Fatalf("window=%d: ball mode=%s at step %d", w, sim.Ball().Mode, k)
			}
			cur := sim.Runners()
			for i := range cur {
				if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
					t.Fatalf("window=%d: runner %d did not move at step %d", w, i, k)
				}
			}
			prev = cur
		}
	}
	sim.SetProvenanceRecorder(nil)
	t.Logf("L: active22 all bodies move every substep with the sidecar attached")
}
