package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// T-050 reaction-reset locality and causal timing study (isolated prototype
// evidence, TEST-ONLY).
//
// The competing semantics and the falsifiable fixture matrix are fixed in the
// session evidence BEFORE this code:
//   vault/07-Session-Logs/2026-10-16-t050-reaction-study-evidence.md
//
// Scope: the actual release -> reaction -> sampling -> intent -> contact ordering
// on the integer clock, and which timing differences come from ReleasePass
// overwriting EVERY runner's ReactAtMS rather than from the local-wakeup
// scheduling path. Nothing here changes runtime behaviour; every paired fixture
// re-runs the SAME real ReleasePass and then restores the pre-release ReactAtMS in
// the control member before the first Step, so the ONLY differing state between a
// pair is ReactAtMS.
//
// No total-world noninterference claim is made: C5/C6 assert the opposite and show
// a remote reset can genuinely change a later physical contact.

// --- helpers ---------------------------------------------------------------

// t050Inertial builds a plain inertial runner with a reaction delay.
func t050Inertial(x, y, ix, iy float64, reactMS int) Runner {
	r := InertialRunner(x, y, ix, iy, DefaultMotionParams())
	r.ReactMS = reactMS
	return r
}

// t050Local builds a local-pursuit runner (optionally also opted into per-runner
// local wakeup) with the given metre sensing radius and reaction delay.
func t050Local(x, y, ix, iy, radiusM float64, reactMS int, wakeup bool) Runner {
	r := t050Inertial(x, y, ix, iy, reactMS)
	if err := r.EnableLocalPursuit(radiusM); err != nil {
		panic("T-050 fixture: EnableLocalPursuit: " + err.Error())
	}
	if wakeup {
		if err := r.EnableLocalWakeup(radiusM); err != nil {
			panic("T-050 fixture: EnableLocalWakeup: " + err.Error())
		}
	}
	return r
}

// t050FarPass is a deterministic (zero-jitter) remote release used by the
// reset-isolation fixtures.
func t050FarPass() PassParams {
	return PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1}
}

// t050LogEvidence logs one complete JSON snapshot plus the ordered full sample and
// event tails for raw evidence. Bounded by the short fixture horizons; the exact
// replay command is documented in the session evidence note.
func t050LogEvidence(t *testing.T, label string, sim *Sim) {
	t.Helper()
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	t.Logf("%s snapshot: %s", label, blob)
	t.Logf("%s samples: %+v", label, sim.Samples())
	t.Logf("%s events: %+v", label, sim.Events())
}

// t050AssertPairEqualExceptReset asserts two sims are fully equal (ball, RNG,
// event log, contact/first-touch records, runner state) apart from each runner's
// ReactAtMS. Used at the release / pre-first-contact instant to prove the
// suppress-reset counterfactual isolates ONLY the reset.
func t050AssertPairEqualExceptReset(t *testing.T, a, b *Sim, label string) {
	t.Helper()
	if !reflect.DeepEqual(a.Ball(), b.Ball()) {
		t.Fatalf("%s: ball differs", label)
	}
	if a.RNGState() != b.RNGState() {
		t.Fatalf("%s: RNG differs", label)
	}
	if !reflect.DeepEqual(a.Events(), b.Events()) {
		t.Fatalf("%s: event log differs", label)
	}
	aContactMS, aContactRunner := a.FirstContact()
	bContactMS, bContactRunner := b.FirstContact()
	if aContactMS != bContactMS || aContactRunner != bContactRunner {
		t.Fatalf("%s: contact record differs %d/%d vs %d/%d", label, aContactMS, aContactRunner, bContactMS, bContactRunner)
	}
	aTouchMS, aTouchRunner := a.FirstTouch()
	bTouchMS, bTouchRunner := b.FirstTouch()
	if aTouchMS != bTouchMS || aTouchRunner != bTouchRunner {
		t.Fatalf("%s: first-touch differs %d/%d vs %d/%d", label, aTouchMS, aTouchRunner, bTouchMS, bTouchRunner)
	}
	ra, rb := a.Runners(), b.Runners()
	if len(ra) != len(rb) {
		t.Fatalf("%s: runner count differs", label)
	}
	for i := range ra {
		x, y := ra[i], rb[i]
		x.ReactAtMS, y.ReactAtMS = 0, 0
		if !reflect.DeepEqual(x, y) {
			t.Fatalf("%s: runner %d differs beyond ReactAtMS", label, i)
		}
	}
}

// t050MaxDivergence steps two sims in lockstep for a fixed number of substeps and
// returns the first clock at which any runner position differs by more than 1e-9
// metres, and the maximum runner displacement in metres. -1 means no divergence.
func t050MaxDivergence(a, b *Sim, substeps int, mid func(*Sim, int)) (int, float64) {
	first := -1
	maxDiff := 0.0
	for k := 0; k < substeps; k++ {
		if mid != nil {
			mid(a, a.ClockMS())
			mid(b, b.ClockMS())
		}
		a.Step()
		b.Step()
		ra, rb := a.Runners(), b.Runners()
		for i := range ra {
			d := DistM(ra[i].X, ra[i].Y, rb[i].X, rb[i].Y)
			if d > maxDiff {
				maxDiff = d
			}
			if first < 0 && d > 1e-9 {
				first = a.ClockMS()
			}
		}
	}
	return first, maxDiff
}

// --- C1: the reset is global and forced -----------------------------------

