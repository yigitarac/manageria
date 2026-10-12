package liveplay

import "errors"

// Opt-in per-runner local event wakeup (T-046).
//
// T-042's shared scheduler wakes EVERY opted-in runner on ANY state-changing ball
// event (release/control/deflect/stop/out) through one global decisionDirty flag,
// plus the periodic lattice. T-045 measured that this leaks a REMOTE event's
// occurrence/timing into a local runner's persisted intent and trajectory even
// though the T-043 radius prevents a remote POSITION write. T-046 turns the T-045
// test-only per-runner gate into the smallest persisted OPT-IN runtime contract:
//
//   - Runner.LocalWakeup (json omitempty) opts a runner in; it requires an armed
//     scheduler (Config.IntentCadenceMS > 0) and a valid sensing record (finite
//     radius > 0). Anything else fails closed.
//   - Runner.WakePending (json omitempty) is the persisted pending wakeup, set at
//     the event emitter when a state-changing event is LOCAL to the runner by
//     event-time geometry, coalesced across one substep, and consumed at the
//     immediate next pre-step publication regardless of the reaction gate.
//
// An UNFLAGGED runner keeps the exact T-042 shared semantics, and the native
// Pursue publication is ALWAYS shared (publishPursuitIntents still runs for every
// non-local-pursuit runner on any event): the native publisher and the local
// sample/local intent are separate layers. Global union locality (any local event
// wakes ALL local runners) was REJECTED in T-045 and is not implemented.
//
// This layer changes NO contact/cooldown/immunity/reaction logic, integrates EVERY
// body EVERY microstep and draws NO RNG. The physical reaction deadline reset that
// ReleasePass performs on every runner is a SEPARATE retained side channel (T-045
// §4.3): T-046 does NOT remove it and therefore claims NO total sensory/physical
// state noninterference.

// Local-wakeup opt-in errors. Broken or impossible input is reported, not trusted.
var (
	// ErrLocalWakeupPending: a persisted WakePending is set on a runner that is not
	// opted in. A pending can only be produced by an opted-in runner, so this is an
	// impossible armed state and Resume rejects it.
	ErrLocalWakeupPending = errors.New("liveplay: local wakeup pending requires the local wakeup opt-in")
	// ErrLocalWakeupNoPerception: a persisted local-wakeup runner (or a set pending)
	// must carry a valid sensing record (finite radius > 0); Resume rejects a
	// missing record.
	ErrLocalWakeupNoPerception = errors.New("liveplay: local wakeup requires a valid perception record")
	// ErrLocalWakeupNoScheduler: local wakeup only runs during an armed scheduler
	// publication, so a persisted local-wakeup runner is inconsistent with a
	// disabled scheduler (Config.IntentCadenceMS == 0).
	ErrLocalWakeupNoScheduler = errors.New("liveplay: local wakeup requires an armed intent scheduler")
)

// EnableLocalWakeup installs the T-043 sensing record with the given metre radius
// and opts the runner into per-runner local event wakeup. It validates the radius
// (finite, strictly positive) and returns an error rather than silently accepting
// broken input. Unlike EnableLocalPursuit it does NOT require inertial motion: a
// sensing-only runner may opt in, because local wakeup gates SAMPLING as well as
// the local intent policy. It imposes no hidden motion parameters.
//
// The fresh opt-in RESETS WakePending to false (EnableSensing also replaces the
// record): a caller opting a runner in starts from a clean, coherent state rather
// than inheriting a stale pending that Resume would then have to accept.
func (r *Runner) EnableLocalWakeup(radiusM float64) error {
	if err := r.EnableSensing(radiusM); err != nil {
		return err
	}
	r.LocalWakeup = true
	r.WakePending = false
	return nil
}

// DisableLocalWakeup clears the T-046 opt-in flag AND its persisted pending, but
// leaves any sensing record in place (DisableSensing removes that separately). A
// pending cannot outlive its opt-in — leaving one would be an impossible state
// that Resume rejects — so disabling always clears it.
func (r *Runner) DisableLocalWakeup() {
	r.LocalWakeup = false
	r.WakePending = false
}

