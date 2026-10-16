package liveplay

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// T-054 offline causal-provenance audit (isolated prototype evidence, TEST-ONLY).
//
// Contract (minimal example, pairing/order/tie/precision/schema/persistence and
// state limitations) is fixed BEFORE this code in:
//   vault/07-Session-Logs/2026-10-16-t054-offline-audit-evidence.md
//
// Report types/helpers are test-only. Inputs are ACTUAL copied data (sidecar
// records + authoritative events/samples). No runtime/API/schema change.

// T054ReportSchemaVersion is the report schema version.
const T054ReportSchemaVersion = 1

// T054RowKind labels a report row.
type T054RowKind string

const (
	// T054Publication is a sidecar pre-step decision publication.
	T054Publication T054RowKind = "publication"
	// T054Event is an authoritative physical event.
	T054Event T054RowKind = "event"
	// T054Sample is an authoritative sample.
	T054Sample T054RowKind = "sample"
)

// T054ReportInput is the actual copied data for one absolute window.
type T054ReportInput struct {
	StartMS              int
	EndMS                int
	SubstepMS            int
	RunnerCount          int
	PrefixHistoryPresent bool
	Overflowed           bool
	Events               []PassEvent
	Samples              []Sample
	Records              []DecisionRecord
}

// T054Row is one report row (publication, event or sample).
type T054Row struct {
	Kind           T054RowKind `json:"kind"`
	ClockMS        int         `json:"clockMs"`
	StepStartMS    int         `json:"stepStartMs"`
	ExactMS        float64     `json:"exactMs"`
	RoundedMS      int         `json:"roundedMs"`
	Runner         int         `json:"runner"`
	Reason         string      `json:"reason,omitempty"`
	Gate           string      `json:"gate,omitempty"`
	Policy         string      `json:"policy,omitempty"`
	Freshness      string      `json:"freshness,omitempty"`
	Applied        bool        `json:"applied,omitempty"`
	Simultaneous   bool        `json:"simultaneous,omitempty"`
	DroppedAttempt bool        `json:"droppedAttempt,omitempty"`
	ObsSeen        bool        `json:"obsSeen,omitempty"`
	ObsClockMS     int         `json:"obsClockMs,omitempty"`
	ObsX           float64     `json:"obsX,omitempty"`
	ObsY           float64     `json:"obsY,omitempty"`
	ObsMode        string      `json:"obsMode,omitempty"`
	ObsOwner       int         `json:"obsOwner,omitempty"`
	TargetX        float64     `json:"targetX,omitempty"`
	TargetY        float64     `json:"targetY,omitempty"`
	OwnX           float64     `json:"ownX,omitempty"`
	OwnY           float64     `json:"ownY,omitempty"`
	EventKind      string      `json:"eventKind,omitempty"`
	Seq            int         `json:"seq,omitempty"`
	Frac           float64     `json:"frac,omitempty"`
	BallX          float64     `json:"ballX,omitempty"`
	BallY          float64     `json:"ballY,omitempty"`
	InVX           float64     `json:"inVx,omitempty"`
	InVY           float64     `json:"inVy,omitempty"`
	OutVX          float64     `json:"outVx,omitempty"`
	OutVY          float64     `json:"outVy,omitempty"`
	RunnerX        float64     `json:"runnerX,omitempty"`
	RunnerY        float64     `json:"runnerY,omitempty"`
	SampleMode     string      `json:"sampleMode,omitempty"`
	SampleOwner    int         `json:"sampleOwner,omitempty"`
	SampleBallX    float64     `json:"sampleBallX,omitempty"`
	SampleBallY    float64     `json:"sampleBallY,omitempty"`
	AssocStepMS    int         `json:"associatedStepMs"`
	Coverage       string      `json:"coverage,omitempty"`
}

// T054Report is the complete bounded-window report.
type T054Report struct {
	SchemaVersion        int       `json:"schemaVersion"`
	SubstepMS            int       `json:"substepMs"`
	RunnerCount          int       `json:"runnerCount"`
	StartMS              int       `json:"startMs"`
	EndMS                int       `json:"endMs"`
	PrefixHistoryPresent bool      `json:"prefixHistoryPresent"`
	Overflowed           bool      `json:"overflowed"`
	RecordsInWindow      int       `json:"recordsInWindow"`
	Rows                 []T054Row `json:"rows"`
}

