package liveplay

import (
	"fmt"
	"math"
	"testing"
)

// T-047 paired six-scene comparison aggregator (test-only, isolated).
//
// For each prespecified scene the SAME runners/setup/mid are run three times with
// only the controller mode differing (chase vs fresh-only vs within-age), and the
// full-run paired differences are reported: max player position/velocity delta,
// max ball position/velocity delta, mode/owner mismatches, observation/target
// differences, and the ORDERED actual contact/event sequence with exact fractional
// timestamps over the full horizon. No runtime change; no claim that prediction is
// better. A negative/insufficient result is a valid outcome.

// t047SceneDef is one prespecified paired scene.
type t047SceneDef struct {
	name    string
	runners []Runner
	setup   func(*Sim)
	mid     func(*Sim, int)
	horizon int
	note    string
}

// t047PairedScenes returns the six prespecified scenes plus an explicitly
// SYNTHETIC hand-set stale counterexample (kept separate from the actual
// simulation stale case).
func t047PairedScenes() []t047SceneDef {
	return []t047SceneDef{
		{
			name: "crossing", horizon: 1200, note: "actual simulation",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.53, Y: 0.50, VYM: 7.0}) },
		},
		{
			name: "decel", horizon: 600, note: "actual simulation",
			runners: []Runner{t047Sensor(0.48, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 5.0}) },
		},
		{
			name: "delay", horizon: 500, note: "actual simulation (real ReleasePass)",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 200)},
			setup: func(s *Sim) {
				s.ReleasePass(PassParams{StartX: 0.50, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 6, Passer: -1})
			},
		},
		{
			name: "stale-outside", horizon: 400, note: "actual simulation: ball escapes a small radius faster than the runner",
			runners: []Runner{t047Sensor(0.50, 0.50, 0.02, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 14.0}) },
		},
		{
			name: "deflection", horizon: 300, note: "actual simulation (contact-local miscontrol)",
			runners: []Runner{t047Sensor(0.50, 0.50, 200, 0)},
			setup: func(s *Sim) {
				s.SetBall(Ball{Mode: BallLoose, X: 0.50, Y: 0.50})
				s.controlError = 1.0
			},
		},
		{
			name: "boundary", horizon: 300, note: "actual simulation (ball exits the pitch)",
			runners: []Runner{t047Sensor(0.98, 0.50, 200, 0)},
			setup:   func(s *Sim) { s.SetBall(Ball{Mode: BallRolling, X: 0.995, Y: 0.50, VXM: 5.0}) },
		},
		{
			name: "stale-synthetic", horizon: 260, note: "SYNTHETIC hand-set record (counterexample only, NOT a simulation)",
			runners: []Runner{t047Sensor(0.50, 0.50, 0.02, 0)},
			setup: func(s *Sim) {
				s.SetBall(Ball{Mode: BallDead, X: 0.9, Y: 0.9})
				s.runners[0].Perception = &BallObservation{
					RadiusM: 0.02, Seen: true, ClockMS: 0, X: 0.5, Y: 0.5, VXM: 6, VYM: 0,
					Mode: BallRolling, Owner: -1,
				}
				s.runners[0].ReactAtMS = 0
			},
		},
	}
}

// t047PairMetrics is the full-run paired difference of one policy vs the chase.
type t047PairMetrics struct {
	Steps          int
	MaxPlayerPos   float64
	MaxPlayerVel   float64
	BallPos        float64
	BallVel        float64
	ModeMismatch   int
	OwnerMismatch  int
	ObsDiff        int
	TargetDiff     int
	MaxTargetDelta float64
	Contacts       []string
}

// String renders a compact evidence line.
func (m t047PairMetrics) String() string {
	return fmt.Sprintf("steps=%d posΔmax=%.4fm velΔmax=%.4f m/s ballPosΔmax=%.4fm ballVelΔmax=%.4f m/s "+
		"modeΔ=%d ownerΔ=%d obsΔ=%d targetΔ=%d maxTargetΔ=%.4fm",
		m.Steps, m.MaxPlayerPos, m.MaxPlayerVel, m.BallPos, m.BallVel,
		m.ModeMismatch, m.OwnerMismatch, m.ObsDiff, m.TargetDiff, m.MaxTargetDelta)
}

// t047ContactSeq returns the ORDERED event sequence (control/deflect/out/stop)
// with exact fractional timestamps (ms) over the full horizon.
func t047ContactSeq(recs []t047StepRec) []string {
	var out []string
	for _, rec := range recs {
		for _, e := range rec.Events {
			switch e.Kind {
			case PassEventControl, PassEventDeflect, PassEventOut, PassEventStop:
				exact := float64(rec.ClockMS) - float64(SubstepMS)*(1-e.Frac)
				out = append(out, fmt.Sprintf("%s/r%d@%.2fms", e.Kind, e.Runner, exact))
			}
		}
	}
	return out
}