// TestT050ReactionResetIsGlobalAndForced pins that a real ReleasePass overwrites
// ReactAtMS = clock + ReactMS for EVERY runner (a remote non-observer AND a native
// pursue runner), and that it OVERWRITES a prior elapsed deadline rather than
// max-preserving it.
func TestT050ReactionResetIsGlobalAndForced(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 7)
	remote := t050Local(0.10, 0.50, 0.50, 0.50, 0.5, 130, false) // radius 0.5 m
	native := PursuingRunner(0.20, 0.60, DefaultMotionParams(), 130)
	sim.SetRunners([]Runner{remote, native})

	for sim.ClockMS() < 200 {
		sim.Step()
	}
	// Force a prior ELAPSED deadline on both so overwrite-vs-max is observable.
	sim.runners[0].ReactAtMS = 0
	sim.runners[1].ReactAtMS = 0
	before := []int{sim.runners[0].ReactAtMS, sim.runners[1].ReactAtMS}

	releaseDist := DistM(0.90, 0.90, sim.runners[0].X, sim.runners[0].Y)
	if releaseDist <= sim.runners[0].Perception.RadiusM {
		t.Fatalf("fixture: release is not remote (dist %.3f m <= radius %.3f m)", releaseDist, sim.runners[0].Perception.RadiusM)
	}
	sim.ReleasePass(t050FarPass())
	after := []int{sim.runners[0].ReactAtMS, sim.runners[1].ReactAtMS}

	if after[0] != 330 || after[1] != 330 {
		t.Fatalf("release did not overwrite both deadlines to 200+130: before=%v after=%v", before, after)
	}
	if sim.runners[0].WakePending {
		t.Fatal("remote release set the far runner's WakePending (should be inert)")
	}
	t.Logf("C1: ReleasePass@200 overwrote %v -> %v for a remote non-observer AND a native runner; remote dist %.2f m > radius %.2f m; no pending",
		before, after, releaseDist, sim.runners[0].Perception.RadiusM)
}

// --- C2: awareness revocation ----------------------------------------------

// TestT050ResetRevokesElapsedAwareness pins that a remote release with ReactMS>0
// re-blocks an already-elapsed local policy gate (the deadline is overwritten, so
// awareness can be revoked and postponed).
func TestT050ResetRevokesElapsedAwareness(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 5)
	sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, 500, false)})
	for sim.ClockMS() < 200 {
		sim.Step()
	}
	sim.runners[0].ReactAtMS = 0 // an elapsed, unblocked deadline

	policy0, applied0, gate0 := localDecision(sim.runners[0], sim.ClockMS())
	if !applied0 || gate0 != GateApplied || policy0 != PolicyUnknownBrake {
		t.Fatalf("pre-release gate was not open: policy=%s applied=%v gate=%s", policy0, applied0, gate0)
	}

	sim.ReleasePass(t050FarPass()) // arms 200+500 = 700

	policy1, applied1, gate1 := localDecision(sim.runners[0], sim.ClockMS())
	if applied1 || gate1 != GateReactionBlocked {
		t.Fatalf("remote release did not revoke awareness: policy=%s applied=%v gate=%s", policy1, applied1, gate1)
	}
	if got := sim.runners[0].ReactAtMS; got != 700 {
		t.Fatalf("ReactAtMS=%d, want 700", got)
	}
	t.Logf("C2: local gate %s(open) -> %s(blocked) after a remote release; ReactAtMS 0 -> 700", gate0, gate1)
}

// --- C3: zero-delay inertness ----------------------------------------------

// TestT050ResetInertForZeroDelay pins that with ReactMS=0 the reset is
// behaviourally inert: the complete snapshot is identical every substep to the
// no-reset counterfactual (both ReactAtMS = 0).
func TestT050ResetInertForZeroDelay(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func() *Sim {
		sim := mustSim(t, cfg, 3)
		sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, 0, false)})
		sim.ReleasePass(t050FarPass())
		return sim
	}
	a := build()
	b := build()
	b.runners[0].ReactAtMS = 0 // pre-release value
	if a.runners[0].ReactAtMS != 0 {
		t.Fatalf("ReactMS=0 release set ReactAtMS=%d, want 0", a.runners[0].ReactAtMS)
	}
	for k := 0; k < 30; k++ {
		a.Step()
		b.Step()
		if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
			t.Fatalf("zero-delay reset was not inert at substep %d (clock %d)", k, a.ClockMS())
		}
	}
	move := DistM(0.10, 0.50, a.runners[0].X, a.runners[0].Y)
	t.Logf("C3: ReactMS=0 remote reset: 30 substeps bit-identical snapshots; runner moved %.6f m (local policy ran normally)", move)
}

// --- C4: local trajectory divergence ---------------------------------------

// TestT050RemoteResetChangesLocalTrajectory is the core causal fixture: the SAME
// real remote ReleasePass, SAME RNG and SAME ball, differing ONLY in ReactAtMS
// (500 vs restored 0), produces a local trajectory divergence because the reset
// suppresses the lattice policy (Unknown-brake) until the deadline.
func TestT050RemoteResetChangesLocalTrajectory(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func() *Sim {
		sim := mustSim(t, cfg, 11)
		sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, 500, false)})
		sim.ReleasePass(t050FarPass())
		return sim
	}
	reset := build()
	ctrl := build()
	if reset.runners[0].ReactAtMS != 500 {
		t.Fatalf("reset member ReactAtMS=%d, want 500", reset.runners[0].ReactAtMS)
	}
	ctrl.runners[0].ReactAtMS = 0 // suppress ONLY the reset
	t050AssertPairEqualExceptReset(t, reset, ctrl, "C4 release")

	first, maxDiff := t050MaxDivergence(reset, ctrl, 45, nil) // 900 ms
	if !reflect.DeepEqual(reset.Ball(), ctrl.Ball()) {
		t.Fatal("the pair's balls diverged (isolation lost)")
	}
	dReset := DistM(0.10, 0.50, reset.runners[0].X, reset.runners[0].Y)
	dCtrl := DistM(0.10, 0.50, ctrl.runners[0].X, ctrl.runners[0].Y)
	if !(dReset > 0.30 && dCtrl < 1e-6) {
		t.Fatalf("trajectory isolation failed: reset moved %.4f m, control moved %.4f m (want >0.30 / ~0)", dReset, dCtrl)
	}
	if first != 20 {
		t.Fatalf("first divergence at clock %d, want 20 (the first substep)", first)
	}
	t050LogEvidence(t, "C4 reset@900", reset)
	t050LogEvidence(t, "C4 ctrl@900", ctrl)
	t.Logf("C4: same release/RNG/ball; ReactAtMS 500 vs 0 -> reset moved %.4f m, control %.4f m; first divergence @%d, max runner diff %.4f m",
		dReset, dCtrl, first, maxDiff)
}

