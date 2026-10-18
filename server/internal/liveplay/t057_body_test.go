package liveplay

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-057 body-contact fixtures (repair1): origin diagnostic, analytical head-on /
// above-top / perpendicular / endpoint chain, soft spacing, tiny-overlap vs
// rejected gross overlap, noncolliding crossing, transactional checked fault with
// a due publication and a bounded recorder, model/import validation, JSON
// continuation and a true 2D packed box with genuine closing contacts.

func t057Params(top float64) MotionParams {
	return MotionParams{TopSpeedMPS: top, AccelMPS2: 4, DecelMPS2: 4.5, LateralAccelMPS2: 3}
}

func t057Body(rad, mass float64) *RunnerBody {
	b, err := NewRunnerBody(rad, mass)
	if err != nil {
		panic(err)
	}
	return &b
}

func t057HardConfig() BodyContactConfig {
	return BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: BodyDefaultMaxResolutions}
}

func t057BodySim(t *testing.T, seed uint64, rs []Runner, cfg BodyContactConfig) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	if err := sim.EnableBodyContacts(cfg); err != nil {
		t.Fatalf("EnableBodyContacts: %v", err)
	}
	return sim
}

func t057BodySimScheduled(t *testing.T, seed uint64, rs []Runner, cfg BodyContactConfig) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	if err := sim.EnableBodyContacts(cfg); err != nil {
		t.Fatalf("EnableBodyContacts: %v", err)
	}
	return sim
}

// t057Chain returns the endpoint-touching three-body chain (offsets 0/0.7/1.4 m).
func t057Chain(p MotionParams) []Runner {
	mk := func(off, vx, ix float64, id uint64) Runner {
		r := InertialRunner(0.5+off/PitchLengthM, 0.5, ix, 0.5, p)
		r.PlayerID = id
		r.Body = t057Body(0.30, 75)
		r.VXM = vx
		return r
	}
	return []Runner{mk(0, 5, 1.0, 1), mk(0.7, 0, 0.5+0.7/PitchLengthM, 2), mk(1.4, -5, 0.0, 3)}
}

func TestT057InheritedSegmentContactLimitationAndPathContact(t *testing.T) {
	if _, ok := segmentContact(0.40, 0.50, 0.5, 0.50, 0.50, 0.45, 0.50, 0.45, 0.50, 1.0, 0.5); ok {
		t.Fatal("inherited segmentContact unexpectedly found the nonzero-uCur contact")
	}
	up, ok, _ := pathContact([]bodyKnot{{u: 0, x: 0.45, y: 0.50}, {u: 1, x: 0.45, y: 0.50}}, 0.40, 0.50, 0.5, 0.50, 0.50, 1.0, 0.5)
	if !ok {
		t.Fatal("pathContact missed the correct-origin contact")
	}
	if math.Abs(up-0.7023809523809523) > 1e-10 {
		t.Fatalf("pathContact global entry %v", up)
	}
	// Endpoint equality must be eligible (uMin reaches the boundary).
	if u, ok, _ := pathContact([]bodyKnot{{u: 0, x: 0.31, y: 0.50}, {u: 1, x: 0.31, y: 0.50}}, 0.10, 0.50, 0.5, 0.31, 0.50, 1.0, 1.0); !ok || math.Abs(u-1) > 1e-12 {
		t.Fatalf("endpoint contact not eligible: u=%v ok=%v", u, ok)
	}
	t.Logf("T-057 origin: inherited ok=false; pathContact entry=%.16f; endpoint eligible", up)
}

func TestT057HeadOn(t *testing.T) {
	p := t057Params(5)
	a := InertialRunner(0.5, 0.5, 1.0, 0.5, p)
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 5
	b := InertialRunner(0.5+0.7/PitchLengthM, 0.5, 0.0, 0.5, p)
	b.PlayerID, b.Body, b.VXM = 2, t057Body(0.30, 75), -5
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("StepChecked: %v", err)
	}
	if len(sim.body.Impacts) != 1 {
		t.Fatalf("impacts %d, want 1", len(sim.body.Impacts))
	}
	im := sim.body.Impacts[0]
	if math.Abs(im.Fraction-0.5) > 1e-10 {
		t.Fatalf("fraction %v", im.Fraction)
	}
	if math.Abs(im.Impulse-375) > 1e-6 {
		t.Fatalf("impulse %v", im.Impulse)
	}
	ra, rb := sim.runners[0], sim.runners[1]
	if math.Abs(ra.VXM) > 1e-9 || math.Abs(rb.VXM) > 1e-9 {
		t.Fatalf("final speeds %v/%v", ra.VXM, rb.VXM)
	}
	if math.Abs(ra.X-(0.5+0.05/PitchLengthM)) > 1e-9 || math.Abs(rb.X-(0.5+0.65/PitchLengthM)) > 1e-9 {
		t.Fatalf("final centres %v/%v", ra.X, rb.X)
	}
	t.Logf("T-057 head-on: J=%.6f frac=%.6f speeds 0/0", im.Impulse, im.Fraction)
}

