package liveplay

import (
	"errors"
	"fmt"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// T-057 soft separation and swept body contact (opt-in, isolated).
//
// This file adds an explicit optional body-contact layer to the integrated Sim.
// When Sim.body is nil the engine is byte-for-byte the accepted T056 arithmetic.
// When enabled (model3) every participating inertial runner carries a resolved
// RunnerBody, and each step computes stable pre-state pair separation, a swept
// chronological hard-disc contact solve with zero restitution and bounded residual
// correction, and a checked transactional step that never silently discards time.
//
// Scope boundaries: NO agents, dribbling, tackling, aerial, keeper or team AI; no
// ball->body impulses; no renderer/API/CLI migration.

// Body numerical assumptions (named, fixed). They are prototype model choices,
// not measured truths and not outcome-fitting knobs.
const (
	// BodyMaxProjectionM is the per-body TOTAL positional repair budget per step.
	BodyMaxProjectionM = 0.02
	// BodyResidualTolM is the accepted residual penetration after repair.
	BodyResidualTolM = 1e-6
	// BodyClosingTolMPS is the closing-velocity tolerance treating a contact as
	// effectively resolved (separating/tangent).
	BodyClosingTolMPS = 1e-10
	// BodyMaxParticipants bounds a body-enabled roster so the work claim is real.
	BodyMaxParticipants = 22
	// BodyDefaultRadiusM / BodyDefaultMassKG are the explicit prototype defaults.
	BodyDefaultRadiusM = 0.30
	BodyDefaultMassKG  = 75.0
	// BodyDefaultSpacingM / BodyDefaultSoftAccelMPS2 / BodyDefaultMaxResolutions
	// are the explicit default configuration.
	BodyDefaultSpacingM       = 1.8
	BodyDefaultSoftAccelMPS2  = 2.0
	BodyDefaultMaxResolutions = 65536

	// Named numerical tolerances with distinct units (not interchangeable):
	// bodyEpsM is the coincidence/zero-length geometry tolerance (metres);
	// bodyContactTolM is a ROUNDOFF geometry tolerance for accepting that a swept
	// pair has reached contact (metres); bodyRootTolS accepts a swept entry at/just
	// past the endpoint (seconds). These must NOT merge genuinely distinct positive
	// gaps or distinct physical times.
	bodyEpsM         = 1e-12
	bodyContactTolM  = 1e-12
	bodyRootTolS     = 1e-12
	bodyFiniteFactor = 1e300
)

// Body errors. Broken or impossible input is reported, never coerced.
var (
	// ErrBodyNoIntegrated: body contacts require the integrated shell.
	ErrBodyNoIntegrated = errors.New("liveplay: body contacts require the integrated shell")
	// ErrBodyNotInitial: body contacts can only be enabled at clock 0 before events.
	ErrBodyNotInitial = errors.New("liveplay: body contacts must be enabled at clock 0 before any event")
	// ErrBodyAlreadyEnabled: EnableBodyContacts called twice.
	ErrBodyAlreadyEnabled = errors.New("liveplay: body contacts already enabled")
	// ErrBodyMotion: body contacts require the inertial motion model.
	ErrBodyMotion = errors.New("liveplay: body contacts require inertial motion")
	// ErrBodyMissing: a body-enabled runner is missing its resolved body parameters.
	ErrBodyMissing = errors.New("liveplay: body-enabled runner requires body parameters")
	// ErrBodyParams: a body radius/mass (or config field) is non-finite or non-positive.
	ErrBodyParams = errors.New("liveplay: body parameter is non-finite or out of range")
	// ErrBodyRoster: a body-enabled roster is empty or exceeds BodyMaxParticipants.
	ErrBodyRoster = errors.New("liveplay: body roster size out of range")
	// ErrBodyOverlap: an initial penetration exceeds the repair budget.
	ErrBodyOverlap = errors.New("liveplay: initial body penetration exceeds the repair budget")
	// ErrBodyProjection: residual repair could not converge within its budget.
	ErrBodyProjection = errors.New("liveplay: body residual repair exceeded its budget")
	// ErrBodyNotEnabled: checked stepping requires body contacts.
	ErrBodyNotEnabled = errors.New("liveplay: body contacts are not enabled")
	// ErrBodySchema: a persisted body state/impact/fault failed validation.
	ErrBodySchema = errors.New("liveplay: body state schema is invalid")
	// ErrBodyIncomplete: body parameters were persisted without global body state.
	ErrBodyIncomplete = errors.New("liveplay: persisted body parameters without body state")
	// ErrBodyVersion: body state used with a non-body model version.
	ErrBodyVersion = errors.New("liveplay: body state requires the body model version")
)

// BodyFaultCode is a stable machine-readable fault reason.
type BodyFaultCode string

// Declared fault codes.
const (
	// BodyFaultWorkLimit: the bounded resolution budget was exhausted.
	BodyFaultWorkLimit BodyFaultCode = "work-limit"
	// BodyFaultProjectionLimit: residual repair exceeded the projection budget.
	BodyFaultProjectionLimit BodyFaultCode = "projection-limit"
	// BodyFaultNumerical: a non-finite or unrepresentable numerical state.
	BodyFaultNumerical BodyFaultCode = "numerical"
)

func validBodyFaultCode(c BodyFaultCode) bool {
	switch c {
	case BodyFaultWorkLimit, BodyFaultProjectionLimit, BodyFaultNumerical:
		return true
	default:
		return false
	}
}

// BodyStepFault is the persisted sticky structured fault of a failed checked step.
type BodyStepFault struct {
	Code        BodyFaultCode `json:"code"`
	StepStartMS int           `json:"stepStartMs"`
	Fraction    float64       `json:"fraction"`
}

// bodyFaultError wraps a fault with a stable sentinel for errors.Is callers.
func bodyFaultError(f *BodyStepFault) error {
	if f == nil {
		return nil
	}
	return fmt.Errorf("liveplay: body step fault %s at %d ms fraction %.6f: %w", f.Code, f.StepStartMS, f.Fraction, ErrBodySchema)
}

// RunnerBody is the resolved physical body of an inertial runner.
type RunnerBody struct {
	RadiusM float64 `json:"radiusM"`
	MassKG  float64 `json:"massKg"`
}

// DefaultRunnerBody returns the explicit prototype default body.
func DefaultRunnerBody() RunnerBody {
	return RunnerBody{RadiusM: BodyDefaultRadiusM, MassKG: BodyDefaultMassKG}
}

// NewRunnerBody builds a resolved body, rejecting non-finite/non-positive input.
func NewRunnerBody(radiusM, massKG float64) (RunnerBody, error) {
	b := RunnerBody{RadiusM: radiusM, MassKG: massKG}
	if err := b.validate(); err != nil {
		return RunnerBody{}, err
	}
	return b, nil
}

func (b RunnerBody) validate() error {
	if !isFinite(b.RadiusM) || b.RadiusM <= 0 {
		return fmt.Errorf("%w: radiusM %v", ErrBodyParams, b.RadiusM)
	}
	if !isFinite(b.MassKG) || b.MassKG <= 0 {
		return fmt.Errorf("%w: massKg %v", ErrBodyParams, b.MassKG)
	}
	// Reject a mass whose inverse overflows (a tiny subnormal would inject Inf
	// into every impulse computation).
	if inv := 1.0 / b.MassKG; !isFinite(inv) || inv > bodyFiniteFactor {
		return fmt.Errorf("%w: massKg %v has a non-finite inverse", ErrBodyParams, b.MassKG)
	}
	return nil
}

func cloneBody(b *RunnerBody) *RunnerBody {
	if b == nil {
		return nil
	}
	c := *b
	return &c
}

// BodyContactConfig is the resolved body-contact configuration.
type BodyContactConfig struct {
	SpacingM       float64 `json:"spacingM"`
	SoftAccelMPS2  float64 `json:"softAccelMps2"`
	MaxResolutions int     `json:"maxResolutions"`
}

// DefaultBodyContactConfig returns the explicit prototype default configuration.
func DefaultBodyContactConfig() BodyContactConfig {
	return BodyContactConfig{SpacingM: BodyDefaultSpacingM, SoftAccelMPS2: BodyDefaultSoftAccelMPS2, MaxResolutions: BodyDefaultMaxResolutions}
}

func (c BodyContactConfig) validate() error {
	if !isFinite(c.SpacingM) || c.SpacingM <= 0 {
		return fmt.Errorf("%w: spacingM %v", ErrBodyParams, c.SpacingM)
	}
	if !isFinite(c.SoftAccelMPS2) || c.SoftAccelMPS2 < 0 {
		return fmt.Errorf("%w: softAccelMps2 %v", ErrBodyParams, c.SoftAccelMPS2)
	}
	if c.MaxResolutions < 1 || c.MaxResolutions > BodyDefaultMaxResolutions {
		return fmt.Errorf("%w: maxResolutions %d", ErrBodyParams, c.MaxResolutions)
	}
	return nil
}

// BodyImpact is one factual hard-body impulse. Normal points from PlayerID2 to 1.
type BodyImpact struct {
	SchemaVersion int     `json:"schemaVersion"`
	Seq           int     `json:"seq"`
	StepStartMS   int     `json:"stepStartMs"`
	ClockMS       int     `json:"clockMs"`
	Fraction      float64 `json:"fraction"`
	PlayerID1     uint64  `json:"playerId1"`
	PlayerID2     uint64  `json:"playerId2"`
	NormalX       float64 `json:"normalX"`
	NormalY       float64 `json:"normalY"`
	Impulse       float64 `json:"impulse"`
	X1            float64 `json:"x1"`
	Y1            float64 `json:"y1"`
	X2            float64 `json:"x2"`
	Y2            float64 `json:"y2"`
	InVX1         float64 `json:"inVx1"`
	InVY1         float64 `json:"inVy1"`
	InVX2         float64 `json:"inVx2"`
	InVY2         float64 `json:"inVy2"`
	OutVX1        float64 `json:"outVx1"`
	OutVY1        float64 `json:"outVy1"`
	OutVX2        float64 `json:"outVx2"`
	OutVY2        float64 `json:"outVy2"`
}

// BodyContactSchemaVersion is the body-state/impact schema version.
const BodyContactSchemaVersion = 1

// BodyContactState is the persisted body-contact state.
type BodyContactState struct {
	SchemaVersion int               `json:"schemaVersion"`
	Config        BodyContactConfig `json:"config"`
	NextSeq       int               `json:"nextSeq"`
	Impacts       []BodyImpact      `json:"impacts,omitempty"`
	Fault         *BodyStepFault    `json:"fault,omitempty"`
}

func cloneBodyState(b *BodyContactState) *BodyContactState {
	if b == nil {
		return nil
	}
	c := *b
	c.Impacts = append([]BodyImpact(nil), b.Impacts...)
	if b.Fault != nil {
		f := *b.Fault
		c.Fault = &f
	}
	return &c
}

// EnableBodyContacts activates the optional body-contact layer. It requires the
// integrated shell at clock 0 before any event, validates the whole roster/config
// and the hypothetical initial-overlap repair transactionally, and selects the
// named body model3. It never draws RNG or mutates state on failure.
func (s *Sim) EnableBodyContacts(config BodyContactConfig) error {
	if s.integrated == nil {
		return ErrBodyNoIntegrated
	}
	if s.clock != 0 || len(s.events) != 0 {
		return ErrBodyNotInitial
	}
	if s.body != nil {
		return ErrBodyAlreadyEnabled
	}
	if err := config.validate(); err != nil {
		return err
	}
	// Revalidate the WHOLE current integrated context after any raw setters (a
	// prior EnableIntegrated alone does not license a later inconsistent roster).
	if err := s.validateIntegratedContext(); err != nil {
		return err
	}
	// Full integrated-state revalidation (schema/version/phase/ball/events/
	// orientation) rather than only the phase enum. Known model 1/2/3 accepted;
	// model3 without body state is accepted and preserved.
	snap := s.Snapshot()
	if err := validateIntegratedState(snap.Integrated, snap, s.runners); err != nil {
		return err
	}
	// Stage validated, side-resolved orientation pointers transactionally (an
	// unresolved raw TurnSide0 is resolved here, not silently accepted unchanged).
	resolvedOri := make([]*RunnerOrientation, len(s.runners))
	for i := range s.runners {
		if o := s.runners[i].Orientation; o != nil {
			r, err := o.resolved(s.runners[i].PlayerID)
			if err != nil {
				return fmt.Errorf("runner %d: %w", i, err)
			}
			resolvedOri[i] = &r
		}
	}
	n := len(s.runners)
	if n < 1 || n > BodyMaxParticipants {
		return fmt.Errorf("%w: %d", ErrBodyRoster, n)
	}
	if _, err := runnerIDSet(s.runners); err != nil {
		return err
	}
	for i := range s.runners {
		r := s.runners[i]
		if r.Motion != MotionInertial {
			return fmt.Errorf("runner %d: %w", i, ErrBodyMotion)
		}
		if r.Body == nil {
			return fmt.Errorf("runner %d: %w", i, ErrBodyMissing)
		}
		if err := r.Body.validate(); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
	}
	if err := s.bodyEnableFeasible(); err != nil {
		return err
	}
	for i := range resolvedOri {
		if resolvedOri[i] != nil {
			s.runners[i].Orientation = resolvedOri[i]
		}
	}
	s.integrated.ModelVersion = IntegratedModelVersionBody
	s.body = &BodyContactState{
		SchemaVersion: BodyContactSchemaVersion,
		Config:        config,
		NextSeq:       0,
	}
	return nil
}

// BodyEnabled reports whether the optional body layer is active.
func (s *Sim) BodyEnabled() bool { return s.body != nil }

// StepChecked advances one transactional body-enabled substep. When body contacts
// are not enabled it returns ErrBodyNotEnabled. A numerical failure stores the
// sticky fault and leaves all prior state/RNG/history/diagnostics exact.
func (s *Sim) StepChecked() error { return s.stepChecked(true) }

// StepQuietChecked is StepChecked without recording a sample.
func (s *Sim) StepQuietChecked() error { return s.stepChecked(false) }

// StepError returns the sticky body fault (nil when healthy or disabled).
func (s *Sim) StepError() error {
	if s.body == nil || s.body.Fault == nil {
		return nil
	}
	return bodyFaultError(s.body.Fault)
}

// RunChecked advances checked steps to endMS (exclusive) or until the ball is held,
// returning the first fault.
func (s *Sim) RunChecked(endMS int) error {
	if s.body == nil {
		return ErrBodyNotEnabled
	}
	if s.body.Fault != nil {
		return bodyFaultError(s.body.Fault)
	}
	for s.clock < endMS && s.ball.Mode != BallHeld {
		if err := s.stepChecked(true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sim) stepChecked(record bool) error {
	if s.body == nil {
		return ErrBodyNotEnabled
	}
	if s.body.Fault != nil {
		return bodyFaultError(s.body.Fault)
	}
	trial, ok := s.cloneFullChecked()
	if !ok {
		f := &BodyStepFault{Code: BodyFaultNumerical, StepStartMS: s.clock, Fraction: 0}
		s.body.Fault = f
		return bodyFaultError(f)
	}
	// The trial owns a DETACHED copy of the recorder, so a failed step never
	// writes into the original externally-held recorder. Accepted records are
	// committed back to the original pointer on success.
	if s.provenance != nil {
		trial.provenance = s.provenance.detached()
	}
	if err := trial.stepBody(record); err != nil {
		f := trial.body.Fault
		if f == nil {
			f = &BodyStepFault{Code: BodyFaultNumerical, StepStartMS: s.clock, Fraction: 0}
		}
		s.body.Fault = f
		return bodyFaultError(f)
	}
	if s.provenance != nil && trial.provenance != nil {
		s.provenance.records = trial.provenance.records
		s.provenance.overflowed = trial.provenance.overflowed
	}
	s.adopt(trial)
	return nil
}

// cloneFullChecked deep-copies the whole mutable Sim state for a transactional
// trial step. The optional provenance recorder pointer is NOT shared: stepChecked
// gives the trial a DETACHED recorder copy and commits accepted records back to
// the original on success, so a failed step never writes the original. The RNG is
// cloned by value; all value fields are copied by the struct copy.
func (s *Sim) cloneFullChecked() (*Sim, bool) {
	if s.rnd == nil {
		return nil, false
	}
	r, err := rng.FromState(s.rnd.State())
	if err != nil {
		return nil, false
	}
	c := *s
	c.rnd = r
	c.runners = cloneRunners(s.runners)
	c.events = append([]PassEvent(nil), s.events...)
	c.samples = append([]Sample(nil), s.samples...)
	c.preX = append([]float64(nil), s.preX...)
	c.preY = append([]float64(nil), s.preY...)
	c.localGate = append([]bool(nil), s.localGate...)
	c.localPubMS = append([]int(nil), s.localPubMS...)
	c.localEvent = append([]bool(nil), s.localEvent...)
	c.integrated = cloneIntegrated(s.integrated)
	c.body = cloneBodyState(s.body)
	return &c, true
}

// adopt copies a successfully-stepped trial state back into s.
func (s *Sim) adopt(c *Sim) {
	prov := s.provenance
	*s = *c
	s.provenance = prov
}
