package liveplay

import (
	"fmt"
	"sort"
)

// Authoritative trace export (T-039).
//
// This file serializes the isolated live-play prototype into a versioned,
// self-describing trace so a same-clock renderer can interpolate the very states
// the simulation produced. It is deliberately separate from the production
// MatchResult dump and from the viewer's ballStory retiming: the renderer must
// not invent flight time, a winner or a receipt.
//
// The merged point timeline interleaves the integer-clock samples with one exact
// knot for every state-changing event at its declared within-substep fraction
// (exactTimeMs = clockMs - substepMs*(1-frac)). Each point carries the
// authoritative ball state and every participant's POSITION (x, y) interpolated
// at that fraction. This is deliberately a positions-only render contract, not a
// full simulation snapshot (it has no velocities, intents, reaction/cooldown or
// RNG state; those live in the sim's own Snapshot). A piecewise-linear renderer
// therefore reproduces a contact, a deflection and an ownership transition at the
// exact instant they occurred instead of straight across a bounce or at the
// end-step sample. When several knots share a time, the LAST one (highest event
// Seq / post-state) wins.

// TraceSchemaVersion is the version of the exported authoritative trace contract.
// Bump it whenever the JSON shape or the time/fraction semantics change.
const TraceSchemaVersion = 1

// TracePitch records the metre bridge for the normalized pitch coordinates.
type TracePitch struct {
	LengthM float64 `json:"lengthM"`
	WidthM  float64 `json:"widthM"`
}

// TraceRunnerState is one runner's normalized position at a trace point.
type TraceRunnerState struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TracePoint is one authoritative interpolation knot: an integer-clock sample or
// an exact event knot. The renderer interpolates linearly between consecutive
// points and takes the mode/owner from the LEFT point, so ownership transitions
// at the event knot, never at the end-step sample. Runners carry positions only
// (x, y) — this is a render contract, not a full simulation snapshot. VXM/VYM
// describe the ball's OUTGOING velocity at this instant (zero for a clean
// control and for a stopped/dead ball).
type TracePoint struct {
	TMS     float64            `json:"tMs"`
	Kind    string             `json:"kind"`
	BallX   float64            `json:"ballX"`
	BallY   float64            `json:"ballY"`
	VXM     float64            `json:"vxM"`
	VYM     float64            `json:"vyM"`
	Mode    string             `json:"mode"`
	Owner   int                `json:"owner"`
	Runners []TraceRunnerState `json:"runners"`
}

// TraceEvent is one authoritative live-play event with both its integer end-step
// clock and its exact within-substep time. Scripted marks the fixture's initial
// action (the release); every later contact, deflection and control is produced
// by the physics and the seeded control budget.
type TraceEvent struct {
	Seq         int     `json:"seq"`
	Kind        string  `json:"kind"`
	ClockMS     int     `json:"clockMs"`
	Frac        float64 `json:"frac"`
	ExactTimeMS float64 `json:"exactTimeMs"`
	Runner      int     `json:"runner"`
	BallX       float64 `json:"ballX"`
	BallY       float64 `json:"ballY"`
	VXM         float64 `json:"vxM"`
	VYM         float64 `json:"vyM"`
	OutVXM      float64 `json:"outVxM"`
	OutVYM      float64 `json:"outVyM"`
	RunnerX     float64 `json:"runnerX"`
	RunnerY     float64 `json:"runnerY"`
	Scripted    bool    `json:"scripted"`
}

// Trace is the complete versioned export for one prototype clip.
type Trace struct {
	SchemaVersion int          `json:"schemaVersion"`
	TimeUnit      string       `json:"timeUnit"`
	SubstepMS     int          `json:"substepMs"`
	Seed          uint64       `json:"seed"`
	Slug          string       `json:"slug"`
	Label         string       `json:"label"`
	Description   string       `json:"description"`
	ScriptedNote  string       `json:"scriptedNote"`
	Pitch         TracePitch   `json:"pitch"`
	RunnerCount   int          `json:"runnerCount"`
	SampleCount   int          `json:"sampleCount"`
	FrameCount    int          `json:"frameCount"`
	EventCount    int          `json:"eventCount"`
	Events        []TraceEvent `json:"events"`
	Points        []TracePoint `json:"points"`
}

