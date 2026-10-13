package liveplay

import (
	"errors"
	"fmt"
	"math"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// Configuration errors. A non-positive substep would make Run non-advancing
// (an infinite loop), and a non-finite/negative deceleration would corrupt the
// integration, so both constructors reject them up front.
var (
	// ErrSubstep: SubstepMS must be positive.
	ErrSubstep = errors.New("liveplay: substep must be positive")
	// ErrDecel: DecelMPS2 must be finite and non-negative.
	ErrDecel = errors.New("liveplay: deceleration must be finite and non-negative")
	// ErrLivePassSubstep: a live-pass substep must be shorter than the deflection
	// re-control cooldown, so a miscontrolling runner cannot re-touch the ball
	// inside the same substep. This keeps every runner to at most one touch per
	// substep, which is what makes the chronological-continuation loop terminate
	// naturally (no arbitrary iteration cap).
	ErrLivePassSubstep = errors.New("liveplay: live-pass substep must be shorter than the deflection cooldown")
	// ErrIntentCadence: a configured IntentCadenceMS must be a positive multiple
	// of SubstepMS, so the periodic lattice lands exactly on substep boundaries.
	ErrIntentCadence = errors.New("liveplay: intent cadence must be a positive multiple of the substep")
	// ErrIntentCadenceNeedsLivePass: the opt-in sampled scheduler is part of the
	// live-pass model and is rejected without LivePass.
	ErrIntentCadenceNeedsLivePass = errors.New("liveplay: intent cadence is only valid with LivePass")
)

// validate rejects configurations that could stall or corrupt the integrator.
func (c Config) validate() error {
	if c.SubstepMS <= 0 {
		return fmt.Errorf("%w: got %d ms", ErrSubstep, c.SubstepMS)
	}
	if math.IsNaN(c.DecelMPS2) || math.IsInf(c.DecelMPS2, 0) || c.DecelMPS2 < 0 {
		return fmt.Errorf("%w: got %v", ErrDecel, c.DecelMPS2)
	}
	if c.LivePass && c.SubstepMS >= DeflectionCooldownMS {
		return fmt.Errorf("%w: got %d ms, limit < %d ms", ErrLivePassSubstep, c.SubstepMS, DeflectionCooldownMS)
	}
	if c.IntentCadenceMS != 0 {
		if c.IntentCadenceMS < 0 {
			return fmt.Errorf("%w: got %d ms", ErrIntentCadence, c.IntentCadenceMS)
		}
		if !c.LivePass {
			return fmt.Errorf("%w: got %d ms", ErrIntentCadenceNeedsLivePass, c.IntentCadenceMS)
		}
		if c.IntentCadenceMS%c.SubstepMS != 0 {
			return fmt.Errorf("%w: %d ms is not a multiple of substep %d ms", ErrIntentCadence, c.IntentCadenceMS, c.SubstepMS)
		}
	}
	return nil
}

// BallMode is the authoritative ball state. The first slice keeps only the
// minimal modes needed for a causal ground-pass contest; airborne/aerial play is
// explicitly later work (T-037+).
type BallMode uint8

// Ball modes.
const (
	// BallHeld: the ball is attached to its owner (no free integration).
	BallHeld BallMode = iota
	// BallRolling: a free ground ball integrating under rolling friction.
	BallRolling
	// BallDead: stationary (restart spot, or a rolling ball that friction stopped
	// in the legacy mode).
	BallDead
	// BallLoose is a stationary but STILL COLLECTABLE ground ball (T-037: a live
	// pass that stopped short). It is deliberately distinct from BallDead (a dead
	// restart spot) and BallOut (dead by leaving the field).
	BallLoose
	// BallOut is a dead ball that left the field of play during a live pass.
	BallOut
)

// String returns the mode label for traces and evidence.
func (m BallMode) String() string {
	switch m {
	case BallHeld:
		return "held"
	case BallRolling:
		return "rolling"
	case BallLoose:
		return "loose"
	case BallOut:
		return "out"
	default:
		return "dead"
	}
}

// Ball is the authoritative ball state. Position is normalized pitch units;
// velocity is metres per second (VXM along the length, VYM across the width).
type Ball struct {
	Mode BallMode `json:"mode"`
	X    float64  `json:"x"`
	Y    float64  `json:"y"`
	VXM  float64  `json:"vxM"`
	VYM  float64  `json:"vyM"`
	// HoldDX/HoldDY are the normalized world-frame offset from the owner's
	// reference point to the ball while Held. Zero means "at the owner's feet"
	// (the legacy T-035 owner-follow). Live-pass control stores the physical
	// contact offset so the ball is NOT warped to the feet on receipt (T-037).
	HoldDX float64 `json:"holdDx,omitempty"`
	HoldDY float64 `json:"holdDy,omitempty"`
}

// Runner is a minimal kinematic agent used to test reachability and
// swept-contact timing. The zero-valued fields describe the T-035
// constant-velocity runner so every existing fixture is unchanged; T-036 adds an
// opt-in inertial model (see motion.go). Complete inertial state lives here
// (velocity, intent and resolved physical limits) so a Snapshot preserves it.
type Runner struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	VXM    float64 `json:"vxM"`
	VYM    float64 `json:"vyM"`
	ReachM float64 `json:"reachM"` // combined collection threshold in metres

	// Motion selects the integration model. The zero value MotionKinematic is
	// the T-035 constant-velocity behaviour (backward compatible).
	Motion RunnerMotion `json:"motion,omitempty"`
	// IntentX/Y is the normalized movement destination, used only when
	// Motion==MotionInertial. It is separate from the integrated state.
	IntentX float64 `json:"intentX,omitempty"`
	IntentY float64 `json:"intentY,omitempty"`
	// Resolved physical limits (motion.go). Zero means "use the default
	// envelope"; explicit values keep a resumed run exact.
	TopSpeedMPS      float64 `json:"topSpeedMps,omitempty"`
	AccelMPS2        float64 `json:"accelMps2,omitempty"`
	DecelMPS2        float64 `json:"decelMps2,omitempty"`
	LateralAccelMPS2 float64 `json:"lateralAccelMps2,omitempty"`

	// Pursuit/reaction state (T-038, live mode only). Pursue opts an inertial
	// runner into chasing a free (Rolling/Loose) ball; ReactMS is its reaction
	// delay after a release; ReactAtMS and CooldownUntilMS are the persisted
	// runtime deadlines (reaction activation and post-deflection re-control
	// cooldown) so a mid-contest Resume reproduces them exactly.
	Pursue          bool `json:"pursue,omitempty"`
	ReactMS         int  `json:"reactMs,omitempty"`
	ReactAtMS       int  `json:"reactAtMs,omitempty"`
	CooldownUntilMS int  `json:"cooldownUntilMs,omitempty"`

	// Perception is the optional local ball-observation record (T-043). It is
	// nil unless the runner opts in (see EnableSensing); when present its
	// RadiusM (> 0) is the single enable source. Observation-only: it never
	// changes intent, contact, motion or RNG. Every public state copy deep-copies
	// this pointer so no live state is aliased. Omitted from JSON when nil, so
	// disabled runners and fixtures keep the exact old key set.
	Perception *BallObservation `json:"perception,omitempty"`

	// LocalPursuit opts a runner into the T-044 observation-driven local intent
	// policy, INDEPENDENT of the legacy native-oracle Pursue flag. The native
	// publisher always skips a local runner (no omniscient fallback); the local
	// policy may run ONLY during an armed scheduler publication, after the local
	// perception sample, and reads only this runner's own observation record and
	// position. Zero value = off, so disabled snapshots keep the exact old JSON
	// key set. See localpursuit.go.
	LocalPursuit bool `json:"localPursuit,omitempty"`

	// LocalWakeup opts a runner into the T-046 per-runner local event wakeup: with
	// an armed scheduler and a valid sensing record, this runner's local sample and
	// local intent layers run on the periodic lattice and, off-lattice, only when
	// its OWN WakePending is set — a remote event never wakes it. An unflagged
	// runner keeps the exact T-042 shared semantics, and native Pursue publication
	// is always shared regardless of this flag. Zero value = off, so disabled
	// snapshots keep the exact old JSON key set. See wakeup.go.
	LocalWakeup bool `json:"localWakeup,omitempty"`

	// WakePending is the persisted per-runner local wakeup set at the event emitter
	// when a state-changing ball event is LOCAL to this runner by event-time
	// geometry. It coalesces every event of one substep and is consumed (cleared)
	// at the immediate next pre-step publication regardless of the reaction gate
	// (drop-before-reaction). Zero value = no pending, so disabled snapshots keep
	// the exact old JSON key set. See wakeup.go.
	WakePending bool `json:"wakePending,omitempty"`
}

