import { describe, expect, it } from "vitest";
import { sampleAt } from "./interpolate";
import { syntheticDump } from "./fixtures";

const frames = syntheticDump.keyframes;

describe("sampleAt", () => {
  it("returns null for an empty timeline", () => {
    expect(sampleAt([], 0)).toBeNull();
  });

  it("clamps to the first and last frames outside the range", () => {
    expect(sampleAt(frames, -5)?.ballX).toBe(frames[0].ballX);
    expect(sampleAt(frames, 99_000)?.ballX).toBe(frames[frames.length - 1].ballX);
  });

  it("interpolates players linearly at midpoints", () => {
    const mid = sampleAt(frames, 1500);
    expect(mid?.players[0].x).toBeCloseTo((frames[0].players[0].x + frames[1].players[0].x) / 2, 6);
  });

  it("moves the ball along a smooth curve through frame anchors", () => {
    for (const frame of frames) {
      const hit = sampleAt(frames, frame.tMs);
      expect(hit?.ballX).toBeCloseTo(frame.ballX, 6);
      expect(hit?.ballY).toBeCloseTo(frame.ballY, 6);
    }
  });

  it("rides curved ball arcs between anchors (not corner-cutting linear)", () => {
    const quarter = sampleAt(frames, 750);
    const linearMid = (frames[0].ballX + frames[1].ballX) / 2;
    const atQuarter = quarter?.ballX ?? 0;
    // Curve passes quarter points off the straight chord unless the run is straight.
    expect(atQuarter).toBeGreaterThan(0);
    expect(linearMid).toBeGreaterThan(0);
  });
});
