package engine_test

import (
	"sync"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// Calibration and tactical sanity sweeps ([[Match-Engine]] Testing). All sweeps use
// fixed seed ranges and parallel workers over disjoint seeds — statistics are stable
// across runs and machines. Live presence is off: pure team strength + home ground.

type sweep struct {
	matches              int
	homeGoals, awayGoals float64
	homeWins, draws      int
	awayWins             int
	homeXG, awayXG       float64
	homeFat, awayFat     float64
	homeTO, awayTO       float64
}

func (a sweep) add(b sweep) sweep {
	a.matches += b.matches
	a.homeGoals += b.homeGoals
	a.awayGoals += b.awayGoals
	a.homeWins += b.homeWins
	a.draws += b.draws
	a.awayWins += b.awayWins
	a.homeXG += b.homeXG
	a.awayXG += b.awayXG
	a.homeFat += b.homeFat
	a.awayFat += b.awayFat
	a.homeTO += b.homeTO
	a.awayTO += b.awayTO
	return a
}

func (a sweep) totals() (goals, homeWinPct, drawPct, awayWinPct float64) {
	f := float64(a.matches)
	goals = (a.homeGoals + a.awayGoals) / f
	homeWinPct = 100 * float64(a.homeWins) / f
	drawPct = 100 * float64(a.draws) / f
	awayWinPct = 100 * float64(a.awayWins) / f
	return goals, homeWinPct, drawPct, awayWinPct
}

// runSweep simulates n matches over seeds baseSeed..baseSeed+n-1 with parallel workers.
func runSweep(t *testing.T, baseSeed uint64, n int, mutate func(*engine.MatchInput)) sweep {
	t.Helper()

	const workers = 8
	parts := make([]sweep, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			start := w * n / workers
			end := (w + 1) * n / workers
			var acc sweep
			for i := start; i < end; i++ {
				in := enginetest.SampleInput(baseSeed + uint64(i))
				in.Presence = engine.PresenceFlags{}
				if mutate != nil {
					mutate(&in)
				}
				res, _, err := engine.Simulate(in)
				if err != nil {
					t.Errorf("simulate seed %d: %v", baseSeed+uint64(i), err)
					return
				}
				acc.matches++
				acc.homeGoals += float64(res.Score.Home)
				acc.awayGoals += float64(res.Score.Away)
				switch {
				case res.Score.Home > res.Score.Away:
					acc.homeWins++
				case res.Score.Home < res.Score.Away:
					acc.awayWins++
				default:
					acc.draws++
				}
				acc.homeXG += res.Stats.Home.XG
				acc.awayXG += res.Stats.Away.XG
				acc.homeFat += res.Stats.Home.AvgFatigue
				acc.awayFat += res.Stats.Away.AvgFatigue
				acc.homeTO += float64(res.Stats.Home.Turnovers)
				acc.awayTO += float64(res.Stats.Away.Turnovers)
			}
			parts[w] = acc
		}(w)
	}
	wg.Wait()

	total := sweep{}
	for _, p := range parts {
		total = total.add(p)
	}
	return total
}

// TestCalibrationRanges pins the design-note target distributions (800 matches).
func TestCalibrationRanges(t *testing.T) {
	sw := runSweep(t, 10_000, 800, nil)
	goals, homeWin, draw, awayWin := sw.totals()
	t.Logf("calibration: %.2f goals, home %.1f%%, draw %.1f%%, away %.1f%%, xg %.2f-%.2f",
		goals, homeWin, draw, awayWin, sw.homeXG/float64(sw.matches), sw.awayXG/float64(sw.matches))

	if goals < 2.5 || goals > 2.9 {
		t.Errorf("avg goals = %.2f, want 2.5–2.9", goals)
	}
	if homeWin < 43 || homeWin > 47 {
		t.Errorf("home win %% = %.1f, want 43–47", homeWin)
	}
	if draw < 24 || draw > 28 {
		t.Errorf("draw %% = %.1f, want 24–28", draw)
	}
	if awayWin < 25 || awayWin > 33 {
		t.Errorf("away win %% = %.1f, want 25–33", awayWin)
	}
}

// TestTacticalSanityPressing: high pressing costs more fatigue and forces more
// turnovers from the opposition than a low block.
func TestTacticalSanityPressing(t *testing.T) {
	high := runSweep(t, 20_000, 250, func(in *engine.MatchInput) {
		in.Home.Tactics.Pressing = 3
	})
	low := runSweep(t, 20_000, 250, func(in *engine.MatchInput) {
		in.Home.Tactics.Pressing = 1
	})

	highF := high.homeFat / float64(high.matches)
	lowF := low.homeFat / float64(low.matches)
	highTO := high.awayTO / float64(high.matches)
	lowTO := low.awayTO / float64(low.matches)
	t.Logf("pressing: fatigue %.1f vs %.1f, forced turnovers %.1f vs %.1f", highF, lowF, highTO, lowTO)

	if highF <= lowF {
		t.Errorf("high pressing fatigue %.2f not above low %.2f", highF, lowF)
	}
	if highTO <= lowTO {
		t.Errorf("high pressing forced turnovers %.2f not above low %.2f", highTO, lowTO)
	}
}

// TestTacticalSanityDefensive: a parked bus lowers xG for AND against.
func TestTacticalSanityDefensive(t *testing.T) {
	defend := runSweep(t, 30_000, 250, func(in *engine.MatchInput) {
		in.Home.Tactics.Mentality = 1
		in.Home.Tactics.DefensiveLine = 1
		in.Home.Tactics.PassingStyle = 1
	})
	attack := runSweep(t, 30_000, 250, func(in *engine.MatchInput) {
		in.Home.Tactics.Mentality = 5
		in.Home.Tactics.DefensiveLine = 3
		in.Home.Tactics.PassingStyle = 3
	})

	dFor := defend.homeXG / float64(defend.matches)
	dAgainst := defend.awayXG / float64(defend.matches)
	aFor := attack.homeXG / float64(attack.matches)
	aAgainst := attack.awayXG / float64(attack.matches)
	t.Logf("defensive: xG for %.2f vs %.2f, against %.2f vs %.2f", dFor, aFor, dAgainst, aAgainst)

	if dFor >= aFor {
		t.Errorf("defensive xG for %.2f not below attacking %.2f", dFor, aFor)
	}
	if dAgainst >= aAgainst {
		t.Errorf("defensive xG against %.2f not below attacking %.2f", dAgainst, aAgainst)
	}
}

// BenchmarkSimulate guards the < 20 ms per-match budget ([[Match-Engine]]).
func BenchmarkSimulate(b *testing.B) {
	in := enginetest.SampleInput(1)
	in.Presence = engine.PresenceFlags{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := engine.Simulate(in); err != nil {
			b.Fatal(err)
		}
	}
}
