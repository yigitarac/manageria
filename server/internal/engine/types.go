// Package engine implements the deterministic match simulation (see the Match Engine
// design note). Pure: no I/O, no wall clock, no goroutines, no map iteration whose
// order can affect results. The only randomness comes from the seeded rng package.
//
// Portability rule: outcome math uses only +, -, *, / and bit-trivial helpers
// (abs/min/max/clamp) — no libm functions in decisions, so every platform computes
// bit-identical results.
//
// Serialization contract: JSON is camelCase (coding standard) and IS the client-facing
// dump format (`simcli -out`), so tags here are public API — keep them stable.
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
//	v3 — pattern execution (orchestrated corner routines + attribution, ADR-0009)
//	v4 — balance era mechanics: pressure saturation, traffic drag, behind-space through
//	     balls, man-marking bite, game-state management, combo fatigue (T-007)
//	v5 — football IQ pass: situation-aware receiver choice, one-on-one fixation, role
//	     discipline (defenders hold), carry traffic brake (T-012)
//	v6 — teleport-free kinematics & keeper homes: mirrored keeper anchors, receivers run
//	     to the ball instead of materialising on it (T-014 visual-integrity pass)
//	v7 — emergent behaviour layer stages B/C (T-014): options-based decisions (pass-lane
//	     geometry vs cover shadows, carry space, shoot windows; the dice-picker retires),
//	     one coherent possession-retention budget, unit-aware regime classifier and honest
//	     chain outcomes (T-014 possession-consolidation + chance-creation pass)
//	v8 — capped kinematics (one-second step ≤ 0.049, no 3-second runner leaps) and mirrored
//	     keeper homes (T-018 visual-integrity follow-up)
//	v9 — coordinated lines and honest dueling (T-017): kick-off regrouping, layered depth-band
//	     press + ball-side slide, honest carry pricing, second-ball scrambles, cross aerial
//	     pricing, KPI norms as tests
//	v10 — decision-engine repetition guard: the return-to-recent-supplier penalty now spans a
//	     window of prior passes (not just the last one) and eases under heat (DEC-1)
//	v11 — middle-third support runs: in the progression regime the front cast offers forward
//	     outlets (strikers check into the channel, wingers drop level on the flank, the AM
//	     drifts between the lines) instead of holding anchors (T-020, OFF-1/OFF-2)
//	v12 — striker defensive recovery: a striker who drifts into his own full-back
//	     corridor returns toward a central outlet after the restart (T-027)
//	v13 — scale-free option ranking, saturated tactic stacks, and smoother option
//	     fades keep tactical response bounded (T-016)
//	v14 — local close-down pressure gates emergency outlets and shielding while the
//	     wider defensive block still prices traffic (T-021)
//	v15 — defensive block compactness budget: out of possession the holding block's
//	     lines ride at most compactGap apart, measured forward from the last line
//	     (T-021, DEF-3); touch ledger now records the local close-down read (telemetry)
const EngineVersion = 15

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
	Home int `json:"home"`
	Away int `json:"away"`
}

// Attributes holds the 25 player attributes (scale 1–20). Every attribute feeds named
// mechanics — there is no attribute without a consumer.
type Attributes struct {
	// Technical (9)
	Finishing  uint8 `json:"finishing"`
	LongShots  uint8 `json:"longShots"`
	Passing    uint8 `json:"passing"`
	Crossing   uint8 `json:"crossing"`
	Dribbling  uint8 `json:"dribbling"`
	FirstTouch uint8 `json:"firstTouch"`
	Heading    uint8 `json:"heading"`
	Tackling   uint8 `json:"tackling"`
	SetPieces  uint8 `json:"setPieces"`
	// Mental (7)
	Vision        uint8 `json:"vision"`
	Decisions     uint8 `json:"decisions"`
	Composure     uint8 `json:"composure"`
	Positioning   uint8 `json:"positioning"`
	Concentration uint8 `json:"concentration"`
	WorkRate      uint8 `json:"workRate"`
	Aggression    uint8 `json:"aggression"`
	// Physical (6)
	Pace         uint8 `json:"pace"`
	Acceleration uint8 `json:"acceleration"`
	Stamina      uint8 `json:"stamina"`
	Strength     uint8 `json:"strength"`
	Jumping      uint8 `json:"jumping"`
	Agility      uint8 `json:"agility"`
	// Goalkeeping (3)
	ShotStopping uint8 `json:"shotStopping"`
	Handling     uint8 `json:"handling"`
	Distribution uint8 `json:"distribution"`
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
	ID        PlayerID   `json:"id"`
	Name      string     `json:"name"`
	Pos       Pos        `json:"pos"`
	Attr      Attributes `json:"attr"`
	Condition float64    `json:"condition"` // 0..100
	Morale    float64    `json:"morale"`    // 0..100
}