// ClipInput is a scripted prototype fixture: a released live-play Sim plus the
// export metadata. The release is the SCRIPTED setup action; the exporter never
// decides the outcome, it only records whatever the physics produced.
type ClipInput struct {
	Sim         *Sim
	Seed        uint64
	Slug        string
	Label       string
	Description string
	HorizonMS   int
}

// traceScriptedNote is attached to every clip so the viewer never presents the
// scripted fixture setup as a simulated decision.
const traceScriptedNote = "The initial release is a scripted fixture action; every subsequent contact, deflection and control is produced by the live-play physics and its seeded contact-control budget."

// BuildTrace advances the clip to its horizon (or until the ball is controlled),
// then merges the integer-clock sample frames with exact event knots into one
// interpolation timeline. It is the default (unchecked) convenience: a selected
// body model (the positions-only trace lacks body-impact knots) or a body-step
// fault yields a ZERO Trace rather than a false schema1 success. Callers of the
// feature path should use BuildTraceChecked.
func BuildTrace(in ClipInput) Trace {
	tr, err := buildTrace(in)
	if err != nil {
		return Trace{}
	}
	return tr
}

// BuildTraceChecked is BuildTrace with an explicit error: it rejects an active
// body model3 (the trace cannot certify piecewise body paths) and any body-step
// fault, without mutating the source simulation.
func BuildTraceChecked(in ClipInput) (Trace, error) { return buildTrace(in) }

func buildTrace(in ClipInput) (Trace, error) {
	sim := in.Sim
	if sim.body != nil && sim.body.Fault != nil {
		return Trace{}, bodyFaultError(sim.body.Fault)
	}
	if sim.integrated != nil && sim.integrated.ModelVersion == IntegratedModelVersionBody {
		return Trace{}, fmt.Errorf("liveplay: trace does not support the selected body model3: %w", ErrBodyVersion)
	}
	startClock := sim.ClockMS()
	frames := captureFrames(sim, in.HorizonMS)
	if err := sim.StepError(); err != nil {
		return Trace{}, err
	}
	byClock := make(map[int]TracePoint, len(frames))
	for _, f := range frames {
		byClock[int(f.TMS)] = f
	}

	events := sim.Events()
	traceEvents := make([]TraceEvent, 0, len(events))
	knots := make([]TracePoint, 0, len(events))
	for _, e := range events {
		if e.ClockMS < startClock {
			continue // historical event before this trace's start clock
		}
		knot, hasKnot := eventKnot(e, byClock, sim.cfg.SubstepMS)
		if e.Kind == PassEventRelease {
			if e.ClockMS != startClock {
				continue
			}
		} else if !hasKnot {
			// An event at the start clock has no pre-step frame; the start sample
			// already carries its state, so skip it and keep events/knots 1:1.
			continue
		}
		traceEvents = append(traceEvents, TraceEvent{
			Seq:         e.Seq,
			Kind:        e.Kind.String(),
			ClockMS:     e.ClockMS,
			Frac:        e.Frac,
			ExactTimeMS: eventExactTimeMS(e, sim.cfg.SubstepMS),
			Runner:      e.Runner,
			BallX:       e.BallX,
			BallY:       e.BallY,
			VXM:         e.VXM,
			VYM:         e.VYM,
			OutVXM:      e.OutVXM,
			OutVYM:      e.OutVYM,
			RunnerX:     e.RunnerX,
			RunnerY:     e.RunnerY,
			Scripted:    e.Kind == PassEventRelease,
		})
		if e.Kind != PassEventRelease {
			knots = append(knots, knot)
		}
	}

	points := make([]TracePoint, 0, len(frames)+len(knots))
	points = append(points, frames...)
	points = append(points, knots...)
	sort.SliceStable(points, func(i, j int) bool { return points[i].TMS < points[j].TMS })

	return Trace{
		SchemaVersion: TraceSchemaVersion,
		TimeUnit:      "ms",
		SubstepMS:     sim.cfg.SubstepMS,
		Seed:          in.Seed,
		Slug:          in.Slug,
		Label:         in.Label,
		Description:   in.Description,
		ScriptedNote:  traceScriptedNote,
		Pitch:         TracePitch{LengthM: PitchLengthM, WidthM: PitchWidthM},
		RunnerCount:   len(sim.Runners()),
		SampleCount:   len(frames),
		FrameCount:    len(points),
		EventCount:    len(traceEvents),
		Events:        traceEvents,
		Points:        points,
	}, nil
}

