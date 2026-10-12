package liveplay

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-043 local sampled ball perception tests (isolated prototype evidence).
//
// These tests exercise the OBSERVATION-ONLY per-runner ball record added in
// perception.go and sampled from the opt-in scheduler publication. The record
// must never change intent, contact, motion or RNG; a disabled fixture keeps the
// exact old JSON/trajectory shape; and every public state copy must deep-copy the
// mutable pointer so no live state is aliased. The schema and freshness contract
// are fixed in the session evidence BEFORE this code.

// t043Radius is the fixture sensing radius in metres.
const t043Radius = 4.0

// t043SensingRunner builds a stationary opted-in runner at (x,y).
func t043SensingRunner(t *testing.T, x, y, radiusM float64) Runner {
	t.Helper()
	r := Runner{X: x, Y: y}
	if err := r.EnableSensing(radiusM); err != nil {
		t.Fatalf("EnableSensing: %v", err)
	}
	return r
}

// t043Perception reads a copy of runner i's record.
func t043Perception(t *testing.T, sim *Sim, i int) BallObservation {
	t.Helper()
	o, ok := sim.Perception(i)
	if !ok {
		t.Fatalf("runner %d has no perception record", i)
	}
	return o
}

// t043SnapshotResume takes a JSON snapshot, asserts the perception state survives
// the round-trip and resumes it.
func t043SnapshotResume(t *testing.T, sim *Sim, label string) *Sim {
	t.Helper()
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var back Snapshot
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	if len(back.Runners) != len(sim.runners) {
		t.Fatalf("%s: runner count changed in JSON round-trip", label)
	}
	for i := range back.Runners {
		if !reflect.DeepEqual(back.Runners[i].Perception, sim.runners[i].Perception) {
			t.Fatalf("%s: runner %d perception lost: back=%+v live=%+v",
				label, i, back.Runners[i].Perception, sim.runners[i].Perception)
		}
	}
	resumed, err := Resume(back)
	if err != nil {
		t.Fatalf("%s: Resume: %v", label, err)
	}
	for i := range resumed.runners {
		if !reflect.DeepEqual(resumed.runners[i].Perception, sim.runners[i].Perception) {
			t.Fatalf("%s: Resume changed runner %d perception", label, i)
		}
	}
	return resumed
}

// --- 1. opt-in validation -------------------------------------------------

func TestT043EnableSensingValidation(t *testing.T) {
	var r Runner
	for _, bad := range []float64{0, -1, -0.5, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := r.EnableSensing(bad); !errors.Is(err, ErrSenseRadius) {
			t.Fatalf("EnableSensing(%v) err = %v, want ErrSenseRadius", bad, err)
		}
		if r.Perception != nil {
			t.Fatalf("EnableSensing(%v) installed a record after an error", bad)
		}
	}
	if err := r.EnableSensing(t043Radius); err != nil {
		t.Fatalf("valid radius rejected: %v", err)
	}
	if !r.SenseEnabled() {
		t.Fatal("positive radius did not enable sensing")
	}
	if r.Perception.RadiusM != t043Radius {
		t.Fatalf("radius %v, want %v", r.Perception.RadiusM, t043Radius)
	}
	r.DisableSensing()
	if r.SenseEnabled() || r.Perception != nil {
		t.Fatal("DisableSensing left a record behind")
	}
}

