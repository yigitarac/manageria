import type { Keyframe } from "./types";

export interface FrameSample {
  ballX: number;
  ballY: number;
  /** 22 normalized positions (home 0..10, away 11..21). */
  players: { x: number; y: number; state: number }[];
}

/**
 * Samples the timeline at time tMs. Players move linearly between keyframes;
 * the ball rides a Catmull-Rom curve so passes arc instead of teleporting
 * (design note: client-side interpolation).
 */
export function sampleAt(frames: Keyframe[], tMs: number): FrameSample | null {
  if (frames.length === 0) return null;
  if (tMs <= frames[0].tMs) return frameToSample(frames[0]);
  const last = frames[frames.length - 1];
  if (tMs >= last.tMs) return frameToSample(last);

  const hi = upperBound(frames, tMs);
  const lo = hi - 1;
  const a = frames[lo];
  const b = frames[hi];
  const span = b.tMs - a.tMs;
  const u = span > 0 ? (tMs - a.tMs) / span : 0;

  const players = a.players.map((pa, i) => {
    const pb = b.players[i] ?? pa;
    return {
      x: lerp(pa.x, pb.x, u),
      y: lerp(pa.y, pb.y, u),
      state: u < 0.5 ? pa.state : pb.state,
    };
  });

  return {
    ballX: catmullRom(
      axisPoint(frames, lo - 1, "ballX"),
      a.ballX,
      b.ballX,
      axisPoint(frames, hi + 1, "ballX"),
      u,
    ),
    ballY: catmullRom(
      axisPoint(frames, lo - 1, "ballY"),
      a.ballY,
      b.ballY,
      axisPoint(frames, hi + 1, "ballY"),
      u,
    ),
    players,
  };
}

function frameToSample(frame: Keyframe): FrameSample {
  return {
    ballX: frame.ballX,
    ballY: frame.ballY,
    players: frame.players.map((p) => ({ x: p.x, y: p.y, state: p.state })),
  };
}

function lerp(a: number, b: number, u: number): number {
  return a + (b - a) * u;
}

/** Classic Catmull-Rom value for one segment (p1 → p2 at u). */
function catmullRom(p0: number, p1: number, p2: number, p3: number, u: number): number {
  const u2 = u * u;
  const u3 = u2 * u;
  return (
    0.5 *
    (2 * p1 +
      (-p0 + p2) * u +
      (2 * p0 - 5 * p1 + 4 * p2 - p3) * u2 +
      (-p0 + 3 * p1 - 3 * p2 + p3) * u3)
  );
}

function axisPoint(frames: Keyframe[], index: number, axis: "ballX" | "ballY"): number {
  const frame = frames[Math.max(0, Math.min(frames.length - 1, index))];
  return frame[axis];
}

/** First index whose tMs is strictly greater than t. */
function upperBound(frames: Keyframe[], tMs: number): number {
  let lo = 0;
  let hi = frames.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (frames[mid].tMs <= tMs) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}
