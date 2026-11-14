package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jur4ikoff/utils/telemetry"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogRequests records request outcomes without logging credentials or user data.
func LogRequests(logger zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(telemetry.WithLogger(r.Context(), logger))
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		response := &loggedResponse{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)

		level := zerolog.InfoLevel
		if response.status >= 500 {
			level = zerolog.ErrorLevel
		} else if response.status >= 400 {
			level = zerolog.WarnLevel
		}
		log.Ctx(r.Context()).WithLevel(level).Str("method", r.Method).
			Str("endpoint", endpointGroup(r.URL.Path)).
			Int("status", response.status).
			Dur("duration_ms", time.Since(start)).Msg("HTTP request completed")
	})
}

func endpointGroup(path string) string {
	for _, group := range []string{"auth", "users", "projects", "roles"} {
		prefix := "/api/v1/" + group
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return group
		}
	}
	return "other"
}

type loggedResponse struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *loggedResponse) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggedResponse) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *loggedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
