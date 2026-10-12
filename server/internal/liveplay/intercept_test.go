package liveplay

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func countKind(sim *Sim, kind PassEventKind) int {
	n := 0
	for _, e := range sim.Events() {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// TestLiveContestSweptCatchesGrazingGlance: the diagnostic legacy endpoint grid
// (1 ms reference) catches shallow grazing glances; the NEW swept live mode must
// catch every one of them at the coarse 20 ms clock, because it solves the chord
// relative-motion entry analytically instead of sampling endpoints.
func TestLiveContestSweptCatchesGrazingGlance(t *testing.T) {
	refCaught, liveCaught, liveMiss := 0, 0, 0
	for i := 0; i <= 400; i++ {
		offsetM := 0.9980 + float64(i)*0.0000045 // ~0.9980 .. 0.9998
		ball := Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: ShotMaxMPS}
		runners := []Runner{{X: 0.55, Y: 0.50 + offsetM/PitchWidthM, ReachM: ControlRadiusM}}

		refMS, _ := contactTime(t, ReferenceConfig(), ball, runners, 12000)
		if refMS < 0 {
			continue
		}
		refCaught++

		sim := mustSim(t, LivePassConfig(), 1)
		sim.SetBall(ball)
		sim.SetRunners(runners)
		caught := false
		for sim.ClockMS() < 12000 {
			sim.StepQuiet()
			if sim.Ball().Mode == BallHeld {
				caught = true
				break
			}
		}
		if caught {
			liveCaught++
		} else {
			liveMiss++
		}
	}
	if refCaught == 0 {
		t.Fatal("reference caught no grazing glance in the scan")
	}
	if liveCaught == 0 {
		t.Fatal("swept live mode caught no glancing contact at all")
	}
	if liveMiss != 0 {
		t.Fatalf("swept live mode missed %d/%d reference grazing glances", liveMiss, refCaught)
	}
	t.Logf("grazing scan: reference caught %d, live swept caught %d, missed %d", refCaught, liveCaught, liveMiss)
}

// TestLiveContestUnreachableDefenderCannotIntercept: a defender whose distance
// from the entire ball path exceeds its reach never touches the ball, while a
// defender placed in the path controls it. Chance never grants unreachable
// possession.
func TestLiveContestUnreachableDefenderCannotIntercept(t *testing.T) {
	pass := PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 16.0, Passer: -1}

	far := mustSim(t, LivePassConfig(), 1)
	far.SetRunners([]Runner{{X: 0.50, Y: 0.50 + 3.0/PitchWidthM, ReachM: ControlRadiusM}})
	far.ReleasePass(pass)
	runUntilHeld(far, 8000)
	if ms, r := far.FirstTouch(); ms != -1 || r != -1 {
		t.Fatalf("a defender 3 m off the lane touched the ball: (%d,%d)", ms, r)
	}

	near := mustSim(t, LivePassConfig(), 1)
	near.SetRunners([]Runner{{X: 0.50, Y: 0.50, ReachM: ControlRadiusM}})
	near.ReleasePass(pass)
	runUntilHeld(near, 8000)
	if ms, r := near.FirstTouch(); ms < 0 || r != 0 {
		t.Fatalf("reachable defender did not control: (%d,%d)", ms, r)
	}
}

// TestLiveContestNearerSlowerLosesRace: a nearer but slow pursuer loses the race
// to a farther but fast one. Reach comes from bounded acceleration/top speed, not
// from who started closest.
func TestLiveContestNearerSlowerLosesRace(t *testing.T) {
	slow := MotionParams{TopSpeedMPS: 5.5, AccelMPS2: 1.5, DecelMPS2: 4.5, LateralAccelMPS2: 1.0}
	fast := DefaultMotionParams()

	ballX := 0.60
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetBall(Ball{Mode: BallLoose, X: ballX, Y: 0.50})
	sim.SetRunners([]Runner{
		PursuingRunner(ballX-5.0/PitchLengthM, 0.50, slow, 0), // nearer (index 0)
		PursuingRunner(ballX-8.0/PitchLengthM, 0.50, fast, 0), // farther (index 1)
	})
	for sim.ClockMS() < 12000 && sim.Ball().Mode != BallHeld {
		sim.StepQuiet()
	}
	ms, r := sim.FirstContact()
	if ms < 0 {
		t.Fatal("nobody reached the loose ball")
	}
	if r != 1 {
		t.Fatalf("nearer slower runner (index 0) won the race (winner=%d)", r)
	}
}

