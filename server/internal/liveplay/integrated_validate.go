package liveplay

import (
	"fmt"
)

// T-055 integrated import validation (split out of integrated.go for cohesion).
//
// Everything here guards ONLY the opt-in integrated path. The nil-legacy import
// path is deliberately UNRESTRICTED (Resume calls this only when
// Snapshot.Integrated != nil), exactly as before T-055. The guards reject a
// malformed public snapshot: unknown versions, an invalid ball/motion/phase enum,
// a non-finite body/target/limit/reach, incoherent owner/index and contact/touch
// records, a non-contiguous or non-chronological source event log, a journal that
// is not an exact ordered one-to-one mapping of every factual source event, and a
// phase/last-touch that does not reconcile with that journal.

// validRunnerMotion reports whether m is a declared runner motion model.
func validRunnerMotion(m RunnerMotion) bool { return m == MotionKinematic || m == MotionInertial }

// validPassEventKind reports whether k is a declared authoritative event kind.
func validPassEventKind(k PassEventKind) bool { return k <= PassEventDeflect }

// ballFinite reports whether every ball scalar is finite.
func ballFinite(b Ball) bool {
	return isFinite(b.X) && isFinite(b.Y) && isFinite(b.VXM) && isFinite(b.VYM) &&
		isFinite(b.HoldDX) && isFinite(b.HoldDY)
}

// eventFinite reports whether every authoritative event scalar is finite.
func eventFinite(e PassEvent) bool {
	return isFinite(e.BallX) && isFinite(e.BallY) && isFinite(e.VXM) && isFinite(e.VYM) &&
		isFinite(e.OutVXM) && isFinite(e.OutVYM) && isFinite(e.Frac) &&
		isFinite(e.RunnerX) && isFinite(e.RunnerY)
}

// runnerIDSet builds the stable-ID membership set and rejects a zero or duplicate
// PlayerID. Membership only: it is never iterated for a decision.
func runnerIDSet(runners []Runner) (map[uint64]bool, error) {
	ids := make(map[uint64]bool, len(runners))
	for i := range runners {
		id := runners[i].PlayerID
		if id == 0 {
			return nil, fmt.Errorf("%w: runner %d has zero id", ErrIntegratedPlayerID, i)
		}
		if ids[id] {
			return nil, fmt.Errorf("%w: duplicate id %d", ErrIntegratedPlayerID, id)
		}
		ids[id] = true
	}
	return ids, nil
}

// validateBodyFinite rejects a non-finite body/target/limit/reach scalar, an
// undeclared motion model and a negative persisted deadline. Non-positive motion
// limits keep their documented "use the default envelope" meaning and are NOT
// rejected (only non-finite is).
func validateBodyFinite(r Runner) error {
	if !validRunnerMotion(r.Motion) {
		return fmt.Errorf("%w: motion %d", ErrIntegratedBody, r.Motion)
	}
	if !isFinite(r.X) || !isFinite(r.Y) || !isFinite(r.VXM) || !isFinite(r.VYM) ||
		!isFinite(r.IntentX) || !isFinite(r.IntentY) || !isFinite(r.ReachM) ||
		!isFinite(r.TopSpeedMPS) || !isFinite(r.AccelMPS2) || !isFinite(r.DecelMPS2) || !isFinite(r.LateralAccelMPS2) {
		return ErrIntegratedBody
	}
	if r.ReactMS < 0 || r.ReactAtMS < 0 || r.CooldownUntilMS < 0 {
		return fmt.Errorf("%w: negative runner deadline", ErrIntegratedJournal)
	}
	return nil
}

// validateControlError rejects a non-finite or out-of-[0,1] control budget. The
// ledger requires ControlError to be a finite intended budget in [0,1].
func validateControlError(v float64) error {
	if !isFinite(v) || v < 0 || v > 1 {
		return fmt.Errorf("%w: controlError %v outside [0,1]", ErrIntegratedBody, v)
	}
	return nil
}

