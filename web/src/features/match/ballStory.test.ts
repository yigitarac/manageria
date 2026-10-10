import { describe, expect, it } from "vitest";
import { ballAt, buildBallStory } from "./ballStory";
import { playersAt, syntheticDump } from "./fixtures";
import { sampleAt } from "./interpolate";
import type { BallEpisode } from "./ballStory";
import type { MatchDump, Touch } from "./types";

function touch(t: Partial<Touch> & { tick: number }): Touch {
  return {
    chain: 1,
    team: 0,
    kind: "pass",
    actor: 0,
    target: -1,
    success: true,
    x: 0.5,
    y: 0.5,
    ...t,
  };
}

/** Runs the keyframe runway past the last touch so restarts and tails stay covered. */
function runway(dump: MatchDump, endMs: number): MatchDump {
  return {
    ...dump,
    keyframes: [
      ...dump.keyframes,
      { tMs: endMs, ballX: 0.5, ballY: 0.5, ballOwner: -1, players: playersAt(0.5, 0.5) },
    ],
  };
}

/** The no-teleport contract: exact tiling, timed displacements, bounded speed. */
function expectCoherent(dump: MatchDump, episodes: BallEpisode[]) {
  // Neighbouring episodes tile exactly — no teleport-worthy gaps between them.
  for (let i = 1; i < episodes.length; i++) {
    expect(episodes[i].t0).toBe(episodes[i - 1].t1);
  }
  // Every displaced leg takes physical time (the atomic-ledger regression).
  for (const ep of episodes) {
    const dist = Math.hypot((ep.x1 - ep.x0) * 105, (ep.y1 - ep.y0) * 68);
    if (dist > 0.5) {
      expect(ep.t1 - ep.t0).toBeGreaterThanOrEqual(380);
    }
  }
  // Sampling the rendered ball every 50 ms never teleports it.
  const first = episodes[0].t0;
  const last = episodes[episodes.length - 1].t1;
  let prev: { x: number; y: number } | null = null;
  for (let t = first; t <= last; t += 50) {
    const players = sampleAt(dump.keyframes, t)?.players;
    const b = ballAt(episodes, t, players);
    expect(b).not.toBeNull(); // covered: no fallback flicker inside the story window
    if (!b) continue;
    if (prev && b.visible) {
      const dm = Math.hypot((b.x - prev.x) * 105, (b.y - prev.y) * 68);
      expect(dm).toBeLessThanOrEqual(2.5); // below a thrown ball's pace (50 ms step)
    }
    prev = b.visible ? b : null;
  }
}

