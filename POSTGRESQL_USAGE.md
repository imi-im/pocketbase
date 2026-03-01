# PocketBase PostgreSQL Usage Guide

This document provides a shared, practical workflow for developing, debugging, and validating PostgreSQL support in this repository.

## 1. Scope

- Goal: run PocketBase with PostgreSQL and verify the `examples/base` runtime path (`go run` and `go build`).
- Typical use cases: local development, feature validation, regression checks, and handoff between collaborators.

## 2. Prerequisites

- Go `1.24+`
- Docker
- Available local ports:
  - `5432` for PostgreSQL
  - `8090` / `8091` for `examples/base` health checks

## 3. Quick Start (recommended)

From the repository root, run:

```bash
make test-pg
```

What this does:

- Starts or reuses the `pb17-test` PostgreSQL container.
- Resets test databases (`pbtestdb`, `pbtestaux`).
- Runs PostgreSQL smoke checks.
- Runs PostgreSQL parity test groups (migrations, hooks, realtime, backup).

Use this command as the default acceptance gate before opening or updating a PR.

## 4. Manual PostgreSQL Container Setup

Start container:

```bash
docker run --name pb17-test --rm -d \
  -p 5432:5432 \
  -e POSTGRES_PASSWORD=pbtest \
  -e POSTGRES_DB=postgres \
  postgres:17-alpine
```

Readiness check:

```bash
docker exec pb17-test pg_isready -U postgres
```

Create dedicated data and aux databases (do not reuse the same DB for both):

```bash
docker exec pb17-test psql -U postgres -d postgres -c "CREATE DATABASE pbtestdb;"
docker exec pb17-test psql -U postgres -d postgres -c "CREATE DATABASE pbtestaux;"
```

Reset databases for a clean rerun:

```bash
docker exec pb17-test psql -U postgres -d postgres -c "DROP DATABASE IF EXISTS pbtestdb;"
docker exec pb17-test psql -U postgres -d postgres -c "DROP DATABASE IF EXISTS pbtestaux;"
docker exec pb17-test psql -U postgres -d postgres -c "CREATE DATABASE pbtestdb;"
docker exec pb17-test psql -U postgres -d postgres -c "CREATE DATABASE pbtestaux;"
```

## 5. Runtime Environment Variables

- Required: `PB_DATA_DB_CONN=postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable`
- Optional: `PB_AUX_DB_CONN=...` (if omitted, it falls back to `PB_DATA_DB_CONN`)
- Optional: `PB_DB_DIALECT=postgres` (auto-forced when `PB_DATA_DB_CONN` is a PostgreSQL URI)

Environment resolution rules (to reduce configuration):

- If `PB_AUX_DB_CONN` is empty, it defaults to `PB_DATA_DB_CONN`.
- If `PB_DATA_DB_CONN` is a PostgreSQL URI (`postgres://` or `postgresql://`), `PB_DB_DIALECT` is forced to `postgres`.

Minimal setup example (single variable):

```bash
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable'
```

Recommended explicit setup (separate data/aux databases):

```bash
PB_DB_DIALECT=postgres
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable'
PB_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable'
```

For PostgreSQL test suites in this repo:

- `PB_TEST_PG_DATA_DB_CONN`
- `PB_TEST_PG_AUX_DB_CONN`

## 6. Validate with go run

```bash
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' \
go run ./examples/base serve --http=127.0.0.1:8090 --dir=./tmp/pb_pg_run
```

If you want a dedicated aux database, add:

```bash
PB_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable'
```

Health check:

```bash
curl -i --max-time 5 http://127.0.0.1:8090/api/health
```

Expected result: `HTTP/1.1 200 OK` and a healthy API payload.

Health response visibility:

- Guest or regular auth: `data` is an empty object (`{}`).
- Superuser auth: `data` includes diagnostic fields such as `dbType`, `canBackup`, `realIP`, and `possibleProxyHeader`.

## 7. Validate with go build

Build:

```bash
go build -o ./tmp/base-pg-test ./examples/base
```

Run binary:

```bash
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' \
./tmp/base-pg-test serve --http=127.0.0.1:8091 --dir=./tmp/pb_pg_build
```

