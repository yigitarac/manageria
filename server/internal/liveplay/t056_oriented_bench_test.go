package liveplay

import (
	"encoding/json"
	"testing"
)

// T-056 oriented active22 benchmarks (isolated prototype evidence, TEST-ONLY).
//
// Fresh deterministic baselines are built OUTSIDE the timer (same seed + IDs +
// optional orientation + enable + release + bounded scratch), so the timed region
// measures steady-state cost. The baseline (non-oriented integrated) and oriented
// runs are MATCHING scenes, separately measured TOTAL costs from which the planner
// can compute qualified oriented/recorder deltas or ratios (same seed, same window,
// same runner count). JSON export/marshal and bytes are reported separately. The
// <20 ms full-match target and any football calibration remain unproven.

func t056NewBenchSim(b *testing.B, seed uint64, control, oriented, withRec bool) *Sim {
	sim, err := NewSim(ScheduledLivePassConfig(100), seed)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	rs := t055AssignIDs(t053Runners(control))
	if oriented {
		o, err := NewOrientation(1, 0, 1)
		if err != nil {
			b.Fatal(err)
		}
		rs[0].Orientation = &o
	}
	sim.SetRunners(rs)
	if err := sim.EnableIntegrated(); err != nil {
		b.Fatal(err)
	}
	if control {
		sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	} else {
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	}
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
	return sim
}

func benchmarkT056Window(b *testing.B, control, oriented, withRec bool) {
	seed := uint64(t053ControlSeed)
	if !control {
		seed = t053DeflectSeed
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t056NewBenchSim(b, seed, control, oriented, withRec)
		b.StartTimer()
		t053AdvanceWindow(sim, control, nil)
	}
}

// Baseline (non-oriented integrated) windows.
func BenchmarkT056ControlBaseline22(b *testing.B) { benchmarkT056Window(b, true, false, false) }

// Oriented windows.
func BenchmarkT056ControlOriented22(b *testing.B) { benchmarkT056Window(b, true, true, false) }

// Oriented + sidecar TOTAL.
func BenchmarkT056ControlOrientedRecorder22(b *testing.B) { benchmarkT056Window(b, true, true, true) }

func BenchmarkT056DeflectBaseline22(b *testing.B) { benchmarkT056Window(b, false, false, false) }

func BenchmarkT056DeflectOriented22(b *testing.B) { benchmarkT056Window(b, false, true, false) }

func BenchmarkT056DeflectOrientedRecorder22(b *testing.B) { benchmarkT056Window(b, false, true, true) }

// BenchmarkT056JSONExport measures the TOTAL offline export on an oriented window:
// clone/advance + BuildDecisionProvenance + marshal the oriented snapshot.
func BenchmarkT056JSONExport(b *testing.B) {
	sim := t056NewBenchSimB(b)
	start := sim.ClockMS()
	bytes := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blob, err := json.Marshal(sim.Snapshot())
		if err != nil {
			b.Fatal(err)
		}
		prov, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 4096, HorizonMS: start + 200})
		if err != nil {
			b.Fatal(err)
		}
		pb, err := json.Marshal(prov)
		if err != nil {
			b.Fatal(err)
		}
		bytes = len(blob) + len(pb)
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
}

// BenchmarkT056JSONSnapshotMarshalOnly isolates the oriented snapshot marshal.
func BenchmarkT056JSONSnapshotMarshalOnly(b *testing.B) {
	sim := t056NewBenchSimB(b)
	snap := sim.Snapshot()
	bytes := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blob, err := json.Marshal(snap)
		if err != nil {
			b.Fatal(err)
		}
		bytes = len(blob)
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
}

func t056NewBenchSimB(b *testing.B) *Sim {
	sim := t056NewBenchSim(b, t053DeflectSeed, false, true, false)
	for k := 0; k < 50; k++ {
		sim.Step()
	}
	return sim
}
