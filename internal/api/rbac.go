package api

import (
	"net/http"
)

func RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedRoles))

	for _, role := range allowedRoles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := principalFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
				return
			}

			if !allowed[principal.Role] {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