// t054Rank orders rows at equal exact time: publication < event < sample.
func t054Rank(k T054RowKind) int {
	switch k {
	case T054Publication:
		return 0
	case T054Event:
		return 1
	default:
		return 2
	}
}

// T054BuildReport builds the deterministic report from actual copied data. It
// never mutates or aliases the source sim.
func T054BuildReport(in T054ReportInput) T054Report {
	rep := T054Report{
		SchemaVersion: T054ReportSchemaVersion, SubstepMS: in.SubstepMS, RunnerCount: in.RunnerCount,
		StartMS: in.StartMS, EndMS: in.EndMS, PrefixHistoryPresent: in.PrefixHistoryPresent, Overflowed: in.Overflowed,
	}
	pubByStepRunner := map[[2]int]bool{}
	pubByStep := map[int]bool{}
	for _, r := range in.Records {
		if r.ClockMS < in.StartMS || r.ClockMS >= in.EndMS {
			continue
		}
		pubByStepRunner[[2]int{r.ClockMS, r.Runner}] = true
		pubByStep[r.ClockMS] = true
		rep.RecordsInWindow++
		rep.Rows = append(rep.Rows, T054Row{
			Kind: T054Publication, ClockMS: r.ClockMS, StepStartMS: r.ClockMS, ExactMS: float64(r.ClockMS), RoundedMS: r.ClockMS,
			Runner: r.Runner, Reason: string(r.Reason), Gate: r.GateOutcome, Policy: string(r.Policy), Freshness: r.Freshness,
			Applied: r.Applied, Simultaneous: r.Simultaneous,
			DroppedAttempt: !r.Applied && (r.GateOutcome == GateReactionBlocked || r.GateOutcome == GateInvalidEnabling),
			ObsSeen:        r.Observation.Seen, ObsClockMS: r.Observation.ClockMS, ObsX: r.Observation.X, ObsY: r.Observation.Y,
			ObsMode: r.Observation.Mode, ObsOwner: r.Observation.Owner, TargetX: r.TargetX, TargetY: r.TargetY, OwnX: r.OwnX, OwnY: r.OwnY,
			AssocStepMS: r.ClockMS,
		})
	}
	for _, e := range in.Events {
		stepStart := e.ClockMS - in.SubstepMS
		ex := float64(e.ClockMS)
		rounded := e.ClockMS
		switch e.Kind {
		case PassEventRelease: // between-step event
			stepStart = e.ClockMS
		case PassEventPath: // emitted at the END clock with Frac 0
			// stepStart is ClockMS-SubstepMS; exact/rounded are the END clock.
		default: // true fractional control/deflect/out/stop
			ex = float64(stepStart) + e.Frac*float64(in.SubstepMS)
			rounded = eventTimeMS(stepStart, in.SubstepMS, e.Frac)
		}
		if stepStart < in.StartMS || stepStart >= in.EndMS {
			continue
		}
		cov := "no-sidecar-record"
		if e.Runner < 0 {
			cov = "no-participant"
		} else if pubByStepRunner[[2]int{stepStart, e.Runner}] {
			cov = "decision-at-step(same-runner)"
		} else if pubByStep[stepStart] {
			cov = "decision-at-step(other-runner)"
		}
		rep.Rows = append(rep.Rows, T054Row{
			Kind: T054Event, ClockMS: e.ClockMS, StepStartMS: stepStart, ExactMS: ex, RoundedMS: rounded, Runner: e.Runner,
			EventKind: e.Kind.String(), Seq: e.Seq, Frac: e.Frac, BallX: e.BallX, BallY: e.BallY,
			InVX: e.VXM, InVY: e.VYM, OutVX: e.OutVXM, OutVY: e.OutVYM, RunnerX: e.RunnerX, RunnerY: e.RunnerY,
			AssocStepMS: stepStart, Coverage: cov,
		})
	}
	for _, s := range in.Samples {
		stepStart := s.ClockMS - in.SubstepMS
		if stepStart < in.StartMS || stepStart >= in.EndMS {
			continue
		}
		rep.Rows = append(rep.Rows, T054Row{
			Kind: T054Sample, ClockMS: s.ClockMS, StepStartMS: stepStart, ExactMS: float64(s.ClockMS), RoundedMS: s.ClockMS,
			SampleMode: s.Mode.String(), SampleOwner: s.Owner, SampleBallX: s.BallX, SampleBallY: s.BallY, AssocStepMS: stepStart,
		})
	}
	sort.SliceStable(rep.Rows, func(i, j int) bool {
		a, b := rep.Rows[i], rep.Rows[j]
		if a.ExactMS != b.ExactMS {
			return a.ExactMS < b.ExactMS
		}
		if ra, rb := t054Rank(a.Kind), t054Rank(b.Kind); ra != rb {
			return ra < rb
		}
		if a.Runner != b.Runner {
			return a.Runner < b.Runner
		}
		if a.Kind == T054Event {
			return a.Seq < b.Seq
		}
		return a.ClockMS < b.ClockMS
	})
	return rep
}

