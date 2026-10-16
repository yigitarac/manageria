package liveplay

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-055 integrated shell contract tests (isolated prototype evidence).
//
// The field/version/order/transition contract is fixed BEFORE this code in
// vault/05-Tasks/T055-Integrated-Contract.md. These tests exercise: opt-in gates,
// the factual journal (kinds/order/phases/last-touch/source mapping), clock
// timing at 10/20 ms plus fraction 0/1, the stable-ID tie, the disabled/opt-in
// engine-purity boundary, complete JSON Resume at every new transition and
// invalid-import rejection. Existing T-050/T-052 fixtures/helpers are reused; no
// runtime prediction/teamAI/default switch is touched.

// --- helpers ---------------------------------------------------------------

// t055AssignIDs returns a copy of rs with contiguous nonzero stable PlayerIDs.
func t055AssignIDs(rs []Runner) []Runner {
	out := append([]Runner(nil), rs...)
	for i := range out {
		out[i].PlayerID = uint64(i + 1)
	}
	return out
}

// t055EnableOrFatal enables the shell or fails.
func t055EnableOrFatal(t *testing.T, sim *Sim) {
	t.Helper()
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
}

// t055StripIntegrated removes the opt-in field for an engine-behaviour compare.
func t055StripIntegrated(s Snapshot) Snapshot {
	s.Integrated = nil
	return s
}

// t055IntegratedControlScene is a real release -> interior clean control with one
// stable PlayerID assigned BEFORE enable/release.
func t055IntegratedControlScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
	t055EnableOrFatal(t, sim)
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	return sim
}

// t055IntegratedDeflectScene is a real release -> interior forced miscontrol.
func t055IntegratedDeflectScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
	t055EnableOrFatal(t, sim)
	sim.ReleasePass(t052InteriorPass(2.0, 1.0))
	return sim
}

// t055IntegratedStopScene releases a slow ball that comes to rest short.
func t055IntegratedStopScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(t055AssignIDs([]Runner{t052Sensing(t, 0.95, 0.90, 0.01)}))
	t055EnableOrFatal(t, sim)
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
	return sim
}

// t055IntegratedOutScene releases a fast ball off the endline.
func t055IntegratedOutScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(t055AssignIDs([]Runner{t052Sensing(t, 0.05, 0.90, 0.01)}))
	t055EnableOrFatal(t, sim)
	sim.ReleasePass(PassParams{StartX: 0.90, StartY: 0.50, TargetX: 0.98, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1})
	return sim
}

// t055StepUntilTouch steps to the first control/deflect or the horizon.
func t055StepUntilTouch(sim *Sim, horizon int) {
	for sim.ClockMS() < horizon && len(t052Touches(sim)) == 0 {
		sim.Step()
	}
}

// t055AssertJournalReconciles checks the persisted journal against its source
// events and its own sequence, exactly as Resume validates.
func t055AssertJournalReconciles(t *testing.T, sim *Sim) IntegratedState {
	t.Helper()
	st, ok := sim.IntegratedState()
	if !ok {
		t.Fatal("integrated shell not enabled")
	}
	if st.NextSeq != len(st.Journal) {
		t.Fatalf("nextSeq %d != journal %d", st.NextSeq, len(st.Journal))
	}
	bySeq := map[int]PassEvent{}
	for _, e := range sim.Events() {
		bySeq[e.Seq] = e
	}
	for i := range st.Journal {
		f := st.Journal[i]
		if f.Seq != i {
			t.Fatalf("fact %d has seq %d", i, f.Seq)
		}
		e, ok := bySeq[f.SourceSeq]
		if !ok {
			t.Fatalf("fact %d sourceSeq %d has no source event", i, f.SourceSeq)
		}
		fk, ok := factKindOf(e.Kind)
		if !ok || fk != f.Kind {
			t.Fatalf("fact %d kind %s vs source kind %s", i, f.Kind, e.Kind)
		}
		if f.ClockMS != e.ClockMS || f.Fraction != e.Frac {
			t.Fatalf("fact %d clock/fraction mismatch", i)
		}
		if f.X != e.BallX || f.Y != e.BallY || f.VXM != e.VXM || f.VYM != e.VYM ||
			f.OutVXM != e.OutVXM || f.OutVYM != e.OutVYM {
			t.Fatalf("fact %d ball state does not match source", i)
		}
	}
	return st
}

// --- 1. opt-in gates and validation ----------------------------------------

