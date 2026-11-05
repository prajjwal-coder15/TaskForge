package api

import (
	"net/http"

	"taskforge/internal/jobs"
)

type Handler struct {
	jobs jobs.Service
	auth AuthService
}

// NewRouter registers public health and authenticated endpoints.
func NewRouter(jobService jobs.Service, authService AuthService, authenticator Authenticator) http.Handler {
	h := &Handler{jobs: jobService, auth: authService}

	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)

	// Protected endpoints with role-based permissions.
	protected := http.NewServeMux()

	// Create jobs: owner, admin, and member.
	protected.Handle(
		"POST /api/v1/jobs",
		RequireRoles("owner", "admin", "member")(
			http.HandlerFunc(h.createJob),
		),
	)

	// View an individual job: all roles.
	protected.Handle(
		"GET /api/v1/jobs/{id}",
		RequireRoles("owner", "admin", "member", "viewer")(
			http.HandlerFunc(h.getJob),
		),
	)

	// Authentication wraps RBAC so the principal is available in context.
	authenticated := RequireAuthentication(authenticator, protected)

	mux.Handle("/api/v1/jobs", authenticated)
	mux.Handle("/api/v1/jobs/", authenticated)

	return mux
}