// T054RenderReport renders a deterministic, readable report string.
func T054RenderReport(r T054Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "t054 schema=%d window=[%d,%d) substep=%d runners=%d prefix=%t overflow=%t records=%d\n",
		r.SchemaVersion, r.StartMS, r.EndMS, r.SubstepMS, r.RunnerCount, r.PrefixHistoryPresent, r.Overflowed, r.RecordsInWindow)
	for _, row := range r.Rows {
		switch row.Kind {
		case T054Publication:
			fmt.Fprintf(&b, "[%010.4f publication] r%d reason=%s gate=%s applied=%t simultaneous=%t dropped=%t policy=%s freshness=%s obs=(seen=%t clock=%d x=%.6f y=%.6f mode=%s owner=%d) target=(%.6f,%.6f) own=(%.6f,%.6f)\n",
				row.ExactMS, row.Runner, row.Reason, row.Gate, row.Applied, row.Simultaneous, row.DroppedAttempt, row.Policy, row.Freshness,
				row.ObsSeen, row.ObsClockMS, row.ObsX, row.ObsY, row.ObsMode, row.ObsOwner, row.TargetX, row.TargetY, row.OwnX, row.OwnY)
		case T054Event:
			fmt.Fprintf(&b, "[%010.4f event] r%d seq=%d kind=%s frac=%.6f stepStart=%d rounded=%d coverage=%s ball=(%.6f,%.6f) in=(%.6f,%.6f) out=(%.6f,%.6f) runnerAt=(%.6f,%.6f)\n",
				row.ExactMS, row.Runner, row.Seq, row.EventKind, row.Frac, row.StepStartMS, row.RoundedMS, row.Coverage,
				row.BallX, row.BallY, row.InVX, row.InVY, row.OutVX, row.OutVY, row.RunnerX, row.RunnerY)
		default:
			fmt.Fprintf(&b, "[%010.4f sample] mode=%s owner=%d ball=(%.6f,%.6f)\n",
				row.ExactMS, row.SampleMode, row.SampleOwner, row.SampleBallX, row.SampleBallY)
		}
	}
	return b.String()
}

// T054GoldenReport renders the pinned fixture report from a scene.
func T054GoldenReport(t *testing.T) T054Report {
	t.Helper()
	scene := t054WatcherScene(t, 9, 60)
	return T054BuildReport(scene)
}

// T054ReportGolden is the pinned real report fixture (no existing golden changed).
const T054ReportGolden = `t054 schema=1 window=[0,60) substep=20 runners=3 prefix=true overflow=false records=3
[00000.0000 publication] r1 reason=periodic gate=reaction-blocked applied=false simultaneous=true dropped=true policy=none freshness=unknown obs=(seen=false clock=0 x=0.000000 y=0.000000 mode=unknown owner=-1) target=(0.485714,0.500000) own=(0.485714,0.500000)
[00000.0000 publication] r2 reason=periodic gate=reaction-blocked applied=false simultaneous=false dropped=true policy=none freshness=unknown obs=(seen=false clock=0 x=0.000000 y=0.000000 mode=unknown owner=-1) target=(0.100000,0.100000) own=(0.100000,0.100000)
[00000.0000 event] r-1 seq=0 kind=release frac=0.000000 stepStart=0 rounded=0 coverage=no-participant ball=(0.490286,0.500000) in=(2.000000,0.000000) out=(0.000000,0.000000) runnerAt=(0.000000,0.000000)
[00010.0604 event] r0 seq=1 kind=control frac=0.503018 stepStart=0 rounded=10 coverage=decision-at-step(other-runner) ball=(0.490476,0.500000) in=(1.988000,0.000000) out=(0.000000,0.000000) runnerAt=(0.500000,0.500000)
[00020.0000 publication] r1 reason=local-event gate=reaction-blocked applied=false simultaneous=false dropped=true policy=none freshness=unknown obs=(seen=false clock=0 x=0.000000 y=0.000000 mode=unknown owner=-1) target=(0.485714,0.500000) own=(0.485714,0.500000)
[00020.0000 sample] mode=held owner=0 ball=(0.490476,0.500000)
[00040.0000 sample] mode=held owner=0 ball=(0.490476,0.500000)
[00060.0000 sample] mode=held owner=0 ball=(0.490476,0.500000)
`

