package liveplay

import "testing"

// T-050 active-22 reaction benchmarks and guards (isolated prototype evidence,
// TEST-ONLY).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record buffers are
// rewound in place, so each active-22 number is the real microstep cost of the
// live path on a controlled always-active scene: a rolling free ball, 22 inertial
// bodies genuinely moving every substep, the swept contact scan, path events and
// one sample. This is a synthetic always-active prototype workload, NOT a 90-minute
// 11v11 game.
//
// Two paired active points isolate the reaction-gate state:
//   - Elapsed: ReactAtMS already passed, so the local policy refreshes intents and
//     the bodies chase the observed ball (the normal active path).
//   - Armed  : every ReactAtMS is deliberately in the FUTURE (the "just reset"
//     world), so the policy is reaction-blocked but integration still runs; the
//     fixture pre-sets each intent toward the lane so the bodies still move.
//
// The one-time ReleasePass reset cost and the optional provenance diagnostics are
// measured SEPARATELY (diagnostic cost is not part of the microstep number).

func benchmarkT050Active22(b *testing.B, runners func() []Runner, rearm func(*Sim)) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(runners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	rearm(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rearm(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT050Active22ReactionElapsed: 22 local-pursuit + local-wakeup bodies
// with an elapsed deadline (the normal active path). This is ALSO the baseline
// for the diagnostic delta below.
func BenchmarkT050Active22ReactionElapsed(b *testing.B) {
	benchmarkT050Active22(b, func() []Runner { return t046BenchRunners(true) }, t046RearmBench)
}

// BenchmarkT050Active22ReactionArmed: the SAME 22 bodies with every reaction
// deadline re-armed into the future (the "just reset" world). Integration never
// freezes; only the policy is gated.
func BenchmarkT050Active22ReactionArmed(b *testing.B) {
	benchmarkT050Active22(b, t050ArmedBenchRunners, t050RearmArmed)
}

// BenchmarkT050ReleaseReset22 measures the ONE-TIME physical cost of a real
// ReleasePass resetting all 22 reaction deadlines (plus the two declared RNG draws
// and the event emission). It is diagnostic and deliberately separate from the
// per-substep active-22 number. The event slice is rewound OUTSIDE the timed
// region each iteration (b.StopTimer) so the append cannot grow unboundedly; the
// rewound allocation is therefore excluded and the timed region is exactly one
// ReleasePass on a 22-body world.
func BenchmarkT050ReleaseReset22(b *testing.B) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t046BenchRunners(true))
	sim.samples = make([]Sample, 0, 256)
	sim.events = make([]PassEvent, 0, 64)
	p := PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 3.0, Passer: -1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim.events = sim.events[:0] // bound the log; excluded from the timed region
		b.StartTimer()
		sim.ReleasePass(p)
	}
}

// BenchmarkT050ProvenanceDiagnostic22 measures the TOTAL cost of the active-22
// window WITH the optional T-049 provenance recorder attached (sim + recording).
// The recorder is (re)constructed in the excluded region, so the number is the
// full window cost; the diagnostic-only delta is this number MINUS
// BenchmarkT050Active22ReactionElapsed (same fixture, no recorder), and must be
// computed separately rather than presented as a microstep cost.
func BenchmarkT050ProvenanceDiagnostic22(b *testing.B) {
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
