package apis_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHealthAPIPostgres(t *testing.T) {
	t.Parallel()

	guestScenario := tests.ApiScenario{
		Name:   "GET health status postgres (guest)",
		Method: http.MethodGet,
		URL:    "/api/health",
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			return newPostgresApiTestApp(t)
		},
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`"code":200`,
			`"data":{}`,
		},
		NotExpectedContent: []string{
			"dbType",
			"canBackup",
			"realIP",
			"possibleProxyHeader",
		},
		ExpectedEvents: map[string]int{"*": 0},
	}

	guestScenario.Test(t)

	superuserHeaders := map[string]string{}
	superuserScenario := tests.ApiScenario{
		Name:    "GET health status postgres (superuser)",
		Method:  http.MethodGet,
		URL:     "/api/health",
		Headers: superuserHeaders,
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app := newPostgresApiTestApp(t)

			superuser, _ := seedRealtimePostgresSuperusers(t, app)
			token, err := superuser.NewAuthToken()
			if err != nil {
				t.Fatalf("failed to create postgres superuser token: %v", err)
			}

			superuserHeaders["Authorization"] = token

			return app
		},
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`"code":200`,
			`"data":{`,
			`"dbType":"postgres"`,
			`"canBackup":true`,
			`"realIP"`,
			`"possibleProxyHeader"`,
		},
		ExpectedEvents: map[string]int{"*": 0},
	}

	superuserScenario.Test(t)
}
