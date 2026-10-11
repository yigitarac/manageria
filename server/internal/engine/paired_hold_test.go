package engine_test

import (
	"math"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
)

// Fast deterministic fixtures for the T-034 paired hold diagnostics: pure event slices
// and integer samples only. The extraction and the statistics are unit-tested WITHOUT
// running a single simulation, so the mathematics stays cheap and machine-independent.

func goalEv(minute, tick int32, club string) engine.Event {
	return engine.Event{Minute: minute, Tick: tick, Kind: engine.EventGoal, Club: club}
}

func TestFirstConcedeMinuteExtraction(t *testing.T) {
	const home, away = "FC Home", "FC Away"
	for _, tc := range []struct {
		name         string
		events       []engine.Event
		wantMinute   int
		wantConceded bool
	}{
		{
			name:         "empty event list is the clean-sheet sentinel",
			events:       nil,
			wantMinute:   holdCleanSheetSentinel,
			wantConceded: false,
		},
		{
			name: "home scoring events are ignored (no own-goal feature: a goal event credits the scoring club)",
			events: []engine.Event{
				goalEv(10, 600, home),
				goalEv(70, 4200, home),
			},
			wantMinute:   holdCleanSheetSentinel,
			wantConceded: false,
		},
		{
			name: "first opposition goal in result event order wins",
			events: []engine.Event{
				goalEv(25, 5400, away),
				goalEv(12, 720, away),
				goalEv(80, 4800, home),
			},
			wantMinute:   25,
			wantConceded: true,
		},
		{
			name:         "value is Event.Minute, not Tick (tick 5400 would read minute 91)",
			events:       []engine.Event{goalEv(25, 5400, away)},
			wantMinute:   25,
			wantConceded: true,
		},
		{
			name: "multiple subsequent concessions do not affect the first",
			events: []engine.Event{
				goalEv(20, 1200, away),
				goalEv(30, 1800, away),
				goalEv(90, 5400, away),
			},
			wantMinute:   20,
			wantConceded: true,
		},
		{
			name:         "a real 95' goal is (95, conceded) — sentinel scalar, NOT a clean sheet",
			events:       []engine.Event{goalEv(95, 5700, away)},
			wantMinute:   95,
			wantConceded: true,
		},
		{
			name:         "a real 97' goal stays 97 (late goals never clamp to the 95 sentinel)",
			events:       []engine.Event{goalEv(97, 5820, away)},
			wantMinute:   97,
			wantConceded: true,
		},
		{
			name: "non-goal opposition events are not concessions",
			events: []engine.Event{
				{Minute: 8, Kind: engine.EventShotSaved, Club: away},
				{Minute: 88, Kind: engine.EventShotOffTarget, Club: away},
			},
			wantMinute:   holdCleanSheetSentinel,
			wantConceded: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			minute, conceded := firstConcedeMinute(home, tc.events)
			if minute != tc.wantMinute || conceded != tc.wantConceded {
				t.Fatalf("firstConcedeMinute = (%d, %v), want (%d, %v)", minute, conceded, tc.wantMinute, tc.wantConceded)
			}
		})
	}
}

// holdObs is one stance's first-concession observation for one seed.
type holdObs struct {
	minute   int  // first opponent-goal minute, or holdCleanSheetSentinel
	conceded bool // false = sentinel-imputed clean sheet (distinct from a real 95' goal)
}

// pairedHoldTotals accumulates exact integer sums: n / Σx / Σx² keeps the legacy raw
// sums bit-identical (integer Event.Minute values and the 95 sentinel) and makes every
// derived quantity reproducible. Per-seed differences are aggregated DIRECTLY — the
// paired estimator uses the sample difference itself, never sqrt of summed independent
// variances.
type pairedHoldTotals struct {
	n                                    int64
	defendSum, defendSumSq               int64
	attackSum, attackSumSq               int64
	diffSum, diffSumSq                   int64
	defendCleanSheets, attackCleanSheets int64
}

// addPair tallies one same-seed pair.
func (t *pairedHoldTotals) addPair(defend, attack holdObs) {
	t.n++
	t.defendSum += int64(defend.minute)
	t.defendSumSq += int64(defend.minute * defend.minute)
	t.attackSum += int64(attack.minute)
	t.attackSumSq += int64(attack.minute * attack.minute)
	d := defend.minute - attack.minute
	t.diffSum += int64(d)
	t.diffSumSq += int64(d * d)
	if !defend.conceded {
		t.defendCleanSheets++
	}
	if !attack.conceded {
		t.attackCleanSheets++
	}
}

