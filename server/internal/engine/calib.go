package engine

// Calibration constants. Every number here is a tuning knob exercised by the
// calibration tests (goal rates, home/away balance) and the tactical sanity tests.
// Change values here, never inline magic numbers inside action code.

// Outcome shaping
const (
	// shotBase scales shot conversion quality before goalkeeper opposition.
	shotBase = 0.54
	// gkSaveShare is how strongly goalkeeping cancels shot quality.
	gkSaveShare = 0.55
	// shotDistanceFalloff shrinks conversion with distance (0..1 of the shot zone).
	shotDistanceFalloff = 0.55
	// shotZoneDist is the normalization distance for the falloff (≈ box edge to goal);
	// boxFinishDist is the penalty area — inside it finishing rules, outside it is
	// long-shot merchant country (honest shooting ranges).
	shotZoneDist  = 0.34
	boxFinishDist = 0.34
	longShotDist  = 0.50
	// longShotMin gates outside-the-box potshots to long-shot merchants (attr/20).
	longShotMin = 0.70
	// shotPressureShare dilutes shot quality under closing-down pressure.
	shotPressureShare = 0.14
	// blockBase is the baseline chance a defender blocks a strike.
	blockBase = 0.28
	// blockCongestionShare multiplies block strength with bodies packed near the ball.
	blockCongestionShare = 0.5
	// behindSpaceGain rewards through-balls into the grass behind a high defensive line.
	behindSpaceGain = 0.8
	// congestionDilution discounts the chance value itself when bodies pack the ball
	// zone (mirrors real xG models that dilute by defender proximity). This is the
	// parked bus' teeth against box-presence hunting.
	congestionDilution = 0.22
	// contactChainDecay dims successive contacts in a pattern chain (broken balls).
	contactChainDecay = 0.7
	// tempoDrain is the extra fatigue per tick for high-tempo play beyond Tempo 3.
	tempoDrain = 0.0015
	// comboFatigue multiplies drain when pressing AND attacking are stacked high —
	// five-knob maximalism is physically unsustainable (it fades late in matches).
	comboFatigue = 0.5
	// trafficDrag shrinks pass/dribble gains in packed zones (3+ bodies = full drag).
	// Build-ups get jammed in crowds; counters into open grass do not.
	trafficDrag = 0.3
)

