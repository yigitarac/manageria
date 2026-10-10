package engine

import "math"

// This file holds the open-play behaviour layer (ADR-0011 stage C): the carrier
// enumerates REAL options (pass lanes versus cover shadows, carry space, shoot
// windows), scores them with attributes and the chain regime's goal, and executes
// the chosen play. The weighted-dice action picker is retired: no more rolling for
// action KINDS, only situation evaluations that happen to involve chance.
//
// Failure model: one coherent retention budget (calib.go: passLoss*) instead of three
// stacked dice channels. WHERE the ball is lost emerges from WHICH option was picked —
// cut lanes become interceptions, heat becomes misplacement, mixed company becomes a
// heavy first touch.
//
// Draw-order rule: every play resolves its random draws in a fixed code order and
// option enumeration scans slots in index order — no draw depends on map iteration.

// Play kinds (candidate courses of action).
const (
	playPass = iota
	playThrough
	playCross
	playCarry
	playShoot
	playHold
)

// passOption is one concrete pass candidate the carrier can see.
type passOption struct {
	to       int
	dist     float64 // travel distance (Manhattan, normalized pitch)
	gain     float64 // attack-normalized forward progress
	risk     float64 // 0..1 lane danger: cover shadows cutting the passing lane
	openness float64 // 0..1 receiver freedom from markers
	longRun  bool    // served into space behind the line (offside applies)
}

// play is a candidate course of action with its evaluated utility.
type play struct {
	kind int
	opt  passOption
	util float64
}

// act resolves one open-play possession tick: enumerate, evaluate, choose, execute.
func (s *simState) act() {
	if s.owner < 0 {
		s.contestLoose()
		return
	}
	team, idx := int(s.owner), int(s.ownerIdx)
	s.maybeInjury(team, idx)

	sit := s.situation(team, idx)
	opts := s.passOptions(team, idx, sit)
	plays := s.buildPlays(team, idx, sit, opts)
	p := s.choosePlay(team, idx, plays)
	s.executePlay(team, idx, p, sit)
	s.cooldown = s.actionCooldown(team)
}

// situation gathers the carrier's tactical context (explainable inputs to tables).
type situation struct {
	goalDist  float64 // 0 at the opponent goal line, 1 at the own goal line
	wide      bool
	press     float64 // broader defending block around the carrier
	challenge float64 // defenders close enough to close down the carrier
	gapBehind float64 // open grass behind the opponent's defensive line
}

func (s *simState) situation(team, idx int) situation {
	// The game's location is the BALL (players chase it; distance and wings are
	// judged where the ball actually is).
	goalX := 1.0
	if team == 1 {
		goalX = 0.0
	}
	return situation{
		goalDist:  absf(goalX - s.ballX),
		wide:      absf(s.ballY-0.5) > 0.28,
		press:     s.pressure(team, idx),
		challenge: s.challengePressure(team, idx),
		gapBehind: absf(s.defensiveLineX(1-team) - goalXFor(1-team)),
	}
}

// ---- option enumeration (stage C: real candidates, not action kinds) ----

// passOptions enumerates every reachable teammate as a concrete pass candidate,
// scored for lane danger (cover shadows), receiver freedom and forward gain.
// Scanning slots in index order keeps the draw order deterministic.
func (s *simState) passOptions(team, idx int, sit situation) []passOption {
	dir := 1.0
	if team == 1 {
		dir = -1
	}
	bx, by := s.ballX, s.ballY
	desperate := sit.challenge > 2.2
	opts := make([]passOption, 0, 10)
	for i := 0; i < 11; i++ {
		if i == idx || s.cards[team*11+i].Off {
			continue
		}
		isGK := s.onPitch[team*11+i].Pos == PosGK
		ps := s.players[team*11+i]
		dist := absf(ps.X-bx) + absf(ps.Y-by)
		gain := dir * (ps.X - bx)
		// Keepers are emergency outlets only: a retreat ball under heavy heat, or
		// the sweeper sweeping. Nobody lasoons forty metres at his goalkeeper.
		if isGK && !(desperate || gain <= 0) {
			continue
		}
		longRun := false
		switch {
		case dist <= passReach:
		case dist <= throughReach && s.runnerBehindLine(team, i) && sit.gapBehind > behindGapMin:
			longRun = true
		default:
			continue
		}
		opts = append(opts, passOption{
			to:       i,
			dist:     dist,
			gain:     gain,
			risk:     s.laneRisk(team, bx, by, ps.X, ps.Y),
			openness: clamp(s.nearestDistTo(1-team, ps.X, ps.Y)/0.12, 0, 1),
			longRun:  longRun,
		})
	}
	return opts
}

// laneRisk measures how badly the DEFENDING side cuts the passing lane (their cover
// shadows): 0 = a clean lane, 1 = played through a wall. Six segment probes keep the
// geometry square-root free and bit-trivial (portable determinism rule). The owning
// team is explicit — dead-ball walks have no owner yet.
func (s *simState) laneRisk(team int, x1, y1, x2, y2 float64) float64 {
	best := math.MaxFloat64
	opp := 1 - team
	for i := 0; i < 11; i++ {
		if s.cards[opp*11+i].Off {
			continue
		}
		ps := s.players[opp*11+i]
		for k := 1; k <= laneProbes; k++ {
			u := float64(k) / float64(laneProbes+1)
			sx := x1 + (x2-x1)*u
			sy := y1 + (y2-y1)*u
			if d := absf(ps.X-sx) + absf(ps.Y-sy); d < best {
				best = d
			}
		}
	}
	return clamp(1-best/laneClearance, 0, 1)
}

