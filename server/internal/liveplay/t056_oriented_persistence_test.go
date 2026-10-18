package liveplay

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-056 oriented persistence/import/clone tests: transactional activation with
// strict non-unit rejection, model selection, complete JSON Resume at mid-arrival
// (slowing active), before/after second release and mid-turn, lock-time movement,
// model2-without-orientation acceptance, invalid-orientation rejection and
// caller/snapshot/resume isolation.

// t056OrientedActive22 builds the T-053 active22 fixture with slot 0 oriented and
// the integrated shell enabled (selects the oriented model2).
func t056OrientedActive22(t *testing.T, seed uint64, control bool) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	rs := t055AssignIDs(t053Runners(control))
	o, err := NewOrientation(1, 0, 1)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	rs[0].Orientation = &o
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
	return sim
}

// t056ArrivalSidecarScene: an oriented runner arriving with a binding slowing
// radius (20 m, explicit fixture value) plus two local observers that carry the
// T-049 sidecar.
func t056ArrivalSidecarScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	o, err := NewOrientation(1, 0, 1)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	o.SlowingRadiusM = 20.0
	passer := InertialRunner(0.10, 0.50, 0.20, 0.50, DefaultMotionParams())
	passer.Orientation = &o
	r1 := t050Local(0.30, 0.53, 0.80, 0.53, 200.0, 0, true)
	r2 := t050Local(0.10, 0.30, 0.90, 0.30, 200.0, 0, true)
	sim.SetRunners(t055AssignIDs([]Runner{passer, r1, r2}))
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 8.0, DirectionJitterMPS: 0.5, WeightJitterFrac: 0.05, Passer: 0})
	return sim
}

// t056RawRunner builds a runner with a hand-set (possibly invalid) orientation,
// bypassing NewOrientation so raw-import rejection can be tested.
func t056RawRunner(playerID uint64, o RunnerOrientation) Runner {
	r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	r.PlayerID = playerID
	r.Orientation = &o
	return r
}

// t056NaNSentinel replaces NaN in an orientation copy so a before/after compare of
// an UNCHANGED state is stable (reflect.DeepEqual never equates NaN).
const t056NaNSentinel = 3.0e300

func t056NormNaN(s Snapshot) Snapshot {
	for i := range s.Runners {
		o := s.Runners[i].Orientation
		if o == nil {
			continue
		}
		c := *o
		if math.IsNaN(c.FX) {
			c.FX = t056NaNSentinel
		}
		if math.IsNaN(c.FY) {
			c.FY = t056NaNSentinel
		}
		if math.IsNaN(c.StandTurnRadsPerSec) {
			c.StandTurnRadsPerSec = t056NaNSentinel
		}
		if math.IsNaN(c.BackwardSpeedShare) {
			c.BackwardSpeedShare = t056NaNSentinel
		}
		if math.IsNaN(c.SlowingRadiusM) {
			c.SlowingRadiusM = t056NaNSentinel
		}
		if math.IsNaN(c.KickAlignmentSpread) {
			c.KickAlignmentSpread = t056NaNSentinel
		}
		s.Runners[i].Orientation = &c
	}
	return s
}

