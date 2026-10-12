package liveplay

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// T-044 observation-driven local pursuit tests (isolated prototype evidence).
//
// These tests exercise the OPT-IN per-runner local intent policy added in
// localpursuit.go and wired into the T-042 scheduler publication AFTER the T-043
// sample. The policy is fully independent of the legacy native oracle `Pursue`:
// the native publisher always skips a local runner, so a disabled scheduler or an
// invalid radius can never reveal the remote ball position. The policy draws no
// RNG and changes no motion/contact/reach/order/cooldown.
//
// The contract (policy table + near/far fixture) is fixed in the session evidence
// BEFORE this code. No test asserts that the local model equals the native oracle;
// targeted moments where they differ are documented.

// t044LocalRunner builds an inertial local-pursuit runner at (x,y) starting with a
// trivial placeholder intent (its own position) and opts it in.
func t044LocalRunner(t *testing.T, x, y, radiusM float64) Runner {
	t.Helper()
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	if err := r.EnableLocalPursuit(radiusM); err != nil {
		t.Fatalf("EnableLocalPursuit: %v", err)
	}
	return r
}

// t044Intent reads a runner's published destination intent.
func t044Intent(sim *Sim, i int) (float64, float64) {
	return sim.runners[i].IntentX, sim.runners[i].IntentY
}

// t044Perception reads a copy of runner i's observation record.
func t044Perception(t *testing.T, sim *Sim, i int) BallObservation {
	t.Helper()
	o, ok := sim.Perception(i)
	if !ok {
		t.Fatalf("runner %d has no perception record", i)
	}
	return o
}

// --- 1. opt-in helper validation ------------------------------------------

// TestT044EnableLocalPursuitValidation proves the helper validates the radius
// (delegating to EnableSensing) AND requires inertial motion, and imposes no
// hidden motion parameters.
func TestT044EnableLocalPursuitValidation(t *testing.T) {
	// Non-inertial motion is rejected outright (no hidden InertialRunner).
	var kin Runner
	if err := kin.EnableLocalPursuit(4.0); !errors.Is(err, ErrLocalPursuitMotion) {
		t.Fatalf("kinematic EnableLocalPursuit err = %v, want ErrLocalPursuitMotion", err)
	}
	if kin.LocalPursuit || kin.Perception != nil {
		t.Fatal("kinematic helper installed state after an error")
	}

	// Invalid radii are rejected and install nothing.
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
		if err := r.EnableLocalPursuit(bad); !errors.Is(err, ErrSenseRadius) {
			t.Fatalf("EnableLocalPursuit(%v) err = %v, want ErrSenseRadius", bad, err)
		}
		if r.LocalPursuit || r.Perception != nil {
			t.Fatalf("EnableLocalPursuit(%v) installed state after an error", bad)
		}
	}

	r := InertialRunner(0.5, 0.5, 0.5, 0.5, DefaultMotionParams())
	if err := r.EnableLocalPursuit(4.0); err != nil {
		t.Fatalf("valid opt-in rejected: %v", err)
	}
	if !r.LocalPursuit || !r.LocalPursuitEnabled() {
		t.Fatal("valid opt-in did not enable local pursuit")
	}
	if r.Perception == nil || r.Perception.RadiusM != 4.0 {
		t.Fatalf("opt-in did not install the sensing record: %+v", r.Perception)
	}
	// No hidden motion parameters were imposed: the resolved envelope is the
	// caller's (here the DefaultMotionParams envelope).
	if r.Params() != DefaultMotionParams() {
		t.Fatalf("opt-in imposed motion params: %+v", r.Params())
	}
	r.DisableLocalPursuit()
	if r.LocalPursuit || r.LocalPursuitEnabled() {
		t.Fatal("DisableLocalPursuit left the flag set")
	}
	if r.Perception == nil {
		t.Fatal("DisableLocalPursuit must leave the sensing record in place")
	}
}

// --- 2. near chases observed, far (unknown) brakes ------------------------

// TestT044NearChasesObservedFarBrakes is the fixed near/far fixture: at the first
// lattice publication the near runner's intent becomes the OBSERVED ball coords
// (fresh free ball), while the never-seen far runner brakes at its own position
// and never receives the remote ball position.
func TestT044NearChasesObservedFarBrakes(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)

	near := t044LocalRunner(t, 0.60, 0.50+2.0/PitchWidthM, 4.0)
	near.IntentX, near.IntentY = 0.95, 0.95 // non-trivial placeholder
	far := t044LocalRunner(t, 0.10, 0.10, 4.0)
	far.IntentX, far.IntentY = 0.95, 0.95
	farStartX, farStartY := far.X, far.Y
	sim.SetRunners([]Runner{near, far})
	// A stationary loose ball 2 m off the near runner (inside the 4 m circle,
	// outside the 1 m control reach).
	sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})

	sim.Step() // publication at stepStart 0

	nx, ny := t044Intent(sim, 0)
	if nx != 0.60 || ny != 0.50 {
		t.Fatalf("near intent = (%v,%v), want the observed ball coords (0.60,0.50)", nx, ny)
	}
	o := t044Perception(t, sim, 0)
	if !o.Seen || o.Mode != BallLoose || o.X != 0.60 {
		t.Fatalf("near observation wrong: %+v", o)
	}

	fx, fy := t044Intent(sim, 1)
	if fx != farStartX || fy != farStartY {
		t.Fatalf("far (unknown) intent = (%v,%v), want its own start (%v,%v)", fx, fy, farStartX, farStartY)
	}
	if fx == 0.60 && fy == 0.50 {
		t.Fatal("far runner received the remote ball position (oracle fallback)")
	}
	if o := t044Perception(t, sim, 1); o.Seen {
		t.Fatalf("far runner observed an out-of-range ball: %+v", o)
	}
	t.Logf("near chased observed (%.3f,%.3f); far braked at own position (unknown, no leak)", nx, ny)
}

// --- 3. explicit stale-at-DUE retains the prior target ---------------------

