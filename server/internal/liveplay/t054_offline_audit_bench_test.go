package liveplay

import (
	"encoding/json"
	"testing"
)

// T-054 offline report cost/size benchmarks (isolated prototype evidence,
// TEST-ONLY). The report build is OFFLINE (fixed copied input) and INDEPENDENT of
// simulation; simulation/diagnostic costs are the T-053 measured fresh-window
// numbers (exact reference), not re-measured here. No full-match/<20 ms claim.

// t054FixedReport builds one REAL T-053 moving22 transition-heavy scene and
// returns the ACTUAL copied report input (outside any timer). It asserts a real
// physical transition and non-empty publications before timing.
func t054FixedReport(b *testing.B) T054ReportInput {
	sim, err := NewSim(ScheduledLivePassConfig(100), t053DeflectSeed)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t053Runners(false))
	rec := mustRecorderB(b, 4096)
	sim.SetProvenanceRecorder(rec)
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
	for sim.ClockMS() < 2000 {
		sim.Step()
	}
	defl := 0
	for _, e := range sim.Events() {
		if e.Kind == PassEventDeflect {
			defl++
		}
	}
	if defl == 0 {
		b.Fatal("fixed report scene has no physical transition")
	}
	if rec.Len() == 0 {
		b.Fatal("fixed report scene has no publications")
	}
	return T054ReportInput{
		StartMS: 0, EndMS: 2000, SubstepMS: sim.Config().SubstepMS, RunnerCount: len(sim.runners),
		PrefixHistoryPresent: true, Overflowed: rec.Overflowed(),
		Events: sim.Events(), Samples: sim.Samples(), Records: rec.Records(),
	}
}

// BenchmarkT054ReportBuild measures the TOTAL offline report cost: build + render
// + JSON marshal, on a fixed copied input. Reports ns/op, report_bytes and
// render_bytes, plus B/op and allocs/op.
func BenchmarkT054ReportBuild(b *testing.B) {
	in := t054FixedReport(b)
	var jsonBytes, renderBytes int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rep := T054BuildReport(in)
		s := T054RenderReport(rep)
		blob, err := json.Marshal(rep)
		if err != nil {
			b.Fatal(err)
		}
		jsonBytes, renderBytes = len(blob), len(s)
	}
	b.StopTimer()
	b.ReportMetric(float64(jsonBytes), "report_bytes")
	b.ReportMetric(float64(renderBytes), "render_bytes")
}

// BenchmarkT054ReportMarshalOnly isolates the JSON marshal on a fixed built report.
func BenchmarkT054ReportMarshalOnly(b *testing.B) {
	in := t054FixedReport(b)
	rep := T054BuildReport(in)
	var jsonBytes int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blob, err := json.Marshal(rep)
		if err != nil {
			b.Fatal(err)
		}
		jsonBytes = len(blob)
	}
	b.StopTimer()
	b.ReportMetric(float64(jsonBytes), "report_bytes")
}
