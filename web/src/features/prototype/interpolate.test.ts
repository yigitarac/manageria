import { describe, expect, it } from "vitest";
import { interpolateTrace, traceDurationMs } from "./interpolate";
import { PROTOTYPE_FIXTURE } from "./fixture";
import { isPrototypeTrace, type PrototypePoint, type PrototypeTrace } from "./types";

function point(
  tMs: number,
  kind: string,
  ballX: number,
  ballY: number,
  mode: string,
  owner: number,
  runners: { x: number; y: number }[],
): PrototypePoint {
  return { tMs, kind, ballX, ballY, vxM: 0, vyM: 0, mode, owner, runners };
}

/** A deflect fixture: the ball travels +x, is deflected, then leaves -y. */
function deflectTrace(): PrototypeTrace {
  return {
    ...PROTOTYPE_FIXTURE,
    slug: "deflect-fixture",
    sampleCount: 3,
    frameCount: 4,
    eventCount: 2,
    events: [],
    points: [
      point(0, "sample", 0.3, 0.5, "rolling", -1, [{ x: 0.3, y: 0.5 }]),
      point(20, "sample", 0.5, 0.5, "rolling", -1, [{ x: 0.5, y: 0.5 }]),
      point(25, "deflect", 0.52, 0.5, "rolling", -1, [{ x: 0.5, y: 0.5 }]),
      point(40, "sample", 0.52, 0.4, "rolling", -1, [{ x: 0.52, y: 0.46 }]),
    ],
  };
}

describe("interpolateTrace", () => {
  it("returns null for an empty timeline", () => {
    expect(interpolateTrace({ ...PROTOTYPE_FIXTURE, points: [] }, 0)).toBeNull();
  });

  it("clamps to the first and last authoritative knots", () => {
    expect(interpolateTrace(PROTOTYPE_FIXTURE, -100)?.ballX).toBe(0.3);
    expect(interpolateTrace(PROTOTYPE_FIXTURE, 99999)?.ballX).toBe(0.435);
  });

  it("keeps every sample knot on the substep grid and equal at its own time", () => {
    for (const p of PROTOTYPE_FIXTURE.points) {
      if (p.kind === "sample") {
        expect(p.tMs % PROTOTYPE_FIXTURE.substepMs).toBe(0);
      }
      const at = interpolateTrace(PROTOTYPE_FIXTURE, p.tMs);
      expect(at?.ballX).toBeCloseTo(p.ballX, 9);
      expect(at?.ballY).toBeCloseTo(p.ballY, 9);
      expect(at?.owner).toBe(p.owner);
    }
    expect(traceDurationMs(PROTOTYPE_FIXTURE)).toBe(40);
  });

  it("transitions ownership exactly at the control knot, not at the end-step sample", () => {
    const before = interpolateTrace(PROTOTYPE_FIXTURE, 28 - 0.1);
    const at = interpolateTrace(PROTOTYPE_FIXTURE, 28);
    const after = interpolateTrace(PROTOTYPE_FIXTURE, 28 + 0.1);
    expect(before?.owner).toBe(-1);
    expect(before?.mode).toBe("rolling");
    expect(at?.owner).toBe(0);
    expect(at?.mode).toBe("held");
    expect(after?.owner).toBe(0);
    // The interpolated runner is placed at the event fraction too.
    expect(at?.runners[0].x).toBeCloseTo(0.585, 9);
    expect(Math.abs((before?.runners[0].x ?? 0) - 0.585)).toBeGreaterThan(1e-6);
  });

  it("does not interpolate straight across a deflect knot", () => {
    const trace = deflectTrace();
    const at30 = interpolateTrace(trace, 30);
    // Actual path after the deflect goes -y; a naive t20->t40 straight line would
    // give a very different point.
    expect(at30?.ballX).toBeCloseTo(0.52, 9);
    expect(at30?.ballY).toBeCloseTo(0.5 + (0.4 - 0.5) * ((30 - 25) / 15), 9);
    const naiveY = 0.5 + (0.4 - 0.5) * ((30 - 20) / 20);
    expect(at30?.ballY).not.toBeCloseTo(naiveY, 3);
  });

  it("stays stateless and coherent under seek, reverse and warp queries", () => {
    const query = [0, 8, 40, 28, 5, 100, -5, 28, 20, 39.999];
    for (const t of query) {
      const a = interpolateTrace(PROTOTYPE_FIXTURE, t);
      const b = interpolateTrace(PROTOTYPE_FIXTURE, t);
      expect(a).not.toBeNull();
      expect(Number.isFinite(a?.ballX)).toBe(true);
      expect(Number.isFinite(a?.ballY)).toBe(true);
      expect(a).toEqual(b);
    }
    // Reversing the direction of the query must not change the answers.
    const forward = query.map((t) => interpolateTrace(PROTOTYPE_FIXTURE, t)?.ballX);
    const backward = [...query].reverse().map((t) => interpolateTrace(PROTOTYPE_FIXTURE, t)?.ballX);
    expect(backward).toEqual([...forward].reverse());
  });

  it("guards the loaded trace shape", () => {
    expect(isPrototypeTrace(PROTOTYPE_FIXTURE)).toBe(true);
    expect(isPrototypeTrace(JSON.parse(JSON.stringify(PROTOTYPE_FIXTURE)))).toBe(true);
    expect(isPrototypeTrace({ points: [] })).toBe(false);
    expect(isPrototypeTrace(null)).toBe(false);
  });
});

