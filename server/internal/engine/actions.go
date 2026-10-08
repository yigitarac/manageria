package engine

import "math"

// This file holds the open-play action tables: selection (with the `decisions`
// override), pass/through-ball/cross/dribble/shot resolution, duels, fouls, cards
// and injuries. Set pieces live in restarts.go.
//
// Draw-order rule: every action resolves its random draws in a fixed code order and
// participant picks scan slices in index order — no draw depends on map iteration.

// act resolves one open-play action for the ball carrier.
func (s *simState) act() {
	if s.owner < 0 {
		s.contestLoose()
		return
	}
	team, idx := int(s.owner), int(s.ownerIdx)
	s.maybeInjury(team, idx)

	sit := s.situation(team, idx)
	switch s.pickAction(team, idx, sit) {
	case actionShoot:
		s.finishShot(team, idx, false, sit.goalDist, 0)
	case actionCross:
		s.cross(team, idx, sit)
	case actionThrough:
		s.throughBall(team, idx, sit)
	case actionDribble:
		s.dribble(team, idx, sit)
	case actionHold:
		s.holdUp(team, idx, sit)
	default:
		s.shortPass(team, idx, sit)
	}
	s.cooldown = s.actionCooldown(team)
}

type actionKind int

const (
	actionPass actionKind = iota
	actionThrough
	actionCross
	actionDribble
	actionShoot
	actionHold
)

// situation gathers the carrier's tactical context (explainable inputs to tables).
type situation struct {
	goalDist  float64 // 0 at the opponent goal line, 1 at the own goal line
	wide      bool
	press     float64
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
		gapBehind: absf(s.defensiveLineX(1-team) - goalXFor(1-team)),
	}
}

// pickAction draws a weighted action; `decisions` lets smart players override the
// draw with the situational best choice (visible improvement, no hidden momentum).
func (s *simState) pickAction(team, idx int, sit situation) actionKind {
	tac := s.tactics[team]
	carrier := s.onPitch[team*11+idx].Attr

	w := [6]float64{wPass, wThrough, wCross, wDribble, wShoot, wHold}
	w[actionThrough] *= quality(carrier.Vision) * (1 + 0.3*float64(tac.Mentality-3)/2) * (1 + 1.8*(sit.gapBehind-0.3))
	if tac.CounterAttack {
		w[actionThrough] *= 1.25
	}
	w[actionCross] *= quality(carrier.Crossing) * float64(tac.Width) / 3
	if absf(s.ballY-0.5) <= 0.24 {
		w[actionCross] = 0
	}
	w[actionDribble] *= quality(carrier.Dribbling) * (1 + 0.2*sit.press)
	// Shooting happens in the shooting zones (0.36 ≈ 35 m); no pots from distance.
	// Under heavy closing-down the shot appetite drops — players lay off instead of
	// snatching at it (chronic box squatters therefore circulate rather than unload).
	w[actionShoot] = 0
	switch {
	case sit.goalDist < 0.16:
		w[actionShoot] = wShoot * 0.84 * quality(carrier.Finishing) * (0.6 + 0.4*(1-sit.goalDist/0.16)) / (1 + 0.22*sit.press)
	case sit.goalDist < 0.36:
		w[actionShoot] = wShoot * 0.5 * quality(carrier.LongShots) * (0.2 + 0.3*(1-sit.goalDist/0.36)) * (1 + 0.08*float64(tac.Mentality-3)) / (1 + 0.22*sit.press)
	}
	if d := s.scoreDiff(team); d < 0 {
		// Chasing a deficit: more shot appetite (the scoreboard is visible to everyone).
		w[actionShoot] *= 1 + gameStateChase*float64(minInt(-d, 2))/2
	}
	// Football IQ: nobody squares a through-on-goal chance — power the shot home.
	if sit.goalDist < oneOnOneDist && sit.press < oneOnOnePress && s.isAheadOfDefense(team, idx) {
		w[actionShoot] *= fixationBoost
	}
	// Role discipline: keepers never shoot, centre-backs rarely (headed threat stays).
	w[actionShoot] *= s.strikeFactor(team, idx)
	w[actionHold] *= quality(carrier.Composure) * (1 + 0.3*sit.press)
	w[actionPass] *= quality(carrier.Passing) * (1 + 0.2*(2-float64(tac.PassingStyle)))

	best := actionPass
	for i := actionPass; i <= actionHold; i++ {
		if w[i] > w[best] {
			best = i
		}
	}
	q := quality(carrier.Decisions)
	if s.rnd.Float64() < q*q*0.6 {
		return best
	}
	return s.drawWeighted(w)
}

