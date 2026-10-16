package liveplay

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// T-051 observation-stimulus identity and awareness/reachability study
// (isolated prototype evidence, TEST-ONLY).
//
// The candidate contract, ordering, persisted controller state and falsifiable
// fixture matrix are fixed BEFORE this code in:
//   vault/07-Session-Logs/2026-10-16-t051-stimulus-identity-evidence.md
//
// Nothing here changes runtime behaviour: the S0 engine ReactAtMS/contact/
// cooldown/immunity are never written, and the test-only controller reads only the
// sampled BallObservation plus the runner's own clock/ReactMS. Candidate A (the
// event-cue) is an explicit COUNTERFACTUAL diagnostic, not an admissible observer.

// --- test-only controller ---------------------------------------------------

// t051Stimulus labels an observation-derived awareness stimulus.
type t051Stimulus uint8

const (
	// t051NoStimulus: no transition observed.
	t051NoStimulus t051Stimulus = iota
	// t051FirstSighting: Unknown -> Seen.
	t051FirstSighting
	// t051ModeChange: the observed ball mode changed since the last sighting.
	t051ModeChange
	// t051OwnerChange: the observed held-owner changed since the last sighting.
	t051OwnerChange
	// t051PositionDiscontinuity: same mode/owner, but the position jump exceeds
	// the velocity-consistent bound.
	t051PositionDiscontinuity
)

// String returns the stable label for evidence.
func (s t051Stimulus) String() string {
	switch s {
	case t051FirstSighting:
		return "first-sighting"
	case t051ModeChange:
		return "mode-change"
	case t051OwnerChange:
		return "owner-change"
	case t051PositionDiscontinuity:
		return "position-discontinuity"
	default:
		return "none"
	}
}

// t051Cue labels a COUNTERFACTUAL event cue (Candidate A). It consumes
// authoritative event truth and is never used as an observer.
type t051Cue uint8

const (
	t051CueNone t051Cue = iota
	t051CueRelease
	t051CueControl
	t051CueDeflect
	t051CueStop
	t051CueOut
)

// t051EventCue is the COUNTERFACTUAL diagnostic stimulus rule: it reads the
// authoritative event KIND (hidden truth). It is provided only to quantify what an
// event-local contract would need — not as a production-approved observer.
func t051EventCue(kind PassEventKind) (t051Cue, bool) {
	switch kind {
	case PassEventRelease:
		return t051CueRelease, true
	case PassEventControl:
		return t051CueControl, true
	case PassEventDeflect:
		return t051CueDeflect, true
	case PassEventStop:
		return t051CueStop, true
	case PassEventOut:
		return t051CueOut, true
	default:
		return t051CueNone, false
	}
}

// t051StateChangingEvents returns the authoritative state-changing events (path
// is inert).
func t051StateChangingEvents(events []PassEvent) []PassEvent {
	out := []PassEvent{}
	for _, e := range events {
		if isStateChangingEvent(e.Kind) {
			out = append(out, e)
		}
	}
	return out
}

// t051DiscontinuityMarginM is the explicit, non-measured tolerance added to the
// velocity-consistent displacement bound.
const t051DiscontinuityMarginM = 0.05

// t051AwarenessDelayMS is the test-only awareness delay armed on a stimulus.
const t051AwarenessDelayMS = 300

// t051Controller is the complete, JSON-serialisable test-only awareness controller.
// It observes only sampled data and own state; it never reads the event log and
// never writes the Sim.
type t051Controller struct {
	LastSeen       bool         `json:"lastSeen"`
	LastSightingMS int          `json:"lastSightingMs"`
	LastMode       BallMode     `json:"lastMode"`
	LastOwner      int          `json:"lastOwner"`
	LastX          float64      `json:"lastX"`
	LastY          float64      `json:"lastY"`
	LastVXM        float64      `json:"lastVxm"`
	LastVYM        float64      `json:"lastVym"`
	ArmedMS        int          `json:"armedMs"`
	StimulusCount  int          `json:"stimulusCount"`
	LastStimulus   t051Stimulus `json:"lastStimulus"`
	GatedApplied   int          `json:"gatedApplied"`
	GatedBlocked   int          `json:"gatedBlocked"`
}

// newT051Controller returns a clean controller (never armed).
func newT051Controller() t051Controller {
	return t051Controller{ArmedMS: -1, LastStimulus: t051NoStimulus}
}

// t051SpeedMPS is the observed ball speed.
func t051SpeedMPS(vx, vy float64) float64 { return math.Sqrt(vx*vx + vy*vy) }

