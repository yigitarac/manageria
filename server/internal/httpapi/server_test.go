package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetHealthz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "get returns ok", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "unknown route is not found", method: http.MethodGet, path: "/other", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
		{name: "wrong method is rejected", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody == "" {
				return
			}
			body := rec.Body.String()
			if body != tt.wantBody {
				// JSON bodies are compared semantically to avoid key-order flakes.
				if !isJSONEqual(t, body, tt.wantBody) {
					t.Fatalf("body = %q, want %q", body, tt.wantBody)
				}
			}
		})
	}
}

func isJSONEqual(t *testing.T, got, want string) bool {
	t.Helper()

	var gotVal, wantVal any
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		return false
	}
	gotNorm, err := json.Marshal(gotVal)
	if err != nil {
		return false
	}
	wantNorm, err := json.Marshal(wantVal)
	return err == nil && string(gotNorm) == string(wantNorm)
}
