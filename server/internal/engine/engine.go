package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// Sentinel errors for invalid inputs. The engine never panics on bad data.
var (
	ErrEngineVersion   = errors.New("engine: match input engine version mismatch")
	ErrBadLineup       = errors.New("engine: lineup must contain exactly 11 players with the goalkeeper first")
	ErrDuplicatePlayer = errors.New("engine: duplicate player id")
	ErrBadAttribute    = errors.New("engine: attribute out of range 1..20")
	ErrBadCondition    = errors.New("engine: condition or morale out of range 0..100")
	ErrBadTactics      = errors.New("engine: invalid tactics")
	ErrBadIntervention = errors.New("engine: unsupported intervention")
)

const (
	regulationTicks = 90 * 60
	halfTicks       = regulationTicks / 2
	keyframeEvery   = 3 // game seconds between keyframes
	snapshotEvery   = 60
	maxHalfStoppage = 300.0
)

// Simulate runs a match without interventions (the revision 0 pre-simulation).
func Simulate(in MatchInput) (MatchResult, []Snapshot, error) {
	return run(in, nil, nil)
}

// SimulateIntervened runs a whole match honouring the given intervention list.
// Used as the baseline for re-simulation equivalence tests and for replays.
func SimulateIntervened(in MatchInput, ivs []Intervention) (MatchResult, []Snapshot, error) {
	return run(in, ivs, nil)
}

// Resume continues a match from a snapshot with a (possibly revised) intervention
// list; interventions effective before the snapshot are already baked into its state.
// The returned result covers the resumed window: Events keeps the snapshot prefix,
// Keyframes covers ticks from the snapshot onwards.
func Resume(snap Snapshot, in MatchInput, ivs []Intervention) (MatchResult, []Snapshot, error) {
	return run(in, ivs, &snap)
}

func run(in MatchInput, ivs []Intervention, snap *Snapshot) (MatchResult, []Snapshot, error) {
	if err := validate(in); err != nil {
		return MatchResult{}, nil, err
	}
	s, err := newSimState(in, ivs, snap)
	if err != nil {
		return MatchResult{}, nil, err
	}
	if err := s.loop(); err != nil {
		return MatchResult{}, nil, err
	}
	return s.result(), s.snaps, nil
}

// simState is the whole match machine. Everything in here must survive a snapshot.
type simState struct {
	in         MatchInput
	rnd        *rng.Rand
	tick       int32
	pss        [22]PlayerSnapshot // static view: home 0..10, away 11..21
	players    [22]PlayerState
	ballX      float64
	ballY      float64
	owner      int8 // 0 home, 1 away, -1 loose
	ownerIdx   int8
	cooldown   int8
	score      Score
	events     []Event
	stats      MatchStats
	possTicks  [2]int
	stoppage   [2]float64
	secondHalf bool
	tactics    [2]Tactics
	anchors    [2][11][2]float64 // formation anchors in absolute coordinates
	snaps      []Snapshot
	keyframes  []Keyframe
	pending    []Intervention
	present    PresenceFlags
}

func newSimState(in MatchInput, ivs []Intervention, snap *Snapshot) (*simState, error) {
	s := &simState{in: in, present: in.Presence}
	copy(s.pss[:11], in.Home.Players)
	copy(s.pss[11:], in.Away.Players)
	s.tactics = [2]Tactics{in.Home.Tactics, in.Away.Tactics}
	s.recomputeAnchors(0)
	s.recomputeAnchors(1)

	pending := slices.Clone(ivs)
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].EffectiveTick < pending[j].EffectiveTick
	})

	if snap != nil {
		s.rnd, _ = rng.FromState(snap.RND)
		s.tick = snap.Tick
		s.score = snap.Score
		s.events = slices.Clone(snap.Events)
		s.stats = snap.Stats
		s.possTicks = snap.PossTicks
		s.players = snap.Players
		s.ballX, s.ballY = snap.BallX, snap.BallY
		s.owner, s.ownerIdx, s.cooldown = snap.Owner, snap.OwnerIdx, snap.Cooldown
		s.stoppage = snap.Stoppage
		s.secondHalf = snap.SecondHalf
		s.tactics = snap.Tactics
		s.recomputeAnchors(0)
		s.recomputeAnchors(1)
		s.pending = pendingFilterFrom(pending, snap.Tick)
		return s, nil
	}

	s.rnd = rng.New(in.Seed)
	s.owner, s.ownerIdx, s.cooldown = 0, int8(restartIdx(s.pss[:11])), 0
	s.ballX, s.ballY = 0.5, 0.5
	s.players = initialPositions(s.anchors)
	s.pending = pending
	return s, nil
}