// Config is the spike's clock/friction configuration.
type Config struct {
	SubstepMS int     `json:"substepMs"`
	DecelMPS2 float64 `json:"decelMps2"`
	// LivePass enables the T-037 authoritative live ground-pass semantics: the
	// ball keeps a physical rolling trajectory, ownership is unassigned during
	// travel, contact preserves the ball's physical position (no warp to the
	// feet), a friction-stopped ball stays a collectable loose ball, and a ball
	// that leaves the field dies out of play. The zero value (false) keeps the
	// exact T-035/T-036 endpoint/owner-follow behaviour, so no existing fixture
	// changes meaning. See livepass.go.
	LivePass bool `json:"livePass,omitempty"`
	// IntentCadenceMS arms the opt-in sampled intent scheduler (T-042) on the
	// live path. Zero (the default) keeps the exact native per-substep intent
	// publication; a non-zero value publishes pursuit intents on the stable
	// periodic lattice and on out-of-band state-changing events. It must be a
	// positive multiple of SubstepMS and is only valid with LivePass. See
	// scheduler.go.
	IntentCadenceMS int `json:"intentCadenceMs,omitempty"`
}

// DefaultConfig is the recommended configuration the spec proposes.
func DefaultConfig() Config {
	return Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2}
}

// ReferenceConfig is the high-resolution (1 ms) numerical reference used to
// size candidate clocks. It is the finest integration measured here, not an
// analytic or exact truth.
func ReferenceConfig() Config {
	return Config{SubstepMS: ReferenceSubstepMS, DecelMPS2: RollDecelMPS2}
}

