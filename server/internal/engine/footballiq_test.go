package engine_test

import (
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// Football IQ regression suite (T-012): moment-level plausibility, not just
// distributions (owner lesson: realistic aggregates ≠ realistic moments).

const iqMatches = 50

// TestDefendersRarelyShoot: open-play strikes by GK/CB/DM stay rare (owners notice
// a centre-back thundering one in from 25 metres far more than any statistic).
func TestDefendersRarelyShoot(t *testing.T) {
	t.Parallel()

	var shots, defenderStrikes int
	for seed := 1; seed <= iqMatches; seed++ {
		res, _, err := engine.Simulate(enginetest.SampleInput(uint64(seed)))
		if err != nil {
			t.Fatalf("Simulate: %v", err)
		}
		for _, side := range []engine.TeamStats{res.Stats.Home, res.Stats.Away} {
			shots += side.Shots
			defenderStrikes += side.StrikesFromDefenders
		}
	}
	share := float64(defenderStrikes) / float64(max(shots, 1))
	t.Logf("defender strike share: %.1f%% (%d/%d)", 100*share, defenderStrikes, shots)
	if share > 0.05 {
		t.Errorf("defender strike share = %.1f%%, want ≤ 5%%", 100*share)
	}
}

// TestCentreBacksHoldShape: in open play centre-backs may step out, not maraud.
// Frames within 20 s of a dead ball are excluded — set-piece box visits (headers to
// win corners, scrambles) are legitimate football; the wilderness hunt targets the
// open-play "sprinting stopper" instead.
func TestCentreBacksHoldShape(t *testing.T) {
	t.Parallel()

	const cbCap = 0.85
	worst := 0.0
	for seed := 1; seed <= iqMatches; seed++ {
		res, _, err := engine.Simulate(enginetest.SampleInput(uint64(seed)))
		if err != nil {
			t.Fatalf("Simulate: %v", err)
		}
		dead := deadBallWindows(res.Events)
		for _, kf := range res.Keyframes {
			if dead(int32(kf.TMs / 1000)) {
				continue
			}
			for _, slot := range []int{1, 2} {
				worst = maxf(worst, kf.Players[slot].X)
			}
			for _, slot := range []int{12, 13} {
				worst = maxf(worst, 1-kf.Players[slot].X)
			}
		}
	}
	t.Logf("furthest open-play centre-back territory reached: %.3f", worst)
	if worst > cbCap {
		t.Errorf("centre-back reached %.3f in open play, want ≤ %.2f", worst, cbCap)
	}
}

// deadBallWindows returns a predicate covering 20 s after every restart-worthy event.
func deadBallWindows(events []engine.Event) func(tick int32) bool {
	type span struct{ lo, hi int32 }
	var spans []span
	for _, ev := range events {
		switch ev.Kind {
		case engine.EventGoal, engine.EventShotSaved, engine.EventShotOffTarget,
			engine.EventFoul, engine.EventOffside, engine.EventRedCard, engine.EventInjury:
			spans = append(spans, span{ev.Tick, ev.Tick + 20})
		}
	}
	return func(tick int32) bool {
		for _, s := range spans {
			if tick >= s.lo && tick <= s.hi {
				return true
			}
		}
		return false
	}
}

// TestAttackersScoreTheGoals: goals overwhelmingly come from the attacking cast —
// the "screamer from deep" and "keeper lob" species stay rare birds.
func TestAttackersScoreTheGoals(t *testing.T) {
	t.Parallel()

	goals, attackerGoals := 0, 0
	for seed := 1; seed <= iqMatches; seed++ {
		in := enginetest.SampleInput(uint64(seed))
		res, _, err := engine.Simulate(in)
		if err != nil {
			t.Fatalf("Simulate: %v", err)
		}
		posByID := map[engine.PlayerID]engine.Pos{}
		for _, p := range append(append([]engine.PlayerSnapshot{}, in.Home.Players...), in.Away.Players...) {
			posByID[p.ID] = p.Pos
		}
		for _, ev := range res.Events {
			if ev.Kind != engine.EventGoal {
				continue
			}
			goals++
			switch posByID[ev.Player] {
			case engine.PosST, engine.PosW, engine.PosAM:
				attackerGoals++
			}
		}
	}
	share := float64(attackerGoals) / float64(max(goals, 1))
	t.Logf("attacker goal share: %.1f%% (%d/%d)", 100*share, attackerGoals, goals)
	if share < 0.55 {
		t.Errorf("attacker goal share = %.1f%%, want ≥ 55%%", 100*share)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
