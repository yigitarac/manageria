package liveplay

import (
	"reflect"
	"testing"
)

// TestT045SharedModeMatchesUnmodifiedScheduler (restored accepted scope): the
// harness's shared mode is byte-identical to a direct T-042 scheduler run (same
// publications, same complete final Sim state).
func TestT045SharedModeMatchesUnmodifiedScheduler(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	direct := mustSim(t, cfg, 1)
	t045UnknownMovingFixture(direct)
	pubsDirect := t042Run(t, direct, 300, nil)

	h := t045MustHarness(t, cfg, 1, t045Shared)
	t045UnknownMovingFixture(h.sim)
	tr := h.run(300, nil)

	if !reflect.DeepEqual(pubsDirect, tr.pubClocks) {
		t.Fatalf("harness shared pubs %v != direct scheduler pubs %v", tr.pubClocks, pubsDirect)
	}
	if !reflect.DeepEqual(direct.Snapshot(), h.sim.Snapshot()) {
		t.Fatal("harness shared mode changed the complete Sim state")
	}
	t.Logf("shared mode byte-identical to the unmodified T-042 scheduler: pubs=%v", tr.pubClocks)
}

// TestT045DeterministicReplay (restored accepted scope): two identical per-runner
// runs are bit-identical in complete state and trajectory.
func TestT045DeterministicReplay(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	x := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	y := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	t045StaleFixture(x.sim)
	t045StaleFixture(y.sim)
	tx := x.run(1200, t045StaleMid)
	ty := y.run(1200, t045StaleMid)
	if !reflect.DeepEqual(x.sim.Snapshot(), y.sim.Snapshot()) {
		t.Fatal("two identical per-runner runs produced different complete snapshots")
	}
	if d := t045DiffTraces(tx, ty); !d.completeEqual {
		t.Fatalf("two identical per-runner runs diverged: %+v", d)
	}
	t.Logf("two identical per-runner runs bit-identical (complete state + trajectory)")
}

// TestT045LatticeStableDeadline (planner request): an off-lattice state-changing
// event must NOT shift the periodic lattice. A run WITH an event at 60 ms and a run
// WITHOUT keep an identical nextDecisionMS at every pre-step, always a multiple of
// the cadence.
func TestT045LatticeStableDeadline(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	with := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	without := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	t045UnknownMovingFixture(with.sim)
	t045UnknownMovingFixture(without.sim)
	c := cfg.IntentCadenceMS
	for with.sim.ClockMS() < 400 {
		t045RemoteRelease60(with.sim, with.sim.ClockMS())
		with.step()
		without.step()
		if with.sim.nextDecisionMS != without.sim.nextDecisionMS {
			t.Fatalf("@%d: event shifted the lattice: %d vs %d", with.sim.ClockMS(), with.sim.nextDecisionMS, without.sim.nextDecisionMS)
		}
		if with.sim.nextDecisionMS%c != 0 {
			t.Fatalf("@%d: nextDecisionMS %d not a multiple of %d", with.sim.ClockMS(), with.sim.nextDecisionMS, c)
		}
	}
	t.Logf("lattice stable at every pre-step: an off-lattice event does not shift nextDecisionMS")
}

// TestT045CoalescedDeflectOutSingleWake (restored accepted scope, T-044 proven
// scene): a defender miscontrols a ball leaving the pitch in one substep (one
// deflect + one out). Both events are locally visible to a wide-radius watcher and
// COALESCE into exactly ONE local-wake publication at the next pre-step; the
// watcher observes the Out.
func TestT045CoalescedDeflectOutSingleWake(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	h.sim.SetRunners([]Runner{
		t045Watcher(0.93, 0.50, 20.0, 0),
		{X: 0.990, Y: 0.50, ReachM: ControlRadiusM},
	})
	h.sim.controlError = 1.0 // force the miscontrol (test-only)
	h.sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
	h.step() // step 0 (lattice): the deflect+out coalesce within the step

	if countKind(h.sim, PassEventDeflect) != 1 || countKind(h.sim, PassEventOut) != 1 {
		t.Fatalf("coalesced fixture did not produce one deflect + one out: def=%d out=%d",
			countKind(h.sim, PassEventDeflect), countKind(h.sim, PassEventOut))
	}
	if !h.pendingLocal[0] {
		t.Fatal("coalesced events left no pending local wakeup")
	}
	base := h.pubSteps
	h.step() // step 20 (off-lattice): exactly one coalesced wake
	if h.pubSteps-base != 1 {
		t.Fatalf("coalesced events caused %d publications, want exactly 1", h.pubSteps-base)
	}
	if got := t045AttemptClocks(h.pubs, 0, "local-wake"); !reflect.DeepEqual(got, []int{20}) {
		t.Fatalf("watcher local-wake clocks = %v, want [20]", got)
	}
	if o, _ := h.sim.Perception(0); o.Mode != BallOut {
		t.Fatalf("watcher observed mode %s, want out", o.Mode)
	}
	t.Logf("coalesced deflect+out: exactly ONE local-wake at 20ms; the watcher observed the Out")
}

// TestT045SharedNativeUnmodifiedWithLocalPresent proves a far, never-seeing local
// runner does not perturb a native pursuer in shared mode (native semantics are
// preserved, like the T-044 mixed check).
func TestT045SharedNativeUnmodifiedWithLocalPresent(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()
	setBall := func(sim *Sim) {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	solo := t045MustHarness(t, cfg, 1, t045Shared)
	solo.sim.SetRunners([]Runner{PursuingRunner(0.60, 0.50, p, 0)})
	setBall(solo.sim)
	trS := solo.run(2000, nil)

	mix := t045MustHarness(t, cfg, 1, t045Shared)
	mix.sim.SetRunners([]Runner{PursuingRunner(0.60, 0.50, p, 0), t045Watcher(0.05, 0.05, 4.0, 0)})
	setBall(mix.sim)
	trM := mix.run(2000, nil)

	if len(trS.frames) != len(trM.frames) {
		t.Fatalf("frame count %d vs %d", len(trS.frames), len(trM.frames))
	}
	for i := range trS.frames {
		a, b := trS.frames[i], trM.frames[i]
		if a.xs[0] != b.xs[0] || a.ys[0] != b.ys[0] || a.vxs[0] != b.vxs[0] || a.vys[0] != b.vys[0] {
			t.Fatalf("native runner diverged at frame %d when a far local runner is present", i)
		}
	}
	t.Logf("native runner trajectory identical with/without a far local runner (shared mode)")
}
