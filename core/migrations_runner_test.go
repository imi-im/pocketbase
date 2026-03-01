package core_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

type pgSchemaSnapshot struct {
	Tables  []string
	Columns []string
	Indexes []string
}

func TestMigrationsRunnerUpAndDown(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	callsOrder := []string{}

	l := core.MigrationsList{}
	l.Register(func(app core.App) error {
		callsOrder = append(callsOrder, "up2")
		return nil
	}, func(app core.App) error {
		callsOrder = append(callsOrder, "down2")
		return nil
	}, "2_test")
	l.Register(func(app core.App) error {
		callsOrder = append(callsOrder, "up3")
		return nil
	}, func(app core.App) error {
		callsOrder = append(callsOrder, "down3")
		return nil
	}, "3_test")
	l.Register(func(app core.App) error {
		callsOrder = append(callsOrder, "up1")
		return nil
	}, func(app core.App) error {
		callsOrder = append(callsOrder, "down1")
		return nil
	}, "1_test")
	l.Register(func(app core.App) error {
		callsOrder = append(callsOrder, "up4")
		return nil
	}, func(app core.App) error {
		callsOrder = append(callsOrder, "down4")
		return nil
	}, "4_test")
	l.Add(&core.Migration{
		Up: func(app core.App) error {
			callsOrder = append(callsOrder, "up5")
			return nil
		},
		Down: func(app core.App) error {
			callsOrder = append(callsOrder, "down5")
			return nil
		},
		File: "5_test",
		ReapplyCondition: func(txApp core.App, runner *core.MigrationsRunner, fileName string) (bool, error) {
			return true, nil
		},
	})

	runner := core.NewMigrationsRunner(app, l)

	// ---------------------------------------------------------------
	// simulate partially out-of-order applied migration
	// ---------------------------------------------------------------

	_, err := app.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
		"file":    "4_test",
		"applied": time.Now().UnixMicro() - 2,
	}).Execute()
	if err != nil {
		t.Fatalf("Failed to insert 5_test migration: %v", err)
	}

	_, err = app.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
		"file":    "5_test",
		"applied": time.Now().UnixMicro() - 1,
	}).Execute()
	if err != nil {
		t.Fatalf("Failed to insert 5_test migration: %v", err)
	}

	_, err = app.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
		"file":    "2_test",
		"applied": time.Now().UnixMicro(),
	}).Execute()
	if err != nil {
		t.Fatalf("Failed to insert 2_test migration: %v", err)
	}

	// ---------------------------------------------------------------
	// Up()
	// ---------------------------------------------------------------

	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}

	expectedUpCallsOrder := `["up1","up3","up5"]` // skip up2 and up4 since they were applied already (up5 has extra reapply condition)

	upCallsOrder, err := json.Marshal(callsOrder)
	if err != nil {
		t.Fatal(err)
	}

	if v := string(upCallsOrder); v != expectedUpCallsOrder {
		t.Fatalf("Expected Up() calls order %s, got %s", expectedUpCallsOrder, upCallsOrder)
	}

	// ---------------------------------------------------------------

	// reset callsOrder
	callsOrder = []string{}

	// simulate unrun migration
	l.Register(nil, func(app core.App) error {
		callsOrder = append(callsOrder, "down6")
		return nil
	}, "6_test")

	// simulate applied migrations from different migrations list
	_, err = app.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
		"file":    "from_different_list",
		"applied": time.Now().UnixMicro(),
	}).Execute()
	if err != nil {
		t.Fatalf("Failed to insert from_different_list migration: %v", err)
	}

	// ---------------------------------------------------------------

	// ---------------------------------------------------------------
	// Down()
	// ---------------------------------------------------------------

	if _, err := runner.Down(2); err != nil {
		t.Fatal(err)
	}

	expectedDownCallsOrder := `["down5","down3"]` // revert in the applied order

	downCallsOrder, err := json.Marshal(callsOrder)
	if err != nil {
		t.Fatal(err)
	}

	if v := string(downCallsOrder); v != expectedDownCallsOrder {
		t.Fatalf("Expected Down() calls order %s, got %s", expectedDownCallsOrder, downCallsOrder)
	}
}

func TestMigrationsRunnerRemoveMissingAppliedMigrations(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// mock migrations history
	for i := 1; i <= 3; i++ {
		_, err := app.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
			"file":    fmt.Sprintf("%d_test", i),
			"applied": time.Now().UnixMicro(),
		}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}

	if !isMigrationApplied(app, "2_test") {
		t.Fatalf("Expected 2_test migration to be applied")
	}

	// create a runner without 2_test to mock deleted migration
	l := core.MigrationsList{}
	l.Register(func(app core.App) error {
		return nil
	}, func(app core.App) error {
		return nil
	}, "1_test")
	l.Register(func(app core.App) error {
		return nil
	}, func(app core.App) error {
		return nil
	}, "3_test")

	r := core.NewMigrationsRunner(app, l)

	if err := r.RemoveMissingAppliedMigrations(); err != nil {
		t.Fatalf("Failed to remove missing applied migrations: %v", err)
	}

	if isMigrationApplied(app, "2_test") {
		t.Fatalf("Expected 2_test migration to NOT be applied")
	}
}

