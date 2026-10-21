# TableBook

[![CI](https://github.com/d4b0mb/tablebook/actions/workflows/ci.yml/badge.svg)](https://github.com/d4b0mb/tablebook/actions/workflows/ci.yml)

A small restaurant reservation REST API in Go: create, list, update and cancel table bookings, with double-booking prevention. Standard library `net/http`, SQLite, Docker, GitHub Actions.

## Run it

With Go:

```bash
go run ./cmd/server
```

With Docker:

```bash
docker build -t tablebook .
docker run --rm -p 8080:8080 tablebook
```

Configuration (environment variables): `ADDR` (default `:8080`), `DB_PATH` (default `tablebook.db`; `/data/tablebook.db` in the container, mount a volume on `/data` to keep data).

Tests: `go test ./...`

## Endpoints

| Method | Path | Does | Success | Errors |
|--------|------|------|---------|--------|
| GET | `/health` | Liveness check | 200 | none |
| GET | `/reservations?date=2027-03-06` | List bookings, optional date filter (UTC day) | 200 | 400 bad date |
| GET | `/reservations/{id}` | One booking | 200 | 404 |
| POST | `/reservations` | Create | 201 + `Location` | 400 invalid, 409 table taken |
| PUT | `/reservations/{id}` | Replace | 200 | 400, 404, 409 |
| DELETE | `/reservations/{id}` | Cancel | 204 | 404 |

Errors are JSON: `{"error": "party_size must be 1-20"}`.

| Field | Rule |
|-------|------|
| `guest_name` | required, 2-60 characters |
| `party_size` | 1-20 |
| `table_number` | 1-30 |
| `starts_at` | RFC 3339, must be in the future |
| `notes` | optional, max 200 characters |

**Double-booking rule:** a booking is rejected with 409 if the same table has another booking starting less than 2 hours before or after it. The check runs in SQL inside the same transaction as the insert.

## Examples

```bash
# create
curl -i -X POST localhost:8080/reservations \
  -d '{"guest_name":"Maria Rossi","party_size":4,"table_number":7,"starts_at":"2027-03-06T19:00:00Z"}'

# same table 90 minutes later -> 409
curl -i -X POST localhost:8080/reservations \
  -d '{"guest_name":"Luca Bianchi","party_size":2,"table_number":7,"starts_at":"2027-03-06T20:30:00Z"}'

# list one day, then cancel
curl 'localhost:8080/reservations?date=2027-03-06'
curl -i -X DELETE localhost:8080/reservations/1
```

The `starts_at` date must be in the future, so adjust the dates in these examples once they have passed.

## Layout

```
cmd/server/            config, DB, server start and shutdown
internal/reservation/  model + validation, SQL store, HTTP handlers, tests
migrations/            SQL schema (embedded into the binary)
```

Handlers never write SQL; the store never touches HTTP.

## Design notes

- **Single SQLite connection.** The server calls `SetMaxOpenConns(1)`, so the check-then-insert for double-booking cannot interleave with another request.
- **Conflict check is SQL.** It runs in the same transaction as the insert or update, against all bookings on that table.
- **Strict JSON decoding.** Unknown fields are rejected and request bodies are capped at 1 MiB.
- **Timestamps** are stored and returned in UTC with second precision (unix seconds in SQLite).
- **PUT is a full replace.** It rechecks every create rule, so a booking whose start has already passed cannot be updated.

## Next steps

- Authentication for staff
- Opening hours and per-table capacity checks
- Multiple restaurants
- Confirmation emails
- Versioned migrations
