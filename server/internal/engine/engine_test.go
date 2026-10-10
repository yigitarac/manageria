package engine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// TestGoldenDigest pins the exact output of a fixture match. Bump the expected value
// deliberately whenever EngineVersion behaviour changes (see the testing rules).
func TestGoldenDigest(t *testing.T) {
	t.Parallel()
	// v16: both seeds re-pinned for the congestion sensor repair (sieges read as
	// sieges again — packed boxes dilute chance value) with the paired refreeze
	// shotBase 0.60→0.63, homeAdvantage 1.10→1.12. v15 digests in history.
	for _, tc := range []struct {
		seed uint64
		want string
	}{
		{42, "e2407e8ee538329ee263352593d4cf050ebd3b3f6d24f6eb5ddbfa18d5c17ae9"},
		{987, "9c842fe17872403d338258b83327cc11c3d3384857ddf104a00eeb8596e0dc87"},
		{987, "9c842fe17872403d338258b83327cc11c3d3384857ddf104a00eeb8596e0dc87"},
	} {
		res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
			return engine.Simulate(enginetest.SampleInput(tc.seed))
		})
		got, err := enginetest.Digest(res)
		if err != nil {
			t.Fatalf("Digest: %v", err)
		}
		if got != tc.want {
			t.Fatalf("seed %d golden digest = %s, want %s (update deliberately on EngineVersion bumps)", tc.seed, got, tc.want)
		}
	}
}

func TestNoThreeSecondPlayerLeap(t *testing.T) {
	for _, seed := range []uint64{42, 123, 987} {
		res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
			return engine.Simulate(enginetest.SampleInput(seed))
		})
		for i := 1; i < len(res.Keyframes); i++ {
			for slot := range res.Keyframes[i].Players {
				a := res.Keyframes[i-1].Players[slot]
				b := res.Keyframes[i].Players[slot]
				dx, dy := b.X-a.X, b.Y-a.Y
				if dx*dx+dy*dy > 0.15*0.15 {
					t.Fatalf("seed %d, tick %d, slot %d: squared leap %.3f exceeds limit", seed, res.Keyframes[i].TMs/1000, slot, dx*dx+dy*dy)
				}
			}
		}
	}
}

func TestKickoffsHaveLegalPositions(t *testing.T) {
	for _, seed := range []uint64{42, 123, 987} {
		res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
			return engine.Simulate(enginetest.SampleInput(seed))
		})
		kickoffs := 0
		for _, event := range res.Events {
			if event.Kind != engine.EventKickOff {
				continue
			}
			kickoffs++
			var frame *engine.Keyframe
			for i := range res.Keyframes {
				if res.Keyframes[i].TMs > uint32(event.Tick)*1000 {
					break
				}
				frame = &res.Keyframes[i]
			}
			if frame == nil {
				t.Fatalf("seed %d: no frame for kick-off tick %d", seed, event.Tick)
			}
			for slot, player := range frame.Players {
				roster := res.Teams.Home.Players
				if slot >= 11 {
					roster = res.Teams.Away.Players
				}
				dismissed := false
				for _, prior := range res.Events {
					if prior.Tick <= event.Tick && prior.Kind == engine.EventRedCard && prior.Player == roster[slot%11].ID {
						dismissed = true
						break
					}
				}
				if dismissed {
					continue
				}
				if slot < 11 && player.X > 0.501 || slot >= 11 && player.X < 0.499 {
					t.Fatalf("seed %d, tick %d: player %d crossed halfway at kick-off (x %.3f)", seed, event.Tick, slot, player.X)
				}
				if dx, dy := player.X-0.5, player.Y-0.5; dx*dx+dy*dy < 0.09*0.09 &&
					(slot < 11) != (event.Club == res.Teams.Home.Club) {
					t.Fatalf("seed %d, tick %d: opponent %d inside centre circle", seed, event.Tick, slot)
				}
			}
		}
		if kickoffs < 2 {
			t.Fatalf("seed %d: expected initial and second-half kick-offs", seed)
		}
	}
}

