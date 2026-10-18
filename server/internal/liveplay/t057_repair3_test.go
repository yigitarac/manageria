package liveplay

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-057 repair3 completion receipts: combined JSON tails, held-carrier bump,
// trace/nil-checked gates, sticky zero-horizon/held faults, touching-separating
// and a true tiny-gap fraction, ball stable-ID tie, box permutations, soft-sum
// permutation, real overflow transaction, model3-without-body enable and strict
// import algebra.

// t057BoxRunners builds the predeclared 22-body 2D packed box (6-col by 4-row,
// last row 4) with a common nonzero drift plus a small centre-directed relative
// velocity and a METRIC-aligned far target.
func t057BoxRunners() []Runner {
	const spacing = 0.60
	const driftX, driftY = 0.5, 0.2
	const inward = 0.05
	var rs []Runner
	id := uint64(1)
	add := func(cx, cy int) {
		x := 0.5 + (float64(cx)-2.5)*spacing/PitchLengthM
		y := 0.5 + (float64(cy)-1.5)*spacing/PitchWidthM
		dx := (0.5 - x) * PitchLengthM
		dy := (0.5 - y) * PitchWidthM
		dn := math.Sqrt(dx*dx + dy*dy)
		vx, vy := driftX, driftY
		if dn > 1e-9 {
			vx += inward * dx / dn
			vy += inward * dy / dn
		}
		speed := math.Sqrt(vx*vx + vy*vy)
		ux, uy := vx/speed, vy/speed
		p := MotionParams{TopSpeedMPS: speed, AccelMPS2: 4, DecelMPS2: 4.5, LateralAccelMPS2: 3}
		r := InertialRunner(x, y, x+ux*50/PitchLengthM, y+uy*50/PitchWidthM, p) // 50 m METRIC target
		r.PlayerID = id
		r.Body = t057Body(0.30, 75)
		r.VXM, r.VYM = vx, vy
		rs = append(rs, r)
		id++
	}
	for cy := 0; cy < 3; cy++ {
		for cx := 0; cx < 6; cx++ {
			add(cx, cy)
		}
	}
	for cx := 0; cx < 4; cx++ {
		add(cx, 3)
	}
	return rs
}

// t057CombinedScene: three body impacts (50 ms, 250 ms and 350 ms) + a far local
// spectator + scripted releases at t=200 ms (controlled at 240 ms) and t=300 ms
// (controlled at 340 ms) so every split (20/60/200/220/260) has a distinct state
// and nonempty event/journal/impact/sample/sidecar tails.
func t057CombinedScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	a := InertialRunner(0.5, 0.5, 0.6, 0.5, t057Params(1))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), 1
	b := InertialRunner(0.5+0.7/PitchLengthM, 0.5, 0.0, 0.5, t057Params(1))
	b.PlayerID, b.Body, b.VXM = 2, t057Body(0.30, 75), -1
	// second colliding pair (impact at 250 ms) on a separate row
	row2Y := 0.5 + 3.0/PitchWidthM
	c := InertialRunner(0.80, row2Y, 0.85, row2Y, t057Params(0.2))
	c.PlayerID, c.Body, c.VXM = 4, t057Body(0.30, 75), 0.2
	d := InertialRunner(0.80+0.7/PitchLengthM, row2Y, 0.75, row2Y, t057Params(0.2))
	d.PlayerID, d.Body, d.VXM = 5, t057Body(0.30, 75), -0.2
	// third colliding pair (impact at 350 ms) on a separate row for post-control tail
	row3Y := 0.5 - 3.0/PitchWidthM
	e := InertialRunner(0.20, row3Y, 0.25, row3Y, t057Params(0.2))
	e.PlayerID, e.Body, e.VXM = 6, t057Body(0.30, 75), 0.2
	f := InertialRunner(0.20+0.74/PitchLengthM, row3Y, 0.15, row3Y, t057Params(0.2))
	f.PlayerID, f.Body, f.VXM = 7, t057Body(0.30, 75), -0.2
	spec := t050Local(0.90, 0.90, 0.90, 0.90, 200.0, 0, true)
	spec.PlayerID, spec.Body = 3, t057Body(0.30, 75)
	sim.SetRunners([]Runner{a, b, c, d, e, f, spec})
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
		t.Fatalf("body enable: %v", err)
	}
	return sim
}

func t057CombinedMid(s *Sim, clock int) {
	switch clock {
	case 200:
		// Runner A stopped at centre + 0.05 m with default ControlRadiusM = 1.0 m.
		// StartX at centre - 1.05 m places the ball 1.10 m from A: after 20 ms
		// travel (5 - 0.6*0.02)*0.02 = 0.09976 m, distance is 1.00024 m > 1.0 m at
		// 220 ms (still rolling in flight), then controlled by A in 220->240 ms.
		s.ReleasePass(PassParams{StartX: 0.50 - 1.05/PitchLengthM, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 5.0, Passer: -1})
	case 300:
		// Analogous second release after the 260 ms post-control split: StartX at
		// 0.80 - 1.05 m is 1.10 m from runner C (0.80 + 0.05 m, default 1.0 m
		// reach), still rolling at 320 ms and controlled by C in 320->340 ms.
		row2Y := 0.5 + 3.0/PitchWidthM
		s.ReleasePass(PassParams{StartX: 0.80 - 1.05/PitchLengthM, StartY: row2Y, TargetX: 0.85, TargetY: row2Y, SpeedMPS: 5.0, Passer: 0})
	}
}

