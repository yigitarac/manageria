package liveplay

import (
	"errors"
	"fmt"
	"math"
)

// T-056 independent facing and oriented arrival (opt-in, isolated).
//
// A runner may carry an optional RunnerOrientation pointer. When it is nil the
// runner is integrated by the EXACT pre-T056 arithmetic (motion.go), so the
// disabled state/JSON/physics/RNG are byte-identical. Orientation is only valid
// on an inertial body with a stable nonzero PlayerID on the integrated path
// (T-055); it makes the body turn toward its intent under a bounded, trig-free
// chord, caps its desired speed by facing alignment (backward share), slows as it
// arrives, and scales a release's execution jitter by kick alignment.
//
// All parameters are named prototype ASSUMPTIONS, not real-world calibration. No
// new RNG site is added: the release still draws exactly direction then weight.

// Default orientation parameters. Explicit assumptions, resolved at activation by
// a constructor, never lazily defaulted from a zero value.
const (
	// DefaultStandTurnRadsPerSec is the standing (low-speed) facing turn limit.
	DefaultStandTurnRadsPerSec = 4.0
	// DefaultBackwardSpeedShare is the fraction of top speed allowed when the
	// facing is fully opposed to the movement direction.
	DefaultBackwardSpeedShare = 0.45
	// DefaultSlowingRadiusM is the distance at which arrival slowing begins.
	DefaultSlowingRadiusM = 3.0
	// DefaultKickAlignmentSpread is the base kick-alignment spread coefficient.
	DefaultKickAlignmentSpread = 1.0
)

// Orientation errors. Broken or impossible input is reported, never coerced.
var (
	// ErrOrientationNoIntegrated: orientation is only valid on the integrated path.
	ErrOrientationNoIntegrated = errors.New("liveplay: orientation requires the integrated shell")
	// ErrOrientationNotInitial: orientation can only be added at clock 0 before events.
	ErrOrientationNotInitial = errors.New("liveplay: orientation must be added at clock 0 before any event")
	// ErrOrientationMotion: orientation requires the inertial motion model.
	ErrOrientationMotion = errors.New("liveplay: orientation requires inertial motion")
	// ErrOrientationPlayerID: orientation requires a stable nonzero PlayerID.
	ErrOrientationPlayerID = errors.New("liveplay: orientation requires a stable nonzero player id")
	// ErrOrientationFacing: the facing vector must be finite and nonzero (unit).
	ErrOrientationFacing = errors.New("liveplay: orientation facing must be finite and nonzero")
	// ErrOrientationSide: the turn side must be -1, +1, or 0 (unspecified at input).
	ErrOrientationSide = errors.New("liveplay: orientation turn side must be -1, +1 or 0")
	// ErrOrientationParams: an orientation limit is non-finite or out of range.
	ErrOrientationParams = errors.New("liveplay: orientation parameter is non-finite or out of range")
	// ErrOrientationIndex: the requested runner index is out of range.
	ErrOrientationIndex = errors.New("liveplay: orientation runner index out of range")
	// ErrOrientationModel: an import selected model1 while carrying orientation.
	ErrOrientationModel = errors.New("liveplay: orientation requires the oriented model version")
)

// RunnerOrientation is the persisted orientation state of one inertial runner.
// FX/FY are a unit facing vector; TurnSide is ±1; the remaining fields are the
// resolved physical limits.
type RunnerOrientation struct {
	FX                  float64 `json:"fx"`
	FY                  float64 `json:"fy"`
	TurnSide            int     `json:"turnSide"`
	StandTurnRadsPerSec float64 `json:"standTurnRadsPerSec"`
	BackwardSpeedShare  float64 `json:"backwardSpeedShare"`
	SlowingRadiusM      float64 `json:"slowingRadiusM"`
	KickAlignmentSpread float64 `json:"kickAlignmentSpread"`
}

// cloneOrientation returns an independent copy (nil stays nil).
func cloneOrientation(o *RunnerOrientation) *RunnerOrientation {
	if o == nil {
		return nil
	}
	c := *o
	return &c
}

// NewOrientation builds a resolved orientation with the named defaults and the
// supplied facing direction (normalized) and turn side. turnSide 0 means
// "unspecified", resolved from PlayerID parity at activation. It returns an error
// rather than accepting a zero/non-finite facing or an out-of-range parameter.
func NewOrientation(dirX, dirY float64, turnSide int) (RunnerOrientation, error) {
	if turnSide != -1 && turnSide != 0 && turnSide != 1 {
		return RunnerOrientation{}, fmt.Errorf("%w: got %d", ErrOrientationSide, turnSide)
	}
	n := math.Sqrt(dirX*dirX + dirY*dirY)
	if !isFinite(n) || n == 0 {
		return RunnerOrientation{}, fmt.Errorf("%w: (%v,%v)", ErrOrientationFacing, dirX, dirY)
	}
	o := RunnerOrientation{
		FX:                  dirX / n,
		FY:                  dirY / n,
		TurnSide:            turnSide,
		StandTurnRadsPerSec: DefaultStandTurnRadsPerSec,
		BackwardSpeedShare:  DefaultBackwardSpeedShare,
		SlowingRadiusM:      DefaultSlowingRadiusM,
		KickAlignmentSpread: DefaultKickAlignmentSpread,
	}
	if err := o.validateParams(); err != nil {
		return RunnerOrientation{}, err
	}
	return o, nil
}

