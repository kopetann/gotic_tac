# GoTicTacToe

A multiplayer Tic Tac Toe server and terminal client written in Go. Two clients
connect, are paired automatically by a matchmaking queue, and play a game over a
live connection. Players register and log in, every action carries a token,
results are persisted, and a leaderboard tracks wins, losses and draws.

The interesting part of this project is not the game - it is nine cells and
eight winning lines. It is the boundary between the rules, the application
logic, and the machinery around them.

---

## Quick start

### With Docker

```shell
docker compose up --build
```

The server listens on `:8080`. The database lives on a named volume, so results
survive `docker compose down` and come back on the next `up`.

To read the database directly (the image ships the `sqlite3` CLI for this):

```shell
docker compose exec server sqlite3 /data/gotic.db "SELECT name, wins, losses, draws FROM players;"
docker compose exec server sqlite3 /data/gotic.db "SELECT id, status, moves FROM matches;"
```

### Locally

```shell
export JWT_SECRET="$(openssl rand -base64 32)"
go run ./cmd/server
```

Then, in two more terminals:

```shell
go run ./cmd/client -register -name alice -password password123
go run ./cmd/client -register -name bob   -password password123
```

Type `join` in both. The two clients are paired automatically and the game
starts. `move 4` plays the centre; the board prints cell numbers in the empty
squares so there is nothing to memorise.

### Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `ADDR` | `:8080` | Address the server listens on |
| `DB_PATH` | `gotic.db` | SQLite file. `:memory:` runs without persistence |
| `JWT_SECRET` | — | Signing key. The server refuses to start without one, or with one under 32 bytes |
| `TOKEN_TTL` | `24h` | Token lifetime, as a Go duration (`30m`, `24h`) |

---

## Protocol

The API is split by **who initiates**, not by resource:

- **HTTP** carries everything a client asks for and waits on. Request/response
  fits it, and HTTP status codes are available to say what went wrong.
- **WebSocket** carries everything the *server* initiates: your opponent moved,
  a match was found, the game ended. There is no way to express that over
  request/response without polling.

A client can play a complete game over HTTP alone by polling `GET /match`. The
socket is what makes it pleasant, not what makes it work.

### HTTP endpoints

| Method | Path | Auth | Body | Success |
| --- | --- | --- | --- | --- |
| `GET` | `/healthz` | — | — | `200 ok` (`503` if SQLite is unreachable) |
| `POST` | `/register` | — | `{"name","password"}` | `201 {"token"}` |
| `POST` | `/login` | — | `{"name","password"}` | `200 {"token"}` |
| `GET` | `/leaderboard?limit=10` | — | — | `200 {"standings":[…]}` |
| `POST` | `/matchmaking/join` | ✔ | — | `202 {"waiting":1}` |
| `POST` | `/matchmaking/leave` | ✔ | — | `200 {"waiting":0}` |
| `GET` | `/match` | ✔ | — | `200 <state>` |
| `POST` | `/match/move` | ✔ | `{"cell":0-8}` | `200 <state>` |
| `POST` | `/match/resign` | ✔ | — | `204` |
| `GET` | `/ws` | ✔ | — | `101 Switching Protocols` |

Authenticated requests carry `Authorization: Bearer <token>`. The header is used
rather than a query parameter because query strings end up in access logs,
proxy logs and browser history.

`limit` is clamped to 1–100 rather than rejected: a nonsensical value from a
client is not worth failing a read-only request over.

### The state object

Returned by `/match` and `/match/move`, and nested under `state` in every
server event:

```json
{
  "match_id": "7d9dc777-9a7c-466a-bb3b-cddd44012e62",
  "cells":    ["X", "", "O", "", "X", "", "", "", ""],
  "your_mark": "X",
  "turn":      "O",
  "status":    "in_progress",
  "outcome":   "undecided",
  "opponent":  "bob",
  "finished":  false
}
```

`status` is one of `in_progress`, `x_won`, `o_won`, `draw`. `outcome` is
`win`, `loss`, `draw` or `undecided`, and is **relative to the recipient** —
the same match produces a different object for each player. That is why
notifications are addressed to one player rather than broadcast.

Note what is *not* here: the opponent's player ID. A client is told a display
name and nothing more.

