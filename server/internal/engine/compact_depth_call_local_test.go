package engine

import (
	"fmt"
	"math"
	"testing"
)

// T-033 contract: compactDepth's call-local distance cache must reproduce the
// ORIGINAL engagedRank membership — rank counts the active teammates (goalkeeper
// included) STRICTLY closer to the ball, equal distances rank EQUAL (no index
// tie-break), and rank < pressing marks a chaser who sits out of the block. The
// frozen compactDepthV17 oracle keeps calling the unchanged engagedRank, so every
// case below is diffed against truly independent membership logic.
//
// Scenario geometry uses exact dyadic coordinates (multiples of 1/16 or finer)
// wherever a rank value is hand-asserted, so every Manhattan leg is exact and the
// hand-computed ranks are bit-truthworthy. Mirrored rows negate x around the ball
// (or mirror through 0.5) — both team orientations are diffed every time.

// cdDyadMirror places xs0/ys on team 0 and the x-negated copy on team 1. With the
// ball at x = 0.5 both mirrors share the exact distance multiset.
func cdDyadMirror(s *simState, xs0, ys [11]float64) {
	xs1 := xs0
	for i := range xs1 {
		xs1[i] = 1 - xs0[i]
	}
	cdPlace(s, 0, xs0, ys)
	cdPlace(s, 1, xs1, ys)
}

// cdManhattan is the engine's live distance expression, duplicated here only to
// SANITY-ASSERT the rigs (1-ULP adjacency, hand ties) — never to feed the engine.
func cdManhattan(s *simState, team, i int) float64 {
	return math.Abs(s.players[team*11+i].X-s.ballX) + math.Abs(s.players[team*11+i].Y-s.ballY)
}

// cdAssertPassthrough pins the thin-block semantics: with fewer than four block
// holders every query must return tx bit-for-bit (the early-return path).
func cdAssertPassthrough(t *testing.T, label string, s *simState) {
	t.Helper()
	for team := 0; team < 2; team++ {
		for idx := 0; idx < 11; idx++ {
			for _, tx := range cdTxSamples {
				if got := s.compactDepth(team, idx, tx); math.Float64bits(got) != math.Float64bits(tx) {
					t.Fatalf("%s: compactDepth(%d,%d,%g) = %016x (%g), want tx %016x passthrough",
						label, team, idx, tx, math.Float64bits(got), got, math.Float64bits(tx))
				}
			}
		}
	}
}

// cdAssertClamped pins the engaged-block semantics: at least one query must come
// back clamped (a ceiling exists), so "all passthrough" mistakes are distinguishable.
func cdAssertClamped(t *testing.T, label string, s *simState) {
	t.Helper()
	clamped := false
	for team := 0; team < 2 && !clamped; team++ {
		for idx := 0; idx < 11 && !clamped; idx++ {
			for _, tx := range cdTxSamples {
				if got := s.compactDepth(team, idx, tx); math.Float64bits(got) != math.Float64bits(tx) {
					clamped = true
					break
				}
			}
		}
	}
	if !clamped {
		t.Fatalf("%s: expected at least one clamped query (block engaged, ceilings written), got all passthrough", label)
	}
}

