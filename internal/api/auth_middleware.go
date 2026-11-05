package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// RecoverMiddleware converts panics from downstream handlers into generic
// server errors without exposing internal details to clients.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error(
					"HTTP handler panic",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", recovered,
				)

				writeError(
					w,
					http.StatusInternalServerError,
					"INTERNAL_ERROR",
					"Internal server error",
				)
			}
		}()

		next.ServeHTTP(w, r)
	})

}

// RequestLogger records request metadata without logging request bodies,
// authorization headers, or tokens.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		slog.Info(
			"HTTP request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})

}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func RequireAuthentication(
	authenticator Authenticator,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")

		if header == "" {
			writeError(
				w,
				http.StatusUnauthorized,
				"UNAUTHORIZED",
				"Missing authorization header",
			)
			return
		}

		parts := strings.SplitN(header, " ", 2)

		if len(parts) != 2 ||
			!strings.EqualFold(parts[0], "Bearer") ||
			strings.TrimSpace(parts[1]) == "" {
			writeError(
				w,
				http.StatusUnauthorized,
				"UNAUTHORIZED",
				"Invalid authorization header",
			)
			return
		}

		principal, err := authenticator.Authenticate(parts[1])
		if err != nil {
			writeError(
				w,
				http.StatusUnauthorized,
				"UNAUTHORIZED",
				"Invalid or expired access token",
			)
			return
		}

		ctx := withPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