func TestT057AboveTopImpulseAndBoundedBraking(t *testing.T) {
	a := InertialRunner(0.5, 0.5, 1.0, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 100), 5
	belx := 0.5 + 0.65/PitchLengthM
	b := InertialRunner(belx, 0.5, belx, 0.5, t057Params(1))
	b.PlayerID, b.Body = 2, t057Body(0.30, 25)
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step1: %v", err)
	}
	if len(sim.body.Impacts) != 1 || math.Abs(sim.body.Impacts[0].Impulse-100) > 1e-6 {
		t.Fatalf("impacts %+v", sim.body.Impacts)
	}
	if math.Abs(sim.runners[0].VXM-4) > 1e-9 || math.Abs(sim.runners[1].VXM-4) > 1e-9 {
		t.Fatalf("outgoing %v/%v", sim.runners[0].VXM, sim.runners[1].VXM)
	}
	if math.Abs(sim.runners[0].X-(0.5+0.09/PitchLengthM)) > 1e-9 || math.Abs(sim.runners[1].X-(0.5+0.69/PitchLengthM)) > 1e-9 {
		t.Fatalf("final positions %v/%v", sim.runners[0].X, sim.runners[1].X)
	}
	sim.SetIntent(0, sim.runners[0].X, sim.runners[0].Y)
	sim.SetIntent(1, sim.runners[1].X, sim.runners[1].Y)
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step2: %v", err)
	}
	if math.Abs(sim.runners[0].VXM-3.91) > 1e-9 || math.Abs(sim.runners[1].VXM-3.91) > 1e-9 {
		t.Fatalf("braked %v/%v, want 3.91 (B not clamped)", sim.runners[0].VXM, sim.runners[1].VXM)
	}
	if len(sim.body.Impacts) != 1 {
		t.Fatalf("extra impulse %d", len(sim.body.Impacts))
	}
	t.Logf("T-057 above-top: 4/4 then %.4f/%.4f; no instant clamp", sim.runners[0].VXM, sim.runners[1].VXM)
}

