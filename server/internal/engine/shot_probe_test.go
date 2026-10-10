package engine_test

import (
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

type shotProbeTotals struct {
	count                int
	xg, distance         float64
	congestion, markFree float64
	blockProb            float64
}

func TestShotDiagnosticsDoNotChangeMatch(t *testing.T) {
	in := enginetest.SampleInput(42)
	want, _, err := engine.Simulate(in)
	if err != nil {
		t.Fatal(err)
	}
	got, shots, err := engine.DiagnoseShots(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("diagnostics changed the match result")
	}
	if len(shots) == 0 {
		t.Fatal("diagnostics captured no shots")
	}
}

func (a *shotProbeTotals) add(p engine.ShotDiagnostic) {
	a.count++
	a.xg += p.XG
	a.distance += p.Distance
	a.congestion += p.Congestion
	a.markFree += p.MarkFree
	a.blockProb += p.BlockProb
}

// TestDefensiveShotMix is a reproducible diagnostic for the missing defensive
// suppression property. It uses the same paired seeds and tactics as the law.
func TestDefensiveShotMix(t *testing.T) {
	if os.Getenv("ENGINE_SHOT_PROBE") != "1" {
		t.Skip("set ENGINE_SHOT_PROBE=1 to run the 2400-match diagnostic")
	}
	const matches = 1200
	for _, setup := range []struct {
		name   string
		mutate func(*engine.MatchInput)
	}{
		{"bunker", func(in *engine.MatchInput) {
			in.Home.Tactics.Mentality = 1
			in.Home.Tactics.DefensiveLine = 1
			in.Home.Tactics.PassingStyle = 1
		}},
		{"attack", func(in *engine.MatchInput) {
			in.Home.Tactics.Mentality = 5
			in.Home.Tactics.DefensiveLine = 3
			in.Home.Tactics.PassingStyle = 3
		}},
	} {
		bySource := map[string]*shotProbeTotals{}
		var all shotProbeTotals
		var bookedXG float64
		for i := 0; i < matches; i++ {
			in := enginetest.SampleInput(30_000 + uint64(i))
			in.Presence = engine.PresenceFlags{}
			setup.mutate(&in)
			res, probes, err := engine.DiagnoseShots(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range probes {
				if p.Team != 1 {
					continue
				}
				all.add(p)
				bucket := bySource[p.Source]
				if bucket == nil {
					bucket = &shotProbeTotals{}
					bySource[p.Source] = bucket
				}
				bucket.add(p)
			}
			bookedXG += res.Stats.Away.XG
		}
		t.Logf("%s: booked xG %.3f, probed xG %.3f, shots %.2f/match, mean distance %.3f, congestion %.3f, mark freedom %.3f, block chance %.3f",
			setup.name, bookedXG/matches, all.xg/matches, float64(all.count)/matches,
			all.distance/float64(all.count), all.congestion/float64(all.count),
			all.markFree/float64(all.count), all.blockProb/float64(all.count))
		// MatchResult rounds each match to cents; summed rounding can differ
		// from the unrounded probe total by at most 0.005 per match.
		if math.Abs(bookedXG-all.xg) > 0.005*matches+1e-6 {
			t.Errorf("%s: shot probes miss %.6f xG beyond result rounding", setup.name, bookedXG-all.xg)
		}
		for _, source := range []string{"strike", "cross", "corner", "pattern", "free_kick", "penalty"} {
			b := bySource[source]
			if b == nil {
				continue
			}
			t.Logf("%s %s: shots %.2f/match, xG %.3f/match, xG/shot %.3f, distance %.3f, congestion %.3f, mark freedom %.3f, block chance %.3f",
				setup.name, source, float64(b.count)/matches, b.xg/matches, b.xg/float64(b.count),
				b.distance/float64(b.count), b.congestion/float64(b.count),
				b.markFree/float64(b.count), b.blockProb/float64(b.count))
		}
	}
}
