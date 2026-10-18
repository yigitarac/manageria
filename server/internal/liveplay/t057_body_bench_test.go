package liveplay

import (
	"encoding/json"
	"math"
	"testing"
)

// T-057 body-enabled benchmarks (isolated prototype evidence, TEST-ONLY). Fresh
// deterministic baselines are built OUTSIDE the timer; the body path uses a full
// transactional clone, so the clone/staging cost is part of the measured TOTAL.
// Every body step is checked and b.Fatal on error. Matching scenes permit
// qualified deltas/ratios; `<20 ms` full-match readiness is not claimed.

// t057ImpactRunners is the SINGLE shared impact fixture (used by the guard and the
// benchmark): two head-on colliding pairs plus 18 movers, all sharing a nonzero
// +y tangential velocity, with metric-aligned far targets.
func t057ImpactRunners() []Runner {
	var rs []Runner
	id := uint64(1)
	top := math.Sqrt(5*5 + 0.3*0.3)
	add := func(x, y, vx, vy float64) {
		r := InertialRunner(x, y, x+vx*50/PitchLengthM, y+vy*50/PitchWidthM, t057Params(top))
		r.PlayerID = id
		r.Body = t057Body(0.30, 75)
		r.VXM, r.VYM = vx, vy
		rs = append(rs, r)
		id++
	}
	add(0.5, 0.40, 5, 0.3)
	add(0.5+0.7/PitchLengthM, 0.40, -5, 0.3)
	add(0.5, 0.60, 5, 0.3)
	add(0.5+0.7/PitchLengthM, 0.60, -5, 0.3)
	for k := range 18 {
		x := 0.05 + float64(k%9)*0.10
		y := 0.10 + float64(k/9)*0.75
		add(x, y, 5, 0.3)
	}
	return rs
}

func t057BenchBody22(b *testing.B, seed uint64, control, withRec bool) *Sim {
	sim, err := NewSim(ScheduledLivePassConfig(100), seed)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	rs := t055AssignIDs(t053Runners(control))
	for i := range rs {
		rs[i].Body = t057Body(0.30, 75)
	}
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		b.Fatal(err)
	}
	if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
		b.Fatal(err)
	}
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
	t057PreallocBench(sim, b, withRec)
	return sim
}

func t057PreallocBench(sim *Sim, b *testing.B, withRec bool) {
	sim.ensurePreCapacity(len(sim.runners))
	if sim.localGate == nil {
		sim.localGate = make([]bool, len(sim.runners))
	}
	if sim.localPubMS == nil {
		sim.localPubMS = make([]int, len(sim.runners))
	}
	for i := range sim.localPubMS {
		sim.localPubMS[i] = -1
	}
	if withRec {
		sim.SetProvenanceRecorder(mustRecorderB(b, 8192))
	}
}

// t057AdvanceCheckedWindow advances the shared window with the SAME scripted mid
// hook, failing the benchmark on any checked-step error (never hides a fault).
func t057AdvanceCheckedWindow(sim *Sim, control bool, b *testing.B) {
	for k := 0; k < t053WindowSteps; k++ {
		if control && k == 50 {
			sim.ReleasePass(t053SecondReleaseParams())
		}
		if err := sim.StepChecked(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkT057Window(b *testing.B, control, withRec bool) {
	seed := uint64(t053ControlSeed)
	if !control {
		seed = t053DeflectSeed
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t057BenchBody22(b, seed, control, withRec)
		b.StartTimer()
		t057AdvanceCheckedWindow(sim, control, b)
	}
}

func BenchmarkT057BodyControl22(b *testing.B)         { benchmarkT057Window(b, true, false) }
func BenchmarkT057BodyControlRecorder22(b *testing.B) { benchmarkT057Window(b, true, true) }
func BenchmarkT057BodyDeflect22(b *testing.B)         { benchmarkT057Window(b, false, false) }
func BenchmarkT057BodyDeflectRecorder22(b *testing.B) { benchmarkT057Window(b, false, true) }

func t057BenchBaseline22(b *testing.B, seed uint64, control bool) *Sim {
	sim, err := NewSim(ScheduledLivePassConfig(100), seed)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.SetRunners(t055AssignIDs(t053Runners(control)))
	if err := sim.EnableIntegrated(); err != nil {
		b.Fatal(err)
	}
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
	t057PreallocBench(sim, b, false)
	return sim
}

func BenchmarkT057BaselineControl22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t057BenchBaseline22(b, t053ControlSeed, true)
		b.StartTimer()
		t053AdvanceWindow(sim, true, nil)
	}
}

func BenchmarkT057BaselineDeflect22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t057BenchBaseline22(b, t053DeflectSeed, false)
		b.StartTimer()
		t053AdvanceWindow(sim, false, nil)
	}
}

