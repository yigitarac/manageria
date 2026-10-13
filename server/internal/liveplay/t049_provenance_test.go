package liveplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// T-049 local decision provenance sidecar tests (isolated prototype evidence).
//
// These tests exercise the OPTIONAL, BOUNDED diagnostic recorder over the ACCEPTED
// T-044 local pursuit + T-046 per-runner wakeup publications. Every scene is driven
// through the REAL Step()/scheduler path with a recorder attached, and each record
// is asserted against the TRUE per-runner publication (reason, applied, policy,
// observation, target vs own pre-step position). The schema/contract is fixed in
// the session evidence BEFORE this code
// (vault/07-Session-Logs/2026-10-13-t049-decision-provenance-evidence.md).
//
// No test asserts a prediction, a runtime policy change, a production/default bit
// or a realism/cost claim. Recording only READS: the recorder on/off snapshots are
// identical every substep.

// --- helpers ---------------------------------------------------------------

// t049Recorded builds a scene, attaches a bounded recorder, runs to the horizon
// (with an optional between-step hook) and returns the sim plus the captured
// record VALUE copies.
func t049Recorded(t *testing.T, cfg Config, seed uint64, build func(*Sim), horizon, capacity int, mid func(*Sim, int)) (*Sim, []DecisionRecord) {
	t.Helper()
	sim := mustSim(t, cfg, seed)
	build(sim)
	rec, err := NewProvenanceRecorder(capacity)
	if err != nil {
		t.Fatalf("NewProvenanceRecorder: %v", err)
	}
	sim.SetProvenanceRecorder(rec)
	for sim.ClockMS() < horizon {
		if mid != nil {
			mid(sim, sim.ClockMS())
		}
		sim.Step()
	}
	return sim, rec.Records()
}

// t049MustRecord returns the single record at (clock, runner) or fails.
func t049MustRecord(t *testing.T, recs []DecisionRecord, clock, runner int) DecisionRecord {
	t.Helper()
	for _, r := range recs {
		if r.ClockMS == clock && r.Runner == runner {
			return r
		}
	}
	t.Fatalf("no provenance record at clock %d runner %d (have %d records)", clock, runner, len(recs))
	return DecisionRecord{}
}

// t049Expect asserts the common per-record fields.
func t049Expect(t *testing.T, rec DecisionRecord, reason DecisionReason, simult, applied bool, gate string, policy DecisionPolicy, freshness string) {
	t.Helper()
	if rec.Reason != reason {
		t.Fatalf("@%d r%d: reason %q, want %q", rec.ClockMS, rec.Runner, rec.Reason, reason)
	}
	if rec.Simultaneous != simult {
		t.Fatalf("@%d r%d: simultaneous %v, want %v", rec.ClockMS, rec.Runner, rec.Simultaneous, simult)
	}
	if rec.Applied != applied {
		t.Fatalf("@%d r%d: applied %v, want %v", rec.ClockMS, rec.Runner, rec.Applied, applied)
	}
	if rec.GateOutcome != gate {
		t.Fatalf("@%d r%d: gateOutcome %q, want %q", rec.ClockMS, rec.Runner, rec.GateOutcome, gate)
	}
	if rec.Policy != policy {
		t.Fatalf("@%d r%d: policy %q, want %q", rec.ClockMS, rec.Runner, rec.Policy, policy)
	}
	if rec.Freshness != freshness {
		t.Fatalf("@%d r%d: freshness %q, want %q", rec.ClockMS, rec.Runner, rec.Freshness, freshness)
	}
}

// t049Local builds an inertial local-pursuit + local-wakeup runner with a tiny
// reach so it chases without controlling the fixture ball.
func t049Local(t *testing.T, x, y, radiusM float64) Runner {
	t.Helper()
	r := InertialRunner(x, y, x, y, DefaultMotionParams())
	r.ReachM = 0.05
	if err := r.EnableLocalPursuit(radiusM); err != nil {
		t.Fatalf("EnableLocalPursuit: %v", err)
	}
	if err := r.EnableLocalWakeup(radiusM); err != nil {
		t.Fatalf("EnableLocalWakeup: %v", err)
	}
	return r
}

// --- 1. actual scheduler scenes -------------------------------------------

