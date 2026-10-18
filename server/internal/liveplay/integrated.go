package liveplay

import (
	"errors"
	"fmt"
)

// T-055 integrated state, ordering and baseline shell (opt-in, minimal).
//
// The live-play prototype already has a complete, persisted micro-step state
// (Snapshot) and a factual event log (PassEvent). T-055 adds the SMALLEST
// opt-in shell that gives that state a versioned schema, a stable player
// identity, an explicit match phase and a factual fact journal whose ordering
// and timing come from the ACTUAL emitters — not from any later report sort.
//
// Opt-in is a single pointer, Runner.PlayerID plus Sim.Integrated (mirrored by
// Snapshot.Integrated). When it is nil the engine is BYTE-FOR-BYTE the legacy
// shape: no new JSON keys, no new RNG draw, no tie-rule change and no phase.
// EnableIntegrated() turns it on at clock 0 before any release/event and only
// with LivePass; it never switches on implicitly.
//
// Scope boundaries (deliberately NOT implemented here — each needs its own
// planner contract): T-056 facing/turn/arrival, T-057 spacing/radius/mass,
// T-058 FSM/commitment/awareness, T-059 knock-on/first-touch. No speculative
// future state is stored.

// Integrated schema versions. Each is validated INDEPENDENTLY on import and an
// unknown value is rejected, never coerced.
const (
	// IntegratedSnapshotVersion is the version of IntegratedState/Snapshot.Integrated.
	IntegratedSnapshotVersion = 1
	// IntegratedModelVersion is the version of the physical/rule model the state
	// was produced under.
	IntegratedModelVersion = 1
	// IntegratedModelVersionOriented is the model selected when the T-056 oriented
	// facing/arrival physics is active for at least one runner. Imports accept 1 or
	// 2 but require 2 whenever orientation exists.
	IntegratedModelVersionOriented = 2
	// IntegratedModelVersionBody is the model selected when the T-057 body-contact
	// layer is globally enabled. Imports accept 1/2/3; active body state requires 3.
	IntegratedModelVersionBody = 3
	// IntegratedEventSchemaVersion is the version of FactEvent.
	IntegratedEventSchemaVersion = 1
	// IntegratedRulebookVersion names the explicit, LIMITED rule profile. It is a
	// prototype-ground profile, NOT IFAB-complete.
	IntegratedRulebookVersion = "prototype-ground-v1"
)

// integratedJournalPrealloc is the explicit bounded preallocation for the fact
// journal, so a normal clip does not reallocate as facts accumulate.
const integratedJournalPrealloc = 64

// Integrated opt-in / import errors. Broken or impossible input is reported, not
// trusted; an unknown version/profile is never coerced to a known one.
var (
	// ErrIntegratedAlreadyOn: EnableIntegrated was called twice.
	ErrIntegratedAlreadyOn = errors.New("liveplay: integrated shell already enabled")
	// ErrIntegratedNotLivePass: the integrated shell only describes the live-pass model.
	ErrIntegratedNotLivePass = errors.New("liveplay: integrated shell requires LivePass")
	// ErrIntegratedNotInitial: the shell can only be enabled at clock 0 before any event.
	ErrIntegratedNotInitial = errors.New("liveplay: integrated shell must be enabled at clock 0 before any event")
	// ErrIntegratedPlayerID: integrated mode requires unique non-zero PlayerIDs.
	ErrIntegratedPlayerID = errors.New("liveplay: integrated shell requires unique non-zero player ids")
	// ErrIntegratedBody: a body/ball/target/limit scalar is not finite.
	ErrIntegratedBody = errors.New("liveplay: integrated body state is not finite")
	// ErrIntegratedIndex: a persisted owner/index is out of range or incoherent.
	ErrIntegratedIndex = errors.New("liveplay: integrated owner/index is inconsistent")
	// ErrIntegratedPhase: the persisted phase is undeclared.
	ErrIntegratedPhase = errors.New("liveplay: integrated phase is invalid")
	// ErrIntegratedVersion: a persisted version/profile is unknown.
	ErrIntegratedVersion = errors.New("liveplay: integrated version/profile is unknown")
	// ErrIntegratedJournal: the journal is internally inconsistent (sequence, kinds,
	// phase chaining or next-sequence).
	ErrIntegratedJournal = errors.New("liveplay: integrated journal is inconsistent")
	// ErrIntegratedSource: a journal fact does not correspond to a persisted source event.
	ErrIntegratedSource = errors.New("liveplay: integrated journal does not match persisted source events")
	// ErrIntegratedTouch: the persisted last touch does not reconcile with the journal.
	ErrIntegratedTouch = errors.New("liveplay: integrated last touch is inconsistent")
)

