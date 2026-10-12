package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"
)

// T-040 sensitivity and workload study harness (isolated prototype evidence).
//
// The declared numerical reference is the 1 ms LIVE SWEPT simulation
// (Config{SubstepMS:1, DecelMPS2:RollDecelMPS2, LivePass:true}). It is the
// finest LIVE integration measured here. It is NOT analytic truth and NOT a
// bound on true error: the reference has its own finite-step bias, so a
// candidate/reference discrepancy can cancel or add to the true discretisation
// error. All numbers below are a candidate-vs-reference DISCREPANCY over the
// stated fixture and time scope.
//
// Scope qualifiers used throughout:
//   - "event-free prefix" = integer clocks from the candidate before the FIRST
//     state-changing event of either run (free travel only, no held/deflected
//     remainder);
//   - "full run" = every aligned integer clock up to the common horizon,
//     INCLUDING held movement and post-deflection travel;
//   - event-local errors = per aligned ordered-event position/time differences.
//
// Nothing here changes the default clock, the accepted architecture or any
// existing fixture; it only measures. No pass%/goals/tactical-win fitting.

// liveRefConfig is the declared 1 ms LIVE swept numerical reference.
func liveRefConfig() Config {
	return Config{SubstepMS: ReferenceSubstepMS, DecelMPS2: RollDecelMPS2, LivePass: true}
}

// liveFrame is one authoritative end-of-substep state (ball + every runner,
// positions AND velocities, plus ball mode and owner) needed for aligned
// trajectory error and mode/owner mismatch measurement.
type liveFrame struct {
	clock        int
	ballX, ballY float64
	mode         BallMode
	owner        int
	xs, ys       []float64
	vxs, vys     []float64
}

// liveTrajectory is a captured run: frames keyed by end-step clock plus the full
// event log and terminal state. Captures run the FULL horizon regardless of
// Held/Out so held movement and dead-ball remainders can be compared.
type liveTrajectory struct {
	dt            int
	horizon       int
	frames        []liveFrame
	byClock       map[int]int
	events        []PassEvent
	contactMS     int
	contactRunner int
	ballMode      BallMode
}

// captureLiveTraj runs a scene under the live swept path to the horizon.
func captureLiveTraj(t *testing.T, cfg Config, seed uint64, setup func(*Sim), horizonMS int) *liveTrajectory {
	return captureLiveTrajWith(t, cfg, seed, setup, horizonMS, nil)
}

// captureLiveTrajWith is captureLiveTraj with an optional pre-step hook (used by
// the offline intention-cadence study; nil for the normal scenes). The capture
// continues THROUGH held/out states to the horizon so the full run is comparable.
func captureLiveTrajWith(t *testing.T, cfg Config, seed uint64, setup func(*Sim), horizonMS int, preStep func(*Sim)) *liveTrajectory {
	t.Helper()
	sim, err := NewSim(cfg, seed)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	setup(sim)
	tr := &liveTrajectory{dt: cfg.SubstepMS, horizon: horizonMS, byClock: make(map[int]int)}
	record := func() {
		b := sim.Ball()
		rs := sim.Runners()
		owner := -1
		if b.Mode == BallHeld {
			owner = sim.contactRunner
		}
		f := liveFrame{
			clock: sim.ClockMS(),
			ballX: b.X, ballY: b.Y,
			mode:  b.Mode,
			owner: owner,
			xs:    make([]float64, len(rs)), ys: make([]float64, len(rs)),
			vxs: make([]float64, len(rs)), vys: make([]float64, len(rs)),
		}
		for i := range rs {
			f.xs[i], f.ys[i] = rs[i].X, rs[i].Y
			f.vxs[i], f.vys[i] = rs[i].VXM, rs[i].VYM
		}
		tr.byClock[f.clock] = len(tr.frames)
		tr.frames = append(tr.frames, f)
	}
	record()
	for sim.ClockMS() < horizonMS {
		if preStep != nil {
			preStep(sim)
		}
		sim.StepQuiet()
		record()
	}
	tr.events = sim.Events()
	tr.contactMS, tr.contactRunner = sim.FirstContact()
	tr.ballMode = sim.Ball().Mode
	return tr
}

// firstLiveEvent returns the earliest state-changing free-ball event
// (control/deflect/out/stop) by exact time, or ok=false when none occurred.
func firstLiveEvent(tr *liveTrajectory) (PassEvent, bool) {
	best := -1
	for i, e := range tr.events {
		switch e.Kind {
		case PassEventControl, PassEventDeflect, PassEventOut, PassEventStop:
			if best < 0 || eventExactTimeMS(e, tr.dt) < eventExactTimeMS(tr.events[best], tr.dt) {
				best = i
			}
		}
	}
	if best < 0 {
		return PassEvent{}, false
	}
	return tr.events[best], true
}

// liveEventFields returns (exactMS, kind, runner) for an optional first event.
func liveEventFields(e PassEvent, ok bool, dt int) (float64, string, int) {
	if !ok {
		return math.NaN(), "none", -1
	}
	return eventExactTimeMS(e, dt), e.Kind.String(), e.Runner
}

// alignedPrefixError compares a candidate against the reference over the common
// EVENT-FREE prefix (free travel only; ends at the first state-changing event of
// either run). samples=0 means no aligned clocks existed (logged as N/A by the
// caller, never as a zero error).
func alignedPrefixError(ref, cand *liveTrajectory) (ballMax, ballMean, playerMax, playerMean float64, samples, playerSamples int) {
	end := math.Inf(1)
	if e, ok := firstLiveEvent(ref); ok {
		end = math.Min(end, eventExactTimeMS(e, ref.dt))
	}
	if e, ok := firstLiveEvent(cand); ok {
		end = math.Min(end, eventExactTimeMS(e, cand.dt))
	}
	limit := math.MaxInt
	if !math.IsInf(end, 1) {
		limit = int(math.Floor(end))
	}
	var ballSum, playerSum float64
	for _, cf := range cand.frames {
		if cf.clock == 0 || cf.clock > limit {
			continue
		}
		rf, ok := frameAtClock(ref, cf.clock)
		if !ok {
			continue
		}
		d := DistM(cf.ballX, cf.ballY, rf.ballX, rf.ballY)
		if d > ballMax {
			ballMax = d
		}
		ballSum += d
		samples++
		for i := range cf.xs {
			pd := DistM(cf.xs[i], cf.ys[i], rf.xs[i], rf.ys[i])
			if pd > playerMax {
				playerMax = pd
			}
			playerSum += pd
			playerSamples++
		}
	}
	if samples > 0 {
		ballMean = ballSum / float64(samples)
	}
	if playerSamples > 0 {
		playerMean = playerSum / float64(playerSamples)
	}
	return
}

