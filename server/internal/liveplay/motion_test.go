package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// inertScene builds a simulator with a single inertial runner and a dead ball
// (no contest), so motion can be studied without the contact endpoint firing.
func inertScene(t *testing.T, r Runner) *Sim {
	t.Helper()
	sim := mustSim(t, DefaultConfig(), 1)
	sim.SetRunners([]Runner{r})
	return sim
}

func stepN(sim *Sim, n int) {
	for i := 0; i < n; i++ {
		sim.StepQuiet()
	}
}

func deltaX(r Runner) float64 { return (r.IntentX - r.X) * PitchLengthM }
func deltaY(r Runner) float64 { return (r.IntentY - r.Y) * PitchWidthM }

func distToIntent(r Runner) float64 {
	dx, dy := deltaX(r), deltaY(r)
	return math.Sqrt(dx*dx + dy*dy)
}

// TestMotionModeBackwardCompatible: a zero-valued Runner is kinematic and
// integrates with the exact T-035 constant-velocity formula, so existing spike
// fixtures keep their pinned results without change.
func TestMotionModeBackwardCompatible(t *testing.T) {
	var r Runner
	if r.Motion != MotionKinematic {
		t.Fatalf("zero Runner motion = %v, want kinematic", r.Motion)
	}
	if MotionKinematic.String() != "kinematic" || MotionInertial.String() != "inertial" {
		t.Fatal("motion mode labels changed")
	}
	const vxm = 6.0
	sim := inertScene(t, Runner{X: 0.40, Y: 0.50, VXM: vxm})
	sim.StepQuiet()
	got := sim.Runners()[0]
	wantX := 0.40 + (vxm/PitchLengthM)*(float64(SubstepMS)/1000)
	if math.Abs(got.X-wantX) > 1e-15 {
		t.Fatalf("kinematic step X = %.17g, want %.17g", got.X, wantX)
	}
	if got.VXM != vxm {
		t.Fatalf("kinematic runner speed changed: %v", got.VXM)
	}
}

// TestInertialAcceleratesToSpeedCap: from rest the runner accelerates at no more
// than AccelMPS2 per second and settles at (never above) the top speed.
func TestInertialAcceleratesToSpeedCap(t *testing.T) {
	p := DefaultMotionParams()
	// Intent ~50 m ahead so the brake limit never binds inside the window.
	r := InertialRunner(0.10, 0.50, 0.10+50.0/PitchLengthM, 0.50, p)
	sim := inertScene(t, r)

	dt := float64(SubstepMS) / 1000
	maxStep := p.AccelMPS2 * dt
	prev := 0.0
	reached := false
	for i := 0; i < 200; i++ {
		sim.StepQuiet()
		got := sim.Runners()[0]
		speed := got.SpeedMPS()
		if speed > p.TopSpeedMPS+1e-9 {
			t.Fatalf("step %d speed %.6f exceeded cap %.6f", i, speed, p.TopSpeedMPS)
		}
		if speed-prev > maxStep+1e-9 {
			t.Fatalf("step %d speed jump %.6f exceeds accel step %.6f", i, speed-prev, maxStep)
		}
		if speed >= p.TopSpeedMPS-1e-9 {
			reached = true
		}
		prev = speed
	}
	if !reached {
		t.Fatalf("runner never reached top speed %.3f (final %.3f)", p.TopSpeedMPS, prev)
	}
}

// TestInertialStopsAtIntent: a runner travelling at speed toward a reachable
// destination decelerates and comes to rest at it (bounded braking, no instant
// stop, no teleport).
func TestInertialStopsAtIntent(t *testing.T) {
	p := DefaultMotionParams()
	const ahead = 12.0 // metres
	startX := 0.30
	r := Runner{
		Motion: MotionInertial,
		X:      startX, Y: 0.50,
		VXM:     p.TopSpeedMPS,
		IntentX: startX + ahead/PitchLengthM, IntentY: 0.50,
	}
	r.SetMotionParams(p)
	sim := inertScene(t, r)

	maxStep := p.TopSpeedMPS * float64(SubstepMS) / 1000
	prevX := startX
	for i := 0; i < 800; i++ {
		sim.StepQuiet()
		got := sim.Runners()[0]
		if disp := math.Abs(got.X-prevX) * PitchLengthM; disp > maxStep+1e-9 {
			t.Fatalf("step %d displacement %.6f exceeds %.6f (teleport)", i, disp, maxStep)
		}
		prevX = got.X
	}
	got := sim.Runners()[0]
	if d := distToIntent(got); d > ArriveRadiusM+0.5 {
		t.Fatalf("runner stopped %.3f m from intent, want <= %.3f", d, ArriveRadiusM+0.5)
	}
	if s := got.SpeedMPS(); s > 0.2 {
		t.Fatalf("runner still moving at %.3f m/s after arriving", s)
	}
}