// TestT043ResumeRejectsBrokenObservation proves broken persisted input is
// reported, not silently resumed, including the contextual Seen schema.
func TestT043ResumeRejectsBrokenObservation(t *testing.T) {
	base := func(o *BallObservation, clockMS int) Snapshot {
		return Snapshot{
			Cfg:     ScheduledLivePassConfig(100),
			ClockMS: clockMS,
			RNG:     [4]uint64{1, 2, 3, 4},
			Runners: []Runner{{X: 0.5, Y: 0.5, Perception: o}},
		}
	}
	seen := func(o BallObservation) *BallObservation { o.Seen = true; return &o }

	cases := []struct {
		name string
		snap Snapshot
		want error
	}{
		{"infinite-radius", base(&BallObservation{RadiusM: math.Inf(1)}, 0), ErrSenseRadius},
		{"nan-radius", base(&BallObservation{RadiusM: math.NaN()}, 0), ErrSenseRadius},
		{"zero-radius", base(&BallObservation{RadiusM: 0}, 0), ErrSenseRadius},
		{"negative-radius", base(&BallObservation{RadiusM: -1}, 0), ErrSenseRadius},
		{"nan-coords", base(&BallObservation{RadiusM: 4, X: math.NaN()}, 0), ErrSenseCoordinates},
		{"future-clock", base(seen(BallObservation{RadiusM: 4, ClockMS: 200, Mode: BallRolling, Owner: -1}), 100), ErrSenseSchema},
		{"negative-clock", base(seen(BallObservation{RadiusM: 4, ClockMS: -20, Mode: BallRolling, Owner: -1}), 100), ErrSenseSchema},
		{"invalid-mode", base(seen(BallObservation{RadiusM: 4, ClockMS: 40, Mode: BallMode(99), Owner: -1}), 100), ErrSenseSchema},
		{"held-owner-out-of-range", base(seen(BallObservation{RadiusM: 4, ClockMS: 40, Mode: BallHeld, Owner: 5}), 100), ErrSenseSchema},
		{"held-owner-negative", base(seen(BallObservation{RadiusM: 4, ClockMS: 40, Mode: BallHeld, Owner: -1}), 100), ErrSenseSchema},
		{"unheld-owner-set", base(seen(BallObservation{RadiusM: 4, ClockMS: 40, Mode: BallRolling, Owner: 0}), 100), ErrSenseSchema},
	}
	for _, c := range cases {
		if _, err := Resume(c.snap); !errors.Is(err, c.want) {
			t.Fatalf("%s: Resume error = %v, want %v", c.name, err, c.want)
		}
	}

	// Valid contexts resume cleanly: a consistent Held owner and a never-seen
	// record with unused zero values.
	if _, err := Resume(base(seen(BallObservation{RadiusM: 4, ClockMS: 40, Mode: BallHeld, Owner: 0}), 100)); err != nil {
		t.Fatalf("Resume rejected a valid held observation: %v", err)
	}
	if _, err := Resume(base(&BallObservation{RadiusM: 4}, 100)); err != nil {
		t.Fatalf("Resume rejected a never-seen record: %v", err)
	}
	if _, err := Resume(base(nil, 100)); err != nil {
		t.Fatalf("Resume rejected a nil (disabled) record: %v", err)
	}
}

// TestT043SamplerFailsClosedOnInvalidRecord proves a raw record hand-set into
// Runner.Perception (bypassing EnableSensing, which SetRunners cannot reject)
// never leaks: SensingEnabled is finite-positive, and the sampler skips a
// non-finite radius or a NaN distance instead of admitting an all-world / NaN
// observation.
func TestT043SamplerFailsClosedOnInvalidRecord(t *testing.T) {
	for _, bad := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), 0, -1} {
		if o := (&BallObservation{RadiusM: bad}); o.SensingEnabled() {
			t.Fatalf("SensingEnabled(%v) = true, want false (fail closed)", bad)
		}
	}
	if !(&BallObservation{RadiusM: t043Radius}).SensingEnabled() {
		t.Fatal("valid radius must enable sensing")
	}

	// +Inf radius set directly would otherwise admit the whole pitch.
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := t043SensingRunner(t, 0.10, 0.10, t043Radius)
	r.Perception.RadiusM = math.Inf(1) // bypass EnableSensing
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 1.0})
	for sim.ClockMS() < 400 {
		sim.Step()
	}
	if o := t043Perception(t, sim, 0); o.Seen {
		t.Fatalf("+Inf radius leaked an all-world observation: %+v", o)
	}

	// A NaN ball coordinate must not be admitted by `NaN > radius == false`.
	sim2 := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim2.SetRunners([]Runner{t043SensingRunner(t, 0.50, 0.50, t043Radius)})
	sim2.SetBall(Ball{Mode: BallRolling, X: math.NaN(), Y: math.NaN(), VXM: 1.0})
	for sim2.ClockMS() < 400 {
		sim2.Step()
	}
	if o := t043Perception(t, sim2, 0); o.Seen {
		t.Fatalf("NaN distance admitted an observation: %+v", o)
	}
}

// --- 2. disabled contract and JSON keys -----------------------------------