// liveRunDiff aggregates a FULL-RUN aligned comparison (through held movement and
// deflections). Mode and owner mismatches are reported separately from position
// error, so a post-divergence state difference is classified rather than averaged.
type liveRunDiff struct {
	aligned        int
	modeMismatch   int
	ownerMismatch  int
	ballMax        float64
	ballMean       float64
	playerMax      float64
	playerMean     float64
	firstModeDiff  int // clock of the first mode mismatch, -1 if none
	firstOwnerDiff int // clock of the first owner mismatch, -1 if none
}

func frameAtClock(tr *liveTrajectory, clock int) (liveFrame, bool) {
	i, ok := tr.byClock[clock]
	if !ok {
		return liveFrame{}, false
	}
	return tr.frames[i], true
}

// alignedFullRun compares every aligned clock up to the common horizon, whether
// the ball is rolling, held, loose or out.
func alignedFullRun(ref, cand *liveTrajectory) liveRunDiff {
	out := liveRunDiff{firstModeDiff: -1, firstOwnerDiff: -1}
	var ballSum, playerSum float64
	var playerN int
	for _, cf := range cand.frames {
		if cf.clock == 0 {
			continue
		}
		rf, ok := frameAtClock(ref, cf.clock)
		if !ok {
			continue
		}
		out.aligned++
		d := DistM(cf.ballX, cf.ballY, rf.ballX, rf.ballY)
		if d > out.ballMax {
			out.ballMax = d
		}
		ballSum += d
		for i := range cf.xs {
			pd := DistM(cf.xs[i], cf.ys[i], rf.xs[i], rf.ys[i])
			if pd > out.playerMax {
				out.playerMax = pd
			}
			playerSum += pd
			playerN++
		}
		if cf.mode != rf.mode {
			out.modeMismatch++
			if out.firstModeDiff < 0 {
				out.firstModeDiff = cf.clock
			}
		}
		if cf.owner != rf.owner {
			out.ownerMismatch++
			if out.firstOwnerDiff < 0 {
				out.firstOwnerDiff = cf.clock
			}
		}
	}
	if out.aligned > 0 {
		out.ballMean = ballSum / float64(out.aligned)
	}
	if playerN > 0 {
		out.playerMean = playerSum / float64(playerN)
	}
	return out
}

// eventRow is one ordered state-changing event of a run (for event-local
// alignment and classification).
type eventRow struct {
	kind    string
	runner  int
	exactMS float64
	ballX   float64
	ballY   float64
}

func contactEventRows(tr *liveTrajectory) []eventRow {
	var rows []eventRow
	for _, e := range tr.events {
		switch e.Kind {
		case PassEventControl, PassEventDeflect, PassEventOut, PassEventStop:
			rows = append(rows, eventRow{e.Kind.String(), e.Runner, eventExactTimeMS(e, tr.dt), e.BallX, e.BallY})
		}
	}
	return rows
}

// eventSeqDiff classifies an index-aligned comparison of ordered event
// sequences: `consistent` share kind+runner, `divergent` differ; the time/ball
// errors are reported only over consistent rows.
type eventSeqDiff struct {
	refCount, candCount   int
	consistent, divergent int
	maxTimeErrMS          float64
	maxBallErrM           float64
}

