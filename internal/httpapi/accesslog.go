package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/wesadex/typespec-http/internal/api"
)

// maxLoggedErrorBody limits how much of an error response is kept for the log.
const maxLoggedErrorBody = 4 << 10

// accessLog writes one log line per request, for every status code.
// Level: INFO for 2xx/3xx, WARN for 4xx, ERROR for 5xx.
// For errors it also logs error_code and error from the response body, so the
// log says why a request failed.
func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		switch {
		case rec.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case rec.status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("remote", r.RemoteAddr),
		}
		if rec.status >= http.StatusBadRequest {
			var body api.ErrorBody
			if err := json.Unmarshal(rec.errBody.Bytes(), &body); err == nil && body.Code != "" {
				attrs = append(attrs, slog.String("error_code", body.Code), slog.String("error", body.Message))
			}
		}
		log.LogAttrs(r.Context(), level, "request", attrs...)
	})
}

// statusRecorder remembers the status code and body size of a response, and
// the start of the body for error responses.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
	errBody     bytes.Buffer
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	if r.status >= http.StatusBadRequest {
		if room := maxLoggedErrorBody - r.errBody.Len(); room > 0 {
			r.errBody.Write(b[:min(n, room)])
		}
	}
	return n, err
}

// Unwrap lets http.ResponseController reach the real ResponseWriter.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
