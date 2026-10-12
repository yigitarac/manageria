package liveplay

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
)

// T-042 opt-in persisted intent-scheduler tests (isolated prototype evidence).
//
// These tests exercise the OPT-IN sampled scheduler added in scheduler.go and
// wired into stepLive. The zero value (Config.IntentCadenceMS == 0) must keep the
// exact native per-substep behaviour and the old JSON key set; a non-zero cadence
// is a DIFFERENT sampled model, valid only with LivePass and as a positive
// multiple of SubstepMS. Integration is never changed: every body integrates
// every substep, and reaction/cooldown/immunity gates are never bypassed.
//
// The contract (periodic lattice + event invalidation ordering + the two
// persisted snapshot fields) is fixed in the session evidence BEFORE this code.
// The tests here assert the timeline EXPLICITLY (exact due clocks/count/order),
// not merely that the suite is green.

// t042TimeEpsilon is the float numeric epsilon for exact-time gate checks. A
// reaction/cooldown deadline is an integer millisecond; only the fractional
// contact-time reconstruction can be off by rounding noise, so the tolerance is
// small, not a per-substep margin.
const t042TimeEpsilon = 1e-6

// t042Run advances a sim to horizonMS, returning the exact pre-step clocks at
// which the scheduler published intents. An optional mid hook runs before each
// substep (used to inject a dynamic second release at a chosen clock).
func t042Run(t *testing.T, sim *Sim, horizonMS int, mid func(*Sim, int)) []int {
	t.Helper()
	var pubs []int
	last := sim.DecisionPublishCount()
	for sim.ClockMS() < horizonMS {
		if mid != nil {
			mid(sim, sim.ClockMS())
		}
		start := sim.ClockMS()
		sim.Step()
		if c := sim.DecisionPublishCount(); c > last {
			last = c
			pubs = append(pubs, start)
		}
	}
	return pubs
}

// t042Invalidating reports whether an emitted event invalidates stale intent for
// the NEXT pre-step. It is deliberately INDEPENDENT of the T-041 study oracle:
// T-042 makes RELEASE an invalidating event too (the initial release and any
// dynamic later release invoked between steps), which the T-041 oracle excluded
// as an "initial condition". Control/deflect/stop/out invalidate; path does not.
func t042Invalidating(k PassEventKind) bool {
	return k == PassEventRelease || t041StateChanging(k)
}

// t042ExpectedPubs computes the exact publication set the contract requires:
// every periodic lattice clock plus the pre-step clock after each invalidating
// event (release/control/deflect/stop/out). Path events must contribute nothing.
func t042ExpectedPubs(cfg Config, events []PassEvent, lastStepStart int) []int {
	set := map[int]bool{}
	if cfg.IntentCadenceMS > 0 {
		for c := 0; c <= lastStepStart; c += cfg.IntentCadenceMS {
			set[c] = true
		}
	}
	for _, e := range events {
		if !t042Invalidating(e.Kind) {
			continue
		}
		if e.ClockMS <= lastStepStart {
			set[e.ClockMS] = true
		}
	}
	out := make([]int, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

func t042IntsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// t042AssertTails proves a resumed sim reproduces the straight run exactly:
// the COMPLETE serializable Snapshot (config-independent state: clock, ball,
// runners, contact/first-touch records, event log + sequence, immunity, release/
// path bookkeeping, control budget, RNG, and the two scheduler fields
// NextDecisionMS/DecisionDirty) AND the derived sample tail. The evidence-only
// counters (decisionPublishes/lastDecisionMS) are intentionally NOT persisted, so
// they are excluded by construction. Complete snapshots are DeepEqual because
// Resume seeds the event log/sequence from the snapshot, so the resumed event log
// is the cumulative prefix+tail — not just a tail.
func t042AssertTails(t *testing.T, straight, resumed *Sim, label string) {
	t.Helper()
	if straight.ClockMS() != resumed.ClockMS() {
		t.Fatalf("%s: clock %d != %d", label, straight.ClockMS(), resumed.ClockMS())
	}
	if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
		t.Fatalf("%s: COMPLETE snapshot differs after Resume:\n straight=%+v\n resumed =%+v",
			label, straight.Snapshot(), resumed.Snapshot())
	}
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 || !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatalf("%s: resumed sample tail differs from the straight run (head=%d)", label, head)
	}
}

// t042SnapshotResume takes a JSON snapshot of sim (the isolated liveplay snapshot
// contract) and resumes it, asserting the persisted scheduler state survived the
// round-trip.
func t042SnapshotResume(t *testing.T, sim *Sim, label string) *Sim {
	t.Helper()
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var back Snapshot
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	if back.NextDecisionMS != sim.nextDecisionMS || back.DecisionDirty != sim.decisionDirty {
		t.Fatalf("%s: scheduler state lost in JSON round-trip: next %d/%d dirty %v/%v",
			label, back.NextDecisionMS, sim.nextDecisionMS, back.DecisionDirty, sim.decisionDirty)
	}
	resumed, err := Resume(back)
	if err != nil {
		t.Fatalf("%s: Resume: %v", label, err)
	}
	if resumed.nextDecisionMS != sim.nextDecisionMS || resumed.decisionDirty != sim.decisionDirty {
		t.Fatalf("%s: Resume changed scheduler state", label)
	}
	return resumed
}

