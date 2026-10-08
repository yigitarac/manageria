package engine_test

import (
	"sort"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// TestBehaviouralKPIs encodes the ADR-0011 behavioural contract as assertions
// (T-017): the option-menu era must never regress into fragmentation (the v6
// disease: 429 chains/match, median 1 touch) nor into sterile keep-ball.
// Fixed seed window 42..61 — deterministic, same numbers as `simcli -kpi 20`.
// Norm band 100–155 set for the v9 behaviour era (owner call, 2026-10-08):
// active defending and second-ball scrambles legitimately break possession a
// touch more often than the v7 passive-defending calibration.
func TestBehaviouralKPIs(t *testing.T) {
	t.Parallel()

	const n = 20
	var chains, touches, passes int
	var chainLens []int
	regimes := map[string]bool{}
	for i := 0; i < n; i++ {
		res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
			return engine.Simulate(enginetest.SampleInput(42 + uint64(i)))
		})
		chains += len(res.Chains)
		for _, c := range res.Chains {
			chainLens = append(chainLens, c.Touches)
			regimes[c.Regime] = true
		}
		for _, tc := range res.Touches {
			touches++
			if tc.Kind == engine.TouchPass {
				passes++
			}
		}
	}
	f := float64(n)

	if avg := float64(chains) / f; avg < 100 || avg > 155 {
		t.Errorf("chains/match = %.0f, want 100–155 (fragmentation guard)", avg)
	}

	sort.Ints(chainLens)
	median := chainLens[len(chainLens)/2]
	if median < 4 || median > 8 {
		t.Errorf("median touches/chain = %d, want 4–8", median)
	}

	if avg := float64(passes) / f; avg < 700 || avg > 1000 {
		t.Errorf("passes/match = %.0f, want 700–1000", avg)
	}

	// Six readable regimes: the classifier must keep telling distinct plays
	// apart (buildUp vs progression vs finalThird vs transition vs setPiece
	// vs regroup) — a collapsed mix means the ledger stopped narrating.
	for _, r := range []engine.Regime{engine.RegimeBuildUp, engine.RegimeProgression, engine.RegimeFinalThird,
		engine.RegimeTransition, engine.RegimeSetPiece, engine.RegimeRegroup} {
		if !regimes[r.String()] {
			t.Errorf("regime %s never appears in the %d-match window", r, n)
		}
	}
}