// TestInertial180TurnSlowAndBounded: a full reversal of a player at top speed
// cannot happen instantly. The runner must slow through zero, and every step's
// displacement stays within topSpeed*dt.
func TestInertial180TurnSlowAndBounded(t *testing.T) {
	p := DefaultMotionParams()
	startX := 0.60
	r := Runner{
		Motion: MotionInertial,
		X:      startX, Y: 0.50,
		VXM:     p.TopSpeedMPS,
		IntentX: startX - 26.25/PitchLengthM, IntentY: 0.50, // behind the runner
	}
	r.SetMotionParams(p)
	sim := inertScene(t, r)

	maxStep := p.TopSpeedMPS * float64(SubstepMS) / 1000
	prevX := startX
	sawReverse := false
	minSpeed := math.Inf(1)
	for i := 0; i < 600; i++ {
		sim.StepQuiet()
		got := sim.Runners()[0]
		if disp := math.Abs(got.X-prevX) * PitchLengthM; disp > maxStep+1e-9 {
			t.Fatalf("step %d displacement %.6f exceeds %.6f (teleport)", i, disp, maxStep)
		}
		prevX = got.X
		if got.VXM < 0 {
			sawReverse = true
		}
		if s := got.SpeedMPS(); s < minSpeed {
			minSpeed = s
		}
	}
	if !sawReverse {
		t.Fatal("runner never reversed toward a target behind it")
	}
	// The reversal must have passed near zero speed (no instant flip).
	if minSpeed > 0.5 {
		t.Fatalf("180-degree turn never slowed below %.3f m/s (min speed)", minSpeed)
	}
	if got := sim.Runners()[0]; got.X >= startX {
		t.Fatalf("runner did not end up behind its start (x=%.4f >= %.4f)", got.X, startX)
	}
}

// TestInertialChasesMovingTarget: a faster runner with its intent updated to a
// moving target each substep closes the gap and settles at the braking-distance
// equilibrium (~vTarget^2 / 2*decel) behind the target.
func TestInertialChasesMovingTarget(t *testing.T) {
	p := DefaultMotionParams()
	startX := 0.30
	chaser := InertialRunner(startX, 0.50, startX, 0.50, p)
	sim := inertScene(t, chaser)

	targetX := startX + 20.0/PitchLengthM
	targetV := 2.5 // m/s, slower than the chaser's top speed
	dt := float64(SubstepMS) / 1000
	initialGap := (targetX - startX) * PitchLengthM
	minGap := initialGap
	for i := 0; i < 1600; i++ {
		targetX += targetV * dt / PitchLengthM
		sim.SetIntent(0, targetX, 0.50)
		sim.StepQuiet()
		got := sim.Runners()[0]
		if gap := (targetX - got.X) * PitchLengthM; gap < minGap {
			minGap = gap
		}
	}
	if minGap >= initialGap {
		t.Fatalf("chaser never closed the gap (initial %.2f, min %.2f)", initialGap, minGap)
	}
	if minGap > 1.0 {
		t.Fatalf("faster chaser never caught the target (min gap %.2f m)", minGap)
	}
	// The chaser must have been moving faster than the target, i.e. it actually
	// ran the target down rather than the target stopping.
	if sim.Runners()[0].SpeedMPS() < targetV-0.1 {
		t.Fatalf("chaser settled below target speed: %.3f < %.3f", sim.Runners()[0].SpeedMPS(), targetV)
	}
}