func pendingFilterFrom(ivs []Intervention, from int32) []Intervention {
	out := ivs[:0]
	for _, iv := range ivs {
		if iv.EffectiveTick >= from {
			out = append(out, iv)
		}
	}
	return out
}

func (s *simState) loop() error {
	if s.tick == 0 {
		s.emit(EventKickOff, 0, restartIdx(s.pss[:11]))
	}
	for s.tick < int32(regulationTicks+int(s.stoppage[0]+s.stoppage[1])) {
		if s.tick == halfTicks && !s.secondHalf {
			s.secondHalf = true
			s.emit(EventHalfTime, 1, restartIdx(s.pss[11:]))
			s.ballX, s.ballY = 0.5, 0.5
			s.cooldown = 0
		}
		if err := s.applyInterventions(); err != nil {
			return err
		}
		if s.tick%keyframeEvery == 0 {
			s.keyframes = append(s.keyframes, s.sampleKeyframe())
		}
		s.move()
		s.fatigue()
		if s.cooldown > 0 {
			s.cooldown--
		} else {
			s.act()
		}
		if s.owner >= 0 {
			s.possTicks[s.owner]++
		}
		s.tick++
		if s.tick%snapshotEvery == 0 {
			s.snaps = append(s.snaps, s.snapshot())
		}
	}
	return nil
}

// applyInterventions honours every intervention whose effective tick has been reached.
func (s *simState) applyInterventions() error {
	for len(s.pending) > 0 && s.pending[0].EffectiveTick <= s.tick {
		iv := s.pending[0]
		s.pending = s.pending[1:]

		team, err := s.clubTeam(iv.Club)
		if err != nil {
			return err
		}
		switch iv.Kind {
		case "tactics":
			var tac Tactics
			if err := json.Unmarshal(iv.Payload, &tac); err != nil {
				return fmt.Errorf("decoding tactics intervention: %w", err)
			}
			if err := validateTactics(tac); err != nil {
				return err
			}
			s.tactics[team] = tac
			s.recomputeAnchors(team)
		default:
			return fmt.Errorf("%w: %s", ErrBadIntervention, iv.Kind)
		}
	}
	return nil
}

func (s *simState) clubTeam(club string) (int, error) {
	switch club {
	case s.in.Home.Club:
		return 0, nil
	case s.in.Away.Club:
		return 1, nil
	default:
		return 0, fmt.Errorf("%w: club %q not in match", ErrBadIntervention, club)
	}
}

// move integrates players toward their tactical targets and eases the ball along.
func (s *simState) move() {
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ps := &s.players[team*11+i]
			tx, ty := s.target(team, i, ps)
			eff := 1 - 0.3*math.Min(ps.Fatigue/100, 1)
			k := 0.05 + 0.05*(float64(s.pss[team*11+i].Attr.Pace)/20)*eff
			ps.X = clamp(ps.X+(tx-ps.X)*k, 0.02, 0.98)
			ps.Y = clamp(ps.Y+(ty-ps.Y)*k, 0.04, 0.96)
		}
	}
	// Loose ball settles slowly toward its spot; carried ball sits with its owner.
	if s.owner < 0 {
		s.ballX = clamp(s.ballX+(0.5-s.ballX)*0.05, 0.02, 0.98)
		s.ballY = clamp(s.ballY+(0.5-s.ballY)*0.05, 0.04, 0.96)
		return
	}
	op := &s.players[int(s.owner)*11+int(s.ownerIdx)]
	s.ballX, s.ballY = op.X, op.Y
}

// target computes one player's tactical destination in absolute coordinates.
func (s *simState) target(team, idx int, ps *PlayerState) (float64, float64) {
	ax, ay := s.anchors[team][idx][0], s.anchors[team][idx][1]

	tac := s.tactics[team]
	blockShift := (float64(tac.Mentality)-3)*0.02 + (float64(tac.DefensiveLine)-2)*0.03
	if int8(team) == s.owner {
		blockShift += 0.04
	} else {
		blockShift -= 0.02
	}
	width := 0.55 + 0.15*float64(tac.Width)
	if idx > 0 { // outfielders shift with the block, keeper stays
		if team == 0 {
			ax += blockShift
		} else {
			ax -= blockShift
		}
		ay = 0.5 + (ay-0.5)*width
	}

	// Ball attraction: nearer players bend toward the ball zone.
	dx, dy := s.ballX-ax, s.ballY-ay
	d2 := dx*dx + dy*dy
	pull := 0.10 / (1 + 8*d2)
	return clamp(ax+dx*pull, 0.02, 0.98), clamp(ay+dy*pull, 0.04, 0.96)
}