// Observe consumes the CURRENT sampled observation (and ReactMS) and returns the
// stimulus it produced. A read whose ClockMS did not advance (a stale/repeated
// read between publications) never fires.
func (c *t051Controller) Observe(o BallObservation, reactMS int) (t051Stimulus, bool) {
	if !o.Seen {
		return t051NoStimulus, false
	}
	if c.LastSeen && o.ClockMS <= c.LastSightingMS {
		return t051NoStimulus, false
	}
	stim := t051NoStimulus
	switch {
	case !c.LastSeen:
		stim = t051FirstSighting
	case o.Mode != c.LastMode:
		stim = t051ModeChange
	case o.Owner != c.LastOwner:
		stim = t051OwnerChange
	default:
		dt := o.ClockMS - c.LastSightingMS
		exp := math.Max(t051SpeedMPS(c.LastVXM, c.LastVYM), t051SpeedMPS(o.VXM, o.VYM)) * float64(dt) / 1000.0
		if DistM(c.LastX, c.LastY, o.X, o.Y) > exp+t051DiscontinuityMarginM {
			stim = t051PositionDiscontinuity
		}
	}
	c.LastSeen = true
	c.LastSightingMS = o.ClockMS
	c.LastMode = o.Mode
	c.LastOwner = o.Owner
	c.LastX, c.LastY = o.X, o.Y
	c.LastVXM, c.LastVYM = o.VXM, o.VYM
	if stim != t051NoStimulus {
		c.ArmedMS = o.ClockMS + reactMS
		c.StimulusCount++
		c.LastStimulus = stim
	}
	return stim, stim != t051NoStimulus
}

// t051StimRecord is bounded evidence of one fired stimulus.
type t051StimRecord struct {
	ClockMS  int
	Stimulus t051Stimulus
}

// t051Harness couples a Sim to the test-only controller. It is evidence-only.
type t051Harness struct {
	sim      *Sim
	ctl      t051Controller
	reactMS  int
	stimuli  []t051StimRecord
	policyAt []int // clocks at which the test-only gated policy applied
}

// newT051Harness wraps a sim with a clean controller.
func newT051Harness(sim *Sim, reactMS int) *t051Harness {
	return &t051Harness{sim: sim, ctl: newT051Controller(), reactMS: reactMS}
}

// step advances the engine one substep, then feeds the CURRENT observation to the
// controller (after the engine step that produced the publication).
func (h *t051Harness) step() {
	h.sim.Step()
	if o, ok := h.sim.Perception(0); ok {
		if _, fired := h.ctl.Observe(o, h.reactMS); fired {
			h.stimuli = append(h.stimuli, t051StimRecord{ClockMS: h.ctl.LastSightingMS, Stimulus: h.ctl.LastStimulus})
		}
	}
}

// applyTestPolicy is the ONLY explicitly test-only policy proposal the awareness
// deadline may gate. It never touches engine contact/motion.
func (h *t051Harness) applyTestPolicy() {
	if h.ctl.ArmedMS >= 0 && h.sim.ClockMS() >= h.ctl.ArmedMS {
		h.ctl.GatedApplied++
		h.policyAt = append(h.policyAt, h.sim.ClockMS())
	} else {
		h.ctl.GatedBlocked++
	}
}

// snapshot is the combined engine + controller state. ReactMS is serialized
// because it affects future controller behaviour and must be restored from the
// decoded combined state, never from the original harness.
func (h *t051Harness) snapshot() t051Combined {
	return t051Combined{Engine: h.sim.Snapshot(), Controller: h.ctl, ReactMS: h.reactMS}
}

// t051Combined is the persisted combined state (complete engine + controller +
// the test-only awareness delay configuration).
type t051Combined struct {
	Engine     Snapshot       `json:"engine"`
	Controller t051Controller `json:"controller"`
	ReactMS    int            `json:"reactMs"`
}

// t051JSONResume marshals the combined state, unmarshals it and resumes. The
// resumed harness's ReactMS comes ONLY from the decoded state.
func t051JSONResume(t *testing.T, h *t051Harness) *t051Harness {
	t.Helper()
	blob, err := json.Marshal(h.snapshot())
	if err != nil {
		t.Fatalf("marshal combined: %v", err)
	}
	var cs t051Combined
	if err := json.Unmarshal(blob, &cs); err != nil {
		t.Fatalf("unmarshal combined: %v", err)
	}
	sim, err := Resume(cs.Engine)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	return &t051Harness{sim: sim, ctl: cs.Controller, reactMS: cs.ReactMS}
}

