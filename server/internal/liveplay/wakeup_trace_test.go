package liveplay

import (
	"math"
	"reflect"
)

// t045Frame is one complete end-of-substep state: ball (position, velocity, mode,
// owner) + every runner (position, velocity, intent, reaction/cooldown deadlines)
// + every runner's observation record. Nothing that could carry a side channel is
// omitted.
type t045Frame struct {
	clock                          int
	ballX, ballY, ballVXM, ballVYM float64
	mode                           BallMode
	owner                          int
	xs, ys, vxs, vys, ix, iy       []float64
	reactAt, cooldown              []int
	obs                            []BallObservation
	hasObs                         []bool
}

// t045Trace is a captured run.
type t045Trace struct {
	mode        t045Mode
	frames      []t045Frame
	events      []PassEvent
	pubClocks   []int     // pre-step clocks the sim's scheduler published (shared mode)
	harnessPubs []t045Pub // per-runner publication records (gated modes)
}

func (tr *t045Trace) record(sim *Sim) {
	b := sim.Ball()
	owner := -1
	if b.Mode == BallHeld {
		owner = sim.contactRunner
	}
	rs := sim.runners
	f := t045Frame{
		clock: sim.ClockMS(), ballX: b.X, ballY: b.Y, ballVXM: b.VXM, ballVYM: b.VYM, mode: b.Mode, owner: owner,
		xs: make([]float64, len(rs)), ys: make([]float64, len(rs)),
		vxs: make([]float64, len(rs)), vys: make([]float64, len(rs)),
		ix: make([]float64, len(rs)), iy: make([]float64, len(rs)),
		reactAt: make([]int, len(rs)), cooldown: make([]int, len(rs)),
		obs: make([]BallObservation, len(rs)), hasObs: make([]bool, len(rs)),
	}
	for i := range rs {
		f.xs[i], f.ys[i] = rs[i].X, rs[i].Y
		f.vxs[i], f.vys[i] = rs[i].VXM, rs[i].VYM
		f.ix[i], f.iy[i] = rs[i].IntentX, rs[i].IntentY
		f.reactAt[i] = rs[i].ReactAtMS
		f.cooldown[i] = rs[i].CooldownUntilMS
		if o := rs[i].Perception; o != nil {
			f.obs[i] = *o
			f.hasObs[i] = true
		}
	}
	tr.frames = append(tr.frames, f)
}

// run advances to horizonMS, recording a frame every substep, the sim scheduler's
// publication clocks and the per-runner harness publications.
func (h *t045Harness) run(horizonMS int, mid func(*Sim, int)) *t045Trace {
	tr := &t045Trace{mode: h.mode}
	tr.record(h.sim)
	for h.sim.ClockMS() < horizonMS {
		if mid != nil {
			mid(h.sim, h.sim.ClockMS())
		}
		start := h.sim.ClockMS()
		h.step()
		if c := h.sim.DecisionPublishCount(); c > h.lastCount {
			h.lastCount = c
			tr.pubClocks = append(tr.pubClocks, start)
		}
		tr.record(h.sim)
	}
	tr.events = h.sim.Events()
	tr.harnessPubs = append([]t045Pub(nil), h.pubs...)
	return tr
}

// stepClocks returns the pre-step clocks at which the run published, using the
// sim's scheduler clocks in shared mode or the deduplicated harness publication
// clocks in gated modes (the sim counter is inert there).
func (tr *t045Trace) stepClocks() []int {
	if len(tr.pubClocks) > 0 {
		return tr.pubClocks
	}
	return t045DedupClocks(tr.harnessPubs)
}

// t045DedupClocks returns publication clocks in first-seen order, deduplicated.
func t045DedupClocks(pubs []t045Pub) []int {
	seen := make(map[int]bool, len(pubs))
	var out []int
	for _, p := range pubs {
		if !seen[p.Clock] {
			seen[p.Clock] = true
			out = append(out, p.Clock)
		}
	}
	return out
}

// t045AppliedClocks returns the pre-step clocks at which runner idx's local policy
// actually EXECUTED (Attempt=true) for a reason.
func t045AppliedClocks(pubs []t045Pub, runner int, reason string) []int {
	var out []int
	for _, p := range pubs {
		if p.Runner == runner && p.Applied && (reason == "" || p.Reason == reason) {
			out = append(out, p.Clock)
		}
	}
	return out
}

