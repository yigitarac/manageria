package liveplay

import "testing"

// benchBall is an always-rolling ground ball used to keep benchmarks on the
// ACTIVE path. Looping a single simulator until friction stops it (the previous
// mistake) mostly measured the trivial Dead path and skipped friction and the
// contact scan, so every benchmark here either resets the ball each iteration or
// explicitly reports the active/dead split.
func benchBall() Ball { return Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 16.0} }

// benchRearmEvery re-arms the ball every N iterations (power of two) so a
// benchmark stays on the active rolling path while amortizing the re-arm cost.
// 64 steps at the coarsest candidate (200 ms) loses 7.7 m/s from 16 m/s, still
// rolling, so no candidate falls to Dead inside a window.
const benchRearmEvery = 64

func benchRearm(sim *Sim) {
	sim.ball.X, sim.ball.Y = 0.30, 0.50
	sim.ball.VXM, sim.ball.VYM = 16.0, 0.0
}

// BenchmarkStepActive measures one active rolling substep (friction + position,
// no runners, no sample). The ball is re-armed periodically so it never reaches
// Dead; TestActiveBenchStaysRolling guards that.
func BenchmarkStepActive(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetBall(benchBall())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearm(sim)
		}
		sim.StepQuiet()
	}
}

// BenchmarkStep22Active measures one active rolling substep that also runs the
// 22-body contact scan (runners parked out of reach so no contact fires).
func BenchmarkStep22Active(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetBall(benchBall())
	runners := make([]Runner, 22)
	for i := range runners {
		runners[i] = Runner{X: 0.20 + float64(i)*0.03, Y: 0.15, VXM: 2.0, VYM: 0.5, ReachM: ControlRadiusM}
	}
	sim.SetRunners(runners)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearm(sim)
		}
		sim.StepQuiet()
	}
}

// BenchmarkStepDead measures the Dead path (no ball integration, no runners) so
// the active/dead cost split is explicit rather than hidden in one number.
func BenchmarkStepDead(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetBall(Ball{Mode: BallDead, X: 0.30, Y: 0.50})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sim.StepQuiet()
	}
}

// BenchmarkRecordSample measures pure authoritative sampling (one Sample append
// per substep). The sample slice length is rewound periodically so the backing
// array is reused and the number reflects steady-state appends, not growth.
func BenchmarkRecordSample(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.samples = make([]Sample, 0, 4096)
	sim.ball = benchBall()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(sim.samples) == cap(sim.samples) {
			sim.samples = sim.samples[:0]
		}
		sim.recordSample()
	}
}

// BenchmarkContactToCollection measures one realistic contest (release + run to
// first contact) at the recommended clock, quiet so it is all integration.
func BenchmarkContactToCollection(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sim, err := NewSim(DefaultConfig(), uint64(i))
		if err != nil {
			b.Fatal(err)
		}
		sim.ReleaseBall(ReleaseParams{StartX: 0.35, StartY: 0.50, SpeedMPS: 16.0, LateralJitterMPS: 0.2})
		sim.SetRunners([]Runner{{X: 0.60, Y: 0.50, VXM: -6.0, ReachM: ControlRadiusM}})
		for sim.ClockMS() < 4000 && sim.Ball().Mode != BallHeld {
			sim.StepQuiet()
		}
	}
}

// BenchmarkStepActiveAt reports the active rolling substep cost for each clock
// candidate, so the budget can be scaled by the chosen rate.
func BenchmarkStepActiveAt(b *testing.B) {
	for _, cs := range candidateSubsteps {
		b.Run(substepName(cs), func(b *testing.B) {
			sim, err := NewSim(Config{SubstepMS: cs, DecelMPS2: RollDecelMPS2}, 1)
			if err != nil {
				b.Fatal(err)
			}
			sim.SetBall(benchBall())
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i&(benchRearmEvery-1) == 0 {
					benchRearm(sim)
				}
				sim.StepQuiet()
			}
		})
	}
}

