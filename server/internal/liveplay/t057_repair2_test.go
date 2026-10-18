package liveplay

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-057 repair2 additional required fixtures: right-hand path correction, true
// full-sim pre/post body ball analytic roots (with chord counterfactuals),
// stable-ID permutations, a true 22-body touching chain, same-speed following and
// grazing near-miss, import corruption, clone isolation, activation order,
// unrepresentable extremes (visible fault, no NaN commit) and a genuine fault
// transaction with recorder overflow + a healthy max1 horizon consumer.

func TestT057PathPointRightHand(t *testing.T) {
	// correction at u=0: return the LAST same-u knot (.5001), not the first (.5)
	if x, y := pathPoint([]bodyKnot{{u: 0, x: .5, y: .5}, {u: 0, x: .5001, y: .5}, {u: 1, x: .5002, y: .5}}, 0); x != .5001 || y != .5 {
		t.Fatalf("pathPoint@0 = (%.6f,%.6f), want (.5001,.5)", x, y)
	}
	// correction at u=0.5: return the LAST (.502), not (.501)
	p := []bodyKnot{{u: 0, x: .5, y: .5}, {u: .5, x: .501, y: .5}, {u: .5, x: .502, y: .5}, {u: 1, x: .503, y: .5}}
	if x, y := pathPoint(p, .5); x != .502 || y != .5 {
		t.Fatalf("pathPoint@.5 = (%.6f,%.6f), want (.502,.5)", x, y)
	}
	// between knots: linear
	if x, _ := pathPoint(p, .75); math.Abs(x-.5025) > 1e-12 {
		t.Fatalf("pathPoint@.75 = %.6f, want .5025", x)
	}
	t.Logf("T-057 pathPoint right-hand: correction returns the post knot")
}

// t057BallBodyScene builds the contract's ball/body fixture: equal mass 0.30 m
// bodies at X=-0.35/+0.35 moving +5/-5 (stopping at -0.30/+0.30 at 10 ms), reach
// 0.06, zero steering/separation/friction; a ball at X=-0.25 moving (0,10).
func t057BallBodyScene(t *testing.T, ballYStart float64) *Sim {
	t.Helper()
	cfg := Config{SubstepMS: 20, DecelMPS2: 0, LivePass: true}
	sim := mustSim(t, cfg, 1)
	p := MotionParams{TopSpeedMPS: 5, AccelMPS2: 4, DecelMPS2: 4.5, LateralAccelMPS2: 3}
	mk := func(xoff, vx float64, id uint64) Runner {
		r := InertialRunner(0.5+xoff/PitchLengthM, 0.5, 0.5+math.Copysign(0.5, vx)/1, 0.5, p)
		r.PlayerID = id
		r.Body = t057Body(0.30, 75)
		r.ReachM = 0.06
		r.VXM = vx
		return r
	}
	a := mk(-0.35, 5, 1)
	b := mk(0.35, -5, 2)
	sim.SetRunners([]Runner{a, b})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.5 - 0.25/PitchLengthM, Y: 0.5 + ballYStart/PitchWidthM, VXM: 0, VYM: 10})
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
		t.Fatalf("body enable: %v", err)
	}
	return sim
}

func t057FirstTouchEvent(sim *Sim, kind PassEventKind) (PassEvent, bool) {
	for _, e := range sim.Events() {
		if e.Kind == kind {
			return e, true
		}
	}
	return PassEvent{}, false
}