// captureFrames records every participant's position at every integer clock from
// the sim's current position until the horizon or a control. The first frame is
// the current (possibly initial t0) state, so a trace always includes its start.
func captureFrames(sim *Sim, horizonMS int) []TracePoint {
	frames := []TracePoint{frameAt(sim)}
	for sim.ClockMS() < horizonMS && sim.Ball().Mode != BallHeld {
		sim.Step()
		if sim.body != nil && sim.body.Fault != nil {
			break // a body-step fault leaves the clock unchanged; stop, do not spin
		}
		frames = append(frames, frameAt(sim))
	}
	return frames
}

// frameAt snapshots the authoritative state at the sim's current clock.
func frameAt(sim *Sim) TracePoint {
	b := sim.Ball()
	runners := sim.Runners()
	states := make([]TraceRunnerState, len(runners))
	for i := range runners {
		states[i] = TraceRunnerState{X: runners[i].X, Y: runners[i].Y}
	}
	return TracePoint{
		TMS:     float64(sim.ClockMS()),
		Kind:    "sample",
		BallX:   b.X,
		BallY:   b.Y,
		VXM:     b.VXM,
		VYM:     b.VYM,
		Mode:    b.Mode.String(),
		Owner:   ownerOf(sim),
		Runners: states,
	}
}

// ownerOf returns the controlling runner while the ball is held, else -1.
func ownerOf(sim *Sim) int {
	if sim.Ball().Mode != BallHeld {
		return -1
	}
	if _, runner := sim.FirstContact(); runner >= 0 {
		return runner
	}
	return -1
}

// eventExactTimeMS is the declared exact time of an event: release/path describe
// post-integration state and are stamped at their end-step clock; a
// contact/deflect/out/stop event is stamped at clock - substep*(1 - frac).
func eventExactTimeMS(e PassEvent, substepMS int) float64 {
	if e.Kind == PassEventRelease || e.Kind == PassEventPath {
		return float64(e.ClockMS)
	}
	return float64(e.ClockMS-substepMS) + e.Frac*float64(substepMS)
}

