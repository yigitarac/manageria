package liveplay

import "math"

// Inertial player motion (T-036).
//
// T-035 gave every runner a constant velocity so the contact clock could be
// studied in isolation. T-036 replaces that with a bounded acceleration /
// deceleration / turning model: a runner has a velocity vector, an explicit
// movement *intent* (a destination on the pitch) and named physical limits, and
// the integrator can never teleport or reverse instantaneously.
//
// This file adds motion ONLY. It does not touch the ball, the contact endpoint,
// the release sites or any production code. A runner opts in with
// MotionInertial; the zero value (MotionKinematic) keeps the exact T-035
// constant-velocity behaviour, so every existing spike fixture and its pinned
// first-contact time stay valid without a single change.
//
// Design invariants (same as the package): pure arithmetic and math.Sqrt only
// (no trig, no map iteration, no wall clock), deterministic, and every field
// needed to Resume mid-movement lives in Runner so a snapshot round-trips
// exactly.

// RunnerMotion selects how a runner is integrated each substep.
type RunnerMotion uint8

const (
	// MotionKinematic is the T-035 default: position advances by velocity*dt
	// with no acceleration, no heading change and no momentum. It is the zero
	// value on purpose, so existing fixtures keep their exact meaning.
	MotionKinematic RunnerMotion = iota
	// MotionInertial is the T-036 model: velocity moves toward the intent under
	// bounded acceleration, deceleration and lateral (turning) acceleration.
	MotionInertial
)

// String returns the stable mode label for traces and evidence.
func (m RunnerMotion) String() string {
	switch m {
	case MotionKinematic:
		return "kinematic"
	case MotionInertial:
		return "inertial"
	default:
		return "unknown"
	}
}

// Physical parameter envelope for player locomotion. Every constant is named
// with its unit and is an explicit design assumption (a plausible range for a
// fit outfielder), NOT a measured truth and NOT an outcome-fitting knob. The
// upper top-speed bound deliberately equals PlayerTopSpeedMPS from units.go so
// T-036 and the T-035 reachability fixtures agree on the same envelope.
const (
	// MinPlayerTopSpeedMPS / PlayerTopSpeedMPS bound the pace -> top-speed map.
	// A full sprint of a fit outfielder is ~7-8 m/s; the slow end of the 1-20
	// attribute range maps to a jog.
	MinPlayerTopSpeedMPS = 5.5
	// PlayerAccelMinMPS2 / PlayerAccelMaxMPS2 bound the acceleration -> accel
	// map. Human linear acceleration over a sprint is roughly 2-4 m/s^2 after
	// the first steps.
	PlayerAccelMinMPS2 = 1.5
	PlayerAccelMaxMPS2 = 4.0
	// PlayerLateralAccelMinMPS2 / PlayerLateralAccelMaxMPS2 bound the turning
	// (centripetal) acceleration available to change heading. Turning harder
	// than this is not physically possible at speed, which is exactly what
	// removes the T-035 instant 180-degree reversal.
	PlayerLateralAccelMinMPS2 = 1.0
	PlayerLateralAccelMaxMPS2 = 3.0
	// PlayerDecelMPS2 is the braking deceleration envelope. A player can shed
	// speed faster than he builds it (weight transfer + ground friction), so the
	// brake constant is above the acceleration envelope. Assumption, not measured.
	PlayerDecelMPS2 = 4.5
	// ArriveRadiusM is the distance at which an in-motion runner considers the
	// intent reached and commands zero speed. It is the same order as
	// ControlRadiusM and is an explicit assumption.
	ArriveRadiusM = 0.25
	// FatigueSpeedShare mirrors the production engine's move() fatigue term
	// eff = 1 - 0.3*min(Fatigue/100, 1): at full fatigue a player keeps 70% of
	// his top speed and acceleration. Reusing the documented production
	// relation keeps the two models talking about the same thing.
	FatigueSpeedShare = 0.30
)

