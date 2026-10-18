package liveplay

import (
	"fmt"
	"math"
	"sort"
)

// T-057 body solver: stable pre-state separation, swept chronological hard-disc
// contacts with zero restitution and stable same-time full cohorts, bounded
// residual projection, a bounded work limit with a visible fault, and piecewise
// body paths (including bounded same-time repair knots) that feed the ball
// contact resolver.
//
// Positions are normalized pitch units; velocities are metres/second. Any advance
// therefore converts the metric velocity by /PitchLengthM or /PitchWidthM.

// bodyKnot is one knot of a runner's piecewise-linear trajectory within a step.
type bodyKnot struct {
	u float64 // global step fraction [0,1]
	x float64 // normalized pitch position
	y float64
}

// canonicalOrder returns runner indices sorted by stable PlayerID (ID-canonical
// pair iteration, never roster order).
func (s *Sim) canonicalOrder() []int {
	order := make([]int, len(s.runners))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return s.runners[order[a]].PlayerID < s.runners[order[b]].PlayerID
	})
	return order
}

// separationForces sums the soft personal-spacing acceleration for every pair
// within SpacingM, computed from the immutable PRE-state positions and in stable
// ID-canonical order. Coincident pairs use a deterministic stable ID axis.
func (s *Sim) separationForces() ([][2]float64, bool) {
	n := len(s.runners)
	sep := make([][2]float64, n)
	ds := s.body.Config.SpacingM
	k := s.body.Config.SoftAccelMPS2
	if !isFinite(ds) || ds <= 0 || !isFinite(k) || k < 0 {
		return nil, false
	}
	if k == 0 {
		return sep, true
	}
	order := s.canonicalOrder()
	for ai := range n {
		i := order[ai]
		for bi := ai + 1; bi < n; bi++ {
			j := order[bi]
			dx := (s.runners[i].X - s.runners[j].X) * PitchLengthM
			dy := (s.runners[i].Y - s.runners[j].Y) * PitchWidthM
			d2 := dx*dx + dy*dy
			if !isFinite(dx) || !isFinite(dy) || !isFinite(d2) {
				return nil, false
			}
			d := math.Sqrt(d2)
			if d > ds {
				continue
			}
			var nx, ny float64
			if d > bodyEpsM {
				nx, ny = dx/d, dy/d
			} else {
				nx, ny = stablePairAxis(s.runners[i].PlayerID, s.runners[j].PlayerID)
			}
			w := 1 - d/ds
			mag := k * w * w
			if !isFinite(mag) {
				return nil, false
			}
			sep[i][0] += mag * nx
			sep[i][1] += mag * ny
			sep[j][0] -= mag * nx
			sep[j][1] -= mag * ny
		}
	}
	for i := range sep {
		if !isFinite(sep[i][0]*sep[i][0] + sep[i][1]*sep[i][1]) {
			return nil, false
		}
	}
	return sep, true
}

// stablePairAxis returns a deterministic unit axis from an ID pair (coincidence).
func stablePairAxis(idLo, idHi uint64) (float64, float64) {
	if (idLo+idHi)&1 == 0 {
		return 1, 0
	}
	return 0, 1
}

// ballStepValid checks that the ball mode is valid, all ball scalars are finite,
// and its metric position, velocity and hold-offset squares are finite across all
// branches (rolling/loose/held/dead/out), independent of runner eligibility.
func ballStepValid(b Ball) bool {
	if !validBallMode(b.Mode) || !ballFinite(b) {
		return false
	}
	bxM := b.X * PitchLengthM
	byM := b.Y * PitchWidthM
	hxM := b.HoldDX * PitchLengthM
	hyM := b.HoldDY * PitchWidthM
	return isFinite(bxM*bxM+byM*byM) &&
		isFinite(b.VXM*b.VXM+b.VYM*b.VYM) &&
		isFinite(hxM*hxM+hyM*hyM)
}

