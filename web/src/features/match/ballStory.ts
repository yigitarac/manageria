import { sampleAt } from "./interpolate";
import type { MatchDump, Touch } from "./types";

/**
 * Ball story: the match told through the ball itself (ADR-0011 spirit), built from
 * the engine's touch ledger + goal events so the animation IS the match.
 *
 * The engine resolves actions atomically — a pass can be struck and collected in the
 * very same ledger tick — but the viewer must never teleport the ball. So the story
 * is re-timed elastically: every displaced leg takes physical flight time and borrows
 * the idle slack that follows inside its ledger window (same-tick cascades spill a
 * little and the next generous window absorbs the drift). Between actions the ball
 * rests at its holder's feet — but only when the keyframe player is actually there:
 * the ledger's atomic contests sometimes award the ball to a distant winner, and the
 * ball then stays at the ledger spot instead of teleporting to a far pair of feet.
 * Dead-ball placement is carried out of view; live rebounds, knockdowns and
 * clearances fly visibly.
 */

export interface BallEpisode {
  t0: number;
  t1: number;
  kind: "pass" | "carry" | "duel" | "loose" | "shot" | "goal" | "idle" | "deadBall";
  /** Frozen endpoints — the fallback path when no player samples are supplied. */
  x0: number;
  y0: number;
  x1: number;
  y1: number;
  /** The ball rests at this player's live feet for the whole episode. */
  holdSlot?: number;
  /** A flight that aims at this player's live feet (he runs onto the ball). */
  aimSlot?: number;
}

const FLIGHT_BASE_MS = 420;
const FLIGHT_PER_M_MS = 22;
const MIN_LEG_MS = 380; // a visible displacement always takes real time — the no-teleport floor
const MAX_LEG_MS = 2600; // elastic ceiling (hoofed clearances hang longest)
const GOAL_FLIGHT_MS = 1100;
const NET_HOLD_MS = 900;
const REBOUND_MS = 2000; // a nearer next touch is live play (save, parry), not a restart
const HANDOVER_MIN_M = 0.5; // below this a "transfer" is just a touch between two players
const FOOT_PROXIMITY_M = 10; // beyond this the ledger touch and the player are different facts