// t054WatcherScene builds the T-052 control-transition watcher scene and returns
// the actual copied report input for the window [0,end).
func t054WatcherScene(t *testing.T, seed uint64, end int) T054ReportInput {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	toucher := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams()) // react 0, non-local
	nearX := 0.50 - 1.5/PitchLengthM
	near := t050Local(nearX, 0.50, nearX, 0.50, 3.0, 300, true)
	far := t050Local(0.10, 0.10, 0.10, 0.10, 0.5, 300, true)
	rec := mustRecorder(t, 256)
	sim.SetProvenanceRecorder(rec)
	sim.SetRunners([]Runner{toucher, near, far})
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	for sim.ClockMS() < end {
		sim.Step()
	}
	return T054ReportInput{
		StartMS: 0, EndMS: end, SubstepMS: sim.Config().SubstepMS, RunnerCount: len(sim.runners),
		PrefixHistoryPresent: true, Overflowed: rec.Overflowed(),
		Events: sim.Events(), Samples: sim.Samples(), Records: rec.Records(),
	}
}

// --- tests -------------------------------------------------------------------

// TestT054ReportGolden pins one real report fixture (always asserts).
func TestT054ReportGolden(t *testing.T) {
	got := T054RenderReport(T054GoldenReport(t))
	t.Logf("T054 report:\n%s", got)
	if got != T054ReportGolden {
		t.Fatalf("report golden mismatch:\n--- got ---\n%s--- want ---\n%s", got, T054ReportGolden)
	}
}

// TestT054PairingCoverageAndLimits pins the explicit pairing and honest coverage
// (a non-local participant with NO sidecar record while local decisions are
// contemporaneous).
func TestT054PairingCoverageAndLimits(t *testing.T) {
	rep := T054GoldenReport(t)
	var control *T054Row
	var release *T054Row
	localEvent := false
	for i := range rep.Rows {
		r := &rep.Rows[i]
		if r.Kind == T054Event && r.EventKind == "control" {
			control = r
		}
		if r.Kind == T054Event && r.EventKind == "release" {
			release = r
		}
		if r.Kind == T054Publication && r.Reason == "local-event" && r.Gate == "reaction-blocked" {
			localEvent = true
		}
	}
	if control == nil {
		t.Fatal("no control event in the report")
	}
	if control.Coverage != "decision-at-step(other-runner)" {
		t.Fatalf("control coverage=%q, want decision-at-step(other-runner) (non-local participant)", control.Coverage)
	}
	if !(control.Frac > 0 && control.Frac < 1) || control.RoundedMS != 10 {
		t.Fatalf("control frac=%v rounded=%d", control.Frac, control.RoundedMS)
	}
	if control.ExactMS >= float64(control.StepStartMS+rep.SubstepMS) {
		t.Fatalf("exact time %.4f outside the step", control.ExactMS)
	}
	if release == nil || release.Coverage != "no-participant" {
		t.Fatalf("release coverage wrong: %+v", release)
	}
	if !localEvent {
		t.Fatal("no local-event reaction-blocked publication")
	}
	t.Logf("T054 pairing: control r0 stepStart=%d coverage=%s (no local record); release coverage=%s; watcher local-event reaction-blocked present",
		control.StepStartMS, control.Coverage, release.Coverage)
	t.Logf("T054 REPORT:\n%s", T054RenderReport(rep))
}