func t057FirstBodyImpactAt(s *Sim) (BodyImpact, bool) {
	if s.body == nil || len(s.body.Impacts) == 0 {
		return BodyImpact{}, false
	}
	return s.body.Impacts[0], true
}

// TestT057JSONCombinedTails (receipt 1).
func TestT057JSONCombinedTails(t *testing.T) {
	const horizon = 440
	for _, split := range []int{20, 60, 200, 220, 260} {
		straight := t057CombinedScene(t, 1)
		for straight.ClockMS() < split {
			t057CombinedMid(straight, straight.ClockMS())
			if err := straight.StepChecked(); err != nil {
				t.Fatalf("split %d approach: %v", split, err)
			}
		}
		// split-label transition assertions
		switch split {
		case 20:
			if _, ok := t057FirstBodyImpactAt(straight); ok {
				t.Fatalf("split 20 already has a body impact (expected before 50ms)")
			}
		case 60:
			im, ok := t057FirstBodyImpactAt(straight)
			if !ok || math.Abs(im.Fraction-0.5) > 1e-10 {
				t.Fatalf("split 60 expected first impact at frac 0.5, got %+v ok=%v", im, ok)
			}
		case 200:
			if len(straight.Events()) != 0 {
				t.Fatalf("split 200 pre-release unexpectedly has events: %+v", straight.Events())
			}
		case 220:
			// Release at 200 ms happened BEFORE split 220; ball is still rolling in flight.
			rel := t051ReleaseClocks(straight.Events())
			if len(rel) != 1 || rel[0] != 200 {
				t.Fatalf("split 220 expected release at 200 before split, got %v", rel)
			}
			if straight.Ball().Mode != BallRolling {
				t.Fatalf("split 220 expected rolling ball in flight, got %s", straight.Ball().Mode)
			}
			if _, ok := t057FirstTouchEvent(straight, PassEventControl); ok {
				t.Fatal("split 220 unexpectedly already controlled before 240ms")
			}
		case 260:
			// Post-control split: first pass was controlled at 240 ms and held at 260 ms.
			ev, ok := t057FirstTouchEvent(straight, PassEventControl)
			if !ok || ev.ClockMS != 240 {
				t.Fatalf("split 260 expected prior control at 240ms, got %+v ok=%v", ev, ok)
			}
			if straight.Ball().Mode != BallHeld || straight.Snapshot().Integrated.LastTouchID != 1 {
				t.Fatalf("split 260 expected held ball by ID 1, mode=%s touch=%d", straight.Ball().Mode, straight.Snapshot().Integrated.LastTouchID)
			}
		}
		jAt := len(straight.Snapshot().Integrated.Journal)
		evAt := len(straight.Events())
		smpAt := len(straight.Samples())
		impAt := len(straight.body.Impacts)
		res := t046JSONResume(t, straight)
		srec := mustRecorder(t, 4096)
		straight.SetProvenanceRecorder(srec)
		rrec := mustRecorder(t, 4096)
		res.SetProvenanceRecorder(rrec)
		for straight.ClockMS() < horizon {
			t057CombinedMid(straight, straight.ClockMS())
			t057CombinedMid(res, res.ClockMS())
			if err := straight.StepChecked(); err != nil {
				t.Fatalf("straight: %v", err)
			}
			if err := res.StepChecked(); err != nil {
				t.Fatalf("resumed: %v", err)
			}
			if !reflect.DeepEqual(straight.Snapshot(), res.Snapshot()) || !snapEqualPerStep(straight.Snapshot(), res.Snapshot()) {
				t.Fatalf("split %d: snapshot diverged at %d", split, straight.ClockMS())
			}
		}
		// absolute tails (values), nonempty and unshifted
		sImp := straight.body.Impacts[impAt:]
		rImp := res.body.Impacts[impAt:]
		if !reflect.DeepEqual(sImp, rImp) || len(sImp) == 0 {
			t.Fatalf("split %d: body-impact tail empty or differs (%d/%d)", split, len(sImp), len(rImp))
		}
		sEv := straight.Events()[evAt:]
		rEv := res.Events()[evAt:]
		if !reflect.DeepEqual(sEv, rEv) || len(sEv) == 0 {
			t.Fatalf("split %d: event tail empty or differs", split)
		}
		hasControlTail := false
		for _, e := range rEv {
			if e.Kind == PassEventControl {
				hasControlTail = true
				if split == 260 && e.ClockMS != 340 {
					t.Fatalf("split 260 post-control tail expected second control at 340ms, got %d", e.ClockMS)
				}
				break
			}
		}
		if !hasControlTail {
			t.Fatalf("split %d: event tail has no control event: %+v", split, rEv)
		}
		sJ := straight.Snapshot().Integrated.Journal[jAt:]
		rJ := res.Snapshot().Integrated.Journal[jAt:]
		if !reflect.DeepEqual(sJ, rJ) || len(sJ) == 0 {
			t.Fatalf("split %d: journal tail empty or differs", split)
		}
		if !reflect.DeepEqual(res.Samples(), straight.Samples()[smpAt:]) || len(res.Samples()) == 0 {
			t.Fatalf("split %d: sample tail empty or differs", split)
		}
		if !reflect.DeepEqual(srec.Records(), rrec.Records()) || len(rrec.Records()) == 0 {
			t.Fatalf("split %d: sidecar tail empty or differs (%d/%d)", split, len(srec.Records()), len(rrec.Records()))
		}
		t.Logf("T-057 JSON split=%d: impact(%d)/event(%d)/journal(%d)/sample(%d)/sidecar(%d) tails nonempty+equal", split, len(rImp), len(rEv), len(rJ), len(res.Samples()), len(rrec.Records()))
	}
}

