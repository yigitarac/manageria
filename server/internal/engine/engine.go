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
	in           MatchInput
	rnd          *rng.Rand
	tick         int32
	onPitch      [22]PlayerSnapshot // current occupants: home 0..10, away 11..21
	players      [22]PlayerState
	ratings      []PlayerRating // deposited for substituted-off players
	ballX        float64
	ballY        float64
	owner        int8 // 0 home, 1 away, -1 loose
	ownerIdx     int8
	cooldown     int8
	kickoffTeam  int8
	kickoffTicks int8
	restart      Restart
	cards        [22]CardState
	score        Score
	events       []Event
	stats        MatchStats
	possTicks    [2]int
	stoppage     [2]float64
	secondHalf   bool
	tactics      [2]Tactics
	anchors      [2][11][2]float64
	snaps        []Snapshot
	keyframes    []Keyframe
	pending      []Intervention
	present      PresenceFlags
	pattern      PatternCursor
	patTag       string // transient attribution tag (set/cleared within one tick's chain)
	// Possession-chain bookkeeping (sensory layer, ADR-0011).
	chainID    int32
	chainTeam  int8
	chainStart int32
	chainReg   Regime
	chainGone  int32
	touches    []Touch
	chains     []ChainInfo
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
		s.kickoffTeam, s.kickoffTicks = snap.KickoffTeam, snap.KickoffTicks
		s.restart = snap.Restart
		s.cards = snap.Cards
		s.stoppage = snap.Stoppage
		s.secondHalf = snap.SecondHalf
		s.tactics = snap.Tactics
		s.pattern = snap.Pattern
		s.chainID = snap.ChainID
		s.chainTeam = snap.ChainTeam
		s.chainStart = snap.ChainStart
		s.chainReg = snap.ChainReg
		s.chainGone = snap.ChainGone
		s.touches = slices.Clone(snap.Touches)
		s.chains = slices.Clone(snap.Chains)
		s.recomputeAnchors(0)
		s.recomputeAnchors(1)
		s.pending = pendingFilterFrom(pending, snap.Tick)
		return s, nil
	}

	s.rnd = rng.New(in.Seed)
	s.owner = 0
	s.ownerIdx = int8(restartIdx(s.onPitchFor(0)))
	s.ballX, s.ballY = 0.5, 0.5
	s.chainTeam = -1
	s.openChain(0, "kickoff")
	for team := 0; team < 2; team++ {
		for i := 0; i < 11; i++ {
			ps := s.onPitch[team*11+i]
			x, y := s.kickoffPosition(team, i, 0)
			s.players[team*11+i] = PlayerState{X: x, Y: y, Morale: ps.Morale}
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
			s.closeChainIfLive("half")
			s.restart = Restart{}
			s.clearPattern()
			s.beginKickoff(1)
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
		case s.kickoffTicks > 0:
			s.kickoffTicks--
			if s.kickoffTicks == 0 {
				s.kickOff(int(s.kickoffTeam))
				s.appendEvent(EventKickOff, int(s.kickoffTeam), restartIdx(s.onPitchFor(int(s.kickoffTeam))), "")
			}
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
		s.senseChain()
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
			if !s.patternActiveFor(team, i) && s.beyondRoleCap(team, i) {
				// Caught upfield after a set piece? Recover shape at a scramble.
				k *= 2
			}
			tx, ty := s.target(team, i, ps)
			patterned := false
			if s.kickoffTicks > 0 {
				tx, ty = s.kickoffPosition(team, i, int(s.kickoffTeam))
				k = 0.32
				patterned = true
			} else if pt, ok := s.patternTarget(team, i); ok {
				// Pattern runs are bursts: pace-scaled sprint easing toward the moving mark.
				tx, ty = pt[0], pt[1]
				k = 0.10 + 0.15*quality(s.onPitch[team*11+i].Attr.Pace)*eff
				patterned = true
			} else if rt, ok := s.frontJobRun(team, i); ok {
				// Runner jobs burst too: breaking runs and box arrivals are sprints.
				tx, ty = rt[0], rt[1]
				k = 0.07 + 0.12*quality(s.onPitch[team*11+i].Attr.Pace)*eff
				patterned = true
			} else if s.owner == int8(team) && s.ownerIdx == int8(i) {
				// Collect first (sprint to where the ball landed), then carry with
				// breathing room — no teleporting, no ball-on-a-leash dragging.
				if absf(ps.X-s.ballX)+absf(ps.Y-s.ballY) >= 0.05 {
					tx, ty = s.ballX, s.ballY
				} else if s.nearestDistTo(1-team, ps.X, ps.Y) > carryBrakeDist {
					tx = clamp(ps.X+dir*0.015, 0.02, 0.98)
					ty = clamp(ps.Y+(0.5-ps.Y)*0.05, 0.04, 0.96)
				} else {
					tx, ty = ps.X, ps.Y
				}
			}
			if s.kickoffTicks == 0 && s.owner == int8(1-team) && i > 0 && s.engagedRank(team, i) < int(s.tactics[team].Pressing) {
				k = 0.14 + 0.10*quality(s.onPitch[team*11+i].Attr.Acceleration)*eff
			}
			if !patterned {
				tx, ty = s.roleClamp(team, i, tx, ty)
				tx, ty = s.defensiveRoleTarget(team, i, tx, ty)
			}
			stepX, stepY := (tx-ps.X)*k, (ty-ps.Y)*k
			// Runner jobs may switch targets instantly, but feet cannot cross a
			// quarter of the field between two 3-second viewer frames.
			const maxMovePerTick = 0.049
			if distance := math.Hypot(stepX, stepY); distance > maxMovePerTick {
				stepX *= maxMovePerTick / distance
				stepY *= maxMovePerTick / distance
			}
			ps.X = clamp(ps.X+stepX, 0.02, 0.98)
			ps.Y = clamp(ps.Y+stepY, 0.04, 0.96)
		}
	}
	switch {
	case s.kickoffTicks > 0:
		s.ballX, s.ballY = 0.5, 0.5
	case s.restart.Kind != RestartNone:
		s.ballX, s.ballY = s.restart.X, s.restart.Y
	case s.owner < 0:
		s.ballX = clamp(s.ballX+(0.5-s.ballX)*0.05, 0.02, 0.98)
		s.ballY = clamp(s.ballY+(0.5-s.ballY)*0.05, 0.04, 0.96)
	default:
		op := &s.players[int(s.owner)*11+int(s.ownerIdx)]
		// The ball waits where it landed until the receiver collects it — flight is
		// faster than feet, and dragging it back to his hips erases the pass's progress.
		if absf(op.X-s.ballX)+absf(op.Y-s.ballY) < 0.05 {
			s.ballX, s.ballY = op.X, op.Y
		}
	}
}