// TestActiveBenchStaysRolling guards the active-path benchmarks: the re-arm
// pattern must keep every candidate Rolling and advance the clock, so the
// benchmarked loop never silently degenerates to the Dead path.
func TestActiveBenchStaysRolling(t *testing.T) {
	const total = 64 * 500
	for _, cs := range candidateSubsteps {
		sim, err := NewSim(Config{SubstepMS: cs, DecelMPS2: RollDecelMPS2}, 1)
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		sim.SetBall(benchBall())
		dead := 0
		for i := 0; i < total; i++ {
			if i&(benchRearmEvery-1) == 0 {
				benchRearm(sim)
			}
			if sim.ball.Mode != BallRolling {
				dead++
			}
			sim.StepQuiet()
		}
		if dead != 0 {
			t.Fatalf("dt=%d ms: %d/%d dead steps in the active bench pattern", cs, dead, total)
		}
		if got := sim.ClockMS(); got != total*cs {
			t.Fatalf("dt=%d ms: clock %d, want %d", cs, got, total*cs)
		}
	}
}

func substepName(ms int) string {
	digits := []byte{}
	if ms == 0 {
		return "0ms"
	}
	for n := ms; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits) + "ms"
}

// inertialSlot returns the deterministic start and far-side intent for one of
// the 22 benchmark bodies, laid out across the pitch with real moving intents.
func inertialSlot(i int) (x, y, targetX, targetY float64) {
	x = 0.15 + float64(i%7)*0.10
	y = 0.20 + float64(i/7)*0.20
	targetX = 0.85
	if x > 0.5 {
		targetX = 0.15
	}
	return x, y, targetX, 1 - y
}

// inertial22 builds 22 inertial runners with REAL moving intents: each starts at
// rest and is told to sprint to the far side/opposite half, so the integrator
// stays on the active acceleration+lateral path rather than settling into a
// no-op. Used by the T-036 benchmark and its guard test.
func inertial22() []Runner {
	p := DefaultMotionParams()
	rs := make([]Runner, 22)
	for i := range rs {
		x, y, tx, ty := inertialSlot(i)
		rs[i] = InertialRunner(x, y, tx, ty, p)
	}
	return rs
}

// benchRearmInertial22 resets positions/velocities and re-publishes far intents
// so every re-arm window starts a fresh, genuinely moving acceleration. It
// writes in place to keep the benchmark allocation-free.
func benchRearmInertial22(sim *Sim) {
	for i := range sim.runners {
		x, y, tx, ty := inertialSlot(i)
		r := &sim.runners[i]
		r.X, r.Y = x, y
		r.VXM, r.VYM = 0, 0
		r.IntentX, r.IntentY = tx, ty
	}
}

// BenchmarkStep22InertialActive measures one active rolling substep with 22
// inertial bodies that are actually accelerating toward real moving intents.
// The ball is re-armed so it never stops; TestInertialBenchStaysActive guards
// both the rolling ball and the moving bodies.
func BenchmarkStep22InertialActive(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetBall(benchBall())
	sim.SetRunners(inertial22())
	benchRearmInertial22(sim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearm(sim)
			benchRearmInertial22(sim)
		}
		sim.StepQuiet()
	}
}

// BenchmarkStep22KinematicActive is the T-035 constant-velocity counterpart, so
// the inertial overhead can be read directly against the old number.
func BenchmarkStep22KinematicActive(b *testing.B) {
	sim, err := NewSim(DefaultConfig(), 1)
	if err != nil {
		b.Fatal(err)
	}
	sim.SetBall(benchBall())
	runners := make([]Runner, 22)
	for i := range runners {
		runners[i] = Runner{X: 0.20 + float64(i)*0.03, Y: 0.15, VXM: 2.0, VYM: 0.5, ReachM: ControlRadiusM}
	}
	sim.SetRunners(runners)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&(benchRearmEvery-1) == 0 {
			benchRearm(sim)
		}
		sim.StepQuiet()
	}
}