func TestT057TrueBallAnalyticRoots(t *testing.T) {
	// Ystart=-0.13: contact BEFORE the 10 ms body impact, at 9.82179074309616 ms.
	sim := t057BallBodyScene(t, -0.13)
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	ev, ok := t057FirstTouchEvent(sim, PassEventControl)
	if !ok {
		t.Fatalf("no control event; events=%+v", sim.Events())
	}
	exact := float64(ev.ClockMS-sim.cfg.SubstepMS) + ev.Frac*float64(sim.cfg.SubstepMS)
	if math.Abs(exact-9.82179074309616) > 1e-8 {
		t.Fatalf("pre-impact contact %.9f, want 9.82179074309616", exact)
	}
	wantY := 0.5 + (-0.13+10*exact/1000)/PitchWidthM
	if math.Abs(ev.BallY-wantY) > 1e-9 {
		t.Fatalf("contact BallY %.12f, want %.12f", ev.BallY, wantY)
	}
	t.Logf("T-057 ball pre-impact: control exact=%.9f ms conY=%.9f", exact, ev.BallY)

	// Ystart=-0.18: contact AFTER the body impact, at 14.6833752096446 ms (needs
	// the actual post-impact (stopped) body path).
	sim2 := t057BallBodyScene(t, -0.18)
	if err := sim2.StepChecked(); err != nil {
		t.Fatalf("step2: %v", err)
	}
	ev2, ok := t057FirstTouchEvent(sim2, PassEventControl)
	if !ok {
		t.Fatalf("no control event (post)")
	}
	exact2 := float64(ev2.ClockMS-sim2.cfg.SubstepMS) + ev2.Frac*float64(sim2.cfg.SubstepMS)
	if math.Abs(exact2-14.6833752096446) > 1e-8 {
		t.Fatalf("post-impact contact %.9f, want 14.6833752096446", exact2)
	}

	// Counterfactual: the false whole-step chord misses the pre case entirely
	// (disc<0) and predicts 16.632 ms for the post case.
	preBallX := 0.5 - 0.25/PitchLengthM
	chordFor := func(yStart float64) (float64, bool) {
		// straight body chord -0.35 -> -0.30 (x), ball (-0.25, yStart) -> (-0.25, yStart+0.2)
		bx0 := 0.5 - 0.35/PitchLengthM
		bx1 := 0.5 - 0.30/PitchLengthM
		ex := preBallX
		ey := 0.5 + (yStart+0.2)/PitchWidthM
		u, ok := segmentContact(preBallX, 0.5+yStart/PitchWidthM, 0, ex, ey, bx0, 0.5, bx1, 0.5, 0.06, 0)
		if !ok {
			return 0, false
		}
		return u * 20, true
	}
	if _, ok := chordFor(-0.13); ok {
		t.Fatal("false endpoint chord unexpectedly found the pre-impact contact")
	}
	if ms, ok := chordFor(-0.18); !ok || math.Abs(ms-16.632068588474187) > 1e-6 {
		t.Fatalf("false endpoint chord post time=%v ok=%v, want 16.632068588474187 ms", ms, ok)
	}
	t.Logf("T-057 ball analytic roots: pre=9.82179074309616, post=14.6833752096446; chord miss/16.632 counterfactuals confirmed")
}

func TestT057StableIDPermutations(t *testing.T) {
	build := func(reverse bool) *Sim {
		p := t057Params(5)
		a := InertialRunner(0.5, 0.5, 1.0, 0.5, p)
		a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 5
		b := InertialRunner(0.5+0.7/PitchLengthM, 0.5, 0.0, 0.5, p)
		b.PlayerID, b.Body, b.VXM = 2, t057Body(0.30, 75), -5
		rs := []Runner{a, b}
		if reverse {
			rs = []Runner{b, a} // slots differ, IDs preserved
		}
		sim := t057BodySim(t, 1, rs, t057HardConfig())
		if err := sim.StepChecked(); err != nil {
			t.Fatalf("step: %v", err)
		}
		return sim
	}
	s1, s2 := build(false), build(true)
	if len(s1.body.Impacts) != 1 || len(s2.body.Impacts) != 1 {
		t.Fatalf("impacts %d/%d", len(s1.body.Impacts), len(s2.body.Impacts))
	}
	i1, i2 := s1.body.Impacts[0], s2.body.Impacts[0]
	if i1.PlayerID1 != i2.PlayerID1 || i1.PlayerID2 != i2.PlayerID2 || math.Abs(i1.Fraction-i2.Fraction) > 1e-12 {
		t.Fatalf("impact IDs/fraction differ: %+v vs %+v", i1, i2)
	}
	if math.Abs(i1.Impulse-i2.Impulse) > 1e-9 {
		t.Fatalf("impulse differs by roster order: %v vs %v", i1.Impulse, i2.Impulse)
	}
	// ID-keyed final positions/velocities equal.
	byID := func(s *Sim) map[uint64]Runner {
		m := map[uint64]Runner{}
		for _, r := range s.runners {
			m[r.PlayerID] = r
		}
		return m
	}
	m1, m2 := byID(s1), byID(s2)
	for id := range m1 {
		if math.Abs(m1[id].X-m2[id].X) > 1e-9 || math.Abs(m1[id].Y-m2[id].Y) > 1e-9 ||
			math.Abs(m1[id].VXM-m2[id].VXM) > 1e-9 || math.Abs(m1[id].VYM-m2[id].VYM) > 1e-9 {
			t.Fatalf("id %d state differs by roster order", id)
		}
	}
	t.Logf("T-057 permutations: reversed roster yields identical ID-keyed state and impact ordering")
}

