package engine

// Dead-ball machinery: pending restarts, delivery tables for goal kicks, throw-ins,
// corners, free kicks and penalties. Deliveries resolve after a short "walk over".

// queueRestart freezes play and schedules a dead-ball delivery.
func (s *simState) queueRestart(kind RestartKind, team int, x, y float64) {
	s.restart = Restart{Kind: kind, Team: int8(team), X: clamp(x, 0.02, 0.98), Y: clamp(y, 0.04, 0.96), Ticks: restartPauseTicks}
	s.owner = -1
	s.cooldown = 0
	s.ballX, s.ballY = s.restart.X, s.restart.Y
}

// tickRestart advances the walk-over and fires the delivery when due.
func (s *simState) tickRestart() {
	if s.restart.Kind == RestartNone {
		return
	}
	if s.restart.Ticks > 0 {
		s.restart.Ticks--
		return
	}
	s.resolveRestart()
}

// resolveRestart plays out the dead-ball situation.
func (s *simState) resolveRestart() {
	r := s.restart
	s.restart = Restart{Kind: RestartNone}
	team := int(r.Team)

	switch r.Kind {
	case RestartGoalKick:
		s.goalKick(team)
	case RestartThrowIn:
		s.throwIn(team, r)
	case RestartCorner:
		s.corner(team)
	case RestartFreeKick:
		s.freeKick(team, r)
	case RestartPenalty:
		s.penalty(team)
	}
	s.cooldown = s.actionCooldown(team)
}

// restartIdxBall hands possession to a player (loose-ball and interception ends).
// Possession flips count as turnovers for the losing team (tactical sanity stats).
func (s *simState) restartIdxBall(team, idx int) {
	if s.owner >= 0 && s.owner != int8(team) {
		s.statsFor(int(s.owner)).Turnovers++
	}
	s.owner, s.ownerIdx = int8(team), int8(idx)
	op := &s.players[team*11+idx]
	s.ballX, s.ballY = op.X, op.Y
}

// ballOut awards the restart implied by the ball leaving the pitch. Attacker dribbles
// over the byline mostly yield a goal kick; corners come from deflections behind.
func (s *simState) ballOut(lastTeam int, x, y float64) {
	opp := 1 - lastTeam
	s.owner = -1
	switch {
	case y <= 0.04 || y >= 0.96:
		s.queueRestart(RestartThrowIn, opp, x, clamp(y, 0.03, 0.97))
	case (lastTeam == 0 && x <= 0.02) || (lastTeam == 1 && x >= 0.98):
		s.queueRestart(RestartGoalKick, lastTeam, 0.08+0.84*float64(lastTeam), 0.5)
	default:
		if s.rnd.Float64() < 0.35 {
			s.queueRestart(RestartCorner, lastTeam, x, y)
			return
		}
		s.queueRestart(RestartGoalKick, 1-lastTeam, 0.08+0.84*float64(1-lastTeam), 0.5)
	}
}

// setPieceTaker returns the designated taker slot, or the deterministic best option.
func (s *simState) setPieceTaker(team int, prefer func(Attributes) float64) int {
	desired := s.tactics[team].SetPieces.Taker
	for i := 0; i < 11; i++ {
		if s.cards[team*11+i].Off {
			continue
		}
		if s.onPitch[team*11+i].ID == desired {
			return i
		}
	}
	return s.bestSlot(team, prefer)
}