// --- C5/C6: physical eligibility conflation --------------------------------

// TestT050RemoteResetChangesPhysicalContact pins that the reset gates PHYSICAL
// contact. The pair is identical except ReactAtMS up to the first control; after
// different control times the ball modes/positions GENUINELY diverge, so this is
// the explicit denial of total-world noninterference. It does NOT assert final
// ball equality (which would be false).
func TestT050RemoteResetChangesPhysicalContact(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func() *Sim {
		sim := mustSim(t, cfg, 13)
		r := t050Inertial(0.50, 0.50, 0.50, 0.50, 500) // stationary, default reach
		sim.SetRunners([]Runner{r})
		sim.ReleasePass(PassParams{StartX: 0.48, StartY: 0.50, TargetX: 0.55, TargetY: 0.50, SpeedMPS: 4.0, Passer: -1})
		return sim
	}
	reset := build()
	ctrl := build()
	ctrl.runners[0].ReactAtMS = 0
	t050AssertPairEqualExceptReset(t, reset, ctrl, "C5 release/pre-contact")

	const steps = 60 // 1200 ms
	firstDiv := -1
	for k := 0; k < steps; k++ {
		reset.Step()
		ctrl.Step()
		if firstDiv < 0 && !reflect.DeepEqual(reset.Ball(), ctrl.Ball()) {
			firstDiv = reset.ClockMS()
		}
	}
	touchA, runnerA := reset.FirstTouch()
	touchB, runnerB := ctrl.FirstTouch()
	if touchB < 0 {
		t.Fatalf("control runner never touched the ball (fixture broken)")
	}
	if touchA >= 0 && touchA <= touchB {
		t.Fatalf("reset did not change the physical contact: reset=%d control=%d", touchA, touchB)
	}
	if firstDiv < 0 {
		t.Fatal("pair never diverged despite different control times")
	}
	if firstDiv != touchB {
		t.Fatalf("first divergence at %d, want the control contact clock %d", firstDiv, touchB)
	}
	if reset.Ball().Mode == ctrl.Ball().Mode && touchA == touchB {
		t.Fatal("post-contact ball modes did not diverge")
	}
	t050LogEvidence(t, "C5 reset@1200", reset)
	t050LogEvidence(t, "C5 ctrl@1200", ctrl)
	t.Logf("C5/C6: pre-contact pair identical except ReactAtMS; first divergence @%d == control contact; touch reset=%d(runner %d) control=%d(runner %d); final modes reset=%s control=%s",
		firstDiv, touchA, runnerA, touchB, runnerB, reset.Ball().Mode, ctrl.Ball().Mode)
}

// --- C8: two real releases (far, local-wakeup) ------------------------------

// TestT050SecondRealReleaseFarLocalWakeup pins the retained remote-reset side
// channel under a genuinely FAR second release with a local-wakeup runner: the
// scheduling layer does NOT wake on the remote event (no remote pending, no
// refreshed far observation), the reset still writes ReactAtMS at its own clock,
// and — because the local policy would stale-retain anyway — the reset is
// behaviourally inert here (measured, not assumed).
func TestT050SecondRealReleaseFarLocalWakeup(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const radius = 4.0
	build := func() *Sim {
		sim := mustSim(t, cfg, 17)
		sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.50, 0.50, radius, 300, true)})
		// First release NEAR (3.15 m < 4 m), rolling away: an initial observation.
		sim.ReleasePass(PassParams{StartX: 0.53, StartY: 0.50, TargetX: 0.62, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		return sim
	}
	if d := DistM(0.90, 0.90, 0.50, 0.50); d <= radius {
		t.Fatalf("fixture: second release is not far (%.2f m <= %.2f m)", d, radius)
	}
	reset := build()
	ctrl := build()
	second := PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1}

	firstDiv, maxDiff := -1, 0.0
	for reset.ClockMS() < 1100 {
		if reset.ClockMS() == 620 {
			reset.ReleasePass(second)
			ctrl.ReleasePass(second)
			if reset.runners[0].ReactAtMS != 920 {
				t.Fatalf("second release ReactAtMS=%d, want 920", reset.runners[0].ReactAtMS)
			}
			ctrl.runners[0].ReactAtMS = 300 // first release's value
			if reset.runners[0].WakePending {
				t.Fatal("far second release woke the local runner (remote scheduling leak)")
			}
		}
		reset.Step()
		ctrl.Step()
		d := 0.0
		for i := range reset.runners {
			if dd := DistM(reset.runners[i].X, reset.runners[i].Y, ctrl.runners[i].X, ctrl.runners[i].Y); dd > d {
				d = dd
			}
		}
		if d > maxDiff {
			maxDiff = d
		}
		if firstDiv < 0 && d > 1e-9 {
			firstDiv = reset.ClockMS()
		}
	}

	clocks := []int{}
	for _, e := range reset.Events() {
		if e.Kind == PassEventRelease {
			clocks = append(clocks, e.ClockMS)
		}
	}
	if len(clocks) != 2 || clocks[0] != 0 || clocks[1] != 620 {
		t.Fatalf("release clocks = %v, want [0 620]", clocks)
	}
	if reset.runners[0].ReactAtMS != 920 || ctrl.runners[0].ReactAtMS != 300 {
		t.Fatalf("ReactAtMS reset=%d ctrl=%d, want 920/300", reset.runners[0].ReactAtMS, ctrl.runners[0].ReactAtMS)
	}
	obs, ok := reset.Perception(0)
	if !ok || !obs.Seen || obs.ClockMS >= 620 {
		t.Fatalf("fixture: expected an initial near observation before 620, got %+v", obs)
	}
	if obs.Freshness(620) != ObservationStale {
		t.Fatalf("observation freshness at 620 = %s, want stale", obs.Freshness(620))
	}
	if maxDiff > 1e-9 {
		t.Fatalf("expected a stale-retain-inert far reset; got runner divergence %.6f m at %d", maxDiff, firstDiv)
	}
	t.Logf("C8: two real releases [0,620]; far 2nd (%.1f m > %.1f m) re-armed ReactAtMS 300->920 but set NO pending; initial observation @%d, stale at 620; reset vs suppress runner divergence %.2e m (behaviourally inert under stale retention)",
		DistM(0.90, 0.90, 0.50, 0.50), radius, obs.ClockMS, maxDiff)
}

