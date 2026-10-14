package trazrextension

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/medxops/trazr-otel/pkg/auth"
)

func TestWriteOnlyMachineCannotReadThroughNonGETSubresources(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "machine-scope-regression-test-secret")
	db, mock := sessionTestDatabase(t)
	token := sessionTestToken(t, 42, "write-key")
	for _, path := range []string{"/api/v1/metrics/gauge/1", "/api/v1/logs/cursor", "/api/v1/profiles/1"} {
		expectSessionLookup(mock, 42, []string{"user"}, []string{"tenant_a"})
		mock.ExpectQuery(`SELECT org_id,project_id,COALESCE\(scopes,`).WithArgs("write-key", int64(42)).
			WillReturnRows(sqlmock.NewRows([]string{"org_id", "project_id", "scopes", "expires_at"}).AddRow(nil, nil, []byte(`["telemetry:write"]`), nil))
		r := httptest.NewRequest(http.MethodPost, path+"?schema=tenant_a", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r = r.WithContext(auth.WithSessionDatabase(r.Context(), db))
		called := false
		w := httptest.NewRecorder()
		withMachineScopes(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || called {
			t.Fatalf("write token reached read handler at %s: status %d called %v", path, w.Code, called)
		}
	}
}
