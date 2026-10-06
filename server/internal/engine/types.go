// Package engine implements the deterministic match simulation (see the Match Engine
// design note). Pure: no I/O, no wall clock, no goroutines, no map iteration whose
// order can affect results. The only randomness comes from the seeded rng package.
//
// Portability rule: outcome math uses only +, -, *, / and bit-trivial helpers
// (abs/min/max/clamp) — no libm functions in decisions, so every platform computes
// bit-identical results.
package engine

import (
	"encoding/json"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// EngineVersion identifies simulation behaviour. Bump it deliberately whenever the
// behaviour changes and update the golden tests at the same time.
//
//	v1 — skeleton action tables (superseded)
//	v2 — full action tables: duels, crosses, set pieces, fouls/cards, injuries, subs
const EngineVersion = 2

// PlayerID identifies a player across a match (UUIDv7 string at the storage boundary).
type PlayerID string

// Pos is a coarse playing position.
type Pos uint8

// Playing positions. Goalkeepers are always slot 0 of a lineup.
const (
	PosGK Pos = iota
	PosCB
	PosFB
	PosDM
	PosCM
	PosAM
	PosW
	PosST
)

// Score is the final (or current) score. Home listed first.
type Score struct {
	Home int
	Away int
}

// Attributes holds the 25 player attributes (scale 1–20). Every attribute feeds named
// mechanics — there is no attribute without a consumer.
type Attributes struct {
	// Technical (9)
	Finishing  uint8
	LongShots  uint8
	Passing    uint8
	Crossing   uint8
	Dribbling  uint8
	FirstTouch uint8
	Heading    uint8
	Tackling   uint8
	SetPieces  uint8
	// Mental (7)
	Vision        uint8
	Decisions     uint8
	Composure     uint8
	Positioning   uint8
	Concentration uint8
	WorkRate      uint8
	Aggression    uint8
	// Physical (6)
	Pace         uint8
	Acceleration uint8
	Stamina      uint8
	Strength     uint8
	Jumping      uint8
	Agility      uint8
	// Goalkeeping (3)
	ShotStopping uint8
	Handling     uint8
	Distribution uint8
}

// all returns every attribute value in a fixed order (validation helper).
func (a Attributes) all() []uint8 {
	return []uint8{
		a.Finishing, a.LongShots, a.Passing, a.Crossing, a.Dribbling, a.FirstTouch,
		a.Heading, a.Tackling, a.SetPieces,
		a.Vision, a.Decisions, a.Composure, a.Positioning, a.Concentration, a.WorkRate, a.Aggression,
		a.Pace, a.Acceleration, a.Stamina, a.Strength, a.Jumping, a.Agility,
		a.ShotStopping, a.Handling, a.Distribution,
	}
}

// PlayerSnapshot is the immutable pre-match view of one player.
type PlayerSnapshot struct {
	ID        PlayerID
	Name      string
	Pos       Pos
	Attr      Attributes
	Condition float64 // 0..100
	Morale    float64 // 0..100
}

// SetPiecesConfig are the designated set-piece choices.
type SetPiecesConfig struct {
	CornerRoutine   int8     // 1 near post, 2 far post, 3 edge of box, 4 short
	FreeKickRoutine int8     // 1 shoot, 2 cross, 3 lay-off
	Taker           PlayerID // empty = engine picks the best candidate deterministically
}

// Tactics are the manager knobs; every field maps to documented engine effects.
type Tactics struct {
	Formation     [3]int // outfield rows defence→attack, e.g. {4,4,2}; sums to 10
	Mentality     int8   // 1 very defensive .. 5 very attacking
	Pressing      int8   // 1 low .. 3 high
	Tempo         int8   // 1 slow .. 5 fast
	Width         int8   // 1 narrow .. 3 wide
	PassingStyle  int8   // 1 short .. 3 direct
	DefensiveLine int8   // 1 deep .. 3 high
	Marking       int8   // 1 zonal, 2 man-oriented
	Tackling      int8   // 1 fair, 2 hard
	CounterAttack bool
	SetPieces     SetPiecesConfig
}

// TeamSnapshot is a submitted lineup plus tactics. 11–18 players: the first 11 are the
// starters ([0] is the goalkeeper), the rest are the bench.
type TeamSnapshot struct {
	Club    string
	Players []PlayerSnapshot
	Tactics Tactics
}

// MatchContext carries competition rules.
type MatchContext struct {
	Competition    string
	NeutralVenue   bool
	AllowExtraTime bool
	AllowPenalties bool
}

// PresenceFlags are the live-presence inputs (small, visible morale boost, see design note).
type PresenceFlags struct {
	Home bool
	Away bool
}

// MatchInput is everything Simulate needs. Same input + seed ⇒ same result, bit for bit.
type MatchInput struct {
	EngineVersion int
	Seed          uint64
	Home          TeamSnapshot
	Away          TeamSnapshot
	Context       MatchContext
	Presence      PresenceFlags
}

// Intervention kinds understood by the engine.
const (
	InterventionTactics      = "tactics"
	InterventionSubstitution = "substitution"
)

// Intervention is a manager change submitted during the match. The server resolves
// EffectiveTick via the "next stoppage, ≤60 s" rule; the engine just honours it.
type Intervention struct {
	EffectiveTick int32
	Club          string
	Kind          string // InterventionTactics (payload: Tactics) or InterventionSubstitution
	Payload       json.RawMessage
}

// SubstitutionPayload is the payload of InterventionSubstitution.
type SubstitutionPayload struct {
	PlayerOn  PlayerID `json:"playerOn"`
	PlayerOff PlayerID `json:"playerOff"`
}

// Event kinds emitted by the action tables.
const (
	EventKickOff       = "kick_off"
	EventHalfTime      = "half_time"
	EventGoal          = "goal"
	EventShotSaved     = "shot_saved"
	EventShotOffTarget = "shot_off_target"
	EventOffside       = "offside"
	EventFoul          = "foul"
	EventYellowCard    = "yellow_card"
	EventRedCard       = "red_card"
	EventInjury        = "injury"
	EventSubstitution  = "substitution"
	EventError         = "error" // concentration lapse gift
)

// Event is one match incident. Detail carries explainability extras
// (e.g. "morale:+4", "minute_out:10").
type Event struct {
	Tick   int32
	Minute int32
	Kind   string
	Club   string
	Player PlayerID
	Detail string
}

// TeamStats are per-team aggregate numbers (explainability).
type TeamStats struct {
	PossessionPct float64
	Shots         int
	OnTarget      int
	Goals         int
	XG            float64
	Corners       int
	Fouls         int
	Turnovers     int
	AvgFatigue    float64 // mean final fatigue of the XI (0..100)
	AvgShotDist   float64 // mean shot distance (normalized pitch units)
	DistSum       float64 // internal sum backing AvgShotDist
}

// MatchStats aggregates both teams.
type MatchStats struct {
	Home TeamStats
	Away TeamStats
}

// PlayerRating is one player's post-match rating (sorted by PlayerID in results).
type PlayerRating struct {
	PlayerID PlayerID
	Rating   float64
}

// KFPlayer is one player's visual state in a keyframe.
type KFPlayer struct {
	X, Y  float64
	State uint8
}

// Keyframe visual states (the client animates from these).
const (
	KFIdle uint8 = iota
	KFRun
	KFSprint
	KFDuel
	KFKick
	KFShot
	KFCelebrate
)

// Keyframe is one sampled moment of the 2D timeline (3 s lattice). Generated on
// demand, never persisted (see the live-matches decision).
type Keyframe struct {
	TMs       uint32
	BallX     float64
	BallY     float64
	BallOwner int8 // 0 home, 1 away, -1 loose
	Players   [22]KFPlayer
}

// MatchResult is a completed simulation. Keyframes cover the simulated window only
// when produced by Resume (the straight run covers the whole match).
type MatchResult struct {
	Revision      int // caller-assigned revision number (0 = pre-simulation)
	Score         Score
	Events        []Event
	Stats         MatchStats
	PlayerRatings []PlayerRating
	Keyframes     []Keyframe
}

// PlayerState is the simulated dynamic state of one on-pitch slot.
type PlayerState struct {
	X, Y    float64
	Fatigue float64 // 0..100
	Morale  float64 // 0..100, shifts with goals (surfaced in goal event details)
	Acc     float64 // rating accumulator
	Hurt    bool    // playing through an injury (perf penalty until subbed)
}

// CardState tracks disciplinary state per on-pitch slot.
type CardState struct {
	Yellow uint8
	Off    bool // sent off; the slot stops participating
}

// RestartKind enumerates dead-ball situations.
type RestartKind int8

// Dead-ball situations.
const (
	RestartNone RestartKind = iota
	RestartGoalKick
	RestartThrowIn
	RestartCorner
	RestartFreeKick
	RestartPenalty
)

// Restart is a pending dead-ball delivery (resolved after Ticks ticks of "walk over").
type Restart struct {
	Kind  RestartKind
	Team  int8
	X, Y  float64
	Ticks int8
}

// Snapshot is the complete serializable state at a tick boundary. Resuming from it
// reproduces the straight run bit for bit. It is self-contained apart from the seed:
// on-pitch player copies travel with it (substitutions change occupants).
type Snapshot struct {
	Tick       int32
	Score      Score
	Events     []Event
	Stats      MatchStats
	PossTicks  [2]int
	OnPitch    [22]PlayerSnapshot // current occupants: home 0..10, away 11..21
	Players    [22]PlayerState
	Ratings    []PlayerRating // accumulated for everyone who played
	BallX      float64
	BallY      float64
	Owner      int8 // 0 home, 1 away, -1 loose
	OwnerIdx   int8
	Cooldown   int8
	Restart    Restart
	Cards      [22]CardState
	Stoppage   [2]float64
	SecondHalf bool
	Tactics    [2]Tactics
	RND        rng.State
}