// SetPiecesConfig are the designated set-piece choices.
type SetPiecesConfig struct {
	CornerRoutine   int8     `json:"cornerRoutine"`   // 1 near post, 2 far post, 3 edge of box, 4 short
	FreeKickRoutine int8     `json:"freeKickRoutine"` // 1 shoot, 2 cross, 3 lay-off
	Taker           PlayerID `json:"taker"`           // empty = engine picks the best candidate
}

// Tactics are the manager knobs; every field maps to documented engine effects.
type Tactics struct {
	Formation     [3]int          `json:"formation"` // outfield rows defence→attack; sums to 10
	Mentality     int8            `json:"mentality"`
	Pressing      int8            `json:"pressing"`
	Tempo         int8            `json:"tempo"`
	Width         int8            `json:"width"`
	PassingStyle  int8            `json:"passingStyle"`
	DefensiveLine int8            `json:"defensiveLine"`
	Marking       int8            `json:"marking"`
	Tackling      int8            `json:"tackling"`
	CounterAttack bool            `json:"counterAttack"`
	SetPieces     SetPiecesConfig `json:"setPieces"`
	Patterns      []PatternSpec   `json:"patterns,omitempty"` // prepared patterns (cap 10)
}

// TeamSnapshot is a submitted lineup plus tactics. 11–18 players: the first 11 are the
// starters ([0] is the goalkeeper), the rest are the bench.
type TeamSnapshot struct {
	Club    string           `json:"club"`
	Players []PlayerSnapshot `json:"players"`
	Tactics Tactics          `json:"tactics"`
}

// MatchContext carries competition rules.
type MatchContext struct {
	Competition    string `json:"competition"`
	NeutralVenue   bool   `json:"neutralVenue"`
	AllowExtraTime bool   `json:"allowExtraTime"`
	AllowPenalties bool   `json:"allowPenalties"`
}

// PresenceFlags are the live-presence inputs (small, visible morale boost).
type PresenceFlags struct {
	Home bool `json:"home"`
	Away bool `json:"away"`
}

// MatchInput is everything Simulate needs. Same input + seed ⇒ same result, bit for bit.
type MatchInput struct {
	EngineVersion int           `json:"engineVersion"`
	Seed          uint64        `json:"seed"`
	Home          TeamSnapshot  `json:"home"`
	Away          TeamSnapshot  `json:"away"`
	Context       MatchContext  `json:"context"`
	Presence      PresenceFlags `json:"presence"`
}

// Intervention kinds understood by the engine.
const (
	InterventionTactics      = "tactics"
	InterventionSubstitution = "substitution"
)

// Intervention is a manager change submitted during the match. The server resolves
// EffectiveTick via the "next stoppage, ≤60 s" rule; the engine just honours it.
type Intervention struct {
	EffectiveTick int32           `json:"effectiveTick"`
	Club          string          `json:"club"`
	Kind          string          `json:"kind"` // InterventionTactics or InterventionSubstitution
	Payload       json.RawMessage `json:"payload"`
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
// (e.g. "morale:+4", "minute_out:10", "pattern:id").
type Event struct {
	Tick   int32    `json:"tick"`
	Minute int32    `json:"minute"`
	Kind   string   `json:"kind"`
	Club   string   `json:"club"`
	Player PlayerID `json:"player"`
	Detail string   `json:"detail"`
}

// TeamStats are per-team aggregate numbers (explainability).
type TeamStats struct {
	PossessionPct float64 `json:"possessionPct"`
	Shots         int     `json:"shots"`
	OnTarget      int     `json:"onTarget"`
	Goals         int     `json:"goals"`
	XG            float64 `json:"xg"`
	Corners       int     `json:"corners"`
	Fouls         int     `json:"fouls"`
	Turnovers     int     `json:"turnovers"`
	AvgFatigue    float64 `json:"avgFatigue"`
	AvgShotDist   float64 `json:"avgShotDist"`
	DistSum       float64 `json:"distSum"` // internal sum backing AvgShotDist
	PatternShots  int     `json:"patternShots"`
	PatternXG     float64 `json:"patternXg"`
	// StrikesFromDefenders counts open-play strikes by GK/CB/DM — the automated
	// watch-through smell (T-012). Headers are excluded: big defenders may head.
	StrikesFromDefenders int `json:"strikesFromDefenders"`
}

// MatchStats aggregates both teams.
type MatchStats struct {
	Home TeamStats `json:"home"`
	Away TeamStats `json:"away"`
}

// PlayerRating is one player's post-match rating (sorted by PlayerID in results).
type PlayerRating struct {
	PlayerID PlayerID `json:"playerId"`
	Rating   float64  `json:"rating"`
}

// KFPlayer is one player's visual state in a keyframe.
type KFPlayer struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	State uint8   `json:"state"`
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
	TMs       uint32       `json:"tMs"`
	BallX     float64      `json:"ballX"`
	BallY     float64      `json:"ballY"`
	BallOwner int8         `json:"ballOwner"` // 0 home, 1 away, -1 loose
	Players   [22]KFPlayer `json:"players"`
}