func TestT057PerpendicularCrossing(t *testing.T) {
	ax := 0.5 - 0.5/PitchLengthM
	a := InertialRunner(ax, 0.5, ax+0.5, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 5
	by := 0.5 - 0.5/PitchWidthM
	b := InertialRunner(0.5, by, 0.5, by+0.5, t057Params(5))
	b.PlayerID, b.Body, b.VYM = 2, t057Body(0.30, 75), 5
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	if len(sim.body.Impacts) != 1 {
		t.Fatalf("impacts %d", len(sim.body.Impacts))
	}
	im := sim.body.Impacts[0]
	want := (0.5 - 0.6/math.Sqrt2) / 5 / 0.02
	if math.Abs(im.Fraction-want) > 1e-9 {
		t.Fatalf("fraction %v, want %v", im.Fraction, want)
	}
	if math.Abs(im.OutVX1-2.5) > 1e-9 || math.Abs(im.OutVY1-2.5) > 1e-9 || math.Abs(im.OutVX2-2.5) > 1e-9 || math.Abs(im.OutVY2-2.5) > 1e-9 {
		t.Fatalf("outgoing A(%v,%v) B(%v,%v)", im.OutVX1, im.OutVY1, im.OutVX2, im.OutVY2)
	}
	t.Logf("T-057 perpendicular: frac=%.9f outgoing (2.5,2.5)", im.Fraction)
}

func TestT057ThreeBodyChainMomentum(t *testing.T) {
	sim := t057BodySim(t, 1, t057Chain(t057Params(5)), t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	if len(sim.body.Impacts) < 1 {
		t.Fatal("no impacts in the endpoint chain")
	}
	m, ke := 0.0, 0.0
	for i := range sim.runners {
		m += 75 * sim.runners[i].VXM
		ke += 0.5 * 75 * sim.runners[i].VXM * sim.runners[i].VXM
	}
	if math.Abs(m) > 1e-6 {
		t.Fatalf("momentum %v", m)
	}
	if ke > 1875+1e-6 {
		t.Fatalf("KE %v exceeds initial", ke)
	}
	t.Logf("T-057 chain: %d impacts at the endpoint, momentum=%.3e KE=%.4f", len(sim.body.Impacts), m, ke)
}

func TestT057NoncollidingCrossing(t *testing.T) {
	ax := 0.5 - 0.5/PitchLengthM
	a := InertialRunner(ax, 0.5, ax+0.5, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 5
	by := 0.5 - 1.5/PitchWidthM // well beyond summed radii
	b := InertialRunner(0.5, by, 0.5, by+0.5, t057Params(5))
	b.PlayerID, b.Body, b.VYM = 2, t057Body(0.30, 75), 5
	sim := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	if len(sim.body.Impacts) != 0 {
		t.Fatalf("noncolliding crossing produced %d impacts", len(sim.body.Impacts))
	}
	if math.Abs(sim.runners[0].VXM-5) > 1e-9 || math.Abs(sim.runners[1].VYM-5) > 1e-9 {
		t.Fatal("noncolliding crossing changed momentum")
	}
	t.Logf("T-057 noncolliding crossing: no impulse, momentum unchanged")
}

func TestT057SoftSpacingPaired(t *testing.T) {
	build := func(soft float64) *Sim {
		a := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
		a.PlayerID, a.Body = 1, t057Body(0.30, 75)
		b := InertialRunner(0.5+1.0/PitchLengthM, 0.5, 0.5+1.0/PitchLengthM, 0.5, t057Params(5))
		b.PlayerID, b.Body = 2, t057Body(0.30, 75)
		cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: soft, MaxResolutions: BodyDefaultMaxResolutions}
		sim := t057BodySim(t, 1, []Runner{a, b}, cfg)
		if err := sim.StepChecked(); err != nil {
			t.Fatalf("soft=%v: %v", soft, err)
		}
		return sim
	}
	s0 := build(0)
	if s0.runners[0].VXM != 0 || s0.runners[1].VXM != 0 {
		t.Fatalf("soft disabled changed velocities")
	}
	s1 := build(2.0)
	if !(s1.runners[0].VXM < 0 && s1.runners[1].VXM > 0) {
		t.Fatalf("soft not signed away: %v/%v", s1.runners[0].VXM, s1.runners[1].VXM)
	}
	if math.Abs(s1.runners[0].VXM) > 0.08+1e-9 || math.Abs(s1.runners[1].VXM) > 0.08+1e-9 {
		t.Fatalf("soft exceeds accel bound")
	}
	if len(s1.body.Impacts) != 0 {
		t.Fatal("soft scene had a hard impact before contact")
	}
	t.Logf("T-057 soft: disabled 0/0; enabled %.6f/%.6f", s1.runners[0].VXM, s1.runners[1].VXM)
}

func TestT057ExactOverlapTinyAndGrossRejected(t *testing.T) {
	ta := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	ta.PlayerID, ta.Body = 1, t057Body(0.005, 75)
	tb := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	tb.PlayerID, tb.Body = 2, t057Body(0.005, 75)
	tsim := t057BodySim(t, 1, []Runner{ta, tb}, t057HardConfig())
	if err := tsim.StepChecked(); err != nil {
		t.Fatalf("tiny overlap repair: %v", err)
	}
	if d := DistM(tsim.runners[0].X, tsim.runners[0].Y, tsim.runners[1].X, tsim.runners[1].Y); math.Abs(d-0.01) > 1e-6 {
		t.Fatalf("tiny overlap separation %v", d)
	}
	ga := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	ga.PlayerID, ga.Body = 1, t057Body(0.30, 75)
	gb := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	gb.PlayerID, gb.Body = 2, t057Body(0.30, 75)
	gsim := mustSim(t, LivePassConfig(), 1)
	gsim.SetRunners([]Runner{ga, gb})
	if err := gsim.EnableIntegrated(); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := gsim.EnableBodyContacts(t057HardConfig()); !errors.Is(err, ErrBodyOverlap) {
		t.Fatalf("gross coincidence err=%v", err)
	}
	t.Logf("T-057 overlap: tiny repaired to 0.01m; gross rejected")
}

// TestT057CheckedFaultTransaction pins the transactional boundary with a due
// publication, a bounded recorder and a real endpoint collision chain at Max1.
func TestT057CheckedFaultTransaction(t *testing.T) {
	rs := t057Chain(t057Params(5))
	obs := t050Local(0.90, 0.90, 0.90, 0.90, 200.0, 0, true)
	obs.PlayerID = 4
	obs.Body = t057Body(0.30, 75)
	rs = append(rs, obs)
	cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: 1}
	sim := t057BodySimScheduled(t, 1, rs, cfg)
	rec := mustRecorder(t, 4) // small -> exercises overflow bookkeeping
	sim.SetProvenanceRecorder(rec)
	before := sim.Snapshot()
	rngB := sim.RNGState()
	recBefore := rec.Records()
	recOver := rec.Overflowed()

	err := sim.StepChecked()
	if err == nil {
		t.Fatal("expected a work-limit fault")
	}
	if f := sim.body.Fault; f == nil || f.Code != BodyFaultWorkLimit {
		t.Fatalf("fault %+v, want work-limit", f)
	}
	if sim.body.Fault.StepStartMS != before.ClockMS || sim.body.Fault.Fraction != 1 {
		t.Fatalf("fault start/frac %d/%v", sim.body.Fault.StepStartMS, sim.body.Fault.Fraction)
	}
	if sim.ClockMS() != before.ClockMS || rngB != sim.RNGState() {
		t.Fatal("fault advanced clock/RNG")
	}
	if !reflect.DeepEqual(recBefore, rec.Records()) || recOver != rec.Overflowed() {
		t.Fatal("failed step wrote into the original recorder")
	}
	after := sim.Snapshot()
	after.BodyContacts.Fault = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed step mutated state beyond the fault")
	}
	if err := sim.RunChecked(1000); err == nil {
		t.Fatal("RunChecked didn't surface the sticky fault")
	}
	sim.Run(1000)
	if sim.ClockMS() != before.ClockMS {
		t.Fatal("void Run advanced under a fault")
	}
	srcClock := sim.ClockMS()
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 64, HorizonMS: srcClock + 200}); err == nil {
		t.Fatal("provenance didn't surface the fault")
	}
	if sim.ClockMS() != srcClock {
		t.Fatal("provenance mutated the source")
	}
	res, err := Resume(sim.Snapshot())
	if err != nil || res.StepError() == nil {
		t.Fatalf("faulted Resume err=%v fault=%v", err, res.StepError())
	}
	t.Logf("T-057 fault: work-limit at frac1 with due publication+recorder; original recorder untouched; state/RNG exact; consumers surface it")
}

