package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"
)

// pointKindCount counts trace points with a given kind.
func pointKindCount(trace Trace, kind string) int {
	n := 0
	for i := range trace.Points {
		if trace.Points[i].Kind == kind {
			n++
		}
	}
	return n
}

// clipBySlug builds the prototype clips once and indexes them by slug.
func clipBySlug(t *testing.T) map[string]Trace {
	t.Helper()
	clips, err := BuildPrototypeClips()
	if err != nil {
		t.Fatalf("BuildPrototypeClips: %v", err)
	}
	if len(clips) != 3 {
		t.Fatalf("BuildPrototypeClips returned %d clips, want 3", len(clips))
	}
	bySlug := make(map[string]Trace, len(clips))
	for _, c := range clips {
		bySlug[c.Slug] = c
	}
	return bySlug
}

// TestBuildPrototypeClipsComplete pins the versioned trace contract: schema
// fields, the sample grid, full runner state per point (including t0) and the
// scripted release.
func TestBuildPrototypeClipsComplete(t *testing.T) {
	clips, err := BuildPrototypeClips()
	if err != nil {
		t.Fatalf("BuildPrototypeClips: %v", err)
	}
	for _, trace := range clips {
		if trace.SchemaVersion != TraceSchemaVersion {
			t.Fatalf("%s schemaVersion = %d, want %d", trace.Slug, trace.SchemaVersion, TraceSchemaVersion)
		}
		if trace.TimeUnit != "ms" {
			t.Fatalf("%s timeUnit = %q, want ms", trace.Slug, trace.TimeUnit)
		}
		if trace.SubstepMS != SubstepMS {
			t.Fatalf("%s substepMs = %d, want %d", trace.Slug, trace.SubstepMS, SubstepMS)
		}
		if trace.RunnerCount == 0 {
			t.Fatalf("%s has no runners", trace.Slug)
		}
		if len(trace.Points) == 0 {
			t.Fatalf("%s has no points", trace.Slug)
		}
		if trace.FrameCount != len(trace.Points) {
			t.Fatalf("%s frameCount = %d, len(points) = %d", trace.Slug, trace.FrameCount, len(trace.Points))
		}
		if trace.EventCount != len(trace.Events) {
			t.Fatalf("%s eventCount = %d, len(events) = %d", trace.Slug, trace.EventCount, len(trace.Events))
		}
		if got := pointKindCount(trace, "sample"); trace.SampleCount != got {
			t.Fatalf("%s sampleCount = %d, sample points = %d", trace.Slug, trace.SampleCount, got)
		}
		if trace.Points[0].TMS != 0 {
			t.Fatalf("%s first point tMs = %v, want 0 (must include t0)", trace.Slug, trace.Points[0].TMS)
		}

		release := false
		for _, e := range trace.Events {
			if e.Kind == "release" {
				release = true
				if !e.Scripted {
					t.Fatalf("%s release event is not marked scripted", trace.Slug)
				}
				if e.ClockMS != 0 {
					t.Fatalf("%s release clock = %d, want 0", trace.Slug, e.ClockMS)
				}
			}
		}
		if !release {
			t.Fatalf("%s has no release event", trace.Slug)
		}

		for _, p := range trace.Points {
			if len(p.Runners) != trace.RunnerCount {
				t.Fatalf("%s point at %v has %d runners, want %d", trace.Slug, p.TMS, len(p.Runners), trace.RunnerCount)
			}
			if p.Kind == "sample" && math.Mod(p.TMS, float64(trace.SubstepMS)) != 0 {
				t.Fatalf("%s sample point tMs %v is off the %d ms grid", trace.Slug, p.TMS, trace.SubstepMS)
			}
		}
	}
}