If needed, append `PB_AUX_DB_CONN` to use a separate aux database.

Health check:

```bash
curl -i --max-time 5 http://127.0.0.1:8091/api/health
```

Tip: if you need to verify PostgreSQL dialect exposure, call `/api/health` with a valid superuser auth token and confirm `"dbType":"postgres"`.

## 8. Useful Make Targets

- `make pg-test-db-reset`: ensure container is ready and recreate test DBs.
- `make test-pg-smoke`: build and run smoke test with PostgreSQL.
- `make test-pg-migration-consistency`: check full-init vs incremental migration parity.
- `make test-pg-hooks-parity`: validate transaction hook parity.
- `make test-pg-realtime-parity`: validate realtime/auth parity and PostgreSQL CRUD parity tests.
- `make test-pg-backup-parity`: validate backup/restore error-path parity.
- `make test-pg`: run all PostgreSQL gates above.

## 9. Troubleshooting

### 9.1 `functions in index expression must be marked IMMUTABLE`

Cause: PostgreSQL restricts non-immutable functions in index expressions.

Guidelines:

- Prefer immutable expressions in indexes.
- Avoid function chains that are not immutable in index definitions.

### 9.2 `_migrations.applied` overflow (`int4`)

Cause: migration timestamp values (for example `UnixMicro`) exceed `int4` range.

Guideline:

- Ensure `_migrations.applied` uses `BIGINT` in PostgreSQL.

### 9.3 `rowid` usage errors

Cause: PostgreSQL does not support SQLite `rowid` semantics.

Guideline:

- Use explicit sortable fields (`created`, `id`, etc.) instead of `rowid`.

### 9.4 Quoting mismatch in index `WHERE` clauses

Cause: backticks in SQL fragments are SQLite-specific and invalid in PostgreSQL.

Guideline:

- Ensure PostgreSQL SQL uses standard double quotes (`"..."`) when quoting identifiers.

### 9.5 Container is running but tests fail to connect

Checks:

- Confirm `pb17-test` exists and is healthy (`docker ps`, `pg_isready`).
- Confirm `pbtestdb` and `pbtestaux` were created.
- Confirm connection strings point to `127.0.0.1:5432` with `sslmode=disable`.
- Run `make pg-test-db-reset` and retry.

## 10. Collaboration Checklist (before PR)

- Run `make test-pg` at least once on a clean reset.
- If migration SQL changed, validate on an empty PostgreSQL database.
- Include in PR description:
  - container startup command
  - sanitized data/aux connection strings
  - health-check output (`/api/health`)
  - relevant `make test-pg` summary

## 11. Cleanup

Stop and remove test container:

```bash
docker rm -f pb17-test
```

## 12. SQLite to PostgreSQL Migration Plan

This section describes a safe, staged migration approach when your current environment is SQLite and the target is PostgreSQL.

### 12.1 Migration Principles

- Use the same PocketBase version on source (SQLite) and target (PostgreSQL).
- Validate on staging first; do not start with production data.
- Prefer a short write-freeze window for final cutover to avoid data divergence.
- Keep rollback simple: preserve the original SQLite deployment until PostgreSQL is verified.

### 12.2 Recommended Phases

1. **Prepare target PostgreSQL environment**
  - Provision PostgreSQL and create dedicated `data` and `aux` databases.
  - Start PocketBase with PostgreSQL connection variables:
    - required: `PB_DATA_DB_CONN=...`
    - optional: `PB_AUX_DB_CONN=...`
    - optional: `PB_DB_DIALECT=postgres`
  - Run `make test-pg` to ensure the environment is healthy before importing data.

2. **Migrate schema first**
  - Ensure all collections, indexes, and migrations are applied on the PostgreSQL target.
  - If your project uses migration files, execute them on the target and verify no migration errors.

3. **Migrate business data**
  - Export records from SQLite and import into PostgreSQL in deterministic batches.
  - Migrate in dependency order where applicable (for example: reference tables before dependent records).
  - Preserve primary identifiers and critical timestamps when your migration path supports it.

4. **Validate parity**
  - Compare key metrics between source and target:
    - per-collection record counts
    - sampled record checks (IDs, key fields, relations)
    - auth/login flow checks
    - file access and API behavior checks
  - Run smoke checks against target `/api/health` and core API paths.

