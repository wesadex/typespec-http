package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wesadex/typespec-http/internal/order"
	"github.com/wesadex/typespec-http/internal/storage/memory"
)

func TestAccessLog(t *testing.T) {
	tests := []struct {
		name       string
		repo       order.Repository
		method     string
		path       string
		wantStatus int
		wantLevel  string
		wantCode   string // error_code; empty means no error fields in the log
		wantError  string // part of the error text
	}{
		{name: "ok", method: http.MethodGet, path: "/orders/1", wantStatus: 200, wantLevel: "INFO"},
		{name: "not found in handler", method: http.MethodGet, path: "/orders/2", wantStatus: 404, wantLevel: "WARN",
			wantCode: "order_not_found", wantError: "order 2 not found"},
		{name: "id below minimum", method: http.MethodGet, path: "/orders/0", wantStatus: 400, wantLevel: "WARN",
			wantCode: "invalid_request", wantError: "must be at least 1"},
		{name: "id not a number", method: http.MethodGet, path: "/orders/abc", wantStatus: 400, wantLevel: "WARN",
			wantCode: "invalid_request", wantError: "invalid integer"},
		{name: "unknown path", method: http.MethodGet, path: "/nope", wantStatus: 404, wantLevel: "WARN",
			wantCode: "not_found", wantError: "no such endpoint"},
		{name: "wrong method", method: http.MethodDelete, path: "/orders/1", wantStatus: 405, wantLevel: "WARN",
			wantCode: "method_not_allowed", wantError: "method not allowed"},
		{name: "server error", repo: failingRepo{}, method: http.MethodGet, path: "/orders/1", wantStatus: 500, wantLevel: "ERROR",
			wantCode: "internal_error", wantError: "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			if repo == nil {
				repo = memory.New(testOrder)
			}
			var buf bytes.Buffer
			srv, err := NewServer(order.NewService(repo), slog.New(slog.NewJSONHandler(&buf, nil)))
			if err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

			line := findLogLine(t, buf.String(), "request")
			if line["status"] != float64(tt.wantStatus) || line["level"] != tt.wantLevel {
				t.Errorf("log status/level = %v/%v, want %d/%s", line["status"], line["level"], tt.wantStatus, tt.wantLevel)
			}
			if line["method"] != tt.method || line["path"] != tt.path {
				t.Errorf("log method/path = %v %v, want %s %s", line["method"], line["path"], tt.method, tt.path)
			}
			if line["bytes"] != float64(rec.Body.Len()) {
				t.Errorf("log bytes = %v, want %d", line["bytes"], rec.Body.Len())
			}
			if _, ok := line["duration_ms"].(float64); !ok {
				t.Errorf("log has no duration_ms: %v", line)
			}

			if tt.wantCode == "" {
				if _, ok := line["error_code"]; ok {
					t.Errorf("success must not log error fields: %v", line)
				}
				return
			}
			errText, _ := line["error"].(string)
			if line["error_code"] != tt.wantCode || !strings.Contains(errText, tt.wantError) {
				t.Errorf("log error = %v / %q, want %s / ...%s...", line["error_code"], errText, tt.wantCode, tt.wantError)
			}
		})
	}
}

// findLogLine returns the only JSON log line with the given msg.
func findLogLine(t *testing.T, logs, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for l := range strings.Lines(logs) {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", l)
		}
		if m["msg"] == msg {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want 1 %q log line, got %d; logs:\n%s", msg, len(found), logs)
	}
	return found[0]
}
