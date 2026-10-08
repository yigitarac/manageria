import { useEffect, useRef } from "react";
import { Application, Container, Graphics, Text } from "pixi.js";
import { ballAt, buildBallStory } from "./ballStory";
import { cameraFrame, easeCamera, timelineJump } from "./camera";
import { sampleAt } from "./interpolate";
import type { FrameSample } from "./interpolate";
import type { MatchDump } from "./types";

const PITCH_W = 1050;
const PITCH_H = 680;
// Stand bands around the field: the broadcast world is bigger than the paint.
const PAD = 140;
const WORLD_W = PITCH_W + 2 * PAD;
const WORLD_H = PITCH_H + 2 * PAD;
const DOT_R = 13;

// Kit palette per squad identity (fictional clubs): kit primary, trim, keeper kit.
interface Kit {
  primary: number;
  trim: number;
  gk: number;
  numberFill: string;
}
const HOME_KIT: Kit = { primary: 0x38bdf8, trim: 0xe0f2fe, gk: 0xa3e635, numberFill: "#082f49" };
const AWAY_KIT: Kit = { primary: 0xfb923c, trim: 0x431407, gk: 0xc084fc, numberFill: "#431407" };

interface Props {
  dump: MatchDump;
  /** Live clock source read by the render ticker (no React churn per frame). */
  getTimeMs: () => number;
}

