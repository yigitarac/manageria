package engine

// Pattern execution (ADR-0009: orchestrated, never scripted). Patterns walk timed
// route curves; the ordinary duel tables judge every contact. T-009 spike subset:
// corner routines with ≤4 actors and 2-point Hermite routes.
//
// Transient note: s.patTag is set and cleared inside one synchronous chain within a
// single tick — snapshots are only taken at loop boundaries, so it never needs to be
// part of the snapshot (and Resume equivalence holds trivially across it).

// hermite interpolates a segment with given endpoint tangents (plain arithmetic).
func hermite(p0, p1, tan0, tan1 [2]float64, u float64) [2]float64 {
	u2 := u * u
	u3 := u2 * u
	h00 := 2*u3 - 3*u2 + 1
	h10 := u3 - 2*u2 + u
	h01 := -2*u3 + 3*u2
	h11 := u3 - u2
	return [2]float64{
		h00*p0[0] + h10*tan0[0] + h01*p1[0] + h11*tan1[0],
		h00*p0[1] + h10*tan0[1] + h01*p1[1] + h11*tan1[1],
	}
}

// tangentAt computes a param-time slope vector for Hermite continuity.
func tangentAt(route []Waypoint, i int) [2]float64 {
	n := len(route)
	switch {
	case n <= 1:
		return [2]float64{0, 0}
	case i == 0:
		dt := route[1].T - route[0].T
		if dt <= 0 {
			return [2]float64{0, 0}
		}
		return [2]float64{(route[1].X - route[0].X) / dt, (route[1].Y - route[0].Y) / dt}
	case i == n-1:
		dt := route[i].T - route[i-1].T
		if dt <= 0 {
			return [2]float64{0, 0}
		}
		return [2]float64{(route[i].X - route[i-1].X) / dt, (route[i].Y - route[i-1].Y) / dt}
	default:
		dt := route[i+1].T - route[i-1].T
		if dt <= 0 {
			return [2]float64{0, 0}
		}
		return [2]float64{(route[i+1].X - route[i-1].X) / dt, (route[i+1].Y - route[i-1].Y) / dt}
	}
}

// routeAt evaluates an actor's smoothed route at time t (seconds from trigger).
// Points carry their own time marks; the curve is Hermite with endpoint tangents.
// Runners who cannot keep the schedule trail behind it — pace is the clock, and late
// arrivals earn worse marks grades when the ball arrives.
func routeAt(route []Waypoint, t float64) [2]float64 {
	if len(route) == 0 {
		return [2]float64{}
	}
	if t <= route[0].T {
		return [2]float64{route[0].X, route[0].Y}
	}
	last := route[len(route)-1]
	if t >= last.T {
		return [2]float64{last.X, last.Y}
	}
	for i := 0; i < len(route)-1; i++ {
		a, b := route[i], route[i+1]
		if t >= a.T && t < b.T {
			span := b.T - a.T
			if span <= 0 {
				return [2]float64{b.X, b.Y}
			}
			u := (t - a.T) / span
			return hermite(
				[2]float64{a.X, a.Y}, [2]float64{b.X, b.Y},
				tangentAt(route, i), tangentAt(route, i+1), u,
			)
		}
	}
	return [2]float64{last.X, last.Y}
}

// marksGrade scores how well a runner hit his finish mark (1 perfect → 0 hopeless).
func marksGrade(ps PlayerState, route []Waypoint) float64 {
	if len(route) == 0 {
		return 1
	}
	last := route[len(route)-1]
	d := absf(ps.X-last.X) + absf(ps.Y-last.Y)
	return clamp(1-4*d, 0, 1)
}

func goalXFor(team int) float64 {
	if team == 0 {
		return 1
	}
	return 0
}

// patternTarget returns the route point for a pattern actor at the current tick.
func (s *simState) patternTarget(team, idx int) ([2]float64, bool) {
	if !s.pattern.Active || int(s.pattern.Team) != team {
		return [2]float64{}, false
	}
	pats := s.tactics[team].Patterns
	if int(s.pattern.Index) >= len(pats) {
		return [2]float64{}, false
	}
	for _, a := range pats[s.pattern.Index].Actors {
		if a.Slot == idx {
			t := float64(s.tick - s.pattern.StartTick)
			return routeAt(a.Route, t), true
		}
	}
	return [2]float64{}, false
}