// --- C10: fractional control AND deflect do not re-arm ----------------------

// TestT050FractionalControlAndDeflect pins INTERIOR fractional timing: the ball
// starts just beyond the 1 m reach and sweeps into it WITHIN one 20 ms substep, so
// both a clean CONTROL and a DEFLECT land at a strictly interior fraction
// (0<Frac<1). It checks true event ordering and geometry, unchanged ReactAtMS, the
// exact rounded deflect cooldown and a complete JSON Resume taken AFTER the actual
// interior event state.
func TestT050FractionalControlAndDeflect(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	// 1.02 m: beyond reach at the step start, inside reach after one substep.
	startX := 0.50 - 1.02/PitchLengthM
	run := func(seed uint64, controlError float64) (*Sim, *ProvenanceRecorder, PassEvent) {
		sim := mustSim(t, cfg, seed)
		sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.50, 0.50, 200.0, 0, true)})
		rec, err := NewProvenanceRecorder(64)
		if err != nil {
			t.Fatalf("NewProvenanceRecorder: %v", err)
		}
		sim.SetProvenanceRecorder(rec)
		sim.runners[0].ReactAtMS = 999 // sentinel: only the release may overwrite it
		sim.ReleasePass(PassParams{StartX: startX, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1, ControlError: controlError})
		if got := sim.runners[0].ReactAtMS; got != 0 {
			t.Fatalf("release did not write ReactAtMS (got %d, want 0)", got)
		}
		sim.Step()
		var ev PassEvent
		for _, e := range sim.Events() {
			if e.Kind == PassEventControl || e.Kind == PassEventDeflect {
				ev = e
				break
			}
		}
		return sim, rec, ev
	}
	ctrl, ctrlRec, ctrlEv := run(81, 0.0)
	defl, deflRec, deflEv := run(82, 1.0)

	if ctrlEv.Kind != PassEventControl {
		t.Fatalf("ControlError=0 produced %s, want control (events %v)", ctrlEv.Kind, ctrl.Events())
	}
	if deflEv.Kind != PassEventDeflect {
		t.Fatalf("ControlError=1 produced %s, want deflect (events %v)", deflEv.Kind, defl.Events())
	}
	// Interior fraction and the end-step clock on both contacts.
	for _, e := range []PassEvent{ctrlEv, deflEv} {
		if e.ClockMS != SubstepMS {
			t.Fatalf("%s contact clock=%d, want the end-step %d", e.Kind, e.ClockMS, SubstepMS)
		}
		if !(e.Frac > 0 && e.Frac < 1) {
			t.Fatalf("%s contact Frac=%v, want strictly interior (0,1)", e.Kind, e.Frac)
		}
	}
	// True event ordering: release first, then the contact.
	evs := ctrl.Events()
	if len(evs) < 2 || evs[0].Kind != PassEventRelease || evs[1].Kind != PassEventControl {
		t.Fatalf("unexpected control event order: %v", evs)
	}
	// True geometry: the contact ball lies exactly on the reach circle around the
	// touching runner at the contact fraction.
	for _, e := range []PassEvent{ctrlEv, deflEv} {
		if d := DistM(e.BallX, e.BallY, e.RunnerX, e.RunnerY); math.Abs(d-ControlRadiusM) > 1e-6 {
			t.Fatalf("%s contact ball/runner distance %.9f, want reach %.3f", e.Kind, d, ControlRadiusM)
		}
	}
	if ctrl.runners[0].ReactAtMS != 0 || defl.runners[0].ReactAtMS != 0 {
		t.Fatalf("a fractional contact re-armed ReactAtMS: control=%d deflect=%d", ctrl.runners[0].ReactAtMS, defl.runners[0].ReactAtMS)
	}
	if ctrl.runners[0].CooldownUntilMS != 0 {
		t.Fatalf("clean control stamped a cooldown: %d", ctrl.runners[0].CooldownUntilMS)
	}
	if want := eventTimeMS(0, SubstepMS, deflEv.Frac) + DeflectionCooldownMS; defl.runners[0].CooldownUntilMS != want {
		t.Fatalf("deflect cooldown=%d, want rounded event time %d", defl.runners[0].CooldownUntilMS, want)
	}
	// The fixture really stamped the post-event states and recorded provenance.
	if ctrlRec.Overflowed() || ctrlRec.Len() == 0 {
		t.Fatalf("control provenance missing/overflowed: len=%d overflow=%v", ctrlRec.Len(), ctrlRec.Overflowed())
	}
	if deflRec.Overflowed() || deflRec.Len() == 0 {
		t.Fatalf("deflect provenance missing/overflowed: len=%d overflow=%v", deflRec.Len(), deflRec.Overflowed())
	}
	// Complete JSON Resume AFTER the actual interior event state.
	ctrlTail := t046JSONResume(t, ctrl)
	t046AssertRuntimeTailsMid(t, ctrl, ctrlTail, 400, "t050-interior-control", nil)
	deflTail := t046JSONResume(t, defl)
	t046AssertRuntimeTailsMid(t, defl, deflTail, 400, "t050-interior-deflect", nil)
	t.Logf("C10: interior contacts clock=%d ctrlFrac=%.3f deflFrac=%.3f; release wrote ReactAtMS=0; clean control no cooldown; deflect cooldown=%d (rounded event time); JSON tails identical after each interior event",
		ctrlEv.ClockMS, ctrlEv.Frac, deflEv.Frac, defl.runners[0].CooldownUntilMS)
}