func (s *simState) drawWeighted(w [6]float64) actionKind {
	total := 0.0
	for _, v := range w {
		total += v
	}
	if total <= 0 {
		return actionPass
	}
	r := s.rnd.Float64() * total
	for i, v := range w {
		r -= v
		if r <= 0 {
			return actionKind(i)
		}
	}
	return actionPass
}

// duelChance turns two quality inputs (0..1) into a win probability for a.
// Plain arithmetic only (portable determinism rule).
func duelChance(a, b float64) float64 {
	d := a - b
	return 0.5 + 0.5*d/(1+absf(d))
}

// pressure sums proximity-weighted closing-in of the opponents.
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

// skill multiplies a focused attribute slice by the player's live performance.
func (s *simState) skill(team, idx int, focus func(Attributes) float64) float64 {
	return focus(s.onPitch[team*11+idx].Attr) * s.perf(team, idx)
}

func (s *simState) shortPass(team, idx int, sit situation) {
	length := minf(0.07+0.03*float64(s.tactics[team].PassingStyle)/3, 0.1)
	s.deliverPass(team, idx, sit, length, false)
}

// deliverPass resolves a pass (through-balls set longRun), with first-touch checks.
func (s *simState) deliverPass(team, idx int, sit situation, length float64, longRun bool) {
	carrier := s.onPitch[team*11+idx].Attr
	passQ := s.skill(team, idx, func(a Attributes) float64 {
		return 0.7*quality(a.Passing) + 0.3*quality(a.Vision)
	})
	interQ := s.bestOpponentAttr(team, func(a Attributes) float64 {
		return quality(a.Positioning)
	})
	pIntercept := clamp(0.18*duelChance(interQ, passQ), 0.02, 0.25) * (1 + 0.15*sit.press)
	pMisplace := clamp(0.05*(1+0.4*sit.press)-0.06*quality(carrier.Composure)+
		0.025*float64(maxInt(int(s.tactics[team].Tempo)-3, 0)), 0.01, 0.25)

	receiver := s.pickSmartReceiver(team, idx)
	roll := s.rnd.Float64()
	switch {
	case roll < pMisplace:
		s.recordTouch(TouchPass, int8(idx), -1, false)
		s.lapseConsequence(team, idx)
		return
	case roll < pMisplace+pIntercept:
		defIdx := s.pickReceiver(1 - team)
		s.recordTouch(TouchPass, int8(idx), int8(defIdx), false)
		s.wonDuel(1-team, defIdx, 0.2)
		s.restartIdxBall(1-team, defIdx)
		return
	}

	recvQ := s.skill(team, receiver, func(a Attributes) float64 {
		return quality(a.FirstTouch)
	})
	pHeavy := clamp(0.08+0.08*sit.press-0.10*recvQ, 0.01, 0.3) * (1 - 0.7*max(0, sit.gapBehind-0.3))
	if longRun {
		pHeavy += 0.03
	}
	if s.rnd.Float64() < pHeavy {
		s.recordTouch(TouchPass, int8(idx), int8(receiver), false)
		s.lapseConsequence(team, receiver)
		return
	}

	s.recordTouch(TouchPass, int8(idx), int8(receiver), true)
	s.players[team*11+idx].Acc += 0.05
	traffic := 1 - trafficDrag*minf(s.boxCongestion(1-team), 1)
	s.advanceBall(team, receiver, length*traffic)
	if longRun && s.offsideCheck(team, receiver) {
		return
	}
	s.ownerIdx = int8(receiver)
}

func (s *simState) throughBall(team, idx int, sit situation) {
	// Space behind the opponent's line rewards runners — high lines gamble on the trap.
	gapBehind := absf(s.defensiveLineX(1-team) - goalXFor(1-team))
	length := (0.14 + 0.06*quality(s.onPitch[team*11+idx].Attr.Vision)) * (1 + behindSpaceGain*(gapBehind-0.3))
	if s.tactics[team].CounterAttack {
		length += 0.05
	}
	s.deliverPass(team, idx, sit, length, true)
}

func (s *simState) cross(team, idx int, sit situation) {
	crossQ := s.skill(team, idx, func(a Attributes) float64 {
		return quality(a.Crossing)
	})

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
		s.restartIdxBall(1-team, 0)
		return
	}

	// Aerial duel: best attacker vs best defender (index-order scans).
	att := s.bestSlot(team, aerialFocus)
	def := s.bestOpponentSlot(team, aerialFocus)
	attQ := s.skill(team, att, aerialFocus)
	defQ := s.skill(1-team, def, aerialFocus) + s.manMarkBonus(1-team)
	if s.rnd.Float64() >= duelChance(attQ, defQ) {
		s.wonDuel(1-team, def, 0.2)
		s.restartIdxBall(1-team, def)
		return
	}

	s.players[team*11+att].Acc += 0.25
	s.ballX, s.ballY = s.players[team*11+att].X, s.players[team*11+att].Y
	if s.rnd.Float64() < 0.55 {
		s.finishShot(team, att, true, 0.12, 0)
		return
	}
	support := s.pickReceiver(team)
	s.advanceBall(team, support, 0.03)
	s.ownerIdx = int8(support)
}

