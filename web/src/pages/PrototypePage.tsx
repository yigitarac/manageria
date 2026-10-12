import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Pause, Play } from "lucide-react";
import { PrototypeCanvas } from "../features/prototype/PrototypeCanvas";
import { interpolateTrace, traceDurationMs } from "../features/prototype/interpolate";
import { isPrototypeTrace, type PrototypeTrace } from "../features/prototype/types";

const CLIPS = [
  { slug: "unopposed-reception", labelKey: "prototype.clip.unopposed" },
  { slug: "reachable-interception", labelKey: "prototype.clip.interception" },
  { slug: "miscontrol-recovery", labelKey: "prototype.clip.miscontrol" },
] as const;

const SPEEDS = [0.25, 0.5, 1, 2, 4];

/**
 * Same-clock causal prototype viewer (T-039). It plays the versioned
 * authoritative traces emitted by `server/cmd/liveplaydemo` and interpolates
 * only between the emitted knots — no ballStory retiming, no invented winner.
 * The generated clips live under web/public/prototype (gitignored); regenerate
 * them with `make prototype`.
 */
export function PrototypePage() {
  const { t } = useTranslation();
  const [slug, setSlug] = useState<string>(() => {
    const requested = new URLSearchParams(window.location.search).get("clip");
    return CLIPS.some((c) => c.slug === requested) ? (requested as string) : CLIPS[0].slug;
  });
  const [trace, setTrace] = useState<PrototypeTrace | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [playing, setPlaying] = useState(true);
  const [speed, setSpeed] = useState(1);
  const [tMs, setTMs] = useState(0);

  const duration = trace ? traceDurationMs(trace) : 0;
  const playingRef = useRef(playing);
  playingRef.current = playing;
  const speedRef = useRef(speed);
  speedRef.current = speed;

  useEffect(() => {
    const base = import.meta.env.BASE_URL ?? "/";
    const controller = new AbortController();
    setTrace(null);
    setNotice(null);
    setTMs(0);
    fetch(`${base}prototype/${slug}.json`, { cache: "no-store", signal: controller.signal })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error("missing"))))
      .then((json: unknown) => {
        if (controller.signal.aborted) return;
        let valid: boolean;
        try {
          valid = isPrototypeTrace(json);
        } catch {
          valid = false; // a malformed file must never crash the UI
        }
        if (valid) {
          setTrace(json as PrototypeTrace);
          setNotice(null);
        } else {
          setNotice(t("prototype.badFile"));
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setNotice(t("prototype.missing"));
      });
    return () => controller.abort();
  }, [slug, t]);

  useEffect(() => {
    if (!trace) return;
    let raf = 0;
    let last = performance.now();
    const loop = (now: number) => {
      const dt = Math.min(now - last, 250);
      last = now;
      if (playingRef.current) {
        setTMs((prev) => {
          const next = prev + dt * speedRef.current;
          return next >= duration ? 0 : next;
        });
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [trace, duration]);

  const state = trace ? interpolateTrace(trace, tMs) : null;
  const visibleEvents = trace ? trace.events.filter((e) => e.kind !== "path") : [];

  return (
    <main className="mx-auto flex min-h-screen max-w-7xl flex-col gap-4 p-4">
      <header className="rounded-xl border border-border bg-card px-4 py-3">
        <h1 className="text-lg font-bold">{t("prototype.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("prototype.subtitle")}</p>
      </header>

      <div className="flex flex-wrap gap-2">
        {CLIPS.map((clip) => (
          <button
            key={clip.slug}
            type="button"
            aria-pressed={slug === clip.slug}
            onClick={() => setSlug(clip.slug)}
            className={
              "rounded-md border px-3 py-1.5 text-sm " +
              (slug === clip.slug
                ? "border-primary bg-primary/10 font-medium"
                : "border-border bg-card hover:bg-accent")
            }
          >
            {t(clip.labelKey)}
          </button>
        ))}
      </div>

      {trace && state ? (
        <>
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-2">
            <button
              type="button"
              className="rounded-md border border-border p-1.5 hover:bg-accent"
              aria-label={playing ? t("prototype.pause") : t("prototype.play")}
              onClick={() => setPlaying((p) => !p)}
            >
              {playing ? <Pause size={16} /> : <Play size={16} />}
            </button>
            <span className="w-20 font-mono text-sm tabular-nums" aria-live="polite">
              {tMs.toFixed(0)} ms
            </span>
            <input
              type="range"
              min={0}
              max={duration}
              step={1}
              value={Math.min(tMs, duration)}
              onChange={(e) => setTMs(Number(e.target.value))}
              className="w-40 accent-sky-500 md:w-72"
              aria-label={t("prototype.scrub")}
            />
            <select
              value={speed}
              onChange={(e) => setSpeed(Number(e.target.value))}
              className="rounded-md border border-input bg-background px-2 py-1 text-sm"
              aria-label={t("prototype.speed")}
            >
              {SPEEDS.map((s) => (
                <option key={s} value={s}>
                  {s}×
                </option>
              ))}
            </select>
            <span className="font-mono text-[11px] text-muted-foreground/80">
              {t("prototype.meta", {
                slug: trace.slug,
                version: trace.schemaVersion,
                substep: trace.substepMs,
                seed: trace.seed,
              })}
            </span>
          </div>

          <p className="text-xs text-muted-foreground">{t("prototype.scriptedNote")}</p>

          <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
            <div className="h-[46vh] min-h-[320px] lg:h-[62vh]">
              <PrototypeCanvas trace={trace} state={state} label={t("prototype.canvasLabel")} />
            </div>
            <div className="h-[46vh] min-h-[320px] overflow-auto rounded-xl border border-border bg-card p-3 lg:h-[62vh]">
              <h2 className="mb-2 text-sm font-semibold">{t("prototype.events")}</h2>
              <ul className="flex flex-col gap-1 font-mono text-xs">
                {visibleEvents.map((e) => (
                  <li
                    key={e.seq}
                    className={
                      e.exactTimeMs <= tMs ? "text-foreground" : "text-muted-foreground/60"
                    }
                  >
                    {e.scripted ? "◆ " : "· "}
                    {t("prototype.eventLine", {
                      kind: t(`prototype.event.${e.kind}`),
                      time: e.exactTimeMs.toFixed(1),
                      runner: e.runner,
                    })}
                    {e.kind !== "release"
                      ? ` (${t("prototype.eventClock", {
                          clock: e.clockMs,
                          frac: e.frac.toFixed(3),
                        })})`
                      : ""}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border-2 border-dashed border-border p-12 text-center">
          <p className="text-sm text-muted-foreground">{notice ?? t("prototype.loading")}</p>
          <p className="mt-2 font-mono text-xs text-muted-foreground">
            make prototype → web/public/prototype/*.json
          </p>
        </div>
      )}
    </main>
  );
}
