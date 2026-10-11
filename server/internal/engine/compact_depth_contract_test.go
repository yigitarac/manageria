package engine

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// T-032 contract: the allocation sweep of compactDepth (stack mate buffer, non
// reflective stable sort, fixed ceiling slots) must be BIT-PRESERVING against the
// v17 original algorithm. The oracle below is that original, retained verbatim on
// purpose (growing slice + reflective sort.SliceStable + map ceilings) so every
// future optimisation attempt re-proves itself against the frozen behaviour.

// compactDepthV17 is the faithful v17 original of compactDepth (pre T-032). Test
// only; every lookup, comparison and arithmetic step mirrors engine.go's former
// implementation exactly.
func compactDepthV17(s *simState, team, idx int, tx float64) float64 {
	pressing := int(s.tactics[team].Pressing)
	type mate struct {
		idx int
		ax  float64
	}
	var mates []mate
	for i := 1; i < 11; i++ {
		if s.cards[team*11+i].Off || s.engagedRank(team, i) < pressing {
			continue
		}
		ax := s.players[team*11+i].X
		if team == 1 {
			ax = 1 - ax
		}
		mates = append(mates, mate{i, ax})
	}
	if len(mates) < 4 {
		return tx
	}
	sort.SliceStable(mates, func(a, b int) bool {
		if mates[a].ax != mates[b].ax {
			return mates[a].ax < mates[b].ax
		}
		return mates[a].idx < mates[b].idx
	})
	anchor := (mates[1].ax + mates[2].ax) / 2

	ceil := make(map[int]float64, len(mates))
	lastCeil := 0.0
	armed := false
	for i := 0; i < len(mates); {
		j := i
		top, plane := mates[i].ax, 0.0
		for ; j < len(mates) && mates[j].ax-mates[i].ax <= lineMerge; j++ {
			top = mates[j].ax
			plane += mates[j].ax
		}
		plane /= float64(j - i)
		if plane < anchor {
			i = j
			continue
		}
		if !armed {
			lastCeil, armed = top, true
		} else {
			lastCeil += compactGap
		}
		for k := i; k < j; k++ {
			ceil[mates[k].idx] = lastCeil
		}
		i = j
	}

	lim, ok := ceil[idx]
	if !ok {
		return tx
	}
	ax := tx
	if team == 1 {
		ax = 1 - tx
	}
	if ax <= lim {
		return tx
	}
	ax = lim
	if team == 1 {
		return 1 - ax
	}
	return ax
}

// cdTxSamples spreads the queried depth across and beyond the pitch so clamping,
// mirroring and pure pass-through paths are all compared.
var cdTxSamples = []float64{-2, -0.5, 0, 0.02, 0.1, 0.25, 0.5, 0.75, 0.98, 1, 1.5, 2}

// cdAssertEquivalent diffs compactDepth against the v17 oracle bit-for-bit for
// every pitch index (0..10, including absent/sent-off/chaser queries) and both
// team mirrors.
func cdAssertEquivalent(t *testing.T, label string, s *simState) {
	t.Helper()
	for team := 0; team < 2; team++ {
		for idx := 0; idx < 11; idx++ {
			for _, tx := range cdTxSamples {
				want := compactDepthV17(s, team, idx, tx)
				got := s.compactDepth(team, idx, tx)
				if math.Float64bits(got) != math.Float64bits(want) {
					t.Fatalf("%s: compactDepth(%d,%d,%g) = %016x (%g), v17 oracle %016x (%g) — float bits differ",
						label, team, idx, tx, math.Float64bits(got), got, math.Float64bits(want), want)
				}
			}
		}
	}
}

// cdPlace puts one team's eleven on the sheet (index 0 = goalkeeper).
func cdPlace(s *simState, team int, xs, ys [11]float64) {
	for i := 0; i < 11; i++ {
		s.players[team*11+i] = PlayerState{X: xs[i], Y: ys[i]}
	}
}

// cdRow fills one coordinate column with a single value.
func cdRow(v float64) [11]float64 {
	out := [11]float64{}
	for i := range out {
		out[i] = v
	}
	return out
}

