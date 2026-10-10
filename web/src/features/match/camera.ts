/**
 * Broadcast camera math (ADR-0012): frame the ACTION like a TV truck — zoomed in,
 * ball centred, clamped inside the pitch, eased every frame. Pure functions so the
 * framing rules stay unit-testable.
 */

export const CAMERA_ZOOM = 1.9;

export const TRAIL_WINDOW_MS = 1200;

/** Sample elapsed story during playback; a seek starts a fresh ribbon at its destination. */
export function trailSampleTimes(previousMs: number, nextMs: number, scrubbed: boolean): number[] {
  const stepMs = nextMs - previousMs;
  if (scrubbed || previousMs < 0 || stepMs <= 0) return [nextMs];
  const count = Math.min(64, Math.max(1, Math.ceil(stepMs / 60)));
  return Array.from({ length: count }, (_, i) => previousMs + (stepMs * (i + 1)) / count);
}

export interface CameraFrame {
  scale: number;
  x: number;
  y: number;
}

/** Computes the ideal (unsmoothed) camera frame around the ball. */
export function cameraFrame(
  viewW: number,
  viewH: number,
  pitchW: number,
  pitchH: number,
  ballX: number,
  ballY: number,
  zoom = CAMERA_ZOOM,
): CameraFrame {
  const base = Math.min(viewW / pitchW, viewH / pitchH);
  const scale = base * zoom;
  const frameW = pitchW * scale;
  const frameH = pitchH * scale;

  // Centre the ball (normalized 0..1), then clamp so the camera never leaves the pitch.
  let x = viewW / 2 - ballX * pitchW * scale;
  let y = viewH / 2 - ballY * pitchH * scale;
  x = Math.min(0, Math.max(x, viewW - frameW));
  y = Math.min(0, Math.max(y, viewH - frameH));
  return { scale, x, y };
}

/** Exponential easing toward the ideal frame (no jitter, TV-smooth pans). */
export function easeCamera(current: CameraFrame, target: CameraFrame, alpha = 0.08): CameraFrame {
  return {
    scale: current.scale + (target.scale - current.scale) * alpha,
    x: current.x + (target.x - current.x) * alpha,
    y: current.y + (target.y - current.y) * alpha,
  };
}
