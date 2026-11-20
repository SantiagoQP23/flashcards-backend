package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SantiagoQP23/flashcards-backend/internal/auth"
	"github.com/SantiagoQP23/flashcards-backend/internal/database"
	"github.com/SantiagoQP23/flashcards-backend/migrations"
)

// These tests hit a real Postgres. They are skipped unless TEST_DATABASE_URL
// is set (CI sets it; locally run `make db-up && make test-db`).

type fakeGoogle map[string]auth.GoogleIdentity

func (f fakeGoogle) Verify(_ context.Context, token string) (auth.GoogleIdentity, error) {
	id, ok := f[token]
	if !ok {
		return auth.GoogleIdentity{}, auth.ErrInvalidToken
	}
	return id, nil
}

type testAPI struct {
	t       *testing.T
	handler http.Handler
	pool    *pgxpool.Pool
}

func newTestAPI(t *testing.T, google auth.GoogleVerifier) *testAPI {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := migrations.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Deps{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:           pool,
		Pool:         pool,
		AccessTokens: auth.NewAccessTokens([]byte("test-secret-test-secret-test-secret"), 15*time.Minute),
		Google:       google,
		RefreshTTL:   24 * time.Hour,
	})
	return &testAPI{t: t, handler: h, pool: pool}
}

type resp struct {
	Code int
	Body map[string]any
	Raw  string
}

func (r resp) str(key string) string { s, _ := r.Body[key].(string); return s }

func (r resp) user() map[string]any { u, _ := r.Body["user"].(map[string]any); return u }

func (r resp) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (ta *testAPI) do(method, path, token string, body any, header ...string) resp {
	ta.t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	ta.handler.ServeHTTP(rec, req)
	out := resp{Code: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.Body)
	return out
}

func (ta *testAPI) expect(r resp, code int) resp {
	ta.t.Helper()
	if r.Code != code {
		ta.t.Fatalf("got %d, want %d: %s", r.Code, code, r.Raw)
	}
	return r
}

func (ta *testAPI) expectErr(r resp, code int, errCode string) {
	ta.t.Helper()
	ta.expect(r, code)
	if r.errCode() != errCode {
		ta.t.Fatalf("error code = %q, want %q: %s", r.errCode(), errCode, r.Raw)
	}
}

func TestRegisterLoginAndMe(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})

	reg := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{
		"email": " Ana@Example.com ", "password": "correct horse", "display_name": "Ana", "timezone": "America/Guayaquil",
	}, "Accept-Language", "en-US,en;q=0.9"), http.StatusCreated)
	u := reg.user()
	if u["email"] != "ana@example.com" || u["display_name"] != "Ana" || u["timezone"] != "America/Guayaquil" ||
		u["locale"] != "en" || u["has_password"] != true || u["has_google"] != false ||
		u["preferred_accent"] != "US" || u["daily_new_limit"] != float64(20) {
		t.Fatalf("user = %v", u)
	}
	if reg.str("access_token") == "" || reg.str("refresh_token") == "" || reg.str("token_type") != "Bearer" || reg.Body["expires_in"] != float64(900) {
		t.Fatalf("session = %v", reg.Body)
	}

	ta.expectErr(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "ANA@example.com", "password": "another one"}), http.StatusConflict, "email_taken")
	ta.expectErr(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "nope", "password": "correct horse"}), http.StatusBadRequest, "invalid_email")
	ta.expectErr(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "b@example.com", "password": "short"}), http.StatusBadRequest, "invalid_password")
	ta.expectErr(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "b@example.com", "password": "correct horse", "timezone": "Mars/Base"}), http.StatusBadRequest, "invalid_request")

	// No language header and no timezone: Spanish and UTC.
	b := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "b@example.com", "password": "correct horse"}), http.StatusCreated)
	if b.user()["locale"] != "es" || b.user()["timezone"] != "UTC" || b.user()["display_name"] != "" {
		t.Fatalf("defaults = %v", b.user())
	}

	login := ta.expect(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@EXAMPLE.com", "password": "correct horse"}), http.StatusOK)
	ta.expectErr(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "wrong horse"}), http.StatusUnauthorized, "invalid_credentials")
	ta.expectErr(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ghost@example.com", "password": "correct horse"}), http.StatusUnauthorized, "invalid_credentials")

	me := ta.expect(ta.do("GET", "/v1/me", login.str("access_token"), nil), http.StatusOK)
	if me.str("id") != u["id"] {
		t.Fatalf("me = %v", me.Body)
	}
	ta.expectErr(ta.do("GET", "/v1/me", "", nil), http.StatusUnauthorized, "unauthorized")
	ta.expectErr(ta.do("GET", "/v1/me", "not-a-token", nil), http.StatusUnauthorized, "invalid_token")
}