// beyondRoleCap reports a player stranded beyond their open-play territory (used to
// hurry defenders home after set-piece box visits).
func (s *simState) beyondRoleCap(team, idx int) bool {
	maxAttack := 0.98
	switch s.onPitch[team*11+idx].Pos {
	case PosGK:
		maxAttack = 0.18
	case PosCB:
		maxAttack = roleMaxXCB
	case PosDM:
		maxAttack = roleMaxXDM
	}
	x := s.players[team*11+idx].X
	if team == 0 {
		return x > maxAttack+0.03
	}
	return x < 1-maxAttack-0.03
}

// patternActiveFor reports that this slot is currently running a pattern route.
func (s *simState) patternActiveFor(team, idx int) bool {
	_, ok := s.patternTarget(team, idx)
	return ok
}

// frontJobRun is the stage-B runner job (T-014 units & jobs): the front cast does
// NOT shuffle with the block when the chain's playbook calls for movement. Jobs per
// the ADR-0011 vocabulary: ST pins the box corridor (pinCBs), W HOLDS WIDTH on the
// flank (holdWidth), AM arrives on the edge (arriveEdge); transition raids let all of
// them break BEHIND the ball (counters have runners). Returns the run target when the
// job fires.
func (s *simState) frontJobRun(team, idx int) ([2]float64, bool) {
	// The carrier owns the ball's story (collect, then carry) — runners run FOR him.
	if s.owner != int8(team) || s.ownerIdx == int8(idx) {
		return [2]float64{}, false
	}
	var job byte
	switch s.onPitch[team*11+idx].Pos {
	case PosST:
		job = 1 // pinCBs
	case PosW:
		job = 2 // holdWidth
	case PosAM:
		job = 3 // arriveEdge
	default:
		return [2]float64{}, false
	}
	switch s.chainReg {
	case RegimeTransition, RegimeFinalThird:
	case RegimeProgression:
		// handled below (middle-third support)
	default:
		return [2]float64{}, false
	}
	dir := 1.0
	if team == 1 {
		dir = -1
	}
	ps := s.players[team*11+idx]
	parity := float64(idx % 2) // deterministic near/far assignment
	gx := goalXFor(team)

	if s.chainReg == RegimeProgression {
		// Middle-third support: offer forward outlets instead of holding anchors, so
		// the carrier is not forced backwards. Strikers check into the channel, wide
		// men stay wide but drop level with the ball, the AM drifts between the lines.
		switch job {
		case 1:
			return [2]float64{clamp(s.ballX+dir*progressionRunLead, 0.02, 0.98), 0.40 + 0.20*parity}, true
		case 2:
			return [2]float64{clamp(s.ballX+dir*progressionWideLead, 0.02, 0.98), 0.10 + 0.80*parity}, true
		default:
			return [2]float64{clamp(s.ballX+dir*progressionAMLead, 0.02, 0.98), 0.42 + 0.16*parity}, true
		}
	}
	if s.chainReg == RegimeTransition {
		// Break into the space ahead of the ball — the counter's spearhead.
		// Wingers run the flanks, strikers the channels, AM trails the wave.
		x := clamp(s.ballX+dir*counterSurge*(1+0.3*parity), 0.02, 0.98)
		y := ps.Y
		switch job {
		case 1:
			y = 0.42 + 0.16*parity
		case 2:
			y = 0.12 + 0.76*parity // hugging one flank
		case 3:
			y = ps.Y + (0.5-ps.Y)*0.4
		}
		return [2]float64{x, clamp(y, 0.04, 0.96)}, true
	}
	// Settled final-third attack: the jobs occupy complementary heights instead of
	// a three-man goalmouth pile (that caricature fed itself to death).
	switch job {
	case 1: // pinCBs: the striker holds the corridor at the six-yard lane.
		return [2]float64{clamp(gx-dir*boxPinDepth, 0.02, 0.98), 0.38 + 0.24*parity}, true
	case 2: // holdWidth: the winger hugs the flank and stretches the back line.
		return [2]float64{clamp(gx-dir*widthHoldDepth, 0.02, 0.98), 0.08 + 0.84*parity}, true
	default: // arriveEdge: the AM ghosts onto the cutback zone at the box edge.
		return [2]float64{clamp(gx-dir*arriveEdgeDepth, 0.02, 0.98), 0.44 + 0.12*parity}, true
	}
}

