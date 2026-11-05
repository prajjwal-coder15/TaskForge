package api

import (
	"time"

	"taskforge/internal/jobs"
)

// JobResponse is the public representation of a job.
// Tenant identifiers and payload contents are deliberately omitted.
type JobResponse struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Status      jobs.Status `json:"status"`
	Attempts    int         `json:"attempts"`
	MaxAttempts int         `json:"max_attempts"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	ScheduledAt time.Time   `json:"scheduled_at"`
}

func toJobResponse(job jobs.Job) JobResponse {
	return JobResponse{
		ID:          job.ID,
		Type:        job.Type,
		Status:      job.Status,
		Attempts:    job.Attempts,
		MaxAttempts: job.MaxAttempts,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
		ScheduledAt: job.ScheduledAt,
	}
}
