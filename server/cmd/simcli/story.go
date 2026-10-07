package main

// Story + KPI modes (T-014 watch-through protocol): the engine's possession ledger
// narrated chain-by-chain and measured against the ADR-0011 behavioural norms.
// Aggregates lied by omission; stories cannot.

import (
	"fmt"
	"sort"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// kpiReport prints the behavioural KPIs of N matches — the v0.1 progress meter.
func kpiReport(seed uint64, n int) {
	var chains, touches, passes, carries, duels, regains int
	regimeMix := map[string]int{}
	var chainLens []int
	for i := 0; i < n; i++ {
		res, _, err := engine.Simulate(enginetest.SampleInput(seed + uint64(i)))
		if err != nil {
			fatal(err)
		}
		chains += len(res.Chains)
		for _, c := range res.Chains {
			chainLens = append(chainLens, c.Touches)
			regimeMix[c.Regime]++
		}
		for _, t := range res.Touches {
			touches++
			switch t.Kind {
			case engine.TouchPass:
				passes++
			case engine.TouchCarry:
				carries++
			case engine.TouchDuel:
				duels++
			case engine.TouchRegain:
				regains++
			}
		}
	}
	f := float64(n)
	sort.Ints(chainLens)
	median := 0
	if len(chainLens) > 0 {
		median = chainLens[len(chainLens)/2]
	}
	fmt.Printf("behavioural KPIs over %d matches (seeds %d..%d)\n", n, seed, seed+uint64(n)-1)
	fmt.Printf("  chains/match      : %.0f   (norm 100–150)\n", float64(chains)/f)
	fmt.Printf("  touches/chain med : %d      (norm 4–8)\n", median)
	fmt.Printf("  touches/match     : %.0f  (pass %d, carry %d, duel %d, regain %d)\n",
		float64(touches)/f, int(float64(passes)/f), int(float64(carries)/f), int(float64(duels)/f), int(float64(regains)/f))
	fmt.Printf("  regime mix        :")
	for _, r := range []string{"buildUp", "progression", "finalThird", "transition", "setPiece", "regroup"} {
		fmt.Printf(" %s %d", r, regimeMix[r])
	}
	fmt.Println()
}

// storyMatch prints one match as a possession narrative — THE watch-through lens.
func storyMatch(seed uint64) {
	in := enginetest.SampleInput(seed)
	res, _, err := engine.Simulate(in)
	if err != nil {
		fatal(err)
	}

	short := [2]string{"RVA", "STB"}
	names := map[engine.PlayerID]string{}
	for _, p := range append(append([]engine.PlayerSnapshot{}, in.Home.Players...), in.Away.Players...) {
		names[p.ID] = p.Name
	}
	slotName := [22]string{}
	for i, p := range in.Home.Players {
		if i < 11 {
			slotName[i] = p.Name
		}
	}
	for i, p := range in.Away.Players {
		if i < 11 {
			slotName[11+i] = p.Name
		}
	}
	_ = names

	frameAt := func(sec int32) (float64, float64) {
		for _, kf := range res.Keyframes {
			if int32(kf.TMs/1000) >= sec {
				return kf.BallX, kf.BallY
			}
		}
		return 0.5, 0.5
	}

	fmt.Printf("MATCH STORY — RVA vs STB (seed %d)\n", seed)
	fmt.Printf("final %d-%d | %d possession chains | %d touches\n\n", res.Score.Home, res.Score.Away, len(res.Chains), len(res.Touches))

	for _, c := range res.Chains {
		fromX, fromY := frameAt(c.Start)
		toX, toY := frameAt(c.End)
		fx, tx := orient(fromX, c.Team), orient(toX, c.Team)
		line := fmt.Sprintf("%2d:%02d-%2d:%02d [%s/%s] %2dt %s %.2f %.2f %s %.2f %.2f  %s",
			c.Start/60, c.Start%60, c.End/60, c.End%60,
			short[c.Team], c.Regime, c.Touches,
			"@", fx, fromY, "→", tx, toY, c.Outcome)

		// Narrate this chain's events (half-open window [start,end) — boundaries never
		// belong to two chains).
		for _, ev := range res.Events {
			if ev.Tick < c.Start || ev.Tick >= c.End {
				continue
			}
			if ev.Kind == engine.EventKickOff {
				continue
			}
			who := names[ev.Player]
			if who == "" {
				who = string(ev.Player)
			}
			line += "  « " + ev.Kind + ":" + who + " »"
		}

		// Smells scoped to this chain's frames (the nonsense budget).
		smells := map[string]bool{}
		for _, kf := range res.Keyframes {
			sec := int32(kf.TMs / 1000)
			if sec < c.Start || sec > c.End {
				continue
			}
			for slot := 0; slot < 22; slot++ {
				ax := kf.Players[slot].X
				if slot >= 11 {
					ax = 1 - ax
				}
				switch slot % 11 {
				case 0:
					if ax > 0.35 {
						smells["GK-ADVENTURE"] = true
					}
				case 1, 2, 3:
					if ax > 0.78 && kf.BallOwner != int8(slot/11) {
						smells["DEF-ROAM"] = true
					}
				}
			}
		}
		for s := range smells {
			line += "  ⚠ " + s
		}
		_ = slotName
		fmt.Println(line)
	}
}

func orient(x float64, team int8) float64 {
	if team == 1 {
		return 1 - x
	}
	return x
}
