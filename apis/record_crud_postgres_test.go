package apis_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

func newPostgresApiTestApp(t testing.TB) *tests.TestApp {
	t.Helper()

	dataConn := os.Getenv("PB_TEST_PG_DATA_DB_CONN")
	auxConn := os.Getenv("PB_TEST_PG_AUX_DB_CONN")
	if dataConn == "" || auxConn == "" {
		t.Skip("PB_TEST_PG_DATA_DB_CONN and PB_TEST_PG_AUX_DB_CONN are required")
	}

	app, err := tests.NewTestAppWithDialect(tests.TestAppDBConfig{
		Dialect:          core.DBDialectPostgres,
		DataDBConnString: dataConn,
		AuxDBConnString:  auxConn,
	})
	if err != nil {
		t.Fatalf("failed to init postgres test app: %v", err)
	}

	return app
}

func TestRecordCrudCreatePostgresPublicCollection(t *testing.T) {
	t.Parallel()

	const collectionName = "pg_api_crud_create"

	scenario := tests.ApiScenario{
		Name:   "postgres public collection create record",
		Method: http.MethodPost,
		URL:    "/api/collections/" + collectionName + "/records",
		Body:   strings.NewReader(`{"title":"pg-create-smoke"}`),
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app := newPostgresApiTestApp(t)

			collection := core.NewBaseCollection(collectionName)
			collection.CreateRule = types.Pointer("")
			collection.ListRule = types.Pointer("")
			collection.ViewRule = types.Pointer("")
			collection.UpdateRule = types.Pointer("")
			collection.DeleteRule = types.Pointer("")
			collection.Fields.Add(&core.TextField{Name: "title", Required: true})

			if err := app.Save(collection); err != nil {
				t.Fatalf("failed to create test collection: %v", err)
			}

			return app
		},
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`"title":"pg-create-smoke"`,
		},
		ExpectedEvents: map[string]int{
			"OnRecordCreateRequest":      1,
			"OnRecordAfterCreateSuccess": 1,
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			record, err := app.FindFirstRecordByFilter(collectionName, "title = 'pg-create-smoke'")
			if err != nil {
				t.Fatalf("failed to find created postgres record: %v", err)
			}

			if record.GetString("title") != "pg-create-smoke" {
				t.Fatalf("expected created title pg-create-smoke, got %q", record.GetString("title"))
			}
		},
	}

	scenario.Test(t)
}