// roleClamp keeps open-play shape honest: goalkeepers and centre-backs do not holiday
// upfield (pattern actors are routed separately and bypass this).
func (s *simState) roleClamp(team, idx int, tx, ty float64) (float64, float64) {
	maxAttack := 0.98
	switch s.onPitch[team*11+idx].Pos {
	case PosGK:
		maxAttack = 0.18
	case PosCB:
		maxAttack = roleMaxXCB
	case PosDM:
		maxAttack = roleMaxXDM
	}
	if team == 0 {
		tx = math.Min(tx, maxAttack)
	} else {
		tx = math.Max(tx, 1-maxAttack)
	}
	return tx, ty
}

// defensiveRoleTarget brings a striker back toward the central outlet after a
// defensive restart or chase. He can help in his own half, but cannot become a
// permanent full-back just because he happened to be nearest during a scramble.
// Dead balls and rehearsed runs keep their own targets until play resumes.
func (s *simState) defensiveRoleTarget(team, idx int, tx, ty float64) (float64, float64) {
	if s.owner != int8(1-team) || s.restart.Kind != RestartNone || s.onPitch[team*11+idx].Pos != PosST {
		return tx, ty
	}
	attackX := tx
	if team == 1 {
		attackX = 1 - tx
	}
	if attackX >= 0.30 || (ty >= 0.20 && ty <= 0.80) {
		return tx, ty
	}
	if team == 0 {
		tx = math.Max(tx, 0.30)
	} else {
		tx = math.Min(tx, 0.70)
	}
	return tx, clamp(ty, 0.20, 0.80)
}

