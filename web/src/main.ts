// Entry point for the Plink client. Vite bundles everything reachable from here.

import { trackKeyboard } from "./game.ts";
import { connect } from "./network.ts";
import { startRenderLoop } from "./render.ts";

const canvas = document.querySelector<HTMLCanvasElement>("#game");
if (!canvas) {
  throw new Error("missing #game canvas");
}

const conn = connect(trackKeyboard());
startRenderLoop(canvas, conn);

// Exposed for poking at from the DevTools console.
Object.assign(window, { conn });