func (s *simState) fatigue() {
	press := float64(s.tactics[maxInt(int(s.owner), 0)].Pressing)
	drain := 0.003 + 0.002*press
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ps := &s.players[team*11+i]
			stamina := float64(s.pss[team*11+i].Attr.Stamina) / 20
			work := float64(s.pss[team*11+i].Attr.WorkRate) / 20
			ps.Fatigue = math.Min(ps.Fatigue+drain*(0.6+0.6*work)*(1-0.4*stamina), 100)
		}
	}
}

// act resolves one action for the ball carrier (skeleton action tables).
func (s *simState) act() {
	if s.owner < 0 {
		s.contestLoose()
		return
	}
	team, idx := int(s.owner), int(s.ownerIdx)
	carrier := &s.pss[team*11+idx]
	dir, goalX := 1.0, 1.0
	if team == 1 {
		dir, goalX = -1, 0.0
	}
	dist := math.Abs(goalX - s.ballX)

	shootProb := 0.0
	if dist < 0.32 {
		shootProb = 0.25 + 0.25*(float64(carrier.Attr.Finishing)-10)/20
	}
	if s.rnd.Float64() < shootProb {
		s.resolveShot(team, idx, dist)
		return
	}
	s.resolveBuildUp(team, idx, dir)
}

func (s *simState) resolveShot(team, idx int, dist float64) {
	carrier := &s.pss[team*11+idx]
	gk := &s.pss[(1-team)*11]

	attr := float64(carrier.Attr.Finishing)
	if dist >= 0.18 {
		attr = float64(carrier.Attr.LongShots)
	}
	quality := (attr / 20) * s.perf(team, idx)
	stop := (float64(gk.Attr.ShotStopping) / 20) * s.perf(1-team, 0)
	distFactor := 1 - 0.6*math.Min(dist/0.32, 1)
	pGoal := clamp(0.55*quality*(1-0.6*stop)*distFactor, 0.01, 0.55)
	pSave := clamp(0.35*quality-0.2*stop, 0.05, 0.6)

	ts := s.statsFor(team)
	ts.Shots++
	s.players[team*11+idx].Acc += 0.3

	roll := s.rnd.Float64()
	switch {
	case roll < pGoal:
		s.addGoal(team, idx)
	case roll < pGoal+pSave:
		ts.OnTarget++
		s.appendEvent(EventShotSaved, team, idx, "")
		s.restart(1-team, 0, 0.10+0.80*float64(1-team))
	default:
		s.appendEvent(EventShotOffTarget, team, idx, "")
		s.restart(1-team, 0, 0.08+0.84*float64(1-team))
	}
	s.cooldown = s.actionCooldown(team)
}

func (s *simState) addGoal(team, idx int) {
	s.score = addScore(s.score, team)
	ts := s.statsFor(team)
	ts.Goals++
	ts.OnTarget++
	s.players[team*11+idx].Acc += 1.5
	s.appendEvent(EventGoal, team, idx, "")
	half := 0
	if s.secondHalf {
		half = 1
	}
	s.stoppage[half] = math.Min(s.stoppage[half]+20+float64(s.rnd.Intn(21)), maxHalfStoppage)
	s.restart(1-team, restartIdx(s.pss[(1-team)*11:(1-team)*11+11]), 0.5)
}

func (s *simState) resolveBuildUp(team, idx int, dir float64) {
	carrier := &s.pss[team*11+idx]
	oppTac := s.tactics[1-team]
	myTac := s.tactics[team]

	carrierPass := (float64(carrier.Attr.Passing) / 20) * s.perf(team, idx)
	pressure := 0.10 + 0.05*float64(oppTac.Pressing) + 0.03*float64(oppTac.Tackling)
	turnoverP := clamp(pressure-0.25*carrierPass-0.05*(float64(carrier.Attr.FirstTouch)/20)+0.12, 0.03, 0.65)

	receiver := s.rnd.Intn(11) // fixed-order draw
	if s.rnd.Float64() < turnoverP {
		defIdx := s.rnd.Intn(11)
		s.players[(1-team)*11+defIdx].Acc += 0.2
		s.restartIdxBall(1-team, defIdx)
		s.cooldown = s.actionCooldown(1 - team)
		return
	}

	vision := float64(carrier.Attr.Vision) / 20
	dribble := float64(carrier.Attr.Dribbling) / 20
	advance := 0.015 + 0.01*float64(myTac.PassingStyle)/3 + 0.015*vision*s.perf(team, idx)
	if myTac.CounterAttack {
		advance += 0.01 * dribble
	}
	s.ballX = clamp(s.ballX+dir*advance, 0.02, 0.98)
	_, ry := s.target(team, receiver, &s.players[team*11+receiver])
	s.ballY = clamp(s.ballY+(ry-s.ballY)*0.5, 0.04, 0.96)
	s.players[team*11+idx].Acc += 0.05
	s.ownerIdx = int8(receiver)
	s.cooldown = s.actionCooldown(team)
}