// Sample is one authoritative moment on the fixed integer clock. Samples are
// derived from the simulation, so a snapshot never needs to persist them: a
// resumed run regenerates the identical sample stream.
type Sample struct {
	ClockMS int      `json:"clockMs"`
	BallX   float64  `json:"ballX"`
	BallY   float64  `json:"ballY"`
	Mode    BallMode `json:"mode"`
	Owner   int      `json:"owner"` // runner index that owns a held ball, -1 otherwise
}

// Sim is the whole micro-step machine. Every field survives a snapshot so that
// Resume from any integer-substep boundary reproduces the straight run exactly.
type Sim struct {
	cfg   Config
	rnd   *rng.Rand
	clock int

	ball    Ball
	runners []Runner

	contactMS     int // first collection/control time, -1 if none yet
	contactRunner int // runner index of the first collection/control, -1 if none

	samples []Sample

	// Live-pass state (T-037). All of it is serialized so a mid-flight Resume
	// reproduces the remaining travel, events and control exactly.
	events        []PassEvent // explicit release/path/control/stop/out log
	eventSeq      int         // monotonic event sequence number
	immunityUntil int         // clock until which the passer may not re-collect
	releaseRunner int         // passer index of the current pass, -1 if none
	lastPathMS    int         // clock of the last recorded path event

	// Live-contest state (T-038). Also serialized: the contact-local control
	// budget of the current pass and the first physical touch (control OR
	// deflection), which a mid-contest Resume must reproduce.
	controlError     float64 // control/execution uncertainty budget of the current pass
	firstTouchMS     int     // first physical contact time, -1 if none yet
	firstTouchRunner int     // runner index of the first physical contact, -1 if none

	// Opt-in sampled intent scheduler state (T-042). nextDecisionMS and
	// decisionDirty are serialized (exact Resume); decisionPublishes and
	// lastDecisionMS are evidence-only counters. All four are inert while
	// Config.IntentCadenceMS == 0. See scheduler.go.
	nextDecisionMS    int  // next periodic decision deadline
	decisionDirty     bool // a prior completed step emitted a state-changing event
	decisionPublishes int  // evidence: number of scheduler publications
	lastDecisionMS    int  // evidence: pre-step clock of the last publication, -1 if none

	// Opt-in local event wakeup evidence (T-046). None of these is serialized; they
	// are diagnostics that must NOT be confused with the shared global scheduler
	// clocks above (a remote event still bumps decisionPublishes/lastDecisionMS).
	// localWakePublications counts per-runner local-layer SCHEDULING ATTEMPTS for
	// runners opted into LocalWakeup (the reaction gate may still drop the sample in
	// that attempt); localPubMS is the last pre-step clock each opted runner's local
	// layer was scheduled (-1 if never); localGate is reused per-step scratch so the
	// per-runner gating never allocates. See wakeup.go.
	localWakePublications int
	localPubMS            []int
	localGate             []bool

	// localEvent is reused per-publication scratch for the OPTIONAL provenance
	// recorder (T-049): the per-runner event signal that coincided with a
	// publication (an opted-in runner's OWN pre-consumption WakePending, or the
	// shared dirty flag for an unflagged runner). It is allocated ONLY while a
	// provenance recorder is attached; the default path allocates nothing. Never
	// serialized and never read by a simulation decision.
	localEvent []bool

	// Optional decision-provenance recorder (T-049). It is a diagnostic sink
	// ONLY: never serialized in Snapshot, never read by any simulation decision,
	// and nil by default so the disabled path allocates nothing. See provenance.go.
	provenance *ProvenanceRecorder

	// Pre-step scratch (T-038): explicit pre-integration reference positions for
	// the exact substep chords. Not serialized and reused every substep, so the
	// live resolver never allocates per step.
	preBall Ball
	preX    []float64
	preY    []float64
}

// NewSim builds a simulator from an explicit seed. RNG is used only at declared
// release sites (see release.go); contact detection itself is deterministic and
// draws nothing. The configuration is validated so Run always advances.
func NewSim(cfg Config, seed uint64) (*Sim, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Sim{
		cfg:              cfg,
		rnd:              rng.New(seed),
		ball:             Ball{Mode: BallDead},
		contactMS:        -1,
		contactRunner:    -1,
		releaseRunner:    -1,
		firstTouchMS:     -1,
		firstTouchRunner: -1,
		lastDecisionMS:   -1,
	}, nil
}