// validateContactRecord checks a persisted contact/touch record: the sentinel
// (-1) must agree between time and index, and a real time must be inside
// [0,snapshotClock] with a valid runner index.
func validateContactRecord(ms, runner, clock, n int) error {
	if runner < -1 || runner >= n {
		return fmt.Errorf("%w: contact runner %d", ErrIntegratedIndex, runner)
	}
	if runner == -1 {
		if ms != -1 {
			return fmt.Errorf("%w: contact time %d without runner", ErrIntegratedIndex, ms)
		}
		return nil
	}
	if ms < 0 || ms > clock {
		return fmt.Errorf("%w: contact time %d outside [0,%d]", ErrIntegratedIndex, ms, clock)
	}
	return nil
}

// validateIntegratedContext validates the whole current context for enabling the
// shell: LivePass, configuration, ball/motion enums, finite bodies/ball/targets/
// limits/reach, unique IDs, coherent owner/index/contact state and non-negative
// absolute deadlines.
func (s *Sim) validateIntegratedContext() error {
	if err := s.cfg.validate(); err != nil {
		return err
	}
	if !s.cfg.LivePass {
		return ErrIntegratedNotLivePass
	}
	if !validBallMode(s.ball.Mode) {
		return fmt.Errorf("%w: ball mode %d", ErrIntegratedBody, s.ball.Mode)
	}
	if !ballFinite(s.ball) {
		return ErrIntegratedBody
	}
	if err := validateControlError(s.controlError); err != nil {
		return err
	}
	if s.immunityUntil < 0 || s.lastPathMS < 0 || s.nextDecisionMS < 0 {
		return fmt.Errorf("%w: negative absolute deadline", ErrIntegratedJournal)
	}
	n := len(s.runners)
	if _, err := runnerIDSet(s.runners); err != nil {
		return err
	}
	for i := range s.runners {
		r := s.runners[i]
		if err := validateBodyFinite(r); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
		// Validate the EXISTING awareness context at enable time exactly as Resume
		// does, so a validly-enabled state can always JSON Resume: local-pursuit,
		// local-wakeup and the perception record.
		if err := validateLocalPursuitContext(r, s.cfg); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
		if err := validateLocalWakeupContext(r, s.cfg); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
		if o := r.Perception; o != nil {
			if err := o.validate(s.clock, n); err != nil {
				return fmt.Errorf("runner %d: %w", i, err)
			}
		}
	}
	if s.contactRunner < -1 || s.contactRunner >= n {
		return fmt.Errorf("%w: contactRunner %d", ErrIntegratedIndex, s.contactRunner)
	}
	if s.firstTouchRunner < -1 || s.firstTouchRunner >= n {
		return fmt.Errorf("%w: firstTouchRunner %d", ErrIntegratedIndex, s.firstTouchRunner)
	}
	if s.releaseRunner < -1 || s.releaseRunner >= n {
		return fmt.Errorf("%w: releaseRunner %d", ErrIntegratedIndex, s.releaseRunner)
	}
	if s.ball.Mode == BallHeld && (s.contactRunner < 0 || s.contactRunner >= n) {
		return fmt.Errorf("%w: held ball without owner", ErrIntegratedIndex)
	}
	return nil
}

