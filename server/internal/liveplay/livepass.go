package liveplay

import "math"

// Authoritative live ground pass (T-037).
//
// T-035/T-036 modelled the ball and the players. T-037 makes one unopposed
// ground pass a real causal episode on that clock:
//
//   - the ball is released as a rolling vector with a seeded execution error
//     (aim and weight), NOT as an already-decided successful action;
//   - ownership stays unassigned while the ball travels; the intended receiver
//     is an aim point, not a preselected winner — whoever physically reaches the
//     ball first controls it;
//   - the receiver moves during flight, so bad weighting/direction can overrun,
//     stop short (a still-collectable loose ball) or send the ball out of play;
//   - release, path and control are recorded as separate events on the same
//     integer clock;
//   - contact preserves the physical ball position (no warp to the feet), and a
//     mid-flight Snapshot/Resume reproduces the remaining travel and control.
//
// Everything here is opt-in via Config.LivePass; the legacy endpoint prototype
// is byte-for-byte unchanged. This file does not touch production code, shots,
// aerials or team AI, and deliberately does NOT implement T-038 swept
// interception / reaction / stochastic deflection.

// Live-pass physical constants. Named, unit-tagged, explicit assumptions — not
// measured truths and not outcome-fitting knobs.
const (
	// PasserImmunityMS is the minimum time after a release before the releasing
	// player may re-control the same ball. A player cannot meaningfully play the
	// ball to himself instantly; this is a persisted physical re-control cooldown,
	// not an outcome roll. Explicit assumption, not measured.
	PasserImmunityMS = 400
	// PathRecordMS is the cadence at which explicit path events trace the free
	// travel (position only), decoupled from the physics substep.
	PathRecordMS = 100
)

// PassEventKind labels one entry in the live-pass event log. Release, path and
// control are recorded separately and share the integer simulation clock.
type PassEventKind uint8

// Declared live-pass event kinds.
const (
	// PassEventRelease: the ball left the passer's foot.
	PassEventRelease PassEventKind = iota
	// PassEventPath: a periodic position trace of the free travel.
	PassEventPath
	// PassEventControl: a runner physically reached and controlled the ball.
	PassEventControl
	// PassEventStop: the rolling ball came to rest, still live and collectable.
	PassEventStop
	// PassEventOut: the ball left the field of play (dead, restart).
	PassEventOut
)

// String returns the stable label for traces and evidence. Unknown kinds return
// "unknown" rather than panicking.
func (k PassEventKind) String() string {
	switch k {
	case PassEventRelease:
		return "release"
	case PassEventPath:
		return "path"
	case PassEventControl:
		return "control"
	case PassEventStop:
		return "stop"
	case PassEventOut:
		return "out"
	default:
		return "unknown"
	}
}

// PassEvent is one authoritative live-pass moment. Seq is monotonic within the
// Sim and survives a Resume; Runner is the passer on release, the controller on
// control, and -1 for path/stop/out events.
type PassEvent struct {
	Seq     int           `json:"seq"`
	ClockMS int           `json:"clockMs"`
	Kind    PassEventKind `json:"kind"`
	Runner  int           `json:"runner"`
	BallX   float64       `json:"ballX"`
	BallY   float64       `json:"ballY"`
	VXM     float64       `json:"vxM,omitempty"`
	VYM     float64       `json:"vyM,omitempty"`
}

// PassParams is the physical intent of one live ground-pass release. TargetX/Y
// is the AIM (a receiver intent), not a selected winner; Passer is the releasing
// runner index (-1 if none) and is shielded from immediate re-collection.
type PassParams struct {
	StartX, StartY     float64 // normalized release point
	TargetX, TargetY   float64 // normalized aim point (receiver intent)
	SpeedMPS           float64 // intended ball speed along the aim (weight)
	DirectionJitterMPS float64 // ± perpendicular velocity execution error (m/s)
	WeightJitterFrac   float64 // ± fraction of SpeedMPS weight error
	Passer             int     // releasing runner index for immunity, -1 if none
}

// LivePassConfig returns the recommended live-pass configuration.
func LivePassConfig() Config {
	return Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2, LivePass: true}
}

// ReleasePass launches a rolling ground pass. Draw order is fixed and declared:
// direction error, then weight error (see release.go). Ownership is left
// unassigned (contactMS reset to -1) and no success is decided: the ball simply
// starts travelling, and the outcome is whatever the physics produces.
//
// The aim direction is the unit start->target vector expressed directly in
// metres/second components (no libm trig); the direction error is a signed
// perpendicular velocity so arbitrary pass directions are supported.
func (s *Sim) ReleasePass(p PassParams) {
	dirErr := (s.rnd.Float64()*2 - 1) * p.DirectionJitterMPS  // DrawPassDirection
	weightErr := (s.rnd.Float64()*2 - 1) * p.WeightJitterFrac // DrawPassWeight

	dx := (p.TargetX - p.StartX) * PitchLengthM
	dy := (p.TargetY - p.StartY) * PitchWidthM
	d := math.Sqrt(dx*dx + dy*dy)
	var ux, uy float64
	if d > 0 {
		ux, uy = dx/d, dy/d
	} else {
		// Degenerate aim (target == start): default to a straight +x pass.
		ux, uy = 1, 0
	}
	// Perpendicular unit in metric space (rotate +90 without trig).
	px, py := -uy, ux

	speed := p.SpeedMPS * (1 + weightErr)
	s.ball = Ball{
		Mode: BallRolling,
		X:    p.StartX,
		Y:    p.StartY,
		VXM:  ux*speed + px*dirErr,
		VYM:  uy*speed + py*dirErr,
	}

	// Ownership is unassigned during travel; a release does not flip possession.
	s.contactMS = -1
	s.contactRunner = -1
	s.releaseRunner = p.Passer
	s.immunityUntil = s.clock + PasserImmunityMS
	s.lastPathMS = s.clock

	s.emitEvent(PassEventRelease, p.Passer, s.clock, s.ball)
}

// emitEvent appends one event to the log and bumps the monotonic sequence.
func (s *Sim) emitEvent(kind PassEventKind, runner, clock int, b Ball) {
	s.events = append(s.events, PassEvent{
		Seq:     s.eventSeq,
		ClockMS: clock,
		Kind:    kind,
		Runner:  runner,
		BallX:   b.X,
		BallY:   b.Y,
		VXM:     b.VXM,
		VYM:     b.VYM,
	})
	s.eventSeq++
}