// TestT049ActualSceneReasonsAndPolicies drives each required moment through the
// real scheduler and asserts the captured reason/outcome/policy/freshness and the
// post-intent target vs the runner's own pre-step position.
func TestT049ActualSceneReasonsAndPolicies(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	p := DefaultMotionParams()

	t.Run("periodic-chase-near", func(t *testing.T) {
		ownY := 0.50 + 2.0/PitchWidthM
		sim, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			sim.SetRunners([]Runner{t049Local(t, 0.60, ownY, 4.0)})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
		}, 20, 16, nil)
		_ = sim
		rec := t049MustRecord(t, recs, 0, 0)
		t049Expect(t, rec, ReasonPeriodic, false, true, GateApplied, PolicyChase, "fresh")
		if !rec.Observation.Seen || rec.Observation.Mode != "loose" || rec.Observation.X != 0.60 || rec.Observation.Y != 0.50 {
			t.Fatalf("near observation wrong: %+v", rec.Observation)
		}
		if rec.TargetX != 0.60 || rec.TargetY != 0.50 {
			t.Fatalf("chase target (%v,%v), want observed (0.60,0.50)", rec.TargetX, rec.TargetY)
		}
		if rec.OwnX != 0.60 || rec.OwnY != ownY {
			t.Fatalf("own pre-step (%v,%v), want (0.60,%v)", rec.OwnX, rec.OwnY, ownY)
		}
	})

	t.Run("unknown-brake", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			sim.SetRunners([]Runner{t049Local(t, 0.10, 0.10, 2.0)})
			sim.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 2.0})
		}, 20, 16, nil)
		rec := t049MustRecord(t, recs, 0, 0)
		t049Expect(t, rec, ReasonPeriodic, false, true, GateApplied, PolicyUnknownBrake, "unknown")
		if rec.Observation.Seen || rec.Observation.Owner != -1 {
			t.Fatalf("unknown observation not clean: %+v", rec.Observation)
		}
		if rec.TargetX != 0.10 || rec.TargetY != 0.10 {
			t.Fatalf("unknown-brake target (%v,%v), want own (0.10,0.10)", rec.TargetX, rec.TargetY)
		}
	})

	t.Run("stale-retain", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			r := t049Local(t, 0.50, 0.50+1.5/PitchWidthM, 2.0)
			sim.SetRunners([]Runner{r})
			sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
		}, 3000, 256, nil)
		var lastFresh DecisionRecord
		haveFresh := false
		found := false
		for _, rec := range recs {
			if rec.Runner != 0 {
				continue
			}
			switch rec.Freshness {
			case "fresh":
				lastFresh, haveFresh = rec, true
			case "stale":
				if !haveFresh {
					t.Fatal("stale record before any fresh record")
				}
				t049Expect(t, rec, ReasonPeriodic, false, true, GateApplied, PolicyStaleRetain, "stale")
				if rec.TargetX != lastFresh.TargetX || rec.TargetY != lastFresh.TargetY {
					t.Fatalf("stale-retain target (%v,%v) != last fresh (%v,%v)", rec.TargetX, rec.TargetY, lastFresh.TargetX, lastFresh.TargetY)
				}
				found = true
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatal("stale fixture produced no stale record (vacuous)")
		}
	})

	t.Run("held-nonfree-brake", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			holder := InertialRunner(0.50, 0.50, 0.50, 0.50, p)
			watcher := t049Local(t, 0.52, 0.50, 8.0)
			sim.SetRunners([]Runner{holder, watcher})
			sim.SetBall(Ball{Mode: BallHeld, X: 0.50, Y: 0.50})
			sim.contactRunner = 0
			sim.contactMS = 0
		}, 20, 16, nil)
		rec := t049MustRecord(t, recs, 0, 1)
		t049Expect(t, rec, ReasonPeriodic, false, true, GateApplied, PolicyNonFreeBrake, "fresh")
		if !rec.Observation.Seen || rec.Observation.Mode != "held" || rec.Observation.Owner != 0 {
			t.Fatalf("held observation wrong: %+v", rec.Observation)
		}
		if rec.TargetX != 0.52 || rec.TargetY != 0.50 {
			t.Fatalf("held brake target (%v,%v), want own (0.52,0.50)", rec.TargetX, rec.TargetY)
		}
	})

	t.Run("local-event", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			sim.SetRunners([]Runner{t049Local(t, 0.50, 0.50, 2.0)})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		}, 80, 16, func(s *Sim, clock int) {
			if clock == 60 {
				s.ReleasePass(PassParams{StartX: 0.505, StartY: 0.505, TargetX: 0.52, TargetY: 0.505, SpeedMPS: 0.5, Passer: -1})
			}
		})
		rec := t049MustRecord(t, recs, 60, 0)
		t049Expect(t, rec, ReasonLocalEvent, false, true, GateApplied, PolicyChase, "fresh")
		if rec.Observation.X != 0.505 || rec.Observation.Y != 0.505 {
			t.Fatalf("local-event observation (%v,%v), want (0.505,0.505)", rec.Observation.X, rec.Observation.Y)
		}
		if rec.TargetX != 0.505 || rec.TargetY != 0.505 {
			t.Fatalf("local-event target (%v,%v), want observed (0.505,0.505)", rec.TargetX, rec.TargetY)
		}
	})

	t.Run("shared-event", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			// UNFLAGGED local-pursuit runner (sensing only): the shared dirty signal
			// schedules it off-lattice.
			r := InertialRunner(0.50, 0.50, 0.95, 0.95, p)
			r.ReachM = 0.05
			if err := r.EnableLocalPursuit(2.0); err != nil {
				t.Fatalf("EnableLocalPursuit: %v", err)
			}
			sim.SetRunners([]Runner{r})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		}, 80, 16, func(s *Sim, clock int) {
			if clock == 60 {
				s.ReleasePass(PassParams{StartX: 0.05, StartY: 0.50, TargetX: 0.30, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
			}
		})
		rec := t049MustRecord(t, recs, 60, 0)
		t049Expect(t, rec, ReasonSharedEvent, false, true, GateApplied, PolicyUnknownBrake, "unknown")
	})

	t.Run("real-second-release", func(t *testing.T) {
		sim, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			r := t049Local(t, 0.50, 0.50, 2.0)
			sim.SetRunners([]Runner{r})
			// REAL first release at 0 (in range, fresh at the 0 lattice).
			sim.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
		}, 900, 64, func(s *Sim, clock int) {
			if clock == 820 {
				s.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1})
			}
		})
		// The REAL fixture must actually emit exactly two releases [0, 820].
		if rel := t042EventClocks(sim.Events(), PassEventRelease); !t042IntsEqual(rel, []int{0, 820}) {
			t.Fatalf("release clocks %v, want [0 820]", rel)
		}
		// First record: the real in-range release at 0 coincided with the 0 lattice.
		first := t049MustRecord(t, recs, 0, 0)
		t049Expect(t, first, ReasonPeriodic, true, true, GateApplied, PolicyChase, "fresh")
		// The remote second release schedules the opted-in runner only off-lattice
		// if the event is LOCAL; a remote event leaves no pending, so the opted
		// runner is scheduled on the lattice only. Assert no record at 820 and that
		// a lattice record exists at 800.
		for _, rec := range recs {
			if rec.Runner == 0 && rec.ClockMS == 820 {
				t.Fatalf("remote second release woke the opted-in runner off-lattice: %+v", rec)
			}
		}
		if _, ok := t049RecordAt(recs, 800, 0); !ok {
			t.Fatal("no lattice record at 800")
		}
	})

	t.Run("deflect-coalesce", func(t *testing.T) {
		sim, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			w := t049Local(t, 0.93, 0.50, 20.0)
			sim.SetRunners([]Runner{w, {X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
			sim.controlError = 1.0
			sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
		}, 40, 32, nil)
		// The coalescence claim requires the REAL fixture to have produced BOTH a
		// deflect and an out in the SAME substep (both stamped at clock 20).
		def := t042EventClocks(sim.Events(), PassEventDeflect)
		out := t042EventClocks(sim.Events(), PassEventOut)
		if len(def) != 1 || len(out) != 1 || def[0] != 20 || out[0] != 20 {
			t.Fatalf("coalesced fixture deflect=%v out=%v, want exactly [20] and [20]", def, out)
		}
		at20 := 0
		for _, rec := range recs {
			if rec.Runner == 0 && rec.ClockMS == 20 {
				at20++
			}
		}
		if at20 != 1 {
			t.Fatalf("coalesced deflect+out produced %d records at clock 20, want exactly 1", at20)
		}
		rec := t049MustRecord(t, recs, 20, 0)
		t049Expect(t, rec, ReasonLocalEvent, false, true, GateApplied, PolicyNonFreeBrake, "fresh")
		if rec.Observation.Mode != "out" {
			t.Fatalf("coalesced observation mode %q, want out", rec.Observation.Mode)
		}
	})

	t.Run("reaction-blocked", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			r := t049Local(t, 0.50, 0.50, 2.0)
			r.ReactMS = 300
			r.ReactAtMS = 0 // re-armed by the release below
			sim.SetRunners([]Runner{r})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		}, 80, 16, func(s *Sim, clock int) {
			if clock == 60 {
				// A visible release inside the circle, slow so it stays in range
				// through the 360 ms reaction deadline.
				s.ReleasePass(PassParams{StartX: 0.505, StartY: 0.505, TargetX: 0.52, TargetY: 0.505, SpeedMPS: 0.5, Passer: -1})
			}
		})
		if _, ok := t049RecordAt(recs, 60, 0); !ok {
			t.Fatal("no scheduling attempt recorded at the pre-reaction local event")
		}
		rec := t049MustRecord(t, recs, 60, 0)
		t049Expect(t, rec, ReasonLocalEvent, false, false, GateReactionBlocked, PolicyNone, "unknown")
		if rec.Observation.Seen {
			t.Fatalf("reaction-blocked record observed the ball: %+v", rec.Observation)
		}
	})

	t.Run("periodic-simultaneous", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			sim.SetRunners([]Runner{t049Local(t, 0.50, 0.50, 2.0)})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
		}, 120, 32, func(s *Sim, clock int) {
			if clock == 100 {
				// A local release emitted BETWEEN steps at the lattice clock: its
				// own pending AND the lattice due coincide.
				s.ReleasePass(PassParams{StartX: 0.51, StartY: 0.50, TargetX: 0.52, TargetY: 0.50, SpeedMPS: 0.5, Passer: -1})
			}
		})
		rec := t049MustRecord(t, recs, 100, 0)
		t049Expect(t, rec, ReasonPeriodic, true, true, GateApplied, PolicyChase, "fresh")
	})

	t.Run("near-far-pair", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			near := t049Local(t, 0.60, 0.50+2.0/PitchWidthM, 4.0)
			far := t049Local(t, 0.10, 0.10, 2.0)
			sim.SetRunners([]Runner{near, far})
			sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
		}, 20, 16, nil)
		near := t049MustRecord(t, recs, 0, 0)
		t049Expect(t, near, ReasonPeriodic, false, true, GateApplied, PolicyChase, "fresh")
		far := t049MustRecord(t, recs, 0, 1)
		t049Expect(t, far, ReasonPeriodic, false, true, GateApplied, PolicyUnknownBrake, "unknown")
		if far.TargetX != 0.10 || far.TargetY != 0.10 {
			t.Fatalf("far braked to (%v,%v), want own (0.10,0.10)", far.TargetX, far.TargetY)
		}
	})

	t.Run("invalid-enabling", func(t *testing.T) {
		_, recs := t049Recorded(t, cfg, 1, func(sim *Sim) {
			r := InertialRunner(0.50, 0.50, 0.95, 0.95, p)
			r.LocalPursuit = true // raw flag, NO sensing record: the enabling is invalid
			sim.SetRunners([]Runner{r})
			sim.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.50, VXM: 2.0})
		}, 20, 16, nil)
		rec := t049MustRecord(t, recs, 0, 0)
		t049Expect(t, rec, ReasonPeriodic, false, false, GateInvalidEnabling, PolicyNone, "unknown")
	})

	t.Run("disabled-scheduler-no-records", func(t *testing.T) {
		// The disabled (native per-substep) path never runs the accepted T-044/
		// T-046 local layer, so the recorder captures NOTHING even with a raw local
		// opt-in present.
		_, recs := t049Recorded(t, LivePassConfig(), 1, func(sim *Sim) {
			r := InertialRunner(0.50, 0.50, 0.95, 0.95, p)
			r.LocalPursuit = true
			if err := r.EnableSensing(4.0); err != nil {
				t.Fatalf("EnableSensing: %v", err)
			}
			sim.SetRunners([]Runner{r})
			sim.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.50, VXM: 2.0})
		}, 200, 16, nil)
		if len(recs) != 0 {
			t.Fatalf("disabled scheduler recorded %d decisions, want 0", len(recs))
		}
	})
}