// TestT055EnableGates pins the enable contract: only with LivePass, only at clock
// 0 before any event, and only with unique nonzero finite IDs.
func TestT055EnableGates(t *testing.T) {
	// Not LivePass.
	legacy := mustSim(t, DefaultConfig(), 1)
	legacy.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())}))
	if err := legacy.EnableIntegrated(); err != ErrIntegratedNotLivePass {
		t.Fatalf("legacy enable err=%v, want ErrIntegratedNotLivePass", err)
	}

	// Already enabled.
	sim := t055IntegratedControlScene(t, 1)
	if err := sim.EnableIntegrated(); err != ErrIntegratedAlreadyOn {
		t.Fatalf("double enable err=%v, want ErrIntegratedAlreadyOn", err)
	}

	// Zero / duplicate IDs.
	z := mustSim(t, LivePassConfig(), 1)
	z.SetRunners([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())})
	if err := z.EnableIntegrated(); !errors.Is(err, ErrIntegratedPlayerID) {
		t.Fatalf("zero id enable err=%v, want ErrIntegratedPlayerID", err)
	}
	d := mustSim(t, LivePassConfig(), 1)
	r0 := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	r1 := InertialRunner(0.4, 0.5, 0.4, 0.5, DefaultMotionParams())
	r0.PlayerID, r1.PlayerID = 7, 7
	d.SetRunners([]Runner{r0, r1})
	if err := d.EnableIntegrated(); !errors.Is(err, ErrIntegratedPlayerID) {
		t.Fatalf("duplicate id enable err=%v, want ErrIntegratedPlayerID", err)
	}

	// Non-finite body.
	nf := mustSim(t, LivePassConfig(), 1)
	bad := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	bad.PlayerID = 1
	bad.X = math.NaN()
	nf.SetRunners([]Runner{bad})
	if err := nf.EnableIntegrated(); !errors.Is(err, ErrIntegratedBody) {
		t.Fatalf("non-finite enable err=%v, want ErrIntegratedBody", err)
	}

	// After a release/event.
	after := mustSim(t, LivePassConfig(), 1)
	after.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())}))
	after.ReleasePass(t052InteriorPass(2.0, 0.0))
	if err := after.EnableIntegrated(); !errors.Is(err, ErrIntegratedNotInitial) {
		t.Fatalf("late enable err=%v, want ErrIntegratedNotInitial", err)
	}

	// Local-pursuit/wakeup/perception context is validated at ENABLE time exactly
	// as Resume does, so a validly enabled state can always JSON Resume.
	lw := mustSim(t, LivePassConfig(), 1) // scheduler disabled
	lwR := t050Local(0.5, 0.5, 0.5, 0.5, 3.0, 0, true)
	lwR.PlayerID = 1
	lw.SetRunners([]Runner{lwR})
	if err := lw.EnableIntegrated(); !errors.Is(err, ErrLocalWakeupNoScheduler) && !errors.Is(err, ErrLocalPursuitNoScheduler) {
		t.Fatalf("local runner with a disabled scheduler enable err=%v", err)
	}

	// Invalid raw perception record rejected at enable.
	pn := mustSim(t, LivePassConfig(), 1)
	pnR := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	pnR.PlayerID = 1
	pnR.Perception = &BallObservation{RadiusM: math.NaN()}
	pn.SetRunners([]Runner{pnR})
	if err := pn.EnableIntegrated(); !errors.Is(err, ErrSenseRadius) {
		t.Fatalf("bad perception enable err=%v, want ErrSenseRadius", err)
	}

	// Non-finite / out-of-range control budget rejected at enable.
	cn := mustSim(t, LivePassConfig(), 1)
	cn.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())}))
	cn.controlError = math.NaN()
	if err := cn.EnableIntegrated(); !errors.Is(err, ErrIntegratedBody) {
		t.Fatalf("nan controlError enable err=%v, want ErrIntegratedBody", err)
	}
	crange := mustSim(t, LivePassConfig(), 1)
	crange.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())}))
	crange.controlError = 1.5
	if err := crange.EnableIntegrated(); !errors.Is(err, ErrIntegratedBody) {
		t.Fatalf("high controlError enable err=%v, want ErrIntegratedBody", err)
	}

	// Non-finite ReachM rejected at enable by the body check.
	rn := mustSim(t, LivePassConfig(), 1)
	rnR := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	rnR.PlayerID = 1
	rnR.ReachM = math.Inf(1)
	rn.SetRunners([]Runner{rnR})
	if err := rn.EnableIntegrated(); !errors.Is(err, ErrIntegratedBody) {
		t.Fatalf("inf ReachM enable err=%v, want ErrIntegratedBody", err)
	}

	t.Logf("T-055 enable gates: LivePass-only, clock0-before-event, unique nonzero finite IDs, ReachM/ControlError finiteness, and local-pursuit/wakeup/perception context all enforced")
}

// --- 2. journal transitions, phases, order, source mapping -----------------

// TestT055JournalReleaseControl pins the real release->interior-control journal:
// ordered facts, phases, last touch, source mapping and (implicitly) no path fact.
func TestT055JournalReleaseControl(t *testing.T) {
	sim := t055IntegratedControlScene(t, 1)
	t055LogMoment(t, "release-control.initial", sim)
	sim.Step()
	t055LogMoment(t, "release-control.final", sim)
	st := t055AssertJournalReconciles(t, sim)
	if len(st.Journal) != 2 {
		t.Fatalf("journal %+v, want release+control", st.Journal)
	}
	rel, ctl := st.Journal[0], st.Journal[1]
	if rel.Kind != FactRelease || rel.PhaseBefore != PhaseDead || rel.PhaseAfter != PhaseOpen || rel.PlayerID != 0 {
		t.Fatalf("release fact wrong: %+v", rel)
	}
	if rel.StepStartMS != rel.ClockMS || rel.Fraction != 0 {
		t.Fatalf("release timing must be StepStart==Clock and frac 0: %+v", rel)
	}
	if ctl.Kind != FactControl || ctl.PhaseBefore != PhaseOpen || ctl.PhaseAfter != PhaseOpen || ctl.PlayerID != 1 {
		t.Fatalf("control fact wrong: %+v", ctl)
	}
	if !(ctl.Fraction > 0 && ctl.Fraction < 1) {
		t.Fatalf("control fraction %v not interior", ctl.Fraction)
	}
	if st.Phase != PhaseOpen || st.LastTouchID != 1 {
		t.Fatalf("state phase/lastTouch %s/%d", st.Phase, st.LastTouchID)
	}
	for _, e := range sim.Events() {
		if e.Kind == PassEventPath {
			t.Fatalf("unexpected path event before contact: %+v", e)
		}
	}
	t.Logf("T-055 release->control: journal %s(pid0)->%s(pid1) frac=%.4f; lastTouch=1", rel.Kind, ctl.Kind, ctl.Fraction)
}

