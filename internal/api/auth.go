package api

import (
	"context"
	"errors"

	"taskforge/internal/auth"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
	ErrUserExists      = errors.New("user already exists")
)

// Principal contains identity values derived from a validated access token.
// TenantID must come from a trusted identity-provider claim.
type Principal = auth.Principal
type Authenticator = auth.Authenticator
type contextKey string

const principalContextKey contextKey = "principal"

func withPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

func principalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}