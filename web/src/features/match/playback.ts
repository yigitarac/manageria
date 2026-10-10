export interface PlaybackState {
  tMs: number;
  playing: boolean;
  /** Game-time multiplier: 60 = the match flies by in ~90 real seconds. */
  speed: number;
  /**
   * Provenance counter bumped only by `seek` (T-029). Magnitude heuristics cannot
   * tell a slider scrub from a lagged warp frame — the reducer KNOWS which action
   * moved the clock, so the canvas resets camera easing and the trail ribbon on a
   * seek bump and never on a playback step.
   */
  seekId: number;
}

export type PlaybackAction =
  | { type: "toggle" }
  | { type: "play" }
  | { type: "pause" }
  | { type: "tick"; dtMs: number }
  | { type: "seek"; tMs: number }
  | { type: "speed"; speed: number };

export const SPEED_CHOICES = [1, 5, 30, 60, 180] as const;
export const DEFAULT_SPEED = 30;

export function initialPlayback(): PlaybackState {
  return { tMs: 0, playing: false, speed: DEFAULT_SPEED, seekId: 0 };
}

/** True when the clock moved by seek since the last observed seekId (a real scrub). */
export function isSeekRestart(previousSeekId: number, seekId: number): boolean {
  return previousSeekId !== seekId;
}

/** Pure playback reducer; durationMs clamps every transition. */
export function playbackReducer(
  state: PlaybackState,
  action: PlaybackAction,
  durationMs: number,
): PlaybackState {
  switch (action.type) {
    case "toggle":
      return { ...state, playing: !state.playing && state.tMs < durationMs };
    case "play":
      return { ...state, playing: state.tMs < durationMs };
    case "pause":
      return { ...state, playing: false };
    case "tick": {
      if (!state.playing) return state;
      const tMs = state.tMs + action.dtMs * state.speed;
      if (tMs >= durationMs) return { ...state, tMs: durationMs, playing: false };
      return { ...state, tMs };
    }
    case "seek":
      return {
        ...state,
        tMs: Math.max(0, Math.min(action.tMs, durationMs)),
        playing: action.tMs < durationMs && state.playing,
        seekId: state.seekId + 1,
      };
    case "speed":
      return { ...state, speed: action.speed };
    default:
      return state;
  }
}

/** Formats game milliseconds as MM:SS (match clock). */
export function formatClock(tMs: number): string {
  const total = Math.floor(tMs / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

/** Half-time label for the match clock ("1st" / "2nd"). */
export function halfOf(tMs: number): number {
  return tMs < 45 * 60 * 1000 ? 1 : 2;
}