func compareEventSequences(ref, cand *liveTrajectory) eventSeqDiff {
	rr, cr := contactEventRows(ref), contactEventRows(cand)
	d := eventSeqDiff{refCount: len(rr), candCount: len(cr)}
	n := len(rr)
	if len(cr) < n {
		n = len(cr)
	}
	for i := 0; i < n; i++ {
		if rr[i].kind == cr[i].kind && rr[i].runner == cr[i].runner {
			d.consistent++
			if te := math.Abs(rr[i].exactMS - cr[i].exactMS); te > d.maxTimeErrMS {
				d.maxTimeErrMS = te
			}
			if be := DistM(rr[i].ballX, rr[i].ballY, cr[i].ballX, cr[i].ballY); be > d.maxBallErrM {
				d.maxBallErrM = be
			}
		} else {
			d.divergent++
		}
	}
	d.divergent += absInt(len(rr) - len(cr))
	return d
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// assertSameTrajectory proves same-seed reproducibility: two captures of the
// same scene produce bit-identical frames (positions, velocities, mode, owner)
// and events.
func assertSameTrajectory(t *testing.T, a, b *liveTrajectory) {
	t.Helper()
	if len(a.frames) != len(b.frames) {
		t.Fatalf("frame count %d != %d", len(a.frames), len(b.frames))
	}
	for i := range a.frames {
		af, bf := a.frames[i], b.frames[i]
		if af.clock != bf.clock || af.ballX != bf.ballX || af.ballY != bf.ballY || af.mode != bf.mode || af.owner != bf.owner {
			t.Fatalf("frame %d differs: %+v vs %+v", i, af, bf)
		}
		for j := range af.xs {
			if af.xs[j] != bf.xs[j] || af.ys[j] != bf.ys[j] || af.vxs[j] != bf.vxs[j] || af.vys[j] != bf.vys[j] {
				t.Fatalf("frame %d runner %d differs", i, j)
			}
		}
	}
	if len(a.events) != len(b.events) {
		t.Fatalf("event count %d != %d", len(a.events), len(b.events))
	}
	for i := range a.events {
		if a.events[i] != b.events[i] {
			t.Fatalf("event %d differs: %+v vs %+v", i, a.events[i], b.events[i])
		}
	}
}

// Prespecified fixture envelopes, fixed BEFORE the measurements below were
// taken. Deliberately roomy: they assert only that the coarse clock stays in the
// same causal regime as the reference, not a universal error bound, and are not
// tightened to fit the observed values.
const (
	t040PrefixBallMax20M      = 0.05 // m, event-free prefix, 20 ms vs 1 ms
	t040PrefixPlayerMax20M    = 0.30 // m, event-free prefix, 20 ms vs 1 ms
	t040FirstEventTimeMax20MS = 20   // ms = one 20 ms substep
	t040GrazingMissFrac       = 0.01 // ≤ 1% of reference-caught straight glances may be missed
)

// t040Scene is one fixture for the paired sweep. mustAgree marks scenes whose
// geometry is unambiguous enough that a coarser clock must still report the same
// first-event class (contact vs out) as the reference.
type t040Scene struct {
	name      string
	horizon   int
	mustAgree bool
	setup     func(*Sim)
}

func t040Scenes() []t040Scene {
	p := DefaultMotionParams()
	return []t040Scene{
		{"moving-inertial-contact", 4000, true, func(sim *Sim) {
			sim.SetRunners([]Runner{InertialRunner(0.62, 0.50, 0.42, 0.50, p)})
			sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.58, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
		}},
		{"grazing-glance", 1500, false, func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: ShotMaxMPS})
			sim.SetRunners([]Runner{{X: 0.55, Y: 0.50 + 0.99/PitchWidthM, ReachM: ControlRadiusM}})
		}},
		{"multiple-contacts", 4000, false, func(sim *Sim) {
			sim.SetRunners([]Runner{
				PursuingRunner(0.55, 0.50-0.8/PitchWidthM, p, 0),
				PursuingRunner(0.72, 0.50+0.8/PitchWidthM, p, 0),
			})
			sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
		}},
		{"stationary-recovery", 4000, true, func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallLoose, X: 0.60, Y: 0.50})
			sim.SetRunners([]Runner{PursuingRunner(0.60-5.0/PitchLengthM, 0.50, p, 0)})
		}},
		{"boundary-contact-before-exit", 1200, true, func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.994, Y: 0.50, VXM: ShotMaxMPS})
			sim.SetRunners([]Runner{{X: 1.004, Y: 0.50, ReachM: ControlRadiusM}})
		}},
		{"boundary-exit", 1200, true, func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.994, Y: 0.50, VXM: ShotMaxMPS})
			sim.SetRunners([]Runner{{X: 1.020, Y: 0.50, ReachM: ControlRadiusM}})
		}},
		{"boundary-tangent-travel", 2000, true, func(sim *Sim) {
			// The ball travels ALONG the y=1 boundary line; only a strictly
			// outside endpoint is out (explicit tangency rule).
			sim.SetBall(Ball{Mode: BallRolling, X: 0.45, Y: 1.0, VXM: 8.0})
			sim.SetRunners([]Runner{{X: 0.55, Y: 1.0, ReachM: ControlRadiusM}})
		}},
		{"boundary-resting-on-line", 400, true, func(sim *Sim) {
			// A ball at rest exactly on the boundary line is in play.
			sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 1.0})
			sim.SetRunners([]Runner{{X: 0.50, Y: 1.0, ReachM: ControlRadiusM}})
		}},
	}
}