// t045AttemptClocks returns the pre-step clocks at which runner idx was SCHEDULED
// (attempt, applied or dropped by the reaction gate) for a reason.
func t045AttemptClocks(pubs []t045Pub, runner int, reason string) []int {
	var out []int
	for _, p := range pubs {
		if p.Runner == runner && (reason == "" || p.Reason == reason) {
			out = append(out, p.Clock)
		}
	}
	return out
}

// t045Diff is the classified comparison of two aligned runs.
type t045Diff struct {
	sharedPubs, gatedPubs             []int
	pubOnlyShared, pubOnlyGated       []int
	maxBallM, maxBallVelM, maxPlayerM float64
	maxVelM                           float64
	positionsEqual, intentsEqual      bool
	obsEqual, ballStateEqual          bool
	eventsEqual, completeEqual        bool
}

// t045DiffTraces compares frames (ball pos+vel+mode+owner, runner pos+vel+intent+
// deadlines, observations) and events, and reports publication-clock differences
// independently of position equality.
func t045DiffTraces(shared, gated *t045Trace) t045Diff {
	sp, gp := shared.stepClocks(), gated.stepClocks()
	d := t045Diff{
		sharedPubs: sp, gatedPubs: gp,
		pubOnlyShared:  t045SetDiff(sp, gp),
		pubOnlyGated:   t045SetDiff(gp, sp),
		positionsEqual: true, intentsEqual: true, obsEqual: true, ballStateEqual: true,
		eventsEqual: true, completeEqual: true,
	}
	if len(shared.frames) != len(gated.frames) {
		d.completeEqual = false
		return d
	}
	for i := range shared.frames {
		a, b := shared.frames[i], gated.frames[i]
		if a.clock != b.clock {
			d.completeEqual = false
		}
		if a.ballX != b.ballX || a.ballY != b.ballY || a.ballVXM != b.ballVXM || a.ballVYM != b.ballVYM || a.mode != b.mode || a.owner != b.owner {
			d.ballStateEqual = false
			d.completeEqual = false
		}
		if bd := DistM(a.ballX, a.ballY, b.ballX, b.ballY); bd > d.maxBallM {
			d.maxBallM = bd
		}
		bvx, bvy := a.ballVXM-b.ballVXM, a.ballVYM-b.ballVYM
		if bv := math.Sqrt(bvx*bvx + bvy*bvy); bv > d.maxBallVelM {
			d.maxBallVelM = bv
		}
		if len(a.obs) != len(b.obs) {
			d.obsEqual = false
			d.completeEqual = false
			continue
		}
		for j := range a.obs {
			if a.hasObs[j] != b.hasObs[j] || a.obs[j] != b.obs[j] {
				d.obsEqual = false
				d.completeEqual = false
			}
		}
		for j := range a.xs {
			if pd := DistM(a.xs[j], a.ys[j], b.xs[j], b.ys[j]); pd > d.maxPlayerM {
				d.maxPlayerM = pd
			}
			dx, dy := a.vxs[j]-b.vxs[j], a.vys[j]-b.vys[j]
			if vd := math.Sqrt(dx*dx + dy*dy); vd > d.maxVelM {
				d.maxVelM = vd
			}
			if a.xs[j] != b.xs[j] || a.ys[j] != b.ys[j] || a.vxs[j] != b.vxs[j] || a.vys[j] != b.vys[j] ||
				a.reactAt[j] != b.reactAt[j] || a.cooldown[j] != b.cooldown[j] {
				d.positionsEqual = false
				d.completeEqual = false
			}
			if a.ix[j] != b.ix[j] || a.iy[j] != b.iy[j] {
				d.intentsEqual = false
				d.completeEqual = false
			}
		}
	}
	if !reflect.DeepEqual(shared.events, gated.events) {
		d.eventsEqual = false
		d.completeEqual = false
	}
	return d
}

// t045SetDiff returns the values of a not present in b, preserving order.
func t045SetDiff(a, b []int) []int {
	inB := make(map[int]bool, len(b))
	for _, v := range b {
		inB[v] = true
	}
	var out []int
	for _, v := range a {
		if !inB[v] {
			out = append(out, v)
		}
	}
	return out
}

// t045EventClockOfKind returns the end-step clock of the first event of kind k.
func t045EventClockOfKind(evs []PassEvent, k PassEventKind) int {
	for _, e := range evs {
		if e.Kind == k {
			return e.ClockMS
		}
	}
	return -1
}

// t045PubClocks returns the publication clocks for one runner with a reason.
func t045PubClocks(pubs []t045Pub, runner int, reason string) []int {
	var out []int
	for _, p := range pubs {
		if p.Runner == runner && (reason == "" || p.Reason == reason) {
			out = append(out, p.Clock)
		}
	}
	return out
}