// TestLiveContestPursuitCutsTheLane: a runner starting off the lane cannot touch
// the pass if it does not pursue, but reaches into the lane and intercepts once it
// does — flight is a real contest, not an endpoint roll.
func TestLiveContestPursuitCutsTheLane(t *testing.T) {
	pass := PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 9.0, Passer: -1}
	cutterX, cutterY := 0.55, 0.50-6.0/PitchWidthM
	p := DefaultMotionParams()

	static := mustSim(t, LivePassConfig(), 1)
	static.SetRunners([]Runner{InertialRunner(cutterX, cutterY, cutterX, cutterY, p)})
	static.ReleasePass(pass)
	runUntilHeld(static, 20000)
	if ms, _ := static.FirstTouch(); ms != -1 {
		t.Fatalf("a non-pursuing off-lane runner touched the pass: %d", ms)
	}

	pursuing := mustSim(t, LivePassConfig(), 1)
	pursuing.SetRunners([]Runner{PursuingRunner(cutterX, cutterY, p, 0)})
	pursuing.ReleasePass(pass)
	runUntilHeld(pursuing, 20000)
	if ms, r := pursuing.FirstTouch(); ms < 0 || r != 0 {
		t.Fatalf("pursuing runner did not cut the lane: (%d,%d)", ms, r)
	}
}

// TestLiveContestDeflectionLooseAndCooldown: a contact whose seeded control draw
// falls inside the declared budget miscontrols into a live loose ball (no
// ownership), and the deflector cannot instantly re-touch it during the physical
// re-control cooldown; the bounce stays collectable afterwards.
func TestLiveContestDeflectionLooseAndCooldown(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
		SpeedMPS: 12.0, ControlError: 1.0, Passer: -1,
	})
	deflectClock, deflectFrac := -1, 0.0
	for sim.ClockMS() < 6000 && deflectClock < 0 {
		sim.StepQuiet()
		for _, e := range sim.Events() {
			if e.Kind == PassEventDeflect {
				deflectClock, deflectFrac = e.ClockMS, e.Frac
			}
		}
	}
	if deflectClock < 0 {
		t.Fatal("a full control-error budget never produced a deflection")
	}
	if ms, _ := sim.FirstContact(); ms != -1 {
		t.Fatal("FirstContact was set on a deflection (ownership granted)")
	}
	if m := sim.Ball().Mode; m != BallRolling && m != BallLoose {
		t.Fatalf("deflected ball mode = %v, want rolling/loose (live)", m)
	}
	// The cooldown is anchored to the ACTUAL fractional contact time (declared
	// integer rounding), not the end-step event clock.
	actualContact := deflectClock - SubstepMS + int(math.Round(deflectFrac*float64(SubstepMS)))
	if want := actualContact + DeflectionCooldownMS; sim.runners[0].CooldownUntilMS != want {
		t.Fatalf("deflector cooldown = %d, want %d (actual contact %d)", sim.runners[0].CooldownUntilMS, want, actualContact)
	}

	// Park the (now stationary) ball on the deflector with clean control and step:
	// the physical cooldown must block an instant re-touch, then the loose bounce
	// must be collectable.
	sim.controlError = 0 // subsequent touches control cleanly (test-only)
	sim.SetBall(Ball{Mode: BallLoose, X: sim.runners[0].X, Y: sim.runners[0].Y})
	deadline := actualContact + DeflectionCooldownMS
	var nextControl = -1
	for sim.ClockMS() < deadline+4*DeflectionCooldownMS && nextControl < 0 {
		sim.StepQuiet()
		for _, e := range sim.Events() {
			if e.Kind == PassEventControl {
				nextControl = e.ClockMS - SubstepMS + int(math.Round(e.Frac*float64(SubstepMS)))
			}
		}
	}
	if nextControl < 0 {
		t.Fatal("loose ball was never collectable after the deflection")
	}
	if nextControl < deadline {
		t.Fatalf("deflector re-collected at %d ms, before the cooldown deadline %d ms", nextControl, deadline)
	}
}

