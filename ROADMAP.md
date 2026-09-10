# Plink — Development Roadmap

> Working on a phase? `PHASE-GUIDE.md` has the file-by-file specs and function signatures for it.

A phase-by-phase build order. Each phase has a **Goal**, a **Done when** (the only
thing that decides if you move on), **hints** for what to research, and **traps**
that cost people days.

Rules that keep this project finishable:

1. One phase at a time. A phase isn't done until its *Done when* is literally true.
2. No feature from a later phase, even if it feels easy right now.
3. No Redis until Phase 11. No ORM ever (sqlc only). No OAuth (plain JWT).
4. When stuck for more than a day, go read a reference implementation instead of grinding.

---

## Where you are right now

- `cmd/server/main.go` — serves `web/` and accepts a WebSocket at `/ws`, then logs
  whatever it reads. No game loop, no writes back to the client.
- `internal/game/player.go` — `Player`, `Input`, and an empty `Update`.
- `web/src/{game,network,render}.ts` — empty files.

So: **you are inside Phase 1.** Everything below Phase 1 is future.

---

## Phase 1 — Server skeleton, one player, one circle

**Goal:** you press W in the browser and a circle moves, but the *server* is the one
deciding where the circle is.

**Build**
- A per-connection goroutine layout. This is the shape you keep for the rest of the project:
  - **read goroutine**: `conn.Read` in a loop, decodes JSON input, pushes onto a channel
  - **game loop goroutine**: `time.NewTicker(50 * time.Millisecond)`, applies the latest
    input to the player, advances physics, produces a state snapshot
  - **write goroutine**: takes snapshots off a channel and `conn.Write`s them
- Client: `network.ts` opens the socket and exposes `send(input)` + `onState(cb)`.
  `game.ts` tracks which keys are down. `render.ts` clears the canvas and draws circles.
- Client sends its *current key state* (`{up:true,down:false,...}`), roughly 30×/sec —
  not "keydown" events. This matters a lot in Phase 6.

**Done when:** two browser tabs each show their own circle moving smoothly under WASD,
and killing the server stops the movement (proving the server owns the position).

**Research**
- `go doc github.com/coder/websocket` — `Accept`, `Read`, `Write`, `CloseNow`, and
  why you need `context` on every call.
- Goroutines + channels + `select`. The game loop is a `select` over the ticker channel,
  the input channel, and a "connection closed" channel.
- `encoding/json` struct tags: `X float64 \`json:"x"\``.
- MDN Canvas 2D: `requestAnimationFrame`, `ctx.arc`, `ctx.clearRect`.
- Vite: `npm create vite@latest` with the vanilla-ts template, `npm run build`, and
  point Go's `http.FileServer` at the build output.

**Traps**
- ❌ Applying movement inside the read handler. Movement belongs in the tick loop *only*.
  If you move the player on message receipt, a client that spams messages moves faster.
- ❌ Calling `conn.Write` from two goroutines. `coder/websocket` doesn't allow concurrent
  writes — funnel everything through the one write goroutine.
- ❌ Reconnection logic. Not now. A dropped socket just means the player disappears.
- ❌ Fixing your render rate to the tick rate. Render at 60fps, receive state at 20Hz.

---

## Phase 2 — Two players, real physics

**Goal:** multiple connections share one world.

**Build**
- `internal/game/game.go`: a `Game` struct owning `players map[PlayerID]*Player`,
  with `AddPlayer`, `RemovePlayer`, `ApplyInput`, `Step(dt float64)`, `Snapshot()`.
- Velocity + acceleration + friction instead of teleporting by a fixed delta.
  Something like: input → acceleration → `v += a*dt`, `v *= friction`, `p += v*dt`.
- Clamp players to the arena bounds (1500×1500 from `DESIGN.md`).
- Broadcast the *full* state to everyone each tick. Delta compression is Phase 11.