/** Builds a valid trace wrapper around explicit points (interpolate only). */
function traceWithPoints(points: PrototypePoint[]): PrototypeTrace {
  return {
    ...PROTOTYPE_FIXTURE,
    slug: "same-time-fixture",
    runnerCount: points[0]?.runners.length ?? 1,
    sampleCount: points.filter((p) => p.kind === "sample").length,
    frameCount: points.length,
    eventCount: 0,
    events: [],
    points,
  };
}

describe("interpolateTrace same-time knots", () => {
  it("chooses the rightmost knot at t0 (fraction-0 control beats the initial sample)", () => {
    const trace = traceWithPoints([
      point(0, "sample", 0.3, 0.5, "rolling", -1, [{ x: 0.62, y: 0.5 }]),
      point(0, "control", 0.42, 0.5, "held", 0, [{ x: 0.585, y: 0.5 }]),
    ]);
    const at0 = interpolateTrace(trace, 0);
    expect(at0?.owner).toBe(0);
    expect(at0?.mode).toBe("held");
    expect(at0?.ballX).toBeCloseTo(0.42, 9);
  });

  it("keeps the last state for two simultaneous deflect/control knots", () => {
    const trace = traceWithPoints([
      point(0, "sample", 0.3, 0.5, "rolling", -1, [{ x: 0.3, y: 0.5 }]),
      point(20, "sample", 0.5, 0.5, "rolling", -1, [{ x: 0.5, y: 0.5 }]),
      point(20, "deflect", 0.52, 0.5, "rolling", -1, [{ x: 0.5, y: 0.5 }]),
      point(20, "control", 0.53, 0.5, "held", 0, [{ x: 0.5, y: 0.5 }]),
      point(40, "sample", 0.6, 0.5, "held", 0, [{ x: 0.6, y: 0.5 }]),
    ]);
    const before = interpolateTrace(trace, 19.9);
    const at = interpolateTrace(trace, 20);
    const after = interpolateTrace(trace, 20.1);
    expect(before?.owner).toBe(-1);
    expect(at?.owner).toBe(0);
    expect(after?.owner).toBe(0);
    expect(at?.mode).toBe("held");
    // No zero-duration artifact: the interval 20->40 is well defined.
    expect(Number.isFinite(after?.ballX)).toBe(true);
  });

  it("is coherent at the exact knot and on both sides for a fractional control", () => {
    const at = interpolateTrace(PROTOTYPE_FIXTURE, 28);
    expect(at?.ballX).toBeCloseTo(0.42, 9);
    expect(at?.owner).toBe(0);
    expect(interpolateTrace(PROTOTYPE_FIXTURE, 28 - 1e-6)?.owner).toBe(-1);
    expect(interpolateTrace(PROTOTYPE_FIXTURE, 28 + 1e-6)?.owner).toBe(0);
  });
});