// validateIntegratedState is the import guard for the opt-in shell. It never
// coerces an unknown version and rejects a malformed public snapshot even when
// the journal and the source log were edited together consistently.
func validateIntegratedState(st *IntegratedState, snap Snapshot, runners []Runner) error {
	if st.SnapshotVersion != IntegratedSnapshotVersion {
		return fmt.Errorf("%w: snapshotVersion %d", ErrIntegratedVersion, st.SnapshotVersion)
	}
	if st.ModelVersion != IntegratedModelVersion && st.ModelVersion != IntegratedModelVersionOriented && st.ModelVersion != IntegratedModelVersionBody {
		return fmt.Errorf("%w: modelVersion %d", ErrIntegratedVersion, st.ModelVersion)
	}
	if st.EventSchemaVersion != IntegratedEventSchemaVersion {
		return fmt.Errorf("%w: eventSchemaVersion %d", ErrIntegratedVersion, st.EventSchemaVersion)
	}
	if st.RulebookVersion != IntegratedRulebookVersion {
		return fmt.Errorf("%w: rulebookVersion %q", ErrIntegratedVersion, st.RulebookVersion)
	}
	// The live-pass model is mandatory for the integrated shell.
	if !snap.Cfg.LivePass {
		return ErrIntegratedNotLivePass
	}
	if snap.Cfg.SubstepMS <= 0 || snap.ClockMS < 0 || snap.ClockMS%snap.Cfg.SubstepMS != 0 {
		return fmt.Errorf("%w: clock %d not on substep grid %d", ErrIntegratedJournal, snap.ClockMS, snap.Cfg.SubstepMS)
	}
	if !validPhase(st.Phase) {
		return fmt.Errorf("%w: phase %d", ErrIntegratedPhase, st.Phase)
	}
	if !validBallMode(snap.Ball.Mode) {
		return fmt.Errorf("%w: ball mode %d", ErrIntegratedBody, snap.Ball.Mode)
	}
	if !ballFinite(snap.Ball) {
		return ErrIntegratedBody
	}
	if err := validateControlError(snap.ControlError); err != nil {
		return err
	}
	n := len(runners)
	ids, err := runnerIDSet(runners)
	if err != nil {
		return err
	}
	if snap.ContactRunner < -1 || snap.ContactRunner >= n ||
		snap.FirstTouchRunner < -1 || snap.FirstTouchRunner >= n ||
		snap.ReleaseRunner < -1 || snap.ReleaseRunner >= n {
		return fmt.Errorf("%w: owner/index out of range", ErrIntegratedIndex)
	}
	if err := validateContactRecord(snap.ContactMS, snap.ContactRunner, snap.ClockMS, n); err != nil {
		return err
	}
	if err := validateContactRecord(snap.FirstTouchMS, snap.FirstTouchRunner, snap.ClockMS, n); err != nil {
		return err
	}
	if snap.ImmunityUntil < 0 || snap.LastPathMS < 0 || snap.NextDecisionMS < 0 {
		return fmt.Errorf("%w: negative absolute deadline", ErrIntegratedJournal)
	}
	if snap.Ball.Mode == BallHeld && (snap.ContactRunner < 0 || snap.ContactRunner >= n) {
		return fmt.Errorf("%w: held ball without owner", ErrIntegratedIndex)
	}
	for i := range runners {
		if err := validateBodyFinite(runners[i]); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
	}
	// Orientation is only valid on the integrated path (guaranteed here), on an
	// inertial body with a stable id, and requires the oriented model version.
	anyOriented := false
	for i := range runners {
		o := runners[i].Orientation
		if o == nil {
			continue
		}
		anyOriented = true
		if runners[i].Motion != MotionInertial {
			return fmt.Errorf("runner %d: %w", i, ErrOrientationMotion)
		}
		if runners[i].PlayerID == 0 {
			return fmt.Errorf("runner %d: %w", i, ErrOrientationPlayerID)
		}
		if err := o.validate(); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
	}
	if anyOriented && st.ModelVersion != IntegratedModelVersionOriented && st.ModelVersion != IntegratedModelVersionBody {
		return fmt.Errorf("%w: modelVersion %d with orientation", ErrOrientationModel, st.ModelVersion)
	}
	if err := validateIntegratedEvents(snap); err != nil {
		return err
	}
	if st.NextSeq != len(st.Journal) {
		return fmt.Errorf("%w: nextSeq %d != journal %d", ErrIntegratedJournal, st.NextSeq, len(st.Journal))
	}
	if st.LastTouchID != 0 && !ids[st.LastTouchID] {
		return fmt.Errorf("%w: lastTouch %d not a known player", ErrIntegratedTouch, st.LastTouchID)
	}
	// One-to-one ORDERED mapping: every factual source event has exactly one
	// journal fact, in the same emission order.
	factual := make([]PassEvent, 0, len(snap.Events))
	for _, e := range snap.Events {
		if _, ok := factKindOf(e.Kind); ok {
			factual = append(factual, e)
		}
	}
	if len(st.Journal) != len(factual) {
		return fmt.Errorf("%w: journal %d != factual events %d", ErrIntegratedJournal, len(st.Journal), len(factual))
	}
	phase, touch := PhaseDead, uint64(0)
	for i := range st.Journal {
		f := st.Journal[i]
		e := factual[i]
		if f.Seq != i {
			return fmt.Errorf("%w: fact %d has seq %d", ErrIntegratedJournal, i, f.Seq)
		}
		if f.SchemaVersion != IntegratedEventSchemaVersion {
			return fmt.Errorf("%w: fact %d schema %d", ErrIntegratedVersion, i, f.SchemaVersion)
		}
		if !validFactKind(f.Kind) {
			return fmt.Errorf("%w: fact %d kind %d", ErrIntegratedJournal, i, f.Kind)
		}
		if f.SourceSeq != e.Seq {
			return fmt.Errorf("%w: fact %d sourceSeq %d != event %d", ErrIntegratedSource, i, f.SourceSeq, e.Seq)
		}
		if f.PlayerID != 0 && !ids[f.PlayerID] {
			return fmt.Errorf("%w: fact %d player %d unknown", ErrIntegratedTouch, i, f.PlayerID)
		}
		if (f.Kind == FactControl || f.Kind == FactDeflect) && f.PlayerID == 0 {
			return fmt.Errorf("%w: fact %d %s without a stable actor", ErrIntegratedTouch, i, f.Kind)
		}
		if !phaseAllowsPrior(f.PhaseBefore, f.Kind) {
			return fmt.Errorf("%w: fact %d kind %s from phase %s", ErrIntegratedJournal, i, f.Kind, f.PhaseBefore)
		}
		if i > 0 && f.PhaseBefore != phase {
			return fmt.Errorf("%w: fact %d phaseBefore %s != %s", ErrIntegratedJournal, i, f.PhaseBefore, phase)
		}
		if err := checkFactAgainstSource(f, e, snap, runners); err != nil {
			return fmt.Errorf("fact %d: %w", i, err)
		}
		pa, ta := integrateFact(phase, touch, f.Kind, f.PlayerID)
		if f.PhaseAfter != pa {
			return fmt.Errorf("%w: fact %d phaseAfter %s != %s", ErrIntegratedJournal, i, f.PhaseAfter, pa)
		}
		phase, touch = pa, ta
	}
	if len(st.Journal) > 0 {
		if phase != st.Phase {
			return fmt.Errorf("%w: journal ends phase %s != %s", ErrIntegratedJournal, phase, st.Phase)
		}
		if touch != st.LastTouchID {
			return fmt.Errorf("%w: journal ends touch %d != %d", ErrIntegratedTouch, touch, st.LastTouchID)
		}
	} else if st.LastTouchID != 0 {
		return fmt.Errorf("%w: empty journal with lastTouch %d", ErrIntegratedTouch, st.LastTouchID)
	}
	// The current phase must be coherent with the current ball mode.
	want, ok := ballPhase(snap.Ball.Mode)
	if !ok || want != st.Phase {
		return fmt.Errorf("%w: phase %s incoherent with ball mode %s", ErrIntegratedPhase, st.Phase, snap.Ball.Mode)
	}
	return nil
}