// t051AssertTails steps straight and resumed in lockstep, applying the same mid
// hook AND the same test-only gated policy to BOTH, and compares the COMPLETE
// combined state every substep plus the non-empty absolute stimuli / policy /
// sample / event tails.
func t051AssertTails(t *testing.T, straight, resumed *t051Harness, horizon int, label string, mid func(*t051Harness, int)) {
	t.Helper()
	stimHead := len(straight.stimuli)
	polHead := len(straight.policyAt)
	sampHead := len(straight.sim.Samples())
	evHead := len(straight.sim.Events())
	for straight.sim.ClockMS() < horizon {
		if resumed.sim.ClockMS() != straight.sim.ClockMS() {
			t.Fatalf("%s: clocks diverged %d vs %d", label, straight.sim.ClockMS(), resumed.sim.ClockMS())
		}
		if mid != nil {
			mid(straight, straight.sim.ClockMS())
			mid(resumed, resumed.sim.ClockMS())
		}
		straight.step()
		resumed.step()
		straight.applyTestPolicy()
		resumed.applyTestPolicy()
		if !reflect.DeepEqual(straight.snapshot(), resumed.snapshot()) {
			t.Fatalf("%s @%d: combined state diverged after resume", label, straight.sim.ClockMS())
		}
	}
	if !reflect.DeepEqual(straight.stimuli[stimHead:], resumed.stimuli) {
		t.Fatalf("%s: stimuli tail differs:\nstraight=%+v\nresumed=%+v", label, straight.stimuli[stimHead:], resumed.stimuli)
	}
	if !reflect.DeepEqual(straight.policyAt[polHead:], resumed.policyAt) {
		t.Fatalf("%s: policy tail differs:\nstraight=%v\nresumed=%v", label, straight.policyAt[polHead:], resumed.policyAt)
	}
	if !reflect.DeepEqual(straight.sim.Samples()[sampHead:], resumed.sim.Samples()) {
		t.Fatalf("%s: resumed sample tail differs from the straight run", label)
	}
	// Events ARE persisted: Resume restores the full pre-split log, so both the
	// COMPLETE restored log and the appended tail must match. Samples are
	// regenerated, so only the sample tail matches (checked above).
	sEvents := straight.sim.Events()
	rEvents := resumed.sim.Events()
	if len(sEvents) < evHead || len(rEvents) < evHead {
		t.Fatalf("%s: event log shorter than the pre-split head (%d/%d < %d)", label, len(sEvents), len(rEvents), evHead)
	}
	if !reflect.DeepEqual(sEvents, rEvents) {
		t.Fatalf("%s: complete restored event log differs from the straight run", label)
	}
	if !reflect.DeepEqual(sEvents[evHead:], rEvents[evHead:]) {
		t.Fatalf("%s: resumed event tail differs from the straight run", label)
	}
	if len(resumed.stimuli) == 0 {
		t.Fatalf("%s: resumed tail produced no stimulus (vacuous)", label)
	}
	if len(resumed.policyAt) == 0 {
		t.Fatalf("%s: resumed tail produced no policy decision (vacuous)", label)
	}
}

// t051SensingSim builds a one-runner sim with a sensing record (no local policy).
func t051SensingSim(t *testing.T, seed uint64, radiusM float64) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	r := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
	if err := r.EnableSensing(radiusM); err != nil {
		t.Fatalf("EnableSensing: %v", err)
	}
	sim.SetRunners([]Runner{r})
	return sim
}

// t051ObservationTrail records the runner's observation after each step.
func t051ObservationTrail(h *t051Harness, horizon int) []BallObservation {
	var trail []BallObservation
	for h.sim.ClockMS() < horizon {
		h.step()
		o, _ := h.sim.Perception(0)
		trail = append(trail, o)
	}
	return trail
}

// t051ReleaseClocks returns the clocks of the authoritative release events.
func t051ReleaseClocks(events []PassEvent) []int {
	var clocks []int
	for _, e := range events {
		if e.Kind == PassEventRelease {
			clocks = append(clocks, e.ClockMS)
		}
	}
	return clocks
}

// t051ObsSame compares two observations as NUMERIC-equivalent, NOT bit-identical:
// the discrete fields (Seen/ClockMS/Mode/Owner) must match EXACTLY; the
// coordinates/velocities must match within a declared floating-point tolerance,
// because a same-path re-release reconstructs the velocity through one extra
// multiply/divide (the *same physical state*, not the *same bits*).
func t051ObsSame(a, b BallObservation) bool {
	return a.Seen == b.Seen && a.ClockMS == b.ClockMS && a.Mode == b.Mode && a.Owner == b.Owner &&
		math.Abs(a.X-b.X) <= 1e-9 && math.Abs(a.Y-b.Y) <= 1e-9 &&
		math.Abs(a.VXM-b.VXM) <= 1e-9 && math.Abs(a.VYM-b.VYM) <= 1e-9
}

// t051TrailsSame compares two complete observation histories under t051ObsSame.
func t051TrailsSame(a, b []BallObservation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !t051ObsSame(a[i], b[i]) {
			return false
		}
	}
	return true
}

// t051TrailMaxErr returns the maximum coordinate/velocity difference between two
// equal-length observation histories, reported as the numeric error beside the
// exact discrete fields.
func t051TrailMaxErr(a, b []BallObservation) float64 {
	maxErr := 0.0
	for i := range a {
		for _, d := range []float64{
			math.Abs(a[i].X - b[i].X), math.Abs(a[i].Y - b[i].Y),
			math.Abs(a[i].VXM - b[i].VXM), math.Abs(a[i].VYM - b[i].VYM),
		} {
			if d > maxErr {
				maxErr = d
			}
		}
	}
	return maxErr
}

