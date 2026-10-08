import { describe, expect, it } from "vitest";
import { eventStatsAt, scoreAt } from "./liveMatch";
import { syntheticDump } from "./fixtures";

describe("live match reveal", () => {
  it("does not reveal future goals or shots", () => {
    expect(scoreAt(syntheticDump, 0)).toEqual({ home: 0, away: 0 });
    expect(eventStatsAt(syntheticDump, 0).away.shots).toBe(0);
    expect(scoreAt(syntheticDump, 2_520_000)).toEqual({ home: 1, away: 0 });
    expect(eventStatsAt(syntheticDump, 2_520_000).home.onTarget).toBe(1);
    expect(eventStatsAt(syntheticDump, 5_520_000).away.shots).toBe(1);
  });
});