// TestLiveContestReactionDelayGatesContact: a runner sitting on the ball's path
// cannot collect before its reaction activates, and does collect at activation
// (inside-substep gating, not an endpoint-only check).
func TestLiveContestReactionDelayGatesContact(t *testing.T) {
	const reactMS = 600
	sim := mustSim(t, LivePassConfig(), 1)
	// The ball is released at the runner's feet and stays within reach, so only
	// the reaction deadline can delay the touch.
	sim.SetRunners([]Runner{{X: 0.50, Y: 0.50, ReachM: ControlRadiusM, ReactMS: reactMS}})
	sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.52, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
	runUntilHeld(sim, 4000)
	ms, r := sim.FirstTouch()
	if r != 0 {
		t.Fatalf("first touch runner = %d, want 0", r)
	}
	if ms < reactMS {
		t.Fatalf("touch at %d ms, before the %d ms reaction", ms, reactMS)
	}
	if ms > reactMS+SubstepMS {
		t.Fatalf("touch at %d ms, later than the %d ms reaction (+1 substep)", ms, reactMS)
	}
}

// TestLiveContestChronologyContactBeforeExit: the earliest physical event wins.
// A ball that would leave the pitch later in the substep is intercepted by an
// earlier contact; with the pattern that the old endpoint check marked OUT before
// the contact, the live order must control it instead.
func TestLiveContestChronologyContactBeforeExit(t *testing.T) {
	// Ball just inside the +x touchline moving out fast; a defender on the line
	// reaches it earlier in the same substep.
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.994, Y: 0.50, VXM: 40.0})
	sim.SetRunners([]Runner{{X: 1.004, Y: 0.50, ReachM: ControlRadiusM}})
	for sim.ClockMS() < 2000 && sim.Ball().Mode == BallRolling {
		sim.StepQuiet()
	}
	if countKind(sim, PassEventOut) != 0 {
		t.Fatalf("ball was ruled out despite an earlier contact: events=%v", sim.Events())
	}
	if ms, r := sim.FirstContact(); ms < 0 || r != 0 {
		t.Fatalf("earlier contact did not control: (%d,%d)", ms, r)
	}
	// The contact event carries an exact within-substep fraction.
	frac := -1.0
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl {
			frac = e.Frac
		}
	}
	if frac < 0 || frac >= 1 {
		t.Fatalf("control fraction = %v, want an in-step value in [0,1)", frac)
	}

	// With the defender beyond reach before the exit, the ball leaves play.
	out := mustSim(t, LivePassConfig(), 1)
	out.SetBall(Ball{Mode: BallRolling, X: 0.994, Y: 0.50, VXM: 40.0})
	out.SetRunners([]Runner{{X: 1.020, Y: 0.50, ReachM: ControlRadiusM}})
	for out.ClockMS() < 2000 && out.Ball().Mode == BallRolling {
		out.StepQuiet()
	}
	if out.Ball().Mode != BallOut {
		t.Fatalf("ball mode = %v, want out", out.Ball().Mode)
	}
	if countKind(out, PassEventOut) == 0 {
		t.Fatal("no out event for the exiting ball")
	}
}

// TestLiveContestSliceOrderFirstContact: the first-contact winner does not depend
// on the runner slice order. The same physical runner wins whichever index it is
// given.
func TestLiveContestSliceOrderFirstContact(t *testing.T) {
	ball := Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 16.0}
	a := Runner{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}
	b := Runner{X: 0.70, Y: 0.50, ReachM: ControlRadiusM}

	winnerPos := func(runners []Runner) [2]float64 {
		sim := mustSim(t, LivePassConfig(), 1)
		sim.SetBall(ball)
		sim.SetRunners(runners)
		for sim.ClockMS() < 4000 && sim.Ball().Mode != BallHeld {
			sim.StepQuiet()
		}
		_, r := sim.FirstContact()
		if r < 0 {
			t.Fatal("nobody controlled the ball")
		}
		got := sim.Runners()[r]
		return [2]float64{got.X, got.Y}
	}

	ab := winnerPos([]Runner{a, b})
	ba := winnerPos([]Runner{b, a})
	if ab != ba {
		t.Fatalf("first contact depended on slice order: %v vs %v", ab, ba)
	}
}