// TestPrototypeClipSignatures pins the three demonstrated behaviours.
func TestPrototypeClipSignatures(t *testing.T) {
	bySlug := clipBySlug(t)

	unopposed := bySlug["unopposed-reception"]
	if pointKindCount(unopposed, "control") != 1 || pointKindCount(unopposed, "deflect") != 0 {
		t.Fatalf("unopposed clip: control=%d deflect=%d, want 1/0",
			pointKindCount(unopposed, "control"), pointKindCount(unopposed, "deflect"))
	}

	interception := bySlug["reachable-interception"]
	controlRunner := -1
	for _, e := range interception.Events {
		if e.Kind == "control" {
			controlRunner = e.Runner
		}
	}
	if controlRunner != 1 {
		t.Fatalf("reachable interception control runner = %d, want 1 (the pursuing defender)", controlRunner)
	}

	miscontrol := bySlug["miscontrol-recovery"]
	if pointKindCount(miscontrol, "deflect") == 0 {
		t.Fatal("miscontrol clip produced no deflection")
	}
	if pointKindCount(miscontrol, "control") == 0 {
		t.Fatal("miscontrol clip did not recover the loose ball")
	}
}

// TestTracePointVelocitySemantics pins that a TracePoint describes the ball's
// OUTGOING state: a deflected knot carries the event's outgoing velocity, a clean
// control carries zero, and the event metadata still exposes both incoming and
// outgoing velocities.
func TestTracePointVelocitySemantics(t *testing.T) {
	bySlug := clipBySlug(t)
	trace := bySlug["miscontrol-recovery"]

	sawDeflect, sawControl := false, false
	for _, e := range trace.Events {
		var knot TracePoint
		found := false
		for _, p := range trace.Points {
			if p.Kind == e.Kind && math.Abs(p.TMS-e.ExactTimeMS) < 1e-9 {
				knot, found = p, true
			}
		}
		switch e.Kind {
		case "deflect":
			if !found {
				t.Fatalf("deflect event has no knot")
			}
			sawDeflect = true
			if knot.VXM != e.OutVXM || knot.VYM != e.OutVYM {
				t.Fatalf("deflect knot velocity (%v,%v) != outgoing (%v,%v)", knot.VXM, knot.VYM, e.OutVXM, e.OutVYM)
			}
			if e.VXM == 0 && e.VYM == 0 {
				t.Fatal("deflect event exposes no incoming velocity")
			}
		case "control":
			if !found {
				t.Fatalf("control event has no knot")
			}
			sawControl = true
			if knot.VXM != 0 || knot.VYM != 0 {
				t.Fatalf("control knot velocity (%v,%v) != (0,0)", knot.VXM, knot.VYM)
			}
		}
	}
	if !sawDeflect || !sawControl {
		t.Fatalf("missing deflect/control events (deflect=%v control=%v)", sawDeflect, sawControl)
	}
}

// TestTraceKnotRunnerContactStates pins that every contact knot carries all
// runner positions and that the touching runner's knot position matches the
// event's fractional contact position exactly.
func TestTraceKnotRunnerContactStates(t *testing.T) {
	bySlug := clipBySlug(t)
	for slug, trace := range bySlug {
		for _, e := range trace.Events {
			if e.Kind != "control" && e.Kind != "deflect" {
				continue
			}
			found := false
			for _, p := range trace.Points {
				if p.Kind != e.Kind || math.Abs(p.TMS-e.ExactTimeMS) > 1e-9 {
					continue
				}
				found = true
				if len(p.Runners) != trace.RunnerCount {
					t.Fatalf("%s knot has %d runners, want %d", slug, len(p.Runners), trace.RunnerCount)
				}
				for i, r := range p.Runners {
					if math.IsNaN(r.X) || math.IsNaN(r.Y) || math.IsInf(r.X, 0) || math.IsInf(r.Y, 0) {
						t.Fatalf("%s knot runner %d non-finite", slug, i)
					}
				}
				toucher := p.Runners[e.Runner]
				if toucher.X != e.RunnerX || toucher.Y != e.RunnerY {
					t.Fatalf("%s %s runner %d knot (%v,%v) != event contact (%v,%v)",
						slug, e.Kind, e.Runner, toucher.X, toucher.Y, e.RunnerX, e.RunnerY)
				}
			}
			if !found {
				t.Fatalf("%s %s event has no knot", slug, e.Kind)
			}
		}
	}
}