// t047PairMetricsOf compares a policy run against the chase run step by step.
func t047PairMetricsOf(chase, pol []t047StepRec) t047PairMetrics {
	m := t047PairMetrics{Contacts: t047ContactSeq(pol)}
	n := len(chase)
	if len(pol) < n {
		n = len(pol)
	}
	m.Steps = n
	for s := 0; s < n; s++ {
		c, p := chase[s], pol[s]
		if d := DistM(c.Ball.X, c.Ball.Y, p.Ball.X, p.Ball.Y); d > m.BallPos {
			m.BallPos = d
		}
		if dv := math.Sqrt(sq(c.Ball.VXM-p.Ball.VXM) + sq(c.Ball.VYM-p.Ball.VYM)); dv > m.BallVel {
			m.BallVel = dv
		}
		if c.Ball.Mode != p.Ball.Mode {
			m.ModeMismatch++
		}
		if c.Owner != p.Owner {
			m.OwnerMismatch++
		}
		rn := len(c.Runners)
		if len(p.Runners) < rn {
			rn = len(p.Runners)
		}
		for i := 0; i < rn; i++ {
			cr, pr := c.Runners[i], p.Runners[i]
			if d := DistM(cr.X, cr.Y, pr.X, pr.Y); d > m.MaxPlayerPos {
				m.MaxPlayerPos = d
			}
			if dv := math.Sqrt(sq(cr.VXM-pr.VXM) + sq(cr.VYM-pr.VYM)); dv > m.MaxPlayerVel {
				m.MaxPlayerVel = dv
			}
			if cr.Seen != pr.Seen || cr.ObsClockMS != pr.ObsClockMS || cr.ObsX != pr.ObsX ||
				cr.ObsY != pr.ObsY || cr.ObsVXM != pr.ObsVXM || cr.ObsVYM != pr.ObsVYM || cr.ObsMode != pr.ObsMode {
				m.ObsDiff++
			}
			if cr.IX != pr.IX || cr.IY != pr.IY {
				m.TargetDiff++
				if d := DistM(cr.IX, cr.IY, pr.IX, pr.IY); d > m.MaxTargetDelta {
					m.MaxTargetDelta = d
				}
			}
		}
	}
	return m
}

// sq returns v*v.
func sq(v float64) float64 { return v * v }

// TestT047PairedSceneAggregator runs each prespecified scene under chase,
// fresh-only and within-age with identical setup/mid and reports the paired
// full-run differences and ordered contact sequences over the full horizon.
func TestT047PairedSceneAggregator(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	for _, sc := range t047PairedScenes() {
		chaseRecs, _ := t047Run(t, cfg, sc.runners, t047NewController(t047ModeChase), sc.horizon, sc.setup, sc.mid)
		freshRecs, _ := t047Run(t, cfg, sc.runners, t047NewController(t047ModePredictFresh), sc.horizon, sc.setup, sc.mid)
		withinRecs, _ := t047Run(t, cfg, sc.runners, t047NewController(t047ModePredictWithinAge), sc.horizon, sc.setup, sc.mid)

		if len(freshRecs) != len(chaseRecs) || len(withinRecs) != len(chaseRecs) {
			t.Fatalf("%s: step counts differ chase=%d fresh=%d within=%d", sc.name, len(chaseRecs), len(freshRecs), len(withinRecs))
		}
		if len(chaseRecs) == 0 || chaseRecs[len(chaseRecs)-1].ClockMS != sc.horizon {
			t.Fatalf("%s: run did not reach the full horizon %d", sc.name, sc.horizon)
		}

		mF := t047PairMetricsOf(chaseRecs, freshRecs)
		mW := t047PairMetricsOf(chaseRecs, withinRecs)
		for _, m := range []t047PairMetrics{mF, mW} {
			if !isFinite(m.MaxPlayerPos) || !isFinite(m.MaxPlayerVel) || !isFinite(m.BallPos) || !isFinite(m.BallVel) || !isFinite(m.MaxTargetDelta) {
				t.Fatalf("%s: non-finite paired metric %s", sc.name, m)
			}
		}

		t.Logf("[%s | %s] horizon=%dms", sc.name, sc.note, sc.horizon)
		t.Logf("  chase contacts:  %v", t047ContactSeq(chaseRecs))
		t.Logf("  fresh vs chase:  %s", mF)
		t.Logf("  fresh contacts:  %v", mF.Contacts)
		t.Logf("  within vs chase: %s", mW)
		t.Logf("  within contacts: %v", mW.Contacts)
	}
	t.Logf("paired aggregator: %d scenes compared over the full horizon", len(t047PairedScenes()))
}
