package liveplay

import (
	"encoding/json"
	"testing"
)

// T-040 honest 22-body workload benchmarks (isolated prototype evidence).
//
// The windows preallocate/rewind the record buffers OUTSIDE the timed region
// (b.StopTimer) so integration is measured without reset artifacts, and they do
// NOT reset inside the timed window. Two recording variants are kept distinct:
//   - "PreallocatedAppend": append into a preallocated, rewound buffer (steady
//     state, no growth) — the append cost alone;
//   - "FreshBufferGrowth": buffers are set to nil before the timed window, so the
//     append/growth allocation happens DURING the timed integration. The two are
//     NOT combined into a single "+x%" recording claim.
//
// Sample structs are ball-only; BuildTrace records full per-runner positions and
// is measured separately (offline export).
//
// This is a controlled 22-body prototype workload, NOT a full 90-minute 11v11
// game (no team AI, decisions, shots, keepers, aerials, fouls or restarts).
const t040Window = 100 // substeps ≈ 2 s of game time at 20 ms

// --- rolling workload -----------------------------------------------------

// BenchmarkT040Rolling22Preallocated: preallocated, event-free integration of a
// rolling ball + 22 chasing inertial bodies (quiet: no sample append). Reset is
// excluded from the timed region.
func BenchmarkT040Rolling22Preallocated(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveContestBenchRunners())
	benchRearmLiveContest(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		benchRearmLiveContest(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.StepQuiet()
		}
	}
}

// BenchmarkT040Rolling22PreallocatedAppend: the same window with the sample
// append enabled into a PREALLOCATED, rewound buffer. This is steady-state
// append cost, NOT trace growth; see FreshBufferGrowth for the growing case.
func BenchmarkT040Rolling22PreallocatedAppend(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveContestBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 256)
	benchRearmLiveContest(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		benchRearmLiveContest(sim) // rewinds samples/events to len 0 (preallocated)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// BenchmarkT040Rolling22FreshBufferGrowth: buffers are set to nil before each
// timed window, so append allocation/growth happens DURING the measured
// integration. Reports B/op and allocs/op for genuine trace growth.
func BenchmarkT040Rolling22FreshBufferGrowth(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveContestBenchRunners())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		benchRearmLiveContest(sim)
		sim.samples = nil
		sim.events = nil
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// --- held workload --------------------------------------------------------

// t040RearmHeld resets a held-ball window: ball attached to runner 0 and all 22
// bodies genuinely accelerating toward far intents.
func t040RearmHeld(sim *Sim) {
	for i := range sim.runners {
		x, y, tx, ty := inertialSlot(i)
		r := &sim.runners[i]
		r.X, r.Y = x, y
		r.VXM, r.VYM = 0, 0
		r.IntentX, r.IntentY = tx, ty
	}
	sim.ball = Ball{Mode: BallHeld, X: sim.runners[0].X, Y: sim.runners[0].Y, HoldDX: 0, HoldDY: 0}
	sim.contactRunner = 0
	sim.contactMS = sim.clock
	sim.releaseRunner = 0
	sim.immunityUntil = 0
	sim.controlError = 0
	sim.samples = sim.samples[:0]
	sim.events = sim.events[:0]
}

// BenchmarkT040Held22: a held ball whose owner and all other bodies keep moving.
// The held branch (ball follows owner) plus 22 inertial integrations are timed.
func BenchmarkT040Held22(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(inertial22())
	sim.samples = make([]Sample, 0, 4096)
	t040RearmHeld(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t040RearmHeld(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// --- loose workload -------------------------------------------------------

// t040RearmLoose resets a stationary loose-ball window: the ball is at rest
// (Loose) and 22 inertial pursuers scan and chase it from the far touchline.
func t040RearmLoose(sim *Sim) {
	sim.ball = Ball{Mode: BallLoose, X: 0.90, Y: 0.50}
	sim.contactMS, sim.contactRunner = -1, -1
	sim.releaseRunner = -1
	sim.immunityUntil = 0
	sim.lastPathMS = sim.clock
	sim.controlError = 0
	sim.firstTouchMS, sim.firstTouchRunner = -1, -1
	for i := range sim.runners {
		x, y := liveContestBenchSlot(i)
		r := &sim.runners[i]
		r.X, r.Y = x, y
		r.VXM, r.VYM = 0, 0
		r.IntentX, r.IntentY = x, y
		r.ReactAtMS = 0
		r.CooldownUntilMS = 0
	}
	sim.samples = sim.samples[:0]
	sim.events = sim.events[:0]
}

// BenchmarkT040Loose22: a stationary collectable loose ball, 22 inertial
// pursuers chasing it, swept scan running every substep.
func BenchmarkT040Loose22(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveContestBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	t040RearmLoose(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t040RearmLoose(sim)
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			sim.Step()
		}
	}
}

// --- trace export / size --------------------------------------------------

// BenchmarkT040TraceExport22 measures building and serializing a 22-body, 1 s
// authoritative trace (full per-runner positions; an offline export, not the
// match path and not the ball-only Sample stream).
func BenchmarkT040TraceExport22(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sim, err := NewSim(LivePassConfig(), uint64(i))
		if err != nil {
			b.Fatal(err)
		}
		sim.SetRunners(inertial22())
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
		tr := BuildTrace(ClipInput{Sim: sim, Seed: uint64(i), Slug: "bench", Label: "bench", HorizonMS: 1000})
		if _, err := json.Marshal(tr); err != nil {
			b.Fatal(err)
		}
	}
}

// TestT040TraceSize22 records the authoritative trace size for a 22-body clip
// (full positions), distinct from the ball-only Sample append cost.
func TestT040TraceSize22(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 3)
	sim.SetRunners(inertial22())
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, Passer: -1})
	tr := BuildTrace(ClipInput{Sim: sim, Seed: 3, Slug: "size", Label: "size", HorizonMS: 1000})
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("22-body 1s trace (full positions): points=%d events=%d sampleCount=%d bytes=%d", len(tr.Points), len(tr.Events), tr.SampleCount, len(raw))
}

