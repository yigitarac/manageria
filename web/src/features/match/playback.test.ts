import { describe, expect, it } from "vitest";
import { formatClock, halfOf, initialPlayback, isSeekRestart, playbackReducer } from "./playback";

const DURATION = 90_000;

describe("playbackReducer", () => {
  it("accumulates game time while playing, scaled by speed", () => {
    const playing = playbackReducer(initialPlayback(), { type: "play" }, DURATION);
    const state = playbackReducer(playing, { type: "tick", dtMs: 100 }, DURATION);
    expect(state.tMs).toBe(3000);
  });

  it("holds still while paused", () => {
    const paused = playbackReducer(initialPlayback(), { type: "pause" }, DURATION);
    const state = playbackReducer(paused, { type: "tick", dtMs: 100 }, DURATION);
    expect(state.tMs).toBe(paused.tMs);
  });

  it("stops at the end of the timeline", () => {
    let state = playbackReducer(initialPlayback(), { type: "play" }, DURATION);
    for (let i = 0; i < 100; i++)
      state = playbackReducer(state, { type: "tick", dtMs: 100 }, DURATION);
    expect(state.tMs).toBe(DURATION);
    expect(state.playing).toBe(false);
  });

  it("clamps seeks into the timeline", () => {
    const state = playbackReducer(initialPlayback(), { type: "seek", tMs: 500_000 }, DURATION);
    expect(state.tMs).toBe(DURATION);
  });

  it("toggles play/pause and refuses to resume past the end", () => {
    const atEnd = playbackReducer(
      { tMs: DURATION, playing: false, speed: 1, seekId: 0 },
      { type: "toggle" },
      DURATION,
    );
    expect(atEnd.playing).toBe(false);
    const resumed = playbackReducer(
      { tMs: 10, playing: false, speed: 1, seekId: 0 },
      { type: "toggle" },
      DURATION,
    );
    expect(resumed.playing).toBe(true);
  });

  it("marks scrubs by provenance: only seeks bump the seekId (T-029)", () => {
    // Magnitude heuristics cannot tell a 180× warp step (3000 ms of game time per
    // painted frame) from a slider scrub — the reducer knows which action moved the
    // clock. Playback must NEVER restart the camera/trail; every seek must.
    for (const speed of [1, 5, 30, 60, 180]) {
      let state = playbackReducer(initialPlayback(), { type: "play" }, DURATION);
      state = playbackReducer(state, { type: "speed", speed }, DURATION);
      const before = state.seekId;
      for (let i = 0; i < 200; i++) {
        state = playbackReducer(state, { type: "tick", dtMs: 1000 / 60 }, DURATION);
      }
      expect(isSeekRestart(before, state.seekId)).toBe(false);
    }
    const started = initialPlayback();
    const seeked = playbackReducer(started, { type: "seek", tMs: 5_000 }, DURATION);
    expect(isSeekRestart(started.seekId, seeked.seekId)).toBe(true);
  });
});

describe("clock formatting", () => {
  it("renders MM:SS and halves", () => {
    expect(formatClock(0)).toBe("00:00");
    expect(formatClock(45 * 60_000 + 5_000)).toBe("45:05");
    expect(halfOf(44 * 60_000)).toBe(1);
    expect(halfOf(45 * 60_000)).toBe(2);
  });
});