// Config returns the active clock configuration.
func (s *Sim) Config() Config { return s.cfg }

// ClockMS returns the current integer game time.
func (s *Sim) ClockMS() int { return s.clock }

// Ball returns a copy of the current ball state.
func (s *Sim) Ball() Ball { return s.ball }

// Runners returns a deep copy of the current runner states. Every mutable
// perception record is cloned too, so a caller can never alias live state.
func (s *Sim) Runners() []Runner { return cloneRunners(s.runners) }

// Samples returns the authoritative sample stream recorded so far.
func (s *Sim) Samples() []Sample { return append([]Sample(nil), s.samples...) }

// FirstContact returns the first collection/control time and runner index (-1
// if none).
func (s *Sim) FirstContact() (int, int) { return s.contactMS, s.contactRunner }

// FirstTouch returns the first PHYSICAL contact time and runner index (-1 if
// none). Unlike FirstContact, a contact that miscontrols/deflects still counts:
// it is the first time any player actually reached the ball.
func (s *Sim) FirstTouch() (int, int) { return s.firstTouchMS, s.firstTouchRunner }

// Events returns a copy of the live-pass event log (release/path/control/stop/
// out) recorded so far.
func (s *Sim) Events() []PassEvent { return append([]PassEvent(nil), s.events...) }

// SetBall places a ball in an explicit mode/state (fixture helper).
func (s *Sim) SetBall(b Ball) { s.ball = b }

// SetRunners replaces the runner set (fixture helper). The incoming runners and
// every mutable perception record are deep-copied, so the caller and the sim
// never share a pointer.
func (s *Sim) SetRunners(rs []Runner) {
	s.runners = cloneRunners(rs)
	s.ensurePreCapacity(len(s.runners))
}

// Step advances exactly one substep of the fixed update order:
//
//  1. integrate the ball under rolling friction (semi-implicit Euler: velocity
//     is decelerated first, then position is advanced with the new velocity);
//  2. integrate the runners (kinematic constant velocity, or the inertial
//     model when the runner opts in via Motion==MotionInertial);
//  3. advance the integer clock to the end of the step;
//  4. test contact AFTER integration at the END-step clock, so the recorded
//     event time equals the sample time of the same step (no half-step skew);
//  5. if held, follow the owner (live pass keeps the physical contact offset so
//     the ball is not warped to the feet; legacy glues it to the feet);
//  6. record a sample on the fixed grid.
//
// In live-pass mode step 4 collects a Rolling OR Loose ball and stores the
// contact offset (no warp); the releasing passer is immune to re-collection for
// PasserImmunityMS. A friction-stopped live ball becomes BallLoose (collectable)
// and a ball leaving the pitch becomes BallOut (dead).
//
// Tie rule (stable, deterministic): among all runners within reach, the nearest
// wins; equal distances keep the LOWEST runner index (iteration is ascending and
// only a strictly smaller distance replaces the current best).
func (s *Sim) Step() { s.step(true) }

// StepQuiet advances one substep without recording a sample. Used by the
// reference and sweep sweeps, where millions of substeps would otherwise
// allocate a sample slice; the physics is identical to Step.
func (s *Sim) StepQuiet() { s.step(false) }

func (s *Sim) step(record bool) {
	if s.cfg.LivePass {
		s.stepLive(record)
		return
	}
	s.stepLegacy(record)
}

