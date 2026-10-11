package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// runUntilHeld steps (quiet, no sample allocation) until the ball is controlled
// or the horizon is reached.
func runUntilHeld(sim *Sim, endMS int) {
	for sim.ClockMS() < endMS && sim.Ball().Mode != BallHeld {
		sim.StepQuiet()
	}
}

// controlClock returns the clock of the first control event, or -1.
func controlClock(sim *Sim) int {
	for _, e := range sim.Events() {
		if e.Kind == PassEventControl {
			return e.ClockMS
		}
	}
	return -1
}

// livePassScene releases a straight, jitter-free pass from (0.30,0.50) toward a
// stationary receiver placed along +x at the given metres, and returns the
// receiver's normalized x so callers can probe it.
func livePassScene(t *testing.T, seed uint64, receiverM float64, recvVXM float64) (*Sim, float64) {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	rx := 0.30 + receiverM/PitchLengthM
	sim.SetRunners([]Runner{{X: rx, Y: 0.50, VXM: recvVXM, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50,
		TargetX: rx, TargetY: 0.50,
		SpeedMPS: 16.0, Passer: -1,
	})
	return sim, rx
}

// TestLivePassNoOwnershipAtRelease: a release decides nothing. Ownership is
// unassigned, the ball is simply rolling, and the first event is the release.
func TestLivePassNoOwnershipAtRelease(t *testing.T) {
	sim, _ := livePassScene(t, 1, 20.0, 0)
	if sim.Ball().Mode != BallRolling {
		t.Fatalf("ball mode after release = %v, want rolling", sim.Ball().Mode)
	}
	if ms, r := sim.FirstContact(); ms != -1 || r != -1 {
		t.Fatalf("ownership assigned at release: (%d,%d), want (-1,-1)", ms, r)
	}
	ev := sim.Events()
	if len(ev) == 0 || ev[0].Kind != PassEventRelease || ev[0].ClockMS != 0 {
		t.Fatalf("first event = %+v, want release at clock 0", ev)
	}
	if sim.Ball().VXM <= 0 {
		t.Fatalf("released ball has no forward velocity: %+v", sim.Ball())
	}
}

// TestLivePassNoInstantReception: control cannot happen on the release substep;
// the ball must actually travel before it is controlled.
func TestLivePassNoInstantReception(t *testing.T) {
	sim, _ := livePassScene(t, 1, 20.0, 0)
	start := sim.Ball()
	sim.StepQuiet()
	if sim.Ball().Mode == BallHeld {
		t.Fatal("ball controlled on the very first substep (instant reception)")
	}
	if ms, _ := sim.FirstContact(); ms != -1 {
		t.Fatalf("contact recorded during first substep: %d", ms)
	}
	runUntilHeld(sim, 4000)
	cc := controlClock(sim)
	if cc <= 0 {
		t.Fatalf("control clock = %d, want > 0", cc)
	}
	// The ball must have moved a meaningful distance before control.
	travelled := DistM(start.X, start.Y, sim.Ball().X, sim.Ball().Y)
	if travelled < 5.0 {
		t.Fatalf("ball travelled only %.2f m before control", travelled)
	}
}

// TestLivePassTravelTimeScalesWithLengthAndSpeed: a longer pass takes longer, a
// faster pass arrives sooner.
func TestLivePassTravelTimeScalesWithLengthAndSpeed(t *testing.T) {
	near, _ := livePassScene(t, 1, 8.0, 0)
	far, _ := livePassScene(t, 1, 20.0, 0)
	runUntilHeld(near, 8000)
	runUntilHeld(far, 8000)
	nearMS, farMS := controlClock(near), controlClock(far)
	if nearMS < 0 || farMS < 0 {
		t.Fatalf("reach failed: near=%d far=%d", nearMS, farMS)
	}
	if farMS <= nearMS {
		t.Fatalf("far pass (%d ms) not slower than near (%d ms)", farMS, nearMS)
	}

	slow := mustSim(t, LivePassConfig(), 1)
	slow.SetRunners([]Runner{{X: 0.30 + 15.0/PitchLengthM, Y: 0.50, ReachM: ControlRadiusM}})
	slow.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.30 + 15.0/PitchLengthM, TargetY: 0.50, SpeedMPS: 10.0, Passer: -1})
	fast := mustSim(t, LivePassConfig(), 1)
	fast.SetRunners([]Runner{{X: 0.30 + 15.0/PitchLengthM, Y: 0.50, ReachM: ControlRadiusM}})
	fast.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.30 + 15.0/PitchLengthM, TargetY: 0.50, SpeedMPS: 16.0, Passer: -1})
	runUntilHeld(slow, 8000)
	runUntilHeld(fast, 8000)
	if controlClock(fast) <= 0 || controlClock(slow) <= 0 {
		t.Fatalf("speed reach failed: slow=%d fast=%d", controlClock(slow), controlClock(fast))
	}
	if controlClock(fast) >= controlClock(slow) {
		t.Fatalf("faster pass (%d ms) not sooner than slow (%d ms)", controlClock(fast), controlClock(slow))
	}
}