func (s *simState) dribble(team, idx int, sit situation) {
	_ = sit
	att := s.skill(team, idx, func(a Attributes) float64 {
		return 0.6*quality(a.Dribbling) + 0.4*quality(a.Agility)
	})
	defIdx := s.nearestOpponent(team, idx)
	def := s.onPitch[(1-team)*11+defIdx].Attr
	defQ := (0.6*quality(def.Tackling) + 0.4*quality(def.Positioning)) * s.perf(1-team, defIdx)
	hard := float64(s.tactics[1-team].Tackling) - 1 // 0 fair, 1 hard

	if s.rnd.Float64() < duelChance(att, defQ) {
		s.players[team*11+idx].Acc += 0.15
		s.recordTouch(TouchCarry, int8(idx), -1, true)
		traffic := 1 - trafficDrag*minf(s.boxCongestion(1-team), 1)
		s.advanceBall(team, idx, (0.08+0.04*quality(s.onPitch[team*11+idx].Attr.Dribbling))*traffic)
		if s.rnd.Float64() < foulBase*0.5*(0.4+0.3*quality(def.Aggression))*(1+hard) {
			s.foulConsequence(1-team, defIdx, team, idx)
		}
		return
	}

	if s.rnd.Float64() < foulBase*(0.5+0.5*quality(def.Aggression))*(0.6+0.7*hard) {
		s.foulConsequence(1-team, defIdx, team, idx)
		return
	}
	s.wonDuel(1-team, defIdx, 0.25)
	s.recordTouch(TouchDuel, int8(idx), int8(defIdx), false)
	s.restartIdxBall(1-team, defIdx)
}