// TestT043DisabledInertAndKeysUnchanged proves the zero value adds NO new JSON
// keys and that the disabled scheduler NEVER samples even when a runner carries a
// perception record. A perception record only appears when a runner opts in.
func TestT043DisabledInertAndKeysUnchanged(t *testing.T) {
	for _, cfg := range []Config{DefaultConfig(), LivePassConfig()} {
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal cfg: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal cfg: %v", err)
		}
		for _, bad := range []string{"perception", "intentCadenceMs", "senseRadiusM"} {
			if _, ok := m[bad]; ok {
				t.Fatalf("default config serialized %q: %s", bad, raw)
			}
		}
	}

	// A runner without a record serializes no perception key.
	raw, err := json.Marshal(Runner{X: 0.5, Y: 0.5})
	if err != nil {
		t.Fatalf("marshal runner: %v", err)
	}
	var rm map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rm); err != nil {
		t.Fatalf("unmarshal runner: %v", err)
	}
	if _, ok := rm["perception"]; ok {
		t.Fatalf("runner without a record serialized perception: %s", raw)
	}

	// Disabled scheduler: an opted-in runner exists but is NEVER sampled, and the
	// disabled snapshot keeps the old scheduler keys (none).
	sim := mustSim(t, LivePassConfig(), 3) // IntentCadenceMS == 0
	sim.SetRunners([]Runner{t043SensingRunner(t, 0.40, 0.50, 30.0)})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.42, Y: 0.50, VXM: 5.0})
	for i := 0; i < 50; i++ {
		sim.Step()
	}
	o := t043Perception(t, sim, 0)
	if o.Seen {
		t.Fatalf("disabled scheduler sampled the ball (Seen at clock %d)", o.ClockMS)
	}
	if o.Freshness(sim.ClockMS()) != ObservationUnknown {
		t.Fatalf("disabled record freshness = %s, want unknown", o.Freshness(sim.ClockMS()))
	}
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var sm map[string]json.RawMessage
	if err := json.Unmarshal(blob, &sm); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	for _, bad := range []string{"nextDecisionMs", "decisionDirty"} {
		if _, ok := sm[bad]; ok {
			t.Fatalf("disabled snapshot serialized scheduler key %q", bad)
		}
	}
}

// --- 3. no observation before the first in-range publication / early reaction

// TestT043NoObservationBeforeFirstInRangePublication proves sampling happens only
// on a publication: a far Dead ball is never observed, and a ball brought into
// range mid-lattice is first seen at the next due clock, never earlier.
func TestT043NoObservationBeforeFirstInRangePublication(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners([]Runner{t043SensingRunner(t, 0.50, 0.50, t043Radius)})
	sim.SetBall(Ball{Mode: BallDead, X: 0, Y: 0})

	for sim.ClockMS() < 40 {
		sim.Step()
	}
	if o := t043Perception(t, sim, 0); o.Seen {
		t.Fatalf("observed the far ball at clock %d", sim.ClockMS())
	}
	// Bring the ball into range between lattice clocks; the next due clock is 100.
	sim.SetBall(Ball{Mode: BallLoose, X: 0.52, Y: 0.50})
	firstSeen := -1
	var atSeen BallObservation
	for sim.ClockMS() < 300 {
		start := sim.ClockMS()
		sim.Step()
		if firstSeen < 0 && sim.runners[0].Perception.Seen {
			firstSeen = start
			atSeen = *sim.runners[0].Perception
		}
	}
	if firstSeen != 100 {
		t.Fatalf("first observation at %d ms, want the first due clock 100 (no earlier sampling)", firstSeen)
	}
	if atSeen.ClockMS != 100 || atSeen.Freshness(100) != ObservationFresh {
		t.Fatalf("record at first sighting = clock %d/%s, want 100/fresh", atSeen.ClockMS, atSeen.Freshness(100))
	}
	// By the horizon (300 ms) the ball is still in range, so the latest record
	// must be the 200 ms lattice refresh and stale relative to 300 ms.
	if o := t043Perception(t, sim, 0); o.ClockMS != 200 {
		t.Fatalf("latest record clock %d, want the 200 ms lattice refresh", o.ClockMS)
	}
}

