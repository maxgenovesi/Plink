# Plink — Phase Guide (the assignment sheet)

Companion to `ROADMAP.md`. That file says *what* and *why*; this one says *which files,
which types, which function signatures*.

**How to read this:** every `go` block below is a **spec, not code**. You get the type
definitions and the function signatures (that's Go layout convention you'd otherwise have
to guess at). Every function body is left to you and described as numbered steps in prose.
The blocks will not compile until you write those bodies — that's intentional.

Phases 1–4 are specified in full. Phases 5–7 are specified at file-and-responsibility
level, because the exact shapes will depend on decisions you make in 1–4. Phases 8–11 stay
in `ROADMAP.md` until you get there.

---

# Phase 1 — Server skeleton

## Files you will touch

```
cmd/server/main.go          rewrite  — wiring only, no logic
internal/game/vec.go        new      — 2D vector math, used by everything
internal/game/player.go     rewrite  — the simulation
internal/server/server.go   new      — HTTP routes
internal/server/client.go   new      — one connection's three goroutines
internal/server/messages.go new      — wire types (moves to protocol/ in Phase 4)
web/src/network.ts          new
web/src/game.ts             new
web/src/render.ts           new
```

## 1a. `internal/game/vec.go` and `internal/game/player.go`

Both are `package game`, so they share one namespace — `Vec` declared in `vec.go` is
visible in `player.go` with no import. Vec gets its own file because `Projectile` uses
the same math in Phase 3; it is not Player-specific.

```go
package game

// Vec is a 2D vector. Used for positions, velocities and directions.
// Methods are on the value receiver: a Vec is small and always copied.
type Vec struct {
	X, Y float64
}

// Add returns the component-wise sum v+o. It does not modify v.
func (v Vec) Add(o Vec) Vec

// Sub returns the component-wise difference v-o.
// Direction matters: a.Sub(b) points from b towards a.
func (v Vec) Sub(o Vec) Vec

// Scale returns v with both components multiplied by s.
// Scaling by a negative s reverses the direction.
func (v Vec) Scale(s float64) Vec

// Len returns the magnitude of v. It uses math.Sqrt, so prefer comparing
// squared lengths in hot paths like collision checks.
func (v Vec) Len() float64

// Normalized returns a unit vector in the same direction.
// It must return the zero Vec if v is the zero Vec (do not divide by zero).
func (v Vec) Normalized() Vec

// PlayerID identifies a player for the lifetime of one connection.
type PlayerID string

// Player is one controllable circle in the arena.
type Player struct {
	ID  PlayerID
	Pos Vec
	Vel Vec
}

// Input is the set of controls a client is holding during one tick.
// It is a snapshot of state, not an event: "W is currently down",
// not "W was just pressed".
type Input struct {
	Up, Down, Left, Right bool
}

// Tuning constants. Units are arena-units and seconds.
const (
	PlayerRadius   = 15.0
	PlayerAccel    = 3000.0 // units/s^2 while a key is held
	PlayerMaxSpeed = 400.0  // units/s
	PlayerFriction = 8.0    // velocity lost per second, proportional to speed
)

// Update advances the player by exactly one fixed timestep.
// It is pure: same Player + same Input + same dt => same result, every time.
func (p *Player) Update(in Input, dt float64)

// ClampTo keeps the player fully inside a w x h arena, accounting for
// PlayerRadius, and zeroes the velocity component that hit the wall.
func (p *Player) ClampTo(w, h float64)
```

**`Update` — write these five steps in order:**

1. Turn the four booleans into a direction `Vec` (right = +X, down = +Y to match canvas
   coordinates). Left+Right held together should cancel to zero.
2. `Normalized()` that direction, then `Scale(PlayerAccel)`. *This is the whole reason
   `Normalized` exists* — without it, holding W+D makes you move 1.41× faster diagonally,
   which is a classic bug.
3. Apply acceleration to velocity: `Vel += accel * dt`.
4. Apply friction: reduce `Vel` proportionally to itself — `Vel -= Vel * PlayerFriction * dt`.
   Then clamp the result's `Len()` to `PlayerMaxSpeed`.
5. Apply velocity to position: `Pos += Vel * dt`.