**Done when:** two tabs see each other move, and closing one tab removes that circle
from the other within a tick or two.

**Research**
- Who owns the map? Either a `sync.Mutex` on `Game`, or — better and more idiomatic —
  the map is only ever touched by the game-loop goroutine and everything else talks to
  it through channels. Pick one and be consistent. Read "Share memory by communicating".
- `go test ./internal/game` — this is the payoff of keeping `game` pure. Write a test
  that steps 100 ticks with a fixed input and asserts a position. No network involved.
- `go vet` and `go test -race`. Run the race detector early; it will catch the mutex
  mistakes you don't know you made.

**Traps**
- ❌ Variable `dt` from wall-clock time. Use a fixed `dt` (0.05s) matching your ticker.
  Fixed timestep = deterministic = replayable in Phase 9.
- ❌ Removing a player from the map while iterating it in another goroutine.

---

## Phase 3 — Shooting and death

**Goal:** the game is actually a game.

**Build**
- Client sends aim angle (mouse position relative to your player) and a `shoot` bool.
- `internal/game/projectile.go`: position, velocity, owner ID, TTL in ticks.
- Each tick: advance projectiles, decrement TTL, drop expired ones, then check
  circle-circle collision against every player that isn't the owner.
- Death → player becomes a spectator (still receives state, sends no input).
- Round ends when ≤1 player is alive; after a few seconds, respawn everyone.

**Done when:** you can kill the other tab, it goes spectator, and the round resets.

**Research**
- Circle-circle collision: compare squared distance to `(r1+r2)²`. Never use `math.Sqrt`
  in a collision check.
- Fire-rate limiting on the server (a cooldown in ticks). The client asking to shoot is a
  *request*; the server decides.
- Removing elements from a slice in place without allocating: the `s[:0]` filter idiom.

**Traps**
- ❌ Trusting the client's claimed position or claimed hit. The server simulates; the
  client only ever sends intent.
- ❌ Fast projectiles tunneling through players between ticks. Note it, live with it for
  now, fix with swept collision later if it's actually noticeable.

---

## Phase 4 — A real protocol

**Goal:** stop hand-rolling ad-hoc JSON shapes.

**Build**
- `internal/protocol/`: typed messages with a `type` discriminator field.
  - server→client: `welcome` (your player ID, arena size, tick rate), `state`, `event`
    (kill, round start, round end)
  - client→server: `input` (keys, aim, shoot, **sequence number**, client tick)
- TypeScript discriminated unions mirroring them, so `switch (msg.type)` narrows.
- Every input carries a monotonically increasing `seq`; every state snapshot echoes back
  the last `seq` the server processed for *that* player.

**Done when:** adding a new message type requires touching exactly two files (one Go, one TS),
and the compiler tells you everywhere that needs updating.

**Research**
- Custom `UnmarshalJSON` or a two-pass decode (`json.RawMessage`) to dispatch on `type`.
- TS discriminated unions + exhaustive `switch` with a `never` default case.

**Traps**
- ❌ Skipping the sequence number because nothing uses it yet. It exists *for Phase 6* and
  retrofitting it is annoying.
- ❌ Letting `internal/game` import `internal/protocol`. Dependencies point one way:
  server → protocol → game. `game` imports nothing of yours.

---

## Phase 5 — Rooms and matchmaking

**Goal:** more than one match at a time.

**Build**
- `Room`: an ID, its own `Game`, its own ticker, its own goroutine, its own set of clients.
  A room is a small server. Connections join a room; the room's loop is the only thing
  that touches its `Game`.
- Lifecycle: create on demand, shut down when empty (the goroutine must actually exit —
  use `context.Context` cancellation).
- HTTP: `/health` returning 200, `/rooms` returning a JSON list (id, player count, state).
- Then an in-memory matchmaker: a queue of waiting players; when it has enough
  (start with 2, target 4–8), create a room and move them in.

