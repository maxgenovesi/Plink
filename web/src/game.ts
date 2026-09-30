// Client-side game state. For now that is only which movement keys are held;
// the server owns everything else.

import type { InputState } from "./network.ts";

/**
 * Physical key → direction. Uses KeyboardEvent.code, not .key, so WASD stays
 * in the same place on AZERTY/Dvorak and is unaffected by Shift or Caps Lock.
 */
const KEY_BINDINGS: Record<string, keyof InputState> = {
  KeyW: "up",
  KeyS: "down",
  KeyA: "left",
  KeyD: "right",
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
};

/**
 * Starts listening to the keyboard and returns a sampler for the held keys,
 * ready to pass straight to connect().
 *
 * @param target where to listen for key events; window unless testing
 * @return       a function returning a fresh copy of the held keys on each call
 */
export function trackKeyboard(target: Window = window): () => InputState {
  const held: InputState = { up: false, down: false, left: false, right: false };

  target.addEventListener("keydown", (e) => {
    const dir = KEY_BINDINGS[e.code];
    if (dir) {
      held[dir] = true;
      // Stops the arrow keys from scrolling the page.
      e.preventDefault();
    }
  });

  target.addEventListener("keyup", (e) => {
    const dir = KEY_BINDINGS[e.code];
    if (dir) {
      held[dir] = false;
    }
  });

  // Alt-tabbing away mid-press means the keyup goes to another window and never
  // arrives here, which would leave the player running forever. Losing focus
  // releases everything instead.
  target.addEventListener("blur", () => {
    held.up = held.down = held.left = held.right = false;
  });

  // A copy, so a caller holding onto the result never sees it change underneath.
  return () => ({ ...held });
}
