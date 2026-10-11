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
)

// validate rejects configurations that could stall or corrupt the integrator.
func (c Config) validate() error {
	if c.SubstepMS <= 0 {
		return fmt.Errorf("%w: got %d ms", ErrSubstep, c.SubstepMS)
	}
	if math.IsNaN(c.DecelMPS2) || math.IsInf(c.DecelMPS2, 0) || c.DecelMPS2 < 0 {
		return fmt.Errorf("%w: got %v", ErrDecel, c.DecelMPS2)
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
}

// NewSim builds a simulator from an explicit seed. RNG is used only at declared
// release sites (see release.go); contact detection itself is deterministic and
// draws nothing. The configuration is validated so Run always advances.
func NewSim(cfg Config, seed uint64) (*Sim, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Sim{
		cfg:           cfg,
		rnd:           rng.New(seed),
		ball:          Ball{Mode: BallDead},
		contactMS:     -1,
		contactRunner: -1,
		releaseRunner: -1,
	}, nil
}

// Config returns the active clock configuration.
func (s *Sim) Config() Config { return s.cfg }

// ClockMS returns the current integer game time.
func (s *Sim) ClockMS() int { return s.clock }

// Ball returns a copy of the current ball state.
func (s *Sim) Ball() Ball { return s.ball }

// Runners returns a copy of the current runner states.
func (s *Sim) Runners() []Runner { return append([]Runner(nil), s.runners...) }

// Samples returns the authoritative sample stream recorded so far.
func (s *Sim) Samples() []Sample { return append([]Sample(nil), s.samples...) }

// FirstContact returns the first collection/control time and runner index (-1
// if none).
func (s *Sim) FirstContact() (int, int) { return s.contactMS, s.contactRunner }

// Events returns a copy of the live-pass event log (release/path/control/stop/
// out) recorded so far.
func (s *Sim) Events() []PassEvent { return append([]PassEvent(nil), s.events...) }

// SetBall places a ball in an explicit mode/state (fixture helper).
func (s *Sim) SetBall(b Ball) { s.ball = b }

// SetRunners replaces the runner set (fixture helper).
func (s *Sim) SetRunners(rs []Runner) { s.runners = append([]Runner(nil), rs...) }

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
}

// Snapshot captures the current state.
func (s *Sim) Snapshot() Snapshot {
	return Snapshot{
		Cfg:           s.cfg,
		ClockMS:       s.clock,
		Ball:          s.ball,
		Runners:       append([]Runner(nil), s.runners...),
		ContactMS:     s.contactMS,
		ContactRunner: s.contactRunner,
		RNG:           s.rnd.State(),
		Events:        append([]PassEvent(nil), s.events...),
		EventSeq:      s.eventSeq,
		ImmunityUntil: s.immunityUntil,
		ReleaseRunner: s.releaseRunner,
		LastPathMS:    s.lastPathMS,
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
	return &Sim{
		cfg:           snap.Cfg,
		rnd:           r,
		clock:         snap.ClockMS,
		ball:          snap.Ball,
		runners:       append([]Runner(nil), snap.Runners...),
		contactMS:     snap.ContactMS,
		contactRunner: snap.ContactRunner,
		events:        append([]PassEvent(nil), snap.Events...),
		eventSeq:      snap.EventSeq,
		immunityUntil: snap.ImmunityUntil,
		releaseRunner: snap.ReleaseRunner,
		lastPathMS:    snap.LastPathMS,
	}, nil
}

// RNGState exposes the generator state for evidence/tests.
func (s *Sim) RNGState() [4]uint64 { return s.rnd.State() }
