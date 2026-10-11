package liveplay

// DrawSite enumerates every place the live-play prototype consumes randomness.
// The order is part of the contract: a released ball draws its lateral error
// first, then its speed error, so a resimulated snapshot replays the identical
// sequence (production determinism rule: "draw order is declared before
// drawing"). Contact detection and integration never draw.
//
// Labels are exposed through String so there is no exported mutable state (the
// package rule forbids global mutable variables).
type DrawSite uint8

// Declared RNG draw sites.
const (
	// DrawReleaseLateral is the signed lateral release error (±1 before scaling).
	DrawReleaseLateral DrawSite = iota
	// DrawReleaseSpeed is the signed release speed error (±1 before scaling).
	DrawReleaseSpeed
	// drawSiteCount is the number of declared sites (ordering guard).
	drawSiteCount
)

// String returns the stable label for evidence output. Unknown sites return
// "unknown" rather than panicking.
func (d DrawSite) String() string {
	switch d {
	case DrawReleaseLateral:
		return "release_lateral"
	case DrawReleaseSpeed:
		return "release_speed"
	default:
		return "unknown"
	}
}

// ReleaseParams is the physical intent of one ground-pass release.
type ReleaseParams struct {
	StartX, StartY   float64 // normalized release point
	SpeedMPS         float64 // intended ball speed along +x
	LateralJitterMPS float64 // ± lateral velocity error (metres/second)
	SpeedJitterFrac  float64 // ± fraction of SpeedMPS
}

// ReleaseBall launches a rolling ground ball. Draw order is fixed: lateral
// error, then speed error. The direction is represented without libm trig
// (velocity components directly) to avoid a trig-implementation dependency; it
// does not by itself guarantee cross-platform bit-identity.
//
// A release resets the first-contact record so a second release in the same
// simulator can register its own collection (multi-release fixtures/tests).
func (s *Sim) ReleaseBall(p ReleaseParams) {
	lateral := (s.rnd.Float64()*2 - 1) * p.LateralJitterMPS // DrawReleaseLateral
	speedErr := (s.rnd.Float64()*2 - 1) * p.SpeedJitterFrac // DrawReleaseSpeed

	s.contactMS = -1
	s.contactRunner = -1
	s.ball = Ball{
		Mode: BallRolling,
		X:    p.StartX,
		Y:    p.StartY,
		VXM:  p.SpeedMPS * (1 + speedErr),
		VYM:  lateral,
	}
}