// TestCompactDepthMatchesV17Original is the bit-preservation proof across the
// contracted scenario families: pressing 1..3, sent-off mates, blocks thinner
// than four, absent ceiling indices, depth ties, and cluster/anchor edges. Every
// case reads BOTH team mirrors and compares raw float64 bit patterns.
func TestCompactDepthMatchesV17Original(t *testing.T) {
	// Stretched block (same shape as TestDefensiveBlockCompactnessBudget):
	// keeper 0.05, straggler 0.10, back four 0.20–0.23, mid three 0.50, front
	// two 0.80. The away side mirrors it around x = 0.5.
	stretched := func(s *simState) {
		var xs0, xs1 [11]float64
		xs0[0], xs1[0] = 0.05, 0.95
		xs0[1], xs1[1] = 0.10, 0.90
		for i := 2; i <= 5; i++ {
			xs0[i] = 0.20 + 0.01*float64(i-2)
			xs1[i] = 0.80 - 0.01*float64(i-2)
		}
		for i := 6; i <= 8; i++ {
			xs0[i], xs1[i] = 0.50, 0.50
		}
		xs0[9], xs0[10] = 0.80, 0.80
		xs1[9], xs1[10] = 0.20, 0.20
		ys := cdRow(0.5)
		cdPlace(s, 0, xs0, ys)
		cdPlace(s, 1, xs1, ys)
		s.ballX, s.ballY = 0.5, 0.5
	}

	base := func(name string, mutate func(*simState)) func(*testing.T) {
		return func(t *testing.T) {
			s := &simState{}
			stretched(s)
			mutate(s)
			cdAssertEquivalent(t, name, s)
		}
	}

	t.Run("stretched block both mirrors", base("stretched", func(*simState) {}))

	// Pressing 1..3 (plus the 0 all-hold base): the engagedRank < pressing
	// eligibility gate changes who lands in the mate buffer, and the chasers'
	// queries must miss the ceiling lookup exactly like the map miss did.
	for _, p := range []int8{0, 1, 2, 3} {
		for bi, ball := range [][2]float64{{0.5, 0.5}, {0.15, 0.5}, {0.8, 0.3}} {
			t.Run(fmt.Sprintf("pressing %d ball %d", p, bi), base(fmt.Sprintf("pressing %d ball %d", p, bi), func(s *simState) {
				s.tactics[0].Pressing = p
				s.tactics[1].Pressing = p
				s.ballX, s.ballY = ball[0], ball[1]
			}))
		}
	}
	// Asymmetric pressing rigs: one mirror presses, the other holds.
	t.Run("mixed pressing rigs", base("mixed pressing", func(s *simState) {
		s.tactics[0].Pressing = 3
		s.tactics[1].Pressing = 1
		s.ballX, s.ballY = 0.35, 0.35
	}))

	// Sent-off mates reshape the block; the sweep's per-index queries include the
	// sent-off men themselves (their rank collapses to 99 — same absent-ceiling
	// result through the same gate).
	t.Run("straggler sent off", base("straggler off", func(s *simState) {
		s.cards[1].Off, s.cards[12].Off = true, true
	}))
	t.Run("front pair sent off", base("front pair off", func(s *simState) {
		for _, g := range []int{9, 10, 20, 21} {
			s.cards[g].Off = true
		}
	}))
	t.Run("back four sent off", base("back four off", func(s *simState) {
		for _, g := range []int{2, 3, 4, 5, 13, 14, 15, 16} {
			s.cards[g].Off = true
		}
	}))
	t.Run("queried mid sent off with pressing rig", base("queried off", func(s *simState) {
		s.cards[6].Off, s.cards[17].Off = true, true
		s.tactics[0].Pressing = 2
		s.tactics[1].Pressing = 2
		s.ballX, s.ballY = 0.5, 0.5
	}))

	// Blocks thinner than four eligible mates take the early-return path.
	t.Run("three mates left", base("3 mates", func(s *simState) {
		for _, g := range []int{2, 3, 4, 5, 6, 7, 8} {
			s.cards[g].Off = true
			s.cards[g+11].Off = true
		}
	}))
	t.Run("two mates left", base("2 mates", func(s *simState) {
		for _, g := range []int{2, 3, 4, 5, 6, 7, 8, 9} {
			s.cards[g].Off = true
			s.cards[g+11].Off = true
		}
	}))
	t.Run("single survivor", base("1 mate", func(s *simState) {
		for g := 2; g <= 10; g++ {
			s.cards[g].Off = true
			s.cards[g+11].Off = true
		}
	}))
	t.Run("everyone gone", base("0 mates", func(s *simState) {
		for g := 1; g <= 21; g++ {
			s.cards[g].Off = true
		}
	}))

	// Depth ties: identical live depths must feed the plane sums in the identical
	// (depth, idx) order or the float additions reorder and the bits drift.
	allTied := cdRow(0.25)
	t.Run("ten mates bit-identical depths", base("all tied", func(s *simState) {
		ys := cdRow(0.5)
		cdPlace(s, 0, allTied, ys)
		cdPlace(s, 1, cdRow(0.75), ys) // mirrored ties: 1-0.75 shares ax with team 0
	}))
	t.Run("tie groups at cluster seams", base("tie groups", func(s *simState) {
		var xs0 [11]float64
		copy(xs0[:], []float64{0.05, 0.1, 0.2, 0.2, 0.2, 0.2, 0.2, 0.45, 0.45, 0.7, 0.7})
		xs1 := xs0
		for i := range xs1 {
			xs1[i] = 1 - xs0[i]
		}
		ys := cdRow(0.5)
		cdPlace(s, 0, xs0, ys)
		cdPlace(s, 1, xs1, ys)
	}))

	// Cluster/anchor edges: exact plane == anchor (whole block tied), cluster
	// seams built from the lineMerge constant itself (the subtraction lands one
	// ULP either side of the <= test — both branches get exercised across the
	// rows), one-ULP seam splits, and one-ULP plane nudges around the anchor cut.
	seams := func(s *simState) {
		var xs0 [11]float64
		atSeam := 0.20 + lineMerge // neighbour exactly lineMerge deeper than 0.20
		splitAbove := math.Nextafter(atSeam, math.Inf(1))
		xs0[0] = 0.05
		xs0[1] = 0.10
		xs0[2] = 0.20
		xs0[3] = atSeam
		xs0[4] = splitAbove // one ULP past the seam
		xs0[5] = math.Nextafter(0.20, math.Inf(-1))
		xs0[6] = xs0[4] // tie with the split member (idx tiebreak seam)
		xs0[7] = 0.55
		xs0[8] = 0.55
		xs0[9] = 0.90
		xs0[10] = math.Nextafter(0.90, math.Inf(1))
		xs1 := xs0
		for i := range xs1 {
			xs1[i] = 1 - xs0[i]
		}
		ys := cdRow(0.5)
		cdPlace(s, 0, xs0, ys)
		cdPlace(s, 1, xs1, ys)
	}
	t.Run("cluster seams at one ulp either side", base("seams", seams))

	ulpRows := [][11]float64{
		// plane == anchor exactly (deeper trio ties below the merged cluster).
		{0.05, 0.1, 0.1, 0.3, 0.3, 0.3, 0.3, 0.3, 0.3, 0.5, 0.5},
		// plane one ULP above / below anchor via a single nudged member.
		{0.05, 0.1, 0.1, 0.3, 0.3, 0.3, 0.3, 0.3, math.Nextafter(0.3, math.Inf(-1)), 0.5, 0.5},
		{0.05, 0.1, 0.1, 0.3, 0.3, 0.3, 0.3, 0.3, math.Nextafter(0.3, math.Inf(1)), 0.5, 0.5},
		// one giant cluster: the whole block merges into a single line.
		{0.05, 0.4, 0.4, 0.41, 0.41, 0.42, 0.42, 0.43, 0.43, 0.44, 0.44},
		// pure singletons: every seam exceeds lineMerge.
		{0.05, 0.05, 0.15, 0.25, 0.35, 0.45, 0.55, 0.65, 0.75, 0.85, 0.95},
	}
	for i, xs0 := range ulpRows {
		xs0 := xs0
		xs0[0] = 0.05 // keep keepers at home
		xs1 := xs0
		for j := range xs1 {
			xs1[j] = 1 - xs0[j]
		}
		t.Run(fmt.Sprintf("anchor edge row %d", i), base(fmt.Sprintf("anchor edge %d", i), func(s *simState) {
			ys := cdRow(0.5)
			cdPlace(s, 0, xs0, ys)
			cdPlace(s, 1, xs1, ys)
		}))
	}

	// Explicit index-absence contract: a chaser's query, the keeper's query and a
	// deep-laggard's query (his line sits under the anchor, so no ceiling is ever
	// written — the former map miss) must each pass tx through UNCHANGED and agree
	// bit-for-bit with the oracle.
	t.Run("index absence passes tx through", func(t *testing.T) {
		s := &simState{}
		stretched(s)
		s.tactics[0].Pressing = 3
		s.tactics[1].Pressing = 3
		s.ballX, s.ballY = 0.5, 0.5
		for _, q := range []struct {
			team, idx int
			reason    string
		}{{0, 0, "keeper"}, {1, 0, "keeper"}, {0, 1, "deep laggard line"}, {1, 1, "deep laggard line"}} {
			for _, tx := range cdTxSamples {
				want := compactDepthV17(s, q.team, q.idx, tx)
				got := s.compactDepth(q.team, q.idx, tx)
				if math.Float64bits(got) != math.Float64bits(want) || math.Float64bits(got) != math.Float64bits(tx) {
					t.Fatalf("%s idx %d tx %g: got %016x, oracle %016x, tx %016x — absence must pass tx through",
						q.reason, q.idx, tx, math.Float64bits(got), math.Float64bits(want), math.Float64bits(tx))
				}
			}
		}
		// Chasers: find real ones from the live rig instead of guessing indices.
		chaser := 0
		for i := 1; i < 11 && chaser == 0; i++ {
			if s.engagedRank(0, i) < 3 {
				chaser = i
			}
		}
		if chaser == 0 {
			t.Fatal("rig produced no chaser for the absence check")
		}
		for _, tx := range cdTxSamples {
			want := compactDepthV17(s, 0, chaser, tx)
			got := s.compactDepth(0, chaser, tx)
			if math.Float64bits(got) != math.Float64bits(want) || math.Float64bits(got) != math.Float64bits(tx) {
				t.Fatalf("chaser idx %d tx %g: got %016x, oracle %016x, tx %016x",
					chaser, tx, math.Float64bits(got), math.Float64bits(want), math.Float64bits(tx))
			}
		}
	})
}