// TestT057HeldCarrierBump (receipt 2): independent analytic control root, XY
// body/ball contact, hold offsets and END positions after the full post-impact
// remainder, checked with METRE tolerances (not state-derived tautologies).
func TestT057HeldCarrierBump(t *testing.T) {
	const (
		substepS = 0.02
		centreXM = 0.5 * PitchLengthM
		centreYM = 0.5 * PitchWidthM
		aX0M     = -0.35
		aY0M     = 0.0
		aVXM     = 5.0
		bX0M     = 0.35
		bY0M     = 0.0
		bVXM     = -5.0
		radM     = 0.30
		massKG   = 75.0
		reachM   = 0.06
		ballX0M  = -0.25
		ballY0M  = -0.13
		ballVYM  = 10.0
	)
	// Independent analytic pre-impact ball-control root of 125 t^2 - 3.6 t + 0.0233 = 0.
	tControlS := (3.6 - math.Sqrt(3.6*3.6-4*125*0.0233)) / (2 * 125)
	tControlMS := tControlS * 1000.0
	if math.Abs(tControlMS-9.82179074309616) > 1e-12 {
		t.Fatalf("analytic root %.15f ms != 9.82179074309616 ms", tControlMS)
	}
	wantControlFrac := tControlS / substepS
	wantRunnerConXM := centreXM + aX0M + aVXM*tControlS
	wantRunnerConYM := centreYM + aY0M
	wantBallConXM := centreXM + ballX0M
	wantBallConYM := centreYM + ballY0M + ballVYM*tControlS
	wantHoldDXM := wantBallConXM - wantRunnerConXM
	wantHoldDYM := wantBallConYM - wantRunnerConYM
	if math.Abs(math.Hypot(wantHoldDXM, wantHoldDYM)-reachM) > 1e-12 {
		t.Fatalf("analytic contact distance %.15f m != reach %.15f m", math.Hypot(wantHoldDXM, wantHoldDYM), reachM)
	}
	// Independent analytic body impact at TOI = (0.70 - 0.60) / 10 = 0.01 s (10 ms, frac 0.5).
	tImpactS := ((bX0M - aX0M) - 2*radM) / (aVXM - bVXM)
	wantImpactFrac := tImpactS / substepS
	wantImpX1M := centreXM + aX0M + aVXM*tImpactS
	wantImpY1M := centreYM + aY0M
	wantImpX2M := centreXM + bX0M + bVXM*tImpactS
	wantImpY2M := centreYM + bY0M
	wantImpulse := (aVXM - bVXM) / (1/massKG + 1/massKG)
	// After zero-restitution equal-mass impact at 10 ms, both A and B have outgoing
	// velocity 0 for the remaining 10 ms, so their end positions at 20 ms equal
	// their 10 ms impact positions, and the held ball ends at A_end + holdOffset.
	wantAEndXM, wantAEndYM := wantImpX1M, wantImpY1M
	wantBEndXM, wantBEndYM := wantImpX2M, wantImpY2M
	wantBallEndXM := wantAEndXM + wantHoldDXM
	wantBallEndYM := wantAEndYM + wantHoldDYM

	sim := t057BallBodyScene(t, ballY0M)
	if err := sim.StepChecked(); err != nil {
		t.Fatalf("step: %v", err)
	}
	ev, ok := t057FirstTouchEvent(sim, PassEventControl)
	if !ok {
		t.Fatal("no control event")
	}
	if ev.Runner != 0 {
		t.Fatalf("control actor %d, want 0 (A)", ev.Runner)
	}
	gotControlMS := float64(ev.ClockMS-sim.cfg.SubstepMS) + ev.Frac*float64(sim.cfg.SubstepMS)
	if math.Abs(gotControlMS-tControlMS) > 1e-8 || math.Abs(ev.Frac-wantControlFrac) > 1e-10 {
		t.Fatalf("control time=%.12f ms (frac=%.12f), want %.12f ms (frac=%.12f)", gotControlMS, ev.Frac, tControlMS, wantControlFrac)
	}
	if math.Abs(ev.RunnerX*PitchLengthM-wantRunnerConXM) > 1e-9 || math.Abs(ev.RunnerY*PitchWidthM-wantRunnerConYM) > 1e-9 {
		t.Fatalf("control runner XY=(%.12f,%.12f) m, want (%.12f,%.12f) m", ev.RunnerX*PitchLengthM, ev.RunnerY*PitchWidthM, wantRunnerConXM, wantRunnerConYM)
	}
	if math.Abs(ev.BallX*PitchLengthM-wantBallConXM) > 1e-9 || math.Abs(ev.BallY*PitchWidthM-wantBallConYM) > 1e-9 {
		t.Fatalf("control ball XY=(%.12f,%.12f) m, want (%.12f,%.12f) m", ev.BallX*PitchLengthM, ev.BallY*PitchWidthM, wantBallConXM, wantBallConYM)
	}
	if math.Abs(sim.ball.HoldDX*PitchLengthM-wantHoldDXM) > 1e-9 || math.Abs(sim.ball.HoldDY*PitchWidthM-wantHoldDYM) > 1e-9 {
		t.Fatalf("hold offset=(%.12f,%.12f) m, want (%.12f,%.12f) m", sim.ball.HoldDX*PitchLengthM, sim.ball.HoldDY*PitchWidthM, wantHoldDXM, wantHoldDYM)
	}
	im, ok := t057FirstBodyImpactAt(sim)
	if !ok || im.ClockMS != 20 || math.Abs(im.Fraction-wantImpactFrac) > 1e-10 {
		t.Fatalf("body impact %+v, want frac %.12f clock 20", im, wantImpactFrac)
	}
	if im.PlayerID1 != 1 || im.PlayerID2 != 2 || math.Abs(im.Impulse-wantImpulse) > 1e-6 {
		t.Fatalf("body impact ids=%d/%d impulse=%.9f, want 1/2 %.9f", im.PlayerID1, im.PlayerID2, im.Impulse, wantImpulse)
	}
	if math.Abs(im.X1*PitchLengthM-wantImpX1M) > 1e-9 || math.Abs(im.Y1*PitchWidthM-wantImpY1M) > 1e-9 ||
		math.Abs(im.X2*PitchLengthM-wantImpX2M) > 1e-9 || math.Abs(im.Y2*PitchWidthM-wantImpY2M) > 1e-9 {
		t.Fatalf("body impact positions A(%.12f,%.12f) B(%.12f,%.12f) m", im.X1*PitchLengthM, im.Y1*PitchWidthM, im.X2*PitchLengthM, im.Y2*PitchWidthM)
	}
	if math.Abs(im.OutVX1) > 1e-9 || math.Abs(im.OutVY1) > 1e-9 || math.Abs(im.OutVX2) > 1e-9 || math.Abs(im.OutVY2) > 1e-9 {
		t.Fatalf("body impact outgoing velocities (%v,%v)/(%v,%v)", im.OutVX1, im.OutVY1, im.OutVX2, im.OutVY2)
	}
	a, b := sim.runners[0], sim.runners[1]
	if math.Abs(a.X*PitchLengthM-wantAEndXM) > 1e-9 || math.Abs(a.Y*PitchWidthM-wantAEndYM) > 1e-9 ||
		math.Abs(b.X*PitchLengthM-wantBEndXM) > 1e-9 || math.Abs(b.Y*PitchWidthM-wantBEndYM) > 1e-9 {
		t.Fatalf("end runner positions A(%.12f,%.12f) B(%.12f,%.12f) m, want A(%.12f,%.12f) B(%.12f,%.12f) m",
			a.X*PitchLengthM, a.Y*PitchWidthM, b.X*PitchLengthM, b.Y*PitchWidthM, wantAEndXM, wantAEndYM, wantBEndXM, wantBEndYM)
	}
	if sim.Ball().Mode != BallHeld {
		t.Fatalf("ball mode %s, want held", sim.Ball().Mode)
	}
	if math.Abs(sim.ball.X*PitchLengthM-wantBallEndXM) > 1e-9 || math.Abs(sim.ball.Y*PitchWidthM-wantBallEndYM) > 1e-9 {
		t.Fatalf("end held ball XY=(%.12f,%.12f) m, want (%.12f,%.12f) m", sim.ball.X*PitchLengthM, sim.ball.Y*PitchWidthM, wantBallEndXM, wantBallEndYM)
	}
	// Counterfactuals: pre-impact carry moved the ball +5*(0.01-tControlS) m past
	// its control X before the 10 ms bump stopped A (0.05 m short of unbumped chord).
	unbumpedBallEndXM := centreXM + (aX0M + aVXM*substepS) + wantHoldDXM
	if math.Abs((unbumpedBallEndXM-sim.ball.X*PitchLengthM)-0.05) > 1e-9 {
		t.Fatalf("bumped vs unbumped carrier delta %.12f m, want 0.05 m", unbumpedBallEndXM-sim.ball.X*PitchLengthM)
	}
	smps := sim.Samples()
	if len(smps) != 1 || smps[0].ClockMS != 20 || smps[0].Mode != BallHeld || smps[0].Owner != 0 ||
		math.Abs(smps[0].BallX*PitchLengthM-wantBallEndXM) > 1e-9 || math.Abs(smps[0].BallY*PitchWidthM-wantBallEndYM) > 1e-9 {
		t.Fatalf("end sample %+v does not match analytic end ball state", smps)
	}
	t.Logf("T-057 held bump: tControl=%.12f ms, TOI=10 ms, holdOffset=(%.9f,%.9f) m, endBall=(%.9f,%.9f) m",
		gotControlMS, wantHoldDXM, wantHoldDYM, wantBallEndXM, wantBallEndYM)
}