// TestT044StaleAtDueRetainsPriorTarget proves that at a DUE publication where the
// ball is ACTUALLY outside the sensing radius (not merely aged between steps) a
// stale observation retains the last fresh destination — never a brake and never
// the remote ball.
func TestT044StaleAtDueRetainsPriorTarget(t *testing.T) {
	const cadence = 100
	cfg := ScheduledLivePassConfig(cadence)
	sim := mustSim(t, cfg, 1)

	// Watcher 1.5 m off the ball lane (inside a 2 m circle, outside the reach);
	// ReachM is tiny so chasing can never control the ball and perturb the fixture.
	r := t044LocalRunner(t, 0.50, 0.50+1.5/PitchWidthM, 2.0)
	r.ReachM = 0.05
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})

	type entry struct {
		clock   int
		fresh   ObservationFreshness
		dist    float64
		intentX float64
		intentY float64
		obsX    float64
		obsY    float64
	}
	var log []entry
	last := sim.DecisionPublishCount()
	for sim.ClockMS() < 4000 {
		start := sim.ClockMS()
		preDist := DistM(sim.ball.X, sim.ball.Y, sim.runners[0].X, sim.runners[0].Y)
		sim.Step()
		if sim.DecisionPublishCount() == last {
			continue
		}
		last = sim.DecisionPublishCount()
		o := sim.runners[0].Perception
		if o == nil {
			continue
		}
		ix, iy := t044Intent(sim, 0)
		log = append(log, entry{start, o.Freshness(start), preDist, ix, iy, o.X, o.Y})
	}

	lastFresh := -1
	for i := range log {
		if log[i].fresh == ObservationFresh {
			lastFresh = i
		}
	}
	if lastFresh < 0 {
		t.Fatal("no fresh publication observed the ball")
	}
	fresh := log[lastFresh]
	if fresh.intentX != fresh.obsX || fresh.intentY != fresh.obsY {
		t.Fatalf("fresh publication did not chase the observation: intent=(%v,%v) obs=(%v,%v)",
			fresh.intentX, fresh.intentY, fresh.obsX, fresh.obsY)
	}

	outside := -1
	for i := lastFresh + 1; i < len(log); i++ {
		if log[i].fresh == ObservationStale && log[i].dist > 2.0 {
			outside = i
			break
		}
	}
	if outside < 0 {
		t.Fatal("no DUE publication found the ball actually outside the radius after the last sighting")
	}
	out := log[outside]
	if out.intentX != fresh.intentX || out.intentY != fresh.intentY {
		t.Fatalf("stale-at-due did NOT retain the prior target: got (%v,%v) want last fresh (%v,%v)",
			out.intentX, out.intentY, fresh.intentX, fresh.intentY)
	}
	t.Logf("last fresh due @%d obs=(%.4f,%.4f) | first outside due @%d dist=%.3fm retained prior target (no brake, no leak)",
		fresh.clock, fresh.obsX, fresh.obsY, out.clock, out.dist)
}

// --- 4. remote second release off-lattice 820 does not leak ---------------

// TestT044RemoteReleaseNoTargetChange proves an off-lattice (820 ms) remote second
// release schedules an invalidation publication but the local runner's target and
// record are unchanged (the new ball is outside its radius).
func TestT044RemoteReleaseNoTargetChange(t *testing.T) {
	const cadence = 100
	cfg := ScheduledLivePassConfig(cadence)
	sim := mustSim(t, cfg, 1)
	r := t044LocalRunner(t, 0.90+2.0/PitchLengthM, 0.50, 4.0)
	r.ReachM = 0.05 // chase but never control the stationary loose ball
	r.IntentX, r.IntentY = 0.95, 0.95
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.50})
	const remoteRelease = 820 // off the 100 ms lattice
	if remoteRelease%cadence == 0 {
		t.Fatal("fixture release is on the lattice")
	}
	for sim.ClockMS() < remoteRelease {
		sim.Step()
	}
	before := t044Perception(t, sim, 0)
	bx, by := t044Intent(sim, 0)
	if before.Mode != BallLoose || bx == 0.95 {
		t.Fatalf("pre-release local state wrong: obs=%+v intent=(%v,%v)", before, bx, by)
	}

	mid := func(s *Sim, clock int) {
		if clock == remoteRelease {
			s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.50, TargetX: 0.30, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
		}
	}
	pubs := t042Run(t, sim, 1600, mid)
	if !containsInt(pubs, remoteRelease) {
		t.Fatalf("remote release did not publish an invalidation at %d: %v", remoteRelease, pubs)
	}
	after := t044Perception(t, sim, 0)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("remote ball leaked into the local record:\n before=%+v\n after =%+v", before, after)
	}
	ax, ay := t044Intent(sim, 0)
	if ax != bx || ay != by {
		t.Fatalf("remote release changed the local target: (%v,%v) -> (%v,%v)", bx, by, ax, ay)
	}
	if after.X == 0.05 {
		t.Fatal("local record holds the remote release position")
	}
	t.Logf("remote release @%dms published but did not change the local target/record", remoteRelease)
}

// --- 5. deflect + out coalesced -> a single refresh -----------------------

