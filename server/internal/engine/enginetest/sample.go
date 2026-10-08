// Package enginetest provides deterministic sample inputs for engine tests and
// developer tools (simcli). Not part of the production engine surface.
package enginetest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/yigitarac/manageria/server/internal/engine"
)

// SampleInput returns a fully valid, deterministic match input around two
// fictional clubs with evenly matched squads (11 starters + 5 bench).
func SampleInput(seed uint64) engine.MatchInput {
	return engine.MatchInput{
		EngineVersion: engine.EngineVersion,
		Seed:          seed,
		Home:          sampleTeam("Redvale FC"),
		Away:          sampleTeam("Stonebrook Athletic"),
		Context: engine.MatchContext{
			Competition:    "league",
			NeutralVenue:   false,
			AllowExtraTime: false,
			AllowPenalties: false,
		},
		Presence: engine.PresenceFlags{Home: true, Away: false},
	}
}

// SampleTactics returns the default 4-4-2 setup used by the sample teams.
func SampleTactics() engine.Tactics {
	return engine.Tactics{
		Formation:     [3]int{4, 4, 2},
		Mentality:     3,
		Pressing:      2,
		Tempo:         3,
		Width:         2,
		PassingStyle:  2,
		DefensiveLine: 2,
		Marking:       1,
		Tackling:      1,
		SetPieces: engine.SetPiecesConfig{
			CornerRoutine:   2, // far post
			FreeKickRoutine: 2, // cross
		},
	}
}

// SampleCornerPattern returns a home-oriented corner routine (home attacks +x):
// two striker darts into the six-yard box, a centre-back power run and a late edge
// arrival. Attach to `Tactics.Patterns` to rehearse or arm it.
func SampleCornerPattern() engine.PatternSpec {
	return engine.PatternSpec{
		ID: "six_yard_darts",
		Actors: []engine.PatternActor{
			{Slot: 9, Contact: engine.ContactAttackBall, Route: []engine.Waypoint{
				{X: 0.70, Y: 0.38, T: 0}, {X: 0.93, Y: 0.46, T: 3.5},
			}},
			{Slot: 10, Contact: engine.ContactAttackBall, Route: []engine.Waypoint{
				{X: 0.74, Y: 0.62, T: 0}, {X: 0.92, Y: 0.55, T: 4.0},
			}},
			{Slot: 1, Contact: engine.ContactAttackBall, Route: []engine.Waypoint{
				{X: 0.66, Y: 0.52, T: 0}, {X: 0.90, Y: 0.48, T: 4.5},
			}},
			{Slot: 5, Contact: engine.ContactEdgeRunner, Route: []engine.Waypoint{
				{X: 0.62, Y: 0.40, T: 0}, {X: 0.84, Y: 0.44, T: 5.5},
			}},
		},
		Delivery: engine.PatternDelivery{
			Kind:    1, // cross
			Aim:     [2]float64{0.93, 0.50},
			Spread:  0.07,
			Power:   0.7,
			Targets: []int{0, 1, 2},
		},
	}
}