func t057BenchImpact(b *testing.B, withRec bool) *Sim {
	sim, err := NewSim(LivePassConfig(), 3)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.SetRunners(t057ImpactRunners())
	if err := sim.EnableIntegrated(); err != nil {
		b.Fatal(err)
	}
	if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
		b.Fatal(err)
	}
	t057PreallocBench(sim, b, withRec)
	return sim
}

func BenchmarkT057BodyImpact22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t057BenchImpact(b, false)
		b.StartTimer()
		for k := 0; k < t053WindowSteps; k++ {
			if err := sim.StepChecked(); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkT057BodyImpactRecorder22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t057BenchImpact(b, true)
		b.StartTimer()
		for k := 0; k < t053WindowSteps; k++ {
			if err := sim.StepChecked(); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkT057BaselineImpact22: the matching impact roster with body DISABLED.
func BenchmarkT057BaselineImpact22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim, err := NewSim(LivePassConfig(), 3)
		if err != nil {
			b.Fatal(err)
		}
		sim.samples = make([]Sample, 0, 4096)
		sim.events = make([]PassEvent, 0, 256)
		rs := t057ImpactRunners()
		for i := range rs {
			rs[i].Body = nil
		}
		sim.SetRunners(rs)
		if err := sim.EnableIntegrated(); err != nil {
			b.Fatal(err)
		}
		t057PreallocBench(sim, b, false)
		b.StartTimer()
		for k := 0; k < t053WindowSteps; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT057ProvenanceExport: the ACTUAL checked provenance builder (clone +
// checked stepping + sidecar capture) + JSON marshal, with nonempty records.
func BenchmarkT057ProvenanceExport(b *testing.B) {
	sim := t057BenchBody22(b, t053DeflectSeed, false, false)
	for k := 0; k < 20; k++ {
		if err := sim.StepChecked(); err != nil {
			b.Fatal(err)
		}
	}
	start := sim.ClockMS()
	bytes, recs := 0, 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prov, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 4096, HorizonMS: start + 200})
		if err != nil {
			b.Fatal(err)
		}
		blob, err := json.Marshal(prov)
		if err != nil {
			b.Fatal(err)
		}
		bytes, recs = len(blob), prov.RecordCount
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
	b.ReportMetric(float64(recs), "records")
}

// BenchmarkT057SnapshotResumeAdvanceMarshal: offline snapshot+Resume+advance+marshal
// (NOT a provenance export; named truthfully).
func BenchmarkT057SnapshotResumeAdvanceMarshal(b *testing.B) {
	sim := t057BenchBody22(b, t053DeflectSeed, false, false)
	for k := 0; k < 20; k++ {
		if err := sim.StepChecked(); err != nil {
			b.Fatal(err)
		}
	}
	start := sim.ClockMS()
	bytes := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clone, err := Resume(sim.Snapshot())
		if err != nil {
			b.Fatal(err)
		}
		for clone.ClockMS() < start+200 {
			if err := clone.StepChecked(); err != nil {
				b.Fatal(err)
			}
		}
		blob, err := json.Marshal(clone.Snapshot())
		if err != nil {
			b.Fatal(err)
		}
		bytes = len(blob)
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
}

// TestT057ImpactWindowGuard (guard): the shared impact window has genuine contacts
// at the exact analytic TOI=10 ms (frac=0.5) on known colliding pairs (1,2) and
// (3,4) with nonzero tangential motion (0.3 m/s), all22 move every microstep, and
// three fresh same-seed windows are identical.
func TestT057ImpactWindowGuard(t *testing.T) {
	var inits, finals []Snapshot
	for range 3 {
		sim := mustSim(t, LivePassConfig(), 3)
		sim.SetRunners(t057ImpactRunners())
		if err := sim.EnableIntegrated(); err != nil {
			t.Fatalf("enable: %v", err)
		}
		if err := sim.EnableBodyContacts(t057HardConfig()); err != nil {
			t.Fatalf("body enable: %v", err)
		}
		inits = append(inits, sim.Snapshot())
		prev := sim.Runners()
		for k := range t053WindowSteps {
			if err := sim.StepChecked(); err != nil {
				t.Fatalf("impact step %d: %v", k, err)
			}
			cur := sim.Runners()
			t053AssertAllMoved(t, "impact", prev, cur, k)
			prev = cur
		}
		finals = append(finals, sim.Snapshot())
		if len(sim.body.Impacts) < 2 {
			t.Fatalf("impact window produced %d contacts, want >= 2", len(sim.body.Impacts))
		}
		wantPairs := [2][2]uint64{{1, 2}, {3, 4}}
		for idx, wantPair := range wantPairs {
			im := sim.body.Impacts[idx]
			if im.PlayerID1 != wantPair[0] || im.PlayerID2 != wantPair[1] {
				t.Fatalf("impact %d pair=(%d,%d), want (%d,%d)", idx, im.PlayerID1, im.PlayerID2, wantPair[0], wantPair[1])
			}
			if im.StepStartMS != 0 || im.ClockMS != 20 || math.Abs(im.Fraction-0.5) > 1e-10 {
				t.Fatalf("impact %d timing start=%d clock=%d frac=%.12f, want 0/20/0.5 (TOI 10ms)", idx, im.StepStartMS, im.ClockMS, im.Fraction)
			}
			if math.Abs(im.Impulse-375) > 1e-6 || math.Abs(im.NormalX+1) > 1e-9 || math.Abs(im.NormalY) > 1e-9 {
				t.Fatalf("impact %d impulse=%.9f normal=(%.9f,%.9f), want 375/(-1,0)", idx, im.Impulse, im.NormalX, im.NormalY)
			}
			if math.Abs(im.InVX1-5) > 1e-9 || math.Abs(im.InVY1-0.3) > 1e-9 ||
				math.Abs(im.InVX2+5) > 1e-9 || math.Abs(im.InVY2-0.3) > 1e-9 {
				t.Fatalf("impact %d incoming (%v,%v)/(%v,%v)", idx, im.InVX1, im.InVY1, im.InVX2, im.InVY2)
			}
			if math.Abs(im.OutVX1) > 1e-9 || math.Abs(im.OutVY1-0.3) > 1e-9 ||
				math.Abs(im.OutVX2) > 1e-9 || math.Abs(im.OutVY2-0.3) > 1e-9 {
				t.Fatalf("impact %d outgoing (%v,%v)/(%v,%v), want (0,0.3)/(0,0.3)", idx, im.OutVX1, im.OutVY1, im.OutVX2, im.OutVY2)
			}
		}
	}
	for w := 1; w < 3; w++ {
		if !snapEqualPerStep(inits[0], inits[w]) || !snapEqualPerStep(finals[0], finals[w]) {
			t.Fatalf("impact window %d differs from window 0", w)
		}
	}
	t.Logf("T-057 impact window: 3 fresh identical windows; pairs (1,2)&(3,4) hit at TOI=10ms (frac=0.5) J=375 out=(0,0.3); all22 moved every step")
}

// snapEqualPerStep compares snapshots tolerating NaN-free equality.
func snapEqualPerStep(a, b Snapshot) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
