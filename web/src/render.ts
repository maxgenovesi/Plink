// Draws the latest server state onto the canvas once per display frame.
// Pure view: it reads the connection and never writes to it.

import type { Connection } from "./network.ts";

// Mirrors internal/game/player.go. The server is authoritative; these only
// decide how things look.
const ARENA_WIDTH = 1500;
const ARENA_HEIGHT = 1500;
const PLAYER_RADIUS = 15;

/** Fraction of the shorter screen side the arena may fill, leaving a margin. */
const ARENA_FILL = 0.95;

const COLORS = {
  page: "#101014",
  arena: "#1b1b22",
  border: "#3a3a48",
  player: "#4fc3f7",
  text: "#9a9aae",
};

/**
 * Sizes the canvas to its CSS box and redraws it on every animation frame
 * for as long as the page is open.
 *
 * @param canvas the element to draw into; its CSS decides how big it appears
 * @param conn   the connection whose latest state is drawn each frame
 */
export function startRenderLoop(canvas: HTMLCanvasElement, conn: Connection): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("2D canvas not supported");
  }

  const frame = () => {
    resizeToDisplay(canvas);
    draw(ctx, canvas, conn);
    requestAnimationFrame(frame);
  };
  requestAnimationFrame(frame);
}

/**
 * Matches the canvas's pixel buffer to its on-screen size times the device
 * pixel ratio, so drawing stays sharp on Retina screens and after resizes.
 * Cheap when nothing changed: it only touches width/height on a mismatch,
 * because assigning them clears the canvas.
 *
 * @param canvas the canvas to resize
 */
function resizeToDisplay(canvas: HTMLCanvasElement): void {
  const dpr = window.devicePixelRatio || 1;
  const width = Math.round(canvas.clientWidth * dpr);
  const height = Math.round(canvas.clientHeight * dpr);
  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width;
    canvas.height = height;
  }
}

/**
 * Draws one frame: background, arena, and the player if a state has arrived,
 * otherwise a status line.
 *
 * @param ctx    the canvas's 2D context
 * @param canvas the canvas, for its current pixel size
 * @param conn   the connection to read status and latest state from
 */
function draw(ctx: CanvasRenderingContext2D, canvas: HTMLCanvasElement, conn: Connection): void {
  // Reset any transform from the previous frame before painting the page.
  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.fillStyle = COLORS.page;
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  // From here on, draw in arena-units: scale to fit, centred, letterboxed.
  const scale =
    Math.min(canvas.width / ARENA_WIDTH, canvas.height / ARENA_HEIGHT) * ARENA_FILL;
  const offsetX = (canvas.width - ARENA_WIDTH * scale) / 2;
  const offsetY = (canvas.height - ARENA_HEIGHT * scale) / 2;
  ctx.setTransform(scale, 0, 0, scale, offsetX, offsetY);

  ctx.fillStyle = COLORS.arena;
  ctx.fillRect(0, 0, ARENA_WIDTH, ARENA_HEIGHT);
  ctx.strokeStyle = COLORS.border;
  ctx.lineWidth = 4;
  ctx.strokeRect(0, 0, ARENA_WIDTH, ARENA_HEIGHT);

  const state = conn.latest;
  if (conn.status !== "open" || !state) {
    ctx.fillStyle = COLORS.text;
    ctx.font = "48px system-ui, sans-serif";
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    const label = conn.status === "closed" ? "disconnected — refresh to retry" : "connecting…";
    ctx.fillText(label, ARENA_WIDTH / 2, ARENA_HEIGHT / 2);
    return;
  }

  ctx.fillStyle = COLORS.player;
  ctx.beginPath();
  ctx.arc(state.you.x, state.you.y, PLAYER_RADIUS, 0, Math.PI * 2);
  ctx.fill();
}
