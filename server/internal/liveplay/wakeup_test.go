package liveplay

import "testing"

// T-045 local event-wakeup isolation study (isolated prototype evidence).
// See vault/07-Session-Logs/2026-10-12-t045-local-wakeup-evidence.md.
//
// Modes:
//   - t045Shared          : the unmodified T-042 scheduler (control).
//   - t045GatedUnion      : REJECTED global-union gating (any local event wakes ALL).
//   - t045GatedPerRunner  : true per-runner local gating (the studied controller).

type t045Mode int

const (
	t045Shared t045Mode = iota
	t045GatedUnion
	t045GatedPerRunner
)

// String returns the stable label for evidence output.
func (m t045Mode) String() string {
	switch m {
	case t045GatedUnion:
		return "gated-union"
	case t045GatedPerRunner:
		return "gated-per-runner"
	default:
		return "shared"
	}
}

// t045StateChanging is the shared invalidation set (release included, path excluded).
func t045StateChanging(k PassEventKind) bool {
	switch k {
	case PassEventRelease, PassEventControl, PassEventDeflect, PassEventStop, PassEventOut:
		return true
	}
	return false
}

// t045Pub is one per-runner publication record (time, runner, reason, target,
// observation at publication) with an explicit attempt-vs-executed label:
// ReactEligible is the reaction gate (fell due), Applied means the local intent
// policy actually ran (a flagged, inertial, sensing, reacted local runner), and
// Sampled means the observation record was refreshed at this clock.
type t045Pub struct {
	Clock         int             `json:"clock"`
	Runner        int             `json:"runner"`
	Reason        string          `json:"reason"` // lattice | local-wake | union-wake
	IntentX       float64         `json:"intentX"`
	IntentY       float64         `json:"intentY"`
	Obs           BallObservation `json:"obs"`
	ReactEligible bool            `json:"reactEligible"`
	Applied       bool            `json:"applied"`
	Sampled       bool            `json:"sampled"`
}

// t045HarnessState is the complete serializable harness state. The Sim snapshot
// carries the lattice (NextDecisionMS) and decisionDirty separately.
type t045HarnessState struct {
	Mode       t045Mode `json:"mode"`
	PendingAny bool     `json:"pendingAny"`
	Cursor     int      `json:"cursor"`
	Pending    []bool   `json:"pending"`
}

// t045Save captures a runner's suppressible opt-in fields during a per-runner
// publication so they are restored before any physics runs.
type t045Save struct {
	per           *BallObservation
	local, pursue bool
}

// t045Harness drives a Sim under one wakeup source. In the gated modes the
// harness owns the per-runner publication; the Sim's own scheduler is kept inert
// (decisionDirty cleared, lattice advanced) so Step runs physics/contact only.
type t045Harness struct {
	sim          *Sim
	mode         t045Mode
	cursor       int
	pendingLocal []bool
	pendingAny   bool
	lastCount    int
	recordPubs   bool

	preX, preY []float64
	endX, endY []float64
	eligBuf    []bool
	saveBuf    []t045Save
	pubs       []t045Pub
	pubSteps   int // number of pre-steps at which a gated publication happened
}

// t045NewHarness builds a harness over a fresh sim.
func t045NewHarness(cfg Config, seed uint64, mode t045Mode) (*t045Harness, error) {
	sim, err := NewSim(cfg, seed)
	if err != nil {
		return nil, err
	}
	return &t045Harness{sim: sim, mode: mode, lastCount: sim.DecisionPublishCount(), recordPubs: true}, nil
}

// t045MustHarness is the test helper constructor.
func t045MustHarness(t *testing.T, cfg Config, seed uint64, mode t045Mode) *t045Harness {
	t.Helper()
	h, err := t045NewHarness(cfg, seed, mode)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	return h
}

// syncLen sizes the per-runner scratch to the current runner count.
func (h *t045Harness) syncLen() {
	n := len(h.sim.runners)
	if cap(h.pendingLocal) < n {
		h.pendingLocal = make([]bool, n)
	} else {
		h.pendingLocal = h.pendingLocal[:n]
	}
}

// capture copies current runner positions into reusable storage.
func (h *t045Harness) capture(dstX, dstY []float64) ([]float64, []float64) {
	n := len(h.sim.runners)
	if cap(dstX) < n {
		dstX = make([]float64, n)
	}
	dstX = dstX[:n]
	if cap(dstY) < n {
		dstY = make([]float64, n)
	}
	dstY = dstY[:n]
	for i := 0; i < n; i++ {
		dstX[i] = h.sim.runners[i].X
		dstY[i] = h.sim.runners[i].Y
	}
	return dstX, dstY
}

// scanNew consumes unread events and sets the per-runner local wakeup for every
// opted-in runner whose own closed sensing circle contains the event's recorded
// ball position at its OWN event-time geometry (runner position interpolated at
// Frac between pre and end). pendingAny records any state-changing event for the
// shared-semantics native path.
func (h *t045Harness) scanNew(px, py, ex, ey []float64) {
	evs := h.sim.events
	for h.cursor < len(evs) {
		e := evs[h.cursor]
		h.cursor++
		if !t045StateChanging(e.Kind) {
			continue
		}
		h.pendingAny = true
		for i := range h.sim.runners {
			r := &h.sim.runners[i]
			if !r.LocalPursuitEnabled() && !r.SenseEnabled() {
				continue
			}
			rx := px[i] + e.Frac*(ex[i]-px[i])
			ry := py[i] + e.Frac*(ey[i]-py[i])
			if DistM(e.BallX, e.BallY, rx, ry) <= r.Perception.RadiusM {
				h.pendingLocal[i] = true
			}
		}
	}
}

