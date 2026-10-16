package liveplay

import (
	"math"
	"testing"
)

// T-056 oriented-motion fixtures: rest facing, bounded 180° turn (both sides),
// strafing, matched-prestate arrival probes against the frozen-verified OLD nil
// branch, slowing trajectory, braking saturation, and the kick-alignment spread.
// Runtime stays trig-free; the test uses trig only to audit accumulated turning.

func t056Oriented(t *testing.T, x, y, ix, iy, fx, fy float64, side int) Runner {
	t.Helper()
	r := InertialRunner(x, y, ix, iy, DefaultMotionParams())
	r.PlayerID = 1
	o, err := NewOrientation(fx, fy, side)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	r.Orientation = &o
	return r
}

func t056MotionSim(t *testing.T, rs []Runner) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	return sim
}

func t056Face(t *testing.T, sim *Sim, i int) RunnerOrientation {
	t.Helper()
	rs := sim.Runners()
	if i < 0 || i >= len(rs) || rs[i].Orientation == nil {
		t.Fatalf("runner %d has no orientation", i)
	}
	return *rs[i].Orientation
}

// t056StepRunner advances ONE live-pass substep for a single runner and returns
// its post-step state. It uses a VALIDATED integrated sim (model2 when the runner
// is oriented, model1 for the nil twin), so causal evidence exercises the legal
// opted-in path rather than a raw un-activated roster.
func t056StepRunner(t *testing.T, r Runner) Runner {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners([]Runner{r})
	if err := sim.EnableIntegrated(); err != nil {
		t.Fatalf("EnableIntegrated: %v", err)
	}
	sim.Step()
	return sim.Runners()[0]
}

// t056Omega is the runtime turn envelope for a pre-step speed.
func t056Omega(o RunnerOrientation, p MotionParams, preSpeed float64) float64 {
	omega := o.StandTurnRadsPerSec
	if preSpeed > 1e-9 {
		if lat := p.LateralAccelMPS2 / preSpeed; lat < omega {
			omega = lat
		}
	}
	return omega
}

// t056Threshold is the independent per-step rational facing-turn bound.
func t056Threshold(o RunnerOrientation, p MotionParams, preSpeed, dt float64) float64 {
	h := t056Omega(o, p, preSpeed) * dt
	if h > 1 {
		h = 1
	}
	return (1-h*h/4)/(1+h*h/4) - 1e-12
}

// TestT056RestFacingPersists: an at-rest runner (intent == position) keeps its
// explicit facing over many steps and never drifts.
func TestT056RestFacingPersists(t *testing.T) {
	r := t056Oriented(t, 0.5, 0.5, 0.5, 0.5, 3, 4, 1)
	sim := t056MotionSim(t, []Runner{r})
	before := t056Face(t, sim, 0)
	for i := 0; i < 25; i++ {
		sim.Step()
	}
	after := t056Face(t, sim, 0)
	if after.FX != before.FX || after.FY != before.FY {
		t.Fatalf("rest facing changed: %+v -> %+v", before, after)
	}
	cur := sim.Runners()[0]
	if cur.X != 0.5 || cur.Y != 0.5 {
		t.Fatalf("rest runner drifted to (%v,%v)", cur.X, cur.Y)
	}
	if n := math.Sqrt(after.FX*after.FX + after.FY*after.FY); math.Abs(n-1) > 1e-9 {
		t.Fatalf("facing norm %v", n)
	}
	t.Logf("T-056 rest facing: (%.6f,%.6f) retained 25 steps; runner stationary", after.FX, after.FY)
}