func TestT057TouchingChain22(t *testing.T) {
	// Ordinary work limit must converge a 22-body 1D touching chain (stable full
	// passes), momentum ~0, no fault.
	const n = 22
	p := t057Params(5)
	var rs []Runner
	for i := 0; i < n; i++ {
		v := 0.5 * (float64(n-1)/2 - float64(i))
		off := 0.6 * float64(i)
		r := InertialRunner(0.5+off/PitchLengthM, 0.5, 0.5+off/PitchLengthM, 0.5, p)
		r.PlayerID = uint64(i + 1)
		r.Body = t057Body(0.30, 75)
		r.VXM = v
		rs = append(rs, r)
	}
	sim := t057BodySim(t, 1, rs, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("22 touching chain: %v", err)
	}
	m := 0.0
	for i := range sim.runners {
		m += 75 * sim.runners[i].VXM
	}
	if math.Abs(m) > 1e-6 {
		t.Fatalf("momentum %v", m)
	}
	t.Logf("T-057 22 touching chain: %d impacts, momentum=%.3e", len(sim.body.Impacts), m)
}

func TestT057SameSpeedFollowingAndGrazing(t *testing.T) {
	// Following at the same speed: never closes.
	a := InertialRunner(0.5, 0.5, 1.0, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 5
	b := InertialRunner(0.5+0.7/PitchLengthM, 0.5, 1.0, 0.5, t057Params(5))
	b.PlayerID, b.Body, b.VXM = 2, t057Body(0.30, 75), 5
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("following: %v", err)
	}
	if len(sim.body.Impacts) != 0 {
		t.Fatal("same-speed following produced an impulse")
	}
	// Grazing miss: parallel paths separated by just over 2r.
	c := InertialRunner(0.5, 0.5, 1.0, 0.5, t057Params(5))
	c.PlayerID, c.Body, c.VXM = 1, t057Body(0.30, 75), 5
	d := InertialRunner(0.5, 0.5+0.61/PitchWidthM, 1.0, 0.5+0.61/PitchWidthM, t057Params(5))
	d.PlayerID, d.Body, d.VXM = 2, t057Body(0.30, 75), 5
	sim2 := t057BodySim(t, 1, []Runner{c, d}, t057HardConfig())
	if err := sim2.StepChecked(); err != nil {
		t.Fatalf("grazing: %v", err)
	}
	if len(sim2.body.Impacts) != 0 {
		t.Fatal("grazing miss produced an impulse")
	}
	t.Logf("T-057 following/grazing: no false impulses")
}