// --- 1. validation, no implicit default -----------------------------------

// TestT042CadenceValidation: a configured cadence must be a positive multiple of
// SubstepMS and requires LivePass; the zero value is the disabled default and no
// constructor ever injects an implicit 100 ms cadence. Both constructors and the
// JSON snapshot path reject invalid input.
func TestT042CadenceValidation(t *testing.T) {
	if DefaultConfig().IntentCadenceMS != 0 {
		t.Fatalf("DefaultConfig injected an implicit cadence %d", DefaultConfig().IntentCadenceMS)
	}
	if LivePassConfig().IntentCadenceMS != 0 {
		t.Fatalf("LivePassConfig injected an implicit cadence %d", LivePassConfig().IntentCadenceMS)
	}

	type tc struct {
		name string
		cfg  Config
		sent error
	}
	base := func(cadence int, live bool) Config {
		return Config{SubstepMS: 20, DecelMPS2: RollDecelMPS2, LivePass: live, IntentCadenceMS: cadence}
	}
	cases := []tc{
		{"negative", base(-20, true), ErrIntentCadence},
		{"non-multiple", base(30, true), ErrIntentCadence},
		{"needs-livepass", base(100, false), ErrIntentCadenceNeedsLivePass},
	}
	for _, c := range cases {
		if _, err := NewSim(c.cfg, 1); err == nil {
			t.Fatalf("%s: NewSim accepted an invalid cadence config", c.name)
		} else if !errors.Is(err, c.sent) {
			t.Fatalf("%s: NewSim error = %v, want %v", c.name, err, c.sent)
		}
		if _, err := Resume(Snapshot{Cfg: c.cfg, RNG: [4]uint64{1, 2, 3, 4}}); err == nil {
			t.Fatalf("%s: Resume accepted an invalid cadence config", c.name)
		}
	}
	for _, cadence := range []int{SubstepMS, 100, 200} {
		if _, err := NewSim(base(cadence, true), 1); err != nil {
			t.Fatalf("valid cadence %d rejected: %v", cadence, err)
		}
	}
	// A cadence that is exactly a multiple but the substep was raised above the
	// live-pass cooldown is still rejected first by the cooldown bound.
	over := base(100, true)
	over.SubstepMS = DeflectionCooldownMS
	if _, err := NewSim(over, 1); err == nil {
		t.Fatal("NewSim accepted a live-pass substep >= cooldown")
	}
}

// --- 2. disabled contract and JSON keys -----------------------------------

// TestT042DisabledInertAndKeysUnchanged: the zero cadence leaves the scheduler
// inert (no publications, no deadline) and adds NO new Snapshot/Config JSON keys,
// even after a state-changing control event. A live-pass default config therefore
// keeps the exact old JSON contract.
func TestT042DisabledInertAndKeysUnchanged(t *testing.T) {
	for _, cfg := range []Config{LivePassConfig(), DefaultConfig()} {
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal cfg: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal cfg: %v", err)
		}
		if _, ok := m["intentCadenceMs"]; ok {
			t.Fatalf("disabled config serialized intentCadenceMs: %s", raw)
		}
	}

	sim := mustSim(t, LivePassConfig(), 4)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	for sim.ClockMS() < 6000 && sim.Ball().Mode != BallHeld {
		sim.Step()
	}
	if sim.DecisionPublishCount() != 0 {
		t.Fatalf("disabled scheduler published %d times", sim.DecisionPublishCount())
	}
	if sim.NextDecisionMS() != 0 {
		t.Fatalf("disabled scheduler has deadline %d", sim.NextDecisionMS())
	}

	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	for _, bad := range []string{"nextDecisionMs", "decisionDirty"} {
		if _, ok := m[bad]; ok {
			t.Fatalf("disabled snapshot serialized scheduler key %q", bad)
		}
	}
}

// TestT042EnabledSnapshotSerializesSchedulerState: the two scheduler fields DO
// appear when armed (additive), and round-trip exactly.
func TestT042EnabledSnapshotSerializesSchedulerState(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 4)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	sim.StepQuiet() // consumes the release invalidation, flush dirty, advance lattice

	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if _, ok := m["nextDecisionMs"]; !ok {
		t.Fatalf("armed snapshot missing nextDecisionMs: %s", raw)
	}
	if sim.nextDecisionMS != 100 {
		t.Fatalf("next deadline %d, want 100", sim.nextDecisionMS)
	}
	t042SnapshotResume(t, sim, "armed-roundtrip")
}

// --- 3. equivalence anchor: cadence == substep ----------------------------