// TestT044CoalescedDeflectOutSingleRefresh reuses the proven T-042 coalescing
// fixture (a defender miscontrols a ball leaving the pitch in the same substep)
// and proves the local policy refreshes exactly ONCE at the next pre-step and
// brakes on the observed Out ball.
func TestT044CoalescedDeflectOutSingleRefresh(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	// A local watcher with a wide circle observes the Rolling ball, then the Out.
	watcher := t044LocalRunner(t, 0.93, 0.50, 20.0)
	watcher.IntentX, watcher.IntentY = 0.95, 0.95
	sim.SetRunners([]Runner{watcher, {X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
	sim.controlError = 1.0 // force the miscontrol (test-only, no ReleasePass)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})

	if _, ok := sim.Perception(0); !ok {
		t.Fatal("watcher has no perception record")
	}
	sim.Step() // publication at 0: chase the Rolling ball; deflect+out coalesce in the step
	if countKind(sim, PassEventDeflect) != 1 || countKind(sim, PassEventOut) != 1 {
		t.Fatalf("coalesced fixture did not produce one deflect + one out: def=%d out=%d",
			countKind(sim, PassEventDeflect), countKind(sim, PassEventOut))
	}
	if !sim.decisionDirty {
		t.Fatal("coalesced events left no pending invalidation")
	}

	b0 := sim.DecisionPublishCount()
	preOwnX, preOwnY := sim.runners[0].X, sim.runners[0].Y
	start := sim.ClockMS() // 20
	sim.Step()
	if got := sim.DecisionPublishCount() - b0; got != 1 {
		t.Fatalf("coalesced events caused %d publications, want exactly 1", got)
	}
	if sim.LastDecisionMS() != start {
		t.Fatalf("single refresh at %d ms, want %d", sim.LastDecisionMS(), start)
	}
	o := t044Perception(t, sim, 0)
	if o.Mode != BallOut {
		t.Fatalf("watcher observed mode %s, want out", o.Mode)
	}
	// Fresh Out -> brake at EXACTLY the watcher's own pre-step position.
	ix, iy := t044Intent(sim, 0)
	if ix != preOwnX || iy != preOwnY {
		t.Fatalf("Out observation target (%v,%v), want own pre-step position (%v,%v)", ix, iy, preOwnX, preOwnY)
	}
	t.Logf("coalesced deflect+out: exactly ONE local refresh at %dms; fresh Out braked at own pre-step position (%.4f,%.4f)", start, preOwnX, preOwnY)
}

// --- 6. reaction 130 between lattice clocks 100/200 -----------------------

// TestT044ReactionGateBetweenLattice proves the local target is NEVER updated
// before the reaction deadline: a 130 ms reaction (between lattice 100 and 200)
// first updates at 200.
func TestT044ReactionGateBetweenLattice(t *testing.T) {
	const cadence = 100
	const reactMS = 130
	cfg := ScheduledLivePassConfig(cadence)
	sim := mustSim(t, cfg, 7)
	// Placeholder = own position, so the runner stays put before its reaction.
	r := t044LocalRunner(t, 0.52, 0.50, 4.0)
	r.ReactMS = reactMS
	sim.SetRunners([]Runner{r})
	// A stationary loose ball 2.1 m away: inside the sensing circle, outside reach,
	// so no contact can confound the reaction gate before it arrives.
	sim.ReleasePass(PassParams{StartX: 0.54, StartY: 0.50, TargetX: 0.54, TargetY: 0.50, SpeedMPS: 0, Passer: -1})
	if got := sim.runners[0].ReactAtMS; got != reactMS {
		t.Fatalf("ReactAtMS=%d, want %d", got, reactMS)
	}
	placeholderX, placeholderY := 0.52, 0.50

	firstChange := -1
	var firstIntentX float64
	last := sim.DecisionPublishCount()
	for sim.ClockMS() < 600 {
		start := sim.ClockMS()
		sim.Step()
		if sim.DecisionPublishCount() == last {
			continue
		}
		last = sim.DecisionPublishCount()
		ix, iy := t044Intent(sim, 0)
		if ix != placeholderX || iy != placeholderY { // placeholder changed
			if start < reactMS {
				t.Fatalf("local target updated at %d ms before the %d ms reaction", start, reactMS)
			}
			if firstChange < 0 {
				firstChange = start
				firstIntentX = ix
			}
		}
	}
	wantSense := ((reactMS + cadence - 1) / cadence) * cadence // 200
	if firstChange != wantSense {
		t.Fatalf("first local target update at %d ms, want %d (first lattice >= reaction)", firstChange, wantSense)
	}
	if firstIntentX == placeholderX {
		t.Fatal("first update did not move the intent")
	}
	t.Logf("reaction %d ms gated: first local target update at %d ms (first lattice >= reaction)", reactMS, firstChange)
}

// --- 7. sensed Held/Dead/Out brake; all bodies integrate every substep ------

// TestT044SensedNonFreeModesBrake proves a fresh Held/Dead/Out observation brakes
// the local runner at its own position: the runner starts moving (VX 2 m/s), its
// first publication target is its own pre-step position, it decelerates
// monotonically to rest, every body integrates within the physical step bound, and
// the non-local holder moves EVERY substep.
func TestT044SensedNonFreeModesBrake(t *testing.T) {
	const eps = 1e-9
	cfg := ScheduledLivePassConfig(100)
	cases := []struct {
		name string
		mode BallMode
	}{
		{"held", BallHeld},
		{"dead", BallDead},
		{"out", BallOut},
	}
	const maxStepM = PlayerTopSpeedMPS*float64(SubstepMS)/1000.0 + 1e-9
	for _, c := range cases {
		sim := mustSim(t, cfg, 1)
		holder := InertialRunner(0.50, 0.50, 0.95, 0.95, DefaultMotionParams()) // non-local, keeps moving
		watcher := t044LocalRunner(t, 0.52, 0.50, 8.0)
		watcher.IntentX, watcher.IntentY = 0.95, 0.95
		watcher.VXM = 2.0 // moving, so braking is observable
		watcherStartX, watcherStartY := watcher.X, watcher.Y
		sim.SetRunners([]Runner{holder, watcher})
		sim.SetBall(Ball{Mode: c.mode, X: 0.50, Y: 0.50})
		if c.mode == BallHeld {
			sim.contactRunner = 0
			sim.contactMS = 0
		}

		// First publication (stepStart 0) must brake at the watcher's own pos.
		preOwnX, preOwnY := sim.runners[1].X, sim.runners[1].Y
		prev := sim.Runners()
		sim.Step()
		if tx, ty := t044Intent(sim, 1); tx != preOwnX || ty != preOwnY {
			t.Fatalf("%s: first publication target (%v,%v) != own pre-step position (%v,%v)", c.name, tx, ty, preOwnX, preOwnY)
		}
		// The first step shows inertial motion (nonzero displacement) within bound.
		if d := DistM(prev[1].X, prev[1].Y, sim.runners[1].X, sim.runners[1].Y); d <= eps {
			t.Fatalf("%s: watcher did not move on the first inertial step", c.name)
		}

		prev = sim.Runners()
		prevSpeed := sim.runners[1].SpeedMPS()
		steps := 0
		for sim.ClockMS() < 2000 {
			sim.Step()
			cur := sim.Runners()
			for i := range cur {
				if d := DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y); d > maxStepM {
					t.Fatalf("%s step %d: runner %d jumped %.4f m (> %.4f)", c.name, steps, i, d, maxStepM)
				}
			}
			// Non-local holder must move EVERY substep (not just be moving at the end).
			if d := DistM(prev[0].X, prev[0].Y, cur[0].X, cur[0].Y); d <= eps {
				t.Fatalf("%s: non-local holder did not move at step %d", c.name, steps)
			}
			// Braking: the watcher's speed never increases.
			if sp := cur[1].SpeedMPS(); sp > prevSpeed+1e-9 {
				t.Fatalf("%s step %d: watcher speed increased (%.4f > %.4f) under braking", c.name, steps, sp, prevSpeed)
			} else {
				prevSpeed = sp
			}
			prev = cur
			steps++
		}
		if sim.runners[1].SpeedMPS() > 0.05 {
			t.Fatalf("%s: watcher did not stop under braking (speed %.3f m/s)", c.name, sim.runners[1].SpeedMPS())
		}
		moved := DistM(watcherStartX, watcherStartY, sim.runners[1].X, sim.runners[1].Y)
		if moved < 0.05 || moved > 0.6 {
			t.Fatalf("%s: watcher inertial displacement %.3f m outside the expected braking range", c.name, moved)
		}
		t.Logf("%s: first target = own pre-step pos; watcher braked 2->%.3f m/s over %.3f m; holder moved every one of %d substeps",
			c.name, sim.runners[1].SpeedMPS(), moved, steps)
	}
}

// --- 8. contact reach / order / cooldown intact ---------------------------

// TestT044LocalChaseReachesContactWithinReach proves a local chaser physically
// controls a loose ball and that the contact honours the reach disc (the swept
// resolver is unchanged by the local intent policy).
func TestT044LocalChaseReachesContactWithinReach(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	sim.SetRunners([]Runner{t044LocalRunner(t, 0.50, 0.50, 20.0)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
	for sim.ClockMS() < 4000 && countKind(sim, PassEventControl) == 0 {
		sim.Step()
	}
	var ctrl *PassEvent
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl {
			ev := e
			ctrl = &ev
		}
	}
	if ctrl == nil {
		t.Fatal("local chaser never controlled the loose ball")
	}
	reach := sim.runners[0].ReachM
	if reach <= 0 {
		reach = ControlRadiusM
	}
	if d := DistM(ctrl.BallX, ctrl.BallY, ctrl.RunnerX, ctrl.RunnerY); d > reach+1e-9 {
		t.Fatalf("contact at distance %.4f m exceeds reach %.4f m", d, reach)
	}
	t.Logf("local chaser controlled at %d ms within reach %.2f m", ctrl.ClockMS, reach)
}

// TestT044LocalChaseDeflectionCooldownIntact proves a miscontrol through the local
// policy still sets the persisted physical re-control cooldown, then advances WELL
// past it and checks every later deflector contact against the saved cooldown using
// the ACTUAL fractional event time (eventExactTimeMS), not the integer clock.
func TestT044LocalChaseDeflectionCooldownIntact(t *testing.T) {
	const eps = 1e-6
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 3)
	sim.SetRunners([]Runner{t044LocalRunner(t, 0.55, 0.50, 20.0)})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	for sim.ClockMS() < 8000 && countKind(sim, PassEventDeflect) == 0 {
		sim.Step()
	}
	if countKind(sim, PassEventDeflect) == 0 {
		t.Fatal("no deflection occurred")
	}
	cooldown := sim.runners[0].CooldownUntilMS
	deflectExact := -1.0
	for _, e := range sim.Events() {
		if e.Kind == PassEventDeflect && e.Runner == 0 {
			deflectExact = eventExactTimeMS(e, cfg.SubstepMS)
			break
		}
	}
	if deflectExact < 0 {
		t.Fatal("no deflector deflect event found")
	}
	if float64(cooldown) <= deflectExact+eps {
		t.Fatalf("deflector cooldown %d not after the deflect at %.4f ms", cooldown, deflectExact)
	}

	// Advance to cooldown+500 ms and inspect EVERY later deflector contact by its
	// exact fractional time.
	horizon := cooldown + 500
	for sim.ClockMS() < horizon {
		sim.Step()
	}
	var later []float64
	for _, e := range sim.Events() {
		if e.Runner != 0 || (e.Kind != PassEventControl && e.Kind != PassEventDeflect) {
			continue
		}
		ex := eventExactTimeMS(e, cfg.SubstepMS)
		if ex <= deflectExact+eps {
			continue
		}
		later = append(later, ex)
		if ex < float64(cooldown)-eps {
			t.Fatalf("deflector touched at %.4f ms before its cooldown %d (integer clock %d)", ex, cooldown, e.ClockMS)
		}
	}
	if len(later) == 0 {
		t.Fatalf("no later deflector contact in (%.4f, %d] ms: the cooldown was never exercised (dedicated fixture covers the gate)", deflectExact, horizon)
	}
	t.Logf("local chase deflect @%.4f ms set cooldown until %d ms; %d later contact(s) at %.4f..%.4f ms all >= cooldown",
		deflectExact, cooldown, len(later), later[0], later[len(later)-1])
}

// TestT044CooldownGateBlocksThenSucceeds is a DEDICATED forced near-ball recapture
// fixture: a local runner already inside reach with an active re-control cooldown
// must NOT touch the ball until the cooldown elapses, and MUST touch at/after it.
// This proves the gate blocks and then succeeds, rather than inferring "no early"
// from the absence of a later touch.
func TestT044CooldownGateBlocksThenSucceeds(t *testing.T) {
	const eps = 1e-6
	const cooldown = 500
	cfg := ScheduledLivePassConfig(100)
	sim := mustSim(t, cfg, 1)
	sim.SetRunners([]Runner{t044LocalRunner(t, 0.50, 0.50, 20.0)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.505, Y: 0.50}) // already within reach 1 m
	sim.runners[0].CooldownUntilMS = cooldown             // forced active cooldown
	sim.contactMS, sim.contactRunner = -1, -1

	firstClock := -1
	for sim.ClockMS() < 3000 {
		sim.Step()
		if countKind(sim, PassEventControl)+countKind(sim, PassEventDeflect) > 0 {
			firstClock = sim.ClockMS()
			break
		}
	}
	if firstClock < 0 {
		t.Fatal("forced recapture never touched")
	}
	var first PassEvent
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl || e.Kind == PassEventDeflect {
			first = e
			break
		}
	}
	ex := eventExactTimeMS(first, cfg.SubstepMS)
	if ex < float64(cooldown)-eps {
		t.Fatalf("forced recapture touched at %.4f ms before cooldown %d", ex, cooldown)
	}
	if ex > float64(cooldown)+float64(SubstepMS)+eps {
		t.Fatalf("gate did not release promptly: touch at %.4f ms, cooldown %d", ex, cooldown)
	}
	t.Logf("forced near-ball recapture: blocked until the %.4f ms touch (cooldown %d), then control succeeded", ex, cooldown)
}

// --- 9. invalid raw opt-ins never fall back to the oracle -----------------

// TestT044InvalidRawOptInsNoFallback proves a hand-set local-pursuit runner (which
// SetRunners cannot reject) never receives the remote ball position, even when the
// legacy native oracle is ARMED on the same runner (Pursue=true). The scene places
// the ball far outside the sensing radius, so an oracle fallback would visibly move
// the intent to the remote ball; the guard is the only thing preventing it.
//
// Deleting the `if r.LocalPursuit { continue }` guard in publishPursuitIntents
// makes EVERY subtest here fail (the armed oracle would overwrite the intent).
func TestT044InvalidRawOptInsNoFallback(t *testing.T) {
	const placeholder = 0.95
	const ownX, ownY = 0.60, 0.50
	// Ball far outside every fixture radius, so a leaked oracle target is
	// unambiguous (not coincidentally equal to a brake/observed target).
	ball := Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 1.0}

	// invalidCheck: the local policy must do NOTHING (no oracle, no target change).
	invalidCheck := func(t *testing.T, cfg Config, build func() Runner) {
		t.Helper()
		sim := mustSim(t, cfg, 1)
		r := build()
		if !r.Pursue {
			t.Fatalf("fixture must ARM the native oracle (Pursue=true) so a deleted guard is observable")
		}
		sim.SetRunners([]Runner{r})
		sim.SetBall(ball)
		for sim.ClockMS() < 1000 {
			sim.Step()
		}
		ix, iy := t044Intent(sim, 0)
		if ix == ball.X && iy == ball.Y {
			t.Fatalf("invalid raw opt-in received the remote ball position (oracle fallback): (%v,%v)", ix, iy)
		}
		if ix != placeholder || iy != placeholder {
			t.Fatalf("invalid raw opt-in changed the intent: (%v,%v), want the placeholder", ix, iy)
		}
	}

	t.Run("disabled-scheduler", func(t *testing.T) {
		invalidCheck(t, LivePassConfig(), func() Runner { // IntentCadenceMS == 0
			r := t044LocalRunner(t, ownX, ownY, 4.0) // VALID record + flag
			r.Pursue = true                          // legacy oracle armed; must be skipped
			r.IntentX, r.IntentY = placeholder, placeholder
			return r
		})
	})
	t.Run("missing-record", func(t *testing.T) {
		invalidCheck(t, ScheduledLivePassConfig(100), func() Runner {
			r := InertialRunner(ownX, ownY, placeholder, placeholder, DefaultMotionParams())
			r.Pursue = true
			r.LocalPursuit = true // no perception record
			return r
		})
	})
	t.Run("infinite-radius", func(t *testing.T) {
		invalidCheck(t, ScheduledLivePassConfig(100), func() Runner {
			r := t044LocalRunner(t, ownX, ownY, 4.0)
			r.Pursue = true
			r.Perception.RadiusM = math.Inf(1) // bypass EnableLocalPursuit
			r.IntentX, r.IntentY = placeholder, placeholder
			return r
		})
	})
	t.Run("kinematic-motion", func(t *testing.T) {
		invalidCheck(t, ScheduledLivePassConfig(100), func() Runner {
			r := Runner{X: ownX, Y: ownY, IntentX: placeholder, IntentY: placeholder, LocalPursuit: true, Pursue: true}
			if err := r.EnableSensing(4.0); err != nil {
				t.Fatalf("EnableSensing: %v", err)
			}
			return r
		})
	})

	// Valid local runner (record + flag) with the oracle ARMED: the ball is far out
	// of range, so the local policy brakes at its OWN position while a deleted guard
	// would let the oracle chase the remote ball.
	t.Run("valid-with-armed-oracle-brakes", func(t *testing.T) {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		r := t044LocalRunner(t, ownX, ownY, 4.0)
		r.Pursue = true
		r.IntentX, r.IntentY = placeholder, placeholder
		sim.SetRunners([]Runner{r})
		sim.SetBall(ball)
		for sim.ClockMS() < 1000 {
			sim.Step()
		}
		ix, iy := t044Intent(sim, 0)
		if ix == ball.X && iy == ball.Y {
			t.Fatalf("valid local runner leaked the remote ball position: (%v,%v)", ix, iy)
		}
		if ix != ownX || iy != ownY {
			t.Fatalf("valid unknown local runner did not brake at its own position: (%v,%v)", ix, iy)
		}
	})

	// And a near/far PAIR with the oracle armed: the near runner chases its observed
	// position, the far runner brakes at its own — neither receives the remote ball
	// through the armed native branch.
	t.Run("valid-near-far-with-armed-oracle", func(t *testing.T) {
		sim := mustSim(t, ScheduledLivePassConfig(100), 1)
		near := t044LocalRunner(t, 0.62, 0.50, 4.0)
		near.Pursue = true
		near.IntentX, near.IntentY = placeholder, placeholder
		far := t044LocalRunner(t, 0.10, 0.10, 4.0)
		far.Pursue = true
		far.IntentX, far.IntentY = placeholder, placeholder
		farStartX, farStartY := far.X, far.Y
		sim.SetRunners([]Runner{near, far})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
		sim.Step()
		if ix, iy := t044Intent(sim, 0); ix != 0.60 || iy != 0.50 {
			t.Fatalf("near runner did not chase its observation: (%v,%v)", ix, iy)
		}
		if fx, fy := t044Intent(sim, 1); fx != farStartX || fy != farStartY {
			t.Fatalf("far runner did not brake at its own position: (%v,%v)", fx, fy)
		}
	})
}