// TestT055JournalDeflectStopOut pins deflect/stop/out transitions and that a
// second release is allowed from StoppedOut.
func TestT055JournalDeflectStopOut(t *testing.T) {
	// Deflect.
	def := t055IntegratedDeflectScene(t, 2)
	t055StepUntilTouch(def, 2000)
	t055LogMoment(t, "deflect.final", def)
	st := t055AssertJournalReconciles(t, def)
	if len(st.Journal) < 2 || st.Journal[1].Kind != FactDeflect || st.Journal[1].PlayerID != 1 {
		t.Fatalf("deflect journal wrong: %+v", st.Journal)
	}
	if st.LastTouchID != 1 || st.Phase != PhaseOpen {
		t.Fatalf("deflect state %s/%d", st.Phase, st.LastTouchID)
	}

	// Stop stays live (phase Open, Loose).
	stop := t055IntegratedStopScene(t, 3)
	for stop.ClockMS() < 2500 {
		stop.Step()
	}
	ss := t055AssertJournalReconciles(t, stop)
	last := ss.Journal[len(ss.Journal)-1]
	if last.Kind != FactStop || last.PhaseAfter != PhaseOpen {
		t.Fatalf("stop journal wrong: %+v", last)
	}
	if stop.Ball().Mode != BallLoose {
		t.Fatalf("stop ball mode=%s, want loose", stop.Ball().Mode)
	}
	if ss.LastTouchID != 0 {
		t.Fatalf("stop lastTouch=%d, want retained 0", ss.LastTouchID)
	}

	// Out stops live play; a second release is allowed from StoppedOut.
	out := t055IntegratedOutScene(t, 4)
	for out.ClockMS() < 1000 && out.Ball().Mode != BallOut {
		out.Step()
	}
	t055LogMoment(t, "out.final", out)
	os := t055AssertJournalReconciles(t, out)
	olast := os.Journal[len(os.Journal)-1]
	if olast.Kind != FactOut || olast.PhaseAfter != PhaseStoppedOut || os.Phase != PhaseStoppedOut {
		t.Fatalf("out journal wrong: %+v (phase %s)", olast, os.Phase)
	}
	out.ReleasePass(PassParams{StartX: 0.40, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	os2 := t055AssertJournalReconciles(t, out)
	rel2 := os2.Journal[len(os2.Journal)-1]
	if rel2.Kind != FactRelease || rel2.PhaseBefore != PhaseStoppedOut || rel2.PhaseAfter != PhaseOpen {
		t.Fatalf("second release wrong: %+v", rel2)
	}
	t.Logf("T-055 transitions: deflect pid=1 Open; stop stays Loose/Open; out -> StoppedOut; second release StoppedOut->Open")
}

// --- 3. clock timing at 10 ms / 20 ms and fraction 0/1 ---------------------

// TestT055ClockTiming pins release StepStart==Clock/frac0, interior contact
// StepStart=Clock-substep, and the fraction 0/1 boundaries at 10 ms and 20 ms.
func TestT055ClockTiming(t *testing.T) {
	for _, sub := range []int{10, 20} {
		cfg := Config{SubstepMS: sub, DecelMPS2: RollDecelMPS2, LivePass: true}
		sim := mustSim(t, cfg, 1)
		sim.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
		t055EnableOrFatal(t, sim)
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
		t055StepUntilTouch(sim, 2000)
		st := t055AssertJournalReconciles(t, sim)
		rel, ctl := st.Journal[0], st.Journal[len(st.Journal)-1]
		if rel.StepStartMS != rel.ClockMS || rel.Fraction != 0 {
			t.Fatalf("sub=%d release timing %+v", sub, rel)
		}
		if ctl.StepStartMS != ctl.ClockMS-sub || !(ctl.Fraction > 0 && ctl.Fraction < 1) {
			t.Fatalf("sub=%d interior contact timing %+v", sub, ctl)
		}

		// fraction 0: already inside reach at the pre-step.
		f0 := mustSim(t, cfg, 2)
		f0.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
		f0.SetBall(Ball{Mode: BallRolling, X: 0.505, Y: 0.50, VXM: 2.0})
		t055EnableOrFatal(t, f0)
		f0.Step()
		s0 := t055AssertJournalReconciles(t, f0)
		c0 := s0.Journal[len(s0.Journal)-1]
		if c0.Fraction != 0 || c0.StepStartMS != c0.ClockMS-sub {
			t.Fatalf("sub=%d frac0 contact %+v", sub, c0)
		}

		// fraction 1: gated to the step end by the cooldown.
		f1 := mustSim(t, cfg, 3)
		f1.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
		f1.SetBall(Ball{Mode: BallRolling, X: 0.505, Y: 0.50, VXM: 2.0})
		f1.runners[0].CooldownUntilMS = sub
		t055EnableOrFatal(t, f1)
		f1.Step()
		s1 := t055AssertJournalReconciles(t, f1)
		c1 := s1.Journal[len(s1.Journal)-1]
		if c1.Fraction != 1 || c1.StepStartMS != c1.ClockMS-sub {
			t.Fatalf("sub=%d frac1 contact %+v", sub, c1)
		}
	}
	t.Logf("T-055 timing: 10 ms and 20 ms clocks; release frac0/StepStart==Clock; interior/frac0/frac1 contacts have StepStart=Clock-substep")
}

// --- 4. stable identity tie -------------------------------------------------

// t055TieRun returns the winning runner's slot and (when enabled) its PlayerID.
func t055TieRun(t *testing.T, enabled bool, idA, idB uint64) (int, uint64) {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 3)
	off := 0.5 / PitchWidthM
	r0 := InertialRunner(0.50, 0.50-off, 0.50, 0.50-off, DefaultMotionParams())
	r1 := InertialRunner(0.50, 0.50+off, 0.50, 0.50+off, DefaultMotionParams())
	r0.PlayerID, r1.PlayerID = idA, idB
	sim.SetRunners([]Runner{r0, r1})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.50 - 0.9/PitchLengthM, Y: 0.50, VXM: 2.0})
	if enabled {
		t055EnableOrFatal(t, sim)
	}
	t055StepUntilTouch(sim, 4000)
	touches := t052Touches(sim)
	if len(touches) == 0 {
		t.Fatalf("tie run: no contact")
	}
	slot := touches[0].Runner
	if !enabled {
		return slot, 0
	}
	st, _ := sim.IntegratedState()
	if len(st.Journal) == 0 || st.Journal[0].Kind != FactControl {
		t.Fatalf("tie run: journal %+v", st.Journal)
	}
	return slot, st.Journal[0].PlayerID
}

