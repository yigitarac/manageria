import { useEffect, useRef } from "react";
import { Application, Container, Graphics, Text } from "pixi.js";
import { ballAt, buildBallStory } from "./ballStory";
import { cameraFrame, easeCamera } from "./camera";
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
    let disposed = false;
    let ready = false;

    // StrictMode double-mounts effects; destroying a Pixi Application before its
    // async init() settles blows up the resize plugin (_cancelResize). Guard both
    // directions: init resolution cleans up a disposed scene, cleanup waits for init.
    const destroy = () => {
      try {
        app.destroy({ removeView: true }, { children: true });
      } catch {
        // teardown races are not actionable
      }
    };

    void app.init({ resizeTo: host, background: "#14532d", antialias: true }).then(() => {
      ready = true;
      if (disposed) {
        destroy();
        return;
      }
      host.appendChild(app.canvas);

      const scene = buildScene(app.stage);
      // The ball plays the match's own story (touch ledger): passes arc from touch to
      // touch, shots fly at goal, goals nestle into the net — no invented motion.
      const ballStory = buildBallStory(dump);
      let cam = cameraFrame(app.screen.width, app.screen.height, PITCH_W, PITCH_H, 0.5, 0.5);
      app.ticker.add(() => {
        const tMs = timeRef.current();
        const sample = sampleAt(dump.keyframes, tMs);
        if (sample) {
          applySample(scene, sample, tMs);
          const b = ballAt(ballStory, tMs);
          if (b) scene.ball.position.set(b.x * PITCH_W, b.y * PITCH_H);
          // Broadcast camera: glide to frame the action like a TV truck.
          const target = cameraFrame(
            app.screen.width,
            app.screen.height,
            PITCH_W,
            PITCH_H,
            scene.ball.x / PITCH_W,
            scene.ball.y / PITCH_H,
          );
          cam = easeCamera(cam, target);
          scene.root.scale.set(cam.scale);
          scene.root.position.set(cam.x, cam.y);
        }
      });
    });

    return () => {
      disposed = true;
      if (ready) destroy();
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
  trail: Graphics;
  prevBall: { x: number; y: number };
}

function buildScene(stage: Container): Scene {
  const root = new Container();
  stage.addChild(root);

  // Mowed stripes — the first step from diagram to broadcast.
  const stripes = new Graphics();
  for (let i = 0; i < 10; i += 2) {
    stripes.rect((i * PITCH_W) / 10, 0, PITCH_W / 10, PITCH_H).fill("#1a6b36");
  }
  root.addChild(stripes);

  const pitch = new Graphics()
    .rect(8, 8, PITCH_W - 16, PITCH_H - 16)
    .stroke({ width: 3, color: "#ffffffcc" })
    .moveTo(PITCH_W / 2, 8)
    .lineTo(PITCH_W / 2, PITCH_H - 8)
    .stroke({ width: 3, color: "#ffffffcc" })
    .circle(PITCH_W / 2, PITCH_H / 2, 92)
    .stroke({ width: 3, color: "#ffffffcc" })
    .rect(8, PITCH_H / 2 - 160, 155, 320)
    .rect(PITCH_W - 163, PITCH_H / 2 - 160, 155, 320)
    .rect(8, PITCH_H / 2 - 72, 58, 144)
    .rect(PITCH_W - 66, PITCH_H / 2 - 72, 58, 144)
    .stroke({ width: 3, color: "#ffffffcc" });
  root.addChild(pitch);

  // Angled goal nets: hatched boxes behind both goal lines.
  const nets = new Graphics();
  for (const side of [0, 1]) {
    const gx = side === 0 ? 2 : PITCH_W - 22;
    nets.rect(gx, PITCH_H / 2 - 44, 20, 88).fill("#e8e8e8cc");
    for (let i = 0; i <= 20; i += 5) {
      nets
        .moveTo(gx + i, PITCH_H / 2 - 44)
        .lineTo(gx + i, PITCH_H / 2 + 44)
        .stroke({ width: 1, color: "#94a3b8" })
        .moveTo(gx, PITCH_H / 2 - 44 + i * 4.4)
        .lineTo(gx + 20, PITCH_H / 2 - 44 + i * 4.4)
        .stroke({ width: 1, color: "#94a3b8" });
    }
  }
  root.addChild(nets);

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
  const trail = new Graphics();
  root.addChild(trail, ball);

  return { root, dots, labels, ball, trail, prevBall: { x: PITCH_W / 2, y: PITCH_H / 2 } };
}

function applySample(scene: Scene, sample: FrameSample, tMs: number) {
  for (let i = 0; i < scene.dots.length; i++) {
    const p = sample.players[i];
    if (!p) continue;
    // Micro-locomotion: real footballers breathe/jockey in place — pure statues read
    // as broken. A tiny per-player phase sway sells life without inventing travel.
    const sway = Math.sin(tMs / 640 + i * 1.7) * 1.4;
    const x = p.x * PITCH_W + sway;
    const y = p.y * PITCH_H + Math.cos(tMs / 720 + i) * 1.1;
    scene.dots[i].position.set(x, y);
    scene.labels[i].position.set(x, y);
  }
  const bx = sample.ballX * PITCH_W;
  const by = sample.ballY * PITCH_H;
  // Ball trail: motion you can read at any playback speed.
  scene.trail.clear();
  scene.trail
    .moveTo(scene.prevBall.x, scene.prevBall.y)
    .lineTo(bx, by)
    .stroke({ width: 3, color: "#f8fafc66" });
  scene.prevBall = { x: bx, y: by };
  scene.ball.position.set(bx, by);
}


