package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth_ValidTokenOnProtectedRoute(t *testing.T) {
	// Health bypass is handled by Chi route groups (see TestNewServerAuthIntegration).
	// This test verifies the middleware itself accepts valid tokens.
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	r.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBearerAuth_EmptyTokenDisablesAuth(t *testing.T) {
	handler := BearerAuth("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("empty token: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBearerAuth_MissingHeader(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing header: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBearerAuth_WrongScheme(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong scheme: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBearerAuth_InvalidToken(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("invalid token: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBearerAuth_ValidToken(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
	r.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBearerAuth_ConstantTimeComparison(t *testing.T) {
	// Ensure a similar-but-wrong token is rejected.
	handler := BearerAuth("abcdef123456")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	r.Header.Set("Authorization", "Bearer abcdef123457")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("similar token: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBearerAuth_CaseInsensitiveScheme(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	r.Header.Set("Authorization", "bearer secret")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("lowercase bearer: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBearerAuth_EmptyBearerValue(t *testing.T) {
	handler := BearerAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	r.Header.Set("Authorization", "Bearer ")
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("empty bearer value: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