// TestLiveContestCoincidentTieLowestIndex: two runners at the same position reach
// the ball on the same step at the same distance; the LOWEST index wins
// (deterministic tie).
func TestLiveContestCoincidentTieLowestIndex(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 16.0})
	sim.SetRunners([]Runner{
		{X: 0.50, Y: 0.50, ReachM: ControlRadiusM},
		{X: 0.50, Y: 0.50, ReachM: ControlRadiusM},
	})
	for sim.ClockMS() < 4000 && sim.Ball().Mode != BallHeld {
		sim.StepQuiet()
	}
	if _, r := sim.FirstContact(); r != 0 {
		t.Fatalf("tie winner = %d, want 0 (lowest index)", r)
	}
}

// TestLiveContestChasesStoppedBall: a stationary live loose ball is still chased
// and collected (the pursuit targets Rolling OR Loose balls).
func TestLiveContestChasesStoppedBall(t *testing.T) {
	p := DefaultMotionParams()
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetBall(Ball{Mode: BallLoose, X: 0.70, Y: 0.50})
	sim.SetRunners([]Runner{PursuingRunner(0.40, 0.50, p, 0)})
	for sim.ClockMS() < 12000 && sim.Ball().Mode != BallHeld {
		sim.StepQuiet()
	}
	if ms, r := sim.FirstContact(); ms < 0 || r != 0 {
		t.Fatalf("pursuer did not collect the stopped loose ball: (%d,%d)", ms, r)
	}
}

// TestLiveContestMidContestSnapshotResume: a JSON snapshot taken mid-contest
// (loose deflected ball, a pursuer, an active cooldown and consumed RNG) resumes
// bit-identically for the remaining events, samples, first touch and RNG state.
func TestLiveContestMidContestSnapshotResume(t *testing.T) {
	build := func() *Sim {
		sim := mustSim(t, LivePassConfig(), 9)
		p := DefaultMotionParams()
		sim.SetRunners([]Runner{
			{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}, // deflector
			PursuingRunner(0.45, 0.40, p, 0),           // chaser
		})
		sim.ReleasePass(PassParams{
			StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
			SpeedMPS: 12.0, DirectionJitterMPS: 1.0, WeightJitterFrac: 0.05,
			ControlError: 1.0, Passer: -1,
		})
		return sim
	}
	straight := build()
	for straight.ClockMS() < 3000 && straight.Ball().Mode != BallHeld {
		straight.Step()
	}
	if straight.firstTouchMS < 0 {
		t.Fatal("snapshot taken before any contest event")
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
		t.Fatal("resumed runner contest state differs immediately after Resume")
	}

	for straight.ClockMS() < 8000 && straight.Ball().Mode != BallHeld {
		straight.Step()
	}
	for resumed.ClockMS() < 8000 && resumed.Ball().Mode != BallHeld {
		resumed.Step()
	}
	stm, str := straight.FirstTouch()
	rtm, rtr := resumed.FirstTouch()
	if stm != rtm || str != rtr {
		t.Fatalf("resumed first touch (%d,%d) != straight (%d,%d)", rtm, rtr, stm, str)
	}
	scm, scr := straight.FirstContact()
	rcm, rcr := resumed.FirstContact()
	if scm != rcm || scr != rcr {
		t.Fatalf("resumed first contact (%d,%d) != straight (%d,%d)", rcm, rcr, scm, scr)
	}
	if straight.RNGState() != resumed.RNGState() {
		t.Fatal("resumed RNG state diverged")
	}
	allEv := straight.Events()
	headEv := len(allEv) - len(resumed.Events())
	if headEv < 0 || !reflect.DeepEqual(allEv[headEv:], resumed.Events()) {
		t.Fatal("resumed event tail differs from the straight run")
	}
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 || !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatal("resumed sample tail differs from the straight run")
	}
}

