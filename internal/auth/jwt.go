package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken  = errors.New("invalid access token")
	ErrInvalidSecret = errors.New("JWT secret must contain at least 32 characters")
)

type JWTAuthenticator struct {
	secret   []byte
	issuer   string
	audience string
}

type Claims struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`

	jwt.RegisteredClaims
}

// NewJWTAuthenticator creates a JWT authenticator.
func NewJWTAuthenticator(
	secret string,
	issuer string,
	audience string,
) (*JWTAuthenticator, error) {
	secret = strings.TrimSpace(secret)
	issuer = strings.TrimSpace(issuer)
	audience = strings.TrimSpace(audience)

	if len(secret) < 32 {
		return nil, ErrInvalidSecret
	}

	if issuer == "" {
		return nil, errors.New("JWT issuer is required")
	}

	if audience == "" {
		return nil, errors.New("JWT audience is required")
	}

	return &JWTAuthenticator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
	}, nil

}

// Authenticate validates an access token and returns its principal.
func (a *JWTAuthenticator) Authenticate(
	tokenString string,
) (Principal, error) {
	if strings.TrimSpace(tokenString) == "" {
		return Principal{}, ErrInvalidToken
	}

	claims := &Claims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}

			return a.secret, nil
		},
		jwt.WithIssuer(a.issuer),
		jwt.WithAudience(a.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(30*time.Second),
	)

	if err != nil || token == nil || !token.Valid {
		return Principal{}, fmt.Errorf(
			"%w: token validation failed",
			ErrInvalidToken,
		)
	}

	if claims.Subject == "" ||
		claims.TenantID == "" ||
		claims.Role == "" {
		return Principal{}, ErrInvalidToken
	}

	return Principal{
		Subject:  claims.Subject,
		TenantID: claims.TenantID,
		Role:     claims.Role,
	}, nil

}

// IssueToken creates a signed access token.
func (a *JWTAuthenticator) IssueToken(
	subject string,
	tenantID string,
	role string,
	ttl time.Duration,
) (string, error) {
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("JWT subject is required")
	}

	if strings.TrimSpace(tenantID) == "" {
		return "", errors.New("JWT tenant ID is required")
	}

	if strings.TrimSpace(role) == "" {
		return "", errors.New("JWT role is required")
	}

	if ttl <= 0 {
		return "", errors.New("JWT expiration must be positive")
	}

	now := time.Now()

	claims := Claims{
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    a.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{a.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	signedToken, err := token.SignedString(a.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return signedToken, nil

}
