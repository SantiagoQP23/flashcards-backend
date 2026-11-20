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

func get(t *testing.T, db Pinger, path string) *httptest.ResponseRecorder {
	t.Helper()
	router := NewRouter(NewServer(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		want       Health
	}{
		{"database up", nil, http.StatusOK, Health{Status: HealthStatusOk, Database: HealthDatabaseUp}},
		{"database down", errors.New("down"), http.StatusServiceUnavailable, Health{Status: HealthStatusDegraded, Database: HealthDatabaseDown}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, fakePinger{tt.pingErr}, "/v1/health")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var got Health
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("body = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestServesSpec(t *testing.T) {
	rec := get(t, fakePinger{}, "/openapi.yaml")
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("status = %d, body length = %d", rec.Code, rec.Body.Len())
	}
}

func TestSpecMatchesGeneratedCode(t *testing.T) {
	if _, err := GetSwagger(); err != nil {
		t.Fatalf("embedded spec does not load: %v", err)
	}
}