// TestT056BoundedTurn180AndSides: a 180° target turns the facing under the
// independent rational bound with no instantaneous velocity reversal; both turn
// sides are deterministic mirrors.
func TestT056BoundedTurn180AndSides(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	var faces [3][][2]float64
	for _, side := range []int{1, -1} {
		r := t056Oriented(t, 0.5, 0.5, 0.1, 0.5, 1, 0, side)
		r.VXM = 4.0
		sim := t056MotionSim(t, []Runner{r})
		prev := t056Face(t, sim, 0)
		var hist [][2]float64
		for step := 0; step < 250; step++ {
			pre := sim.Runners()[0]
			preSpeed := pre.SpeedMPS()
			sim.Step()
			cur := t056Face(t, sim, 0)
			if n := math.Sqrt(cur.FX*cur.FX + cur.FY*cur.FY); math.Abs(n-1) > 1e-9 {
				t.Fatalf("side %d step %d facing norm %v", side, step, n)
			}
			dot := prev.FX*cur.FX + prev.FY*cur.FY
			th := t056Threshold(prev, p, preSpeed, dt)
			if dot < th-1e-12 {
				t.Fatalf("side %d step %d turn %.15f < bound %.15f", side, step, dot, th)
			}
			cr := sim.Runners()[0]
			if dv := math.Abs(cr.VXM - pre.VXM); dv > math.Max(p.AccelMPS2, p.DecelMPS2)*dt+1e-9 {
				t.Fatalf("side %d step %d |dVx|=%v exceeds bound (instant reversal)", side, step, dv)
			}
			if sp := cr.SpeedMPS(); sp > p.TopSpeedMPS+1e-9 {
				t.Fatalf("side %d step %d speed %v > top", side, step, sp)
			}
			prev = cur
			hist = append(hist, [2]float64{cur.FX, cur.FY})
		}
		end := hist[len(hist)-1]
		if end[0] > -0.9 {
			t.Fatalf("side %d did not complete the turn: end facing (%.4f,%.4f)", side, end[0], end[1])
		}
		faces[sideIndex(side)] = hist
	}
	a := faces[sideIndex(1)]
	b := faces[sideIndex(-1)]
	if len(a) != len(b) {
		t.Fatal("mirror histories differ in length")
	}
	for i := range a {
		if a[i][0] != b[i][0] || a[i][1] != -b[i][1] {
			t.Fatalf("step %d not mirrored: side+1 (%.12f,%.12f) side-1 (%.12f,%.12f)", i, a[i][0], a[i][1], b[i][0], b[i][1])
		}
	}
	t.Logf("T-056 180 turn: both sides bounded and mirrored; end side+1 (%.6f,%.6f)", a[len(a)-1][0], a[len(a)-1][1])
}

func sideIndex(side int) int {
	if side > 0 {
		return 1
	}
	return 2
}

// TestT056AccumulatedTurnBoundAndCompletion audits accumulated turning: the signed
// per-step angle (test-only trig) must never exceed the summed independent bound
// 2*atan(min(omega*dt,1)/2), must progress toward the target and complete ±π.
func TestT056AccumulatedTurnBoundAndCompletion(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	for _, side := range []int{1, -1} {
		r := t056Oriented(t, 0.5, 0.5, 0.1, 0.5, 1, 0, side)
		r.VXM = 4.0
		sim := t056MotionSim(t, []Runner{r})
		prev := t056Face(t, sim, 0)
		sumSigned, sumAbs, sumBound := 0.0, 0.0, 0.0
		for step := 0; step < 300; step++ {
			pre := sim.Runners()[0]
			preSpeed := pre.SpeedMPS()
			sim.Step()
			cur := t056Face(t, sim, 0)
			dot := prev.FX*cur.FX + prev.FY*cur.FY
			crs := prev.FX*cur.FY - prev.FY*cur.FX
			stepAngle := math.Atan2(crs, dot)
			sumSigned += stepAngle
			sumAbs += math.Abs(stepAngle)
			h := t056Omega(prev, p, preSpeed) * dt
			if h > 1 {
				h = 1
			}
			sumBound += 2 * math.Atan(h/2)
			if sumAbs > sumBound+1e-9 {
				t.Fatalf("side %d step %d cumulative |angle| %.12f exceeds bound %.12f", side, step, sumAbs, sumBound)
			}
			prev = cur
		}
		if math.Abs(sumSigned) > sumBound+1e-9 {
			t.Fatalf("side %d accumulated signed |%.12f| exceeds bound %.12f", side, math.Abs(sumSigned), sumBound)
		}
		if math.Abs(sumAbs-math.Pi) > 0.05 {
			t.Fatalf("side %d absolute turn %.6f, want ~pi", side, sumAbs)
		}
		want := math.Pi
		if side < 0 {
			want = -math.Pi
		}
		if math.Abs(sumSigned-want) > 0.05 {
			t.Fatalf("side %d accumulated %.6f, want ~%.6f (direction/completion)", side, sumSigned, want)
		}
		t.Logf("T-056 accumulated turn side %d: signed %.6f (bound sum %.6f), completed toward the target", side, sumSigned, sumBound)
	}
}