// TestT043EarlyReactionNotSensed proves the reaction gate is applied to sensing
// exactly as to pursuit: a runner whose 130 ms reaction falls between the 100 ms
// lattice clocks first observes at the first lattice clock >= its reaction.
func TestT043EarlyReactionNotSensed(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := t043SensingRunner(t, 0.52, 0.50, t043Radius)
	sim.SetRunners([]Runner{r})
	sim.runners[0].ReactAtMS = 130 // between lattice clocks 100 and 200
	sim.SetBall(Ball{Mode: BallLoose, X: 0.54, Y: 0.50})

	firstSeen := -1
	for sim.ClockMS() < 600 {
		start := sim.ClockMS()
		sim.Step()
		if firstSeen < 0 && sim.runners[0].Perception.Seen {
			firstSeen = start
		}
	}
	if firstSeen != 200 {
		t.Fatalf("first observation at %d ms, want 200 (first lattice >= 130 reaction)", firstSeen)
	}
}

// --- 4. current vs stale / outside radius ---------------------------------

// TestT043CurrentThenStaleOutsideRadius proves a record is FRESH at the exact
// publication clock that saw the ball and, at a later DUE publication where the
// ball is actually OUTSIDE the circle (distance > radius), is STALE and
// byte-value-unchanged. A between-due frame is ALSO stale by sample-age even when
// the ball is still in range, so staleness alone is NOT proof of being outside.
func TestT043CurrentThenStaleOutsideRadius(t *testing.T) {
	const cadence = 100
	sim := mustSim(t, ScheduledLivePassConfig(cadence), 1)
	// Watcher offset 2 m off the ball lane: inside the 4 m sensing circle at
	// closest approach, outside the 1 m control reach, so it observes the rolling
	// ball without ever controlling it.
	sim.SetRunners([]Runner{t043SensingRunner(t, 0.50, 0.50+2.0/PitchWidthM, t043Radius)})
	// A ball rolling +x through the runner's circle, then away.
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})

	type t043Entry struct {
		clock int
		due   bool
		fresh ObservationFreshness
		dist  float64
		rec   BallObservation
	}
	var log []t043Entry
	for sim.ClockMS() < 4000 {
		start := sim.ClockMS()
		// The sampler reads the PRE-step ball, so capture the pre-step distance.
		preDist := DistM(sim.ball.X, sim.ball.Y, sim.runners[0].X, sim.runners[0].Y)
		sim.Step()
		o := sim.runners[0].Perception
		if o == nil || !o.Seen {
			continue
		}
		log = append(log, t043Entry{start, start%cadence == 0, o.Freshness(start), preDist, *o})
	}

	lastFresh := -1
	for i := range log {
		if log[i].due && log[i].fresh == ObservationFresh {
			lastFresh = i
		}
	}
	if lastFresh < 0 {
		t.Fatal("no due publication observed the ball")
	}
	freshRec := log[lastFresh].rec

	outside := -1
	for i := lastFresh + 1; i < len(log); i++ {
		if log[i].due && log[i].dist > t043Radius {
			outside = i
			break
		}
	}
	if outside < 0 {
		t.Fatal("no due publication found the ball actually OUTSIDE the radius after the last sighting")
	}
	out := log[outside]
	if out.fresh != ObservationStale {
		t.Fatalf("outside due record freshness = %s, want stale", out.fresh)
	}
	if out.dist <= t043Radius {
		t.Fatalf("outside due distance %.3f m <= radius %.3f m", out.dist, t043Radius)
	}
	if !reflect.DeepEqual(out.rec, freshRec) {
		t.Fatalf("record changed while actually outside range:\n fresh=%+v\n outside=%+v", freshRec, out.rec)
	}
	if out.rec.ClockMS != freshRec.ClockMS {
		t.Fatalf("outside record clock %d, want the last fresh clock %d", out.rec.ClockMS, freshRec.ClockMS)
	}
	if freshRec.Owner != -1 {
		t.Fatalf("fresh rolling record owner %d, want -1", freshRec.Owner)
	}

	// Diagnostic: a between-due frame is stale by sample-age while still IN range.
	inRangeStale := -1
	for i := range log {
		if !log[i].due && log[i].fresh == ObservationStale && log[i].dist <= t043Radius && log[i].clock > freshRec.ClockMS {
			inRangeStale = i
			break
		}
	}
	if inRangeStale < 0 {
		t.Fatal("expected a between-due stale frame still in range (to show stale != outside)")
	}
	t.Logf("last fresh due @%dms x=%.4f | first outside due @%dms dist=%.3fm record preserved | between-due stale in range @%dms dist=%.3fm (stale is sample-age, not proof of outside)",
		freshRec.ClockMS, freshRec.X, out.clock, out.dist, log[inRangeStale].clock, log[inRangeStale].dist)
}