func (s *simState) contestLoose() {
	side := 0
	if s.rnd.Float64() >= 0.5 {
		side = 1
	}
	idx := s.rnd.Intn(11)
	s.restartIdxBall(side, idx)
	s.cooldown = s.actionCooldown(side)
}

// restart places a restart for a team at a spot; keeper restarts put him on the ball.
func (s *simState) restart(team, idx int, x float64) {
	y := 0.5
	if idx != 0 {
		_, y = s.target(team, idx, &s.players[team*11+idx])
	}
	s.owner, s.ownerIdx = int8(team), int8(idx)
	s.ballX, s.ballY = x, y
}

func (s *simState) restartIdxBall(team, idx int) {
	s.owner, s.ownerIdx = int8(team), int8(idx)
	op := &s.players[team*11+idx]
	s.ballX, s.ballY = op.X, op.Y
}

func (s *simState) actionCooldown(team int) int8 {
	return int8(maxInt(9-int(s.tactics[team].Tempo), 2))
}

// perf is the visible performance multiplier: morale band (±3%), live-presence boost
// and fatigue decay. Nothing hidden beyond these (explainable results principle).
func (s *simState) perf(team, idx int) float64 {
	ps := s.players[team*11+idx]
	p := s.pss[team*11+idx]
	morale := 1 + 0.03*(p.Morale-50)/50
	presence := 1.0
	if (team == 0 && s.present.Home) || (team == 1 && s.present.Away) {
		presence = 1.03
	}
	fatigue := 1 - 0.3*math.Min(ps.Fatigue/100, 1)
	return morale * presence * fatigue
}

func (s *simState) emit(kind string, team, idx int) {
	s.appendEvent(kind, team, idx, "")
}

func (s *simState) appendEvent(kind string, team, idx int, detail string) {
	tick := s.tick
	s.events = append(s.events, Event{
		Tick:   tick,
		Minute: tick/60 + 1,
		Kind:   kind,
		Club:   s.teamClub(team),
		Player: s.pss[team*11+idx].ID,
		Detail: detail,
	})
}

func (s *simState) teamClub(team int) string {
	if team == 0 {
		return s.in.Home.Club
	}
	return s.in.Away.Club
}

func (s *simState) statsFor(team int) *TeamStats {
	if team == 0 {
		return &s.stats.Home
	}
	return &s.stats.Away
}

func (s *simState) sampleKeyframe() Keyframe {
	kf := Keyframe{
		TMs:       uint32(s.tick) * 1000,
		BallX:     s.ballX,
		BallY:     s.ballY,
		BallOwner: s.owner,
	}
	for i := range s.players {
		ps := s.players[i]
		state := byte(KFIdle)
		if i == int(s.owner)*11+int(s.ownerIdx) && s.cooldown == 0 {
			state = KFKick
		} else if math.Abs(s.ballX-ps.X)+math.Abs(s.ballY-ps.Y) < 0.12 {
			state = KFDuel
		} else if math.Abs(ps.X-s.anchors[i/11][i%11][0]) > 0.03 {
			state = KFRun
		}
		kf.Players[i] = KFPlayer{X: ps.X, Y: ps.Y, State: state}
	}
	return kf
}

func (s *simState) snapshot() Snapshot {
	return Snapshot{
		Tick:       s.tick,
		Score:      s.score,
		Events:     slices.Clone(s.events),
		Stats:      s.stats,
		PossTicks:  s.possTicks,
		Players:    s.players,
		BallX:      s.ballX,
		BallY:      s.ballY,
		Owner:      s.owner,
		OwnerIdx:   s.ownerIdx,
		Cooldown:   s.cooldown,
		Stoppage:   s.stoppage,
		SecondHalf: s.secondHalf,
		Tactics:    s.tactics,
		RND:        s.rnd.State(),
	}
}