// merge folds b into a. Sweeps merge worker partitions in a fixed ascending order so
// runs are reproducible (the exact integer sums make the result order-independent).
func (t pairedHoldTotals) merge(b pairedHoldTotals) pairedHoldTotals {
	t.n += b.n
	t.defendSum += b.defendSum
	t.defendSumSq += b.defendSumSq
	t.attackSum += b.attackSum
	t.attackSumSq += b.attackSumSq
	t.diffSum += b.diffSum
	t.diffSumSq += b.diffSumSq
	t.defendCleanSheets += b.defendCleanSheets
	t.attackCleanSheets += b.attackCleanSheets
	return t
}

// pairedHoldStats is the honest-precision read: full-precision means, the paired
// defend-minus-attack delta (mean of the per-seed differences), the UNBIASED (n−1)
// sample variance of those differences, SE = sqrt(var/n) and an APPROXIMATE normal 95%
// interval (delta ± 1.96·SE). The interval assumes roughly normal sample-mean behaviour;
// it is an approximation, labelled as such, not a guarantee. Below two pairs there is no
// honest spread estimate: sufficient is false and the spread fields stay zero instead of
// fabricating certainty (or NaN).
type pairedHoldStats struct {
	n                                    int
	defendMean, attackMean               float64
	deltaMean                            float64
	deltaVar, deltaSE                    float64
	ci95Low, ci95High                    float64
	defendSE, attackSE                   float64
	defendCleanSheets, attackCleanSheets int
	sufficient                           bool
}

// stats derives the reportable quantities from the raw integer sums. Dividing by the
// COMPLETED pair count (t.n) keeps the divisor honest when a simulation error shortened
// a worker partition.
func (t pairedHoldTotals) stats() pairedHoldStats {
	s := pairedHoldStats{
		n:                 int(t.n),
		defendCleanSheets: int(t.defendCleanSheets),
		attackCleanSheets: int(t.attackCleanSheets),
	}
	if t.n == 0 {
		return s // insufficient: no mean, no spread, no fabricated interval
	}
	f := float64(t.n)
	s.defendMean = float64(t.defendSum) / f // legacy scalar mean: same raw sum, same division
	s.attackMean = float64(t.attackSum) / f
	s.deltaMean = float64(t.diffSum) / f // direct mean of the per-seed differences
	if t.n < 2 {
		return s // insufficient: one pair cannot estimate spread
	}
	s.sufficient = true
	s.deltaVar = unbiasedVar(t.n, t.diffSum, t.diffSumSq)
	s.deltaSE = math.Sqrt(s.deltaVar / f)
	s.defendSE = math.Sqrt(unbiasedVar(t.n, t.defendSum, t.defendSumSq) / f)
	s.attackSE = math.Sqrt(unbiasedVar(t.n, t.attackSum, t.attackSumSq) / f)
	s.ci95Low = s.deltaMean - 1.96*s.deltaSE
	s.ci95High = s.deltaMean + 1.96*s.deltaSE
	return s
}

// unbiasedVar is the (n−1) sample variance from exact integer raw sums:
// (n·Σx² − (Σx)²) / (n·(n−1)). The numerator is exact integer arithmetic before the
// single division, so streams with cancelling deviations yield EXACTLY zero variance.
func unbiasedVar(n, sum, sumSq int64) float64 {
	v := float64(n*sumSq-sum*sum) / float64(n*(n-1))
	if v < 0 {
		return 0 // unreachable with exact numerators; guard against future drift
	}
	return v
}

// TestPairedHoldTotalsCancelExactly: two LARGE per-stance spreads can cancel pairwise.
// Here every per-seed difference is the same constant, so the paired variance and SE are
// EXACTLY zero while both per-stance SEs stay large. This is why the paired estimator is
// used instead of sqrt(seDefend² + seAttack²) on same-seed sweeps.
func TestPairedHoldTotalsCancelExactly(t *testing.T) {
	var tot pairedHoldTotals
	for i := 0; i < 8; i++ {
		base := 12 + 11*i // wide shared swing: large variance in BOTH stances
		tot.addPair(
			holdObs{minute: base + 6, conceded: true},
			holdObs{minute: base, conceded: true},
		)
	}
	s := tot.stats()
	if !s.sufficient {
		t.Fatalf("n=8 pairs reported insufficient")
	}
	if s.deltaMean != 6 {
		t.Errorf("paired delta = %v, want exactly 6", s.deltaMean)
	}
	if s.deltaVar != 0 || s.deltaSE != 0 {
		t.Errorf("paired variance/SE = %v/%v, want exactly 0/0 (constant differences)", s.deltaVar, s.deltaSE)
	}
	if s.defendSE <= 0 || s.attackSE <= 0 {
		t.Errorf("per-stance SEs = %v/%v, want both > 0 (large marginal spreads)", s.defendSE, s.attackSE)
	}
}

