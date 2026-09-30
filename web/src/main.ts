// Entry point for the Plink client. Vite bundles everything reachable from here.

const canvas = document.querySelector<HTMLCanvasElement>("#game");
if (!canvas) {
  throw new Error("missing #game canvas");
}

console.log("Plink client loaded", canvas);
