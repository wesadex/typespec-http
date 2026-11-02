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
	}{
		{name: "ok", method: http.MethodGet, path: "/orders/1", wantStatus: 200, wantLevel: "INFO"},
		{name: "not found in handler", method: http.MethodGet, path: "/orders/2", wantStatus: 404, wantLevel: "WARN"},
		{name: "rejected by validator", method: http.MethodGet, path: "/orders/0", wantStatus: 400, wantLevel: "WARN"},
		{name: "unknown path", method: http.MethodGet, path: "/nope", wantStatus: 404, wantLevel: "WARN"},
		{name: "wrong method", method: http.MethodDelete, path: "/orders/1", wantStatus: 405, wantLevel: "WARN"},
		{name: "server error", repo: failingRepo{}, method: http.MethodGet, path: "/orders/1", wantStatus: 500, wantLevel: "ERROR"},
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
