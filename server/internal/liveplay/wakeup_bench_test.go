package liveplay

import "testing"

// T-045 active22 wakeup benchmark (isolated prototype evidence).
//
// Reset is EXCLUDED from the timed region (b.StopTimer) and the record/scheduler
// buffers are rewound, so each number is the microstep cost of the live path on a
// controlled always-active scene: a rolling free ball, all 22 bodies local and
// genuinely chasing it, the swept contact scan, path events and one sample. This
// is a synthetic always-active prototype workload, NOT a 90-minute 11v11 game.
//
// CAVEAT (honest): the 200 m sensing radius makes EVERY event local on a 105 m
// pitch, so these three points measure the SCHEDULER/wakeup overhead, not a sparse
// local/remote event mix, and the gated points additionally include the TEST
// HARNESS's per-substep event scan + flag toggling (no persisted state), NOT a
// proposed production emitter cost. Publication recording is disabled for timing.

// t045Rearm resets the contest, scheduler, perception records AND the harness state
// to a clean lattice window (reset excluded from timing).
func (h *t045Harness) t045Rearm() {
	t043RearmBench(h.sim)
	h.cursor = 0
	h.pendingAny = false
	n := len(h.sim.runners)
	if cap(h.pendingLocal) < n {
		h.pendingLocal = make([]bool, n)
	}
	h.pendingLocal = h.pendingLocal[:n]
	for i := range h.pendingLocal {
		h.pendingLocal[i] = false
	}
	h.preX = h.preX[:0]
	h.preY = h.preY[:0]
	h.endX = h.endX[:0]
	h.endY = h.endY[:0]
	h.pubs = h.pubs[:0]
	h.pubSteps = 0
	h.lastCount = h.sim.DecisionPublishCount()
}

func benchmarkT045Wakeup(b *testing.B, mode t045Mode) {
	h, err := t045NewHarness(ScheduledLivePassConfig(100), 1, mode)
	if err != nil {
		b.Fatal(err)
	}
	h.recordPubs = false
	h.sim.SetRunners(t044BenchRunners(true)) // all 22 local, 200 m sensing
	h.sim.samples = make([]Sample, 0, 4096)
	h.sim.events = make([]PassEvent, 0, 256)
	h.t045Rearm()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		h.t045Rearm()
		b.StartTimer()
		for k := 0; k < t040Window; k++ {
			h.step()
		}
	}
}

// BenchmarkT045Wakeup22Shared: unmodified T-042 scheduler through the harness.
func BenchmarkT045Wakeup22Shared(b *testing.B) { benchmarkT045Wakeup(b, t045Shared) }

// BenchmarkT045Wakeup22Union: REJECTED global-union gating.
func BenchmarkT045Wakeup22Union(b *testing.B) { benchmarkT045Wakeup(b, t045GatedUnion) }

// BenchmarkT045Wakeup22PerRunner: the studied per-runner gated controller.
func BenchmarkT045Wakeup22PerRunner(b *testing.B) { benchmarkT045Wakeup(b, t045GatedPerRunner) }

// TestT045Active22StaysActive guards every window: the ball stays rolling, all 22
// bodies move every substep, the scheduler publishes exactly on the lattice and the
// clock advances one substep per Step.
func TestT045Active22StaysActive(t *testing.T) {
	for _, mode := range []t045Mode{t045Shared, t045GatedUnion, t045GatedPerRunner} {
		h := t045MustHarness(t, ScheduledLivePassConfig(100), 1, mode)
		h.sim.SetRunners(t044BenchRunners(true))
		for w := 0; w < 3; w++ {
			h.t045Rearm()
			clock0 := h.sim.ClockMS()
			prev := h.sim.Runners()
			pubs := 0
			last := 0
			count := func() int {
				if mode == t045Shared {
					return h.sim.DecisionPublishCount()
				}
				return h.pubSteps
			}
			for k := 0; k < t040Window; k++ {
				h.step()
				if h.sim.Ball().Mode != BallRolling {
					t.Fatalf("mode=%s window=%d: ball mode=%s at step %d", mode, w, h.sim.Ball().Mode, k)
				}
				if len(h.sim.Runners()) != 22 {
					t.Fatalf("mode=%s window=%d: runner count %d", mode, w, len(h.sim.Runners()))
				}
				if ms, _ := h.sim.FirstContact(); ms >= 0 {
					t.Fatalf("mode=%s window=%d: contact at step %d", mode, w, k)
				}
				cur := h.sim.Runners()
				for i := range cur {
					if DistM(prev[i].X, prev[i].Y, cur[i].X, cur[i].Y) <= 1e-9 {
						t.Fatalf("mode=%s window=%d: runner %d did not move at step %d", mode, w, i, k)
					}
				}
				prev = cur
				if c := count(); c > last {
					last = c
					pubs++
				}
			}
			if got := h.sim.ClockMS() - clock0; got != t040Window*h.sim.Config().SubstepMS {
				t.Fatalf("mode=%s window=%d: clock advanced %d ms, want %d", mode, w, got, t040Window*h.sim.Config().SubstepMS)
			}
			want := t040Window * h.sim.Config().SubstepMS / h.sim.Config().IntentCadenceMS
			if pubs != want {
				t.Fatalf("mode=%s window=%d: %d publications, want %d lattice ticks", mode, w, pubs, want)
			}
			if paths := countKind(h.sim, PassEventPath); paths < 10 {
				t.Fatalf("mode=%s window=%d: only %d path events in a 2 s rolling window", mode, w, paths)
			}
		}
	}
}
