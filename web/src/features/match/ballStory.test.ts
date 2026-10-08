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

  it("keeps the ball on a continuous path from a shot to its restart", () => {
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
    expect(ballAt(episodes, 25_000)).not.toBeNull();
    expect(ballAt(episodes, 29_900)?.x).toBeCloseTo(0.95, 2);
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

  it("walks the ball to the restart spot across dead-ball gaps (no teleporting)", () => {
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
    };
    const episodes = buildBallStory(dump);
    const idle = episodes.find((e) => e.kind === "idle");
    expect(idle).toBeDefined();
    // The ball travels to where the restart happens — smoothly, not by teleport.
    expect(idle!.x0).not.toBe(idle!.x1);
    const midway = ballAt(episodes, (idle!.t0 + idle!.t1) / 2);
    expect(midway!.x).toBeGreaterThan(idle!.x0);
    expect(midway!.x).toBeLessThan(idle!.x1);
  });
});