// laneCutter returns the opponent most likely to intercept this lane (the deepest
// cover shadow on the segment) — the honest beneficiary of a cut pass.
func (s *simState) laneCutter(team int, x1, y1, x2, y2 float64) int {
	opp := 1 - team
	best, bestD := s.nearestTo(opp, x1, y1), math.MaxFloat64
	for i := 0; i < 11; i++ {
		if s.cards[opp*11+i].Off {
			continue
		}
		ps := s.players[opp*11+i]
		d := math.MaxFloat64
		for k := 1; k <= laneProbes; k++ {
			u := float64(k) / float64(laneProbes+1)
			sx := x1 + (x2-x1)*u
			sy := y1 + (y2-y1)*u
			if dd := absf(ps.X-sx) + absf(ps.Y-sy); dd < d {
				d = dd
			}
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// runnerBehindLine reports a teammate already beyond the opponent's defensive line
// (the trigger for a through-ball service into the space behind).
func (s *simState) runnerBehindLine(team, idx int) bool {
	line := s.defensiveLineX(1 - team)
	return (1-2*float64(team))*(s.players[team*11+idx].X-line) > 0
}

// shootWindow grades the shooting window (0 = no window): distance honesty first —
// INSIDE the box finishing rules (closer is sweeter), OUTSIDE it is long-shot
// merchant country — then the traffic and pressure between ball and goal.
func (s *simState) shootWindow(team, idx int, sit situation) (float64, bool) {
	carrier := s.onPitch[team*11+idx].Attr
	switch {
	case sit.goalDist < boxFinishDist:
		// Inside the box: finishing instincts, priced by proximity to goal.
		w := quality(carrier.Finishing) * (0.5 + 0.5*(1-sit.goalDist/boxFinishDist))
		return w / (1 + shotWindowDensity*sit.press), true
	case sit.goalDist < longShotDist:
		// Outside the box: the right foot for the range (attr gate) or lay off.
		ls := quality(carrier.LongShots)
		if ls < longShotMin {
			return 0, false
		}
		w := (ls - longShotMin) * (0.25 + 0.35*(1-(sit.goalDist-boxFinishDist)/(longShotDist-boxFinishDist)))
		return w / (1 + shotWindowDensity*sit.press), true
	}
	return 0, false
}

// carrySpace grades the grass ahead of the carrier (0 = a thicket). The open
// term measures from the BALL with the duel threshold (0.13) — the same
// separation carry() will actually roll against, so a challenger on the ball
// is a thicket here too, not open grass.
func (s *simState) carrySpace(team, idx int, sit situation) float64 {
	dir := 1.0
	if team == 1 {
		dir = -1
	}
	ps := s.players[team*11+idx]
	tx := clamp(ps.X+dir*carryProbe, 0.02, 0.98)
	lane := s.laneRisk(team, ps.X, ps.Y, tx, ps.Y)
	open := clamp(s.nearestDistTo(1-team, s.ballX, s.ballY)/0.13, 0, 1)
	space := (1-lane)*0.55 + open*0.45
	if s.boxCongestion(1-team) > 0.8 {
		space *= 0.5 // traffic drag: packed zones jam carries too
	}
	_ = sit
	return space
}

// boxPresence counts teammates attacking the shooting corridor (cross hunger).
func (s *simState) boxPresence(team int) int {
	gx := goalXFor(team)
	n := 0
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		ps := s.players[team*11+i]
		if absf(gx-ps.X) < 0.25 && absf(ps.Y-0.5) < 0.3 {
			n++
		}
	}
	return n
}

// ---- evaluation & selection (vision widens, decisions ranks, composure steadies) ----

// passUtility scores one pass candidate against the regime's goal and the manager's
// knobs. Build-up protects (risk punished), transition spears (gain rewarded), the
// final third hunts the box.
func (s *simState) passUtility(team, idx int, o passOption, sit situation) float64 {
	tac := s.tactics[team]
	gainW, riskW, openW, boxW := 1.0, 1.0, 1.0, 0.6
	switch s.chainReg {
	case RegimeBuildUp:
		gainW, riskW = 0.55, 1.6
	case RegimeProgression:
		gainW, riskW, boxW = 1.0, 1.25, 0.9
	case RegimeFinalThird:
		gainW, riskW, openW, boxW = 1.15, 1.0, 1.3, 1.5
	case RegimeTransition:
		gainW, riskW, boxW = 1.7, 0.75, 1.2
	case RegimeRegroup:
		gainW, riskW = 0.4, 1.8 // settle, do not gamble
	}
	// Manager knobs: attacking minds buy gain with risk; direct play travels.
	// Every mentality premium shares one modest slope (0.06): a dugout shout is not
	// a force multiplier (giant premiums detonated the preset goal bands).
	gainKnob, riskKnob := 1.0, 1.0
	gainKnob *= 1 + 0.06*float64(tac.Mentality-3)
	riskKnob *= 1 + 0.06*float64(3-tac.Mentality)
	switch tac.PassingStyle {
	case 1: // short: safety first
		riskKnob *= 1.15
	case 3: // direct: risk tolerated for progress
		riskKnob *= 0.85
		gainKnob *= 1.15
	}
	if o.longRun {
		gainKnob *= 1 + behindSpaceGain*(sit.gapBehind-0.3)
		if tac.CounterAttack {
			gainKnob *= 1.25
		}
		riskKnob *= 1.3
	}
	// The manager's whole stack shares one armour budget (T-016): stacked aggressive
	// knobs compound no further, so preset extremes bend toward the calibrated core
	// instead of detonating past it. Regime weights stay OUTSIDE the armour — they
	// are the football meaning of the phase, not a shouted instruction.
	gainW *= armour(gainKnob)
	riskW *= armour(riskKnob)

	// Box presence: a FREE man in the corridor is worth feeding (final third
	// especially) — feeding a marked pin is a hopeful ball, not a gift.
	gx := goalXFor(team)
	boxBonus := 0.0
	if absf(gx-s.players[team*11+o.to].X) < 0.25 {
		boxBonus = boxW * (0.15 + 0.85*o.openness)
	}
	// Square/back balls are retention: valued when the lane is clean and the
	// regime wants patience (this is how chains consolidate instead of dying).
	// The credit FADES with lane danger across a wide band (T-016): the old hard cut
	// at risk 0.25 flipped near-identical options, and a narrow transition band proved
	// insufficient — measured 2026-10-09, narrowing the bands heated aggressive preset
	// rows by +0.2..0.4 goals. A wide fade keeps borderline candidates COMPETING in the
	// menu instead of flipping; menu diversity absorbs extremism.
	recycle := 0.0
	if o.gain <= 0 {
		recycle = 0.12 * riskW * clamp((0.35-o.risk)/0.25, 0, 1)
	}
	// Recency/repetition penalty: handing the ball straight back to a recent
	// supplier without a genuine escape reason keeps a two- or three-man loop
	// spinning forever (ping-pong). The penalty fades with how far back the supplier
	// was, is exempted by a progressive return (a wide fade over gain — no cliff at
	// 0.05), and eases under heavy heat where a backward outlet is a legitimate escape.
	returnPenalty := 0.0
	if ramp := clamp((0.10-o.gain)/0.10, 0, 1); ramp > 0 {
		n := len(s.touches)
		for back := 1; back <= recencyWindow && back <= n; back++ {
			t := s.touches[n-back]
			if t.Kind == TouchPass && t.Success && t.Team == int8(team) && t.Actor == int8(o.to) {
				returnPenalty = recencyPenalty / float64(back)
				break
			}
		}
		returnPenalty *= ramp * clamp(1-0.25*sit.press, 0.2, 1)
	}
	return 1.9*gainW*clamp(o.gain, -0.25, 0.5) +
		1.6*openW*o.openness +
		boxBonus -
		riskW*o.risk*(1+0.35*sit.press) -
		0.6*o.dist +
		recycle - returnPenalty
}

// buildPlays turns the visible candidates into the carrier's actual menu.
func (s *simState) buildPlays(team, idx int, sit situation, opts []passOption) []play {
	tac := s.tactics[team]
	plays := make([]play, 0, len(opts)+4)
	for _, o := range opts {
		kind := playPass
		if o.longRun {
			kind = playThrough
		}
		plays = append(plays, play{kind: kind, opt: o, util: s.passUtility(team, idx, o, sit)})
	}

	// Carry: eat the space ahead yourself — but price the grass honestly. A carry
	// with a challenger on the ball is a coin-flip duel, not a plan (the 0.13
	// separation band mirrors carry()'s free-path threshold); carrying into a
	// shirt must lose the menu to the safe pass, while open grass stays greedy.
	// Regime weights mirror passUtility's: build-up protects (a CB dribbling out
	// through the press is a highlight, not a plan), transition spears (counters
	// RUN with the ball).
	// Carry admittance fades in from space 0.05 (T-016): the old hard gate at 0.2
	// made near-identical geometry flip the whole carry option on and off. The wide
	// fade doubles as menu diversity — see the recycle note (narrow bands were
	// measured worse for aggressive presets).
	if space := s.carrySpace(team, idx, sit); space > 0.05 {
		carrier := s.onPitch[team*11+idx].Attr
		sep := clamp(s.nearestDistTo(1-team, s.ballX, s.ballY)/0.13, 0, 1)
		u := 0.30 + 0.9*quality(carrier.Dribbling)*space + 0.30*(space-0.45) - 0.20*sit.press + 0.55*sep
		switch s.chainReg {
		case RegimeBuildUp:
			u *= 0.55
		case RegimeProgression:
			u *= 0.90
		case RegimeTransition:
			u *= 1.35
		case RegimeFinalThird:
			u *= 1.15
		}
		u *= 1 + 0.06*float64(tac.Mentality-3)
		u *= clamp((space-0.05)/0.15, 0, 1)
		if u > 0 {
			plays = append(plays, play{kind: playCarry, util: u})
		}
	}

	// Shoot: only inside an open window, and only with the right foot for it.
	if win, ok := s.shootWindow(team, idx, sit); ok {
		u := wShoot * win * s.strikeFactor(team, idx)
		// A tightly marked shooter is squeezed — snatching through a shirt is a
		// worse read than laying off or pinning it (options: windows include your
		// own freedom). Unmarked men bang away; prisoners wrestle and recycle).
		marked := clamp(s.nearestDistTo(1-team, s.ballX, s.ballY)/0.12, 0, 1)
		read := 0.65 + 0.35*marked
		read *= 1 + 0.04*float64(tac.Mentality-3)
		if d := s.scoreDiff(team); d < 0 {
			read *= 1 + gameStateChase*float64(minInt(-d, 2))/2
		}
		// Nobody squares a through-on-goal chance — power the shot home.
		if sit.goalDist < oneOnOneDist && sit.press < oneOnOnePress && s.isAheadOfDefense(team, idx) {
			read *= fixationBoost
		}
		// A clean hit-your-marks contact with the keeper unsighted is a green light.
		read *= 1 + 0.5*s.shootSightline(team, idx)
		// The whole read stack shares the armour budget (T-016): a green-light
		// cascade bends toward 1+knobArmour instead of compounding to ×4+.
		u *= armour(read)
		if u > 0 {
			plays = append(plays, play{kind: playShoot, util: u})
		}
	}

	// Cross: from the wide lanes, hungry box — and only when the box battle is
	// actually winnable. The crosser reads the aerial odds his delivery will
	// roll against: a cross into a lost cause (a lone striker vs the CB pair)
	// demotes below the safe pass, the way real wide men check back instead of
	// lobbing hope into a crowd.
	if sit.wide && absf(s.ballY-0.5) > 0.24 {
		carrier := s.onPitch[team*11+idx].Attr
		if np := s.boxPresence(team); np > 0 {
			u := wCross * quality(carrier.Crossing) * float64(minInt(np, 3)) / 3 * float64(tac.Width) / 3
			u *= 0.35 + 0.65*s.aerialLean(team)
			plays = append(plays, play{kind: playCross, util: u})
		}
	}

	// Hold: under the cosh with nothing on — pin it, let support arrive.
	// NOTE: a stronger shield price (2026-10-08 trial) improved retention but
	// blew both contracts — shots (each a chain closure) and goals erupted.
	// Isolation football stays contested-carry territory until T-016 revisits
	// it with the retention budget as one piece.
	if sit.challenge > 0 {
		carrier := s.onPitch[team*11+idx].Attr
		u := wHold * quality(carrier.Composure) * sit.press * 0.5
		// Holding only pays if SOMEONE is eventually playable.
		if len(opts) > 0 {
			plays = append(plays, play{kind: playHold, opt: s.safestOption(opts), util: u})
		}
	}
	return plays
}

// shootSightline grades how obscured the sightline to goal is (0 clean, 1 smothered)
// — inverted into a bonus when the lane is clear.
func (s *simState) shootSightline(team, idx int) float64 {
	ps := s.players[team*11+idx]
	gx := goalXFor(team)
	return 1 - s.laneRisk(team, ps.X, ps.Y, gx, 0.5)
}

// safestOption picks the retention ball (least danger, closest) — the escape valve.
func (s *simState) safestOption(opts []passOption) passOption {
	best, bestU := opts[0], math.MaxFloat64
	for _, o := range opts {
		u := o.risk*2 + o.dist
		if u < bestU {
			bestU, best = u, o
		}
	}
	return best
}

// choosePlay applies the mental gate: `vision` widens the menu actually seen,
// `decisions` ranks it (or fumbles the ranking), composure is consumed at execution.
// The weighted-dice action picker is gone: options compete, brains pick.
func (s *simState) choosePlay(team, idx int, plays []play) play {
	if len(plays) == 0 {
		// Nothing humanly visible: poke it to the nearest shirt (rare — the hold
		// play covers most crowding scenarios).
		to := s.nearestSupport(team, idx)
		ps := s.players[team*11+to]
		emergency := passOption{to: to, openness: 0.2}
		emergency.dist = absf(ps.X-s.ballX) + absf(ps.Y-s.ballY)
		emergency.gain = (ps.X - s.ballX) * (1 - 2*float64(team))
		emergency.risk = s.laneRisk(team, s.ballX, s.ballY, ps.X, ps.Y)
		return play{kind: playPass, opt: emergency, util: 0}
	}
	attr := s.onPitch[team*11+idx].Attr
	// Vision: how much of the menu the player perceives (best options first).
	seen := 2 + int(4*quality(attr.Vision)+0.5)
	if seen > len(plays) {
		seen = len(plays)
	}
	ordered := make([]play, len(plays))
	copy(ordered, plays)
	sortPlaysByUtil(ordered)
	menu := ordered[:seen]

	// Decisions: the probability he actually takes the best read.
	if s.rnd.Float64() < decisionsArgmaxFloor+decisionsArgmaxShare*quality(attr.Decisions)*quality(attr.Decisions) {
		return menu[0]
	}
	// Misranking draw: favours near-best reads — graded on the SHARED scale of the
	// offered menu (its utility spread) with a psychometric noise floor below which two
	// reads are honestly indistinguishable. Odds depend on the menu's shape alone:
	// stacked multipliers tilt the menu but can no longer amplify the odds ratio
	// without bound (T-016 menu stabiliser).
	best := menu[0].util
	mean := 0.0
	for _, p := range plays {
		mean += p.util
	}
	mean /= float64(len(plays))
	variance := 0.0
	for _, p := range plays {
		d := p.util - mean
		variance += d * d
	}
	spread := math.Sqrt(variance / float64(len(plays)))
	unit := math.Sqrt(spread*spread + decisionNoiseFloor*decisionNoiseFloor)
	weight := func(p play) float64 {
		return 1 / (1 + decisionGap*(best-p.util)/unit)
	}
	total := 0.0
	for _, p := range menu {
		total += weight(p)
	}
	r := s.rnd.Float64() * total
	for _, p := range menu {
		r -= weight(p)
		if r <= 0 {
			return p
		}
	}
	return menu[0]
}

// armour saturates a composite posture multiplier (T-016): mild stacks pass through
// nearly unchanged, compounding stacks bend toward 1+knobArmour. Downsides survive —
// armour only stops the upward arms race.
func armour(m float64) float64 {
	return 1 + knobArmour*math.Tanh((m-1)/knobArmour)
}

// sortPlaysByUtil orders plays by utility descending; ties keep enumeration order
// (stable sort ⇒ deterministic).
func sortPlaysByUtil(plays []play) {
	for i := 1; i < len(plays); i++ {
		for j := i; j > 0 && plays[j].util > plays[j-1].util; j-- {
			plays[j], plays[j-1] = plays[j-1], plays[j]
		}
	}
}

// retainPass executes the best-visible pass for a set-piece taker (throw-in, goal
// kick, free-kick lay-off): a restart's first ball is an ordinary evaluated pass —
// the options model owns it like any other touch.
func (s *simState) retainPass(team, idx int) {
	sit := s.situation(team, idx)
	opts := s.passOptions(team, idx, sit)
	if len(opts) == 0 {
		s.advanceBall(team, idx, 0.02) // turn with it
		return
	}
	best, bestU := opts[0], s.passUtility(team, idx, opts[0], sit)
	for _, o := range opts[1:] {
		if u := s.passUtility(team, idx, o, sit); u > bestU {
			best, bestU = o, u
		}
	}
	s.executePass(team, idx, best, best.longRun)
}

// nearestSupport is the closest playable teammate (scramble fallback).
func (s *simState) nearestSupport(team, idx int) int {
	best, bestD := idx, math.MaxFloat64
	for i := 0; i < 11; i++ {
		if i == idx || s.cards[team*11+i].Off || s.onPitch[team*11+i].Pos == PosGK {
			continue
		}
		ps := s.players[team*11+i]
		if d := absf(ps.X-s.ballX) + absf(ps.Y-s.ballY); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// executePlay dispatches the chosen play.
func (s *simState) executePlay(team, idx int, p play, sit situation) {
	switch p.kind {
	case playShoot:
		s.finishShot(team, idx, false, sit.goalDist, 0)
	case playCross:
		s.cross(team, idx, sit)
	case playCarry:
		s.carry(team, idx, sit)
	case playHold:
		s.holdUp(team, idx, sit, p.opt)
	case playThrough:
		s.executePass(team, idx, p.opt, true)
	default:
		s.executePass(team, idx, p.opt, false)
	}
}

// duelChance turns two quality inputs (0..1) into a win probability for a.
// Plain arithmetic only (portable determinism rule).
func duelChance(a, b float64) float64 {
	d := a - b
	return 0.5 + 0.5*d/(1+absf(d))
}

// pressure measures the wider defending block around the carrier. It still
// prices lane risk and shooting traffic while close-down reads use challengePressure.
func (s *simState) pressure(team, idx int) float64 {
	opp := 1 - team
	bx, by := s.players[team*11+idx].X, s.players[team*11+idx].Y
	sum := 0.0
	for i := 0; i < 11; i++ {
		if s.cards[opp*11+i].Off {
			continue
		}
		ps := s.players[opp*11+i]
		d := absf(ps.X-bx) + absf(ps.Y-by)
		bite := 0.4 + 0.3*quality(s.onPitch[opp*11+i].Attr.WorkRate) + 0.3*float64(s.tactics[opp].Pressing)/3
		sum += bite / (1 + 6*d*d)
	}
	return sum
}

// challengePressure counts only opponents inside the near close-down band.
// The linear kernel reaches zero at its edge, so distant shirts cannot trigger
// emergency outlet or shielding decisions (T-021 DEF-6).
func (s *simState) challengePressure(team, idx int) float64 {
	opp := 1 - team
	carrier := s.players[team*11+idx]
	sum := 0.0
	for i := 0; i < 11; i++ {
		if s.cards[opp*11+i].Off {
			continue
		}
		ps := s.players[opp*11+i]
		d := absf(ps.X-carrier.X) + absf(ps.Y-carrier.Y)
		if d >= challengeRadius {
			continue
		}
		bite := 0.4 + 0.3*quality(s.onPitch[opp*11+i].Attr.WorkRate) + 0.3*float64(s.tactics[opp].Pressing)/3
		sum += bite * (1 - d/challengeRadius)
	}
	return minf(challengeScale*sum, challengeCap)
}

// skill multiplies a focused attribute slice by the player's live performance.
func (s *simState) skill(team, idx int, focus func(Attributes) float64) float64 {
	return focus(s.onPitch[team*11+idx].Attr) * s.perf(team, idx)
}

// ---- execution: one retention budget, failure flavour from the situation ----

// executePass resolves the chosen pass. The failure probability is the honest
// retention budget: lane danger, heat, stretch and first-touch company subtract
// craft (passing, composure, receiver control). On failure the SITUATION names the
// flavour: cut lanes are interceptions, heat is misplacement, else heavy touch.
func (s *simState) executePass(team, idx int, opt passOption, longRun bool) {
	carrier := s.onPitch[team*11+idx].Attr
	passQ := s.skill(team, idx, func(a Attributes) float64 {
		return 0.7*quality(a.Passing) + 0.3*quality(a.Vision)
	})
	recvQ := s.skill(team, opt.to, func(a Attributes) float64 {
		return quality(a.FirstTouch)
	})
	press := s.pressure(team, idx)
	composure := quality(carrier.Composure) * s.perf(team, idx)

	stretch := clamp((opt.dist-0.10)/0.25, 0, 1)
	pFail := passLossBase +
		passLossRisk*opt.risk*(1+passLossPressRisk*press) +
		passLossPress*press +
		passLossStretch*stretch -
		passLossSkill*(0.5*passQ+0.25*composure+0.25*recvQ)
	if opt.gain <= 0 {
		pFail *= recycleSafety // square/back balls retain by design
	}
	if longRun {
		pFail *= throughRisk
	}
	// More urgent pressing closes the release window even when the nearest
	// presser is shielding rather than tackling the carrier.
	pressLevel := int(s.tactics[1-team].Pressing) - 1
	pFail *= 1 + 0.12*float64(pressLevel*pressLevel)
	pFail += tempoRush * float64(maxInt(int(s.tactics[team].Tempo)-3, 0))
	pFail = clamp(pFail, 0.01, 0.5)

	if s.rnd.Float64() < pFail {
		// Flavour roll (fixed order): the failure names itself from the geometry.
		icptShare := clamp(opt.risk, 0.15, 0.7)
		roll := s.rnd.Float64()
		switch {
		case roll < icptShare:
			defIdx := s.laneCutter(team, s.ballX, s.ballY, s.players[team*11+opt.to].X, s.players[team*11+opt.to].Y)
			s.recordTouch(TouchPass, int8(idx), int8(defIdx), false)
			s.wonDuel(1-team, defIdx, 0.2)
			if s.foulAftermath(1-team, defIdx, idx) {
				return
			}
			s.setOwner(1-team, defIdx, "regain")
			s.recordTouch(TouchRegain, int8(defIdx), int8(idx), true)
		case roll < icptShare+misplaceShare:
			s.recordTouch(TouchPass, int8(idx), int8(opt.to), false)
			s.lapseConsequence(team, idx)
		default:
			s.recordTouch(TouchPass, int8(idx), int8(opt.to), false)
			s.lapseConsequence(team, opt.to)
		}
		return
	}

	s.recordTouch(TouchPass, int8(idx), int8(opt.to), true)
	if opt.gain > 0.05 {
		s.players[team*11+idx].Acc += 0.08 // a progressive ball is a valuable ball
	} else {
		s.players[team*11+idx].Acc += 0.04
	}
	s.advanceBall(team, opt.to, s.serviceLead(team, opt, longRun))
	if longRun && s.offsideCheck(team, opt.to) {
		return
	}
}

// serviceLead is the ball's landing beyond the receiver — he runs onto it (passes
// lead runners; retreat balls arrive at his feet).
func (s *simState) serviceLead(team int, opt passOption, longRun bool) float64 {
	lead := 0.006
	if opt.gain > 0 {
		lead += 0.012 * quality(s.onPitch[team*11+opt.to].Attr.Acceleration)
	}
	if longRun {
		lead += throughLead
	}
	return lead
}

// carry eats the grass ahead with the ball at the feet (dribbling/agility versus
// the nearest challenger's tackling/positioning).
func (s *simState) carry(team, idx int, sit situation) {
	_ = sit
	if s.nearestDistTo(1-team, s.ballX, s.ballY) > 0.13 {
		s.players[team*11+idx].Acc += 0.08
		s.recordTouch(TouchCarry, int8(idx), -1, true)
		s.advanceBall(team, idx, 0.08+0.04*quality(s.onPitch[team*11+idx].Attr.Dribbling))
		return
	}
	att := s.skill(team, idx, func(a Attributes) float64 {
		return 0.6*quality(a.Dribbling) + 0.4*quality(a.Agility)
	})
	defIdx := s.nearestOpponent(team, idx)
	def := s.onPitch[(1-team)*11+defIdx].Attr
	defQ := (0.6*quality(def.Tackling) + 0.4*quality(def.Positioning)) * s.perf(1-team, defIdx)
	hard := float64(s.tactics[1-team].Tackling) - 1 // 0 fair, 1 hard

	challenger := s.players[(1-team)*11+defIdx]
	separation := absf(challenger.X-s.ballX) + absf(challenger.Y-s.ballY)
	winChance := clamp(duelChance(att, defQ)+0.30*clamp(separation/0.13, 0, 1), 0.1, 0.85)
	if s.rnd.Float64() < winChance {
		s.players[team*11+idx].Acc += 0.15
		s.recordTouch(TouchCarry, int8(idx), -1, true)
		traffic := 1 - trafficDrag*minf(s.boxCongestion(1-team), 1)
		s.advanceBall(team, idx, (0.06+0.05*quality(s.onPitch[team*11+idx].Attr.Dribbling))*traffic)
		if s.rnd.Float64() < foulBase*0.5*(0.4+0.3*quality(def.Aggression))*(1+hard) {
			s.foulConsequence(1-team, defIdx, team, idx)
		}
		return
	}

	if s.rnd.Float64() < foulBase*(0.5+0.5*quality(def.Aggression))*(0.6+0.7*hard) {
		s.recordTouch(TouchCarry, int8(idx), -1, false)
		s.foulConsequence(1-team, defIdx, team, idx)
		return
	}
	s.recordTouch(TouchDuel, int8(idx), int8(defIdx), false)
	s.wonDuel(1-team, defIdx, 0.25)
	if s.foulAftermath(1-team, defIdx, idx) {
		return
	}
	// A dispossesson is often a poke, not a pin: the ball squirts live at the
	// tackle spot and the dispossessor still has to win the scramble.
	if s.rnd.Float64() < tackleScatterShare {
		s.squirtLoose()
		return
	}
	s.setOwner(1-team, defIdx, "regain")
	s.recordTouch(TouchRegain, int8(defIdx), int8(idx), true)
}

// holdUp pins the ball against the challenger and lays it off to the safest option
// (the target-man's bread and butter).
func (s *simState) holdUp(team, idx int, sit situation, opt passOption) {
	strength := s.skill(team, idx, func(a Attributes) float64 {
		return 0.6*quality(a.Strength) + 0.4*quality(a.Composure)
	})
	defIdx := s.nearestOpponent(team, idx)
	defQ := quality(s.onPitch[(1-team)*11+defIdx].Attr.Strength) * s.perf(1-team, defIdx)
	s.recordTouch(TouchCarry, int8(idx), -1, true)
	if s.rnd.Float64() < holdLossShare*duelChance(defQ, strength) {
		s.recordTouch(TouchDuel, int8(idx), int8(defIdx), false)
		s.wonDuel(1-team, defIdx, 0.2)
		if s.foulAftermath(1-team, defIdx, idx) {
			return
		}
		if s.rnd.Float64() < tackleScatterShare {
			s.squirtLoose()
			return
		}
		s.setOwner(1-team, defIdx, "regain")
		s.recordTouch(TouchRegain, int8(defIdx), int8(idx), true)
		return
	}
	s.players[team*11+idx].Acc += 0.04
	s.executePass(team, idx, opt, false)
	_ = sit
}

// cross delivers from the wide lane into the box battle (early clearance, keeper
// claim, then the aerial duel — shared vocabulary with set-piece deliveries).
func (s *simState) cross(team, idx int, sit situation) {
	crossQ := s.skill(team, idx, func(a Attributes) float64 {
		return quality(a.Crossing)
	})
	s.recordTouch(TouchPass, int8(idx), -1, true)
	s.players[team*11+idx].Acc += 0.05

	// Early clearance by the covering defenders (packed boxes smother deliveries).
	congestion := 1 + 0.25*s.boxCongestion(1-team)
	clearQ := s.bestOpponentAttr(team, func(a Attributes) float64 {
		return quality(a.Heading)*0.6 + quality(a.Positioning)*0.4
	})
	if s.rnd.Float64() < minf(clearanceShare*congestion, 0.6)*duelChance(clearQ, crossQ) {
		s.defensiveClear(team)
		return
	}

	// Keeper claim on the cross.
	gk := s.onPitch[(1-team)*11]
	if s.rnd.Float64() < minf(keeperClaimShare*congestion, 0.6)*duelChance(quality(gk.Attr.Handling)*s.perf(1-team, 0), crossQ) {
		s.restartIdxBall(1-team, 0, "restart") // GK distribution rebuilds from the back
		return
	}

	// Aerial duel: best attacker vs best defender (index-order scans).
	att := s.bestSlot(team, aerialFocus)
	def := s.bestOpponentSlot(team, aerialFocus)
	attQ := s.skill(team, att, aerialFocus)
	defQ := s.skill(1-team, def, aerialFocus) + s.manMarkBonus(1-team)
	if s.rnd.Float64() >= duelChance(attQ, defQ) {
		s.recordTouch(TouchDuel, int8(att), int8(def), false)
		s.wonDuel(1-team, def, 0.2)
		// A defended delivery is not a controlled possession: the header sprays
		// live and both teams contest the knockdown.
		if s.rnd.Float64() < deliveryScatterShare {
			s.scatterLoose(1 - team)
			return
		}
		s.setOwner(1-team, def, "regain")
		s.recordTouch(TouchRegain, int8(def), int8(att), true)
		return
	}

	s.players[team*11+att].Acc += 0.25
	s.ballX, s.ballY = s.players[team*11+att].X, s.players[team*11+att].Y
	s.recordTouch(TouchDuel, int8(att), int8(def), true)
	if s.rnd.Float64() < 0.45 {
		s.finishShot(team, att, true, absf(goalXFor(team)-s.players[team*11+att].X), 0)
		return
	}
	supportOpts := s.passOptions(team, att, s.situation(team, att))
	if len(supportOpts) == 0 {
		s.advanceBall(team, att, 0.02) // cushion it down for himself
		return
	}
	support := s.safestOption(supportOpts)
	s.advanceBall(team, support.to, 0.03)
}

// finishShot resolves strikes and headers (shared by open play, crosses, set pieces).
// markFree is the runner's hit-your-marks grade (0 = buried in the crowd, 1 = clean
// free header): a man who beat his marker earns air — less congestion discount, fewer blocks.
func (s *simState) finishShot(team, idx int, header bool, goalDist, markFree float64) {
	carrier := s.onPitch[team*11+idx].Attr
	attr := float64(carrier.Finishing)
	switch {
	case header:
		attr = 0.5*float64(carrier.Finishing) + 0.5*float64(carrier.Heading)
	case goalDist >= boxFinishDist:
		attr = float64(carrier.LongShots) // outside the box is specialist ground
	}
	press := s.pressure(team, idx)
	// Saturating pressure: a packed six-yard box must not multiply-shot-shy forever —
	// real xG treats crowded close-range headers as prime chances. Congestion dilution
	// below carries the "bodies everywhere" penalty separately.
	sat := press / (1 + 0.3*press)
	qShot := (attr / 20) * s.perf(team, idx) * (1 - shotPressureShare*sat)
	composure := 1 - 0.10*(1-quality(carrier.Composure))*(1+0.5*sat)
	if header {
		qShot *= headerShotPenalty
	}
	gk := s.onPitch[(1-team)*11]
	stop := (quality(gk.Attr.ShotStopping)*0.7 + quality(gk.Attr.Agility)*0.3) * s.perf(1-team, 0)

	distFrac := clamp(goalDist/shotZoneDist, 0, 1)
	congestion := 1 - congestionDilution*(1-0.5*markFree)*s.boxCongestion(1-team)
	pGoal := clamp(shotBase*qShot*(1-gkSaveShare*stop)*composure*(1-shotDistanceFalloff*distFrac)*congestion, 0.01, 0.55)
	pSave := clamp(0.35*qShot-0.2*stop, 0.05, 0.60)

	ts := s.statsFor(team)
	ts.Shots++
	s.recordTouch(TouchShot, int8(idx), -1, true)
	blockProb := blockBase * duelChance(
		s.bestOpponentAttr(team, func(a Attributes) float64 {
			return 0.6*quality(a.Positioning) + 0.4*quality(a.Aggression)
		})*(1+blockCongestionShare*s.boxCongestion(1-team)),
		qShot,
	)
	// A free-header runner (high markFree) has already escaped the block line.
	blockProb *= 1 - 0.4*markFree
	// xG books the post-block expectation: a smothered effort is barely a chance at all
	// (this is what makes a parked bus concede LESS xG — siege pinball isn't value).
	ts.XG += pGoal*(1-blockProb) + 0.02*blockProb
	ts.DistSum += goalDist
	if !header && s.strikeFactor(team, idx) < 1 {
		ts.StrikesFromDefenders++
	}
	if s.patTag != "" {
		ts.PatternShots++
		ts.PatternXG += pGoal
	}
	s.players[team*11+idx].Acc += 0.3

	// Block attempts: packed defences smother strikes. Chance value already booked
	// post-block above; this roll decides the outcome (loose ball or deflection).
	if s.rnd.Float64() < blockProb {
		s.closeChainIfLive("shot")
		if s.rnd.Float64() < deflectCornerShare {
			cx, cy := s.cornerSpot(team, 0.95)
			s.queueRestart(RestartCorner, team, cx, cy)
			return
		}
		s.owner = -1
		s.cooldown = 0
		return
	}

	roll := s.rnd.Float64()
	switch {
	case roll < pGoal:
		s.addGoal(team, idx)
	case roll < pGoal+pSave:
		ts.OnTarget++
		s.appendEvent(EventShotSaved, team, idx, s.patTag)
		s.players[(1-team)*11].Acc += 0.1
		s.closeChainIfLive("shot")
		if s.rnd.Float64() < (1-quality(gk.Attr.Handling))*0.3 {
			// Spilled save: scramble in the six-yard box.
			s.owner = -1
			s.cooldown = 0
			s.ballX, s.ballY = s.players[(1-team)*11].X, s.players[(1-team)*11].Y
			return
		}
		s.setOwner(1-team, 0, "restart") // GK distribution rebuilds from the back
	default:
		s.closeChainIfLive("shot")
		if s.rnd.Float64() < deflectCornerShare {
			// Deflected behind: corner instead of a goal kick.
			cx, cy := s.cornerSpot(team, 0.95)
			s.queueRestart(RestartCorner, team, cx, cy)
			return
		}
		s.appendEvent(EventShotOffTarget, team, idx, s.patTag)
		s.queueRestart(RestartGoalKick, 1-team, 0.08+0.84*float64(1-team), 0.5)
	}
}

// foulAftermath rolls the late-challenge foul when a defender wins the ball THROUGH
// the man — where football's open-play fouls actually come from (cards follow).
// Returns true when the foul kills the contest before any possession changed hands.
func (s *simState) foulAftermath(winnerTeam, winner, loser int) bool {
	hard := float64(s.tactics[winnerTeam].Tackling) - 1
	aggro := quality(s.onPitch[winnerTeam*11+winner].Attr.Aggression)
	if s.rnd.Float64() < foulOnRegain*(0.4+0.3*aggro)*(0.6+0.7*hard) {
		s.foulConsequence(winnerTeam, winner, 1-winnerTeam, loser)
		return true
	}
	return false
}

// lapseConsequence turns a heavy touch or misplaced pass into a turnover; the worst
// ones surface as visible error events (rare — matches are judged by their highlights).
func (s *simState) lapseConsequence(team, idx int) {
	opp := 1 - team
	defIdx := s.nearestOpponent(team, idx) // nearest opponent to the culprit
	if s.rnd.Float64() < errorEventShare {
		s.appendEvent(EventError, team, idx, "")
	}
	s.players[team*11+idx].Acc -= 0.25
	s.wonDuel(opp, defIdx, 0.1)
	if s.foulAftermath(opp, defIdx, idx) {
		return // fouled as he lost it — the ball never changed hands
	}
	s.setOwner(opp, defIdx, "regain")
	s.recordTouch(TouchRegain, int8(defIdx), int8(idx), true)
}

// foulConsequence: foul (and possible card/penalty) by defIdx on the attacker.
func (s *simState) foulConsequence(team, defIdx, attTeam, attIdx int) {
	s.statsFor(team).Fouls++
	s.appendEvent(EventFoul, team, defIdx, "")

	hard := float64(s.tactics[team].Tackling) - 1
	pCard := clamp(cardBase*(0.4+0.6*quality(s.onPitch[team*11+defIdx].Attr.Aggression))*(1+0.6*hard), 0.05, 0.6)
	if s.rnd.Float64() < pCard {
		s.bookPlayer(team, defIdx)
	}

	ps := s.players[attTeam*11+attIdx]
	inBox := inPenaltyArea(attTeam, ps.X, ps.Y)
	if inBox && s.rnd.Float64() < penaltyAreaFoulShare {
		penX := 0.89
		if attTeam == 1 {
			penX = 0.11
		}
		s.queueRestart(RestartPenalty, attTeam, penX, 0.5)
		return
	}
	s.queueRestart(RestartFreeKick, attTeam, ps.X, ps.Y)
}

func inPenaltyArea(team int, x, y float64) bool {
	if team == 0 {
		return x > 0.82 && absf(y-0.5) < 0.22
	}
	return x < 0.18 && absf(y-0.5) < 0.22
}

// bookPlayer books a player; a second yellow turns into a red (slot stops playing).
func (s *simState) bookPlayer(team, idx int) {
	slot := team*11 + idx
	s.cards[slot].Yellow++
	if s.cards[slot].Yellow >= 2 {
		s.cards[slot].Off = true
		s.appendEvent(EventRedCard, team, idx, "second_yellow")
		return
	}
	s.appendEvent(EventYellowCard, team, idx, "")
}

// manMarkBonus stiffens aerial defense when the defending side plays man-oriented
// marking (sticking to designated threats applies beyond set pieces).
func (s *simState) manMarkBonus(defTeam int) float64 {
	if s.tactics[defTeam].Marking == 2 {
		return 0.05
	}
	return 0
}

// wonDuel credits the winning player (ratings accumulator).
func (s *simState) wonDuel(team, idx int, acc float64) {
	s.players[team*11+idx].Acc += acc
}

// addGoal books a goal with visible morale swings and a kick-off for the conceding side.
func (s *simState) addGoal(team, idx int) {
	s.closeChainIfLive("goal")
	s.score = addScore(s.score, team)
	ts := s.statsFor(team)
	ts.Goals++
	ts.OnTarget++
	s.players[team*11+idx].Acc += 1.5

	s.shiftMorale(team, +moraleGoalSwing)
	s.shiftMorale(1-team, -moraleGoalSwing)
	detail := "morale:+3|-3"
	if s.patTag != "" {
		detail += ";" + s.patTag
	}
	s.appendEvent(EventGoal, team, idx, detail)

	half := 0
	if s.secondHalf {
		half = 1
	}
	s.stoppage[half] = minf(s.stoppage[half]+20+float64(s.rnd.Intn(21)), maxHalfStoppage)

	s.beginKickoff(1 - team)
}

func (s *simState) shiftMorale(team int, delta float64) {
	for i := 0; i < 11; i++ {
		ps := &s.players[team*11+i]
		ps.Morale = clamp(ps.Morale+delta, 0, 100)
	}
}

// maybeInjury rolls the per-tick injury hazard (fatigue raises it).
func (s *simState) maybeInjury(team, idx int) {
	ps := s.players[team*11+idx]
	if ps.Hurt {
		return
	}
	if s.rnd.Float64() < injuryBase*(1+ps.Fatigue/40) {
		minutesOut := 5 + s.rnd.Intn(41)
		s.players[team*11+idx].Hurt = true
		s.appendEvent(EventInjury, team, idx, minutesOutLabel(minutesOut))
	}
}

// contestLoose resolves a scramble for a loose ball (positioning decides the lean).
// The winner takes over WHERE THE BALL IS — feet collect, the ball never teleports.
func (s *simState) contestLoose() {
	n0 := s.nearestToBall(0)
	n1 := s.nearestToBall(1)
	q0 := s.skill(0, n0, func(a Attributes) float64 { return quality(a.Positioning) })
	q1 := s.skill(1, n1, func(a Attributes) float64 { return quality(a.Positioning) })
	side, idx := 0, n0
	if s.rnd.Float64() >= duelChance(q0, q1) {
		side, idx = 1, n1
	}
	s.setOwner(side, idx, "regain")
	s.recordTouch(TouchRegain, int8(idx), -1, true)
	s.wonDuel(side, idx, 0.1)
	s.cooldown = s.actionCooldown(side)
}

// scatterLoose knocks a defended delivery out of the duel crowd and leaves it
// LIVE: no controlled possession for the defence — contestLoose() decides who
// collects the next hop, and an attacking read keeps the chain alive as a
// knockdown (second-ball football: crowded boxes stay dangerous).
func (s *simState) scatterLoose(defTeam int) {
	dir := 1.0
	if defTeam == 1 {
		dir = -1
	}
	s.ballX = clamp(s.ballX+dir*looseScatter, 0.02, 0.98)
	s.ballY = clamp(s.ballY+(s.rnd.Float64()-0.5)*0.12, 0.04, 0.96)
	s.owner, s.ownerIdx, s.cooldown = -1, -1, 2
}

// squirtLoose dislodges the ball at the tackle spot (a poke, not a pin): the
// scramble is live and positional — whoever reads it first collects.
func (s *simState) squirtLoose() {
	s.ballX = clamp(s.ballX+(s.rnd.Float64()-0.5)*0.08, 0.02, 0.98)
	s.ballY = clamp(s.ballY+(s.rnd.Float64()-0.5)*0.08, 0.04, 0.96)
	s.owner, s.ownerIdx, s.cooldown = -1, -1, 2
}

// ---- shared attribute foci and slot scans (index order ⇒ deterministic) ----

func aerialFocus(a Attributes) float64 {
	return 0.5*quality(a.Heading) + 0.3*quality(a.Jumping) + 0.2*quality(a.Strength)
}

// aerialLean grades the delivery battle the attacking side would roll: best
// aerial attacker against best aerial defender (plus marking bite) — the same
// duel the delivery will actually contest. Wide men read this before lobbing.
func (s *simState) aerialLean(team int) float64 {
	att := s.bestSlot(team, aerialFocus)
	def := s.bestOpponentSlot(team, aerialFocus)
	attQ := s.skill(team, att, aerialFocus)
	defQ := s.skill(1-team, def, aerialFocus) + s.manMarkBonus(1-team)
	return duelChance(attQ, defQ)
}

func (s *simState) bestSlot(team int, f func(Attributes) float64) int {
	best, bestQ := 0, -1.0
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		if q := f(s.onPitch[team*11+i].Attr); q > bestQ {
			best, bestQ = i, q
		}
	}
	return best
}

func (s *simState) bestOpponentSlot(team int, f func(Attributes) float64) int {
	return s.bestSlot(1-team, f)
}

// bestOpponentAttr returns the best-scoring opponent attribute under focus f.
func (s *simState) bestOpponentAttr(team int, f func(Attributes) float64) float64 {
	return f(s.onPitch[(1-team)*11+s.bestOpponentSlot(team, f)].Attr)
}

// boxCongestion counts defending bodies close to the ball (0..~3).
func (s *simState) boxCongestion(team int) float64 {
	n := 0.0
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		ps := s.players[team*11+i]
		if absf(ps.X-s.ballX)+absf(ps.Y-s.ballY) < 0.15 {
			n++
		}
	}
	return n / 3
}

func (s *simState) nearestOpponent(team, idx int) int {
	return s.nearestTo(1-team, s.players[team*11+idx].X, s.players[team*11+idx].Y)
}

func (s *simState) nearestToBall(team int) int {
	return s.nearestTo(team, s.ballX, s.ballY)
}

func (s *simState) nearestTo(team int, x, y float64) int {
	best, bestD := 0, math.MaxFloat64
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		ps := s.players[team*11+i]
		if d := absf(ps.X-x) + absf(ps.Y-y); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// nearestDistTo returns the |dx|+|dy| distance to the closest active opponent.
func (s *simState) nearestDistTo(team int, x, y float64) float64 {
	best := math.MaxFloat64
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		ps := s.players[team*11+i]
		if d := absf(ps.X-x) + absf(ps.Y-y); d < best {
			best = d
		}
	}
	return best
}

// isAheadOfDefense reports that the carrier has got beyond the opponent's line.
func (s *simState) isAheadOfDefense(team, idx int) bool {
	line := s.defensiveLineX(1 - team)
	x := s.players[team*11+idx].X
	return (1-2*float64(team))*(x-line) > aheadMargin
}

// strikeFactor is position-based open-play strike appetite (headed threat is separate
// and stays legal for everyone — big centre-backs score headers).
func (s *simState) strikeFactor(team, idx int) float64 {
	switch s.onPitch[team*11+idx].Pos {
	case PosGK:
		return strikeGK
	case PosCB:
		return strikeCB
	case PosDM:
		return strikeDM
	case PosFB:
		return strikeFB
	default:
		return 1
	}
}

// advanceBall moves ball and possession to a receiver, landing `lead` beyond him in
// the direction of play — he runs onto the service (no teleporting athletes).
// Landing obeys role discipline: a centre-back cannot collect beyond his cap.
func (s *simState) advanceBall(team, receiver int, lead float64) {
	dir := 1.0
	if team == 1 {
		dir = -1
	}
	ps := s.players[team*11+receiver]
	rawX := ps.X + dir*lead
	rawY := s.ballY + (ps.Y-s.ballY)*0.7 // bent toward the receiver's lane
	if outOfPlay(rawX, rawY) {
		s.ballOut(team, rawX, rawY)
		return
	}
	s.ballX, s.ballY = clamp(rawX, 0.02, 0.98), clamp(rawY, 0.04, 0.96)
	s.ballX, s.ballY = s.roleClamp(team, receiver, s.ballX, s.ballY)
	s.setOwner(team, receiver, "")
}

// offsideCheck flags runs behind the defensive line (deep lines trap better).
func (s *simState) offsideCheck(team, receiver int) bool {
	line := s.defensiveLineX(1 - team)
	attX := s.players[team*11+receiver].X
	if (1-2*float64(team))*(attX-line) <= 0.02 {
		return false
	}
	trap := float64(s.tactics[1-team].DefensiveLine-2) * 0.02
	run := 0.5*quality(s.onPitch[team*11+receiver].Attr.Acceleration) +
		0.5*quality(s.onPitch[team*11+receiver].Attr.Vision)
	pOff := clamp(offsideBase+0.10*trap+0.10*(0.5-run), 0.03, 0.35)
	if s.rnd.Float64() < pOff {
		s.appendEvent(EventOffside, team, receiver, "")
		s.queueRestart(RestartFreeKick, 1-team, attX, s.ballY)
		return true
	}
	return false
}

func (s *simState) defensiveLineX(defTeam int) float64 {
	x := 0.0
	n := 0
	for i := 1; i < 11; i++ {
		if s.cards[defTeam*11+i].Off {
			continue
		}
		x += s.players[defTeam*11+i].X
		n++
	}
	if n == 0 {
		return 0.5
	}
	return x / float64(n)
}

// defensiveClear boots the ball away from danger (edge-of-box second ball possible).
func (s *simState) defensiveClear(attTeam int) {
	defTeam := 1 - attTeam
	idx := s.bestSlot(defTeam, func(a Attributes) float64 { return quality(a.Heading) * 0.6 })
	s.recordTouch(TouchDuel, int8(idx), -1, true)
	s.wonDuel(defTeam, idx, 0.15)
	dir := 1.0
	if defTeam == 1 {
		dir = -1
	}
	s.ballX = clamp(s.players[defTeam*11+idx].X+dir*0.2, 0.02, 0.98)
	s.closeChainIfLive("dead_ball")
	if s.rnd.Float64() < secondBallShare {
		// The clearance fell short: second ball at the edge (transition flash).
		s.setOwner(attTeam, s.bestSlot(attTeam, aerialFocus), "regain")
		return
	}
	s.setOwner(defTeam, idx, "regroup")
}

func minutesOutLabel(minutes int) string {
	return "minute_out:" + itoa(minutes)
}

// itoa avoids pulling fmt into the hot path label formatting.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