// TestT042ScheduledAtSubstepMatchesNative is the equivalence anchor: a cadence of
// exactly one substep publishes intents at every pre-step, reproducing the
// engine's native per-substep pursuit BIT-IDENTICALLY on every contact scene.
// Only this cadence makes an equivalence claim.
func TestT042ScheduledAtSubstepMatchesNative(t *testing.T) {
	for _, sc := range t041Scenes() {
		setup := func(sim *Sim) {
			sim.SetRunners(sc.runners())
			sc.setupBall(sim)
		}
		nat := captureLiveTrajWith(t, LivePassConfig(), 1, setup, sc.horizon, nil)
		sched := captureLiveTrajWith(t, ScheduledLivePassConfig(SubstepMS), 1, setup, sc.horizon, nil)
		assertSameTrajectory(t, nat, sched)
		t.Logf("scene=%s: cadence=SubstepMS is bit-identical to native per-substep publication", sc.name)
	}
}

// --- 4. coarser cadence is a DIFFERENT model (reported, not equivalent) ----

// TestT042Scheduled100msIsDifferentModel reports the 20 ms vs 100 ms sampled
// model difference on the contact scenes. It asserts they are NOT bit-identical
// on at least one scene and that the coarse run still respects the cooldown gate.
// No equivalence is claimed for any cadence coarser than one substep.
func TestT042Scheduled100msIsDifferentModel(t *testing.T) {
	differs := 0
	for _, sc := range t041Scenes() {
		setup := func(sim *Sim) {
			sim.SetRunners(sc.runners())
			sc.setupBall(sim)
		}
		ref := captureLiveTrajWith(t, ScheduledLivePassConfig(SubstepMS), 1, setup, sc.horizon, nil)
		cand := captureLiveTrajWith(t, ScheduledLivePassConfig(100), 1, setup, sc.horizon, nil)
		t041AssertCooldownHonoured(t, sc, cand)
		d := alignedFullRun(ref, cand)
		if d.ballMax > 0 || d.playerMax > 0 {
			differs++
		}
		t.Logf("scene=%-20s cadence=100ms vs 20ms: ball=%.4fm player=%.4fm modeMis=%d ownerMis=%d (NOT claimed equivalent)",
			sc.name, d.ballMax, d.playerMax, d.modeMismatch, d.ownerMismatch)
	}
	if differs == 0 {
		t.Fatal("cadence=100ms was bit-identical to cadence=SubstepMS on every scene; expected a different sampled model")
	}
}

// --- 5. explicit periodic-lattice timeline --------------------------------