// TestT056EnableModelAndTransactional pins model selection, strict non-unit
// rejection on EnableOrientation (transactional), and side parity.
func TestT056EnableModelAndTransactional(t *testing.T) {
	plain := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	plain.PlayerID = 1
	sim := t056MotionSim(t, []Runner{plain})
	if st, _ := sim.IntegratedState(); st.ModelVersion != IntegratedModelVersion {
		t.Fatalf("non-oriented enable model %d, want 1", st.ModelVersion)
	}
	before := sim.Snapshot()
	rng := sim.RNGState()
	bad, err := NewOrientation(1, 0, 1)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	bad.StandTurnRadsPerSec = -1
	if err := sim.EnableOrientation(0, bad); err == nil {
		t.Fatal("EnableOrientation accepted a non-positive stand turn limit")
	}
	if !reflect.DeepEqual(before, sim.Snapshot()) || rng != sim.RNGState() {
		t.Fatal("invalid EnableOrientation mutated the state/RNG")
	}

	base := RunnerOrientation{FX: 1, FY: 0, TurnSide: 1, StandTurnRadsPerSec: 4, BackwardSpeedShare: 0.45, SlowingRadiusM: 3, KickAlignmentSpread: 1}
	nan := base
	nan.FX = math.NaN()
	badSide := base
	badSide.TurnSide = 7
	zero := base
	zero.FX, zero.FY = 0, 0
	for _, tc := range []struct {
		name string
		o    RunnerOrientation
	}{{"non-unit (2,0)", func() RunnerOrientation { x := base; x.FX = 2; return x }()}, {"zero vector", zero}, {"nonfinite", nan}, {"bad side", badSide}} {
		b2 := sim.Snapshot()
		r2 := sim.RNGState()
		if err := sim.EnableOrientation(0, tc.o); err == nil {
			t.Fatalf("EnableOrientation accepted %s", tc.name)
		}
		if !reflect.DeepEqual(t056NormNaN(b2), t056NormNaN(sim.Snapshot())) || r2 != sim.RNGState() {
			t.Fatalf("EnableOrientation %s mutated the state/RNG", tc.name)
		}
	}

	good, _ := NewOrientation(0, 1, 0)
	if err := sim.EnableOrientation(0, good); err != nil {
		t.Fatalf("EnableOrientation: %v", err)
	}
	st, _ := sim.IntegratedState()
	if st.ModelVersion != IntegratedModelVersionOriented {
		t.Fatalf("orientation did not select model2: %d", st.ModelVersion)
	}
	f := t056Face(t, sim, 0)
	if f.FX != good.FX || f.FY != good.FY || f.TurnSide != 1 {
		t.Fatalf("activation changed supplied bits or parity: %+v want %+v", f, good)
	}

	even := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	even.PlayerID = 2
	sim2 := t056MotionSim(t, []Runner{even})
	g2, _ := NewOrientation(1, 0, 0)
	if err := sim2.EnableOrientation(0, g2); err != nil {
		t.Fatalf("EnableOrientation: %v", err)
	}
	if side := t056Face(t, sim2, 0).TurnSide; side != -1 {
		t.Fatalf("even PlayerID side %d, want -1", side)
	}

	simM := mustSim(t, LivePassConfig(), 1)
	orM := t056Oriented(t, 0.5, 0.5, 0.5, 0.5, 1, 0, 1)
	orM.Motion = MotionKinematic
	simM.SetRunners([]Runner{orM})
	if err := simM.EnableIntegrated(); !errors.Is(err, ErrOrientationMotion) {
		t.Fatalf("kinematic orientation enable err=%v, want ErrOrientationMotion", err)
	}
	if err := mustSim(t, LivePassConfig(), 1).EnableOrientation(0, good); !errors.Is(err, ErrOrientationNoIntegrated) {
		t.Fatalf("orientation without integrated err=%v", err)
	}
	if err := sim.EnableOrientation(5, good); !errors.Is(err, ErrOrientationIndex) {
		t.Fatalf("out-of-range orientation err=%v", err)
	}
	after := t056MotionSim(t, []Runner{plain})
	after.ReleasePass(t052InteriorPass(2.0, 0.0))
	if err := after.EnableOrientation(0, good); !errors.Is(err, ErrOrientationNotInitial) {
		t.Fatalf("late orientation err=%v", err)
	}
	t.Logf("T-056 activation: model1->model2, strict non-unit rejection, parity, and motion/index/no-integrated/no-event gates enforced")
}