// armPattern arms the team's corner pattern when its corner is queued and stretches
// the walk-over so runners can make their timed runs.
func (s *simState) armPattern(team int) {
	if s.pattern.Active || len(s.tactics[team].Patterns) == 0 {
		return
	}
	s.pattern = PatternCursor{Active: true, Team: int8(team), Index: 0, StartTick: s.tick}
	s.restart.Ticks = patternSetupTicks
}

func (s *simState) clearPattern() {
	s.pattern = s.pattern.reset()
}

// resolvePatternCorner plays a choreographed corner and returns the index of the
// priority target who made the final contact (−1 when nobody did). Delivery is aimed
// with quality-scaled spread; contacts run the ordinary aerial duel tables with a
// hit-your-marks multiplier — patterns orchestrate, duels decide.
func (s *simState) resolvePatternCorner(team int, pat PatternSpec) int {
	s.patTag = "pattern:" + pat.ID
	defer func() { s.patTag = "" }()

	taker := s.setPieceTaker(team, func(a Attributes) float64 {
		return quality(a.SetPieces)*0.7 + quality(a.Crossing)*0.3
	})
	delivery := s.skill(team, taker, func(a Attributes) float64 {
		return 0.6*quality(a.SetPieces) + 0.4*quality(a.Crossing)
	})
	s.players[team*11+taker].Acc += 0.05

	spread := pat.Delivery.Spread * patternAimSpread * (2 - delivery)
	aimX := clamp(pat.Delivery.Aim[0]+(s.rnd.Float64()*2-1)*spread, 0.55, 0.98)
	aimY := clamp(pat.Delivery.Aim[1]+(s.rnd.Float64()*2-1)*spread, 0.05, 0.95)
	s.ballX, s.ballY = aimX, aimY

	if !s.cornerEntry(team, delivery) {
		return -1
	}

	goalDist := absf(goalXFor(team) - aimX)
	decay := 1.0 // successive contacts are broken balls, not clean headers
	for _, ti := range pat.Delivery.Targets {
		if ti < 0 || ti >= len(pat.Actors) {
			continue
		}
		actor := pat.Actors[ti]
		grade := marksGrade(s.players[team*11+actor.Slot], actor.Route)
		attQ := s.skill(team, actor.Slot, aerialFocus) *
			(0.7 + 0.3*delivery) *
			(patternMarkFloor + patternMarkBonus*grade) *
			decay
		defIdx := s.bestOpponentSlot(team, aerialFocus)
		defQ := s.skill(1-team, defIdx, aerialFocus) + s.manMarkBonus(1-team)
		s.players[team*11+actor.Slot].Acc += 0.1
		if s.rnd.Float64() < duelChance(attQ, defQ) {
			s.players[team*11+actor.Slot].Acc += 0.25
			s.ballX, s.ballY = s.players[team*11+actor.Slot].X, s.players[team*11+actor.Slot].Y
			s.finishShot(team, actor.Slot, true, goalDist, grade)
			return ti
		}
		decay *= contactChainDecay
		// Lost the header: the ball cannons on to the next priority target.
	}
	s.defensiveClear(team)
	return -1
}

// DrillOutcome classifies one rehearsal repetition.
type DrillOutcome struct {
	Outcome  string  // "goal" | "saved" | "cleared"
	XG       float64 // chance value booked for the attacking side
	MarksAvg float64 // average marks grade of the priority targets (patterns only)
	Target   int     // index of the actor who made the final contact (−1 = none)
}