// --- C9: cooldown / immunity independence -----------------------------------

// TestT050CooldownImmunityIndependentOfReset pins that the reset changes only
// ReactAtMS: immunityUntil (passer) and CooldownUntilMS (deflector) are separate
// gates, unmoved by the reset and unmoved by each other's events.
func TestT050CooldownImmunityIndependentOfReset(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 23)
	passer := t050Inertial(0.10, 0.50, 0.10, 0.50, 0)
	defl := t050Inertial(0.50, 0.50, 0.50, 0.50, 0)
	sim.SetRunners([]Runner{passer, defl})
	sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: 0, ControlError: 1.0})
	if sim.immunityUntil != PasserImmunityMS {
		t.Fatalf("immunityUntil=%d, want %d", sim.immunityUntil, PasserImmunityMS)
	}
	if sim.runners[0].ReactAtMS != 0 || sim.runners[1].ReactAtMS != 0 {
		t.Fatalf("release ReactAtMS=%d/%d, want 0/0", sim.runners[0].ReactAtMS, sim.runners[1].ReactAtMS)
	}
	sim.Step()
	if countKind(sim, PassEventDeflect) == 0 {
		t.Fatalf("non-passer runner did not deflect: %v", sim.Events())
	}
	if sim.runners[1].CooldownUntilMS <= 0 {
		t.Fatal("deflector cooldown not stamped")
	}
	if sim.runners[1].ReactAtMS != 0 || sim.runners[0].ReactAtMS != 0 {
		t.Fatalf("deflect changed ReactAtMS: %d/%d", sim.runners[0].ReactAtMS, sim.runners[1].ReactAtMS)
	}
	if sim.immunityUntil != PasserImmunityMS {
		t.Fatalf("deflect changed immunityUntil: %d", sim.immunityUntil)
	}
	t.Logf("C9: reset moved only ReactAtMS; immunityUntil=%d (passer) and CooldownUntilMS=%d (deflector) are independent gates",
		sim.immunityUntil, sim.runners[1].CooldownUntilMS)
}

// --- C11: native mode channel ----------------------------------------------

// TestT050NativeModeResetChannel pins that the reset channel exists with the
// scheduler DISABLED: a native Pursue runner's oracle publication is suppressed by
// the re-armed deadline, so only ReactAtMS differs between the pair.
func TestT050NativeModeResetChannel(t *testing.T) {
	cfg := LivePassConfig() // no intent scheduler
	build := func() *Sim {
		sim := mustSim(t, cfg, 29)
		sim.SetRunners([]Runner{PursuingRunner(0.10, 0.50, DefaultMotionParams(), 500)})
		sim.ReleasePass(t050FarPass())
		return sim
	}
	reset := build()
	ctrl := build()
	ctrl.runners[0].ReactAtMS = 0
	for k := 0; k < 25; k++ { // up to 500 ms
		reset.Step()
		ctrl.Step()
	}
	dReset := DistM(0.10, 0.50, reset.runners[0].X, reset.runners[0].Y)
	dCtrl := DistM(0.10, 0.50, ctrl.runners[0].X, ctrl.runners[0].Y)
	if !(dReset < 1e-6 && dCtrl > 0.30) {
		t.Fatalf("native-mode isolation failed: reset moved %.4f m, ctrl moved %.4f m (want ~0 / >0.30)", dReset, dCtrl)
	}
	t.Logf("C11: scheduler OFF; native oracle suppressed until 500 ms -> reset moved %.4f m vs ctrl %.4f m", dReset, dCtrl)
}

// --- C11b: shared vs local mode --------------------------------------------

// TestT050ResetChannelSharedAndLocalModes pins that the reset channel is present
// in BOTH the shared (scheduler armed, no local-wakeup opt-in) and the local
// (per-runner local wakeup) modes, i.e. it sits below the scheduling layer.
func TestT050ResetChannelSharedAndLocalModes(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	for _, wakeup := range []bool{false, true} {
		build := func() *Sim {
			sim := mustSim(t, cfg, 31)
			sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, 500, wakeup)})
			sim.ReleasePass(t050FarPass())
			return sim
		}
		reset := build()
		ctrl := build()
		ctrl.runners[0].ReactAtMS = 0
		_, maxDiff := t050MaxDivergence(reset, ctrl, 40, nil)
		if maxDiff < 0.30 {
			t.Fatalf("wakeup=%v: reset channel absent (max diff %.4f m)", wakeup, maxDiff)
		}
		t.Logf("C11b: wakeup=%v -> reset vs suppress max runner diff %.4f m", wakeup, maxDiff)
	}
}

// --- C12: not an RNG channel ------------------------------------------------