// TestT056RawRosterTransactional pins the raw-roster activation contract: an
// invalid orientation anywhere rejects the WHOLE roster transactionally, with no
// partial resolution of an earlier valid runner, and the full state/RNG unchanged.
func TestT056RawRosterTransactional(t *testing.T) {
	valid := RunnerOrientation{FX: 1, FY: 0, TurnSide: 0, StandTurnRadsPerSec: 4, BackwardSpeedShare: 0.45, SlowingRadiusM: 3, KickAlignmentSpread: 1}
	nonunit := RunnerOrientation{FX: 2, FY: 0, TurnSide: 1, StandTurnRadsPerSec: 4, BackwardSpeedShare: 0.45, SlowingRadiusM: 3, KickAlignmentSpread: 1}

	// (a) valid first runner, BAD second: the earlier must NOT be resolved/committed.
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners([]Runner{t056RawRunner(1, valid), t056RawRunner(2, nonunit)})
	before := sim.Snapshot()
	rngB := sim.RNGState()
	if err := sim.EnableIntegrated(); err == nil {
		t.Fatal("EnableIntegrated accepted a bad second-runner orientation")
	}
	if sim.IntegratedEnabled() {
		t.Fatal("EnableIntegrated mutated the shell on a bad orientation")
	}
	if sim.runners[0].Orientation.TurnSide != 0 {
		t.Fatalf("earlier valid runner was partially resolved (side %d)", sim.runners[0].Orientation.TurnSide)
	}
	if !reflect.DeepEqual(t056NormNaN(before), t056NormNaN(sim.Snapshot())) || rngB != sim.RNGState() {
		t.Fatal("failed EnableIntegrated mutated the full state/RNG")
	}

	// (b) table of bad single-runner raw orientations.
	nan := valid
	nan.FX = math.NaN()
	badSide := valid
	badSide.TurnSide = 7
	zero := valid
	zero.FX, zero.FY = 0, 0
	badParam := valid
	badParam.StandTurnRadsPerSec = 0
	for _, tc := range []struct {
		name string
		o    RunnerOrientation
	}{{"zero", zero}, {"nonfinite", nan}, {"bad side", badSide}, {"bad params", badParam}} {
		s := mustSim(t, LivePassConfig(), 1)
		s.SetRunners([]Runner{t056RawRunner(1, tc.o)})
		b := s.Snapshot()
		r := s.RNGState()
		if err := s.EnableIntegrated(); err == nil {
			t.Fatalf("raw %s orientation accepted", tc.name)
		}
		if s.IntegratedEnabled() {
			t.Fatalf("raw %s orientation enabled the shell", tc.name)
		}
		if !reflect.DeepEqual(t056NormNaN(b), t056NormNaN(s.Snapshot())) || r != s.RNGState() {
			t.Fatalf("raw %s orientation mutated the full state/RNG", tc.name)
		}
		if tc.name == "nonfinite" && !math.IsNaN(s.runners[0].Orientation.FX) {
			t.Fatal("nonfinite raw orientation field changed")
		}
	}
	t.Logf("T-056 raw roster: invalid orientation (incl. a bad SECOND runner) rejected transactionally; no partial resolve, full state/RNG unchanged")
}

// t056ArrivalStrictSlowing reports whether the arrival scene's oriented runner is
// ever strictly slower than a matched-prestate nil twin (actual runtime slowing).
func t056ArrivalStrictSlowing(t *testing.T, seed uint64) bool {
	t.Helper()
	sim := t056ArrivalSidecarScene(t, seed)
	strict := false
	for step := 0; step < 80; step++ {
		pre := sim.Runners()[0]
		twin := pre
		twin.Orientation = nil
		rn := t056StepRunner(t, twin)
		sim.Step()
		if cur := sim.Runners()[0]; cur.SpeedMPS() < rn.SpeedMPS()-1e-9 {
			strict = true
		}
	}
	return strict
}