// runnerStepValid checks that a body-enabled runner has finite state, body and
// optional orientation/perception scalars, and that its metric coordinates,
// velocity, intent delta, reach and motion envelope have finite squares/products.
func runnerStepValid(r Runner, dt float64) bool {
	if r.Motion != MotionInertial || r.PlayerID == 0 || r.Body == nil {
		return false
	}
	if r.Body.validate() != nil || validateBodyFinite(r) != nil {
		return false
	}
	if r.Orientation != nil && r.Orientation.validate() != nil {
		return false
	}
	if o := r.Perception; o != nil {
		if !isFinite(o.RadiusM) || o.RadiusM <= 0 || !isFinite(o.X) || !isFinite(o.Y) || !isFinite(o.VXM) || !isFinite(o.VYM) {
			return false
		}
		oxM, oyM := o.X*PitchLengthM, o.Y*PitchWidthM
		if !isFinite(oxM*oxM+oyM*oyM) || !isFinite(o.VXM*o.VXM+o.VYM*o.VYM) || !isFinite(o.RadiusM*o.RadiusM) {
			return false
		}
	}
	rxM := r.X * PitchLengthM
	ryM := r.Y * PitchWidthM
	ixM := r.IntentX * PitchLengthM
	iyM := r.IntentY * PitchWidthM
	dxM := (r.IntentX - r.X) * PitchLengthM
	dyM := (r.IntentY - r.Y) * PitchWidthM
	dist2 := dxM*dxM + dyM*dyM
	v2 := r.VXM*r.VXM + r.VYM*r.VYM
	if !isFinite(rxM*rxM+ryM*ryM) || !isFinite(ixM*ixM+iyM*iyM) || !isFinite(dist2) || !isFinite(v2) {
		return false
	}
	distM := math.Sqrt(dist2)
	p := r.Params()
	if !isFinite(p.TopSpeedMPS*p.TopSpeedMPS) || !isFinite(p.AccelMPS2*p.AccelMPS2) ||
		!isFinite(p.DecelMPS2*p.DecelMPS2) || !isFinite(p.LateralAccelMPS2*p.LateralAccelMPS2) ||
		!isFinite(2*p.DecelMPS2*distM) || !isFinite(distM/dt) {
		return false
	}
	reach := r.ReachM
	if reach <= 0 {
		reach = ControlRadiusM
	}
	if !isFinite(reach) || !isFinite(reach*reach) {
		return false
	}
	return true
}

// bodyStepStateValid validates the complete post-step trial state across all
// branches without wholesale JSON serialization or extra slice allocations.
func (s *Sim) bodyStepStateValid(stepStart int, dt float64, evStart, jStart, impStart int, record bool) bool {
	if s.clock != stepStart+s.cfg.SubstepMS {
		return false
	}
	if validateControlError(s.controlError) != nil || !ballStepValid(s.ball) {
		return false
	}
	n := len(s.runners)
	if n < 1 || n > BodyMaxParticipants {
		return false
	}
	if s.contactRunner < -1 || s.contactRunner >= n ||
		s.firstTouchRunner < -1 || s.firstTouchRunner >= n ||
		s.releaseRunner < -1 || s.releaseRunner >= n {
		return false
	}
	if s.ball.Mode == BallHeld && (s.contactRunner < 0 || s.contactRunner >= n) {
		return false
	}
	for i := range s.runners {
		if !runnerStepValid(s.runners[i], dt) {
			return false
		}
	}
	for i := evStart; i < len(s.events); i++ {
		e := s.events[i]
		if !eventFinite(e) || e.Frac < 0 || e.Frac > 1 {
			return false
		}
	}
	if s.integrated != nil {
		if !validPhase(s.integrated.Phase) {
			return false
		}
		for i := jStart; i < len(s.integrated.Journal); i++ {
			f := s.integrated.Journal[i]
			if !isFinite(f.Fraction) || f.Fraction < 0 || f.Fraction > 1 ||
				!isFinite(f.X) || !isFinite(f.Y) || !isFinite(f.VXM) || !isFinite(f.VYM) ||
				!isFinite(f.OutVXM) || !isFinite(f.OutVYM) {
				return false
			}
		}
	}
	for i := impStart; i < len(s.body.Impacts); i++ {
		im := s.body.Impacts[i]
		if !isFinite(im.Fraction) || im.Fraction < 0 || im.Fraction > 1 ||
			!isFinite(im.NormalX) || !isFinite(im.NormalY) || !isFinite(im.Impulse) || im.Impulse <= 0 ||
			!isFinite(im.X1) || !isFinite(im.Y1) || !isFinite(im.X2) || !isFinite(im.Y2) ||
			!isFinite(im.InVX1) || !isFinite(im.InVY1) || !isFinite(im.InVX2) || !isFinite(im.InVY2) ||
			!isFinite(im.OutVX1) || !isFinite(im.OutVY1) || !isFinite(im.OutVX2) || !isFinite(im.OutVY2) {
			return false
		}
	}
	if record {
		if len(s.samples) == 0 {
			return false
		}
		smp := s.samples[len(s.samples)-1]
		if !isFinite(smp.BallX) || !isFinite(smp.BallY) || !validBallMode(smp.Mode) {
			return false
		}
	}
	if s.provenance != nil {
		for i := range s.provenance.records {
			rec := s.provenance.records[i]
			if !isFinite(rec.Observation.X) || !isFinite(rec.Observation.Y) ||
				!isFinite(rec.Observation.VXM) || !isFinite(rec.Observation.VYM) ||
				!isFinite(rec.TargetX) || !isFinite(rec.TargetY) ||
				!isFinite(rec.OwnX) || !isFinite(rec.OwnY) {
				return false
			}
		}
	}
	return true
}