// TestT040LiveSweptTrajectoryVsReference is scope item 1: the paired
// 20/10/5 ms live swept comparison against the declared 1 ms live reference.
// For each scene it reports the event-free-prefix diagnostic (N/A when no
// aligned clocks), the FULL-RUN aligned position error with mode/owner mismatch
// classification, the first-event class/runner/time, and the ordered
// event-sequence classification. Each candidate is captured twice to prove
// reproducibility; envelopes are prespecified (see const block).
func TestT040LiveSweptTrajectoryVsReference(t *testing.T) {
	refCfg := liveRefConfig()
	for _, sc := range t040Scenes() {
		ref := captureLiveTraj(t, refCfg, 1, sc.setup, sc.horizon)
		ref2 := captureLiveTraj(t, refCfg, 1, sc.setup, sc.horizon)
		assertSameTrajectory(t, ref, ref2)
		re, rok := firstLiveEvent(ref)
		refTime, refKind, refRunner := liveEventFields(re, rok, ref.dt)
		refRows := contactEventRows(ref)
		t.Logf("scene=%s | reference 1ms: first=%s@%.2fms runner=%d events=%d terminal=%s",
			sc.name, refKind, refTime, refRunner, len(refRows), ref.ballMode)

		for _, dt := range []int{20, 10, 5} {
			cfg := Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}
			cand := captureLiveTraj(t, cfg, 1, sc.setup, sc.horizon)
			candRepro := captureLiveTraj(t, cfg, 1, sc.setup, sc.horizon)
			assertSameTrajectory(t, cand, candRepro)

			bm, bmean, pm, pmean, n, pn := alignedPrefixError(ref, cand)
			full := alignedFullRun(ref, cand)
			seq := compareEventSequences(ref, cand)
			ce, cok := firstLiveEvent(cand)
			candTime, candKind, candRunner := liveEventFields(ce, cok, cand.dt)
			timeErr := math.NaN()
			if rok && cok {
				timeErr = math.Abs(candTime - refTime)
			}

			if n == 0 {
				t.Logf("  dt=%2dms | PREFIX: N/A (0 aligned event-free clocks) | first=%s@%.2fms r%d timeErr=%.2fms",
					dt, candKind, candTime, candRunner, timeErr)
			} else {
				t.Logf("  dt=%2dms | PREFIX: ball max=%.4fm mean=%.4fm | player max=%.4fm mean=%.4fm (n=%d/%d) | first=%s@%.2fms r%d timeErr=%.2fms",
					dt, bm, bmean, pm, pmean, n, pn, candKind, candTime, candRunner, timeErr)
			}
			t.Logf("         FULLRUN: aligned=%d ball max=%.4fm mean=%.4fm player max=%.4fm mean=%.4fm | modeMismatch=%d(first@%d) ownerMismatch=%d(first@%d) | terminal=%s",
				full.aligned, full.ballMax, full.ballMean, full.playerMax, full.playerMean,
				full.modeMismatch, full.firstModeDiff, full.ownerMismatch, full.firstOwnerDiff, cand.ballMode)
			t.Logf("         EVENTS: ref=%d cand=%d consistent=%d divergent=%d | consistent maxTimeErr=%.2fms maxBallErr=%.4fm",
				seq.refCount, seq.candCount, seq.consistent, seq.divergent, seq.maxTimeErrMS, seq.maxBallErrM)

			// Prespecified: on the multiple-contact fixture the ordered contact
			// sequence (kind+runner per index) must be preserved — a discrete
			// causal expectation, not an upper error bound.
			if sc.name == "multiple-contacts" && (seq.refCount != seq.candCount || seq.divergent != 0) {
				t.Fatalf("scene %s dt=%d: contact order diverges (ref=%d cand=%d divergent=%d)", sc.name, dt, seq.refCount, seq.candCount, seq.divergent)
			}
			// Coverage: a nonzero event-free prefix must exist whenever the first
			// event is later than one substep.
			if n == 0 {
				if e, ok := firstLiveEvent(cand); ok && eventExactTimeMS(e, cand.dt) > float64(dt) {
					t.Fatalf("scene %s dt=%d: zero event-free prefix despite first event at %.2fms", sc.name, dt, eventExactTimeMS(e, cand.dt))
				}
			}

			// Prespecified envelopes (documented before measurement).
			if n > 0 {
				if dt == 20 && bm > t040PrefixBallMax20M {
					t.Fatalf("scene %s dt=20 prefix ball max %.4f m exceeds prespecified %.4f m", sc.name, bm, t040PrefixBallMax20M)
				}
				if dt == 20 && pm > t040PrefixPlayerMax20M {
					t.Fatalf("scene %s dt=20 prefix player max %.4f m exceeds prespecified %.4f m", sc.name, pm, t040PrefixPlayerMax20M)
				}
			}
			if rok && cok && dt == 20 && timeErr > float64(t040FirstEventTimeMax20MS) {
				t.Fatalf("scene %s dt=20 first-event time error %.2f ms exceeds prespecified %d ms", sc.name, timeErr, t040FirstEventTimeMax20MS)
			}
			if sc.mustAgree && candKind != refKind {
				t.Fatalf("scene %s dt=%d: first-event class %q != reference %q", sc.name, dt, candKind, refKind)
			}
			if sc.mustAgree && refKind == "control" && cok && candRunner != refRunner {
				t.Fatalf("scene %s dt=%d: contact runner %d != reference %d", sc.name, dt, candRunner, refRunner)
			}
			// Full-run coverage: every candidate frame after t0 must align.
			if want := len(cand.frames) - 1; full.aligned != want {
				t.Fatalf("scene %s dt=%d: full-run aligned %d, want %d", sc.name, dt, full.aligned, want)
			}
			// Unambiguous scenes converge to the same terminal mode/owner.
			if sc.mustAgree {
				cLast, rLast := cand.frames[len(cand.frames)-1], ref.frames[len(ref.frames)-1]
				if cLast.mode != rLast.mode {
					t.Fatalf("scene %s dt=%d: terminal mode %s != reference %s", sc.name, dt, cLast.mode, rLast.mode)
				}
				if cLast.mode == BallHeld && cLast.owner != rLast.owner {
					t.Fatalf("scene %s dt=%d: terminal owner %d != reference %d", sc.name, dt, cLast.owner, rLast.owner)
				}
			}
		}
	}
}

// TestT040GrazingLiveSweptSweep scans near-tangent straight glance offsets and
// compares the LIVE swept 20/10/5 ms clocks against the LIVE 1 ms reference.
// The swept chord is exact for a straight ball segment, so misses here are
// expected to be rare; this does NOT certify curved/finite-integration grazing
// (an inertial chord is a straight approximation of a curved path, and any
// finite step can still step over a tangent). Prespecified: ≤ 1% + 1 misses.
func TestT040GrazingLiveSweptSweep(t *testing.T) {
	caught := map[int]int{1: 0, 20: 0, 10: 0, 5: 0}
	miss := map[int]int{20: 0, 10: 0, 5: 0}
	const offsets = 201
	for i := 0; i < offsets; i++ {
		offsetM := 0.9980 + float64(i)*0.0000045 // ~0.9980 .. 0.9989 m
		setup := func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: ShotMaxMPS})
			sim.SetRunners([]Runner{{X: 0.55, Y: 0.50 + offsetM/PitchWidthM, ReachM: ControlRadiusM}})
		}
		refMs, _ := func() (int, bool) {
			tr := captureLiveTraj(t, liveRefConfig(), 1, setup, 1200)
			return tr.contactMS, tr.contactMS >= 0
		}()
		if refMs < 0 {
			continue
		}
		caught[1]++
		for _, dt := range []int{20, 10, 5} {
			cfg := Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}
			tr := captureLiveTraj(t, cfg, 1, setup, 1200)
			if tr.contactMS >= 0 {
				caught[dt]++
			} else {
				miss[dt]++
			}
		}
	}
	limit := int(math.Ceil(float64(caught[1])*t040GrazingMissFrac)) + 1
	for _, dt := range []int{20, 10, 5} {
		t.Logf("grazing straight sweep (%d offsets): 1ms caught=%d | %dms caught=%d missed=%d (limit %d)",
			offsets, caught[1], dt, caught[dt], miss[dt], limit)
		if miss[dt] > limit {
			t.Fatalf("grazing %dms missed %d/%d reference straight glances, over prespecified limit %d", dt, miss[dt], caught[1], limit)
		}
	}
	if caught[1] == 0 {
		t.Fatal("reference caught no straight grazing glance")
	}
}

