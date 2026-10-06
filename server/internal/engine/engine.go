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
	ErrBadLineup       = errors.New("engine: lineup must have 11-18 players with the goalkeeper first")
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
	onPitch    [22]PlayerSnapshot // current occupants: home 0..10, away 11..21
	players    [22]PlayerState
	ratings    []PlayerRating // deposited for substituted-off players
	ballX      float64
	ballY      float64
	owner      int8 // 0 home, 1 away, -1 loose
	ownerIdx   int8
	cooldown   int8
	restart    Restart
	cards      [22]CardState
	score      Score
	events     []Event
	stats      MatchStats
	possTicks  [2]int
	stoppage   [2]float64
	secondHalf bool
	tactics    [2]Tactics
	anchors    [2][11][2]float64
	snaps      []Snapshot
	keyframes  []Keyframe
	pending    []Intervention
	present    PresenceFlags
	pattern    PatternCursor
	patTag     string // transient attribution tag (set/cleared within one tick's chain)
}

func newSimState(in MatchInput, ivs []Intervention, snap *Snapshot) (*simState, error) {
	s := &simState{in: in, present: in.Presence}
	copy(s.onPitch[:11], in.Home.Players)
	copy(s.onPitch[11:], in.Away.Players)
	s.tactics = [2]Tactics{in.Home.Tactics, in.Away.Tactics}
	s.recomputeAnchors(0)
	s.recomputeAnchors(1)

	pending := slices.Clone(ivs)
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].EffectiveTick < pending[j].EffectiveTick
	})

	if snap != nil {
		rnd, err := rng.FromState(snap.RND)
		if err != nil {
			return nil, fmt.Errorf("restoring rng: %w", err)
		}
		s.rnd = rnd
		s.tick = snap.Tick
		s.score = snap.Score
		s.events = slices.Clone(snap.Events)
		s.stats = snap.Stats
		s.possTicks = snap.PossTicks
		s.onPitch = snap.OnPitch
		s.players = snap.Players
		s.ratings = slices.Clone(snap.Ratings)
		s.ballX, s.ballY = snap.BallX, snap.BallY
		s.owner, s.ownerIdx, s.cooldown = snap.Owner, snap.OwnerIdx, snap.Cooldown
		s.restart = snap.Restart
		s.cards = snap.Cards
		s.stoppage = snap.Stoppage
		s.secondHalf = snap.SecondHalf
		s.tactics = snap.Tactics
		s.pattern = snap.Pattern
		s.recomputeAnchors(0)
		s.recomputeAnchors(1)
		s.pending = pendingFilterFrom(pending, snap.Tick)
		return s, nil
	}

	s.rnd = rng.New(in.Seed)
	s.owner = 0
	s.ownerIdx = int8(restartIdx(s.onPitchFor(0)))
	s.ballX, s.ballY = 0.5, 0.5
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ps := s.onPitch[team*11+i]
			s.players[team*11+i] = PlayerState{X: s.anchors[team][i][0], Y: s.anchors[team][i][1], Morale: ps.Morale}
		}
	}
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
		s.appendEvent(EventKickOff, 0, restartIdx(s.onPitchFor(0)), "")
	}
	for s.tick < int32(regulationTicks+int(s.stoppage[0]+s.stoppage[1])) {
		if s.tick == halfTicks && !s.secondHalf {
			s.secondHalf = true
			s.appendEvent(EventHalfTime, 1, restartIdx(s.onPitchFor(1)), "")
			s.ballX, s.ballY = 0.5, 0.5
			s.owner = 1
			s.ownerIdx = int8(restartIdx(s.onPitchFor(1)))
			s.cooldown = 0
			s.restart = Restart{}
			s.clearPattern()
		}
		if err := s.applyInterventions(); err != nil {
			return err
		}
		if s.pattern.Active && s.tick-s.pattern.StartTick > patternMaxTicks {
			s.clearPattern()
		}
		if s.tick%keyframeEvery == 0 {
			s.keyframes = append(s.keyframes, s.sampleKeyframe())
		}
		s.move()
		s.fatigue()
		switch {
		case s.restart.Kind != RestartNone:
			s.tickRestart()
		case s.cooldown > 0:
			s.cooldown--
		default:
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
		case InterventionTactics:
			var tac Tactics
			if err := json.Unmarshal(iv.Payload, &tac); err != nil {
				return fmt.Errorf("decoding tactics intervention: %w", err)
			}
			if err := validateTactics(tac); err != nil {
				return err
			}
			s.tactics[team] = tac
			s.recomputeAnchors(team)
		case InterventionSubstitution:
			var sub SubstitutionPayload
			if err := json.Unmarshal(iv.Payload, &sub); err != nil {
				return fmt.Errorf("decoding substitution intervention: %w", err)
			}
			if err := s.substitute(team, sub); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %s", ErrBadIntervention, iv.Kind)
		}
	}
	return nil
}