// TestT055StableIdentityTie pins the deterministic tie: opt-in equal-time contact
// uses the lowest PlayerID (roster-order independent); disabled keeps the lowest
// slot; a strictly earlier exact time outranks the id.
func TestT055StableIdentityTie(t *testing.T) {
	slot, id := t055TieRun(t, true, 2, 1)
	if id != 1 || slot != 1 {
		t.Fatalf("enabled tie (2,1): slot=%d id=%d, want slot1 id1", slot, id)
	}
	slot, id = t055TieRun(t, true, 1, 2)
	if id != 1 || slot != 0 {
		t.Fatalf("enabled tie (1,2): slot=%d id=%d, want slot0 id1", slot, id)
	}
	slot, _ = t055TieRun(t, false, 2, 1)
	if slot != 0 {
		t.Fatalf("disabled tie: slot=%d, want lowest slot 0", slot)
	}

	// Different exact times outrank the id: the nearer runner (higher id 2) wins.
	sim := mustSim(t, LivePassConfig(), 3)
	r0 := InertialRunner(0.52, 0.50, 0.52, 0.50, DefaultMotionParams())
	r1 := InertialRunner(0.49, 0.50, 0.49, 0.50, DefaultMotionParams())
	r0.PlayerID, r1.PlayerID = 1, 2
	sim.SetRunners([]Runner{r0, r1})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.50 - 1.5/PitchLengthM, Y: 0.50, VXM: 2.0})
	t055EnableOrFatal(t, sim)
	t055StepUntilTouch(sim, 4000)
	st, _ := sim.IntegratedState()
	if len(st.Journal) == 0 || st.Journal[0].PlayerID != 2 {
		t.Fatalf("earlier-time winner journal %+v, want pid 2", st.Journal)
	}
	t.Logf("T-055 tie: opt-in equal-time contact -> lowest PlayerID 1 (roster-order independent); disabled -> lowest slot 0; earlier exact time outranks id")
}

// --- 5. engine purity / observation boundary -------------------------------

// TestT055EnginePurity proves the journal is never a decision input and the
// reaction/cooldown/immunity gates are unchanged: a scene run with and without
// the shell (same stable IDs) is bit-identical every substep after removing the
// shell field, and the RNG draws are unchanged.
func TestT055EnginePurity(t *testing.T) {
	build := func(enabled bool) *Sim {
		sim := mustSim(t, ScheduledLivePassConfig(100), 9)
		toucher := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
		nearX := 0.50 - 1.5/PitchLengthM
		near := t050Local(nearX, 0.50, nearX, 0.50, 3.0, 300, true)
		far := t050Local(0.10, 0.10, 0.10, 0.10, 0.5, 300, true)
		sim.SetRunners(t055AssignIDs([]Runner{toucher, near, far}))
		if enabled {
			t055EnableOrFatal(t, sim)
		}
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
		return sim
	}
	plain := build(false)
	on := build(true)
	if plain.RNGState() != on.RNGState() {
		t.Fatal("enable changed the initial RNG state")
	}
	sawControl := false
	for i := 0; i < 200; i++ {
		plain.Step()
		on.Step()
		if !reflect.DeepEqual(t055StripIntegrated(plain.Snapshot()), t055StripIntegrated(on.Snapshot())) {
			t.Fatalf("shell changed engine state at step %d", i)
		}
		if plain.RNGState() != on.RNGState() {
			t.Fatalf("shell changed RNG draws at step %d", i)
		}
		if i == 199 {
			st, _ := on.IntegratedState()
			for _, f := range st.Journal {
				if f.Kind == FactControl {
					sawControl = true
				}
			}
		}
	}
	if !sawControl {
		t.Fatal("no control fact in the shell run")
	}
	// Explicit gate equality.
	pr := plain.Runners()
	orr := on.Runners()
	for i := range pr {
		if pr[i].ReactAtMS != orr[i].ReactAtMS || pr[i].CooldownUntilMS != orr[i].CooldownUntilMS || pr[i].WakePending != orr[i].WakePending {
			t.Fatalf("runner %d gate state changed (react/cooldown/pending)", i)
		}
	}
	if plain.Snapshot().ImmunityUntil != on.Snapshot().ImmunityUntil {
		t.Fatal("immunity changed")
	}
	t.Logf("T-055 purity: disabled vs opt-in bit-identical 200 substeps (state+RNG); reaction/cooldown/immunity/pending unchanged; journal is not a decision input")
}

// --- 6. complete JSON Resume at new transitions ----------------------------

// t055Active22Build builds the T-053 active22 fixture with stable IDs and an
// optional shell enabled BEFORE the initial release.
func t055Active22Build(t *testing.T, seed uint64, control, enabled bool) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	sim.SetRunners(t055AssignIDs(t053Runners(control)))
	if enabled {
		t055EnableOrFatal(t, sim)
	}
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
	return sim
}

