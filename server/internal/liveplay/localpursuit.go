package liveplay

import (
	"errors"
	"fmt"
)

// Observation-driven local pursuit (T-044).
//
// T-038 gave the native live path an OMNIORACLE pursuit: every opted-in runner
// republishes its intent to the ball's TRUE current position (publishPursuitIntents,
// reading s.ball). T-043 added an observation-only per-runner local ball record
// (perception.go) but did not consume it. T-044 adds the smallest possible
// CONSUMER of that record: a per-runner local intent policy that decides its
// movement destination from its OWN last local sighting, never from the remote
// world.
//
// This is deliberately a DIFFERENT, TRIVIAL sampled model, not an optimization of
// the native oracle, and it makes NO realism/team-AI claim:
//   - Unknown (never seen the ball)      -> brake at the runner's own position;
//   - Stale (seen, not refreshed now)    -> RETAIN the prior destination (an
//     explicit assumption; no velocity extrapolation, no time-to-intercept);
//   - Fresh Rolling/Loose                -> chase the observed ball coords;
//   - Fresh Held/Dead/Out                -> brake at the runner's own position
//     (a trivial local policy; the holder is NOT coordinated with any team AI).
//
// Runtime safety guards (not a plausibility promise): a missing/invalid perception
// record, a non-inertial runner or an undeclared observed mode yields NO oracle and
// NO new target (the runner keeps its existing intent). The runtime checks only the
// opt-in FLAG, the MOTION model, the finite-positive RADIUS, the reaction gate and
// the observed MODE; it does NOT validate whether an observation is physically
// plausible (that is the import path's job: EnableLocalPursuit validates the radius
// and Resume additionally validates the timestamp/mode/owner against the snapshot
// clock). A raw record hand-set into Runner.Perception/LocalPursuit outside those
// paths (trusted same-package fixtures) is honoured as-is for stale retention. The
// native publisher ALWAYS skips a local runner, so a disabled scheduler or an
// invalid radius can never silently reveal the remote ball position. The policy
// draws NO RNG and changes no motion, contact, reach, order, cooldown, immunity or
// reaction behaviour.

// Local-pursuit opt-in errors. Broken input is reported, not silently accepted.
var (
	// ErrLocalPursuitMotion: local pursuit requires the inertial motion model
	// (the only model that integrates an intent). The helper rejects a
	// non-inertial runner rather than imposing hidden motion parameters.
	ErrLocalPursuitMotion = errors.New("liveplay: local pursuit requires inertial motion")
	// ErrLocalPursuitNoPerception: a persisted local-pursuit runner must carry a
	// valid sensing record (finite radius > 0); Resume rejects a missing record.
	ErrLocalPursuitNoPerception = errors.New("liveplay: local pursuit requires a valid perception record")
	// ErrLocalPursuitNoScheduler: local pursuit only runs during an armed
	// scheduler publication, so a persisted local runner is inconsistent with a
	// disabled scheduler (Config.IntentCadenceMS == 0).
	ErrLocalPursuitNoScheduler = errors.New("liveplay: local pursuit requires an armed intent scheduler")
)

// EnableLocalPursuit installs the T-043 sensing record with the given metre
// radius AND opts the runner into the T-044 local intent policy. It validates the
// radius (finite, strictly positive) and requires Motion==MotionInertial, and
// imposes NO hidden motion parameters (the caller is responsible for the inertial
// envelope, e.g. via InertialRunner). It returns an error rather than silently
// accepting broken input.
func (r *Runner) EnableLocalPursuit(radiusM float64) error {
	if r.Motion != MotionInertial {
		return fmt.Errorf("%w: motion=%s", ErrLocalPursuitMotion, r.Motion)
	}
	if err := r.EnableSensing(radiusM); err != nil {
		return err
	}
	r.LocalPursuit = true
	return nil
}

// DisableLocalPursuit clears the T-044 opt-in flag but leaves any sensing record
// in place (DisableSensing removes that separately).
func (r *Runner) DisableLocalPursuit() { r.LocalPursuit = false }

