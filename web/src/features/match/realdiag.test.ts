/**
 * Real-dump ball continuity guard (T-028/T-029 forensics). Runs only when a sample
 * dump exists (`make sample SEED=42` — the dump is gitignored), and asserts the
 * properties that were measured broken before the fixes:
 *   1. the story never teleports the ball on real engine data (0 m/m-frame jumps at
 *      fine sampling, exact episode tiling, full coverage);
 *   2. the ball never blinks out of existence (owner: "it feels like it teleports"):
 *      placement is a visible roll or a short faded broadcast cut — hidden spells
 *      measured 482 s across 75 blinks with 98 m reappearing pops before the fix;
 *   3. playback steps at ANY speed are playback, never scrubs (seek provenance), so
 *      the trail ribbon survives every warp frame and only a real seek resets it.
 */
// @ts-expect-error -- node builtin works under vitest; @types/node is not part of the app toolchain
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { initialPlayback, isSeekRestart, playbackReducer } from "./playback";
import { ballAt, buildBallStory } from "./ballStory";
import { sampleAt } from "./interpolate";
import type { MatchDump } from "./types";

const SAMPLE = "public/samples/match-42.json";

const raw = (() => {
  try {
    return readFileSync(SAMPLE, "utf8") as string;
  } catch {
    return null;
  }
})();

describe.skipIf(raw === null)("real sample dump", () => {
  const dump = JSON.parse(raw as string) as MatchDump;
  const eps = buildBallStory(dump);
  const duration = dump.keyframes[dump.keyframes.length - 1].tMs;
  const sample = (t: number) => {
    const s = sampleAt(dump.keyframes, t);
    return s ? ballAt(eps, t, s.players) : ballAt(eps, t);
  };

  it("tiles the story exactly and covers every moment of the match", () => {
    for (let i = 1; i < eps.length; i++) {
      expect(eps[i].t0).toBe(eps[i - 1].t1); // no gap a fallback could flicker through
    }
    expect(eps[0].t0).toBe(0);
    expect(eps[eps.length - 1].t1).toBe(duration);

    for (let t = 0; t <= duration; t += 25) {
      expect(sample(t)).not.toBeNull();
    }
  });

  it("never teleports the ball: no rendered-frame hop over 1.5 m at fine sampling", () => {
    let prev: { x: number; y: number } | null = null;
    for (let t = 0; t <= duration; t += 25) {
      const b = sample(t);
      if (!b || !b.visible) {
        prev = null; // a faded cut may reappear anywhere (the jump is at zero alpha)
        continue;
      }
      if (prev) {
        const dm = Math.hypot((b.x - prev.x) * 105, (b.y - prev.y) * 68);
        expect(dm).toBeLessThanOrEqual(1.5);
      }
      prev = b;
    }
  });

  it("never hides the ball longer than a faded broadcast cut", () => {
    let run = 0;
    let longest = 0;
    let hiddenTotal = 0;
    for (let t = 0; t <= duration; t += 25) {
      const b = sample(t);
      if (b && !b.visible) {
        run += 25;
        hiddenTotal += 25;
        longest = Math.max(longest, run);
      } else {
        run = 0;
      }
    }
    expect(longest).toBeLessThanOrEqual(1200); // a dissolve dip, not a disappearance
    expect(hiddenTotal).toBeLessThan(30_000); // cuts only (was 482 s of vanished ball)
  });

  it("keeps the trail ribbon alive on every steady playback frame at every speed", () => {
    // Steady playback (even 3000 ms of game time per painted frame at 180x) must
    // never restart the ribbon — only a seek may. Magnitude heuristics misread warp
    // frames as scrubs (the ribbon blinked out on 1799 of 1800 frames at 180x).
    for (const speed of [1, 5, 30, 60, 180]) {
      let state = playbackReducer(initialPlayback(), { type: "play" }, duration);
      state = playbackReducer(state, { type: "speed", speed }, duration);
      const before = state.seekId;
      const stepGame = (1000 / 60) * speed;
      // A few hundred painted frames exercise each speed, including skipped
      // multi-second flights at 180×, without simulating an entire 90-minute
      // match at 1× inside one unit test.
      for (let frame = 0; frame < 300; frame++) {
        state = playbackReducer(state, { type: "tick", dtMs: 1000 / 60 }, duration);
        expect(isSeekRestart(before, state.seekId)).toBe(false);
      }
      expect(state.tMs).toBeGreaterThan(250 * stepGame);
    }
    const started = initialPlayback();
    const seeked = playbackReducer(started, { type: "seek", tMs: 45_000 }, duration);
    expect(isSeekRestart(started.seekId, seeked.seekId)).toBe(true);
  });
});
