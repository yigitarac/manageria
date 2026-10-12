/**
 * Same-clock authoritative interpolation (T-039).
 *
 * The renderer interpolates ONLY between the authoritative knots the exporter
 * emitted. It never borrows flight time, invents a winner/receipt, simulates
 * physics, or retimes ownership: the exporter already placed an exact knot at
 * every event fraction (`exactTimeMs = clockMs - substepMs*(1-frac)`) and
 * interpolated every runner to that fraction, so piecewise-linear interpolation
 * across those knots reproduces the real ball path and the real contact.
 *
 * Ownership/mode come from the LEFT knot, so a control transitions at the event
 * instant and never at the following end-step sample.
 */

import type {
  PrototypePoint,
  PrototypeRunnerState,
  PrototypeState,
  PrototypeTrace,
} from "./types";

/** Returns a defensive copy of a single knot's state. */
function stateOfPoint(point: PrototypePoint): PrototypeState {
  return {
    ballX: point.ballX,
    ballY: point.ballY,
    mode: point.mode,
    owner: point.owner,
    runners: point.runners.map((r) => ({ x: r.x, y: r.y })),
  };
}

/**
 * Collapses knots that share a timestamp onto the LAST one. The exporter appends
 * samples before event knots and events in Seq order, so the last same-time knot
 * is the post-state (highest event Seq); this makes a same-time tie deterministic
 * and removes zero-duration intervals. In particular a fraction-0 control at t0
 * wins over the initial sample, and two simultaneous deflect/control knots keep
 * the later (control) state.
 */
function collapseSameTime(points: PrototypePoint[]): PrototypePoint[] {
  const out: PrototypePoint[] = [];
  for (const point of points) {
    const prev = out[out.length - 1];
    if (prev && point.tMs === prev.tMs) {
      out[out.length - 1] = point;
    } else {
      out.push(point);
    }
  }
  return out;
}

/**
 * Interpolates the authoritative trace at `tMs` (game milliseconds). Before the
 * first knot or after the last it clamps to that boundary; between knots it is
 * piecewise linear, which is exact for the straight ball segments and the
 * per-substep runner samples the exporter emitted. Mode/owner always come from
 * the LEFT (collapsed) knot, so ownership transitions exactly at the event knot.
 */
export function interpolateTrace(trace: PrototypeTrace, tMs: number): PrototypeState | null {
  if (trace.points.length === 0) {
    return null;
  }
  const points = collapseSameTime(trace.points);
  if (tMs <= points[0].tMs) {
    return stateOfPoint(points[0]);
  }
  const lastIndex = points.length - 1;
  if (tMs >= points[lastIndex].tMs) {
    return stateOfPoint(points[lastIndex]);
  }

  // Binary search for the last knot at or before tMs.
  let lo = 0;
  let hi = lastIndex;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (points[mid].tMs <= tMs) {
      lo = mid;
    } else {
      hi = mid;
    }
  }

  const a = points[lo];
  const b = points[hi];
  const span = b.tMs - a.tMs;
  const u = span > 0 ? (tMs - a.tMs) / span : 0;
  const runners: PrototypeRunnerState[] = a.runners.map((ra, i) => {
    const rb = b.runners[i] ?? ra;
    return { x: ra.x + (rb.x - ra.x) * u, y: ra.y + (rb.y - ra.y) * u };
  });

  return {
    ballX: a.ballX + (b.ballX - a.ballX) * u,
    ballY: a.ballY + (b.ballY - a.ballY) * u,
    mode: a.mode,
    owner: a.owner,
    runners,
  };
}

/** Total authoritative duration of the trace in game milliseconds. */
export function traceDurationMs(trace: PrototypeTrace): number {
  const last = trace.points[trace.points.length - 1];
  return last ? last.tMs : 0;
}
