import type { MatchDump } from "./types";

export function scoreAt(dump: MatchDump, tMs: number): { home: number; away: number } {
  const score = { home: 0, away: 0 };
  for (const event of dump.events) {
    if (event.tick * 1000 > tMs || event.kind !== "goal") continue;
    if (event.club === dump.teams.home.club) score.home++;
    else if (event.club === dump.teams.away.club) score.away++;
  }
  return score;
}

export function eventStatsAt(dump: MatchDump, tMs: number) {
  const home = { shots: 0, onTarget: 0, fouls: 0, offsides: 0 };
  const away = { shots: 0, onTarget: 0, fouls: 0, offsides: 0 };
  for (const event of dump.events) {
    if (event.tick * 1000 > tMs) continue;
    const side = event.club === dump.teams.home.club ? home : away;
    if (event.kind === "goal" || event.kind.startsWith("shot_")) side.shots++;
    if (event.kind === "goal" || event.kind === "shot_saved") side.onTarget++;
    if (event.kind === "foul") side.fouls++;
    if (event.kind === "offside") side.offsides++;
  }
  return { home, away };
}