func (s *simState) result() MatchResult {
	total := s.possTicks[0] + s.possTicks[1]
	res := MatchResult{
		Score:     s.score,
		Events:    s.events,
		Stats:     s.stats,
		Keyframes: s.keyframes,
	}
	if total > 0 {
		res.Stats.Home.PossessionPct = math.Round(1000*float64(s.possTicks[0])/float64(total)) / 10
		res.Stats.Away.PossessionPct = math.Round(1000*float64(s.possTicks[1])/float64(total)) / 10
	}
	ratings := make([]PlayerRating, 0, 22)
	for i := range s.players {
		// Rating grows with concrete involvements (passes, duels, shots, goals).
		rating := clamp(5.5+math.Min(s.players[i].Acc, 3.5), 3, 10)
		ratings = append(ratings, PlayerRating{
			PlayerID: s.pss[i].ID,
			Rating:   math.Round(rating*100) / 100,
		})
	}
	sort.SliceStable(ratings, func(i, j int) bool {
		return ratings[i].PlayerID < ratings[j].PlayerID
	})
	res.PlayerRatings = ratings
	return res
}

// recomputeAnchors rebuilds formation anchors for one team (absolute coordinates).
func (s *simState) recomputeAnchors(team int) {
	rows := s.tactics[team].Formation
	xs := [3]float64{0.22, 0.45, 0.68}
	table := [11][2]float64{}
	table[0] = [2]float64{0.05, 0.5} // goalkeeper
	i := 1
	for row, n := range rows {
		for j := 0; j < n && i < 11; j++ {
			ax, ay := xs[row], (float64(j)+0.5)/float64(n)
			if team == 1 {
				ax = 1 - ax
			}
			table[i] = [2]float64{ax, ay}
			i++
		}
	}
	for ; i < 11; i++ { // defensive formations may leave spare slots
		table[i] = table[i-1]
	}
	s.anchors[team] = table
}

func initialPositions(anchors [2][11][2]float64) [22]PlayerState {
	var ps [22]PlayerState
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ps[team*11+i] = PlayerState{X: anchors[team][i][0], Y: anchors[team][i][1]}
		}
	}
	return ps
}

// restartIdx picks a midfield conductor slot (scanned in lineup order — deterministic).
func restartIdx(team []PlayerSnapshot) int {
	for i, p := range team {
		if p.Pos == PosCM {
			return i
		}
	}
	return 4
}

func addScore(sc Score, team int) Score {
	if team == 0 {
		sc.Home++
	} else {
		sc.Away++
	}
	return sc
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(v, hi))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func validate(in MatchInput) error {
	if in.EngineVersion != EngineVersion {
		return fmt.Errorf("%w: input %d, engine %d", ErrEngineVersion, in.EngineVersion, EngineVersion)
	}
	seen := make(map[PlayerID]struct{}, 22)
	for team, t := range []TeamSnapshot{in.Home, in.Away} {
		if err := validateTeam(t); err != nil {
			return fmt.Errorf("team %d: %w", team, err)
		}
		for _, p := range t.Players {
			if _, dup := seen[p.ID]; dup {
				return fmt.Errorf("%w: %s", ErrDuplicatePlayer, p.ID)
			}
			seen[p.ID] = struct{}{}
		}
	}
	return nil
}

func validateTeam(t TeamSnapshot) error {
	if len(t.Players) != 11 || t.Players[0].Pos != PosGK {
		return ErrBadLineup
	}
	for _, p := range t.Players {
		for _, a := range p.Attr.all() {
			if a < 1 || a > 20 {
				return fmt.Errorf("%w: player %s", ErrBadAttribute, p.ID)
			}
		}
		if p.Condition < 0 || p.Condition > 100 || p.Morale < 0 || p.Morale > 100 {
			return fmt.Errorf("%w: player %s", ErrBadCondition, p.ID)
		}
	}
	return validateTactics(t.Tactics)
}

func validateTactics(tac Tactics) error {
	sum := 0
	for _, n := range tac.Formation {
		if n < 1 || n > 6 {
			return fmt.Errorf("%w: formation rows out of range", ErrBadTactics)
		}
		sum += n
	}
	ok := sum == 10 &&
		tac.Mentality >= 1 && tac.Mentality <= 5 &&
		tac.Pressing >= 1 && tac.Pressing <= 3 &&
		tac.Tempo >= 1 && tac.Tempo <= 5 &&
		tac.Width >= 1 && tac.Width <= 3 &&
		tac.PassingStyle >= 1 && tac.PassingStyle <= 3 &&
		tac.DefensiveLine >= 1 && tac.DefensiveLine <= 3 &&
		tac.Marking >= 1 && tac.Marking <= 2 &&
		tac.Tackling >= 1 && tac.Tackling <= 2
	if !ok {
		return ErrBadTactics
	}
	return nil
}