// TestT057TraceAndNilCheckedGates (receipt 3).
func TestT057TraceAndNilCheckedGates(t *testing.T) {
	// nil-body checked behavior.
	plain := mustSim(t, LivePassConfig(), 1)
	plain.SetRunners([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())})
	before := plain.Snapshot()
	if err := plain.StepChecked(); !errors.Is(err, ErrBodyNotEnabled) {
		t.Fatalf("nil StepChecked err=%v", err)
	}
	if err := plain.StepQuietChecked(); !errors.Is(err, ErrBodyNotEnabled) {
		t.Fatalf("nil StepQuietChecked err=%v", err)
	}
	if err := plain.RunChecked(1000); !errors.Is(err, ErrBodyNotEnabled) {
		t.Fatalf("nil RunChecked err=%v", err)
	}
	if !reflect.DeepEqual(before, plain.Snapshot()) {
		t.Fatal("nil checked calls mutated state")
	}
	// active model3 trace rejection + zero old Trace + source purity.
	sim := t057BodySim(t, 1, t057Chain(t057Params(5)), t057HardConfig())
	full := sim.Snapshot()
	rng := sim.RNGState()
	if _, err := BuildTraceChecked(ClipInput{Sim: sim, HorizonMS: 200}); err == nil {
		t.Fatal("BuildTraceChecked accepted active model3")
	}
	if tr := BuildTrace(ClipInput{Sim: sim, HorizonMS: 200}); !reflect.DeepEqual(tr, Trace{}) {
		t.Fatal("BuildTrace did not return a zero Trace for model3")
	}
	if !reflect.DeepEqual(full, sim.Snapshot()) || rng != sim.RNGState() {
		t.Fatal("trace rejection mutated the source")
	}
	// imported/selected model3 WITHOUT body also rejected.
	imp := mustSim(t, LivePassConfig(), 1)
	imp.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())}))
	if err := imp.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	snap := imp.Snapshot()
	snap.Integrated.ModelVersion = IntegratedModelVersionBody
	sel, err := Resume(snap)
	if err != nil {
		t.Fatalf("model3-no-body resume: %v", err)
	}
	if _, err := BuildTraceChecked(ClipInput{Sim: sel, HorizonMS: 200}); err == nil {
		t.Fatal("BuildTraceChecked accepted selected model3 without body state")
	}
	t.Logf("T-057 trace/nil gates: nil checked ErrBodyNotEnabled/no mutation; model3 (active and selected-no-body) rejected; old Trace zero; source pure")
}