// TestT040ContactOrderDifferences reports the ordered contact sequence of the
// multiple-contact scene at each clock, so contact-order divergence (an
// outcome-relevant difference, not just a position lag) is explicit.
func TestT040ContactOrderDifferences(t *testing.T) {
	var scene t040Scene
	for _, sc := range t040Scenes() {
		if sc.name == "multiple-contacts" {
			scene = sc
		}
	}
	sequence := func(dt int) []string {
		cfg := Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}
		tr := captureLiveTraj(t, cfg, 1, scene.setup, scene.horizon)
		var out []string
		for _, r := range contactEventRows(tr) {
			out = append(out, r.kind+"@"+strconv.FormatFloat(r.exactMS, 'f', 2, 64)+"r"+strconv.Itoa(r.runner))
		}
		return out
	}
	for _, dt := range []int{1, 20, 10, 5} {
		t.Logf("multiple-contacts dt=%2dms: %v", dt, sequence(dt))
	}
}

// --- scope item 2 (sweeps) helpers ---------------------------------------

// liveFirstContactMS returns the exact time of the first control/deflect event.
func liveFirstContactMS(t *testing.T, cfg Config, seed uint64, setup func(*Sim), horizon int) (int, bool) {
	t.Helper()
	tr := captureLiveTraj(t, cfg, seed, setup, horizon)
	best := math.Inf(1)
	found := false
	for _, e := range tr.events {
		if e.Kind == PassEventControl || e.Kind == PassEventDeflect {
			if et := eventExactTimeMS(e, tr.dt); et < best {
				best, found = et, true
			}
		}
	}
	if !found {
		return 0, false
	}
	return int(math.Round(best)), true
}

// TestT040PrespecifiedReachSweep varies the collection reach against a fixed
// 0.90 m off-lane approach. Prespecified range 0.50..2.00 m; below the 0.90 m
// minimum approach no contact is possible (discrete threshold), above it a
// larger reach contacts no later. Response only; nothing fitted.
func TestT040PrespecifiedReachSweep(t *testing.T) {
	reaches := []float64{0.5, 0.75, 0.9, 1.0, 1.25, 1.5, 2.0}
	prev := math.Inf(1)
	contacted := 0
	for _, reach := range reaches {
		r := reach
		setup := func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.20, Y: 0.50, VXM: 16.0})
			sim.SetRunners([]Runner{{X: 0.55, Y: 0.50 + 0.9/PitchWidthM, ReachM: r}})
		}
		ms, found := liveFirstContactMS(t, LivePassConfig(), 1, setup, 3000)
		if !found {
			t.Logf("reach=%.2f m -> no contact (threshold: 0.90 m minimum approach)", reach)
			continue
		}
		contacted++
		if float64(ms) > prev {
			t.Fatalf("reach sweep not monotone: reach=%.2f contact=%dms > previous %.0fms", reach, ms, prev)
		}
		prev = float64(ms)
		t.Logf("reach=%.2f m -> first contact %d ms", reach, ms)
	}
	if contacted < 4 {
		t.Fatalf("reach sweep produced only %d contacts; expected >0.90 m to contact", contacted)
	}
}

// TestT040PrespecifiedDecelerationSweep varies grass rolling friction for a
// 6 m/s ground ball. Prespecified range 0.20..1.20 m/s2; abrupt deceleration
// never rolls the ball further. Horizon sized so even 0.20 m/s2 stops in play.
func TestT040PrespecifiedDecelerationSweep(t *testing.T) {
	decels := []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2}
	setup := func(sim *Sim) { sim.SetBall(Ball{Mode: BallRolling, X: 0.05, Y: 0.50, VXM: 6.0}) }
	prev := math.Inf(1)
	for _, d := range decels {
		cfg := Config{SubstepMS: SubstepMS, DecelMPS2: d, LivePass: true}
		tr := captureLiveTraj(t, cfg, 1, setup, 32000)
		last := tr.frames[len(tr.frames)-1]
		dist := (last.ballX - 0.05) * PitchLengthM
		stopMS := -1
		for _, e := range tr.events {
			if e.Kind == PassEventStop {
				stopMS = int(math.Round(eventExactTimeMS(e, tr.dt)))
			}
		}
		if tr.ballMode == BallOut {
			t.Logf("decel=%.2f m/s2 -> left the pitch before stopping (numerical limit)", d)
			continue
		}
		if dist > prev+0.5 {
			t.Fatalf("decel sweep not monotone: decel=%.2f roll=%.2fm > previous %.2fm", d, dist, prev)
		}
		prev = dist
		want := 36.0 / (2 * d)
		t.Logf("decel=%.2f m/s2 -> roll=%.2fm (ideal v^2/2a=%.2fm) stop=%dms", d, dist, want, stopMS)
	}
}