// t051DiscreteEqual reports whether two histories match EXACTLY on the discrete
// fields (Seen/ClockMS/Mode/Owner), independent of the numeric tolerance.
func t051DiscreteEqual(a, b []BallObservation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Seen != b[i].Seen || a[i].ClockMS != b[i].ClockMS || a[i].Mode != b[i].Mode || a[i].Owner != b[i].Owner {
			return false
		}
	}
	return true
}

// --- F1/F2/F3: first sighting and no periodic re-arming ----------------------

// TestT051FirstSightingAndNoPeriodicRearm pins that a stationary in-range ball is
// sighted once and then never re-arms the awareness deadline on repeated reads.
func TestT051FirstSightingAndNoPeriodicRearm(t *testing.T) {
	sim := t051SensingSim(t, 1, 200.0)
	sim.SetBall(Ball{Mode: BallLoose, X: 0.52, Y: 0.50})
	h := newT051Harness(sim, t051AwarenessDelayMS)
	for h.sim.ClockMS() < 1500 {
		h.step()
	}
	if h.ctl.StimulusCount != 1 || h.ctl.LastStimulus != t051FirstSighting {
		t.Fatalf("stationary sighting stimuli=%d last=%s, want 1 first-sighting", h.ctl.StimulusCount, h.ctl.LastStimulus)
	}
	if want := 0 + t051AwarenessDelayMS; h.ctl.ArmedMS != want {
		t.Fatalf("ArmedMS=%d, want %d", h.ctl.ArmedMS, want)
	}
	t.Logf("F1/F2: stationary in-range ball -> %d stimulus (%s, stimulus clock=%d, last sample clock=%d), ArmedMS=%d; repeated reads never re-armed",
		h.ctl.StimulusCount, h.ctl.LastStimulus, h.stimuli[0].ClockMS, h.ctl.LastSightingMS, h.ctl.ArmedMS)
}

// TestT051MovingRollingNoRearm pins that a smoothly rolling in-range ball never
// re-arms on periodic sightings (the velocity-consistent bound holds).
func TestT051MovingRollingNoRearm(t *testing.T) {
	sim := t051SensingSim(t, 2, 200.0)
	sim.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 2.0})
	h := newT051Harness(sim, t051AwarenessDelayMS)
	for h.sim.ClockMS() < 1400 {
		h.step()
	}
	if h.ctl.StimulusCount != 1 || h.ctl.LastStimulus != t051FirstSighting {
		t.Fatalf("moving rolling stimuli=%d last=%s, want 1 first-sighting", h.ctl.StimulusCount, h.ctl.LastStimulus)
	}
	t.Logf("F3: smoothly rolling ball -> %d stimulus (%s); no discontinuity false-fire over %d ms", h.ctl.StimulusCount, h.ctl.LastStimulus, 1400)
}

// --- F4: mode / owner transitions -------------------------------------------

// TestT051ModeAndOwnerTransitions pins that observable mode changes (a control and
// a second REAL release inside the sensing radius) arm the deadline. The second
// release is inside 3 m but outside the 1 m physical reach, moving away, so it is
// observable (Held -> fresh Rolling) without contacting the runner.
func TestT051ModeAndOwnerTransitions(t *testing.T) {
	sim := t051SensingSim(t, 3, 3.0)
	sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	h := newT051Harness(sim, t051AwarenessDelayMS)
	for h.sim.ClockMS() < 500 {
		h.step()
	}
	heldObs, _ := sim.Perception(0)
	if heldObs.Mode != BallHeld {
		t.Fatalf("expected a held observation before the 2nd release, got %+v", heldObs)
	}
	d := DistM(0.52, 0.50, sim.runners[0].X, sim.runners[0].Y)
	if d >= 3.0 || d <= ControlRadiusM {
		t.Fatalf("fixture geometry: 2nd release dist %.3f m must be inside 3 m and outside %.1f m reach", d, ControlRadiusM)
	}
	sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	h.step()
	rollObs, _ := sim.Perception(0)
	if rollObs.Mode != BallRolling || !rollObs.Seen || rollObs.ClockMS != 500 {
		t.Fatalf("expected a fresh rolling observation at 500 after the held event, got %+v", rollObs)
	}
	for h.sim.ClockMS() < 800 {
		h.step()
	}
	if h.ctl.StimulusCount != 3 {
		t.Fatalf("stimuli=%d, want 3 (first-sighting, mode-change to held, mode-change to rolling): %+v", h.ctl.StimulusCount, h.stimuli)
	}
	if h.stimuli[1].Stimulus != t051ModeChange || h.stimuli[2].Stimulus != t051ModeChange {
		t.Fatalf("unexpected stimuli: %+v", h.stimuli)
	}
	t.Logf("F4: 2nd release dist %.2f m (radius 3, reach %.1f); held@%d -> rolling@%d; stimuli %+v", d, ControlRadiusM, heldObs.ClockMS, rollObs.ClockMS, h.stimuli)
}