// t056Rollout drives rotateFacing directly toward a FIXED unit target with a
// constant h and returns the facing history (a test-only helper rollout).
func t056Rollout(fx, fy, gx, gy, h float64, side, steps int) [][2]float64 {
	hist := make([][2]float64, 0, steps)
	x, y := fx, fy
	for i := 0; i < steps; i++ {
		nx, ny, ok := rotateFacing(x, y, gx, gy, h, side)
		if !ok {
			break
		}
		x, y = nx, ny
		hist = append(hist, [2]float64{x, y})
	}
	return hist
}

// TestT056Accumulated90TurnAbsolute isolates a fixed-unit-target 90° rollout:
// absolute step angles (no cancellation), monotonic target-error reduction,
// per-step and cumulative arithmetic bounds, and completion to the target.
func TestT056Accumulated90TurnAbsolute(t *testing.T) {
	const h = 0.1
	perStep := 2 * math.Atan(h/2)
	for _, g := range [][2]float64{{0, 1}, {0, -1}} {
		hist := t056Rollout(1, 0, g[0], g[1], h, 1, 100)
		errTo := func(x, y float64) float64 { return math.Abs(math.Atan2(x*g[1]-y*g[0], x*g[0]+y*g[1])) }
		prev := [2]float64{1, 0}
		lastErr := errTo(prev[0], prev[1])
		sumAbs, sumBound := 0.0, 0.0
		for i, c := range hist {
			dot := prev[0]*c[0] + prev[1]*c[1]
			crs := prev[0]*c[1] - prev[1]*c[0]
			step := math.Abs(math.Atan2(crs, dot))
			if step > perStep+1e-9 {
				t.Fatalf("g=(%v,%v) step %d angle %v > per-step bound %v", g[0], g[1], i, step, perStep)
			}
			sumAbs += step
			sumBound += perStep
			if sumAbs > sumBound+1e-9 {
				t.Fatalf("g=(%v,%v) step %d cumulative %.12f exceeds bound %.12f", g[0], g[1], i, sumAbs, sumBound)
			}
			if e := errTo(c[0], c[1]); e > lastErr+1e-12 {
				t.Fatalf("g=(%v,%v) step %d target error increased %.12f -> %.12f", g[0], g[1], i, lastErr, e)
			} else {
				lastErr = e
			}
			prev = c
		}
		if math.Abs(sumAbs-math.Pi/2) > 0.02 {
			t.Fatalf("g=(%v,%v) absolute turn %.6f, want ~pi/2", g[0], g[1], sumAbs)
		}
		if lastErr > 1e-9 {
			t.Fatalf("g=(%v,%v) did not reach the target: residual %.12f", g[0], g[1], lastErr)
		}
		t.Logf("T-056 90° absolute rollout g=(%.0f,%.0f): sum |angle| %.6f (bound %.6f), residual %.3e", g[0], g[1], sumAbs, sumBound, lastErr)
	}
}

// TestT056RotateFacingShortestWay pins the turn-direction rule: only an exact
// numerical antiparallel tie uses the persisted side; every other target (incl.
// obtuse non-antiparallel) turns the shortest way regardless of side.
func TestT056RotateFacingShortestWay(t *testing.T) {
	na := math.Sqrt(0.999*0.999 + 0.01*0.01)
	cases := []struct{ fx, fy, gx, gy float64 }{
		{1, 0, -0.6, 0.8},
		{1, 0, -0.8, -0.6},
		{1, 0, -0.999 / na, 0.01 / na}, // near-antiparallel but NOT a tie (|cross|>1e-12), UNIT
	}
	for _, c := range cases {
		for _, side := range []int{1, -1} {
			for _, h := range []float64{0.01, 0.05, 0.2, 0.5} {
				nx, ny, ok := rotateFacing(c.fx, c.fy, c.gx, c.gy, h, side)
				if !ok {
					t.Fatalf("no progress f=(%v,%v) g=(%v,%v) side=%d h=%v", c.fx, c.fy, c.gx, c.gy, side, h)
				}
				if n := math.Sqrt(nx*nx + ny*ny); math.Abs(n-1) > 1e-9 {
					t.Fatalf("non-unit result norm %v", n)
				}
				oldAngle := math.Abs(math.Atan2(c.fx*c.gy-c.fy*c.gx, c.fx*c.gx+c.fy*c.gy))
				newAngle := math.Abs(math.Atan2(nx*c.gy-ny*c.gx, nx*c.gx+ny*c.gy))
				if newAngle > oldAngle+1e-12 {
					t.Fatalf("turned AWAY f=(%v,%v) g=(%v,%v) side=%d h=%v: %.6f -> %.6f", c.fx, c.fy, c.gx, c.gy, side, h, oldAngle, newAngle)
				}
				stepAngle := math.Abs(math.Atan2(c.fx*ny-c.fy*nx, c.fx*nx+c.fy*ny))
				if stepAngle > h+1e-9 {
					t.Fatalf("per-step turn %v > h %v", stepAngle, h)
				}
			}
		}
	}
	// Exact antiparallel tie: the persisted side decides direction.
	for _, side := range []int{1, -1} {
		nx, ny, ok := rotateFacing(1, 0, -1, 0, 0.1, side)
		if !ok {
			t.Fatal("exact antiparallel produced no progress")
		}
		wantSign := float64(side)
		if ny*wantSign <= 0 {
			t.Fatalf("antiparallel side %d turned the wrong way: (%.6f,%.6f)", side, nx, ny)
		}
	}
	t.Logf("T-056 rotateFacing: non-tie targets turn the shortest way for both sides; exact antiparallel tie uses the persisted side")
}

