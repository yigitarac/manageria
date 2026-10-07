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
	"strings"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

func main() {
	seed := flag.Uint64("seed", 42, "match seed (matches/rehearse modes: first seed)")
	runs := flag.Int("runs", 2, "identical re-runs to verify determinism")
	matches := flag.Int("matches", 0, "simulate N matches with consecutive seeds and print distribution stats")
	tactic := flag.String("tactic", "default", "matches mode: home tactic preset (default|defensive|attacking|high_press)")
	rehearse := flag.Int("rehearse", 0, "run N corner drills (drilled pattern vs default routine) and print outcome bands")
	audit := flag.Bool("audit", false, "sweep the home-tactic preset matrix over -matches rounds and flag balance violations")
	analyze := flag.Int("analyze", 0, "run N matches and print football-IQ smell metrics (moment-level sanity)")
	out := flag.String("out", "", "write the match result JSON to this file")
	flag.Parse()

	if *rehearse > 0 {
		rehearseCorners(*seed, *rehearse)
		return
	}
	if *audit {
		auditPresets(*seed, *matches)
		return
	}
	if *analyze > 0 {
		analyzeSmells(*seed, *analyze)
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
		raw, err := json.Marshal(res)
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

// auditPresets sweeps the home-tactic preset matrix (each vs a default opponent) and
// flags balance violations: total goals outside 1.8–3.4 or home win rate above 60%.
func auditPresets(seed uint64, n int) {
	if n < 50 {
		n = 150
	}
	type preset struct {
		name string
		mut  func(*engine.MatchInput)
	}
	set := func(ment, line, passing *int8, press, tempo, width *int8, mark, tackle *int8, counter *bool) func(*engine.MatchInput) {
		return func(in *engine.MatchInput) {
			t := &in.Home.Tactics
			if ment != nil {
				t.Mentality = *ment
			}
			if line != nil {
				t.DefensiveLine = *line
			}
			if passing != nil {
				t.PassingStyle = *passing
			}
			if press != nil {
				t.Pressing = *press
			}
			if tempo != nil {
				t.Tempo = *tempo
			}
			if width != nil {
				t.Width = *width
			}
			if mark != nil {
				t.Marking = *mark
			}
			if tackle != nil {
				t.Tackling = *tackle
			}
			if counter != nil {
				t.CounterAttack = *counter
			}
		}
	}
	i8 := func(v int8) *int8 { return &v }
	b := func(v bool) *bool { return &v }
	// setAway mirrors `set` for the visiting side (counter probes arm both benches).
	setAway := func(ment, line, passing *int8, press, tempo, width *int8, mark, tackle *int8, counter *bool) func(*engine.MatchInput) {
		return func(in *engine.MatchInput) {
			t := &in.Away.Tactics
			if ment != nil {
				t.Mentality = *ment
			}
			if line != nil {
				t.DefensiveLine = *line
			}
			if passing != nil {
				t.PassingStyle = *passing
			}
			if press != nil {
				t.Pressing = *press
			}
			if tempo != nil {
				t.Tempo = *tempo
			}
			if width != nil {
				t.Width = *width
			}
			if mark != nil {
				t.Marking = *mark
			}
			if tackle != nil {
				t.Tackling = *tackle
			}
			if counter != nil {
				t.CounterAttack = *counter
			}
		}
	}

	matrix := []preset{
		{"default", func(*engine.MatchInput) {}},
		{"defensive", set(i8(1), i8(1), i8(1), nil, nil, nil, nil, nil, nil)},
		{"attacking", set(i8(5), i8(3), i8(3), nil, nil, nil, nil, nil, nil)},
		{"high_press", set(nil, nil, nil, i8(3), nil, nil, nil, nil, nil)},
		{"low_block_counter", set(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))},
		{"gegenpress", set(i8(4), nil, nil, i8(3), i8(4), nil, nil, nil, b(true))},
		{"tiki_taka", set(i8(3), nil, i8(1), i8(2), i8(2), i8(2), nil, nil, nil)},
		{"wing_play", set(i8(4), nil, i8(2), nil, nil, i8(3), nil, nil, nil)},
		{"direct_long", set(i8(4), nil, i8(3), nil, i8(4), nil, nil, nil, nil)},
		{"deep_park", set(i8(1), i8(1), i8(1), i8(1), i8(1), nil, nil, i8(1), nil)},
		{"ultra_attack", set(i8(5), i8(3), i8(3), i8(3), i8(5), nil, nil, nil, nil)},
		{"hard_press_mark", set(nil, nil, nil, i8(3), nil, nil, i8(2), i8(2), nil)},
		{"narrow_dense", set(i8(3), nil, nil, nil, nil, i8(1), nil, nil, nil)},
		{"fast_breaks", set(i8(4), nil, i8(3), nil, i8(5), nil, nil, nil, b(true))},
		{"tall_target", set(i8(4), nil, i8(3), nil, nil, i8(3), nil, nil, nil)},
		{"high_wire", set(i8(5), i8(3), nil, nil, nil, i8(3), nil, nil, nil)},
		// Counter-matchup probes (names carry "_vs_"): rock-paper-scissors must exist —
		// the counter side plays a prepared counter-punch style and must blunt the aggressor.
		{"ultra_vs_lbc", func(in *engine.MatchInput) {
			set(i8(5), i8(3), i8(3), i8(3), i8(5), nil, nil, nil, nil)(in)
			setAway(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))(in)
		}},
		{"fast_vs_lbc", func(in *engine.MatchInput) {
			set(i8(4), nil, i8(3), nil, i8(5), nil, nil, nil, b(true))(in)
			setAway(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))(in)
		}},
		{"tall_vs_hpc", func(in *engine.MatchInput) {
			set(i8(4), nil, i8(3), nil, nil, i8(3), nil, nil, nil)(in)
			setAway(nil, nil, i8(3), i8(3), nil, nil, i8(2), i8(2), b(true))(in)
		}},
	}

	fmt.Printf("tactic preset audit: %d rounds each (seeds %d..), home preset vs default\n", n, seed)
	fmt.Printf("%-18s %6s %7s %7s %7s %9s  %s\n", "preset", "goals", "home%", "draw%", "away%", "xG", "verdict")
	violations := 0
	for pi, p := range matrix {
		var hg, ag float64
		var hw, dr, aw int
		var hxg, axg float64
		for i := 0; i < n; i++ {
			in := enginetest.SampleInput(seed + uint64(pi*1000+i))
			in.Presence = engine.PresenceFlags{}
			p.mut(&in)
			res, _, err := engine.Simulate(in)
			if err != nil {
				fatal(err)
			}
			hg += float64(res.Score.Home)
			ag += float64(res.Score.Away)
			switch {
			case res.Score.Home > res.Score.Away:
				hw++
			case res.Score.Home < res.Score.Away:
				aw++
			default:
				dr++
			}
			hxg += res.Stats.Home.XG
			axg += res.Stats.Away.XG
		}
		f := float64(n)
		goals := (hg + ag) / f
		homePct := 100 * float64(hw) / f
		drawPct := 100 * float64(dr) / f
		awayPct := 100 * float64(aw) / f

		verdict := "ok"
		if strings.Contains(p.name, "_vs_") {
			// Counter probes (all-out matchups legitimately erupt): the aggressor must
			// come down to near-even odds against a prepared counter-punch.
			if goals > 4.2 {
				verdict = "GOALS?"
				violations++
			}
			if homePct > 56 {
				verdict += "+UNCOUNTED?"
				violations++
			}
		} else {
			if goals < 1.8 || goals > 3.5 {
				verdict = "GOALS?"
				violations++
			}
			if homePct > 60 {
				verdict += "+WIN?"
				violations++
			}
		}
		fmt.Printf("%-18s %6.2f %6.1f%% %6.1f%% %6.1f%% %4.1f-%-4.1f  %s\n",
			p.name, goals, homePct, drawPct, awayPct, hxg/f, axg/f, verdict)
	}
	fmt.Printf("violations: %d (preset rows: goals 1.8–3.5, home win ≤ 60%%; counter probes: home ≤ 56%%)\n", violations)
}