// goalKick: short to a defender or long to contest the aerial ball.
func (s *simState) goalKick(team int) {
	distrQ := s.skill(team, 0, func(a Attributes) float64 {
		return 0.7*quality(a.Distribution) + 0.3*quality(a.Passing)
	})
	s.players[team*11].Acc += 0.05
	if s.rnd.Float64() < 0.5+0.2*(distrQ-0.5) {
		to := s.bestSlot(team, func(a Attributes) float64 { return quality(a.Passing) })
		s.advanceBall(team, to, 0.05)
		return
	}
	// Route one: long ball up for a physical duel.
	att := s.bestSlot(team, aerialFocus)
	def := s.bestOpponentSlot(team, aerialFocus)
	s.ballX, s.ballY = 0.5, 0.5
	if s.rnd.Float64() < duelChance(s.skill(team, att, aerialFocus), s.skill(1-team, def, aerialFocus)) {
		s.wonDuel(team, att, 0.15)
		s.advanceBall(team, att, 0.12)
		return
	}
	s.wonDuel(1-team, def, 0.15)
	s.restartIdxBall(1-team, def)
}

// throwIn: cheap retention with a small contest risk.
func (s *simState) throwIn(team int, r Restart) {
	thrower := s.bestSlot(team, func(a Attributes) float64 { return quality(a.Passing) })
	s.ballX, s.ballY = r.X, r.Y
	pLost := 0.25 - 0.10*quality(s.onPitch[team*11+thrower].Attr.Passing)
	if s.rnd.Float64() < pLost {
		defIdx := s.nearestTo(1-team, r.X, r.Y)
		s.restartIdxBall(1-team, defIdx)
		return
	}
	to := s.pickReceiver(team)
	s.advanceBall(team, to, 0.03)
	s.players[team*11+thrower].Acc += 0.05
}

// corner: delivery quality vs the box battle, with keeper claims and second balls.
func (s *simState) corner(team int) {
	s.statsFor(team).Corners++
	taker := s.setPieceTaker(team, func(a Attributes) float64 {
		return quality(a.SetPieces)*0.7 + quality(a.Crossing)*0.3
	})
	delivery := s.skill(team, taker, func(a Attributes) float64 { return quality(a.SetPieces) })
	s.players[team*11+taker].Acc += 0.05

	if s.tactics[team].SetPieces.CornerRoutine == 4 {
		// Short corner: recycle into open play around the box.
		s.ballX, s.ballY = s.cornerSpot(team, 0.72)
		s.advanceBall(team, s.pickReceiver(team), 0.02)
		return
	}
	if s.rnd.Float64() < keeperClaimShare*duelChance(
		quality(s.onPitch[(1-team)*11].Attr.Handling)*s.perf(1-team, 0), delivery) {
		s.players[(1-team)*11].Acc += 0.1
		s.restartIdxBall(1-team, 0)
		return
	}
	if s.rnd.Float64() < clearanceShare {
		s.defensiveClear(team)
		return
	}

	att := s.bestSlot(team, aerialFocus)
	def := s.bestOpponentSlot(team, aerialFocus)
	marking := 0.0
	if s.tactics[1-team].Marking == 2 {
		marking = 0.06 // man-oriented marking bites on set pieces
	}
	attQ := s.skill(team, att, aerialFocus) * (0.7 + 0.3*delivery)
	defQ := s.skill(1-team, def, aerialFocus) + marking
	if s.rnd.Float64() >= duelChance(attQ, defQ) {
		s.wonDuel(1-team, def, 0.2)
		s.restartIdxBall(1-team, def)
		return
	}
	s.players[team*11+att].Acc += 0.25
	s.ballX, s.ballY = s.players[team*11+att].X, s.players[team*11+att].Y
	if s.rnd.Float64() < 0.6 {
		s.finishShot(team, att, true, 0.10)
		return
	}
	support := s.pickReceiver(team)
	s.advanceBall(team, support, 0.03)
}

