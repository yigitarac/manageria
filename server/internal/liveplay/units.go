// Package liveplay is an isolated, deterministic micro-step prototype for the
// causal live-play design requested by T-035. It is deliberately NOT part of the
// production match engine: it imports nothing from internal/engine, touches no
// production snapshot/golden/EngineVersion, and exists only to measure the clock,
// units, contact-timing and cost contract before any migration is authorized
// (see vault/06-Decisions/ADR-0014-authoritative-live-play.md).
//
// Design invariants (mirroring the production determinism rules):
//   - Pure: no I/O, no wall clock, no goroutines, no map iteration whose order
//     can affect results.
//   - Randomness only from internal/rng, with a declared, fixed draw order.
//   - Floating point uses only +, -, *, / and math.Sqrt (correctly rounded by
//     IEEE-754; unlike libm trig/hypot it is bit-identical across platforms).
package liveplay

import "math"

// Pitch geometry. Normalized pitch coordinates run x,y in [0,1]: x along the
// goal-to-goal length, y across the width. Home attacks +x and away attacks -x,
// matching the production engine's orientation. One normalized x-unit is
// PitchLengthM metres, one y-unit is PitchWidthM metres.
//
// These constants are the unit bridge between the engine's normalized spatial
// model and the metre/second physics the causal design needs. They are a design
// choice (a full-size FIFA pitch), not a measured value.
const (
	PitchLengthM = 105.0
	PitchWidthM  = 68.0
)

// Physical parameter ranges and the recommended spike constants. Every value is
// named with its unit and meaning; none of them is an outcome-fitting knob.
const (
	// GroundPassMinMPS / GroundPassMaxMPS bound a rolled ground pass. A firm
	// 25 m ground pass leaves the foot around 14-18 m/s; 30 m/s is a hard driven
	// ball. Used only to size fixtures.
	GroundPassMinMPS = 6.0
	GroundPassMaxMPS = 30.0
	// ShotMaxMPS is the extreme registered-ball speed for swept-contact fixtures.
	ShotMaxMPS = 40.0
	// PlayerTopSpeedMPS is the fastest a runner moves in the reachability
	// fixtures (conservative, above the production pace cap).
	PlayerTopSpeedMPS = 8.5
	// RollDecelMPS2 is grass rolling friction for a ground ball: constant
	// deceleration opposing velocity. ~0.5-0.8 m/s^2 is a documented plausible
	// range; 0.6 is the spike value.
	RollDecelMPS2 = 0.6
	// ControlRadiusM is the combined ball+player control reach: the ball is
	// collected when its centre comes within this distance of the runner's
	// reference point. 1.0 m is a deliberate, explicit assumption (foot reach
	// plus ball radius); it is a parameter to calibrate later, not a measured
	// truth.
	ControlRadiusM = 1.0
	// SubstepMS is the recommended authoritative substep (integer game
	// milliseconds). It is the value the T-035 sweep selects on error/cost
	// evidence; see sweep_test.go. 20 ms = 50 Hz.
	SubstepMS = 20
	// DecisionTickMS is the recommended cadence at which intents/decisions are
	// re-evaluated (and RNG is drawn), decoupled from the physics substep so the
	// draw count does not scale with the substep rate.
	DecisionTickMS = 100
	// SampleMS is the authoritative sampling cadence written to the render/trace
	// contract. Equal to the substep so each sample lands on the integration grid.
	// Sampling is NOT free: it appends a Sample per substep (see the sampling
	// benchmark), so a production design should sample the render contract at a
	// coarser cadence than the physics substep.
	SampleMS = SubstepMS
	// ReferenceSubstepMS is the 1 ms NUMERICAL REFERENCE integration used to size
	// candidate clocks. It is not an analytic or exact truth: a tangent/glancing
	// trajectory can be missed by any finite step, so the reference only bounds
	// errors for the fixture grid, not universally.
	ReferenceSubstepMS = 1
)

// LengthM converts a normalized x displacement to metres.
func LengthM(xNorm float64) float64 { return xNorm * PitchLengthM }

// WidthM converts a normalized y displacement to metres.
func WidthM(yNorm float64) float64 { return yNorm * PitchWidthM }

// DistM is the Euclidean distance in metres between two normalized points.
// math.Sqrt is used directly (not math.Hypot) because Sqrt is a correctly
// rounded IEEE-754 operation with less implementation variance than Hypot; this
// reduces, but does not guarantee, cross-platform bit-identity.
func DistM(ax, ay, bx, by float64) float64 {
	dx := (bx - ax) * PitchLengthM
	dy := (by - ay) * PitchWidthM
	return math.Sqrt(dx*dx + dy*dy)
}

// Accepted contact-timing error limits for the recommended clock. These are the
// explicit numeric acceptance numbers from T-035 that later tasks must not
// regress. They are asserted in sweep_test.go against the 1 ms reference.
const (
	// MaxCrossingTimeErrorMS: the worst contact-time lag over the fixture sweep
	// must not exceed one substep (20 ms).
	MaxCrossingTimeErrorMS = SubstepMS
	// RequiredTunnelingMisses, RequiredFalseContacts and RequiredClassErrors are
	// the accepted fixture counts: the sweep must be exactly clean.
	RequiredTunnelingMisses = 0
	RequiredFalseContacts   = 0
	RequiredClassErrors     = 0
)