**Checkpoint — write `internal/game/player_test.go` before moving on:**
- A player with no input and nonzero velocity comes to a near-stop within ~1 second of ticks.
- Holding Right for 1 second of ticks moves you a positive X distance and leaves Y at 0.
- Holding Right+Down for 1 second moves you the **same total distance** as holding Right
  alone. If this fails, step 2 is wrong.
- `ClampTo` on a player pushed into a wall leaves `Pos.X >= PlayerRadius`.

Run `go test ./internal/game/ -v`. This is the whole payoff of `game` importing nothing:
you are testing your game with no server, no browser, no network.

## 1b. `internal/server/messages.go`

```go
package server

// clientInput is what the browser sends us, ~30x/second.
// Seq is unused in Phase 1 — you will need it in Phase 6 and adding
// the field now is free.
type clientInput struct {
	Seq   uint32 `json:"seq"`
	Up    bool   `json:"up"`
	Down  bool   `json:"down"`
	Left  bool   `json:"left"`
	Right bool   `json:"right"`
}

// serverState is what we send back every tick.
type serverState struct {
	Type   string      `json:"type"` // always "state" for now
	Tick   uint64      `json:"tick"`
	AckSeq uint32      `json:"ackSeq"` // last Seq we processed
	You    playerState `json:"you"`
}

type playerState struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}
```

Note the lowercase type names: these are unexported, package-private. `json` struct tags
control the wire names, so Go can use `X` while TypeScript sees `x`. Read up on why
exported fields are required for `encoding/json` to see them at all.

## 1c. `internal/server/client.go`

This is the file that matters. **Three goroutines per connection**, and this shape stays
with you through Phase 11.

```go
package server

const (
	TickRate     = 20
	TickDuration = time.Second / TickRate
	TickDelta    = 1.0 / float64(TickRate) // the fixed dt you pass to Update
)

// client owns one WebSocket connection and the goroutines that serve it.
type client struct {
	id   game.PlayerID
	conn *websocket.Conn

	// inputs carries decoded input from readLoop to simLoop.
	// Buffered: a slow tick must not block the reader.
	inputs chan clientInput

	// outgoing carries encoded frames from simLoop to writeLoop.
	// Buffered: a slow client must not stall the simulation.
	outgoing chan []byte
}

// newClient allocates a client and its two channels. Both channels must be
// buffered: an unbuffered inputs channel lets a fast sender block the reader,
// and an unbuffered outgoing channel lets a slow client stall the simulation.
func newClient(id game.PlayerID, conn *websocket.Conn) *client

// readLoop decodes frames off the socket until ctx is cancelled or the
// connection fails. It returns the error that ended it.
func (c *client) readLoop(ctx context.Context) error

// simLoop ticks the simulation at TickRate and is the ONLY place the player
// is allowed to move. Receiving an input records intent; it never simulates.
func (c *client) simLoop(ctx context.Context) error

// writeLoop drains c.outgoing to the socket. It is the ONLY goroutine
// permitted to call conn.Write — coder/websocket forbids concurrent writes.
func (c *client) writeLoop(ctx context.Context) error

// run starts all three loops and blocks until the first one fails,
// then cancels the other two.
func (c *client) run(ctx context.Context) error
```

**`readLoop`:** loop on `conn.Read(ctx)`; `json.Unmarshal` into a `clientInput`; do a
**non-blocking send** onto `c.inputs` (a `select` with a `default:` that drops the input).
Dropping stale input under load is correct — you only ever care about the newest one.

**`simLoop` — the heart of it:**

```
player := &game.Player{ID: c.id, Pos: <arena centre>}
var latest clientInput
ticker := time.NewTicker(TickDuration)
defer ticker.Stop()

loop forever, select on:
   ctx.Done()  -> return ctx.Err()
   in := <-c.inputs -> latest = in            // just record it, do NOT move
   <-ticker.C  -> 1. player.Update(latest -> game.Input, TickDelta)
                  2. player.ClampTo(arenaW, arenaH)
                  3. build a serverState, json.Marshal it
                  4. non-blocking send onto c.outgoing
```

Read that again: **the input case does not move the player.** It only stores the intent.
Only the ticker case advances the simulation. If you move on receipt, a client that sends
60×/second moves three times as fast as one sending 20×/second — and you will not notice
until Phase 5.