// TestCompactDepthTieAndKeeperContract covers the membership traps of the call
// local cache: exact distance ties must rank EQUAL on both sides of the pressing
// cut (an index tie-break silently splits a tie group), the goalkeeper counts as
// a closer only when strictly nearer (GK ties lose), a sent-off goalkeeper counts
// for nobody, and a sent-off twin must never raise anyone's rank.
func TestCompactDepthTieAndKeeperContract(t *testing.T) {
	// Trap 1 — "ties chase together": idx1/idx2 tie at distance 1/32 from the ball,
	// on opposite sides of it. At Pressing 1 both rank 0 and BOTH are chasers, so
	// only three holders remain and every query passes through. An index tie-break
	// would admit exactly one twin and engage a four-man block.
	t.Run("tied leaders chase together", func(t *testing.T) {
		s := &simState{}
		var xs [11]float64
		xs[0], xs[1], xs[2] = 0.03125, 0.53125, 0.46875
		xs[3], xs[4], xs[5] = 0.25, 0.75, 0.5
		// idx6..idx10 stay sent off (positions unused): five active outfielders.
		ys5 := cdRow(0.5)
		ys5[5] = 0.8125
		cdDyadMirror(s, xs, ys5)
		for _, g := range []int{6, 7, 8, 9, 10} {
			s.cards[g].Off, s.cards[g+11].Off = true, true
		}
		s.tactics[0].Pressing, s.tactics[1].Pressing = 1, 1
		s.ballX, s.ballY = 0.5, 0.5

		if r1, r2 := s.engagedRank(0, 1), s.engagedRank(0, 2); r1 != 0 || r2 != 0 {
			t.Fatalf("tied leaders must both rank 0 (no idx tie-break), got %d and %d", r1, r2)
		}
		if r3, r4 := s.engagedRank(0, 3), s.engagedRank(0, 4); r3 != 2 || r4 != 2 {
			t.Fatalf("tied second pair must both rank 2, got %d and %d", r3, r4)
		}
		if r := s.engagedRank(0, 5); r != 4 {
			t.Fatalf("lone fifth man must rank 4, got %d", r)
		}
		cdAssertPassthrough(t, "tied leaders chase together", s)
	})

	// Trap 2 — "ties hold together at the cut": two clear chasers plus a TRIAL of
	// three mixed-coordinate ties (dx, -dy, -dx splits, all exactly 1/8 from the
	// ball). Each ranks exactly 2 — on the Pressing-2 boundary — so all three hold
	// and the four-man block engages. Demoting one tied man "because later index"
	// would thin the block to three and pass everything through.
	t.Run("tied block holds together at the cut", func(t *testing.T) {
		s := &simState{}
		var xs [11]float64
		ysT := cdRow(0.5)
		xs[0], xs[1], xs[2] = 0.03125, 0.53125, 0.5625
		xs[3], xs[4], xs[5], xs[6] = 0.625, 0.5, 0.375, 0.5
		ysT[4], ysT[6] = 0.375, 0.6875
		cdDyadMirror(s, xs, ysT)
		for _, g := range []int{7, 8, 9, 10} {
			s.cards[g].Off, s.cards[g+11].Off = true, true
		}
		s.tactics[0].Pressing, s.tactics[1].Pressing = 2, 2
		s.ballX, s.ballY = 0.5, 0.5

		for _, i := range []int{3, 4, 5} {
			if r := s.engagedRank(0, i); r != 2 {
				t.Fatalf("tied trio idx %d must rank exactly 2 (on the pressing cut), got %d", i, r)
			}
		}
		cdAssertClamped(t, "tied block holds together at the cut", s)
	})

	// Trap 3 — the sent-off twin at distance 1/64 must NOT count as a closer for
	// P (3/64): P's true rank is 1 (only idx1 closer) and he chases at Pressing
	// 2, leaving three holders and passthrough. Counting the sent-off twin would
	// promote P and engage a four-man block.
	t.Run("sent-off twin is not a closer", func(t *testing.T) {
		s := &simState{}
		var xs [11]float64
		xs[0], xs[1], xs[2], xs[3] = 0.03125, 0.53125, 0.515625, 0.546875
		xs[4], xs[5], xs[6] = 0.3125, 0.5, 0.5
		ys6 := cdRow(0.5)
		ys6[5], ys6[6] = 0.6875, 0.3125
		cdDyadMirror(s, xs, ys6)
		s.cards[2].Off, s.cards[13].Off = true, true // the sent-off twin
		for _, g := range []int{7, 8, 9, 10} {
			s.cards[g].Off, s.cards[g+11].Off = true, true
		}
		s.tactics[0].Pressing, s.tactics[1].Pressing = 2, 2
		s.ballX, s.ballY = 0.5, 0.5

		if r := s.engagedRank(0, 3); r != 1 {
			t.Fatalf("P must rank 1 (sent-off twin excluded), got %d", r)
		}
		cdAssertPassthrough(t, "sent-off twin is not a closer", s)
	})

	// Keeper variants over one roster (Pressing 3, ball centered): idx3's rank is
	// exactly 2 without keeper help and exactly 3 when the keeper is strictly
	// closer — that single rank flips his membership across the cut.
	base := func(gk func(*simState)) *simState {
		s := &simState{}
		var xs [11]float64
		ysK := cdRow(0.5)
		xs[0], xs[1], xs[2], xs[3] = 0.03125, 0.53125, 0.5625, 0.59375
		xs[4], xs[5], xs[6] = 0.5, 0.5, 0.5
		ysK[4], ysK[5], ysK[6] = 0.8125, 0.1875, 0.875
		cdDyadMirror(s, xs, ysK)
		for _, g := range []int{7, 8, 9, 10} {
			s.cards[g].Off, s.cards[g+11].Off = true, true
		}
		s.tactics[0].Pressing, s.tactics[1].Pressing = 3, 3
		s.ballX, s.ballY = 0.5, 0.5
		gk(s)
		s.cards[11].Off = s.cards[0].Off // mirror the keeper state
		return s
	}

	t.Run("far keeper does not disturb the cut", func(t *testing.T) {
		s := base(func(s *simState) {
			s.players[0] = PlayerState{X: 0.03125, Y: 0.5} // dist 15/32, nowhere near
			s.players[11] = PlayerState{X: 0.96875, Y: 0.5}
		})
		if r := s.engagedRank(0, 3); r != 2 {
			t.Fatalf("idx3 must rank 2 (keeper far), got %d", r)
		}
		cdAssertPassthrough(t, "far keeper", s)
	})
	t.Run("keeper tie is not closer", func(t *testing.T) {
		s := base(func(s *simState) {
			// Keeper EXACTLY tied with idx3 at 3/32 — strict d < me must lose the tie.
			s.players[0] = PlayerState{X: 0.59375, Y: 0.5}
			s.players[11] = PlayerState{X: 0.40625, Y: 0.5}
		})
		if d0, d3 := cdManhattan(s, 0, 0), cdManhattan(s, 0, 3); math.Float64bits(d0) != math.Float64bits(d3) {
			t.Fatalf("rig broken: keeper dist %016x must tie idx3 dist %016x", math.Float64bits(d0), math.Float64bits(d3))
		}
		if r := s.engagedRank(0, 3); r != 2 {
			t.Fatalf("idx3 must still rank 2 (keeper tie loses), got %d", r)
		}
		cdAssertPassthrough(t, "keeper tie", s)
	})
	t.Run("strictly closer keeper promotes the boundary man", func(t *testing.T) {
		s := base(func(s *simState) {
			s.players[0] = PlayerState{X: 0.546875, Y: 0.5} // dist 3/64 < idx3's 3/32
			s.players[11] = PlayerState{X: 0.453125, Y: 0.5}
		})
		if r := s.engagedRank(0, 3); r != 3 {
			t.Fatalf("idx3 must rank 3 (keeper strictly closer), got %d", r)
		}
		cdAssertClamped(t, "closer keeper", s)
	})
	t.Run("sent-off keeper counts for nobody", func(t *testing.T) {
		s := base(func(s *simState) {
			s.players[0] = PlayerState{X: 0.546875, Y: 0.5} // as above…
			s.players[11] = PlayerState{X: 0.453125, Y: 0.5}
			s.cards[0].Off = true // …but sent off
		})
		if r := s.engagedRank(0, 3); r != 2 {
			t.Fatalf("idx3 must rank 2 again (sent-off keeper ignored), got %d", r)
		}
		cdAssertPassthrough(t, "keeper sent off", s)
	})
}