// TestT056RotateFacingClampAndTermination pins the h<=1 clamp, tiny-h progress and
// no-snap fallback of the bounded chord.
func TestT056RotateFacingClampAndTermination(t *testing.T) {
	// Coarse h (e.g. h>1 from a direct caller) clamps: the result is within the
	// h=1 bound and never snaps to the target.
	nx, ny, ok := rotateFacing(1, 0, -1, 0, 5.0, 1)
	if !ok {
		t.Fatal("coarse-h produced no progress")
	}
	limit1 := (1 - 1.0/4) / (1 + 1.0/4) // cos bound for h=1
	if dot := 1*nx + 0*ny; dot < limit1-1e-12 {
		t.Fatalf("h clamp not respected: dot %.12f < %.12f", dot, limit1)
	}
	if nx == -1 && ny == 0 {
		t.Fatal("snapped to the target under coarse h")
	}
	// Tiny h with an off target still makes bounded (possibly zero) progress and
	// never snaps to the target.
	tx, ty, tok := rotateFacing(1, 0, 0, 1, 1e-12, 1)
	if !tok {
		t.Fatal("tiny h returned no progress flag")
	}
	if n := math.Sqrt(tx*tx + ty*ty); math.Abs(n-1) > 1e-9 {
		t.Fatalf("tiny-h result not unit: %v", n)
	}
	if tx == 0 && ty == 1 {
		t.Fatal("tiny h snapped to the target")
	}
	// h<=0 is a no-op.
	if ax, ay, aok := rotateFacing(1, 0, 0, 1, 0, 1); aok || ax != 1 || ay != 0 {
		t.Fatalf("h=0 not a no-op: (%.6f,%.6f,%v)", ax, ay, aok)
	}
	t.Logf("T-056 rotateFacing clamp/termination: coarse h within the h=1 bound, tiny h unit and non-snapping, h=0 a no-op")
}

// TestT056StrafeFacingNotVelocity: moving sideways while facing forward keeps the
// facing independent of the velocity direction.
func TestT056StrafeFacingNotVelocity(t *testing.T) {
	r := t056Oriented(t, 0.5, 0.5, 0.5, 0.9, 1, 0, 1)
	sim := t056MotionSim(t, []Runner{r})
	sim.Step()
	cur := sim.Runners()[0]
	f := t056Face(t, sim, 0)
	if !(cur.VYM > 0) || math.Abs(cur.VXM) > 1e-12 {
		t.Fatalf("strafe velocity not lateral: (%v,%v)", cur.VXM, cur.VYM)
	}
	if f.FX < 0.99 {
		t.Fatalf("facing turned too far in one strafe step: %+v", f)
	}
	vdotf := (cur.VXM*f.FX + cur.VYM*f.FY) / (cur.SpeedMPS() * math.Sqrt(f.FX*f.FX+f.FY*f.FY))
	if vdotf > 0.5 {
		t.Fatalf("facing aligned with strafe velocity (dot=%.4f); not independent", vdotf)
	}
	t.Logf("T-056 strafe: velocity (%.6f,%.6f) lateral, facing (%.6f,%.6f) forward; dot=%.4f", cur.VXM, cur.VYM, f.FX, f.FY, vdotf)
}