// TestT051OwnerChangeSyntheticPerception is an explicitly SYNTHETIC
// perception-only fixture: it feeds hand-built BallObservation values (no world
// state, no outcome, no event) so the OwnerChange branch is genuinely exercised
// even though the physical resolver does not switch a Held ball's owner.
func TestT051OwnerChangeSyntheticPerception(t *testing.T) {
	c := newT051Controller()
	o1 := BallObservation{RadiusM: 1.0, Seen: true, ClockMS: 100, Mode: BallHeld, Owner: 0, X: 0.50, Y: 0.50}
	o2 := BallObservation{RadiusM: 1.0, Seen: true, ClockMS: 200, Mode: BallHeld, Owner: 1, X: 0.50, Y: 0.50}
	if _, fired := c.Observe(o1, t051AwarenessDelayMS); !fired || c.LastStimulus != t051FirstSighting {
		t.Fatalf("first synthetic sighting did not fire first-sighting")
	}
	stim, fired := c.Observe(o2, t051AwarenessDelayMS)
	if !fired || stim != t051OwnerChange {
		t.Fatalf("same-Held different-owner did not fire OwnerChange: stim=%s fired=%v", stim, fired)
	}
	t.Logf("F4b (SYNTHETIC perception-only): same Held mode, owner 0->1 -> %s, ArmedMS=%d", stim, c.ArmedMS)
}

// --- F5: unseen / stale ------------------------------------------------------

// TestT051UnseenAndStale pins that an unobserved ball never arms, and that a ball
// moving AWAY from the runner (in range only at first, beyond reach, no contact)
// produces exactly one first-sighting and then a frozen stale record.
func TestT051UnseenAndStale(t *testing.T) {
	unseen := t051SensingSim(t, 4, 0.5)
	unseen.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.90, VXM: 1.0})
	hu := newT051Harness(unseen, t051AwarenessDelayMS)
	for hu.sim.ClockMS() < 800 {
		hu.step()
	}
	if hu.ctl.StimulusCount != 0 {
		t.Fatalf("unseen ball armed %d stimuli", hu.ctl.StimulusCount)
	}

	stale := t051SensingSim(t, 4, 3.0)
	// Within 3 m radius but OUTSIDE the 1 m reach, moving AWAY: no contact.
	stale.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 20.0})
	hs := newT051Harness(stale, t051AwarenessDelayMS)
	hs.step()
	first, _ := stale.Perception(0)
	if !first.Seen || first.ClockMS != 0 || first.Mode != BallRolling {
		t.Fatalf("no true initial fresh sighting: %+v", first)
	}
	for hs.sim.ClockMS() < 900 {
		hs.step()
	}
	if ms, _ := stale.FirstContact(); ms >= 0 {
		t.Fatalf("stale scene produced a contact at %d (ball should move away)", ms)
	}
	o, _ := stale.Perception(0)
	if hs.ctl.StimulusCount != 1 {
		t.Fatalf("stale scene stimuli=%d, want 1 (initial sighting only): %+v", hs.ctl.StimulusCount, hs.stimuli)
	}
	if o.ClockMS >= 900 {
		t.Fatalf("record refreshed at %d after the ball left the radius (not stale)", o.ClockMS)
	}
	if DistM(0.52, 0.50, stale.Ball().X, stale.Ball().Y) <= 1e-9 {
		t.Fatal("ball froze (movement should continue)")
	}
	t.Logf("F5: unseen stimuli=%d; stale scene stimulus=%s at record clock %d (< horizon), no contact, ball moved addr=%.3f", hu.ctl.StimulusCount, hs.ctl.LastStimulus, o.ClockMS, stale.Ball().X)
}

// --- F6: equal history, different authoritative events -----------------------

// TestT051EqualHistoryDifferentEvents pins that Candidate B produces identical
// decisions for a real release and a plain pre-existing rolling ball, i.e. it
// cannot identify that a release occurred, while the event-cue can.
func TestT051EqualHistoryDifferentEvents(t *testing.T) {
	build := func(release bool) (*t051Harness, *Sim) {
		sim := t051SensingSim(t, 7, 200.0)
		if release {
			sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		} else {
			sim.SetBall(Ball{Mode: BallRolling, X: 0.52, Y: 0.50, VXM: 2.0})
		}
		return newT051Harness(sim, t051AwarenessDelayMS), sim
	}
	hA, simA := build(true)
	hB, simB := build(false)
	trailA := t051ObservationTrail(hA, 600)
	trailB := t051ObservationTrail(hB, 600)

	if !reflect.DeepEqual(trailA, trailB) {
		t.Fatalf("observation histories differ (isolation lost):\nA=%+v\nB=%+v", trailA, trailB)
	}
	if hA.ctl.StimulusCount != hB.ctl.StimulusCount || hA.ctl.LastStimulus != hB.ctl.LastStimulus {
		t.Fatalf("controller decisions differ: A(%d,%s) B(%d,%s)", hA.ctl.StimulusCount, hA.ctl.LastStimulus, hB.ctl.StimulusCount, hB.ctl.LastStimulus)
	}
	evA := t051StateChangingEvents(simA.Events())
	evB := t051StateChangingEvents(simB.Events())
	cuesA, cuesB := 0, 0
	for _, e := range evA {
		if _, ok := t051EventCue(e.Kind); ok {
			cuesA++
		}
	}
	for _, e := range evB {
		if _, ok := t051EventCue(e.Kind); ok {
			cuesB++
		}
	}
	if cuesA != 1 || cuesB != 0 {
		t.Fatalf("event-cue counts A=%d B=%d, want 1/0", cuesA, cuesB)
	}
	if hA.ctl.StimulusCount != 1 {
		t.Fatalf("controller count=%d, want 1", hA.ctl.StimulusCount)
	}
	t.Logf("F6: equal %d-sample observation histories; controller identical (%s); event-cue A=%d (release) vs B=%d -> Candidate B cannot identify the release", len(trailA), hA.ctl.LastStimulus, cuesA, cuesB)
}

