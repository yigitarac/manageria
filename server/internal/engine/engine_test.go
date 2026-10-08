package engine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

// TestGoldenDigest pins the exact output of a fixture match. Bump the expected value
// deliberately whenever EngineVersion behaviour changes (see the testing rules).
func TestGoldenDigest(t *testing.T) {
	t.Parallel()

	res := mustSimulate(t, func() (engine.MatchResult, []engine.Snapshot, error) {
		return engine.Simulate(enginetest.SampleInput(42))
	})
	got, err := enginetest.Digest(res)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	// v7 goldens — the emergent behaviour layer (T-014 stages B/C/D-lite): options-based
	// decisions with pass-lane geometry and shoot windows, one coherent retention budget,
	// runner jobs (pin/held-width/edge), box mark discipline and honest chain outcomes.
	// Calibration bands green (2.56 goals · 45.6/26.9/27.5), behavioural KPI contract
	// green (chains ~125, median chain 4, ~860 passes) and the T-013 bunker paradox
	// SOLVED (low blocks hold longer: 59.6′ vs 53.8′). Frozen deliberately.
	const want = "491f51ec8fd1def9ec00ab89e9d308b6a66a83a7dff408aa865006b31b2b9d7b"
	if got != want {
		t.Fatalf("golden digest = %s, want %s (update deliberately on EngineVersion bumps)", got, want)
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
