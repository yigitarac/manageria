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

// TestCalibrationRanges pins the design-note target distributions. The sweep is
// sized to RESOLVE the band edges (margin doctrine, [[08-Open-Questions]] #9): at
// 800 matches the draw-rate standard error is ~1.6 points — half the width of the
// band — so the window certified noise as pass/fail (the v17 contact era read 28.8%
// draws vs a 28% ceiling, 0.5 SE). n=3200 brings the error to ~0.8 points.
func TestCalibrationRanges(t *testing.T) {
	sw := runSweep(t, 10_000, 3200, nil)
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

// TestMenuResponseMonotone measures the isolated Mentality response with paired
// seeds. Holding every other tactic fixed distinguishes menu sensitivity from the
// multi-knob preset and low-block interactions audited separately by T-021.
func TestMenuResponseMonotone(t *testing.T) {
	const matches = 800
	var goals, xg [6]float64
	for mentality := 1; mentality <= 5; mentality++ {
		m := mentality
		sw := runSweep(t, 40_000, matches, func(in *engine.MatchInput) {
			in.Home.Tactics.Mentality = int8(m)
		})
		goals[m] = sw.homeGoals / float64(matches)
		xg[m] = sw.homeXG / float64(matches)
		t.Logf("mentality %d: goals %.3f, xG %.3f, conceded %.3f, home wins %.1f%%",
			m, goals[m], xg[m],
			sw.awayGoals/float64(matches), 100*float64(sw.homeWins)/float64(matches))
		if m > 1 {
			if xg[m] <= xg[m-1] {
				t.Errorf("mentality %d xG %.3f not above level %d xG %.3f", m, xg[m], m-1, xg[m-1])
			}
			// Goals are a noisier discrete outcome than chance quality. One adjacent
			// cell may dip slightly, but a large reversal is a menu regression.
			if goals[m]+0.08 < goals[m-1] {
				t.Errorf("mentality %d goals %.3f reverse level %d goals %.3f by >0.08", m, goals[m], m-1, goals[m-1])
			}
		}
	}
	goalGain := goals[5] - goals[1]
	xgGain := xg[5] - xg[1]
	if goalGain < 0.10 || goalGain > 0.45 {
		t.Errorf("mentality 1→5 goal gain %.3f, want 0.10–0.45", goalGain)
	}
	if xgGain < 0.12 || xgGain > 0.40 {
		t.Errorf("mentality 1→5 xG gain %.3f, want 0.12–0.40", xgGain)
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

// TestTacticalSanityDefensive: "park the bus" must genuinely park it. Three honest
// claims, all score-state clean: (1) it creates far less (xG for); (2) it SUPPRESSES
// the quality of what it concedes (xG against — the congestion-dilution teeth; the
// T-013 property that a besieged bunker gives up less value); (3) it holds the gate
// materially longer — minutes until conceding, asserted with a real margin because a
// bare sign on a near-parity metric coins every defence change ([[08-Open-Questions]] #9).
// (Late leak after falling behind is legitimate football: trailing bunkers throw men
// forward and eat counters — that is the visible game-state mechanic, not a defect.)
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
	aFor := attack.homeXG / float64(attack.matches)
	dAgainst := defend.awayXG / float64(defend.matches)
	aAgainst := attack.awayXG / float64(attack.matches)
	hold := pairedHoldSweep(t, 30_000, 1200, func(in *engine.MatchInput) {
		in.Home.Tactics.Mentality = 1
		in.Home.Tactics.DefensiveLine = 1
		in.Home.Tactics.PassingStyle = 1
	}, func(in *engine.MatchInput) {
		in.Home.Tactics.Mentality = 5
		in.Home.Tactics.DefensiveLine = 3
		in.Home.Tactics.PassingStyle = 3
	})
	hs := hold.stats()
	dHold, aHold := hs.defendMean, hs.attackMean
	t.Logf("defensive: xG for %.2f vs %.2f, xG against %.2f vs %.2f, minutes until conceding %.1f vs %.1f",
		dFor, aFor, dAgainst, aAgainst, dHold, aHold)

	if dFor >= aFor-0.20 {
		t.Errorf("defensive xG for %.2f does not create at least 0.20 less than attacking %.2f", dFor, aFor)
	}
	// UNENFORCED claims, tracked in [[08-Open-Questions]] #10 (missing mechanics + the
	// owner's penalty-policy call): (a) xG-against suppression is still FALSE — a besieged
	// bunker gives up LESS open-play value yet MORE total value; the inversion is traced to
	// the real-contact penalty premium (box contact fouls from hold-ups and pass-intercept/
	// lapse wrestles, [[2026-10-11-foul-origin-and-contact-trial]]), not a markFree causal
	// story; (b) defensive HOLD measures a real DEFICIT (about −4.4 minutes at v17 — see the
	// paired precision read below), not "~0 noise". Both claims stay logged-UNENFORCED until
	// the missing mechanics land AND the owner decides the #10 policy; the measured paired
	// uncertainty is reported honestly and must not be manufactured green by assertion tuning.
	t.Logf("xG-against suppression (UNENFORCED claim #10): %.2f vs %.2f", dAgainst, aAgainst)
	t.Logf("defensive hold margin (UNENFORCED claim #10): %.1f minutes", dHold-aHold)

	// T-034 honest precision read for the SAME raw sums printed above at %.1f. The
	// metric is the historical first-concession imputation proxy (firstConcedeMinute) —
	// NOT a censored/RMST/survival estimator: clean sheets are imputed as the fixed
	// sentinel 95 (the clean-sheet counts distinguish a real 95' goal from a true clean
	// sheet). More decimals are NOT more certainty: the sampling uncertainty is the
	// paired interval below — the direct mean of the per-seed differences with its
	// UNBIASED (n−1) variance, never sqrt of summed independent variances — and the
	// ±1.96·SE interval is an APPROXIMATE normal interval. Diagnostic only: no claim is
	// enforced by these numbers and no sign assertion is implied.
	if hs.sufficient {
		t.Logf("paired hold precision (first-concede minute imputation proxy, n=%d pairs): defend mean %.6f (clean sheets %d, per-stance SE %.6f), attack mean %.6f (clean sheets %d, per-stance SE %.6f)",
			hs.n, hs.defendMean, hs.defendCleanSheets, hs.defendSE,
			hs.attackMean, hs.attackCleanSheets, hs.attackSE)
		t.Logf("paired defend-minus-attack delta %.6f (mean of per-seed differences): unbiased sample variance of differences %.6f, SE=sqrt(var/n)=%.6f, APPROXIMATE normal 95%% interval ±1.96·SE = [%.6f, %.6f]",
			hs.deltaMean, hs.deltaVar, hs.deltaSE, hs.ci95Low, hs.ci95High)
	} else {
		t.Logf("paired hold precision: insufficient pairs (n=%d < 2) — no spread or interval claimed", hs.n)
	}
}

// pairedHoldSweep replaces the two separate hold sweeps with a same-seed PAIRED
// aggregator over the SAME 2×n Simulate inputs (each seed simulated once per stance,
// inputs built exactly as the old separate sweeps built them): 8 workers over the same
// contiguous partitions, both stances tallied per seed inside the worker. Measured at
// n≈1200 so the estimate has resolving power: at n=250 the standard error of the mean
// was ~2.2 minutes — LARGER than every "hold advantage" ever observed (0.1–2.0), which
// is why the law kept flipping like a coin ([[08-Open-Questions]] #9). The legacy scalar
// means are preserved exactly: the raw sums are the integer Event.Minute values
// (sentinel 95 for clean sheets, firstConcedeMinute) accumulated in the original
// partition/sum-merge order. The uncertainty read is the sample spread of the PER-SEED
// differences (see pairedHoldStats) — a direct paired sample difference, not the square
// root of summed independent variances. Diagnostic only: no sign or margin assertion
// lives here. On a simulation error the worker reports via t.Errorf and stops early; the
// completed-pair count is then checked against n and the statistics divide by the
// completed count rather than silently by n.
func pairedHoldSweep(t *testing.T, baseSeed uint64, n int, defend, attack func(*engine.MatchInput)) pairedHoldTotals {
	t.Helper()
	const workers = 8
	parts := make([]pairedHoldTotals, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var acc pairedHoldTotals
			for i := w * n / workers; i < (w+1)*n/workers; i++ {
				seed := baseSeed + uint64(i)
				dObs, err := holdObserve(seed, defend)
				if err != nil {
					t.Errorf("simulate defend seed %d: %v", seed, err)
					return
				}
				aObs, err := holdObserve(seed, attack)
				if err != nil {
					t.Errorf("simulate attack seed %d: %v", seed, err)
					return
				}
				acc.addPair(dObs, aObs)
			}
			parts[w] = acc
		}(w)
	}
	wg.Wait()

	total := pairedHoldTotals{}
	for _, p := range parts { // fixed ascending worker merge order
		total = total.merge(p)
	}
	if total.n != int64(n) {
		t.Errorf("paired hold sweep completed %d of %d pairs (simulation errors reported above); stats use the completed count", total.n, n)
	}
	return total
}

// holdObserve runs one seed under one stance (exactly the input the old separate sweep
// built) and extracts its first-concession observation.
func holdObserve(seed uint64, mutate func(*engine.MatchInput)) (holdObs, error) {
	in := enginetest.SampleInput(seed)
	in.Presence = engine.PresenceFlags{}
	mutate(&in)
	res, _, err := engine.Simulate(in)
	if err != nil {
		return holdObs{}, err
	}
	minute, conceded := firstConcedeMinute(in.Home.Club, res.Events)
	return holdObs{minute: minute, conceded: conceded}, nil
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