**`run`:** the clean way is `golang.org/x/sync/errgroup` — `g, ctx := errgroup.WithContext(ctx)`,
then `g.Go(...)` each loop, then `g.Wait()`. When any loop returns an error the derived
`ctx` is cancelled, so the others unblock and exit. Worth adding the dependency just to
learn the pattern; a hand-rolled `context.WithCancel` + `sync.WaitGroup` is the alternative.

## 1d. `internal/server/server.go`

```go
package server

// Server holds process-wide dependencies and routes.
type Server struct {
	mux     *http.ServeMux
	webDir  string
	nextID  atomic.Uint64
}

// New builds a Server with its routes registered.
func New(webDir string) *Server

// ServeHTTP makes *Server satisfy http.Handler, so main.go can pass it
// straight to http.ListenAndServe. Idiomatic Go: your app is a Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)

// handleWS upgrades the request, builds a client, and blocks in client.run
// until the connection dies.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request)
```

`handleWS` should: `websocket.Accept` → `defer conn.CloseNow()` → mint an ID from
`nextID.Add(1)` → `newClient(...)` → `c.run(r.Context())` → log the returned error.
Note `r.Context()` is already cancelled when the client disconnects — free lifecycle
management, use it rather than `context.Background()`.

## 1e. `cmd/server/main.go`

Should shrink to ~15 lines: read a `PORT` env var (default `8080`), `server.New("./web/dist")`,
`http.ListenAndServe`. **No game logic, no WebSocket code.** `main` wires; packages do.

## 1f. The client

`npm create vite@latest web -- --template vanilla-ts` (or set it up in place), then have Go
serve `web/dist`.

```ts
// network.ts
export interface PlayerState { id: string; x: number; y: number }
export interface StateMessage { type: "state"; tick: number; ackSeq: number; you: PlayerState }
export interface InputMessage { seq: number; up: boolean; down: boolean; left: boolean; right: boolean }

export class Connection {
  /** Opens the socket immediately. Reconnection is out of scope until Phase 5. */
  constructor(url: string);
  /** Registers the handler for every inbound state frame. */
  onState(cb: (msg: StateMessage) => void): void;
  /** Serialises and sends one input. Must silently no-op while the socket is
   *  not OPEN, since the first inputs race the handshake. */
  send(input: InputMessage): void;
  /** Closes the socket and stops delivering to the onState handler. */
  close(): void;
}
```

```ts
// game.ts
/** Tracks which keys are currently held. Listens on keydown/keyup. */
export class InputTracker {
  constructor(target: Window);
  /** The CURRENT held-key state, with a fresh incrementing seq. */
  snapshot(): InputMessage;
}

/** Wires Connection + InputTracker + Renderer together and starts both loops. */
export function start(canvas: HTMLCanvasElement): void;
```

```ts
// render.ts
export interface Renderer {
  /** Clears the canvas and draws one frame. Pure: it reads state and touches
   *  nothing else, so the same state always produces the same picture. */
  draw(state: StateMessage): void;
}

/** Captures the 2D context once and returns a Renderer bound to this canvas. */
export function createRenderer(canvas: HTMLCanvasElement): Renderer;
```

**Two loops, different rates, and they must stay separate:**
- `setInterval(..., 33)` → `conn.send(tracker.snapshot())` — ~30Hz input send
- `requestAnimationFrame` → `renderer.draw(lastState)` — ~60Hz draw

You receive state at 20Hz and draw at 60Hz. In Phase 1 the circle will visibly step. Let it.
Fixing that is Phase 6, and doing it now teaches you nothing.

## Phase 1 is done when

Two browser tabs each drive their own circle with WASD, diagonal movement is the same speed
as straight, and `Ctrl-C` on the server freezes both. `go test ./... -race` is clean.

---

# Phase 2 — Two players in one world

## New/changed files

```
internal/game/game.go       new      — the world
internal/server/hub.go      new      — one goroutine owning one Game
internal/server/client.go   change   — simLoop moves OUT of the client
```

The key restructure: in Phase 1 each connection simulated its own player. Now **one**
goroutine simulates everything, and clients become dumb pipes.