// t055LogMoment logs full copied JSON moment evidence (snapshot/journal/events/
// samples) for one bounded critical clock.
func t055LogMoment(t *testing.T, label string, sim *Sim) {
	t.Helper()
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	jb, _ := json.Marshal(mustIntegratedJournal(t, sim))
	eb, _ := json.Marshal(sim.Events())
	sb, _ := json.Marshal(sim.Samples())
	t.Logf("T-055 moment %s @%d snapshot=%s", label, sim.ClockMS(), blob)
	t.Logf("T-055 moment %s journal=%s", label, jb)
	t.Logf("T-055 moment %s events=%s", label, eb)
	t.Logf("T-055 moment %s samples=%s", label, sb)
}

// t055AssertSplitPrecondition pins the ACTUAL state at each declared split clock.
func t055AssertSplitPrecondition(t *testing.T, label string, control bool, split int, sim *Sim) {
	t.Helper()
	touches := t052Touches(sim)
	rel := t051ReleaseClocks(sim.Events())
	if control {
		switch split {
		case 20:
			if len(touches) == 0 || touches[0].Kind != PassEventControl {
				t.Fatalf("%s split 20: expected a control contact, got %+v", label, touches)
			}
		case 200:
			if sim.Ball().Mode != BallHeld {
				t.Fatalf("%s split 200: ball mode %s, want held", label, sim.Ball().Mode)
			}
		case 980:
			if len(rel) != 1 || len(touches) == 0 {
				t.Fatalf("%s split 980: releases %v touches %d, want pre-second-release with a control", label, rel, len(touches))
			}
		case 1020:
			if len(rel) != 2 || rel[1] != t053SecondRelMS {
				t.Fatalf("%s split 1020: releases %v, want [0 %d]", label, rel, t053SecondRelMS)
			}
		}
		return
	}
	switch split {
	case 20, 900:
		if len(touches) != 0 {
			t.Fatalf("%s split %d: unexpected touch before the interior deflect: %+v", label, split, touches)
		}
	case 1100:
		if len(touches) == 0 || sim.runners[0].CooldownUntilMS <= sim.ClockMS() {
			t.Fatalf("%s split 1100: deflect/cooldown not active (%+v, cooldown %d)", label, touches, sim.runners[0].CooldownUntilMS)
		}
	}
}

// t055JSONOneSplit resumes one active22 window at split through COMPLETE JSON and
// proves: persisted journal AND event prefix preserved; regenerated resumed samples
// equal the straight sample tail; every subsequent Snapshot equal; appended
// absolute event/journal tails equal; a predeclared future fact/event tail is
// nonempty; post-transition splits invent no facts; and a real reattached sidecar
// tail is equal (and nonempty where predeclared).
func t055JSONOneSplit(t *testing.T, label string, seed uint64, control bool, split int, mid func(*Sim, int), futureFacts, wantRecords bool) {
	t.Helper()
	const end = 1900
	straight := t055Active22Build(t, seed, control, true)
	t053StepToWithMid(straight, split, mid)
	if straight.ClockMS() != split {
		t.Fatalf("%s: split %d reached %d", label, split, straight.ClockMS())
	}
	t055AssertSplitPrecondition(t, label, control, split, straight)

	jAtSplit := len(mustIntegratedJournal(t, straight))
	evAtSplit := len(straight.Events())
	sampAtSplit := len(straight.Samples())

	resumed := t046JSONResume(t, straight)
	if !resumed.IntegratedEnabled() {
		t.Fatalf("%s split %d: resume lost the shell", label, split)
	}
	if !reflect.DeepEqual(mustIntegratedJournal(t, resumed), mustIntegratedJournal(t, straight)) {
		t.Fatalf("%s split %d: persisted journal not preserved through JSON", label, split)
	}
	if !reflect.DeepEqual(resumed.Events(), straight.Events()) {
		t.Fatalf("%s split %d: persisted event prefix not preserved through JSON", label, split)
	}
	if len(resumed.Samples()) != 0 {
		t.Fatalf("%s split %d: resumed samples not regenerated from the split", label, split)
	}

	srec := mustRecorder(t, 8192)
	straight.SetProvenanceRecorder(srec)
	rrec := mustRecorder(t, 8192)
	resumed.SetProvenanceRecorder(rrec)
	for straight.ClockMS() < end {
		if mid != nil {
			mid(straight, straight.ClockMS())
			mid(resumed, resumed.ClockMS())
		}
		straight.Step()
		resumed.Step()
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("%s split %d: complete snapshot diverged at %d", label, split, straight.ClockMS())
		}
	}

	// Regenerated samples: exact absolute tail from the split.
	if !reflect.DeepEqual(resumed.Samples(), straight.Samples()[sampAtSplit:]) {
		t.Fatalf("%s split %d: resumed samples differ from the straight absolute tail", label, split)
	}
	// Appended absolute event/journal tails (both carry the persisted prefix).
	evTail := straight.Events()[evAtSplit:]
	reTail := resumed.Events()[evAtSplit:]
	if !reflect.DeepEqual(evTail, reTail) {
		t.Fatalf("%s split %d: appended event tail differs after resume", label, split)
	}
	jTail := mustIntegratedJournal(t, straight)[jAtSplit:]
	rjTail := mustIntegratedJournal(t, resumed)[jAtSplit:]
	if !reflect.DeepEqual(jTail, rjTail) {
		t.Fatalf("%s split %d: appended journal tail differs after resume", label, split)
	}
	if futureFacts && (len(evTail) == 0 || len(jTail) == 0) {
		t.Fatalf("%s split %d: predeclared future fact/event tail is empty (events=%d facts=%d)", label, split, len(evTail), len(jTail))
	}
	// Stable post-transition split: full persisted journal preserved exactly.
	if !reflect.DeepEqual(mustIntegratedJournal(t, straight), mustIntegratedJournal(t, resumed)) {
		t.Fatalf("%s split %d: complete persisted journal not preserved", label, split)
	}
	if !reflect.DeepEqual(srec.Records(), rrec.Records()) {
		t.Fatalf("%s split %d: recorder tail differs after resume", label, split)
	}
	if wantRecords && len(rrec.Records()) == 0 {
		t.Fatalf("%s split %d: predeclared sidecar record tail is empty", label, split)
	}
	t.Logf("T-055 JSON %s split=%d -> %d: snapshot equal every substep; journal prefix %d preserved; appended facts %d events %d samples %d records %d; predeclared future=%v sidecar=%v",
		label, split, end, jAtSplit, len(jTail), len(evTail), len(resumed.Samples()), len(rrec.Records()), futureFacts, wantRecords)
	if wantRecords {
		t.Logf("T-055 JSON %s split=%d sidecar records: %+v", label, split, rrec.Records())
	}
}

