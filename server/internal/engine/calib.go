package engine

// Calibration constants. Every number here is a tuning knob exercised by the
// calibration tests (goal rates, home/away balance) and the tactical sanity tests.
// Change values here, never inline magic numbers inside action code.

// Outcome shaping
const (
	// shotBase scales shot conversion quality before goalkeeper opposition.
	shotBase = 1.35
	// gkSaveShare is how strongly goalkeeping cancels shot quality.
	gkSaveShare = 0.55
	// shotDistanceFalloff shrinks conversion with distance (0..1 of the shot zone).
	shotDistanceFalloff = 0.55
	// shotZoneDist is the normalization distance for the falloff (≈ box edge to goal).
	shotZoneDist = 0.34
	// shotPressureShare dilutes shot quality under closing-down pressure.
	shotPressureShare = 0.14
	// blockBase is the baseline chance a defender blocks a strike.
	blockBase = 0.22
	// blockCongestionShare multiplies block strength with bodies packed near the ball.
	blockCongestionShare = 0.35
	// congestionDilution discounts the chance value itself when bodies pack the ball
	// zone (mirrors real xG models that dilute by defender proximity).
	congestionDilution = 0.16
	// headerShotPenalty reduces headed attempts vs struck shots.
	headerShotPenalty = 0.75
	// penaltyBaseGoal is the baseline penalty conversion before duels.
	penaltyBaseGoal = 0.76
	// keeperClaimShare is the chance a keeper punches/holds a delivered cross.
	keeperClaimShare = 0.35
	// clearanceShare is the chance defenders clear a cross before a header duel.
	clearanceShare = 0.30
	// secondBallShare falls to the edge of the box after a clearance (long-shot wave).
	secondBallShare = 0.35
	// offsideBase is the baseline chance a through-ball run is caught offside.
	offsideBase = 0.20
	// foulBase is the baseline chance a lost duel is a foul for the defender.
	foulBase = 0.26
	// penaltyAreaFoulShare converts defender fouls inside the box into penalties.
	penaltyAreaFoulShare = 0.85
	// cardBase is the baseline booking chance on a foul (aggression & hard tackling raise it).
	cardBase = 0.30
	// injuryBase is the per-tick injury hazard at fatigue 0 (grows with fatigue & duels).
	injuryBase = 0.00012
	// errorEventShare is how many lost balls are severe enough to log as a visible gift.
	errorEventShare = 0.03
	// deflectCornerShare sends off-target efforts behind for a corner instead of a goal kick.
	deflectCornerShare = 0.08
	// homeAdvantage is the familiar-ground/crowd multiplier (visible, documented).
	homeAdvantage = 1.10
	// moraleGoalSwing is the visible morale shift after a goal (+winner / −conceder).
	moraleGoalSwing = 4.0
	// gameStateShift moves the block with the scoreboard: trailing teams push up,
	// leaders manage the game (visible, explainable — the scoreboard is public).
	gameStateShift = 0.015
	// gameStateChase tilts shot appetite for chasing teams late.
	gameStateChase = 0.10
	// hurtPenalty reduces a player's contribution while carrying an injury.
	hurtPenalty = 0.15
	// presenceBoost is the live-presence performance multiplier.
	presenceBoost = 1.03
	// moraleBand is the max morale-driven performance swing at morale 0/100.
	moraleBand = 0.03
)

// Action weights (selection priors; attributes & tactics shift them).
const (
	wPass    = 1.7
	wThrough = 1.0
	wCross   = 1.4
	wDribble = 0.9
	wShoot   = 2.2
	wHold    = 0.5
)

// Experience/RNG pacing
const (
	baseCooldown   = 8 // ticks between touches at Tempo 3
	minCooldown     = 2
	restartPauseTicks = 3 // "walk over" delay before a dead-ball delivery
	manMarkRadius   = 0.06
)

// quality maps a 1–20 attribute to 0.05..1.0.
func quality(attr uint8) float64 {
	return float64(attr) / 20
}