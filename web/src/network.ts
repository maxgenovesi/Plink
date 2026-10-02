// WebSocket link to the Go server. Mirrors internal/server/messages.go: the
// client streams held keys up, the server says who we are once and then
// streams the authoritative state of the whole world down.

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

/** Wire shape of playerState: one player's entry in a WorldState. */
export interface PlayerState {
  id: string;
  x: number;
  y: number;
  /** Last input seq the server recorded for this player. */
  ackSeq: number;
}

/** Wire shape of serverWelcome, the first frame on every connection. */
export interface ServerWelcome {
  type: "welcome";
  id: string;
}

/** Wire shape of worldState, sent once per server tick. Same for every client. */
export interface WorldState {
  type: "state";
  tick: number;
  /** Every player in the game, in no particular order; match by id. */
  players: PlayerState[];
}

export type ConnectionStatus = "connecting" | "open" | "closed";

/** One live connection to /ws. Read latest each frame; it is replaced, never mutated. */
export class Connection {
  status: ConnectionStatus = "connecting";

  /** Our own player id, or null until the server's welcome arrives. */
  id: string | null = null;

  /** Most recent state from the server, or null until the first one arrives. */
  latest: WorldState | null = null;

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
   * Decodes one frame from the server: a welcome sets our id, a state update
   * replaces latest, and anything else is ignored.
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
    if (isWelcome(msg)) {
      this.id = msg.id;
    } else if (isWorldState(msg)) {
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
 * Narrows an unknown decoded message to a ServerWelcome.
 *
 * @param msg the value produced by JSON.parse
 * @return    true if msg has the "welcome" type the server sends
 */
function isWelcome(msg: unknown): msg is ServerWelcome {
  return typeof msg === "object" && msg !== null && (msg as { type?: unknown }).type === "welcome";
}

/**
 * Narrows an unknown decoded message to a WorldState.
 *
 * @param msg the value produced by JSON.parse
 * @return    true if msg has the "state" type the server sends
 */
function isWorldState(msg: unknown): msg is WorldState {
  return typeof msg === "object" && msg !== null && (msg as { type?: unknown }).type === "state";
}