// TestT056JSONResumeMidArrival covers mid-arrival splits (slowing cap active) and
// before/after second-release splits through complete JSON Resume, with exact
// nonempty absolute event/journal/sample tails and a nonempty equal sidecar.
func TestT056JSONResumeMidArrival(t *testing.T) {
	const end = 1600
	mid := func(s *Sim, clock int) {
		switch clock {
		case 1000:
			s.ReleasePass(PassParams{StartX: 0.55, StartY: 0.50, TargetX: 0.70, TargetY: 0.50, SpeedMPS: 3.0, DirectionJitterMPS: 0.4, WeightJitterFrac: 0.04, Passer: 0})
		case 1400:
			s.ReleasePass(PassParams{StartX: 0.60, StartY: 0.50, TargetX: 0.75, TargetY: 0.50, SpeedMPS: 3.0, DirectionJitterMPS: 0.4, WeightJitterFrac: 0.04, Passer: 0})
		}
	}
	// Actual runtime slowing (matched-prestate nil) happens at some stage.
	if !t056ArrivalStrictSlowing(t, 11) {
		t.Fatal("arrival scene never slowed strictly below the matched nil twin")
	}
	for _, split := range []int{300, 500, 1020} {
		straight := t056ArrivalSidecarScene(t, 11)
		for straight.ClockMS() < split {
			mid(straight, straight.ClockMS())
			straight.Step()
		}
		if straight.ClockMS() != split {
			t.Fatalf("split %d reached %d", split, straight.ClockMS())
		}
		if split < 1000 {
			// The slowing CAP is active at the mid-arrival split (not a claim about
			// the instantaneous actual speed).
			pre := straight.Runners()[0]
			d := DistM(pre.X, pre.Y, pre.IntentX, pre.IntentY)
			nilW := math.Sqrt(2 * DefaultMotionParams().DecelMPS2 * d)
			orW := DefaultMotionParams().TopSpeedMPS * d / pre.Orientation.SlowingRadiusM
			if !(orW < nilW) {
				t.Fatalf("split %d: slowing cap not active (oriented %.4f, nil %.4f)", split, orW, nilW)
			}
		}
		jAt := len(mustIntegratedJournal(t, straight))
		evAt := len(straight.Events())
		sampAt := len(straight.Samples())
		resumed := t046JSONResume(t, straight)
		if !resumed.IntegratedEnabled() {
			t.Fatalf("split %d: resume lost the shell", split)
		}
		if !reflect.DeepEqual(resumed.Events(), straight.Events()) {
			t.Fatalf("split %d: persisted event prefix not preserved", split)
		}
		srec := mustRecorder(t, 8192)
		straight.SetProvenanceRecorder(srec)
		rrec := mustRecorder(t, 8192)
		resumed.SetProvenanceRecorder(rrec)
		for straight.ClockMS() < end {
			mid(straight, straight.ClockMS())
			mid(resumed, resumed.ClockMS())
			straight.Step()
			resumed.Step()
			if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
				t.Fatalf("split %d: full snapshot diverged at %d", split, straight.ClockMS())
			}
		}
		if !reflect.DeepEqual(resumed.Samples(), straight.Samples()[sampAt:]) {
			t.Fatalf("split %d: sample tails differ", split)
		}
		sj := mustIntegratedJournal(t, straight)
		rj := mustIntegratedJournal(t, resumed)
		if !reflect.DeepEqual(sj, rj) {
			t.Fatalf("split %d: journal differs after resume", split)
		}
		if !reflect.DeepEqual(straight.Events()[evAt:], resumed.Events()[evAt:]) {
			t.Fatalf("split %d: event suffix differs", split)
		}
		if len(sj) <= jAt || len(straight.Events()) <= evAt || len(resumed.Samples()) == 0 {
			t.Fatalf("split %d: empty future journal/event/sample suffix (jAt=%d evAt=%d)", split, jAt, evAt)
		}
		if !reflect.DeepEqual(srec.Records(), rrec.Records()) || len(rrec.Records()) == 0 {
			t.Fatalf("split %d: sidecar tail empty or differs (%d/%d)", split, len(srec.Records()), len(rrec.Records()))
		}
		t.Logf("T-056 JSON split=%d -> %d: full snapshot equal every substep; journal suffix %d, event suffix %d, sample suffix %d, sidecar records %d (nonempty, equal)",
			split, end, len(sj)-jAt, len(straight.Events())-evAt, len(resumed.Samples()), len(rrec.Records()))
	}

	// Mid-turn split via the active22 oriented fixture.
	st := t056OrientedActive22(t, t053ControlSeed, true)
	t053StepToWithMid(st, 60, t053MidSecondRelease)
	f0 := t056Face(t, st, 0)
	res := t046JSONResume(t, st)
	for st.ClockMS() < 600 {
		t053MidSecondRelease(st, st.ClockMS())
		t053MidSecondRelease(res, res.ClockMS())
		st.Step()
		res.Step()
		if !reflect.DeepEqual(st.Snapshot(), res.Snapshot()) {
			t.Fatalf("mid-turn resume diverged at %d", st.ClockMS())
		}
	}
	if f1 := t056Face(t, st, 0); f1.FX == f0.FX && f1.FY == f0.FY {
		t.Fatal("mid-turn fixture did not turn")
	}
	t.Logf("T-056 JSON mid-turn split=60 -> 600: full snapshot equal every substep; facing (%.4f,%.4f) -> (%.4f,%.4f)", f0.FX, f0.FY, t056Face(t, st, 0).FX, t056Face(t, st, 0).FY)
}