### WebSocket

Connect to `ws://host/ws` with the bearer header. The token is verified
**before** the upgrade, so a bad one is a plain `401` rather than an
immediately-closed socket.

Client to server:

```json
{"type": "join"}            {"type": "leave"}
{"type": "move", "cell": 4} {"type": "state"}    {"type": "resign"}
```

Server to client:

| `type` | When |
| --- | --- |
| `match_started` | An opponent was found. Carries `state`. |
| `state_changed` | Either player moved. Carries `state`. |
| `match_ended` | Somebody won or the board filled. Carries the final `state`. |
| `opponent_left` | The other player disconnected or resigned. Carries `state`. |
| `ack` | A command succeeded. `{"type":"ack","of":"join"}` |
| `error` | A command failed. `{"type":"error","code":…,"message":…}` |

**No client message contains a player ID.** Identity is established once, at
the handshake, and read from the connection thereafter. A `move` frame says
*what* to do, never *who* is doing it — otherwise any client could play as
anyone.

The server pings every 54 seconds and drops a connection that has not answered
within 60. Without that, a client that vanishes without closing (a closed
laptop, a pulled cable) would occupy a slot indefinitely.

### Errors

Every failure, on either transport, carries a stable machine-readable `code`
alongside a human-readable `message`:

```json
{"code": "NOT_YOUR_TURN", "message": "it is not your turn"}
```

| Code | HTTP | Meaning |
| --- | --- | --- |
| `EMPTY_NAME`, `NAME_TOO_LONG`, `PASSWORD_TOO_SHORT` | 400 | Registration input rejected |
| `CELL_OUT_OF_BOUNDS`, `MALFORMED_BODY`, `UNKNOWN_COMMAND` | 400 | Request rejected |
| `INVALID_CREDENTIALS`, `INVALID_TOKEN`, `MISSING_TOKEN` | 401 | Not authenticated |
| `PLAYER_NOT_FOUND`, `NO_ACTIVE_MATCH` | 404 | Nothing there |
| `NAME_TAKEN`, `ALREADY_QUEUED`, `ALREADY_IN_MATCH` | 409 | Conflicts with current state |
| `CELL_OCCUPIED`, `NOT_YOUR_TURN`, `MATCH_FINISHED` | 409 | Conflicts with the board |
| `INTERNAL` | 500 | A bug or a broken dependency |

The codes are defined once, in `internal/domain/errors.go` and
`internal/usecase/errors.go`, and the transport only maps them to statuses
(`internal/adapter/transport/errors.go`). The mapping is deliberately one-way:
**anything that is not a recognised error becomes a bare `INTERNAL`**, so a raw
SQLite or JWT message can never reach a client.

### A worked session

```shell
# Two players register and keep their tokens.
ALICE=$(curl -s -XPOST localhost:8080/register \
  -d '{"name":"alice","password":"password123"}' | jq -r .token)
BOB=$(curl -s -XPOST localhost:8080/register \
  -d '{"name":"bob","password":"password123"}' | jq -r .token)

# Alice queues, then Bob — the two longest waiters are paired, and whoever
# waited longer takes X.
curl -s -XPOST localhost:8080/matchmaking/join -H "Authorization: Bearer $ALICE"
# {"waiting":1}
curl -s -XPOST localhost:8080/matchmaking/join -H "Authorization: Bearer $BOB"
# {"waiting":0}

# Bob is O, so moving first is refused.
curl -s -XPOST localhost:8080/match/move -H "Authorization: Bearer $BOB" -d '{"cell":0}'
# {"code":"NOT_YOUR_TURN","message":"it is not your turn"}   409

# Alice takes the top row while Bob answers underneath.
for m in "$ALICE 0" "$BOB 3" "$ALICE 1" "$BOB 4" "$ALICE 2"; do
  set -- $m
  curl -s -XPOST localhost:8080/match/move -H "Authorization: Bearer $1" -d "{\"cell\":$2}"
done
# …{"status":"x_won","outcome":"win","finished":true}

curl -s localhost:8080/leaderboard
# {"standings":[{"rank":1,"name":"alice","wins":1,…},{"rank":2,"name":"bob",…}]}
```