// TestT042LatticeTimelineNoPathInvalidation proves the periodic lattice is
// exactly {0, C, 2C, ...} and that PATH events (a pure position trace) never add
// an off-lattice publication. A fast rolling ball with no reachable runner emits
// path events every 100 ms but no state-changing event in the window.
func TestT042LatticeTimelineNoPathInvalidation(t *testing.T) {
	const horizon = 1000
	const cadence = 160
	setup := func(sim *Sim) {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	sim := mustSim(t, ScheduledLivePassConfig(cadence), 1)
	setup(sim)
	pubs := t042Run(t, sim, horizon, nil)

	pathClocks := 0
	for _, e := range sim.Events() {
		if e.Kind == PassEventPath {
			pathClocks++
		}
	}
	if pathClocks == 0 {
		t.Fatal("scene emitted no path events; cannot prove path does not invalidate")
	}
	lastStepStart := horizon - sim.Config().SubstepMS
	want := t042ExpectedPubs(sim.Config(), sim.Events(), lastStepStart)
	if !t042IntsEqual(pubs, want) {
		t.Fatalf("publication timeline %v != expected lattice %v", pubs, want)
	}
	for _, c := range pubs {
		if c%cadence != 0 {
			t.Fatalf("off-lattice publication at %d ms (cadence %d): %v", c, cadence, pubs)
		}
	}
	if len(pubs) != lastStepStart/cadence+1 {
		t.Fatalf("published %d times, want %d lattice ticks", len(pubs), lastStepStart/cadence+1)
	}
	t.Logf("cadence=%dms rolling scene: %d path events, pubs=%v (pure lattice, no path invalidation)", cadence, pathClocks, pubs)
}

// TestT042ContactTimelineInvalidationAndNoShift proves that a state-changing
// event adds a publication at the pre-step clock AFTER the event while the
// periodic lattice is NOT shifted, so the next publication is still the next
// lattice multiple (not eventClock + cadence).
func TestT042ContactTimelineInvalidationAndNoShift(t *testing.T) {
	var sc t041Scene
	for _, s := range t041Scenes() {
		if s.name == "multi-deflection" {
			sc = s
		}
	}
	if sc.name == "" {
		t.Fatal("multi-deflection scene not found")
	}
	const cadence = 200
	setup := func(sim *Sim) {
		sim.SetRunners(sc.runners())
		sc.setupBall(sim)
	}
	sim := mustSim(t, ScheduledLivePassConfig(cadence), 1)
	setup(sim)
	pubs := t042Run(t, sim, sc.horizon, nil)

	lastStepStart := sc.horizon - sim.Config().SubstepMS
	want := t042ExpectedPubs(sim.Config(), sim.Events(), lastStepStart)
	if !t042IntsEqual(pubs, want) {
		t.Fatalf("publication timeline %v != expected lattice+invalidation %v", pubs, want)
	}
	for i := 1; i < len(pubs); i++ {
		if pubs[i] <= pubs[i-1] {
			t.Fatalf("publication clocks not strictly increasing: %v", pubs)
		}
	}
	// An off-lattice publication exists (an event invalidation) and is NOT
	// followed by another off-lattice publication cadence later (no shift).
	offLattice := -1
	for _, c := range pubs {
		if c%cadence != 0 {
			offLattice = c
			break
		}
	}
	if offLattice < 0 {
		t.Fatalf("no event-invalidation publication found off the lattice: %v", pubs)
	}
	shift := offLattice + cadence
	if shift <= lastStepStart && containsInt(pubs, shift) && shift%cadence != 0 {
		t.Fatalf("periodic deadline shifted to %d after off-lattice event at %d: %v", shift, offLattice, pubs)
	}
	t.Logf("cadence=%dms contact scene: pubs=%v | first off-lattice=%d (lattice unshifted)", cadence, pubs, offLattice)
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// --- 6. Resume before/after due clocks ------------------------------------

// TestT042ResumeBeforeAndAfterDue snapshots a scheduled deflection scene at
// clocks just before, exactly at, and just after a lattice deadline and proves
// the resumed tail (samples/events/RNG/runners) is bit-identical, i.e. the
// persisted next deadline continues the lattice exactly.
func TestT042ResumeBeforeAndAfterDue(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(sim *Sim) {
		sim.SetRunners([]Runner{
			PursuingRunner(0.55, 0.50+0.8/PitchWidthM, DefaultMotionParams(), 0),
			PursuingRunner(0.72, 0.50-0.8/PitchWidthM, DefaultMotionParams(), 0),
		})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	const horizon = 4000
	for _, split := range []int{60, 100, 120, 1900} {
		straight := mustSim(t, cfg, 1)
		setup(straight)
		for straight.ClockMS() < split {
			straight.Step()
		}
		resumed := t042SnapshotResume(t, straight, "split")
		nextAtSplit := straight.NextDecisionMS()
		dirtyAtSplit := straight.decisionDirty
		t042Run(t, straight, horizon, nil)
		t042Run(t, resumed, horizon, nil)
		t042AssertTails(t, straight, resumed, "before/after due")
		t.Logf("split@%dms: next deadline %d, dirty=%v at snapshot; resumed tail bit-identical",
			split, nextAtSplit, dirtyAtSplit)
	}
}

// --- 7. dynamic second release across Resume ------------------------------

// TestT042SecondReleaseSameScheduleAcrossResume snapshots a scheduled scene,
// injects a dynamic SECOND release OFF the lattice after the snapshot on both
// paths, and proves the periodic deadline (900 ms) is unchanged before/after the
// invalidation, the resumed publication schedule is identical, and the COMPLETE
// snapshot is bit-identical.
func TestT042SecondReleaseSameScheduleAcrossResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const split = 400
	const secondRelease = 820 // OFF-lattice (820 % 100 == 20): must ADD an invalidation
	const horizon = 3000
	setup := func(sim *Sim) {
		sim.SetRunners([]Runner{
			PursuingRunner(0.62, 0.50, DefaultMotionParams(), 0),
			PursuingRunner(0.78, 0.42, DefaultMotionParams(), 0),
		})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	}
	deadlineBefore, deadlineAfter := -1, -1
	second := func(sim *Sim, clock int) {
		if clock == secondRelease {
			deadlineBefore = sim.NextDecisionMS()
			sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.60, TargetX: 0.20, TargetY: 0.35, SpeedMPS: 14.0, Passer: -1})
		}
		if clock == secondRelease+SubstepMS {
			deadlineAfter = sim.NextDecisionMS()
		}
	}
	straight := mustSim(t, cfg, 9)
	setup(straight)
	for straight.ClockMS() < split {
		straight.Step()
	}
	nextAtSplit := straight.NextDecisionMS()
	resumed := t042SnapshotResume(t, straight, "second-release")
	if resumed.NextDecisionMS() != nextAtSplit {
		t.Fatalf("lattice moved across Resume: %d != %d", resumed.NextDecisionMS(), nextAtSplit)
	}
	pubsStraight := t042Run(t, straight, horizon, second)
	pubsResumed := t042Run(t, resumed, horizon, second)

	if secondRelease%cfg.IntentCadenceMS == 0 {
		t.Fatalf("second release clock %d is on the lattice; cannot exercise invalidation", secondRelease)
	}
	if deadlineBefore != 900 || deadlineAfter != 900 {
		t.Fatalf("periodic deadline at/after the second release was %d/%d, want 900/900 (lattice unshifted)",
			deadlineBefore, deadlineAfter)
	}
	if !containsInt(pubsStraight, secondRelease) {
		t.Fatalf("second-release invalidation did not publish at %dms: %v", secondRelease, pubsStraight)
	}
	// Full oracle for the tail: lattice ∪ invalidating-event clocks (release
	// INCLUDED, so the later/second release is predicted).
	lastStepStart := horizon - cfg.SubstepMS
	want := t042ExpectedPubs(cfg, straight.Events(), lastStepStart)
	var wantTail []int
	for _, c := range want {
		if c >= split {
			wantTail = append(wantTail, c)
		}
	}
	if !t042IntsEqual(pubsStraight, wantTail) {
		t.Fatalf("tail publication set %v != oracle %v (lattice unshifted, release included)", pubsStraight, wantTail)
	}
	if !t042IntsEqual(pubsStraight, pubsResumed) {
		t.Fatalf("second-release publication schedule differs across Resume:\n straight %v\n resumed  %v", pubsStraight, pubsResumed)
	}
	t042AssertTails(t, straight, resumed, "second-release")
	if countKind(straight, PassEventRelease) < 2 {
		t.Fatalf("expected a dynamic second release, got %d releases", countKind(straight, PassEventRelease))
	}
	t.Logf("second release @%dms (off-lattice): deadline %d unchanged, published at %d; lattice continued at %d; %d pubs identical across Resume",
		secondRelease, deadlineBefore, secondRelease, nextAtSplit, len(pubsStraight))
}

