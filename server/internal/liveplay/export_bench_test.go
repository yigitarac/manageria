package liveplay

import (
	"encoding/json"
	"testing"
)

// BenchmarkPrototypeExport measures the full T-039 export on an actual
// 22-player inertial microstep scene: 22 bodies integrate, the swept contest
// path runs, and the exporter builds (and serializes) the merged trace. It
// reports ns/op, bytes/trace (via SetBytes) and allocations so the export cost
// and size are recorded beside the physics benchmarks.
func BenchmarkPrototypeExport(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sim, err := NewSim(LivePassConfig(), 1)
		if err != nil {
			b.Fatal(err)
		}
		sim.SetRunners(liveBenchRunners())
		sim.ReleasePass(PassParams{
			StartX: 0.30, StartY: 0.50, TargetX: 0.95, TargetY: 0.50,
			SpeedMPS: 16.0, Passer: -1,
		})
		trace := BuildTrace(ClipInput{
			Sim: sim, Seed: 1, Slug: "bench", Label: "bench", Description: "bench", HorizonMS: 1000,
		})
		raw, err := json.Marshal(trace)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(raw)))
	}
}