func (s *simState) holdUp(team, idx int, sit situation) {
	strength := s.skill(team, idx, func(a Attributes) float64 {
		return 0.6*quality(a.Strength) + 0.4*quality(a.Composure)
	})
	defIdx := s.nearestOpponent(team, idx)
	defQ := quality(s.onPitch[(1-team)*11+defIdx].Attr.Strength) * s.perf(1-team, defIdx)
	if s.rnd.Float64() < 0.25*duelChance(defQ, strength) {
		s.wonDuel(1-team, defIdx, 0.2)
		s.restartIdxBall(1-team, defIdx)
		return
	}
	s.players[team*11+idx].Acc += 0.04
	s.deliverPass(team, idx, sit, 0.04, false)
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
	case goalDist >= 0.16:
		attr = float64(carrier.LongShots)
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
		if s.rnd.Float64() < (1-quality(gk.Attr.Handling))*0.3 {
			// Spilled save: scramble in the six-yard box.
			s.owner = -1
			s.cooldown = 0
			s.ballX, s.ballY = s.players[(1-team)*11].X, s.players[(1-team)*11].Y
			return
		}
		s.restartIdxBall(1-team, 0)
	default:
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

// lapseConsequence turns a heavy touch or misplaced pass into a turnover; the worst
// ones surface as visible error events (rare — matches are judged by their highlights).
func (s *simState) lapseConsequence(team, idx int) {
	opp := 1 - team
	defIdx := s.nearestOpponent(opp, idx)
	if s.rnd.Float64() < errorEventShare {
		s.appendEvent(EventError, team, idx, "")
	}
	s.players[team*11+idx].Acc -= 0.25
	s.wonDuel(opp, defIdx, 0.1)
	s.restartIdxBall(opp, defIdx)
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
	s.score = addScore(s.score, team)
	ts := s.statsFor(team)
	ts.Goals++
	ts.OnTarget++
	s.players[team*11+idx].Acc += 1.5

	s.shiftMorale(team, +moraleGoalSwing)
	s.shiftMorale(1-team, -moraleGoalSwing)
	detail := "morale:+4|-4"
	if s.patTag != "" {
		detail += ";" + s.patTag
	}
	s.appendEvent(EventGoal, team, idx, detail)

	half := 0
	if s.secondHalf {
		half = 1
	}
	s.stoppage[half] = minf(s.stoppage[half]+20+float64(s.rnd.Intn(21)), maxHalfStoppage)

	s.ballX, s.ballY = 0.5, 0.5
	s.owner = int8(1 - team)
	s.ownerIdx = int8(restartIdx(s.onPitchFor(1 - team)))
	s.cooldown = s.actionCooldown(1 - team)
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
func (s *simState) contestLoose() {
	n0 := s.nearestToBall(0)
	n1 := s.nearestToBall(1)
	q0 := s.skill(0, n0, func(a Attributes) float64 { return quality(a.Positioning) })
	q1 := s.skill(1, n1, func(a Attributes) float64 { return quality(a.Positioning) })
	side, idx := 0, n0
	if s.rnd.Float64() >= duelChance(q0, q1) {
		side, idx = 1, n1
	}
	s.wonDuel(side, idx, 0.1)
	s.recordTouch(TouchRegain, int8(idx), -1, true)
	s.restartIdxBall(side, idx)
	s.cooldown = s.actionCooldown(side)
}

// ---- shared attribute foci and slot scans (index order ⇒ deterministic) ----

func aerialFocus(a Attributes) float64 {
	return 0.5*quality(a.Heading) + 0.3*quality(a.Jumping) + 0.2*quality(a.Strength)
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

// pickReceiver draws a playable teammate (skips keepers and sent-off slots).
func (s *simState) pickReceiver(team int) int {
	active := make([]int, 0, 11)
	for i := 0; i < 11; i++ {
		if !s.cards[team*11+i].Off && s.onPitch[team*11+i].Pos != PosGK {
			active = append(active, i)
		}
	}
	if len(active) == 0 {
		return 1
	}
	return active[s.rnd.Intn(len(active))]
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

// pickSmartReceiver chooses a pass target by openness and forward gain — the fix for
// "through on goal, squares it to the corner flag". Deterministic argmax (index-order
// ties) so snapshots stay reproducible.
func (s *simState) pickSmartReceiver(team, fromIdx int) int {
	dir := 1 - 2*float64(team)
	best, bestW := fromIdx, -1.0e9
	for i := 0; i < 11; i++ {
		if i == fromIdx || s.cards[team*11+i].Off {
			continue
		}
		if s.onPitch[team*11+i].Pos == PosGK {
			continue // keepers stay home; they are not passing outlets
		}
		ps := s.players[team*11+i]
		// Reachable support only: nobody laces 40 m balls at a distant silhouette.
		reach := absf(ps.X-s.ballX) + absf(ps.Y-s.ballY)
		if reach > 0.35 {
			continue
		}
		open := clamp(s.nearestDistTo(1-team, ps.X, ps.Y)/0.15, 0, 1)
		gain := (ps.X - s.ballX) * dir
		boxPresence := math.Max(0, 0.22-absf(goalXFor(team)-ps.X))
		// Nearby, open, helpful men win passes; speculative deep targets lose them.
		w := 0.5 + 1.1*math.Max(gain, 0) + 1.6*open + 0.6*boxPresence - 0.9*math.Max(-gain, 0) - 0.8*reach
		if w > bestW {
			bestW, best = w, i
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

// advanceBall moves ball and possession to a receiver. Progression is linear from
// the current ball position (a pass gains `length` upfield) — chains can build.
func (s *simState) advanceBall(team, receiver int, length float64) {
	_, ty := s.target(team, receiver, &s.players[team*11+receiver])
	dir := 1.0
	if team == 1 {
		dir = -1
	}
	rawX := s.ballX + dir*length
	if outOfPlay(rawX, ty) {
		s.ballOut(team, rawX, ty)
		return
	}
	s.ballX = clamp(rawX, 0.02, 0.98)
	s.ballY = clamp(ty, 0.04, 0.96)
	// Landing obeys role discipline: a centre-back cannot control beyond his cap —
	// the pass out-runs him and dies (all reception call-sites share this funnel).
	s.ballX, s.ballY = s.roleClamp(team, receiver, s.ballX, s.ballY)
	// The ball flies to the landing spot; the receiver RUNS to it (no teleporting
	// athletes — motion is earned at pace).
	s.owner, s.ownerIdx = int8(team), int8(receiver)
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
	s.wonDuel(defTeam, idx, 0.15)
	dir := 1.0
	if defTeam == 1 {
		dir = -1
	}
	s.ballX = clamp(s.players[defTeam*11+idx].X+dir*0.2, 0.02, 0.98)
	if s.rnd.Float64() < secondBallShare {
		s.owner = int8(attTeam)
		s.ownerIdx = int8(s.bestSlot(attTeam, aerialFocus))
		return
	}
	s.restartIdxBall(defTeam, idx)
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