5. **Cutover**
  - Announce a short write freeze on SQLite.
  - Run final incremental sync (delta since initial export).
  - Switch runtime to PostgreSQL connection settings.
  - Monitor logs, health endpoint, and critical APIs.

6. **Rollback plan (must be ready before cutover)**
  - Keep SQLite deployment and data snapshot intact.
  - If severe issues occur after cutover, switch traffic back to SQLite and investigate off-path.

### 12.3 Command-line Playbook (SQLite -> PostgreSQL)

The commands below are a practical template. Adjust paths, table lists, and credentials for your environment.

1. **Back up source SQLite first**

```bash
mkdir -p ./tmp/migration
cp ./pb_data/data.db ./tmp/migration/data.db.bak
cp ./pb_data/auxiliary.db ./tmp/migration/auxiliary.db.bak 2>/dev/null || true
```

2. **Prepare PostgreSQL and reset target databases**

```bash
make pg-test-db-reset
```

3. **Apply PocketBase schema/migrations on PostgreSQL target**

```bash
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' \
go run ./examples/base serve --http=127.0.0.1:8092 --dir=./tmp/pb_pg_migrate
```

If you maintain a separate aux DB, prepend:

```bash
PB_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable'
```

In another terminal, verify startup and stop after schema initialization:

```bash
curl -fsS http://127.0.0.1:8092/api/health
pkill -f 'examples/base serve --http=127.0.0.1:8092' || true
```

4. **Export data from SQLite as CSV (table-by-table)**

```bash
mkdir -p ./tmp/migration/export

# list tables
sqlite3 ./pb_data/data.db ".tables"

# example exports (repeat for all business tables)
sqlite3 ./pb_data/data.db -cmd ".headers on" -cmd ".mode csv" \
  "SELECT * FROM _collections;" > ./tmp/migration/export/_collections.csv

sqlite3 ./pb_data/data.db -cmd ".headers on" -cmd ".mode csv" \
  "SELECT * FROM _externalAuths;" > ./tmp/migration/export/_externalAuths.csv
```

5. **Import CSV data into PostgreSQL**

```bash
export PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable'

# example imports (same table order as your dependency graph)
psql "$PB_DATA_DB_CONN" -c "\\copy \"_collections\" FROM './tmp/migration/export/_collections.csv' WITH (FORMAT csv, HEADER true)"
psql "$PB_DATA_DB_CONN" -c "\\copy \"_externalAuths\" FROM './tmp/migration/export/_externalAuths.csv' WITH (FORMAT csv, HEADER true)"
```

6. **Run parity checks (counts + health)**

```bash
# source counts (SQLite)
sqlite3 ./pb_data/data.db "SELECT COUNT(*) FROM _collections;"

# target counts (PostgreSQL)
psql "$PB_DATA_DB_CONN" -tAc "SELECT COUNT(*) FROM \"_collections\";"

# gate checks
make test-pg
```

7. **Cutover and monitor**

```bash
PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' \
./tmp/base-pg-test serve --http=127.0.0.1:8091 --dir=./tmp/pb_pg_prod

curl -i --max-time 5 http://127.0.0.1:8091/api/health
```

Optional (recommended for production): set `PB_AUX_DB_CONN` to a dedicated aux database.

8. **Rollback (if required)**

```bash
# stop postgres-backed instance
pkill -f 'base-pg-test serve --http=127.0.0.1:8091' || true

# restore sqlite-backed runtime (example)
go run ./examples/base serve --http=127.0.0.1:8090 --dir=./pb_data
```

### 12.4 Practical Validation Checklist

- `make test-pg` passes on the target environment.
- `/api/health` returns `200`.
- Superuser-only diagnostics in `/api/health` are visible as expected (including `dbType`).
- Core CRUD flows work for representative collections.
- Auth flows (login, refresh, protected routes) succeed.
- Realtime and backup-related checks pass for your workload.

### 12.5 Common Pitfalls During Migration

- Source and target running different PocketBase versions.
- Missing migration execution on target before importing data.
- Skipping final delta sync during write freeze.
- No rollback runbook prepared before production cutover.