// t049RecordAt is a non-fatal lookup.
func t049RecordAt(recs []DecisionRecord, clock, runner int) (DecisionRecord, bool) {
	for _, r := range recs {
		if r.ClockMS == clock && r.Runner == runner {
			return r, true
		}
	}
	return DecisionRecord{}, false
}

// --- 2. recording is a pure read: on/off snapshots identical ---------------

// TestT049RecorderOnOffSnapshotsIdentical steps two identical scenes (one with a
// recorder attached, one without) in lockstep and asserts the COMPLETE snapshot
// and the derived sample stream are identical every substep.
func TestT049RecorderOnOffSnapshotsIdentical(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(sim *Sim) {
		a := t049Local(t, 0.60, 0.50+2.0/PitchWidthM, 30.0)
		b := t049Local(t, 0.78, 0.42, 30.0)
		sim.SetRunners([]Runner{a, b})
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	on := mustSim(t, cfg, 5)
	off := mustSim(t, cfg, 5)
	build(on)
	build(off)
	rec, err := NewProvenanceRecorder(1024)
	if err != nil {
		t.Fatal(err)
	}
	on.SetProvenanceRecorder(rec)
	for on.ClockMS() < 2500 {
		on.Step()
		off.Step()
		if !reflect.DeepEqual(on.Snapshot(), off.Snapshot()) {
			t.Fatalf("@%d: recorder changed the complete snapshot", on.ClockMS())
		}
	}
	if !reflect.DeepEqual(on.Samples(), off.Samples()) {
		t.Fatal("recorder changed the sample stream")
	}
	if rec.Len() == 0 {
		t.Fatal("recorder captured no records on an active local scene")
	}
	t.Logf("recorder on/off: identical snapshots every substep; %d records captured", rec.Len())
}

// TestT049RecorderNilNoAllocations proves the nil-recorder path allocates no
// diagnostics on a local-wakeup scene (the gated path is steady state).
func TestT049NilRecorderNoAllocations(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t046LocalBenchRunners(true))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	if sim.provenance != nil {
		t.Fatal("fixture unexpectedly has a recorder attached")
	}
	allocs := testing.AllocsPerRun(20, func() {
		t046RearmBench(sim)
		for k := 0; k < 50; k++ {
			sim.Step()
		}
	})
	if allocs != 0 {
		t.Fatalf("nil-recorder local path allocated %.2f allocs/run, want 0", allocs)
	}
	if sim.localEvent != nil {
		t.Fatal("nil-recorder path allocated the provenance event scratch")
	}
	t.Logf("nil recorder on a 22-body local scene: 0 allocs/run; no event scratch")
}