// Phase is the explicit match phase of the integrated shell. It is a deliberately
// LIMITED prototype model, not a complete referee/restart state machine (T-070+).
type Phase uint8

// Declared phases. The zero value is PhaseDead (the simplest supported baseline).
const (
	// PhaseDead: a dead ball at a restart spot (the initial baseline).
	PhaseDead Phase = iota
	// PhaseOpen: open play; the ball may be free (rolling/loose) or held.
	PhaseOpen
	// PhaseStoppedOut: live play stopped because the ball CENTRE crossed a pitch
	// boundary. This is the existing POINT-BALL prototype exit, NOT an IFAB
	// whole-ball determination; the full-ball laws and restart awards are T-070+.
	PhaseStoppedOut
)

// String returns the stable label for traces and evidence.
func (p Phase) String() string {
	switch p {
	case PhaseDead:
		return "dead"
	case PhaseOpen:
		return "open"
	case PhaseStoppedOut:
		return "stopped-out"
	default:
		return "unknown"
	}
}

// validPhase reports whether p is a declared phase.
func validPhase(p Phase) bool { return p <= PhaseStoppedOut }

// FactKind labels one integrated fact. It is the factual subset of PassEventKind:
// the periodic path trace carries no fact (it is a pure position trace).
type FactKind uint8

// Declared fact kinds.
const (
	// FactRelease: the ball left the passer's foot.
	FactRelease FactKind = iota
	// FactControl: a runner physically reached and controlled the ball.
	FactControl
	// FactDeflect: a physical touch miscontrolled/deflected the ball loose.
	FactDeflect
	// FactStop: the rolling ball came to rest, still live and collectable.
	FactStop
	// FactOut: the ball CENTRE crossed a pitch boundary (the existing point-ball
	// prototype exit, not an IFAB whole-ball law).
	FactOut
)

// String returns the stable label for traces and evidence.
func (k FactKind) String() string {
	switch k {
	case FactRelease:
		return "release"
	case FactControl:
		return "control"
	case FactDeflect:
		return "deflect"
	case FactStop:
		return "stop"
	case FactOut:
		return "out"
	default:
		return "unknown"
	}
}

// validFactKind reports whether k is a declared fact kind.
func validFactKind(k FactKind) bool { return k <= FactOut }

// factKindOf maps an authoritative PassEvent kind to its integrated fact kind.
// Path (and any future non-factual kind) is NOT a fact.
func factKindOf(k PassEventKind) (FactKind, bool) {
	switch k {
	case PassEventRelease:
		return FactRelease, true
	case PassEventControl:
		return FactControl, true
	case PassEventDeflect:
		return FactDeflect, true
	case PassEventStop:
		return FactStop, true
	case PassEventOut:
		return FactOut, true
	default:
		return 0, false
	}
}

// phaseAllowsPrior reports whether a fact of kind k may follow phase p (the
// explicit transition table). It rejects a fact emitted from an incoherent state.
func phaseAllowsPrior(p Phase, k FactKind) bool {
	switch k {
	case FactRelease:
		return p == PhaseDead || p == PhaseOpen || p == PhaseStoppedOut
	case FactControl, FactDeflect, FactStop, FactOut:
		return p == PhaseOpen
	default:
		return false
	}
}