// Digest returns a stable content hash of a match result (golden tests, simcli).
func Digest(res engine.MatchResult) (string, error) {
	raw, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshaling result: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sampleTeam(club string) engine.TeamSnapshot {
	specs := []struct {
		name string
		pos  engine.Pos
		mod  uint8
	}{
		// Starting XI (4-4-2)
		{"Halden Moor", engine.PosGK, 0},
		{"Bram Kovac", engine.PosCB, 1},
		{"Idris Vale", engine.PosCB, 0},
		{"Tomas Rilk", engine.PosFB, 2},
		{"Ned Barrow", engine.PosFB, 1},
		{"Cass Whitlock", engine.PosDM, 0},
		{"Emre Sadik", engine.PosCM, 3},
		{"Ollie Fen", engine.PosW, 2},
		{"Dario Salk", engine.PosW, 1},
		{"Miro Thane", engine.PosST, 3},
		{"Kenji Alba", engine.PosST, 2},
		// Bench
		{"Piet Halloran", engine.PosGK, 1},
		{"Sven Adako", engine.PosCB, 2},
		{"Luc Brandt", engine.PosCM, 1},
		{"Rhys Calder", engine.PosW, 0},
		{"Anzo Peres", engine.PosST, 1},
	}
	if club == "Stonebrook Athletic" {
		awayNames := [...]string{
			"Luca Venn", "Ari Solen", "Kellan Frost", "Rian Mercer", "Tavi Lorne",
			"Jules Maren", "Noel Voss", "Sami Elian", "Felix Arden", "Ivo Senna",
			"Rafi Calder", "Oren Hale", "Nico Sayer", "Leon Caspar", "Theo Varek", "Eli Marin",
		}
		for i := range specs {
			specs[i].name = awayNames[i]
		}
	}
	players := make([]engine.PlayerSnapshot, 0, len(specs))
	for i, sp := range specs {
		players = append(players, engine.PlayerSnapshot{
			ID:        engine.PlayerID(fmt.Sprintf("%s#%d", club, i)),
			Name:      sp.name,
			Pos:       sp.pos,
			Attr:      sampleAttrs(sp.pos, uint8(11)+sp.mod),
			Condition: 100,
			Morale:    55,
		})
	}
	return engine.TeamSnapshot{Club: club, Players: players, Tactics: SampleTactics()}
}

func sampleAttrs(pos engine.Pos, base uint8) engine.Attributes {
	clampAttr := func(v uint8) uint8 {
		if v < 1 {
			return 1
		}
		if v > 20 {
			return 20
		}
		return v
	}
	// Position-shaped football realism (T-012 lesson): strikers finish, creators pass,
	// stopers defend and head. Generic base + per-position offsets.
	off := func(delta int) uint8 {
		return clampAttr(uint8(int(base) + delta))
	}
	var finishing, heading, tackling, passing, vision, pace, setP uint8
	switch pos {
	case engine.PosST:
		finishing, heading, tackling, passing, vision, pace, setP = off(4), off(3), off(-4), off(-1), off(0), off(1), off(-2)
	case engine.PosW:
		finishing, heading, tackling, passing, vision, pace, setP = off(2), off(-3), off(-2), off(1), off(1), off(3), off(1)
	case engine.PosAM:
		finishing, heading, tackling, passing, vision, pace, setP = off(2), off(-1), off(-2), off(3), off(3), off(0), off(2)
	case engine.PosCM:
		finishing, heading, tackling, passing, vision, pace, setP = off(-1), off(0), off(1), off(3), off(2), off(0), off(1)
	case engine.PosDM:
		finishing, heading, tackling, passing, vision, pace, setP = off(-3), off(1), off(3), off(1), off(0), off(-1), off(-1)
	case engine.PosFB:
		finishing, heading, tackling, passing, vision, pace, setP = off(-4), off(-1), off(2), off(1), off(0), off(2), off(-1)
	case engine.PosCB:
		finishing, heading, tackling, passing, vision, pace, setP = off(-5), off(4), off(4), off(-1), off(-2), off(-2), off(-2)
	default: // GK
		finishing, heading, tackling, passing, vision, pace, setP = 1, off(-3), off(-4), off(0), off(-3), off(-3), off(-4)
	}

	a := engine.Attributes{
		Finishing: finishing, LongShots: clampAttr(finishing - 2), Passing: passing,
		Crossing: clampAttr(pace + off(-5)), Dribbling: clampAttr(pace - 1), FirstTouch: passing,
		Heading: heading, Tackling: tackling, SetPieces: setP,

		Vision: vision, Decisions: off(0), Composure: off(0),
		Positioning: tackling, Concentration: off(-1), WorkRate: off(0),
		Aggression: clampAttr(tackling - 2),

		Pace: pace, Acceleration: pace, Stamina: off(1),
		Strength: heading, Jumping: heading, Agility: clampAttr(pace - 1),

		ShotStopping: 1, Handling: 1, Distribution: 1,
	}
	if pos == engine.PosGK {
		a.ShotStopping = off(5)
		a.Handling = off(4)
		a.Distribution = off(3)
		a.Agility = off(1)
	}
	return a
}