// TestLivePassMovingReceiverChangesControl: a receiver running toward the ball
// meets it sooner than one standing still, which meets it sooner than one
// running away (the receiver's motion is physical, not decided).
func TestLivePassMovingReceiverChangesControl(t *testing.T) {
	still, _ := livePassScene(t, 1, 15.0, 0)
	toward, _ := livePassScene(t, 1, 15.0, -6.0)
	away, _ := livePassScene(t, 1, 15.0, 6.0)
	runUntilHeld(still, 8000)
	runUntilHeld(toward, 8000)
	runUntilHeld(away, 8000)
	s, tw, aw := controlClock(still), controlClock(toward), controlClock(away)
	if s < 0 || tw < 0 || aw < 0 {
		t.Fatalf("reach failed: still=%d toward=%d away=%d", s, tw, aw)
	}
	if tw >= s || s >= aw {
		t.Fatalf("moving receiver did not change control order: toward=%d still=%d away=%d", tw, s, aw)
	}
}

// TestLivePassIntendedReceiverNotWinner: the aim points at runner 0 far down the
// pitch, but runner 1 on the early path physically reaches the ball first and
// controls it — the intended receiver is an intent, not a selected winner.
func TestLivePassIntendedReceiverNotWinner(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	aimX := 0.30 + 25.0/PitchLengthM
	sim.SetRunners([]Runner{
		{X: aimX, Y: 0.50, ReachM: ControlRadiusM},                    // intended receiver (0)
		{X: 0.30 + 7.0/PitchLengthM, Y: 0.50, ReachM: ControlRadiusM}, // intervening runner (1)
	})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: aimX, TargetY: 0.50, SpeedMPS: 16.0, Passer: -1})
	runUntilHeld(sim, 8000)
	_, runner := sim.FirstContact()
	if runner != 1 {
		t.Fatalf("controller = %d, want 1 (the physically-reached runner, not the aim)", runner)
	}
}

// TestLivePassShortStopStaysLooseNotDead: a weak pass that stops short becomes a
// LIVE collectable loose ball, never a dead restart spot. It is still
// collectable afterwards, and the legacy mode still dies (mode distinction).
func TestLivePassShortStopStaysLooseNotDead(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.40, TargetY: 0.50, SpeedMPS: 4.0, Passer: -1})
	runUntilHeld(sim, 12000)
	if got := sim.Ball().Mode; got != BallLoose {
		t.Fatalf("stopped short ball mode = %v, want loose (live)", got)
	}
	if controlClock(sim) != -1 {
		t.Fatalf("a short-stop loose ball should not be controlled without a runner near it")
	}
	sawStop, sawOut := false, false
	for _, e := range sim.Events() {
		switch e.Kind {
		case PassEventStop:
			sawStop = true
		case PassEventOut:
			sawOut = true
		}
	}
	if !sawStop || sawOut {
		t.Fatalf("expected a stop event and no out event (stop=%v out=%v)", sawStop, sawOut)
	}

	// The loose ball stays live and collectable: a runner arriving can control it.
	b := sim.Ball()
	sim.SetRunners([]Runner{{X: b.X, Y: b.Y, ReachM: ControlRadiusM}})
	sim.StepQuiet()
	if sim.Ball().Mode != BallHeld {
		t.Fatal("a stationary loose ball was not collectable")
	}

	// Mode distinction: the SAME weak ball in legacy mode dies, not loose.
	legacy := mustSim(t, DefaultConfig(), 1)
	legacy.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 4.0})
	for legacy.ClockMS() < 12000 && legacy.Ball().Mode == BallRolling {
		legacy.StepQuiet()
	}
	if legacy.Ball().Mode != BallDead {
		t.Fatalf("legacy stopped ball mode = %v, want dead", legacy.Ball().Mode)
	}
}