// TestT056ArrivalMatchedPrestateProbes: the exact closed-form probe from the
// planner (aligned d=0.5 m, preV=1.5) against the frozen-verified OLD nil branch.
func TestT056ArrivalMatchedPrestateProbes(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	const x0 = 0.10
	const d = 0.5
	const preV = 1.5
	mk := func(oriented bool) Runner {
		r := InertialRunner(x0, 0.5, x0+d/PitchLengthM, 0.5, p)
		r.PlayerID = 1
		r.VXM = preV
		if oriented {
			o, err := NewOrientation(1, 0, 1)
			if err != nil {
				t.Fatalf("NewOrientation: %v", err)
			}
			r.Orientation = &o
		}
		return r
	}
	ro := t056StepRunner(t, mk(true))
	rn := t056StepRunner(t, mk(false))

	wantO := p.TopSpeedMPS * d / DefaultSlowingRadiusM // 8.5*0.5/3
	wantN := math.Sqrt(2 * p.DecelMPS2 * d)            // sqrt(4.5)
	if !(wantN > wantO) {
		t.Fatalf("fixture not in the slowing-limited regime: nil %.6f <= oriented %.6f", wantN, wantO)
	}
	if math.Abs(ro.VXM-wantO) > 1e-9 {
		t.Fatalf("oriented speed %v, want slowing-limited %v", ro.VXM, wantO)
	}
	if math.Abs(rn.VXM-1.58) > 1e-9 {
		t.Fatalf("nil speed %v, want accel-capped 1.58", rn.VXM)
	}
	if math.Abs(ro.X-(x0+ro.VXM*dt/PitchLengthM)) > 1e-9 {
		t.Fatalf("oriented position %v != x0+V*dt", ro.X)
	}
	if math.Abs(rn.X-(x0+rn.VXM*dt/PitchLengthM)) > 1e-9 {
		t.Fatalf("nil position %v != x0+V*dt", rn.X)
	}
	if f := ro.Orientation; f == nil || f.FX != 1 || f.FY != 0 {
		t.Fatalf("aligned probe facing changed: %+v", f)
	}
	t.Logf("T-056 arrival probe: matched prestate d=%.2f preV=%.2f -> oriented V=%.10f (slowing-limited), nil V=%.10f; positions exact", d, preV, ro.VXM, rn.VXM)
}

// TestT056ArrivalFromRestSlowingTrajectory: a from-rest trajectory whose slowing
// term binds (explicit slowing radius 10 m so top*d/Rs < brake through the
// approach); every step is compared against a matched-prestate nil twin.
func TestT056ArrivalFromRestSlowingTrajectory(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	o, err := NewOrientation(1, 0, 1)
	if err != nil {
		t.Fatalf("NewOrientation: %v", err)
	}
	o.SlowingRadiusM = 10.0 // explicit fixture value; binds through the approach
	r := InertialRunner(0.10, 0.5, 0.10+5.0/PitchLengthM, 0.5, p)
	r.PlayerID = 1
	r.Orientation = &o
	sim := t056MotionSim(t, []Runner{r})

	strict := false
	for step := 0; step < 300; step++ {
		pre := sim.Runners()[0]
		twin := pre
		twin.Orientation = nil
		rn := t056StepRunner(t, twin)
		sim.Step()
		cur := sim.Runners()[0]
		if cur.SpeedMPS() > rn.SpeedMPS()+1e-9 {
			t.Fatalf("oriented faster than matched nil at %d: %.10f vs %.10f", step, cur.SpeedMPS(), rn.SpeedMPS())
		}
		if cur.SpeedMPS() < rn.SpeedMPS()-1e-9 {
			strict = true
		}
		if d := DistM(pre.X, pre.Y, cur.X, cur.Y); d > p.TopSpeedMPS*dt+1e-9 {
			t.Fatalf("teleport at %d: %v m", step, d)
		}
		if dv := math.Abs(cur.VXM - pre.VXM); dv > math.Max(p.AccelMPS2, p.DecelMPS2)*dt+1e-9 {
			t.Fatalf("accel bound broken at %d: %v", step, dv)
		}
		if sp := cur.SpeedMPS(); sp > p.TopSpeedMPS+1e-9 {
			t.Fatalf("speed %v > top at %d", sp, step)
		}
	}
	if !strict {
		t.Fatal("from-rest trajectory never slowed strictly below the matched nil twin")
	}
	t.Logf("T-056 from-rest slowing: oriented never exceeds and strictly undershoots the matched-prestate nil twin; bounds and no-teleport hold")
}

