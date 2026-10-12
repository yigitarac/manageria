import { describe, expect, it } from "vitest";
import exportedFixture from "./exported-fixture.json";
import { PROTOTYPE_FIXTURE } from "./fixture";
import { isPrototypeTrace } from "./types";

/** Deep clone through JSON so mutations cannot leak into the shared fixture. */
function clone(): Record<string, unknown> {
  return JSON.parse(JSON.stringify(PROTOTYPE_FIXTURE)) as Record<string, unknown>;
}

describe("isPrototypeTrace", () => {
  it("accepts the checked-in fixture, its JSON clone and the real exported fixture", () => {
    expect(isPrototypeTrace(PROTOTYPE_FIXTURE)).toBe(true);
    expect(isPrototypeTrace(JSON.parse(JSON.stringify(PROTOTYPE_FIXTURE)))).toBe(true);
    expect(isPrototypeTrace(exportedFixture)).toBe(true);
  });

  it("rejects non-objects and empty shapes without throwing", () => {
    for (const value of [null, undefined, 0, "x", [], {}, { points: [], events: [] }]) {
      expect(() => isPrototypeTrace(value)).not.toThrow();
      expect(isPrototypeTrace(value)).toBe(false);
    }
  });

  it("rejects the unsafe minimal object the old guard accepted", () => {
    const unsafe = {
      schemaVersion: 999,
      points: [{}],
      events: [],
      pitch: { lengthM: 105, widthM: 68 },
    };
    expect(isPrototypeTrace(unsafe)).toBe(false);
  });

  it("rejects an unsupported schema version or time unit", () => {
    const version = clone();
    version.schemaVersion = 2;
    expect(isPrototypeTrace(version)).toBe(false);
    const unit = clone();
    unit.timeUnit = "s";
    expect(isPrototypeTrace(unit)).toBe(false);
  });

  it("rejects non-positive / non-finite substep and pitch", () => {
    for (const bad of [0, -20, Number.NaN, Number.POSITIVE_INFINITY]) {
      const trace = clone();
      trace.substepMs = bad;
      expect(isPrototypeTrace(trace)).toBe(false);
    }
    const pitch = clone();
    pitch.pitch = { lengthM: -105, widthM: 68 };
    expect(isPrototypeTrace(pitch)).toBe(false);
    const nan = clone();
    nan.pitch = { lengthM: Number.NaN, widthM: 68 };
    expect(isPrototypeTrace(nan)).toBe(false);
  });

  it("rejects a point with the wrong runner count, bad coords, mode or owner", () => {
    const mismatch = clone();
    (mismatch.points as Record<string, unknown>[])[0].runners = [{ x: 0, y: 0 }];
    expect(isPrototypeTrace(mismatch)).toBe(false);

    const coord = clone();
    ((coord.points as Record<string, unknown>[])[0].runners as Record<string, unknown>[])[0].x =
      Number.NaN;
    expect(isPrototypeTrace(coord)).toBe(false);

    const mode = clone();
    (mode.points as Record<string, unknown>[])[0].mode = "flying";
    expect(isPrototypeTrace(mode)).toBe(false);

    const owner = clone();
    (owner.points as Record<string, unknown>[])[0].owner = 99;
    expect(isPrototypeTrace(owner)).toBe(false);
  });

  it("rejects non-monotonic point times", () => {
    const trace = clone();
    const points = trace.points as Record<string, unknown>[];
    points[0].tMs = 1000;
    expect(isPrototypeTrace(trace)).toBe(false);
  });

  it("rejects a malformed event", () => {
    const missing = clone();
    (missing.events as Record<string, unknown>[])[0] = { seq: 0 };
    expect(isPrototypeTrace(missing)).toBe(false);

    const kind = clone();
    (kind.events as Record<string, unknown>[])[0].kind = "teleport";
    expect(isPrototypeTrace(kind)).toBe(false);

    const frac = clone();
    (frac.events as Record<string, unknown>[])[0].frac = 1.5;
    expect(isPrototypeTrace(frac)).toBe(false);

    const nan = clone();
    (nan.events as Record<string, unknown>[])[0].ballX = Number.POSITIVE_INFINITY;
    expect(isPrototypeTrace(nan)).toBe(false);
  });
});