// TestT049RecorderNotSerialized proves the recorder is never serialized in a
// Snapshot, and a fresh recorder attached after a JSON Resume captures the
// IDENTICAL absolute-time tail.
func TestT049RecorderNotSerialized(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(sim *Sim) {
		sim.SetRunners([]Runner{
			t049Local(t, 0.60, 0.50, 30.0),
			t049Local(t, 0.80, 0.40, 30.0),
		})
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	straight := mustSim(t, cfg, 5)
	build(straight)
	recA, err := NewProvenanceRecorder(2048)
	if err != nil {
		t.Fatal(err)
	}
	straight.SetProvenanceRecorder(recA)
	const split = 250 // not a lattice clock
	for straight.ClockMS() < split {
		straight.Step()
	}
	blob, err := json.Marshal(straight.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(blob), "provenance") || strings.Contains(string(blob), "schemaVersion") {
		t.Fatalf("snapshot serialized the recorder: %s", blob)
	}
	var snap Snapshot
	if err := json.Unmarshal(blob, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resumed, err := Resume(snap)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.ProvenanceRecorder() != nil {
		t.Fatal("Resume attached a recorder from the snapshot")
	}
	recB, err := NewProvenanceRecorder(2048)
	if err != nil {
		t.Fatal(err)
	}
	resumed.SetProvenanceRecorder(recB)
	for straight.ClockMS() < 2500 {
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("@%d: tail snapshot diverged", straight.ClockMS())
		}
		straight.Step()
		resumed.Step()
	}
	var wantTail []DecisionRecord
	for _, r := range recA.Records() {
		if r.ClockMS >= split {
			wantTail = append(wantTail, r)
		}
	}
	if !reflect.DeepEqual(wantTail, recB.Records()) {
		t.Fatalf("fresh-recorder tail differs from the straight absolute-time tail:\n want=%+v\n got =%+v", wantTail, recB.Records())
	}
	if len(wantTail) == 0 {
		t.Fatal("no records in the resumed tail (vacuous)")
	}
	t.Logf("recorder not serialized; fresh recorder captured %d identical absolute-time tail records", len(recB.Records()))
}

// --- 3. export purity + bounded overflow ----------------------------------

// TestT049ExportClonePurityAndOverflow proves BuildDecisionProvenance clones the
// source (no mutation/alias), returns value copies, and reports bounded overflow
// explicitly while still returning a valid export.
func TestT049ExportClonePurityAndOverflow(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	source := mustSim(t, cfg, 3)
	source.SetRunners([]Runner{t049Local(t, 0.62, 0.50, 30.0), t049Local(t, 0.78, 0.42, 30.0)})
	source.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	// A recorder attached to the SOURCE must never receive clone records.
	srcRec, err := NewProvenanceRecorder(64)
	if err != nil {
		t.Fatal(err)
	}
	source.SetProvenanceRecorder(srcRec)

	before := source.Snapshot()
	exp, err := BuildDecisionProvenance(ProvenanceInput{Sim: source, Capacity: 4096, HorizonMS: 2500})
	if err != nil {
		t.Fatalf("BuildDecisionProvenance: %v", err)
	}
	if !reflect.DeepEqual(before, source.Snapshot()) {
		t.Fatal("BuildDecisionProvenance mutated the source simulation")
	}
	if srcRec.Len() != 0 {
		t.Fatalf("source recorder captured %d clone records", srcRec.Len())
	}
	if err := exp.Validate(); err != nil {
		t.Fatalf("valid export rejected: %v", err)
	}
	if len(exp.Records) == 0 {
		t.Fatal("export captured no records (vacuous)")
	}

	// Mutating the returned records must not affect a fresh export.
	exp.Records[0].TargetX = 12345
	again, err := BuildDecisionProvenance(ProvenanceInput{Sim: source, Capacity: 4096, HorizonMS: 2500})
	if err != nil {
		t.Fatal(err)
	}
	if again.Records[0].TargetX == 12345 {
		t.Fatal("returned records alias the recorder or source state")
	}

	// Bounded overflow is explicit but the export stays valid.
	small, err := BuildDecisionProvenance(ProvenanceInput{Sim: source, Capacity: 2, HorizonMS: 2500})
	if !errors.Is(err, ErrProvenanceOverflow) {
		t.Fatalf("overflow err=%v, want ErrProvenanceOverflow", err)
	}
	if !small.Overflowed || small.RecordCount != 2 || small.Capacity != 2 {
		t.Fatalf("overflow export wrong: overflowed=%v count=%d cap=%d", small.Overflowed, small.RecordCount, small.Capacity)
	}
	if err := small.Validate(); err != nil {
		t.Fatalf("overflowed export should stay structurally valid: %v", err)
	}

	// Construction errors.
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: nil, Capacity: 2, HorizonMS: 10}); !errors.Is(err, ErrProvenanceNilSim) {
		t.Fatalf("nil sim err=%v, want ErrProvenanceNilSim", err)
	}
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: source, Capacity: 0, HorizonMS: 10}); !errors.Is(err, ErrProvenanceCapacity) {
		t.Fatalf("zero capacity err=%v, want ErrProvenanceCapacity", err)
	}
	// HorizonMS is an ABSOLUTE clock: a negative horizon is rejected.
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: source, Capacity: 8, HorizonMS: -20}); !errors.Is(err, ErrProvenanceHorizon) {
		t.Fatalf("negative horizon err=%v, want ErrProvenanceHorizon", err)
	}
	// A horizon BEFORE the source's current clock is rejected (pre-start). The
	// source above is at clock 0, so advance a fresh sim past the horizon.
	stepped := mustSim(t, cfg, 3)
	stepped.SetRunners([]Runner{t049Local(t, 0.62, 0.50, 30.0)})
	stepped.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	for stepped.ClockMS() < 100 {
		stepped.Step()
	}
	if _, err := BuildDecisionProvenance(ProvenanceInput{Sim: stepped, Capacity: 8, HorizonMS: 60}); !errors.Is(err, ErrProvenanceHorizon) {
		t.Fatalf("pre-start horizon err=%v, want ErrProvenanceHorizon", err)
	}
	t.Logf("export cloned the source with no mutation/alias; %d records; capacity 2 overflow flagged explicitly", len(exp.Records))
}