// TestT050ResetIsNotAnRNGChannel pins that the reset draws no RNG: two releases
// that differ only in ReactMS leave identical RNG state and ball.
func TestT050ResetIsNotAnRNGChannel(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(reactMS int) *Sim {
		sim := mustSim(t, cfg, 47)
		sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, reactMS, false)})
		sim.ReleasePass(t050FarPass())
		return sim
	}
	a := build(130)
	b := build(0)
	if a.RNGState() != b.RNGState() {
		t.Fatalf("reset changed RNG state: %v vs %v", a.RNGState(), b.RNGState())
	}
	if !reflect.DeepEqual(a.Ball(), b.Ball()) {
		t.Fatal("ReactMS changed the released ball")
	}
	if a.runners[0].ReactAtMS != 130 || b.runners[0].ReactAtMS != 0 {
		t.Fatalf("ReactAtMS=%d/%d, want 130/0", a.runners[0].ReactAtMS, b.runners[0].ReactAtMS)
	}
	t.Logf("C12: ReactMS 130 vs 0 -> identical RNG %v and identical ball; only ReactAtMS differs", a.RNGState())
}

// --- C10b / pending & visible-release ordering ------------------------------

// TestT050PendingVisibilityAndReset pins the publication ordering with a VISIBLE
// local release: the reset member's sampling and policy are both reaction-blocked
// (observation unknown, policy none), while the suppress-reset member samples the
// visible .51 ball and CHASES it (fresh rolling).
func TestT050PendingVisibilityAndReset(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func() (*Sim, *ProvenanceRecorder) {
		sim := mustSim(t, cfg, 37)
		sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.90, 0.50, 200.0, 300, true)})
		rec, err := NewProvenanceRecorder(64)
		if err != nil {
			t.Fatalf("NewProvenanceRecorder: %v", err)
		}
		sim.SetProvenanceRecorder(rec)
		sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
		return sim, rec
	}
	reset, rec := build()
	if !reset.runners[0].WakePending {
		t.Fatal("visible local release did not set WakePending")
	}
	reset.Step()
	if reset.runners[0].WakePending {
		t.Fatal("pending was not consumed at the next pre-step")
	}
	if reset.runners[0].IntentX != 0.90 {
		t.Fatalf("reaction-blocked member applied a policy: intentX=%v (want the initial 0.90)", reset.runners[0].IntentX)
	}
	blocked := 0
	for _, r := range rec.Records() {
		if r.ClockMS == 0 && r.GateOutcome == GateReactionBlocked {
			blocked++
			if r.Freshness != "unknown" {
				t.Fatalf("blocked record freshness=%s, want unknown", r.Freshness)
			}
			if r.Observation.Seen {
				t.Fatal("blocked record observed the ball (sampling gate failed)")
			}
		}
	}
	if blocked == 0 {
		t.Fatalf("no reaction-blocked provenance record at clock 0: %+v", rec.Records())
	}

	ctrl, rec2 := build()
	ctrl.runners[0].ReactAtMS = 0 // suppress the reset
	ctrl.Step()
	if ctrl.runners[0].IntentX != 0.51 {
		t.Fatalf("suppress-reset member did not chase the visible .51 ball: intentX=%v", ctrl.runners[0].IntentX)
	}
	o, ok := ctrl.Perception(0)
	if !ok || !o.Seen || o.X != 0.51 || o.Mode != BallRolling {
		t.Fatalf("suppress-reset observation = %+v (want fresh rolling .51)", o)
	}
	chased := false
	for _, r := range rec2.Records() {
		if r.ClockMS == 0 && r.Applied && r.Policy == PolicyChase && r.Freshness == "fresh" {
			chased = true
		}
	}
	if !chased {
		t.Fatalf("no applied chase record at clock 0: %+v", rec2.Records())
	}
	t.Logf("C10b: visible release; blocked member (intent %.2f, unknown, %d records) vs suppress-reset member (intent %.2f, fresh rolling .51 chase)",
		reset.runners[0].IntentX, blocked, ctrl.runners[0].IntentX)
}

// --- near/far reset coverage ------------------------------------------------