// TestBuildTraceResumedTailMatchesStraight pins that BuildTrace on a resumed sim
// produces a tail whose events (associated, no historical/release) and whole
// points (including contact knots) are identical to the straight run's.
func TestBuildTraceResumedTailMatchesStraight(t *testing.T) {
	build := func() *Sim {
		sim, err := NewSim(LivePassConfig(), 7)
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		sim.SetRunners([]Runner{InertialRunner(0.66, 0.50, 0.30, 0.50, DefaultMotionParams())})
		sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
		return sim
	}
	const mid = 1000

	full := build()
	fullTrace := BuildTrace(ClipInput{Sim: full, Seed: 7, Slug: "full", Label: "full", Description: "full", HorizonMS: 8000})

	tail := build()
	for tail.ClockMS() < mid && tail.Ball().Mode != BallHeld {
		tail.Step()
	}
	resumed, err := Resume(tail.Snapshot())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	resumedTrace := BuildTrace(ClipInput{Sim: resumed, Seed: 7, Slug: "tail", Label: "tail", Description: "tail", HorizonMS: 8000})

	if resumedTrace.Points[0].TMS != float64(mid) {
		t.Fatalf("resumed trace starts at %v, want %d", resumedTrace.Points[0].TMS, mid)
	}

	bySeq := make(map[int]TraceEvent, len(fullTrace.Events))
	for _, e := range fullTrace.Events {
		bySeq[e.Seq] = e
	}
	for _, e := range resumedTrace.Events {
		if e.ClockMS <= mid {
			t.Fatalf("resumed trace included historical event seq %d clock %d", e.Seq, e.ClockMS)
		}
		if e.Kind == "release" {
			t.Fatal("resumed tail included the release")
		}
		se, ok := bySeq[e.Seq]
		if !ok || !reflect.DeepEqual(se, e) {
			t.Fatalf("resumed event seq %d differs from straight", e.Seq)
		}
	}
	if len(resumedTrace.Events) == 0 {
		t.Fatal("resumed tail had no events")
	}

	key := func(p TracePoint) string { return p.Kind + "@" + strconv.FormatFloat(p.TMS, 'g', -1, 64) }
	fullPoints := make(map[string]TracePoint, len(fullTrace.Points))
	for _, p := range fullTrace.Points {
		fullPoints[key(p)] = p
	}
	for _, p := range resumedTrace.Points {
		sp, ok := fullPoints[key(p)]
		if !ok || !reflect.DeepEqual(sp, p) {
			t.Fatalf("resumed point %s not equal to the straight run", key(p))
		}
	}
}

// TestPrototypeFixtureTrace pins that the tiny web fixture is real exporter
// output containing at least one sample and one clean control knot.
func TestPrototypeFixtureTrace(t *testing.T) {
	trace, err := PrototypeFixtureTrace()
	if err != nil {
		t.Fatalf("PrototypeFixtureTrace: %v", err)
	}
	if trace.SchemaVersion != TraceSchemaVersion || len(trace.Points) == 0 {
		t.Fatalf("fixture shape invalid: %+v", trace)
	}
	if pointKindCount(trace, "control") == 0 {
		t.Fatal("fixture has no control knot")
	}
	if pointKindCount(trace, "sample") == 0 {
		t.Fatal("fixture has no sample points")
	}
}

// TestTraceEventTimesAndKnots pins exactTimeMs and proves a control knot exists
// at that time with ownership set there (not at the end-step sample).
func TestTraceEventTimesAndKnots(t *testing.T) {
	bySlug := clipBySlug(t)
	for slug, trace := range bySlug {
		for _, e := range trace.Events {
			if e.Kind == "release" || e.Kind == "path" {
				if e.ExactTimeMS != float64(e.ClockMS) {
					t.Fatalf("%s %s exactTimeMs = %v, want clock %d", slug, e.Kind, e.ExactTimeMS, e.ClockMS)
				}
				continue
			}
			want := float64(e.ClockMS-trace.SubstepMS) + e.Frac*float64(trace.SubstepMS)
			if math.Abs(e.ExactTimeMS-want) > 1e-9 {
				t.Fatalf("%s %s exactTimeMs = %v, want %v", slug, e.Kind, e.ExactTimeMS, want)
			}
		}
		for _, e := range trace.Events {
			if e.Kind != "control" {
				continue
			}
			found := false
			for _, p := range trace.Points {
				if p.Kind == "control" && math.Abs(p.TMS-e.ExactTimeMS) < 1e-9 {
					found = true
					if p.Owner != e.Runner {
						t.Fatalf("%s control knot owner = %d, want %d", slug, p.Owner, e.Runner)
					}
				}
			}
			if !found {
				t.Fatalf("%s has no control knot at %v", slug, e.ExactTimeMS)
			}
		}
	}
}

