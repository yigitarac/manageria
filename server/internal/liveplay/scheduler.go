package liveplay

// Opt-in sampled intent scheduler (T-042).
//
// The native live path republishes every pursuing runner's movement intent to
// the ball's current position at the START of EVERY substep
// (publishPursuitIntents). T-042 adds an OPT-IN sampled model in which that
// perception/decision publication is SCHEDULED instead of run every substep:
//
//   - on a stable periodic lattice `t0, cadence, 2*cadence, ...`, and
//   - out-of-band, on the substep after any state-changing ball event
//     (release/control/deflect/stop/out) emitted by a PRIOR completed step.
//
// Integration is untouched: every body integrates EVERY substep (including Held
// and Dead), and the reaction, cooldown and passer-immunity gates are never
// bypassed by scheduling. A held publication is a no-op on this pursuit-only
// layer; no held-player duties are claimed.
//
// T-043 adds the observation-only local ball perception sample to this same
// publication: samplePerception records each opted-in runner's CURRENT pre-step
// local sighting after the reaction gate. It never changes intents, contacts,
// motion or RNG.
//
// T-044 adds a deliberately trivial CONSUMER of that record, again on this same
// publication and AFTER the sample: publishLocalPursuitIntents lets a runner with
// LocalPursuit chase its own last local sighting (or brake when it has none / the
// ball is not free), reading only its own record and position. The native oracle
// always skips local runners, so a disabled scheduler or an invalid radius can
// never reveal the remote ball position.
//
// The zero value of Config.IntentCadenceMS (0) keeps the exact native
// per-substep behaviour, so no existing fixture, bit or JSON key changes. A
// non-zero cadence is valid only with LivePass and must be a positive multiple
// of SubstepMS.
//
// This is a DIFFERENT SAMPLED DECISION MODEL, not a bitwise optimization of the
// native per-substep pursuit: only cadence == SubstepMS is bit-identical to
// native (the equivalence anchor). Coarser cadences change trajectories and are
// measured, never assumed safe.

// schedulerEnabled reports whether the opt-in sampled scheduler is active.
func (s *Sim) schedulerEnabled() bool { return s.cfg.IntentCadenceMS > 0 }

// ScheduledLivePassConfig returns the recommended live-pass configuration with
// the opt-in sampled intent scheduler armed at cadenceMS milliseconds. The
// cadence must be a positive multiple of SubstepMS; NewSim/Resume validate it.
// There is deliberately NO default cadence: a caller must opt in explicitly.
func ScheduledLivePassConfig(cadenceMS int) Config {
	return Config{SubstepMS: SubstepMS, DecelMPS2: RollDecelMPS2, LivePass: true, IntentCadenceMS: cadenceMS}
}

// maybePublishIntents decides whether THIS substep republishes pursuit intents.
// It runs at the very start of a live substep, before any integration, so a
// publication always reads the consistent CURRENT (pre-step) world.
//
// Disabled: publish every substep (the exact native behaviour). Enabled: publish
// when the periodic lattice is due and/or a state-changing event from a prior
// completed step is pending. Only the periodic branch advances the lattice;
// event invalidation NEVER postpones or shifts the periodic deadline — it only
// adds a publication.
func (s *Sim) maybePublishIntents(stepStart int) {
	if !s.schedulerEnabled() {
		s.publishPursuitIntents()
		return
	}
	publish := false
	if stepStart >= s.nextDecisionMS {
		publish = true
		// Advance the stable lattice to the first deadline strictly after this
		// step. The lattice is fixed (t0, cadence, 2*cadence, ...); an
		// out-of-band event never moves it. This is an O(1) closed form (no
		// catch-up loop), so a jump in the clock cannot make a step O(clock).
		s.nextDecisionMS = (stepStart/s.cfg.IntentCadenceMS + 1) * s.cfg.IntentCadenceMS
	}
	if s.decisionDirty {
		publish = true
	}
	if !publish {
		return
	}
	s.publishPursuitIntents()
	s.samplePerception(stepStart)
	s.publishLocalPursuitIntents(stepStart)
	s.decisionDirty = false
	s.decisionPublishes++
	s.lastDecisionMS = stepStart
}

// noteDecisionEvent marks a state-changing ball event as a pending invalidation.
// It is O(1) (a single flag) and is called ONLY from the event emitters, so the
// scheduler never scans the growing historical event log each substep — the
// simple dirty flag is enough. It is inert while the scheduler is disabled, so
// the disabled JSON contract is unchanged. Release counts, so a release invoked
// between steps (including a dynamic SECOND release) invalidates the next step.
// Path is a pure position trace and never invalidates.
func (s *Sim) noteDecisionEvent(kind PassEventKind) {
	if !s.schedulerEnabled() {
		return
	}
	switch kind {
	case PassEventRelease, PassEventControl, PassEventDeflect, PassEventStop, PassEventOut:
		s.decisionDirty = true
	}
}

// NextDecisionMS returns the next periodic decision deadline (0 when the
// scheduler is disabled). It is the persisted lattice continuation.
func (s *Sim) NextDecisionMS() int { return s.nextDecisionMS }

// DecisionPublishCount returns how many times this run has published intents
// through the scheduler (evidence only; not part of the Resume contract).
func (s *Sim) DecisionPublishCount() int { return s.decisionPublishes }

// LastDecisionMS returns the pre-step clock of the most recent scheduler
// publication, or -1 if none yet (evidence only; not part of the Resume
// contract).
func (s *Sim) LastDecisionMS() int { return s.lastDecisionMS }