// TestLiveContestSeededDeflectionDeterministic: the same seed reproduces the same
// deflection exactly; different seeds produce different loose trajectories.
func TestLiveContestSeededDeflectionDeterministic(t *testing.T) {
	releaseVel := func(seed uint64) (float64, float64) {
		sim := mustSim(t, LivePassConfig(), seed)
		sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
		sim.ReleasePass(PassParams{
			StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
			SpeedMPS: 12.0, ControlError: 1.0, Passer: -1,
		})
		for sim.ClockMS() < 4000 {
			sim.StepQuiet()
			if countKind(sim, PassEventDeflect) > 0 {
				return sim.Ball().VXM, sim.Ball().VYM
			}
		}
		t.Fatal("no deflection")
		return 0, 0
	}
	a1x, a1y := releaseVel(7)
	a2x, a2y := releaseVel(7)
	if a1x != a2x || a1y != a2y {
		t.Fatalf("same seed produced different deflections: (%v,%v) vs (%v,%v)", a1x, a1y, a2x, a2y)
	}
	distinct := map[[2]float64]bool{}
	for seed := uint64(1); seed <= 8; seed++ {
		vx, vy := releaseVel(seed)
		distinct[[2]float64{vx, vy}] = true
	}
	if len(distinct) < 2 {
		t.Fatalf("deflection did not vary across seeds: %d distinct", len(distinct))
	}
}

// TestLiveContestEventFractionsOnGrid: contact/exit events keep the integer
// substep clock and carry an in-step fraction, so a renderer can place the actual
// segment contact rather than the end-step endpoint.
func TestLiveContestEventFractionsOnGrid(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
	runUntilHeld(sim, 8000)
	sawContact := false
	for _, e := range sim.Events() {
		if e.ClockMS%SubstepMS != 0 {
			t.Fatalf("event %v clock %d off the %d ms grid", e.Kind, e.ClockMS, SubstepMS)
		}
		if e.Kind == PassEventControl || e.Kind == PassEventDeflect {
			sawContact = true
			if e.Frac < 0 || e.Frac > 1 {
				t.Fatalf("contact %v fraction %v out of [0,1]", e.Kind, e.Frac)
			}
		}
	}
	if !sawContact {
		t.Fatal("no contact event recorded")
	}
}

// TestLiveContestKinematicRunnerKeepsFullStepMotion: a kinematic runner contacted
// mid-substep must still finish the FULL v*dt of the substep — a contact never
// rewinds the touching participant (the rejected implementation stopped it at the
// contact fraction, a contact-driven slowdown).
func TestLiveContestKinematicRunnerKeepsFullStepMotion(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners([]Runner{{X: 0.50, Y: 0.50, VXM: 3.0, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1})
	dt := float64(SubstepMS) / 1000
	controlled := false
	for i := 0; i < 4000 && !controlled; i++ {
		before := sim.Runners()[0]
		sim.StepQuiet()
		after := sim.Runners()[0]
		if sim.Ball().Mode == BallHeld {
			controlled = true
			wantX := before.X + (before.VXM/PitchLengthM)*dt
			if math.Abs(after.X-wantX) > 1e-15 {
				t.Fatalf("contacted kinematic runner did not finish v*dt: got %.17g want %.17g", after.X, wantX)
			}
			frac := -1.0
			for _, e := range sim.Events() {
				if e.Kind == PassEventControl {
					frac = e.Frac
				}
			}
			if frac < 0 || frac >= 1 {
				t.Fatalf("control fraction = %v, want a mid-step contact in [0,1)", frac)
			}
		}
	}
	if !controlled {
		t.Fatal("pass was never controlled")
	}
}

// TestLiveContestHeldBallFollowsOwnerRemainingFraction: on a clean control the
// ball tracks the owner over the REMAINING fraction, so the end-step sample is the
// owner's end position plus the contact offset — physically different from the
// fractional contact event (no end-step warp to the contact point).
func TestLiveContestHeldBallFollowsOwnerRemainingFraction(t *testing.T) {
	sim, _ := livePassScene(t, 1, 15.0, -3.0) // receiver runs toward the ball
	runUntilHeld(sim, 8000)
	var ev PassEvent
	found := false
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl {
			ev, found = e, true
		}
	}
	if !found {
		t.Fatal("no control event")
	}
	owner := sim.Runners()[ev.Runner]
	offX := ev.BallX - ev.RunnerX
	offY := ev.BallY - ev.RunnerY
	b := sim.Ball()
	if math.Abs(b.X-(owner.X+offX)) > 1e-12 || math.Abs(b.Y-(owner.Y+offY)) > 1e-12 {
		t.Fatalf("end-step ball (%.6f,%.6f) != owner end (%.6f,%.6f) + contact offset (%.6f,%.6f)",
			b.X, b.Y, owner.X, owner.Y, offX, offY)
	}
	moved := math.Abs(owner.X-ev.RunnerX) > 1e-9 || math.Abs(owner.Y-ev.RunnerY) > 1e-9
	if ev.Frac < 1 && moved {
		if math.Abs(b.X-ev.BallX) < 1e-12 && math.Abs(b.Y-ev.BallY) < 1e-12 {
			t.Fatal("held ball did not advance with the owner over the remaining fraction")
		}
	}
}

