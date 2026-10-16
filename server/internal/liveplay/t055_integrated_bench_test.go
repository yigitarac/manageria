package liveplay

import (
	"encoding/json"
	"testing"
)

// T-055 integrated-shell benchmarks (isolated prototype evidence, TEST-ONLY).
//
// Each timed iteration builds a FRESH deterministic active22 baseline OUTSIDE the
// timer (same declared seed + IDs + optional enable + release + bounded scratch),
// so the timed region measures steady-state cost only. Guard and cost share the
// same window helper, so they cannot drift.
//
// Labels are INDEPENDENT (never subtracted across different workloads):
//   - ...Disabled22 / ...Integrated22 : engine simulation runtime.
//   - ...IntegratedRecorder22 : the SAME window with the shell AND the T-049
//     sidecar attached (TOTAL); the sidecar delta is TOTAL minus the matching
//     Integrated22 window.
//   - JSONExport : marshal the snapshot (incl. the journal) + BuildDecisionProvenance
//     export; JSONMarshalOnly isolates the marshal on a fixed payload.
// The <20 ms full-match target remains unproven.

// t055NewBenchSim builds a fresh, fully reset deterministic baseline outside the
// timer, with bounded preallocation sized before timing.
func t055NewBenchSim(b *testing.B, seed uint64, control, integrated, withRec bool) *Sim {
	sim, err := NewSim(ScheduledLivePassConfig(100), seed)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	sim.SetRunners(t055AssignIDs(t053Runners(control)))
	if integrated {
		if err := sim.EnableIntegrated(); err != nil {
			b.Fatal(err)
		}
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

func benchmarkT055Window(b *testing.B, control, integrated, withRec bool) {
	seed := uint64(t053ControlSeed)
	if !control {
		seed = t053DeflectSeed
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t055NewBenchSim(b, seed, control, integrated, withRec)
		b.StartTimer()
		t053AdvanceWindow(sim, control, nil)
	}
}

// BenchmarkT055ControlDisabled22: control window, shell off (frozen legacy).
func BenchmarkT055ControlDisabled22(b *testing.B) { benchmarkT055Window(b, true, false, false) }

// BenchmarkT055ControlIntegrated22: control window, shell on.
func BenchmarkT055ControlIntegrated22(b *testing.B) { benchmarkT055Window(b, true, true, false) }

// BenchmarkT055ControlIntegratedRecorder22: control window, shell + sidecar (TOTAL).
func BenchmarkT055ControlIntegratedRecorder22(b *testing.B) { benchmarkT055Window(b, true, true, true) }

// BenchmarkT055DeflectDisabled22: deflect window, shell off.
func BenchmarkT055DeflectDisabled22(b *testing.B) { benchmarkT055Window(b, false, false, false) }

// BenchmarkT055DeflectIntegrated22: deflect window, shell on.
func BenchmarkT055DeflectIntegrated22(b *testing.B) { benchmarkT055Window(b, false, true, false) }

// BenchmarkT055DeflectIntegratedRecorder22: deflect window, shell + sidecar (TOTAL).
func BenchmarkT055DeflectIntegratedRecorder22(b *testing.B) {
	benchmarkT055Window(b, false, true, true)
}

// BenchmarkT055JSONExport measures the offline export cost: advance a bounded
// deflect window, then marshal the complete snapshot (incl. journal) and build the
// decision-provenance sidecar. Reports ns/op and json_bytes.
func BenchmarkT055JSONExport(b *testing.B) {
	sim := t055NewBenchSimB(b)
	start := sim.ClockMS()
	bytes := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := sim.Snapshot()
		blob, err := json.Marshal(snap)
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

// BenchmarkT055JSONSnapshotMarshalOnly isolates the snapshot+journal marshal on a
// fixed built payload.
func BenchmarkT055JSONSnapshotMarshalOnly(b *testing.B) {
	sim := t055NewBenchSimB(b)
	snap := sim.Snapshot()
	bytes := len(mustMarshalB(b, snap))
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

// t055NewBenchSimB builds a bounded deflect window with the shell on for the
// offline export benchmarks (past the deflect, so the journal is nonempty).
func t055NewBenchSimB(b *testing.B) *Sim {
	sim := t055NewBenchSim(b, t053DeflectSeed, false, true, false)
	for k := 0; k < 50; k++ {
		sim.Step()
	}
	return sim
}

func mustMarshalB(b *testing.B, v any) []byte {
	blob, err := json.Marshal(v)
	if err != nil {
		b.Fatal(err)
	}
	return blob
}