// TestCompactDepthULPDistanceTies drives the strict closer test at one-ULP grain:
// the leader order is (keeper, c−, c, c∥tie, c+) with c = 1/8 and c± one ULP
// apart, which leaves rank 3 EMPTY — only an exact strict comparison reproduces
// ranks 1/2/2/4. Ball sits at the origin so each distance IS its coordinate and
// the 1-ULP claims are exact rather than subtraction folklore.
func TestCompactDepthULPDistanceTies(t *testing.T) {
	c := 0.125
	cPlus := math.Nextafter(c, math.Inf(1))
	cMinus := math.Nextafter(c, math.Inf(-1))

	rig := func(t *testing.T, actives [11]bool) *simState {
		s := &simState{}
		var xs [11]float64
		xs[0], xs[1], xs[2], xs[3] = 0.015625, c, cPlus, cMinus
		xs[4], xs[5], xs[6], xs[7] = 0, 0.25, 0.375, 0.5
		xs[8], xs[9], xs[10] = 0.625, 0.75, 0.875
		ys := cdRow(0)
		ys[4], ys[5], ys[8], ys[10] = 0.125, 0.25, 0.125, 0.125 // idx4 ties c across the y axis
		for i := 0; i < 11; i++ {
			x, y := xs[i], ys[i]
			// Team 1 mirrors by negation: |−x − 0| + |y − 0| shares the multiset.
			s.players[i] = PlayerState{X: x, Y: y}
			s.players[11+i] = PlayerState{X: -x, Y: y}
			if !actives[i] {
				s.cards[i].Off, s.cards[11+i].Off = true, true
			}
		}
		s.ballX, s.ballY = 0, 0
		return s
	}

	// Sanity: the 1-ULP adjacency claims of the rig (test-side math only).
	full := [11]bool{true, true, true, true, true, true, true, true, true, true, true}
	s := rig(t, full)
	if !(cdManhattan(s, 0, 3) == cMinus && cdManhattan(s, 0, 2) == cPlus &&
		math.Nextafter(cMinus, math.Inf(1)) == c && math.Nextafter(c, math.Inf(1)) == cPlus) {
		t.Fatalf("rig broken: want c=%016x surrounded by one-ULP neighbours %016x/%016x",
			math.Float64bits(c), math.Float64bits(cMinus), math.Float64bits(cPlus))
	}
	if math.Float64bits(cdManhattan(s, 0, 4)) != math.Float64bits(cdManhattan(s, 0, 1)) {
		t.Fatal("rig broken: idx4 must tie idx1 exactly across the axes")
	}

	// Rank pin: the ULP-separated order leaves rank 3 vacant; ties rank equal.
	for team := 0; team < 2; team++ {
		for i, want := range map[int]int{1: 2, 2: 4, 3: 1, 4: 2, 5: 6, 6: 5, 7: 6} {
			if got := s.engagedRank(team, i); got != want {
				t.Fatalf("engagedRank(%d,%d) = %d, want %d (strict < at 1 ULP grain)", team, i, got, want)
			}
		}
	}
	cdAssertEquivalent(t, "ulp full roster", s)

	// Count trap U-a (men 1..6, Pressing 3): correct holders are {2,5,6} — three
	// men, passthrough. A tie-as-closer (<=) comparison pulls idx1/idx4 up to rank
	// 3 and engages a five-man block: caught as a clamped query.
	sUa := rig(t, [11]bool{true, true, true, true, true, true, true})
	sUa.tactics[0].Pressing, sUa.tactics[1].Pressing = 3, 3
	cdAssertPassthrough(t, "ulp trap U-a", sUa)

	// Count trap U-b (men 1..7, Pressing 3): correct holders are {2,5,6,7} — four
	// men, engaged. A rounded/epsilon distance compare collapses idx2's one-ULP
	// edge and demotes him to rank 2: caught as all-passthrough.
	sUb := rig(t, [11]bool{true, true, true, true, true, true, true, true})
	sUb.tactics[0].Pressing, sUb.tactics[1].Pressing = 3, 3
	cdAssertClamped(t, "ulp trap U-b", sUb)
}

