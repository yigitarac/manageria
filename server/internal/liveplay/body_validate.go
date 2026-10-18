package liveplay

import (
	"fmt"
	"math"
	"sort"
)

// T-057 body-state import validation (mirrors the integrated import guard style).

// resumeBodyState validates and deep-copies the optional body state, or returns
// (nil, nil) when disabled. Persisted runner body params WITHOUT global body state
// are rejected as incomplete.
func resumeBodyState(snap Snapshot, runners []Runner) (*BodyContactState, error) {
	if snap.BodyContacts == nil {
		for i := range runners {
			if runners[i].Body != nil {
				return nil, fmt.Errorf("resuming: runner %d: %w", i, ErrBodyIncomplete)
			}
		}
		return nil, nil
	}
	if snap.Integrated == nil {
		return nil, fmt.Errorf("resuming: %w", ErrBodyNoIntegrated)
	}
	if snap.Integrated.ModelVersion != IntegratedModelVersionBody {
		return nil, fmt.Errorf("resuming: %w", ErrBodyVersion)
	}
	st := cloneBodyState(snap.BodyContacts)
	if err := validateBodyState(st, snap, runners); err != nil {
		return nil, fmt.Errorf("resuming body: %w", err)
	}
	return st, nil
}

// bodyOrder returns runner indices sorted by stable PlayerID.
func bodyOrder(runners []Runner) []int {
	order := make([]int, len(runners))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return runners[order[a]].PlayerID < runners[order[b]].PlayerID })
	return order
}