// TestLivePassOutOfPlayIsDead: a pass aimed off the pitch dies out of play with
// an out event and is NOT a collectable loose ball.
func TestLivePassOutOfPlayIsDead(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.50, TargetY: -0.30, SpeedMPS: 15.0, Passer: -1})
	for sim.ClockMS() < 12000 && sim.Ball().Mode != BallOut {
		sim.StepQuiet()
	}
	if sim.Ball().Mode != BallOut {
		t.Fatalf("off-pitch ball mode = %v, want out", sim.Ball().Mode)
	}
	sawOut, sawStop := false, false
	for _, e := range sim.Events() {
		switch e.Kind {
		case PassEventOut:
			sawOut = true
		case PassEventStop:
			sawStop = true
		}
	}
	if !sawOut || sawStop {
		t.Fatalf("expected an out event and no stop event (out=%v stop=%v)", sawOut, sawStop)
	}
}

// TestLivePassSeededExecutionError: the same seed reproduces the release exactly,
// and different seeds produce different execution errors (deterministic variance).
func TestLivePassSeededExecutionError(t *testing.T) {
	releaseVel := func(seed uint64) (float64, float64) {
		sim := mustSim(t, LivePassConfig(), seed)
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.70, TargetY: 0.50, SpeedMPS: 15.0, DirectionJitterMPS: 3.0, WeightJitterFrac: 0.15, Passer: -1})
		b := sim.Ball()
		return b.VXM, b.VYM
	}
	a1x, a1y := releaseVel(7)
	a2x, a2y := releaseVel(7)
	if a1x != a2x || a1y != a2y {
		t.Fatalf("same seed produced different release: (%v,%v) vs (%v,%v)", a1x, a1y, a2x, a2y)
	}

	seen := map[[2]float64]bool{}
	for seed := uint64(1); seed <= 10; seed++ {
		vx, vy := releaseVel(seed)
		seen[[2]float64{vx, vy}] = true
	}
	if len(seen) < 5 {
		t.Fatalf("seeded execution error did not vary across seeds: only %d distinct releases", len(seen))
	}
}

// TestLivePassRepeatedReleaseResetsContact: a second release clears the stale
// control record and starts a fresh, separately-timed episode.
func TestLivePassRepeatedReleaseResetsContact(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 5)
	rx := 0.30 + 12.0/PitchLengthM
	sim.SetRunners([]Runner{{X: rx, Y: 0.50, ReachM: ControlRadiusM}})
	p := PassParams{StartX: 0.30, StartY: 0.50, TargetX: rx, TargetY: 0.50, SpeedMPS: 15.0, Passer: -1}
	sim.ReleasePass(p)
	runUntilHeld(sim, 8000)
	first, firstRunner := sim.FirstContact()
	if first < 0 {
		t.Fatal("first pass never controlled")
	}
	sim.ReleasePass(p)
	if ms, r := sim.FirstContact(); ms != -1 || r != -1 {
		t.Fatalf("second release left stale contact (%d,%d)", ms, r)
	}
	if sim.Ball().Mode != BallRolling {
		t.Fatal("second release did not put the ball back in flight")
	}
	runUntilHeld(sim, 16000)
	second, _ := sim.FirstContact()
	if second <= first {
		t.Fatalf("second control (%d) not after the first (%d)", second, first)
	}
	if second == first && firstRunner >= 0 {
		t.Fatal("second release reused the first episode")
	}
}

// TestLivePassPasserImmunity: the releasing player cannot immediately re-collect
// the ball; a very soft pass that stays within reach is only collected after the
// immunity cooldown, not before.
func TestLivePassPasserImmunity(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.SetRunners([]Runner{{X: 0.50, Y: 0.50, ReachM: ControlRadiusM}})
	sim.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.52, TargetY: 0.50, SpeedMPS: 1.0, Passer: 0})
	// The pass never leaves the passer's reach, so without immunity it would be
	// collected almost immediately.
	runUntilHeld(sim, 4000)
	cc := controlClock(sim)
	if cc < 0 {
		t.Fatal("soft pass was never collected after immunity expired")
	}
	if cc < PasserImmunityMS {
		t.Fatalf("passer re-collected at %d ms, before immunity %d ms", cc, PasserImmunityMS)
	}
	if _, r := sim.FirstContact(); r != 0 {
		t.Fatalf("controller = %d, want the passer (0)", r)
	}
}