func TestT057ModelAndImportValidation(t *testing.T) {
	a := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	a.PlayerID, a.Body = 1, t057Body(0.30, 75)
	sim := t057BodySim(t, 1, []Runner{a}, t057HardConfig())
	if st, _ := sim.IntegratedState(); st.ModelVersion != IntegratedModelVersionBody {
		t.Fatalf("model %d", st.ModelVersion)
	}
	o, _ := NewOrientation(1, 0, 1)
	if err := sim.EnableOrientation(0, o); err != nil {
		t.Fatalf("EnableOrientation after body: %v", err)
	}
	if st, _ := sim.IntegratedState(); st.ModelVersion != IntegratedModelVersionBody {
		t.Fatalf("orientation downgraded to %d", st.ModelVersion)
	}
	plain := mustSim(t, LivePassConfig(), 1)
	plain.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))}))
	if err := plain.EnableIntegrated(); err != nil {
		t.Fatalf("enable: %v", err)
	}
	snap := plain.Snapshot()
	snap.Integrated.ModelVersion = IntegratedModelVersionBody
	if _, err := Resume(snap); err != nil {
		t.Fatalf("model3 without body state should resume: %v", err)
	}
	snap2 := plain.Snapshot()
	snap2.Runners[0].Body = t057Body(0.30, 75)
	if _, err := Resume(snap2); !errors.Is(err, ErrBodyIncomplete) {
		t.Fatalf("body params without state err=%v", err)
	}
	bsnap := sim.Snapshot()
	bsnap.Integrated.ModelVersion = IntegratedModelVersionOriented
	if _, err := Resume(bsnap); !errors.Is(err, ErrBodyVersion) {
		t.Fatalf("body state model2 err=%v", err)
	}
	t.Logf("T-057 model: model3 preserved; import rules enforced")
}