describe("ballStory", () => {
  it("draws pass arcs between consecutive touches (the ball IS the story)", () => {
    const episodes = buildBallStory(syntheticDump);
    expect(episodes.length).toBeGreaterThan(1);
    const mid = ballAt(episodes, episodes[0].t0 + (episodes[0].t1 - episodes[0].t0) / 2);
    expect(mid).not.toBeNull();
    // The midpoint of a pass bows upward (arc), not a straight teleport.
    expect(mid!.y).toBeLessThan((episodes[0].y0 + episodes[0].y1) / 2);
  });

  it("flies shots into the net when the ledger says goal", () => {
    const dump = {
      ...syntheticDump,
      touches: [touch({ tick: 20, kind: "shot", actor: 1, x: 0.9, y: 0.4 })],
      events: [{ tick: 20, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    const episodes = buildBallStory(dump);
    const goal = episodes.find((e) => e.kind === "goal");
    expect(goal).toBeDefined();
    expect(goal!.x1).toBeCloseTo(1.005, 6); // ball nestles in the home-side net
  });

  it("cuts softly to the restart after a goal and resumes at the taker's boots", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 20, kind: "shot", actor: 9, x: 0.9, y: 0.5 }),
          touch({ tick: 56, chain: 2, team: 1, kind: "pass", actor: 6, target: 7, x: 0.5, y: 0.5 }),
        ],
        events: [
          { tick: 20, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" },
        ],
      },
      60_000,
    );
    const episodes = buildBallStory(dump);
    expect(ballAt(episodes, 21_500)?.x).toBeCloseTo(1.005, 3); // resting in the net
    // The ~50 m goal reset is a broadcast cut: one short faded dip, then the ball
    // rests in view near the restart spot — never a 50 m blinking teleport.
    let hiddenRun = 0;
    let longestHidden = 0;
    for (let t = 22_000; t <= 56_000; t += 25) {
      const b = ballAt(episodes, t, sampleAt(dump.keyframes, t)?.players);
      if (b && !b.visible) {
        hiddenRun += 25;
        longestHidden = Math.max(longestHidden, hiddenRun);
      } else {
        hiddenRun = 0;
      }
    }
    expect(longestHidden).toBeGreaterThan(0); // the cut does dip out…
    expect(longestHidden).toBeLessThanOrEqual(1200); // …≈2 painted frames at 30×, never a blink
    expect(ballAt(episodes, 30_000)?.visible).toBe(true); // back in view before the restart
    const nearSpot = sampleAt(dump.keyframes, 55_900)!;
    const rested = ballAt(episodes, 55_900, nearSpot.players)!;
    expect(rested.visible).toBe(true);
    expect(Math.abs(rested.x - 0.5)).toBeLessThan(0.2); // resting at the centre-spot end
    const resumed = sampleAt(dump.keyframes, 56_500);
    const b = ballAt(episodes, 56_500, resumed!.players)!;
    expect(b.visible).toBe(true); // the restart strike is on camera
    expect(b.x).toBeCloseTo(resumed!.players[17].x, 6); // away player 6 has the ball
  });

  it("rolls the ball visibly to the restart spot after a saved shot", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 20, kind: "shot", actor: 9, success: false, x: 0.8, y: 0.4 }),
          touch({
            tick: 30,
            chain: 2,
            team: 1,
            kind: "pass",
            actor: 0,
            target: 2,
            x: 0.95,
            y: 0.5,
          }),
        ],
        events: [],
      },
      34_000,
    );
    const episodes = buildBallStory(dump);
    // Short placement is a visible roll: the ball never vanishes on the way.
    for (let t = 24_500; t <= 30_000; t += 50) {
      const b = ballAt(episodes, t, sampleAt(dump.keyframes, t)?.players);
      expect(b?.visible).toBe(true);
    }
    const roll = episodes.find((e) => e.kind === "deadBall")!;
    expect(roll).toBeDefined();
    expect(roll.t1 - roll.t0).toBeGreaterThanOrEqual(700); // a real roll, not a blip
    expect(Math.abs(roll.x1 - roll.x0) * 105).toBeLessThan(8); // only the short trip it is
    const atKick = ballAt(episodes, 30_200, sampleAt(dump.keyframes, 30_200)!.players)!;
    expect(atKick.visible).toBe(true); // goal kick struck on camera
  });

  it("does not credit a nearby opponent goal to a shot", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        touch({ tick: 20, team: 1, kind: "shot", actor: 9, success: false, x: 0.2, y: 0.4 }),
      ],
      events: [{ tick: 21, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    expect(buildBallStory(dump)[0].kind).toBe("shot");
  });

  it("rolls the ball to the spot and rests it across a long dead-ball gap", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 5, chain: 1, x: 0.3, y: 0.5 }),
          touch({ tick: 120, chain: 2, actor: 1, target: 2, x: 0.6, y: 0.5 }),
        ],
        chains: [
          {
            id: 1,
            team: 0,
            regime: "progression",
            start: 5,
            end: 5,
            touches: 1,
            outcome: "dead_ball",
          },
          { id: 2, team: 0, regime: "setPiece", start: 120, end: 120, touches: 1, outcome: "half" },
        ],
      },
      130_000,
    );
    const episodes = buildBallStory(dump);
    const roll = episodes.find((e) => e.kind === "deadBall")!;
    expect(roll).toBeDefined();
    expect(roll.x1).toBeGreaterThan(roll.x0); // the ball journeys to the restart spot
    // Across the whole dead-ball spell the ball stays in view (rolled, then rested).
    for (let t = 5_500; t <= 120_000; t += 250) {
      const b = ballAt(episodes, t, sampleAt(dump.keyframes, t)?.players);
      expect(b?.visible).toBe(true);
    }
    expect(Math.abs((ballAt(episodes, 60_000)?.x ?? 0) - 0.6)).toBeLessThan(0.1); // on the spot
    expect(ballAt(episodes, 120_500)?.visible).toBe(true); // delivery resumes on camera
  });

  it("treats a six-second same-chain service as a pass and glues the receiver's feet", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 5, chain: 1, kind: "pass", actor: 6, target: 9, x: 0.4, y: 0.5 }),
          touch({ tick: 11, chain: 1, kind: "carry", actor: 9, x: 0.62, y: 0.46 }),
        ],
      },
      12_000,
    );
    const episodes = buildBallStory(dump);
    expect(episodes[0].kind).toBe("pass");
    const atNine = sampleAt(dump.keyframes, 9_000)!;
    const b = ballAt(episodes, 9_000, atNine.players)!;
    expect(b.visible).toBe(true);
    // After the flight the ball lives at the receiver's feet, not on the grass.
    expect(b.x).toBeCloseTo(atNine.players[9].x, 6);
    expect(b.y).toBeCloseTo(atNine.players[9].y, 6);
  });

  it("shows a live save rebound instead of hiding it", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 20, kind: "shot", actor: 9, success: false, x: 0.8, y: 0.4 }),
          touch({ tick: 21, chain: 2, team: 1, kind: "regain", actor: 3, x: 0.3, y: 0.6 }),
        ],
        events: [],
      },
      24_000,
    );
    const episodes = buildBallStory(dump);
    const rebound = episodes.find((e) => e.kind === "loose");
    expect(rebound).toBeDefined(); // the ball springs back into play visibly
    expect(ballAt(episodes, 21_800)?.visible).toBe(true);
  });

  it("never teleports the ball, even across atomic same-tick ledger pairs", () => {
    // Torture ledger modelled on the real defect: the engine resolves a long pass and
    // its collection in ONE tick (zero gap), followed by same-tick cascades, a 65 m
    // clearance picked up by the rival, a carry spell and a shot.
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 10, chain: 1, kind: "carry", actor: 4, x: 0.3, y: 0.5 }),
          touch({ tick: 10, chain: 1, kind: "pass", actor: 4, target: 8, x: 0.3, y: 0.5 }),
          touch({ tick: 10, chain: 1, kind: "carry", actor: 8, x: 0.85, y: 0.5 }), // struck AND collected at tick 10
          touch({ tick: 16, chain: 1, kind: "pass", actor: 8, target: 2, x: 0.9, y: 0.45 }),
          touch({ tick: 22, chain: 2, team: 1, kind: "duel", actor: 5, x: 0.2, y: 0.6 }), // hoofed 65 m away
          touch({ tick: 28, chain: 2, team: 1, kind: "carry", actor: 5, x: 0.3, y: 0.62 }),
          touch({ tick: 34, chain: 3, team: 0, kind: "shot", actor: 7, x: 0.85, y: 0.5 }),
        ],
        chains: [
          {
            id: 1,
            team: 0,
            regime: "transition",
            start: 10,
            end: 16,
            touches: 4,
            outcome: "turnover",
          },
          {
            id: 2,
            team: 1,
            regime: "buildUp",
            start: 22,
            end: 28,
            touches: 2,
            outcome: "turnover",
          },
          { id: 3, team: 0, regime: "transition", start: 34, end: 34, touches: 1, outcome: "shot" },
        ],
        events: [],
      },
      42_000,
    );
    const episodes = buildBallStory(dump);
    // The zero-gap 55 m service is hung in the air, not teleported.
    const service = episodes.find((e) => e.kind === "pass")!;
    expect(service).toBeDefined();
    expect(service.t1 - service.t0).toBeGreaterThanOrEqual(1000);
    expectCoherent(dump, episodes);
  });

  it("borrows the idle slack so flights read as football at warp speeds", () => {
    // 63 m service (feet are nowhere near either ledger spot, so the anchors are the
    // spots themselves) with 10 s of ledger window ahead: the flight stretches toward
    // its receiver (capped by FLOAT_MIN_SPEED and STRETCH_CAP) instead of a 1.8 s
    // blip that vanishes in two painted frames at 30× playback (T-029).
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 5, chain: 1, kind: "pass", actor: 0, target: 9, x: 0.2, y: 0.5 }),
          touch({ tick: 15, chain: 1, kind: "carry", actor: 9, x: 0.8, y: 0.5 }),
        ],
      },
      20_000,
    );
    const service = buildBallStory(dump).find((e) => e.kind === "pass")!;
    expect(service).toBeDefined();
    expect(Math.abs(service.x1 - service.x0)).toBeGreaterThan(0.5); // really a long leg
    const dur = service.t1 - service.t0;
    expect(dur).toBeGreaterThanOrEqual(2500); // borrowed slack, visibly airborne
    expect(dur).toBeLessThanOrEqual(3800); // but still paced like a real ball
  });

  it("settles into the receiver: the arrival decelerates", () => {
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 5, chain: 1, kind: "pass", actor: 0, target: 9, x: 0.2, y: 0.5 }),
          touch({ tick: 15, chain: 1, kind: "carry", actor: 9, x: 0.8, y: 0.5 }),
        ],
      },
      20_000,
    );
    const episodes = buildBallStory(dump);
    const service = episodes.find((e) => e.kind === "pass")!;
    const at = (frac: number) => ballAt(episodes, service.t0 + (service.t1 - service.t0) * frac)!;
    const dist = (a: { x: number; y: number }, b: { x: number; y: number }) =>
      Math.hypot((b.x - a.x) * 105, (b.y - a.y) * 68);
    const early = dist(at(0.05), at(0.3));
    const late = dist(at(0.7), at(0.95));
    expect(early).toBeGreaterThan(late); // fast off the foot, soft at the receiver
  });

  it("hands the story over even when a placement roll fills its whole window", () => {
    // Regression (T-029, measured on the real dump as a 30.7 m goal-mouth hop): the
    // roll's degenerate trailing hold must still transfer the anchor, or the next
    // flight departs from the shot landing instead of the restart spot.
    const dump = runway(
      {
        ...syntheticDump,
        touches: [
          touch({ tick: 20, kind: "shot", actor: 9, success: false, x: 0.8, y: 0.4 }),
          touch({
            tick: 23,
            chain: 2,
            team: 1,
            kind: "pass",
            actor: 3,
            target: 4,
            x: 0.75,
            y: 0.5,
          }),
          touch({ tick: 30, chain: 2, team: 1, kind: "carry", actor: 5, x: 0.7, y: 0.55 }),
        ],
        events: [],
      },
      34_000,
    );
    const episodes = buildBallStory(dump);
    const roll = episodes.find((e) => e.kind === "deadBall")!;
    expect(roll).toBeDefined();
    const rollIndex = episodes.indexOf(roll);
    expect(roll.t1).toBe(23_000); // the roll really did eat its whole window
    const after = episodes[rollIndex + 1];
    expect(after).toBeDefined();
    expect(Math.abs(after.x0 - roll.x1)).toBeLessThan(0.005); // departs from the spot
    expect(Math.abs(after.y0 - roll.y1)).toBeLessThan(0.005);
  });
});
