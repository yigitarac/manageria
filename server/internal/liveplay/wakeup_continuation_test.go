package liveplay

import (
	"encoding/json"
	"reflect"
	"testing"
)

// t045State returns an independent copy of the serializable harness state.
func (h *t045Harness) t045State() t045HarnessState {
	return t045HarnessState{
		Mode:       h.mode,
		PendingAny: h.pendingAny,
		Cursor:     h.cursor,
		Pending:    append([]bool(nil), h.pendingLocal...),
	}
}

// t045ResumeFrom JSON-round-trips the Sim snapshot + harness state and resumes the
// harness. It returns the resumed harness and the state it round-tripped, so a
// test can assert pending=true and the per-runner tail. The harness slices are
// copied, never aliased to the live sim.
func t045ResumeFrom(t *testing.T, h *t045Harness) (*t045Harness, t045HarnessState) {
	t.Helper()
	snapBlob, err := json.Marshal(h.sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal sim snapshot: %v", err)
	}
	want := h.t045State()
	stateBlob, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal harness state: %v", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(snapBlob, &snap); err != nil {
		t.Fatalf("unmarshal sim snapshot: %v", err)
	}
	var st t045HarnessState
	if err := json.Unmarshal(stateBlob, &st); err != nil {
		t.Fatalf("unmarshal harness state: %v", err)
	}
	if !reflect.DeepEqual(st, want) {
		t.Fatalf("harness state lost in JSON round-trip: %+v vs %+v", st, want)
	}
	rsim, err := Resume(snap)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	rh := &t045Harness{
		sim:          rsim,
		mode:         st.Mode,
		pendingAny:   st.PendingAny,
		cursor:       st.Cursor,
		pendingLocal: append([]bool(nil), st.Pending...),
		lastCount:    rsim.DecisionPublishCount(),
		recordPubs:   h.recordPubs,
	}
	return rh, st
}

// t045AdvanceTo steps the harness until its sim clock reaches clock.
func t045AdvanceTo(h *t045Harness, clock int) {
	for h.sim.ClockMS() < clock {
		h.step()
	}
}

// t045CompareTails steps straight and resumed harnesses to horizon in lockstep,
// asserting the COMPLETE Sim snapshot, the full harness state AND the per-runner
// publication records match at every step. It returns the appended publication
// records from the split onward for each (the straight slice is offset by the
// pre-split record count). The gated modes do not use the sim's own publication
// counter, so the harness record is the diagnostic publication tail.
func t045CompareTails(t *testing.T, straight, resumed *t045Harness, horizon int, label string) ([]t045Pub, []t045Pub) {
	t.Helper()
	basePub := len(straight.pubs)
	for straight.sim.ClockMS() < horizon {
		sStart := straight.sim.ClockMS()
		straight.step()
		resumed.step()
		if straight.sim.ClockMS() != resumed.sim.ClockMS() {
			t.Fatalf("%s @%d: clock diverged %d vs %d", label, sStart, straight.sim.ClockMS(), resumed.sim.ClockMS())
		}
		if !reflect.DeepEqual(straight.sim.Snapshot(), resumed.sim.Snapshot()) {
			t.Fatalf("%s @%d: complete Sim snapshot diverged after resume", label, straight.sim.ClockMS())
		}
		if !reflect.DeepEqual(straight.t045State(), resumed.t045State()) {
			t.Fatalf("%s @%d: harness state diverged after resume: %+v vs %+v", label, straight.sim.ClockMS(), straight.t045State(), resumed.t045State())
		}
	}
	if !reflect.DeepEqual(t042SamplesTail(straight.sim, resumed.sim), resumed.sim.Samples()) {
		t.Fatalf("%s: resumed sample tail differs", label)
	}
	return append([]t045Pub(nil), straight.pubs[basePub:]...), append([]t045Pub(nil), resumed.pubs...)
}

// t042SamplesTail returns the straight run's sample suffix aligned to the resumed
// run's length.
func t042SamplesTail(straight, resumed *Sim) []Sample {
	all := straight.Samples()
	head := len(all) - len(resumed.Samples())
	if head < 0 {
		return nil
	}
	return all[head:]
}