// TestLiveContestMiscontrolIntegratesRemainingFraction: after a miscontrol the
// ball integrates the REMAINING fraction under the deflected velocity — no
// discarded travel.
func TestLiveContestMiscontrolIntegratesRemainingFraction(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
		SpeedMPS: 12.0, ControlError: 1.0, Passer: -1,
	})
	var ev PassEvent
	found := false
	for sim.ClockMS() < 6000 && !found {
		sim.StepQuiet()
		for _, e := range sim.Events() {
			if e.Kind == PassEventDeflect {
				ev, found = e, true
			}
		}
	}
	if !found {
		t.Fatal("no deflection")
	}
	// No further event in that substep for this open scene, so the ball sits at
	// the segment endpoint reached from the contact under the deflected velocity.
	b := sim.Ball()
	remaining := float64(SubstepMS) * (1 - ev.Frac) / 1000
	expX := ev.BallX + (b.VXM/PitchLengthM)*remaining
	expY := ev.BallY + (b.VYM/PitchWidthM)*remaining
	if math.Abs(b.X-expX) > 1e-12 || math.Abs(b.Y-expY) > 1e-12 {
		t.Fatalf("deflected ball end (%.9f,%.9f) != contact + v*remaining (%.9f,%.9f)", b.X, b.Y, expX, expY)
	}
}