// compactDepth enforces the defending block's vertical compactness budget
// (T-021, DEF-3). While the opponents hold the ball, the holding block is read as
// LINES (same-depth neighbours within lineMerge cluster together) and every line
// ahead of the last rides at most compactGap of pitch depth beyond the line
// beneath it. The last line is the reference (its plane = the deepest quartile's
// median, robust against one straggling recovery run) and is never lifted: the
// budget only pulls dangling support back, it never marches a deep block upfield.
func (s *simState) compactDepth(team, idx int, tx float64) float64 {
	pressing := int(s.tactics[team].Pressing)
	type mate struct {
		idx int
		ax  float64 // live depth, attack-normalized (0 = own goal)
	}
	var mates []mate
	for i := 1; i < 11; i++ {
		if s.cards[team*11+i].Off || s.engagedRank(team, i) < pressing {
			continue // the chasers earn their own ball-focused targets
		}
		ax := s.players[team*11+i].X
		if team == 1 {
			ax = 1 - ax
		}
		mates = append(mates, mate{i, ax})
	}
	if len(mates) < 4 {
		return tx
	}
	sort.SliceStable(mates, func(a, b int) bool {
		if mates[a].ax != mates[b].ax {
			return mates[a].ax < mates[b].ax
		}
		return mates[a].idx < mates[b].idx
	})
	anchor := (mates[1].ax + mates[2].ax) / 2 // median of the four deepest

	// Cluster into lines deepest-first and assign each line its depth ceiling.
	ceil := make(map[int]float64, len(mates))
	lastCeil := 0.0
	armed := false
	for i := 0; i < len(mates); {
		j := i
		top, plane := mates[i].ax, 0.0
		for ; j < len(mates) && mates[j].ax-mates[i].ax <= lineMerge; j++ {
			top = mates[j].ax
			plane += mates[j].ax
		}
		plane /= float64(j - i)
		if plane < anchor {
			// The last line of resistance (and laggards behind it) keep their depth.
			i = j
			continue
		}
		if !armed {
			lastCeil, armed = top, true // the reference line: unmoved
		} else {
			lastCeil += compactGap
		}
		for k := i; k < j; k++ {
			ceil[mates[k].idx] = lastCeil
		}
		i = j
	}

	lim, ok := ceil[idx]
	if !ok {
		return tx
	}
	ax := tx
	if team == 1 {
		ax = 1 - tx
	}
	if ax <= lim {
		return tx
	}
	ax = lim
	if team == 1 {
		return 1 - ax
	}
	return ax
}

// ---- possession-chain lifecycle (sensory layer, ADR-0011) ----

// setOwner hands possession to (team, idx) and maintains the chain lifecycle at the
// flip: the old chain dies with an honest outcome, the newborn one is classified by
// its birth circumstances (the `cause`). Turnover bookkeeping happens here too —
// ownership flips are the losing team's turnovers.
func (s *simState) setOwner(team, idx int, cause string) {
	if s.owner >= 0 && s.owner != int8(team) {
		s.statsFor(int(s.owner)).Turnovers++
	}
	if s.liveChain() && s.chainTeam != int8(team) {
		s.closeChain("turnover")
	}
	s.owner, s.ownerIdx = int8(team), int8(idx)
	if !s.liveChain() {
		if cause == "" {
			cause = "regain"
		}
		s.openChain(int8(team), cause)
	}
}

// closeChainIfLive ends the live chain with the given outcome (a shot, a goal, a
// dead ball). Safe when the chain already ended (e.g. a goal after the shot touch).
func (s *simState) closeChainIfLive(outcome string) {
	if s.liveChain() {
		s.closeChain(outcome)
	}
}

