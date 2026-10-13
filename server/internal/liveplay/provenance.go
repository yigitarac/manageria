package liveplay

import (
	"errors"
	"fmt"
)

// Local decision provenance sidecar (T-049).
//
// This file adds an OPTIONAL, BOUNDED diagnostic record of the accepted T-044
// local pursuit + T-046 per-runner wakeup publications. It is deliberately
// SEPARATE from the authoritative state: a recorder is never serialized in
// Snapshot, is never needed for any simulation decision, and while it is absent
// the publication hook returns immediately without allocating, so the default
// JSON, trajectory, RNG and allocation profile are unchanged.
//
// The record captures the ACTUAL publication moment: the scheduler reason
// (periodic lattice / the runner's own local event / the shared event signal), an
// explicit attempt-vs-applied gate outcome (a reaction-dropped attempt is never
// presented as a decision), the observation freshness and last-seen state the
// policy read, the policy it applied and the resulting target vs the runner's own
// pre-step position. No future state, no prediction and no outcome is recorded.
//
// Export is side-effect free: BuildDecisionProvenance clones the source via a
// complete Snapshot/Resume before advancing it, and returns record VALUE copies,
// so the source simulation and the recorder can never be aliased or mutated.

// ProvenanceSchemaVersion is the version of the exported decision-provenance
// contract. Bump it whenever the JSON shape or the field semantics change.
const ProvenanceSchemaVersion = 1

// Provenance identity. The prototype has NO stable runner id, so a record's
// `runner` is the slice index and the export declares "index" explicitly.
const provenanceIdentity = "index"

// Decision-provenance errors. Broken or impossible input is reported, not trusted.
var (
	// ErrProvenanceCapacity: a recorder capacity must be strictly positive.
	ErrProvenanceCapacity = errors.New("liveplay: provenance capacity must be positive")
	// ErrProvenanceOverflow: the bounded recorder was full and dropped a record.
	ErrProvenanceOverflow = errors.New("liveplay: provenance recorder overflow")
	// ErrProvenanceSchema: an exported sidecar failed structural validation.
	ErrProvenanceSchema = errors.New("liveplay: provenance schema is invalid")
	// ErrProvenanceNilSim: the export builder needs a source simulator.
	ErrProvenanceNilSim = errors.New("liveplay: provenance export requires a source simulator")
	// ErrProvenanceHorizon: the requested horizon is negative or precedes the
	// source's current (absolute) clock, so the window would be ill-defined.
	ErrProvenanceHorizon = errors.New("liveplay: provenance horizon must be a non-negative absolute clock at/after the start")
)

// DecisionReason labels WHY a runner's local layer was scheduled, taken from the
// actual scheduler gate at the publication (never inferred after Step).
type DecisionReason string

// Decision reasons.
const (
	// ReasonPeriodic: the stable periodic lattice was due.
	ReasonPeriodic DecisionReason = "periodic"
	// ReasonLocalEvent: an opted-in runner was woken off-lattice by its OWN
	// pre-consumption local pending (T-046 per-runner event locality).
	ReasonLocalEvent DecisionReason = "local-event"
	// ReasonSharedEvent: an unflagged runner ran off-lattice on the shared T-042
	// decision-dirty signal.
	ReasonSharedEvent DecisionReason = "shared-event"
)

func validReason(r DecisionReason) bool {
	switch r {
	case ReasonPeriodic, ReasonLocalEvent, ReasonSharedEvent:
		return true
	default:
		return false
	}
}

// DecisionPolicy labels the accepted T-044 local policy applied at the
// publication. There is deliberately no prediction policy (T-047/T-048 were
// rejected; no runtime prediction is authorized).
type DecisionPolicy string

// Decision policies.
const (
	// PolicyChase: Fresh Rolling/Loose ⇒ target = observed position.
	PolicyChase DecisionPolicy = "chase"
	// PolicyUnknownBrake: Unknown ⇒ target = own position.
	PolicyUnknownBrake DecisionPolicy = "unknown-brake"
	// PolicyStaleRetain: Stale (or an undeclared observed mode, failing closed)
	// ⇒ retain the prior destination.
	PolicyStaleRetain DecisionPolicy = "stale-retain"
	// PolicyNonFreeBrake: Fresh Held/Dead/Out ⇒ target = own position.
	PolicyNonFreeBrake DecisionPolicy = "nonfree-brake"
	// PolicyNone: no policy applied (reaction-blocked / invalid enabling).
	PolicyNone DecisionPolicy = "none"
)