// eventKnot builds the exact interpolation knot for a state-changing event:
// authoritative fractional ball state plus every runner position interpolated at
// the event fraction from the substep's pre-step and end-step samples.
func eventKnot(e PassEvent, byClock map[int]TracePoint, substepMS int) (TracePoint, bool) {
	end, ok := byClock[e.ClockMS]
	if !ok {
		return TracePoint{}, false
	}
	pre, ok := byClock[e.ClockMS-substepMS]
	if !ok || len(pre.Runners) != len(end.Runners) {
		return TracePoint{}, false
	}
	states := make([]TraceRunnerState, len(end.Runners))
	for i := range end.Runners {
		states[i] = TraceRunnerState{
			X: pre.Runners[i].X + e.Frac*(end.Runners[i].X-pre.Runners[i].X),
			Y: pre.Runners[i].Y + e.Frac*(end.Runners[i].Y-pre.Runners[i].Y),
		}
	}

	owner := -1
	mode := end.Mode
	vxm, vym := e.VXM, e.VYM // path/release describe their own travelled segment
	switch e.Kind {
	case PassEventControl:
		owner = e.Runner
		mode = BallHeld.String()
		vxm, vym = 0, 0 // a clean control holds the ball
	case PassEventDeflect:
		mode = BallRolling.String()
		vxm, vym = e.OutVXM, e.OutVYM // the OUTGOING (bounced) velocity
	case PassEventStop:
		mode = BallLoose.String()
		vxm, vym = 0, 0
	case PassEventOut:
		mode = BallOut.String()
		vxm, vym = 0, 0
	case PassEventPath:
		owner = end.Owner // a path trace changes nothing
	}

	return TracePoint{
		TMS:     eventExactTimeMS(e, substepMS),
		Kind:    e.Kind.String(),
		BallX:   e.BallX,
		BallY:   e.BallY,
		VXM:     vxm,
		VYM:     vym,
		Mode:    mode,
		Owner:   owner,
		Runners: states,
	}, true
}

// BuildPrototypeClips builds the three reproducible T-039 demo clips: an
// unopposed moving reception, a reachable interception, and a miscontrol with a
// loose-ball recovery. Each clip's initial release is scripted; the outcome is
// simulated, never preselected.
func BuildPrototypeClips() ([]Trace, error) {
	unopposed, err := buildUnopposedClip()
	if err != nil {
		return nil, err
	}
	interception, err := buildInterceptionClip()
	if err != nil {
		return nil, err
	}
	miscontrol, err := buildMiscontrolClip()
	if err != nil {
		return nil, err
	}
	return []Trace{unopposed, interception, miscontrol}, nil
}

// buildUnopposedClip: a moving receiver runs back to meet a released ground pass
// with no defender contesting it. The control is physical (the two bodies
// physically close), not a decided pass completion.
func buildUnopposedClip() (Trace, error) {
	const seed = 7
	sim, err := NewSim(LivePassConfig(), seed)
	if err != nil {
		return Trace{}, err
	}
	p := DefaultMotionParams()
	sim.SetRunners([]Runner{InertialRunner(0.66, 0.50, 0.30, 0.50, p)})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50,
		SpeedMPS: 14.0, Passer: -1,
	})
	return BuildTrace(ClipInput{
		Sim:         sim,
		Seed:        seed,
		Slug:        "unopposed-reception",
		Label:       "Unopposed moving reception",
		Description: "A ground pass is released toward a receiver who runs back to meet it; no defender contests the ball. Control happens where the bodies physically meet.",
		HorizonMS:   8000,
	}), nil
}

// buildInterceptionClip: an off-lane defender with bounded acceleration and zero
// reaction delay pursues the pass and cuts the lane; it reaches the ball before
// the intended (farther) receiver and controls it. Reach is physical, not a
// winner roll.
func buildInterceptionClip() (Trace, error) {
	const seed = 11
	sim, err := NewSim(LivePassConfig(), seed)
	if err != nil {
		return Trace{}, err
	}
	p := DefaultMotionParams()
	receiver := InertialRunner(0.84, 0.50, 0.95, 0.50, p)
	defender := PursuingRunner(0.66, 0.50-2.5/PitchWidthM, p, 0)
	sim.SetRunners([]Runner{receiver, defender})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.85, TargetY: 0.50,
		SpeedMPS: 11.0, Passer: -1,
	})
	return BuildTrace(ClipInput{
		Sim:         sim,
		Seed:        seed,
		Slug:        "reachable-interception",
		Label:       "Reachable interception",
		Description: "A defender starting 2.5 m off the lane pursues the released pass and reaches the ball before the intended receiver. The interception is the physical contest, not a chosen outcome.",
		HorizonMS:   8000,
	}), nil
}

