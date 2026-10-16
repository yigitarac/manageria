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
	// PassEventDeflect: a physical touch miscontrolled/deflected the ball into a
	// live loose trajectory (T-038); ownership stays unassigned.
	PassEventDeflect
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
	case PassEventDeflect:
		return "deflect"
	default:
		return "unknown"
	}
}

// PassEvent is one authoritative live-pass moment. Seq is monotonic within the
// Sim and survives a Resume; Runner is the passer on release, the controller or
// deflector on a contact, and -1 for path/stop/out events. Frac is the exact
// within-substep fraction (0 = step start, 1 = step end) at which a contact/exit
// event occurred, so a renderer can place the actual segment contact rather than
// the end-step endpoint (T-038).
type PassEvent struct {
	Seq     int           `json:"seq"`
	ClockMS int           `json:"clockMs"`
	Kind    PassEventKind `json:"kind"`
	Runner  int           `json:"runner"`
	BallX   float64       `json:"ballX"`
	BallY   float64       `json:"ballY"`
	VXM     float64       `json:"vxM,omitempty"`
	VYM     float64       `json:"vyM,omitempty"`
	Frac    float64       `json:"frac,omitempty"`
	// RunnerX/RunnerY are the touching runner's position at the fractional
	// contact instant (T-038). They are 0 for non-contact events, so a renderer
	// can place the actual contact pair even though the end-step sample keeps
	// every body at its common end-step state.
	RunnerX float64 `json:"runnerX,omitempty"`
	RunnerY float64 `json:"runnerY,omitempty"`
	// OutVXM/OutVYM are the ball's OUTGOING velocity after a contact (T-038):
	// the deflected velocity on a miscontrol, and zero on a clean control (held).
	// VXM/VYM are the INCOMING velocity at the contact instant. Together with
	// Frac this lets a renderer reconstruct the piecewise within-substep ball
	// path instead of inferring a bounce from the pre- and end-step samples.
	OutVXM float64 `json:"outVxM,omitempty"`
	OutVYM float64 `json:"outVyM,omitempty"`
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
	// ControlError is the contact-local control/execution uncertainty budget
	// (T-038), in [0,1]. A physical contact whose seeded draw falls below this
	// budget miscontrols/deflects into a live loose trajectory instead of clean
	// control. Zero (the default) draws nothing and always controls cleanly.
	ControlError float64
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

	// Live-contest state (T-038): a fresh contact record, the contact-local
	// control budget of THIS pass, and each runner's reaction activation deadline.
	s.controlError = p.ControlError
	s.firstTouchMS = -1
	s.firstTouchRunner = -1
	for i := range s.runners {
		s.runners[i].ReactAtMS = s.clock + s.runners[i].ReactMS
	}

	s.emitEvent(PassEventRelease, p.Passer, s.clock, s.ball)
}

// emitEvent appends one event to the log and bumps the monotonic sequence.
func (s *Sim) emitEvent(kind PassEventKind, runner, clock int, b Ball) {
	s.emitEventAt(kind, runner, clock, 0, b)
}

// emitEventAt appends one event with an exact within-substep fraction (T-038
// contacts/exits) to the log and bumps the monotonic sequence.
func (s *Sim) emitEventAt(kind PassEventKind, runner, clock int, frac float64, b Ball) {
	s.appendPassEvent(PassEvent{
		ClockMS: clock,
		Kind:    kind,
		Runner:  runner,
		BallX:   b.X,
		BallY:   b.Y,
		VXM:     b.VXM,
		VYM:     b.VYM,
		Frac:    frac,
	})
}

// emitContactEvent appends a physical-contact event. `b` carries the ball's
// FRACTIONAL contact position and its INCOMING velocity; outVX/outVY are the
// ball's OUTGOING velocity after the contact (zero for a held/clean control);
// runnerX/runnerY is the touching runner's position at that fraction.
func (s *Sim) emitContactEvent(kind PassEventKind, runner, clock int, frac float64, b Ball, outVX, outVY, runnerX, runnerY float64) {
	s.appendPassEvent(PassEvent{
		ClockMS: clock,
		Kind:    kind,
		Runner:  runner,
		BallX:   b.X,
		BallY:   b.Y,
		VXM:     b.VXM,
		VYM:     b.VYM,
		OutVXM:  outVX,
		OutVYM:  outVY,
		Frac:    frac,
		RunnerX: runnerX,
		RunnerY: runnerY,
	})
}

// appendPassEvent is the single authoritative emitter. It stamps the monotonic
// sequence, appends the event, then — in declared order — records the opt-in
// integrated fact synchronously at THIS emitter (T-055; a no-op when disabled),
// marks the scheduler invalidation and sets any local wakeup pending. Every
// gameplay event goes through here, so the integrated journal and the legacy log
// share one emission order by construction.
func (s *Sim) appendPassEvent(e PassEvent) {
	e.Seq = s.eventSeq
	s.events = append(s.events, e)
	s.eventSeq++
	s.noteIntegratedFact(e)
	s.noteDecisionEvent(e.Kind)
	s.noteLocalWake(e.Kind, e.BallX, e.BallY, e.Frac)
}