// TestCompactDepthMixedCoordsAndSentOffs floods deterministic mixed-coordinate
// scatter (both sides of the ball on both axes) with pressing sweeps and sent-off
// pairs, the call-local cache's parity case: ties formed across dx/dy splits.
func TestCompactDepthMixedCoordsAndSentOffs(t *testing.T) {
	// Offsets are exact binary fractions around (0.375, 0.625), so every leg is
	// exact and several different (dx, dy) splits land on identical distances.
	ballX, ballY := 0.375, 0.625
	dx := [11]float64{0, -0.28125, 0.0625, -0.125, 0.125, -0.0625, 0.1875, -0.3125, 0.25, -0.4375, 0.5}
	dy := [11]float64{0, 0.03125, -0.09375, 0.0625, -0.0625, 0.09375, -0.21875, 0.28125, -0.375, 0.4375, -0.5}
	for _, offPair := range []struct {
		name string
		off  []int
	}{
		{"clean", nil},
		{"one per side", []int{3, 14}},
		{"wide pair", []int{2, 13}},
		{"keeper plus mid", []int{0, 11, 6, 17}},
	} {
		for p := int8(0); p <= 3; p++ {
			for bi, ball := range [][2]float64{{ballX, ballY}, {0.125, 0.5}, {0.75, 0.25}} {
				t.Run(fmt.Sprintf("%s pressing %d ball %d", offPair.name, p, bi), func(t *testing.T) {
					s := &simState{}
					var xs, ys [11]float64
					for i := 0; i < 11; i++ {
						xs[i], ys[i] = ball[0]+dx[i], ball[1]+dy[i]
					}
					// Team 1 keeps its own mixed scatter (x mirrored through 0.5,
					// y straight): mirror-UNSYMMETRIC on purpose versus an
					// off-center ball.
					var xs1 [11]float64
					for i := range xs1 {
						xs1[i] = 1 - xs[i]
					}
					cdPlace(s, 0, xs, ys)
					cdPlace(s, 1, xs1, ys)
					for _, g := range offPair.off {
						s.cards[g].Off = true
					}
					s.tactics[0].Pressing, s.tactics[1].Pressing = p, p
					s.ballX, s.ballY = ball[0], ball[1]
					cdAssertEquivalent(t, fmt.Sprintf("mixed %s p%d b%d", offPair.name, p, bi), s)
				})
			}
		}
	}
}

// cdSignature folds a handful of deep (always-clamping) queries into one word.
func cdSignature(s *simState) uint64 {
	var sig uint64
	for _, q := range []struct{ team, idx int }{{0, 3}, {0, 6}, {0, 9}, {1, 3}, {1, 6}, {1, 9}} {
		sig ^= math.Float64bits(s.compactDepth(q.team, q.idx, 2.0))
	}
	return sig
}