// --- 8. reaction between lattice clocks -----------------------------------

// TestT042ReactionFallsBetweenLatticeClocks proves a runner whose reaction
// deadline falls between two lattice clocks is not sensed early (never published
// an intent before its reaction) and IS activated at the first lattice clock at
// or after the reaction.
func TestT042ReactionFallsBetweenLatticeClocks(t *testing.T) {
	const cadence = 100
	const reactMS = 130 // falls between lattice clocks 100 and 200
	sim := mustSim(t, ScheduledLivePassConfig(cadence), 7)
	sim.SetRunners([]Runner{PursuingRunner(0.45, 0.50, DefaultMotionParams(), reactMS)})
	// A live release arms each runner's reaction deadline (ReactAtMS = clock +
	// ReactMS); a plain SetBall would leave it at zero.
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.45, TargetY: 0.50, SpeedMPS: 8.0, Passer: -1})

	initial := sim.Runners()[0]
	reactAt := sim.runners[0].ReactAtMS
	if reactAt != reactMS {
		t.Fatalf("ReactAtMS=%d, want %d", reactAt, reactMS)
	}
	firstSense := -1
	for sim.ClockMS() < 3000 && sim.Ball().Mode != BallHeld {
		start := sim.ClockMS()
		sim.Step()
		r := sim.Runners()[0]
		if firstSense < 0 && (r.IntentX != initial.IntentX || r.IntentY != initial.IntentY) {
			firstSense = start
		}
	}
	if firstSense < 0 {
		t.Fatal("runner never sensed the ball")
	}
	if firstSense%cadence != 0 {
		t.Fatalf("runner sensed off-lattice at %d ms", firstSense)
	}
	if firstSense < reactAt {
		t.Fatalf("runner sensed at %d ms before its %d ms reaction", firstSense, reactAt)
	}
	wantSense := ((reactAt + cadence - 1) / cadence) * cadence
	if firstSense != wantSense {
		t.Fatalf("first sense at %d ms, want the first lattice clock >= reaction (%d)", firstSense, wantSense)
	}
	for _, e := range sim.Events() {
		if e.Runner == 0 && (e.Kind == PassEventControl || e.Kind == PassEventDeflect) {
			// Use the PHYSICS substep (20 ms) for the exact-time reconstruction, not
			// the decision cadence (100 ms). The reaction gate is an integer
			// ReactAtMS with no cooldown rounding here, so the only tolerance is a
			// float numeric epsilon on the fractional contact time.
			if et := eventExactTimeMS(e, sim.Config().SubstepMS); et < float64(reactAt)-t042TimeEpsilon {
				t.Fatalf("runner touched at %.6f ms before its %d ms reaction", et, reactAt)
			}
		}
	}
	t.Logf("cadence=%dms, reaction=%dms: first sense at %dms (first lattice >= reaction); no early contact", cadence, reactAt, firstSense)
}

// --- 9. held tail: every body integrates, publication is a no-op ----------