// --- 4. schema validation rejects structurally impossible data -------------

// t049ValidExport returns a hand-built, structurally VALID export.
func t049ValidExport() DecisionProvenance {
	return DecisionProvenance{
		SchemaVersion: ProvenanceSchemaVersion,
		TimeUnit:      "ms",
		SubstepMS:     20,
		Identity:      provenanceIdentity,
		RunnerCount:   2,
		StartMS:       0,
		EndMS:         40,
		Capacity:      4,
		RecordCount:   1,
		Records: []DecisionRecord{{
			ClockMS:      20,
			Runner:       0,
			Reason:       ReasonPeriodic,
			Simultaneous: false,
			Applied:      true,
			GateOutcome:  GateApplied,
			Policy:       PolicyChase,
			Freshness:    "fresh",
			Observation: ProvenanceObservation{
				Seen: true, ClockMS: 20, X: 0.5, Y: 0.5, VXM: 1, VYM: 0, Mode: "rolling", Owner: -1,
			},
			TargetX: 0.5, TargetY: 0.5, OwnX: 0.5, OwnY: 0.5,
		}},
	}
}

// TestT049ValidateRejectsInvalid proves Validate rejects bad data rather than
// trusting it.
func TestT049ValidateRejectsInvalid(t *testing.T) {
	base := t049ValidExport()
	if err := base.Validate(); err != nil {
		t.Fatalf("baseline valid export rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*DecisionProvenance)
	}{
		{"schema-version", func(p *DecisionProvenance) { p.SchemaVersion = 2 }},
		{"time-unit", func(p *DecisionProvenance) { p.TimeUnit = "s" }},
		{"identity", func(p *DecisionProvenance) { p.Identity = "runner" }},
		{"substep", func(p *DecisionProvenance) { p.SubstepMS = 0 }},
		{"runner-count", func(p *DecisionProvenance) { p.RunnerCount = 0 }},
		{"capacity", func(p *DecisionProvenance) { p.Capacity = 0 }},
		{"end-before-start", func(p *DecisionProvenance) { p.EndMS = -1 }},
		{"off-grid", func(p *DecisionProvenance) { p.StartMS = 5 }},
		{"record-count", func(p *DecisionProvenance) { p.RecordCount = 2 }},
		{"overflow-inconsistent", func(p *DecisionProvenance) { p.Overflowed = true }},
		{"reason", func(p *DecisionProvenance) { p.Records[0].Reason = "mystery" }},
		{"policy", func(p *DecisionProvenance) { p.Records[0].Policy = "predict" }},
		{"gate", func(p *DecisionProvenance) { p.Records[0].GateOutcome = "maybe" }},
		{"applied-gate", func(p *DecisionProvenance) { p.Records[0].Applied = false }},
		{"policy-applied", func(p *DecisionProvenance) { p.Records[0].Policy = PolicyNone }},
		{"runner-range", func(p *DecisionProvenance) { p.Records[0].Runner = 5 }},
		{"clock-range", func(p *DecisionProvenance) { p.Records[0].ClockMS = 100 }},
		{"clock-grid", func(p *DecisionProvenance) { p.Records[0].ClockMS = 25; p.Records[0].Observation.ClockMS = 25 }},
		{"target-nan", func(p *DecisionProvenance) { p.Records[0].TargetX = math.NaN() }},
		{"target-out-of-envelope", func(p *DecisionProvenance) { p.Records[0].TargetX = 5.0 }},
		{"own-inf", func(p *DecisionProvenance) { p.Records[0].OwnY = math.Inf(1) }},
		{"fresh-unseen", func(p *DecisionProvenance) { p.Records[0].Observation.Seen = false }},
		{"stale-at-clock", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "stale"
		}},
		{"stale-future", func(p *DecisionProvenance) {
			p.Records[0].Observation.ClockMS = 40
		}},
		{"unknown-seen", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "unknown"
		}},
		{"unseen-owner", func(p *DecisionProvenance) {
			p.Records[0].Observation = ProvenanceObservation{Owner: 1}
			p.Records[0].Freshness = "unknown"
		}},
		{"held-owner-range", func(p *DecisionProvenance) {
			p.Records[0].Observation.Mode = "held"
			p.Records[0].Observation.Owner = 9
		}},
		{"unheld-owner", func(p *DecisionProvenance) {
			p.Records[0].Observation.Owner = 1
		}},
		{"bad-mode", func(p *DecisionProvenance) { p.Records[0].Observation.Mode = "flying" }},
		{"obs-nonfinite", func(p *DecisionProvenance) { p.Records[0].Observation.X = math.Inf(1) }},
		{"duplicate", func(p *DecisionProvenance) {
			p.Records = append(p.Records, p.Records[0])
			p.RecordCount = 2
		}},
		{"record-count-over-capacity", func(p *DecisionProvenance) {
			p.Capacity = 1
			p.Records = []DecisionRecord{p.Records[0], {
				ClockMS: 40, Runner: 1, Reason: ReasonPeriodic, Applied: true,
				GateOutcome: GateApplied, Policy: PolicyChase, Freshness: "fresh",
				Observation: ProvenanceObservation{Seen: true, ClockMS: 40, X: 0.5, Y: 0.5, Mode: "rolling", Owner: -1},
				TargetX:     0.5, TargetY: 0.5, OwnX: 0.5, OwnY: 0.5,
			}}
			p.RecordCount = 2
		}},
		{"simultaneous-nonperiodic", func(p *DecisionProvenance) {
			p.Records[0].Reason = ReasonLocalEvent
			p.Records[0].Simultaneous = true
		}},
		{"unseen-nan", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "unknown"
			p.Records[0].Observation = ProvenanceObservation{Seen: false, Owner: -1, Mode: "unknown", X: math.NaN()}
		}},
		{"unseen-inf-velocity", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "unknown"
			p.Records[0].Observation = ProvenanceObservation{Seen: false, Owner: -1, Mode: "unknown", VYM: math.Inf(1)}
		}},
		{"unseen-noncanonical-mode", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "unknown"
			p.Records[0].Observation = ProvenanceObservation{Seen: false, Owner: -1, Mode: "rolling"}
		}},
		{"unseen-negative-clock", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "unknown"
			p.Records[0].Observation = ProvenanceObservation{Seen: false, Owner: -1, Mode: "unknown", ClockMS: -1}
		}},
		{"seen-out-of-envelope", func(p *DecisionProvenance) {
			p.Records[0].Observation.X = 1.5
		}},
		{"seen-negative-clock", func(p *DecisionProvenance) {
			p.Records[0].Freshness = "stale"
			p.Records[0].Observation.ClockMS = -1
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := t049ValidExport()
			c.mutate(&p)
			if err := p.Validate(); !errors.Is(err, ErrProvenanceSchema) {
				t.Fatalf("Validate err=%v, want ErrProvenanceSchema", err)
			}
		})
	}
}