// TestCompactDepthMatchesV17OnGeneratedGeometries floods the same bit-for-bit
// diff over deterministic generated squads. The generator draws from the engine's
// OWN seeded PRNG (never the global math/rand) so every geometry is finite,
// reproducible and independent of test order.
func TestCompactDepthMatchesV17OnGeneratedGeometries(t *testing.T) {
	const generations = 48
	for seed := 0; seed < 8; seed++ {
		r := rng.New(0xC0DECAFE + uint64(seed))
		for g := 0; g < generations; g++ {
			s := &simState{}
			grid := g%2 == 0 // every other pass quantizes depths into 0.025 buckets: forced ties and clusters
			for team := 0; team < 2; team++ {
				var xs, ys [11]float64
				for i := 0; i < 11; i++ {
					x, y := r.Float64()*0.96+0.02, r.Float64()*0.92+0.04
					if grid {
						x = clamp(0.02+0.025*float64(r.Intn(39)), 0.02, 0.98)
						y = clamp(0.04+0.04*float64(r.Intn(23)), 0.04, 0.96)
					}
					xs[i], ys[i] = x, y
					// ~15% send-offs, but every third generation plays clean so
					// both the full-block and damaged-block shapes appear.
					if g%3 != 0 && r.Intn(100) < 15 {
						s.cards[team*11+i].Off = true
					}
				}
				cdPlace(s, team, xs, ys)
				s.tactics[team].Pressing = int8(r.Intn(4)) // 0..3 — the 1..3 rigs included
			}
			s.ballX, s.ballY = r.Float64(), r.Float64()
			cdAssertEquivalent(t, fmt.Sprintf("seed %d gen %d", seed, g), s)
		}
	}
}