// validateIntegratedEvents checks the persisted authoritative event log as an
// ACTUAL emission order: contiguous sequence with EventSeq == len, declared kinds,
// finite scalars, substep-grid clocks not after the snapshot clock, a valid
// generating step, kind-appropriate actor, physical fractions, and
// non-decreasing exact times (not any report category sort).
func validateIntegratedEvents(snap Snapshot) error {
	if snap.EventSeq != len(snap.Events) {
		return fmt.Errorf("%w: eventSeq %d != events %d", ErrIntegratedJournal, snap.EventSeq, len(snap.Events))
	}
	sub := snap.Cfg.SubstepMS
	n := len(snap.Runners)
	havePrev := false
	prevExact := 0.0
	for i, e := range snap.Events {
		if e.Seq != i {
			return fmt.Errorf("%w: event %d has seq %d", ErrIntegratedJournal, i, e.Seq)
		}
		if !validPassEventKind(e.Kind) {
			return fmt.Errorf("%w: event %d kind %d", ErrIntegratedJournal, i, e.Kind)
		}
		if !eventFinite(e) {
			return fmt.Errorf("%w: event %d has a non-finite scalar", ErrIntegratedBody, i)
		}
		if e.ClockMS < 0 || e.ClockMS%sub != 0 {
			return fmt.Errorf("%w: event %d clock %d off grid %d", ErrIntegratedJournal, i, e.ClockMS, sub)
		}
		if e.ClockMS > snap.ClockMS {
			return fmt.Errorf("%w: event %d clock %d after snapshot %d", ErrIntegratedJournal, i, e.ClockMS, snap.ClockMS)
		}
		if e.Frac < 0 || e.Frac > 1 {
			return fmt.Errorf("%w: event %d frac %v outside [0,1]", ErrIntegratedJournal, i, e.Frac)
		}
		if (e.Kind == PassEventRelease || e.Kind == PassEventPath) && e.Frac != 0 {
			return fmt.Errorf("%w: event %d %s frac %v, want 0", ErrIntegratedJournal, i, e.Kind, e.Frac)
		}
		stepStart := e.ClockMS - sub
		if e.Kind == PassEventRelease {
			stepStart = e.ClockMS
		}
		if stepStart < 0 {
			return fmt.Errorf("%w: event %d generating step %d", ErrIntegratedJournal, i, stepStart)
		}
		switch e.Kind {
		case PassEventRelease:
			if e.Runner < -1 || e.Runner >= n {
				return fmt.Errorf("%w: event %d release runner %d", ErrIntegratedIndex, i, e.Runner)
			}
		case PassEventPath, PassEventStop, PassEventOut:
			if e.Runner != -1 {
				return fmt.Errorf("%w: event %d %s runner %d, want -1", ErrIntegratedIndex, i, e.Kind, e.Runner)
			}
		default: // control/deflect
			if e.Runner < 0 || e.Runner >= n {
				return fmt.Errorf("%w: event %d %s runner %d", ErrIntegratedIndex, i, e.Kind, e.Runner)
			}
		}
		exact := eventExactTimeMS(e, sub)
		if havePrev && exact < prevExact {
			return fmt.Errorf("%w: event %d exact %.6f precedes %.6f", ErrIntegratedJournal, i, exact, prevExact)
		}
		prevExact, havePrev = exact, true
	}
	return nil
}