// TestTraceTailMatchesSim proves the exported timeline ends at the sim's true
// final state (the exporter state tail is not approximated).
func TestTraceTailMatchesSim(t *testing.T) {
	sim, err := NewSim(LivePassConfig(), 7)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	sim.SetRunners([]Runner{InertialRunner(0.66, 0.50, 0.30, 0.50, DefaultMotionParams())})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
	trace := BuildTrace(ClipInput{Sim: sim, Seed: 7, Slug: "tail", Label: "tail", Description: "tail", HorizonMS: 8000})

	var last TracePoint
	for _, p := range trace.Points {
		if p.Kind == "sample" {
			last = p
		}
	}
	ball := sim.Ball()
	if last.BallX != ball.X || last.BallY != ball.Y || last.Mode != ball.Mode.String() {
		t.Fatalf("last sample (%v,%v,%s) != sim ball (%v,%v,%s)",
			last.BallX, last.BallY, last.Mode, ball.X, ball.Y, ball.Mode.String())
	}
	runners := sim.Runners()
	if len(last.Runners) != len(runners) {
		t.Fatalf("last sample has %d runners, sim has %d", len(last.Runners), len(runners))
	}
	for i := range runners {
		if last.Runners[i].X != runners[i].X || last.Runners[i].Y != runners[i].Y {
			t.Fatalf("last sample runner %d (%v,%v) != sim (%v,%v)",
				i, last.Runners[i].X, last.Runners[i].Y, runners[i].X, runners[i].Y)
		}
	}
}

// TestTraceSnapshotTailContinuity proves a mid-flight Resume produces the exact
// same exported sample tail as the straight run.
func TestTraceSnapshotTailContinuity(t *testing.T) {
	straight, err := NewSim(LivePassConfig(), 7)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	straight.SetRunners([]Runner{InertialRunner(0.66, 0.50, 0.30, 0.50, DefaultMotionParams())})
	straight.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
	for straight.ClockMS() < 1000 && straight.Ball().Mode != BallHeld {
		straight.Step()
	}
	resumed, err := Resume(straight.Snapshot())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	straightTail := captureFrames(straight, 8000)
	resumedTail := captureFrames(resumed, 8000)
	if !reflect.DeepEqual(straightTail, resumedTail) {
		t.Fatal("resumed exported tail differs from the straight run")
	}
}

// TestTraceDeterministic proves the whole export is reproducible.
func TestTraceDeterministic(t *testing.T) {
	a, err := BuildPrototypeClips()
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	b, err := BuildPrototypeClips()
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("BuildPrototypeClips is not deterministic")
	}
}

// TestTraceJSONSchemaKeys locks the camelCase wire keys the renderer mirrors.
func TestTraceJSONSchemaKeys(t *testing.T) {
	clips, err := BuildPrototypeClips()
	if err != nil {
		t.Fatalf("BuildPrototypeClips: %v", err)
	}
	raw, err := json.Marshal(clips[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal top: %v", err)
	}
	for _, key := range []string{
		"schemaVersion", "timeUnit", "substepMs", "seed", "slug", "label", "description",
		"scriptedNote", "pitch", "runnerCount", "sampleCount", "frameCount", "eventCount", "events", "points",
	} {
		if _, ok := top[key]; !ok {
			t.Fatalf("trace JSON missing key %q", key)
		}
	}

	var points []map[string]json.RawMessage
	if err := json.Unmarshal(top["points"], &points); err != nil {
		t.Fatalf("unmarshal points: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("no points to inspect")
	}
	for _, key := range []string{"tMs", "kind", "ballX", "ballY", "vxM", "vyM", "mode", "owner", "runners"} {
		if _, ok := points[0][key]; !ok {
			t.Fatalf("point JSON missing key %q", key)
		}
	}
}