// buildMiscontrolClip: a receiver with a partial control-error budget miscontrols
// (deflects) the first contact, then chases its own live loose ball and recovers
// it with a clean second contact. The seed is selected so the physics actually
// produces that deflect-then-recover sequence; the outcome is not scripted.
func buildMiscontrolClip() (Trace, error) {
	seed, err := findPartialErrorSeed()
	if err != nil {
		return Trace{}, err
	}
	sim, err := NewSim(LivePassConfig(), seed)
	if err != nil {
		return Trace{}, err
	}
	sim.SetRunners([]Runner{PursuingRunner(0.56, 0.50, DefaultMotionParams(), 0)})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.72, TargetY: 0.50,
		SpeedMPS: 12.0, ControlError: 0.5, Passer: -1,
	})
	return BuildTrace(ClipInput{
		Sim:         sim,
		Seed:        seed,
		Slug:        "miscontrol-recovery",
		Label:       "Miscontrol and loose recovery",
		Description: "A partial control budget miscontrols the first contact into a live loose ball; the same player chases it and recovers with a clean second contact. The seed is chosen for the sequence, the outcome is simulated.",
		HorizonMS:   8000,
	}), nil
}

// findPartialErrorSeed searches deterministic seeds for the first one whose
// partial control budget miscontrols the first contact and later recovers the
// loose ball. It fixes a seed so the clip is reproducible without scripting the
// outcome.
func findPartialErrorSeed() (uint64, error) {
	const horizonMS = 8000
	for seed := uint64(1); seed <= 600; seed++ {
		sim, err := NewSim(LivePassConfig(), seed)
		if err != nil {
			return 0, err
		}
		sim.SetRunners([]Runner{PursuingRunner(0.56, 0.50, DefaultMotionParams(), 0)})
		sim.ReleasePass(PassParams{
			StartX: 0.30, StartY: 0.50, TargetX: 0.72, TargetY: 0.50,
			SpeedMPS: 12.0, ControlError: 0.5, Passer: -1,
		})
		for sim.ClockMS() < horizonMS && sim.Ball().Mode != BallHeld {
			sim.Step()
			if err := sim.StepError(); err != nil {
				return 0, fmt.Errorf("seed %d step: %w", seed, err)
			}
		}
		if countEventKind(sim, PassEventDeflect) == 0 {
			continue
		}
		if ms, runner := sim.FirstContact(); ms >= 0 && runner >= 0 {
			return seed, nil
		}
	}
	return 0, fmt.Errorf("liveplay: no partial-control-error seed in 1..600 produced a deflection followed by a recovery")
}

// countEventKind counts events of one kind in the sim's live-pass log.
func countEventKind(sim *Sim, kind PassEventKind) int {
	n := 0
	for _, e := range sim.Events() {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// PrototypeFixtureTrace builds a tiny REAL exported trace for the web tests, so
// they never depend on the gitignored generated clips. It is a short scripted
// release with one clean control, produced by the same exporter. Regenerate the
// checked-in copy with:
//
//	go run ./cmd/liveplaydemo -fixture ../web/src/features/prototype/exported-fixture.json
func PrototypeFixtureTrace() (Trace, error) {
	const seed = 3
	sim, err := NewSim(LivePassConfig(), seed)
	if err != nil {
		return Trace{}, err
	}
	sim.SetRunners([]Runner{InertialRunner(0.34, 0.50, 0.60, 0.50, DefaultMotionParams())})
	sim.ReleasePass(PassParams{
		StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50,
		SpeedMPS: 8.0, Passer: -1,
	})
	return BuildTrace(ClipInput{
		Sim:         sim,
		Seed:        seed,
		Slug:        "exported-fixture",
		Label:       "Exported fixture",
		Description: "Tiny real exporter output used by the web tests.",
		HorizonMS:   2000,
	}), nil
}