// TestT056InvalidOrientationImportsAndClones rejects inconsistent oriented
// snapshots and proves isolation (the isolation checks run on the ORIENTED sim,
// before any non-oriented replacement).
func TestT056InvalidOrientationImportsAndClones(t *testing.T) {
	sim := t056OrientedActive22(t, t053ControlSeed, true)
	for sim.ClockMS() < 200 {
		sim.Step()
	}
	base := sim.Snapshot()
	if _, err := Resume(base); err != nil {
		t.Fatalf("baseline oriented resume should succeed: %v", err)
	}
	baseCopy := deepCopySnapshot(t, base)
	reject := func(label string, s Snapshot) {
		t.Helper()
		if _, err := Resume(s); err == nil {
			t.Fatalf("%s: Resume accepted an inconsistent oriented snapshot", label)
		}
	}
	mut := func(f func(*Snapshot)) Snapshot { s := deepCopySnapshot(t, baseCopy); f(&s); return s }
	reject("model1+orientation", mut(func(s *Snapshot) { s.Integrated.ModelVersion = IntegratedModelVersion }))
	reject("unknown model99", mut(func(s *Snapshot) { s.Integrated.ModelVersion = 99 }))
	reject("kinematic orientation", mut(func(s *Snapshot) { s.Runners[0].Motion = MotionKinematic }))
	reject("non-unit facing", mut(func(s *Snapshot) { s.Runners[0].Orientation.FX = 2; s.Runners[0].Orientation.FY = 0 }))
	reject("zero facing", mut(func(s *Snapshot) { s.Runners[0].Orientation.FX = 0; s.Runners[0].Orientation.FY = 0 }))
	reject("nan facing", mut(func(s *Snapshot) { s.Runners[0].Orientation.FX = math.NaN() }))
	reject("bad side 0", mut(func(s *Snapshot) { s.Runners[0].Orientation.TurnSide = 0 }))
	reject("bad side 2", mut(func(s *Snapshot) { s.Runners[0].Orientation.TurnSide = 2 }))
	reject("nonpositive standOmega", mut(func(s *Snapshot) { s.Runners[0].Orientation.StandTurnRadsPerSec = 0 }))
	reject("backwardShare>1", mut(func(s *Snapshot) { s.Runners[0].Orientation.BackwardSpeedShare = 1.5 }))
	reject("backwardShare<0", mut(func(s *Snapshot) { s.Runners[0].Orientation.BackwardSpeedShare = -0.1 }))
	reject("slowingRadius<=0", mut(func(s *Snapshot) { s.Runners[0].Orientation.SlowingRadiusM = -1 }))
	reject("negative kick coef", mut(func(s *Snapshot) { s.Runners[0].Orientation.KickAlignmentSpread = -1 }))
	reject("zero player id", mut(func(s *Snapshot) { s.Runners[0].PlayerID = 0 }))
	reject("orientation without integrated", mut(func(s *Snapshot) { s.Integrated = nil }))

	// Isolation BEFORE any non-oriented replacement (the oriented roster is here).
	o0 := t056Face(t, sim, 0)
	rs := sim.Runners()
	rs[0].Orientation.FX = -1
	rs[0].Orientation.FY = 0
	if got := t056Face(t, sim, 0); got.FX != o0.FX || got.FY != o0.FY {
		t.Fatal("Runners() aliases live orientation")
	}
	sn := sim.Snapshot()
	sn.Runners[0].Orientation.FX = -1
	if got := t056Face(t, sim, 0); got.FX != o0.FX {
		t.Fatal("Snapshot aliases live orientation")
	}
	res, err := Resume(sim.Snapshot())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	res.runners[0].Orientation.FX = -1
	if got := t056Face(t, sim, 0); got.FX != o0.FX {
		t.Fatal("Resume aliases live orientation")
	}

	// model2 WITHOUT orientation is accepted, and a raw non-oriented replacement
	// does not downgrade it (run last: it removes orientation from the sim).
	noOr := deepCopySnapshot(t, baseCopy)
	for i := range noOr.Runners {
		noOr.Runners[i].Orientation = nil
	}
	if _, err := Resume(noOr); err != nil {
		t.Fatalf("model2 without orientation should resume: %v", err)
	}
	sim.SetRunners(t055AssignIDs(t053Runners(true)))
	if st, _ := sim.IntegratedState(); st.ModelVersion != IntegratedModelVersionOriented {
		t.Fatalf("replacement downgraded model to %d", st.ModelVersion)
	}
	if _, err := Resume(sim.Snapshot()); err != nil {
		t.Fatalf("post-replacement model2 should resume: %v", err)
	}
	t.Logf("T-056 invalid imports: model1/unknown/kinematic/non-unit/NaN/bad-limit/zero-id/orientation-without-integrated rejected; model2-without-orientation accepted and not downgraded; clones isolated")
}