// stepLegacy is the T-035/T-036 endpoint update order. It is only reached when
// LivePass is false; the live branch has its own swept, chronological order in
// stepLive (contest.go / below). Its `s.cfg.LivePass` branches are therefore
// inert here and kept verbatim so the pinned legacy behaviour is untouched.
func (s *Sim) stepLegacy(record bool) {
	dt := float64(s.cfg.SubstepMS) / 1000.0

	// Terminal ball events (out/stop) describe the POST-integration state, so
	// they must be stamped at the END-step clock (the same clock as this step's
	// authoritative sample). Emitting them from inside the ball-integration block
	// would use the pre-advance clock and misdate them one substep early.
	var terminalKind PassEventKind
	terminalEvent := false

	if s.ball.Mode == BallRolling {
		speed := math.Sqrt(s.ball.VXM*s.ball.VXM + s.ball.VYM*s.ball.VYM)
		stopped := false
		if newSpeed := speed - s.cfg.DecelMPS2*dt; newSpeed <= 0 {
			s.ball.VXM, s.ball.VYM = 0, 0
			stopped = true
		} else {
			scale := newSpeed / speed
			s.ball.VXM *= scale
			s.ball.VYM *= scale
		}
		s.ball.X += (s.ball.VXM / PitchLengthM) * dt
		s.ball.Y += (s.ball.VYM / PitchWidthM) * dt

		switch {
		case s.cfg.LivePass && s.ballOutOfBounds():
			// A live ball that leaves the field dies out of play: a restart, not
			// a collectable loose ball.
			s.ball.Mode = BallOut
			s.ball.VXM, s.ball.VYM = 0, 0
			terminalKind, terminalEvent = PassEventOut, true
		case stopped:
			if s.cfg.LivePass {
				// Stopped short: stays LIVE and collectable (BallLoose), never a
				// dead restart spot.
				s.ball.Mode = BallLoose
				terminalKind, terminalEvent = PassEventStop, true
			} else {
				s.ball.Mode = BallDead
			}
		}
	}

	for i := range s.runners {
		r := &s.runners[i]
		if r.Motion == MotionInertial {
			// T-036: each runner integrates from its own state + intent only, so
			// the loop order cannot confer an advantage.
			r.integrateInertial(dt)
			continue
		}
		r.X += (r.VXM / PitchLengthM) * dt
		r.Y += (r.VYM / PitchWidthM) * dt
	}

	s.clock += s.cfg.SubstepMS

	if terminalEvent {
		s.emitEvent(terminalKind, -1, s.clock, s.ball)
	}

	if s.cfg.LivePass && s.ball.Mode == BallRolling && s.clock-s.lastPathMS >= PathRecordMS {
		// Explicit path trace of the free travel, separate from the control event.
		s.emitEvent(PassEventPath, -1, s.clock, s.ball)
		s.lastPathMS = s.clock
	}

	if s.cfg.LivePass {
		if s.ball.Mode == BallRolling || s.ball.Mode == BallLoose {
			best, bestDist := -1, math.Inf(1)
			for i := range s.runners {
				if i == s.releaseRunner && s.clock < s.immunityUntil {
					continue // passer immunity: the source cannot re-collect yet
				}
				r := s.runners[i]
				reach := r.ReachM
				if reach <= 0 {
					reach = ControlRadiusM
				}
				if d := DistM(s.ball.X, s.ball.Y, r.X, r.Y); d <= reach && d < bestDist {
					best, bestDist = i, d
				}
			}
			if best >= 0 {
				// Preserve the physical ball position at contact via a held
				// offset: the ball is NOT warped to the controller's feet.
				r := s.runners[best]
				s.ball.HoldDX = s.ball.X - r.X
				s.ball.HoldDY = s.ball.Y - r.Y
				s.ball.Mode = BallHeld
				s.ball.VXM, s.ball.VYM = 0, 0
				if s.contactMS < 0 {
					s.contactMS, s.contactRunner = s.clock, best
				}
				s.emitEvent(PassEventControl, best, s.clock, s.ball)
			}
		}
	} else if s.ball.Mode == BallRolling {
		best, bestDist := -1, math.Inf(1)
		for i := range s.runners {
			r := s.runners[i]
			reach := r.ReachM
			if reach <= 0 {
				reach = ControlRadiusM
			}
			if d := DistM(s.ball.X, s.ball.Y, r.X, r.Y); d <= reach && d < bestDist {
				best, bestDist = i, d
			}
		}
		if best >= 0 {
			s.ball.Mode = BallHeld
			s.ball.VXM, s.ball.VYM = 0, 0
			if s.contactMS < 0 {
				s.contactMS, s.contactRunner = s.clock, best
			}
		}
	}

	if s.ball.Mode == BallHeld && s.contactRunner >= 0 && s.contactRunner < len(s.runners) {
		r := s.runners[s.contactRunner]
		if s.cfg.LivePass {
			// Continuous held position: owner + the physical contact offset.
			s.ball.X = r.X + s.ball.HoldDX
			s.ball.Y = r.Y + s.ball.HoldDY
		} else {
			s.ball.X, s.ball.Y = r.X, r.Y
		}
	}

	if record {
		s.recordSample()
	}
}

