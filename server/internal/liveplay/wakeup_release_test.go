package liveplay

import "testing"

// t045Watcher builds a moving, local, Unknown watcher: placeholder intent far
// away, initial velocity, tiny reach (never controls the studied ball).
func t045Watcher(x, y, radius float64, reactMS int) Runner {
	r := InertialRunner(x, y, 0.95, 0.50, DefaultMotionParams())
	r.VXM = 5.0
	r.ReachM = 0.05
	r.ReactMS = reactMS
	r.ReactAtMS = reactMS
	if err := r.EnableLocalPursuit(radius); err != nil {
		panic(err)
	}
	return r
}

// t045ObsEqual asserts runner idx has an identical observation record at EVERY
// step of two aligned traces (the local history), independent of intent.
func t045ObsEqual(t *testing.T, a, b *t045Trace, idx int, label string) {
	t.Helper()
	if len(a.frames) != len(b.frames) {
		t.Fatalf("%s: frame count %d vs %d", label, len(a.frames), len(b.frames))
	}
	for i := range a.frames {
		fa, fb := a.frames[i], b.frames[i]
		if fa.hasObs[idx] != fb.hasObs[idx] || fa.obs[idx] != fb.obs[idx] {
			t.Fatalf("%s @%d: runner %d observation differs: has=%v %+v vs has=%v %+v",
				label, fa.clock, idx, fa.hasObs[idx], fa.obs[idx], fb.hasObs[idx], fb.obs[idx])
		}
	}
}

// t045ObsIntentEqual asserts an identical observation record AND intent at EVERY step.
func t045ObsIntentEqual(t *testing.T, a, b *t045Trace, idx int, label string) {
	t.Helper()
	t045ObsEqual(t, a, b, idx, label)
	for i := range a.frames {
		fa, fb := a.frames[i], b.frames[i]
		if fa.ix[idx] != fb.ix[idx] || fa.iy[idx] != fb.iy[idx] {
			t.Fatalf("%s @%d: runner %d intent differs: (%v,%v) vs (%v,%v)",
				label, fa.clock, idx, fa.ix[idx], fa.iy[idx], fb.ix[idx], fb.iy[idx])
		}
	}
}

// t045IntentAt returns runner idx's IntentX at the first frame with the clock.
func t045IntentAt(tr *t045Trace, idx, clock int) float64 {
	for i := range tr.frames {
		if tr.frames[i].clock == clock {
			return tr.frames[i].ix[idx]
		}
	}
	return -1
}

// t045UnknownMovingFixture is one local watcher and a far stationary loose ball.
func t045UnknownMovingFixture(sim *Sim) {
	sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, 2.0, 0)})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
}

// t045RemoteRelease60 injects a remote off-lattice release at clock 60 (while the
// unknown runner is still moving, before it brakes to rest).
func t045RemoteRelease60(s *Sim, clock int) {
	if clock == 60 {
		s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.50, TargetX: 0.30, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	}
}

// TestT045SharedRemoteReleaseLeak is the shared-baseline mandatory scene: the
// unmodified scheduler with vs without a remote off-lattice release at 60 ms. The
// local RECORD history is identical (the ball never enters the radius), but the
// shared global dirty flag leaks: an extra publication at 60 and a changed intent
// at the END frame of the pre-step-60 step (frame clock 80). NOTE the frame at the
// pre-step clock itself reflects the PREVIOUS step.
func TestT045SharedRemoteReleaseLeak(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	with := t045MustHarness(t, cfg, 1, t045Shared)
	without := t045MustHarness(t, cfg, 1, t045Shared)
	t045UnknownMovingFixture(with.sim)
	t045UnknownMovingFixture(without.sim)
	tw := with.run(300, t045RemoteRelease60)
	two := without.run(300, nil)

	t045ObsEqual(t, tw, two, 0, "shared with/without remote release")
	if !containsInt(tw.pubClocks, 60) {
		t.Fatalf("shared did not publish at the off-lattice release 60: %v", tw.pubClocks)
	}
	if containsInt(two.pubClocks, 60) {
		t.Fatalf("baseline published at 60 without a release: %v", two.pubClocks)
	}
	iWith := t045IntentAt(tw, 0, 80)
	iWo := t045IntentAt(two, 0, 80)
	if iWith == iWo {
		t.Fatalf("shared leak not realised: intent at frame 80 equal (%.6f)", iWith)
	}
	t.Logf("shared remote release @60ms: extra publication at 60; frame-80 intent with=%.6f without=%.6f; record history identical", iWith, iWo)
}