// substitute swaps an on-pitch player for a bench player of the same club.
func (s *simState) substitute(team int, sub SubstitutionPayload) error {
	off := -1
	for i := 0; i < 11; i++ {
		if s.onPitch[team*11+i].ID == sub.PlayerOff {
			off = i
			break
		}
	}
	if off < 0 {
		return fmt.Errorf("%w: %s is not on the pitch", ErrBadIntervention, sub.PlayerOff)
	}

	squad := s.in.Home.Players
	if team == 1 {
		squad = s.in.Away.Players
	}
	in := -1
	for i := 11; i < len(squad); i++ {
		if squad[i].ID == sub.PlayerOn {
			in = i
			break
		}
	}
	if in < 0 {
		return fmt.Errorf("%w: %s is not on the bench", ErrBadIntervention, sub.PlayerOn)
	}

	slot := team*11 + off
	// Deposit the departing player's rating accumulator; the incoming player starts fresh.
	s.ratings = append(s.ratings, PlayerRating{PlayerID: s.onPitch[slot].ID, Rating: s.players[slot].Acc})
	departing := s.onPitch[slot].Name
	old := s.players[slot]
	s.onPitch[slot] = squad[in]
	s.players[slot] = PlayerState{X: old.X, Y: old.Y, Morale: squad[in].Morale}
	s.cards[slot] = CardState{}
	s.appendEvent(EventSubstitution, team, off, "replaces "+departing)
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
// The ball carrier gets a slow forward carry target instead of his formation anchor:
// otherwise the anchor pull drags possession backwards between touches.
func (s *simState) move() {
	for team := 0; team < 2; team++ {
		dir := 1.0
		if team == 1 {
			dir = -1
		}
		for i := 0; i < 11; i++ {
			if s.cards[team*11+i].Off {
				continue
			}
			ps := &s.players[team*11+i]
			eff := 1 - 0.3*math.Min(ps.Fatigue/100, 1)
			k := 0.05 + 0.05*quality(s.onPitch[team*11+i].Attr.Pace)*eff
			tx, ty := s.target(team, i, ps)
			if pt, ok := s.patternTarget(team, i); ok {
				// Pattern runs are bursts: pace-scaled sprint easing toward the moving mark.
				tx, ty = pt[0], pt[1]
				k = 0.10 + 0.15*quality(s.onPitch[team*11+i].Attr.Pace)*eff
			} else if s.owner == int8(team) && s.ownerIdx == int8(i) {
				tx = clamp(ps.X+dir*0.015, 0.02, 0.98)
				ty = clamp(ps.Y+(0.5-ps.Y)*0.05, 0.04, 0.96)
			}
			ps.X = clamp(ps.X+(tx-ps.X)*k, 0.02, 0.98)
			ps.Y = clamp(ps.Y+(ty-ps.Y)*k, 0.04, 0.96)
		}
	}
	switch {
	case s.restart.Kind != RestartNone:
		s.ballX, s.ballY = s.restart.X, s.restart.Y
	case s.owner < 0:
		s.ballX = clamp(s.ballX+(0.5-s.ballX)*0.05, 0.02, 0.98)
		s.ballY = clamp(s.ballY+(0.5-s.ballY)*0.05, 0.04, 0.96)
	default:
		op := &s.players[int(s.owner)*11+int(s.ownerIdx)]
		s.ballX, s.ballY = op.X, op.Y
	}
}

// target computes one player's tactical destination in absolute coordinates.
func (s *simState) target(team, idx int, ps *PlayerState) (float64, float64) {
	ax, ay := s.anchors[team][idx][0], s.anchors[team][idx][1]

	tac := s.tactics[team]
	blockShift := (float64(tac.Mentality)-3)*0.02 + (float64(tac.DefensiveLine)-2)*0.03
	blockShift += s.gameStateShift(team)
	if int8(team) == s.owner {
		blockShift += 0.04
	} else {
		blockShift -= 0.02
	}
	width := 0.55 + 0.15*float64(tac.Width)
	if idx > 0 {
		if team == 0 {
			ax += blockShift
		} else {
			ax -= blockShift
		}
		ay = 0.5 + (ay-0.5)*width
	}

	dx, dy := s.ballX-ax, s.ballY-ay
	d2 := dx*dx + dy*dy
	pull := 0.10 / (1 + 8*d2)
	return clamp(ax+dx*pull, 0.02, 0.98), clamp(ay+dy*pull, 0.04, 0.96)
}

// fatigue drains condition shaped by work rate, pressing, tempo and stamina.
func (s *simState) fatigue() {
	pressT := s.tactics[maxInt(int(s.owner), 0)]
	press := float64(pressT.Pressing)
	tempo := float64(maxInt(int(pressT.Tempo)-3, 0))
	drain := 0.003 + 0.002*press + tempoDrain*tempo
	pressIntensity := float64(maxInt(int(pressT.Pressing)-1, 0)) / 2
	attackIntensity := float64(maxInt(int(pressT.Mentality)-3, 0)) / 2
	drain *= 1 + comboFatigue*pressIntensity*attackIntensity
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			if s.cards[team*11+i].Off {
				continue
			}
			ps := &s.players[team*11+i]
			stamina := quality(s.onPitch[team*11+i].Attr.Stamina)
			work := quality(s.onPitch[team*11+i].Attr.WorkRate)
			ps.Fatigue = math.Min(ps.Fatigue+drain*(0.6+0.6*work)*(1-0.4*stamina), 100)
		}
	}
}

