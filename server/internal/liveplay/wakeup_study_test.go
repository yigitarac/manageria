package liveplay

import "testing"

// TestT045PerRunnerNearFarLateReaction: a two-local near/far pair with a LATE
// reaction and a real emitted RELEASE visible only to the near runner. True
// per-runner gating wakes ONLY the near runner (publication reason local-wake at
// the event clock); the far runner is not woken. The wakeup is dropped by the
// reaction gate (intent unchanged before reaction), so the visible event is
// consumed but coalesced into a single near-runner publication.
func TestT045PerRunnerNearFarLateReaction(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	h.sim.SetRunners([]Runner{
		t045Watcher(0.50, 0.50, 2.0, 300), // near
		t045Watcher(0.80, 0.50, 2.0, 300), // far
	})
	h.sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.90})
	tr := h.run(800, func(s *Sim, clock int) {
		if clock == 260 {
			// y offset 0.02 (~1.36 m) keeps it inside the near 2 m circle but off the
			// near runner's lane, so it can never be controlled.
			s.ReleasePass(PassParams{StartX: 0.51, StartY: 0.52, TargetX: 0.52, TargetY: 0.52, SpeedMPS: 2.0, Passer: -1})
		}
	})

	nearAttempts := t045AttemptClocks(tr.harnessPubs, 0, "local-wake")
	farAttempts := t045AttemptClocks(tr.harnessPubs, 1, "local-wake")
	nearApplied := t045AppliedClocks(tr.harnessPubs, 0, "local-wake")
	if !containsInt(nearAttempts, 260) {
		t.Fatalf("near runner not scheduled at the visible event 260: %v", nearAttempts)
	}
	if len(farAttempts) != 0 {
		t.Fatalf("far runner scheduled by a near-only event: %v", farAttempts)
	}
	if containsInt(nearApplied, 260) {
		t.Fatalf("near wake APPLIED before reaction (drop failed): %v", nearApplied)
	}
	if len(t045PubClocks(tr.harnessPubs, 0, "lattice")) == 0 {
		t.Fatal("no lattice publications recorded")
	}
	// The pre-step-260 attempt shows in the END frame of that step (clock 280).
	if got := t045IntentAt(tr, 0, 280); got != 0.95 {
		t.Fatalf("near intent changed before reaction (drop failed): %v", got)
	}
	if got := t045IntentAt(tr, 1, 280); got != 0.95 {
		t.Fatalf("far intent changed: %v", got)
	}
	t.Logf("near/far late reaction: near attempt=%v applied=%v (dropped), far attempt=%v; intents unchanged before reaction", nearAttempts, nearApplied, farAttempts)
}

// TestT045EventTimeVsNextPreStepGeometry proves the oracle uses the event's OWN
// event-time geometry (recorded ball position + interpolated runner), not the
// future next-pre-step ball: a fast release is inside the radius at its event time
// but outside by the next pre-step.
func TestT045EventTimeVsNextPreStepGeometry(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	radius := DistM(0.51, 0.50, 0.50, 0.50)
	h := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	h.sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, radius, 0)})
	h.sim.SetBall(Ball{Mode: BallLoose, X: 0.95, Y: 0.90})
	h.sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 40.0, Passer: -1})
	evs := h.sim.Events()
	ev := evs[len(evs)-1]
	if ev.Kind != PassEventRelease {
		t.Fatalf("last event %s, want release", ev.Kind)
	}
	eventDist := DistM(ev.BallX, ev.BallY, 0.50, 0.50)
	if !h.t045ClassifyCurrent(0, ev) {
		t.Fatalf("event-time release (dist %.6f, radius %.6f) not local", eventDist, radius)
	}
	h.syncLen()
	px, py := h.capture(nil, nil)
	h.scanNew(px, py, px, py)
	if !h.pendingLocal[0] {
		t.Fatal("event-time geometry did not set the per-runner wakeup")
	}
	h.step()
	endDist := DistM(h.sim.ball.X, h.sim.ball.Y, h.sim.runners[0].X, h.sim.runners[0].Y)
	if endDist <= radius {
		t.Fatalf("next-pre-step distance %.6f not outside radius %.6f; crossing not shown", endDist, radius)
	}
	t.Logf("event-time dist %.6f <= radius %.6f -> local; next-pre-step dist %.6f > radius -> a future-ball test would wrongly say remote", eventDist, radius, endDist)
}