// TestLiveContestTwoEventsSameStepOrdered: a miscontrol and a pitch exit can both
// happen in the SAME substep; their events share the integer end-step clock and
// are ordered by non-decreasing fraction.
func TestLiveContestTwoEventsSameStepOrdered(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.controlError = 1.0 // force the deflection (test-only, no ReleasePass needed)
	// Ball just inside the line moving out fast; a defender BEHIND it deflects it
	// further out within the same substep.
	sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
	sim.SetRunners([]Runner{{X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
	sim.StepQuiet()

	byKind := map[PassEventKind][]PassEvent{}
	for _, e := range sim.Events() {
		byKind[e.Kind] = append(byKind[e.Kind], e)
	}
	def, out := byKind[PassEventDeflect], byKind[PassEventOut]
	if len(def) != 1 || len(out) != 1 {
		t.Fatalf("want one deflect and one out in one step, got deflect=%d out=%d: %v", len(def), len(out), sim.Events())
	}
	if def[0].ClockMS != SubstepMS || out[0].ClockMS != SubstepMS {
		t.Fatalf("same-step events not on the first end-step clock: deflect=%d out=%d", def[0].ClockMS, out[0].ClockMS)
	}
	if def[0].Seq >= out[0].Seq || def[0].Frac > out[0].Frac {
		t.Fatalf("events not ordered by fraction: deflect(seq=%d frac=%v) out(seq=%d frac=%v)",
			def[0].Seq, def[0].Frac, out[0].Seq, out[0].Frac)
	}
	if sim.Ball().Mode != BallOut {
		t.Fatalf("ball mode = %v, want out", sim.Ball().Mode)
	}
}

// TestLiveContestEventFractionsMonotone: across a rich multi-contact scene, every
// event stays on the integer substep grid and, within a substep, fractions never
// decrease (the chronological contract).
func TestLiveContestEventFractionsMonotone(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 5)
	p := DefaultMotionParams()
	sim.SetRunners([]Runner{
		{X: 0.45, Y: 0.48, ReachM: ControlRadiusM},
		PursuingRunner(0.40, 0.44, p, 0),
		{X: 0.60, Y: 0.52, ReachM: ControlRadiusM},
	})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50,
		SpeedMPS: 18.0, DirectionJitterMPS: 2.0, ControlError: 0.8, Passer: -1,
	})
	for sim.ClockMS() < 6000 && sim.Ball().Mode != BallHeld {
		sim.StepQuiet()
	}
	prevClock, prevFrac := -1, -1.0
	for i, e := range sim.Events() {
		if e.Seq != i {
			t.Fatalf("event %d Seq = %d", i, e.Seq)
		}
		if e.ClockMS%SubstepMS != 0 {
			t.Fatalf("event %v clock %d off the %d ms grid", e.Kind, e.ClockMS, SubstepMS)
		}
		if e.ClockMS < prevClock {
			t.Fatalf("event clock went backwards: %d after %d", e.ClockMS, prevClock)
		}
		physical := e.Kind == PassEventControl || e.Kind == PassEventDeflect ||
			e.Kind == PassEventOut || e.Kind == PassEventStop
		if physical {
			if e.ClockMS == prevClock && e.Frac < prevFrac {
				t.Fatalf("same-substep fraction decreased: %v at %d after %v", e.Frac, e.ClockMS, prevFrac)
			}
			prevClock, prevFrac = e.ClockMS, e.Frac
		}
	}
}

// TestLiveContestStationaryLooseMiscontrolKeepsRolling: a stationary LIVE loose
// ball that is miscontrolled must switch to Rolling (not stay Loose) and travel
// for the rest of the BOTH the contact substep and the following substep. This is
// the regression for a deflected ball silently keeping Mode==Loose so it never
// integrates and travel stalls.
func TestLiveContestStationaryLooseMiscontrolKeepsRolling(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.controlError = 1.0 // force the miscontrol (test-only; no ReleasePass)
	sim.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
	sim.SetRunners([]Runner{{X: 0.50, Y: 0.50, ReachM: ControlRadiusM}})
	sim.StepQuiet()

	if countKind(sim, PassEventDeflect) != 1 {
		t.Fatalf("stationary loose contact did not deflect: %v", sim.Events())
	}
	if sim.Ball().Mode != BallRolling {
		t.Fatalf("deflected loose ball mode = %v, want rolling", sim.Ball().Mode)
	}
	afterStep := sim.Ball()

	// The ball must keep integrating on the NEXT substep (the bug left it Loose).
	sim.StepQuiet()
	next := sim.Ball()
	if next.X == afterStep.X && next.Y == afterStep.Y {
		t.Fatalf("deflected ball did not travel on the next substep: %+v", next)
	}
	if Disp := DistM(afterStep.X, afterStep.Y, next.X, next.Y); Disp == 0 {
		t.Fatal("no displacement on the following substep")
	}
}

// TestLiveContestDeflectEventExposesOutgoingVelocity: the deflect event carries
// the incoming velocity in VXM/VYM and the outgoing (bounced) velocity in
// OutVXM/OutVYM, matching the authoritative ball velocity when no further event
// follows in the substep.
func TestLiveContestDeflectEventExposesOutgoingVelocity(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
		SpeedMPS: 12.0, ControlError: 1.0, Passer: -1,
	})
	var ev PassEvent
	found := false
	for sim.ClockMS() < 6000 && !found {
		sim.StepQuiet()
		for _, e := range sim.Events() {
			if e.Kind == PassEventDeflect {
				ev, found = e, true
			}
		}
	}
	if !found {
		t.Fatal("no deflection")
	}
	if ev.VXM == 0 && ev.VYM == 0 {
		t.Fatal("deflect event has no incoming velocity")
	}
	if ev.OutVXM == 0 && ev.OutVYM == 0 {
		t.Fatal("deflect event has no outgoing velocity")
	}
	b := sim.Ball()
	if ev.OutVXM != b.VXM || ev.OutVYM != b.VYM {
		t.Fatalf("outgoing velocity (%v,%v) != authoritative ball (%v,%v)", ev.OutVXM, ev.OutVYM, b.VXM, b.VYM)
	}
}