// --- 10. Resume contextual validation -------------------------------------

// TestT044ResumeContextValidation proves Resume rejects an inconsistent persisted
// local runner (non-inertial, missing record, disabled scheduler) while the raw
// SetRunners API never errors and instead fails closed at runtime.
func TestT044ResumeContextValidation(t *testing.T) {
	base := func(cfg Config, r Runner) Snapshot {
		return Snapshot{Cfg: cfg, ClockMS: 40, RNG: [4]uint64{1, 2, 3, 4}, Runners: []Runner{r}}
	}
	valid := func() Runner {
		r := t044LocalRunner(t, 0.60, 0.50, 4.0)
		if err := r.Perception.validate(40, 1); err != nil {
			t.Fatalf("fixture record invalid: %v", err)
		}
		return r
	}

	kin := Runner{X: 0.6, Y: 0.5, LocalPursuit: true}
	_ = kin.EnableSensing(4.0)
	if _, err := Resume(base(ScheduledLivePassConfig(100), kin)); !errors.Is(err, ErrLocalPursuitMotion) {
		t.Fatalf("kinematic Resume err = %v, want ErrLocalPursuitMotion", err)
	}

	noRec := InertialRunner(0.6, 0.5, 0.6, 0.5, DefaultMotionParams())
	noRec.LocalPursuit = true
	if _, err := Resume(base(ScheduledLivePassConfig(100), noRec)); !errors.Is(err, ErrLocalPursuitNoPerception) {
		t.Fatalf("missing-record Resume err = %v, want ErrLocalPursuitNoPerception", err)
	}

	if _, err := Resume(base(LivePassConfig(), valid())); !errors.Is(err, ErrLocalPursuitNoScheduler) {
		t.Fatalf("disabled-scheduler Resume err = %v, want ErrLocalPursuitNoScheduler", err)
	}

	if _, err := Resume(base(ScheduledLivePassConfig(100), valid())); err != nil {
		t.Fatalf("valid local runner rejected: %v", err)
	}
}