// TestCompactDepthTracksLiveMovesBetweenCalls proves the cache can never go
// stale the way a per-tick hoist would: positions move BETWEEN calls (including
// move()'s exact hazard — player i integrated before player i+1's query) and the
// output must track the live geometry at every single query. Signatures are also
// asserted to SHIFT as the block translates, so silent freezing cannot pass.
func TestCompactDepthTracksLiveMovesBetweenCalls(t *testing.T) {
	stamp := func(s *simState, label string) uint64 {
		t.Helper()
		cdAssertEquivalent(t, label, s)
		return cdSignature(s)
	}

	t.Run("translated block walks its ceilings", func(t *testing.T) {
		s := &simState{}
		var xs, xs1, ys [11]float64
		xs[0], xs[1] = 0.03125, 0.0625
		for i := 2; i <= 5; i++ {
			xs[i] = 0.1875 + 0.015625*float64(i-2)
		}
		for i := 6; i <= 8; i++ {
			xs[i] = 0.5
		}
		xs[9], xs[10] = 0.8, 0.8
		for i := range xs1 {
			xs1[i] = 1 - xs[i]
		}
		ys[5], ys[10] = 0.375, 0.625
		cdPlace(s, 0, xs, ys)
		cdPlace(s, 1, xs1, ys)
		s.ballX, s.ballY = 0.35, 0.4
		s.tactics[0].Pressing, s.tactics[1].Pressing = 1, 1

		prev := stamp(s, "walk step 0")
		for step := 1; step <= 6; step++ {
			// Whole-block advance (0.02 ax per step) plus one drifting full-back:
			// every step MUST move the clamped outputs with it.
			for i := 0; i < 11; i++ {
				s.players[i].X += 0.02
				s.players[11+i].X -= 0.02
			}
			s.players[4].Y += 0.0078125
			got := stamp(s, fmt.Sprintf("walk step %d", step))
			if got == prev {
				t.Fatalf("walk step %d: signature froze — cache not tracking live moves", step)
			}
			prev = got
		}
		// Drastic re-layout must land exactly on the oracle as well.
		var rx, rx1 [11]float64
		for i := range rx {
			rx[i], rx1[i] = 0.1+0.05*float64(i), 0.9-0.05*float64(i)
		}
		cdPlace(s, 0, rx, cdRow(0.5))
		cdPlace(s, 1, rx1, cdRow(0.5))
		if got := stamp(s, "walk relayout"); got == prev {
			t.Fatal("relayout: signature froze — stale cache survived a fresh geometry")
		}
	})

	t.Run("integrate-then-query interleaving", func(t *testing.T) {
		// Reproduces move()'s within-tick hazard exactly: each query happens at a
		// DIFFERENT live geometry because mover i steps before query i+1 runs.
		s := &simState{}
		var xs, xs1, ys [11]float64
		for i := 0; i < 11; i++ {
			xs[i], xs1[i] = 0.15+0.07*float64(i), 0.85-0.07*float64(i)
		}
		ys[3], ys[7], ys[10] = 0.3, 0.7, 0.55
		cdPlace(s, 0, xs, ys)
		cdPlace(s, 1, xs1, ys)
		s.ballX, s.ballY = 0.42, 0.58
		s.tactics[0].Pressing, s.tactics[1].Pressing = 2, 2

		distinct := map[uint64]bool{cdSignature(s): true}
		for step := 0; step < 24; step++ {
			for i := 1; i < 11; i++ {
				for team := 0; team < 2; team++ {
					// One query at the live state, verified bit-for-bit…
					for _, tx := range cdTxSamples {
						want := compactDepthV17(s, team, i, tx)
						got := s.compactDepth(team, i, tx)
						if math.Float64bits(got) != math.Float64bits(want) {
							t.Fatalf("interleave step %d team %d idx %d tx %g: got %016x, oracle %016x",
								step, team, i, tx, math.Float64bits(got), math.Float64bits(want))
						}
					}
					// …then THIS player integrates before the next query runs.
					p := &s.players[team*11+i]
					p.X += 0.01 * (1 + float64(i%3))
					if team == 1 {
						p.X -= 0.02 * (1 + float64(i%3))
					}
					p.Y += 0.00390625 * float64(i%2)
				}
			}
			distinct[cdSignature(s)] = true
		}
		if len(distinct) < 2 {
			t.Fatalf("interleave produced %d distinct signatures — cache looks frozen", len(distinct))
		}
	})
}
