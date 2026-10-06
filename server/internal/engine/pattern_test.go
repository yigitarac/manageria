package engine_test

import (
	"strings"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

func withPattern(seed uint64) engine.MatchInput {
	in := enginetest.SampleInput(seed)
	in.Home.Tactics.Patterns = []engine.PatternSpec{enginetest.SampleCornerPattern()}
	return in
}

// TestPatternDeterministic: identical patterned inputs reproduce identical results,
// and a repeated rehearsal with the same seed is bit-identical too.
func TestPatternDeterministic(t *testing.T) {
	t.Parallel()

	in := withPattern(4)
	a, _, err := engine.Simulate(in)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	b, _, err := engine.Simulate(in)
	if err != nil {
		t.Fatalf("Simulate (repeat): %v", err)
	}
	assertSameCore(t, a, b)
	assertSameKeyframes(t, a, b, 0)

	pat := enginetest.SampleCornerPattern()
	d1, err := engine.RehearseCorner(in, &pat, 99)
	if err != nil {
		t.Fatalf("RehearseCorner: %v", err)
	}
	d2, err := engine.RehearseCorner(in, &pat, 99)
	if err != nil {
		t.Fatalf("RehearseCorner (repeat): %v", err)
	}
	assertJSONEqual(t, "drill", d1, d2)
}

// TestResumeMatchesStraightRunWithPattern: re-simulation equivalence while a pattern
// is armed/playing (pattern cursor state lives in the snapshot).
func TestResumeMatchesStraightRunWithPattern(t *testing.T) {
	t.Parallel()

	in := withPattern(17)
	straight, snaps, err := engine.Simulate(in)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	snap := snapAt(t, snaps, 900)

	resumed, _, err := engine.Resume(snap, in, nil)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	assertSameCore(t, straight, resumed)
	assertSameKeyframes(t, straight, resumed, snap.Tick)
}

// TestPatternAttribution: pattern-produced chances carry their pattern id and land
// in the pattern stats columns.
func TestPatternAttribution(t *testing.T) {
	t.Parallel()

	foundShot, foundEvent := false, false
	for seed := 1; seed <= 20 && (!foundShot || !foundEvent); seed++ {
		res, _, err := engine.Simulate(withPattern(uint64(seed)))
		if err != nil {
			t.Fatalf("Simulate: %v", err)
		}
		if res.Stats.Home.PatternShots > 0 {
			foundShot = true
		}
		for _, ev := range res.Events {
			if strings.Contains(ev.Detail, "pattern:") {
				foundEvent = true
			}
		}
	}
	if !foundShot {
		t.Fatal("no pattern shots attributed across 20 seeds")
	}
	if !foundEvent {
		t.Fatal("no pattern-tagged events across 20 seeds")
	}
}

// TestRehearsalBands: 200 pattern drills produce sane outcome bands (the rehearsal
// UX contract from Tactics-Schema) — corner conversion lives in a football-like range.
func TestRehearsalBands(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(50_000)
	pat := enginetest.SampleCornerPattern()
	var goals, saved, cleared int
	var xg, marks float64
	const n = 200
	for i := 0; i < n; i++ {
		out, err := engine.RehearseCorner(in, &pat, 50_000+uint64(i))
		if err != nil {
			t.Fatalf("RehearseCorner: %v", err)
		}
		switch out.Outcome {
		case "goal":
			goals++
		case "saved":
			saved++
		default:
			cleared++
		}
		xg += out.XG
		marks += out.MarksAvg
	}
	f := float64(n)
	rate := float64(goals) / f
	t.Logf("pattern bands: goal %.1f%% | saved %.1f%% | cleared %.1f%% | xG/rep %.3f | marks %.2f",
		100*rate, 100*float64(saved)/f, 100*float64(cleared)/f, xg/f, marks/f)

	// Corner conversion lives in a football-like band (window noise tolerated); the
	// directional claim vs the default routine is TestPatternImprovesCorners' job.
	if rate < 0.002 || rate > 0.15 {
		t.Errorf("pattern corner conversion = %.1f%%, want 0.2–15%%", 100*rate)
	}
	if marks/f < 0.5 {
		t.Errorf("average marks grade = %.2f, runners not reaching their marks", marks/f)
	}
}

// TestPatternImprovesCorners (T-009 acceptance): a drilled routine out-converts the
// default corner over 500 paired drills.
func TestPatternImprovesCorners(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(60_000)
	pat := enginetest.SampleCornerPattern()
	const n = 500

	run := func(pattern *engine.PatternSpec) (goals int, xg float64) {
		for i := 0; i < n; i++ {
			out, err := engine.RehearseCorner(in, pattern, 60_000+uint64(i))
			if err != nil {
				t.Fatalf("RehearseCorner: %v", err)
			}
			if out.Outcome == "goal" {
				goals++
			}
			xg += out.XG
		}
		return goals, xg
	}

	patGoals, patXG := run(&pat)
	defGoals, defXG := run(nil)
	t.Logf("corners over %d drills: pattern %d goals (%.1f xG) vs default %d goals (%.1f xG)",
		n, patGoals, patXG, defGoals, defXG)

	if patXG <= defXG {
		t.Errorf("pattern xG %.1f not above default %.1f", patXG, defXG)
	}
	if patGoals <= defGoals {
		t.Errorf("pattern goals %d not above default %d", patGoals, defGoals)
	}
}

// TestPatternValidation: the spike grammar rejects malformed patterns.
func TestPatternValidation(t *testing.T) {
	t.Parallel()

	bad := withPattern(2)
	bad.Home.Tactics.Patterns[0].Actors[0].Route = []engine.Waypoint{
		{X: 0.7, Y: 0.4, T: 3}, {X: 0.9, Y: 0.5, T: 2}, // decreasing time
	}
	if _, _, err := engine.Simulate(bad); err == nil {
		t.Fatal("decreasing route times accepted")
	}

	bad2 := withPattern(2)
	bad2.Home.Tactics.Patterns[0].Actors[0].Slot = 3
	bad2.Home.Tactics.Patterns[0].Actors[1].Slot = 3 // duplicate slots
	if _, _, err := engine.Simulate(bad2); err == nil {
		t.Fatal("duplicate actor slots accepted")
	}
}