/**
 * 2D match view: a PixiJS broadcast scene — mowed pitch inside its stands, kit-coloured
 * numbered shirts with names, player/ball shadows, corner flags, and a ball that plays
 * the match's own touch-ledger story with a readable trail.
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

    void app.init({ resizeTo: host, background: "#0b1220", antialias: true }).then(() => {
      ready = true;
      if (disposed) {
        destroy();
        return;
      }
      host.appendChild(app.canvas);

      const scene = buildScene(app.stage, dump);
      // The ball plays the match's own story (touch ledger): passes arc from touch to
      // touch, shots fly at goal, goals nestle into the net — no invented motion.
      const ballStory = buildBallStory(dump);
      let cam = cameraFrame(app.screen.width, app.screen.height, WORLD_W, WORLD_H, 0.5, 0.5);
      let previousTime = -1;
      app.ticker.add(() => {
        const tMs = timeRef.current();
        const sample = sampleAt(dump.keyframes, tMs);
        if (sample) {
          const jumped = timelineJump(previousTime, tMs);
          const b = ballAt(ballStory, tMs);
          const ball = b ?? { x: sample.ballX, y: sample.ballY };
          applySample(scene, sample, ball, tMs, previousTime);
          previousTime = tMs;
          // Broadcast camera: glide to frame the action like a TV truck.
          const target = cameraFrame(
            app.screen.width,
            app.screen.height,
            WORLD_W,
            WORLD_H,
            scene.ball.x / WORLD_W,
            scene.ball.y / WORLD_H,
          );
          cam = jumped ? target : easeCamera(cam, target);
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
  shadows: Graphics[];
  labels: Text[];
  names: Text[];
  ball: Graphics;
  ballShadow: Graphics;
  trail: Graphics;
  trailPts: { x: number; y: number }[];
}

function buildScene(stage: Container, dump: MatchDump): Scene {
  const root = new Container();
  stage.addChild(root);

  root.addChild(buildStands());
  root.addChild(buildPitch());

  const shadows: Graphics[] = [];
  const dots: Graphics[] = [];
  const labels: Text[] = [];
  const names: Text[] = [];
  for (let i = 0; i < 22; i++) {
    const home = i < 11;
    const kit = home ? HOME_KIT : AWAY_KIT;
    const keeper = i % 11 === 0;
    const body = keeper ? kit.gk : kit.primary;

    const shadow = new Graphics().ellipse(0, 0, 15, 5.5).fill({ color: 0x000000, alpha: 0.32 });
    const dot = new Graphics()
      .circle(0, 0, DOT_R)
      .fill(body)
      .stroke({ width: 2.5, color: kit.trim });
    const label = new Text({
      text: String((i % 11) + 1),
      style: { fontSize: 15, fontWeight: "700", fill: kit.numberFill },
      anchor: 0.5,
    });
    const roster = home ? dump.teams.home.players : dump.teams.away.players;
    const name = new Text({
      text: roster[i % 11]?.name ?? "",
      style: { fontSize: 11, fontWeight: "600", fill: "#f8fafc" },
      anchor: { x: 0.5, y: 0 },
    });
    name.alpha = 0.75;
    root.addChild(shadow, dot, label, name);
    shadows.push(shadow);
    dots.push(dot);
    labels.push(label);
    names.push(name);
  }

  const ballShadow = new Graphics().ellipse(0, 0, 9, 3.5).fill({ color: 0x000000, alpha: 0.3 });
  const ball = new Graphics()
    .circle(0, 0, 8)
    .fill("#f8fafc")
    .stroke({ width: 2, color: "#0f172a" });
  const trail = new Graphics();
  root.addChild(trail, ballShadow, ball);

  return { root, dots, shadows, labels, names, ball, ballShadow, trail, trailPts: [] };
}

// buildStands paints the world backdrop: dark tiered stands with a static crowd
// texture and scarves sprinkled in both club colours (deterministic pattern — no
// randomness anywhere in rendering).
function buildStands(): Graphics {
  const g = new Graphics();
  g.rect(0, 0, WORLD_W, WORLD_H).fill("#111a2e");
  // Tier shading bands.
  for (const [inset, shade] of [
    [0, 0x1c2740],
    [34, 0x223052],
    [68, 0x2a3a63],
  ] as const) {
    g.rect(inset, inset, WORLD_W - 2 * inset, WORLD_H - 2 * inset).stroke({
      width: 30,
      color: shade,
    });
  }
  // Crowd texture: seated rows of heads (hash pattern, cheap and stable).
  for (let row = 0; row < 7; row++) {
    for (let col = 0; col < 90; col++) {
      const h = Math.abs(Math.sin(row * 12.9898 + col * 78.233));
      const x = (col / 90) * WORLD_W + (h - 0.5) * 8;
      const bands = [
        { y: 18 + row * 15, w: WORLD_W },
        { y: WORLD_H - 26 - row * 15, w: WORLD_W },
      ];
      for (const band of bands) {
        const scarf = h > 0.93 ? 0xfb923c : h > 0.86 ? 0x38bdf8 : 0x334155;
        g.circle(x, band.y + (h - 0.5) * 6, 3.2).fill({ color: scarf, alpha: 0.55 });
      }
      const sideY = (row / 7) * (PITCH_H + 2 * PAD) + 40;
      for (const sx of [14 + row * 15, WORLD_W - 22 - row * 15]) {
        g.circle(sx + (h - 0.5) * 6, sideY, 3.2).fill({ color: 0x334155, alpha: 0.5 });
      }
    }
  }
  return g;
}

// buildPitch paints the field proper: mowed stripes, markings, angled nets and the
// four corner flags — offset into the stand-ringed world by PAD.
function buildPitch(): Graphics {
  const g = new Graphics();
  // Mowed stripes — the first step from diagram to broadcast.
  for (let i = 0; i < 10; i += 2) {
    g.rect(PAD + (i * PITCH_W) / 10, PAD, PITCH_W / 10, PITCH_H).fill("#1a6b36");
  }
  for (let i = 1; i < 10; i += 2) {
    g.rect(PAD + (i * PITCH_W) / 10, PAD, PITCH_W / 10, PITCH_H).fill("#155e32");
  }

  const line = { width: 3, color: "#ffffffcc" };
  g.rect(PAD + 8, PAD + 8, PITCH_W - 16, PITCH_H - 16)
    .stroke(line)
    .moveTo(PAD + PITCH_W / 2, PAD + 8)
    .lineTo(PAD + PITCH_W / 2, PAD + PITCH_H - 8)
    .stroke(line)
    .circle(PAD + PITCH_W / 2, PAD + PITCH_H / 2, 92)
    .stroke(line)
    .rect(PAD + 8, PAD + PITCH_H / 2 - 160, 155, 320)
    .rect(PAD + PITCH_W - 163, PAD + PITCH_H / 2 - 160, 155, 320)
    .rect(PAD + 8, PAD + PITCH_H / 2 - 72, 58, 144)
    .rect(PAD + PITCH_W - 66, PAD + PITCH_H / 2 - 72, 58, 144)
    .stroke(line);

  // Angled goal nets: hatched boxes behind both goal lines.
  for (const side of [0, 1]) {
    const gx = side === 0 ? PAD + 2 : PAD + PITCH_W - 22;
    g.rect(gx, PAD + PITCH_H / 2 - 44, 20, 88).fill("#e8e8e8cc");
    for (let i = 0; i <= 20; i += 5) {
      g.moveTo(gx + i, PAD + PITCH_H / 2 - 44)
        .lineTo(gx + i, PAD + PITCH_H / 2 + 44)
        .stroke({ width: 1, color: "#94a3b8" })
        .moveTo(gx, PAD + PITCH_H / 2 - 44 + i * 4.4)
        .lineTo(gx + 20, PAD + PITCH_H / 2 - 44 + i * 4.4)
        .stroke({ width: 1, color: "#94a3b8" });
    }
  }

  // Corner flags: pole + pennant at each corner of the field.
  for (const [cx, cy, dir] of [
    [PAD + 4, PAD + 4, 1],
    [PAD + PITCH_W - 4, PAD + 4, -1],
    [PAD + 4, PAD + PITCH_H - 4, 1],
    [PAD + PITCH_W - 4, PAD + PITCH_H - 4, -1],
  ] as const) {
    g.moveTo(cx, cy)
      .lineTo(cx, cy - 22)
      .stroke({ width: 2, color: "#e2e8f0" });
    g.moveTo(cx, cy - 22)
      .lineTo(cx + 14 * dir, cy - 18)
      .lineTo(cx, cy - 13)
      .fill(0xfacc15);
  }
  return g;
}

// worldOf maps pitch-space (0..1) into the stand-ringed world.
function worldOf(nx: number, ny: number): { x: number; y: number } {
  return { x: PAD + nx * PITCH_W, y: PAD + ny * PITCH_H };
}

function applySample(
  scene: Scene,
  sample: FrameSample,
  ball: { x: number; y: number },
  tMs: number,
  previousTime: number,
) {
  const bw = worldOf(ball.x, ball.y);
  const nearby: { index: number; distance: number; x: number; y: number }[] = [];
  for (let i = 0; i < scene.dots.length; i++) {
    const p = sample.players[i];
    if (!p) continue;
    // Micro-locomotion: real footballers breathe/jockey in place — pure statues read
    // as broken. A tiny per-player phase sway sells life without inventing travel.
    const sway = Math.sin(tMs / 640 + i * 1.7) * 1.4;
    const w = worldOf(p.x, p.y);
    const x = w.x + sway;
    const y = w.y + Math.cos(tMs / 720 + i) * 1.1;
    scene.dots[i].position.set(x, y);
    scene.labels[i].position.set(x, y);
    scene.names[i].position.set(x, y + DOT_R + 4);
    scene.names[i].visible = false;
    scene.shadows[i].position.set(x, y + DOT_R + 2);
    const distance = Math.hypot(x - bw.x, y - bw.y);
    if (distance < 160) nearby.push({ index: i, distance, x, y });
  }
  // A broadcast labels the players in the action, not all 22 at once. Avoid
  // stacking two long names when opponents contest the same ball.
  nearby.sort((a, b) => a.distance - b.distance);
  const labeled: typeof nearby = [];
  for (const player of nearby) {
    if (labeled.length >= 3) break;
    if (
      labeled.some(
        (other) => Math.abs(other.x - player.x) < 90 && Math.abs(other.y - player.y) < 28,
      )
    )
      continue;
    scene.names[player.index].visible = true;
    labeled.push(player);
  }
  // Ball trail: a tapered ribbon of the last frames — motion readable at any speed.
  if (timelineJump(previousTime, tMs)) scene.trailPts.length = 0;
  scene.trailPts.push(bw);
  if (scene.trailPts.length > 14) scene.trailPts.shift();
  scene.trail.clear();
  for (let i = 1; i < scene.trailPts.length; i++) {
    const a = scene.trailPts[i - 1];
    const b = scene.trailPts[i];
    const fade = i / scene.trailPts.length;
    scene.trail
      .moveTo(a.x, a.y)
      .lineTo(b.x, b.y)
      .stroke({ width: 1 + 3 * fade, color: 0xf8fafc, alpha: 0.06 + 0.3 * fade });
  }
  scene.ball.position.set(bw.x, bw.y);
  scene.ballShadow.position.set(bw.x, bw.y + 11);
}
