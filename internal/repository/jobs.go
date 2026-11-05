package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"taskforge/internal/jobs"
)

var (
	ErrConflict = errors.New("job already exists")
)

type JobRepository interface {
	Create(ctx context.Context, job jobs.Job) error
	GetByID(ctx context.Context, id, tenantID string) (jobs.Job, error)
}

type PostgresJobRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresJobRepository(pool *pgxpool.Pool) *PostgresJobRepository {
	return &PostgresJobRepository{
		pool: pool,
	}
}

// Create inserts a new job into PostgreSQL.
func (r *PostgresJobRepository) Create(
	ctx context.Context,
	job jobs.Job,
) error {
	if _, err := uuid.Parse(job.ID); err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	if job.TenantID == "" {
		return errors.New("tenant ID is required")
	}

	if job.Type == "" || len(job.Payload) == 0 {
		return jobs.ErrInvalidPayload
	}

	if !jsonObject(job.Payload) {
		return jobs.ErrInvalidPayload
	}

	if job.MaxAttempts <= 0 {
		return errors.New("max attempts must be positive")
	}

	if job.Attempts < 0 || job.Attempts > job.MaxAttempts {
		return errors.New("invalid attempt count")
	}

	if job.Status == "" {
		job.Status = jobs.StatusPending
	}

	if !validJobStatus(job.Status) {
		return fmt.Errorf("invalid job status: %s", job.Status)
	}

	now := time.Now().UTC()

	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}

	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = job.CreatedAt
	}

	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = job.CreatedAt
	}

	const query = `
	INSERT INTO jobs (
		id,
		tenant_id,
		type,
		payload,
		status,
		attempts,
		max_attempts,
		scheduled_at,
		created_at,
		updated_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`

	_, err := r.pool.Exec(
		ctx,
		query,
		job.ID,
		job.TenantID,
		job.Type,
		job.Payload,
		job.Status,
		job.Attempts,
		job.MaxAttempts,
		job.ScheduledAt,
		job.CreatedAt,
		job.UpdatedAt,
	)

	if err != nil {
		var pgErr interface {
			SQLState() string
		}

		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return ErrConflict
		}

		return fmt.Errorf("insert job: %w", err)
	}

	return nil

}

// GetByID retrieves a job belonging to the specified tenant.
func (r *PostgresJobRepository) GetByID(
	ctx context.Context,
	id string,
	tenantID string,
) (jobs.Job, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return jobs.Job{}, jobs.ErrJobNotFound
	}

	if tenantID == "" {
		return jobs.Job{}, jobs.ErrJobNotFound
	}

	const query = `
	SELECT
		id,
		tenant_id,
		type,
		payload,
		status,
		attempts,
		max_attempts,
		scheduled_at,
		created_at,
		updated_at
	FROM jobs
	WHERE id = $1
	  AND tenant_id = $2
`

	var job jobs.Job

	err = r.pool.QueryRow(
		ctx,
		query,
		parsedID,
		tenantID,
	).Scan(
		&job.ID,
		&job.TenantID,
		&job.Type,
		&job.Payload,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.ScheduledAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Job{}, jobs.ErrJobNotFound
	}

	if err != nil {
		return jobs.Job{}, fmt.Errorf("get job: %w", err)
	}

	return job, nil

}

// jsonObject verifies that the payload is a valid JSON object.
func jsonObject(payload []byte) bool {
	var value map[string]any

	if err := json.Unmarshal(payload, &value); err != nil {
		return false
	}

	return value != nil

}

// validJobStatus checks whether a status is defined by the jobs package.
func validJobStatus(status jobs.Status) bool {
	switch status {
	case jobs.StatusPending,
		jobs.StatusRunning,
		jobs.StatusRetrying,
		jobs.StatusSucceeded,
		jobs.StatusFailed,
		jobs.StatusDead:
		return true
	default:
		return false
	}
}