// TestT056FastApproachBrakingSaturation: a top-speed approach into a nearby target
// brakes hard but never stops or reverses in one substep and never teleports.
func TestT056FastApproachBrakingSaturation(t *testing.T) {
	p := DefaultMotionParams()
	const dt = 0.02
	r := t056Oriented(t, 0.10, 0.5, 0.10+1.0/PitchLengthM, 0.5, 1, 0, 1)
	r.VXM = 8.5
	sim := t056MotionSim(t, []Runner{r})
	prev := sim.Runners()[0]
	for step := 0; step < 80; step++ {
		sim.Step()
		cur := sim.Runners()[0]
		if dv := math.Abs(cur.VXM - prev.VXM); dv > p.DecelMPS2*dt+1e-9 {
			t.Fatalf("instant stop/reversal at %d: |dVx|=%v", step, dv)
		}
		if d := DistM(prev.X, prev.Y, cur.X, cur.Y); d > p.TopSpeedMPS*dt+1e-9 {
			t.Fatalf("teleport at %d: %v m", step, d)
		}
		if sp := cur.SpeedMPS(); sp > p.TopSpeedMPS+1e-9 {
			t.Fatalf("speed %v > top at %d", sp, step)
		}
		prev = cur
	}
	if prev.SpeedMPS() > 8.5 {
		t.Fatalf("never braked: speed still %v", prev.SpeedMPS())
	}
	t.Logf("T-056 fast approach: braking saturation honest (bounded per-step decel, no instant stop/reversal/teleport), final speed %.4f", prev.SpeedMPS())
}

// TestT056KickAlignmentSpread: at an identical seed the oriented kick spread
// scales the execution errors by the prescribed multiplier, the RNG after the
// identical draws is exact, and zero coefficient/unknown passer is inert.
func TestT056KickAlignmentSpread(t *testing.T) {
	const speed = 10.0
	const dj = 2.0
	const wj = 0.1
	run := func(fx, fy float64, coef float64, passer int) (Ball, [4]uint64) {
		sim := mustSim(t, LivePassConfig(), 7)
		r := InertialRunner(0.30, 0.50, 0.30, 0.50, DefaultMotionParams())
		r.PlayerID = 1
		o, err := NewOrientation(fx, fy, 1)
		if err != nil {
			t.Fatalf("NewOrientation: %v", err)
		}
		o.KickAlignmentSpread = coef
		r.Orientation = &o
		sim.SetRunners([]Runner{r})
		if err := sim.EnableIntegrated(); err != nil {
			t.Fatalf("EnableIntegrated: %v", err)
		}
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: speed, DirectionJitterMPS: dj, WeightJitterFrac: wj, Passer: passer})
		return sim.Ball(), sim.RNGState()
	}

	aligned, rngA := run(1, 0, 1.0, 0)
	opposed, rngO := run(-1, 0, 1.0, 0)
	if rngA != rngO {
		t.Fatal("identical seed/draw count produced different RNG state")
	}
	if d := (opposed.VXM - speed) - 2*(aligned.VXM-speed); math.Abs(d) > 1e-9 {
		t.Fatalf("weight spread multiplier mismatch: aligned dV=%v opposed dV=%v", aligned.VXM-speed, opposed.VXM-speed)
	}
	if d := opposed.VYM - 2*aligned.VYM; math.Abs(d) > 1e-9 {
		t.Fatalf("direction spread multiplier mismatch: aligned=%v opposed=%v", aligned.VYM, opposed.VYM)
	}
	if zeroCoef, _ := run(-1, 0, 0.0, 0); zeroCoef != aligned {
		t.Fatalf("zero coefficient not inert: %+v vs %+v", zeroCoef, aligned)
	}
	if unknown, _ := run(-1, 0, 1.0, -1); unknown != aligned {
		t.Fatalf("passer=-1 not inert: %+v vs %+v", unknown, aligned)
	}
	if oob, _ := run(-1, 0, 1.0, 5); oob != aligned {
		t.Fatalf("out-of-range passer not inert: %+v vs %+v", oob, aligned)
	}
	t.Logf("T-056 kick spread: opposed/aligned weight & direction error ratio exactly 2 (1e-9); RNG exact; zero-coef/unknown/oob passer inert")
}
