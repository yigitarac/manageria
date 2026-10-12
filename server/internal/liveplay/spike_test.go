package liveplay

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func mustSim(t *testing.T, cfg Config, seed uint64) *Sim {
	t.Helper()
	sim, err := NewSim(cfg, seed)
	if err != nil {
		t.Fatalf("NewSim(%+v): %v", cfg, err)
	}
	return sim
}

// spikeScene is the shared deterministic ground-pass contest: a ball released
// down the pitch toward a receiver running back to meet it.
func spikeScene(t *testing.T) *Sim {
	t.Helper()
	sim := mustSim(t, DefaultConfig(), 42)
	sim.ReleaseBall(ReleaseParams{
		StartX: 0.35, StartY: 0.50,
		SpeedMPS:         16.0,
		LateralJitterMPS: 0.2,
		SpeedJitterFrac:  0.0,
	})
	sim.SetRunners([]Runner{{X: 0.60, Y: 0.50, VXM: -6.0, VYM: 0, ReachM: ControlRadiusM}})
	return sim
}

// TestSpikeDeterministic: the same scene twice produces identical contact time,
// runner index and full sample stream.
func TestSpikeDeterministic(t *testing.T) {
	a, b := spikeScene(t), spikeScene(t)
	a.Run(4000)
	b.Run(4000)

	am, ar := a.FirstContact()
	bm, br := b.FirstContact()
	if am < 0 || ar < 0 {
		t.Fatalf("scene never collected the ball (contactMS=%d runner=%d)", am, ar)
	}
	if am != bm || ar != br {
		t.Fatalf("nondeterministic contact: (%d,%d) vs (%d,%d)", am, ar, bm, br)
	}
	if !reflect.DeepEqual(a.Samples(), b.Samples()) {
		t.Fatal("nondeterministic sample stream")
	}
}

// TestSpikeSameClockTrace pins the clock and the exact first-contact moment, and
// asserts the event time and the sample time of the same step AGREE (the review
// defect: the old code stamped the event one substep before its sample).
func TestSpikeSameClockTrace(t *testing.T) {
	const wantFirstContactMS = 1180 // pinned 2026-10-11 (T-035 review: end-step stamp)
	sim := spikeScene(t)
	sim.Run(4000)
	got, runner := sim.FirstContact()
	if got != wantFirstContactMS || runner != 0 {
		t.Fatalf("first contact = (%d ms, runner %d), want (%d ms, 0)", got, runner, wantFirstContactMS)
	}
	samples := sim.Samples()
	if len(samples) == 0 {
		t.Fatal("no samples recorded")
	}
	contactSample := -1
	for i, smp := range samples {
		if want := (i + 1) * SubstepMS; smp.ClockMS != want {
			t.Fatalf("sample %d clock = %d, want %d (fixed %d ms grid)", i, smp.ClockMS, want, SubstepMS)
		}
		if smp.Mode == BallHeld && contactSample < 0 {
			contactSample = i
		}
	}
	if contactSample < 0 {
		t.Fatal("no held-ball sample after collection")
	}
	// Event time == the clock of the first held sample.
	if samples[contactSample].ClockMS != got {
		t.Fatalf("event time %d ms != contact sample time %d ms", got, samples[contactSample].ClockMS)
	}
	if contactSample == 0 || samples[contactSample-1].Mode != BallRolling {
		t.Fatal("the sample immediately before contact must be a rolling ball")
	}
	if samples[contactSample].Owner != runner {
		t.Fatalf("held sample owner = %d, want %d", samples[contactSample].Owner, runner)
	}
}

// TestSpikeOwnerFollowsBall: after collection the ball is glued to the owner's
// moving feet for every subsequent sample (explicit continuation behaviour; the
// alternative would be an explicit stop limitation).
func TestSpikeOwnerFollowsBall(t *testing.T) {
	sim := spikeScene(t)
	sim.Run(4000)
	contactMS, runner := sim.FirstContact()
	if contactMS < 0 {
		t.Fatal("no contact")
	}
	// Continue well past contact; the receiver keeps running (-6 m/s along x).
	for i := 0; i < 20; i++ {
		sim.Step()
	}
	ball := sim.Ball()
	owner := sim.Runners()[runner]
	if ball.Mode != BallHeld {
		t.Fatalf("ball mode after contact = %v, want held", ball.Mode)
	}
	if math.Abs(ball.X-owner.X) > 1e-12 || math.Abs(ball.Y-owner.Y) > 1e-12 {
		t.Fatalf("held ball (%.6f,%.6f) not at owner feet (%.6f,%.6f)", ball.X, ball.Y, owner.X, owner.Y)
	}
	samples := sim.Samples()
	last := samples[len(samples)-1]
	if last.Mode != BallHeld || last.Owner != runner {
		t.Fatalf("final held sample mode=%v owner=%d, want held/%d", last.Mode, last.Owner, runner)
	}
	// The owner moved, so the held ball must have moved to its current feet.
	if math.Abs(last.BallX-owner.X) > 1e-12 || math.Abs(last.BallY-owner.Y) > 1e-12 {
		t.Fatalf("last held sample ball (%.6f,%.6f) not at owner feet (%.6f,%.6f)",
			last.BallX, last.BallY, owner.X, owner.Y)
	}
}