// TestT042HeldTailIntegratesEveryBody proves a held ball keeps every body
// integrating every substep and that a held publication is a no-op on the
// pursuit-only layer, so the scheduled trajectory is bit-identical to the
// disabled one.
func TestT042HeldTailIntegratesEveryBody(t *testing.T) {
	setup := func(sim *Sim) {
		sim.SetRunners(inertial22())
		sim.ball = Ball{Mode: BallHeld, X: sim.runners[0].X, Y: sim.runners[0].Y}
		sim.contactRunner = 0
		sim.contactMS = 0
	}
	disabled := captureLiveTrajWith(t, LivePassConfig(), 1, setup, 2000, nil)
	scheduled := captureLiveTrajWith(t, ScheduledLivePassConfig(100), 1, setup, 2000, nil)
	assertSameTrajectory(t, disabled, scheduled)

	// Runtime guard: every one of the 22 bodies moves every substep while held.
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	setup(sim)
	prev := sim.Runners()
	steps := 0
	for sim.ClockMS() < 2000 {
		sim.Step()
		cur := sim.Runners()
		for i := range cur {
			if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
				t.Fatalf("held substep %d: runner %d did not move", steps, i)
			}
		}
		prev = cur
		steps++
	}
	if want := 2000 / SubstepMS; steps != want {
		t.Fatalf("held run did %d substeps, want %d", steps, want)
	}
	t.Logf("held tail: scheduled == disabled (bit-identical); all 22 bodies integrated %d/%d substeps", steps, steps)
}

// --- 10. deflection RNG / cooldown ----------------------------------------

// TestT042DeflectionConsumesRNGAndResume reproduces a real contact-local
// deflection under the scheduler: the deflection consumes RNG and sets the
// re-control cooldown; a JSON Resume taken right after it (pending invalidation)
// restores the RNG state, the cooldown and the remaining event tail exactly.
func TestT042DeflectionConsumesRNGAndResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(sim *Sim) {
		sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	straight := mustSim(t, cfg, 3)
	setup(straight)
	rng0 := straight.RNGState()
	for straight.ClockMS() < 4000 && countKind(straight, PassEventDeflect) == 0 {
		straight.Step()
	}
	if countKind(straight, PassEventDeflect) == 0 {
		t.Fatal("no deflection occurred")
	}
	if straight.RNGState() == rng0 {
		t.Fatal("deflection did not consume RNG")
	}
	if straight.runners[0].CooldownUntilMS <= straight.ClockMS() {
		t.Fatalf("deflector cooldown %d not active at %d", straight.runners[0].CooldownUntilMS, straight.ClockMS())
	}
	if !straight.decisionDirty {
		t.Fatal("deflection did not mark the scheduler dirty for the next step")
	}
	deflectClock := straight.ClockMS()
	cooldown := straight.runners[0].CooldownUntilMS
	resumed := t042SnapshotResume(t, straight, "after-deflection")
	if !resumed.decisionDirty {
		t.Fatal("pending invalidation lost across Resume")
	}
	t042Run(t, straight, 8000, nil)
	t042Run(t, resumed, 8000, nil)
	t042AssertTails(t, straight, resumed, "after-deflection")
	t.Logf("deflection @%dms consumed RNG, cooldown until %d; Resume reproduced RNG/cooldown/events",
		deflectClock, cooldown)
}

// --- 11. release between steps invalidates immediately --------------------

// TestT042ReleaseBetweenStepsInvalidatesNextStep proves a release invoked between
// steps (here a dynamic second release injected at a fixed clock) publishes on
// the very next substep, and that the consumed dirty marker does not leak.
func TestT042ReleaseBetweenStepsInvalidatesNextStep(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 5)
	sim.SetRunners([]Runner{PursuingRunner(0.50, 0.50, DefaultMotionParams(), 0)})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	// Advance to just after a lattice tick so the next step would NOT publish
	// periodically; then release between steps at clock 220.
	for sim.ClockMS() < 220 {
		sim.Step()
	}
	before := sim.DecisionPublishCount()
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	if !sim.decisionDirty {
		t.Fatal("release between steps did not mark the scheduler dirty")
	}
	start := sim.ClockMS()
	sim.Step()
	if sim.DecisionPublishCount() != before+1 {
		t.Fatalf("next step published %d times, want 1", sim.DecisionPublishCount()-before)
	}
	if sim.LastDecisionMS() != start {
		t.Fatalf("publication clock %d, want %d", sim.LastDecisionMS(), start)
	}
	if start%cfg.IntentCadenceMS == 0 {
		t.Fatalf("test clock %d accidentally on the lattice; cannot prove invalidation", start)
	}
	if sim.decisionDirty {
		t.Fatal("dirty marker not consumed after publication")
	}
	t.Logf("release between steps published at the next step %dms (off-lattice), dirty consumed", start)
}

// --- 12. coalesced same-substep events ------------------------------------

