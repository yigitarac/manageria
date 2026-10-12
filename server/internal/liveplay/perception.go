package liveplay

import (
	"errors"
	"fmt"
	"math"
)

// Local sampled ball perception (T-043).
//
// This layer is OBSERVATION-ONLY. It adds a per-runner record of the last local
// ball sighting, sampled at the opt-in scheduler's publication (T-042) from the
// consistent CURRENT pre-step world, after the same reaction gate the pursuit
// layer uses. It deliberately adds NO intent policy, duty, team AI or
// held-player behaviour, and it never changes an intent, a contact, the motion
// or an RNG draw.
//
// Opt-in is per-runner and has a SINGLE enable source: a non-nil
// Runner.Perception whose RadiusM (metres) is finite and positive. The sensing
// radius is an explicit ASSUMPTION (a local sound/vision circle), not a measured
// truth. Sampling only runs while the scheduler is armed
// (Config.IntentCadenceMS > 0); the native per-substep path never samples.
//
// No remote leak: a state-changing event only schedules another publication; the
// radius gate is always enforced, so an out-of-range ball never writes its
// position. Held/Dead/Out modes ARE observable when in range (unlike native
// pursuit, which no-ops while the ball is not Rolling/Loose).

// Configuration errors for the perception opt-in. Broken input is reported, not
// silently accepted.
var (
	// ErrSenseRadius: a sensing radius must be finite and strictly positive.
	ErrSenseRadius = errors.New("liveplay: sensing radius must be finite and positive")
	// ErrSenseCoordinates: a persisted observation must carry finite ball state.
	ErrSenseCoordinates = errors.New("liveplay: observation coordinates must be finite")
	// ErrSenseSchema: a persisted SEEN observation must carry a self-consistent
	// timestamp/mode/owner that could actually have been observed at or before the
	// snapshot clock (a future sighting would leak impossible knowledge).
	ErrSenseSchema = errors.New("liveplay: observation timestamp/mode/owner is inconsistent")
)

// BallObservation is the last-known local ball state for one runner. It is the
// value behind Runner.Perception. RadiusM is the single enable source (finite,
// > 0); the remaining fields are the last successful sighting. Every field is
// serialized without omitempty once the record is present, so a legitimate zero
// coordinate, the zero BallMode or a clock-0 sighting is never lost; the record's
// absence (nil pointer) is what marks "not opted in", and Seen/ClockMS mark
// "never seen" vs "stale".
type BallObservation struct {
	// RadiusM is the named local sensing radius in metres. Finite and > 0 when
	// the record is installed; it both enables sensing and defines the closed
	// metric circle.
	RadiusM float64 `json:"radiusM"`
	// Seen reports whether any observation has ever been made.
	Seen bool `json:"seen"`
	// ClockMS is the pre-step observation clock of the last successful sighting.
	ClockMS int `json:"clockMs"`
	// X/Y is the ball position (normalized pitch units) at the last sighting.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// VXM/VYM is the ball velocity (m/s) at the last sighting.
	VXM float64 `json:"vxM"`
	VYM float64 `json:"vyM"`
	// Mode is the ball mode at the last sighting.
	Mode BallMode `json:"mode"`
	// Owner is the held-owner index at the last sighting, -1 when unowned.
	Owner int `json:"owner"`
}

// ObservationFreshness classifies a runner's local observation at a query clock.
// It is DERIVED from the stored Seen/ClockMS, never a second persisted fact.
type ObservationFreshness uint8

// Observation freshness values.
const (
	// ObservationUnknown: the runner has never observed the ball.
	ObservationUnknown ObservationFreshness = iota
	// ObservationFresh: the observation was refreshed at exactly the query clock.
	ObservationFresh
	// ObservationStale: the runner has a record, but not from the query clock.
	ObservationStale
)

// String returns the stable label for traces and evidence.
func (f ObservationFreshness) String() string {
	switch f {
	case ObservationFresh:
		return "fresh"
	case ObservationStale:
		return "stale"
	default:
		return "unknown"
	}
}

// Freshness classifies the record at the query clock nowMS. A record is FRESH
// only at the exact publication clock that produced it; at any later clock it is
// STALE. A never-seen record is UNKNOWN.
func (o BallObservation) Freshness(nowMS int) ObservationFreshness {
	if !o.Seen {
		return ObservationUnknown
	}
	if o.ClockMS == nowMS {
		return ObservationFresh
	}
	return ObservationStale
}

// AgeMS returns nowMS - ClockMS for a seen record, or -1 when never seen.
func (o BallObservation) AgeMS(nowMS int) int {
	if !o.Seen {
		return -1
	}
	return nowMS - o.ClockMS
}

// SensingEnabled reports whether the record opts its runner into sampling. It
// fails CLOSED: a nil record, a non-finite radius (NaN comparison would be false
// and an +Inf radius would admit the whole pitch) or a non-positive radius all
// disable sensing. A raw record hand-set into Runner.Perception is therefore
// never trusted by the sampler.
func (o *BallObservation) SensingEnabled() bool {
	return o != nil && isFinite(o.RadiusM) && o.RadiusM > 0
}

// SenseEnabled reports whether this runner samples the local ball.
func (r Runner) SenseEnabled() bool { return r.Perception.SensingEnabled() }

// EnableSensing installs a fresh local-sensing record with the given metre
// radius. It validates the radius (finite, strictly positive) and returns an
// error rather than silently accepting broken input.
func (r *Runner) EnableSensing(radiusM float64) error {
	if math.IsNaN(radiusM) || math.IsInf(radiusM, 0) || radiusM <= 0 {
		return fmt.Errorf("%w: got %v", ErrSenseRadius, radiusM)
	}
	r.Perception = &BallObservation{RadiusM: radiusM}
	return nil
}

