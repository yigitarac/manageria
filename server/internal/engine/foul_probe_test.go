package engine_test

import (
	"fmt"
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

// sepBands buckets foul separations by plausible body-contact reach (norm units):
// sure contact, shirt/challenge reach, tackle-at-a-stretch, clear non-contact.
var sepBands = [...]struct {
	label string
	max   float64
}{
	{"<0.03", 0.03},
	{"0.03-0.06", 0.06},
	{"0.06-0.12", 0.12},
	{">=0.12", 1e9},
}

type foulTotals struct {
	fouls, inBox, penalties int
	separation, ballGap     float64
	bands                   [4]int
	penBands                [4]int
}

func bandIndex(sep float64) int {
	for i, b := range sepBands {
		if sep < b.max {
			return i
		}
	}
	return len(sepBands) - 1
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
	i := bandIndex(p.Separation)
	a.bands[i]++
	if p.Penalty {
		a.penBands[i]++
	}
}

func (a *foulTotals) bandSummary() string {
	out := ""
	for i, b := range sepBands {
		if i > 0 {
			out += ", "
		}
		pct := 0.0
		if a.fouls > 0 {
			pct = 100 * float64(a.bands[i]) / float64(a.fouls)
		}
		out += fmt.Sprintf("%s %.0f%%", b.label, pct)
	}
	return out
}

func (a *foulTotals) penBandSummary() string {
	out := ""
	for i, b := range sepBands {
		if i > 0 {
			out += ", "
		}
		pct := 0.0
		if a.penalties > 0 {
			pct = 100 * float64(a.penBands[i]) / float64(a.penalties)
		}
		out += fmt.Sprintf("%s %.0f%%", b.label, pct)
	}
	return out
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestFailedPassFoulsNeedBodyContact is the v17 contact-economy regression: a
// failed-pass foul may only follow a challenge that actually reached a body.
// A clean lane cut, an untouched shank or an unchallenged miscontrol has nobody
// to foul — that phantom penalty factory is what skewed the penalty economy
// against deep defences ([[08-Open-Questions]] #10).
func TestFailedPassFoulsNeedBodyContact(t *testing.T) {
	const (
		matches  = 40
		contact  = 0.06 // tolerant bound above foulContactReach
		failSrcs = 3
	)
	sourceCount := map[string]int{}
	for i := 0; i < matches; i++ {
		_, fouls, err := engine.DiagnoseFouls(enginetest.SampleInput(70_000 + uint64(i)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range fouls {
			switch p.Source {
			case "pass_intercept", "pass_misplace", "touch_heavy":
				sourceCount[p.Source]++
				if p.Separation >= contact {
					t.Fatalf("%s foul at separation %.3f: failed-pass fouls require body contact (<%.2f)",
						p.Source, p.Separation, contact)
				}
			}
		}
	}
	if len(sourceCount) != failSrcs {
		t.Fatalf("failed-pass foul sources seen = %v, want all three flavours", sourceCount)
	}
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
		t.Logf("%s: fouls %.2f, box fouls %.2f, penalties %.2f per match, defender separation %.3f, ball gap %.3f, sep bands [%s], pen sep bands [%s]",
			setup.name, float64(all.fouls)/matches, float64(all.inBox)/matches,
			float64(all.penalties)/matches, all.separation/float64(all.fouls), all.ballGap/float64(all.fouls),
			all.bandSummary(), all.penBandSummary())
		for _, source := range []string{"pass_intercept", "pass_misplace", "touch_heavy", "carry_win", "carry_loss", "carry_regain", "hold_regain"} {
			b := bySource[source]
			if b == nil {
				continue
			}
			t.Logf("%s %s: fouls %.2f, box %.2f, pens %.2f/match, separation %.3f, ball gap %.3f, sep bands [%s], pen sep bands [%s]",
				setup.name, source, float64(b.fouls)/matches, float64(b.inBox)/matches,
				float64(b.penalties)/matches, b.separation/float64(b.fouls), b.ballGap/float64(b.fouls),
				b.bandSummary(), b.penBandSummary())
		}
	}
}
