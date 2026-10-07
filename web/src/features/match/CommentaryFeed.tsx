import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "../../lib/utils";
import { patternTagOf, playerNameLookup, type MatchDump } from "./types";

interface Props {
  dump: MatchDump;
  tMs: number;
}

interface FeedRow {
  tick: number;
  minute: number;
  kind: string;
  club: string;
  player: string;
  detail: string;
  home: boolean;
  playerName: string;
  pattern: string | null;
}

/** Newest-first synchronized commentary: spoken rows bright, future rows faint. */
export function CommentaryFeed({ dump, tMs }: Props) {
  const { t } = useTranslation();

  const rows = useMemo<FeedRow[]>(() => {
    const names = playerNameLookup(dump);
    return dump.events
      .map((ev) => ({
        ...ev,
        home: ev.club === dump.teams.home.club,
        playerName: names.get(ev.player) ?? ev.player,
        pattern: patternTagOf(ev.detail),
      }))
      .reverse();
  }, [dump]);

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-card p-4">
      <h2 className="mb-3 text-sm font-semibold text-muted-foreground">{t("viewer.commentary")}</h2>
      <ol className="flex-1 space-y-2 overflow-y-auto pr-1" data-testid="commentary-feed">
        {rows.map((row) => {
          const spoken = row.tick * 1000 <= tMs;
          return (
            <li
              key={`${row.tick}-${row.kind}-${row.player}`}
              className={cn(
                "rounded-lg border border-transparent px-2 py-1.5 text-sm transition-opacity",
                spoken ? "bg-secondary opacity-100" : "opacity-40",
              )}
              aria-current={spoken ? "true" : undefined}
            >
              <div className="flex items-baseline gap-2">
                <span className="w-10 shrink-0 font-mono text-xs text-muted-foreground">
                  {row.minute}'
                </span>
                <span
                  className={cn(
                    "inline-block h-2 w-2 shrink-0 rounded-full",
                    row.home ? "bg-sky-400" : "bg-orange-400",
                  )}
                  aria-hidden
                />
                <span className="font-medium">{t(`event.${row.kind}`)}</span>
                <span className="truncate text-muted-foreground">{row.playerName}</span>
              </div>
              {row.pattern && (
                <span
                  data-testid="commentary-pattern-chip"
                  className="ml-12 mt-0.5 inline-block rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[11px] text-primary"
                >
                  ⚡ {t("viewer.patternTag", { id: row.pattern })}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