// DisableSensing removes the local-sensing record. It is the exact zero state:
// the runner no longer serializes a perception key.
func (r *Runner) DisableSensing() { r.Perception = nil }

// validBallMode reports whether m is one of the declared ball modes.
func validBallMode(m BallMode) bool { return m <= BallOut }

// validate checks a persisted observation record against the RESUMED context
// (snapshot clock and runner count). It always rejects a non-finite/non-positive
// radius and non-finite ball state. When Seen is true it additionally requires a
// self-consistent sighting: the timestamp cannot be in the future of the snapshot
// clock, the mode must be declared, and the owner must be a real runner index for
// a Held ball (or -1 for any unheld ball). A never-seen record skips the
// timestamp/mode/owner contract (its zero values are simply unused).
func (o *BallObservation) validate(nowMS, runnerCount int) error {
	if !isFinite(o.RadiusM) || o.RadiusM <= 0 {
		return fmt.Errorf("%w: radius %v", ErrSenseRadius, o.RadiusM)
	}
	if !isFinite(o.X) || !isFinite(o.Y) || !isFinite(o.VXM) || !isFinite(o.VYM) {
		return fmt.Errorf("%w: x=%v y=%v vx=%v vy=%v", ErrSenseCoordinates, o.X, o.Y, o.VXM, o.VYM)
	}
	if !o.Seen {
		return nil
	}
	if o.ClockMS < 0 || o.ClockMS > nowMS {
		return fmt.Errorf("%w: seen clock %d outside [0,%d]", ErrSenseSchema, o.ClockMS, nowMS)
	}
	if !validBallMode(o.Mode) {
		return fmt.Errorf("%w: mode %d", ErrSenseSchema, o.Mode)
	}
	if o.Mode == BallHeld {
		if o.Owner < 0 || o.Owner >= runnerCount {
			return fmt.Errorf("%w: held owner %d outside [0,%d)", ErrSenseSchema, o.Owner, runnerCount)
		}
	} else if o.Owner != -1 {
		return fmt.Errorf("%w: unheld owner %d, want -1", ErrSenseSchema, o.Owner)
	}
	return nil
}

// isFinite reports whether v is neither NaN nor an infinity.
func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// cloneObservation returns an independent copy of a record (nil stays nil), so
// no public state copy ever shares a live mutable pointer.
func cloneObservation(o *BallObservation) *BallObservation {
	if o == nil {
		return nil
	}
	c := *o
	return &c
}

// cloneRunners returns a deep copy of a runner slice: the structs AND every
// mutable perception record are copied, so callers cannot alias live state.
func cloneRunners(rs []Runner) []Runner {
	out := append([]Runner(nil), rs...)
	for i := range out {
		out[i].Perception = cloneObservation(out[i].Perception)
	}
	return out
}

// resumeRunners validates any persisted perception record against the resumed
// context (snapshot clock and runner count) and returns a deep copy of the runner
// slice, so a resumed sim never aliases the snapshot's pointers and never restarts
// from broken input.
//
// Trusted fixtures that set Runner.Perception DIRECTLY (same-package tests) bypass
// EnableSensing and are not validated by SetRunners (which has no error return);
// the sampler fails CLOSED on such a record at runtime, and only the validated
// import path (EnableSensing / Resume) rejects it up front.
func resumeRunners(rs []Runner, nowMS int) ([]Runner, error) {
	for i := range rs {
		if o := rs[i].Perception; o != nil {
			if err := o.validate(nowMS, len(rs)); err != nil {
				return nil, fmt.Errorf("runner %d: %w", i, err)
			}
		}
	}
	return cloneRunners(rs), nil
}

// samplePerception records the CURRENT pre-step ball state for every opted-in
// runner whose local circle contains the ball and whose reaction has arrived. It
// is called ONLY from the armed scheduler publication (never the native path), so
// it reads the pre-integration world and writes only perception records. The
// metric test is closed (`DistM <= RadiusM`); an out-of-range ball leaves the
// last-seen record untouched. Held/Dead/Out modes are recorded when visible.
//
// It fails CLOSED: a nil record or a sensing radius that is not finite-and-positive
// is skipped (never an all-world or NaN leak), the reaction gate is applied, and a
// non-finite ball/runner distance (e.g. a NaN coordinate) skips the write rather
// than being admitted by the `NaN > radius` false comparison.
func (s *Sim) samplePerception(stepStart int) {
	for i := range s.runners {
		r := &s.runners[i]
		o := r.Perception
		if !o.SensingEnabled() {
			continue
		}
		if s.clock < r.ReactAtMS {
			continue // reaction gate: never sense before the reaction arrives
		}
		d := DistM(s.ball.X, s.ball.Y, r.X, r.Y)
		if !isFinite(d) || d > o.RadiusM {
			continue // outside the local circle (or broken input): preserve last seen
		}
		o.Seen = true
		o.ClockMS = stepStart
		o.X, o.Y = s.ball.X, s.ball.Y
		o.VXM, o.VYM = s.ball.VXM, s.ball.VYM
		o.Mode = s.ball.Mode
		if s.ball.Mode == BallHeld {
			o.Owner = s.contactRunner
		} else {
			o.Owner = -1
		}
	}
}

// Perception returns a VALUE copy of a runner's observation record and whether
// the runner is opted in. It never exposes the live pointer.
func (s *Sim) Perception(index int) (BallObservation, bool) {
	if index < 0 || index >= len(s.runners) || s.runners[index].Perception == nil {
		return BallObservation{}, false
	}
	return *s.runners[index].Perception, true
}
