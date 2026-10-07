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
        { tick: 20, chain: 1, team: 0, kind: "shot", actor: 1, target: -1, success: true, x: 0.9, y: 0.4 },
      ],
      events: [{ tick: 20, minute: 1, kind: "goal", club: "Redvale FC", player: "h9", detail: "" }],
    };
    const episodes = buildBallStory(dump);
    const goal = episodes.find((e) => e.kind === "goal");
    expect(goal).toBeDefined();
    expect(goal!.x1).toBeCloseTo(1.005, 6); // ball nestles in the home-side net
  });

  it("parks the ball across dead-ball gaps instead of drifting", () => {
    const dump = {
      ...syntheticDump,
      touches: [
        { tick: 5, chain: 1, team: 0, kind: "pass", actor: 0, target: 1, success: true, x: 0.3, y: 0.5 },
        { tick: 120, chain: 2, team: 0, kind: "pass", actor: 1, target: 2, success: true, x: 0.6, y: 0.5 },
      ],
    };
    const episodes = buildBallStory(dump);
    const idle = episodes.find((e) => e.kind === "idle");
    expect(idle).toBeDefined();
    expect(idle!.x0).toBe(idle!.x1);
  });
});