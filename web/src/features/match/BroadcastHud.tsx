import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { formatClock, halfOf } from "./playback";
import { playerNameLookup, type MatchDump } from "./types";

/**
 * Broadcast HUD (ADR-0012): the TV scoreboard bug floating over the pitch and the
 * goal banner — match moments get a voice, nothing important whispers.
 */
export function BroadcastHud({
  dump,
  tMs,
  short,
}: {
  dump: MatchDump;
  tMs: number;
  short: readonly [string, string];
}) {
  const { t } = useTranslation();
  const names = useMemo(() => playerNameLookup(dump), [dump]);
  const goal = dump.events.find(
    (ev) => ev.kind === "goal" && ev.tick * 1000 <= tMs && tMs - ev.tick * 1000 < 4000,
  );

  return (
    <>
      {/* Scoreboard bug — top-left, TV style */}
      <div className="pointer-events-none absolute left-3 top-3 z-10 flex items-center gap-2 rounded-md bg-black/75 px-3 py-1.5 font-mono text-sm text-white shadow-lg backdrop-blur">
        <span className="font-semibold">{short[0]}</span>
        <span className="tabular-nums">
          {dump.score.home}–{dump.score.away}
        </span>
        <span className="font-semibold">{short[1]}</span>
        <span className="ml-2 text-xs text-white/70 tabular-nums">
          {formatClock(tMs)} · {halfOf(tMs)}
        </span>
      </div>

      {/* Goal banner — the moment gets its theatre */}
      {goal && (
        <div className="pointer-events-none absolute inset-x-0 top-1/3 z-10 flex justify-center">
          <div className="animate-bounce rounded-xl bg-primary px-6 py-3 text-xl font-bold text-primary-foreground shadow-2xl">
            {t("viewer.goalBanner", {
              minute: goal.minute,
              scorer: names.get(goal.player) ?? goal.player,
            })}
          </div>
        </div>
      )}
    </>
  );
}