// PlayerInfo is one squad member surfaced for presentation (commentary, ratings).
type PlayerInfo struct {
	ID      PlayerID `json:"id"`
	Name    string   `json:"name"`
	Pos     Pos      `json:"pos"`
	Starter bool     `json:"starter"`
}

// TeamInfo is the presentation roster of one side in a dump.
type TeamInfo struct {
	Club    string       `json:"club"`
	Players []PlayerInfo `json:"players"`
}

// MatchTeams carries both presentation rosters (additive contract for viewers).
type MatchTeams struct {
	Home TeamInfo `json:"home"`
	Away TeamInfo `json:"away"`
}

// Regimes (ADR-0011): every possession is born into a playbook.
type Regime int8

// Possession regimes.
const (
	RegimeBuildUp Regime = iota
	RegimeProgression
	RegimeFinalThird
	RegimeTransition
	RegimeSetPiece
	RegimeRegroup
)

// String returns the regime label for narratives and KPIs.
func (r Regime) String() string {
	switch r {
	case RegimeBuildUp:
		return "buildUp"
	case RegimeProgression:
		return "progression"
	case RegimeFinalThird:
		return "finalThird"
	case RegimeTransition:
		return "transition"
	case RegimeSetPiece:
		return "setPiece"
	default:
		return "regroup"
	}
}

// Touch kinds in the sensory ledger.
const (
	TouchPass   = "pass"
	TouchCarry  = "carry"
	TouchDuel   = "duel"
	TouchRegain = "regain"
	TouchShot   = "shot"
)