func TestOpenPlayStaysActive(t *testing.T) {
	for _, seed := range []uint64{42, 123, 987} {
		res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
			return engine.Simulate(enginetest.SampleInput(seed))
		})
		moving, observed := 0, 0
		for i := 1; i < len(res.Keyframes); i++ {
			before, after := res.Keyframes[i-1], res.Keyframes[i]
			if before.BallOwner < 0 || before.BallOwner != after.BallOwner {
				continue
			}
			defender := 1 - int(before.BallOwner)
			for slot := defender*11 + 1; slot < defender*11+11; slot++ {
				p, q := before.Players[slot], after.Players[slot]
				if math.Hypot(p.X-q.X, p.Y-q.Y) > 0.005 {
					moving++
				}
				observed++
			}
		}
		if observed == 0 || float64(moving)/float64(observed) < 0.55 {
			t.Fatalf("seed %d: defending movement %.1f%% of open-play frames", seed, 100*float64(moving)/float64(observed))
		}

		carries, repeat, longest := 0, 0, 0
		lastPair := [3]int{-1, -1, -1}
		for _, touch := range res.Touches {
			if touch.Kind == engine.TouchCarry && touch.Success {
				carries++
			}
			if touch.Kind != engine.TouchPass || !touch.Success {
				repeat = 0
				lastPair = [3]int{-1, -1, -1}
				continue
			}
			a, b := int(touch.Actor), int(touch.Target)
			if a > b {
				a, b = b, a
			}
			pair := [3]int{int(touch.Team), a, b}
			if pair == lastPair {
				repeat++
			} else {
				repeat = 1
				lastPair = pair
			}
			if repeat > longest {
				longest = repeat
			}
		}
		if carries < 120 || longest > 6 {
			t.Fatalf("seed %d: %d carries, longest reciprocal pass run %d", seed, carries, longest)
		}
	}
}

func TestSimulateDeterministic(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(42)
	first := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
		return engine.Simulate(in)
	})
	again := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
		return engine.Simulate(in)
	})
	assertSameCore(t, first, again)
	assertSameKeyframes(t, first, again, 0)

	// The same match computed concurrently in another goroutine must match too.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		par, _, perr := engine.Simulate(in)
		if perr != nil {
			t.Errorf("Simulate (parallel): %v", perr)
			return
		}
		assertSameCore(t, first, par)
		assertSameKeyframes(t, first, par, 0)
	}()
	wg.Wait()
}

// TestResumeMatchesStraightRun enforces the re-simulation equivalence rule:
// resuming from a minute-boundary snapshot reproduces the straight run bit for bit.
func TestResumeMatchesStraightRun(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(7)
	straight, snaps, err := engine.SimulateIntervened(in, nil)
	if err != nil {
		t.Fatalf("SimulateIntervened: %v", err)
	}
	snap := snapAt(t, snaps, 300)

	resumed, _, err := engine.Resume(snap, in, nil)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	assertSameCore(t, straight, resumed)
	assertSameKeyframes(t, straight, resumed, snap.Tick)
}

// TestResumeMatchesStraightRunWithIntervention: the same guarantee when a tactical
// change lands mid-match (both runs carry the identical intervention list).
func TestResumeMatchesStraightRunWithIntervention(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(9)
	tac := enginetest.SampleTactics()
	tac.Mentality = 5
	tac.Pressing = 3
	tac.Tempo = 5
	tac.CounterAttack = true
	payload, err := json.Marshal(tac)
	if err != nil {
		t.Fatalf("marshal tactics: %v", err)
	}
	ivs := []engine.Intervention{{
		EffectiveTick: 1200,
		Club:          in.Home.Club,
		Kind:          "tactics",
		Payload:       payload,
	}}

	straight, snaps, err := engine.SimulateIntervened(in, ivs)
	if err != nil {
		t.Fatalf("SimulateIntervened: %v", err)
	}
	snap := snapAt(t, snaps, 600)

	resumed, _, err := engine.Resume(snap, in, ivs)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	assertSameCore(t, straight, resumed)
	assertSameKeyframes(t, straight, resumed, snap.Tick)
}

