package jobs

// Import the packages required for context management, random ID generation,
// JSON validation, time handling, and error reporting.
import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxPayloadSize defines the largest allowed JSON payload for a job.
// A 256 KiB cap helps prevent oversized requests and reduces memory pressure.
const MaxPayloadSize = 256 * 1024 // 256 KiB

// allowedTypes contains the set of job types that the system accepts.
var allowedTypes = map[string]struct{}{
	"email":           {},
	"report.generate": {},
}

// ValidateInput checks that the submitted job input is structurally valid and
// matches the accepted job types and payload rules.
func ValidateInput(input CreateInput) error {
	// Reject any unrecognized job type before checking payload contents.
	if _, ok := allowedTypes[input.Type]; !ok {
		return ErrInvalidJobType
	}

	// The payload must exist, fit within the size limit, and be valid JSON.
	if len(input.Payload) == 0 ||
		len(input.Payload) > MaxPayloadSize ||
		!json.Valid(input.Payload) {
		return ErrInvalidPayload
	}

	// Decode the JSON object to ensure the payload is an object and not an array,
	// string, or other top-level JSON value.
	var payload map[string]json.RawMessage

	decoder := json.NewDecoder(bytes.NewReader(input.Payload))

	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("%w: invalid JSON object", ErrInvalidPayload)
	}

	// A nil map indicates an empty or null payload, which is invalid for a job.
	if payload == nil {
		return ErrInvalidPayload
	}

	return nil
}

// Status represents the current lifecycle state of a job.
type Status string

const (
	// StatusPending means the job has been created and is waiting to run.
	StatusPending Status = "pending"
	// StatusRunning means the job is currently being processed.
	StatusRunning Status = "running"
	// StatusRetrying means the job failed and is waiting to be retried.
	StatusRetrying Status = "retrying"
	// StatusSucceeded means the job finished successfully.
	StatusSucceeded Status = "succeeded"
	// StatusFailed means the job has failed permanently or exhausted retries.
	StatusFailed Status = "failed"
	// StatusDead means the job is considered inactive or terminally blocked.
	StatusDead Status = "dead"
	// StatusCancelled means the job was explicitly cancelled.
	StatusCancelled Status = "cancelled"
)

// Job is the persisted record for a background task.
type Job struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"-"`
	Type        string    `json:"type"`
	Payload     []byte    `json:"-"`
	Status      Status    `json:"status"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

// CreateInput contains the data required to submit a new job.
type CreateInput struct {
	TenantID string `json:"-"`
	Type     string `json:"type"`
	Payload  []byte `json:"-"`
}

var (
	// ErrInvalidJobType is returned when the requested job type is not allowed.
	ErrInvalidJobType = errors.New("invalid job type")
	// ErrInvalidPayload is returned when the job payload is missing or malformed.
	ErrInvalidPayload = errors.New("invalid job payload")
	// ErrJobNotFound is returned when a job cannot be located for the tenant.
	ErrJobNotFound = errors.New("job not found")
	// ErrConflict indicates the job is in a state that blocks the requested action.
	ErrConflict = errors.New("job state conflict")
	// ErrUnavailable indicates the background job service is temporarily unavailable.
	ErrUnavailable = errors.New("job service unavailable")
)

// ErrUnauthenticated indicates the caller is missing a valid tenant identity.
var ErrUnauthenticated = errors.New("unauthenticated")

// Repository defines the persistence contract for job records.
type Repository interface {
	// Create stores a newly submitted job.
	Create(ctx context.Context, job Job) error
	// GetByID loads a job by its id and tenant owner.
	GetByID(ctx context.Context, id, tenantID string) (Job, error)
}

// Service defines the application-level behavior for job submission and lookup.
type Service interface {
	// Submit creates and persists a job.
	Submit(ctx context.Context, input CreateInput) (Job, error)
	// GetByID retrieves a job for a tenant.
	GetByID(ctx context.Context, id, tenantID string) (Job, error)
}

// service is the concrete implementation of the job service.
type service struct {
	repo        Repository
	maxAttempts int
	now         func() time.Time
	newID       func() (string, error)
}

// NewService constructs a service with the default settings and dependencies.
func NewService(repo Repository) Service {
	return &service{
		repo:        repo,
		maxAttempts: 3,
		now:         time.Now,
		newID:       generateID,
	}
}

// Submit validates, creates, and persists a new job record.
func (s *service) Submit(
	ctx context.Context,
	input CreateInput,
) (Job, error) {
	// Validate the job type and payload before generating or storing anything.
	if err := ValidateInput(input); err != nil {
		return Job{}, err
	}

	// Generate a unique job ID so each record can be identified independently.
	id, err := s.newID()
	if err != nil {
		return Job{}, fmt.Errorf("generate job ID: %w", err)
	}

	// Use the current UTC time as the creation and scheduling reference.
	now := s.now().UTC()

	// Build the persisted job model with the default pending state.
	job := Job{
		ID:          id,
		TenantID:    input.TenantID,
		Type:        input.Type,
		Payload:     append([]byte(nil), input.Payload...),
		Status:      StatusPending,
		Attempts:    0,
		MaxAttempts: s.maxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
		ScheduledAt: now,
	}

	// Tenant ownership is required for all job submissions.
	if strings.TrimSpace(input.TenantID) == "" {
		return Job{}, ErrUnauthenticated
	}

	// Persist the job before returning it to the caller.
	if err := s.repo.Create(ctx, job); err != nil {
		return Job{}, fmt.Errorf("persist job: %w", err)
	}

	return job, nil
}

// GetByID retrieves a job by identifier while enforcing tenant ownership.
func (s *service) GetByID(
	ctx context.Context,
	id string,
	tenantID string,
) (Job, error) {
	// Empty identifiers are not valid, so reject them immediately.
	if id == "" || tenantID == "" {
		return Job{}, ErrJobNotFound
	}

	// Fetch the record from the repository using the tenant-scoped identity.
	job, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return Job{}, err
	}
	// Double-check ownership to prevent cross-tenant data leakage.
	if job.TenantID != tenantID {
		return Job{}, ErrJobNotFound
	}

	return job, nil
}

// generateID creates a random UUID-like identifier suitable for job records.
func generateID() (string, error) {
	var id [16]byte

	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}

	// Set UUID version 4 and variant bits.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(id[:])

	return encoded[:8] + "-" +
		encoded[8:12] + "-" +
		encoded[12:16] + "-" +
		encoded[16:20] + "-" +
		encoded[20:], nil
}