// --- F7/F8: same-mode second release ----------------------------------------

// TestT051SameModeSecondReleasePositions pins the identifiable case (a far second
// release is a position discontinuity) and the ambiguous case (a same-path second
// release is unidentifiable: its complete observation history equals a
// NO-second-release engine while the authoritative event log differs).
func TestT051SameModeSecondReleasePositions(t *testing.T) {
	first := PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1}

	// Identifiable: a far second release is a position discontinuity.
	far := t051SensingSim(t, 8, 200.0)
	far.ReleasePass(first)
	hf := newT051Harness(far, t051AwarenessDelayMS)
	for hf.sim.ClockMS() < 600 {
		hf.step()
	}
	beforeFar := t051ReleaseClocks(far.Events())
	if len(beforeFar) != 1 || beforeFar[0] != 0 {
		t.Fatalf("far: releases before 2nd = %v, want [0]", beforeFar)
	}
	far.ReleasePass(PassParams{StartX: 0.90, StartY: 0.90, TargetX: 0.95, TargetY: 0.90, SpeedMPS: 2.0, Passer: -1})
	for hf.sim.ClockMS() < 900 {
		hf.step()
	}
	afterFar := t051ReleaseClocks(far.Events())
	if len(afterFar) != 2 || afterFar[0] != 0 || afterFar[1] != 600 {
		t.Fatalf("far: release clocks = %v, want [0 600]", afterFar)
	}
	if hf.ctl.StimulusCount != 2 || hf.ctl.LastStimulus != t051PositionDiscontinuity {
		t.Fatalf("identifiable 2nd release stimuli=%d last=%s, want 2 position-discontinuity", hf.ctl.StimulusCount, hf.ctl.LastStimulus)
	}

	// Ambiguous: a same-path second release on the lattice (600) is unidentifiable.
	build := func() *t051Harness {
		sim := t051SensingSim(t, 8, 200.0)
		sim.ReleasePass(first)
		return newT051Harness(sim, t051AwarenessDelayMS)
	}
	amb, noRel := build(), build()
	for amb.sim.ClockMS() < 600 {
		amb.step()
		noRel.step()
	}
	beforeAmb := t051ReleaseClocks(amb.sim.Events())
	b := amb.sim.Ball()
	sp := t051SpeedMPS(b.VXM, b.VYM)
	amb.sim.ReleasePass(PassParams{
		StartX: b.X, StartY: b.Y,
		TargetX: b.X + (b.VXM/PitchLengthM)*0.1, TargetY: b.Y + (b.VYM/PitchWidthM)*0.1,
		SpeedMPS: sp, Passer: -1,
	})
	afterAmb := t051ReleaseClocks(amb.sim.Events())
	afterNo := t051ReleaseClocks(noRel.sim.Events())
	if len(beforeAmb) != 1 || beforeAmb[0] != 0 {
		t.Fatalf("amb: releases before 2nd = %v, want [0]", beforeAmb)
	}
	if len(afterAmb) != 2 || afterAmb[1] != 600 {
		t.Fatalf("amb: release clocks = %v, want [0 600]", afterAmb)
	}
	if len(afterNo) != 1 || afterNo[0] != 0 {
		t.Fatalf("noRel: releases = %v, want [0]", afterNo)
	}

	trailAmb := t051ObservationTrail(amb, 900)
	trailNo := t051ObservationTrail(noRel, 900)
	if !t051DiscreteEqual(trailAmb, trailNo) {
		t.Fatalf("same-path release changed the discrete observation fields:\namb=%+v\nno=%+v", trailAmb, trailNo)
	}
	if !t051TrailsSame(trailAmb, trailNo) {
		t.Fatalf("same-path release changed the observation history beyond tolerance:\namb=%+v\nno=%+v", trailAmb, trailNo)
	}
	maxErr := t051TrailMaxErr(trailAmb, trailNo)
	if amb.ctl.StimulusCount != 1 || noRel.ctl.StimulusCount != 1 {
		t.Fatalf("controller counts amb=%d noRel=%d, want 1/1: amb=%+v noRel=%+v", amb.ctl.StimulusCount, noRel.ctl.StimulusCount, amb.stimuli, noRel.stimuli)
	}
	if amb.ctl.LastStimulus != noRel.ctl.LastStimulus {
		t.Fatalf("controllers differ: amb=%s noRel=%s", amb.ctl.LastStimulus, noRel.ctl.LastStimulus)
	}
	cuesAmb, cuesNo := 0, 0
	for _, e := range t051StateChangingEvents(amb.sim.Events()) {
		if _, ok := t051EventCue(e.Kind); ok {
			cuesAmb++
		}
	}
	for _, e := range t051StateChangingEvents(noRel.sim.Events()) {
		if _, ok := t051EventCue(e.Kind); ok {
			cuesNo++
		}
	}
	if cuesAmb != 2 || cuesNo != 1 {
		t.Fatalf("event-cue counts amb=%d noRel=%d, want 2/1", cuesAmb, cuesNo)
	}
	t.Logf("F7/F8: identifiable far 2nd release -> %s (count 2, releases %v); ambiguous same-path 2nd release (on-lattice @600): releases amb=%v vs noRel=%v, %d-sample histories NUMERIC-equivalent (discrete fields exact, max numeric err %.3e m), controller stays 1 stimulus -> unidentifiable; event cues amb=%d vs noRel=%d",
		hf.ctl.LastStimulus, afterFar, afterAmb, afterNo, len(trailAmb), maxErr, cuesAmb, cuesNo)
}

