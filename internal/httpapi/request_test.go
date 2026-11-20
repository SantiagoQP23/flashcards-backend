package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", `{"name":"Daily"}`, false},
		{"empty", ``, true},
		{"unknown field", `{"name":"x","extra":1}`, true},
		{"two objects", `{"name":"a"}{"name":"b"}`, true},
		{"not JSON", `name=x`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var p payload
			err := decodeJSON(rec, req, &p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !writeBadRequest(rec, err) {
				t.Fatal("decode errors must be answered as 400")
			}
			if err != nil && rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}
}
