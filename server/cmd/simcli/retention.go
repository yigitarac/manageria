package main

// Pressured-retention lens (T-021 instrumentation). The vault's next step demands
// measuring pressured retention versus unpressed loops BEFORE touching the option
// menu: every touch now carries the local close-down read (Touch.Press), and this
// lens splits the ledger into free / bothered / closed bands. Two questions get
// numbers instead of vibes:
//
//   - retention: how does holding the ball fare as the close-down arrives
//     (pass OK, carry OK, duel won)?
//   - loops: what does the UNPRESSED carrier do — advance, or spin give-and-go
//     return passes and stall runs (the DEC-1 ping-pong's footprint)?
//
// Approximation note: "return pass" reuses the engine's recency window idea and
// flags a pass to the actor of any of the previous two touches in the same chain.

import (
	"fmt"
	"sort"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

const (
	// freeBand: below this nobody is meaningfully engaged on the carrier.
	freeBand = 0.5
	// closedBand: the emergency/shielding close-down read threshold (DEF-6).
	closedBand = 2.2
	// stallRunLen is a candidate unpressed stall run (touches, in one chain).
	stallRunLen = 4
	// stallNetProgress is the net pitch progress below which a run counts as a loop.
	stallNetProgress = 0.05
)

var bandNames = [...]string{"free ", "naggd", "closed"}

type bandStat struct {
	n, hold          int
	passes, passOK   int
	carries, carryOK int
	duels, duelWon   int
	progs, returns   int // denominator: passes with a prior touch
	fwdTouches       int
}

func bandOf(press float64) int {
	switch {
	case press < freeBand:
		return 0
	case press < closedBand:
		return 1
	default:
		return 2
	}
}

func retentionReport(seed uint64, n int, row string) {
	if n < 8 {
		n = 48
	}
	rowPreset := preset{name: ""}
	for _, p := range presetMatrix() {
		if p.name == row {
			rowPreset = p
			break
		}
	}
	if rowPreset.name == "" {
		fatal(fmt.Errorf("unknown -row %q (see -audit matrix)", row))
	}

	var stat [2][3]bandStat
	var stallRuns [2][]int
	var totPress [2]float64
	var totTouches [2]int

	for i := 0; i < n; i++ {
		in := enginetest.SampleInput(seed + uint64(i))
		in.Presence = engine.PresenceFlags{}
		rowPreset.mut(&in)
		res, _, err := engine.Simulate(in)
		if err != nil {
			fatal(err)
		}
		analyseRetention(res, &stat, &stallRuns, &totPress, &totTouches)
	}

	fmt.Printf("pressured-retention lens: row %s — %d matches (seeds %d..%d)\n", row, n, seed, seed+uint64(n)-1)
	for team := 0; team < 2; team++ {
		label := "HOME"
		if team == 1 {
			label = "AWAY "
		}
		avg := 0.0
		if totTouches[team] > 0 {
			avg = totPress[team] / float64(totTouches[team])
		}
		fmt.Printf("%s assessed %d touches (avg close-down read %.2f)\n", label, totTouches[team], avg)
		fmt.Println("  band   n   share  hold%  passOK%  carryOK%  duelWon%  giveGo%  fwd%")
		total := 0
		for b := 0; b < 3; b++ {
			total += stat[team][b].n
		}
		for b := 0; b < 3; b++ {
			st := &stat[team][b]
			if st.n == 0 {
				fmt.Printf("  %s  %5d   0.0%%\n", bandNames[b], 0)
				continue
			}
			f := float64(st.n)
			fmt.Printf("  %s %5d  %5.1f%%  %5.1f  %7s  %8s  %8s  %7s  %5.1f%%\n",
				bandNames[b], st.n, 100*f/float64(maxInt(total, 1)),
				100*float64(st.hold)/f,
				pctOrDash(st.passOK, st.passes), pctOrDash(st.carryOK, st.carries),
				pctOrDash(st.duelWon, st.duels), pctOrDash(st.returns, st.progs),
				100*float64(st.fwdTouches)/f)
		}
		runs := stallRuns[team]
		sort.Ints(runs)
		med := 0
		if len(runs) > 0 {
			med = runs[len(runs)/2]
		}
		fmt.Printf("  unpressed stall runs/match: %.2f (median run length %d touches)\n", float64(len(runs))/float64(n), med)
	}
}

func pctOrDash(num, den int) string {
	if den == 0 {
		return "   –  "
	}
	return fmt.Sprintf("%5.1f%%", 100*float64(num)/float64(den))
}

func analyseRetention(res engine.MatchResult, stat *[2][3]bandStat, stallRuns *[2][]int, totPress *[2]float64, totTouches *[2]int) {
	// Touches stay in ledger order; group per chain without map iteration order
	// ever mattering (determinism-friendly: chain list is appearance-ordered).
	chainOrder := make([]int32, 0, len(res.Chains))
	chainTouches := map[int32][]engine.Touch{}
	for _, t := range res.Touches {
		if _, seen := chainTouches[t.Chain]; !seen {
			chainOrder = append(chainOrder, t.Chain)
		}
		chainTouches[t.Chain] = append(chainTouches[t.Chain], t)
	}

	for _, cid := range chainOrder {
		var runStartX, runEndX float64
		var runLen int
		runTeam := int8(-1)
		flushRun := func() {
			if runLen >= stallRunLen && runTeam >= 0 {
				dir := 1.0
				if runTeam == 1 {
					dir = -1
				}
				if (runEndX-runStartX)*dir < stallNetProgress {
					(*stallRuns)[runTeam] = append((*stallRuns)[runTeam], runLen)
				}
			}
			runLen = 0
			runTeam = -1
		}
		for i, t := range chainTouches[cid] {
			assessed := t.Kind == engine.TouchPass || t.Kind == engine.TouchCarry || t.Kind == engine.TouchDuel
			if assessed {
				team, b := int(t.Team), bandOf(t.Press)
				st := &stat[team][b]
				st.n++
				totPress[team] += t.Press
				totTouches[team]++
				if t.Success {
					st.hold++
				}
				switch t.Kind {
				case engine.TouchPass:
					st.passes++
					if t.Success {
						st.passOK++
					}
				case engine.TouchCarry:
					st.carries++
					if t.Success {
						st.carryOK++
					}
				case engine.TouchDuel:
					st.duels++
					if t.Success {
						st.duelWon++
					}
				}
				// Give-and-go footprint: pass to a very recent toucher of this chain.
				if t.Kind == engine.TouchPass && t.Target >= 0 {
					st.progs++
					back := maxInt(i-recencyLookback, 0)
					for j := i - 1; j >= back; j-- {
						if chainTouches[cid][j].Actor == t.Target && chainTouches[cid][j].Team == t.Team {
							st.returns++
							break
						}
					}
				}
				// Forward progress vs the previous touch of this chain.
				if i > 0 && chainTouches[cid][i-1].Team == t.Team {
					dir := 1.0
					if t.Team == 1 {
						dir = -1
					}
					if (t.X-chainTouches[cid][i-1].X)*dir > 0.01 {
						st.fwdTouches++
					}
				}
			}

			// Unpressed stall runs over assessed touches.
			if t.Press < freeBand {
				if runLen == 0 {
					runStartX = t.X
					runTeam = t.Team
				}
				runLen++
				runEndX = t.X
			} else {
				flushRun()
			}
		}
		flushRun()
	}
}

// recencyLookback mirrors the engine's repetition window for the give-and-go read.
const recencyLookback = 2

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
