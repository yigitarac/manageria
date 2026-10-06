// Command simcli simulates sample matches for engine inspection, determinism
// checks and calibration sweeps.
//
// Examples:
//
//	go run ./cmd/simcli -seed 42 -runs 3          # determinism check + summary
//	go run ./cmd/simcli -seed 1 -matches 200      # distribution/calibration sweep
//	go run ./cmd/simcli -seed 1 -matches 200 -tactic defensive
//	go run ./cmd/simcli -seed 42 -out match.json  # dump one match for the viewer
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

func main() {
	seed := flag.Uint64("seed", 42, "match seed (matches/rehearse modes: first seed)")
	runs := flag.Int("runs", 2, "identical re-runs to verify determinism")
	matches := flag.Int("matches", 0, "simulate N matches with consecutive seeds and print distribution stats")
	tactic := flag.String("tactic", "default", "matches mode: home tactic preset (default|defensive|attacking|high_press)")
	rehearse := flag.Int("rehearse", 0, "run N corner drills (drilled pattern vs default routine) and print outcome bands")
	out := flag.String("out", "", "write the match result JSON to this file")
	flag.Parse()

	if *rehearse > 0 {
		rehearseCorners(*seed, *rehearse)
		return
	}
	if *matches > 0 {
		distribution(*seed, *matches, *tactic)
		return
	}
	if *runs < 1 {
		fatal(fmt.Errorf("-runs must be >= 1"))
	}

	in := enginetest.SampleInput(*seed)
	var (
		firstRaw []byte
		result   engine.MatchResult
	)
	for i := 0; i < *runs; i++ {
		res, _, err := engine.Simulate(in)
		if err != nil {
			fatal(err)
		}
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fatal(err)
		}
		if firstRaw == nil {
			firstRaw, result = raw, res
			continue
		}
		if !bytes.Equal(firstRaw, raw) {
			fatal(fmt.Errorf("run %d differs from run 1 — determinism broken", i+1))
		}
	}

	digest, err := enginetest.Digest(result)
	if err != nil {
		fatal(err)
	}

	if *out != "" {
		if err := os.WriteFile(*out, firstRaw, 0o644); err != nil {
			fatal(err)
		}
	}

	fmt.Printf("seed:        %d\n", *seed)
	fmt.Printf("score:       %d-%d\n", result.Score.Home, result.Score.Away)
	fmt.Printf("xg:          %.2f-%.2f\n", result.Stats.Home.XG, result.Stats.Away.XG)
	fmt.Printf("possession:  %.1f%%-%.1f%%\n", result.Stats.Home.PossessionPct, result.Stats.Away.PossessionPct)
	fmt.Printf("shots:       %d-%d (on target %d-%d)\n",
		result.Stats.Home.Shots, result.Stats.Away.Shots,
		result.Stats.Home.OnTarget, result.Stats.Away.OnTarget)
	fmt.Printf("events:      %d (keyframes %d)\n", len(result.Events), len(result.Keyframes))
	fmt.Printf("digest:      %s\n", digest)
	fmt.Printf("determinism: OK (%d identical runs)\n", *runs)
}

