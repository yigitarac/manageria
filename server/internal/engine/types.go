// Package engine implements the deterministic match simulation (see the Match Engine
// design note). Pure: no I/O, no wall clock, no goroutines, no map iteration whose
// order can affect results. The only randomness comes from the seeded rng package.
package engine

import (
	"encoding/json"

	"github.com/yigitarac/manageria/server/internal/rng"
)

// EngineVersion identifies simulation behaviour. Bump it deliberately whenever the
// behaviour changes and update the golden tests at the same time.
const EngineVersion = 1

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
}

// TeamSnapshot is a submitted lineup plus tactics. Exactly 11 players, [0] is the keeper.
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

// Intervention is a manager change submitted during the match. The server resolves
// EffectiveTick via the "next stoppage, ≤60 s" rule; the engine just honours it.
type Intervention struct {
	EffectiveTick int32
	Club          string
	Kind          string // "tactics" (payload: Tactics JSON) — skeleton supports only this
	Payload       json.RawMessage
}

// Event kinds emitted by the skeleton action tables.
const (
	EventKickOff       = "kick_off"
	EventHalfTime      = "half_time"
	EventGoal          = "goal"
	EventShotSaved     = "shot_saved"
	EventShotOffTarget = "shot_off_target"
)

// Event is one match incident.
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

// PlayerState is the simulated kinematic/dynamic state of one player.
type PlayerState struct {
	X, Y    float64
	Fatigue float64
	Acc     float64 // rating accumulator
}

// Snapshot is the complete serializable state at a tick boundary. Resuming from it
// reproduces the straight run bit for bit.
type Snapshot struct {
	Tick       int32
	Score      Score
	Events     []Event
	Stats      MatchStats
	PossTicks  [2]int
	Players    [22]PlayerState // home 0..10, away 11..21
	BallX      float64
	BallY      float64
	Owner      int8 // 0 home, 1 away, -1 loose
	OwnerIdx   int8
	Cooldown   int8
	Stoppage   [2]float64
	SecondHalf bool
	Tactics    [2]Tactics
	RND        rng.State
}