func TestUpdateAndDeleteMe(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	s := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "ana@example.com", "password": "correct horse", "display_name": "Ana"}), http.StatusCreated)
	access := s.str("access_token")

	me := ta.expect(ta.do("PATCH", "/v1/me", access, map[string]any{
		"timezone": "Europe/Madrid", "locale": "en", "preferred_accent": "UK", "daily_new_limit": 10,
	}), http.StatusOK)
	if me.str("timezone") != "Europe/Madrid" || me.str("locale") != "en" || me.str("preferred_accent") != "UK" ||
		me.Body["daily_new_limit"] != float64(10) || me.str("display_name") != "Ana" {
		t.Fatalf("me = %v", me.Body)
	}
	me = ta.expect(ta.do("PATCH", "/v1/me", access, map[string]any{"display_name": "  "}), http.StatusOK)
	if me.str("display_name") != "" || me.str("locale") != "en" {
		t.Fatalf("clearing the name changed other fields: %v", me.Body)
	}

	for _, bad := range []map[string]any{
		{},
		{"locale": "fr"},
		{"preferred_accent": "AU"},
		{"daily_new_limit": 501},
		{"timezone": ""},
		{"timezone": "Local"},
		{"email": "x@example.com"},
	} {
		ta.expectErr(ta.do("PATCH", "/v1/me", access, bad), http.StatusBadRequest, "invalid_request")
	}

	ta.expect(ta.do("DELETE", "/v1/me", access, nil), http.StatusNoContent)
	ta.expectErr(ta.do("GET", "/v1/me", access, nil), http.StatusUnauthorized, "invalid_token")
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": s.str("refresh_token")}), http.StatusUnauthorized, "invalid_refresh_token")
	ta.expectErr(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusUnauthorized, "invalid_credentials")
}

func TestRefreshRotationAndLogout(t *testing.T) {
	ta := newTestAPI(t, fakeGoogle{})
	s := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusCreated)
	first := s.str("refresh_token")

	r1 := ta.expect(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": first}), http.StatusOK)
	second := r1.str("refresh_token")
	if second == first || r1.str("access_token") == "" || r1.user()["email"] != "ana@example.com" {
		t.Fatalf("refresh = %v", r1.Body)
	}

	// A retry with the just-rotated token (lost response) still works.
	retry := ta.expect(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": first}), http.StatusOK)

	// Outside the grace window, reusing a rotated token revokes the family.
	if _, err := ta.pool.Exec(context.Background(), `UPDATE refresh_tokens SET rotated_at = now() - interval '2 minutes' WHERE rotated_at IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": first}), http.StatusUnauthorized, "invalid_refresh_token")
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": second}), http.StatusUnauthorized, "invalid_refresh_token")
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": retry.str("refresh_token")}), http.StatusUnauthorized, "invalid_refresh_token")

	// Logout signs out that device only; unknown tokens are fine.
	phone := ta.expect(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusOK)
	tablet := ta.expect(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusOK)
	ta.expect(ta.do("POST", "/v1/auth/logout", "", map[string]any{"refresh_token": phone.str("refresh_token")}), http.StatusNoContent)
	ta.expect(ta.do("POST", "/v1/auth/logout", "", map[string]any{"refresh_token": "unknown"}), http.StatusNoContent)
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": phone.str("refresh_token")}), http.StatusUnauthorized, "invalid_refresh_token")
	ta.expect(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": tablet.str("refresh_token")}), http.StatusOK)

	// Expired tokens are rejected.
	if _, err := ta.pool.Exec(context.Background(), `UPDATE refresh_tokens SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	other := ta.expect(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusOK)
	if _, err := ta.pool.Exec(context.Background(), `UPDATE refresh_tokens SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": other.str("refresh_token")}), http.StatusUnauthorized, "invalid_refresh_token")
}

func TestGoogleSignIn(t *testing.T) {
	google := fakeGoogle{
		"new":        {Subject: "sub-new", Email: "New@Example.com", EmailVerified: true, Name: "New Person"},
		"ana":        {Subject: "sub-ana", Email: "ana@example.com", EmailVerified: true, Name: "Ana G"},
		"unverified": {Subject: "sub-x", Email: "x@example.com", EmailVerified: false},
	}
	ta := newTestAPI(t, google)

	// First sign-in creates a verified, password-less account.
	n := ta.expect(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "new", "timezone": "America/Guayaquil"}), http.StatusOK)
	u := n.user()
	if u["email"] != "new@example.com" || u["display_name"] != "New Person" || u["has_password"] != false || u["has_google"] != true || u["timezone"] != "America/Guayaquil" {
		t.Fatalf("user = %v", u)
	}
	again := ta.expect(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "new"}), http.StatusOK)
	if again.user()["id"] != u["id"] {
		t.Fatal("returning Google user got a new account")
	}
	// A Google-only account can't log in with a password.
	ta.expectErr(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "new@example.com", "password": "anything goes"}), http.StatusUnauthorized, "invalid_credentials")

	// Linking to a password account with the same email: the email was never
	// proven, so the password and its sessions go.
	ana := ta.expect(ta.do("POST", "/v1/auth/register", "", map[string]any{"email": "ana@example.com", "password": "correct horse", "display_name": "Ana"}), http.StatusCreated)
	linked := ta.expect(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "ana"}), http.StatusOK)
	if linked.user()["id"] != ana.user()["id"] || linked.user()["has_password"] != false || linked.user()["display_name"] != "Ana" {
		t.Fatalf("linked = %v", linked.user())
	}
	ta.expectErr(ta.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": ana.str("refresh_token")}), http.StatusUnauthorized, "invalid_refresh_token")
	ta.expectErr(ta.do("POST", "/v1/auth/login", "", map[string]any{"email": "ana@example.com", "password": "correct horse"}), http.StatusUnauthorized, "invalid_credentials")

	ta.expectErr(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "unverified"}), http.StatusUnauthorized, "email_not_verified")
	ta.expectErr(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "forged"}), http.StatusUnauthorized, "invalid_google_token")
	ta.expectErr(ta.do("POST", "/v1/auth/google", "", map[string]any{}), http.StatusBadRequest, "invalid_request")
}

func TestGoogleNotConfigured(t *testing.T) {
	ta := newTestAPI(t, auth.NewGoogleVerifier(nil))
	ta.expectErr(ta.do("POST", "/v1/auth/google", "", map[string]any{"id_token": "x"}), http.StatusNotImplemented, "google_not_configured")
}