// TestT055JSONResumeTails covers pre/post contact, held, pre/post second release,
// pre-transition and cooldown splits, plus pre/post OUT splits, all through
// complete JSON Resume.
func TestT055JSONResumeTails(t *testing.T) {
	// control: 20 contact, 200 held, 980 BEFORE second release (future release at
	// 1000), 1020 AFTER it (no future fact); deflect: 20 pre, 900 BEFORE the
	// interior deflect (future), 1100 cooldown.
	t055JSONOneSplit(t, "control", t053ControlSeed, true, 20, t053MidSecondRelease, false, false)
	t055JSONOneSplit(t, "control", t053ControlSeed, true, 200, t053MidSecondRelease, false, false)
	t055JSONOneSplit(t, "control", t053ControlSeed, true, 980, t053MidSecondRelease, true, false)
	t055JSONOneSplit(t, "control", t053ControlSeed, true, 1020, t053MidSecondRelease, false, false)
	t055JSONOneSplit(t, "deflect", t053DeflectSeed, false, 20, nil, false, false)
	t055JSONOneSplit(t, "deflect", t053DeflectSeed, false, 900, nil, true, true)
	t055JSONOneSplit(t, "deflect", t053DeflectSeed, false, 1100, nil, false, false)
	t055OutResumeSplits(t)
}

