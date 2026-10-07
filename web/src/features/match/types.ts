/**
 * Dump contract mirror of the engine's MatchResult JSON. The Go types in
 * `server/internal/engine/types.go` ARE the spec (camelCase tags) — keep in sync.
 */

export interface PlayerInfo {
  id: string;
  name: string;
  pos: number;
  starter: boolean;
}

export interface TeamInfo {
  club: string;
  players: PlayerInfo[];
}

export interface MatchEvent {
  tick: number;
  minute: number;
  kind: string;
  club: string;
  player: string;
  detail: string;
}

export interface Touch {
  tick: number;
  chain: number;
  team: number;
  kind: string;
  actor: number;
  target: number;
  success: boolean;
  x: number;
  y: number;
}

export interface ChainInfo {
  id: number;
  team: number;
  regime: string;
  start: number;
  end: number;
  touches: number;
  outcome: string;
}

export interface TeamStatsDump {
  possessionPct: number;
  shots: number;
  onTarget: number;
  goals: number;
  xg: number;
  corners: number;
  fouls: number;
  turnovers: number;
  avgFatigue: number;
  avgShotDist: number;
  patternShots: number;
  patternXg: number;
}

export interface KFPlayerDump {
  x: number;
  y: number;
  state: number;
}

export interface Keyframe {
  tMs: number;
  ballX: number;
  ballY: number;
  ballOwner: number;
  players: KFPlayerDump[];
}

export interface MatchDump {
  revision: number;
  teams: { home: TeamInfo; away: TeamInfo };
  score: { home: number; away: number };
  events: MatchEvent[];
  touches: Touch[];
  chains: ChainInfo[];
  stats: { home: TeamStatsDump; away: TeamStatsDump };
  playerRatings: { playerId: string; rating: number }[];
  keyframes: Keyframe[];
}

/** Minimal structural guard for dropped/loaded JSON. */
export function isMatchDump(value: unknown): value is MatchDump {
  const v = value as Partial<MatchDump> | null;
  return (
    !!v &&
    Array.isArray(v.keyframes) &&
    v.keyframes.length > 0 &&
    Array.isArray(v.events) &&
    !!v.teams?.home &&
    !!v.teams?.away &&
    !!v.stats?.home &&
    !!v.stats?.away
  );
}

/** Extracts the "pattern:<id>" attribution tag from an event detail, if any. */
export function patternTagOf(detail: string): string | null {
  const match = detail.match(/pattern:([A-Za-z0-9_-]+)/);
  return match ? match[1] : null;
}

/** Builds id → display name lookups from both rosters. */
export function playerNameLookup(dump: MatchDump): Map<string, string> {
  const map = new Map<string, string>();
  for (const p of dump.teams.home.players) map.set(p.id, p.name);
  for (const p of dump.teams.away.players) map.set(p.id, p.name);
  return map;
}

/** Total timeline length of a dump in milliseconds. */
export function durationMsOf(dump: MatchDump): number {
  const last = dump.keyframes[dump.keyframes.length - 1];
  return last ? last.tMs : 0;
}
