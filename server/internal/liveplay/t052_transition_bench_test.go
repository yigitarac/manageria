package liveplay

import "testing"

// T-052 active-22 benchmarks (isolated prototype evidence, TEST-ONLY).
//
// Reset is EXCLUDED from the timed region (b.StopTimer). Two separated points:
//   - Engine22     : engine only (runtime baseline).
//   - Diagnostics22: engine + the optional T-049 provenance recorder (TOTAL window
//     cost; the diagnostic-only delta is this number minus the engine baseline).
// Synthetic always-active 22-body scene, NOT a 90-minute 11v11 game; the <20 ms
// target stays open.

// BenchmarkT052Engine22: engine-only active-22 baseline.
func BenchmarkT052Engine22(b *testing.B) {
	benchmarkT050Active22(b, func() []Runner { return t046BenchRunners(true) }, t046RearmBench)
}

// BenchmarkT052Diagnostics22: the active-22 scene with the sidecar recorder
// attached. TOTAL window cost; the delta is kept separate.
func BenchmarkT052Diagnostics22(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t046BenchRunners(true))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	t046RearmBench(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rec, err := NewProvenanceRecorder(8192)
		if err != nil {
			b.Fatal(err)
		}
		sim.SetProvenanceRecorder(rec)
		t046RearmBench(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
	b.StopTimer()
	sim.SetProvenanceRecorder(nil)
}
