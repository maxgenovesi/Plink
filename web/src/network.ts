// WebSocket link to the Go server. Mirrors internal/server/messages.go: the
// client streams held keys up, the server streams authoritative state down.

/** How often held keys are sent to the server. Matches the ~30/s in messages.go. */
const INPUT_RATE_HZ = 30;

/** Which movement keys are held right now. */
export interface InputState {
  up: boolean;
  down: boolean;
  left: boolean;
  right: boolean;
}

/** Wire shape of clientInput. */
interface ClientInput extends InputState {
  seq: number;
}

/** Wire shape of playerState. */
export interface PlayerState {
  id: string;
  x: number;
  y: number;
}

/** Wire shape of serverState, sent once per server tick. */
export interface ServerState {
  type: "state";
  tick: number;
  ackSeq: number;
  you: PlayerState;
}

export type ConnectionStatus = "connecting" | "open" | "closed";

/** One live connection to /ws. Read latest each frame; it is replaced, never mutated. */
export class Connection {
  status: ConnectionStatus = "connecting";

  /** Most recent state from the server, or null until the first one arrives. */
  latest: ServerState | null = null;

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
    const msg: ClientInput = { seq: this.seq, ...this.readInput() };
    this.ws.send(JSON.stringify(msg));
  }

  /**
   * Decodes one frame from the server and stores it if it is a state update.
   *
   * @param e the raw message event; the server only sends JSON text frames
   */
  private handleMessage(e: MessageEvent): void {
    let msg: unknown;
    try {
      msg = JSON.parse(e.data);
    } catch {
      console.warn("dropping non-JSON frame", e.data);
      return;
    }
    if (isServerState(msg)) {
      this.latest = msg;
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

/**
 * Narrows an unknown decoded message to a ServerState.
 *
 * @param msg the value produced by JSON.parse
 * @return    true if msg has the "state" shape the server sends
 */
function isServerState(msg: unknown): msg is ServerState {
  return typeof msg === "object" && msg !== null && (msg as { type?: unknown }).type === "state";
}