// TestT045PerRunnerRemoteReleaseIsolation is the paired SAME-controller
// (per-runner) with/without the remote release at 60 ms: the observation record,
// the publication clocks AND the intent are identical every step — the remote
// release leaks nothing per-runner. The release's reaction-deadline reset is
// audited separately (it changes persisted ReactAt even with ReactMS=0).
func TestT045PerRunnerRemoteReleaseIsolation(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	with := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	without := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	t045UnknownMovingFixture(with.sim)
	t045UnknownMovingFixture(without.sim)
	tw := with.run(300, t045RemoteRelease60)
	two := without.run(300, nil)

	t045ObsIntentEqual(t, tw, two, 0, "per-runner with/without remote release")
	if got := t045AttemptClocks(tw.harnessPubs, 0, "local-wake"); len(got) != 0 {
		t.Fatalf("per-runner leaked a local wake from the remote release: %v", got)
	}
	if got := t045DedupClocks(tw.harnessPubs); len(got) != len(t045DedupClocks(two.harnessPubs)) {
		t.Fatalf("publication clocks differ: %v vs %v", got, t045DedupClocks(two.harnessPubs))
	}
	t.Logf("per-runner: remote release @60ms -> NO watcher wake; record history + publication clocks + intents identical every step")
}

// TestT045UnionRejectedTwoWatcher keeps the REJECTED global-union gating in the
// correctly-scoped two-watcher near/far fixture: a release visible ONLY to the near
// runner. Union (OR of local pending) wakes the far runner too; per-runner wakes
// only the near runner. A remote event would NOT differentiate them, which is why
// the union rejection belongs here.
func TestT045UnionRejectedTwoWatcher(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	mid := func(s *Sim, clock int) {
		if clock == 60 {
			// Off-lane (1.36 m) so it is never controlled; inside near's 2 m circle.
			s.ReleasePass(PassParams{StartX: 0.51, StartY: 0.52, TargetX: 0.52, TargetY: 0.52, SpeedMPS: 2.0, Passer: -1})
		}
	}
	build := func(sim *Sim) {
		sim.SetRunners([]Runner{
			t045Watcher(0.50, 0.50, 2.0, 0), // near
			t045Watcher(0.30, 0.50, 2.0, 0), // far (~22 m from the event)
		})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.90})
	}

	per := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	build(per.sim)
	tp := per.run(300, mid)
	uni := t045MustHarness(t, cfg, 1, t045GatedUnion)
	build(uni.sim)
	tu := uni.run(300, mid)

	if !containsInt(t045AttemptClocks(tp.harnessPubs, 0, "local-wake"), 60) {
		t.Fatalf("per-runner did not wake the near runner at 60: %v", t045AttemptClocks(tp.harnessPubs, 0, "local-wake"))
	}
	if got := t045AttemptClocks(tp.harnessPubs, 1, "local-wake"); len(got) != 0 {
		t.Fatalf("per-runner woken the far runner from a near-only event: %v", got)
	}
	if !containsInt(t045AttemptClocks(tu.harnessPubs, 1, "union-wake"), 60) {
		t.Fatalf("union did not (wrongly) wake the far runner: %v", t045AttemptClocks(tu.harnessPubs, 1, "union-wake"))
	}
	perFar := t045IntentAt(tp, 1, 80)
	uniFar := t045IntentAt(tu, 1, 80)
	if perFar == uniFar {
		t.Fatalf("union and per-runner far intent at frame 80 are equal (%.6f); union leak not demonstrated", perFar)
	}
	t.Logf("union (REJECTED) wakes far @60 (frame-80 far intent %.6f vs per-runner %.6f); per-runner isolates", uniFar, perFar)
}