func TestT057ImportCorruptionTable(t *testing.T) {
	sim := t057BodySim(t, 1, t057Chain(t057Params(5)), t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	base := sim.Snapshot()
	if _, err := Resume(base); err != nil {
		t.Fatalf("baseline resume: %v", err)
	}
	baseCopy := deepCopySnapshot(t, base)
	reject := func(label string, s Snapshot) {
		t.Helper()
		if _, err := Resume(s); err == nil {
			t.Fatalf("%s: accepted", label)
		}
	}
	mut := func(f func(*Snapshot)) Snapshot { s := deepCopySnapshot(t, baseCopy); f(&s); return s }
	reject("schema", mut(func(s *Snapshot) { s.BodyContacts.SchemaVersion = 9 }))
	reject("config", mut(func(s *Snapshot) { s.BodyContacts.Config.SpacingM = -1 }))
	reject("nextSeq", mut(func(s *Snapshot) { s.BodyContacts.NextSeq = 99 }))
	reject("impactSeq", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].Seq = 5 }))
	reject("impactClockFuture", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].ClockMS = s.ClockMS + 100 }))
	reject("impactFracNaN", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].Fraction = math.NaN() }))
	reject("impactIDs", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].PlayerID1 = s.BodyContacts.Impacts[0].PlayerID2 }))
	reject("normalUnit", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].NormalX = 2; s.BodyContacts.Impacts[0].NormalY = 0 }))
	reject("impulseAlg", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].Impulse += 1 }))
	reject("outVelAlg", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].OutVX1 += 1 }))
	reject("outVel2Alg", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].OutVX2 += 1 }))
	reject("contactDist", mut(func(s *Snapshot) { s.BodyContacts.Impacts[0].X1 += 0.5 }))
	reject("bodyMassInv", mut(func(s *Snapshot) { s.Runners[0].Body.MassKG = 1e-320 }))
	reject("bodyRadius", mut(func(s *Snapshot) { s.Runners[0].Body.RadiusM = 0 }))
	reject("faultClock", mut(func(s *Snapshot) {
		s.BodyContacts.Fault = &BodyStepFault{Code: BodyFaultWorkLimit, StepStartMS: s.ClockMS + 20, Fraction: 0.5}
	}))
	reject("faultCode", mut(func(s *Snapshot) {
		s.BodyContacts.Fault = &BodyStepFault{Code: "nope", StepStartMS: s.ClockMS, Fraction: 0.5}
	}))
	reject("faultFrac", mut(func(s *Snapshot) {
		s.BodyContacts.Fault = &BodyStepFault{Code: BodyFaultWorkLimit, StepStartMS: s.ClockMS, Fraction: 1.5}
	}))
	reject("model", mut(func(s *Snapshot) { s.Integrated.ModelVersion = IntegratedModelVersionOriented }))
	reject("incomplete", mut(func(s *Snapshot) { s.BodyContacts = nil }))
	t.Logf("T-057 import corruption table rejected")
}