// distribution sweeps N matches (presence off — pure team strength + home ground)
// and prints the calibration headline numbers. Presets tweak the home tactics for
// tactical sanity investigations.
func distribution(seed uint64, n int, preset string) {
	mutate := func(in *engine.MatchInput) {}
	switch preset {
	case "defensive":
		mutate = func(in *engine.MatchInput) {
			in.Home.Tactics.Mentality = 1
			in.Home.Tactics.DefensiveLine = 1
			in.Home.Tactics.PassingStyle = 1
		}
	case "attacking":
		mutate = func(in *engine.MatchInput) {
			in.Home.Tactics.Mentality = 5
			in.Home.Tactics.DefensiveLine = 3
			in.Home.Tactics.PassingStyle = 3
		}
	case "high_press":
		mutate = func(in *engine.MatchInput) {
			in.Home.Tactics.Pressing = 3
		}
	case "default":
	default:
		fatal(fmt.Errorf("unknown -tactic %q", preset))
	}

	var (
		homeGoals, awayGoals                     float64
		homeWins, draws, awayWins                int
		homeXG, awayXG                           float64
		homeShots, awayShots                     int
		homeDist, awayDist                       float64
		homeTO, awayTO                           float64
		homeFat, awayFat                         float64
		homePoss                                 float64
		totalCards, totalInjuries, totalSubs     int
		totalOffsides, totalCorners, totalErrors int
	)
	for i := 0; i < n; i++ {
		in := enginetest.SampleInput(seed + uint64(i))
		in.Presence = engine.PresenceFlags{} // calibration: no live-presence boost
		mutate(&in)
		res, _, err := engine.Simulate(in)
		if err != nil {
			fatal(err)
		}
		homeGoals += float64(res.Score.Home)
		awayGoals += float64(res.Score.Away)
		switch {
		case res.Score.Home > res.Score.Away:
			homeWins++
		case res.Score.Home < res.Score.Away:
			awayWins++
		default:
			draws++
		}
		homeXG += res.Stats.Home.XG
		awayXG += res.Stats.Away.XG
		homeShots += res.Stats.Home.Shots
		awayShots += res.Stats.Away.Shots
		homeDist += res.Stats.Home.AvgShotDist
		awayDist += res.Stats.Away.AvgShotDist
		homeTO += float64(res.Stats.Home.Turnovers)
		awayTO += float64(res.Stats.Away.Turnovers)
		homeFat += res.Stats.Home.AvgFatigue
		awayFat += res.Stats.Away.AvgFatigue
		homePoss += res.Stats.Home.PossessionPct
		for _, ev := range res.Events {
			switch ev.Kind {
			case engine.EventYellowCard, engine.EventRedCard:
				totalCards++
			case engine.EventInjury:
				totalInjuries++
			case engine.EventSubstitution:
				totalSubs++
			case engine.EventOffside:
				totalOffsides++
			case engine.EventError:
				totalErrors++
			}
		}
		totalCorners += res.Stats.Home.Corners + res.Stats.Away.Corners
	}

	f := float64(n)
	fmt.Printf("matches:      %d (seeds %d..%d, presence off, home tactic %q)\n", n, seed, seed+uint64(n)-1, preset)
	fmt.Printf("goals/match:  %.2f  (home %.2f / away %.2f)\n", (homeGoals+awayGoals)/f, homeGoals/f, awayGoals/f)
	fmt.Printf("outcomes:     home %.1f%% / draw %.1f%% / away %.1f%%\n",
		100*float64(homeWins)/f, 100*float64(draws)/f, 100*float64(awayWins)/f)
	fmt.Printf("xg/match:     %.2f-%.2f\n", homeXG/f, awayXG/f)
	fmt.Printf("shots/match:  %.1f-%.1f (avg dist %.2f-%.2f)\n",
		float64(homeShots)/f, float64(awayShots)/f, homeDist/f, awayDist/f)
	fmt.Printf("turnovers:    %.0f-%.0f   possession %.1f%%-%.1f%%   fatigue %.1f-%.1f\n",
		homeTO/f, awayTO/f, homePoss/f, 100-homePoss/f, homeFat/f, awayFat/f)
	fmt.Printf("per match:    %.1f corners, %.1f cards, %.1f offsides, %.1f errors, %.1f injuries, %.1f subs\n",
		float64(totalCorners)/f, float64(totalCards)/f, float64(totalOffsides)/f,
		float64(totalErrors)/f, float64(totalInjuries)/f, float64(totalSubs)/f)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "simcli:", err)
	os.Exit(1)
}

// rehearseCorners compares a drilled corner pattern against the default routine over
// N deterministic drills — the Pattern Studio rehearsal contract (Tactics-Schema).
func rehearseCorners(seed uint64, n int) {
	in := enginetest.SampleInput(seed)
	pat := enginetest.SampleCornerPattern()

	bands := func(pattern *engine.PatternSpec, label string) {
		var goals, saved, cleared int
		xg, marks, targets := 0.0, 0.0, 0.0
		for i := 0; i < n; i++ {
			out, err := engine.RehearseCorner(in, pattern, seed+uint64(i))
			if err != nil {
				fatal(err)
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
			if out.Target >= 0 {
				targets++
			}
		}
		f := float64(n)
		fmt.Printf("%-9s goal %5.1f%% | saved %5.1f%% | cleared %5.1f%% | xG/rep %.3f | marks %.2f | contact %3.0f%%\n",
			label, 100*float64(goals)/f, 100*float64(saved)/f, 100*float64(cleared)/f,
			xg/f, marks/f, 100*targets/f)
	}

	fmt.Printf("corner rehearsal: %d drills (seeds %d..%d, sample pattern %q)\n", n, seed, seed+uint64(n)-1, pat.ID)
	bands(&pat, "pattern:")
	bands(nil, "default:")
}
