package middleware

import "net/http"

func APIKey(required string) func(http.Handler) http.Handler {
	if required == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-API-Key")
			if got == "" {
				http.Error(w, "missing api key", http.StatusUnauthorized)
				return
			}
			if got != required {
				http.Error(w, "invalid api key", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