// --- F9: coalesced window ----------------------------------------------------

// TestT051CoalescedWindowLoss pins that multiple state-changing events inside one
// cadence window coalesce into fewer observation transitions.
func TestT051CoalescedWindowLoss(t *testing.T) {
	sim := t051SensingSim(t, 9, 3.0)
	sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	h := newT051Harness(sim, t051AwarenessDelayMS)
	for h.sim.ClockMS() < 300 {
		h.step()
	}
	events := t051StateChangingEvents(sim.Events())
	cues := 0
	for _, e := range events {
		if _, ok := t051EventCue(e.Kind); ok {
			cues++
		}
	}
	modeChanges := 0
	for _, s := range h.stimuli {
		if s.Stimulus == t051ModeChange {
			modeChanges++
		}
	}
	if cues < 2 {
		t.Fatalf("expected >=2 state-changing events (release+control), got %d: %+v", cues, events)
	}
	if modeChanges != 1 {
		t.Fatalf("expected exactly one observable mode-change for release+control, got %d: %+v", modeChanges, h.stimuli)
	}
	t.Logf("F9: %d authoritative state-changing events in the first window -> %d observable mode-change stimulus (coalesced loss)", cues, modeChanges)
}

// --- F10: combined JSON tails ------------------------------------------------

// TestT051CombinedJSONTails pins deterministic COMPLETE engine+controller Resume
// with non-empty absolute tails across the awareness deadlines.
func TestT051CombinedJSONTails(t *testing.T) {
	build := func() *t051Harness {
		sim := t051SensingSim(t, 10, 3.0)
		sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		return newT051Harness(sim, t051AwarenessDelayMS)
	}
	mid := func(h *t051Harness, clock int) {
		if clock == 600 {
			h.sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		}
	}
	for _, split := range []int{0, 60, 240, 600} {
		straight := build()
		for straight.sim.ClockMS() < split {
			straight.step()
		}
		resumed := t051JSONResume(t, straight)
		t051AssertTails(t, straight, resumed, 1100, "t051-json", mid)
		if straight.sim.ClockMS() != 1100 {
			t.Fatalf("split %d: horizon not reached", split)
		}
		t.Logf("F10: split=%d -> combined engine+controller tail 1100 ms identical every substep", split)
	}
}

// --- F11: observational purity -----------------------------------------------

// TestT051ControllerObservationalPurity pins that attaching the test-only
// controller changes NOTHING in the engine: the engine Snapshot is bit-identical
// with and without the controller, every substep.
func TestT051ControllerObservationalPurity(t *testing.T) {
	build := func() *Sim {
		sim := t051SensingSim(t, 11, 3.0)
		sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		return sim
	}
	withCtl := build()
	plain := build()
	h := newT051Harness(withCtl, t051AwarenessDelayMS)
	for i := 0; i < 100; i++ {
		h.step() // attaches controller observe + policy (no engine write)
		h.applyTestPolicy()
		plain.Step()
		if !reflect.DeepEqual(withCtl.Snapshot(), plain.Snapshot()) {
			t.Fatalf("controller changed the engine at step %d", i)
		}
	}
	t.Logf("F11: engine bit-identical with/without controller for 100 substeps; gated policy applied=%d blocked=%d (test-only)", h.ctl.GatedApplied, h.ctl.GatedBlocked)
}

// --- F12b: moving-local provenance scene -------------------------------------

