package httpmiddleware

import "net/http"

// ReadOnly rejects state-changing requests before they reach a handler.
func ReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(w, "read-only instance", http.StatusMethodNotAllowed)
		}
	})
}
