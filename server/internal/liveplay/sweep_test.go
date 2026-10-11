package liveplay

import "testing"

// candidateSubsteps are the fixed-step candidates evaluated against the 1 ms
// numerical reference. Values in integer game milliseconds.
var candidateSubsteps = []int{200, 100, 50, 40, 25, 20, 10, 5}

// Fixture-grid sizes (guarded so the evidence count cannot silently drift):
// static = 9 speeds × (6 inside + 3 outside offsets) = 81;
// reach  = 9 speeds × 2 receiver speeds × 8 offsets = 144; total 225.
const (
	staticFixtureCount = 81
	reachFixtureCount  = 144
	totalFixtureCount  = staticFixtureCount + reachFixtureCount
)

// contactTime runs a scene to collection (or the horizon) and returns the first
// contact time and runner. Quiet stepping avoids per-substep sample allocation.
func contactTime(t *testing.T, cfg Config, ball Ball, runners []Runner, untilMS int) (int, int) {
	t.Helper()
	sim, err := NewSim(cfg, 1)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim.SetBall(ball)
	sim.SetRunners(runners)
	for sim.ClockMS() < untilMS {
		sim.StepQuiet()
		if sim.Ball().Mode == BallHeld {
			ms, runner := sim.FirstContact()
			return ms, runner
		}
	}
	return -1, -1
}

// sweepMetrics aggregates one candidate's fit to the reference.
type sweepMetrics struct {
	substepMS  int
	fixtures   int
	misses     int // reference contact, candidate none (tunneling)
	falsePos   int // candidate contact, reference none
	maxTimeErr int // ms
	sumTimeErr int
	timed      int
	classErr   int
}

func (m sweepMetrics) meanTimeErr() float64 {
	if m.timed == 0 {
		return 0
	}
	return float64(m.sumTimeErr) / float64(m.timed)
}

type fixtureTruth struct {
	refMS  int
	hasRef bool
	cands  []struct {
		ms    int
		found bool
	}
}

// runSweep evaluates every candidate against the reference over both fixture
// families and returns the metrics.
func runSweep(t *testing.T) []sweepMetrics {
	t.Helper()

	speeds := []float64{6, 10, 14, 18, 22, 26, 30, 34, 40}
	insideOffsets := []float64{0.0, 0.2, 0.4, 0.6, 0.8, 0.9}
	outsideOffsets := []float64{1.1, 1.3, 1.6}
	recvSpeeds := []float64{4, 8}
	recvOffsets := []float64{1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 8.0, 12.0}

	ref := ReferenceConfig()
	var truths []fixtureTruth

	eval := func(ball Ball, runners []Runner) {
		refMS, _ := contactTime(t, ref, ball, runners, 12000)
		tr := fixtureTruth{refMS: refMS, hasRef: refMS >= 0}
		for _, cs := range candidateSubsteps {
			ms, _ := contactTime(t, Config{SubstepMS: cs, DecelMPS2: RollDecelMPS2}, ball, runners, 12000)
			tr.cands = append(tr.cands, struct {
				ms    int
				found bool
			}{ms, ms >= 0})
		}
		truths = append(truths, tr)
	}

	for _, v := range speeds {
		for _, off := range append(append([]float64(nil), insideOffsets...), outsideOffsets...) {
			eval(
				Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: v},
				[]Runner{{X: 0.55, Y: 0.50 + off/PitchWidthM, ReachM: ControlRadiusM}},
			)
		}
	}
	for _, v := range speeds {
		for _, u := range recvSpeeds {
			for _, off := range recvOffsets {
				eval(
					Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: v},
					[]Runner{{X: 0.55, Y: 0.50 + off/PitchWidthM, VYM: -u, ReachM: ControlRadiusM}},
				)
			}
		}
	}

	if len(truths) != totalFixtureCount {
		t.Fatalf("fixture count = %d, want %d", len(truths), totalFixtureCount)
	}
	verifyFixtureCounts(t)

	metrics := make([]sweepMetrics, len(candidateSubsteps))
	for i, cs := range candidateSubsteps {
		metrics[i].substepMS = cs
	}
	for _, tr := range truths {
		for i := range candidateSubsteps {
			m := &metrics[i]
			m.fixtures++
			c := tr.cands[i]
			switch {
			case tr.hasRef && !c.found:
				m.misses++
				m.classErr++
			case !tr.hasRef && c.found:
				m.falsePos++
				m.classErr++
			case tr.hasRef && c.found:
				err := c.ms - tr.refMS
				if err < 0 {
					err = -err
				}
				if err > m.maxTimeErr {
					m.maxTimeErr = err
				}
				m.sumTimeErr += err
				m.timed++
			}
		}
	}

	for _, m := range metrics {
		t.Logf("dt=%3d ms | fixtures=%3d misses=%d falsePos=%d classErr=%d timeErr mean=%.2f max=%d ms",
			m.substepMS, m.fixtures, m.misses, m.falsePos, m.classErr, m.meanTimeErr(), m.maxTimeErr)
	}
	return metrics
}

// verifyFixtureCounts anchors the evidence's fixture arithmetic.
func verifyFixtureCounts(t *testing.T) {
	t.Helper()
	const speeds, offsets, recvSpeeds, recvOffsets = 9, 9, 2, 8
	if static := speeds * offsets; static != staticFixtureCount {
		t.Fatalf("static fixture count = %d, want %d", static, staticFixtureCount)
	}
	if reach := speeds * recvSpeeds * recvOffsets; reach != reachFixtureCount {
		t.Fatalf("reach fixture count = %d, want %d", reach, reachFixtureCount)
	}
}