// TestResumeFromJSONSnapshot proves snapshots survive serialization (workers persist
// them) and still resume bit-identically.
func TestResumeFromJSONSnapshot(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(11)
	_, snaps, err := engine.Simulate(in)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	snap := snapAt(t, snaps, 120)

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var restored engine.Snapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	base, _, err := engine.Resume(snap, in, nil)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	fromJSON, _, err := engine.Resume(restored, in, nil)
	if err != nil {
		t.Fatalf("Resume (from JSON): %v", err)
	}
	assertSameCore(t, base, fromJSON)
	assertSameKeyframes(t, base, fromJSON, snap.Tick)
}

// TestInterventionChangesOutcomes checks tactical interventions actually matter.
func TestInterventionChangesOutcomes(t *testing.T) {
	t.Parallel()

	tac := enginetest.SampleTactics()
	tac.Mentality = 5
	tac.Pressing = 3
	tac.Tempo = 5
	tac.CounterAttack = true
	payload, err := json.Marshal(tac)
	if err != nil {
		t.Fatalf("marshal tactics: %v", err)
	}

	changedOnce := false
	for seed := uint64(1); seed <= 10 && !changedOnce; seed++ {
		in := enginetest.SampleInput(seed)
		plain, _, err := engine.Simulate(in)
		if err != nil {
			t.Fatalf("Simulate (seed %d): %v", seed, err)
		}
		changed, _, err := engine.SimulateIntervened(in, []engine.Intervention{{
			EffectiveTick: 900,
			Club:          in.Home.Club,
			Kind:          "tactics",
			Payload:       payload,
		}})
		if err != nil {
			t.Fatalf("SimulateIntervened (seed %d): %v", seed, err)
		}
		dPlain, _ := enginetest.Digest(plain)
		dChanged, _ := enginetest.Digest(changed)
		changedOnce = dPlain != dChanged
	}
	if !changedOnce {
		t.Fatal("an all-out tactical intervention never changed any outcome across 10 seeds")
	}
}

func TestValidationRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*engine.MatchInput)
		wantErr error
	}{{
		name:    "engine version mismatch",
		mutate:  func(in *engine.MatchInput) { in.EngineVersion = 999 },
		wantErr: engine.ErrEngineVersion,
	}, {
		name:    "short lineup",
		mutate:  func(in *engine.MatchInput) { in.Home.Players = in.Home.Players[:10] },
		wantErr: engine.ErrBadLineup,
	}, {
		name: "keeper not first",
		mutate: func(in *engine.MatchInput) {
			in.Home.Players[0].Pos = engine.PosCB
			in.Home.Players[5].Pos = engine.PosGK
		},
		wantErr: engine.ErrBadLineup,
	}, {
		name: "duplicate player id",
		mutate: func(in *engine.MatchInput) {
			in.Away.Players[3].ID = in.Home.Players[5].ID
		},
		wantErr: engine.ErrDuplicatePlayer,
	}, {
		name: "attribute out of range",
		mutate: func(in *engine.MatchInput) {
			in.Home.Players[2].Attr.Passing = 21
		},
		wantErr: engine.ErrBadAttribute,
	}, {
		name: "morale out of range",
		mutate: func(in *engine.MatchInput) {
			in.Home.Players[2].Morale = 101
		},
		wantErr: engine.ErrBadCondition,
	}, {
		name: "bad tactics slider",
		mutate: func(in *engine.MatchInput) {
			in.Home.Tactics.Tempo = 9
		},
		wantErr: engine.ErrBadTactics,
	}, {
		name: "bad formation",
		mutate: func(in *engine.MatchInput) {
			in.Home.Tactics.Formation = [3]int{4, 4, 3}
		},
		wantErr: engine.ErrBadTactics,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := enginetest.SampleInput(1)
			tt.mutate(&in)
			_, _, err := engine.Simulate(in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnsupportedInterventionKind(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(1)
	_, _, err := engine.SimulateIntervened(in, []engine.Intervention{{
		EffectiveTick: 300,
		Club:          in.Home.Club,
		Kind:          "magic_spell",
		Payload:       json.RawMessage(`{}`),
	}})
	if !errors.Is(err, engine.ErrBadIntervention) {
		t.Fatalf("error = %v, want %v", err, engine.ErrBadIntervention)
	}
}

// TestSubstitutionIntervention checks the substitution intervention end to end.
func TestSubstitutionIntervention(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(21)
	sub := engine.SubstitutionPayload{
		PlayerOn:  in.Home.Players[14].ID,
		PlayerOff: in.Home.Players[9].ID,
	}
	payload, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal substitution: %v", err)
	}
	res, _, err := engine.SimulateIntervened(in, []engine.Intervention{{
		EffectiveTick: 2700,
		Club:          in.Home.Club,
		Kind:          engine.InterventionSubstitution,
		Payload:       payload,
	}})
	if err != nil {
		t.Fatalf("SimulateIntervened: %v", err)
	}

	found := false
	for _, ev := range res.Events {
		if ev.Kind == engine.EventSubstitution && ev.Player == sub.PlayerOn {
			found = true
		}
	}
	if !found {
		t.Fatal("no substitution event for the incoming player")
	}
	// Both the departing and the incoming player get exactly one rating entry.
	for _, id := range []engine.PlayerID{sub.PlayerOff, sub.PlayerOn} {
		n := 0
		for _, r := range res.PlayerRatings {
			if r.PlayerID == id {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("player %s has %d rating entries, want 1", id, n)
		}
	}
}

// TestResumeMatchesStraightRunWithSubstitution: re-simulation equivalence across a
// mid-match substitution (ratings deposit must survive the snapshot cut).
func TestResumeMatchesStraightRunWithSubstitution(t *testing.T) {
	t.Parallel()

	in := enginetest.SampleInput(23)
	payload, err := json.Marshal(engine.SubstitutionPayload{
		PlayerOn:  in.Away.Players[13].ID,
		PlayerOff: in.Away.Players[8].ID,
	})
	if err != nil {
		t.Fatalf("marshal substitution: %v", err)
	}
	ivs := []engine.Intervention{{
		EffectiveTick: 2400,
		Club:          in.Away.Club,
		Kind:          engine.InterventionSubstitution,
		Payload:       payload,
	}}

	straight, snaps, err := engine.SimulateIntervened(in, ivs)
	if err != nil {
		t.Fatalf("SimulateIntervened: %v", err)
	}
	snap := snapAt(t, snaps, 1800)

	resumed, _, err := engine.Resume(snap, in, ivs)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	assertSameCore(t, straight, resumed)
	assertSameKeyframes(t, straight, resumed, snap.Tick)
}

func mustSimulate(t *testing.T, sim func() (engine.MatchResult, []engine.Snapshot, error)) engine.MatchResult {
	t.Helper()
	res, _, err := sim()
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	return res
}

func snapAt(t *testing.T, snaps []engine.Snapshot, tick int32) engine.Snapshot {
	t.Helper()
	for _, s := range snaps {
		if s.Tick == tick {
			return s
		}
	}
	t.Fatalf("no snapshot at tick %d (have %d snapshots)", tick, len(snaps))
	return engine.Snapshot{}
}

// assertSameCore compares everything but the keyframe window.
func assertSameCore(t *testing.T, a, b engine.MatchResult) {
	t.Helper()

	if a.Score != b.Score {
		t.Fatalf("score = %+v, want %+v", a.Score, b.Score)
	}
	assertJSONEqual(t, "events", a.Events, b.Events)
	assertJSONEqual(t, "stats", a.Stats, b.Stats)
	assertJSONEqual(t, "player ratings", a.PlayerRatings, b.PlayerRatings)
}

// assertSameKeyframes compares keyframes from the given tick onwards (a resumed run
// only produces the suffix window).
func assertSameKeyframes(t *testing.T, a, b engine.MatchResult, fromTick int32) {
	t.Helper()

	filter := func(kfs []engine.Keyframe) []engine.Keyframe {
		out := make([]engine.Keyframe, 0, len(kfs))
		for _, kf := range kfs {
			if kf.TMs >= uint32(fromTick)*1000 {
				out = append(out, kf)
			}
		}
		return out
	}
	assertJSONEqual(t, "keyframes", filter(a.Keyframes), filter(b.Keyframes))
}

func assertJSONEqual(t *testing.T, what string, a, b any) {
	t.Helper()

	ra, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal %s (a): %v", what, err)
	}
	rb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal %s (b): %v", what, err)
	}
	if !bytes.Equal(ra, rb) {
		t.Fatalf("%s mismatch:\n a = %s\n b = %s", what, ra, rb)
	}
}
