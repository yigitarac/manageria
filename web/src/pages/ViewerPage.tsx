import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { Pause, Play, Upload } from "lucide-react";
import { Button } from "../components/ui/button";
import { BroadcastHud } from "../features/match/BroadcastHud";
import { scoreAt } from "../features/match/liveMatch";
import { CommentaryFeed } from "../features/match/CommentaryFeed";
import { MatchCanvas } from "../features/match/MatchCanvas";
import { StatsPanel } from "../features/match/StatsPanel";
import { durationMsOf, isMatchDump, type MatchDump } from "../features/match/types";
import {
  formatClock,
  halfOf,
  initialPlayback,
  playbackReducer,
  SPEED_CHOICES,
} from "../features/match/playback";
import { useTranslation } from "react-i18next";

const SAMPLE_URL = `${import.meta.env.BASE_URL ?? "/"}samples/match-42.json`;

/**
 * Match viewer: plays engine dumps (`simcli -out match.json`) as 2D circles with
 * synchronized commentary and stats. Drag & drop any dump to watch it.
 */
export function ViewerPage() {
  const { t } = useTranslation();
  const [dump, setDump] = useState<MatchDump | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  const durationMs = dump ? durationMsOf(dump) : 0;
  const [state, dispatch] = useReducer(
    (s: ReturnType<typeof playbackReducer>, a: Parameters<typeof playbackReducer>[1]) =>
      playbackReducer(s, a, durationMs),
    undefined,
    initialPlayback,
  );
  const score = dump ? scoreAt(dump, state.tMs) : null;

  const stateRef = useRef(state);
  stateRef.current = state;
  const getTimeMs = useCallback(() => stateRef.current.tMs, []);
  const getSeekId = useCallback(() => stateRef.current.seekId, []);

  // Load the generated sample if present (make sample); otherwise prompt for a drop.
  useEffect(() => {
    let cancelled = false;
    fetch(`${SAMPLE_URL}?v=${Date.now()}`, { cache: "no-store" })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error("missing"))))
      .then((json: unknown) => {
        if (cancelled) return;
        if (isMatchDump(json)) {
          setDump(json);
          setNotice(null);
        } else {
          setNotice(t("viewer.badFile"));
        }
      })
      .catch(() => {
        if (!cancelled) setNotice(t("viewer.dropHint"));
      });
    return () => {
      cancelled = true;
    };
  }, [t]);

  // Real-time clock loop: dispatches ticks, scene reads state directly per frame.
  useEffect(() => {
    if (!dump) return;
    let raf = 0;
    let last = performance.now();
    const loop = (now: number) => {
      const dt = Math.min(now - last, 250);
      last = now;
      dispatch({ type: "tick", dtMs: dt });
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [dump]);

  // Keyboard transport: space = play/pause.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code === "Space" && !(e.target instanceof HTMLInputElement)) {
        e.preventDefault();
        dispatch({ type: "toggle" });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const onDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files[0];
    if (!file) return;
    try {
      const json: unknown = JSON.parse(await file.text());
      if (!isMatchDump(json)) throw new Error("bad dump");
      setDump(json);
      setNotice(null);
      dispatch({ type: "seek", tMs: 0 });
      dispatch({ type: "play" });
    } catch {
      setNotice(t("viewer.badFile"));
    }
  };

  return (
    <main
      className="mx-auto flex min-h-screen max-w-7xl flex-col gap-4 p-4"
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={onDrop}
    >
      <header className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card px-4 py-3">
        <div>
          <h1 className="text-lg font-bold">{t("viewer.title")}</h1>
          {dump ? (
            <>
              <p className="text-sm text-muted-foreground">
                {dump.teams.home.club}{" "}
                <span className="font-mono">
                  {score?.home}–{score?.away}
                </span>{" "}
                {dump.teams.away.club}
              </p>
              {/* Dump fingerprint: ends "old vs new match" disputes */}
              <p className="font-mono text-[11px] text-muted-foreground/70">
                #{dump.keyframes.length}f · {dump.touches.length}t · {dump.chains.length}c
              </p>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">{notice ?? t("viewer.loading")}</p>
          )}
        </div>
        {dump && (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => dispatch({ type: "toggle" })}
              aria-label={state.playing ? t("viewer.pause") : t("viewer.play")}
            >
              {state.playing ? <Pause size={16} /> : <Play size={16} />}
            </Button>
            <span className="w-24 font-mono text-sm tabular-nums" aria-live="polite">
              {formatClock(state.tMs)}
              <span className="ml-1 text-xs text-muted-foreground">{halfOf(state.tMs)}.</span>
            </span>
            <input
              type="range"
              min={0}
              max={durationMs}
              step={1000}
              value={state.tMs}
              onChange={(e) => dispatch({ type: "seek", tMs: Number(e.target.value) })}
              className="w-40 accent-sky-500 md:w-72"
              aria-label={t("viewer.scrub")}
            />
            <select
              value={state.speed}
              onChange={(e) => dispatch({ type: "speed", speed: Number(e.target.value) })}
              className="rounded-md border border-input bg-background px-2 py-1 text-sm"
              aria-label={t("viewer.speed")}
            >
              {SPEED_CHOICES.map((s) => (
                <option key={s} value={s}>
                  {s}×
                </option>
              ))}
            </select>
          </div>
        )}
      </header>

      {dump ? (
        <>
          <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
            <div className="relative h-[46vh] min-h-[320px] lg:h-[62vh]">
              <MatchCanvas dump={dump} getTimeMs={getTimeMs} getSeekId={getSeekId} />
              <BroadcastHud
                dump={dump}
                tMs={state.tMs}
                short={[
                  dump.teams.home.club.slice(0, 3).toUpperCase(),
                  dump.teams.away.club.slice(0, 3).toUpperCase(),
                ]}
              />
            </div>
            <div className="h-[46vh] min-h-[320px] lg:h-[62vh]">
              <CommentaryFeed dump={dump} tMs={state.tMs} />
            </div>
          </div>
          <StatsPanel dump={dump} tMs={state.tMs} />
        </>
      ) : (
        <div
          className={
            "flex flex-1 flex-col items-center justify-center rounded-xl border-2 border-dashed p-12 text-center " +
            (dragging ? "border-primary bg-primary/5" : "border-border")
          }
        >
          <Upload size={32} className="mb-3 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">{t("viewer.dropHint")}</p>
          <p className="mt-2 font-mono text-xs text-muted-foreground">
            make sample → web/public/samples/match-42.json
          </p>
        </div>
      )}
    </main>
  );
}