// --- 11. deterministic complete JSON Resume -------------------------------

// TestT044DeterministicSnapshotAndResume proves the complete snapshot round-trips
// and the resumed tail is bit-identical for an armed local scene, before/at/after
// a due clock and across a real contact (RNG state included).
func TestT044DeterministicSnapshotAndResume(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	setup := func(sim *Sim) {
		a := t044LocalRunner(t, 0.62, 0.50, 30.0)
		b := t044LocalRunner(t, 0.78, 0.42, 30.0)
		sim.SetRunners([]Runner{a, b})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	}
	// A deterministic anchor first.
	x := mustSim(t, cfg, 9)
	setup(x)
	y := mustSim(t, cfg, 9)
	setup(y)
	for x.ClockMS() < 800 {
		x.Step()
		y.Step()
	}
	if !reflect.DeepEqual(x.Snapshot(), y.Snapshot()) {
		t.Fatal("two identical local runs produced different complete snapshots")
	}

	for _, split := range []int{60, 100, 120, 700} {
		straight := mustSim(t, cfg, 9)
		setup(straight)
		for straight.ClockMS() < split {
			straight.Step()
		}
		resumed := t042SnapshotResume(t, straight, "local-split")
		t042Run(t, straight, 3000, nil)
		t042Run(t, resumed, 3000, nil)
		t042AssertTails(t, straight, resumed, "local-resume")
		t.Logf("split@%dms: complete snapshot + local state resumed value-identically", split)
	}

	// Across a real contact/deflection (RNG state included).
	deflect := mustSim(t, cfg, 3)
	deflect.SetRunners([]Runner{t044LocalRunner(t, 0.55, 0.50, 20.0)})
	deflect.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
	rng0 := deflect.RNGState()
	for deflect.ClockMS() < 8000 && countKind(deflect, PassEventDeflect) == 0 {
		deflect.Step()
	}
	if countKind(deflect, PassEventDeflect) == 0 {
		t.Fatal("no deflection occurred")
	}
	if deflect.RNGState() == rng0 {
		t.Fatal("deflection did not consume RNG")
	}
	if !deflect.decisionDirty {
		t.Fatal("deflection left no pending invalidation")
	}
	deflectResumed := t042SnapshotResume(t, deflect, "local-post-deflection")
	t042Run(t, deflect, 12000, nil)
	t042Run(t, deflectResumed, 12000, nil)
	t042AssertTails(t, deflect, deflectResumed, "local-post-deflection")
	t.Logf("post-deflection: RNG + cooldown + local state resumed value-identically")
}

