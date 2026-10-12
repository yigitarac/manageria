package liveplay

import "math"

// In-flight interception and loose-ball pursuit (T-038).
//
// T-037 released a live ground pass and let the *endpoint* sample decide control:
// the ball was tested against every runner once per substep, so a fast ball could
// tunnel between two samples and a grazing glance was missed entirely. T-038 adds
// a swept, analytic contact test between the ball's substep chord and each
// runner's substep chord (relative motion) and resolves the substep as a sequence
// of chronologically ordered physical events.
//
// The substep is a FULL continuation: the ball and every runner are integrated to
// the common end-step once, pre-step states are captured explicitly, and a contact
// does NOT rewind the touching runner or discard its remaining travel. A clean
// control starts the ball following its owner for the rest of the substep (so the
// end-step sample physically differs from the fractional contact event); a
// miscontrol integrates the remaining fraction under the deflected velocity and
// may meet another runner or leave the pitch in the SAME substep. Every event
// carries the integer end-step clock plus an exact within-substep fraction `Frac`,
// so a renderer places the actual segment contact rather than an end-step warp.
//
// Everything here is live-mode only (Config.LivePass). The legacy endpoint
// prototype keeps its deliberate grazing-tunnelling limitation for diagnosis.

// Live-contest physical constants. Named, unit-tagged, explicit assumptions — not
// measured truths and not outcome-fitting knobs.
const (
	// DeflectionCooldownMS is the minimum time after a miscontrol before the
	// miscontrolling player may re-control the same ball. It stops a same-body
	// instant recapture loop (touch → bounce → touch) without an outcome roll.
	DeflectionCooldownMS = 250
	// DeflectRetainFrac is the share of the incoming ball speed carried by a
	// miscontrol deflection.
	DeflectRetainFrac = 0.45
	// DeflectPushMPS is the base speed the ball is pushed away from the
	// miscontrolling player at contact.
	DeflectPushMPS = 2.0
	// DeflectLateralMPS is the ± seeded lateral perturbation of a deflection.
	DeflectLateralMPS = 1.5
)

// Chronological event kinds considered within one live substep.
const (
	evNone = iota
	evContact
	evOut
	evStop
)

// PursuingRunner builds an inertial runner whose intent is republished to chase a
// free ball every substep once reactMS of reaction time has elapsed. reactMS = 0
// reacts immediately.
func PursuingRunner(x, y float64, p MotionParams, reactMS int) Runner {
	r := InertialRunner(x, y, x, y, p)
	r.Pursue = true
	r.ReactMS = reactMS
	return r
}

// publishPursuitIntents republishes each in-motion pursuer's intent to the ball's
// current position. It runs at the very START of a live substep, before any
// integration, so every intent is derived from the same consistent PRE-step world
// and integration reads only each runner's own state and intent (no update-order
// advantage).
//
// Reaction gates SENSING, not locomotion: a pursuer whose ReactAtMS has not yet
// arrived keeps integrating its pre-existing intent (its ongoing motion) and is
// skipped by the swept-contact gate for the inactive part of the substep. A
// zero-delay pursuer (ReactMS 0, ReactAtMS == release clock) reacts immediately.
func (s *Sim) publishPursuitIntents() {
	if s.ball.Mode != BallRolling && s.ball.Mode != BallLoose {
		return
	}
	for i := range s.runners {
		r := &s.runners[i]
		if r.LocalPursuit {
			// T-044: the native oracle ALWAYS skips a local runner, regardless of
			// the scheduler state, the sensing radius or a missing observation.
			// There is deliberately no fallback to the remote ball position.
			continue
		}
		if !r.Pursue || r.Motion != MotionInertial {
			continue
		}
		if s.clock < r.ReactAtMS {
			continue // not yet sensed the ball; keeps its inherited motion intent
		}
		r.IntentX, r.IntentY = s.ball.X, s.ball.Y
	}
}

// ensurePreCapacity grows the persistent pre-step scratch storage. It is also
// called from the runner mutators so the per-substep capture never allocates.
func (s *Sim) ensurePreCapacity(n int) {
	if cap(s.preX) < n {
		s.preX = make([]float64, n)
		s.preY = make([]float64, n)
	}
}

// capturePreStep copies the pre-integration ball and runner reference positions
// into reusable storage, so the live resolver builds exact substep chords instead
// of recovering them from post-integration velocities. It does not allocate once
// the scratch has been sized.
func (s *Sim) capturePreStep() {
	s.preBall = s.ball
	n := len(s.runners)
	s.ensurePreCapacity(n)
	s.preX = s.preX[:n]
	s.preY = s.preY[:n]
	for i := range s.runners {
		s.preX[i], s.preY[i] = s.runners[i].X, s.runners[i].Y
	}
}