// --- 5. real fixture round-trip -------------------------------------------

// t049RealExport builds a small deterministic REAL export from the live path.
func t049RealExport(t *testing.T) DecisionProvenance {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners([]Runner{
		t049Local(t, 0.60, 0.50+2.0/PitchWidthM, 30.0),
		t049Local(t, 0.30, 0.30, 30.0),
	})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	exp, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 64, HorizonMS: 400})
	if err != nil {
		t.Fatalf("BuildDecisionProvenance: %v", err)
	}
	if len(exp.Records) == 0 {
		t.Fatal("real export captured no records")
	}
	return exp
}

const t049FixturePath = "testdata/t049-provenance.json"

// TestT049FixtureRoundTrip pins a REAL export as a versioned testdata fixture and
// round-trips it through JSON. Regenerate with T049_UPDATE_FIXTURE=1.
func TestT049FixtureRoundTrip(t *testing.T) {
	built := t049RealExport(t)
	if err := built.Validate(); err != nil {
		t.Fatalf("built export invalid: %v", err)
	}
	blob, err := json.MarshalIndent(built, "", "  ")
	if err != nil {
		t.Fatalf("marshal indent: %v", err)
	}
	blob = append(blob, '\n')
	if os.Getenv("T049_UPDATE_FIXTURE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(t049FixturePath, blob, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		t.Logf("wrote fixture %s (%d bytes)", t049FixturePath, len(blob))
	}
	want, err := os.ReadFile(t049FixturePath)
	if err != nil {
		t.Fatalf("read fixture (regenerate with T049_UPDATE_FIXTURE=1): %v", err)
	}
	if string(want) != string(blob) {
		t.Fatalf("fixture drifted from the built export (regenerate with T049_UPDATE_FIXTURE=1)")
	}
	var back DecisionProvenance
	if err := json.Unmarshal(want, &back); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	if !reflect.DeepEqual(back, built) {
		t.Fatalf("JSON round-trip changed the export:\n want=%+v\n got =%+v", built, back)
	}
	t.Logf("real fixture round-trip: schema v%d, %d records, %d bytes", back.SchemaVersion, back.RecordCount, len(want))
}