// TestT057StickyFaultZeroHorizonHeld (receipt 3).
func TestT057StickyFaultZeroHorizonHeld(t *testing.T) {
	rs := t057Chain(t057Params(5))
	obs := t050Local(0.90, 0.90, 0.90, 0.90, 200.0, 0, true)
	obs.PlayerID, obs.Body = 4, t057Body(0.30, 75)
	rs = append(rs, obs)
	cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: 1}
	sim := t057BodySimScheduled(t, 1, rs, cfg)
	if err := sim.StepChecked(); err == nil {
		t.Fatal("expected fault")
	}
	// zero-horizon and held: fault must still surface, no nil deref.
	if err := sim.RunChecked(sim.ClockMS()); err == nil {
		t.Fatal("zero-horizon RunChecked hid the fault")
	}
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 8, HorizonMS: sim.ClockMS()}); err == nil {
		t.Fatal("zero-horizon provenance hid the fault")
	}
	res, err := Resume(sim.Snapshot())
	if err != nil {
		t.Fatalf("faulted Resume: %v", err)
	}
	if res.StepError() == nil {
		t.Fatal("Resume lost the sticky fault")
	}
	sim.SetBall(Ball{Mode: BallHeld, X: 0.5, Y: 0.5})
	if err := sim.StepChecked(); err == nil {
		t.Fatal("held-source StepChecked hid the fault")
	}
	if sim.StepError() == nil {
		t.Fatal("StepError nil on faulted sim")
	}
	t.Logf("T-057 sticky: zero-horizon/held checks surface the fault; Resume stays faulted without nil deref")
}

// TestT057TouchingSeparatingAndTinyGap (receipt 3).
func TestT057TouchingSeparatingAndTinyGap(t *testing.T) {
	// Touching but separating: no impulse.
	a := InertialRunner(0.5, 0.5, 0.0, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.30, 75), -3
	b := InertialRunner(0.5+0.6/PitchLengthM, 0.5, 1.0, 0.5, t057Params(5))
	b.PlayerID, b.Body, b.VXM = 2, t057Body(0.30, 75), 3
	s := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := s.StepChecked(); err != nil {
		t.Fatalf("separating: %v", err)
	}
	if len(s.body.Impacts) != 0 {
		t.Fatal("touching-separating produced an impulse")
	}
	// Tiny positive gap 5e-10 m, closing 10 m/s: true fraction ~2.5e-9 > 0.
	// A at centre moving +5, B at centre+0.6+5e-10 moving -5 (gap 5e-10).
	gap := 5e-10
	a2 := InertialRunner(0.5, 0.5, 1.0, 0.5, t057Params(5))
	a2.PlayerID, a2.Body, a2.VXM = 1, t057Body(0.30, 75), 5
	b2 := InertialRunner(0.5+(0.6+gap)/PitchLengthM, 0.5, 0.0, 0.5, t057Params(5))
	b2.PlayerID, b2.Body, b2.VXM = 2, t057Body(0.30, 75), -5
	s2 := t057BodySim(t, 1, []Runner{a2, b2}, t057HardConfig())
	if err := s2.StepChecked(); err != nil {
		t.Fatalf("tiny gap: %v", err)
	}
	if len(s2.body.Impacts) != 1 {
		t.Fatalf("tiny gap impacts %d, want 1", len(s2.body.Impacts))
	}
	wantFrac := (gap / 10) / 0.02 // 2.5e-9
	if math.Abs(s2.body.Impacts[0].Fraction-wantFrac) > 1e-10 {
		t.Fatalf("tiny gap fraction %.12e, want ~%.12e (not zero)", s2.body.Impacts[0].Fraction, wantFrac)
	}
	t.Logf("T-057 touching-separating no impulse; tiny gap frac=%.3e (nonzero)", s2.body.Impacts[0].Fraction)
}