// TestCompactDepthAllocationFree guards T-032's whole point: the per-tick path
// must not allocate at all. Zero-alloc relies on escape analysis keeping the mate
// buffer and the sort on the stack — that IS toolchain dependent, so the guard is
// deliberately strict: a nonzero read on a newer Go means the buffer escaped
// (perf regression) and deserves investigation even though behaviour is proven
// bit-identical above.
func TestCompactDepthAllocationFree(t *testing.T) {
	s := &simState{}
	ys := cdRow(0.5)
	var xs0, xs1 [11]float64
	xs0[0], xs1[0] = 0.05, 0.95
	xs0[1], xs1[1] = 0.10, 0.90
	for i := 2; i <= 5; i++ {
		xs0[i] = 0.20 + 0.01*float64(i-2)
		xs1[i] = 0.80 - 0.01*float64(i-2)
	}
	for i := 6; i <= 8; i++ {
		xs0[i], xs1[i] = 0.50, 0.50
	}
	xs0[9], xs0[10] = 0.80, 0.80
	xs1[9], xs1[10] = 0.20, 0.20
	cdPlace(s, 0, xs0, ys)
	cdPlace(s, 1, xs1, ys)
	s.ballX, s.ballY = 0.5, 0.5 // Pressing 0: full ten-man mate buffers on both mirrors

	allocs := testing.AllocsPerRun(1000, func() {
		s.compactDepth(0, 6, 0.5)
		s.compactDepth(1, 6, 0.5)
	})
	if allocs != 0 {
		t.Fatalf("compactDepth allocated %.2f objects per mirrored call pair, want 0 (stack buffer escaped?)", allocs)
	}
}