// TestLiveContestSubstepBoundValidation: a live-pass substep must be shorter than
// the deflection cooldown (so a miscontrolling runner cannot re-touch inside the
// same substep); legacy configs (LivePass false) keep their coarse candidates.
func TestLiveContestSubstepBoundValidation(t *testing.T) {
	over := LivePassConfig()
	over.SubstepMS = DeflectionCooldownMS // 250
	if _, err := NewSim(over, 1); err == nil {
		t.Fatal("NewSim accepted a live-pass substep >= the deflection cooldown")
	} else if !errors.Is(err, ErrLivePassSubstep) {
		t.Fatalf("NewSim error = %v, want ErrLivePassSubstep", err)
	}
	if _, err := Resume(Snapshot{Cfg: over, RNG: [4]uint64{1, 2, 3, 4}}); err == nil {
		t.Fatal("Resume accepted a live-pass substep >= the deflection cooldown")
	}

	// The coarse legacy candidate 200 ms is still allowed for a live-pass config
	// (it is below DeflectionCooldownMS), and for a legacy config.
	ok := LivePassConfig()
	ok.SubstepMS = 200
	if _, err := NewSim(ok, 1); err != nil {
		t.Fatalf("live-pass substep 200 ms rejected: %v", err)
	}
	if _, err := NewSim(Config{SubstepMS: 200, DecelMPS2: RollDecelMPS2}, 1); err != nil {
		t.Fatalf("legacy substep 200 ms rejected: %v", err)
	}
}

// TestLiveContestSnapshotAfterDeflectionResume: a JSON snapshot taken AFTER a
// deflection (the ball is loose/rolling, the deflector's re-control cooldown is
// active and the RNG has been consumed) resumes bit-identically.
func TestLiveContestSnapshotAfterDeflectionResume(t *testing.T) {
	build := func() *Sim {
		sim := mustSim(t, LivePassConfig(), 4)
		sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM}})
		sim.ReleasePass(PassParams{
			StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50,
			SpeedMPS: 12.0, DirectionJitterMPS: 1.0, WeightJitterFrac: 0.05,
			ControlError: 1.0, Passer: -1,
		})
		return sim
	}
	straight := build()
	for straight.ClockMS() < 6000 && countKind(straight, PassEventDeflect) == 0 {
		straight.Step()
	}
	if countKind(straight, PassEventDeflect) == 0 {
		t.Fatal("no deflection before the snapshot")
	}
	if straight.runners[0].CooldownUntilMS <= straight.ClockMS() {
		t.Fatalf("deflector cooldown %d not active at snapshot clock %d", straight.runners[0].CooldownUntilMS, straight.ClockMS())
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
		t.Fatal("resumed runner cooldown state differs immediately after Resume")
	}

	for straight.ClockMS() < 8000 && straight.Ball().Mode != BallHeld {
		straight.Step()
	}
	for resumed.ClockMS() < 8000 && resumed.Ball().Mode != BallHeld {
		resumed.Step()
	}
	stm, str := straight.FirstTouch()
	rtm, rtr := resumed.FirstTouch()
	if stm != rtm || str != rtr {
		t.Fatalf("resumed first touch (%d,%d) != straight (%d,%d)", rtm, rtr, stm, str)
	}
	if straight.RNGState() != resumed.RNGState() {
		t.Fatal("resumed RNG state diverged")
	}
	allEv := straight.Events()
	headEv := len(allEv) - len(resumed.Events())
	if headEv < 0 || !reflect.DeepEqual(allEv[headEv:], resumed.Events()) {
		t.Fatal("resumed event tail differs from the straight run")
	}
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 || !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatal("resumed sample tail differs from the straight run")
	}
}