// TestT057BallStableIDTie (receipt 3/4).
func TestT057BallStableIDTie(t *testing.T) {
	build := func(ids []uint64) *Sim {
		sim := mustSim(t, LivePassConfig(), 1)
		var rs []Runner
		for k, id := range ids {
			dy := 0.31 / PitchWidthM
			if k == 1 {
				dy = -0.31 / PitchWidthM
			}
			r := InertialRunner(0.5, 0.5+dy, 0.5, 0.5+dy, t057Params(5))
			r.PlayerID = id
			r.Body = t057Body(0.30, 75)
			r.ReachM = 1.0
			rs = append(rs, r)
		}
		sim.SetRunners(rs)
		sim.SetBall(Ball{Mode: BallRolling, X: 0.5, Y: 0.5, VXM: 2, VYM: 0})
		if err := sim.EnableIntegrated(); err != nil {
			t.Fatalf("enable: %v", err)
		}
		if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
			t.Fatalf("body enable: %v", err)
		}
		if err := sim.StepChecked(); err != nil {
			t.Fatalf("step: %v", err)
		}
		return sim
	}
	s := build([]uint64{2, 1})
	ev, ok := t057FirstTouchEvent(s, PassEventControl)
	if !ok {
		t.Fatal("no ball control")
	}
	// lowest PlayerID (1) must win the equal-time ball tie regardless of roster slot.
	if got := s.runners[ev.Runner].PlayerID; got != 1 {
		t.Fatalf("ball tie winner id %d, want 1", got)
	}
	sRev := build([]uint64{1, 2})
	ev2, _ := t057FirstTouchEvent(sRev, PassEventControl)
	if got := sRev.runners[ev2.Runner].PlayerID; got != 1 {
		t.Fatalf("reversed ball tie winner id %d, want 1", got)
	}
	t.Logf("T-057 ball stable-ID tie: lowest PlayerID wins regardless of roster slot")
}

// TestT057BoxPermutations (receipt 4).
func TestT057BoxPermutations(t *testing.T) {
	step := func(rs []Runner) *Sim {
		sim := t057BodySim(t, 1, rs, t057HardConfig())
		if err := sim.StepChecked(); err != nil {
			t.Fatalf("step: %v", err)
		}
		return sim
	}
	base := t057BoxRunners()
	rev := append([]Runner(nil), base...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	rot := append(append([]Runner(nil), base[7:]...), base[:7]...)
	byID := func(s *Sim) map[uint64]Runner {
		m := map[uint64]Runner{}
		for _, r := range s.runners {
			m[r.PlayerID] = r
		}
		return m
	}
	s0 := step(base)
	m0 := byID(s0)
	for label, rs := range map[string][]Runner{"reverse": rev, "rotate": rot} {
		s := step(rs)
		m := byID(s)
		for id, r0 := range m0 {
			r := m[id]
			if math.Abs(r0.X-r.X) > 1e-9 || math.Abs(r0.Y-r.Y) > 1e-9 || math.Abs(r0.VXM-r.VXM) > 1e-9 || math.Abs(r0.VYM-r.VYM) > 1e-9 {
				t.Fatalf("%s: id %d state differs", label, id)
			}
		}
		if !reflect.DeepEqual(s0.body.Impacts, s.body.Impacts) {
			t.Fatalf("%s: impact ordering differs", label)
		}
	}
	t.Logf("T-057 box permutations: canonical/reverse/rotate identical ID-keyed state and impacts")
}

// TestT057SoftSumPermutation (receipt 4).
func TestT057SoftSumPermutation(t *testing.T) {
	build := func(rs []Runner) *Sim {
		cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 2, MaxResolutions: BodyDefaultMaxResolutions}
		sim := t057BodySim(t, 1, rs, cfg)
		if err := sim.StepChecked(); err != nil {
			t.Fatalf("step: %v", err)
		}
		return sim
	}
	mk := func(id uint64, x, y float64) Runner {
		r := InertialRunner(x, y, x, y, t057Params(5))
		r.PlayerID, r.Body = id, t057Body(0.30, 75)
		return r
	}
	base := []Runner{mk(3, 0.5, 0.5), mk(1, 0.5+1.0/PitchLengthM, 0.5), mk(2, 0.5, 0.5+1.0/PitchWidthM)}
	rev := []Runner{base[2], base[1], base[0]}
	s0 := build(base)
	s1 := build(rev)
	byID := func(s *Sim) map[uint64]Runner {
		m := map[uint64]Runner{}
		for _, r := range s.runners {
			m[r.PlayerID] = r
		}
		return m
	}
	m0, m1 := byID(s0), byID(s1)
	for id, r0 := range m0 {
		r := m1[id]
		if math.Abs(r0.VXM-r.VXM) > 1e-12 || math.Abs(r0.VYM-r.VYM) > 1e-12 {
			t.Fatalf("soft sum id %d differs by roster order", id)
		}
	}
	t.Logf("T-057 soft sum permutation: identical velocity response regardless of slot order")
}

// TestT057OverflowTransaction (receipt 5).
func TestT057OverflowTransaction(t *testing.T) {
	rs := t057Chain(t057Params(5))
	o1 := t050Local(0.90, 0.90, 0.90, 0.90, 200.0, 0, true)
	o1.PlayerID, o1.Body = 4, t057Body(0.30, 75)
	o2 := t050Local(0.10, 0.90, 0.10, 0.90, 200.0, 0, true)
	o2.PlayerID, o2.Body = 5, t057Body(0.30, 75)
	rs = append(rs, o1, o2)
	// capacity 1: two eligible local observers produce two due publications ->
	// the detached trial recorder overflows.
	cfg := BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: 1}
	faulting := t057BodySimScheduled(t, 1, rs, cfg)
	rec := mustRecorder(t, 1)
	faulting.SetProvenanceRecorder(rec)
	before := faulting.Snapshot()
	rng := faulting.RNGState()
	recBefore := rec.Records()
	err := faulting.StepChecked()
	if err == nil || faulting.body.Fault.Code != BodyFaultWorkLimit {
		t.Fatalf("faulting step err=%v fault=%+v", err, faulting.body.Fault)
	}
	if !reflect.DeepEqual(recBefore, rec.Records()) || rec.Overflowed() {
		t.Fatal("faulting step wrote/overflowed the ORIGINAL recorder")
	}
	after := faulting.Snapshot()
	after.BodyContacts.Fault = nil
	if !reflect.DeepEqual(before, after) || rng != faulting.RNGState() {
		t.Fatal("failed step mutated state beyond the fault")
	}
	// Healthy twin (Max 65536) with the same recorder prefix/clock/config runs one
	// step and proves overflow actually occurs inside the detached recorder.
	healthy := t057BodySimScheduled(t, 1, rs, BodyContactConfig{SpacingM: 1.8, SoftAccelMPS2: 0, MaxResolutions: BodyDefaultMaxResolutions})
	href := mustRecorder(t, 1)
	healthy.SetProvenanceRecorder(href)
	if err := healthy.StepChecked(); err != nil {
		t.Fatalf("healthy step: %v", err)
	}
	if !href.Overflowed() || href.Len() != 1 {
		t.Fatalf("healthy twin recorder overflowed=%v len=%d, want true/1", href.Overflowed(), href.Len())
	}
	t.Logf("T-057 overflow transaction: faulting max1 leaves the original recorder untouched; healthy twin (65536) fills+overflows capacity 1")
}