// t049FirstStaleClock runs a scene with a throwaway recorder and returns the
// clock of the first STALE publication, or -1.
func t049FirstStaleClock(t *testing.T, cfg Config, build func(*Sim), mid func(*Sim, int), horizon int) int {
	t.Helper()
	_, recs := t049Recorded(t, cfg, 1, build, horizon, 512, mid)
	for _, r := range recs {
		if r.Freshness == "stale" {
			return r.ClockMS
		}
	}
	return -1
}

// t049AssertResumeTail is the core equivalence check: step the straight run to
// `split`, JSON round-trip the Snapshot, attach a FRESH recorder to the resumed
// sim, run BOTH to `horizon` comparing the COMPLETE snapshot every substep and the
// final snapshot, then assert the resumed record tail exactly equals the straight
// absolute-time tail (clockMs >= split), is nonempty and has no duplicate
// (clockMs, runner).
func t049AssertResumeTail(t *testing.T, cfg Config, build func(*Sim), mid func(*Sim, int), horizon, split int, label string) {
	t.Helper()
	straight := mustSim(t, cfg, 1)
	build(straight)
	recS, err := NewProvenanceRecorder(4096)
	if err != nil {
		t.Fatal(err)
	}
	straight.SetProvenanceRecorder(recS)
	for straight.ClockMS() < split {
		if mid != nil {
			mid(straight, straight.ClockMS())
		}
		straight.Step()
	}
	if straight.ClockMS() != split {
		t.Fatalf("%s: split %d is not a substep boundary (clock %d)", label, split, straight.ClockMS())
	}
	blob, err := json.Marshal(straight.Snapshot())
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(blob, &snap); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	resumed, err := Resume(snap)
	if err != nil {
		t.Fatalf("%s: resume: %v", label, err)
	}
	if resumed.ProvenanceRecorder() != nil {
		t.Fatalf("%s: resume attached a recorder", label)
	}
	recR, err := NewProvenanceRecorder(4096)
	if err != nil {
		t.Fatal(err)
	}
	resumed.SetProvenanceRecorder(recR)
	for straight.ClockMS() < horizon {
		if straight.ClockMS() != resumed.ClockMS() {
			t.Fatalf("%s: clocks diverged %d vs %d", label, straight.ClockMS(), resumed.ClockMS())
		}
		if mid != nil {
			mid(straight, straight.ClockMS())
			mid(resumed, resumed.ClockMS())
		}
		straight.Step()
		resumed.Step()
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("%s: complete snapshot diverged at %d", label, straight.ClockMS())
		}
	}
	if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
		t.Fatalf("%s: final snapshot differs", label)
	}
	var want []DecisionRecord
	for _, r := range recS.Records() {
		if r.ClockMS >= split {
			want = append(want, r)
		}
	}
	if len(want) == 0 {
		t.Fatalf("%s split %d: empty absolute-time tail", label, split)
	}
	if !reflect.DeepEqual(want, recR.Records()) {
		t.Fatalf("%s split %d: resumed tail differs from the straight absolute tail:\n want=%+v\n got =%+v",
			label, split, want, recR.Records())
	}
	dup := map[[2]int]bool{}
	for _, r := range recR.Records() {
		k := [2]int{r.ClockMS, r.Runner}
		if dup[k] {
			t.Fatalf("%s split %d: duplicate record at %d/%d", label, split, r.ClockMS, r.Runner)
		}
		dup[k] = true
	}
}