// TestT054DeflectMissingCoverage pins a no-sidecar-record participant (sensing-only).
func TestT054DeflectMissingCoverage(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 2)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)}) // sensing-only: no decision layer
	rec := mustRecorder(t, 64)
	sim.SetProvenanceRecorder(rec)
	sim.ReleasePass(t052InteriorPass(2.0, 1.0))
	for sim.ClockMS() < 60 {
		sim.Step()
	}
	rep := T054BuildReport(T054ReportInput{StartMS: 0, EndMS: 60, SubstepMS: sim.Config().SubstepMS, RunnerCount: 1, PrefixHistoryPresent: true, Overflowed: rec.Overflowed(), Events: sim.Events(), Samples: sim.Samples(), Records: rec.Records()})
	var defl *T054Row
	for i := range rep.Rows {
		if rep.Rows[i].Kind == T054Event && rep.Rows[i].EventKind == "deflect" {
			defl = &rep.Rows[i]
		}
	}
	if defl == nil {
		t.Fatal("no deflect event")
	}
	if defl.Coverage != "no-sidecar-record" {
		t.Fatalf("deflect coverage=%q, want no-sidecar-record (sensing-only participant)", defl.Coverage)
	}
	t.Logf("T054 missing coverage: interior deflect r0 (frac %.3f) coverage=%s; records=%d", defl.Frac, defl.Coverage, rep.RecordsInWindow)
}

// TestT054UnknownStaleBlockedOverflow pins the explicit state representation.
func TestT054UnknownStaleBlockedOverflow(t *testing.T) {
	// UNKNOWN: a local runner that never sees the ball -> unknown-brake records.
	un := mustSim(t, ScheduledLivePassConfig(100), 30)
	un.SetRunners([]Runner{t050Local(0.10, 0.50, 0.10, 0.50, 0.5, 0, false)})
	unRec := mustRecorder(t, 64)
	un.SetProvenanceRecorder(unRec)
	un.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 1.0})
	for un.ClockMS() < 200 {
		un.Step()
	}
	unRep := T054BuildReport(T054ReportInput{StartMS: 0, EndMS: 200, SubstepMS: un.Config().SubstepMS, RunnerCount: 1, PrefixHistoryPresent: true, Overflowed: unRec.Overflowed(), Events: un.Events(), Samples: un.Samples(), Records: unRec.Records()})
	unknown := 0
	for _, r := range unRep.Rows {
		if r.Kind == T054Publication && r.Policy == "unknown-brake" && r.Freshness == "unknown" {
			unknown++
		}
	}
	if unknown == 0 {
		t.Fatal("no unknown-brake/unknown publication")
	}

	// STALE: saw then lost the ball (moving AWAY) -> stale-retain, no touch.
	st := mustSim(t, ScheduledLivePassConfig(100), 31)
	st.SetRunners([]Runner{t050Local(0.50, 0.50, 0.50, 0.50, 3.0, 0, false)})
	stRec := mustRecorder(t, 64)
	st.SetProvenanceRecorder(stRec)
	st.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 20.0}) // away from the runner
	for st.ClockMS() < 300 {
		st.Step()
	}
	if len(t052Touches(st)) != 0 {
		t.Fatalf("stale scene produced a touch: %v", st.Events())
	}
	stRep := T054BuildReport(T054ReportInput{StartMS: 0, EndMS: 300, SubstepMS: st.Config().SubstepMS, RunnerCount: 1, PrefixHistoryPresent: true, Overflowed: stRec.Overflowed(), Events: st.Events(), Samples: st.Samples(), Records: stRec.Records()})
	sawFresh, sawStale := false, false
	for _, r := range stRep.Rows {
		if r.Kind != T054Publication {
			continue
		}
		if r.ClockMS == 0 && r.Freshness == "fresh" && r.Policy == "chase" {
			sawFresh = true
		}
		if r.Freshness == "stale" && r.Policy == "stale-retain" {
			sawStale = true
		}
	}
	if !sawFresh || !sawStale {
		t.Fatalf("stale scene: initial fresh=%v, stale-retain=%v", sawFresh, sawStale)
	}

	// BLOCKED: the watcher scene has reaction-blocked attempts.
	golden := T054GoldenReport(t)
	blocked := 0
	for _, r := range golden.Rows {
		if r.Kind == T054Publication && r.Gate == "reaction-blocked" {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("no reaction-blocked publication")
	}

	// OVERFLOW: a tiny recorder actually saturates.
	ov := mustSim(t, ScheduledLivePassConfig(100), 32)
	ov.SetRunners([]Runner{t050Local(0.50, 0.50, 0.50, 0.50, 3.0, 0, false)})
	ovRec := mustRecorder(t, 1) // capacity 1
	ov.SetProvenanceRecorder(ovRec)
	ov.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 1.0})
	for ov.ClockMS() < 300 {
		ov.Step()
	}
	ovRep := T054BuildReport(T054ReportInput{StartMS: 0, EndMS: 300, SubstepMS: ov.Config().SubstepMS, RunnerCount: 1, PrefixHistoryPresent: true, Overflowed: ovRec.Overflowed(), Events: ov.Events(), Samples: ov.Samples(), Records: ovRec.Records()})
	if !ovRep.Overflowed || ovRep.RecordsInWindow > 1 {
		t.Fatalf("overflow not represented: overflowed=%v records=%d", ovRep.Overflowed, ovRep.RecordsInWindow)
	}
	t.Logf("T054 states: unknown=%d staleOK=%v blocked=%d overflow=%v(records=%d)", unknown, sawStale, blocked, ovRep.Overflowed, ovRep.RecordsInWindow)
}