// TestInertialOppositeDirections: two runners converging head-on pass each other
// with bounded speeds and no teleportation, then each arrives at its intent.
func TestInertialOppositeDirections(t *testing.T) {
	p := DefaultMotionParams()
	a := InertialRunner(0.30, 0.50, 0.70, 0.50, p) // runs +x
	b := InertialRunner(0.70, 0.50, 0.30, 0.50, p) // runs -x
	sim := mustSim(t, DefaultConfig(), 1)
	sim.SetRunners([]Runner{a, b})

	maxStep := p.TopSpeedMPS * float64(SubstepMS) / 1000
	prevA, prevB := 0.30, 0.70
	crossed := false
	for i := 0; i < 1200; i++ {
		sim.StepQuiet()
		rs := sim.Runners()
		if d := math.Abs(rs[0].X-prevA) * PitchLengthM; d > maxStep+1e-9 {
			t.Fatalf("runner A step %d displacement %.6f exceeds %.6f", i, d, maxStep)
		}
		if d := math.Abs(rs[1].X-prevB) * PitchLengthM; d > maxStep+1e-9 {
			t.Fatalf("runner B step %d displacement %.6f exceeds %.6f", i, d, maxStep)
		}
		if rs[0].X > rs[1].X {
			crossed = true
		}
		prevA, prevB = rs[0].X, rs[1].X
	}
	if !crossed {
		t.Fatal("runners never crossed despite converging")
	}
	rs := sim.Runners()
	if d := distToIntent(rs[0]); d > ArriveRadiusM+0.5 {
		t.Fatalf("runner A stopped %.3f m from its intent", d)
	}
	if d := distToIntent(rs[1]); d > ArriveRadiusM+0.5 {
		t.Fatalf("runner B stopped %.3f m from its intent", d)
	}
}

// TestInertialAttributeSensitivity: pace, acceleration and fatigue map onto the
// physical limits monotonically and produce the expected motion response.
func TestInertialAttributeSensitivity(t *testing.T) {
	fast := MotionParamsFromAttributes(20, 20, 0)
	slowPace := MotionParamsFromAttributes(5, 20, 0)
	slowAccel := MotionParamsFromAttributes(20, 1, 0)
	tired := MotionParamsFromAttributes(20, 20, 90)

	if !(fast.TopSpeedMPS > slowPace.TopSpeedMPS) {
		t.Fatalf("pace did not raise top speed: %.3f vs %.3f", fast.TopSpeedMPS, slowPace.TopSpeedMPS)
	}
	if !(fast.AccelMPS2 > slowAccel.AccelMPS2) {
		t.Fatalf("acceleration attribute did not raise accel: %.3f vs %.3f", fast.AccelMPS2, slowAccel.AccelMPS2)
	}
	if !(fast.TopSpeedMPS > tired.TopSpeedMPS && fast.AccelMPS2 > tired.AccelMPS2) {
		t.Fatalf("fatigue did not reduce limits: top %.3f->%.3f accel %.3f->%.3f",
			fast.TopSpeedMPS, tired.TopSpeedMPS, fast.AccelMPS2, tired.AccelMPS2)
	}

	// Higher acceleration reaches a given speed sooner from rest.
	const shortMS = 600 // 12 substeps
	mk := func(p MotionParams) *Sim {
		return inertScene(t, InertialRunner(0.10, 0.50, 0.10+50.0/PitchLengthM, 0.50, p))
	}
	accelFast, accelSlow := mk(fast), mk(slowAccel)
	stepN(accelFast, shortMS/SubstepMS)
	stepN(accelSlow, shortMS/SubstepMS)
	if accelFast.Runners()[0].SpeedMPS() <= accelSlow.Runners()[0].SpeedMPS() {
		t.Fatalf("acceleration had no early-speed effect: %.3f vs %.3f",
			accelFast.Runners()[0].SpeedMPS(), accelSlow.Runners()[0].SpeedMPS())
	}

	// Over a fixed time the fresher/faster runner covers more ground.
	fastSim, tiredSim := mk(fast), mk(tired)
	stepN(fastSim, 300)
	stepN(tiredSim, 300)
	fastTravel := fastSim.Runners()[0].X - 0.10
	tiredTravel := tiredSim.Runners()[0].X - 0.10
	if fastTravel <= tiredTravel {
		t.Fatalf("fatigue had no travel effect: full %.4f vs tired %.4f", fastTravel, tiredTravel)
	}
}