func TestMigrationsRunnerPostgresSchemaConsistencyFullVsIncremental(t *testing.T) {
	dataConn := os.Getenv("PB_TEST_PG_DATA_DB_CONN")
	auxConn := os.Getenv("PB_TEST_PG_AUX_DB_CONN")

	if dataConn == "" || auxConn == "" {
		t.Skip("PB_TEST_PG_DATA_DB_CONN and PB_TEST_PG_AUX_DB_CONN are required")
	}

	app, cleanup, err := newRawPostgresMigrationTestApp(dataConn, auxConn)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := resetPostgresSchemas(app); err != nil {
		t.Fatalf("failed to reset postgres schemas before full migration run: %v", err)
	}

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("full migration run failed: %v", err)
	}

	fullDataSnapshot, err := capturePostgresSchemaSnapshot(app.DB())
	if err != nil {
		t.Fatalf("failed to capture full data snapshot: %v", err)
	}

	fullAuxSnapshot, err := capturePostgresSchemaSnapshot(app.AuxDB())
	if err != nil {
		t.Fatalf("failed to capture full aux snapshot: %v", err)
	}

	if err := resetPostgresSchemas(app); err != nil {
		t.Fatalf("failed to reset postgres schemas before incremental migration run: %v", err)
	}

	combined := core.MigrationsList{}
	combined.Copy(core.SystemMigrations)
	combined.Copy(core.AppMigrations)

	items := combined.Items()
	if len(items) < 2 {
		t.Fatalf("expected at least 2 migrations, got %d", len(items))
	}

	split := len(items) / 2
	firstHalf := core.MigrationsList{}
	for _, item := range items[:split] {
		firstHalf.Add(item)
	}

	if _, err := core.NewMigrationsRunner(app, firstHalf).Up(); err != nil {
		t.Fatalf("incremental first-half migration run failed: %v", err)
	}

	if _, err := core.NewMigrationsRunner(app, combined).Up(); err != nil {
		t.Fatalf("incremental full migration completion failed: %v", err)
	}

	incrementalDataSnapshot, err := capturePostgresSchemaSnapshot(app.DB())
	if err != nil {
		t.Fatalf("failed to capture incremental data snapshot: %v", err)
	}

	incrementalAuxSnapshot, err := capturePostgresSchemaSnapshot(app.AuxDB())
	if err != nil {
		t.Fatalf("failed to capture incremental aux snapshot: %v", err)
	}

	if !reflect.DeepEqual(fullDataSnapshot, incrementalDataSnapshot) {
		t.Fatalf("data schema mismatch between full and incremental runs\nfull=%+v\nincremental=%+v", fullDataSnapshot, incrementalDataSnapshot)
	}

	if !reflect.DeepEqual(fullAuxSnapshot, incrementalAuxSnapshot) {
		t.Fatalf("aux schema mismatch between full and incremental runs\nfull=%+v\nincremental=%+v", fullAuxSnapshot, incrementalAuxSnapshot)
	}
}

func newRawPostgresMigrationTestApp(dataConn, auxConn string) (*core.BaseApp, func(), error) {
	tempDir, err := os.MkdirTemp("", "pb_pg_migrations_test_*")
	if err != nil {
		return nil, nil, err
	}

	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:          tempDir,
		EncryptionEnv:    "pb_test_env",
		DBDialect:        core.DBDialectPostgres,
		DataDBConnString: dataConn,
		AuxDBConnString:  auxConn,
	})

	if err := app.Bootstrap(); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, nil, err
	}

	cleanup := func() {
		_ = app.ResetBootstrapState()
		_ = os.RemoveAll(tempDir)
	}

	return app, cleanup, nil
}

func resetPostgresSchemas(app core.App) error {
	dbs := []dbx.Builder{app.DB(), app.AuxDB()}
	for _, db := range dbs {
		if _, err := db.NewQuery("DROP SCHEMA IF EXISTS public CASCADE").Execute(); err != nil {
			return err
		}

		if _, err := db.NewQuery("CREATE SCHEMA public").Execute(); err != nil {
			return err
		}
	}

	return nil
}

func capturePostgresSchemaSnapshot(db dbx.Builder) (pgSchemaSnapshot, error) {
	snapshot := pgSchemaSnapshot{}

	err := db.NewQuery(`
		SELECT tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname = 'public'
		ORDER BY tablename
	`).Column(&snapshot.Tables)
	if err != nil {
		return snapshot, err
	}

	err = db.NewQuery(`
		SELECT table_name || '.' || column_name || ':' || udt_name || ':' || is_nullable || ':' || COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`).Column(&snapshot.Columns)
	if err != nil {
		return snapshot, err
	}

	err = db.NewQuery(`
		SELECT schemaname || '.' || tablename || '.' || indexname || ':' || indexdef
		FROM pg_indexes
		WHERE schemaname = 'public'
		ORDER BY tablename, indexname
	`).Column(&snapshot.Indexes)
	if err != nil {
		return snapshot, err
	}

	return snapshot, nil
}

func isMigrationApplied(app core.App, file string) bool {
	var exists int

	err := app.DB().Select("(1)").
		From(core.DefaultMigrationsTable).
		Where(dbx.HashExp{"file": file}).
		Limit(1).
		Row(&exists)

	return err == nil && exists > 0
}