// TestT054ResumeAbsoluteTails uses COMPLETE JSON engine continuation
// (t046JSONResume), steps straight and resumed in lockstep with full Snapshot
// equality, and compares report rows on a moving22 T-053 deflect window whose
// interior deflect falls AFTER the split.
func TestT054ResumeAbsoluteTails(t *testing.T) {
	build := func() *Sim {
		sim := mustSim(t, ScheduledLivePassConfig(100), t053DeflectSeed)
		sim.SetRunners(t053Runners(false))
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.90, TargetY: 0.50, SpeedMPS: 20.0, Passer: -1, ControlError: 1.0})
		return sim
	}
	const split, end = 900, 2000
	straight := build()
	for straight.ClockMS() < split {
		straight.Step()
	}
	if len(t052Touches(straight)) != 0 {
		t.Fatal("split 900 already contains a touch (deflect should be after the split)")
	}
	resumed := t046JSONResume(t, straight) // complete JSON engine continuation
	srec := mustRecorder(t, 4096)
	straight.SetProvenanceRecorder(srec)
	rrec := mustRecorder(t, 4096)
	resumed.SetProvenanceRecorder(rrec)
	for straight.ClockMS() < end {
		if resumed.ClockMS() != straight.ClockMS() {
			t.Fatalf("clocks diverged %d vs %d", straight.ClockMS(), resumed.ClockMS())
		}
		straight.Step()
		resumed.Step()
		if !reflect.DeepEqual(straight.Snapshot(), resumed.Snapshot()) {
			t.Fatalf("full engine snapshot diverged at %d after JSON resume", straight.ClockMS())
		}
	}
	sRep := T054BuildReport(T054ReportInput{StartMS: split, EndMS: end, SubstepMS: straight.Config().SubstepMS, RunnerCount: len(straight.runners), PrefixHistoryPresent: true, Overflowed: srec.Overflowed(), Events: straight.Events(), Samples: straight.Samples(), Records: srec.Records()})
	rRep := T054BuildReport(T054ReportInput{StartMS: split, EndMS: end, SubstepMS: resumed.Config().SubstepMS, RunnerCount: len(resumed.runners), PrefixHistoryPresent: false, Overflowed: rrec.Overflowed(), Events: resumed.Events(), Samples: resumed.Samples(), Records: rrec.Records()})
	if len(rRep.Rows) == 0 {
		t.Fatal("resumed tail report is empty")
	}
	if !reflect.DeepEqual(sRep.Rows, rRep.Rows) {
		t.Fatalf("resumed absolute tail rows differ from the straight run")
	}
	hasDeflect, hasPub, hasSample := false, false, false
	for _, r := range rRep.Rows {
		switch {
		case r.Kind == T054Event && r.EventKind == "deflect":
			hasDeflect = true
			if !(r.Frac > 0 && r.Frac < 1) {
				t.Fatalf("tail deflect frac %v not interior", r.Frac)
			}
		case r.Kind == T054Publication:
			hasPub = true
		case r.Kind == T054Sample:
			hasSample = true
		}
	}
	if !hasDeflect || !hasPub || !hasSample {
		t.Fatalf("tail not non-vacuous: deflect=%v pub=%v sample=%v", hasDeflect, hasPub, hasSample)
	}
	blob, err := json.Marshal(rRep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back T054Report
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(rRep, back) {
		t.Fatal("report did not JSON round-trip")
	}
	t.Logf("T054 resume: %d absolute tail rows (incl. the post-split interior deflect) reproduced after complete JSON Resume; JSON round-trip exact", len(rRep.Rows))
}