// perf is the visible performance multiplier: home advantage, morale band (±3%),
// live-presence boost, fatigue decay and carrying an injury. Nothing hidden beyond
// these (explainable results principle).
func (s *simState) perf(team, idx int) float64 {
	ps := s.players[team*11+idx]
	p := s.onPitch[team*11+idx]
	morale := 1 + moraleBand*(ps.Morale-50)/50
	presence := 1.0
	if (team == 0 && s.present.Home) || (team == 1 && s.present.Away) {
		presence = presenceBoost
	}
	home := 1.0
	if team == 0 && !s.in.Context.NeutralVenue {
		home = homeAdvantage
	}
	fatigue := 1 - 0.3*math.Min(ps.Fatigue/100, 1)
	hurt := 1.0
	if ps.Hurt {
		hurt = 1 - hurtPenalty
	}
	return morale * presence * home * fatigue * hurt * conditionFactor(p.Condition)
}

func conditionFactor(condition float64) float64 {
	return 0.9 + 0.1*condition/100
}

func (s *simState) actionCooldown(team int) int8 {
	return int8(maxInt(baseCooldown-int(s.tactics[team].Tempo), minCooldown))
}

func (s *simState) appendEvent(kind string, team, idx int, detail string) {
	tick := s.tick
	s.events = append(s.events, Event{
		Tick:   tick,
		Minute: tick/60 + 1,
		Kind:   kind,
		Club:   s.teamClub(team),
		Player: s.onPitch[team*11+idx].ID,
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

func (s *simState) onPitchFor(team int) []PlayerSnapshot {
	return s.onPitch[team*11 : team*11+11]
}

// scoreDiff returns (own − opponent) goals for a team.
func (s *simState) scoreDiff(team int) int {
	if team == 0 {
		return s.score.Home - s.score.Away
	}
	return s.score.Away - s.score.Home
}

// gameStateShift tilts the block with the scoreboard: trailing teams push up,
// leaders manage the game. Clamped to ±2 goals.
func (s *simState) gameStateShift(team int) float64 {
	d := s.scoreDiff(team)
	if d > 2 {
		d = 2
	}
	if d < -2 {
		d = -2
	}
	shift := -float64(d) * gameStateShift
	if d > 0 {
		shift -= float64(d) * leaderManage
	}
	return shift
}

func (s *simState) avgFatigue(team int) float64 {
	sum := 0.0
	for i := 0; i < 11; i++ {
		sum += s.players[team*11+i].Fatigue
	}
	return sum / 11
}

func avgShotDist(ts TeamStats) float64 {
	if ts.Shots == 0 {
		return 0
	}
	return round2(ts.DistSum / float64(ts.Shots))
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
		switch {
		case s.cards[i].Off:
			state = KFIdle
		case i == int(s.owner)*11+int(s.ownerIdx) && s.cooldown == 0:
			state = KFKick
		case absf(s.ballX-ps.X)+absf(s.ballY-ps.Y) < 0.12:
			state = KFDuel
		case absf(ps.X-s.anchors[i/11][i%11][0]) > 0.03:
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
		OnPitch:    s.onPitch,
		Players:    s.players,
		Ratings:    slices.Clone(s.ratings),
		BallX:      s.ballX,
		BallY:      s.ballY,
		Owner:      s.owner,
		OwnerIdx:   s.ownerIdx,
		Cooldown:   s.cooldown,
		Restart:    s.restart,
		Cards:      s.cards,
		Pattern:    s.pattern,
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
	res.Stats.Home.XG = round2(res.Stats.Home.XG)
	res.Stats.Away.XG = round2(res.Stats.Away.XG)
	res.Stats.Home.AvgFatigue = round2(s.avgFatigue(0))
	res.Stats.Away.AvgFatigue = round2(s.avgFatigue(1))
	res.Stats.Home.AvgShotDist = avgShotDist(res.Stats.Home)
	res.Stats.Away.AvgShotDist = avgShotDist(res.Stats.Away)

	ratings := slices.Clone(s.ratings) // substituted-off players (accumulators)
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ratings = append(ratings, PlayerRating{
				PlayerID: s.onPitch[team*11+i].ID,
				Rating:   s.players[team*11+i].Acc,
			})
		}
	}
	for i := range ratings {
		ratings[i].Rating = round2(clamp(5.5+math.Min(ratings[i].Rating, 3.5), 3, 10))
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
	for ; i < 11; i++ {
		table[i] = table[i-1]
	}
	s.anchors[team] = table
}

// restartIdx picks a midfield conductor slot (scanned in lineup order — deterministic).
func restartIdx(team []PlayerSnapshot) int {
	for i, p := range team {
		if p.Pos == PosCM && i > 0 {
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

func absf(v float64) float64 {
	return math.Abs(v)
}

func minf(a, b float64) float64 {
	return math.Min(a, b)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func validate(in MatchInput) error {
	if in.EngineVersion != EngineVersion {
		return fmt.Errorf("%w: input %d, engine %d", ErrEngineVersion, in.EngineVersion, EngineVersion)
	}
	seen := make(map[PlayerID]struct{}, 36)
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
	if len(t.Players) < 11 || len(t.Players) > 18 || t.Players[0].Pos != PosGK {
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
		tac.Tackling >= 1 && tac.Tackling <= 2 &&
		tac.SetPieces.CornerRoutine >= 1 && tac.SetPieces.CornerRoutine <= 4 &&
		tac.SetPieces.FreeKickRoutine >= 1 && tac.SetPieces.FreeKickRoutine <= 3
	if !ok {
		return ErrBadTactics
	}
	if len(tac.Patterns) > 10 {
		return fmt.Errorf("%w: more than 10 patterns", ErrBadTactics)
	}
	for _, pat := range tac.Patterns {
		if err := validatePattern(pat); err != nil {
			return err
		}
	}
	return nil
}

// validatePattern enforces the T-009 spike subset of the Tactics-Schema grammar.
func validatePattern(pat PatternSpec) error {
	if len(pat.Actors) < 1 || len(pat.Actors) > 4 {
		return fmt.Errorf("%w: pattern %q needs 1..4 actors (spike subset)", ErrBadTactics, pat.ID)
	}
	for i, a := range pat.Actors {
		if a.Slot < 0 || a.Slot > 10 {
			return fmt.Errorf("%w: pattern %q actor slot out of range", ErrBadTactics, pat.ID)
		}
		for j := 0; j < i; j++ {
			if pat.Actors[j].Slot == a.Slot {
				return fmt.Errorf("%w: pattern %q uses slot %d twice", ErrBadTactics, pat.ID, a.Slot)
			}
		}
		if len(a.Route) != 2 || a.Route[0].T >= a.Route[1].T || a.Route[1].T > 8 {
			return fmt.Errorf("%w: pattern %q actor routes are 2 timed marks (T0 < T1 ≤ 8)", ErrBadTactics, pat.ID)
		}
		for _, wp := range a.Route {
			if wp.X < 0 || wp.X > 1 || wp.Y < 0 || wp.Y > 1 {
				return fmt.Errorf("%w: pattern %q waypoint out of pitch", ErrBadTactics, pat.ID)
			}
		}
	}
	if len(pat.Delivery.Targets) == 0 ||
		pat.Delivery.Kind < 1 || pat.Delivery.Kind > 4 ||
		pat.Delivery.Spread < 0 || pat.Delivery.Spread > 1 ||
		pat.Delivery.Power < 0 || pat.Delivery.Power > 1 {
		return fmt.Errorf("%w: pattern %q delivery incomplete", ErrBadTactics, pat.ID)
	}
	for _, ti := range pat.Delivery.Targets {
		if ti < 0 || ti >= len(pat.Actors) {
			return fmt.Errorf("%w: pattern %q target index out of range", ErrBadTactics, pat.ID)
		}
	}
	return nil
}