// TestT040PrespecifiedAccelerationAndTurningSweep varies linear acceleration
// (time to 3.0 m/s from rest) and lateral/turning acceleration (time until a
// 90-degree intent produces 1.0 m/s of cross velocity). Prespecified accel range
// 1.0..4.0 m/s2, lateral range 0.5..3.0 m/s2; more response reaches a given
// speed earlier.
func TestT040PrespecifiedAccelerationAndTurningSweep(t *testing.T) {
	accels := []float64{1.0, 1.5, 2.0, 3.0, 4.0}
	prev := math.Inf(1)
	for _, a := range accels {
		p := MotionParams{TopSpeedMPS: PlayerTopSpeedMPS, AccelMPS2: a, DecelMPS2: PlayerDecelMPS2, LateralAccelMPS2: PlayerLateralAccelMaxMPS2}
		setup := func(sim *Sim) { sim.SetRunners([]Runner{InertialRunner(0.20, 0.50, 0.95, 0.50, p)}) }
		tr := captureLiveTraj(t, Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2, LivePass: true}, 1, setup, 4000)
		reach := -1
		for _, f := range tr.frames {
			if math.Sqrt(f.vxs[0]*f.vxs[0]+f.vys[0]*f.vys[0]) >= 3.0 {
				reach = f.clock
				break
			}
		}
		if reach < 0 {
			t.Fatalf("accel=%.1f never reached 3.0 m/s", a)
		}
		if float64(reach) > prev {
			t.Fatalf("accel sweep not monotone: accel=%.1f time=%dms > previous %.0fms", a, reach, prev)
		}
		prev = float64(reach)
		t.Logf("accel=%.1f m/s2 -> 3.0 m/s at %d ms", a, reach)
	}

	lats := []float64{0.5, 1.0, 1.5, 2.0, 3.0}
	prev = math.Inf(1)
	for _, l := range lats {
		p := MotionParams{TopSpeedMPS: PlayerTopSpeedMPS, AccelMPS2: PlayerAccelMaxMPS2, DecelMPS2: PlayerDecelMPS2, LateralAccelMPS2: l}
		setup := func(sim *Sim) {
			r := InertialRunner(0.50, 0.50, 0.50, 0.90, p)
			r.VXM = PlayerTopSpeedMPS // entering at top speed, intent is 90 degrees across
			sim.SetRunners([]Runner{r})
		}
		tr := captureLiveTraj(t, Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2, LivePass: true}, 1, setup, 4000)
		cross := -1
		for _, f := range tr.frames {
			if f.vys[0] >= 1.0 {
				cross = f.clock
				break
			}
		}
		if cross < 0 {
			t.Fatalf("lateral=%.1f never produced 1 m/s of cross velocity", l)
		}
		if float64(cross) > prev {
			t.Fatalf("turning sweep not monotone: lateral=%.1f time=%dms > previous %.0fms", l, cross, prev)
		}
		prev = float64(cross)
		t.Logf("lateral=%.1f m/s2 -> 1 m/s cross velocity at %d ms", l, cross)
	}
}

// TestT040PrespecifiedReactionSweep varies the reaction delay with the ball
// already in reach. Prespecified range 0..1200 ms; touch never precedes the
// reaction. A reaction beyond the horizon yields no touch (asserted absent).
func TestT040PrespecifiedReactionSweep(t *testing.T) {
	reacts := []int{0, 100, 200, 400, 800, 1200}
	prev := -1
	for _, react := range reacts {
		r := react
		setup := func(sim *Sim) {
			sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM, ReactMS: r}})
			sim.ReleasePass(PassParams{StartX: 0.55, StartY: 0.50, TargetX: 0.57, TargetY: 0.50, SpeedMPS: 0.5, Passer: -1})
		}
		ms, found := liveFirstContactMS(t, LivePassConfig(), 1, setup, 2000)
		if !found {
			t.Logf("react=%d ms -> no touch within 2000 ms (numerical limit)", react)
			continue
		}
		if ms < react {
			t.Fatalf("react=%d ms: touch at %d ms precedes the reaction", react, ms)
		}
		if ms < prev {
			t.Fatalf("reaction sweep not monotone: react=%d touch=%dms < previous %dms", react, ms, prev)
		}
		prev = ms
		t.Logf("react=%d ms -> first touch %d ms", react, ms)
	}

	// Beyond the horizon: no touch must be recorded.
	beyond := func(sim *Sim) {
		sim.SetRunners([]Runner{{X: 0.55, Y: 0.50, ReachM: ControlRadiusM, ReactMS: 5000}})
		sim.ReleasePass(PassParams{StartX: 0.55, StartY: 0.50, TargetX: 0.57, TargetY: 0.50, SpeedMPS: 0.5, Passer: -1})
	}
	if ms, found := liveFirstContactMS(t, LivePassConfig(), 1, beyond, 2000); found {
		t.Fatalf("react=5000 ms produced a touch at %d ms within a 2000 ms horizon", ms)
	}
	t.Log("react=5000 ms horizon 2000 ms -> no touch (asserted absent, as expected)")
}

// --- scope item 4: offline cadence / event-scoping equivalence study -------

// t040StudyPursuers builds study bodies placed ahead of a +x rolling ball so a
// real contact happens inside the study horizon. pursue=true is the automatic
// per-substep reference; pursue=false lets the test republish intents manually.
func t040StudyPursuers(pursue bool) []Runner {
	p := DefaultMotionParams()
	blob := []float64{
		0.55, 0.50,
		0.50, 0.56,
		0.50, 0.44,
		0.45, 0.50,
	}
	rs := make([]Runner, 0, len(blob)/2)
	for i := 0; i < len(blob); i += 2 {
		r := InertialRunner(blob[i], blob[i+1], blob[i], blob[i+1], p)
		r.Pursue = pursue
		rs = append(rs, r)
	}
	return rs
}