// TestT054NoMutationAndDeterministicOrder pins purity and the tie order.
func TestT054NoMutationAndDeterministicOrder(t *testing.T) {
	sim := mustSim(t, ScheduledLivePassConfig(100), 34)
	sim.SetRunners([]Runner{t050Local(0.50, 0.50, 0.50, 0.50, 3.0, 0, false)})
	rec := mustRecorder(t, 64)
	sim.SetProvenanceRecorder(rec)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 1.0})
	for sim.ClockMS() < 200 {
		sim.Step()
	}
	before := sim.Snapshot()
	in := T054ReportInput{StartMS: 0, EndMS: 200, SubstepMS: sim.Config().SubstepMS, RunnerCount: 1, PrefixHistoryPresent: true, Overflowed: rec.Overflowed(), Events: sim.Events(), Samples: sim.Samples(), Records: rec.Records()}
	base := T054BuildReport(in)
	baseCopy := T054BuildReport(in)
	if !reflect.DeepEqual(base, baseCopy) {
		t.Fatal("two builds differ (non-deterministic)")
	}
	if !reflect.DeepEqual(before, sim.Snapshot()) {
		t.Fatal("build mutated the source sim")
	}
	mut := base
	if len(mut.Rows) > 0 {
		mut.Rows[0].Runner = 999 // mutate the returned report
	}
	if !reflect.DeepEqual(before, sim.Snapshot()) {
		t.Fatal("report aliases the source sim")
	}
	// No alias: mutating the INPUT events must not change an already-built report,
	// but a fresh build must see the mutation (value copies, no shared backing).
	e := in.Events[0]
	in.Events[0].BallX = e.BallX + 100
	after := T054BuildReport(in)
	in.Events[0] = e
	if reflect.DeepEqual(baseCopy, after) {
		t.Fatal("build ignored the input mutation (alias/copy defect)")
	}
	if !reflect.DeepEqual(before, sim.Snapshot()) {
		t.Fatal("sim mutated during the input-mutation check")
	}

	// Real-record tie order (golden watcher report): at t=0 publications come by
	// ascending runner (r1 before r2) and precede the release event; the global
	// order invariant (time -> category -> runner -> seq) holds throughout.
	g := T054GoldenReport(t)
	at := map[string]int{}
	for i, r := range g.Rows {
		if r.ExactMS == 0 {
			at[fmt.Sprintf("%s:%d", r.Kind, r.Runner)] = i
		}
	}
	if at["publication:1"] >= at["publication:2"] || at["publication:2"] >= at["event:-1"] {
		t.Fatalf("tie order at t=0 wrong: %v", at)
	}
	for i := 1; i < len(g.Rows); i++ {
		a, b := g.Rows[i-1], g.Rows[i]
		if a.ExactMS > b.ExactMS {
			t.Fatalf("rows not sorted by exact time at %d", i)
		}
		if a.ExactMS == b.ExactMS {
			if t054Rank(a.Kind) > t054Rank(b.Kind) {
				t.Fatalf("tie category order wrong at %d", i)
			}
			if t054Rank(a.Kind) == t054Rank(b.Kind) && a.Kind == T054Event && a.Seq > b.Seq {
				t.Fatalf("tie seq order wrong at %d", i)
			}
		}
	}
	t.Logf("T054 purity: deterministic, no mutation/alias; real-record tie order (r1<r2<release at t=0) verified; %d rows", len(g.Rows))
}

// t054ReportFor builds a report over the sim's full elapsed window (no records).
func t054ReportFor(sim *Sim) T054Report {
	return T054BuildReport(T054ReportInput{
		StartMS: 0, EndMS: sim.ClockMS() + 1, SubstepMS: sim.Config().SubstepMS, RunnerCount: len(sim.runners),
		PrefixHistoryPresent: true, Events: sim.Events(), Samples: sim.Samples(),
	})
}