// stepLive is the T-038 live-mode update order. It differs from stepLegacy in
// three ways:
//
//  1. pursuit intents are republished from the consistent PRE-step world before
//     any integration, and the pre-step reference positions are captured
//     explicitly (never recovered from post-integration velocities);
//  2. the ball/runner substep is resolved by a swept, analytic chord contact
//     (contest.go) instead of an endpoint sample, so fast and grazing balls
//     cannot tunnel;
//  3. the substep is a full chronological continuation: events are applied in
//     declared time order without rewinding any body, and a contact stores the
//     fractional contact state while every body keeps its common end-step state.
//
// A contact may miscontrol/deflect (contact-local seeded uncertainty) into a live
// loose trajectory with a persisted physical re-control cooldown, and the
// remaining fraction is integrated so another contact or a pitch exit can still
// occur in the same substep. All state is serialized, so a mid-contest Resume is
// exact.
func (s *Sim) stepLive(record bool) {
	dt := float64(s.cfg.SubstepMS) / 1000.0
	stepStart := s.clock

	s.capturePreStep()
	s.maybePublishIntents(stepStart)

	// 1. Ball integration under rolling friction (semi-implicit Euler: velocity
	// is decelerated first, then position advanced with the new velocity).
	stopped := false
	if s.ball.Mode == BallRolling {
		speed := math.Sqrt(s.ball.VXM*s.ball.VXM + s.ball.VYM*s.ball.VYM)
		if newSpeed := speed - s.cfg.DecelMPS2*dt; newSpeed <= 0 {
			s.ball.VXM, s.ball.VYM = 0, 0
			stopped = true
		} else {
			scale := newSpeed / speed
			s.ball.VXM *= scale
			s.ball.VYM *= scale
		}
		s.ball.X += (s.ball.VXM / PitchLengthM) * dt
		s.ball.Y += (s.ball.VYM / PitchWidthM) * dt
	}

	// 2. Runner integration (each runner reads only its own state + intent). The
	// end-step positions are FINAL: a contact never rewinds a runner.
	for i := range s.runners {
		r := &s.runners[i]
		if r.Motion == MotionInertial {
			r.integrateInertial(dt)
			continue
		}
		r.X += (r.VXM / PitchLengthM) * dt
		r.Y += (r.VYM / PitchWidthM) * dt
	}

	// 3. Advance the clock to the end of the step; all events of this step share
	// this integer clock (with an exact within-step fraction on physical events).
	s.clock += s.cfg.SubstepMS

	// 4. Resolve the free-ball contest as a full chronological continuation.
	if s.ball.Mode == BallRolling || s.ball.Mode == BallLoose {
		s.resolveFreeBall(stepStart, dt, stopped)
	}

	// 5. Held ball follows its owner (live mode keeps the physical contact offset,
	// so the ball is never warped to the feet).
	if s.ball.Mode == BallHeld && s.contactRunner >= 0 && s.contactRunner < len(s.runners) {
		r := s.runners[s.contactRunner]
		s.ball.X = r.X + s.ball.HoldDX
		s.ball.Y = r.Y + s.ball.HoldDY
	}

	// 6. Periodic path trace of free travel.
	if s.ball.Mode == BallRolling && s.clock-s.lastPathMS >= PathRecordMS {
		s.emitEvent(PassEventPath, -1, s.clock, s.ball)
		s.lastPathMS = s.clock
	}

	if record {
		s.recordSample()
	}
}

