package liveplay

import (
	"encoding/json"
	"testing"
)

// T-053 transition-heavy active22 benchmarks (isolated prototype evidence,
// TEST-ONLY). Synthetic deterministic clips, not full matches.
//
// Each timed iteration constructs a FRESH deterministic baseline OUTSIDE the
// timer (NewSim with the SAME declared seed + t053Fill + optional recorder), so
// clock, RNG, event sequence, scheduler state and perception records all start
// identical and the timed cost corresponds exactly to the guarded fixture (the
// initial release is part of the fixture setup, excluded from timing). The window
// is advanced by the SAME shared helper as the guard (t053AdvanceWindow), so guard
// and cost cannot drift.
//
// Costs are INDEPENDENTLY labelled (never subtracted across different workloads):
//   - ControlWindow22 / DeflectWindow22 : engine simulation runtime (the control
//     window INCLUDES its real mid-window second-release schedule).
//   - ...Diagnostic22 : the SAME window with the T-049 sidecar attached (TOTAL);
//     the diagnostic delta is TOTAL minus the matching no-recorder window.
//   - JSONExport : TOTAL offline replay/export = BuildDecisionProvenance
//     (clone + simulate + capture) + JSON marshal; JSONMarshalOnly isolates the
//     marshal on a fixed built payload. Bytes/allocations reported exactly.
// The <20 ms full-match target is unproven.

func mustRecorderB(b *testing.B, capacity int) *ProvenanceRecorder {
	rec, err := NewProvenanceRecorder(capacity)
	if err != nil {
		b.Fatal(err)
	}
	return rec
}

// t053NewBenchSim builds a fresh, fully reset deterministic baseline with the
// declared seed, pre-allocating the engine's bounded scratch OUTSIDE the timed
// region so the timed region measures steady-state transition cost, not one-time
// allocation. It is called outside the timer.
func t053NewBenchSim(b *testing.B, seed uint64, control, withRec bool) *Sim {
	sim, err := NewSim(ScheduledLivePassConfig(100), seed)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t053Fill(sim, control)
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

func benchmarkT053Window(b *testing.B, control, withRec bool) {
	seed := uint64(t053ControlSeed)
	if !control {
		seed = t053DeflectSeed
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := t053NewBenchSim(b, seed, control, withRec)
		b.StartTimer()
		t053AdvanceWindow(sim, control, nil)
	}
}

// BenchmarkT053ControlWindow22: control window, engine only.
func BenchmarkT053ControlWindow22(b *testing.B) { benchmarkT053Window(b, true, false) }

// BenchmarkT053ControlWindowDiagnostic22: control window + sidecar (TOTAL).
func BenchmarkT053ControlWindowDiagnostic22(b *testing.B) { benchmarkT053Window(b, true, true) }

// BenchmarkT053DeflectWindow22: deflect window, engine only.
func BenchmarkT053DeflectWindow22(b *testing.B) { benchmarkT053Window(b, false, false) }

// BenchmarkT053DeflectWindowDiagnostic22: deflect window + sidecar (TOTAL).
func BenchmarkT053DeflectWindowDiagnostic22(b *testing.B) { benchmarkT053Window(b, false, true) }

// BenchmarkT053JSONExport measures the TOTAL offline replay/export cost:
// BuildDecisionProvenance (clone + simulate + capture) + JSON marshal, on a bounded
// deflect-window history. Reports ns/op, json_bytes, B/op and allocs/op.
func BenchmarkT053JSONExport(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), t053DeflectSeed)
	if err != nil {
		b.Fatal(err)
	}
	t053Fill(sim, false)
	for k := 0; k < 50; k++ { // past the deflect; bounded history
		sim.Step()
	}
	start := sim.ClockMS()
	bytes := 0
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
		bytes = len(blob)
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
}

// BenchmarkT053JSONMarshalOnly isolates the JSON marshal on a FIXED built payload.
func BenchmarkT053JSONMarshalOnly(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), t053DeflectSeed)
	if err != nil {
		b.Fatal(err)
	}
	t053Fill(sim, false)
	for k := 0; k < 50; k++ {
		sim.Step()
	}
	start := sim.ClockMS()
	prov, err := BuildDecisionProvenance(ProvenanceInput{Sim: sim, Capacity: 4096, HorizonMS: start + 200})
	if err != nil {
		b.Fatal(err)
	}
	bytes := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blob, err := json.Marshal(prov)
		if err != nil {
			b.Fatal(err)
		}
		bytes = len(blob)
	}
	b.StopTimer()
	b.ReportMetric(float64(bytes), "json_bytes")
}
