package trazrextension

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/medxops/trazr-otel/pkg/auth"
)

func TestValidatedJWTKeepsAuthorizationContext(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "authorization-context-test-secret")
	t.Setenv("TRAZR_JWT_SECRETS", "")
	t.Setenv("TRAZR_JWT_KID", "")
	t.Setenv("TRAZR_JWT_ISSUER", "zzfuse-test")
	t.Setenv("TRAZR_JWT_AUDIENCE", "zzfuse-test-client")
	token, err := auth.GenerateJWTWithFullContext(7, "admin@example.test", "admin", "tenant-one", []string{"otel"}, []string{"admin"}, []string{"read"}, time.Hour, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	claims, err := ValidateJWTFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(claims["roles"], []string{"admin"}) || !reflect.DeepEqual(claims["schemas"], []string{"otel"}) || claims["tenant_id"] != "tenant-one" {
		t.Fatal("validated token lost its authorization context")
	}
	recorder := httptest.NewRecorder()
	AdminOnlyMiddleware(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("verified admin token rejected: status %d", recorder.Code)
	}
	nonadmin, err := auth.GenerateJWTWithFullContext(8, "user@example.test", "user", "tenant-one", []string{"otel"}, []string{"user"}, nil, time.Hour, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token  string
		status int
	}{{nonadmin, http.StatusForbidden}, {"invalid", http.StatusUnauthorized}, {"", http.StatusUnauthorized}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?role=admin", nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		AdminOnlyMiddleware(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })(w, r)
		if w.Code != tc.status {
			t.Fatalf("unprivileged request status %d, want %d", w.Code, tc.status)
		}
	}
}

func TestIsAdminSupportsTypedAndDecodedRoleLists(t *testing.T) {
	for _, tc := range []struct {
		roles any
		admin bool
	}{{[]string{"user", "admin"}, true}, {[]any{"user", "admin"}, true}, {[]any{42, "user"}, false}, {[]string{"administrator"}, false}, {"admin", false}, {nil, false}} {
		if IsAdmin(jwt.MapClaims{"roles": tc.roles}) != tc.admin {
			t.Fatalf("unexpected admin decision for roles type %T", tc.roles)
		}
	}
}