// validateParams checks the finite/range contract of the physical limits.
func (o RunnerOrientation) validateParams() error {
	if !isFinite(o.StandTurnRadsPerSec) || o.StandTurnRadsPerSec <= 0 {
		return fmt.Errorf("%w: standTurnRadsPerSec %v", ErrOrientationParams, o.StandTurnRadsPerSec)
	}
	if !isFinite(o.BackwardSpeedShare) || o.BackwardSpeedShare < 0 || o.BackwardSpeedShare > 1 {
		return fmt.Errorf("%w: backwardSpeedShare %v", ErrOrientationParams, o.BackwardSpeedShare)
	}
	if !isFinite(o.SlowingRadiusM) || o.SlowingRadiusM <= 0 {
		return fmt.Errorf("%w: slowingRadiusM %v", ErrOrientationParams, o.SlowingRadiusM)
	}
	if !isFinite(o.KickAlignmentSpread) || o.KickAlignmentSpread < 0 {
		return fmt.Errorf("%w: kickAlignmentSpread %v", ErrOrientationParams, o.KickAlignmentSpread)
	}
	return nil
}

// validate checks a PERSISTED orientation: unit facing within 1e-9, a resolved
// ±1 side and valid limits. It never mutates the value.
func (o RunnerOrientation) validate() error {
	if !isFinite(o.FX) || !isFinite(o.FY) {
		return fmt.Errorf("%w: (%v,%v)", ErrOrientationFacing, o.FX, o.FY)
	}
	if n := math.Sqrt(o.FX*o.FX + o.FY*o.FY); math.Abs(n-1) > 1e-9 {
		return fmt.Errorf("%w: norm %v", ErrOrientationFacing, n)
	}
	if o.TurnSide != 1 && o.TurnSide != -1 {
		return fmt.Errorf("%w: got %d", ErrOrientationSide, o.TurnSide)
	}
	return o.validateParams()
}

// resolved returns a side-resolved, validated COPY. It does NOT normalize the
// facing: activation must REJECT a non-unit vector within the 1e-9 tolerance,
// never coerce it. It never mutates the receiver, so activation is transactional.
func (o RunnerOrientation) resolved(playerID uint64) (RunnerOrientation, error) {
	r := o
	if r.TurnSide == 0 {
		if playerID&1 == 1 {
			r.TurnSide = 1
		} else {
			r.TurnSide = -1
		}
	}
	if err := r.validate(); err != nil {
		return RunnerOrientation{}, err
	}
	return r, nil
}

// EnableOrientation attaches a resolved orientation to one runner of an
// already-integrated Sim at clock 0 before any event. It is transactional: the
// complete resolved state is validated before anything is mutated, so an invalid
// activation leaves the full Snapshot/RNG unchanged. Enabling orientation selects
// the oriented integrated model version and never downgrades it.
func (s *Sim) EnableOrientation(index int, state RunnerOrientation) error {
	if s.integrated == nil {
		return ErrOrientationNoIntegrated
	}
	if s.clock != 0 || len(s.events) != 0 {
		return ErrOrientationNotInitial
	}
	if index < 0 || index >= len(s.runners) {
		return fmt.Errorf("%w: %d", ErrOrientationIndex, index)
	}
	r := &s.runners[index]
	if r.Motion != MotionInertial {
		return ErrOrientationMotion
	}
	if r.PlayerID == 0 {
		return ErrOrientationPlayerID
	}
	resolved, err := state.resolved(r.PlayerID)
	if err != nil {
		return err
	}
	r.Orientation = &resolved
	if s.integrated.ModelVersion != IntegratedModelVersionBody {
		s.integrated.ModelVersion = IntegratedModelVersionOriented
	}
	return nil
}

// passSpread returns the kick-alignment execution-jitter multiplier for a passer:
// 1 when the shell is off or the passer is unknown/unoriented, otherwise
// 1 + k*(1 - facing·aimUnit)/2.
func (s *Sim) passSpread(passer int, ux, uy float64) float64 {
	if s.integrated == nil || passer < 0 || passer >= len(s.runners) {
		return 1
	}
	o := s.runners[passer].Orientation
	if o == nil {
		return 1
	}
	align := o.FX*ux + o.FY*uy
	return 1 + o.KickAlignmentSpread*(1-align)/2
}

// rotateFacing turns unit facing (fx,fy) toward unit target (gx,gy) by at most
// omega*dt, using the canonical bounded chord (no trig). h is clamped to at most
// 1. The persisted-perpendicular intermediate is chosen ONLY for an exact
// numerical antiparallel tie (dot < 0 and |cross| <= 1e-12); every other target,
// including obtuse non-antiparallel ones, turns the SHORTEST way regardless of
// side. At most 64 halvings are tried and the old facing is preserved (ok=false)
// rather than snapping when no representable progress exists.
func rotateFacing(fx, fy, gx, gy, h float64, side int) (float64, float64, bool) {
	if !(h > 0) {
		return fx, fy, false
	}
	if h > 1 {
		h = 1
	}
	threshold := (1-h*h/4)/(1+h*h/4) - 1e-12
	tx, ty := gx, gy
	dot := fx*gx + fy*gy
	cross := fx*gy - fy*gx
	if dot < 0 && math.Abs(cross) <= 1e-12 {
		// Exactly antiparallel (a numerical tie): pick the persisted side's
		// perpendicular so the turn direction is deterministic and stable.
		s := float64(side)
		tx, ty = -fy*s, fx*s
	}
	lambda := 1.0
	for i := 0; i < 64; i++ {
		cx := (1-lambda)*fx + lambda*tx
		cy := (1-lambda)*fy + lambda*ty
		if n := math.Sqrt(cx*cx + cy*cy); n > 0 {
			cx, cy = cx/n, cy/n
			if fx*cx+fy*cy >= threshold {
				return cx, cy, true
			}
		}
		lambda *= 0.5
	}
	return fx, fy, false
}