// TestPairedHoldTotalsKnownVariance: per-seed differences {1,2,3,4,5} have mean 3 and
// UNBIASED (n−1) sample variance 10/4 = 2.5, hence SE = sqrt(2.5/5) = sqrt(0.5) and the
// approximate normal 95% interval 3 ± 1.96·sqrt(0.5).
func TestPairedHoldTotalsKnownVariance(t *testing.T) {
	var tot pairedHoldTotals
	for _, d := range []int{1, 2, 3, 4, 5} {
		tot.addPair(
			holdObs{minute: 50 + d, conceded: true},
			holdObs{minute: 50, conceded: true},
		)
	}
	s := tot.stats()
	se := math.Sqrt(0.5)
	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"defend mean", s.defendMean, 53},
		{"attack mean", s.attackMean, 50},
		{"paired delta mean", s.deltaMean, 3},
		{"unbiased difference variance", s.deltaVar, 2.5},
		{"paired SE", s.deltaSE, se},
		{"approx 95% interval low", s.ci95Low, 3 - 1.96*se},
		{"approx 95% interval high", s.ci95High, 3 + 1.96*se},
	} {
		if math.Abs(tc.got-tc.want) > 1e-12 {
			t.Errorf("%s = %.15f, want %.15f", tc.name, tc.got, tc.want)
		}
	}
}

// TestPairedHoldTotalsInsufficientPairs: below two pairs there is no honest spread
// estimate. The stat must declare insufficient, keep spread fields at zero and never
// fabricate certainty (no NaN/Inf out of a 0/0).
func TestPairedHoldTotalsInsufficientPairs(t *testing.T) {
	var none, one pairedHoldTotals
	one.addPair(holdObs{minute: 30, conceded: true}, holdObs{minute: 80, conceded: true})

	for _, tc := range []struct {
		name string
		tot  pairedHoldTotals
	}{
		{"n=0", none},
		{"n=1", one},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.tot.stats()
			if s.sufficient {
				t.Errorf("sufficient = true, want false below two pairs")
			}
			if s.deltaVar != 0 || s.deltaSE != 0 || s.defendSE != 0 || s.attackSE != 0 {
				t.Errorf("spread fields nonzero without evidence: %+v", s)
			}
			for _, fv := range []struct {
				name string
				val  float64
			}{
				{"defendMean", s.defendMean}, {"attackMean", s.attackMean}, {"deltaMean", s.deltaMean},
				{"deltaVar", s.deltaVar}, {"deltaSE", s.deltaSE},
				{"ci95Low", s.ci95Low}, {"ci95High", s.ci95High},
				{"defendSE", s.defendSE}, {"attackSE", s.attackSE},
			} {
				if math.IsNaN(fv.val) || math.IsInf(fv.val, 0) {
					t.Errorf("%s = %v — fabricated non-finite statistic", fv.name, fv.val)
				}
			}
		})
	}

	s := one.stats()
	if s.defendMean != 30 || s.attackMean != 80 || s.deltaMean != -50 {
		t.Errorf("single-pair means = %v/%v (delta %v), want 30/80 (delta −50)", s.defendMean, s.attackMean, s.deltaMean)
	}
}

// TestPairedHoldTotalsPartitionMerge: a fixed ascending merge of contiguous worker
// partitions agrees with serial accumulation (totals AND derived stats), and the
// completed-pair count equals the sample count — the divisor-validation invariant the
// sweep relies on.
func TestPairedHoldTotalsPartitionMerge(t *testing.T) {
	const n = 37 // odd: exercises the w·n/workers partition boundaries
	type pair struct{ defend, attack holdObs }
	samples := make([]pair, n)
	for i := range samples {
		samples[i] = pair{
			defend: holdObs{minute: 5 + (i*7)%89, conceded: i%3 != 0},
			attack: holdObs{minute: 20 + (i*3)%70, conceded: i%4 == 0},
		}
	}

	var serial pairedHoldTotals
	for _, s := range samples {
		serial.addPair(s.defend, s.attack)
	}

	const workers = 8
	parts := make([]pairedHoldTotals, workers)
	for w := 0; w < workers; w++ {
		for i := w * n / workers; i < (w+1)*n/workers; i++ {
			parts[w].addPair(samples[i].defend, samples[i].attack)
		}
	}
	merged := pairedHoldTotals{}
	for _, p := range parts { // fixed ascending worker merge order
		merged = merged.merge(p)
	}

	if merged != serial {
		t.Errorf("partition merge = %+v, want serial %+v", merged, serial)
	}
	if merged.n != n {
		t.Errorf("completed pairs = %d, want %d", merged.n, n)
	}
	if ms, ss := merged.stats(), serial.stats(); ms != ss {
		t.Errorf("merged stats %+v differ from serial stats %+v", ms, ss)
	}
}
