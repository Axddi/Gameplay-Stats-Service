# Gameplay Stats Service

A small REST service in Go that records finished multiplayer matches and serves per-player stats and a leaderboard. It uses only the Go standard library, ships as a small non-root Docker image, and has CI that runs the tests with the race detector.

I built it to practise the pieces a backend team uses every day in a live-service setting: a clean HTTP API, safe concurrent state, retries that don't double-count, tests, metrics, a container and a pipeline.

## API

| Method | Path | What it does | Status codes |
|---|---|---|---|
| `POST` | `/matches` | Record a finished match | 201 created, 400 bad JSON, 409 duplicate match ID, 422 invalid match |
| `GET` | `/players/{id}/stats` | Matches, wins, total score and win rate for one player | 200, 404 |
| `GET` | `/leaderboard?limit=10` | Top players by wins, then total score, then ID | 200, 400 bad `limit` (1 to 100) |
| `GET` | `/healthz` | Liveness check | 200 |
| `GET` | `/metrics` | Request counts by route and status, Prometheus text format | 200 |

### Example

```bash
curl -X POST localhost:8080/matches -d '{
  "id": "match-001",
  "players": [
    {"player_id": "alice", "score": 30, "won": true},
    {"player_id": "bob",   "score": 20, "won": false}
  ]
}'

curl localhost:8080/players/alice/stats
# {"player_id":"alice","matches":1,"wins":1,"total_score":30,"win_rate":1}

curl "localhost:8080/leaderboard?limit=5"
```

## Run it

```bash
go run .                       # listens on :8080 (set PORT to change)
make test                      # go vet + go test -race -cover
docker build -t gameplay-stats-service . && docker run -p 8080:8080 gameplay-stats-service
```

Requires Go 1.22 or newer (the router uses method and path-parameter patterns added in 1.22).

## Design decisions

- **Idempotent writes.** Every match has a client-supplied ID. Sending the same match twice returns `409` and does not change anyone's stats, so a client can retry after a timeout without double-counting.
- **Validation at the edge.** Unknown JSON fields, bodies over 1 MiB, fewer than 2 players, duplicate players in one match, empty IDs and negative scores are all rejected with a clear status code and message.
- **Concurrency.** The in-memory store is guarded by a `sync.RWMutex`. Reads (stats, leaderboard) share the lock; writes take it exclusively. The leaderboard copies data under the read lock and sorts outside it, so sorting never blocks writers.
- **Deterministic ordering.** Ties on the leaderboard are broken by total score and then player ID, so the same data always gives the same order.
- **Operations.** Graceful shutdown on SIGINT/SIGTERM, a read-header timeout, a `/healthz` endpoint, request metrics, and a multi-stage Docker build that runs as a non-root user on a distroless base image.

## Tests

- Unit tests for the store: stats accumulation, validation cases, duplicate handling, leaderboard order and limit.
- A concurrency test that records 200 matches from 200 goroutines and checks no update is lost. It runs under `go test -race`.
- HTTP tests with `httptest` for every route and status code, plus the metrics output.

Last local run: all tests pass with `-race`, **87.4% statement coverage**.

## Limitations (on purpose, to keep it small)

- Data is in memory and is lost on restart. The `Store` type is the one place to swap in PostgreSQL or Redis.
- No authentication or rate limiting.
- The leaderboard sorts all players on each request, which is fine for thousands of players but not millions. A real version would keep a sorted structure or use a database index.

## Next steps

- Add a `Store` interface and a PostgreSQL implementation (the existing tests become a contract test for both).
- Deploy to Kubernetes with a Helm chart, readiness and liveness probes, and a Prometheus scrape config.
- Add a Grafana dashboard for request rate and error rate.

## Layout

```
main.go          server start-up and graceful shutdown
server.go        routes, handlers, metrics middleware
store.go         match validation, stats, leaderboard
*_test.go        unit, concurrency and HTTP tests
Dockerfile       multi-stage build, non-root runtime
.github/workflows/ci.yml   gofmt, vet, test -race, docker build
```