// --- 5. closed metric range boundary --------------------------------------

// TestT043RangeBoundaryClosed proves the reach test is a CLOSED metre circle: a
// ball at exactly RadiusM is observed, a ball beyond it is not.
func TestT043RangeBoundaryClosed(t *testing.T) {
	// Chosen so the metres distance is exact in IEEE-754: 105/64 m across 1/64
	// normalized x-unit.
	const radius = 105.0 / 64.0
	atBoundary := 0.5 + 1.0/64.0
	if got := DistM(0.5, 0.5, atBoundary, 0.5); got != radius {
		t.Fatalf("fixture distance %v, want exact %v", got, radius)
	}
	run := func(ballX float64) BallObservation {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		sim.SetRunners([]Runner{t043SensingRunner(t, 0.5, 0.5, radius)})
		sim.SetBall(Ball{Mode: BallLoose, X: ballX, Y: 0.5})
		sim.Step() // publication at clock 0
		return t043Perception(t, sim, 0)
	}
	if o := run(atBoundary); !o.Seen {
		t.Fatal("ball exactly at the closed boundary was not observed")
	}
	if o := run(0.5 + 1.0/32.0); o.Seen {
		t.Fatal("ball beyond the boundary was observed")
	}
}

// --- 6. held structural delta: observable despite pursuit no-op -----------

// TestT043HeldObservationDespitePursuitNoop proves Held is observable even though
// the native pursuit layer no-ops while the ball is not Rolling/Loose, and the
// owner index is recorded.
func TestT043HeldObservationDespitePursuitNoop(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	owner := PursuingRunner(0.50, 0.50, DefaultMotionParams(), 0)
	watcher := t043SensingRunner(t, 0.52, 0.50, 8.0)
	sim.SetRunners([]Runner{owner, watcher})
	sim.SetBall(Ball{Mode: BallHeld, X: 0.50, Y: 0.50})
	sim.contactRunner = 0
	sim.contactMS = 0

	sim.Step() // publication at clock 0; ball is Held (pursuit no-op)
	o := t043Perception(t, sim, 1)
	if !o.Seen {
		t.Fatal("held ball was not observed by the local watcher")
	}
	if o.Mode != BallHeld {
		t.Fatalf("observed mode %s, want held", o.Mode)
	}
	if o.Owner != 0 {
		t.Fatalf("observed owner %d, want 0", o.Owner)
	}
	// The native pursuit layer never sensed anything (ball held).
	if sim.DecisionPublishCount() == 0 {
		t.Fatal("scheduler did not publish")
	}
}

// --- 7. local event invalidation, no remote leak ---------------------------

// TestT043EventInvalidationNoRemoteLeak proves a state-changing event schedules an
// extra publication but the radius gate still blocks any remote position leak.
func TestT043EventInvalidationNoRemoteLeak(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(200), 1)
	// Watcher 2 m from the ball: inside the 4 m sensing circle, outside the 1 m
	// control reach, so it observes without controlling.
	watcher := t043SensingRunner(t, 0.90+2.0/PitchLengthM, 0.50, t043Radius)
	sim.SetRunners([]Runner{watcher})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.50})
	sim.Step() // publication at clock 0 -> fresh
	o0 := t043Perception(t, sim, 0)
	if !o0.Seen || o0.ClockMS != 0 || o0.X != 0.90 {
		t.Fatalf("initial in-range observation wrong: %+v", o0)
	}

	// A remote dynamic release OFF the 200 ms lattice at clock 100.
	const remoteRelease = 100
	mid := func(s *Sim, clock int) {
		if clock == remoteRelease {
			s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.50, TargetX: 0.30, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
		}
	}
	pubs := t042Run(t, sim, 600, mid)
	if !containsInt(pubs, remoteRelease) {
		t.Fatalf("remote release did not schedule an invalidation publication: %v", pubs)
	}
	o1 := t043Perception(t, sim, 0)
	if !reflect.DeepEqual(o0, o1) {
		t.Fatalf("remote ball leaked into the local record:\n before=%+v\n after =%+v", o0, o1)
	}
	if o1.Freshness(sim.ClockMS()) != ObservationStale {
		t.Fatalf("record freshness %s, want stale (remote ball out of range)", o1.Freshness(sim.ClockMS()))
	}
	t.Logf("event-invalidation publication at %dms did not reveal the remote ball (record preserved)", remoteRelease)
}