// MotionParams are the resolved physical limits of one inertial runner. They are
// stored on the Runner (and therefore in the Snapshot) so a resumed run
// integrates identically; the attribute mapping below is the provenance.
type MotionParams struct {
	// TopSpeedMPS: maximum ground speed in metres per second.
	TopSpeedMPS float64 `json:"topSpeedMps"`
	// AccelMPS2: maximum rate of increasing speed (m/s^2).
	AccelMPS2 float64 `json:"accelMps2"`
	// DecelMPS2: maximum rate of decreasing speed (m/s^2).
	DecelMPS2 float64 `json:"decelMps2"`
	// LateralAccelMPS2: maximum centripetal acceleration for changing heading
	// (m/s^2). Bounds turn rate as lateral/v at speed.
	LateralAccelMPS2 float64 `json:"lateralAccelMps2"`
}

// DefaultMotionParams returns the envelope maximum (an attribute-20, fresh
// player). Used when an inertial runner is created without explicit parameters.
func DefaultMotionParams() MotionParams {
	return MotionParams{
		TopSpeedMPS:      PlayerTopSpeedMPS,
		AccelMPS2:        PlayerAccelMaxMPS2,
		DecelMPS2:        PlayerDecelMPS2,
		LateralAccelMPS2: PlayerLateralAccelMaxMPS2,
	}
}

// MotionParamsFromAttributes maps the production 1-20 Pace and Acceleration
// attributes and the production 0-100 Fatigue to physical limits. Higher pace
// raises top speed, higher acceleration raises both linear acceleration and
// turning response, and fatigue scales speed/acceleration down by the shared
// production relation. This is a mapping of *inputs to physics*, never a
// per-tactic outcome bonus.
func MotionParamsFromAttributes(pace, acceleration uint8, fatigue100 float64) MotionParams {
	eff := 1 - FatigueSpeedShare*clamp01(fatigue100/100)
	pq := clamp01(float64(pace) / 20)
	aq := clamp01(float64(acceleration) / 20)
	return MotionParams{
		TopSpeedMPS:      (MinPlayerTopSpeedMPS + (PlayerTopSpeedMPS-MinPlayerTopSpeedMPS)*pq) * eff,
		AccelMPS2:        (PlayerAccelMinMPS2 + (PlayerAccelMaxMPS2-PlayerAccelMinMPS2)*aq) * eff,
		DecelMPS2:        PlayerDecelMPS2 * eff,
		LateralAccelMPS2: (PlayerLateralAccelMinMPS2 + (PlayerLateralAccelMaxMPS2-PlayerLateralAccelMinMPS2)*aq) * eff,
	}
}

// clamp01 clamps to [0,1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// InertialRunner builds a runner in the inertial mode with an intent and
// resolved parameters. It is the fixture-friendly constructor; production-like
// callers can build via MotionParamsFromAttributes, then call SetMotionParams.
func InertialRunner(x, y, intentX, intentY float64, p MotionParams) Runner {
	r := Runner{Motion: MotionInertial, X: x, Y: y, IntentX: intentX, IntentY: intentY}
	r.SetMotionParams(p)
	return r
}

// SetMotionParams copies resolved limits onto the runner.
func (r *Runner) SetMotionParams(p MotionParams) {
	r.TopSpeedMPS = p.TopSpeedMPS
	r.AccelMPS2 = p.AccelMPS2
	r.DecelMPS2 = p.DecelMPS2
	r.LateralAccelMPS2 = p.LateralAccelMPS2
}

// Params returns the runner's resolved limits, substituting the default envelope
// for any non-positive field (so a hand-built inertial runner still integrates).
func (r Runner) Params() MotionParams {
	d := DefaultMotionParams()
	p := MotionParams{
		TopSpeedMPS:      r.TopSpeedMPS,
		AccelMPS2:        r.AccelMPS2,
		DecelMPS2:        r.DecelMPS2,
		LateralAccelMPS2: r.LateralAccelMPS2,
	}
	if p.TopSpeedMPS <= 0 {
		p.TopSpeedMPS = d.TopSpeedMPS
	}
	if p.AccelMPS2 <= 0 {
		p.AccelMPS2 = d.AccelMPS2
	}
	if p.DecelMPS2 <= 0 {
		p.DecelMPS2 = d.DecelMPS2
	}
	if p.LateralAccelMPS2 <= 0 {
		p.LateralAccelMPS2 = d.LateralAccelMPS2
	}
	return p
}

// SpeedMPS returns the runner's current ground speed.
func (r Runner) SpeedMPS() float64 {
	return math.Sqrt(r.VXM*r.VXM + r.VYM*r.VYM)
}

