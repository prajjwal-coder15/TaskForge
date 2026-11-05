package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	jobs map[string]Job
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		jobs: make(map[string]Job),
	}
}

func (f *fakeRepository) Create(
	ctx context.Context,
	job Job,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, exists := f.jobs[job.ID]; exists {
		return errors.New("duplicate job")
	}

	f.jobs[job.ID] = job
	return nil
}

func (f *fakeRepository) GetByID(
	ctx context.Context,
	id string,
	tenantID string,
) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}

	job, ok := f.jobs[id]
	if !ok || job.TenantID != tenantID {
		return Job{}, ErrJobNotFound
	}

	return job, nil
}

func TestSubmitPersistsJob(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	job, err := svc.Submit(
		context.Background(),
		CreateInput{
			TenantID: "test_tenant",
			Type:     "email",
			Payload:  []byte(`{"recipient":"user@example.com"}`),
		},
	)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if job.ID == "" {
		t.Fatal("expected generated job ID")
	}

	if job.Status != StatusPending {
		t.Fatalf("unexpected status: %s", job.Status)
	}

	if job.MaxAttempts != 3 {
		t.Fatalf("unexpected max attempts: %d", job.MaxAttempts)
	}

	stored, err := svc.GetByID(context.Background(), job.ID, job.TenantID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if stored.ID != job.ID {
		t.Fatalf("expected job %s, got %s", job.ID, stored.ID)
	}
}

func TestSubmitRejectsUnsupportedType(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.Submit(
		context.Background(),
		CreateInput{
			Type:    "shell.execute",
			Payload: []byte(`{}`),
		},
	)

	if !errors.Is(err, ErrInvalidJobType) {
		t.Fatalf("expected ErrInvalidJobType, got %v", err)
	}
}

func TestSubmitRespectsCanceledContext(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Submit(ctx, CreateInput{
		TenantID: "test_tenant",
		Type:     "email",
		Payload:  []byte(`{"recipient":"user@example.com"}`),
	})

	if err == nil {
		t.Fatal("expected canceled-context error")
	}
}

func TestJobTimestampIsUTC(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	svc.(*service).now = func() time.Time {
		return time.Date(
			2026, time.January, 1,
			12, 0, 0, 0,
			time.FixedZone("test", 5*60*60+30*60),
		)
	}

	job, err := svc.Submit(context.Background(), CreateInput{
		TenantID: "test_tenant",
		Type:     "email",
		Payload:  []byte(`{"recipient":"user@example.com"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if job.CreatedAt.Location() != time.UTC {
		t.Fatal("expected UTC timestamp")
	}
}