// TestT051MovingLocalProvenanceScene adds one bounded scene with a REAL T-049
// sidecar recorder and a moving local pursuit/wakeup runner. The test-only
// controller is observational only. The sidecar records the ENGINE local-policy
// publications; the controller observes AFTER Step. No claim is made that the
// controller's proposals were engine decisions.
func TestT051MovingLocalProvenanceScene(t *testing.T) {
	cfg := ScheduledLivePassConfig(100)
	build := func(seed uint64) (*Sim, *ProvenanceRecorder) {
		sim := mustSim(t, cfg, seed)
		r := t050Local(0.10, 0.50, 0.90, 0.50, 200.0, 0, true) // moving local pursuit+wakeup
		sim.SetRunners([]Runner{r})
		rec, err := NewProvenanceRecorder(256)
		if err != nil {
			t.Fatalf("NewProvenanceRecorder: %v", err)
		}
		sim.SetProvenanceRecorder(rec)
		sim.ReleasePass(PassParams{StartX: 0.52, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
		return sim, rec
	}
	withSim, withRec := build(20)
	plain, _ := build(20)
	h := newT051Harness(withSim, t051AwarenessDelayMS)
	for i := 0; i < 60; i++ { // 1200 ms
		h.step()
		plain.Step()
		if !reflect.DeepEqual(withSim.Snapshot(), plain.Snapshot()) {
			t.Fatalf("controller changed the moving-local engine at step %d", i)
		}
		if i%10 == 0 {
			r := withSim.runners[0]
			o, _ := withSim.Perception(0)
			t.Logf("t=%d own(x=%.4f y=%.4f vx=%.3f intent=(%.4f,%.4f)) obs(seen=%v clock=%d x=%.4f mode=%s owner=%d) ctl(armed=%d stim=%s n=%d)",
				withSim.ClockMS(), r.X, r.Y, r.VXM, r.IntentX, r.IntentY, o.Seen, o.ClockMS, o.X, o.Mode, o.Owner, h.ctl.ArmedMS, h.ctl.LastStimulus, h.ctl.StimulusCount)
		}
	}
	if DistM(0.10, 0.50, withSim.runners[0].X, withSim.runners[0].Y) <= 0.10 {
		t.Fatalf("moving-local runner did not move enough: %.4f m", DistM(0.10, 0.50, withSim.runners[0].X, withSim.runners[0].Y))
	}
	recs := withRec.Records()
	if len(recs) == 0 {
		t.Logf("F12b: NO sidecar records produced on this scene (classified explicitly; the local policy did not publish)")
	} else {
		for _, rec := range recs {
			t.Logf("F12b sidecar (ENGINE local policy @ publication): clock=%d runner=%d reason=%s applied=%v gate=%s policy=%s freshness=%s target=(%.4f,%.4f) own=(%.4f,%.4f)",
				rec.ClockMS, rec.Runner, rec.Reason, rec.Applied, rec.GateOutcome, rec.Policy, rec.Freshness, rec.TargetX, rec.TargetY, rec.OwnX, rec.OwnY)
		}
	}
	stim := 0
	for _, s := range h.stimuli {
		t.Logf("F12b controller (post-Step, observational): stimulus clock=%d kind=%s", s.ClockMS, s.Stimulus)
		stim++
	}
	if stim == 0 {
		t.Logf("F12b: controller produced no stimulus on this scene (classified explicitly)")
	}
	t.Logf("F12b note: sidecar records are the ENGINE local-policy publications recorded at publication time; the test-only controller observes AFTER Step and its stimuli/proposals are NOT engine decisions. Engine bit-identical with/without controller for 1200 ms.")
}

// --- F13: verbose own/observation/controller timeline ------------------------

// TestT051TimelineEvidence logs the full own motion/intent, observations,
// authoritative events and controller stimuli at critical clocks.
func TestT051TimelineEvidence(t *testing.T) {
	sim := t051SensingSim(t, 12, 3.0)
	sim.ReleasePass(PassParams{StartX: 0.505, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 2.0, Passer: -1})
	h := newT051Harness(sim, t051AwarenessDelayMS)
	for h.sim.ClockMS() < 400 {
		h.step()
		if o, ok := h.sim.Perception(0); ok && o.Seen {
			r := h.sim.runners[0]
			t.Logf("t=%d own(x=%.4f vx=%.3f intent=%.4f) obs(seen=%v clock=%d x=%.4f mode=%s owner=%d) ctl(armed=%d stim=%s n=%d)",
				h.sim.ClockMS(), r.X, r.VXM, r.IntentX, o.Seen, o.ClockMS, o.X, o.Mode, o.Owner, h.ctl.ArmedMS, h.ctl.LastStimulus, h.ctl.StimulusCount)
		}
	}
	blob, err := json.Marshal(h.snapshot())
	if err != nil {
		t.Fatalf("marshal combined: %v", err)
	}
	t.Logf("combined snapshot: %s", blob)
	t.Logf("engine events: %+v", h.sim.Events())
	if len(h.stimuli) == 0 {
		t.Fatal("expected controller stimuli")
	}
}