```go
package game

// Game is the whole world. It is NOT safe for concurrent use — by design.
// Exactly one goroutine may touch a Game. Everything else talks to that
// goroutine over channels. This is the "share memory by communicating" rule,
// and following it is why you will never need a mutex in this package.
type Game struct {
	Width, Height float64
	Tick          uint64

	players map[PlayerID]*Player
	inputs  map[PlayerID]Input // latest input per player, applied each tick
}

// NewGame returns an empty world of the given size, with Tick at 0.
func NewGame(width, height float64) *Game

// AddPlayer creates a player at a spawn point and returns it.
func (g *Game) AddPlayer(id PlayerID) *Player
// RemovePlayer deletes a player and any input recorded for it.
// Removing an unknown id must be a no-op, not a panic: a client can
// disconnect before it is fully joined.
func (g *Game) RemovePlayer(id PlayerID)

// SetInput records a player's latest intent. It does not simulate.
func (g *Game) SetInput(id PlayerID, in Input)

// Step advances the whole world by one fixed timestep.
func (g *Game) Step(dt float64)

// Snapshot returns a value copy of the world, safe to hand to other
// goroutines and to serialise. It must not share memory with the Game.
func (g *Game) Snapshot() Snapshot

// Snapshot is an immutable, self-contained view of one tick. It shares no
// memory with the Game it came from, so it is safe to hand to any goroutine.
type Snapshot struct {
	Tick    uint64
	Players []PlayerSnapshot
}

// PlayerSnapshot is one player's serialisable state. It holds values, never
// pointers, for the same reason.
type PlayerSnapshot struct {
	ID   PlayerID
	X, Y float64
}
```

**`Step` is:** increment `Tick`, then for each player look up its input, `Update`, `ClampTo`.
That's all. Everything you add in later phases hangs off this one method.

**Why `Snapshot` copies:** if it returned `[]*Player`, the writer goroutine would read a
pointer while the sim goroutine mutates it — a data race that `-race` will catch and that
will corrupt frames in production. Returning values makes the race structurally impossible.

## `internal/server/hub.go`

```go
package server

// hub owns exactly one game.Game and is the only goroutine allowed to touch it.
type hub struct {
	game *game.Game

	join   chan *client
	leave  chan *client
	inputs chan playerInput

	clients map[game.PlayerID]*client // hub goroutine only
}

// playerInput pairs a decoded input with the connection it arrived on,
// because the hub serves many clients and the frame itself carries no identity.
type playerInput struct {
	id    game.PlayerID
	input clientInput
}

// newHub builds a hub around a fresh Game. It does not start the loop;
// the caller runs Run in its own goroutine.
func newHub(width, height float64) *hub

// Run is the world's loop. It exits when ctx is cancelled.
func (h *hub) Run(ctx context.Context)

// broadcast marshals a snapshot once and fans the bytes out to every client.
func (h *hub) broadcast(snap game.Snapshot)
```

`Run` is a `select` over five cases: `ctx.Done()`, `join`, `leave`, `inputs`, `ticker.C`.
The `inputs` case calls `g.SetInput` and nothing else; the `ticker.C` case calls `g.Step`
then `h.broadcast`.

`broadcast` should `json.Marshal` **once** and send the same `[]byte` to all clients — not
marshal per client. With 8 players at 20Hz that's 160 marshals/sec vs 20. Note that this
makes the slice shared, so nobody may mutate it — document that.

`client` loses `simLoop` entirely. It keeps `readLoop` (now forwarding to `hub.inputs`) and
`writeLoop`. Deleting code in a refactor is a good sign.

## Phase 2 is done when

Two tabs see each other. Closing one removes it from the other within a tick.
`go test ./... -race` is still clean under two connected clients.

---

# Phase 3 — Shooting and death

## New/changed files

```
internal/game/projectile.go new
internal/game/event.go      new
internal/game/player.go     change  — Alive, Aim, cooldown
internal/game/game.go       change  — Step grows
```

```go
// player.go additions
type Player struct {
	ID    PlayerID
	Pos   Vec
	Vel   Vec
	Alive bool
	Aim   float64 // radians, 0 = +X, increasing clockwise (canvas convention)

	cooldown int // ticks remaining before this player may fire again
}

// Input additions
type Input struct {
	Up, Down, Left, Right bool
	Aim                   float64
	Shoot                 bool
}
```

```go
// projectile.go
type ProjectileID uint64

type Projectile struct {
	ID    ProjectileID
	Owner PlayerID
	Pos   Vec
	Vel   Vec
	TTL   int // ticks remaining; <= 0 means despawn
}

const (
	ProjectileRadius   = 4.0
	ProjectileSpeed    = 900.0 // units/s
	ProjectileTTLTicks = 40    // 2 seconds at 20Hz
	FireCooldownTicks  = 6
)
```