**Done when:** four tabs auto-pair into two independent 2-player matches, and `/rooms`
shows both. Killing all clients in one room leaves zero leaked goroutines.

**Research**
- `context.WithCancel` and passing `ctx` into the room loop; `defer` for cleanup.
- Goroutine leak detection: hit `/debug/pprof/goroutine?debug=1` (via `net/http/pprof`)
  before and after a room empties.
- A `Hub` type owning `map[RoomID]*Room` guarded by a mutex — the one place a mutex is
  clearly right.

**Traps**
- ❌ Redis. In-memory is correct here.
- ❌ A single global game loop ticking all rooms. Per-room goroutines are simpler and
  are the thing that scales.

---

## Phase 6 — Client prediction + server reconciliation ⭐

**This is the centerpiece.** It's what makes the game feel responsive, and it's the part
worth talking about in an interview. Budget more time for it than for Phases 1–5 combined.

**Goal:** zero perceived input latency for your own player, smooth motion for everyone else.

**Build — three separate mechanisms, build them in this order:**

1. **Prediction (your own player).** Apply your input locally the instant you press the key,
   using the *same* physics code as the server. Keep every unacknowledged input in a buffer.
2. **Reconciliation.** When a snapshot arrives, it says "I processed your input up to seq N,
   and you were at (x,y)". Snap your player to that authoritative position, then **re-apply**
   every buffered input after N. If your physics matches the server's, nothing visibly moves.
3. **Entity interpolation (everyone else).** Buffer the last ~2–3 snapshots and render other
   players ~100ms *in the past*, interpolating between the two snapshots that bracket that
   time. Never predict other players.
4. Projectiles: server-authoritative. Optionally spawn a cosmetic local one for feel.

**Done when:** with 150ms of artificial latency, your own movement still feels instant and
other players move smoothly rather than teleporting 20×/sec.

**Research** (read these *before* writing code)
- Gabriel Gambetta, "Fast-Paced Multiplayer" (4 parts, with live demos):
  https://www.gabrielgambetta.com/client-server-game-architecture.html
- Valve, "Source Multiplayer Networking" (on the Valve developer wiki).
- Glenn Fiedler / gafferongames: "Snapshot Interpolation" and "Fix Your Timestep".
- Build a latency simulator first: a dev-mode wrapper that delays sends by N ms and drops
  X% of packets. You cannot debug this on localhost with 0ms ping.