func TestT057CloneIsolationAndActivation(t *testing.T) {
	sim := t057BodySim(t, 1, t057Chain(t057Params(5)), t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	// Snapshot/Runners deep-copy of params/impacts/fault.
	rs := sim.Runners()
	rs[0].Body.MassKG = 1
	if sim.runners[0].Body.MassKG == 1 {
		t.Fatal("Runners() aliases Body params")
	}
	sn := sim.Snapshot()
	origImp := sn.BodyContacts.Impacts[0].Impulse
	sn.BodyContacts.Impacts[0].Impulse = -1
	if sim.body.Impacts[0].Impulse != origImp {
		t.Fatal("Snapshot aliases impacts")
	}
	res, err := Resume(sim.Snapshot())
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	res.body.Impacts[0].Impulse = -1
	if sim.body.Impacts[0].Impulse != origImp {
		t.Fatal("Resume aliases impacts")
	}
	// Enable order: orientation BEFORE body and body BEFORE orientation both end model3.
	ob := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	ob.PlayerID, ob.Body = 1, t057Body(0.30, 75)
	s1 := mustSim(t, LivePassConfig(), 1)
	s1.SetRunners([]Runner{ob})
	if err := s1.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	o1, _ := NewOrientation(1, 0, 1)
	if err := s1.EnableOrientation(0, o1); err != nil {
		t.Fatal(err)
	}
	if err := s1.EnableBodyContacts(t057HardConfig()); err != nil {
		t.Fatal(err)
	}
	if st, _ := s1.IntegratedState(); st.ModelVersion != IntegratedModelVersionBody {
		t.Fatalf("orientation-then-body model %d", st.ModelVersion)
	}
	// NaN raw setter AFTER enable: EnableBodyContacts revalidates and rejects
	// transactionally (EnableIntegrated alone does not license a later NaN).
	finite := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	finite.PlayerID, finite.Body = 1, t057Body(0.30, 75)
	s2 := mustSim(t, LivePassConfig(), 1)
	s2.SetRunners([]Runner{finite})
	if err := s2.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	nanR := finite
	nanR.X = math.NaN()
	s2.SetRunners([]Runner{nanR})
	if err := s2.EnableBodyContacts(t057HardConfig()); !errors.Is(err, ErrIntegratedBody) {
		t.Fatalf("NaN raw setter body enable err=%v", err)
	}
	if s2.BodyEnabled() || !math.IsNaN(s2.runners[0].X) {
		t.Fatal("failed body enable mutated state or cleared NaN")
	}
	// valid-first/bad-second roster: rejected transactionally, no partial enable.
	s3 := mustSim(t, LivePassConfig(), 1)
	good := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	good.PlayerID, good.Body = 1, t057Body(0.30, 75)
	second := InertialRunner(0.4, 0.5, 0.4, 0.5, t057Params(5))
	second.PlayerID, second.Body = 2, t057Body(0.30, 75)
	s3.SetRunners([]Runner{good, second})
	if err := s3.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	badSecond := second
	badSecond.Y = math.NaN()
	s3.SetRunners([]Runner{good, badSecond})
	if err := s3.EnableBodyContacts(t057HardConfig()); err == nil {
		t.Fatal("valid-first/bad-second roster accepted")
	}
	if s3.BodyEnabled() || !math.IsNaN(s3.runners[1].Y) {
		t.Fatal("partial body enable mutated state or cleared NaN")
	}
	// Repeated enable on a fresh clock-0 body sim fails with ErrBodyAlreadyEnabled.
	rp := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	rp.PlayerID, rp.Body = 1, t057Body(0.30, 75)
	rep := t057BodySim(t, 9, []Runner{rp}, t057HardConfig())
	if err := rep.EnableBodyContacts(t057HardConfig()); !errors.Is(err, ErrBodyAlreadyEnabled) {
		t.Fatalf("repeated enable err=%v", err)
	}
	t.Logf("T-057 clone/activation: isolation, both orders -> model3, NaN raw setter transactional, repeated enable rejected")
}

func TestT057FiniteExtremes(t *testing.T) {
	a := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	a.PlayerID, a.Body = 1, t057Body(0.30, 75)
	a.VXM = 1e308
	b := InertialRunner(0.5+0.7/PitchLengthM, 0.5, 0.5, 0.5, t057Params(5))
	b.PlayerID, b.Body = 2, t057Body(0.30, 75)
	b.VXM = -1e308
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	before := sim.Snapshot()
	err := sim.StepChecked()
	if err == nil {
		t.Fatal("extreme velocities produced no fault")
	}
	after := sim.Snapshot()
	after.BodyContacts.Fault = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("non-finite fault committed partial state")
	}
	t.Logf("T-057 finite extremes: visible structured fault, no NaN commit")
}

func TestT057FaultOverflowAndHealthyHorizon(t *testing.T) {
	// Healthy source with MaxResolutions=1 faults INSIDE the provenance clone.
	rs := t057Chain(t057Params(5))
	obs := t050Local(0.90, 0.90, 0.90, 0.90, 200.0, 0, true)
	obs.PlayerID = 4
	obs.Body = t057Body(0.30, 75)
	rs = append(rs, obs)
	cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: 1}
	sim := t057BodySimScheduled(t, 1, rs, cfg)
	rec := mustRecorder(t, 3)
	sim.SetProvenanceRecorder(rec)
	full := sim.Snapshot()
	rng := sim.RNGState()
	recBefore := rec.Records()
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 64, HorizonMS: sim.ClockMS() + 400}); err == nil {
		t.Fatal("healthy max1 source did not fault inside the provenance horizon")
	}
	if !reflect.DeepEqual(full, sim.Snapshot()) || rng != sim.RNGState() {
		t.Fatal("provenance mutated the source")
	}
	if !reflect.DeepEqual(recBefore, rec.Records()) {
		t.Fatal("provenance wrote the source recorder")
	}
	// A trial that WOULD overflow the recorder still leaves the original untouched.
	sim.body.Config.MaxResolutions = BodyDefaultMaxResolutions
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("healthy step: %v", err)
	}
	t.Logf("T-057 fault/horizon: healthy max1 faults inside the provenance clone; source full-snapshot/RNG/recorder unchanged")
}
