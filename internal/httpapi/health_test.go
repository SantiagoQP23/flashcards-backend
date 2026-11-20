package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func serve(t *testing.T, db Pinger, method, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRouter(Deps{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:          db,
		CORSOrigins: []string{"http://localhost:8081"},
	})
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestLiveness(t *testing.T) {
	// Liveness never touches the database, so it answers even when it is down.
	rec := serve(t, fakePinger{errors.New("down")}, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK || body(t, rec)["status"] != "ok" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestReadiness(t *testing.T) {
	rec := serve(t, fakePinger{}, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusOK || body(t, rec)["status"] != "ready" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}

	rec = serve(t, fakePinger{errors.New("down")}, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	e, _ := body(t, rec)["error"].(map[string]any)
	if e["code"] != "database_unavailable" {
		t.Fatalf("error = %v", e)
	}
}

func TestUnknownRouteUsesErrorShape(t *testing.T) {
	rec := serve(t, fakePinger{}, http.MethodGet, "/v1/nope", nil)
	e, _ := body(t, rec)["error"].(map[string]any)
	if rec.Code != http.StatusNotFound || e["code"] != "not_found" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestCORSAllowsExpoWeb(t *testing.T) {
	rec := serve(t, fakePinger{}, http.MethodOptions, "/readyz", map[string]string{
		"Origin":                        "http://localhost:8081",
		"Access-Control-Request-Method": "GET",
	})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}