// LocalWakeupEnabled reports whether this runner opts into per-runner local event
// wakeup, failing CLOSED exactly like the runtime: it requires the flag AND a valid
// sensing record. The caller additionally requires an armed scheduler.
func (r Runner) LocalWakeupEnabled() bool {
	return r.LocalWakeup && r.Perception.SensingEnabled()
}

// isStateChangingEvent reports whether a live-pass event invalidates stale local
// state: release/control/deflect/stop/out. Path is a pure position trace and is
// deliberately inert.
func isStateChangingEvent(k PassEventKind) bool {
	switch k {
	case PassEventRelease, PassEventControl, PassEventDeflect, PassEventStop, PassEventOut:
		return true
	default:
		return false
	}
}

// noteLocalWake sets each opted-in local runner's persisted WakePending when the
// just-emitted state-changing event is LOCAL to it. Locality is measured at the
// event's OWN event-time geometry: the recorded event ball position against the
// runner position at the event instant.
//
// The event instant depends on WHERE the event is emitted:
//   - a RELEASE is emitted BETWEEN steps, so the runner's CURRENT position is its
//     event-time position;
//   - every other state-changing event (control/deflect/stop/out) is emitted
//     DURING a step, so the runner position is interpolated at the event fraction
//     between the captured PRE-step and END-step positions. A contact at fraction
//     0 therefore uses the PRE-step position, NOT the end-step one.
//
// It reads only the emitted event (never the future next-pre-step ball) and writes
// the pending boolean only — no observation record and no owner are taken from the
// event. It is inert while the scheduler is disabled and for path events, so the
// disabled/default JSON and trajectory are unchanged. A non-finite distance
// (broken input) is skipped rather than admitted by a NaN comparison.
func (s *Sim) noteLocalWake(kind PassEventKind, ballX, ballY, frac float64) {
	if !s.schedulerEnabled() {
		return
	}
	if !isStateChangingEvent(kind) {
		return
	}
	interpolate := kind != PassEventRelease
	for i := range s.runners {
		r := &s.runners[i]
		if !r.LocalWakeupEnabled() {
			continue
		}
		rx, ry := r.X, r.Y
		if interpolate && i < len(s.preX) && i < len(s.preY) {
			rx = s.preX[i] + frac*(r.X-s.preX[i])
			ry = s.preY[i] + frac*(r.Y-s.preY[i])
		}
		if d := DistM(ballX, ballY, rx, ry); isFinite(d) && d <= r.Perception.RadiusM {
			r.WakePending = true
		}
	}
}

// anyLocalPending reports whether any opted-in local-wakeup runner currently has a
// pending wakeup that CAN be honoured (raw flag + valid perception). The scheduler
// ORs this into the publication decision, so a legitimately persisted pending is
// never silently ignored merely because decisionDirty was not restored.
func (s *Sim) anyLocalPending() bool {
	for i := range s.runners {
		if s.runners[i].LocalWakeupEnabled() && s.runners[i].WakePending {
			return true
		}
	}
	return false
}

