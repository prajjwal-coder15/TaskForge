package repository

import (
	"context"
	"testing"

	"taskforge/internal/jobs"
)

func TestCreateRejectsInvalidUUID(t *testing.T) {
	repo := &PostgresJobRepository{}

	err := repo.Create(context.Background(), jobs.Job{
		ID:          "not-a-uuid",
		TenantID:    "tenant-1",
		Type:        "email",
		Payload:     []byte(`{"recipient":"user@example.com"}`),
		Status:      jobs.StatusPending,
		Attempts:    0,
		MaxAttempts: 3,
	})

	if err == nil {
		t.Fatal("expected invalid UUID error")
	}
}