// --- 12. mixed native/local: native unchanged, no loop-order advantage -----

// TestT044MixedNativeLocalPreservesNative proves a far, never-seeing local runner
// does not perturb a native oracle runner, and that swapping their order changes
// neither runner's own trajectory (no loop-order read advantage).
func TestT044MixedNativeLocalPreservesNative(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()
	native := func() Runner { return PursuingRunner(0.60, 0.50, p, 0) }
	local := func() Runner {
		r := t044LocalRunner(t, 0.05, 0.05, 4.0) // far, never sees, brakes in place
		return r
	}
	setup := func(sim *Sim) {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	nativeOnly := captureLiveTrajWith(t, cfg, 1, func(s *Sim) { s.SetRunners([]Runner{native()}); setup(s) }, 2500, nil)
	mixedNL := captureLiveTrajWith(t, cfg, 1, func(s *Sim) { s.SetRunners([]Runner{native(), local()}); setup(s) }, 2500, nil)
	mixedLN := captureLiveTrajWith(t, cfg, 1, func(s *Sim) { s.SetRunners([]Runner{local(), native()}); setup(s) }, 2500, nil)

	// The native runner (index 0 in nativeOnly and mixedNL, index 1 in mixedLN)
	// must be identical across all three.
	nativeIdx := map[*liveTrajectory]int{nativeOnly: 0, mixedNL: 0, mixedLN: 1}
	ref := nativeOnly
	for _, cand := range []*liveTrajectory{mixedNL, mixedLN} {
		ri, ci := nativeIdx[ref], nativeIdx[cand]
		if len(ref.frames) != len(cand.frames) {
			t.Fatalf("frame count %d != %d", len(ref.frames), len(cand.frames))
		}
		for i := range ref.frames {
			if ref.frames[i].xs[ri] != cand.frames[i].xs[ci] || ref.frames[i].ys[ri] != cand.frames[i].ys[ci] {
				t.Fatalf("native runner diverged at frame %d when mixed with a local runner", i)
			}
		}
	}
	// The local runner (index 1 in mixedNL, index 0 in mixedLN) must itself be
	// identical across the two orderings.
	if len(mixedNL.frames) != len(mixedLN.frames) {
		t.Fatal("mixed orderings have different frame counts")
	}
	for i := range mixedNL.frames {
		if mixedNL.frames[i].xs[1] != mixedLN.frames[i].xs[0] || mixedNL.frames[i].ys[1] != mixedLN.frames[i].ys[0] {
			t.Fatalf("local runner trajectory depends on loop order at frame %d", i)
		}
	}
	t.Logf("native runner identical mixed/alone; local runner identical across orderings (no loop-order read advantage)")
}

// --- 13. default/disabled JSON shape unchanged ----------------------------

// TestT044DisabledKeysUnchanged proves a runner without the opt-in serializes no
// localPursuit key, and public copies deep-copy the mutable record.
func TestT044DisabledKeysUnchanged(t *testing.T) {
	raw, err := json.Marshal(Runner{X: 0.5, Y: 0.5})
	if err != nil {
		t.Fatalf("marshal runner: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal runner: %v", err)
	}
	if _, ok := m["localPursuit"]; ok {
		t.Fatalf("runner without the opt-in serialized localPursuit: %s", raw)
	}

	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners([]Runner{t044LocalRunner(t, 0.5, 0.5, 5.0)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.5, Y: 0.5})
	sim.Step()

	got := sim.Runners()
	if !got[0].LocalPursuit {
		t.Fatal("Runners() dropped the local-pursuit flag")
	}
	got[0].Perception.RadiusM = 999
	if sim.runners[0].Perception.RadiusM == 999 {
		t.Fatal("Runners() aliased the live perception pointer")
	}
	caller := []Runner{t044LocalRunner(t, 0.3, 0.3, 6.0)}
	sim.SetRunners(caller)
	caller[0].Perception.RadiusM = 42
	if sim.runners[0].Perception.RadiusM == 42 {
		t.Fatal("SetRunners() aliased the caller's perception pointer")
	}
	snap := sim.Snapshot()
	snap.Runners[0].Perception.X = -123
	if sim.runners[0].Perception.X == -123 {
		t.Fatal("Snapshot() aliased the live perception pointer")
	}
}

// --- 14. default scene snapshots/trajectories remain exact -----------------

// TestT044NoLocalRunnersDefaultUnchanged proves the native scheduler publication
// is UNCHANGED for non-local runners by comparing the REAL publisher against an
// INDEPENDENT reference that reproduces the original free-ball+reaction native
// targets manually (no publishPursuitIntents call). Both runs carry NO local
// runner (LocalPursuit=false); only the reference is disarmed (Pursue=false) so
// its real publisher is inert, and its expected pursuers are re-targeted by hand
// on the SAME scheduler timing condition (NextDecision due / DecisionDirty, and
// every substep on the disabled native path). COMPLETE normalized snapshots are
// compared EVERY substep. This is not a same-function tautology.
func TestT044NoLocalRunnersDefaultUnchanged(t *testing.T) {
	p := DefaultMotionParams()
	scheduled := ScheduledLivePassConfig(100)
	native := LivePassConfig() // IntentCadenceMS == 0

	contactBuild := func(controlError float64) func(*Sim) {
		return func(sim *Sim) {
			sim.SetRunners([]Runner{
				PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
				PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
			})
			sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: controlError, Passer: -1})
		}
	}
	reactionBuild := func(sim *Sim) {
		sim.SetRunners([]Runner{
			PursuingRunner(0.45, 0.50, p, 130),
			PursuingRunner(0.60, 0.50, p, 0),
		})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.50, TargetY: 0.50, SpeedMPS: 8.0, Passer: -1})
	}
	heldBuild := func(sim *Sim) {
		sim.SetRunners([]Runner{
			PursuingRunner(0.55, 0.50, p, 0),
			PursuingRunner(0.65, 0.50+0.05, p, 0),
		})
		sim.SetBall(Ball{Mode: BallHeld, X: 0.55, Y: 0.50})
		sim.contactRunner = 0
		sim.contactMS = 0
	}

	cases := []struct {
		name    string
		cfg     Config
		seed    uint64
		build   func(*Sim)
		horizon int
	}{
		{"scheduled-contact", scheduled, 5, contactBuild(0), 2500},
		{"scheduled-deflect", scheduled, 3, contactBuild(1.0), 4000},
		{"scheduled-reaction", scheduled, 7, reactionBuild, 2500},
		{"scheduled-held", scheduled, 1, heldBuild, 1500},
		{"native-contact", native, 5, contactBuild(0), 1500},
		{"native-deflect", native, 3, contactBuild(1.0), 3000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t044AssertNativeReference(t, c.cfg, c.seed, c.build, c.horizon, c.name)
		})
	}

	// Retained determinism check (same function, two identical runs) — distinct
	// from the reference comparison above.
	a := captureLiveTrajWith(t, scheduled, 5, contactBuild(0), 2500, nil)
	b := captureLiveTrajWith(t, scheduled, 5, contactBuild(0), 2500, nil)
	assertSameTrajectory(t, a, b)

	// A local flag on a runner that never samples must not change the RNG.
	plain := mustSim(t, scheduled, 5)
	contactBuild(0)(plain)
	local := mustSim(t, scheduled, 5)
	contactBuild(0)(local)
	local.SetRunners(append(local.Runners(), t044LocalRunner(t, 0.98, 0.95, 1.0))) // never sees
	for i := 0; i < 300; i++ {
		plain.Step()
		local.Step()
	}
	if plain.RNGState() != local.RNGState() {
		t.Fatal("a never-sampling local runner changed the RNG state")
	}
}