// --- active-window guards -------------------------------------------------

// TestT040BenchWindowsStayActive proves each honest window stays on its intended
// active path PER SUBSTEP: count22 at every step, every body genuinely moving
// every step, owner-ball agreement at every held step, no accidental contact,
// and the loose ball genuinely stationary. Net window displacement alone is not
// used. Multiple windows also check clock bookkeeping.
func TestT040BenchWindowsStayActive(t *testing.T) {
	movedSinceLast := func(prev, cur []Runner) int {
		moving := 0
		for i := range cur {
			if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) > 1e-9 {
				moving++
			}
		}
		return moving
	}

	// Rolling: ball stays rolling, 22 bodies move every substep, no contact.
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(liveContestBenchRunners())
	for w := 0; w < 3; w++ {
		benchRearmLiveContest(sim)
		clock0 := sim.ClockMS()
		prev := sim.Runners()
		for k := 0; k < t040Window; k++ {
			sim.StepQuiet()
			if len(sim.Runners()) != 22 {
				t.Fatalf("rolling window: runner count %d != 22", len(sim.Runners()))
			}
			if sim.Ball().Mode != BallRolling {
				t.Fatalf("rolling window: ball mode=%s at step %d", sim.Ball().Mode, k)
			}
			if ms, _ := sim.FirstContact(); ms >= 0 {
				t.Fatalf("rolling window: contact at step %d", k)
			}
			cur := sim.Runners()
			if moved := movedSinceLast(prev, cur); moved != 22 {
				t.Fatalf("rolling window: only %d/22 bodies moved at step %d", moved, k)
			}
			prev = cur
		}
		if got := sim.ClockMS() - clock0; got != t040Window*sim.Config().SubstepMS {
			t.Fatalf("rolling window: clock advanced %d ms, want %d", got, t040Window*sim.Config().SubstepMS)
		}
	}

	// Held: ball stays held, owner-ball agree every step, 22 bodies move.
	sim2 := mustSim(t, LivePassConfig(), 1)
	sim2.SetRunners(inertial22())
	for w := 0; w < 3; w++ {
		t040RearmHeld(sim2)
		clock0 := sim2.ClockMS()
		prev := sim2.Runners()
		for k := 0; k < t040Window; k++ {
			sim2.Step()
			if len(sim2.Runners()) != 22 {
				t.Fatalf("held window: runner count %d != 22", len(sim2.Runners()))
			}
			if sim2.Ball().Mode != BallHeld {
				t.Fatalf("held window: ball mode=%s at step %d", sim2.Ball().Mode, k)
			}
			own := sim2.Runners()[sim2.contactRunner]
			if d := DistM(sim2.Ball().X, sim2.Ball().Y, own.X, own.Y); d > 1e-9 {
				t.Fatalf("held window: ball off owner by %.3e m at step %d", d, k)
			}
			cur := sim2.Runners()
			if moved := movedSinceLast(prev, cur); moved != 22 {
				t.Fatalf("held window: only %d/22 bodies moved at step %d", moved, k)
			}
			prev = cur
		}
		if got := sim2.ClockMS() - clock0; got != t040Window*sim2.Config().SubstepMS {
			t.Fatalf("held window: clock advanced %d ms, want %d", got, t040Window*sim2.Config().SubstepMS)
		}
	}

	// Loose: stationary collectable ball, 22 pursuers move, nobody reaches it.
	sim3 := mustSim(t, LivePassConfig(), 1)
	sim3.SetRunners(liveContestBenchRunners())
	for w := 0; w < 3; w++ {
		t040RearmLoose(sim3)
		prev := sim3.Runners()
		ball0 := sim3.Ball()
		for k := 0; k < t040Window; k++ {
			sim3.StepQuiet()
			if len(sim3.Runners()) != 22 {
				t.Fatalf("loose window: runner count %d != 22", len(sim3.Runners()))
			}
			if sim3.Ball().Mode != BallLoose {
				t.Fatalf("loose window: ball mode=%s at step %d", sim3.Ball().Mode, k)
			}
			if d := DistM(ball0.X, ball0.Y, sim3.Ball().X, sim3.Ball().Y); d > 1e-9 {
				t.Fatalf("loose window: ball moved %.3e m at step %d", d, k)
			}
			cur := sim3.Runners()
			if moved := movedSinceLast(prev, cur); moved != 22 {
				t.Fatalf("loose window: only %d/22 pursuers moved at step %d", moved, k)
			}
			prev = cur
		}
	}
}