// TestInertialNearFarReachability: a near and a far target are both reachable,
// and the far one takes longer to reach (time scales with distance).
func TestInertialNearFarReachability(t *testing.T) {
	p := DefaultMotionParams()
	travelTo := func(metres float64) int {
		sim := inertScene(t, InertialRunner(0.10, 0.50, 0.10+metres/PitchLengthM, 0.50, p))
		for i := 1; i <= 4000; i++ {
			sim.StepQuiet()
			if distToIntent(sim.Runners()[0]) <= 1.0 {
				return i
			}
		}
		return -1
	}
	near := travelTo(6)
	far := travelTo(25)
	if near < 0 || far < 0 {
		t.Fatalf("reachability failed: near=%d far=%d", near, far)
	}
	if far <= near {
		t.Fatalf("far target not slower: near=%d far=%d substeps", near, far)
	}
}

// TestInertialOrderIndependent: a runner's trajectory does not depend on its
// index in the slice or on other runners, i.e. integration reads only its own
// state + intent (no update-order advantage).
func TestInertialOrderIndependent(t *testing.T) {
	p := DefaultMotionParams()
	ra := InertialRunner(0.30, 0.40, 0.70, 0.60, p)
	rb := InertialRunner(0.60, 0.70, 0.20, 0.30, p)

	simA := mustSim(t, DefaultConfig(), 1)
	simA.SetRunners([]Runner{ra, rb})
	simB := mustSim(t, DefaultConfig(), 1)
	simB.SetRunners([]Runner{rb, ra})

	stepN(simA, 500)
	stepN(simB, 500)

	ga := simA.Runners()
	gb := simB.Runners()
	if !reflect.DeepEqual(ga[0], gb[1]) || !reflect.DeepEqual(ga[1], gb[0]) {
		t.Fatalf("trajectory depended on index:\n A = %+v\n B = %+v", ga, gb)
	}
}

// TestInertialJSONMidMovementResume: snapshot in the middle of an inertial
// movement, JSON round-trip it, Resume, and prove the remaining motion is
// bit-identical to the straight run.
func TestInertialJSONMidMovementResume(t *testing.T) {
	p := MotionParamsFromAttributes(17, 15, 25)
	straight := mustSim(t, DefaultConfig(), 1)
	straight.SetRunners([]Runner{
		InertialRunner(0.30, 0.45, 0.75, 0.55, p),
		InertialRunner(0.70, 0.60, 0.25, 0.40, p),
	})
	stepN(straight, 37) // mid-acceleration, nonzero velocity
	if s := straight.Runners()[0].SpeedMPS(); s <= 0 {
		t.Fatalf("runner not moving at snapshot time (speed %.3f)", s)
	}

	blob, err := json.Marshal(straight.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Snapshot
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resumed, err := Resume(back)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !reflect.DeepEqual(straight.Runners(), resumed.Runners()) {
		t.Fatal("resumed inertial runner state differs immediately after Resume")
	}

	stepN(straight, 200)
	stepN(resumed, 200)
	if !reflect.DeepEqual(straight.Runners(), resumed.Runners()) {
		t.Fatalf("resumed inertial motion diverged:\n straight=%+v\n resumed=%+v",
			straight.Runners(), resumed.Runners())
	}
	if !reflect.DeepEqual(straight.Samples(), resumed.Samples()) {
		t.Fatal("resumed sample stream diverged after mid-movement Resume")
	}
}

// TestInertialBenchStaysActive guards the T-036 benchmark pattern: the ball must
// keep rolling and the inertial bodies must keep moving, so the benchmark never
// degenerates into the cheap Dead path or a settled/no-op integrator.
func TestInertialBenchStaysActive(t *testing.T) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim.SetBall(benchBall())
	sim.SetRunners(inertial22())
	const total = 64 * 200
	ballDead, bodiesStill := 0, 0
	for i := 0; i < total; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearm(sim)
			benchRearmInertial22(sim)
		}
		sim.StepQuiet()
		if sim.ball.Mode != BallRolling {
			ballDead++
		}
		// After a step every re-armed body is already accelerating; require them
		// ALL to be genuinely moving so the benchmark never measures a settled
		// integrator or the cheap no-op path.
		moving := 0
		for _, r := range sim.runners {
			if r.SpeedMPS() > 0.05 {
				moving++
			}
		}
		if moving < len(sim.runners) {
			bodiesStill++
		}
	}
	if ballDead != 0 {
		t.Fatalf("%d/%d steps left the ball non-rolling", ballDead, total)
	}
	if bodiesStill != 0 {
		t.Fatalf("%d/%d steps had most inertial bodies still", bodiesStill, total)
	}
}