// t044PursueIndices returns the indices the ORIGINAL native publisher targets
// (armed Pursue && MotionInertial).
func t044PursueIndices(sim *Sim) []int {
	var idx []int
	for i := range sim.runners {
		if sim.runners[i].Pursue && sim.runners[i].Motion == MotionInertial {
			idx = append(idx, i)
		}
	}
	return idx
}

// t044PublicationDue mirrors the scheduler's publication condition exactly.
func t044PublicationDue(sim *Sim, start int) bool {
	if sim.Config().IntentCadenceMS == 0 {
		return true // the disabled native path publishes every substep
	}
	return start >= sim.nextDecisionMS || sim.decisionDirty
}

// t044ManualNativeTargets reproduces the ORIGINAL publishPursuitIntents semantics
// WITHOUT calling it (free-ball gate + reaction gate + pre-step ball position).
func t044ManualNativeTargets(sim *Sim, idx []int, start int) {
	if sim.ball.Mode != BallRolling && sim.ball.Mode != BallLoose {
		return
	}
	for _, i := range idx {
		r := &sim.runners[i]
		if start < r.ReactAtMS {
			continue
		}
		r.IntentX, r.IntentY = sim.ball.X, sim.ball.Y
	}
}

// t044NormalizePursue clears the Pursue flag so an ARMED actual runner and a
// DISARMED reference runner compare equal on the fields that carry meaning.
func t044NormalizePursue(s *Snapshot) {
	for i := range s.Runners {
		s.Runners[i].Pursue = false
	}
}