// TestSubstepSweepChoosesOnErrorEvidence evaluates the candidates and enforces
// the numeric limits accepted for the recommended clock.
func TestSubstepSweepChoosesOnErrorEvidence(t *testing.T) {
	metrics := runSweep(t)

	var chosen *sweepMetrics
	for i := range metrics {
		if metrics[i].substepMS == SubstepMS {
			chosen = &metrics[i]
		}
	}
	if chosen == nil {
		t.Fatalf("recommended SubstepMS=%d not in the sweep set", SubstepMS)
	}
	if chosen.misses != RequiredTunnelingMisses {
		t.Fatalf("dt=%d ms: %d/%d missed contacts (tunneling), limit %d", chosen.substepMS, chosen.misses, chosen.fixtures, RequiredTunnelingMisses)
	}
	if chosen.falsePos != RequiredFalseContacts {
		t.Fatalf("dt=%d ms: %d false contacts, limit %d", chosen.substepMS, chosen.falsePos, RequiredFalseContacts)
	}
	if chosen.classErr != RequiredClassErrors {
		t.Fatalf("dt=%d ms: %d reachability misclassifications, limit %d", chosen.substepMS, chosen.classErr, RequiredClassErrors)
	}
	if chosen.maxTimeErr > MaxCrossingTimeErrorMS {
		t.Fatalf("dt=%d ms: crossing-time error %d ms exceeds limit %d ms", chosen.substepMS, chosen.maxTimeErr, MaxCrossingTimeErrorMS)
	}
	worse := false
	for _, m := range metrics {
		if m.substepMS > SubstepMS && (m.misses > 0 || m.falsePos > 0 || m.classErr > 0) {
			worse = true
		}
	}
	if !worse {
		t.Log("NOTE: no coarser candidate failed the limits; revisit cost optimum")
	}
}

// TestGrazingGlanceCanTunnel documents the spike's hard limitation: a shallow,
// near-tangent trajectory is inside the reach disc for only a tiny arc, so a
// finite fixed step can step over it entirely. This scans grazing offsets and
// requires at least one offset caught by the 1 ms reference but missed by 20 ms.
// The clean sweep result is a property of the finite offset grid, NOT a universal
// no-tunneling guarantee; an analytic/swept contact check is deferred to T-038.
func TestGrazingGlanceCanTunnel(t *testing.T) {
	refCaught, miss20, miss5 := 0, 0, 0
	for i := 0; i <= 400; i++ {
		offsetM := 0.9980 + float64(i)*0.0000045 // ~0.9980 .. 0.9998
		ball := Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: ShotMaxMPS}
		runners := []Runner{{X: 0.55, Y: 0.50 + offsetM/PitchWidthM, ReachM: ControlRadiusM}}

		if refMS, _ := contactTime(t, ReferenceConfig(), ball, runners, 12000); refMS >= 0 {
			refCaught++
			if ms, _ := contactTime(t, Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2}, ball, runners, 12000); ms < 0 {
				miss20++
			}
			if ms, _ := contactTime(t, Config{SubstepMS: 5, DecelMPS2: RollDecelMPS2}, ball, runners, 12000); ms < 0 {
				miss5++
			}
		}
	}
	if refCaught == 0 {
		t.Fatal("reference caught no grazing glance in the scan")
	}
	if miss20 == 0 {
		t.Fatalf("20 ms caught every reference glance (%d); limitation not demonstrated", refCaught)
	}
	t.Logf("grazing scan: reference caught %d, 20 ms missed %d, 5 ms missed %d", refCaught, miss20, miss5)
}

// TestContactTieRule: with two runners in reach at the same distance, the LOWEST
// index wins (stable, deterministic). Then a strictly nearer runner wins.
func TestContactTieRule(t *testing.T) {
	// Two runners symmetric about the ball path, both exactly in reach at the
	// same distance on the step they are first seen.
	ball := Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 0.01}
	sim, err := NewSim(Config{SubstepMS: 20, DecelMPS2: 0}, 1)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim.SetBall(ball)
	sim.SetRunners([]Runner{
		{X: 0.50, Y: 0.50 + 0.9/PitchWidthM, ReachM: ControlRadiusM},
		{X: 0.50, Y: 0.50 - 0.9/PitchWidthM, ReachM: ControlRadiusM},
	})
	sim.StepQuiet()
	_, runner := sim.FirstContact()
	if runner != 0 {
		t.Fatalf("tie winner = %d, want 0 (lowest index)", runner)
	}

	// A strictly nearer runner later in the slice must win.
	sim2, err := NewSim(Config{SubstepMS: 20, DecelMPS2: 0}, 1)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim2.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 0.01})
	sim2.SetRunners([]Runner{
		{X: 0.50, Y: 0.50 + 0.95/PitchWidthM, ReachM: ControlRadiusM},
		{X: 0.50, Y: 0.50 + 0.05/PitchWidthM, ReachM: ControlRadiusM},
	})
	sim2.StepQuiet()
	if _, runner := sim2.FirstContact(); runner != 1 {
		t.Fatalf("nearest winner = %d, want 1", runner)
	}
}

// TestSubstepCostMonotone sanity-checks that finer clocks cost more substeps.
func TestSubstepCostMonotone(t *testing.T) {
	const base = 1000
	for _, cs := range []int{200, 100, 50, 25, 20, 10} {
		if base/cs <= 0 {
			t.Fatalf("dt=%d ms yields no substeps over a second", cs)
		}
	}
}