// TestT056CallerIsolationFromSetRunners: a caller mutating its own runner AFTER
// SetRunners cannot change the running physics.
func TestT056CallerIsolationFromSetRunners(t *testing.T) {
	caller := t056Oriented(t, 0.5, 0.5, 0.3, 0.5, 1, 0, 1)
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners([]Runner{caller})
	caller.Orientation.FX = -1
	caller.Orientation.FY = 0
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	f := t056Face(t, sim, 0)
	if f.FX != 1 || f.FY != 0 {
		t.Fatalf("SetRunners aliased the caller orientation: %+v", f)
	}
	t.Logf("T-056 caller isolation: post-SetRunners caller mutation did not reach the sim (facing still (%.3f,%.3f))", f.FX, f.FY)
}

// TestT056FacingRetainedThroughLocks: facing is retained under reaction/cooldown
// locks with no intent change, on a genuinely dead ball and on a REAL clean
// control (valid owner/phase/last-touch/journal, Resume-validated before/after).
func TestT056FacingRetainedThroughLocks(t *testing.T) {
	// Dead branch: genuinely dead.
	r := t056Oriented(t, 0.5, 0.5, 0.5, 0.5, 1, 0, 1)
	r.ReactAtMS = 1000
	r.CooldownUntilMS = 1000
	sim := t056MotionSim(t, []Runner{r})
	if sim.Ball().Mode != BallDead {
		t.Fatalf("dead scene ball mode %s", sim.Ball().Mode)
	}
	before := t056Face(t, sim, 0)
	for i := 0; i < 30; i++ {
		sim.Step()
	}
	if after := t056Face(t, sim, 0); after != before {
		t.Fatalf("dead-lock facing changed: %+v -> %+v", before, after)
	}
	if _, err := Resume(sim.Snapshot()); err != nil {
		t.Fatalf("dead-lock Resume: %v", err)
	}

	// Held branch: a real clean control first.
	o, err := NewOrientation(1, 0, 1)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	hr := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
	hr.PlayerID = 1
	hr.Orientation = &o
	hsim := mustSim(t, LivePassConfig(), 1)
	hsim.SetRunners([]Runner{hr})
	if err := hsim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	hsim.ReleasePass(t052InteriorPass(2.0, 0.0))
	for hsim.ClockMS() < 2000 && hsim.Ball().Mode != BallHeld {
		hsim.Step()
	}
	if hsim.Ball().Mode != BallHeld {
		t.Fatal("held branch: no clean control was produced")
	}
	if _, err := Resume(hsim.Snapshot()); err != nil {
		t.Fatalf("held Resume before: %v", err)
	}
	hsim.runners[0].ReactAtMS = hsim.ClockMS() + 1000
	hsim.runners[0].CooldownUntilMS = hsim.ClockMS() + 1000
	hb := t056Face(t, hsim, 0)
	for i := 0; i < 30; i++ {
		hsim.Step()
	}
	if after := t056Face(t, hsim, 0); after != hb {
		t.Fatalf("held-lock facing changed: %+v -> %+v", hb, after)
	}
	if _, err := Resume(hsim.Snapshot()); err != nil {
		t.Fatalf("held Resume after: %v", err)
	}
	t.Logf("T-056 facing retained under locks with no intent: dead ball and a real clean control (Resume-validated before/after)")
}