// publishLocalSamples computes the per-runner local publication gate and runs the
// local sample + local intent layers over the selected runners only. It runs once
// per armed scheduler publication, AFTER the (unchanged, always-shared) native
// publisher and BEFORE any integration.
//
// `sharedDue` is the SHARED decision signal (lattice OR shared decisionDirty): an
// unflagged runner retains the exact T-042 shared semantics and is only run when
// sharedDue. An opted-in LocalWakeup runner is additionally selected off-lattice by
// its own pending (lattice OR pending). A pending-only publication (pendingAny
// without lattice/dirty) therefore never runs an unflagged sensing/local runner or
// the native oracle — it only serves the opted-in runner whose pending it carries.
//
// Fast path: with NO opt-in runner (no runner has the raw LocalWakeup flag) it runs
// the EXACT pre-T-046 shared local layer over every runner and allocates no
// diagnostics at all. The per-runner gate/pending evidence slices are created only
// once a local-wakeup opt-in exists, and are reused (never a growing log).
//
// With at least one raw opt-in:
//   - an opted-in runner with a valid record runs iff latticeDue OR its own
//     WakePending; a lattice/publication coincidence is still ONE publication
//     (lattice => one) and the pending is cleared (drop-before-reaction);
//   - an opted-in runner with an INVALID enabling fails CLOSED: its local layer is
//     never run and any pending it cannot honour is cleared;
//   - an unflagged runner runs iff sharedDue (the exact T-042 shared semantics).
func (s *Sim) publishLocalSamples(stepStart int, latticeDue, sharedDue bool) {
	anyOptIn := false
	for i := range s.runners {
		if s.runners[i].LocalWakeup {
			anyOptIn = true
			break
		}
	}
	if !anyOptIn {
		if sharedDue {
			s.samplePerception(stepStart)
			s.publishLocalPursuitIntents(stepStart)
		}
		return
	}
	n := len(s.runners)
	if cap(s.localGate) < n {
		s.localGate = make([]bool, n)
	}
	gate := s.localGate[:n]
	if cap(s.localPubMS) < n {
		ms := make([]int, n)
		copy(ms, s.localPubMS)
		for i := len(s.localPubMS); i < n; i++ {
			ms[i] = -1
		}
		s.localPubMS = ms
	}
	pubMS := s.localPubMS[:n]
	for i := range s.runners {
		r := &s.runners[i]
		if !r.LocalWakeup {
			gate[i] = sharedDue // unflagged: exact T-042 shared semantics
			r.WakePending = false
			continue
		}
		if !r.Perception.SensingEnabled() {
			// Raw flag set but the enabling is invalid: never run the local layer for
			// this runner, and drop any pending it cannot honour.
			gate[i] = false
			r.WakePending = false
			continue
		}
		run := latticeDue || r.WakePending
		gate[i] = run
		if run {
			s.localWakePublications++
			pubMS[i] = stepStart
			r.WakePending = false
		}
	}
	s.localGate = gate
	s.localPubMS = pubMS
	s.samplePerceptionGate(stepStart, gate)
	s.publishLocalPursuitIntentsGate(stepStart, gate)
}

// validateLocalWakeupContext checks a persisted local-wakeup runner against the
// RESUMED context. It is the import-path (EnableLocalWakeup / Resume) guard: the
// raw SetRunners API has no error return and instead fails closed at runtime. A
// runner without the flag and without a pending is always valid. A flagged runner
// must carry a valid sensing record and be resumed with an armed scheduler; a set
// pending on an unflagged runner is an impossible armed state.
func validateLocalWakeupContext(r Runner, cfg Config) error {
	if !r.LocalWakeup && !r.WakePending {
		return nil
	}
	if !r.LocalWakeup {
		return ErrLocalWakeupPending
	}
	if !r.Perception.SensingEnabled() {
		return ErrLocalWakeupNoPerception
	}
	if cfg.IntentCadenceMS == 0 {
		return ErrLocalWakeupNoScheduler
	}
	return nil
}

// LocalWakeupPublishCount returns how many per-runner local-layer SCHEDULING
// ATTEMPTS this run has made for runners opted into LocalWakeup. An attempt means
// the runner's local layer was selected at a pre-step (lattice or its own pending);
// the reaction gate may still DROP the sample/intent in that same attempt, so this
// is NOT a count of applied outcomes. It is evidence only and is NOT the shared
// global scheduler counter (DecisionPublishCount), which a remote event still
// bumps. Per-publication provenance (reason/outcome) is deliberately deferred to
// T-049 rather than added here as an unbounded log.
func (s *Sim) LocalWakeupPublishCount() int { return s.localWakePublications }

// LocalWakeupPublishMS returns the last pre-step clock at which runner index's
// local layer was scheduled (an attempt, which the reaction gate may have dropped),
// or -1 if it never has (or the index is out of range). It is evidence only and is
// NOT serialized.
func (s *Sim) LocalWakeupPublishMS(index int) int {
	if index < 0 || index >= len(s.localPubMS) {
		return -1
	}
	return s.localPubMS[index]
}