// --- 8. observation-only: no trajectory/RNG change ------------------------

// TestT043ObservationDoesNotAlterTrajectory proves the perception sweep changes
// nothing observable: the same two-runner scene with and without sensing produces
// value-identical frames (positions/velocities/mode/owner) and events. The
// stronger RNG/no-draw check is TestT043ObservationOnlyRNGEquality.
func TestT043ObservationDoesNotAlterTrajectory(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()
	plain := func(sim *Sim) {
		sim.SetRunners([]Runner{
			PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
			PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
		})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	sensing := func(sim *Sim) {
		rs := []Runner{
			PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
			PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
		}
		for i := range rs {
			if err := rs[i].EnableSensing(40.0); err != nil {
				t.Fatalf("EnableSensing: %v", err)
			}
		}
		sim.SetRunners(rs)
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	base := captureLiveTrajWith(t, cfg, 1, plain, 4000, nil)
	observed := captureLiveTrajWith(t, cfg, 1, sensing, 4000, nil)
	assertSameTrajectory(t, base, observed)

	// And prove sensing actually happened in the observed run.
	sim := mustSim(t, cfg, 1)
	sensing(sim)
	seen := false
	for sim.ClockMS() < 4000 {
		sim.Step()
		for i := range sim.runners {
			if sim.runners[i].Perception.Seen {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("sensing run never recorded an observation; equality test is vacuous")
	}
}

// TestT043ObservationOnlyRNGEquality proves directly that sampling draws NO RNG
// and changes NO non-perception state: two identical runs (with and without
// sensing) finish with the same RNG state, and their complete snapshots are
// DeepEqual once the perception records are stripped. Frame/event equality alone
// could miss an extra draw that happened to cancel out.
func TestT043ObservationOnlyRNGEquality(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()
	scene := func(sim *Sim, sense bool) {
		rs := []Runner{
			PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
			PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
		}
		if sense {
			for i := range rs {
				if err := rs[i].EnableSensing(40.0); err != nil {
					t.Fatalf("EnableSensing: %v", err)
				}
			}
		}
		sim.SetRunners(rs)
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	plain := mustSim(t, cfg, 5)
	scene(plain, false)
	sensed := mustSim(t, cfg, 5)
	scene(sensed, true)
	for i := 0; i < 200; i++ {
		plain.Step()
		sensed.Step()
	}
	if plain.RNGState() != sensed.RNGState() {
		t.Fatalf("sensing changed the RNG state:\n plain  %v\n sensed %v", plain.RNGState(), sensed.RNGState())
	}
	a, b := plain.Snapshot(), sensed.Snapshot()
	stripPerception(&a)
	stripPerception(&b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("non-perception state differs with sensing on:\n plain  %+v\n sensed %+v", a, b)
	}
	// Guard: the sensed run really did sample, so the equality is not vacuous.
	sampled := false
	for i := range sensed.runners {
		if sensed.runners[i].Perception.Seen {
			sampled = true
		}
	}
	if !sampled {
		t.Fatal("sensing run recorded no observation; RNG-equality test is vacuous")
	}
}

// stripPerception clears every runner's observation record (test helper for the
// observation-only snapshot comparison).
func stripPerception(s *Snapshot) {
	for i := range s.Runners {
		s.Runners[i].Perception = nil
	}
}

// --- 9. deterministic full snapshot + Resume ------------------------------

// TestT043DeterministicFullSnapshotAndResume proves the complete serializable
// snapshot (not just the derived samples) is deterministic across identical runs
// and reproduced exactly across a JSON Resume before/at/after a due clock.
func TestT043DeterministicFullSnapshotAndResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(sim *Sim) {
		rs := []Runner{PursuingRunner(0.62, 0.50, DefaultMotionParams(), 0), PursuingRunner(0.78, 0.42, DefaultMotionParams(), 0)}
		for i := range rs {
			if err := rs[i].EnableSensing(30.0); err != nil {
				t.Fatalf("EnableSensing: %v", err)
			}
		}
		sim.SetRunners(rs)
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	}
	a := mustSim(t, cfg, 9)
	setup(a)
	for a.ClockMS() < 800 {
		a.Step()
	}
	b := mustSim(t, cfg, 9)
	setup(b)
	for b.ClockMS() < 800 {
		b.Step()
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("two identical runs produced different complete snapshots")
	}
	if !a.runners[0].Perception.Seen {
		t.Fatal("sensing run recorded no observation; snapshot equality test is weak")
	}

	for _, split := range []int{60, 100, 120, 700} {
		straight := mustSim(t, cfg, 9)
		setup(straight)
		for straight.ClockMS() < split {
			straight.Step()
		}
		resumed := t043SnapshotResume(t, straight, "split")
		t042Run(t, straight, 3000, nil)
		t042Run(t, resumed, 3000, nil)
		t042AssertTails(t, straight, resumed, "perception-resume")
		t.Logf("split@%dms: complete snapshot + perception resumed value-identically", split)
	}
}

// TestT043ResumeStaleOutsideAndPendingEventRetained covers the two remaining
// Resume contexts: (A) a STALE last-seen record whose ball is actually OUTSIDE the
// radius, and (B) a state-changing deflection that leaves a pending scheduler
// invalidation (decisionDirty) plus a retained observation. Both take a JSON
// snapshot, assert the record + dirty flag survive, and prove the complete
// Snapshot and sample tail resume value-identically via the T-042 helper.
func TestT043ResumeStaleOutsideAndPendingEventRetained(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	// (A) stale last-seen OUTSIDE radius.
	staleSim := mustSim(t, cfg, 1)
	staleSim.SetRunners([]Runner{t043SensingRunner(t, 0.50, 0.50+2.0/PitchWidthM, t043Radius)})
	staleSim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	for staleSim.ClockMS() < 3000 {
		staleSim.Step()
	}
	o := t043Perception(t, staleSim, 0)
	dist := DistM(staleSim.ball.X, staleSim.ball.Y, staleSim.runners[0].X, staleSim.runners[0].Y)
	if !o.Seen || o.Freshness(staleSim.ClockMS()) != ObservationStale || dist <= t043Radius {
		t.Fatalf("stale-outside fixture not reached: seen=%v freshness=%s dist=%.3f", o.Seen, o.Freshness(staleSim.ClockMS()), dist)
	}
	staleResumed := t043SnapshotResume(t, staleSim, "stale-outside")
	if got := t043Perception(t, staleResumed, 0); !reflect.DeepEqual(got, o) {
		t.Fatalf("stale record not retained across Resume:\n before=%+v\n after =%+v", o, got)
	}
	t042Run(t, staleSim, 6000, nil)
	t042Run(t, staleResumed, 6000, nil)
	t042AssertTails(t, staleSim, staleResumed, "stale-outside")
	t.Logf("stale-outside @%dms dist=%.3fm: record + complete snapshot resumed value-identically", staleSim.ClockMS(), dist)

	// (B) post-deflection with a pending invalidation and a retained observation.
	deflectSetup := func(sim *Sim) {
		r := Runner{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}
		if err := r.EnableSensing(30.0); err != nil {
			t.Fatalf("EnableSensing: %v", err)
		}
		sim.SetRunners([]Runner{r})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	}
	deflectSim := mustSim(t, cfg, 3)
	deflectSetup(deflectSim)
	for deflectSim.ClockMS() < 4000 && countKind(deflectSim, PassEventDeflect) == 0 {
		deflectSim.Step()
	}
	if countKind(deflectSim, PassEventDeflect) == 0 {
		t.Fatal("no deflection occurred")
	}
	if !deflectSim.decisionDirty {
		t.Fatal("deflection did not leave a pending scheduler invalidation")
	}
	snap := deflectSim.Snapshot()
	if snap.Runners[0].Perception == nil || !snap.Runners[0].Perception.Seen {
		t.Fatalf("observation not retained in the post-deflection snapshot: %+v", snap.Runners[0].Perception)
	}
	deflectResumed := t043SnapshotResume(t, deflectSim, "post-deflection")
	if !deflectResumed.decisionDirty {
		t.Fatal("pending invalidation lost across Resume")
	}
	t042Run(t, deflectSim, 8000, nil)
	t042Run(t, deflectResumed, 8000, nil)
	t042AssertTails(t, deflectSim, deflectResumed, "post-deflection")
	t.Logf("post-deflection @%dms: pending dirty + observation retained, complete snapshot resumed value-identically", deflectSim.ClockMS())
}

// --- 10. public state-copy ownership --------------------------------------

// TestT043PublicCopiesDoNotAlias proves every public copy path deep-copies the
// mutable perception record so a caller can never corrupt live state.
func TestT043PublicCopiesDoNotAlias(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	sim.SetRunners([]Runner{t043SensingRunner(t, 0.5, 0.5, 5.0)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.5, Y: 0.5})
	sim.Step() // sample

	got := sim.Runners()
	got[0].Perception.RadiusM = 999
	if sim.runners[0].Perception.RadiusM == 999 {
		t.Fatal("Runners() aliased the live perception pointer")
	}

	o := t043Perception(t, sim, 0)
	o.RadiusM = 777
	if sim.runners[0].Perception.RadiusM == 777 {
		t.Fatal("Perception() exposed the live perception pointer")
	}

	snap := sim.Snapshot()
	snap.Runners[0].Perception.X = -123
	if sim.runners[0].Perception.X == -123 {
		t.Fatal("Snapshot() aliased the live perception pointer")
	}

	caller := []Runner{t043SensingRunner(t, 0.3, 0.3, 6.0)}
	sim.SetRunners(caller)
	caller[0].Perception.RadiusM = 42
	if sim.runners[0].Perception.RadiusM == 42 {
		t.Fatal("SetRunners() aliased the caller's perception pointer")
	}

	resumed, err := Resume(sim.Snapshot())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	src := sim.Snapshot()
	resumed2, err := Resume(src)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	src.Runners[0].Perception.RadiusM = 55
	if resumed2.runners[0].Perception.RadiusM == 55 {
		t.Fatal("Resume() aliased the source snapshot's perception pointer")
	}
	_ = resumed
}

// --- 11. enabled JSON shape ------------------------------------------------

// TestT043EnabledSnapshotSerializesPerception proves the record (radius + last
// seen) is serialized when armed and keeps a legitimate zero coordinate.
func TestT043EnabledSnapshotSerializesPerception(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	r := t043SensingRunner(t, 0.0, 0.0, t043Radius) // ball at x=0 must not vanish
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.0, Y: 0.0})
	sim.Step()

	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(blob, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["runners"]; !ok {
		t.Fatalf("snapshot missing runners: %s", blob)
	}
	var runnerMaps []map[string]json.RawMessage
	if err := json.Unmarshal(m["runners"], &runnerMaps); err != nil {
		t.Fatalf("unmarshal runners: %v", err)
	}
	pm, ok := runnerMaps[0]["perception"]
	if !ok {
		t.Fatalf("armed snapshot omitted the perception record: %s", blob)
	}
	var rec map[string]json.RawMessage
	if err := json.Unmarshal(pm, &rec); err != nil {
		t.Fatalf("unmarshal perception: %v", err)
	}
	for _, key := range []string{"radiusM", "seen", "clockMs", "x", "y", "vxM", "vyM", "mode", "owner"} {
		if _, ok := rec[key]; !ok {
			t.Fatalf("perception record missing %q: %s", key, pm)
		}
	}
	o := t043Perception(t, sim, 0)
	if o.X != 0 || o.Y != 0 {
		t.Fatalf("zero coordinates lost: %+v", o)
	}
}

// TestT043ConfigAndRunnerUnchangedAsideFromOptIn is a tiny regression guard that a
// default live runner still carries no perception.
func TestT043ConfigAndRunnerUnchangedAsideFromOptIn(t *testing.T) {
	r := PursuingRunner(0.5, 0.5, DefaultMotionParams(), 0)
	if r.Perception != nil || r.SenseEnabled() {
		t.Fatal("a default runner opted into sensing implicitly")
	}
}