// Football IQ (T-012): legibility of moments, not just distributions.
const (
	// fixationBoost powers the shot when a player is through on goal (beat the line,
	// keeper looming) — nobody squares that to the corner flag. Kept at ×2.5 (not 4):
	// giant multipliers made the shoot/pass menu cliff-edge and wrecked calibration.
	fixationBoost = 2.5
	// oneOnOneDist / oneOnOnePress gate the through-on-goal read.
	oneOnOneDist  = 0.12
	oneOnOnePress = 1.2
	// aheadMargin is how far beyond the defensive line counts as "through".
	aheadMargin = 0.02
	// roleStrike scales open-play strike appetite by position (headers stay legal):
	// keepers never shoot, centre-backs rarely, holders sometimes.
	strikeGK = 0.0
	strikeCB = 0.3
	strikeDM = 0.6
	strikeFB = 0.7
	// roleMaxX (attack-normalized) is how far upfield each line may venture in open
	// play: centre-backs hold, holders shuttle, everyone else roams. Set pieces and
	// patterns exempt their actors explicitly.
	roleMaxXCB = 0.72
	roleMaxXDM = 0.84
	// carryBrakeDist: no leisurely forward carry with an opponent in your shirt.
	carryBrakeDist = 0.06
	// headerShotPenalty reduces headed attempts vs struck shots.
	headerShotPenalty = 0.62
	// penaltyBaseGoal is the baseline penalty conversion before duels.
	penaltyBaseGoal = 0.76
	// keeperClaimShare is the chance a keeper punches/holds a delivered cross.
	keeperClaimShare = 0.45
	// clearanceShare is the chance defenders clear a cross before a header duel.
	clearanceShare = 0.42
	// secondBallShare falls to the edge of the box after a clearance (long-shot wave).
	secondBallShare = 0.35
	// deliveryScatterShare is the fraction of DEFENDED deliveries (crosses,
	// corners) that stay live as a loose second ball instead of a controlled
	// defensive win — knockdowns: crowded boxes stay dangerous, and the
	// scramble belongs to whoever reads it first.
	deliveryScatterShare = 0.45
	// tackleScatterShare is the fraction of lost take-on duels that dislodge
	// the ball live at the tackle spot (a poke, not a pin) — the dispossessor
	// has to win the scramble too.
	tackleScatterShare = 0.30
	// looseScatter is how far a defended header knocks the loose ball (norm units).
	looseScatter = 0.10
	// offsideBase is the baseline chance a through-ball run is caught offside.
	offsideBase = 0.12
	// foulBase is the baseline chance a lost duel is a foul for the defender.
	foulBase = 0.26
	// penaltyAreaFoulShare converts defender fouls inside the box into penalties.
	penaltyAreaFoulShare = 0.85
	// cardBase is the baseline booking chance on a foul (aggression & hard tackling raise it).
	cardBase = 0.22
	// injuryBase is the per-tick injury hazard at fatigue 0 (grows with fatigue & duels).
	injuryBase = 0.00012
	// errorEventShare is how many lost balls are severe enough to log as a visible gift.
	errorEventShare = 0.03
	// deflectCornerShare sends off-target efforts behind for a corner instead of a goal kick.
	deflectCornerShare = 0.32
	// homeAdvantage is the familiar-ground/crowd multiplier (visible, documented).
	// Cranked above intuition because duelChance() compresses quality gaps near
	// parity — a visible ×perf multiplier needs muscle to move outcome bands.
	homeAdvantage = 1.13
	// moraleGoalSwing is the visible morale shift after a goal (+winner / −conceder;
	// the label in the goal event quotes this exact number). The PERFORMANCE band is
	// kept thin (moraleBand) so leads do not snowball through confidence alone.
	moraleGoalSwing = 3.0
	// gameStateShift moves the block with the scoreboard: trailing teams push up,
	// leaders manage the game (visible, explainable — the scoreboard is public).
	// Gentle: heavy scoreboard management starves equalisers (and the draw band).
	gameStateShift = 0.008
	// leaderManage is the extra drop for teams protecting a lead (leads get managed
	// harder than deficits get chased — the classic anti-blowout force).
	leaderManage = 0.005
	// gameStateChase tilts shot appetite for chasing teams late. Kept modest: heavy
	// chase spirals turn deficits into routs and starve the draw band.
	gameStateChase = 0.06
	// hurtPenalty reduces a player's contribution while carrying an injury.
	hurtPenalty = 0.15
	// presenceBoost is the live-presence performance multiplier.
	presenceBoost = 1.03
	// moraleBand is the max morale-driven performance swing at morale 0/100.
	moraleBand = 0.02
)

// Pattern execution (T-009 spike, see ADR-0009).
const (
	// patternSetupTicks is the walk-over before a patterned corner is delivered
	// (long enough for timed runs to develop).
	patternSetupTicks = 8
	// patternMaxTicks clears a stuck cursor (12 s guard).
	patternMaxTicks = 720
	// patternMarkBonus rewards runners who hit their finish mark on time.
	patternMarkBonus = 0.25
	// patternMarkFloor is the worst-case multiplier for hopelessly late runners.
	patternMarkFloor = 0.90
	// patternAimSpread widens aims of weak deliveries (× (2 − deliveryQuality)).
	patternAimSpread = 1.0
)

// Play-menu scales (option evaluation, not dice weights — the carrier compares real
// candidates and these scale how loudly each play competes).
const (
	wCross = 1.0
	wShoot = 1.85
	wHold  = 0.5
)