// senseChain is the per-tick safety net: ownership normally already carries a chain
// (setOwner opens them at the flip), and a loose ball that vanishes into limbo ends
// the stalled chain as a dead ball.
func (s *simState) senseChain() {
	if s.owner < 0 {
		s.chainGone++
		if s.chainGone > 15 {
			s.closeChainIfLive("dead_ball")
		}
		return
	}
	s.chainGone = 0
	if !s.liveChain() {
		s.openChain(s.owner, "regain")
	} else if s.chainTeam != s.owner {
		s.closeChain("turnover")
		s.openChain(s.owner, "regain")
	}
}

// recordTouch writes one on-ball action into the sensory ledger.
func (s *simState) recordTouch(kind string, actor, target int8, success bool) {
	if !s.liveChain() {
		return // dead-ball mechanics are events, not open-play touches
	}
	s.touches = append(s.touches, Touch{
		Tick:    s.tick,
		Chain:   s.chainID,
		Team:    s.chainTeam,
		Kind:    kind,
		Actor:   actor,
		Target:  target,
		Success: success,
		X:       s.ballX,
		Y:       s.ballY,
		Press:   round2(s.challengePressure(int(s.chainTeam), int(actor))),
	})
}

func (s *simState) openChain(team int8, cause string) {
	s.chainID++ // monoton kimlik: her zincir benzersiz (defter bununla sayılıyor)
	s.chainTeam = team
	s.chainStart = s.tick
	s.chainGone = 0
	s.chainReg = s.classifyRegime(team, cause)
}

func (s *simState) closeChain(outcome string) {
	if s.chainID == 0 {
		return
	}
	touches := 0
	for _, t := range s.touches {
		if t.Chain == s.chainID {
			touches++
		}
	}
	s.chains = append(s.chains, ChainInfo{
		ID:      s.chainID,
		Team:    s.chainTeam,
		Regime:  s.chainReg.String(),
		Start:   s.chainStart,
		End:     s.tick,
		Touches: touches,
		Outcome: outcome,
	})
	// NOTE: chainID stays monotonic (it is the ledger key); "no live chain" is the
	// zero value of chainTeam guard below.
	s.chainTeam = -1
}

// liveChain reports whether a chain is currently being written.
func (s *simState) liveChain() bool {
	return s.chainTeam >= 0
}

// classifyRegime reads the playbook the newborn possession should run: where and why
// it was born decides everything (v6 spec — regimes, not dice). A fresh win high up
// against a stretched opponent is a counter window (transition); a settled win in the
// attacking third is sustained pressure (finalThird) — they are not the same play.
func (s *simState) classifyRegime(team int8, cause string) Regime {
	switch cause {
	case "setpiece":
		return RegimeSetPiece
	case "regroup":
		return RegimeRegroup
	}
	x := s.attackX(int(team))
	if cause == "regain" && s.counterWindow(int(team)) {
		// Counter windows: a ball won anywhere in space is a raid for prepared
		// counter-punch outfits (their depth chart IS the counter), a bit further
		// upfield for everybody else.
		if x > 0.45 || (s.tactics[team].CounterAttack && x > 0.35) {
			return RegimeTransition
		}
	}
	switch {
	case x < 0.35:
		return RegimeBuildUp
	case x < 0.68:
		return RegimeProgression
	default:
		return RegimeFinalThird
	}
}

// attackX maps an absolute x to the team's attacking frame (0 own goal → 1 theirs).
func (s *simState) attackX(team int) float64 {
	if team == 0 {
		return s.ballX
	}
	return 1 - s.ballX
}

// counterWindow reports a raid opportunity: counter-attack football always hunts it,
// and any side finds it when the opponent's line is caught upfield.
func (s *simState) counterWindow(team int) bool {
	if s.tactics[team].CounterAttack {
		return true
	}
	line := s.defensiveLineX(1 - team)
	if team == 1 {
		line = 1 - line
	}
	return line > 0.60
}

// finalChains flushes the live chain (with its outcome) at full time.
func (s *simState) finalChains() []ChainInfo {
	if s.liveChain() {
		s.closeChain("half")
	}
	return s.chains
}