**Traps**
- ❌ Duplicating physics in TS by hand, then letting it drift from Go. Keep `Update` tiny
  and port it *exactly*; any divergence shows up as rubber-banding. (If it gets painful,
  compiling the Go sim to WASM is a legitimate Phase-11 answer — don't start there.)
- ❌ Interpolating and predicting the same entity.
- ❌ Snapping instead of reconciling. If you just teleport to the server position each
  snapshot, you've built lag, not prediction.

---

## Phase 7 — Persistence

**Goal:** accounts and match history survive a restart.

**Build**
- `docker-compose.yml`: Postgres 16, a volume, port 5432.
- Schema + migrations: `users`, `matches`, `match_participants`.
- `pgx/v5` for the driver, `sqlc` to generate typed Go from your `.sql` files.
- Register/login HTTP endpoints; passwords via `golang.org/x/crypto/bcrypt`.
- JWT (HS256, `golang-jwt/jwt`), passed on the WebSocket as a query param (browsers can't
  set headers on a WS handshake — this is why, and it's worth knowing).
- Write a `matches` row when a round ends.

**Done when:** you register, log out, log back in, and see your past matches.

**Research**
- `sqlc generate` workflow: write SQL with `-- name: GetUser :one` comments, get Go back.
- `context` deadlines on every query.
- Why bcrypt and not SHA-256.

**Traps**
- ❌ An ORM. ❌ OAuth. ❌ Storing the JWT secret in the repo — use an env var from day one.
- ❌ Blocking the game loop on a DB write. Hand it to a separate goroutine/channel.

---

## Phase 8 — ELO and ranked matchmaking

**Build**
- ELO for a free-for-all: treat the match as every pairwise result (you beat everyone
  who died before you, lost to everyone who outlived you) and sum the deltas.
- Matchmaking queue sorted by rating; a search window that widens over time
  (±50 → ±100 → ±200 the longer someone waits).

**Done when:** ratings move sensibly after matches and a lone queued player eventually gets
matched with someone far from their rating rather than waiting forever.

**Research:** the ELO expected-score formula, K-factor choice, and why K should be higher
for new accounts (placement matches).

---

## Phase 9 — Replays and observability

**Build**
- Record each match: the initial state + every input with its tick. Because the sim is a
  fixed-timestep deterministic function, inputs alone reconstruct the whole match.
- `GET /replay/{matchID}` streams it; the client renders it with the normal renderer.
- `prometheus/client_golang` on `/metrics`: tick duration histogram, active rooms/players,
  messages per second, WS errors.
- Replace `log` with `log/slog` (JSON handler) and attach a request/room ID to every line.

**Done when:** you can replay a finished match in the browser, and a Grafana/`curl` view of
`/metrics` shows tick time under load.

**Traps**
- ❌ Recording full state per tick when inputs suffice. Do that only if determinism turns
  out to be broken — and if it is, that's a bug worth finding.

---

## Phase 10 — Ship it

**Build**
- Multi-stage `Dockerfile`: build the TS with Node, build a static Go binary, and embed the
  frontend with `//go:embed` so the artifact is one file.
- Deploy to Fly.io + the managed Postgres; secrets via `fly secrets`.
- Load test with `k6` or `vegeta`: how many concurrent players before tick time exceeds 50ms?
  Put the number in the README.
- README: a play-now link, a GIF, and an architecture section that actually explains Phase 6.

**Done when:** someone can play it from a link you paste, without you present.

---

## Phase 11 — Optional polish (pick 2–3, not all)

- Binary protocol to replace JSON on the wire, with a measured before/after size.
- Server-side anti-cheat: speed/fire-rate sanity checks, per-connection rate limiting.
- Redis-backed matchmaking across multiple server instances.
- Spectator mode / killcam.
- Multi-region deploy with region-aware matchmaking.
- `testcontainers-go` for real integration tests against Postgres in CI.
- Juice: screen shake, hit flashes, muzzle flashes, sound. Cheap, huge perceived-quality win.

---

## Go concepts, in the order this project makes you learn them

| Phase | What you'll be forced to learn |
|---|---|
| 1 | goroutines, channels, `select`, `time.Ticker`, `context`, struct tags, `encoding/json` |
| 2 | maps, pointer receivers, `sync.Mutex` vs channel ownership, `go test`, `-race` |
| 3 | slices and the filter-in-place idiom, methods on value vs pointer types |
| 4 | interfaces, custom JSON marshalling, package dependency direction |
| 5 | context cancellation, `defer`, goroutine lifecycle, `net/http/pprof` |
| 6 | (mostly a distributed-systems problem, not a Go one) |
| 7 | `database/sql`/pgx, code generation, error wrapping with `%w`, env config |
| 9 | `log/slog`, Prometheus instrumentation, benchmarks (`go test -bench`) |
| 10 | `//go:embed`, static builds, multi-stage Docker |

## Reading list

- Effective Go — https://go.dev/doc/effective_go
- Go by Example — https://gobyexample.com
- Rob Pike, "Go Concurrency Patterns" (talk) — the mental model for the tick loop
- Gambetta, Fast-Paced Multiplayer — https://www.gabrielgambetta.com/client-server-game-architecture.html
- gafferongames — https://gafferongames.com
- MDN Canvas API — https://developer.mozilla.org/en-US/docs/Web/API/Canvas_API
- sqlc — https://sqlc.dev