// validateBodyState rejects an inconsistent persisted body state: wrong schema,
// invalid config, a non-body-enabled runner or invalid body params, infeasible
// overlap, non-contiguous/ill-formed impacts (finite/chronology/geometry/algebra)
// and an unknown/ill-formed fault.
func validateBodyState(st *BodyContactState, snap Snapshot, runners []Runner) error {
	if st.SchemaVersion != BodyContactSchemaVersion {
		return fmt.Errorf("%w: schemaVersion %d", ErrBodySchema, st.SchemaVersion)
	}
	if err := st.Config.validate(); err != nil {
		return err
	}
	n := len(runners)
	if n < 1 || n > BodyMaxParticipants {
		return fmt.Errorf("%w: %d", ErrBodyRoster, n)
	}
	if _, err := runnerIDSet(runners); err != nil {
		return err
	}
	idx := map[uint64]int{}
	for i := range runners {
		if runners[i].Motion != MotionInertial {
			return fmt.Errorf("runner %d: %w", i, ErrBodyMotion)
		}
		if runners[i].Body == nil {
			return fmt.Errorf("runner %d: %w", i, ErrBodyMissing)
		}
		if err := runners[i].Body.validate(); err != nil {
			return fmt.Errorf("runner %d: %w", i, err)
		}
		idx[runners[i].PlayerID] = i
	}
	// Deterministic bounded-overlap feasibility on scratch (no live mutation).
	posX := make([]float64, n)
	posY := make([]float64, n)
	rad := make([]float64, n)
	invMass := make([]float64, n)
	for i := range runners {
		posX[i], posY[i] = runners[i].X, runners[i].Y
		rad[i] = runners[i].Body.RadiusM
		invMass[i] = 1 / runners[i].Body.MassKG
	}
	budget := make([]float64, n)
	if _, err := repairPositions(posX, posY, rad, invMass, bodyOrder(runners), budget, 4096, nil); err != nil {
		return fmt.Errorf("%w: %v", ErrBodyOverlap, err)
	}

	if st.NextSeq != len(st.Impacts) {
		return fmt.Errorf("%w: nextSeq %d != impacts %d", ErrBodySchema, st.NextSeq, len(st.Impacts))
	}
	sub := snap.Cfg.SubstepMS
	lastActual := math.Inf(-1)
	for i := range st.Impacts {
		im := st.Impacts[i]
		if im.SchemaVersion != BodyContactSchemaVersion {
			return fmt.Errorf("%w: impact %d schema %d", ErrBodySchema, i, im.SchemaVersion)
		}
		if im.Seq != i {
			return fmt.Errorf("%w: impact %d seq %d", ErrBodySchema, i, im.Seq)
		}
		if im.StepStartMS < 0 || sub <= 0 || im.StepStartMS%sub != 0 {
			return fmt.Errorf("%w: impact %d stepStart %d", ErrBodySchema, i, im.StepStartMS)
		}
		if im.ClockMS != im.StepStartMS+sub {
			return fmt.Errorf("%w: impact %d clock %d", ErrBodySchema, i, im.ClockMS)
		}
		if !isFinite(im.Fraction) || im.Fraction < 0 || im.Fraction > 1 {
			return fmt.Errorf("%w: impact %d fraction %v", ErrBodySchema, i, im.Fraction)
		}
		if im.ClockMS > snap.ClockMS {
			return fmt.Errorf("%w: impact %d clock %d after snapshot", ErrBodySchema, i, im.ClockMS)
		}
		actual := float64(im.StepStartMS) + im.Fraction*float64(sub)
		if actual < lastActual {
			return fmt.Errorf("%w: impact %d non-monotone time", ErrBodySchema, i)
		}
		lastActual = actual
		if im.PlayerID1 == 0 || im.PlayerID2 == 0 || im.PlayerID1 >= im.PlayerID2 {
			return fmt.Errorf("%w: impact %d ids %d/%d", ErrBodySchema, i, im.PlayerID1, im.PlayerID2)
		}
		i1, ok1 := idx[im.PlayerID1]
		i2, ok2 := idx[im.PlayerID2]
		if !ok1 || !ok2 {
			return fmt.Errorf("%w: impact %d unknown id", ErrBodySchema, i)
		}
		if !isFinite(im.NormalX) || !isFinite(im.NormalY) {
			return fmt.Errorf("%w: impact %d normal", ErrBodySchema, i)
		}
		if nn := im.NormalX*im.NormalX + im.NormalY*im.NormalY; nn < 1-1e-9 || nn > 1+1e-9 {
			return fmt.Errorf("%w: impact %d normal not unit (%v)", ErrBodySchema, i, nn)
		}
		if !isFinite(im.Impulse) || im.Impulse < 0 {
			return fmt.Errorf("%w: impact %d impulse %v", ErrBodySchema, i, im.Impulse)
		}
		for _, v := range []float64{im.X1, im.Y1, im.X2, im.Y2, im.InVX1, im.InVY1, im.InVX2, im.InVY2, im.OutVX1, im.OutVY1, im.OutVX2, im.OutVY2} {
			if !isFinite(v) {
				return fmt.Errorf("%w: impact %d non-finite field", ErrBodySchema, i)
			}
		}
		// Normal must point from PlayerID2 to PlayerID1 (declared contact geometry).
		gx := (im.X1 - im.X2) * PitchLengthM
		gy := (im.Y1 - im.Y2) * PitchWidthM
		gd := math.Sqrt(gx*gx + gy*gy)
		if gd > 1e-6 {
			if math.Abs(gx/gd-im.NormalX) > 1e-6 || math.Abs(gy/gd-im.NormalY) > 1e-6 {
				return fmt.Errorf("%w: impact %d normal vs geometry", ErrBodySchema, i)
			}
		}
		// Impulse algebra: J = -vn/(1/m1+1/m2); out = in +/- J/m n (ALL components).
		vn := (im.InVX1-im.InVX2)*im.NormalX + (im.InVY1-im.InVY2)*im.NormalY
		if !isFinite(vn) || !(vn < -BodyClosingTolMPS) {
			return fmt.Errorf("%w: impact %d not closing (vn=%v)", ErrBodySchema, i, vn)
		}
		want := -vn / (invMass[i1] + invMass[i2])
		if !isFinite(want) || want <= 0 {
			return fmt.Errorf("%w: impact %d non-positive impulse %.9f", ErrBodySchema, i, want)
		}
		if math.Abs(want-im.Impulse) > 1e-6*(1+math.Abs(want)) {
			return fmt.Errorf("%w: impact %d impulse algebra %.9f != %.9f", ErrBodySchema, i, im.Impulse, want)
		}
		out1x := im.InVX1 + im.Impulse*invMass[i1]*im.NormalX
		out1y := im.InVY1 + im.Impulse*invMass[i1]*im.NormalY
		out2x := im.InVX2 - im.Impulse*invMass[i2]*im.NormalX
		out2y := im.InVY2 - im.Impulse*invMass[i2]*im.NormalY
		for _, chk := range []struct {
			name      string
			got, want float64
		}{
			{"outVx1", im.OutVX1, out1x}, {"outVy1", im.OutVY1, out1y},
			{"outVx2", im.OutVX2, out2x}, {"outVy2", im.OutVY2, out2y},
		} {
			if math.Abs(chk.got-chk.want) > 1e-6*(1+math.Abs(chk.want)) {
				return fmt.Errorf("%w: impact %d %s algebra %.9f != %.9f", ErrBodySchema, i, chk.name, chk.got, chk.want)
			}
		}
		vnOut := (out1x-out2x)*im.NormalX + (out1y-out2y)*im.NormalY
		if math.Abs(vnOut) > 1e-6 {
			return fmt.Errorf("%w: impact %d restitution not zero (vnOut=%v)", ErrBodySchema, i, vnOut)
		}
		sumR := runners[i1].Body.RadiusM + runners[i2].Body.RadiusM
		if d := DistM(im.X1, im.Y1, im.X2, im.Y2); math.Abs(d-sumR) > 1e-6*(1+sumR) {
			return fmt.Errorf("%w: impact %d contact distance %.9f != radii %.9f", ErrBodySchema, i, d, sumR)
		}
	}
	if st.Fault != nil {
		if !validBodyFaultCode(st.Fault.Code) {
			return fmt.Errorf("%w: fault code %q", ErrBodySchema, st.Fault.Code)
		}
		if !isFinite(st.Fault.Fraction) || st.Fault.Fraction < 0 || st.Fault.Fraction > 1 {
			return fmt.Errorf("%w: fault fraction %v", ErrBodySchema, st.Fault.Fraction)
		}
		if st.Fault.StepStartMS != snap.ClockMS || (sub > 0 && st.Fault.StepStartMS%sub != 0) {
			return fmt.Errorf("%w: fault stepStart %d != snapshot clock %d", ErrBodySchema, st.Fault.StepStartMS, snap.ClockMS)
		}
	}
	return nil
}
