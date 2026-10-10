import { describe, expect, it } from "vitest";
import { cameraFrame, easeCamera, trailSampleTimes } from "./camera";

describe("cameraFrame", () => {
  it("keeps the camera inside the pitch at every extreme", () => {
    for (const [bx, by] of [
      [0, 0],
      [1, 0],
      [0, 1],
      [1, 1],
      [0.5, 0.5],
    ] as const) {
      const f = cameraFrame(1200, 800, 1050, 680, bx, by);
      expect(f.x).toBeLessThanOrEqual(0);
      expect(f.y).toBeLessThanOrEqual(0);
      expect(f.x).toBeGreaterThanOrEqual(1200 - 1050 * f.scale);
      expect(f.y).toBeGreaterThanOrEqual(800 - 680 * f.scale);
    }
  });

  it("zooms closer than the god view", () => {
    const god = cameraFrame(1200, 800, 1050, 680, 0.5, 0.5, 1);
    const tv = cameraFrame(1200, 800, 1050, 680, 0.5, 0.5);
    expect(tv.scale).toBeGreaterThan(god.scale);
  });
});

describe("easeCamera", () => {
  it("glides toward the target without overshoot", () => {
    const target = cameraFrame(1200, 800, 1050, 680, 0.9, 0.2);
    const start = cameraFrame(1200, 800, 1050, 680, 0.1, 0.8);
    const step = easeCamera(start, target, 0.5);
    // Closer to the target than before, and never past it.
    expect(Math.abs(step.x - target.x)).toBeLessThan(Math.abs(start.x - target.x));
    expect(step.scale).toBeCloseTo((start.scale + target.scale) / 2, 6);
  });
});

describe("trailSampleTimes", () => {
  it("restarts at the destination on a seek, even when the seek moves forward", () => {
    expect(trailSampleTimes(1000, 30_000, true)).toEqual([30_000]);
    expect(trailSampleTimes(30_000, 1000, true)).toEqual([1000]);
  });

  it("samples a fast playback step without treating it as a seek", () => {
    const times = trailSampleTimes(1000, 4000, false);
    expect(times.length).toBe(50);
    expect(times[0]).toBe(1060);
    expect(times[times.length - 1]).toBe(4000);
  });
});
