package liveplay

import "testing"

// T-049 provenance recorder benchmarks and guard (isolated prototype evidence).
//
// The recorder is a bounded OFFLINE diagnostic; enabling it allocates by design.
// These benchmarks measure the SAME always-active 22-body local scene with the
// recorder OFF and ON, so the added cost is the real publication-hook + record
// copy on a genuine microstep workload. Reset is excluded from the timed region
// (b.StopTimer) and the record buffers are rewound in place.
//
// This is a synthetic always-active prototype workload, NOT a 90-minute 11v11 game
// and NOT a runtime-decision cost (the default path never attaches a recorder).

func benchmarkT049Provenance(b *testing.B, withRecorder bool) {
	sim, err := NewSim(ScheduledLivePassConfig(100), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(t046LocalBenchRunners(true))
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	var rec *ProvenanceRecorder
	if withRecorder {
		rec, err = NewProvenanceRecorder(4096)
		if err != nil {
			b.Fatal(err)
		}
		sim.SetProvenanceRecorder(rec)
	}
	rearm := func() {
		t046RearmBench(sim)
		if rec != nil {
			rec.records = rec.records[:0]
			rec.overflowed = false
		}
	}
	rearm()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rearm()
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT049Provenance22RecorderOff: the local-wakeup scene with no recorder.
func BenchmarkT049Provenance22RecorderOff(b *testing.B) { benchmarkT049Provenance(b, false) }

// BenchmarkT049Provenance22RecorderOn: the SAME scene with a bounded recorder.
func BenchmarkT049Provenance22RecorderOn(b *testing.B) { benchmarkT049Provenance(b, true) }

// TestT049Active22StaysActive guards the recorder window: the ball stays rolling,
// all 22 bodies move every substep, the scheduler publishes on the lattice and the
// recorder captures a non-empty, non-overflowing record set.
func TestT049Active22StaysActive(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 1)
	sim.SetRunners(t046LocalBenchRunners(true))
	rec, err := NewProvenanceRecorder(4096)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetProvenanceRecorder(rec)
	for w := 0; w < 3; w++ {
		t046RearmBench(sim)
		rec.records = rec.records[:0]
		rec.overflowed = false
		clock0 := sim.ClockMS()
		prev := sim.Runners()
		pubs := 0
		last := sim.DecisionPublishCount()
		for k := 0; k < t040Window; k++ {
			sim.Step()
			if sim.Ball().Mode != BallRolling {
				t.Fatalf("window=%d: ball mode=%s at step %d", w, sim.Ball().Mode, k)
			}
			cur := sim.Runners()
			for i := range cur {
				if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
					t.Fatalf("window=%d: runner %d did not move at step %d", w, i, k)
				}
			}
			prev = cur
			if c := sim.DecisionPublishCount(); c > last {
				last = c
				pubs++
			}
		}
		if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
			t.Fatalf("window=%d: clock advanced %d ms, want %d", w, got, t040Window*sim.Config().SubstepMS)
		}
		want := t040Window * sim.Config().SubstepMS / sim.Config().IntentCadenceMS
		if pubs != want {
			t.Fatalf("window=%d: %d publications, want %d lattice ticks", w, pubs, want)
		}
		if rec.Len() != pubs*22 {
			t.Fatalf("window=%d: %d records, want %d (22 per publication)", w, rec.Len(), pubs*22)
		}
		if rec.Overflowed() {
			t.Fatalf("window=%d: recorder overflowed at capacity %d", w, rec.capacity)
		}
	}
	t.Logf("recorder window stays active: %d records per window, no overflow", rec.Len())
}
