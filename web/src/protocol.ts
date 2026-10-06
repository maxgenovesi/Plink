// The wire format, mirroring internal/protocol/messages.go. Every frame in
// both directions is an envelope: { type, data }.
//
// Adding a message means, in this file: its data interface, a member of
// ServerMessage or ClientMessage, and (for server messages) an entry in
// SERVER_TYPES. The compiler then flags every switch that must handle it.

/** Wire shape of protocol.Welcome, the first frame on every connection. */
export interface Welcome {
  playerId: string;
  /** How many State frames per second the server sends. */
  tickRate: number;
}

/** Wire shape of protocol.State, sent once per server tick. Same for every client. */
export interface State {
  tick: number;
  /** Every player in the game, in no particular order; match by id. */
  players: PlayerState[];
}

/** Wire shape of protocol.PlayerState: one player's entry in a State. */
export interface PlayerState {
  id: string;
  x: number;
  y: number;
  /** Last input seq the server recorded for this player. */
  ackSeq: number;
}

/** Wire shape of protocol.Input: the movement keys held right now. */
export interface Input {
  seq: number;
  up: boolean;
  down: boolean;
  left: boolean;
  right: boolean;
}

/** Every message the server can send. Switch on `type` to narrow `data`. */
export type ServerMessage =
  | { type: "welcome"; data: Welcome }
  | { type: "state"; data: State };

/** Every message the client can send. */
export type ClientMessage = { type: "input"; data: Input };

/**
 * The server message types decodeServerMessage accepts. Typed as a Record over
 * the union's tags, so adding a member to ServerMessage without listing it here
 * is a compile error rather than a silently dropped frame.
 */
const SERVER_TYPES: Record<ServerMessage["type"], true> = {
  welcome: true,
  state: true,
};

/**
 * Wraps a message for the socket. The type parameter keeps `type` and `data`
 * paired, so encodeClientMessage("input", someWelcome) does not compile.
 *
 * @param type the message's tag
 * @param data the payload that tag carries
 * @return     the JSON text to pass to WebSocket.send
 */
export function encodeClientMessage<T extends ClientMessage["type"]>(
  type: T,
  data: Extract<ClientMessage, { type: T }>["data"],
): string {
  return JSON.stringify({ type, data });
}

/**
 * Parses one frame from the server. Only the envelope is checked: the payload
 * is trusted to match its tag, since the server is ours and Encode refuses
 * types it does not know.
 *
 * @param raw the text of one WebSocket message
 * @return    the decoded message, or null if raw is not JSON or not a known type
 */
export function decodeServerMessage(raw: string): ServerMessage | null {
  let msg: unknown;
  try {
    msg = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof msg !== "object" || msg === null) {
    return null;
  }
  const type = (msg as { type?: unknown }).type;
  if (typeof type !== "string" || !Object.hasOwn(SERVER_TYPES, type)) {
    return null;
  }
  return msg as ServerMessage;
}

/**
 * The default branch of an exhaustive switch over a union's `type`. If a new
 * member is added and not handled, `msg` is no longer `never` at the call site
 * and the build fails there.
 *
 * @param msg the value that should be impossible
 * @return    never; throws if reached at runtime anyway
 */
export function assertNever(msg: never): never {
  throw new Error(`unhandled message: ${JSON.stringify(msg)}`);
}