// stepBody is the body-enabled transactional step on a trial Sim. It mutates
// freely; the caller discards it on any returned error.
func (s *Sim) stepBody(record bool) error {
	stepStart := s.clock
	if s.cfg.SubstepMS <= 0 || !isFinite(s.cfg.DecelMPS2) || s.cfg.DecelMPS2 < 0 {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	dt := float64(s.cfg.SubstepMS) / 1000.0
	if !isFinite(dt) || dt <= 0 {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	if len(s.runners) < 1 || len(s.runners) > BodyMaxParticipants {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	if err := validateControlError(s.controlError); err != nil {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	if !ballStepValid(s.ball) {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	if s.ball.Mode == BallHeld && (s.contactRunner < 0 || s.contactRunner >= len(s.runners)) {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	for i := range s.runners {
		if !runnerStepValid(s.runners[i], dt) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
	}
	evStart := len(s.events)
	jStart := 0
	if s.integrated != nil {
		jStart = len(s.integrated.Journal)
	}
	impStart := len(s.body.Impacts)

	s.capturePreStep()
	s.maybePublishIntents(stepStart)
	for i := range s.runners {
		if !runnerStepValid(s.runners[i], dt) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
	}

	stopped := false
	if s.ball.Mode == BallRolling {
		speedSq := s.ball.VXM*s.ball.VXM + s.ball.VYM*s.ball.VYM
		speed := math.Sqrt(speedSq)
		if !isFinite(speedSq) || !isFinite(speed) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
		newSpeed := speed - s.cfg.DecelMPS2*dt
		if !isFinite(newSpeed) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
		if newSpeed <= 0 {
			s.ball.VXM, s.ball.VYM = 0, 0
			stopped = true
		} else {
			scale := newSpeed / speed
			if !isFinite(scale) {
				return s.setFault(BodyFaultNumerical, stepStart, 0)
			}
			s.ball.VXM *= scale
			s.ball.VYM *= scale
		}
		s.ball.X += (s.ball.VXM / PitchLengthM) * dt
		s.ball.Y += (s.ball.VYM / PitchWidthM) * dt
	}
	if !ballStepValid(s.ball) {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}

	sep, ok := s.separationForces()
	if !ok {
		return s.setFault(BodyFaultNumerical, stepStart, 0)
	}
	for i := range s.runners {
		if !isFinite((sep[i][0]*dt)*(sep[i][0]*dt) + (sep[i][1]*dt)*(sep[i][1]*dt)) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
		s.runners[i].steerBody(dt, sep[i][0], sep[i][1])
		if !runnerStepValid(s.runners[i], dt) {
			return s.setFault(BodyFaultNumerical, stepStart, 0)
		}
	}

	paths, err := s.solveBodySweep(stepStart, dt)
	if err != nil {
		return err
	}

	s.clock += s.cfg.SubstepMS

	if s.ball.Mode == BallRolling || s.ball.Mode == BallLoose {
		if err := s.resolveFreeBallPaths(stepStart, dt, stopped, paths); err != nil {
			return err
		}
	}
	if s.ball.Mode == BallHeld {
		if s.contactRunner < 0 || s.contactRunner >= len(s.runners) {
			return s.setFault(BodyFaultNumerical, stepStart, 1)
		}
		r := s.runners[s.contactRunner]
		s.ball.X = r.X + s.ball.HoldDX
		s.ball.Y = r.Y + s.ball.HoldDY
	}
	if s.ball.Mode == BallRolling && s.clock-s.lastPathMS >= PathRecordMS {
		s.emitEvent(PassEventPath, -1, s.clock, s.ball)
		s.lastPathMS = s.clock
	}
	if record {
		s.recordSample()
	}
	if !s.bodyStepStateValid(stepStart, dt, evStart, jStart, impStart, record) {
		return s.setFault(BodyFaultNumerical, stepStart, 1)
	}
	return nil
}

// repairPositions runs deterministic bounded inverse-mass pair passes over the
// supplied (scratch) positions. It returns whether anything moved. It reports a
// gross-overlap, projection-budget or convergence error otherwise. `count`, when
// non-nil, is invoked per effective projection (work accounting). `budget`, when
// non-nil, enforces the per-body total projection budget.
func repairPositions(posX, posY, rad, invMass []float64, order []int, budget []float64, maxPasses int, count func() error) (bool, error) {
	n := len(posX)
	movedAny := false
	for pass := 0; pass < maxPasses; pass++ {
		moved := false
		for ai := 0; ai < n; ai++ {
			for bi := ai + 1; bi < n; bi++ {
				a, b := order[ai], order[bi]
				dx := (posX[a] - posX[b]) * PitchLengthM
				dy := (posY[a] - posY[b]) * PitchWidthM
				d := math.Sqrt(dx*dx + dy*dy)
				sum := rad[a] + rad[b]
				overlap := sum - d
				if !isFinite(dx) || !isFinite(dy) || !isFinite(d) || !isFinite(sum) || !isFinite(overlap) {
					return movedAny, ErrBodyProjection
				}
				if !(overlap > BodyResidualTolM) {
					continue
				}
				if overlap > BodyMaxProjectionM+BodyResidualTolM {
					return movedAny, ErrBodyOverlap
				}
				if count != nil {
					if err := count(); err != nil {
						return movedAny, err
					}
				}
				var nx, ny float64
				if d > bodyEpsM {
					nx, ny = dx/d, dy/d
				} else {
					nx, ny = stablePairAxis(0, 1)
				}
				wa, wb := invMass[a], invMass[b]
				tot := wa + wb
				if !isFinite(tot) || tot <= 0 {
					return movedAny, ErrBodyProjection
				}
				moveA := overlap * wa / tot
				moveB := overlap * wb / tot
				if budget != nil {
					if budget[a]+moveA > BodyMaxProjectionM || budget[b]+moveB > BodyMaxProjectionM {
						return movedAny, ErrBodyProjection
					}
					budget[a] += moveA
					budget[b] += moveB
				}
				posX[a] += moveA * nx / PitchLengthM
				posY[a] += moveA * ny / PitchWidthM
				posX[b] -= moveB * nx / PitchLengthM
				posY[b] -= moveB * ny / PitchWidthM
				moved = true
				movedAny = true
			}
		}
		if !moved {
			return movedAny, nil
		}
	}
	return movedAny, ErrBodyProjection
}

// bodyEnableFeasible validates the deterministic bounded-overlap repair for the
// current bodies WITHOUT mutating live state (no RNG, no facts).
func (s *Sim) bodyEnableFeasible() error {
	n := len(s.runners)
	posX := make([]float64, n)
	posY := make([]float64, n)
	rad := make([]float64, n)
	invMass := make([]float64, n)
	for i := range s.runners {
		posX[i], posY[i] = s.runners[i].X, s.runners[i].Y
		rad[i] = s.runners[i].Body.RadiusM
		invMass[i] = 1 / s.runners[i].Body.MassKG
	}
	order := s.canonicalOrder()
	budget := make([]float64, n)
	if _, err := repairPositions(posX, posY, rad, invMass, order, budget, 4096, nil); err != nil {
		return fmt.Errorf("%w: %v", ErrBodyOverlap, err)
	}
	return nil
}

// mapRepairErr converts a repair error into a structured body fault.
func (s *Sim) mapRepairErr(err error, stepStart int, frac float64) error {
	if err == ErrBodyOverlap {
		return s.setFault(BodyFaultProjectionLimit, stepStart, frac)
	}
	if err == ErrBodyProjection {
		return s.setFault(BodyFaultProjectionLimit, stepStart, frac)
	}
	// A count() error already stored a work-limit fault.
	return err
}

// solveBodySweep advances every body from its pre-state under the steering-updated
// velocity, resolving swept hard-disc contacts chronologically with zero
// restitution, stable same-time full cohorts and bounded residual projection
// (recorded as same-time correction knots). It writes final positions/velocities
// back to the runners and returns each runner's piecewise path.
func (s *Sim) solveBodySweep(stepStart int, dt float64) ([][]bodyKnot, error) {
	n := len(s.runners)
	posX := make([]float64, n)
	posY := make([]float64, n)
	velX := make([]float64, n)
	velY := make([]float64, n)
	rad := make([]float64, n)
	invMass := make([]float64, n)
	for i := range s.runners {
		posX[i], posY[i] = s.runners[i].X, s.runners[i].Y
		velX[i], velY[i] = s.runners[i].VXM, s.runners[i].VYM
		rad[i] = s.runners[i].Body.RadiusM
		invMass[i] = 1 / s.runners[i].Body.MassKG
		if !isFinite(posX[i]) || !isFinite(posY[i]) || !isFinite(velX[i]) || !isFinite(velY[i]) ||
			!isFinite(rad[i]) || !isFinite(invMass[i]) {
			return nil, s.setFault(BodyFaultNumerical, stepStart, 0)
		}
	}
	order := s.canonicalOrder()
	projBudget := make([]float64, n)
	paths := make([][]bodyKnot, n)
	for i := range paths {
		paths[i] = []bodyKnot{{u: 0, x: posX[i], y: posY[i]}}
	}
	appendKnots := func(t float64) {
		u := t / dt
		for i := range s.runners {
			paths[i] = append(paths[i], bodyKnot{u: u, x: posX[i], y: posY[i]})
		}
	}
	res := 0
	maxRes := s.body.Config.MaxResolutions
	tick := func(t float64) error {
		res++
		if res > maxRes {
			return s.setFault(BodyFaultWorkLimit, stepStart, clamp01(t/dt))
		}
		return nil
	}

	// Initial residual repair, recorded as a same-time correction pair.
	if moved, err := repairPositions(posX, posY, rad, invMass, order, projBudget, 4096, func() error { return tick(0) }); err != nil {
		return nil, s.mapRepairErr(err, stepStart, 0)
	} else if moved {
		appendKnots(0)
	}

	t := 0.0
	outer := 0
	for {
		outer++
		if outer > maxRes+16 {
			return nil, s.setFault(BodyFaultWorkLimit, stepStart, clamp01(t/dt))
		}
		// Full stable same-time cohort passes until no change.
		for {
			changed := false
			for ai := range n {
				for bi := ai + 1; bi < n; bi++ {
					a, b := order[ai], order[bi]
					dx := (posX[a] - posX[b]) * PitchLengthM
					dy := (posY[a] - posY[b]) * PitchWidthM
					d2 := dx*dx + dy*dy
					if !isFinite(dx) || !isFinite(dy) || !isFinite(d2) {
						return nil, s.setFault(BodyFaultNumerical, stepStart, clamp01(t/dt))
					}
					d := math.Sqrt(d2)
					sum := rad[a] + rad[b]
					if !(d <= sum+bodyContactTolM) {
						continue
					}
					var nx, ny float64
					if d > bodyEpsM {
						nx, ny = dx/d, dy/d
					} else {
						nx, ny = stablePairAxis(s.runners[a].PlayerID, s.runners[b].PlayerID)
					}
					vn := (velX[a]-velX[b])*nx + (velY[a]-velY[b])*ny
					if !isFinite(vn) {
						return nil, s.setFault(BodyFaultNumerical, stepStart, clamp01(t/dt))
					}
					if !(vn < -BodyClosingTolMPS) {
						continue
					}
					if err := tick(t); err != nil {
						return nil, err
					}
					inVX1, inVY1 := velX[a], velY[a]
					inVX2, inVY2 := velX[b], velY[b]
					J := -vn / (invMass[a] + invMass[b])
					if !isFinite(J) {
						return nil, s.setFault(BodyFaultNumerical, stepStart, clamp01(t/dt))
					}
					velX[a] += J * invMass[a] * nx
					velY[a] += J * invMass[a] * ny
					velX[b] -= J * invMass[b] * nx
					velY[b] -= J * invMass[b] * ny
					s.appendImpact(stepStart, t, dt, a, b, nx, ny, J, posX[a], posY[a], posX[b], posY[b],
						inVX1, inVY1, inVX2, inVY2, velX[a], velY[a], velX[b], velY[b])
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		frac := clamp01(t / dt)
		if moved, err := repairPositions(posX, posY, rad, invMass, order, projBudget, 4096, func() error { return tick(t) }); err != nil {
			return nil, s.mapRepairErr(err, stepStart, frac)
		} else if moved {
			appendKnots(t)
		}
		// Earliest strictly-future entry (endpoint-inclusive).
		bestT := math.Inf(1)
		for ai := 0; ai < n; ai++ {
			for bi := ai + 1; bi < n; bi++ {
				a, b := order[ai], order[bi]
				tau, ok, bad := pairEntry(posX[a], posY[a], velX[a], velY[a], posX[b], posY[b], velX[b], velY[b], rad[a]+rad[b], dt-t)
				if bad {
					return nil, s.setFault(BodyFaultNumerical, stepStart, frac)
				}
				if ok && tau < bestT {
					bestT = tau
				}
			}
		}
		if math.IsInf(bestT, 1) {
			break
		}
		if t+bestT == t {
			return nil, s.setFault(BodyFaultNumerical, stepStart, frac)
		}
		for i := range s.runners {
			posX[i] += (velX[i] / PitchLengthM) * bestT
			posY[i] += (velY[i] / PitchWidthM) * bestT
			if !isFinite(posX[i]) || !isFinite(posY[i]) {
				return nil, s.setFault(BodyFaultNumerical, stepStart, frac)
			}
		}
		t += bestT
		appendKnots(t)
	}
	if t < dt {
		adv := dt - t
		for i := range s.runners {
			posX[i] += (velX[i] / PitchLengthM) * adv
			posY[i] += (velY[i] / PitchWidthM) * adv
		}
		appendKnots(dt)
	}
	for i := range s.runners {
		if !isFinite(posX[i]) || !isFinite(posY[i]) || !isFinite(velX[i]) || !isFinite(velY[i]) {
			return nil, s.setFault(BodyFaultNumerical, stepStart, 1)
		}
		s.runners[i].X, s.runners[i].Y = posX[i], posY[i]
		s.runners[i].VXM, s.runners[i].VYM = velX[i], velY[i]
	}
	return paths, nil
}

// pairEntry returns the earliest strictly-positive time tau (<= maxT, endpoint
// inclusive within bodyRootTolS) at which two discs enter contact while closing,
// using a numerically stable entry root. bad reports non-finite arithmetic.
func pairEntry(ax, ay, avx, avy, bx, by, bvx, bvy, sum, maxT float64) (tau float64, ok bool, bad bool) {
	if maxT <= 0 {
		return 0, false, false
	}
	rx := (ax - bx) * PitchLengthM
	ry := (ay - by) * PitchWidthM
	vx := avx - bvx
	vy := avy - bvy
	d2 := rx*rx + ry*ry
	if !isFinite(rx) || !isFinite(ry) || !isFinite(d2) || !isFinite(vx) || !isFinite(vy) || !isFinite(sum) || !isFinite(maxT) {
		return 0, false, true
	}
	d := math.Sqrt(d2)
	if d <= sum+bodyContactTolM {
		return 0, false, false // touching/overlapping: cohort/projection handles it
	}
	gap := d - sum
	c := gap * (d + sum)
	dot := rx*vx + ry*vy
	bq := 2 * dot
	if !isFinite(c) || !isFinite(bq) {
		return 0, false, true
	}
	if c <= 0 {
		return 0, false, false
	}
	nx, ny := rx/d, ry/d
	vn0 := vx*nx + vy*ny
	if !isFinite(vn0) {
		return 0, false, true
	}
	if !(vn0 < -BodyClosingTolMPS) {
		return 0, false, false
	}
	a := vx*vx + vy*vy
	if !isFinite(a) {
		return 0, false, true
	}
	if !(a > BodyClosingTolMPS*BodyClosingTolMPS) {
		return 0, false, false
	}
	disc := bq*bq - 4*a*c
	if !isFinite(disc) {
		return 0, false, true
	}
	if disc < 0 {
		return 0, false, false
	}
	sq := math.Sqrt(disc)
	// Normal closing speed at contact is -sq/(2*sum) (m/s). Reject tangent or
	// grazing roots that do not close faster than BodyClosingTolMPS at impact.
	if sq <= 2*sum*BodyClosingTolMPS {
		return 0, false, false
	}
	tau = 2 * c / (-bq + sq) // stable entry root (c>0, bq<0)
	if !isFinite(tau) {
		return 0, false, true
	}
	if tau <= 0 || tau > maxT+bodyRootTolS {
		return 0, false, false
	}
	if tau > maxT || maxT-tau <= bodyRootTolS {
		tau = maxT // snap a contact at/near the endpoint exactly onto the boundary
	}
	return tau, true, false
}

// setFault stores a structured fault on the body state and returns its error.
func (s *Sim) setFault(code BodyFaultCode, stepStart int, frac float64) error {
	if !isFinite(frac) || frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	f := &BodyStepFault{Code: code, StepStartMS: stepStart, Fraction: frac}
	s.body.Fault = f
	return bodyFaultError(f)
}

// appendImpact records one factual hard-body impulse.
func (s *Sim) appendImpact(stepStart int, t, dt float64, a, b int, nx, ny, J,
	x1, y1, x2, y2, inVX1, inVY1, inVX2, inVY2, outVX1, outVY1, outVX2, outVY2 float64) {
	s.body.Impacts = append(s.body.Impacts, BodyImpact{
		SchemaVersion: BodyContactSchemaVersion,
		Seq:           s.body.NextSeq,
		StepStartMS:   stepStart,
		ClockMS:       stepStart + s.cfg.SubstepMS,
		Fraction:      clamp01(t / dt),
		PlayerID1:     s.runners[a].PlayerID,
		PlayerID2:     s.runners[b].PlayerID,
		NormalX:       nx,
		NormalY:       ny,
		Impulse:       J,
		X1:            x1,
		Y1:            y1,
		X2:            x2,
		Y2:            y2,
		InVX1:         inVX1,
		InVY1:         inVY1,
		InVX2:         inVX2,
		InVY2:         inVY2,
		OutVX1:        outVX1,
		OutVY1:        outVY1,
		OutVX2:        outVX2,
		OutVY2:        outVY2,
	})
	s.body.NextSeq++
}

// pathPoint returns the piecewise body position at global fraction u using
// RIGHT-HAND semantics: at a fraction with same-time correction knots it returns
// the LAST knot at that fraction (the post-correction position), never an
// earlier pre-correction knot.
func pathPoint(path []bodyKnot, u float64) (float64, float64) {
	if len(path) == 0 {
		return 0, 0
	}
	last := -1
	for k := range path {
		if path[k].u <= u {
			last = k
		} else {
			break
		}
	}
	if last < 0 {
		return path[0].x, path[0].y
	}
	if path[last].u == u || last == len(path)-1 {
		return path[last].x, path[last].y
	}
	a, b := path[last], path[last+1]
	if b.u == a.u {
		return b.x, b.y
	}
	f := (u - a.u) / (b.u - a.u)
	return a.x + f*(b.x-a.x), a.y + f*(b.y-a.y)
}

// pathContact returns the earliest global fraction u in [uMin,1] at which the ball
// segment (from (px,py)@uCur to (ex,ey)@1) enters reach of the body's piecewise
// path, using the correct global origins and a stable entry root, and verifying
// every candidate against the RIGHT-HAND body position (so a same-time correction
// never yields a false contact). bad reports non-finite arithmetic. An exact u==1
// endpoint contact is eligible.
func pathContact(path []bodyKnot, px, py, uCur, ex, ey, reach, uMin float64) (float64, bool, bool) {
	if uMin > 1 {
		return 0, false, false
	}
	badVerify := false
	verify := func(u float64) bool {
		bx, by := pathPoint(path, u)
		sx, sy := segmentPoint(px, py, uCur, ex, ey, u)
		d := DistM(sx, sy, bx, by)
		if !isFinite(d) {
			badVerify = true
			return false
		}
		return d <= reach+bodyContactTolM
	}
	best := math.Inf(1)
	consider := func(u float64) {
		if u < uMin || u > 1 {
			return
		}
		if !verify(u) {
			return
		}
		if u < best {
			best = u
		}
	}
	if uCur < 1 {
		rateBX := (ex - px) * PitchLengthM / (1 - uCur)
		rateBY := (ey - py) * PitchWidthM / (1 - uCur)
		if !isFinite(rateBX) || !isFinite(rateBY) || !isFinite(rateBX*rateBX+rateBY*rateBY) {
			return 0, false, true
		}
		for k := 1; k < len(path); k++ {
			a, b := path[k-1], path[k]
			if b.u <= a.u {
				continue // zero-length or same-time correction jump: not a linear interval
			}
			lo := math.Max(uMin, a.u)
			hi := b.u
			if hi < lo {
				continue
			}
			rateSX := (b.x - a.x) * PitchLengthM / (b.u - a.u)
			rateSY := (b.y - a.y) * PitchWidthM / (b.u - a.u)
			cx := px*PitchLengthM - rateBX*uCur - (a.x*PitchLengthM - rateSX*a.u)
			cy := py*PitchWidthM - rateBY*uCur - (a.y*PitchWidthM - rateSY*a.u)
			dx := rateBX - rateSX
			dy := rateBY - rateSY
			if !isFinite(cx) || !isFinite(cy) || !isFinite(dx) || !isFinite(dy) {
				return 0, false, true
			}
			qx := cx + dx*lo
			qy := cy + dy*lo
			q2 := qx*qx + qy*qy
			r2 := reach * reach
			if !isFinite(q2) || !isFinite(r2) {
				return 0, false, true
			}
			if q2 <= r2 {
				consider(lo)
				if badVerify {
					return 0, false, true
				}
				continue
			}
			A := dx*dx + dy*dy
			B := 2 * (cx*dx + cy*dy)
			C := cx*cx + cy*cy - r2
			if !isFinite(A) || !isFinite(B) || !isFinite(C) {
				return 0, false, true
			}
			if A == 0 {
				continue
			}
			if B >= 0 {
				continue // receding or stationary: no future entry
			}
			disc := B*B - 4*A*C
			if !isFinite(disc) {
				return 0, false, true
			}
			if disc < 0 {
				continue
			}
			sq := math.Sqrt(disc)
			entry := 2 * C / (-B + sq) // stable smaller root (C>0, B<0)
			if !isFinite(entry) {
				return 0, false, true
			}
			if entry < lo || entry > hi {
				continue
			}
			consider(entry)
			if badVerify {
				return 0, false, true
			}
		}
	}
	if math.IsInf(best, 1) {
		consider(1) // right-hand endpoint eligibility
		if badVerify {
			return 0, false, true
		}
	}
	if math.IsInf(best, 1) {
		return 0, false, false
	}
	return best, true, false
}

// resolveFreeBallPaths mirrors resolveFreeBall but searches each runner's actual
// piecewise body path. Reaction/cooldown/passer-immunity gates and the stable-ID
// tie are preserved. It returns a structured numerical fault if the ball/body
// path geometry is non-finite.
func (s *Sim) resolveFreeBallPaths(stepStart int, dt float64, stopped bool, paths [][]bodyKnot) error {
	px, py := s.preBall.X, s.preBall.Y
	ex, ey := s.ball.X, s.ball.Y
	vx, vy := s.ball.VXM, s.ball.VYM
	uCur := 0.0
	firstSegment := true
	segCount := 0

	for {
		segCount++
		if segCount > len(s.runners)+4 {
			return s.setFault(BodyFaultWorkLimit, stepStart, clamp01(uCur))
		}
		if !isFinite(px) || !isFinite(py) || !isFinite(ex) || !isFinite(ey) ||
			!isFinite(vx) || !isFinite(vy) || !isFinite(uCur) ||
			!isFinite(vx*vx+vy*vy) ||
			!isFinite((px*PitchLengthM)*(px*PitchLengthM)+(py*PitchWidthM)*(py*PitchWidthM)) ||
			!isFinite((ex*PitchLengthM)*(ex*PitchLengthM)+(ey*PitchWidthM)*(ey*PitchWidthM)) {
			return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
		}
		if uCur < 1 {
			rateBX := (ex - px) * PitchLengthM / (1 - uCur)
			rateBY := (ey - py) * PitchWidthM / (1 - uCur)
			if !isFinite(rateBX) || !isFinite(rateBY) || !isFinite(rateBX*rateBX+rateBY*rateBY) {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
		}
		bestKind, bestU, bestRunner := evNone, 2.0, -1
		if u, ok := segmentExit(px, py, ex, ey, uCur); ok && u < bestU {
			if !isFinite(u) {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
			bestKind, bestU = evOut, u
		}
		for i := range s.runners {
			r := &s.runners[i]
			reach := r.ReachM
			if reach <= 0 {
				reach = ControlRadiusM
			}
			uMin := uCur
			if r.ReactAtMS > stepStart {
				if c := float64(r.ReactAtMS-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if r.CooldownUntilMS > stepStart {
				if c := float64(r.CooldownUntilMS-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if i == s.releaseRunner && s.immunityUntil > stepStart {
				if c := float64(s.immunityUntil-stepStart) / float64(s.cfg.SubstepMS); c > uMin {
					uMin = c
				}
			}
			if !isFinite(reach) || !isFinite(reach*reach) || !isFinite(uMin) {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
			if uMin > 1 {
				continue
			}
			u, ok, bad := pathContact(paths[i], px, py, uCur, ex, ey, reach, uMin)
			if bad {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
			if !ok {
				continue
			}
			if u < bestU || (u == bestU && bestKind != evContact) {
				bestKind, bestU, bestRunner = evContact, u, i
			} else if s.integrated != nil && u == bestU && bestKind == evContact {
				if r.PlayerID < s.runners[bestRunner].PlayerID {
					bestRunner = i
				}
			}
		}
		if stopped && firstSegment && bestKind == evNone {
			bestKind, bestU = evStop, 1.0
		}

		switch bestKind {
		case evNone:
			s.ball.X, s.ball.Y = ex, ey
			s.ball.VXM, s.ball.VYM = vx, vy
			return nil
		case evOut:
			bx, by := segmentPoint(px, py, uCur, ex, ey, bestU)
			if !isFinite(bestU) || !isFinite(bx) || !isFinite(by) {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
			s.ball.X, s.ball.Y = bx, by
			s.ball.Mode = BallOut
			s.ball.VXM, s.ball.VYM = 0, 0
			s.emitEventAt(PassEventOut, -1, s.clock, bestU, s.ball)
			return nil
		case evStop:
			s.ball.Mode = BallLoose
			s.emitEventAt(PassEventStop, -1, s.clock, bestU, s.ball)
			return nil
		case evContact:
			bx, by := segmentPoint(px, py, uCur, ex, ey, bestU)
			rxc, ryc := pathPoint(paths[bestRunner], bestU)
			if !isFinite(bestU) || !isFinite(bx) || !isFinite(by) || !isFinite(rxc) || !isFinite(ryc) {
				return s.setFault(BodyFaultNumerical, stepStart, clamp01(uCur))
			}
			r := &s.runners[bestRunner]
			if s.firstTouchMS < 0 {
				s.firstTouchMS, s.firstTouchRunner = s.clock, bestRunner
			}
			miscontrol := false
			if s.controlError > 0 {
				miscontrol = s.rnd.Float64() < s.controlError
			}
			if miscontrol {
				vdx, vdy := s.deflectVelocity(vx, vy, bx, by, rxc, ryc)
				if !isFinite(vdx) || !isFinite(vdy) || !isFinite(vdx*vdx+vdy*vdy) {
					return s.setFault(BodyFaultNumerical, stepStart, clamp01(bestU))
				}
				s.ball.Mode = BallRolling
				if vdx == 0 && vdy == 0 {
					s.ball.Mode = BallLoose
				}
				s.ball.X, s.ball.Y = bx, by
				s.ball.VXM, s.ball.VYM = vdx, vdy
				r.CooldownUntilMS = eventTimeMS(stepStart, s.cfg.SubstepMS, bestU) + DeflectionCooldownMS
				contactBall := Ball{Mode: BallRolling, X: bx, Y: by, VXM: vx, VYM: vy}
				s.emitContactEvent(PassEventDeflect, bestRunner, s.clock, bestU, contactBall, vdx, vdy, rxc, ryc)
				px, py = bx, by
				vx, vy = vdx, vdy
				uCur = bestU
				rem := dt * (1 - bestU)
				ex = px + (vx/PitchLengthM)*rem
				ey = py + (vy/PitchWidthM)*rem
				firstSegment = false
				stopped = false
				continue
			}
			s.ball.HoldDX = bx - rxc
			s.ball.HoldDY = by - ryc
			s.ball.VXM, s.ball.VYM = 0, 0
			s.ball.X = r.X + s.ball.HoldDX
			s.ball.Y = r.Y + s.ball.HoldDY
			s.ball.Mode = BallHeld
			if s.contactMS < 0 {
				s.contactMS, s.contactRunner = s.clock, bestRunner
			}
			contactBall := Ball{Mode: BallHeld, X: bx, Y: by, VXM: vx, VYM: vy}
			s.emitContactEvent(PassEventControl, bestRunner, s.clock, bestU, contactBall, 0, 0, rxc, ryc)
			return nil
		}
	}
}