// kickOff restarts play from the centre spot (kick-off after a goal, or half time).
func (s *simState) kickOff(team int) {
	s.ballX, s.ballY = 0.5, 0.5
	s.owner = -1
	s.setOwner(team, restartIdx(s.onPitchFor(team)), "kickoff")
	s.cooldown = 0
}

// beginKickoff lets both teams return to legal halves before play resumes.
func (s *simState) beginKickoff(team int) {
	s.owner = -1
	s.restart = Restart{}
	s.clearPattern()
	s.kickoffTeam = int8(team)
	s.kickoffTicks = 36
	s.ballX, s.ballY = 0.5, 0.5
	s.cooldown = 0
}

func (s *simState) kickoffPosition(team, idx, takerTeam int) (float64, float64) {
	x, y := s.anchors[team][idx][0], s.anchors[team][idx][1]
	if team == takerTeam && idx == restartIdx(s.onPitchFor(team)) {
		return 0.5, 0.5
	}
	if team == 0 {
		return math.Min(x, 0.39), y
	}
	return math.Max(x, 0.61), y
}

// target computes one player's tactical destination in absolute coordinates.
// Defenders close the carrier in layers while the rest of the unit slides behind them.
func (s *simState) target(team, idx int, ps *PlayerState) (float64, float64) {
	if idx > 0 && s.owner == int8(1-team) {
		rank := s.engagedRank(team, idx)
		if rank < int(s.tactics[team].Pressing) {
			ownGoal := goalXFor(1 - team)
			depth := 0.0
			if rank == 1 {
				depth = 0.28
			} else if rank > 1 {
				depth = 0.35
			}
			x := s.ballX + (ownGoal-s.ballX)*depth
			y := s.ballY + (ps.Y-s.ballY)*float64(rank)*0.5
			return clamp(x, 0.02, 0.98), clamp(y, 0.04, 0.96)
		}
		// Mark threatening runners while the nearest players challenge the ball.
		if mk, ok := s.boxPickup(team, idx); ok {
			return mk[0], mk[1]
		}
	}
	ax, ay := s.anchors[team][idx][0], s.anchors[team][idx][1]

	tac := s.tactics[team]
	blockShift := (float64(tac.Mentality)-3)*mentalityBlockCoeff + (float64(tac.DefensiveLine)-2)*lineBlockCoeff
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
		if s.owner == int8(1-team) {
			ax += clamp(s.ballX-0.5, -0.4, 0.4) * 0.12
			ay += clamp(s.ballY-0.5, -0.45, 0.45) * 0.18
		}
	}

	// Ball pull: attacking support bends toward the ball; defenders only the engaged
	// few (ranked by distance — a fixed-count press), the rest hold their zones.
	pull := 0.10
	if int8(team) != s.owner {
		pull = 0.03
		if s.engagedRank(team, idx) < 2+int(tac.Pressing) {
			pull = 0.10
		}
	}
	dx, dy := s.ballX-ax, s.ballY-ay
	d2 := dx*dx + dy*dy
	f := pull / (1 + 8*d2)
	tx, ty := clamp(ax+dx*f, 0.02, 0.98), clamp(ay+dy*f, 0.04, 0.96)
	if idx > 0 && s.owner == int8(1-team) && s.restart.Kind == RestartNone && s.kickoffTicks == 0 {
		// Out of possession the holding block obeys the vertical compactness
		// budget — lines move as one unit and cannot detach (T-021, DEF-3).
		tx = s.compactDepth(team, idx, tx)
	}
	return tx, ty
}

// engagedRank: 0 = closest to the ball among the defending side (fixed-count press).
func (s *simState) engagedRank(team, idx int) int {
	if s.cards[team*11+idx].Off {
		return 99
	}
	me := absf(s.players[team*11+idx].X-s.ballX) + absf(s.players[team*11+idx].Y-s.ballY)
	rank := 0
	for i := 0; i < 11; i++ {
		if i == idx || s.cards[team*11+i].Off {
			continue
		}
		d := absf(s.players[team*11+i].X-s.ballX) + absf(s.players[team*11+i].Y-s.ballY)
		if d < me {
			rank++
		}
	}
	return rank
}