```go
// event.go — things that HAPPENED this tick, for the client to react to
type EventType string

const (
	EventKill       EventType = "kill"
	EventRoundStart EventType = "round_start"
	EventRoundOver  EventType = "round_over"
)

type Event struct {
	Type     EventType
	Killer   PlayerID
	Victim   PlayerID
	WinnerID PlayerID
}
```

```go
// game.go additions
type RoundState int

const (
	RoundWaiting RoundState = iota
	RoundActive
	RoundOver
)

// Step now returns the events produced during this tick.
func (g *Game) Step(dt float64) []Event

// unexported helpers, called in this order by Step:
// stepPlayers applies each live player's input and clamps it to the arena.
// Dead players are skipped entirely: their input is ignored, not zeroed.
func (g *Game) stepPlayers(dt float64)

// spawnProjectiles fires for every live player whose input requests it and
// whose cooldown has expired, then resets that player's cooldown. This is
// where the server refuses to honour a client that spams Shoot.
func (g *Game) spawnProjectiles()

// stepProjectiles advances every projectile, decrements TTL, and drops the
// expired ones. Use the filter-in-place idiom so it allocates nothing.
func (g *Game) stepProjectiles(dt float64)

// resolveHits tests every projectile against every live player except its
// owner, returning one EventKill per hit. Compare squared distances.
func (g *Game) resolveHits() []Event

// checkRoundEnd transitions RoundActive to RoundOver once at most one player
// is alive. It must be idempotent: returns no events if already RoundOver.
func (g *Game) checkRoundEnd() []Event
```

**Order matters and you should be able to justify it.** Move players, then spawn, then move
projectiles, then resolve collisions. Spawning *after* moving players means a bullet leaves
from where you are now, not where you were last tick.

**`resolveHits`:** for each projectile, for each player where `p.ID != proj.Owner` and
`p.Alive`, test `proj.Pos.Sub(p.Pos).Len() < ProjectileRadius+PlayerRadius`. Square both
sides instead and skip the `math.Sqrt` — write it the fast way from the start, it costs you
nothing and it's the sort of thing that gets noticed.

**Culling projectiles without allocating** — the filter-in-place idiom, worth learning now:

```go
kept := g.projectiles[:0]
for _, p := range g.projectiles {
	if p.TTL > 0 { kept = append(kept, p) }
}
g.projectiles = kept
```

**Death rules:** a dead player keeps receiving snapshots, has its input ignored, and isn't
drawn. Don't remove it from the map — it still needs to be in the scoreboard.

## Phase 3 is done when

You can kill the other tab, it becomes a spectator, the round ends, and everyone respawns
a few seconds later. Fire rate is capped **on the server** — verify by hacking your client
to send `shoot: true` every frame and confirming the rate doesn't change.

---

# Phase 4 — A real protocol

Now move the wire types out of `internal/server` into `internal/protocol` and make them
properly typed. The dependency direction is **server → protocol → game**, and never backwards.

```go
package protocol

type MessageType string

const (
	// server -> client
	TypeWelcome MessageType = "welcome"
	TypeState   MessageType = "state"
	TypeEvent   MessageType = "event"
	// client -> server
	TypeInput MessageType = "input"
)

// Envelope is the outer shape of every frame. Decode reads Type first,
// then unmarshals Data into the right concrete struct.
type Envelope struct {
	Type MessageType     `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Welcome is sent once, immediately after the socket opens. It tells the