// TestT057EnableModel3WithoutBody (receipt 6).
func TestT057EnableModel3WithoutBody(t *testing.T) {
	// Imported selected model3 WITHOUT body state and WITHOUT runner bodies: accepted.
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))}))
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	snap := sim.Snapshot()
	snap.Integrated.ModelVersion = IntegratedModelVersionBody
	sel, err := Resume(snap)
	if err != nil {
		t.Fatalf("model3-no-body resume: %v", err)
	}
	// Add bodies via the raw fixture setter, then EnableBodyContacts accepts the
	// known model3 and preserves it.
	rb := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	rb.PlayerID, rb.Body = 1, t057Body(0.30, 75)
	sel.SetRunners([]Runner{rb})
	if err := sel.EnableBodyContacts(t057HardConfig()); err != nil {
		t.Fatalf("enable body on model3-no-body: %v", err)
	}
	if st, _ := sel.IntegratedState(); st.ModelVersion != IntegratedModelVersionBody {
		t.Fatalf("model %d after enable, want 3", st.ModelVersion)
	}
	// An unresolved raw TurnSide0 orientation introduced AFTER enable is rejected
	// (model1 + orientation is an inconsistent integrated state), not silently kept.
	s2 := mustSim(t, LivePassConfig(), 1)
	s2.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))}))
	if err := s2.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	r2 := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	r2.PlayerID, r2.Body = 1, t057Body(0.30, 75)
	o, _ := NewOrientation(1, 0, 0) // unspecified side
	r2.Orientation = &o
	s2.SetRunners([]Runner{r2})
	if err := s2.EnableBodyContacts(t057HardConfig()); err == nil {
		t.Fatal("unresolved raw orientation on a model1 state was accepted")
	}
	if s2.BodyEnabled() {
		t.Fatal("partial enable on unresolved orientation")
	}
	t.Logf("T-057 model3-no-body enable accepted/preserved; unresolved raw orientation rejected transactionally")
}

// TestT057SlowRepresentableContact: permanent regression for slow-closing swept
// body contacts above BodyClosingTolMPS (1e-10 m/s), where relative speed squared
// is 1e-14 (m/s)^2 < bodyEpsM (1e-12 m). Also verifies the BodyClosingTolMPS
// boundary from both above and below.
func TestT057SlowRepresentableContact(t *testing.T) {
	a := InertialRunner(0.5, 0.5, 1, 0.5, t057Params(1e-7))
	a.PlayerID, a.Body, a.VXM = 1, t057Body(0.3, 75), 1e-7
	bx := 0.5 + (0.6+1e-10)/PitchLengthM
	b := InertialRunner(bx, 0.5, bx, 0.5, t057Params(1e-7))
	b.PlayerID, b.Body = 2, t057Body(0.3, 75)
	gap := (b.X-a.X)*PitchLengthM - 0.6
	want := gap / (1e-7 * 0.02)
	s := t057BodySim(t, 1, []Runner{a, b}, t057HardConfig())
	if err := s.StepChecked(); err != nil {
		t.Fatalf("StepChecked: %v", err)
	}
	if len(s.body.Impacts) != 1 {
		t.Fatalf("missed slow representable contact: impacts=%d, want 1", len(s.body.Impacts))
	}
	im := s.body.Impacts[0]
	if math.Abs(im.Fraction-want) > 1e-10 {
		t.Fatalf("slow contact fraction: got %.17g want %.17g", im.Fraction, want)
	}
	if math.Abs(s.runners[0].VXM-5e-8) > 1e-16 || math.Abs(s.runners[1].VXM-5e-8) > 1e-16 {
		t.Fatalf("slow contact outgoing velocities: %.17g / %.17g, want 5e-8 / 5e-8", s.runners[0].VXM, s.runners[1].VXM)
	}
	sepM := (s.runners[1].X - s.runners[0].X) * PitchLengthM
	if math.Abs(sepM-0.6) > 1e-12 {
		t.Fatalf("slow contact final separation %.17g m, want 0.6 m", sepM)
	}

	// Sub-tolerance closing speed (5e-11 m/s < BodyClosingTolMPS=1e-10 m/s) is
	// treated as non-closing and produces no impulse.
	sa := InertialRunner(0.5, 0.5, 1, 0.5, t057Params(5e-11))
	sa.PlayerID, sa.Body, sa.VXM = 1, t057Body(0.3, 75), 5e-11
	sb := InertialRunner(bx, 0.5, bx, 0.5, t057Params(5e-11))
	sb.PlayerID, sb.Body = 2, t057Body(0.3, 75)
	sSub := t057BodySim(t, 1, []Runner{sa, sb}, t057HardConfig())
	if err := sSub.StepChecked(); err != nil {
		t.Fatalf("sub-tolerance StepChecked: %v", err)
	}
	if len(sSub.body.Impacts) != 0 {
		t.Fatalf("sub-tolerance closing produced %d impacts, want 0", len(sSub.body.Impacts))
	}
	t.Logf("T-057 slow contact: gap=%.17g m frac=%.17g out=5e-8/5e-8 sep=%.17g m; sub-tolerance 0 impacts", gap, im.Fraction, sepM)
}