// t044AssertNativeReference runs a scene twice — ACTUAL (real publisher, runners
// Pursue=true) and REFERENCE (runners Pursue=false, expected targets reproduced
// manually) — and compares COMPLETE normalized snapshots EVERY substep.
func t044AssertNativeReference(t *testing.T, cfg Config, seed uint64, build func(*Sim), horizon int, label string) {
	t.Helper()
	actual, err := NewSim(cfg, seed)
	if err != nil {
		t.Fatalf("%s: NewSim: %v", label, err)
	}
	build(actual)
	idx := t044PursueIndices(actual)
	if len(idx) == 0 {
		t.Fatalf("%s: no armed inertial native pursuer in the fixture", label)
	}

	reference, err := NewSim(cfg, seed)
	if err != nil {
		t.Fatalf("%s: NewSim reference: %v", label, err)
	}
	build(reference)
	if len(reference.runners) != len(actual.runners) {
		t.Fatalf("%s: runner count mismatch", label)
	}
	for i := range reference.runners {
		reference.runners[i].Pursue = false // make the real publisher inert
	}

	a0, r0 := actual.Snapshot(), reference.Snapshot()
	t044NormalizePursue(&a0)
	t044NormalizePursue(&r0)
	if !reflect.DeepEqual(a0, r0) {
		t.Fatalf("%s: initial normalized state mismatch", label)
	}

	for actual.ClockMS() < horizon {
		start := reference.ClockMS()
		if t044PublicationDue(reference, start) {
			t044ManualNativeTargets(reference, idx, start)
		}
		actual.Step()
		reference.Step()
		a, r := actual.Snapshot(), reference.Snapshot()
		t044NormalizePursue(&a)
		t044NormalizePursue(&r)
		if !reflect.DeepEqual(a, r) {
			t.Fatalf("%s @%dms: the real native publisher diverged from the independent reference", label, actual.ClockMS())
		}
	}
	t.Logf("%s: real native publisher == independent reference every substep (%d ms)", label, horizon)
}

// --- 15. complete JSON Resume for the unknown and stale states ------------

// TestT044ResumeUnknownAndStaleTails proves the complete JSON Resume reproduces
// the tail bit-identically for a local runner in the UNKNOWN state (never saw the
// ball) and the STALE state (ball left the sensing circle).
func TestT044ResumeUnknownAndStaleTails(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	// Unknown: a far local runner that never sees the ball.
	unk := mustSim(t, cfg, 1)
	ur := t044LocalRunner(t, 0.10, 0.10, 4.0)
	ur.IntentX, ur.IntentY = 0.95, 0.95
	unk.SetRunners([]Runner{ur})
	unk.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 2.0})
	for unk.ClockMS() < 500 {
		unk.Step()
	}
	if o := t044Perception(t, unk, 0); o.Seen {
		t.Fatalf("unknown fixture saw the ball: %+v", o)
	}
	unkSplit := unk.ClockMS()
	unkResumed := t042SnapshotResume(t, unk, "unknown")
	t042Run(t, unk, 3000, nil)
	t042Run(t, unkResumed, 3000, nil)
	t042AssertTails(t, unk, unkResumed, "unknown")

	// Stale: watcher 1.5 m off the lane; ride until the ball is actually outside
	// the 2 m circle at a DUE publication (pre-step distance > radius), then
	// snapshot/resume. The found flag and the post-loop asserts are mandatory so a
	// never-reached fixture cannot pass vacuously.
	stale := mustSim(t, cfg, 1)
	sr := t044LocalRunner(t, 0.50, 0.50+1.5/PitchWidthM, 2.0)
	sr.ReachM = 0.05
	stale.SetRunners([]Runner{sr})
	stale.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	found := false
	var staleSplit int
	var splitPreDist float64
	for stale.ClockMS() < 12000 {
		start := stale.ClockMS()
		o := stale.runners[0].Perception
		preDist := DistM(stale.ball.X, stale.ball.Y, stale.runners[0].X, stale.runners[0].Y)
		if start%cfg.IntentCadenceMS == 0 && o != nil && o.Seen && o.Freshness(start) == ObservationStale && preDist > 2.0 {
			found = true
			staleSplit = start
			splitPreDist = preDist
			break
		}
		stale.Step()
	}
	if !found {
		t.Fatal("stale fixture never reached a DUE clock outside the radius while Seen")
	}
	if o := stale.runners[0].Perception; o == nil || !o.Seen {
		t.Fatal("stale fixture record is not Seen; cannot be a stale fixture")
	}
	staleResumed := t042SnapshotResume(t, stale, "stale")
	t042Run(t, stale, 12000, nil)
	t042Run(t, staleResumed, 12000, nil)
	t042AssertTails(t, stale, staleResumed, "stale")
	t.Logf("unknown @%d ms (never Seen) and stale @%d ms (Seen, pre-step dist %.3f m > 2.0): complete snapshots + tails resumed value-identically",
		unkSplit, staleSplit, splitPreDist)
}

// --- 16. targeted local-vs-oracle divergence (documented, not asserted equal)

// TestT044LocalVsOracleDivergence documents the FIRST targeted moment where the
// local policy and the native oracle differ on the same scene: the INITIAL UNKNOWN
// publication (the local runner has never seen the far ball, so it brakes at its own
// position while the oracle chases the true ball). It is explicitly NOT the
// stale-exit moment (that case is covered by TestT044StaleAtDueRetainsPriorTarget).
// No equivalence is claimed or asserted.
func TestT044LocalVsOracleDivergence(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	localSim := mustSim(t, cfg, 1)
	lr := t044LocalRunner(t, 0.50, 0.50+1.5/PitchWidthM, 2.0)
	lr.ReachM = 0.05
	localSim.SetRunners([]Runner{lr})
	localSim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})

	oracleSim := mustSim(t, cfg, 1)
	oracleSim.SetRunners([]Runner{PursuingRunner(0.50, 0.50+1.5/PitchWidthM, DefaultMotionParams(), 0)})
	oracleSim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})

	divergedAt := -1
	var lix, orx, preOwnX, preBallX float64
	unknownAtDivergence := false
	for localSim.ClockMS() < 3000 {
		preOwnX = localSim.runners[0].X
		preBallX = oracleSim.ball.X
		localSim.Step()
		oracleSim.Step()
		li, _ := t044Intent(localSim, 0)
		oi, _ := t044Intent(oracleSim, 0)
		if li != oi {
			divergedAt = localSim.ClockMS()
			lix, orx = li, oi
			o := localSim.runners[0].Perception
			unknownAtDivergence = o == nil || !o.Seen
			break
		}
	}
	if divergedAt < 0 {
		t.Fatal("local and oracle intents never diverged; the test is vacuous")
	}
	if !unknownAtDivergence {
		t.Fatal("the first divergence was not the initial UNKNOWN brake; the fixture labels it wrong")
	}
	if lix != preOwnX {
		t.Fatalf("local UNKNOWN target %.5f != its own pre-step position %.5f", lix, preOwnX)
	}
	if orx != preBallX {
		t.Fatalf("oracle target %.5f != the pre-step ball position %.5f", orx, preBallX)
	}
	t.Logf("UNKNOWN initial divergence @%d ms: local brakes at own pre-step (%.4f); oracle chases the true ball (%.4f); different models, no equivalence claimed",
		divergedAt, lix, orx)
}