// freeKick: shoot, cross or lay-off per the routine.
func (s *simState) freeKick(team int, r Restart) {
	taker := s.setPieceTaker(team, func(a Attributes) float64 {
		return quality(a.SetPieces)*0.5 + quality(a.LongShots)*0.3 + quality(a.Finishing)*0.2
	})
	s.ballX, s.ballY = r.X, r.Y
	goalX := 1.0
	if team == 1 {
		goalX = 0.0
	}
	dist := absf(goalX - r.X)

	switch s.tactics[team].SetPieces.FreeKickRoutine {
	case 3:
		// Lay-off: quick restart into feet.
		s.players[team*11+taker].Acc += 0.05
		s.deliverPass(team, taker, situation{goalDist: dist, press: 0.3}, 0.06, false)
	case 2:
		// Wide free kick treated as an early cross.
		s.players[team*11+taker].Acc += 0.05
		s.cross(team, taker, situation{goalDist: dist, wide: true, press: 0.3})
	default:
		if dist < 0.30 {
			s.directFreeKick(team, taker, dist)
			return
		}
		// Too far to shoot: default to the cross.
		s.cross(team, taker, situation{goalDist: dist, wide: true, press: 0.3})
	}
}

// directFreeKick: set-piece technique over the wall against the keeper.
func (s *simState) directFreeKick(team, taker int, dist float64) {
	strike := s.skill(team, taker, func(a Attributes) float64 {
		return quality(a.LongShots)*0.5 + quality(a.SetPieces)*0.3 + quality(a.Composure)*0.2
	})
	gk := s.onPitch[(1-team)*11]
	stop := (quality(gk.Attr.ShotStopping)*0.7 + quality(gk.Attr.Agility)*0.3) * s.perf(1-team, 0)
	pGoal := clamp(0.35*strike*(1-gkSaveShare*stop)*(1-0.4*clamp(dist/0.3, 0, 1)), 0.01, 0.30)

	ts := s.statsFor(team)
	ts.Shots++
	ts.XG += pGoal
	ts.DistSum += dist
	s.players[team*11+taker].Acc += 0.3

	roll := s.rnd.Float64()
	switch {
	case roll < pGoal:
		s.addGoal(team, taker)
	case roll < pGoal+0.35:
		ts.OnTarget++
		s.appendEvent(EventShotSaved, team, taker, "free_kick")
		s.restartIdxBall(1-team, 0)
	default:
		s.appendEvent(EventShotOffTarget, team, taker, "free_kick")
		s.queueRestart(RestartGoalKick, 1-team, 0.08+0.84*float64(1-team), 0.5)
	}
}

// penalty: finishing/composure/set pieces vs the keeper (classic ~0.76 baseline).
func (s *simState) penalty(team int) {
	taker := s.setPieceTaker(team, func(a Attributes) float64 {
		return quality(a.Finishing)*0.5 + quality(a.Composure)*0.3 + quality(a.SetPieces)*0.2
	})
	strike := s.skill(team, taker, func(a Attributes) float64 {
		return quality(a.Finishing)*0.5 + quality(a.Composure)*0.3 + quality(a.SetPieces)*0.2
	})
	gk := s.onPitch[(1-team)*11]
	stop := (quality(gk.Attr.ShotStopping)*0.6 + quality(gk.Attr.Agility)*0.2 + quality(gk.Attr.Positioning)*0.2) *
		s.perf(1-team, 0)
	pGoal := clamp(penaltyBaseGoal*duelChance(strike, stop)+0.10, 0.45, 0.92)

	ts := s.statsFor(team)
	ts.Shots++
	ts.XG += pGoal
	ts.OnTarget++
	s.players[team*11+taker].Acc += 0.3

	if s.rnd.Float64() < pGoal {
		s.addGoal(team, taker)
		return
	}
	s.appendEvent(EventShotSaved, team, taker, "penalty")
	s.restartIdxBall(1-team, 0)
}

// cornerSpot returns a corner-taking coordinate for the attacking team.
func (s *simState) cornerSpot(team int, depth float64) (float64, float64) {
	if team == 0 {
		return clamp(depth, 0.02, 0.98), 0.05
	}
	return clamp(1-depth, 0.02, 0.98), 0.95
}

// outOfPlay reports whether a ball position has crossed a pitch boundary.
func outOfPlay(x, y float64) bool {
	return y < 0.04 || y > 0.96 || x < 0.02 || x > 0.98
}