func validPolicy(p DecisionPolicy) bool {
	switch p {
	case PolicyChase, PolicyUnknownBrake, PolicyStaleRetain, PolicyNonFreeBrake, PolicyNone:
		return true
	default:
		return false
	}
}

// Gate outcomes for an attempt.
const (
	// GateApplied: the policy actually ran.
	GateApplied = "applied"
	// GateReactionBlocked: the attempt was selected but the reaction gate had
	// not fallen due, so the sample AND the policy were dropped.
	GateReactionBlocked = "reaction-blocked"
	// GateInvalidEnabling: the attempt carried an invalid enabling (non-inertial
	// or a missing/invalid sensing record); no policy ran.
	GateInvalidEnabling = "invalid-enabling"
)

func validGateOutcome(g string) bool {
	switch g {
	case GateApplied, GateReactionBlocked, GateInvalidEnabling:
		return true
	default:
		return false
	}
}

// provenanceObsLimit is the declared envelope for the recorded target/own
// position. Every accepted policy derives its target from an in-envelope
// observation, the runner's own in-envelope position or a previously in-envelope
// intent; a raw out-of-envelope intent is a fixture misuse and is rejected.
const provenanceObsLimit = 1.25

// ProvenanceObservation is a VALUE copy of the runner's local sighting at the
// publication. It is deliberately a value struct (never a pointer to the live
// BallObservation) so an exported record cannot alias simulation state.
type ProvenanceObservation struct {
	Seen    bool    `json:"seen"`
	ClockMS int     `json:"clockMs"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	VXM     float64 `json:"vxM"`
	VYM     float64 `json:"vyM"`
	Mode    string  `json:"mode"`
	Owner   int     `json:"owner"`
}

// DecisionRecord is one provenance moment for one runner at one real pre-step
// publication. See the schema contract in the T-049 evidence note.
type DecisionRecord struct {
	ClockMS      int                   `json:"clockMs"`
	Runner       int                   `json:"runner"`
	Reason       DecisionReason        `json:"reason"`
	Simultaneous bool                  `json:"simultaneous"`
	Applied      bool                  `json:"applied"`
	GateOutcome  string                `json:"gateOutcome"`
	Policy       DecisionPolicy        `json:"policy"`
	Freshness    string                `json:"freshness"`
	Observation  ProvenanceObservation `json:"observation"`
	TargetX      float64               `json:"targetX"`
	TargetY      float64               `json:"targetY"`
	OwnX         float64               `json:"ownX"`
	OwnY         float64               `json:"ownY"`
}

// DecisionProvenance is the complete versioned sidecar export for one window.
type DecisionProvenance struct {
	SchemaVersion int              `json:"schemaVersion"`
	TimeUnit      string           `json:"timeUnit"`
	SubstepMS     int              `json:"substepMs"`
	Identity      string           `json:"identity"`
	RunnerCount   int              `json:"runnerCount"`
	StartMS       int              `json:"startMs"`
	EndMS         int              `json:"endMs"`
	Capacity      int              `json:"capacity"`
	RecordCount   int              `json:"recordCount"`
	Overflowed    bool             `json:"overflowed"`
	Records       []DecisionRecord `json:"records"`
}

// ProvenanceRecorder is a bounded, optional diagnostic sink. The zero value is
// NOT usable; construct with NewProvenanceRecorder. It performs no I/O and never
// calls back into the simulation.
type ProvenanceRecorder struct {
	capacity   int
	records    []DecisionRecord
	overflowed bool
}

// NewProvenanceRecorder builds a recorder with a bounded capacity. A non-positive
// capacity is rejected.
func NewProvenanceRecorder(capacity int) (*ProvenanceRecorder, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrProvenanceCapacity, capacity)
	}
	return &ProvenanceRecorder{capacity: capacity, records: make([]DecisionRecord, 0, capacity)}, nil
}

// SetProvenanceRecorder attaches (or clears, with nil) the optional provenance
// recorder. It is never serialized in Snapshot and is not needed for any
// simulation decision.
func (s *Sim) SetProvenanceRecorder(rec *ProvenanceRecorder) { s.provenance = rec }

// ProvenanceRecorder returns the attached recorder (nil when disabled). It is
// evidence-only.
func (s *Sim) ProvenanceRecorder() *ProvenanceRecorder { return s.provenance }

// Len returns the number of captured records.
func (r *ProvenanceRecorder) Len() int { return len(r.records) }

// Overflowed reports whether the bounded recorder dropped a record.
func (r *ProvenanceRecorder) Overflowed() bool { return r.overflowed }

// Err returns ErrProvenanceOverflow when the recorder overflowed, else nil.
func (r *ProvenanceRecorder) Err() error {
	if r.overflowed {
		return ErrProvenanceOverflow
	}
	return nil
}

// Records returns a VALUE copy of the captured records. The returned slice is an
// independent backing array, so mutating it cannot affect the recorder.
func (r *ProvenanceRecorder) Records() []DecisionRecord {
	return append([]DecisionRecord(nil), r.records...)
}

// append adds one record if capacity remains, otherwise it flags overflow and
// drops the record. It never grows past the capacity.
func (r *ProvenanceRecorder) append(rec DecisionRecord) {
	if len(r.records) >= r.capacity {
		r.overflowed = true
		return
	}
	r.records = append(r.records, rec)
}

// provenanceModeLabel maps a stored observation to a stable label. A never-seen
// record has no meaningful mode; an undeclared mode (raw fixture) fails closed to
// "unknown".
func provenanceModeLabel(o *BallObservation) string {
	if o == nil || !o.Seen {
		return "unknown"
	}
	if !validBallMode(o.Mode) {
		return "unknown"
	}
	return o.Mode.String()
}

// localDecision is the single source of truth for the accepted T-044 local
// policy, shared by the runtime publisher and the provenance recorder so the two
// can never diverge.
//
// It returns the policy that would be applied, whether it applies at all
// (reaction gate + validity satisfied), and the explicit gate outcome when it
// does not. It reads only the runner's own record, own position and the reaction
// deadline.
func localDecision(r Runner, stepStart int) (DecisionPolicy, bool, string) {
	if !r.LocalPursuit {
		return PolicyNone, false, GateInvalidEnabling
	}
	if r.Motion != MotionInertial {
		return PolicyNone, false, GateInvalidEnabling
	}
	o := r.Perception
	if !o.SensingEnabled() {
		return PolicyNone, false, GateInvalidEnabling
	}
	if stepStart < r.ReactAtMS {
		return PolicyNone, false, GateReactionBlocked
	}
	switch o.Freshness(stepStart) {
	case ObservationUnknown:
		return PolicyUnknownBrake, true, GateApplied
	case ObservationStale:
		return PolicyStaleRetain, true, GateApplied
	default: // ObservationFresh
		switch o.Mode {
		case BallRolling, BallLoose:
			return PolicyChase, true, GateApplied
		case BallHeld, BallDead, BallOut:
			return PolicyNonFreeBrake, true, GateApplied
		default:
			// Undeclared observed mode: fail closed, retain the prior destination.
			return PolicyStaleRetain, true, GateApplied
		}
	}
}

// recordLocalProvenance is the optional publication hook. It runs at the END of
// publishLocalSamples (after sampling and the local policy, before integration)
// and captures one record per gate-selected local-pursuit runner. It is a NO-OP
// while no recorder is attached and allocates nothing in that case.
//
// `event[i]` is the runner's event signal captured by the CALLER BEFORE any
// pending was consumed (the runner's own pre-consumption WakePending for an opted
// runner, the shared dirty flag for an unflagged runner); `latticeDue` is the
// actual periodic gate. The reason is therefore taken from the real gating, never
// inferred after Step.
func (s *Sim) recordLocalProvenance(stepStart int, gate, event []bool, latticeDue bool) {
	if s.provenance == nil {
		return
	}
	for i := range s.runners {
		if gate != nil && !gate[i] {
			continue
		}
		if s.provenance.overflowed {
			return // full: recording is a diagnostic; never grow unboundedly
		}
		r := &s.runners[i]
		if !r.LocalPursuit {
			continue // no local decision layer ran
		}
		reason := ReasonPeriodic
		if !latticeDue {
			if r.LocalWakeup {
				reason = ReasonLocalEvent
			} else {
				reason = ReasonSharedEvent
			}
		}
		simult := latticeDue && event != nil && i < len(event) && event[i]
		s.provenance.append(s.buildDecisionRecord(*r, i, stepStart, reason, simult))
	}
}

// buildDecisionRecord snapshots one runner's decision at the publication as a
// VALUE-only record (no pointers).
func (s *Sim) buildDecisionRecord(r Runner, index, stepStart int, reason DecisionReason, simult bool) DecisionRecord {
	policy, applied, gateOutcome := localDecision(r, stepStart)
	obs := ProvenanceObservation{Owner: -1, Mode: "unknown"}
	if o := r.Perception; o != nil {
		obs.Seen = o.Seen
		obs.ClockMS = o.ClockMS
		obs.X, obs.Y = o.X, o.Y
		obs.VXM, obs.VYM = o.VXM, o.VYM
		obs.Mode = provenanceModeLabel(o)
		if o.Seen && o.Mode == BallHeld {
			obs.Owner = o.Owner
		} else {
			obs.Owner = -1
		}
	}
	freshness := ObservationUnknown.String()
	if r.Perception != nil {
		freshness = r.Perception.Freshness(stepStart).String()
	}
	return DecisionRecord{
		ClockMS:      stepStart,
		Runner:       index,
		Reason:       reason,
		Simultaneous: simult,
		Applied:      applied,
		GateOutcome:  gateOutcome,
		Policy:       policy,
		Freshness:    freshness,
		Observation:  obs,
		TargetX:      r.IntentX,
		TargetY:      r.IntentY,
		OwnX:         r.X,
		OwnY:         r.Y,
	}
}

// ProvenanceInput is one sidecar export request. The source Sim is cloned, never
// advanced directly.
//
// HorizonMS is an ABSOLUTE simulation clock, not a duration: the clone is advanced
// until its clock reaches HorizonMS (or the ball is held). It must be
// non-negative and at/after the source's current clock; a negative or pre-start
// horizon is rejected explicitly (ErrProvenanceHorizon). The horizon does NOT have
// to land on the substep grid (the clone always stops on a substep boundary).
type ProvenanceInput struct {
	Sim       *Sim
	Capacity  int
	HorizonMS int
}

// BuildDecisionProvenance builds the sidecar export for one window without
// mutating the source simulation. It clones the source via a complete
// Snapshot/Resume, attaches a fresh bounded recorder to the clone, advances the
// clone to the horizon (or a held ball) and returns record VALUE copies.
//
// The returned records do not alias the recorder or the source. A non-nil error
// is either a construction error or ErrProvenanceOverflow (the export is still
// populated and valid, with Overflowed set).
func BuildDecisionProvenance(in ProvenanceInput) (DecisionProvenance, error) {
	if in.Sim == nil {
		return DecisionProvenance{}, ErrProvenanceNilSim
	}
	// HorizonMS is an ABSOLUTE clock: reject a negative or pre-start horizon
	// explicitly rather than silently exporting an ill-defined empty window.
	start := in.Sim.ClockMS()
	if in.HorizonMS < 0 {
		return DecisionProvenance{}, fmt.Errorf("%w: horizon %d", ErrProvenanceHorizon, in.HorizonMS)
	}
	if in.HorizonMS < start {
		return DecisionProvenance{}, fmt.Errorf("%w: horizon %d before start %d", ErrProvenanceHorizon, in.HorizonMS, start)
	}
	rec, err := NewProvenanceRecorder(in.Capacity)
	if err != nil {
		return DecisionProvenance{}, err
	}
	// Clone the source so the caller's sim is never advanced or mutated.
	clone, err := Resume(in.Sim.Snapshot())
	if err != nil {
		return DecisionProvenance{}, fmt.Errorf("cloning source: %w", err)
	}
	clone.SetProvenanceRecorder(rec)
	for clone.ClockMS() < in.HorizonMS && clone.Ball().Mode != BallHeld {
		clone.Step()
	}
	return DecisionProvenance{
		SchemaVersion: ProvenanceSchemaVersion,
		TimeUnit:      "ms",
		SubstepMS:     clone.cfg.SubstepMS,
		Identity:      provenanceIdentity,
		RunnerCount:   len(clone.runners),
		StartMS:       start,
		EndMS:         clone.ClockMS(),
		Capacity:      in.Capacity,
		RecordCount:   rec.Len(),
		Overflowed:    rec.Overflowed(),
		Records:       rec.Records(),
	}, rec.Err()
}

// Validate checks the exported sidecar against the declared contract. It rejects
// structurally impossible or inconsistent data (unknown labels, out-of-range
// indices/clocks, inconsistent observation/freshness, duplicate records).
func (p DecisionProvenance) Validate() error {
	if p.SchemaVersion != ProvenanceSchemaVersion {
		return fmt.Errorf("%w: schemaVersion %d", ErrProvenanceSchema, p.SchemaVersion)
	}
	if p.TimeUnit != "ms" {
		return fmt.Errorf("%w: timeUnit %q", ErrProvenanceSchema, p.TimeUnit)
	}
	if p.Identity != provenanceIdentity {
		return fmt.Errorf("%w: identity %q", ErrProvenanceSchema, p.Identity)
	}
	if p.SubstepMS <= 0 {
		return fmt.Errorf("%w: substepMs %d", ErrProvenanceSchema, p.SubstepMS)
	}
	if p.RunnerCount < 1 {
		return fmt.Errorf("%w: runnerCount %d", ErrProvenanceSchema, p.RunnerCount)
	}
	if p.Capacity < 1 {
		return fmt.Errorf("%w: capacity %d", ErrProvenanceSchema, p.Capacity)
	}
	if p.RecordCount > p.Capacity {
		return fmt.Errorf("%w: recordCount %d exceeds capacity %d", ErrProvenanceSchema, p.RecordCount, p.Capacity)
	}
	if p.StartMS < 0 || p.EndMS < p.StartMS {
		return fmt.Errorf("%w: window [%d,%d]", ErrProvenanceSchema, p.StartMS, p.EndMS)
	}
	if p.StartMS%p.SubstepMS != 0 || p.EndMS%p.SubstepMS != 0 {
		return fmt.Errorf("%w: window clocks not on the substep grid", ErrProvenanceSchema)
	}
	if p.RecordCount != len(p.Records) {
		return fmt.Errorf("%w: recordCount %d != %d records", ErrProvenanceSchema, p.RecordCount, len(p.Records))
	}
	if p.Overflowed && p.RecordCount < p.Capacity {
		return fmt.Errorf("%w: overflowed with only %d/%d records", ErrProvenanceSchema, p.RecordCount, p.Capacity)
	}
	seen := make(map[[2]int]bool, len(p.Records))
	for i, rec := range p.Records {
		if err := rec.validate(p); err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		key := [2]int{rec.ClockMS, rec.Runner}
		if seen[key] {
			return fmt.Errorf("%w: duplicate record at clock %d runner %d", ErrProvenanceSchema, rec.ClockMS, rec.Runner)
		}
		seen[key] = true
	}
	return nil
}

// validate checks one record against the export header.
func (rec DecisionRecord) validate(p DecisionProvenance) error {
	if !validReason(rec.Reason) {
		return fmt.Errorf("%w: reason %q", ErrProvenanceSchema, rec.Reason)
	}
	if !validPolicy(rec.Policy) {
		return fmt.Errorf("%w: policy %q", ErrProvenanceSchema, rec.Policy)
	}
	if !validGateOutcome(rec.GateOutcome) {
		return fmt.Errorf("%w: gateOutcome %q", ErrProvenanceSchema, rec.GateOutcome)
	}
	if rec.GateOutcome == GateApplied != rec.Applied {
		return fmt.Errorf("%w: applied %v vs gateOutcome %q", ErrProvenanceSchema, rec.Applied, rec.GateOutcome)
	}
	if (rec.Policy == PolicyNone) == rec.Applied {
		return fmt.Errorf("%w: policy %q vs applied %v", ErrProvenanceSchema, rec.Policy, rec.Applied)
	}
	if rec.Runner < 0 || rec.Runner >= p.RunnerCount {
		return fmt.Errorf("%w: runner %d outside [0,%d)", ErrProvenanceSchema, rec.Runner, p.RunnerCount)
	}
	if rec.ClockMS%p.SubstepMS != 0 || rec.ClockMS < p.StartMS || rec.ClockMS > p.EndMS {
		return fmt.Errorf("%w: clock %d outside [%d,%d] on grid", ErrProvenanceSchema, rec.ClockMS, p.StartMS, p.EndMS)
	}
	// `simultaneous` means the periodic lattice was due AND an event signal was
	// present; the actual scheduler can only produce that on a periodic reason.
	if rec.Simultaneous && rec.Reason != ReasonPeriodic {
		return fmt.Errorf("%w: simultaneous with reason %q", ErrProvenanceSchema, rec.Reason)
	}
	// Fixed ordered envelope checks (deterministic first error).
	if err := provenanceEnvelope("targetX", rec.TargetX); err != nil {
		return err
	}
	if err := provenanceEnvelope("targetY", rec.TargetY); err != nil {
		return err
	}
	if err := provenanceEnvelope("ownX", rec.OwnX); err != nil {
		return err
	}
	if err := provenanceEnvelope("ownY", rec.OwnY); err != nil {
		return err
	}
	return rec.Observation.validate(rec.ClockMS, rec.Freshness, p.RunnerCount)
}

// provenanceEnvelope rejects a non-finite or out-of-world scalar (target/own and
// seen observation coordinates share the same accepted [-limit,limit] envelope).
func provenanceEnvelope(name string, v float64) error {
	if !isFinite(v) || v < -provenanceObsLimit || v > provenanceObsLimit {
		return fmt.Errorf("%w: %s=%v outside [%v,%v]", ErrProvenanceSchema, name, v, -provenanceObsLimit, provenanceObsLimit)
	}
	return nil
}

// validate checks one observation against the record clock, freshness and the
// runner count. The structural finite/clock checks apply to EVERY observation,
// seen or not: an unseen record still serializes scalars (it must never carry
// NaN/Inf or a clock outside [0,recordClockMS]), exactly as
// BallObservation.validate checks finite coordinates BEFORE its Seen branch. A
// SEEN observation must additionally lie inside the same accepted world envelope
// as target/own, carry a declared mode, and satisfy the mode/owner rules.
func (o ProvenanceObservation) validate(recordClockMS int, freshness string, runnerCount int) error {
	if !isFinite(o.X) || !isFinite(o.Y) || !isFinite(o.VXM) || !isFinite(o.VYM) {
		return fmt.Errorf("%w: observation coordinates not finite", ErrProvenanceSchema)
	}
	if o.ClockMS < 0 || o.ClockMS > recordClockMS {
		return fmt.Errorf("%w: observation clock %d outside [0,%d]", ErrProvenanceSchema, o.ClockMS, recordClockMS)
	}
	switch freshness {
	case "unknown":
		if o.Seen {
			return fmt.Errorf("%w: freshness unknown but seen", ErrProvenanceSchema)
		}
	case "fresh":
		if !o.Seen || o.ClockMS != recordClockMS {
			return fmt.Errorf("%w: freshness fresh requires seen at record clock", ErrProvenanceSchema)
		}
	case "stale":
		if !o.Seen || o.ClockMS >= recordClockMS {
			return fmt.Errorf("%w: freshness stale requires seen before record clock", ErrProvenanceSchema)
		}
	default:
		return fmt.Errorf("%w: freshness %q", ErrProvenanceSchema, freshness)
	}
	if !o.Seen {
		if o.Owner != -1 {
			return fmt.Errorf("%w: unseen observation owner %d", ErrProvenanceSchema, o.Owner)
		}
		// A never-seen record has no meaningful mode: it must be the canonical
		// "unknown", never a stale retained label.
		if o.Mode != "unknown" {
			return fmt.Errorf("%w: unseen observation mode %q, want unknown", ErrProvenanceSchema, o.Mode)
		}
		return nil
	}
	// A seen observation shares the accepted world envelope with target/own (a
	// chase target IS the seen observation).
	if o.X < -provenanceObsLimit || o.X > provenanceObsLimit || o.Y < -provenanceObsLimit || o.Y > provenanceObsLimit {
		return fmt.Errorf("%w: observation (%v,%v) outside [%v,%v]", ErrProvenanceSchema, o.X, o.Y, -provenanceObsLimit, provenanceObsLimit)
	}
	switch o.Mode {
	case "held":
		if o.Owner < 0 || o.Owner >= runnerCount {
			return fmt.Errorf("%w: held owner %d outside [0,%d)", ErrProvenanceSchema, o.Owner, runnerCount)
		}
	case "rolling", "loose", "out", "dead":
		if o.Owner != -1 {
			return fmt.Errorf("%w: unheld owner %d, want -1", ErrProvenanceSchema, o.Owner)
		}
	default:
		return fmt.Errorf("%w: observation mode %q", ErrProvenanceSchema, o.Mode)
	}
	return nil
}