// TestT056LockWithIntentMovesAndTurns: a NONZERO intent still moves and turns the
// body under a reaction/cooldown lock, within the per-step physical envelopes.
func TestT056LockWithIntentMovesAndTurns(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	r := t056Oriented(t, 0.5, 0.5, 0.5, 0.9, 1, 0, 1)
	r.ReactAtMS = 1000
	r.CooldownUntilMS = 1000
	r.Pursue = true
	sim := t056MotionSim(t, []Runner{r})
	prev := sim.Runners()[0]
	initialFace := t056Face(t, sim, 0)
	prevFace := initialFace
	maxDV := 0.0
	for i := 0; i < 30; i++ {
		sim.Step()
		cur := sim.Runners()[0]
		cf := t056Face(t, sim, 0)
		if dvx := math.Abs(cur.VXM - prev.VXM); dvx > p.AccelMPS2*dt+1e-9 {
			t.Fatalf("step %d |dVx|=%v exceeds accel envelope", i, dvx)
		}
		if dvy := math.Abs(cur.VYM - prev.VYM); dvy > p.AccelMPS2*dt+1e-9 {
			t.Fatalf("step %d |dVy|=%v exceeds accel envelope", i, dvy)
		}
		if dv := math.Abs(cur.VYM - prev.VYM); dv > maxDV {
			maxDV = dv
		}
		if sp := cur.SpeedMPS(); sp > p.TopSpeedMPS+1e-9 {
			t.Fatalf("step %d speed %v > top", i, sp)
		}
		if d := DistM(prev.X, prev.Y, cur.X, cur.Y); d > p.TopSpeedMPS*dt+1e-9 {
			t.Fatalf("step %d displacement %v > envelope", i, d)
		} else if d <= 1e-9 {
			t.Fatalf("step %d no motion under a lock with nonzero intent", i)
		}
		if dot := prevFace.FX*cf.FX + prevFace.FY*cf.FY; dot < t056Threshold(prevFace, p, prev.SpeedMPS(), dt)-1e-12 {
			t.Fatalf("step %d facing turn exceeds the rational bound", i)
		}
		prev, prevFace = cur, cf
	}
	if after := t056Face(t, sim, 0); after == initialFace {
		t.Fatal("no turn under a lock with a nonzero intent")
	}
	t.Logf("T-056 lock+intent: bounded motion/turn every step under a reaction/cooldown lock (max |dVy|=%.6f, <= accel*dt=%.6f)", maxDV, p.AccelMPS2*dt)
}
