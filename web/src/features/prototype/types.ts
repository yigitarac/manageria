/**
 * Same-clock causal prototype trace contract (T-039).
 *
 * This mirrors the versioned JSON emitted by `server/cmd/liveplaydemo`
 * (`server/internal/liveplay/export.go`, `TraceSchemaVersion`). It is NOT the
 * legacy engine MatchResult dump (`features/match/types.ts`) and must never be
 * reconstructed with `ballStory` elastic retiming: every point below is
 * authoritative and the renderer only interpolates between the emitted knots.
 */

export interface PrototypePitch {
  lengthM: number;
  widthM: number;
}

export interface PrototypeRunnerState {
  x: number;
  y: number;
}

/**
 * One authoritative interpolation knot. `kind` is "sample" for an integer-clock
 * sample or the event kind ("release" | "path" | "control" | "deflect" |
 * "stop" | "out"). `owner` is -1 unless a held ball has a controller; the
 * renderer takes mode/owner from the LEFT knot so a transition happens exactly at
 * the event's `tMs`, never at the following end-step sample.
 */
export interface PrototypePoint {
  tMs: number;
  kind: string;
  ballX: number;
  ballY: number;
  vxM: number;
  vyM: number;
  mode: string;
  owner: number;
  runners: PrototypeRunnerState[];
}

export interface PrototypeEvent {
  seq: number;
  kind: string;
  clockMs: number;
  frac: number;
  exactTimeMs: number;
  runner: number;
  ballX: number;
  ballY: number;
  vxM: number;
  vyM: number;
  outVxM: number;
  outVyM: number;
  runnerX: number;
  runnerY: number;
  scripted: boolean;
}

export interface PrototypeTrace {
  schemaVersion: number;
  timeUnit: string;
  substepMs: number;
  seed: number;
  slug: string;
  label: string;
  description: string;
  scriptedNote: string;
  pitch: PrototypePitch;
  runnerCount: number;
  sampleCount: number;
  frameCount: number;
  eventCount: number;
  events: PrototypeEvent[];
  points: PrototypePoint[];
}

/** The rendered state at a query time, derived purely from the trace knots. */
export interface PrototypeState {
  ballX: number;
  ballY: number;
  mode: string;
  owner: number;
  runners: PrototypeRunnerState[];
}

/** Ball modes the exporter can emit. */
const BALL_MODES = new Set(["held", "rolling", "loose", "out", "dead"]);
/** Event kinds the exporter can emit. */
const EVENT_KINDS = new Set(["release", "path", "control", "deflect", "stop", "out"]);
/** The only schema version this renderer supports. */
export const PROTOTYPE_SCHEMA_VERSION = 1;

function isFiniteNumber(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v);
}

function isRunnerState(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const r = value as Record<string, unknown>;
  return isFiniteNumber(r.x) && isFiniteNumber(r.y);
}

function isPoint(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const p = value as Record<string, unknown>;
  if (typeof p.kind !== "string" || !BALL_MODES.has(p.mode as string)) return false;
  if (!isFiniteNumber(p.tMs) || !isFiniteNumber(p.ballX) || !isFiniteNumber(p.ballY)) return false;
  if (!isFiniteNumber(p.vxM) || !isFiniteNumber(p.vyM)) return false;
  if (typeof p.owner !== "number" || !Number.isInteger(p.owner) || p.owner < -1) return false;
  if (!Array.isArray(p.runners)) return false;
  return p.runners.every(isRunnerState);
}

function isEvent(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const e = value as Record<string, unknown>;
  if (typeof e.seq !== "number" || !Number.isInteger(e.seq)) return false;
  if (typeof e.kind !== "string" || !EVENT_KINDS.has(e.kind)) return false;
  if (typeof e.clockMs !== "number" || !Number.isInteger(e.clockMs)) return false;
  if (typeof e.runner !== "number" || !Number.isInteger(e.runner) || e.runner < -1) return false;
  if (!isFiniteNumber(e.frac) || e.frac < 0 || e.frac > 1) return false;
  if (!isFiniteNumber(e.exactTimeMs)) return false;
  for (const key of [
    "ballX",
    "ballY",
    "vxM",
    "vyM",
    "outVxM",
    "outVyM",
    "runnerX",
    "runnerY",
  ]) {
    if (!isFiniteNumber(e[key])) return false;
  }
  return typeof e.scripted === "boolean";
}

/**
 * Strict, throw-free structural guard for a loaded/generated prototype trace.
 * Only the supported schema version and a fully finite, internally consistent
 * shape is accepted, so a malformed or incompatible file can never crash the UI
 * (the caller shows a "bad file" notice instead). Checks: supported
 * `schemaVersion === 1`; `timeUnit === "ms"`; positive finite substep and pitch;
 * every point's finite coords/mode/owner and exactly `runnerCount` runners;
 * monotonic point times; and every event's essential fields.
 */
export function isPrototypeTrace(value: unknown): value is PrototypeTrace {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  if (v.schemaVersion !== PROTOTYPE_SCHEMA_VERSION) return false;
  if (v.timeUnit !== "ms") return false;
  if (!isFiniteNumber(v.substepMs) || v.substepMs <= 0) return false;
  if (!isFiniteNumber(v.seed)) return false;
  const pitch = v.pitch;
  if (!pitch || typeof pitch !== "object") return false;
  const { lengthM, widthM } = pitch as Record<string, unknown>;
  if (!isFiniteNumber(lengthM) || lengthM <= 0) return false;
  if (!isFiniteNumber(widthM) || widthM <= 0) return false;
  if (!Number.isInteger(v.runnerCount) || (v.runnerCount as number) < 0) return false;
  const runnerCount = v.runnerCount as number;
  if (!Array.isArray(v.points) || v.points.length === 0) return false;
  if (!Array.isArray(v.events)) return false;

  let previousT = Number.NEGATIVE_INFINITY;
  for (const raw of v.points) {
    if (!isPoint(raw)) return false;
    const point = raw as { tMs: number; owner: number; runners: unknown[] };
    if (point.tMs < previousT) return false; // times must be non-decreasing
    previousT = point.tMs;
    if (point.owner >= runnerCount) return false;
    if (point.runners.length !== runnerCount) return false;
  }
  return v.events.every(isEvent);
}