// analyzeSmells is the automated watch-through (T-012): it measures the moment-level
// absurdities the owner hears on playback but no distribution chart shows.
func analyzeSmells(seed uint64, n int) {
	in := enginetest.SampleInput(seed)
	posByID := map[engine.PlayerID]engine.Pos{}
	for _, p := range append(append([]engine.PlayerSnapshot{}, in.Home.Players...), in.Away.Players...) {
		posByID[p.ID] = p.Pos
	}

	shots, defenderStrikes, goals, attackerGoals := 0, 0, 0, 0
	cbTerritory := 0.0
	for i := 0; i < n; i++ {
		res, _, err := engine.Simulate(enginetest.SampleInput(seed + uint64(i)))
		if err != nil {
			fatal(err)
		}
		for _, side := range []engine.TeamStats{res.Stats.Home, res.Stats.Away} {
			shots += side.Shots
			defenderStrikes += side.StrikesFromDefenders
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
		for _, kf := range res.Keyframes {
			cbTerritory = max(cbTerritory, kf.Players[1].X, kf.Players[2].X, 1-kf.Players[12].X, 1-kf.Players[13].X)
		}
	}
	fmt.Printf("football-IQ smells over %d matches (seeds %d..%d):\n", n, seed, seed+uint64(n)-1)
	denom := shots
	if denom < 1 {
		denom = 1
	}
	goalDenom := goals
	if goalDenom < 1 {
		goalDenom = 1
	}
	fmt.Printf("  defender strike share : %.1f%%  (want ≤ 5%%)\n", 100*float64(defenderStrikes)/float64(denom))
	fmt.Printf("  attacker goal share   : %.1f%%  (want ≥ 55%%)\n", 100*float64(attackerGoals)/float64(goalDenom))
	fmt.Printf("  CB territory reached  : %.3f (set-piece box presence legal; wilderness > 0.95)\n", cbTerritory)
}
