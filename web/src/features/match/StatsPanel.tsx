import { useTranslation } from "react-i18next";
import { cn } from "../../lib/utils";
import { eventStatsAt } from "./liveMatch";
import { durationMsOf } from "./types";
import type { MatchDump, TeamStatsDump } from "./types";

interface Props {
  dump: MatchDump;
  tMs?: number;
}

/** Dual-bar stats table (home left, away right) with pattern attribution chips. */
export function StatsPanel({ dump, tMs = durationMsOf(dump) }: Props) {
  const { t } = useTranslation();
  const complete = tMs >= durationMsOf(dump);
  const live = eventStatsAt(dump, tMs);
  const home = dump.stats.home;
  const away = dump.stats.away;

  return (
    <section className="rounded-xl border border-border bg-card p-4">
      <h2 className="mb-3 text-sm font-semibold text-muted-foreground">{t("viewer.stats")}</h2>
      {!complete ? (
        <div className="space-y-2 text-sm">
          <StatRow
            label={t("stats.shots")}
            home={live.home.shots}
            away={live.away.shots}
            homeRatio={ratio(live.home.shots, live.away.shots)}
          />
          <StatRow
            label={t("stats.onTarget")}
            home={live.home.onTarget}
            away={live.away.onTarget}
            homeRatio={ratio(live.home.onTarget, live.away.onTarget)}
          />
          <StatRow
            label={t("stats.fouls")}
            home={live.home.fouls}
            away={live.away.fouls}
            homeRatio={ratio(live.home.fouls, live.away.fouls)}
          />
          <StatRow
            label={t("stats.offsides")}
            home={live.home.offsides}
            away={live.away.offsides}
            homeRatio={ratio(live.home.offsides, live.away.offsides)}
          />
          <p className="pt-2 text-xs text-muted-foreground">{t("viewer.fullStatsAfterWhistle")}</p>
        </div>
      ) : (
        <>
          <div className="space-y-2 text-sm">
            <StatRow
              label={t("stats.possession")}
              home={`${home.possessionPct.toFixed(0)}%`}
              away={`${away.possessionPct.toFixed(0)}%`}
              homeRatio={ratio(home.possessionPct, away.possessionPct)}
            />
            <StatRow
              label={t("stats.shots")}
              home={home.shots}
              away={away.shots}
              homeRatio={ratio(home.shots, away.shots)}
            />
            <StatRow
              label={t("stats.onTarget")}
              home={home.onTarget}
              away={away.onTarget}
              homeRatio={ratio(home.onTarget, away.onTarget)}
            />
            <StatRow
              label={t("stats.xg")}
              home={home.xg.toFixed(2)}
              away={away.xg.toFixed(2)}
              homeRatio={ratio(home.xg, away.xg)}
            />
            <StatRow
              label={t("stats.corners")}
              home={home.corners}
              away={away.corners}
              homeRatio={ratio(home.corners, away.corners)}
            />
            <StatRow
              label={t("stats.fouls")}
              home={home.fouls}
              away={away.fouls}
              homeRatio={ratio(home.fouls, away.fouls)}
            />
            <StatRow
              label={t("stats.turnovers")}
              home={home.turnovers}
              away={away.turnovers}
              homeRatio={ratio(home.turnovers, away.turnovers)}
            />
            <StatRow
              label={t("stats.avgFatigue")}
              home={home.avgFatigue.toFixed(0)}
              away={away.avgFatigue.toFixed(0)}
              homeRatio={ratio(home.avgFatigue, away.avgFatigue)}
            />
          </div>
          <div className="mt-3 flex flex-wrap gap-2 border-t border-border pt-3">
            <PatternChip stats={home} club={dump.teams.home.club} side="home" />
            <PatternChip stats={away} club={dump.teams.away.club} side="away" />
          </div>
        </>
      )}
    </section>
  );
}

function PatternChip({
  stats,
  club,
  side,
}: {
  stats: TeamStatsDump;
  club: string;
  side: "home" | "away";
}) {
  const { t } = useTranslation();
  if (stats.patternShots <= 0) return null;
  return (
    <span
      data-testid={`pattern-chip-${side}`}
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2.5 py-1 font-mono text-[11px]",
        side === "home" ? "bg-sky-400/15 text-sky-300" : "bg-orange-400/15 text-orange-300",
      )}
      title={club}
    >
      ⚡{" "}
      {t("viewer.patternStat", { club, shots: stats.patternShots, xg: stats.patternXg.toFixed(2) })}
    </span>
  );
}

function StatRow({
  label,
  home,
  away,
  homeRatio,
}: {
  label: string;
  home: string | number;
  away: string | number;
  homeRatio: number;
}) {
  return (
    <div className="grid grid-cols-[3.5rem_1fr_3.5rem] items-center gap-3">
      <div className="text-left font-medium tabular-nums" data-testid={`stat-home-${label}`}>
        {home}
      </div>
      <div>
        <div className="text-center text-[11px] uppercase tracking-wide text-muted-foreground">
          {label}
        </div>
        <div className="mt-1 flex h-1.5 overflow-hidden rounded-full bg-secondary">
          <div className="bg-sky-400" style={{ width: `${homeRatio * 100}%` }} />
          <div className="grow bg-orange-400" />
        </div>
      </div>
      <div className="text-right tabular-nums" data-testid={`stat-away-${label}`}>
        {away}
      </div>
    </div>
  );
}

function ratio(a: number, b: number): number {
  const total = a + b;
  return total > 0 ? a / total : 0.5;
}