// Possession retention & option evaluation (T-014 stage B/C): ONE coherent failure
// budget instead of stacked dice channels. Safe build-up football keeps the ball
// ~95% of the time; gambles in the thick of it die far more often — WHERE the ball
// is lost emerges from WHICH option was chosen.
const (
	// passReach is the Manhattan reach of a service to feet (normalized pitch).
	passReach = 0.30
	// throughReach admits through-balls into space behind (runners only).
	throughReach = 0.42
	// behindGapMin is the minimum grass behind the opponent's line for a through ball.
	behindGapMin = 0.32
	// laneProbes is the number of segment probes measuring cover-shadow danger
	// (square-root-free geometry, portable determinism rule).
	laneProbes = 6
	// laneClearance is the cover-shadow radius: an opponent this close to the lane
	// cuts it completely.
	laneClearance = 0.09
	// passLossBase is the calm-lane, unforced-error floor of the retention budget.
	passLossBase = 0.055
	// passLossRisk weighs cut lanes (cover shadows) into the failure probability.
	passLossRisk = 0.045
	// passLossPressRisk amplifies lane danger under closing-down heat.
	passLossPressRisk = 0.35
	// passLossPress is the raw heat tax per point of pressure.
	passLossPress = 0.004
	// passLossStretch taxes overhit services (distance beyond ten metres).
	passLossStretch = 0.02
	// passLossSkill subtracts craft: passing, composure and the receiver's touch.
	passLossSkill = 0.115
	// recycleSafety is the retention multiplier of square/back balls (teams keep the
	// ball by recycling — this is what consolidates possession).
	recycleSafety = 0.50
	// throughRisk amplifies failure on through-ball services (weighted gambles).
	throughRisk = 1.35
	// misplaceShare is the failed-pass flavour that reads as misplacement (the rest
	// splits between lane interceptions and heavy first touches).
	misplaceShare = 0.30
	// tempoRush is the haste tax per tempo point beyond 3: fast-tempo football buys
	// extra touches with sloppier ones (the physical cost of rushing).
	tempoRush = 0.008
	// foulOnRegain is the late-challenge foul chance when a defender wins the ball
	// THROUGH the man — where football's open-play fouls actually come from.
	foulOnRegain = 0.45
	// holdLossShare is how often shielding against a challenger loses the wrestle.
	holdLossShare = 0.25
	// carryProbe is the grass probed ahead when grading carry space.
	carryProbe = 0.08
	// decisionsArgmaxFloor/Share: P(the carrier takes the best read) = floor + share·q².
	decisionsArgmaxFloor = 0.30
	decisionsArgmaxShare = 0.60
	// decisionGap flattens the misranking draw (bigger = greedier for the best read).
	decisionGap = 3.0
	// throughLead is the extra service lead of a through ball (runners collect).
	throughLead = 0.035
	// counterSurge is how far beyond the ball a transition runner breaks toward goal.
	counterSurge = 0.10
	// boxPinDepth is how far from the goal line strikers pin the corridor (the edge of
	// the six-yard lane: arrivals hop in on crosses — nobody camps on the goalmouth).
	boxPinDepth = 0.22
	// widthHoldDepth is the byline distance wingers hold while stretching the back line.
	widthHoldDepth = 0.30
	// arriveEdgeDepth is the cutback zone distance AMs arrive onto (box edge).
	arriveEdgeDepth = 0.30
	// boxPickupReach is the central corridor (from own goal) inside which runners get
	// picked up by their nearest defender — nobody camps unmarked in your six-yard
	// lane, while width and edge ghosts stay the zone's business.
	boxPickupReach = 0.24
)

// Experience/RNG pacing
const (
	baseCooldown      = 8 // ticks between touches at Tempo 3
	minCooldown       = 2
	restartPauseTicks = 3 // "walk over" delay before a dead-ball delivery
)

// quality maps a 1–20 attribute to 0.05..1.0.
func quality(attr uint8) float64 {
	return float64(attr) / 20
}
