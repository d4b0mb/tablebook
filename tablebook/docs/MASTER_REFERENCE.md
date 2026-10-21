# TableBook master reference

Produced by FIM (project mode), 2026-10-21. Readers: the dev agent extending the repo, and a hiring reviewer who clones it. Target: Dev World "Junior Golang Developer / Stage" (wants Go, REST, Docker, CI, SQL visible).

## Contract (must stay true)

- Endpoints: `GET /health`; `GET /reservations?date=YYYY-MM-DD`; `GET|PUT|DELETE /reservations/{id}`; `POST /reservations` (201 + `Location`). JSON everywhere; errors are `{"error":"..."}`; 400 invalid, 404 missing, 409 table taken.
- Fields: `id` int64 (DB), `guest_name` 2-60, `party_size` 1-20, `table_number` 1-30, `starts_at` RFC 3339 and in the future, `notes` optional max 200, `created_at` server-set.
- Double-booking: same table, start-to-start gap strictly under 2h conflicts; exactly 2h is allowed. Checked in SQL (`store.go`), inside the insert/update transaction, against all bookings on the table.
- Timestamps UTC, second precision (unix seconds in SQLite).
- Strict decoding: unknown JSON fields rejected, body capped at 1 MiB. Document it.
- PUT is a full replace and rechecks every create rule, so a booking whose start has passed cannot be updated. Intended.
- No Content-Type check, deliberately (curl examples work without `-H`).
- One DB connection (`SetMaxOpenConns(1)`): deliberate, an interview talking point.
- `ReadHeaderTimeout` 5s and 10s graceful shutdown are enough for this project.

## Done means

All 6 endpoints work incl. error cases; `go test ./...` passes with 8+ cases; `docker build` and `docker run` work from a clean clone; CI green; README complete; the binary has been run once and exercised with real curl; `docker run` verified.

Out of scope: auth, frontend, multiple restaurants, emails, coverage threshold, structured logging. README "Next steps" list is final. Optional small extras: `go test -race`, `store_test.go`.

## Rules for agents

- Do: edit code, tests, README, Dockerfile, CI inside the project; keep go.mod, Dockerfile and CI on one Go version; gofmt clean; comment every exported identifier; table-driven tests; README claims must match the code.
- Do not: add dependencies beyond `modernc.org/sqlite`; add a router, ORM or framework; change the API contract; break handler/store layering; add a Content-Type check; commit, push or create git repos (the user owns that); run `go mod tidy` unprompted; guess the GitHub username or module path.
- Focus: correctness of the SQL rule, test quality, README accuracy, non-root distroless image that can write `/data`, CI green. Ignore: scale, pagination, versioning, rate limiting, OpenAPI.

## State when written

go 1.27.0 installed; `go.mod` is `go 1.26.0` with `modernc.org/sqlite v1.61.0`; `go vet` clean; `go test ./...` passes.

## Unverified, with the command that closes each

1. RESOLVED: GitHub username is `d4b0mb`; module path `github.com/d4b0mb/tablebook` is set in `go.mod`, all imports and the README badge. The repo `d4b0mb/tablebook` must still be created and pushed by the user.
2. Docker: `docker version`; `docker build -t tablebook .`. Pin the builder tag to match go.mod (`golang:1.26`).
3. Container writes `/data` as nonroot: `docker run --rm -p 8080:8080 tablebook`, POST a booking, expect 201. Named volume inherits `/data` ownership; a bind mount may not.
4. `file:...?_pragma=busy_timeout(5000)` DSN honoured by modernc v1.61: search the module source for `_pragma`, or query the pragma against a real file.
5. Real curl run of all six endpoints (`go run ./cmd/server`, README examples). README example dates (2026-11-07) will start failing the future check once past.
6. CI has never run (no repo yet); triggers on pushes to `main` only.
7. `go test -race ./...` (needs cgo on Windows; optional).