// FactEvent is one authoritative integrated fact. Its Seq is the journal sequence
// (contiguous from 0) and SourceSeq is the matching PassEvent.Seq. The exact time
// is DERIVED from the explicit generating step (StepStartMS) and Fraction; a
// release is a between-step event, so its StepStartMS equals its ClockMS and its
// Fraction is 0.
type FactEvent struct {
	SchemaVersion int      `json:"schemaVersion"`
	Seq           int      `json:"seq"`
	SourceSeq     int      `json:"sourceSeq"`
	Kind          FactKind `json:"kind"`
	ClockMS       int      `json:"clockMs"`
	StepStartMS   int      `json:"stepStartMs"`
	Fraction      float64  `json:"fraction"`
	// PlayerID is the stable actor (a nonzero Runner.PlayerID); 0 means no actor
	// (an unknown/scripted passer or a non-participant event).
	PlayerID uint64 `json:"playerId,omitempty"`
	// X/Y and VXM/VYM are the factual ball position and INCOMING velocity.
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	VXM float64 `json:"vxM"`
	VYM float64 `json:"vyM"`
	// OutVXM/OutVYM are the ball's OUTGOING velocity after a contact (zero for a
	// clean/held control and for stop/out).
	OutVXM float64 `json:"outVxM,omitempty"`
	OutVYM float64 `json:"outVyM,omitempty"`
	// PhaseBefore/PhaseAfter are the explicit transition this fact applied.
	PhaseBefore Phase `json:"phaseBefore"`
	PhaseAfter  Phase `json:"phaseAfter"`
}

// IntegratedState is the complete opt-in integrated shell state. It is present on
// Sim and mirrored on Snapshot only while enabled; nil means the exact legacy
// shape.
type IntegratedState struct {
	SnapshotVersion    int    `json:"snapshotVersion"`
	ModelVersion       int    `json:"modelVersion"`
	EventSchemaVersion int    `json:"eventSchemaVersion"`
	RulebookVersion    string `json:"rulebookVersion"`

	// Phase is the current explicit phase; LastTouchID is the stable PlayerID of
	// the last touch (0 = unknown), NextSeq is the next journal sequence.
	Phase       Phase  `json:"phase"`
	LastTouchID uint64 `json:"lastTouchId,omitempty"`
	NextSeq     int    `json:"nextSeq"`

	// Journal is the ordered factual log (release/control/deflect/stop/out only).
	Journal []FactEvent `json:"journal,omitempty"`
}

// cloneIntegrated returns an independent deep copy (nil stays nil), so no public
// state copy or resume ever aliases a live journal.
func cloneIntegrated(st *IntegratedState) *IntegratedState {
	if st == nil {
		return nil
	}
	c := *st
	c.Journal = append([]FactEvent(nil), st.Journal...)
	return &c
}

// playerIDForRunner maps a runner index to its stable PlayerID, or 0 for a
// non-participant (-1 / out of range).
func playerIDForRunner(runners []Runner, index int) uint64 {
	if index < 0 || index >= len(runners) {
		return 0
	}
	return runners[index].PlayerID
}

// ballPhase maps the ball mode to the coherent phase at enable time.
func ballPhase(m BallMode) (Phase, bool) {
	switch m {
	case BallDead:
		return PhaseDead, true
	case BallOut:
		return PhaseStoppedOut, true
	case BallRolling, BallLoose, BallHeld:
		return PhaseOpen, true
	default:
		return 0, false
	}
}

// EnableIntegrated turns on the minimal integrated shell. It succeeds only at
// clock 0, before any release/event, and only with LivePass; it validates the
// configuration, unique stable PlayerIDs, finite bodies/ball and coherent
// owner/index state, and returns an error rather than coercing broken input.
// There is no implicit opt-in and no default switch.
//
// Raw fixture setters (SetBall/SetRunners) are NOT gameplay events and create no
// facts: after enabling, replacing the roster or ball must preserve the
// integrated invariants (unique stable IDs, coherent phase/ball mode, finite
// bodies) or the resulting snapshot will be rejected by Resume.
func (s *Sim) EnableIntegrated() error {
	if s.integrated != nil {
		return ErrIntegratedAlreadyOn
	}
	if !s.cfg.LivePass {
		return ErrIntegratedNotLivePass
	}
	if s.clock != 0 || len(s.events) != 0 || s.eventSeq != 0 {
		return ErrIntegratedNotInitial
	}
	if err := s.validateIntegratedContext(); err != nil {
		return err
	}
	// Resolve any orientation supplied through a raw roster BEFORE mutating the
	// shell, so a bad orientation is rejected transactionally. A validated
	// orientation selects model2.
	resolved := make([]*RunnerOrientation, len(s.runners))
	oriented := false
	for i := range s.runners {
		o := s.runners[i].Orientation
		if o == nil {
			continue
		}
		if s.runners[i].Motion != MotionInertial {
			return fmt.Errorf("runner %d: %w", i, ErrOrientationMotion)
		}
		if s.runners[i].PlayerID == 0 {
			return fmt.Errorf("runner %d: %w", i, ErrOrientationPlayerID)
		}
		r, err := o.resolved(s.runners[i].PlayerID)
		if err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
		resolved[i] = &r
		oriented = true
	}
	phase, ok := ballPhase(s.ball.Mode)
	if !ok {
		return fmt.Errorf("%w: ball mode %d", ErrIntegratedBody, s.ball.Mode)
	}
	model := IntegratedModelVersion
	if oriented {
		model = IntegratedModelVersionOriented
	}
	// Commit only after the full context (including orientation) validated.
	for i := range resolved {
		if resolved[i] != nil {
			s.runners[i].Orientation = resolved[i]
		}
	}
	s.integrated = &IntegratedState{
		SnapshotVersion:    IntegratedSnapshotVersion,
		ModelVersion:       model,
		EventSchemaVersion: IntegratedEventSchemaVersion,
		RulebookVersion:    IntegratedRulebookVersion,
		Phase:              phase,
		LastTouchID:        0,
		NextSeq:            0,
		Journal:            make([]FactEvent, 0, integratedJournalPrealloc),
	}
	return nil
}

