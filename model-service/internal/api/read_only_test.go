package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jur4ikoff/utils/httpmiddleware"
)

func TestReadOnlyBlocksWritesBeforeHandler(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			called := false
			handler := httpmiddleware.ReadOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, "/", nil))
			allowed := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
			if called != allowed {
				t.Fatalf("handler called = %v, want %v", called, allowed)
			}
			if !allowed && response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", response.Code)
			}
		})
	}
}
