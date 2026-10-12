import type { PrototypeState, PrototypeTrace } from "./types";

/**
 * A deliberately simple, dependency-free SVG renderer for the causal prototype.
 * It draws ONLY the authoritative state handed to it (no simulation, no
 * ballStory retiming): the parent passes the interpolated state from
 * `interpolateTrace`, so what is drawn is what the server simulated.
 */

const RUNNER_COLORS = ["#38bdf8", "#f97316", "#a3e635", "#e879f9"];
const RUNNER_RADIUS_M = 1.1;
const BALL_RADIUS_M = 0.55;

export function PrototypeCanvas({
  trace,
  state,
  label,
}: {
  trace: PrototypeTrace;
  state: PrototypeState;
  label: string;
}) {
  const { lengthM, widthM } = trace.pitch;
  return (
    <svg
      data-testid="prototype-canvas"
      viewBox={`0 0 ${lengthM} ${widthM}`}
      role="img"
      aria-label={label}
      className="h-full w-full rounded-lg bg-emerald-700"
    >
      <rect x={0} y={0} width={lengthM} height={widthM} fill="#2f7d4f" />
      {/* Markings: outline, halfway line, centre circle. */}
      <rect
        x={1}
        y={1}
        width={lengthM - 2}
        height={widthM - 2}
        fill="none"
        stroke="rgba(255,255,255,0.75)"
        strokeWidth={0.4}
      />
      <line
        x1={lengthM / 2}
        y1={1}
        x2={lengthM / 2}
        y2={widthM - 1}
        stroke="rgba(255,255,255,0.75)"
        strokeWidth={0.4}
      />
      <circle
        cx={lengthM / 2}
        cy={widthM / 2}
        r={9.15}
        fill="none"
        stroke="rgba(255,255,255,0.75)"
        strokeWidth={0.4}
      />

      {state.runners.map((r, i) => (
        <circle
          key={i}
          data-testid={`runner-${i}`}
          cx={r.x * lengthM}
          cy={r.y * widthM}
          r={RUNNER_RADIUS_M}
          fill={RUNNER_COLORS[i % RUNNER_COLORS.length]}
          stroke={state.owner === i ? "#ffffff" : "rgba(0,0,0,0.45)"}
          strokeWidth={state.owner === i ? 0.7 : 0.25}
        />
      ))}

      <circle
        data-testid="prototype-ball"
        cx={state.ballX * lengthM}
        cy={state.ballY * widthM}
        r={BALL_RADIUS_M}
        fill={state.mode === "out" ? "#94a3b8" : "#ffffff"}
        stroke="#111827"
        strokeWidth={0.2}
      />
    </svg>
  );
}