// client who it is and the constants it needs to predict correctly in Phase 6.
type Welcome struct {
	PlayerID string  `json:"playerId"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	TickRate int     `json:"tickRate"`
}

// State is the full world, broadcast every tick. AckSeq is per-recipient,
// so this struct is NOT identical across clients.
type State struct {
	Tick        uint64            `json:"tick"`
	AckSeq      uint32            `json:"ackSeq"`
	Players     []PlayerState     `json:"players"`
	Projectiles []ProjectileState `json:"projectiles"`
}

// Input is one sample of the client's controls. Seq increases monotonically
// per connection and is never reused; the server echoes the highest one it
// has processed back in State.AckSeq.
type Input struct {
	Seq   uint32  `json:"seq"`
	Up    bool    `json:"up"`
	Down  bool    `json:"down"`
	Left  bool    `json:"left"`
	Right bool    `json:"right"`
	Aim   float64 `json:"aim"`
	Shoot bool    `json:"shoot"`
}

// Encode wraps a concrete message in an Envelope with the right Type.
// It should reject any type it doesn't recognise.
func Encode(msg any) ([]byte, error)

// Decode returns one of *Welcome, *State, *Event, *Input.
// Callers type-switch on the result.
func Decode(data []byte) (any, error)
```

**`json.RawMessage` is the trick here** — it defers unmarshalling, so you can read `Type`
in a first pass and only then decode `Data` into the right struct. Look up why it's just a
`[]byte` with custom marshal methods.

**The AckSeq contract, and why Phase 6 dies without it:** `State.AckSeq` is per-recipient —
"the last input sequence number *I* processed *from you*". So the state broadcast is no
longer byte-identical across clients. Either marshal per client (simple, slower) or marshal
the shared part once and patch in the ack (faster, fiddlier). Pick the simple one now; make
it a Phase 11 optimisation with a benchmark.

**TypeScript mirror:**

```ts
export type ServerMessage =
  | { type: "welcome"; data: Welcome }
  | { type: "state";   data: State }
  | { type: "event";   data: GameEvent };

// In your handler, switch on msg.type. Add a default branch that assigns
// msg to a `never` variable — TypeScript then fails to compile if you ever
// add a message type and forget to handle it here. Learn this pattern.
```

## Phase 4 is done when

Adding a message type means editing exactly two files, and both compilers point at every
site that needs updating.

---

# Phases 5–7 — file layout only

Specified at a lower resolution deliberately: by the time you arrive, your Phase 1–4
decisions will have changed the details, and a spec written now would be wrong.

## Phase 5 — Rooms

```
internal/server/room.go        hub renamed + given an ID, a ctx, and a lifecycle
internal/server/roomset.go     map[RoomID]*room + a sync.Mutex (the one honest mutex)
internal/matchmaking/queue.go  Add(player), and a matcher that pops N and asks for a room
internal/server/http.go        /health, /rooms
```

The one hard part is **shutdown**: when the last client leaves, the room's ctx must be
cancelled, `Run` must return, and the goroutine must actually exit. Verify with
`net/http/pprof` — hit `/debug/pprof/goroutine?debug=1`, open and close 20 rooms, hit it
again, and confirm the count returns to baseline. A leak here is invisible until deploy.

## Phase 6 — Prediction

```
web/src/predict.ts     input buffer + reconciliation
web/src/interpolate.ts snapshot buffer + render-time interpolation for other players
web/src/sim.ts         a line-for-line port of game.Player.Update
internal/server/lag.go dev-only artificial latency, env-gated
```

Build `lag.go` **first**. Everything in this phase is invisible at 0ms ping.
Read Gambetta's four articles end to end before you write any of it.

## Phase 7 — Persistence

```
docker-compose.yml           change  — postgres service + volume
db/migrations/0001_init.sql  new     — users, matches, match_participants
db/queries/*.sql             new     — annotated with -- name: X :one
sqlc.yaml                    new
internal/db/                 generated by sqlc — do not hand-edit
internal/auth/jwt.go         new     — sign + verify, HS256
internal/auth/password.go    new     — bcrypt hash + compare
```

`internal/db` becomes generated output. Commit it, but never edit it — regenerate.

---

# Go habits to build while you do this

Run these constantly, not at the end:

```
go build ./...
go vet ./...
go test ./... -race
gofmt -l .          # prints files that aren't formatted; should print nothing
```

Idioms this project will teach you, in the order you'll meet them:

- **Errors are values.** Return `error` as the last value; wrap with `fmt.Errorf("...: %w", err)`
  so callers can still `errors.Is` through it. Never `panic` in a server.
- **Accept interfaces, return structs.** `New(...)` returns `*Server`, not an interface.
- **The zero value should be useful.** `var in Input` is a valid "no keys held".
- **Document exported identifiers**, starting with the name: `// Update advances...`.
- **Unexported by default.** Only capitalise what another package genuinely needs.
- **`defer` right after acquiring**, so cleanup can't be skipped by an early return.
- **Channel direction in signatures** (`chan<- []byte`, `<-chan Input`) documents intent
  and lets the compiler enforce it.
