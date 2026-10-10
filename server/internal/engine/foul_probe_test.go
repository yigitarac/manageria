package engine_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

func TestFoulDiagnosticsDoNotChangeMatch(t *testing.T) {
	in := enginetest.SampleInput(42)
	want, _, err := engine.Simulate(in)
	if err != nil {
		t.Fatal(err)
	}
	got, fouls, err := engine.DiagnoseFouls(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("foul diagnostics changed the match result")
	}
	if len(fouls) == 0 {
		t.Fatal("foul diagnostics captured no fouls")
	}
}

type foulTotals struct {
	fouls, inBox, penalties int
	separation, ballGap     float64
}

func (a *foulTotals) add(p engine.FoulDiagnostic) {
	a.fouls++
	if p.InBox {
		a.inBox++
	}
	if p.Penalty {
		a.penalties++
	}
	a.separation += p.Separation
	a.ballGap += abs(p.AttackerX-p.BallX) + abs(p.AttackerY-p.BallY)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestDefensiveFoulOrigins(t *testing.T) {
	if os.Getenv("ENGINE_FOUL_PROBE") != "1" {
		t.Skip("set ENGINE_FOUL_PROBE=1 to run the 2400-match diagnostic")
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
		bySource := map[string]*foulTotals{}
		var all foulTotals
		for i := 0; i < matches; i++ {
			in := enginetest.SampleInput(30_000 + uint64(i))
			in.Presence = engine.PresenceFlags{}
			setup.mutate(&in)
			_, fouls, err := engine.DiagnoseFouls(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range fouls {
				if p.DefendingTeam != 0 {
					continue
				}
				all.add(p)
				bucket := bySource[p.Source]
				if bucket == nil {
					bucket = &foulTotals{}
					bySource[p.Source] = bucket
				}
				bucket.add(p)
			}
		}
		t.Logf("%s: fouls %.2f, box fouls %.2f, penalties %.2f per match, defender separation %.3f, ball gap %.3f",
			setup.name, float64(all.fouls)/matches, float64(all.inBox)/matches,
			float64(all.penalties)/matches, all.separation/float64(all.fouls), all.ballGap/float64(all.fouls))
		for _, source := range []string{"pass_intercept", "pass_lapse", "carry_win", "carry_loss", "carry_regain", "hold_regain"} {
			b := bySource[source]
			if b == nil {
				continue
			}
			t.Logf("%s %s: fouls %.2f, box %.2f, pens %.2f/match, separation %.3f, ball gap %.3f",
				setup.name, source, float64(b.fouls)/matches, float64(b.inBox)/matches,
				float64(b.penalties)/matches, b.separation/float64(b.fouls), b.ballGap/float64(b.fouls))
		}
	}
}