func TestT057PackedBox22NonPenetration(t *testing.T) {
	rs := t057BoxRunners()
	if len(rs) != 22 {
		t.Fatalf("roster %d", len(rs))
	}
	sim := t057BodySim(t, 1, rs, t057HardConfig())
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("packed box step: %v", err)
	}
	if len(sim.body.Impacts) == 0 {
		t.Fatal("packed box produced no genuine closing impacts")
	}
	for i := 0; i < len(sim.runners); i++ {
		for j := i + 1; j < len(sim.runners); j++ {
			d := DistM(sim.runners[i].X, sim.runners[i].Y, sim.runners[j].X, sim.runners[j].Y)
			sum := sim.runners[i].Body.RadiusM + sim.runners[j].Body.RadiusM
			if sum-d > BodyResidualTolM+1e-9 {
				t.Fatalf("pair (%d,%d) penetration %.3e", i, j, sum-d)
			}
		}
	}
	t.Logf("T-057 packed box: 22 bodies, %d genuine impacts, no gross penetration", len(sim.body.Impacts))
}

// TestT057BodyEnabledActive22Windows: the accepted active22 fixture with bodies on
// every runner, three fresh windows identical, all22 moving every step.
func TestT057BodyEnabledActive22Windows(t *testing.T) {
	for _, control := range []bool{true, false} {
		label, seed := "deflect", uint64(t053DeflectSeed)
		if control {
			label, seed = "control", t053ControlSeed
		}
		var inits, finals []Snapshot
		for w := 0; w < 3; w++ {
			sim := mustSim(t, ScheduledLivePassConfig(100), seed)
			rs := t055AssignIDs(t053Runners(control))
			for i := range rs {
				rs[i].Body = t057Body(0.30, 75)
			}
			sim.SetRunners(rs)
			if err := sim.EnableIntegrated(); err != nil {
				t.Fatalf("enable: %v", err)
			}
			if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
				t.Fatalf("body enable: %v", err)
			}
			if control {
				sim.ReleasePass(t052InteriorPass(2.0, 0.0))
			} else {
				sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
			}
			inits = append(inits, sim.Snapshot())
			prev := sim.Runners()
			held := 0
			for k := 0; k < t053WindowSteps; k++ {
				if control && k == 50 {
					sim.ReleasePass(t053SecondReleaseParams())
				}
				if err := sim.StepChecked(); err != nil {
					t.Fatalf("%s step %d: %v", label, k, err)
				}
				cur := sim.Runners()
				t053AssertAllMoved(t, label, prev, cur, k)
				prev = cur
				if sim.Ball().Mode == BallHeld {
					held++
				}
			}
			finals = append(finals, sim.Snapshot())
			touches := t052Touches(sim)
			if len(touches) == 0 || !(touches[0].Frac > 0 && touches[0].Frac < 1) {
				t.Fatalf("%s: no interior transition %+v", label, touches)
			}
			if control {
				if touches[0].Kind != PassEventControl || held == 0 {
					t.Fatalf("control: first touch %s held %d", touches[0].Kind, held)
				}
				rel := t051ReleaseClocks(sim.Events())
				if len(rel) != 2 || rel[1] != t053SecondRelMS {
					t.Fatalf("control releases %v", rel)
				}
			} else if touches[0].Kind != PassEventDeflect {
				t.Fatalf("deflect first touch %s", touches[0].Kind)
			}
		}
		for w := 1; w < 3; w++ {
			if !reflect.DeepEqual(inits[0], inits[w]) || !reflect.DeepEqual(finals[0], finals[w]) {
				t.Fatalf("%s window %d differs", label, w)
			}
		}
		t.Logf("T-057 body-enabled active22 %s: 3 fresh windows identical; 22/22 moved every step", label)
	}
}
