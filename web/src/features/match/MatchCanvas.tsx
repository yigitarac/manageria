import { useEffect, useRef } from "react";
import { Application, Container, Graphics, Text } from "pixi.js";
import { sampleAt } from "./interpolate";
import type { FrameSample } from "./interpolate";
import type { MatchDump } from "./types";

const PITCH_W = 1050;
const PITCH_H = 680;
const HOME_COLOR = "#38bdf8";
const AWAY_COLOR = "#fb923c";
const DOT_R = 13;

interface Props {
  dump: MatchDump;
  /** Live clock source read by the render ticker (no React churn per frame). */
  getTimeMs: () => number;
}

/**
 * 2D match view: a PixiJS pitch with 22 numbered circles and the ball.
 * Positions interpolate between keyframes every frame (circles-on-a-pitch visual).
 */
export function MatchCanvas({ dump, getTimeMs }: Props) {
  const hostRef = useRef<HTMLDivElement>(null);
  const timeRef = useRef(getTimeMs);
  timeRef.current = getTimeMs;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const app = new Application();
    let cancelled = false;

    void app.init({ resizeTo: host, background: "#14532d", antialias: true }).then(() => {
      if (cancelled) {
        app.destroy(true, { children: true });
        return;
      }
      host.appendChild(app.canvas);

      const scene = buildScene(app.stage);
      app.ticker.add(() => {
        const sample = sampleAt(dump.keyframes, timeRef.current());
        if (sample) applySample(scene, sample);
        fitScene(app.screen.width, app.screen.height, scene.root);
      });
    });

    return () => {
      cancelled = true;
      app.destroy(true, { children: true });
    };
  }, [dump]);

  return (
    <div
      ref={hostRef}
      className="h-full w-full overflow-hidden rounded-xl border border-border"
      aria-label="match pitch"
    />
  );
}

interface Scene {
  root: Container;
  dots: Graphics[];
  labels: Text[];
  ball: Graphics;
}

function buildScene(stage: Container): Scene {
  const root = new Container();
  stage.addChild(root);

  const pitch = new Graphics()
    .rect(0, 0, PITCH_W, PITCH_H)
    .fill("#166534")
    .rect(8, 8, PITCH_W - 16, PITCH_H - 16)
    .stroke({ width: 3, color: "#ffffffaa" })
    .moveTo(PITCH_W / 2, 8)
    .lineTo(PITCH_W / 2, PITCH_H - 8)
    .stroke({ width: 3, color: "#ffffffaa" })
    .circle(PITCH_W / 2, PITCH_H / 2, 92)
    .stroke({ width: 3, color: "#ffffffaa" })
    .rect(8, PITCH_H / 2 - 160, 155, 320)
    .rect(PITCH_W - 163, PITCH_H / 2 - 160, 155, 320)
    .stroke({ width: 3, color: "#ffffffaa" });
  root.addChild(pitch);

  const dots: Graphics[] = [];
  const labels: Text[] = [];
  for (let i = 0; i < 22; i++) {
    const dot = new Graphics()
      .circle(0, 0, DOT_R)
      .fill(i < 11 ? HOME_COLOR : AWAY_COLOR)
      .stroke({ width: 2, color: "#0f172a" });
    const label = new Text({
      text: String((i % 11) + 1),
      style: { fontSize: 15, fontWeight: "700", fill: "#0f172a" },
      anchor: 0.5,
    });
    root.addChild(dot, label);
    dots.push(dot);
    labels.push(label);
  }

  const ball = new Graphics()
    .circle(0, 0, 8)
    .fill("#f8fafc")
    .stroke({ width: 2, color: "#0f172a" });
  root.addChild(ball);

  return { root, dots, labels, ball };
}

function applySample(scene: Scene, sample: FrameSample) {
  for (let i = 0; i < scene.dots.length; i++) {
    const p = sample.players[i];
    if (!p) continue;
    const x = p.x * PITCH_W;
    const y = p.y * PITCH_H;
    scene.dots[i].position.set(x, y);
    scene.labels[i].position.set(x, y);
  }
  scene.ball.position.set(sample.ballX * PITCH_W, sample.ballY * PITCH_H);
}

function fitScene(width: number, height: number, root: Container) {
  if (width <= 0 || height <= 0) return;
  const scale = Math.min(width / PITCH_W, height / PITCH_H);
  root.scale.set(scale);
  root.position.set((width - PITCH_W * scale) / 2, (height - PITCH_H * scale) / 2);
}
