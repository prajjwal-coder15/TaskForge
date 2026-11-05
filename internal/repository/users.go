package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user already exists")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	IsActive     bool
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUserWithTenant creates a user, their first tenant,
// and an owner membership in a single transaction.
func (r *UserRepository) CreateUserWithTenant(
	ctx context.Context,
	email string,
	passwordHash string,
	tenantName string,
) (User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	tenantName = strings.TrimSpace(tenantName)

	if email == "" || passwordHash == "" || tenantName == "" {
		return User{}, "", errors.New("email, password hash, and tenant name are required")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, "", fmt.Errorf("begin registration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var user User

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, is_active
	`, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.IsActive,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, "", ErrUserExists
		}
		return User{}, "", fmt.Errorf("create user: %w", err)
	}

	var tenantID string

	err = tx.QueryRow(ctx, `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id
	`, tenantName).Scan(&tenantID)
	if err != nil {
		return User{}, "", fmt.Errorf("create tenant: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_memberships (user_id, tenant_id, role)
		VALUES ($1, $2, 'owner')
	`, user.ID, tenantID)
	if err != nil {
		return User{}, "", fmt.Errorf("create tenant membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, "", fmt.Errorf("commit registration: %w", err)
	}

	return user, tenantID, nil
}

// GetByEmail retrieves a user by email address.
func (r *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user User

	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, is_active
		FROM users
		WHERE LOWER(email) = $1
	`, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.IsActive,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("get user by email: %w", err)
	}

	return user, nil
}

// GetTenantRole retrieves a user's role in a particular tenant.
func (r *UserRepository) GetTenantRole(
	ctx context.Context,
	userID string,
	tenantID string,
) (string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return "", ErrUserNotFound
	}

	var role string

	err := r.pool.QueryRow(ctx, `
		SELECT role
		FROM tenant_memberships
		WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID).Scan(&role)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}

	if err != nil {
		return "", fmt.Errorf("get tenant role: %w", err)
	}

	return role, nil
}

func (r *UserRepository) UpdatePasswordHash(
	ctx context.Context,
	userID string,
	oldHash string,
	newHash string,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $1,
		    updated_at = NOW()
		WHERE id = $2
		  AND password_hash = $3
	`, newHash, userID, oldHash)

	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("password hash was changed concurrently")
	}

	return nil
}

func (r *UserRepository) GetByID(
	ctx context.Context,
	userID string,
) (User, error) {
	var user User

	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, is_active
		FROM users
		WHERE id = $1
	`, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.IsActive,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("get user by ID: %w", err)
	}

	return user, nil
}