// RehearseCorner runs one synthetic corner with the given pattern (nil = the default
// routine) under the given seed. The scene is the standard crowded six-yard box —
// deterministic, shared by rehearsal UX and drill tests ([[Tactics-Schema]]).
// Drills are run for the home side (team 0, attacking +x).
func RehearseCorner(in MatchInput, pattern *PatternSpec, seed uint64) (DrillOutcome, error) {
	in.Seed = seed
	if err := validate(in); err != nil {
		return DrillOutcome{}, err
	}
	s, err := newSimState(in, nil, nil)
	if err != nil {
		return DrillOutcome{}, err
	}
	s.placeCornerLayout(pattern)

	scoreBefore := s.score
	onTargetBefore := s.stats.Home.OnTarget
	xgBefore := s.stats.Home.XG

	target := -1
	if pattern != nil {
		// Setup phase: only the runners move (defences hold their set-piece marks,
		// like a real corner) — they chase their timed marks with pace-scaled bursts.
		for i := 0; i < patternSetupTicks; i++ {
			t := float64(i + 1)
			for _, a := range pattern.Actors {
				p := routeAt(a.Route, t)
				ps := &s.players[a.Slot]
				k := 0.10 + 0.15*quality(s.onPitch[a.Slot].Attr.Pace)
				ps.X = clamp(ps.X+(p[0]-ps.X)*k, 0.02, 0.98)
				ps.Y = clamp(ps.Y+(p[1]-ps.Y)*k, 0.04, 0.96)
			}
			_ = t
		}
		target = s.resolvePatternCorner(0, *pattern)
	} else {
		s.defaultCorner(0)
	}

	res := DrillOutcome{
		Outcome: "cleared",
		XG:      round2(s.stats.Home.XG - xgBefore),
		Target:  target,
	}
	switch {
	case s.score != scoreBefore:
		res.Outcome = "goal"
	case s.stats.Home.OnTarget > onTargetBefore:
		res.Outcome = "saved"
	}

	if pattern != nil {
		grades, n := 0.0, 0
		for _, ti := range pattern.Delivery.Targets {
			if ti < 0 || ti >= len(pattern.Actors) {
				continue
			}
			a := pattern.Actors[ti]
			grades += marksGrade(s.players[a.Slot], a.Route)
			n++
		}
		if n > 0 {
			res.MarksAvg = round2(grades / float64(n))
		}
	}
	return res, nil
}

// place arranges one player in the drill scene.
func (s *simState) place(slot int, x, y float64) {
	s.players[slot].X = x
	s.players[slot].Y = y
}

// placeCornerLayout arranges the deterministic crowded-box scene: keeper on the
// line, five-pack six-yard corridor, near/far post watchers and an edge screen for
// the defence; attackers on their route start marks (patterns) or near the spot.
func (s *simState) placeCornerLayout(pat *PatternSpec) {
	const team = 0
	const opp = 1

	s.owner = -1
	s.ownerIdx = 0
	s.cooldown = 0
	s.restart = Restart{}
	s.ballX, s.ballY = 0.95, 0.05

	// Defence (mirrored geometry: home attacks +x, their goal is at x = 1).
	s.place(opp*11+0, 0.97, 0.50)
	for i, y := range []float64{0.35, 0.42, 0.50, 0.58, 0.65} {
		s.place(opp*11+1+i, 0.90, y)
	}
	s.place(opp*11+6, 0.86, 0.30) // near post
	s.place(opp*11+7, 0.86, 0.70) // far post
	s.place(opp*11+8, 0.80, 0.45) // edge screen
	s.place(opp*11+9, 0.80, 0.60)
	s.place(opp*11+10, 0.93, 0.22)

	// Attack: taker by the flag, actors on their marks, support on the edge.
	taker := s.setPieceTaker(team, func(a Attributes) float64 {
		return quality(a.SetPieces)*0.7 + quality(a.Crossing)*0.3
	})
	s.place(team*11+taker, 0.93, 0.08)

	placed := map[int]bool{taker: true}
	if pat != nil {
		for _, a := range pat.Actors {
			if placed[a.Slot] {
				continue
			}
			p := routeAt(a.Route, a.Route[0].T)
			s.place(team*11+a.Slot, p[0], p[1])
			placed[a.Slot] = true
		}
	}
	support := [2][2]float64{{0.80, 0.42}, {0.80, 0.58}}
	si := 0
	for i := 0; i < 11 && si < len(support); i++ {
		if !placed[i] {
			s.place(team*11+i, support[si][0], support[si][1])
			si++
		}
	}
	for i := 0; i < 11; i++ {
		if !placed[i] {
			s.place(team*11+i, 0.62, 0.25+0.05*float64(i)) // cover stragglers back
		}
	}
}