// TestT040IntentionCadenceStudy measures the offline error of republishing
// pursuit intents only every N substeps (N=1,2,5,10,25 -> 20..500 ms) relative
// to the accepted per-substep reference on a MOVING scene that reaches a real
// contact. It reports the contact winner, ordered event classification,
// event-local time/ball errors and post-contact full-run position/owner
// mismatches, alongside the event-free-prefix position error. N=1 must reproduce
// the reference EXACTLY on the FULL frames and events (not just the prefix). No
// default is switched; there is no persisted cadence model (see
// TestT040CadenceNoPersistedModel).
func TestT040IntentionCadenceStudy(t *testing.T) {
	const horizon = 2500
	refSetup := func(sim *Sim) {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
		sim.SetRunners(t040StudyPursuers(true))
	}
	ref := captureLiveTraj(t, LivePassConfig(), 1, refSetup, horizon)
	refFirst, refOK := firstLiveEvent(ref)
	_, _, refRunner := liveEventFields(refFirst, refOK, ref.dt)
	refWinner := "none"
	if refOK && (refFirst.Kind == PassEventControl || refFirst.Kind == PassEventDeflect) {
		refWinner = refFirst.Kind.String() + "r" + strconv.Itoa(refRunner)
	}
	t.Logf("cadence reference: winner=%s events=%d", refWinner, len(contactEventRows(ref)))

	prevPlayerErr := -1.0
	for _, n := range []int{1, 2, 5, 10, 25} {
		cadence := n
		setup := func(sim *Sim) {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
			sim.SetRunners(t040StudyPursuers(false))
		}
		step := 0
		preStep := func(sim *Sim) {
			if step%cadence == 0 {
				b := sim.Ball()
				if b.Mode == BallRolling || b.Mode == BallLoose {
					for i := range sim.Runners() {
						sim.SetIntent(i, b.X, b.Y)
					}
				}
			}
			step++
		}
		cand := captureLiveTrajWith(t, LivePassConfig(), 1, setup, horizon, preStep)
		bm, _, pm, _, _, _ := alignedPrefixError(ref, cand)
		full := alignedFullRun(ref, cand)
		seq := compareEventSequences(ref, cand)
		ce, cok := firstLiveEvent(cand)
		_, _, candRunner := liveEventFields(ce, cok, cand.dt)
		winner := "none"
		if cok && (ce.Kind == PassEventControl || ce.Kind == PassEventDeflect) {
			winner = ce.Kind.String() + "r" + strconv.Itoa(candRunner)
		}
		t.Logf("cadence N=%d (%dms): prefix ball=%.5fm player=%.5fm | winner=%s (ref %s) | events ref=%d cand=%d consistent=%d divergent=%d maxTimeErr=%.2fms maxBallErr=%.4fm | fullrun player max=%.4fm ownerMismatch=%d modeMismatch=%d",
			n, n*SubstepMS, bm, pm, winner, refWinner, seq.refCount, seq.candCount, seq.consistent, seq.divergent,
			seq.maxTimeErrMS, seq.maxBallErrM, full.playerMax, full.ownerMismatch, full.modeMismatch)

		if n == 1 {
			assertSameTrajectory(t, ref, cand) // full frames + events equality
			if seq.divergent != 0 {
				t.Fatalf("N=1 cadence diverges from the reference sequence (%d divergent)", seq.divergent)
			}
		} else {
			if pm < prevPlayerErr {
				t.Fatalf("cadence error not growing: N=%d player err=%.5f < previous %.5f", n, pm, prevPlayerErr)
			}
			prevPlayerErr = pm
		}
	}
}

// TestT040CadenceNoPersistedModel documents that the cadence studied above is an
// external republish with NO persisted scheduling state: the Snapshot carries no
// cadence field, so a cadence cannot be resumed and is NOT adopted. Any real
// cadence model needs a new persisted field and its own Resume fixture.
func TestT040CadenceNoPersistedModel(t *testing.T) {
	sim := mustSim(t, LivePassConfig(), 1)
	sim.SetRunners(t040StudyPursuers(false))
	sim.SetBall(Ball{Mode: BallRolling, X: 0.30, Y: 0.50, VXM: 12.0})
	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, bad := range []string{"cadence", "intentCadence", "decisionCadence", "nextDecisionMs"} {
		if _, ok := m[bad]; ok {
			t.Fatalf("unexpected persisted cadence field %q", bad)
		}
	}
	t.Log("no persisted cadence field: the studied cadence is a study-only external republish; adoption requires a new persisted scheduling field and its own Resume fixture")
}

// TestT040EventScopeSkipsMovement rejects event-scoped stepping that skips
// movement. Part 1/2 use a DEAD ball with genuinely accelerating bodies (a dead
// ball still requires full player integration); part 3 uses a HELD ball whose
// owner follows, proving held owner-ball motion cannot be frozen either.
func TestT040EventScopeSkipsMovement(t *testing.T) {
	// Part 1: DEAD ball, accelerating bodies, coarse vs 5 ms.
	setup := func(sim *Sim) { sim.SetRunners(inertial22()) }
	ref := captureLiveTraj(t, Config{SubstepMS: 5, DecelMPS2: RollDecelMPS2, LivePass: true}, 1, setup, 800)
	prev := -1.0
	for _, dt := range []int{20, 50, 100} {
		cand := captureLiveTraj(t, Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}, 1, setup, 800)
		_, _, pm, _, _, _ := alignedPrefixError(ref, cand)
		if pm == 0 {
			// No free-ball event with a dead ball: fall back to the full run.
			pm = alignedFullRun(ref, cand).playerMax
		}
		t.Logf("DEAD ball, coarse integration dt=%3dms -> player max err vs 5ms = %.4fm", dt, pm)
		if pm < prev {
			t.Fatalf("coarse-scope error not growing: dt=%d err=%.4f < previous %.4f", dt, pm, prev)
		}
		prev = pm
	}

	// Part 2: freeze every second substep (DEAD ball).
	full := mustSim(t, LivePassConfig(), 1)
	full.SetRunners(inertial22())
	frozen := mustSim(t, LivePassConfig(), 1)
	frozen.SetRunners(inertial22())
	for k := 0; k < 400; k++ {
		full.StepQuiet()
		if k%2 == 0 {
			before := frozen.Runners()
			frozen.StepQuiet()
			frozen.SetRunners(before)
		} else {
			frozen.StepQuiet()
		}
	}
	fr, rf := full.Runners(), frozen.Runners()
	worst := 0.0
	for i := range fr {
		if d := DistM(fr[i].X, fr[i].Y, rf[i].X, rf[i].Y); d > worst {
			worst = d
		}
	}
	t.Logf("DEAD-ball event-scope skip (freeze every 2nd substep, 400 steps) -> worst player divergence = %.4fm", worst)
	if worst < 1.0 {
		t.Fatalf("freezing accelerating bodies produced only %.4fm divergence; expected a large measured error", worst)
	}

	// Part 3: HELD ball, owner follows; freezing the owner must diverge the ball too.
	fullHeld := mustSim(t, LivePassConfig(), 1)
	fullHeld.SetRunners(inertial22())
	t040RearmHeld(fullHeld)
	frozenHeld := mustSim(t, LivePassConfig(), 1)
	frozenHeld.SetRunners(inertial22())
	t040RearmHeld(frozenHeld)
	for k := 0; k < 400; k++ {
		fullHeld.Step()
		if k%2 == 0 {
			before := frozenHeld.Runners()
			frozenHeld.Step()
			frozenHeld.SetRunners(before)
		} else {
			frozenHeld.Step()
		}
	}
	ownerErr := DistM(fullHeld.Runners()[0].X, fullHeld.Runners()[0].Y, frozenHeld.Runners()[0].X, frozenHeld.Runners()[0].Y)
	ballErr := DistM(fullHeld.Ball().X, fullHeld.Ball().Y, frozenHeld.Ball().X, frozenHeld.Ball().Y)
	t.Logf("HELD-ball owner-follow skip (freeze every 2nd substep, 400 steps) -> owner divergence = %.4fm, ball divergence = %.4fm", ownerErr, ballErr)
	if ownerErr < 1.0 || ballErr < 1.0 {
		t.Fatalf("freezing a held owner produced owner=%.4fm ball=%.4fm; expected a large measured error", ownerErr, ballErr)
	}
}

