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
// fictional clubs with evenly matched squads.
func SampleInput(seed uint64) engine.MatchInput {
	return engine.MatchInput{
		EngineVersion: engine.EngineVersion,
		Seed:          seed,
		Home:          sampleTeam("Redvale FC", 11),
		Away:          sampleTeam("Stonebrook Athletic", 11),
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

func sampleTeam(club string, base uint8) engine.TeamSnapshot {
	specs := []struct {
		name string
		pos  engine.Pos
		mod  uint8
	}{
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
	}
	players := make([]engine.PlayerSnapshot, 0, 11)
	for i, sp := range specs {
		players = append(players, engine.PlayerSnapshot{
			ID:        engine.PlayerID(fmt.Sprintf("%s#%d", club, i)),
			Name:      sp.name,
			Pos:       sp.pos,
			Attr:      sampleAttrs(sp.pos, base+sp.mod),
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
	a := engine.Attributes{
		Finishing: clampAttr(base), LongShots: clampAttr(base - 2), Passing: clampAttr(base + 1),
		Crossing: clampAttr(base), Dribbling: clampAttr(base - 1), FirstTouch: clampAttr(base),
		Heading: clampAttr(base - 1), Tackling: clampAttr(base), SetPieces: clampAttr(base - 3),

		Vision: clampAttr(base - 1), Decisions: clampAttr(base), Composure: clampAttr(base),
		Positioning: clampAttr(base), Concentration: clampAttr(base - 1), WorkRate: clampAttr(base),
		Aggression: clampAttr(base - 2),

		Pace: clampAttr(base + 1), Acceleration: clampAttr(base), Stamina: clampAttr(base + 1),
		Strength: clampAttr(base), Jumping: clampAttr(base), Agility: clampAttr(base - 1),

		ShotStopping: clampAttr(base - 5), Handling: clampAttr(base - 5), Distribution: clampAttr(base - 5),
	}
	if pos == engine.PosGK {
		a.ShotStopping = clampAttr(base + 4)
		a.Handling = clampAttr(base + 3)
		a.Distribution = clampAttr(base + 2)
		a.Finishing = 1
		a.LongShots = 1
		a.Dribbling = 1
	}
	return a
}