// TestSpikeSnapshotContinuation: snapshot mid-flight, resume, and prove the
// remainder is bit-identical to the straight run, including RNG state.
func TestSpikeSnapshotContinuation(t *testing.T) {
	straight := spikeScene(t)
	for straight.ClockMS() < 600 {
		straight.Step()
	}
	resumed, err := Resume(straight.Snapshot())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	straight.Run(4000)
	resumed.Run(4000)

	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 {
		t.Fatal("resumed run produced more samples than the straight run")
	}
	if !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatal("resumed tail differs from straight run")
	}
	sm, sr := straight.FirstContact()
	rm, rr := resumed.FirstContact()
	if sm != rm || sr != rr {
		t.Fatalf("resumed contact (%d,%d) != straight (%d,%d)", rm, rr, sm, sr)
	}
	if straight.RNGState() != resumed.RNGState() {
		t.Fatal("resumed RNG state diverged")
	}
}

// TestSpikeSnapshotJSONRoundTrip: the snapshot schema is serializable and a
// JSON round-trip resumes identically (the production snapshot contract).
func TestSpikeSnapshotJSONRoundTrip(t *testing.T) {
	straight := spikeScene(t)
	straight.Run(500)
	snap := straight.Snapshot()

	blob, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Snapshot
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(snap, back) {
		t.Fatal("snapshot JSON round-trip lost state")
	}
	resumed, err := Resume(back)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	straight.Run(4000)
	resumed.Run(4000)
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 {
		t.Fatal("resumed run produced more samples than the straight run")
	}
	if !reflect.DeepEqual(all[head:], resumed.Samples()) {
		t.Fatal("JSON-round-tripped resume diverged")
	}
}

// TestSpikeTwoReleasesAndResumeRNG: a second ReleaseBall must reset the stale
// first-contact record, and Resume must NOT reset the RNG stream, so a second
// release after a snapshot consumes the same draws as the straight run.
func TestSpikeTwoReleasesAndResumeRNG(t *testing.T) {
	straight := mustSim(t, DefaultConfig(), 7)
	p1 := ReleaseParams{StartX: 0.35, StartY: 0.50, SpeedMPS: 16.0, LateralJitterMPS: 0.2, SpeedJitterFrac: 0.05}
	straight.ReleaseBall(p1)
	straight.SetRunners([]Runner{{X: 0.60, Y: 0.50, VXM: -6.0, ReachM: ControlRadiusM}})
	straight.Run(4000)
	if ms, r := straight.FirstContact(); ms < 0 || r != 0 {
		t.Fatalf("first release did not collect: (%d,%d)", ms, r)
	}
	snap := straight.Snapshot()
	resumed, err := Resume(snap)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	// Second release resets stale contact state on both paths.
	p2 := ReleaseParams{StartX: 0.25, StartY: 0.45, SpeedMPS: 18.0, LateralJitterMPS: 0.3, SpeedJitterFrac: 0.05}
	straight.ReleaseBall(p2)
	resumed.ReleaseBall(p2)
	if ms, _ := straight.FirstContact(); ms != -1 {
		t.Fatalf("second release left stale contact time %d", ms)
	}
	if !reflect.DeepEqual(straight.Ball(), resumed.Ball()) {
		t.Fatalf("second release ball diverged: %+v vs %+v", straight.Ball(), resumed.Ball())
	}
	if straight.RNGState() != resumed.RNGState() {
		t.Fatal("Resume reset or diverged the RNG stream")
	}
}

// TestConfigValidation: a non-positive substep or non-finite/negative
// deceleration is rejected by both constructors (prevents a non-advancing Run).
func TestConfigValidation(t *testing.T) {
	bad := []Config{
		{SubstepMS: 0, DecelMPS2: RollDecelMPS2},
		{SubstepMS: -20, DecelMPS2: RollDecelMPS2},
		{SubstepMS: 20, DecelMPS2: -1},
		{SubstepMS: 20, DecelMPS2: math.Inf(1)},
		{SubstepMS: 20, DecelMPS2: math.NaN()},
	}
	for _, cfg := range bad {
		if _, err := NewSim(cfg, 1); err == nil {
			t.Fatalf("NewSim(%+v) accepted an invalid config", cfg)
		} else if !errors.Is(err, ErrSubstep) && !errors.Is(err, ErrDecel) {
			t.Fatalf("NewSim(%+v) error = %v, want ErrSubstep/ErrDecel", cfg, err)
		}
		if _, err := Resume(Snapshot{Cfg: cfg, RNG: [4]uint64{1, 2, 3, 4}}); err == nil {
			t.Fatalf("Resume accepted invalid config %+v", cfg)
		}
	}
	if _, err := NewSim(DefaultConfig(), 1); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

// TestSpikeDrawSites: the declared draw-site order and labels are stable, and
// labels are immutable (String method, no exported mutable state). T-037 adds
// the live-pass direction/weight sites; the legacy lateral/speed order is
// unchanged.
func TestSpikeDrawSites(t *testing.T) {
	if drawSiteCount != 6 {
		t.Fatalf("draw site count = %d, want 6 (update evidence note)", drawSiteCount)
	}
	if DrawReleaseLateral.String() != "release_lateral" || DrawReleaseSpeed.String() != "release_speed" {
		t.Fatal("declared legacy draw-site labels changed")
	}
	if DrawPassDirection.String() != "pass_direction" || DrawPassWeight.String() != "pass_weight" {
		t.Fatal("declared live-pass draw-site labels changed")
	}
	if DrawContactControl.String() != "contact_control" || DrawDeflectLateral.String() != "deflect_lateral" {
		t.Fatal("declared live-contest draw-site labels changed")
	}
	if DrawSite(99).String() != "unknown" {
		t.Fatal("unknown draw site must not panic and must read 'unknown'")
	}
	if DrawReleaseLateral >= DrawReleaseSpeed || DrawReleaseSpeed >= DrawPassDirection ||
		DrawPassDirection >= DrawPassWeight || DrawPassWeight >= DrawContactControl ||
		DrawContactControl >= DrawDeflectLateral {
		t.Fatal("declared draw order changed")
	}
}
