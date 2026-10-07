import type { MatchDump } from "./types";

/** Tiny synthetic dump for component tests (contract-shaped, hand-written). */
export const syntheticDump: MatchDump = {
  revision: 0,
  teams: {
    home: {
      club: "Redvale FC",
      players: [
        { id: "h1", name: "Halden Moor", pos: 0, starter: true },
        { id: "h9", name: "Miro Thane", pos: 7, starter: true },
      ],
    },
    away: {
      club: "Stonebrook Athletic",
      players: [
        { id: "a1", name: "Piet Halloran", pos: 0, starter: true },
        { id: "a10", name: "Kenji Alba", pos: 7, starter: true },
      ],
    },
  },
  score: { home: 1, away: 1 },
  events: [
    { tick: 0, minute: 1, kind: "kick_off", club: "Redvale FC", player: "h9", detail: "" },
    {
      tick: 2520,
      minute: 42,
      kind: "goal",
      club: "Redvale FC",
      player: "h9",
      detail: "morale:+4|-4;pattern:six_yard_darts",
    },
    {
      tick: 5520,
      minute: 92,
      kind: "shot_saved",
      club: "Stonebrook Athletic",
      player: "a10",
      detail: "",
    },
  ],
  stats: {
    home: {
      possessionPct: 54.2,
      shots: 12,
      onTarget: 5,
      goals: 1,
      xg: 1.42,
      corners: 7,
      fouls: 9,
      turnovers: 210,
      avgFatigue: 31.2,
      avgShotDist: 0.14,
      patternShots: 3,
      patternXg: 0.48,
    },
    away: {
      possessionPct: 45.8,
      shots: 10,
      onTarget: 4,
      goals: 1,
      xg: 1.1,
      corners: 5,
      fouls: 11,
      turnovers: 221,
      avgFatigue: 28.4,
      avgShotDist: 0.13,
      patternShots: 0,
      patternXg: 0,
    },
  },
  playerRatings: [
    { playerId: "h9", rating: 7.8 },
    { playerId: "a10", rating: 6.9 },
  ],
  keyframes: [
    { tMs: 0, ballX: 0.5, ballY: 0.5, ballOwner: 0, players: playersAt(0.3, 0.5) },
    { tMs: 3000, ballX: 0.6, ballY: 0.4, ballOwner: 0, players: playersAt(0.4, 0.45) },
    { tMs: 6000, ballX: 0.7, ballY: 0.3, ballOwner: 1, players: playersAt(0.5, 0.4) },
    { tMs: 9000, ballX: 0.8, ballY: 0.35, ballOwner: -1, players: playersAt(0.6, 0.35) },
  ],
};

function playersAt(x: number, y: number) {
  return Array.from({ length: 22 }, (_, i) => ({
    x: x + (i % 5) * 0.02,
    y: (y + (i % 3) * 0.05) % 1,
    state: i % 4,
  }));
}