// TestT050NearFarResetCoverage compares a NEAR and a FAR release for the same
// local-wakeup runner, with a real provenance recorder attached BEFORE any step.
// The reset is global in both, but only the near release wakes (pending) and is
// observed; the far one stays unknown and its Unknown-brake targets the runner's
// OWN pre-step position at 300 (the body is NOT frozen: the blocked interval kept
// its forward intent, so it moves until the 300 publication and keeps coasting).
func TestT050NearFarResetCoverage(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const radius = 4.0
	mk := func(seed uint64, startX float64) (*Sim, *ProvenanceRecorder, float64) {
		sim := mustSim(t, cfg, seed)
		sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.90, 0.50, radius, 300, true)})
		rec, err := NewProvenanceRecorder(256)
		if err != nil {
			t.Fatalf("NewProvenanceRecorder: %v", err)
		}
		sim.SetProvenanceRecorder(rec)
		sim.ReleasePass(PassParams{StartX: startX, StartY: 0.50, TargetX: 0.62, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
		return sim, rec, DistM(startX, 0.50, 0.50, 0.50)
	}
	near, nearRec, nearD := mk(71, 0.53) // 3.15 m < 4 m
	far, farRec, farD := mk(72, 0.90)    // 42 m > 4 m
	if !(nearD < radius && farD > radius) {
		t.Fatalf("fixture geometry near=%.2f far=%.2f radius=%.2f", nearD, farD, radius)
	}
	if near.runners[0].ReactAtMS != 300 || far.runners[0].ReactAtMS != 300 {
		t.Fatalf("reset not global: near=%d far=%d", near.runners[0].ReactAtMS, far.runners[0].ReactAtMS)
	}
	if !near.runners[0].WakePending {
		t.Fatal("near release did not wake the local runner")
	}
	if far.runners[0].WakePending {
		t.Fatal("far release woke the local runner")
	}

	// Advance to exactly 300: the 300 publication happens on the NEXT substep.
	for near.ClockMS() < 300 {
		near.Step()
		far.Step()
	}
	if near.ClockMS() != 300 || far.ClockMS() != 300 {
		t.Fatalf("clocks near=%d far=%d, want 300/300", near.ClockMS(), far.ClockMS())
	}
	// Capture the pre-step own position the far Unknown-brake must target, the
	// invisible far observation and the near pre-step ball the chase must observe.
	farOwnX, farOwnY := far.runners[0].X, far.runners[0].Y
	farPreObs, _ := far.Perception(0)
	nearPreBallX := near.ball.X

	// Step through the 300 publication to 320.
	near.Step()
	far.Step()
	if near.ClockMS() != 320 {
		t.Fatalf("clock=%d, want 320", near.ClockMS())
	}

	// --- far: Unknown-brake to its OWN pre-step position; no observation write ---
	var farRec300 *DecisionRecord
	for _, r := range farRec.Records() {
		if r.ClockMS == 300 {
			rr := r
			farRec300 = &rr
		}
	}
	if farRec300 == nil {
		t.Fatalf("no far provenance record at 300: %+v", farRec.Records())
	}
	if !farRec300.Applied || farRec300.Policy != PolicyUnknownBrake || farRec300.Freshness != "unknown" {
		t.Fatalf("far record = policy %s applied %v freshness %s, want unknown-brake/applied/unknown", farRec300.Policy, farRec300.Applied, farRec300.Freshness)
	}
	if farRec300.Observation.Seen {
		t.Fatal("far record observed a remote ball")
	}
	if farRec300.TargetX != farOwnX || farRec300.TargetY != farOwnY {
		t.Fatalf("far Unknown-brake target (%.6f,%.6f) != own pre-step (%.6f,%.6f)", farRec300.TargetX, farRec300.TargetY, farOwnX, farOwnY)
	}
	farPostObs, _ := far.Perception(0)
	if farPostObs.Seen != farPreObs.Seen || farPostObs.ClockMS != farPreObs.ClockMS {
		t.Fatalf("far invisible observation changed: %+v -> %+v", farPreObs, farPostObs)
	}
	// The body is NOT frozen: it moved from its captured pre-step position.
	if DistM(farOwnX, farOwnY, far.runners[0].X, far.runners[0].Y) <= 1e-9 {
		t.Fatal("far runner froze at the 300 publication")
	}

	// --- near: chase the observed (current) pre-step ball -----------------------
	var nearRec300 *DecisionRecord
	for _, r := range nearRec.Records() {
		if r.ClockMS == 300 {
			rr := r
			nearRec300 = &rr
		}
	}
	if nearRec300 == nil {
		t.Fatalf("no near provenance record at 300: %+v", nearRec.Records())
	}
	if !nearRec300.Applied || nearRec300.Policy != PolicyChase || nearRec300.Freshness != "fresh" {
		t.Fatalf("near record = policy %s applied %v freshness %s, want chase/applied/fresh", nearRec300.Policy, nearRec300.Applied, nearRec300.Freshness)
	}
	if !nearRec300.Observation.Seen || nearRec300.Observation.X != nearPreBallX {
		t.Fatalf("near observation (%.6f, seen %v) != pre-step ball %.6f", nearRec300.Observation.X, nearRec300.Observation.Seen, nearPreBallX)
	}
	if nearRec300.TargetX != nearRec300.Observation.X || nearRec300.TargetY != nearRec300.Observation.Y {
		t.Fatalf("near chase target (%.6f,%.6f) != observation (%.6f,%.6f)", nearRec300.TargetX, nearRec300.TargetY, nearRec300.Observation.X, nearRec300.Observation.Y)
	}
	nearObs, _ := near.Perception(0)
	if !nearObs.Seen || nearObs.ClockMS != 300 {
		t.Fatalf("near observation not refreshed at 300: %+v", nearObs)
	}
	t.Logf("near/far coverage: near(%.2f m) pending+observed+chase(target %.6f); far(%.2f m) no pending+unknown-brake(target own %.6f,%.6f, invisible obs unchanged, still moving)",
		nearD, nearRec300.TargetX, farD, farOwnX, farOwnY)
}

// --- deterministic complete JSON tails --------------------------------------

// TestT050CompleteJSONResumeTails pins deterministic COMPLETE JSON Resume across a
// moving local runner, the reset and a mid-tail second real release.
func TestT050CompleteJSONResumeTails(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 41)
	sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.90, 0.50, 200.0, 300, true)})
	sim.ReleasePass(PassParams{StartX: 0.54, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for sim.ClockMS() < 240 {
		sim.Step()
	}
	resumed := t046JSONResume(t, sim)
	mid := func(s *Sim, clock int) {
		if clock == 620 {
			s.ReleasePass(t050FarPass())
		}
	}
	t046AssertRuntimeTailsMid(t, sim, resumed, 900, "t050-json", mid)
	t.Logf("complete JSON tails: 240->900 ms straight vs JSON-resumed identical every substep incl. the 620 second release")
}

// TestT050JSONResumeAcrossStates pins deterministic COMPLETE JSON Resume across the
// distinct deadline states (not one tail only): a persisted pending, an
// off-lattice due crossing, a deflect cooldown and a passer immunity window.
func TestT050JSONResumeAcrossStates(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	type scene struct {
		name           string
		build          func() *Sim
		split, horizon int
	}
	visible := func(seed uint64) *Sim {
		sim := mustSim(t, cfg, seed)
		sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.90, 0.50, 200.0, 300, true)})
		sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
		return sim
	}
	scenes := []scene{
		{"pending", func() *Sim { return visible(61) }, 0, 400},
		{"due-offlattice", func() *Sim { return visible(62) }, 60, 400},
		{"cooldown", func() *Sim {
			sim := mustSim(t, cfg, 63)
			sim.SetRunners([]Runner{t050Inertial(0.50, 0.50, 0.50, 0.50, 0)})
			sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1, ControlError: 1.0})
			return sim
		}, 20, 400},
		{"immunity", func() *Sim {
			sim := mustSim(t, cfg, 64)
			sim.SetRunners([]Runner{t050Inertial(0.10, 0.50, 0.10, 0.50, 0)})
			sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: 0})
			return sim
		}, 200, 700},
	}
	for _, sc := range scenes {
		sim := sc.build()
		for sim.ClockMS() < sc.split {
			sim.Step()
		}
		if sc.horizon <= sim.ClockMS() {
			t.Fatalf("%s: horizon %d <= split %d", sc.name, sc.horizon, sc.split)
		}
		resumed := t046JSONResume(t, sim)
		t046AssertRuntimeTailsMid(t, sim, resumed, sc.horizon, "t050-json-"+sc.name, nil)
		t.Logf("JSON across states: %-14s split=%d horizon=%d -> straight vs JSON-resumed identical every substep", sc.name, sc.split, sc.horizon)
	}
}

