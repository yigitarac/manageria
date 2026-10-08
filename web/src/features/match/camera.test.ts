import { describe, expect, it } from "vitest";
import { cameraFrame, easeCamera, timelineJump } from "./camera";

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

describe("timelineJump", () => {
  it("keeps 180x playback smooth but detects timeline scrubs", () => {
    expect(timelineJump(10_000, 13_000)).toBe(false);
    expect(timelineJump(10_000, 16_000)).toBe(true);
    expect(timelineJump(10_000, 2_000)).toBe(true);
    expect(timelineJump(-1, 0)).toBe(false);
  });
});