// TestT042CoalescedSameSubstepEvents proves that SEVERAL invalidating events in
// the SAME completed substep coalesce into exactly ONE publication at the next
// pre-step, and that a JSON Resume taken with that pending invalidation preserves
// the coalescing (one publication, dirty cleared once, complete snapshot equal).
//
// The fixture is the PROVEN deterministic one from
// TestLiveContestTwoEventsSameStepOrdered (intercept_test.go): ball just inside
// the line moving out fast, a defender behind it miscontrols it further out, so a
// deflect and an out happen in the SAME substep (clock 20), fraction-ordered.
func TestT042CoalescedSameSubstepEvents(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func() *Sim {
		sim := mustSim(t, cfg, 1)
		sim.controlError = 1.0 // force the miscontrol (test-only, no ReleasePass)
		sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
		sim.SetRunners([]Runner{{X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
		return sim
	}
	sim := build()
	rng0 := sim.RNGState()
	sim.Step() // one substep: deflect then out, both at ClockMS 20

	var def, out []PassEvent
	for _, e := range sim.Events() {
		switch e.Kind {
		case PassEventDeflect:
			def = append(def, e)
		case PassEventOut:
			out = append(out, e)
		}
	}
	if len(def) != 1 || len(out) != 1 {
		t.Fatalf("want exactly one deflect and one out in one step, got def=%d out=%d: %v", len(def), len(out), sim.Events())
	}
	eventClock := SubstepMS
	if def[0].ClockMS != eventClock || out[0].ClockMS != eventClock {
		t.Fatalf("coalesced events not on the same end-step clock: def=%d out=%d", def[0].ClockMS, out[0].ClockMS)
	}
	if def[0].Seq >= out[0].Seq || def[0].Frac > out[0].Frac {
		t.Fatalf("coalesced events not fraction-ordered: def(seq=%d frac=%v) out(seq=%d frac=%v)",
			def[0].Seq, def[0].Frac, out[0].Seq, out[0].Frac)
	}
	if sim.RNGState() == rng0 {
		t.Fatal("coalesced deflection did not consume RNG")
	}
	if !sim.decisionDirty {
		t.Fatal("coalesced events did not leave a single pending invalidation")
	}
	if sim.NextDecisionMS() != 100 {
		t.Fatalf("next periodic deadline %d, want 100", sim.NextDecisionMS())
	}
	if eventClock%cfg.IntentCadenceMS == 0 {
		t.Fatalf("coalesced clock %d is on the lattice", eventClock)
	}

	resumed := t042SnapshotResume(t, sim, "coalesced")
	if !resumed.decisionDirty {
		t.Fatal("pending coalesced invalidation lost across JSON Resume")
	}

	// Exactly ONE publication at the very next pre-step (20); the periodic
	// deadline 100 is unchanged by the event-only refresh, on both paths.
	b0, r0 := sim.DecisionPublishCount(), resumed.DecisionPublishCount()
	sim.Step()
	resumed.Step()
	if sim.DecisionPublishCount() != b0+1 || resumed.DecisionPublishCount() != r0+1 {
		t.Fatalf("coalesced events caused %d/%d publications, want 1/1",
			sim.DecisionPublishCount()-b0, resumed.DecisionPublishCount()-r0)
	}
	if sim.LastDecisionMS() != eventClock || resumed.LastDecisionMS() != eventClock {
		t.Fatalf("coalesced publication at %d/%dms, want %dms", sim.LastDecisionMS(), resumed.LastDecisionMS(), eventClock)
	}
	if sim.NextDecisionMS() != 100 || resumed.NextDecisionMS() != 100 {
		t.Fatalf("coalesced event-only refresh shifted the lattice: %d/%d, want 100",
			sim.NextDecisionMS(), resumed.NextDecisionMS())
	}
	if sim.decisionDirty || resumed.decisionDirty {
		t.Fatal("coalesced dirty marker not cleared exactly once")
	}
	t042Run(t, sim, 400, nil)
	t042Run(t, resumed, 400, nil)
	t042AssertTails(t, sim, resumed, "coalesced")
	t.Logf("proven fixture seed=1: deflect+out both @%dms coalesced to ONE next pre-step publication; deadline 100 unshifted; JSON Resume preserved it", eventClock)
}

// --- 13. stop / out events (real, not no-op fixtures) ---------------------

// TestT042StopEventInvalidatesAndResume: a live pass that stops short emits a real
// PassEventStop (BallLoose); the stop clock is off-lattice, so the ACTUAL
// publication timeline (returned by t042Run) must contain it, the periodic
// deadline must be unchanged by the event-only refresh, and a Resume across it is
// exact.
func TestT042StopEventInvalidatesAndResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const horizon = 4000
	setup := func(sim *Sim) {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
	}
	// Discovery: the deterministic stop clock for this fixture.
	disc := mustSim(t, cfg, 1)
	setup(disc)
	for disc.ClockMS() < horizon && countKind(disc, PassEventStop) == 0 {
		disc.Step()
	}
	if countKind(disc, PassEventStop) == 0 {
		t.Fatal("no PassEventStop emitted (fixture must actually stop)")
	}
	stopClock := disc.ClockMS()
	if stopClock%cfg.IntentCadenceMS == 0 {
		t.Fatalf("stop clock %d is on the lattice; fixture cannot prove invalidation", stopClock)
	}

	// ACTUAL publication tail + a per-pre-step deadline map (not oracle-only).
	sim := mustSim(t, cfg, 1)
	setup(sim)
	deadlineAt := map[int]int{}
	pubs := t042Run(t, sim, horizon, func(s *Sim, clock int) { deadlineAt[clock] = s.NextDecisionMS() })
	if !containsInt(pubs, stopClock) {
		t.Fatalf("stop clock %d not in the ACTUAL publications %v", stopClock, pubs)
	}
	if deadlineAt[stopClock] == 0 || deadlineAt[stopClock+cfg.SubstepMS] == 0 {
		t.Fatalf("deadline not recorded around stop clock %d: %v", stopClock, deadlineAt)
	}
	if deadlineAt[stopClock] != deadlineAt[stopClock+cfg.SubstepMS] {
		t.Fatalf("stop event-only refresh shifted the periodic deadline: %d -> %d",
			deadlineAt[stopClock], deadlineAt[stopClock+cfg.SubstepMS])
	}

	// Resume exactly across the pending stop invalidation.
	r := mustSim(t, cfg, 1)
	setup(r)
	for r.ClockMS() < stopClock {
		r.Step()
	}
	if !r.decisionDirty {
		t.Fatal("stop did not mark the scheduler dirty")
	}
	resumed := t042SnapshotResume(t, r, "stop")
	t042Run(t, r, horizon, nil)
	t042Run(t, resumed, horizon, nil)
	t042AssertTails(t, r, resumed, "stop")
	t.Logf("stop @%dms (off-lattice, BallLoose): actual pubs %v contain it; deadline %d unchanged; Resume exact",
		stopClock, pubs, deadlineAt[stopClock])
}

// TestT042OutEventInvalidatesAndResume: a rolling ball leaving the pitch emits a
// real PassEventOut at an off-lattice clock; the ACTUAL publication timeline must
// contain it, the periodic deadline must be unchanged, and Resume is exact.
// Fixture: X 0.994, VX 40 → physical exit ~15.75 ms, end-step clock 20 (off the
// 100 ms lattice). No runners.
func TestT042OutEventInvalidatesAndResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	const horizon = 400
	setup := func(sim *Sim) {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.994, Y: 0.50, VXM: 40.0})
	}
	disc := mustSim(t, cfg, 1)
	setup(disc)
	for disc.ClockMS() < horizon && countKind(disc, PassEventOut) == 0 {
		disc.Step()
	}
	if countKind(disc, PassEventOut) == 0 {
		t.Fatal("no PassEventOut emitted (fixture must actually leave the pitch)")
	}
	outClock := disc.ClockMS()
	if outClock%cfg.IntentCadenceMS == 0 {
		t.Fatalf("out clock %d is on the lattice; fixture cannot prove invalidation", outClock)
	}

	sim := mustSim(t, cfg, 1)
	setup(sim)
	deadlineAt := map[int]int{}
	pubs := t042Run(t, sim, horizon, func(s *Sim, clock int) { deadlineAt[clock] = s.NextDecisionMS() })
	if !containsInt(pubs, outClock) {
		t.Fatalf("out clock %d not in the ACTUAL publications %v", outClock, pubs)
	}
	if deadlineAt[outClock] == 0 || deadlineAt[outClock+cfg.SubstepMS] == 0 {
		t.Fatalf("deadline not recorded around out clock %d: %v", outClock, deadlineAt)
	}
	if deadlineAt[outClock] != deadlineAt[outClock+cfg.SubstepMS] {
		t.Fatalf("out event-only refresh shifted the periodic deadline: %d -> %d",
			deadlineAt[outClock], deadlineAt[outClock+cfg.SubstepMS])
	}

	r := mustSim(t, cfg, 1)
	setup(r)
	for r.ClockMS() < outClock {
		r.Step()
	}
	if !r.decisionDirty {
		t.Fatal("out did not mark the scheduler dirty")
	}
	resumed := t042SnapshotResume(t, r, "out")
	t042Run(t, r, horizon, nil)
	t042Run(t, resumed, horizon, nil)
	t042AssertTails(t, r, resumed, "out")
	t.Logf("out @%dms (off-lattice, BallOut): actual pubs %v contain it; deadline %d unchanged; Resume exact",
		outClock, pubs, deadlineAt[outClock])
}

// --- 14. unexported-state math sanity -------------------------------------

// TestT042LatticeAdvancesExactly guards the arithmetic: the next deadline is
// always the smallest lattice multiple strictly greater than the current clock.
func TestT042LatticeAdvancesExactly(t *testing.T) {
	cfg := ScheduledLivePassConfig(60)
	for _, clock := range []int{0, 20, 40, 60, 80, 120, 180} {
		sim := mustSim(t, cfg, 1)
		sim.clock = clock
		sim.nextDecisionMS = 0
		sim.decisionDirty = false
		sim.maybePublishIntents(clock)
		want := ((clock / 60) + 1) * 60
		if sim.nextDecisionMS != want {
			t.Fatalf("clock %d: next deadline %d, want %d", clock, sim.nextDecisionMS, want)
		}
	}
}