// LocalPursuitEnabled reports whether this runner would run the local policy,
// failing CLOSED exactly like the runtime does: it requires the flag, the
// inertial model and a valid sensing record.
func (r Runner) LocalPursuitEnabled() bool {
	return r.LocalPursuit && r.Motion == MotionInertial && r.Perception.SensingEnabled()
}

// publishLocalPursuitIntents is the T-044 local policy. It runs ONLY inside an
// armed scheduler publication, AFTER samplePerception, at the consistent pre-step
// clock `stepStart`. It reads ONLY each runner's own observation record and own
// position: it never reads s.ball, the event log, the ball mode/owner, or any
// other runner's state, so there is no remote leak and no update-order advantage.
//
// It fails closed: a non-inertial runner, a nil record or an invalid radius is
// skipped (no oracle, no new target), the reaction gate is applied identically to
// native pursuit, and an undeclared observed mode changes nothing. The disabled
// native path never publishes at all; the cadence==SubstepMS equivalence anchor
// still calls this every substep but is INERT because no local runner is opted in.
//
// The ungated form is retained for direct callers and is covered by the T-044
// tests; the armed scheduler (T-046) calls publishLocalPursuitIntentsGate so an
// opted-in local-wakeup runner is only published when its own per-runner gate
// selects it.
func (s *Sim) publishLocalPursuitIntents(stepStart int) {
	s.publishLocalPursuitIntentsGate(stepStart, nil)
}

// publishLocalPursuitIntentsGate is publishLocalPursuitIntents restricted to the
// runners the caller selected (a nil gate selects every runner, the pre-T-046
// behaviour).
func (s *Sim) publishLocalPursuitIntentsGate(stepStart int, gate []bool) {
	for i := range s.runners {
		if gate != nil && !gate[i] {
			continue
		}
		r := &s.runners[i]
		// The decision table is the single source of truth shared with the T-049
		// provenance recorder (localDecision), so the exported policy can never
		// disagree with the applied policy. A reaction-blocked or invalid
		// enabling returns ok=false and changes nothing (fail closed).
		policy, ok, _ := localDecision(*r, stepStart)
		if !ok {
			continue
		}
		switch policy {
		case PolicyUnknownBrake, PolicyNonFreeBrake:
			// Unknown / non-free: brake at the runner's own position. The
			// integrator decelerates through the existing inertia.
			r.IntentX, r.IntentY = r.X, r.Y
		case PolicyChase:
			// A free ball: chase the observed position.
			r.IntentX, r.IntentY = r.Perception.X, r.Perception.Y
		default: // PolicyStaleRetain: retain the prior destination (no extrapolation)
		}
	}
}

// validateLocalPursuitContext checks a persisted local-pursuit runner against the
// RESUMED context. It is the import-path (EnableLocalPursuit / Resume) guard: the
// raw SetRunners API has no error return and instead fails closed at runtime. A
// runner without the flag is always valid. A flagged runner must be inertial, carry
// a valid sensing record, and be resumed with an armed scheduler.
func validateLocalPursuitContext(r Runner, cfg Config) error {
	if !r.LocalPursuit {
		return nil
	}
	if r.Motion != MotionInertial {
		return fmt.Errorf("%w: motion=%s", ErrLocalPursuitMotion, r.Motion)
	}
	if !r.Perception.SensingEnabled() {
		return ErrLocalPursuitNoPerception
	}
	if cfg.IntentCadenceMS == 0 {
		return ErrLocalPursuitNoScheduler
	}
	return nil
}

// LocalPursuitState returns a value copy of a runner's local-pursuit policy state
// for evidence/tests: the opt-in flag, whether the runtime would actually run it
// (fail-closed conjunction) and the persisted destination intent. It never exposes
// a live pointer.
func (s *Sim) LocalPursuitState(index int) (enabled, active bool, intentX, intentY float64) {
	if index < 0 || index >= len(s.runners) {
		return false, false, 0, 0
	}
	r := s.runners[index]
	return r.LocalPursuit, r.LocalPursuitEnabled(), r.IntentX, r.IntentY
}