// --- provenance: the remote input path --------------------------------------

// TestT050ProvenanceRemoteInputPath captures the remote input path as bounded
// provenance: an off-lattice remote release produces a shared-event publication
// whose gate is reaction-blocked with an unknown observation, while the armed
// horizon export validates.
func TestT050ProvenanceRemoteInputPath(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 43)
	sim.SetRunners([]Runner{t050Local(0.10, 0.50, 0.90, 0.50, 0.5, 300, false)})
	for sim.ClockMS() < 60 {
		sim.Step()
	}
	rec, err := NewProvenanceRecorder(64)
	if err != nil {
		t.Fatalf("NewProvenanceRecorder: %v", err)
	}
	sim.SetProvenanceRecorder(rec)
	sim.ReleasePass(t050FarPass())
	sim.Step() // pre-step 60: off-lattice, shared dirty

	found := false
	for _, r := range rec.Records() {
		if r.ClockMS == 60 && r.Reason == ReasonSharedEvent && r.GateOutcome == GateReactionBlocked {
			found = true
			if r.Freshness != "unknown" {
				t.Fatalf("remote path freshness=%s, want unknown", r.Freshness)
			}
		}
	}
	if !found {
		t.Fatalf("no shared-event reaction-blocked record at 60: %+v", rec.Records())
	}

	prov, perr := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 256, HorizonMS: 200})
	if perr != nil {
		t.Fatalf("BuildDecisionProvenance: %v", perr)
	}
	if err := prov.Validate(); err != nil {
		t.Fatalf("provenance export did not validate: %v", err)
	}
	t.Logf("provenance: remote release @60 -> reason=shared-event gate=reaction-blocked freshness=unknown; armed export %d records valid",
		prov.RecordCount)
}

// --- active-22 guard --------------------------------------------------------

// t050ArmedBenchRunners builds 22 local-pursuit + local-wakeup bodies whose
// reaction deadline is deliberately in the FUTURE, with an initial intent toward
// the ball lane so every body still moves every substep (the gate blocks the
// POLICY, never integration).
func t050ArmedBenchRunners() []Runner {
	rs := t046BenchRunners(true)
	for i := range rs {
		rs[i].ReactMS = 100000
		rs[i].ReactAtMS = 100000
		rs[i].IntentX, rs[i].IntentY = 0.50, 0.50
	}
	return rs
}

// t050RearmArmed rewinds a fresh active window and re-arms every deadline into the
// future (the "just reset" world), keeping the initial intent toward the lane.
func t050RearmArmed(sim *Sim) {
	t046RearmBench(sim)
	for i := range sim.runners {
		sim.runners[i].ReactAtMS = 100000
		sim.runners[i].IntentX, sim.runners[i].IntentY = 0.50, 0.50
	}
}

// TestT050Active22StaysActive guards both benchmark windows: the ball stays
// rolling, no contact fires, all 22 bodies move EVERY substep, and the clock
// advances exactly one substep per Step.
func TestT050Active22StaysActive(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		runners func() []Runner
		rearm   func(*Sim)
	}{
		{"elapsed", ScheduledLivePassConfig(100), func() []Runner { return t046BenchRunners(true) }, t046RearmBench},
		{"armed", ScheduledLivePassConfig(100), t050ArmedBenchRunners, t050RearmArmed},
	}
	for _, c := range cases {
		sim := mustSim(t, c.cfg, 1)
		sim.SetRunners(c.runners())
		for w := 0; w < 3; w++ {
			c.rearm(sim)
			clock0 := sim.ClockMS()
			prev := sim.Runners()
			for k := 0; k < t040Window; k++ {
				sim.Step()
				if sim.Ball().Mode != BallRolling {
					t.Fatalf("%s window=%d: ball mode=%s at step %d", c.name, w, sim.Ball().Mode, k)
				}
				if ms, _ := sim.FirstContact(); ms >= 0 {
					t.Fatalf("%s window=%d: contact at step %d", c.name, w, k)
				}
				cur := sim.Runners()
				if len(cur) != 22 {
					t.Fatalf("%s window=%d: runner count %d", c.name, w, len(cur))
				}
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("%s window=%d: runner %d did not move at step %d", c.name, w, i, k)
					}
				}
				prev = cur
			}
			if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
				t.Fatalf("%s window=%d: clock advanced %d ms, want %d", c.name, w, got, t040Window*sim.Config().SubstepMS)
			}
		}
	}
	t.Logf("active22: elapsed and armed windows both stay fully active (all 22 move every substep, no contact)")
}
