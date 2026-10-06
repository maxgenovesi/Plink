// WebSocket link to the Go server. The client streams held keys up; the
// server says who we are once and then streams the authoritative state of the
// whole world down. Message shapes live in protocol.ts.

import {
  assertNever,
  decodeServerMessage,
  encodeClientMessage,
  type Input,
  type State,
} from "./protocol.ts";

/** How often held keys are sent to the server. Matches the ~30/s in messages.go. */
const INPUT_RATE_HZ = 30;

/** Which movement keys are held right now: an Input without its seq. */
export type InputState = Omit<Input, "seq">;

export type ConnectionStatus = "connecting" | "open" | "closed";

/** One live connection to /ws. Read latest each frame; it is replaced, never mutated. */
export class Connection {
  status: ConnectionStatus = "connecting";

  /** Our own player id, or null until the server's welcome arrives. */
  id: string | null = null;

  /** Server ticks per second, or null until the server's welcome arrives. */
  tickRate: number | null = null;

  /** Most recent state from the server, or null until the first one arrives. */
  latest: State | null = null;

  private ws: WebSocket;
  private readInput: () => InputState;
  private seq = 0;
  private sendTimer: number | undefined;

  /**
   * Opens the socket and starts streaming input once it is open.
   *
   * @param url       the ws:// or wss:// endpoint to connect to
   * @param readInput called on every send to sample the currently held keys
   */
  constructor(url: string, readInput: () => InputState) {
    this.readInput = readInput;
    this.ws = new WebSocket(url);

    this.ws.addEventListener("open", () => {
      this.status = "open";
      console.log("connected to", url);
      this.sendTimer = window.setInterval(() => this.sendInput(), 1000 / INPUT_RATE_HZ);
    });

    this.ws.addEventListener("message", (e) => this.handleMessage(e));

    // "error" is always followed by "close", so all teardown lives here.
    this.ws.addEventListener("close", (e) => {
      this.status = "closed";
      window.clearInterval(this.sendTimer);
      console.log(`disconnected (code ${e.code})`);
    });
  }

  /** Closes the socket. Safe to call more than once. */
  close(): void {
    // 1000 = normal closure, so the server logs "disconnected", not "failed".
    this.ws.close(1000);
  }

  /** Samples the held keys and sends them with the next sequence number. */
  private sendInput(): void {
    if (this.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    this.seq++;
    this.ws.send(encodeClientMessage("input", { seq: this.seq, ...this.readInput() }));
  }

  /**
   * Decodes one frame from the server: a welcome sets our id and tick rate, a
   * state update replaces latest, and anything unrecognised is dropped.
   *
   * @param e the raw message event; the server only sends JSON text frames
   */
  private handleMessage(e: MessageEvent): void {
    const msg = decodeServerMessage(e.data);
    if (!msg) {
      console.warn("dropping unrecognised frame", e.data);
      return;
    }
    switch (msg.type) {
      case "welcome":
        this.id = msg.data.playerId;
        this.tickRate = msg.data.tickRate;
        break;
      case "state":
        this.latest = msg.data;
        break;
      default:
        assertNever(msg);
    }
  }
}

/**
 * Connects to /ws on whatever host served the page, so the same code works
 * behind the Vite dev proxy and when Go serves the build directly.
 *
 * @param readInput called on every send to sample the currently held keys
 * @return          the new connection, already connecting
 */
export function connect(readInput: () => InputState): Connection {
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  return new Connection(`${scheme}://${location.host}/ws`, readInput);
}
