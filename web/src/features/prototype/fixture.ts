import type { PrototypeTrace } from "./types";

/**
 * A tiny CHECKED-IN fixture that mirrors the `liveplaydemo` schema. Tests use it
 * directly so they never depend on the gitignored generated clips in
 * `web/public/prototype/` (whose absence is a regeneration problem, not a test
 * failure). It contains two runners and one control knot at tMs = 28
 * (clockMs 40, frac 0.4) so ownership/interpolation contracts can be pinned.
 */
export const PROTOTYPE_FIXTURE: PrototypeTrace = {
  schemaVersion: 1,
  timeUnit: "ms",
  substepMs: 20,
  seed: 1,
  slug: "fixture",
  label: "Fixture",
  description: "Tiny checked-in fixture for the interpolation contract.",
  scriptedNote: "The initial release is a scripted fixture action.",
  pitch: { lengthM: 105, widthM: 68 },
  runnerCount: 2,
  sampleCount: 3,
  frameCount: 4,
  eventCount: 2,
  events: [
    {
      seq: 0,
      kind: "release",
      clockMs: 0,
      frac: 0,
      exactTimeMs: 0,
      runner: -1,
      ballX: 0.3,
      ballY: 0.5,
      vxM: 14,
      vyM: 0,
      outVxM: 0,
      outVyM: 0,
      runnerX: 0,
      runnerY: 0,
      scripted: true,
    },
    {
      seq: 1,
      kind: "control",
      clockMs: 40,
      frac: 0.4,
      exactTimeMs: 28,
      runner: 0,
      ballX: 0.42,
      ballY: 0.5,
      vxM: 13.96,
      vyM: 0,
      outVxM: 0,
      outVyM: 0,
      runnerX: 0.585,
      runnerY: 0.5,
      scripted: false,
    },
  ],
  points: [
    {
      tMs: 0,
      kind: "sample",
      ballX: 0.3,
      ballY: 0.5,
      vxM: 14,
      vyM: 0,
      mode: "rolling",
      owner: -1,
      runners: [
        { x: 0.62, y: 0.5 },
        { x: 0.45, y: 0.46 },
      ],
    },
    {
      tMs: 20,
      kind: "sample",
      ballX: 0.38,
      ballY: 0.5,
      vxM: 13.98,
      vyM: 0,
      mode: "rolling",
      owner: -1,
      runners: [
        { x: 0.6, y: 0.5 },
        { x: 0.455, y: 0.46 },
      ],
    },
    {
      tMs: 28,
      kind: "control",
      ballX: 0.42,
      ballY: 0.5,
      vxM: 13.96,
      vyM: 0,
      mode: "held",
      owner: 0,
      runners: [
        { x: 0.585, y: 0.5 },
        { x: 0.46, y: 0.46 },
      ],
    },
    {
      tMs: 40,
      kind: "sample",
      ballX: 0.435,
      ballY: 0.5,
      vxM: 0,
      vyM: 0,
      mode: "held",
      owner: 0,
      runners: [
        { x: 0.6, y: 0.5 },
        { x: 0.47, y: 0.46 },
      ],
    },
  ],
};