// t045ClassifyCurrent classifies one emitted event for runner i with the oracle
// at the current positions (boundary/audit helper).
func (h *t045Harness) t045ClassifyCurrent(i int, e PassEvent) bool {
	r := &h.sim.runners[i]
	rx, ry := r.X, r.Y
	return DistM(e.BallX, e.BallY, rx, ry) <= r.Perception.RadiusM
}

// eligible returns the per-runner publication eligibility for this pre-step.
// The periodic lattice publishes every opted-in runner; off-lattice only woken
// runners are published — per-runner pending (t045GatedPerRunner) or the union
// (t045GatedUnion, rejected). Native (non-local) runners keep the shared
// any-event semantics via pendingAny.
func (h *t045Harness) eligible(latticeDue bool) []bool {
	n := len(h.sim.runners)
	if cap(h.eligBuf) < n {
		h.eligBuf = make([]bool, n)
	}
	el := h.eligBuf[:n]
	union := false
	for i := 0; i < n; i++ {
		el[i] = false
		if h.pendingLocal[i] {
			union = true
		}
	}
	for i := 0; i < n; i++ {
		r := &h.sim.runners[i]
		localOpt := r.LocalPursuitEnabled() || r.SenseEnabled()
		nativeOpt := r.Pursue && r.Motion == MotionInertial && !r.LocalPursuit
		if latticeDue {
			el[i] = localOpt || nativeOpt
			continue
		}
		wake := false
		switch h.mode {
		case t045GatedUnion:
			wake = union
		case t045GatedPerRunner:
			wake = h.pendingLocal[i]
		}
		if localOpt && wake {
			el[i] = true
		}
		if nativeOpt && h.pendingAny {
			el[i] = true
		}
	}
	h.eligBuf = el
	return el
}

// publishEligible temporarily suppresses every non-eligible runner and calls the
// REAL per-runner production publication (native oracle, sampling, local policy),
// then restores the flags/records pointer. Only flags/pointers are toggled around
// the call; no physics, contact or integration is altered. It then optionally
// records the per-runner publication for evidence.
func (h *t045Harness) publishEligible(stepStart int, latticeDue bool, el []bool) {
	n := len(h.sim.runners)
	if cap(h.saveBuf) < n {
		h.saveBuf = make([]t045Save, n)
	}
	saved := h.saveBuf[:n]
	any := false
	for i := 0; i < n; i++ {
		r := &h.sim.runners[i]
		saved[i] = t045Save{r.Perception, r.LocalPursuit, r.Pursue}
		if !el[i] {
			r.Perception = nil
			r.LocalPursuit = false
			r.Pursue = false
		} else {
			any = true
		}
	}
	if any {
		h.sim.publishPursuitIntents()
		h.sim.samplePerception(stepStart)
		h.sim.publishLocalPursuitIntents(stepStart)
		h.pubSteps++
	}
	for i := 0; i < n; i++ {
		r := &h.sim.runners[i]
		r.Perception = saved[i].per
		r.LocalPursuit = saved[i].local
		r.Pursue = saved[i].pursue
	}
	h.saveBuf = saved
	if !h.recordPubs {
		return
	}
	for i := 0; i < n; i++ {
		if !el[i] {
			continue
		}
		r := &h.sim.runners[i]
		if !r.LocalPursuitEnabled() && !r.SenseEnabled() {
			continue
		}
		reason := "lattice"
		if !latticeDue {
			if h.mode == t045GatedUnion {
				reason = "union-wake"
			} else {
				reason = "local-wake"
			}
		}
		var obs BallObservation
		if r.Perception != nil {
			obs = *r.Perception
		}
		reactEligible := stepStart >= r.ReactAtMS
		applied := r.LocalPursuit && r.Motion == MotionInertial && r.Perception.SensingEnabled() && reactEligible
		sampled := r.Perception != nil && r.Perception.Seen && r.Perception.ClockMS == stepStart
		h.pubs = append(h.pubs, t045Pub{
			Clock: stepStart, Runner: i, Reason: reason,
			IntentX: r.IntentX, IntentY: r.IntentY, Obs: obs,
			ReactEligible: reactEligible, Applied: applied, Sampled: sampled,
		})
	}
}

// consume clears the pending wakeups that this pre-step published.
func (h *t045Harness) consume(el []bool) {
	for i := range h.pendingLocal {
		if i < len(el) && el[i] {
			h.pendingLocal[i] = false
		}
	}
	h.pendingAny = false
}

// step advances one substep. Shared mode runs the unmodified scheduler inside
// Step. Gated modes run the per-runner publication themselves and keep the sim's
// own scheduler inert so physics/contact is unchanged.
func (h *t045Harness) step() {
	h.syncLen()
	stepStart := h.sim.ClockMS()
	h.preX, h.preY = h.capture(h.preX, h.preY)
	h.scanNew(h.preX, h.preY, h.preX, h.preY) // dynamic events emitted before this step

	if h.mode == t045Shared {
		h.sim.Step()
	} else {
		latticeDue := stepStart >= h.sim.nextDecisionMS
		if latticeDue {
			c := h.sim.cfg.IntentCadenceMS
			h.sim.nextDecisionMS = (stepStart/c + 1) * c
		}
		el := h.eligible(latticeDue)
		h.publishEligible(stepStart, latticeDue, el)
		h.consume(el)
		h.sim.decisionDirty = false
		h.sim.Step()
	}
	h.endX, h.endY = h.capture(h.endX, h.endY)
	h.scanNew(h.preX, h.preY, h.endX, h.endY) // events emitted during this step
}