// TestT054EventTimingClassification pins the event-timing correction: path is NOT
// fractional (exact==rounded==end clock), control/deflect/out/stop are fractional
// with boundary frac 0/1 handled exactly, and the actual cfg interval (not 20) is
// used. It also checks the generating-step window filter.
func TestT054EventTimingClassification(t *testing.T) {
	// Real path event: emitted at the END clock with Frac 0.
	sim := mustSim(t, ScheduledLivePassConfig(100), 40)
	sim.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for sim.ClockMS() < 300 {
		sim.Step()
	}
	rep := t054ReportFor(sim)
	var path *T054Row
	for i := range rep.Rows {
		if rep.Rows[i].Kind == T054Event && rep.Rows[i].EventKind == "path" {
			path = &rep.Rows[i]
			break
		}
	}
	if path == nil {
		t.Fatal("no path event")
	}
	if path.ExactMS != float64(path.ClockMS) || path.RoundedMS != path.ClockMS {
		t.Fatalf("path timing wrong: exact=%v rounded=%d clock=%d", path.ExactMS, path.RoundedMS, path.ClockMS)
	}
	if path.StepStartMS != path.ClockMS-rep.SubstepMS {
		t.Fatalf("path stepStart=%d, want %d", path.StepStartMS, path.ClockMS-rep.SubstepMS)
	}

	// Boundary frac 0: the ball is already within reach at the pre-step.
	b0 := mustSim(t, ScheduledLivePassConfig(100), 41)
	b0.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	b0.SetBall(Ball{Mode: BallRolling, X: 0.505, Y: 0.50, VXM: 2.0})
	b0.Step()
	r0 := t054ControlRow(t, b0)
	if r0.Frac != 0 || r0.ExactMS != float64(r0.StepStartMS) || r0.RoundedMS != r0.StepStartMS {
		t.Fatalf("frac 0 timing wrong: %+v", r0)
	}

	// Boundary frac 1: gated to the step end by the cooldown.
	b1 := mustSim(t, ScheduledLivePassConfig(100), 42)
	b1.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	b1.SetBall(Ball{Mode: BallRolling, X: 0.505, Y: 0.50, VXM: 2.0})
	b1.runners[0].CooldownUntilMS = SubstepMS
	b1.Step()
	r1 := t054ControlRow(t, b1)
	end := r1.StepStartMS + b1.Config().SubstepMS
	if r1.Frac != 1 || r1.ExactMS != float64(end) || r1.RoundedMS != end {
		t.Fatalf("frac 1 timing wrong: %+v (end=%d)", r1, end)
	}

	// Actual cfg interval: a 10 ms substep is used, not a hardcoded 20.
	cfg10 := Config{SubstepMS: 10, DecelMPS2: RollDecelMPS2, LivePass: true}
	sim10 := mustSim(t, cfg10, 43)
	sim10.SetRunners([]Runner{t052Sensing(t, 0.50, 0.50, 3.0)})
	sim10.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.80, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	for sim10.ClockMS() < 200 {
		sim10.Step()
	}
	rep10 := t054ReportFor(sim10)
	if rep10.SubstepMS != 10 {
		t.Fatalf("report substep=%d, want 10", rep10.SubstepMS)
	}
	sawPath10 := false
	for _, r := range rep10.Rows {
		if r.Kind == T054Event && r.EventKind == "path" {
			sawPath10 = true
			if r.StepStartMS != r.ClockMS-10 || r.ExactMS != float64(r.ClockMS) {
				t.Fatalf("cfg10 path timing wrong: %+v", r)
			}
		}
	}
	if !sawPath10 {
		t.Fatal("no path event with a 10 ms cfg")
	}

	// Generating-step window filter: [0,20) keeps the step-0 rows (incl. the
	// sample at clock 20) and drops the sample at clock 40.
	w20 := T054BuildReport(T054ReportInput{StartMS: 0, EndMS: 20, SubstepMS: sim.Config().SubstepMS, RunnerCount: len(sim.runners), PrefixHistoryPresent: true, Events: sim.Events(), Samples: sim.Samples()})
	saw20, saw40 := false, false
	for _, r := range w20.Rows {
		if r.Kind == T054Sample && r.ClockMS == 20 {
			saw20 = true
		}
		if r.Kind == T054Sample && r.ClockMS == 40 {
			saw40 = true
		}
	}
	if !saw20 || saw40 {
		t.Fatalf("window filter wrong: clock20=%v clock40=%v", saw20, saw40)
	}
	t.Logf("T054 timing: path exact==rounded==end clock; control frac0 exact=%v frac1 exact=%v; cfg10 substep=10; window[0,20) keeps clock20 sample and drops clock40",
		float64(r0.StepStartMS), float64(end))
}

// t054ControlRow returns the first control event row of a sim's report.
func t054ControlRow(t *testing.T, sim *Sim) T054Row {
	t.Helper()
	rep := t054ReportFor(sim)
	for _, r := range rep.Rows {
		if r.Kind == T054Event && r.EventKind == "control" {
			return r
		}
	}
	t.Fatalf("no control row: %v", sim.Events())
	return T054Row{}
}