// TestT045ReleaseReactionResetSideChannel audits the SEPARATE side channel that
// ReleasePass resets EVERY runner's reaction deadline to clock+ReactMS (local and
// native alike). It is NOT fixed test-side and is reported explicitly: physical
// gate timing differs even though per-runner position leakage is isolated.
func TestT045ReleaseReactionResetSideChannel(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045Shared)
	local := t045Watcher(0.50, 0.50, 2.0, 130)
	native := PursuingRunner(0.80, 0.50, DefaultMotionParams(), 130)
	native.ReactAtMS = 130
	h.sim.SetRunners([]Runner{local, native})
	h.sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
	t045AdvanceTo(h, 200)
	before := []int{h.sim.runners[0].ReactAtMS, h.sim.runners[1].ReactAtMS}
	h.sim.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
	after := []int{h.sim.runners[0].ReactAtMS, h.sim.runners[1].ReactAtMS}
	if after[0] != 330 || after[1] != 330 {
		t.Fatalf("release did not reset both reaction deadlines to 200+130: before=%v after=%v", before, after)
	}

	zero := t045MustHarness(t, cfg, 1, t045Shared)
	zero.sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, 2.0, 0)})
	zero.sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
	t045AdvanceTo(zero, 200)
	zero.sim.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
	if got := zero.sim.runners[0].ReactAtMS; got != 200 {
		t.Fatalf("ReactMS=0 release set ReactAtMS=%d, want 200", got)
	}
	t.Logf("reaction-reset side channel: ReleasePass@200ms moved ReactAtMS %v -> %v for ALL runners (local+native); ReactMS=0 -> 200 (inert)", before, after)
}

// TestT045RealSecondRelease builds a REAL first ReleasePass at clock 0 and a REAL
// second off-lattice ReleasePass at 820, then compares the SHARED baseline with vs
// without the second release and the per-runner controller. It only asserts no
// per-runner wake AT 820 (local events may legitimately wake at other times).
func TestT045RealSecondRelease(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	first := func(sim *Sim) {
		sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, 2.0, 0)})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.515, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
	}
	second := func(s *Sim, clock int) {
		if clock == 820 {
			s.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
		}
	}

	per := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	first(per.sim)
	tp := per.run(1000, second)
	sharedWith := t045MustHarness(t, cfg, 1, t045Shared)
	first(sharedWith.sim)
	tsWith := sharedWith.run(1000, second)
	sharedWithout := t045MustHarness(t, cfg, 1, t045Shared)
	first(sharedWithout.sim)
	tsWithout := sharedWithout.run(1000, nil)

	releases := 0
	var clocks []int
	for _, e := range tsWith.events {
		if e.Kind == PassEventRelease {
			releases++
			clocks = append(clocks, e.ClockMS)
		}
	}
	if releases < 2 || !containsInt(clocks, 820) {
		t.Fatalf("want >=2 releases incl an 820 off-lattice second release, got %d (%v)", releases, clocks)
	}
	if containsInt(t045AttemptClocks(tp.harnessPubs, 0, "local-wake"), 820) {
		t.Fatalf("per-runner woke AT the remote second release 820: %v", t045AttemptClocks(tp.harnessPubs, 0, "local-wake"))
	}
	if !containsInt(tsWith.pubClocks, 820) {
		t.Fatalf("shared with the second release did not publish at 820: %v", tsWith.pubClocks)
	}
	if containsInt(tsWithout.pubClocks, 820) {
		t.Fatalf("shared WITHOUT the second release still published at 820: %v", tsWithout.pubClocks)
	}
	t.Logf("real second release: events=%d clocks=%v; shared with->820 in pubs, without->not; per-runner no wake AT 820 (%v)", releases, clocks, t045AttemptClocks(tp.harnessPubs, 0, "local-wake"))
}
