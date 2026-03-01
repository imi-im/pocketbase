lint:
	golangci-lint run -c ./golangci.yml ./...

test:
	go test ./... -v --cover

pg-test-db-reset:
	@if [ -z "$$SKIP_PG_DB_RESET" ]; then \
		if docker ps -a --format '{{.Names}}' | grep -q '^pb17-test$$'; then \
			docker start pb17-test >/dev/null; \
		else \
			docker run --name pb17-test -e POSTGRES_PASSWORD=pbtest -p 5432:5432 -d postgres:17-alpine >/dev/null; \
		fi; \
		for i in $$(seq 1 60); do \
			docker exec pb17-test pg_isready -U postgres >/dev/null 2>&1 && \
			docker exec pb17-test psql -U postgres -d postgres -c "SELECT 1" >/dev/null 2>&1 && exit 0; \
			sleep 1; \
		done; \
		echo "PostgreSQL is not ready after 60s"; \
		exit 1; \
	fi
	@run_psql() { \
		if [ -n "$$SKIP_PG_DB_RESET" ]; then \
			PGPASSWORD=pbtest psql -h 127.0.0.1 -U postgres -d postgres -c "$$1"; \
		else \
			docker exec pb17-test psql -U postgres -d postgres -c "$$1"; \
		fi; \
	}; \
	for sql in \
		"DROP DATABASE IF EXISTS pbtestdb;" \
		"DROP DATABASE IF EXISTS pbtestaux;" \
		"CREATE DATABASE pbtestdb;" \
		"CREATE DATABASE pbtestaux;"; do \
		ok=0; \
		for i in $$(seq 1 20); do \
			if run_psql "$$sql" >/dev/null 2>&1; then ok=1; break; fi; \
			sleep 1; \
		done; \
		if [ $$ok -ne 1 ]; then echo "Failed SQL after retries: $$sql"; exit 1; fi; \
	done

test-pg-smoke: pg-test-db-reset
	PB_DB_DIALECT=postgres PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' go build -o ./tmp/base-pg-test ./examples/base
	PB_DB_DIALECT=postgres PB_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' ./tmp/base-pg-test serve --http=127.0.0.1:8091 --dir=./tmp/pb_pg_smoke > ./tmp/pb_pg_smoke.log 2>&1 & echo $$! > ./tmp/base-pg-test.pid
	sleep 5
	curl -fsS http://127.0.0.1:8091/api/health >/dev/null
	kill $$(cat ./tmp/base-pg-test.pid) 2>/dev/null || true; rm -f ./tmp/base-pg-test.pid

test-pg-migration-consistency: pg-test-db-reset
	PB_TEST_PG_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_TEST_PG_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' go test ./core -run TestMigrationsRunnerPostgresSchemaConsistencyFullVsIncremental -count=1 -v

test-pg-hooks-parity: pg-test-db-reset
	PB_TEST_PG_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_TEST_PG_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' go test ./core -run 'TestTransactionHooksCallsPostgresParity|TestTransactionFromInnerHooksPostgres' -count=1 -v

test-pg-realtime-parity: pg-test-db-reset
	PB_TEST_PG_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_TEST_PG_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' go test ./apis -run 'TestRealtimeRecordResolvePostgres|TestRealtimeAuthRecord(Delete|Update)EventPostgres|TestRealtimeCustomAuthModel(Delete|Update)EventPostgres|TestRecordCrudCreatePostgresPublicCollection|TestHealthAPIPostgres' -count=1 -v

test-pg-backup-parity: pg-test-db-reset
	PB_TEST_PG_DATA_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestdb?sslmode=disable' PB_TEST_PG_AUX_DB_CONN='postgres://postgres:pbtest@127.0.0.1:5432/pbtestaux?sslmode=disable' go test ./core -run 'TestCreateBackupPostgres|TestRestoreBackupPostgresErrors' -count=1 -v

test-pg: test-pg-smoke test-pg-migration-consistency test-pg-hooks-parity test-pg-realtime-parity test-pg-backup-parity

jstypes:
	go run ./plugins/jsvm/internal/types/types.go

test-report:
	go test ./... -v --cover -coverprofile=coverage.out
	go tool cover -html=coverage.out