// TestT045RemoteOutRealisedUnderSharedOnly retains the OUT evidence: a remote
// off-lattice OUT is a real event, it diverges the watcher only under the SHARED
// scheduler (extra publication after reaction), and per-runner gating isolates it
// (no wake, record not Seen). The body move is ~0.25 m from the shared leak.
func TestT045RemoteOutRealisedUnderSharedOnly(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(sim *Sim) {
		sim.SetRunners([]Runner{t045Watcher(0.50, 0.50, 2.0, 250)})
		sim.SetBall(Ball{Mode: BallRolling, X: 0.98, Y: 0.50, VXM: 8.5})
	}
	shared := t045MustHarness(t, cfg, 1, t045Shared)
	per := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	build(shared.sim)
	build(per.sim)
	ts := shared.run(1000, nil)
	tp := per.run(1000, nil)

	outClock := t045EventClockOfKind(ts.events, PassEventOut)
	if outClock < 0 || outClock%100 == 0 || outClock <= 250 {
		t.Fatalf("bad out fixture: clock %d", outClock)
	}
	d := t045DiffTraces(ts, tp)
	if !containsInt(d.pubOnlyShared, outClock) {
		t.Fatalf("shared did not add the out publication at %d: %+v", outClock, d)
	}
	if containsInt(t045AttemptClocks(tp.harnessPubs, 0, "local-wake"), outClock) {
		t.Fatalf("per-runner woke AT the remote OUT %d", outClock)
	}
	if d.maxPlayerM <= 0 {
		t.Fatalf("shared out leak did not move a body: %+v", d)
	}
	if o, _ := per.sim.Perception(0); o.Seen {
		t.Fatalf("remote OUT leaked into the per-runner record: %+v", o)
	}
	t.Logf("remote OUT @%dms: shared adds the publication and moves a body (maxPlayerDiff=%.4fm, maxVelDiff=%.4fm/s); per-runner isolates", outClock, d.maxPlayerM, d.maxVelM)
}

// t045StaleFixture: a local runner observes an in-range ball, the ball then leaves
// the circle (record stale), and later a remote event occurs.
func t045StaleFixture(sim *Sim) {
	r := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
	r.ReachM = 0.05
	r.ReactAtMS = 0
	if err := r.EnableLocalPursuit(2.0); err != nil {
		panic(err)
	}
	sim.SetRunners([]Runner{r})
	sim.SetBall(Ball{Mode: BallLoose, X: 0.51, Y: 0.50})
}

func t045StaleMid(s *Sim, clock int) {
	switch clock {
	case 200:
		s.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90}) // leave the 2 m circle
	case 820:
		s.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
	}
}

// TestT045StaleRecordRemoteEventTimingOnly (restored accepted scope): a stale
// record RETAINS its prior destination, so the shared remote publication changes no
// intent and no position; only the publication clock differs.
func TestT045StaleRecordRemoteEventTimingOnly(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	shared := t045MustHarness(t, cfg, 1, t045Shared)
	per := t045MustHarness(t, cfg, 1, t045GatedPerRunner)
	t045StaleFixture(shared.sim)
	t045StaleFixture(per.sim)
	ts := shared.run(1200, t045StaleMid)
	tp := per.run(1200, t045StaleMid)

	o, _ := shared.sim.Perception(0)
	if !o.Seen || o.Freshness(1200) != ObservationStale {
		t.Fatalf("expected a stale record at the horizon: %+v", o)
	}
	d := t045DiffTraces(ts, tp)
	if !d.positionsEqual || !d.intentsEqual {
		t.Fatalf("stale retention diverged between modes: %+v", d)
	}
	if !containsInt(d.pubOnlyShared, 820) {
		t.Fatalf("shared did not add the remote 820 publication: %+v", d)
	}
	t.Logf("stale record: shared added %v; targets/positions identical (pub-clock side channel)", d.pubOnlyShared)
}

// TestT045BoundaryEventLocality (restored accepted scope): the oracle uses the
// closed metric circle at the ACTUAL event-time geometry; an event exactly at the
// computed radius is local and just outside is remote.
func TestT045BoundaryEventLocality(t *testing.T) {
	radius := DistM(0.52, 0.50, 0.50, 0.50)
	classify := func(startX float64) (bool, float64) {
		h := t045MustHarness(t, ScheduledLivePassConfig(100), 1, t045GatedPerRunner)
		r := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
		if err := r.EnableLocalPursuit(radius); err != nil {
			t.Fatalf("EnableLocalPursuit: %v", err)
		}
		h.sim.SetRunners([]Runner{r})
		h.sim.ReleasePass(PassParams{StartX: startX, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 1.0, Passer: -1})
		evs := h.sim.Events()
		e := evs[len(evs)-1]
		return h.t045ClassifyCurrent(0, e), DistM(e.BallX, e.BallY, 0.50, 0.50)
	}
	onBoundary, onDist := classify(0.52)
	if !onBoundary {
		t.Fatalf("on-boundary event (dist %.17g, radius %.17g) classified remote", onDist, radius)
	}
	justOut, outDist := classify(0.52 + 1e-4/PitchLengthM)
	if justOut {
		t.Fatalf("just-outside event (dist %.17g, radius %.17g) classified local", outDist, radius)
	}
	t.Logf("boundary: dist %.17g == radius -> local; dist %.17g (radius+~1e-4 m) -> remote", onDist, outDist)
}