// TestT057OverflowWithoutEligiblePlayers: permanent regression for finite 1e308
// ball/runner values across all branches (rolling with all players reaction-blocked,
// loose, held, dead, and single-runner extreme intent/velocity), verifying visible
// BodyFaultNumerical and exact pre-state preservation except sticky fault.
func TestT057OverflowWithoutEligiblePlayers(t *testing.T) {
	s := mustSim(t, LivePassConfig(), 1)
	a := InertialRunner(0.5, 0.5, 0.5, 0.5, t057Params(5))
	a.PlayerID, a.Body, a.ReactAtMS = 1, t057Body(0.3, 75), 1000
	s.SetRunners([]Runner{a})
	s.SetBall(Ball{Mode: BallRolling, X: 0.1, Y: 0.1, VXM: 1e308})
	if err := s.EnableIntegrated(); err != nil {
		t.Fatal(err)
	}
	if err := s.EnableBodyContacts(t057HardConfig()); err != nil {
		t.Fatal(err)
	}
	rec := mustRecorder(t, 64)
	s.SetProvenanceRecorder(rec)
	before := s.Snapshot()
	samples := s.Samples()
	records := rec.Records()
	err := s.StepChecked()
	if err == nil {
		t.Fatal("numerical overflow committed without checked error")
	}
	if s.StepError() == nil {
		t.Fatal("StepError nil after numerical fault")
	}
	after := s.Snapshot()
	if after.BodyContacts.Fault == nil || after.BodyContacts.Fault.Code != BodyFaultNumerical ||
		after.BodyContacts.Fault.StepStartMS != before.ClockMS || after.BodyContacts.Fault.Fraction != 0 {
		t.Fatalf("wrong fault: %+v", after.BodyContacts.Fault)
	}
	after.BodyContacts.Fault = nil
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(samples, s.Samples()) ||
		!reflect.DeepEqual(records, rec.Records()) || rec.Overflowed() {
		t.Fatal("failed step changed state beyond sticky fault")
	}

	// Additional non-eligible / non-rolling branches (loose, held, dead, single-runner extreme).
	cases := []struct {
		name  string
		setup func(*Sim)
	}{
		{
			name: "loose-extreme-pos-react-blocked",
			setup: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallLoose, X: 1e308, Y: 0.5})
			},
		},
		{
			name: "held-extreme-offset",
			setup: func(sim *Sim) {
				sim.contactRunner, sim.contactMS = 0, 0
				sim.SetBall(Ball{Mode: BallHeld, X: 0.5, Y: 0.5, HoldDX: 1e308})
			},
		},
		{
			name: "dead-extreme-vel",
			setup: func(sim *Sim) {
				sim.SetBall(Ball{Mode: BallDead, X: 0.5, Y: 0.5, VXM: 1e308})
			},
		},
		{
			name: "single-runner-extreme-intent",
			setup: func(sim *Sim) {
				r := sim.Runners()[0]
				r.IntentX = 1e308
				sim.SetRunners([]Runner{r})
			},
		},
		{
			name: "single-runner-extreme-vel",
			setup: func(sim *Sim) {
				r := sim.Runners()[0]
				r.VXM = 1e200
				sim.SetRunners([]Runner{r})
			},
		},
	}
	for _, tc := range cases {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		obs := t050Local(0.5, 0.5, 0.5, 0.5, 200.0, 0, true)
		obs.PlayerID, obs.Body, obs.ReactAtMS = 1, t057Body(0.3, 75), 1000
		sim.SetRunners([]Runner{obs})
		tc.setup(sim)
		if err := sim.EnableIntegrated(); err != nil {
			t.Fatalf("%s enable: %v", tc.name, err)
		}
		if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
			t.Fatalf("%s body enable: %v", tc.name, err)
		}
		r := mustRecorder(t, 16)
		sim.SetProvenanceRecorder(r)
		bSnap := sim.Snapshot()
		bSmp := sim.Samples()
		bRec := r.Records()
		if err := sim.StepChecked(); err == nil {
			t.Fatalf("%s: expected numerical fault, got nil", tc.name)
		}
		aSnap := sim.Snapshot()
		if aSnap.BodyContacts.Fault == nil || aSnap.BodyContacts.Fault.Code != BodyFaultNumerical {
			t.Fatalf("%s: wrong fault %+v", tc.name, aSnap.BodyContacts.Fault)
		}
		aSnap.BodyContacts.Fault = nil
		if !reflect.DeepEqual(bSnap, aSnap) || !reflect.DeepEqual(bSmp, sim.Samples()) ||
			!reflect.DeepEqual(bRec, r.Records()) || r.Overflowed() {
			t.Fatalf("%s: failed step mutated state beyond fault", tc.name)
		}
	}
	t.Logf("T-057 overflow without eligible players + all branches: numerical fault surfaced; pre-state/RNG/samples/recorder exact")
}