// checkFactAgainstSource verifies a fact against its persisted source PassEvent:
// kind, clock, exact ball position and vectors, fraction, participant identity
// and the declared step start.
func checkFactAgainstSource(f FactEvent, e PassEvent, snap Snapshot, runners []Runner) error {
	fk, ok := factKindOf(e.Kind)
	if !ok || fk != f.Kind {
		return fmt.Errorf("%w: kind %s vs source %s", ErrIntegratedSource, f.Kind, e.Kind)
	}
	if f.ClockMS != e.ClockMS || f.Fraction != e.Frac {
		return fmt.Errorf("%w: clock/fraction %d/%.12g vs %d/%.12g", ErrIntegratedSource, f.ClockMS, f.Fraction, e.ClockMS, e.Frac)
	}
	if f.X != e.BallX || f.Y != e.BallY || f.VXM != e.VXM || f.VYM != e.VYM || f.OutVXM != e.OutVXM || f.OutVYM != e.OutVYM {
		return fmt.Errorf("%w: ball state does not match source", ErrIntegratedSource)
	}
	if f.PlayerID != playerIDForRunner(runners, e.Runner) {
		return fmt.Errorf("%w: player %d vs source runner %d", ErrIntegratedSource, f.PlayerID, e.Runner)
	}
	wantStart := f.ClockMS - snap.Cfg.SubstepMS
	if f.Kind == FactRelease {
		wantStart = f.ClockMS
	}
	if f.StepStartMS != wantStart {
		return fmt.Errorf("%w: stepStart %d != %d", ErrIntegratedSource, f.StepStartMS, wantStart)
	}
	return nil
}