// TestT049ResumeTailEquivalence is table-driven over the required scene
// boundaries and splits 80/100/120: a coalesced deflect boundary @20, the
// reaction pending/drop/deadline scene, and a genuinely seen-then-stale scene
// (split at the actual stale boundary). Each case compares complete snapshots
// every substep and the final snapshot, and asserts a nonempty, exactly-equal,
// duplicate-free absolute-time record tail.
func TestT049ResumeTailEquivalence(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)

	coalesceBuild := func(sim *Sim) {
		w := t049Local(t, 0.93, 0.50, 20.0)
		sim.SetRunners([]Runner{w, {X: 0.990, Y: 0.50, ReachM: ControlRadiusM}})
		sim.controlError = 1.0
		sim.SetBall(Ball{Mode: BallRolling, X: 0.9985, Y: 0.50, VXM: 40.0})
	}
	reactionBuild := func(sim *Sim) {
		r := t049Local(t, 0.50, 0.50, 2.0)
		r.ReactMS = 300
		r.ReactAtMS = 0
		sim.SetRunners([]Runner{r})
		sim.SetBall(Ball{Mode: BallLoose, X: 0.90, Y: 0.90})
	}
	reactionMid := func(s *Sim, clock int) {
		if clock == 60 {
			s.ReleasePass(PassParams{StartX: 0.505, StartY: 0.505, TargetX: 0.52, TargetY: 0.505, SpeedMPS: 0.5, Passer: -1})
		}
	}
	staleBuild := func(sim *Sim) {
		r := t049Local(t, 0.50, 0.50+1.5/PitchWidthM, 2.0)
		r.ReachM = 0.05
		sim.SetRunners([]Runner{r})
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	}
	staleBoundary := t049FirstStaleClock(t, cfg, staleBuild, nil, 4000)
	if staleBoundary < 0 {
		t.Fatal("genuinely-stale scene never produced a stale publication (vacuous)")
	}

	type scene struct {
		name    string
		build   func(*Sim)
		mid     func(*Sim, int)
		horizon int
		splits  []int
	}
	scenes := []scene{
		{"coalesced-deflect-20", coalesceBuild, nil, 400, []int{20, 80, 100, 120}},
		{"reaction-pending-drop-deadline", reactionBuild, reactionMid, 600, []int{60, 80, 100, 120, 360}},
		{"seen-then-stale", staleBuild, nil, 4000, []int{80, 100, 120, staleBoundary, staleBoundary + 20}},
	}
	for _, sc := range scenes {
		for _, split := range sc.splits {
			t.Run(fmt.Sprintf("%s/split%d", sc.name, split), func(t *testing.T) {
				t049AssertResumeTail(t, cfg, sc.build, sc.mid, sc.horizon, split, sc.name)
			})
		}
	}
	t.Logf("resume tail equivalence: coalesced@20, reaction 60/80/100/120/360, stale boundary @%d", staleBoundary)
}

// TestT049DumpExample emits a small real JSON excerpt for the evidence note.
func TestT049DumpExample(t *testing.T) {
	built := t049RealExport(t)
	blob, err := json.MarshalIndent(built, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const limit = 1600
	out := string(blob)
	if len(out) > limit {
		out = out[:limit] + "\n... (truncated)"
	}
	t.Logf("T049 REAL EXPORT EXAMPLE:\n%s", out)
}
