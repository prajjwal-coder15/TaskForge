package auth

type Principal struct {
	Subject  string
	TenantID string
	Role     string
}

type Authenticator interface {
	Authenticate(token string) (Principal, error)
}