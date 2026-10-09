import { describe, expect, it } from "vitest";
import { ballAt, buildBallStory } from "./ballStory";
import { syntheticDump } from "./fixtures";

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
      touches: [
        {
          tick: 20,
          chain: 1,
          team: 0,
          kind: "shot",
          actor: 1,
          target: -1,
          success: true,
          x: 0.9,
          y: 0.4,
        },
      ],
      events: [{ tick: 20, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    const episodes = buildBallStory(dump);
    const goal = episodes.find((e) => e.kind === "goal");
    expect(goal).toBeDefined();
    expect(goal!.x1).toBeCloseTo(1.005, 6); // ball nestles in the home-side net
  });

  it("hides the ball during the post-goal reset", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        {
          tick: 20,
          chain: 1,
          team: 0,
          kind: "shot",
          actor: 9,
          target: -1,
          success: true,
          x: 0.9,
          y: 0.5,
        },
        {
          tick: 56,
          chain: 2,
          team: 1,
          kind: "pass",
          actor: 6,
          target: 7,
          success: true,
          x: 0.5,
          y: 0.5,
        },
      ],
      events: [{ tick: 20, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    const episodes = buildBallStory(dump);
    expect(ballAt(episodes, 21_500)?.x).toBeCloseTo(1.005, 3);
    expect(ballAt(episodes, 30_000)?.visible).toBe(false);
    expect(ballAt(episodes, 56_000)).toBeNull(); // keyframe resumes at the centre spot
  });

  it("takes the ball out of view between a shot and its restart", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        {
          tick: 20,
          chain: 1,
          team: 0,
          kind: "shot",
          actor: 9,
          target: -1,
          success: false,
          x: 0.8,
          y: 0.4,
        },
        {
          tick: 30,
          chain: 2,
          team: 1,
          kind: "pass",
          actor: 0,
          target: 2,
          success: true,
          x: 0.95,
          y: 0.5,
        },
      ],
      events: [],
    };
    const episodes = buildBallStory(dump);
    expect(ballAt(episodes, 25_000)?.visible).toBe(false);
    expect(ballAt(episodes, 29_900)?.visible).toBe(false);
    expect(ballAt(episodes, 30_000)).toBeNull(); // keyframe resumes at the goal kick
  });

  it("does not credit a nearby opponent goal to a shot", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        {
          tick: 20,
          chain: 1,
          team: 1,
          kind: "shot",
          actor: 9,
          target: -1,
          success: false,
          x: 0.2,
          y: 0.4,
        },
      ],
      events: [{ tick: 21, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    expect(buildBallStory(dump)[0].kind).toBe("shot");
  });

  it("hides ball placement across dead-ball gaps", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        {
          tick: 5,
          chain: 1,
          team: 0,
          kind: "pass",
          actor: 0,
          target: 1,
          success: true,
          x: 0.3,
          y: 0.5,
        },
        {
          tick: 120,
          chain: 2,
          team: 0,
          kind: "pass",
          actor: 1,
          target: 2,
          success: true,
          x: 0.6,
          y: 0.5,
        },
      ],
      chains: [
        { id: 1, team: 0, regime: "progression", start: 5, end: 5, touches: 1, outcome: "dead_ball" },
        { id: 2, team: 0, regime: "setPiece", start: 120, end: 120, touches: 1, outcome: "half" },
      ],
    };
    const episodes = buildBallStory(dump);
    const deadBall = episodes.find((e) => e.kind === "deadBall");
    expect(deadBall).toBeDefined();
    expect(ballAt(episodes, 60_000)?.visible).toBe(false);
    expect(ballAt(episodes, 120_000)).toBeNull();
  });

  it("treats a six-second same-chain service as a pass, not a stoppage", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        { tick: 5, chain: 1, team: 0, kind: "pass", actor: 6, target: 9, success: true, x: 0.4, y: 0.5 },
        { tick: 11, chain: 1, team: 0, kind: "carry", actor: 9, target: -1, success: true, x: 0.7, y: 0.5 },
      ],
    };
    const episodes = buildBallStory(dump);
    expect(episodes[0].kind).toBe("pass");
    expect(ballAt(episodes, 6_000)?.visible).toBe(true);
    expect(ballAt(episodes, 9_000)?.x).toBeCloseTo(0.7, 3);
  });
});