// TestLivePassPasserImmunityPersistsAcrossResume: a mid-flight Resume 100 ms in
// must keep enforcing the passer immunity deadline (persisted state).
func TestLivePassPasserImmunityPersistsAcrossResume(t *testing.T) {
	straight := mustSim(t, LivePassConfig(), 3)
	straight.SetRunners([]Runner{{X: 0.50, Y: 0.50, ReachM: ControlRadiusM}})
	straight.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.52, TargetY: 0.50, SpeedMPS: 1.0, Passer: 0})
	for straight.ClockMS() < 100 {
		straight.StepQuiet()
	}
	resumed, err := Resume(straight.Snapshot())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	runUntilHeld(straight, 4000)
	runUntilHeld(resumed, 4000)
	if controlClock(resumed) != controlClock(straight) {
		t.Fatalf("resumed immunity diverged: %d vs %d", controlClock(resumed), controlClock(straight))
	}
	if controlClock(resumed) < PasserImmunityMS {
		t.Fatalf("resumed run collected at %d ms, before immunity %d ms", controlClock(resumed), PasserImmunityMS)
	}
}

// TestLivePassEventsShareIntegerClock: release, path and control events are
// distinct, ordered, and all land on the same integer substep grid.
func TestLivePassEventsShareIntegerClock(t *testing.T) {
	sim, _ := livePassScene(t, 1, 18.0, 0)
	runUntilHeld(sim, 8000)
	ev := sim.Events()
	if len(ev) < 2 {
		t.Fatalf("expected at least release+control events, got %d", len(ev))
	}
	if ev[0].Kind != PassEventRelease || ev[0].ClockMS != 0 {
		t.Fatalf("first event = %+v, want release at 0", ev[0])
	}
	last := ev[len(ev)-1]
	if last.Kind != PassEventControl {
		t.Fatalf("last event = %+v, want control", last)
	}
	prev := -1
	for i, e := range ev {
		if e.Seq != i {
			t.Fatalf("event %d Seq = %d, want %d", i, e.Seq, i)
		}
		if e.ClockMS%SubstepMS != 0 {
			t.Fatalf("event %d clock %d not on the %d ms grid", i, e.ClockMS, SubstepMS)
		}
		if e.ClockMS < prev {
			t.Fatalf("event %d clock %d went backwards from %d", i, e.ClockMS, prev)
		}
		prev = e.ClockMS
	}
	if last.ClockMS <= ev[0].ClockMS {
		t.Fatalf("control (%d ms) not after release (%d ms)", last.ClockMS, ev[0].ClockMS)
	}
}

// TestLivePassNoBallWarpOnReceipt: control preserves the ball's physical
// position — the ball does not jump to the controller's feet. The per-substep
// displacement is bounded by the physical speed, the held offset is within
// reach, and the held ball then follows the owner continuously.
func TestLivePassNoBallWarpOnReceipt(t *testing.T) {
	sim, _ := livePassScene(t, 1, 15.0, -1.5) // receiver runs toward the ball
	dt := float64(SubstepMS) / 1000
	var (
		prev Ball
		cur  Ball
	)
	controlled := false
	for i := 0; i < 4000 && !controlled; i++ {
		prev = sim.Ball()
		sim.StepQuiet()
		cur = sim.Ball()
		if cur.Mode == BallHeld && prev.Mode == BallRolling {
			controlled = true
			prevSpeed := math.Sqrt(prev.VXM*prev.VXM + prev.VYM*prev.VYM)
			if disp := DistM(prev.X, prev.Y, cur.X, cur.Y); disp > prevSpeed*dt+1e-9 {
				t.Fatalf("ball warped on receipt: moved %.4f m in one %.0f ms step (max %.4f)", disp, dt*1000, prevSpeed*dt)
			}
		}
	}
	if !controlled {
		t.Fatal("pass was never controlled")
	}
	_, cr := sim.FirstContact()
	owner := sim.Runners()[cr]
	off := DistM(cur.X, cur.Y, owner.X, owner.Y)
	if off > ControlRadiusM+1e-9 {
		t.Fatalf("held offset %.4f m exceeds reach %.4f m", off, ControlRadiusM)
	}
	// The held ball must follow the owner at a fixed world offset.
	ox, oy := cur.X-owner.X, cur.Y-owner.Y
	for i := 0; i < 30; i++ {
		sim.StepQuiet()
		b := sim.Ball()
		o := sim.Runners()[cr]
		if math.Abs((b.X-o.X)-ox) > 1e-12 || math.Abs((b.Y-o.Y)-oy) > 1e-12 {
			t.Fatalf("held ball did not follow the owner continuously: offset drift")
		}
	}
}