// t055OutResumeSplits resumes an out scene at a pre-out and a post-out split,
// proves post-out validity and that a second release from StoppedOut survives.
func t055OutResumeSplits(t *testing.T) {
	// pre-out split (rolling), then out happens in the resumed tail.
	pre := t055IntegratedOutScene(t, 6)
	for pre.ClockMS() < 100 {
		pre.Step()
	}
	if pre.Ball().Mode == BallOut {
		t.Fatal("pre-out split already out")
	}
	rpre := t046JSONResume(t, pre)
	for pre.ClockMS() < 600 {
		pre.Step()
		rpre.Step()
		if !reflect.DeepEqual(pre.Snapshot(), rpre.Snapshot()) {
			t.Fatalf("pre-out resume diverged at %d", pre.ClockMS())
		}
	}
	sp, _ := pre.IntegratedState()
	if pre.Ball().Mode != BallOut || sp.Phase != PhaseStoppedOut || sp.Journal[len(sp.Journal)-1].Kind != FactOut {
		t.Fatalf("pre-out final state wrong: mode=%s phase=%s", pre.Ball().Mode, sp.Phase)
	}

	// post-out split (already out), then a real second release from StoppedOut.
	post := t055IntegratedOutScene(t, 6)
	for post.ClockMS() < 600 {
		post.Step()
	}
	if post.Ball().Mode != BallOut {
		t.Fatal("post-out split not out")
	}
	rpost := t046JSONResume(t, post)
	if !rpost.IntegratedEnabled() {
		t.Fatal("post-out resume lost the shell")
	}
	post.ReleasePass(PassParams{StartX: 0.40, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	rpost.ReleasePass(PassParams{StartX: 0.40, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for post.ClockMS() < 800 {
		post.Step()
		rpost.Step()
		if !reflect.DeepEqual(post.Snapshot(), rpost.Snapshot()) {
			t.Fatalf("post-out resume diverged at %d", post.ClockMS())
		}
	}
	s2, _ := post.IntegratedState()
	last := s2.Journal[len(s2.Journal)-1]
	if last.Kind != FactRelease || last.PhaseBefore != PhaseStoppedOut || s2.Phase != PhaseOpen {
		t.Fatalf("post-out second release wrong: %+v phase=%s", last, s2.Phase)
	}
	t.Logf("T-055 JSON OUT: pre-out split=100 and post-out split=600 -> 800 snapshot equal every substep; final fact %s, phase %s", last.Kind, s2.Phase)
}

func mustIntegratedJournal(t *testing.T, sim *Sim) []FactEvent {
	t.Helper()
	st, ok := sim.IntegratedState()
	if !ok {
		t.Fatalf("integrated shell not enabled")
	}
	return st.Journal
}

// --- 7. invalid imports and clone purity -----------------------------------

// TestT055InvalidImports rejects every declared inconsistency — including snapshots
// where the journal AND the source event log were edited together consistently —
// and proves the accessor/snapshot/resume clones cannot alias the live journal.
func TestT055InvalidImports(t *testing.T) {
	// Two real bases: one with a control fact AND post-release path events, one
	// with a deflect fact and path events.
	ctlSim := t055Active22Build(t, t053ControlSeed, true, true)
	t053StepToWithMid(ctlSim, 1200, t053MidSecondRelease)
	ctlBase := ctlSim.Snapshot()
	if _, err := Resume(ctlBase); err != nil {
		t.Fatalf("control baseline resume should succeed: %v", err)
	}
	defSim := t055Active22Build(t, t053DeflectSeed, false, true)
	t053StepToWithMid(defSim, 1100, nil)
	defBase := defSim.Snapshot()
	if _, err := Resume(defBase); err != nil {
		t.Fatalf("deflect baseline resume should succeed: %v", err)
	}
	ctlJournal := mustIntegratedJournal(t, ctlSim)
	if len(ctlJournal) < 2 {
		t.Fatalf("control baseline has %d facts, want >=2", len(ctlJournal))
	}

	mut := func(base Snapshot, f func(*Snapshot)) Snapshot {
		s := deepCopySnapshot(t, base)
		f(&s)
		return s
	}
	reject := func(label string, s Snapshot) {
		t.Helper()
		if _, err := Resume(s); err == nil {
			t.Fatalf("%s: Resume accepted an inconsistent snapshot", label)
		}
	}

	type row struct {
		label string
		base  Snapshot
		f     func(*Snapshot)
	}
	rows := []row{
		{"snapshotVersion", ctlBase, func(s *Snapshot) { s.Integrated.SnapshotVersion = 99 }},
		{"modelVersion", ctlBase, func(s *Snapshot) { s.Integrated.ModelVersion = 99 }},
		{"eventSchemaVersion", ctlBase, func(s *Snapshot) { s.Integrated.EventSchemaVersion = 99 }},
		{"rulebookVersion", ctlBase, func(s *Snapshot) { s.Integrated.RulebookVersion = "x" }},
		{"notLivePass", ctlBase, func(s *Snapshot) { s.Cfg.LivePass = false }},
		{"invalidBallMode", ctlBase, func(s *Snapshot) { s.Ball.Mode = 99 }},
		{"invalidMotion", ctlBase, func(s *Snapshot) { s.Runners[0].Motion = 99 }},
		{"nanReach", ctlBase, func(s *Snapshot) { s.Runners[1].ReachM = math.NaN() }},
		{"nanControlError", ctlBase, func(s *Snapshot) { s.ControlError = math.NaN() }},
		{"highControlError", ctlBase, func(s *Snapshot) { s.ControlError = 1.5 }},
		{"negControlError", ctlBase, func(s *Snapshot) { s.ControlError = -0.5 }},
		{"zeroId", ctlBase, func(s *Snapshot) { s.Runners[0].PlayerID = 0 }},
		{"dupId", ctlBase, func(s *Snapshot) { s.Runners[1].PlayerID = s.Runners[0].PlayerID }},
		{"nanBall", ctlBase, func(s *Snapshot) { s.Ball.X = math.NaN() }},
		{"nanBody", ctlBase, func(s *Snapshot) { s.Runners[0].IntentY = math.Inf(1) }},
		{"negativeDeadline", ctlBase, func(s *Snapshot) { s.Runners[0].ReactAtMS = -1 }},
		{"nextSeq", ctlBase, func(s *Snapshot) { s.Integrated.NextSeq = 99 }},
		{"journalSeqGap", ctlBase, func(s *Snapshot) { s.Integrated.Journal[0].Seq = 5 }},
		{"sourceSeqMissing", ctlBase, func(s *Snapshot) { s.Integrated.Journal[0].SourceSeq = 999 }},
		{"unorderedSourceSeq", ctlBase, func(s *Snapshot) {
			a, b := s.Integrated.Journal[0].SourceSeq, s.Integrated.Journal[1].SourceSeq
			s.Integrated.Journal[0].SourceSeq, s.Integrated.Journal[1].SourceSeq = b, a
		}},
		{"missingFact", ctlBase, func(s *Snapshot) {
			s.Integrated.Journal = s.Integrated.Journal[:len(s.Integrated.Journal)-1]
			s.Integrated.NextSeq = len(s.Integrated.Journal)
		}},
		{"missingEvent", ctlBase, func(s *Snapshot) { s.Events = s.Events[:len(s.Events)-1] }},
		{"duplicateSourceSeq", ctlBase, func(s *Snapshot) { s.Events[1].Seq = s.Events[0].Seq }},
		{"eventSeq", ctlBase, func(s *Snapshot) { s.EventSeq = 99 }},
		{"unknownEventKind", ctlBase, func(s *Snapshot) { s.Events[0].Kind = 99 }},
		{"nanEvent", ctlBase, func(s *Snapshot) { s.Events[len(s.Events)-1].VXM = math.NaN() }},
		{"playerMismatch", ctlBase, func(s *Snapshot) { s.Integrated.Journal[len(s.Integrated.Journal)-1].PlayerID = 999 }},
		{"controlNoActor", ctlBase, func(s *Snapshot) {
			for i := range s.Integrated.Journal {
				if s.Integrated.Journal[i].Kind == FactControl {
					s.Integrated.Journal[i].PlayerID = 0
				}
			}
		}},
		{"sourceBallMismatch", ctlBase, func(s *Snapshot) { s.Integrated.Journal[0].X += 1 }},
		{"bothFutureClock", ctlBase, func(s *Snapshot) {
			n := len(s.Events) - 1
			s.Events[n].ClockMS = s.ClockMS + s.Cfg.SubstepMS
			for i := range s.Integrated.Journal {
				if s.Integrated.Journal[i].SourceSeq == s.Events[n].Seq {
					s.Integrated.Journal[i].ClockMS = s.Events[n].ClockMS
				}
			}
		}},
		{"bothFraction", ctlBase, func(s *Snapshot) {
			for i := range s.Events {
				if s.Events[i].Kind == PassEventControl || s.Events[i].Kind == PassEventDeflect {
					s.Events[i].Frac = 1.5
					for j := range s.Integrated.Journal {
						if s.Integrated.Journal[j].SourceSeq == s.Events[i].Seq {
							s.Integrated.Journal[j].Fraction = 1.5
						}
					}
					break
				}
			}
		}},
		{"eventClockOffGrid", ctlBase, func(s *Snapshot) { s.Events[len(s.Events)-1].ClockMS += 1 }},
		{"phaseAfter", ctlBase, func(s *Snapshot) { s.Integrated.Journal[0].PhaseAfter = PhaseStoppedOut }},
		{"phaseReconcile", ctlBase, func(s *Snapshot) { s.Integrated.Phase = PhaseStoppedOut }},
		{"lastTouchReconcile", ctlBase, func(s *Snapshot) { s.Integrated.LastTouchID = 999 }},
		{"clockGrid", ctlBase, func(s *Snapshot) { s.ClockMS += 5 }},
		{"heldNoOwner", ctlBase, func(s *Snapshot) { s.Ball.Mode = BallHeld; s.ContactRunner = -1 }},
		{"emptyJournalMismatch", ctlBase, func(s *Snapshot) {
			s.Integrated.Journal = nil
			s.Integrated.NextSeq = 0
			s.Integrated.LastTouchID = 0
			s.Ball.Mode = BallDead
			s.Ball.VXM, s.Ball.VYM = 0, 0
		}},
		{"pathActor", defBase, func(s *Snapshot) {
			for i := range s.Events {
				if s.Events[i].Kind == PassEventPath {
					s.Events[i].Runner = 0
					break
				}
			}
		}},
	}
	for _, r := range rows {
		reject(r.label, mut(r.base, r.f))
	}

	// Clone purity: accessor is a value copy.
	a1, _ := ctlSim.IntegratedState()
	a1.Journal[0].Kind = FactOut
	a2, _ := ctlSim.IntegratedState()
	if a2.Journal[0].Kind != ctlJournal[0].Kind {
		t.Fatal("IntegratedState accessor aliases the live journal")
	}
	// Snapshot deep-copies.
	snap := ctlSim.Snapshot()
	snap.Integrated.Journal[0].Kind = FactOut
	a3, _ := ctlSim.IntegratedState()
	if a3.Journal[0].Kind != ctlJournal[0].Kind {
		t.Fatal("Snapshot aliases the live journal")
	}
	// Resume deep-copies.
	res, err := Resume(ctlSim.Snapshot())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	res.integrated.Journal[0].Kind = FactOut
	a4, _ := ctlSim.IntegratedState()
	if a4.Journal[0].Kind != ctlJournal[0].Kind {
		t.Fatal("Resume aliases the live journal")
	}
	t.Logf("T-055 invalid imports: %d declared inconsistencies rejected (incl. both-source+fact mutations); accessor/snapshot/resume clones cannot alias the live journal", len(rows))
}

// deepCopySnapshot clones a snapshot through JSON so a mutation cannot leak.
func deepCopySnapshot(t *testing.T, s Snapshot) Snapshot {
	t.Helper()
	blob, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var out Snapshot
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return out
}

// --- 8. genuine active22 windows -------------------------------------------

// TestT055Active22Windows runs 3 fresh seeded windows per type with the shell on
// and guards: all22 move every substep, a genuine interior transition, held
// motion and the real second release; the windows are identical.
func TestT055Active22Windows(t *testing.T) {
	for _, control := range []bool{true, false} {
		label := "deflect"
		seed := uint64(t053DeflectSeed)
		if control {
			label = "control"
			seed = t053ControlSeed
		}
		var inits, finals []Snapshot
		var journals [][]FactEvent
		for w := 0; w < 3; w++ {
			sim := t055Active22Build(t, seed, control, true)
			inits = append(inits, sim.Snapshot())
			prev := sim.Runners()
			held := 0
			for k := 0; k < t053WindowSteps; k++ {
				t053StepWithMid(sim, control, k)
				cur := sim.Runners()
				t053AssertAllMoved(t, label, prev, cur, k)
				prev = cur
				if sim.Ball().Mode == BallHeld {
					held++
				}
			}
			finals = append(finals, sim.Snapshot())
			journals = append(journals, t055AssertJournalReconciles(t, sim).Journal)
			touches := t052Touches(sim)
			if len(touches) == 0 {
				t.Fatalf("%s: no transition", label)
			}
			if !(touches[0].Frac > 0 && touches[0].Frac < 1) {
				t.Fatalf("%s: transition frac %v not interior", label, touches[0].Frac)
			}
			if control {
				if touches[0].Kind != PassEventControl {
					t.Fatalf("control first touch %s", touches[0].Kind)
				}
				if held == 0 {
					t.Fatal("control: no held substep")
				}
				rel := t051ReleaseClocks(sim.Events())
				if len(rel) != 2 || rel[0] != 0 || rel[1] != t053SecondRelMS {
					t.Fatalf("control releases %v, want [0 %d]", rel, t053SecondRelMS)
				}
			} else if touches[0].Kind != PassEventDeflect {
				t.Fatalf("deflect first touch %s", touches[0].Kind)
			}
		}
		for w := 1; w < 3; w++ {
			if !reflect.DeepEqual(inits[0], inits[w]) || !reflect.DeepEqual(finals[0], finals[w]) || !reflect.DeepEqual(journals[0], journals[w]) {
				t.Fatalf("%s window %d differs from window 0", label, w)
			}
		}
		t.Logf("T-055 active22 %s: 3 fresh windows identical (init/final snapshots + %d journal facts); 22/22 moved every step", label, len(journals[0]))
	}
}