// IntegratedEnabled reports whether the opt-in shell is on.
func (s *Sim) IntegratedEnabled() bool { return s.integrated != nil }

// IntegratedState returns a deep VALUE copy of the shell state (never a live
// pointer), and whether it is enabled.
func (s *Sim) IntegratedState() (IntegratedState, bool) {
	if s.integrated == nil {
		return IntegratedState{}, false
	}
	c := *s.integrated
	c.Journal = append([]FactEvent(nil), s.integrated.Journal...)
	return c, true
}

// noteIntegratedFact appends the integrated fact for a just-emitted authoritative
// event, in the ACTUAL emitter order. It is a NO-OP (and allocates nothing) when
// the shell is disabled or for a non-factual kind (path), so the legacy path is
// untouched. Phase/last-touch are updated from the transition table.
func (s *Sim) noteIntegratedFact(e PassEvent) {
	st := s.integrated
	if st == nil {
		return
	}
	fk, ok := factKindOf(e.Kind)
	if !ok {
		return
	}
	playerID := playerIDForRunner(s.runners, e.Runner)
	phaseBefore := st.Phase
	phaseAfter, lastTouch := integrateFact(phaseBefore, st.LastTouchID, fk, playerID)
	stepStart := e.ClockMS - s.cfg.SubstepMS
	if fk == FactRelease {
		stepStart = e.ClockMS
	}
	st.Journal = append(st.Journal, FactEvent{
		SchemaVersion: IntegratedEventSchemaVersion,
		Seq:           st.NextSeq,
		SourceSeq:     e.Seq,
		Kind:          fk,
		ClockMS:       e.ClockMS,
		StepStartMS:   stepStart,
		Fraction:      e.Frac,
		PlayerID:      playerID,
		X:             e.BallX,
		Y:             e.BallY,
		VXM:           e.VXM,
		VYM:           e.VYM,
		OutVXM:        e.OutVXM,
		OutVYM:        e.OutVYM,
		PhaseBefore:   phaseBefore,
		PhaseAfter:    phaseAfter,
	})
	st.NextSeq++
	st.Phase = phaseAfter
	st.LastTouchID = lastTouch
}

// integrateFact applies the explicit transition table to (phase, lastTouch) for
// one fact. A release with an unknown/scripted passer (playerID 0) starts a new
// scripted episode with last touch 0.
func integrateFact(before Phase, lastTouch uint64, k FactKind, playerID uint64) (Phase, uint64) {
	switch k {
	case FactRelease:
		return PhaseOpen, playerID
	case FactControl, FactDeflect:
		return PhaseOpen, playerID
	case FactStop:
		return PhaseOpen, lastTouch
	case FactOut:
		return PhaseStoppedOut, lastTouch
	default:
		return before, lastTouch
	}
}

// resumeIntegrated validates and deep-copies the persisted shell state against the
// resumed snapshot, or returns (nil, nil) when the shell was disabled.
func resumeIntegrated(snap Snapshot, runners []Runner) (*IntegratedState, error) {
	if snap.Integrated == nil {
		return nil, nil
	}
	st := cloneIntegrated(snap.Integrated)
	if err := validateIntegratedState(st, snap, runners); err != nil {
		return nil, fmt.Errorf("resuming integrated: %w", err)
	}
	return st, nil
}