// assertTerminalEventMatchesSample pins the same-clock contract for a terminal
// ball event: the event must be stamped at the END-step clock, i.e. the exact
// clock of the authoritative sample that first shows the transitioned mode. It
// also asserts the whole event log is on-grid and non-decreasing in clock.
func assertTerminalEventMatchesSample(t *testing.T, sim *Sim, kind PassEventKind, mode BallMode) {
	t.Helper()
	evClock := -1
	for _, e := range sim.Events() {
		if e.Kind == kind {
			evClock = e.ClockMS
		}
	}
	if evClock < 0 {
		t.Fatalf("no %s event was emitted", kind)
	}
	smpClock := -1
	for _, s := range sim.Samples() {
		if s.Mode == mode && smpClock < 0 {
			smpClock = s.ClockMS
		}
	}
	if smpClock < 0 {
		t.Fatalf("no %s sample was recorded", mode)
	}
	if evClock != smpClock {
		t.Fatalf("%s event clock %d != %s sample clock %d (half-step skew)",
			kind, evClock, mode, smpClock)
	}
	prev := -1
	for i, e := range sim.Events() {
		if e.Seq != i {
			t.Fatalf("event %d Seq = %d, want %d", i, e.Seq, i)
		}
		if e.ClockMS%SubstepMS != 0 {
			t.Fatalf("event %d clock %d is not on the %d ms grid", i, e.ClockMS, SubstepMS)
		}
		if e.ClockMS < prev {
			t.Fatalf("event clock went backwards: %d after %d", e.ClockMS, prev)
		}
		prev = e.ClockMS
	}
}

// TestLivePassTerminalEventSharesEndStepClock: stop and out events describe the
// post-integration ball state, so their timestamp must equal the authoritative
// sample timestamp of the transition (a regression for the old pre-advance
// stamp, which dated both events one substep early).
func TestLivePassTerminalEventSharesEndStepClock(t *testing.T) {
	stop := mustSim(t, LivePassConfig(), 1)
	stop.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.40, TargetY: 0.50, SpeedMPS: 4.0, Passer: -1})
	for stop.ClockMS() < 12000 && stop.Ball().Mode == BallRolling {
		stop.Step()
	}
	assertTerminalEventMatchesSample(t, stop, PassEventStop, BallLoose)

	out := mustSim(t, LivePassConfig(), 1)
	out.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.50, TargetY: -0.30, SpeedMPS: 15.0, Passer: -1})
	for out.ClockMS() < 12000 && out.Ball().Mode != BallOut {
		out.Step()
	}
	assertTerminalEventMatchesSample(t, out, PassEventOut, BallOut)
}

// TestLivePassMidflightSnapshotResume: a snapshot taken while the ball is in
// flight, JSON round-tripped, resumes bit-identically for the remaining travel,
// events, samples, control and RNG state.
func TestLivePassMidflightSnapshotResume(t *testing.T) {
	build := func() *Sim {
		sim := mustSim(t, LivePassConfig(), 11)
		rx := 0.30 + 18.0/PitchLengthM
		sim.SetRunners([]Runner{{X: rx, Y: 0.50, ReachM: ControlRadiusM}})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: rx, TargetY: 0.50, SpeedMPS: 15.0, DirectionJitterMPS: 1.0, WeightJitterFrac: 0.05, Passer: -1})
		return sim
	}
	straight := build()
	for straight.ClockMS() < 300 {
		straight.Step()
	}
	if straight.Ball().Mode != BallRolling {
		t.Fatalf("snapshot taken after control; mode = %v", straight.Ball().Mode)
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

	// Both paths record samples, so the derived sample stream can be compared.
	for straight.ClockMS() < 8000 && straight.Ball().Mode != BallHeld {
		straight.Step()
	}
	for resumed.ClockMS() < 8000 && resumed.Ball().Mode != BallHeld {
		resumed.Step()
	}

	sm, sr := straight.FirstContact()
	rm, rr := resumed.FirstContact()
	if sm != rm || sr != rr {
		t.Fatalf("resumed control (%d,%d) != straight (%d,%d)", rm, rr, sm, sr)
	}
	if straight.RNGState() != resumed.RNGState() {
		t.Fatal("resumed RNG state diverged")
	}

	allEv := straight.Events()
	headEv := len(allEv) - len(resumed.Events())
	if headEv < 0 {
		t.Fatal("resumed emitted more events than the straight run")
	}
	if !reflect.DeepEqual(allEv[headEv:], resumed.Events()) {
		t.Fatal("resumed event tail differs from the straight run")
	}

	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 {
		t.Fatal("resumed produced more samples than the straight run")
	}
	if !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatal("resumed sample tail differs from the straight run")
	}
	if controlClock(straight) != controlClock(resumed) {
		t.Fatalf("control clock diverged: %d vs %d", controlClock(straight), controlClock(resumed))
	}
}