// SetIntent sets a runner's movement destination (normalized pitch units). The
// intent is deliberately separate from integration: participants publish their
// intent before Step, and integration reads only each runner's OWN state and
// intent, so there is no accidental update-order advantage (no runner sees
// another runner's freshly-integrated position).
func (s *Sim) SetIntent(index int, x, y float64) {
	if index < 0 || index >= len(s.runners) {
		return
	}
	s.runners[index].IntentX, s.runners[index].IntentY = x, y
}

// integrateInertial advances one inertial runner by dt seconds. It is pure
// arithmetic plus math.Sqrt:
//
//  1. desired velocity: move toward the intent, braking-limited so the runner
//     can stop at the destination (v = min(top, sqrt(2*decel*d))) and never
//     overshoot within a step (v <= d/dt);
//  2. decompose the needed velocity change into an along-heading component
//     (speed change: bounded by Accel when speeding up, Decel when slowing) and
//     a lateral component (direction change: bounded by LateralAccelMPS2);
//  3. add the bounded change, then clamp the result to the top speed; and
//  4. advance the position with the bounded velocity.
//
// A head-on reversal therefore decelerates through zero and accelerates back:
// the T-035 instant reversal is impossible by construction, without any trig.
func (r *Runner) integrateInertial(dt float64) {
	p := r.Params()

	// 1. Desired velocity from the intent.
	dxm := (r.IntentX - r.X) * PitchLengthM
	dym := (r.IntentY - r.Y) * PitchWidthM
	distM := math.Sqrt(dxm*dxm + dym*dym)

	var desiredVXM, desiredVYM float64
	if distM > ArriveRadiusM {
		want := p.TopSpeedMPS
		if brake := math.Sqrt(2 * p.DecelMPS2 * distM); brake < want {
			want = brake
		}
		if step := distM / dt; step < want {
			want = step
		}
		inv := 1 / distM
		// Unit direction in metres, then expressed as m/s along each axis.
		desiredVXM = dxm * inv * want
		desiredVYM = dym * inv * want
	}

	speed := math.Sqrt(r.VXM*r.VXM + r.VYM*r.VYM)

	// 2a. From rest: accelerate straight toward the desired velocity.
	if speed < 1e-9 {
		ddx, ddy := desiredVXM-r.VXM, desiredVYM-r.VYM
		dmag := math.Sqrt(ddx*ddx + ddy*ddy)
		maxStep := p.AccelMPS2 * dt
		if dmag > maxStep && dmag > 0 {
			scale := maxStep / dmag
			ddx *= scale
			ddy *= scale
		}
		r.VXM += ddx
		r.VYM += ddy
	} else {
		// 2b. Decompose the change into along-heading and lateral components.
		hx, hy := r.VXM/speed, r.VYM/speed
		ddx, ddy := desiredVXM-r.VXM, desiredVYM-r.VYM
		along := ddx*hx + ddy*hy
		latx := ddx - along*hx
		laty := ddy - along*hy

		// Bound the speed change (accel when speeding up, decel when slowing).
		capAlong := p.AccelMPS2 * dt
		if along < 0 {
			capAlong = p.DecelMPS2 * dt
		}
		if along > capAlong {
			along = capAlong
		} else if along < -capAlong {
			along = -capAlong
		}

		// Bound the lateral (turning) change.
		latMag := math.Sqrt(latx*latx + laty*laty)
		capLat := p.LateralAccelMPS2 * dt
		if latMag > capLat && latMag > 0 {
			scale := capLat / latMag
			latx *= scale
			laty *= scale
		}

		r.VXM += along*hx + latx
		r.VYM += along*hy + laty
	}

	// 3. Numerical guard: never exceed the top speed.
	r.clampToTopSpeed(p.TopSpeedMPS)

	// 4. Advance position with the (now bounded) velocity.
	r.X += (r.VXM / PitchLengthM) * dt
	r.Y += (r.VYM / PitchWidthM) * dt
}

// clampToTopSpeed scales the velocity down if it exceeded the top speed.
func (r *Runner) clampToTopSpeed(top float64) {
	if top <= 0 {
		return
	}
	if speed := r.SpeedMPS(); speed > top && speed > 0 {
		scale := top / speed
		r.VXM *= scale
		r.VYM *= scale
	}
}