// segmentContact returns the earliest global fraction u in [uMin, 1] at which the
// ball's current substep segment enters the reach disc of the runner chord
// [r0 -> r1]. The ball is at normalized `pstart` at global fraction `uCur` and at
// normalized `end` at fraction 1; the runner is at `r0` at fraction 0 and `r1` at
// fraction 1. Both motions are affine in u, so the relative position is affine and
// the entry is the smaller root of |rel(u)|² = reach². If the ball is already
// inside reach at uMin (e.g. a reaction activating beside the ball) it returns
// uMin. Work is done in metres so the reach disc is a true circle.
func segmentContact(px, py, uCur, ex, ey, r0x, r0y, r1x, r1y, reach, uMin float64) (float64, bool) {
	if uCur >= 1 {
		return 0, false
	}
	inv := 1 / (1 - uCur)
	// Ball per-unit-global-fraction rate (metres per unit u).
	sx := (ex - px) * PitchLengthM * inv
	sy := (ey - py) * PitchWidthM * inv
	// rel(u) = A + u·D, in metres.
	ax := px*PitchLengthM - r0x*PitchLengthM
	ay := py*PitchWidthM - r0y*PitchWidthM
	dx := sx - (r1x-r0x)*PitchLengthM
	dy := sy - (r1y-r0y)*PitchWidthM

	qx := ax + uMin*dx
	qy := ay + uMin*dy
	if qx*qx+qy*qy <= reach*reach {
		return uMin, true
	}

	a := dx*dx + dy*dy
	if a == 0 {
		return 0, false // no relative motion, starts outside
	}
	b := 2 * (ax*dx + ay*dy)
	c := ax*ax + ay*ay - reach*reach
	disc := b*b - 4*a*c
	if disc < 0 {
		return 0, false
	}
	sq := math.Sqrt(disc)
	u1 := (-b - sq) / (2 * a) // entry root
	u2 := (-b + sq) / (2 * a) // exit root
	if u2 < uMin || u1 > 1 {
		return 0, false
	}
	entry := u1
	if entry < uMin {
		entry = uMin
	}
	if entry > 1 {
		return 0, false
	}
	return entry, true
}

// segmentExit returns the earliest global fraction u in [uCur, 1] at which the
// ball segment leaves the pitch, or ok=false if it stays in play. A ball resting
// exactly on a boundary line is IN play (explicit tangency rule): only an endpoint
// strictly outside [0,1]² counts.
func segmentExit(px, py, ex, ey, uCur float64) (float64, bool) {
	if !(ex < 0 || ex > 1 || ey < 0 || ey > 1) {
		return 0, false
	}
	t := 2.0
	if ex < 0 || ex > 1 {
		bound := 1.0
		if ex < 0 {
			bound = 0
		}
		t = (bound - px) / (ex - px)
	}
	if ey < 0 || ey > 1 {
		bound := 1.0
		if ey < 0 {
			bound = 0
		}
		if ty := (bound - py) / (ey - py); ty < t {
			t = ty
		}
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return uCur + t*(1-uCur), true
}

// segmentPoint returns the ball position at global fraction u on the segment that
// runs from (px,py) at fraction uCur to (ex,ey) at fraction 1.
func segmentPoint(px, py, uCur, ex, ey, u float64) (float64, float64) {
	if uCur >= 1 {
		return ex, ey
	}
	f := (u - uCur) / (1 - uCur)
	return px + f*(ex-px), py + f*(ey-py)
}

// deflectVelocity computes the loose-ball velocity after a miscontrol, given the
// incoming ball velocity (vx,vy) and the ball/defender contact positions. The ball
// keeps a share of its incoming velocity, is pushed away from the defender, and
// takes a seeded lateral perturbation (DrawDeflectLateral).
func (s *Sim) deflectVelocity(vx, vy, ballX, ballY, defenderX, defenderY float64) (float64, float64) {
	dx := (ballX - defenderX) * PitchLengthM
	dy := (ballY - defenderY) * PitchWidthM
	d := math.Sqrt(dx*dx + dy*dy)
	var nx, ny float64
	if d > 1e-9 {
		nx, ny = dx/d, dy/d
	} else if sp := math.Sqrt(vx*vx + vy*vy); sp > 1e-9 {
		nx, ny = vx/sp, vy/sp
	} else {
		nx, ny = 1, 0
	}
	lat := (s.rnd.Float64()*2 - 1) * DeflectLateralMPS // DrawDeflectLateral
	return DeflectRetainFrac*vx + nx*DeflectPushMPS - ny*lat,
		DeflectRetainFrac*vy + ny*DeflectPushMPS + nx*lat
}

// eventTimeMS returns the declared integer-millisecond time of an event at
// global fraction u within the substep that began at stepStart. The rounding is
// explicit and monotone (round to nearest substep millisecond).
func eventTimeMS(stepStart, substepMS int, u float64) int {
	if u <= 0 {
		return stepStart
	}
	if u >= 1 {
		return stepStart + substepMS
	}
	return stepStart + int(math.Round(u*float64(substepMS)))
}