// boxPickup assigns dangerous runners in the own box to their nearest available
// defender: zone defending owns SPACES, but nobody camps unmarked on the six-yard
// lane — a runner in the corridor is always someone's man (stage-D mark discipline,
// the guard against the unmarked-pin feeding frenzy). The lead presser stays on the
// ball. Returns the marker's target (goal-side of his runner).
func (s *simState) boxPickup(team, idx int) ([2]float64, bool) {
	if s.owner != 1-int8(team) || s.cards[team*11+idx].Off {
		return [2]float64{}, false
	}
	gx := goalXFor(1 - team) // the goal this team DEFENDS
	me := s.players[team*11+idx]
	mine, mineD := -1, math.MaxFloat64
	for j := 0; j < 11; j++ {
		if s.cards[(1-team)*11+j].Off {
			continue
		}
		oj := s.players[(1-team)*11+j]
		// The DANGEROUS corridor only: central and close. Wide men and edge ghosts
		// belong to the zone — gluing everybody kills the shape you're defending.
		if absf(gx-oj.X) > boxPickupReach || absf(oj.Y-0.5) > 0.30 {
			continue
		}
		dMe := absf(me.X-oj.X) + absf(me.Y-oj.Y)
		mineRunner := true // the pickup belongs to the closest defender (ties: index order)
		for i := 0; i < 11; i++ {
			if i == idx || s.cards[team*11+i].Off {
				continue
			}
			ps := s.players[team*11+i]
			if d := absf(ps.X-oj.X) + absf(ps.Y-oj.Y); d < dMe {
				mineRunner = false
				break
			}
		}
		if mineRunner && dMe < mineD {
			mine, mineD = j, dMe
		}
	}
	if mine < 0 {
		return [2]float64{}, false
	}
	o := s.players[(1-team)*11+mine]
	// Tuck goal-side of the runner (body between man and net).
	return [2]float64{o.X + (gx-o.X)*0.15, o.Y}, true
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

// teamInfo extracts the presentation roster (starters first) for viewers.
func teamInfo(t TeamSnapshot) TeamInfo {
	info := TeamInfo{Club: t.Club, Players: make([]PlayerInfo, 0, len(t.Players))}
	for i, p := range t.Players {
		info.Players = append(info.Players, PlayerInfo{
			ID:      p.ID,
			Name:    p.Name,
			Pos:     p.Pos,
			Starter: i < 11,
		})
	}
	return info
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
		Tick:         s.tick,
		Score:        s.score,
		Events:       slices.Clone(s.events),
		Stats:        s.stats,
		PossTicks:    s.possTicks,
		OnPitch:      s.onPitch,
		Players:      s.players,
		Ratings:      slices.Clone(s.ratings),
		BallX:        s.ballX,
		BallY:        s.ballY,
		Owner:        s.owner,
		OwnerIdx:     s.ownerIdx,
		Cooldown:     s.cooldown,
		KickoffTeam:  s.kickoffTeam,
		KickoffTicks: s.kickoffTicks,
		Restart:      s.restart,
		Cards:        s.cards,
		Pattern:      s.pattern,
		ChainID:      s.chainID,
		ChainTeam:    s.chainTeam,
		ChainStart:   s.chainStart,
		ChainReg:     s.chainReg,
		ChainGone:    s.chainGone,
		Touches:      slices.Clone(s.touches),
		Chains:       slices.Clone(s.chains),
		Stoppage:     s.stoppage,
		SecondHalf:   s.secondHalf,
		Tactics:      s.tactics,
		RND:          s.rnd.State(),
	}
}

func (s *simState) result() MatchResult {
	total := s.possTicks[0] + s.possTicks[1]
	res := MatchResult{
		Score:     s.score,
		Teams:     MatchTeams{Home: teamInfo(s.in.Home), Away: teamInfo(s.in.Away)},
		Events:    s.events,
		Touches:   s.touches,
		Chains:    s.finalChains(),
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
	gkX := 0.05
	if team == 1 {
		gkX = 0.95 // the keeper anchor must mirror too (he mans his OWN goal!)
	}
	table[0] = [2]float64{gkX, 0.5}
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
