package liveplay

import "testing"

// Live-pass benchmarks. They measure the ACTIVE path only: a rolling ball in
// free travel with 22 inertial bodies actually moving, periodic path events and
// (for the recording variant) one sample per substep. The ball is re-armed onto
// a fresh pass window and the runners are parked away from its lane, so no
// control ever fires and the loop never degenerates to the Held/Dead path.

// liveBenchSlot lays out one of 22 inertial bench bodies along the far touchline
// (y <= 0.11), well clear of the ball lane at y = 0.50 (~26 m away), with a real
// lateral intent so the integrator stays active.
func liveBenchSlot(i int) (x, y, tx float64) {
	x = 0.10 + float64(i%11)*0.07
	y = 0.05 + float64(i/11)*0.06
	tx = x + 0.30
	if tx > 0.95 {
		tx = x - 0.30
	}
	return x, y, tx
}

func liveBenchRunners() []Runner {
	p := DefaultMotionParams()
	rs := make([]Runner, 22)
	for i := range rs {
		x, y, tx := liveBenchSlot(i)
		rs[i] = InertialRunner(x, y, tx, y, p)
	}
	return rs
}

// benchRearmLivePass restarts a fresh pass window: a rolling ball in bounds, no
// contact, no immunity, and every inertial body reset and re-published to a far
// intent. Sample/event backing arrays are rewound in place so the measurement is
// steady-state appends, not slice growth.
func benchRearmLivePass(sim *Sim) {
	sim.ball = Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 16.0, VYM: 0.2}
	sim.contactMS, sim.contactRunner = -1, -1
	sim.releaseRunner = -1
	sim.immunityUntil = 0
	sim.lastPathMS = sim.clock
	for i := range sim.runners {
		x, y, tx := liveBenchSlot(i)
		r := &sim.runners[i]
		r.X, r.Y = x, y
		r.VXM, r.VYM = 0, 0
		r.IntentX, r.IntentY = tx, y
	}
	sim.samples = sim.samples[:0]
	sim.events = sim.events[:0]
}

// BenchmarkLivePassActive measures one active live-pass substep on the full
// contract path: rolling ball + 22 inertial bodies + contact scan + path events
// + one recorded sample.
func BenchmarkLivePassActive(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveBenchRunners())
	sim.samples = make([]Sample, 0, 4096)
	sim.events = make([]PassEvent, 0, 128)
	benchRearmLivePass(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearmLivePass(sim)
		}
		sim.Step()
	}
}

// BenchmarkLivePassQuiet measures the same active path without the sample
// append, isolating the recording cost from the physics/event cost.
func BenchmarkLivePassQuiet(b *testing.B) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetRunners(liveBenchRunners())
	sim.events = make([]PassEvent, 0, 128)
	benchRearmLivePass(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearmLivePass(sim)
		}
		sim.StepQuiet()
	}
}

// TestLivePassBenchStaysActive guards the live-pass benchmarks: the ball must
// stay rolling, every inertial body must keep moving, and path events must be
// emitted, so no benchmark silently measures the Held/Dead path.
func TestLivePassBenchStaysActive(t *testing.T) {
	sim, err := NewSim(LivePassConfig(), 1)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim.SetRunners(liveBenchRunners())
	const total = 64 * 200
	nonRolling, still, pathEvents := 0, 0, 0
	for i := 0; i < total; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearmLivePass(sim)
		}
		sim.StepQuiet()
		if sim.ball.Mode != BallRolling {
			nonRolling++
		}
		moving := 0
		for _, r := range sim.runners {
			if r.SpeedMPS() > 0.05 {
				moving++
			}
		}
		if moving < len(sim.runners) {
			still++
		}
		for _, e := range sim.events {
			if e.Kind == PassEventPath {
				pathEvents++
			}
		}
		sim.events = sim.events[:0] // count without unbounded growth
	}
	if nonRolling != 0 {
		t.Fatalf("%d/%d steps left the ball non-rolling", nonRolling, total)
	}
	if still != 0 {
		t.Fatalf("%d/%d steps had a still inertial body", still, total)
	}
	if pathEvents == 0 {
		t.Fatal("no path events emitted on the active live-pass benchmark path")
	}
}
