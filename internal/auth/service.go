package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"taskforge/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactiveUser       = errors.New("user account is inactive")
	ErrInvalidInput       = errors.New("invalid registration input")
	ErrInvalidMembership  = errors.New("user does not belong to this tenant")
	ErrUserExists         = errors.New("user exist")
)

const AccessTokenTTL = 15 * time.Minute

type AuthService struct {
	users         *repository.UserRepository
	authenticator *JWTAuthenticator
	refreshTokens *repository.RefreshTokenRepository
}

type AuthResult struct {
	UserID       string
	Email        string
	TenantID     string
	Role         string
	Token        string
	RefreshToken string
}

func NewAuthService(
	users *repository.UserRepository,
	authenticator *JWTAuthenticator,
	refreshTokens *repository.RefreshTokenRepository,
) *AuthService {
	return &AuthService{
		users:         users,
		authenticator: authenticator,
		refreshTokens: refreshTokens,
	}
}

func (s *AuthService) Register(
	ctx context.Context,
	email string,
	password string,
	tenantName string,
) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	tenantName = strings.TrimSpace(tenantName)

	if _, err := mail.ParseAddress(email); err != nil {
		return AuthResult{}, ErrInvalidInput
	}

	if len([]byte(password)) < 8 || len([]byte(password)) > 1024 {
		return AuthResult{}, ErrInvalidInput
	}

	if tenantName == "" || len(tenantName) > 100 {
		return AuthResult{}, ErrInvalidInput
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, err
	}

	user, tenantID, err := s.users.CreateUserWithTenant(
		ctx,
		email,
		passwordHash,
		tenantName,
	)
	if err != nil {
		if errors.Is(err, repository.ErrUserExists) {
			return AuthResult{}, ErrUserExists
		}

		return AuthResult{}, err
	}

	const role = "owner"

	return s.issueTokens(
		ctx,
		user.ID,
		user.Email,
		tenantID,
		role,
	)

}

func (s *AuthService) Login(
	ctx context.Context,
	email string,
	password string,
	tenantID string,
) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" || password == "" || tenantID == "" {
		return AuthResult{}, ErrInvalidCredentials
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, fmt.Errorf("look up login user: %w", err)
	}

	if !user.IsActive {
		return AuthResult{}, ErrInactiveUser
	}

	if !VerifyPassword(password, user.PasswordHash) {
		return AuthResult{}, ErrInvalidCredentials
	}

	// Upgrade legacy bcrypt hashes after successful verification.
	if NeedsPasswordUpgrade(user.PasswordHash) {
		newHash, err := HashPassword(password)
		if err != nil {
			return AuthResult{}, err
		}

		err = s.users.UpdatePasswordHash(
			ctx,
			user.ID,
			user.PasswordHash,
			newHash,
		)
		if err != nil {
			return AuthResult{}, err
		}
	}

	role, err := s.users.GetTenantRole(ctx, user.ID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return AuthResult{}, ErrInvalidMembership
		}
		return AuthResult{}, fmt.Errorf("look up tenant membership: %w", err)
	}

	return s.issueTokens(
		ctx,
		user.ID,
		user.Email,
		tenantID,
		role,
	)
}

func (s *AuthService) issueTokens(
	ctx context.Context,
	userID string,
	email string,
	tenantID string,
	role string,
) (AuthResult, error) {
	accessToken, err := s.authenticator.IssueToken(
		userID,
		tenantID,
		role,
		AccessTokenTTL,
	)
	if err != nil {
		return AuthResult{}, err
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	familyID := NewRefreshTokenFamily()

	err = s.refreshTokens.Create(
		ctx,
		userID,
		tenantID,
		familyID,
		repository.HashRefreshToken(refreshToken),
		time.Now().Add(RefreshTokenTTL),
	)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		UserID:       userID,
		Email:        email,
		TenantID:     tenantID,
		Role:         role,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Refresh implements [api.AuthService].
func (s *AuthService) Refresh(
	ctx context.Context,
	rawRefreshToken string,
) (AuthResult, error) {
	if rawRefreshToken == "" {
		return AuthResult{}, ErrInvalidCredentials
	}

	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		return AuthResult{}, fmt.Errorf("generate refresh token: %w", err)
	}

	rotated, err := s.refreshTokens.Rotate(
		ctx,
		repository.HashRefreshToken(rawRefreshToken),
		repository.HashRefreshToken(newRefreshToken),
		time.Now().Add(RefreshTokenTTL),
	)
	if err != nil {
		return AuthResult{}, err
	}

	accessToken, err := s.authenticator.IssueToken(
		rotated.UserID,
		rotated.TenantID,
		rotated.Role,
		AccessTokenTTL,
	)
	if err != nil {
		return AuthResult{}, fmt.Errorf("issue access token: %w", err)
	}

	return AuthResult{
		UserID:       rotated.UserID,
		Email:        rotated.Email,
		TenantID:     rotated.TenantID,
		Role:         rotated.Role,
		Token:        accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

// Logout revokes the refresh-token family associated with the supplied token.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		return ErrInvalidCredentials
	}

	return s.refreshTokens.Revoke(
		ctx,
		repository.HashRefreshToken(refreshToken),
	)
}