// resolveFreeBall resolves one live substep of a free (Rolling or Loose) ball as
// a chronological continuation. The ball travels from its captured pre-step
// position while the runner chords span the full substep. If a contact
// miscontrols, the ball is re-seeded with the deflected velocity and the remaining
// fraction is integrated, so another contact or a pitch exit can still occur in
// the SAME substep. No body is rewound and no travel is discarded. Each runner can
// touch at most once per substep (a deflection sets a cooldown beyond the substep),
// so the loop terminates naturally without an arbitrary iteration cap.
func (s *Sim) resolveFreeBall(stepStart int, dt float64, stopped bool) {
	// Current ball segment: from (px,py) at global fraction uCur to (ex,ey) at
	// fraction 1, moving at (vx,vy) metres/second.
	px, py := s.preBall.X, s.preBall.Y
	ex, ey := s.ball.X, s.ball.Y
	vx, vy := s.ball.VXM, s.ball.VYM
	uCur := 0.0
	firstSegment := true

	for {
		bestKind, bestU, bestRunner := evNone, 2.0, -1

		// Candidate: the ball leaves the pitch (only an endpoint strictly outside
		// counts; resting on a boundary line is in play).
		if u, ok := segmentExit(px, py, ex, ey, uCur); ok && u < bestU {
			bestKind, bestU = evOut, u
		}

		// Candidate: the earliest physical contact, gated by each runner's
		// reaction activation and re-control cooldown and by the passer's
		// immunity — all INSIDE the substep, not merely at its endpoint.
		for i := range s.runners {
			r := &s.runners[i]
			reach := r.ReachM
			if reach <= 0 {
				reach = ControlRadiusM
			}
			uMin := uCur
			if r.ReactAtMS > stepStart {
				if c := float64(r.ReactAtMS-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if r.CooldownUntilMS > stepStart {
				if c := float64(r.CooldownUntilMS-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if i == s.releaseRunner && s.immunityUntil > stepStart {
				if c := float64(s.immunityUntil-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if uMin > 1 {
				continue
			}
			u, ok := segmentContact(px, py, uCur, ex, ey, s.preX[i], s.preY[i], r.X, r.Y, reach, uMin)
			if !ok {
				continue
			}
			// Strictly earlier wins; equal fractions keep the LOWEST index
			// (iteration is ascending). A contact beats an equal-time exit/stop.
			if u < bestU || (u == bestU && bestKind != evContact) {
				bestKind, bestU, bestRunner = evContact, u, i
			}
		}

		// Candidate: the roll ended. The discrete integrator zeroes the velocity
		// before advancing, so the roll ends at the substep BOUNDARY: the stop
		// event is stamped at fraction 1 (the end-step clock), matching the
		// discrete transition. It is only relevant in the first segment (a
		// deflection re-seeds the ball with velocity).
		if stopped && firstSegment && bestKind == evNone {
			bestKind, bestU = evStop, 1.0
		}

		switch bestKind {
		case evNone:
			// No event: finish the travel at this segment's endpoint.
			s.ball.X, s.ball.Y = ex, ey
			s.ball.VXM, s.ball.VYM = vx, vy
			return

		case evOut:
			bx, by := segmentPoint(px, py, uCur, ex, ey, bestU)
			s.ball.X, s.ball.Y = bx, by
			s.ball.Mode = BallOut
			s.ball.VXM, s.ball.VYM = 0, 0
			s.emitEventAt(PassEventOut, -1, s.clock, bestU, s.ball)
			return

		case evStop:
			s.ball.Mode = BallLoose
			s.emitEventAt(PassEventStop, -1, s.clock, bestU, s.ball)
			return

		case evContact:
			bx, by := segmentPoint(px, py, uCur, ex, ey, bestU)
			r := &s.runners[bestRunner]
			rxc := s.preX[bestRunner] + bestU*(r.X-s.preX[bestRunner])
			ryc := s.preY[bestRunner] + bestU*(r.Y-s.preY[bestRunner])
			if s.firstTouchMS < 0 {
				s.firstTouchMS, s.firstTouchRunner = s.clock, bestRunner
			}

			// Contact-local control uncertainty: drawn ONLY at an actual contact.
			// A zero budget (the T-037 default) draws nothing and always controls.
			miscontrol := false
			if s.controlError > 0 {
				miscontrol = s.rnd.Float64() < s.controlError // DrawContactControl
			}

			if miscontrol {
				vdx, vdy := s.deflectVelocity(vx, vy, bx, by, rxc, ryc)
				// The deflected ball is LIVE and moves again: switch the mode
				// explicitly (a starting BallLoose ball would otherwise keep
				// Mode==Loose and never integrate on the next substep). A
				// deflection with zero resulting speed stays a stationary loose
				// ball.
				s.ball.Mode = BallRolling
				if vdx == 0 && vdy == 0 {
					s.ball.Mode = BallLoose
				}
				s.ball.X, s.ball.Y = bx, by
				s.ball.VXM, s.ball.VYM = vdx, vdy
				// Physical re-control cooldown, based on the ACTUAL contact time
				// (declared integer-ms rounding). The LivePass substep is bounded
				// below DeflectionCooldownMS (config validation), so the deflector
				// cannot re-touch its own bounce within this substep or the cooldown.
				r.CooldownUntilMS = eventTimeMS(stepStart, s.cfg.SubstepMS, bestU) + DeflectionCooldownMS
				contactBall := Ball{Mode: BallRolling, X: bx, Y: by, VXM: vx, VYM: vy}
				s.emitContactEvent(PassEventDeflect, bestRunner, s.clock, bestU, contactBall, vdx, vdy, rxc, ryc)
				// Continue: integrate the remaining fraction under the deflected
				// velocity so another contact or a pitch exit can still occur.
				px, py = bx, by
				vx, vy = vdx, vdy
				uCur = bestU
				rem := dt * (1 - bestU)
				ex = px + (vx/PitchLengthM)*rem
				ey = py + (vy/PitchWidthM)*rem
				firstSegment = false
				stopped = false // the deflected ball is moving again
				continue
			}

			// Clean control: the ball follows its owner for the remaining fraction,
			// so the end-step sample physically differs from the fractional event.
			s.ball.HoldDX = bx - rxc
			s.ball.HoldDY = by - ryc
			s.ball.VXM, s.ball.VYM = 0, 0
			s.ball.X = r.X + s.ball.HoldDX
			s.ball.Y = r.Y + s.ball.HoldDY
			s.ball.Mode = BallHeld
			if s.contactMS < 0 {
				s.contactMS, s.contactRunner = s.clock, bestRunner
			}
			contactBall := Ball{Mode: BallHeld, X: bx, Y: by, VXM: vx, VYM: vy}
			s.emitContactEvent(PassEventControl, bestRunner, s.clock, bestU, contactBall, 0, 0, rxc, ryc)
			return
		}
	}
}

// ballOutOfBounds reports whether the ball centre has left the pitch rectangle.
func (s *Sim) ballOutOfBounds() bool {
	return s.ball.X < 0 || s.ball.X > 1 || s.ball.Y < 0 || s.ball.Y > 1
}

func (s *Sim) recordSample() {
	owner := -1
	if s.ball.Mode == BallHeld {
		owner = s.contactRunner
	}
	s.samples = append(s.samples, Sample{
		ClockMS: s.clock,
		BallX:   s.ball.X,
		BallY:   s.ball.Y,
		Mode:    s.ball.Mode,
		Owner:   owner,
	})
}

// Run steps until clock reaches endMS (exclusive) or the ball is collected.
func (s *Sim) Run(endMS int) {
	for s.clock < endMS && s.ball.Mode != BallHeld {
		s.Step()
	}
}

// Snapshot is the complete serializable state at a substep boundary. Resuming
// from it reproduces the straight run bit for bit; samples are intentionally not
// persisted (they are derived and regenerated).
type Snapshot struct {
	Cfg           Config    `json:"cfg"`
	ClockMS       int       `json:"clockMs"`
	Ball          Ball      `json:"ball"`
	Runners       []Runner  `json:"runners"`
	ContactMS     int       `json:"contactMs"`
	ContactRunner int       `json:"contactRunner"`
	RNG           [4]uint64 `json:"rng"`

	// Live-pass state (T-037): the event log and sequence, the passer immunity
	// deadline and the release/path bookkeeping all survive a Resume.
	Events        []PassEvent `json:"events,omitempty"`
	EventSeq      int         `json:"eventSeq,omitempty"`
	ImmunityUntil int         `json:"immunityUntil,omitempty"`
	ReleaseRunner int         `json:"releaseRunner"`
	LastPathMS    int         `json:"lastPathMs,omitempty"`

	// Live-contest state (T-038): the contact-local control budget and the first
	// physical touch survive a mid-contest Resume. Runner reaction/cooldown
	// deadlines ride inside Runners.
	ControlError     float64 `json:"controlError,omitempty"`
	FirstTouchMS     int     `json:"firstTouchMs"`
	FirstTouchRunner int     `json:"firstTouchRunner"`

	// Opt-in sampled scheduler state (T-042). Both are omitted at zero, so a
	// disabled snapshot keeps the exact old JSON key set. They are everything the
	// scheduler needs to continue: the lattice continuation and the pending
	// invalidation flag (the consumed-event marker). The scheduler draws no RNG
	// and scans no event log, so nothing else is persisted.
	NextDecisionMS int  `json:"nextDecisionMs,omitempty"`
	DecisionDirty  bool `json:"decisionDirty,omitempty"`
}

// Snapshot captures the current state.
func (s *Sim) Snapshot() Snapshot {
	return Snapshot{
		Cfg:              s.cfg,
		ClockMS:          s.clock,
		Ball:             s.ball,
		Runners:          cloneRunners(s.runners),
		ContactMS:        s.contactMS,
		ContactRunner:    s.contactRunner,
		RNG:              s.rnd.State(),
		Events:           append([]PassEvent(nil), s.events...),
		EventSeq:         s.eventSeq,
		ImmunityUntil:    s.immunityUntil,
		ReleaseRunner:    s.releaseRunner,
		LastPathMS:       s.lastPathMS,
		ControlError:     s.controlError,
		FirstTouchMS:     s.firstTouchMS,
		FirstTouchRunner: s.firstTouchRunner,
		NextDecisionMS:   s.nextDecisionMS,
		DecisionDirty:    s.decisionDirty,
	}
}

// Resume rebuilds a simulator from a snapshot. It is the only constructor that
// does not advance the RNG, so a resumed run continues the exact draw sequence.
func Resume(snap Snapshot) (*Sim, error) {
	if err := snap.Cfg.validate(); err != nil {
		return nil, fmt.Errorf("resuming: %w", err)
	}
	r, err := rng.FromState(snap.RNG)
	if err != nil {
		return nil, err
	}
	runners, err := resumeRunners(snap.Runners, snap.ClockMS, snap.Cfg)
	if err != nil {
		return nil, fmt.Errorf("resuming: %w", err)
	}
	return &Sim{
		cfg:              snap.Cfg,
		rnd:              r,
		clock:            snap.ClockMS,
		ball:             snap.Ball,
		runners:          runners,
		contactMS:        snap.ContactMS,
		contactRunner:    snap.ContactRunner,
		events:           append([]PassEvent(nil), snap.Events...),
		eventSeq:         snap.EventSeq,
		immunityUntil:    snap.ImmunityUntil,
		releaseRunner:    snap.ReleaseRunner,
		lastPathMS:       snap.LastPathMS,
		controlError:     snap.ControlError,
		firstTouchMS:     snap.FirstTouchMS,
		firstTouchRunner: snap.FirstTouchRunner,
		nextDecisionMS:   snap.NextDecisionMS,
		decisionDirty:    snap.DecisionDirty,
		lastDecisionMS:   -1,
	}, nil
}

// RNGState exposes the generator state for evidence/tests.
func (s *Sim) RNGState() [4]uint64 { return s.rnd.State() }