export function buildBallStory(dump: MatchDump): BallEpisode[] {
  const homeClub = dump.teams.home.club;
  const goalsByTick = new Map<number, number>(); // tick → scoring team
  for (const ev of dump.events) {
    if (ev.kind === "goal") goalsByTick.set(ev.tick, ev.club === homeClub ? 0 : 1);
  }

  const touches = [...dump.touches].sort((a, b) => a.tick - b.tick);
  const outcomes = new Map(dump.chains.map((chain) => [chain.id, chain.outcome]));
  const lastFrame = dump.keyframes[dump.keyframes.length - 1];
  const endOfTime = lastFrame
    ? lastFrame.tMs
    : touches.length
      ? touches[touches.length - 1].tick * 1000
      : 0;

  interface Anchor {
    slot?: number;
    x: number;
    y: number;
  }
  const resolve = (anchor: Anchor, tMs: number) => {
    if (anchor.slot !== undefined) {
      const p = sampleAt(dump.keyframes, tMs)?.players[anchor.slot];
      if (p) return { x: p.x, y: p.y };
    }
    return { x: anchor.x, y: anchor.y };
  };
  /**
   * The ledger touch becomes a foot-glued anchor only when its player is actually
   * there; instant ghost contests keep their ledger spot (no teleporting to feet).
   */
  const anchorOf = (t: Touch): Anchor => {
    const base: Anchor = { x: t.x, y: t.y };
    const slot = t.actor >= 0 && t.actor < 11 ? t.team * 11 + t.actor : undefined;
    if (slot === undefined) return base;
    const feet = sampleAt(dump.keyframes, t.tick * 1000)?.players[slot];
    if (feet && metresBetween(feet.x, feet.y, t.x, t.y) > FOOT_PROXIMITY_M) return base;
    return { slot, ...base };
  };

  const episodes: BallEpisode[] = [];
  let cursor = 0; // story time already committed (episode ends tile exactly)
  let anchor: Anchor = touches.length ? anchorOf(touches[0]) : { x: 0.5, y: 0.5 };

  /** A flight that leaves one anchor and lands at the target's position. */
  const fly = (start: number, to: Anchor, travelM: number, kind: "pass" | "duel" | "loose") => {
    const p0 = resolve(anchor, start);
    const end = start + flightMs(travelM);
    const p1 = resolve(to, end);
    episodes.push({
      t0: start,
      t1: end,
      kind,
      x0: p0.x,
      y0: p0.y,
      x1: p1.x,
      y1: p1.y,
      aimSlot: to.slot,
    });
    cursor = end;
    anchor = to;
  };

  /** The ball lives at its holder's position until the next whistle or touch. */
  const hold = (start: number, end: number, holder: Anchor, kind: "carry" | "idle") => {
    if (end <= start) return;
    const p0 = resolve(holder, start);
    const p1 = resolve(holder, end);
    episodes.push({
      t0: start,
      t1: end,
      kind,
      x0: p0.x,
      y0: p0.y,
      x1: p1.x,
      y1: p1.y,
      holdSlot: holder.slot,
    });
    cursor = end;
    anchor = holder;
  };

  /** Dead-ball placement: the ball is carried to the restart spot out of view. */
  const place = (start: number, end: number, from: { x: number; y: number }, to: Anchor) => {
    if (end <= start) return;
    episodes.push({
      t0: start,
      t1: end,
      kind: "deadBall",
      x0: from.x,
      y0: from.y,
      x1: to.x,
      y1: to.y,
    });
    cursor = end;
    anchor = to;
  };

  for (let i = 0; i < touches.length; i++) {
    const cur = touches[i];
    const next = touches[i + 1];
    const tCur = cur.tick * 1000;
    const tNext = next ? next.tick * 1000 : endOfTime;
    const gapMs = next ? tNext - tCur : Number.POSITIVE_INFINITY;
    const start = Math.max(tCur, cursor);
    const to = next ? anchorOf(next) : anchorOf(cur);

    if (cur.kind === "shot") {
      const scored =
        (goalsByTick.get(cur.tick) ??
          goalsByTick.get(cur.tick + 1) ??
          goalsByTick.get(cur.tick - 1)) === cur.team;
      const goalX = cur.team === 0 ? 0.985 : 0.015;
      const netX = cur.team === 0 ? 1.005 : -0.005;
      const landX = scored ? netX : goalX;
      const landing: Anchor = { x: landX, y: 0.5 };
      const p0 = resolve(anchor, start);
      const travelM = metresBetween(p0.x, p0.y, landX, 0.5);
      const flightEnd = start + Math.max(GOAL_FLIGHT_MS, flightMs(travelM));
      episodes.push({
        t0: start,
        t1: flightEnd,
        kind: scored ? "goal" : "shot",
        x0: p0.x,
        y0: p0.y,
        x1: landX,
        y1: 0.5,
      });
      cursor = flightEnd;
      anchor = landing;
      if (scored) hold(cursor, cursor + NET_HOLD_MS, landing, "idle"); // the net ripples

      if (!next) {
        hold(cursor, endOfTime, landing, "idle");
        continue;
      }
      // Near continuations are live play (saves, parries); wider gaps mean the ball
      // went out or into the net and its placement is off camera.
      if (gapMs > REBOUND_MS) {
        place(cursor, Math.max(tNext, cursor), landing, to);
        continue;
      }
      const reboundM = metresBetween(
        landing.x,
        landing.y,
        resolve(to, cursor).x,
        resolve(to, cursor).y,
      );
      if (reboundM < HANDOVER_MIN_M) {
        hold(cursor, Math.max(tNext, cursor), to, "idle");
      } else {
        fly(cursor, to, reboundM, "loose");
        hold(cursor, Math.max(tNext, cursor), to, "idle");
      }
      continue;
    }

    if (!next) {
      // Full time with the ball at somebody's feet.
      hold(start, endOfTime, anchorOf(cur), "carry");
      continue;
    }

    const sameMove = next.chain === cur.chain && next.team === cur.team;
    const outcome = outcomes.get(cur.chain);
    const travel = resolve(anchor, start);
    const dest = resolve(to, start);
    const travelM = metresBetween(travel.x, travel.y, dest.x, dest.y);

    if (!sameMove && outcome !== "turnover") {
      // Stoppage: the ball is carried to the restart spot out of view and the story
      // resumes at the taker's boots.
      place(start, Math.max(tNext, start), travel, to);
      continue;
    }

    if (sameMove && cur.kind === "carry" && anchor.slot !== undefined && anchor.slot === to.slot) {
      hold(start, Math.max(tNext, start), anchorOf(cur), "carry");
      continue;
    }

    if (travelM < HANDOVER_MIN_M) {
      // A takeover, not travel: the ball stays in the same pocket at the winner's feet.
      hold(start, Math.max(tNext, start), to, "idle");
      continue;
    }

    const kind: "pass" | "duel" | "loose" = sameMove
      ? cur.kind === "pass"
        ? "pass"
        : "duel"
      : "loose"; // a lost ball is never drawn as a pass to the rival
    fly(start, to, travelM, kind);
    hold(cursor, Math.max(tNext, cursor), to, "idle");
  }

  return episodes;
}

/** Samples the ball path at playback time (null = before the first touch). */
export function ballAt(
  episodes: BallEpisode[],
  tMs: number,
  players?: { x: number; y: number; state: number }[],
): { x: number; y: number; visible: boolean } | null {
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
  const ep = episodes[found];
  if (tMs > ep.t1) return null;
  const span = ep.t1 - ep.t0;
  const u = span > 0 ? (tMs - ep.t0) / span : 1;

  if (ep.kind === "deadBall") {
    return { x: ep.x0, y: ep.y0, visible: false }; // placement happens off camera
  }
  const slot = ep.holdSlot ?? ep.aimSlot;
  const feet = slot !== undefined ? players?.[slot] : undefined;
  if (ep.holdSlot !== undefined && feet) {
    return { x: feet.x, y: feet.y, visible: true }; // glued to its holder
  }
  const aim = feet ?? { x: ep.x1, y: ep.y1 };
  // Deliveries and loose balls arc; shots fly flat and fast.
  const bow =
    ep.kind === "pass" ? 0.02 : ep.kind === "loose" ? 0.03 : ep.kind === "duel" ? 0.012 : 0;
  return {
    x: lerp(ep.x0, aim.x, u),
    y: lerp(ep.y0, aim.y, u) - Math.sin(Math.PI * u) * bow,
    visible: true,
  };
}

function flightMs(distM: number): number {
  return Math.max(MIN_LEG_MS, Math.min(MAX_LEG_MS, FLIGHT_BASE_MS + distM * FLIGHT_PER_M_MS));
}

function metresBetween(ax: number, ay: number, bx: number, by: number): number {
  return Math.hypot((bx - ax) * 105, (by - ay) * 68);
}

function lerp(a: number, b: number, u: number): number {
  return a + (b - a) * u;
}