// Touch is one recorded on-ball action — the sensory layer from which every emergent
// statistic derives (ADR-0011: stats are bookkeeping, never manufactured).
type Touch struct {
	Tick    int32   `json:"tick"`
	Chain   int32   `json:"chain"`
	Team    int8    `json:"team"`
	Kind    string  `json:"kind"`
	Actor   int8    `json:"actor"`  // on-pitch slot
	Target  int8    `json:"target"` // receiver/duel opponent slot (−1 = none)
	Success bool    `json:"success"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	// Press is the local close-down read (challengePressure) the actor faced at
	// this touch — the lens through which pressured retention and unpressed loops
	// become measurable (T-021 instrumentation). Telemetry only: never consumed
	// by any action utility.
	Press float64 `json:"press"`
}

// ChainInfo is one possession story: when it was born, who owned it, how it played.
type ChainInfo struct {
	ID      int32  `json:"id"`
	Team    int8   `json:"team"`
	Regime  string `json:"regime"`
	Start   int32  `json:"start"`
	End     int32  `json:"end"`
	Touches int    `json:"touches"`
	Outcome string `json:"outcome"` // "turnover" | "shot" | "goal" | "dead_ball" | "half"
}

// MatchResult is a completed simulation. Keyframes cover the simulated window only
// when produced by Resume (the straight run covers the whole match).
type MatchResult struct {
	Revision      int            `json:"revision"`
	Teams         MatchTeams     `json:"teams"`
	Score         Score          `json:"score"`
	Events        []Event        `json:"events"`
	Touches       []Touch        `json:"touches"`
	Chains        []ChainInfo    `json:"chains"`
	Stats         MatchStats     `json:"stats"`
	PlayerRatings []PlayerRating `json:"playerRatings"`
	Keyframes     []Keyframe     `json:"keyframes"`
}

// Waypoint is one timed point of a pattern route (normalized pitch + seconds from trigger).
type Waypoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	T float64 `json:"t"`
}

// Pattern contact roles.
const (
	ContactAttackBall int8 = iota
	ContactDecoy
	ContactEdgeRunner
	ContactCover
)

// PatternActor is one participant of a pattern (T-009 subset: slot + 2-point route).
type PatternActor struct {
	Slot    int        `json:"slot"`
	Route   []Waypoint `json:"route"` // exactly 2 in the spike subset
	Contact int8       `json:"contact"`
}

// PatternDelivery describes the aimed ball played into the route meeting point.
type PatternDelivery struct {
	Kind    int8       `json:"kind"` // 1 cross, 2 driven pass, 3 shot, 4 lay-off
	Aim     [2]float64 `json:"aim"`
	Spread  float64    `json:"spread"`
	Power   float64    `json:"power"`
	Targets []int      `json:"targets"`
}

// PatternSpec is the T-009 spike subset of the Tactics-Schema pattern grammar:
// corner routines with ≤4 actors and 2-point Hermite routes.
type PatternSpec struct {
	ID       string          `json:"id"`
	Actors   []PatternActor  `json:"actors"`
	Delivery PatternDelivery `json:"delivery"`
}

// PatternCursor tracks an armed or executing pattern. It lives in the snapshot so
// that Resume stays bit-identical across pattern windows.
type PatternCursor struct {
	Active    bool  `json:"active"`
	Team      int8  `json:"team"`
	Index     int8  `json:"index"`
	StartTick int32 `json:"startTick"`
	Delivered bool  `json:"delivered"`
}

// reset returns a cleared cursor.
func (c PatternCursor) reset() PatternCursor {
	return PatternCursor{}
}

// PlayerState is the simulated dynamic state of one on-pitch slot.
type PlayerState struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Fatigue float64 `json:"fatigue"` // 0..100
	Morale  float64 `json:"morale"`  // 0..100, shifts with goals
	Acc     float64 `json:"acc"`     // rating accumulator
	Hurt    bool    `json:"hurt"`    // playing through an injury
}

// CardState tracks disciplinary state per on-pitch slot.
type CardState struct {
	Yellow uint8 `json:"yellow"`
	Off    bool  `json:"off"` // sent off; the slot stops participating
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
	Kind  RestartKind `json:"kind"`
	Team  int8        `json:"team"`
	X     float64     `json:"x"`
	Y     float64     `json:"y"`
	Ticks int8        `json:"ticks"`
}

// Snapshot is the complete serializable state at a tick boundary. Resuming from it
// reproduces the straight run bit for bit. It is self-contained apart from the seed:
// on-pitch player copies travel with it (substitutions change occupants).
type Snapshot struct {
	Tick         int32              `json:"tick"`
	Score        Score              `json:"score"`
	Events       []Event            `json:"events"`
	Stats        MatchStats         `json:"stats"`
	PossTicks    [2]int             `json:"possTicks"`
	OnPitch      [22]PlayerSnapshot `json:"onPitch"`
	Players      [22]PlayerState    `json:"players"`
	Ratings      []PlayerRating     `json:"ratings"`
	BallX        float64            `json:"ballX"`
	BallY        float64            `json:"ballY"`
	Owner        int8               `json:"owner"`
	OwnerIdx     int8               `json:"ownerIdx"`
	Cooldown     int8               `json:"cooldown"`
	KickoffTeam  int8               `json:"kickoffTeam"`
	KickoffTicks int8               `json:"kickoffTicks"`
	Restart      Restart            `json:"restart"`
	Cards        [22]CardState      `json:"cards"`
	Pattern      PatternCursor      `json:"pattern"`
	ChainID      int32              `json:"chainId"`
	ChainTeam    int8               `json:"chainTeam"`
	ChainStart   int32              `json:"chainStart"`
	ChainReg     Regime             `json:"chainReg"`
	ChainGone    int32              `json:"chainGone"`
	Touches      []Touch            `json:"touches"`
	Chains       []ChainInfo        `json:"chains"`
	Stoppage     [2]float64         `json:"stoppage"`
	SecondHalf   bool               `json:"secondHalf"`
	Tactics      [2]Tactics         `json:"tactics"`
	RND          rng.State          `json:"rnd"`
}