// --- scope item 2/Resume: complete snapshot continuity --------------------

// runUntilEvent steps a sim until an event of `kind` appears or the horizon
// passes, returning the event clock (or -1).
func runUntilEvent(sim *Sim, kind PassEventKind, horizonMS int) int {
	for sim.ClockMS() < horizonMS {
		sim.Step()
		for _, e := range sim.Events() {
			if e.Kind == kind && e.ClockMS == sim.ClockMS() {
				return e.ClockMS
			}
		}
	}
	return -1
}

// assertResumeExact advances two sims to the horizon and asserts the complete
// final Snapshot (velocities, intents, reaction/cooldown deadlines, RNG, events)
// and the recorded sample tail are identical.
func assertResumeExact(t *testing.T, label string, straight, resumed *Sim, tailFromClock, horizonMS int) {
	t.Helper()
	for straight.ClockMS() < horizonMS {
		straight.Step()
	}
	for resumed.ClockMS() < horizonMS {
		resumed.Step()
	}
	var tail []Sample
	for _, s := range straight.Samples() {
		if s.ClockMS > tailFromClock {
			tail = append(tail, s)
		}
	}
	if !reflect.DeepEqual(tail, resumed.Samples()) {
		t.Fatalf("%s: resumed sample tail differs from the straight run", label)
	}
	if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
		t.Fatalf("%s: complete final Snapshot differs", label)
	}
	t.Logf("%s: complete Snapshot + %d tail samples identical after JSON round-trip", label, len(tail))
}

// TestT040SnapshotContinuityAcrossClocks proves, at 20/10/5 ms, that a JSON
// snapshot round-trip resumes exactly for (A) mid-flight free travel, (B) a
// post-deflection state with an ACTIVE cooldown and subsequently more contacts
// (and a consumed RNG), and (C) a held-ball owner-follow tail. In every case the
// COMPLETE final Snapshot (not just samples) must match via reflect.DeepEqual.
func TestT040SnapshotContinuityAcrossClocks(t *testing.T) {
	mk := func(dt int) *Sim {
		cfg := Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}
		sim := mustSim(t, cfg, 11)
		p := DefaultMotionParams()
		sim.SetRunners([]Runner{
			InertialRunner(0.62, 0.50, 0.42, 0.50, p),
			PursuingRunner(0.56, 0.50, p, 0),
		})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 12.0, ControlError: 1.0, Passer: -1})
		return sim
	}
	for _, dt := range []int{20, 10, 5} {
		// (A) mid-flight free travel, before any contact.
		a := mk(dt)
		for a.ClockMS() < 200 {
			a.Step()
		}
		as := resumeJSON(t, a)
		assertResumeExact(t, "A-free-flight-"+strconv.Itoa(dt), a, as, 200, 2000)

		// (B) post-deflection with an active cooldown + consumed RNG.
		b := mk(dt)
		rngAfterRelease := b.RNGState()
		if runUntilEvent(b, PassEventDeflect, 3000) < 0 {
			t.Fatalf("dt=%d: no deflection within 3000 ms", dt)
		}
		if b.RNGState() == rngAfterRelease {
			t.Fatalf("dt=%d: the deflection did not consume RNG", dt)
		}
		active := false
		for _, r := range b.Runners() {
			if r.CooldownUntilMS > b.ClockMS() {
				active = true
			}
		}
		if !active {
			t.Fatalf("dt=%d: post-deflection snapshot has no active cooldown", dt)
		}
		snapClock := b.ClockMS()
		bs := resumeJSON(t, b)
		assertResumeExact(t, "B-post-deflect-"+strconv.Itoa(dt), b, bs, snapClock, 4000)

		// (C) held-ball owner-follow tail (a clean control with a moving receiver).
		c := mustSim(t, Config{SubstepMS: dt, DecelMPS2: RollDecelMPS2, LivePass: true}, 7)
		c.SetRunners([]Runner{InertialRunner(0.62, 0.50, 0.30, 0.50, DefaultMotionParams())})
		c.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.58, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
		if runUntilEvent(c, PassEventControl, 5000) < 0 {
			t.Fatalf("dt=%d: no control within 5000 ms", dt)
		}
		for i := 0; i < 5; i++ {
			c.Step()
		}
		cs := resumeJSON(t, c)
		assertResumeExact(t, "C-held-tail-"+strconv.Itoa(dt), c, cs, c.ClockMS(), 6000)
	}
}

// resumeJSON marshals a snapshot, unmarshals it, and resumes — proving the JSON
// round-trip path, not only the in-memory snapshot.
func resumeJSON(t *testing.T, sim *Sim) *Sim {
	t.Helper()
	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resumed, err := Resume(decoded)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	return resumed
}