`internal/adapter/transport/http_test.go` runs exactly this sequence against
the real router.

---

## Authentication

**JWT, signed with HMAC-SHA256.**

### Why a signed token rather than a session

The alternative is an opaque random token with a server-side session table.
That would need a row read on every request and a row write on every login,
against a database that already serialises writes (see
[Persistence](#persistence)). A signed token is verified with one HMAC — no
I/O at all — which matters here because **every** gameplay action is
authenticated, including moves during a live match.

### Why HMAC rather than RS256/ES256

Symmetric signing is right when the signer and the verifier are the same party.
One process issues these tokens and the same process verifies them, so a public
key would buy nothing and cost a keypair to generate, store and mount.

Asymmetric signing earns its keep when a central auth service issues tokens
that *other* services verify, and you want those services to verify without
being able to mint. That is not this system.

The secret must be at least 32 bytes — HS256 is HMAC-SHA256, and a key shorter
than the hash output weakens it. The server refuses to start otherwise
(`cmd/server/config.go`).

### Algorithm confusion

`Verify` pins the accepted algorithm:

```go
jwt.ParseWithClaims(token, &claims, keyFunc, jwt.WithValidMethods([]string{"HS256"}))
```

Without that pin, a forged token declaring `"alg":"none"` — or, in mixed
deployments, one signed with HMAC using a public key as the secret — can pass
verification. `internal/adapter/auth/jwt_test.go` asserts that `alg: none`,
HS512, a wrong secret, an expired token and a tampered payload are all
rejected.

The claims are the registered ones only: `sub` (the player ID), `iat` and
`exp`. Nothing else is needed, and a JWT payload is **base64, not
encryption** — anyone holding a token can read its claims, so nothing secret
may go in one.

### Lifetime

Tokens live 24 hours by default (`TOKEN_TTL`).

Matches last minutes, so the token's real job is to survive a play session
*including a reconnect after a dropped socket*. A short TTL without a refresh
flow would lock a player out mid-match; adding refresh would require
server-side token storage, giving up the statelessness that motivated JWT in
the first place.

The accepted cost is **no revocation** — a stolen token stays valid until it
expires. Fixing that properly means either short TTLs with refresh-token
rotation, or a server-side denylist, and neither is worth it for a game with
no ban feature.

### Which operations need a token

All gameplay: queueing, moving, reading your match, resigning, and the
WebSocket upgrade. Only `/register`, `/login`, `/healthz` and `/leaderboard`
are open — the leaderboard is public information and reads nothing
player-specific.

### Passwords

**bcrypt** at the default cost (`internal/adapter/auth/hash.go`).

Passwords are hashed rather than encrypted because the plaintext is never
needed — only the answer to "is this the same password?" Encryption would mean
that whoever holds the key holds every password.

Not every hash suits passwords, though. SHA-256 is a fine checksum and a bad
password hash, because it is *fast*: billions of guesses per second on a GPU.
bcrypt is deliberately slow and tunable, and embeds a per-password salt in its
output — which is why `players.password_hash` is a single column with no
separate salt field, and why verification must go through
`bcrypt.CompareHashAndPassword` rather than hashing and comparing strings.

argon2id is the stronger modern choice, being memory-hard as well as slow. It
was not used here because it hands you raw bytes and three tunables to encode
and defend, where bcrypt's self-describing output fits the schema as is.

### Account enumeration

`Login` returns an identical `INVALID_CREDENTIALS` for an unknown name and for
a wrong password. If the two differed, the endpoint would confirm which
accounts exist. `TestLoginDoesNotLeakExistingNames` asserts the code *and* the
message match on both paths.

Registration cannot hide the same fact — `NAME_TAKEN` is unavoidable when names
must be unique — but that is a narrower leak than a login oracle, and it is the
same trade every service with public usernames makes.

---

## Architecture

The layout is Clean Architecture with the ceremony removed. Layers point inward
and nothing points out:

```
cmd/server, cmd/client        composition root — the only place concrete types are named
        │
        ▼
internal/adapter/…            sqlite, memory, websocket, hashing, tokens
        │                     each implements ports declared one layer inward
        ▼
internal/usecase              Auth, Matchmaking, Games, Leaderboard
        │                     and the ports they depend on
        ▼
internal/domain               Board, Match, Player — the rules, and nothing else
```

### The dependency rule is checkable, not claimed

```shell
$ go list -deps ./internal/domain | grep gotic
github.com/kopetann/gotic_tac/internal/domain

$ go list -deps ./internal/usecase | grep gotic
github.com/kopetann/gotic_tac/internal/domain
github.com/kopetann/gotic_tac/internal/usecase
```

`internal/domain` imports nothing from this module and nothing outside the
standard library. `internal/usecase` reaches inward to `domain` and no further.
No adapter appears in either list. That output *is* the Dependency Inversion
Principle — the same rule Clean Architecture states as "source code dependencies
point only inwards".

### SOLID, concretely

Clean Architecture is SOLID applied at module scale rather than a separate idea
competing with it. Each principle shows up in a specific place:

| Principle | Where it lives |
| --- | --- |
| **SRP** | `domain.Match` enforces game rules and does nothing else — it has no mutex, no persistence and no rendering. Serialising concurrent access belongs to `usecase.Games`; hashing belongs to an adapter. A mutex inside the entity would give it a second reason to change. |
| **OCP** | Adding a second transport, or replacing SQLite with Postgres, changes no file under `internal/domain` or `internal/usecase`. New behaviour arrives as a new adapter satisfying an existing port. |
| **LSP** | `internal/adapter/storetest` holds one behavioural contract that both `memory.PlayerStore` and `sqlite.PlayerStore` run. Passing it is what makes them interchangeable. It caught two real divergences while being written. |
| **ISP** | One store satisfies three narrow ports — `PlayerRegistry` (2 methods), `PlayerRecorder` (2), `StandingsReader` (1) — split by *consumer*, not by implementing type. `Auth` only ever sees the two methods it uses, so a leaderboard change cannot ripple into it. |
| **DIP** | Every interface in `internal/usecase/ports.go` is declared by the layer that *uses* it and implemented by a layer further out. Verified by the `go list` output above. |

### Decisions worth explaining

**Identifiers are plain strings, not `uuid.UUID`.** `domain.PlayerID` and
`domain.MatchID` are named string types. Generating an identifier requires a
source of randomness, which is ambient nondeterminism of the same kind as a
clock — an infrastructure concern. It sits behind the `IDGenerator` port, so the
domain stays free of third-party imports and tests can inject predictable
identifiers and assert on them exactly.

**`domain.Match` is not safe for concurrent use, deliberately.** Concurrency is
handled one layer out by `usecase.Games`, which owns every match in progress.
One mutex guards both its maps and the matches they point at: a move touches
nine cells, so the critical section is far too short to justify per-match locks
and the lock-ordering rules that would come with them.

The invariant that makes it correct:

> State is mutated and snapshotted **while the lock is held**. Events are
> delivered **after it is released**.

Calling a notifier under the lock would let one player's slow or dead connection
block every other match on the server. `TestConcurrentMovesAreSerialised` fires
eighteen concurrent moves at a single match and replays the persisted history
through `domain.RestoreMatch` — an illegal position fails the test.

**Errors carry a stable code.** `domain.Error{Code, Message}` means the domain
says `CELL_OCCUPIED`, never `400 Bad Request`. The transport maps codes to the
wire format; a CLI could map them to text. Application errors in
`internal/usecase/errors.go` reuse the same type, so the transport has exactly
one mapping rule.

**Persistence failures never fail a move.** The authoritative match lives in
memory. A database problem is logged through the injected `*slog.Logger` and the
game continues — it costs the audit trail for that match, not the match itself.

**`Board` is an array, not a slice.** `Match.Board()` therefore returns a copy,
and no outer layer can mutate a live match through it. `Match.Moves()` copies
explicitly for the same reason, and a test fails if that copy is removed.

### Deliberately left out

Applying every Clean Architecture ring to a nine-cell game would be
over-engineering. What was skipped, and when it would be worth adding:

| Omitted | Reasoning | Add it when |
| --- | --- | --- |
| Presenters / output ports | With a single transport they add indirection and no flexibility. Use cases return Go structs; the adapter marshals them. | A second output format needs different formatting rules |
| Per-boundary DTOs and mappers | No `Entity → Model → DTO` chain. Domain types cross the use case boundary directly. Only the wire format gets its own types, because `Player.PasswordHash` must never be serialised and protocol stability is a separate concern from domain evolution. | The stored or transmitted shape diverges from the domain shape |
| An input-port interface per use case | `Auth` is a concrete struct. Go's implicit interface satisfaction means one can be extracted later without touching the implementation, so adding it now is speculative generality. | A second implementation or a decorator is actually needed |
| `controllers` / `gateways` / `presenters` sub-rings | `internal/adapter` is split by technology instead, which is how Go projects are normally read. | The adapter layer grows past what one directory can hold |

`usecase.MatchRecord` is the one struct that looks like a DTO. It is not
ceremony: it is an immutable snapshot taken while the lock is held, because
handing `*domain.Match` to a store would let the store read the board while
another goroutine mutates it. A concurrency requirement created it, not a
layering rule.

---

## Testing

```shell
go test ./... -race                 # 395 cases, race detector on
go test ./... -cover                # per-package coverage
go vet ./... && gofmt -l .          # both must be silent
```

| Package | Coverage |
| --- | --- |
| `internal/adapter/memory` | 100.0% |
| `internal/usecase` | 91.2% |
| `internal/domain` | 87.6% |
| `internal/adapter/sqlite` | 85.2% |
| `internal/adapter/transport` | 82.6% |
| `internal/adapter/auth` | 78.6% |

`internal/adapter/id` has no tests: it is a three-line delegation to
`uuid.NewString`, and a test would assert that the standard library works.
`internal/adapter/storetest` reports 0% because it *is* test code — it runs
inside the two store packages.

**Game rules.** Every winning line is checked as a row, column and diagonal, in
both directions, for both marks — plus non-winning boards, which is what catches
a detector that reports a false positive. Boards are written as readable layouts
(`"O.X..X..X"`) and failures print a 3×3 grid rather than a raw array.

**Rule enforcement.** Turn alternation, occupied cells, out-of-bounds cells,
moves after the game ends, and moves by someone who is not in the match. A
rejected move must not consume a turn or appear in the history.

**Auth.** Registration validation, duplicate names including padded and
differently-capitalised variants, hasher failure leaving no half-created
account, and the assertion that a wrong password and an unknown user produce
byte-identical errors.

**Concurrency.** Both concurrency tests run under `-race`: eighteen simultaneous
moves against one match, and six simultaneous queue joins that must produce
exactly three disjoint pairs with nobody double-booked.

**Store conformance.** `internal/adapter/storetest` is a single suite run by
both store implementations. Anything one honours and the other does not fails
here rather than in production.

**Round-tripping.** A match saved and reloaded must replay through
`domain.RestoreMatch` into the same position, and corrupt history must fail
loudly rather than produce an impossible board.

**Transport.** `http_test.go` drives the real router over `httptest` with
in-memory stores — nothing faked, handlers included — covering registration
failures, the 401 on every protected route with both a missing and a garbage
token, a full match through to a win, the leaderboard afterwards, and the
assertion that a finished match is dropped so both players are free again.

**WebSocket.** `ws_test.go` opens real sockets against a real server: a bad
token is rejected at the handshake with a `401` rather than a closed socket,
two clients are paired and each told their own mark, a move reaches the
opponent unprompted, an out-of-turn move comes back as an `error` frame, a
disconnect delivers `opponent_left` to the player who stayed, and a second
socket for the same player closes the first instead of orphaning it.

The fakes in `internal/usecase/fakes_test.go` are drop-in replacements for the
ports, which is the point — the use cases cannot tell them from the real thing.

---

## Persistence

Two tables, applied on startup and idempotent, so a restart re-applies them
without complaint.

```sql
players (id, name, name_key UNIQUE, password_hash, wins, losses, draws)
matches (id, player_x, player_o, status, moves)
```

`name_key` holds the lowercased name and carries the unique index, so `Alice`
cannot register alongside `alice` while the chosen capitalisation is still what
gets displayed.

**Moves are stored as `"X0,O3,X1,O4,X2"`**, not JSON, so a game can be read
straight out of the database with the `sqlite3` CLI. The mark is written even
though it alternates predictably — deriving it from position would silently
couple every stored row to the rule that X moves first.

Status is stored as text and is strictly derivable by replaying the moves. It is
denormalised only so finished games can be queried without decoding every row.

`matches` has no foreign key to `players` on purpose. A match is an append-only
record of something that happened, and the constraint would make this store
reject rows the in-memory store accepts — the two must stay substitutable.

`SetMaxOpenConns(1)`: SQLite serialises writers regardless, so a single
connection trades a little read concurrency for never encountering
`SQLITE_BUSY`. At two players per match that costs nothing measurable. A busier
service would raise the limit and lean on the `busy_timeout` pragma instead.

---

## Deployment

A two-stage build. Dependencies download before the source is copied, so editing
code does not invalidate the module cache layer.

**`CGO_ENABLED=0` is a consequence of the driver choice.** `modernc.org/sqlite`
is SQLite transpiled to Go; the common alternative, `mattn/go-sqlite3`, is a cgo
wrapper needing a C compiler at build time and libc at runtime. With cgo off the
binary links statically, and that cascades:

- the runtime image needs no gcc, no libsqlite3 and no glibc
- no musl-versus-glibc mismatch — a cgo binary built against glibc will not run on Alpine
- cross-compilation works with no cross-toolchain
- DNS resolution always uses Go's resolver rather than the system C one, so behaviour does not vary by host

`-trimpath` strips local filesystem paths from the binary, and `-ldflags="-s -w"`
drops the symbol table and debug info to shrink it.

The runtime image is Alpine rather than `scratch` or distroless, purely so the
database can be inspected from a shell; `sqlite3` is installed for that. The
server runs as an unprivileged user, and `/data` is created with that ownership
in the image so an empty named volume inherits it.

---

## Trade-offs and known limitations

- **A disconnect aborts the match rather than forfeiting it.** Neither player's
  stats change. A forfeit would be a new domain rule with its own persistence
  story, and a dropped connection is as often flaky wifi as a rage quit.
- **In-progress matches do not resume across a restart.** Every move is
  persisted and any match can be replayed into its exact position, but the
  server does not rehydrate live games at boot — both clients would have to
  reconnect and re-authenticate first. Player statistics and the leaderboard do
  survive.
- **SQLite with a single connection** is the right shape for this workload and
  the wrong one for a busy service. The ports mean swapping in Postgres is a
  one-line change in the composition root.
- **No rate limiting** on registration or login. A real deployment needs it.
- **The leaderboard is computed on read** from an indexed ordering. Fine at this
  size; a materialised ranking would be needed at scale.

---

## Project layout

```
cmd/server            composition root and process lifecycle
cmd/client            terminal client
internal/domain       entities and rules — standard library only
internal/usecase      application services and the ports they depend on
internal/adapter/
  auth                bcrypt hashing and JWT issuing/verification
  id                  UUID generation
  memory              in-memory stores, used by tests and for running without a database
  sqlite              durable stores, schema and move encoding
  storetest           one behavioural contract both store implementations must satisfy
  transport           HTTP handlers, the WebSocket hub, and the wire format
config/Dockerfile     multi-stage build
docker-compose.yml    single service plus a named volume for the database
```

Each third-party library is confined to exactly one package: `grep -r golang-jwt
internal/` returns one directory, as does `gorilla/websocket` and `bcrypt`.
That is the practical test of whether the dependency rule is real.

## Stack

| Choice | Reason |
| --- | --- |
| Go 1.25.2 | Required by the brief |
| `modernc.org/sqlite` | Pure Go, so `CGO_ENABLED=0` yields a static binary and a minimal image |
| SQLite | Embedded: no service to orchestrate, no connection string to coordinate. The only shared state is one file |
| `gorilla/websocket` | The de facto standard; `net/http` has no WebSocket support |
| `golang-jwt/jwt/v5` | Maintained successor to `dgrijalva/jwt-go`, and the only version that forces explicit algorithm pinning |
| `golang.org/x/crypto/bcrypt` | Password hashing, salt and cost embedded in the output |
| stdlib `net/http` routing | Go 1.22 patterns carry the method (`GET /match`), so no router library is needed |
| Docker + Compose | One command to run, with a named volume so results persist |
