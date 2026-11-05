package repository

import (
"context"
"crypto/sha256"
"encoding/hex"
"errors"
"fmt"
"time"


"github.com/jackc/pgx/v5"
"github.com/jackc/pgx/v5/pgxpool"


)

var (
ErrRefreshTokenInvalid    = errors.New("invalid refresh token")
ErrRefreshTokenExpired    = errors.New("refresh token expired")
ErrRefreshTokenReuse      = errors.New("refresh token reuse detected")
ErrRefreshTokenInactive   = errors.New("refresh token user inactive")
ErrRefreshTokenMembership = errors.New("refresh token membership invalid")
)

type RefreshToken struct {
ID        string
UserID    string
TenantID  string
FamilyID  string
Email     string
Role      string
ExpiresAt time.Time
}

type RefreshTokenRepository struct {
pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
return &RefreshTokenRepository{
pool: pool,
}
}

// HashRefreshToken returns the SHA-256 hash of an opaque refresh token.
// Only the hash should be stored in PostgreSQL.
func HashRefreshToken(token string) string {
hash := sha256.Sum256([]byte(token))
return hex.EncodeToString(hash[:])
}

// Create stores a new refresh token hash.
func (r *RefreshTokenRepository) Create(
ctx context.Context,
userID string,
tenantID string,
familyID string,
tokenHash string,
expiresAt time.Time,
) error {
_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (
			user_id,
			tenant_id,
			family_id,
			token_hash,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5)
	`,
userID,
tenantID,
familyID,
tokenHash,
expiresAt,
)

if err != nil {
	return fmt.Errorf("create refresh token: %w", err)
}

return nil


}

// Rotate atomically revokes the old token and creates its replacement.
//
// If an already-revoked token is reused, the entire token family is revoked.
// User activity and tenant membership are validated inside the transaction.
func (r *RefreshTokenRepository) Rotate(
ctx context.Context,
oldTokenHash string,
newTokenHash string,
newExpiresAt time.Time,
) (RefreshToken, error) {
tx, err := r.pool.Begin(ctx)
if err != nil {
return RefreshToken{}, fmt.Errorf(
"begin refresh transaction: %w",
err,
)
}
defer tx.Rollback(ctx)


var current RefreshToken
var revokedAt *time.Time

// 1. Lock the existing refresh token.
err = tx.QueryRow(ctx, `
	SELECT
		id,
		user_id,
		tenant_id,
		family_id,
		expires_at,
		revoked_at
	FROM refresh_tokens
	WHERE token_hash = $1
	FOR UPDATE
`, oldTokenHash).Scan(
	&current.ID,
	&current.UserID,
	&current.TenantID,
	&current.FamilyID,
	&current.ExpiresAt,
	&revokedAt,
)

if errors.Is(err, pgx.ErrNoRows) {
	return RefreshToken{}, ErrRefreshTokenInvalid
}

if err != nil {
	return RefreshToken{}, fmt.Errorf(
		"find refresh token: %w",
		err,
	)
}

// 2. Detect reuse of an already-revoked token.
if revokedAt != nil {
	if err := revokeTokenFamily(ctx, tx, current.FamilyID); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"revoke reused token family: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"commit family revocation: %w",
			err,
		)
	}

	return RefreshToken{}, ErrRefreshTokenReuse
}

// 3. Validate expiration.
if !current.ExpiresAt.After(time.Now()) {
	return RefreshToken{}, ErrRefreshTokenExpired
}

// 4. Lock and validate the user.
var active bool

err = tx.QueryRow(ctx, `
	SELECT email, is_active
	FROM users
	WHERE id = $1
	FOR UPDATE
`, current.UserID).Scan(
	&current.Email,
	&active,
)

if errors.Is(err, pgx.ErrNoRows) {
	return RefreshToken{}, ErrRefreshTokenInvalid
}

if err != nil {
	return RefreshToken{}, fmt.Errorf(
		"validate refresh token user: %w",
		err,
	)
}

if !active {
	if err := revokeTokenFamily(ctx, tx, current.FamilyID); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"revoke inactive user's token family: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"commit inactive user revocation: %w",
			err,
		)
	}

	return RefreshToken{}, ErrRefreshTokenInactive
}

// 5. Lock and validate tenant membership.
err = tx.QueryRow(ctx, `
	SELECT role
	FROM tenant_memberships
	WHERE user_id = $1
	  AND tenant_id = $2
	FOR UPDATE
`,
	current.UserID,
	current.TenantID,
).Scan(&current.Role)

if errors.Is(err, pgx.ErrNoRows) {
	if err := revokeTokenFamily(ctx, tx, current.FamilyID); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"revoke invalid membership token family: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return RefreshToken{}, fmt.Errorf(
			"commit membership revocation: %w",
			err,
		)
	}

	return RefreshToken{}, ErrRefreshTokenMembership
}

if err != nil {
	return RefreshToken{}, fmt.Errorf(
		"validate tenant membership: %w",
		err,
	)
}

// 6. Insert the replacement refresh token.
var replacementID string

err = tx.QueryRow(ctx, `
	INSERT INTO refresh_tokens (
		user_id,
		tenant_id,
		family_id,
		token_hash,
		expires_at
	)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id
`,
	current.UserID,
	current.TenantID,
	current.FamilyID,
	newTokenHash,
	newExpiresAt,
).Scan(&replacementID)

if err != nil {
	return RefreshToken{}, fmt.Errorf(
		"create replacement token: %w",
		err,
	)
}

// 7. Revoke the previous token and link its replacement.
tag, err := tx.Exec(ctx, `
	UPDATE refresh_tokens
	SET revoked_at = NOW(),
	    replaced_by = $1
	WHERE id = $2
	  AND revoked_at IS NULL
`,
	replacementID,
	current.ID,
)

if err != nil {
	return RefreshToken{}, fmt.Errorf(
		"revoke previous token: %w",
		err,
	)
}

if tag.RowsAffected() != 1 {
	return RefreshToken{}, errors.New(
		"refresh token rotation state changed unexpectedly",
	)
}

// 8. Commit the rotation.
if err := tx.Commit(ctx); err != nil {
	return RefreshToken{}, fmt.Errorf(
		"commit refresh rotation: %w",
		err,
	)
}

current.ID = replacementID
current.ExpiresAt = newExpiresAt

return current, nil


}

// revokeTokenFamily revokes every token belonging to a refresh-token family.
func revokeTokenFamily(
ctx context.Context,
tx pgx.Tx,
familyID string,
) error {
_, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, NOW())
		WHERE family_id = $1
	`, familyID)


return err


}

// Revoke revokes the entire refresh-token family associated with a token hash.
func (r *RefreshTokenRepository) Revoke(
ctx context.Context,
tokenHash string,
) error {
tag, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, NOW())
		WHERE family_id = (
			SELECT family_id
			FROM refresh_tokens
			WHERE token_hash = $1
		)
	`,
tokenHash,
)


if err != nil {
	return fmt.Errorf(
		"revoke refresh token family: %w",
		err,
	)
}

if tag.RowsAffected() == 0 {
	return ErrRefreshTokenInvalid
}

return nil


}
