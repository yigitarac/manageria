import type { MatchDump } from "./types";

/**
 * Ball story: the match told through the ball itself. Built from the engine's touch
 * ledger + goal events so the animation IS the match (ADR-0011 spirit): passes arc
 * from touch to touch, shots fly at goal, goals nestle into the net. Sparse keyframes
 * only back-fill idle periods — they no longer invent motion between actions.
 */

export interface BallEpisode {
  t0: number;
  t1: number;
  kind: "pass" | "carry" | "duel" | "regain" | "shot" | "goal" | "idle";
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

const FLIGHT_MS = 1100;
const HOLD_MS = 900;
const GAP_CAP_MS = 4500;

export function buildBallStory(dump: MatchDump): BallEpisode[] {
  const homeClub = dump.teams.home.club;
  const goalsByTick = new Map<number, boolean>(); // tick → home scored
  for (const ev of dump.events) {
    if (ev.kind === "goal") goalsByTick.set(ev.tick, ev.club === homeClub);
  }

  const touches = [...dump.touches].sort((a, b) => a.tick - b.tick);
  const episodes: BallEpisode[] = [];

  for (let i = 0; i < touches.length; i++) {
    const cur = touches[i];
    const next = touches[i + 1];
    const t0 = cur.tick * 1000;

    if (cur.kind === "shot") {
      const homeScored =
        goalsByTick.get(cur.tick) ?? goalsByTick.get(cur.tick + 1) ?? goalsByTick.get(cur.tick - 1);
      const goalX = cur.team === 0 ? 0.985 : 0.015;
      const kind: BallEpisode["kind"] = homeScored !== undefined ? "goal" : "shot";
      const netX = cur.team === 0 ? 1.005 : -0.005;
      episodes.push({
        t0,
        t1: t0 + FLIGHT_MS,
        kind,
        x0: cur.x,
        y0: cur.y,
        x1: homeScored !== undefined ? netX : goalX,
        y1: 0.5,
      });
      if (next && (next.tick + 1) * 1000 < t0 + FLIGHT_MS + HOLD_MS) {
        // Following action (goal kick / restart) begins after the ball settles.
        continue;
      }
      episodes.push({
        t0: t0 + FLIGHT_MS,
        t1: t0 + FLIGHT_MS + HOLD_MS,
        kind: "idle",
        x0: homeScored !== undefined ? netX : goalX,
        y0: 0.5,
        x1: homeScored !== undefined ? netX : goalX,
        y1: 0.5,
      });
      continue;
    }

    if (!next) break;
    const tNext = next.tick * 1000;
    const gap = tNext - t0;
    if (gap > GAP_CAP_MS) {
      // Dead ball: the ball WALKS to the restart spot (ball-boy physics, no teleport).
      episodes.push({ t0, t1: tNext, kind: "idle", x0: cur.x, y0: cur.y, x1: next.x, y1: next.y });
      continue;
    }
    const flight = Math.min(gap, FLIGHT_MS);
    const kind: BallEpisode["kind"] =
      cur.kind === "pass" || cur.kind === "carry" || cur.kind === "duel" || cur.kind === "regain"
        ? cur.kind === "carry"
          ? "carry"
          : "pass"
        : "pass";
    episodes.push({ t0, t1: t0 + flight, kind, x0: cur.x, y0: cur.y, x1: next.x, y1: next.y });
    if (t0 + flight < tNext) {
      episodes.push({
        t0: t0 + flight,
        t1: tNext,
        kind: "idle",
        x0: next.x,
        y0: next.y,
        x1: next.x,
        y1: next.y,
      });
    }
  }
  return episodes;
}

/** Samples the ball path at playback time (null = fall back to keyframes). */
export function ballAt(episodes: BallEpisode[], tMs: number): { x: number; y: number } | null {
  let lo = 0;
  let hi = episodes.length - 1;
  let found = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (episodes[mid].t0 <= tMs) {
      found = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  if (found < 0) return null;
  for (let i = found; i < episodes.length && episodes[i].t0 <= tMs; i++) {
    const ep = episodes[i];
    if (tMs > ep.t1) continue;
    const span = ep.t1 - ep.t0;
    const u = span > 0 ? (tMs - ep.t0) / span : 1;
    // Passes arc (sine bow); carries and flights are straight.
    const bow = ep.kind === "pass" ? Math.sin(Math.PI * u) * 0.02 : 0;
    return { x: lerp(ep.x0, ep.x1, u), y: lerp(ep.y0, ep.y1, u) - bow };
  }
  return null;
}

function lerp(a: number, b: number, u: number): number {
  return a + (b - a) * u;
}
