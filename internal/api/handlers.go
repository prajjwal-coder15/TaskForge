package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"taskforge/internal/jobs"
)

const maxRequestBody = 1 << 20 // 1 MiB

// CreateJobRequest is the JSON body accepted by the create-job endpoint.
// Tenant identity is intentionally not accepted from the client.
type CreateJobRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) createJob(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType,
			"UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return
	}

	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized,
			"UNAUTHENTICATED", "Authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req CreateJobRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest,
			"INVALID_JSON", "Request must contain one JSON object")
		return
	}

	if strings.TrimSpace(req.Type) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_TYPE", "Job type is required")
		return
	}
	if len(req.Payload) == 0 || !json.Valid(req.Payload) {
		writeError(w, http.StatusBadRequest,
			"INVALID_PAYLOAD", "Payload must be valid JSON")
		return
	}

	job, err := h.jobs.Submit(r.Context(), jobs.CreateInput{
		TenantID: principal.TenantID,
		Type:     req.Type,
		Payload:  append([]byte(nil), req.Payload...),
	})
	if err != nil {
		h.writeJobError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/jobs/"+job.ID)
	writeJSON(w, http.StatusCreated, toJobResponse(job))
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized,
			"UNAUTHENTICATED", "Authentication required")
		return
	}

	job, err := h.jobs.GetByID(r.Context(), r.PathValue("id"), principal.TenantID)
	if err != nil {
		h.writeJobError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toJobResponse(job))
}

func (h *Handler) writeJobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jobs.ErrInvalidJobType):
		writeError(w, http.StatusUnprocessableEntity,
			"INVALID_JOB_TYPE", "Unsupported job type")
	case errors.Is(err, jobs.ErrInvalidPayload):
		writeError(w, http.StatusUnprocessableEntity,
			"INVALID_PAYLOAD", "Job payload is invalid")
	case errors.Is(err, jobs.ErrJobNotFound):
		writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found")
	case errors.Is(err, jobs.ErrConflict):
		writeError(w, http.StatusConflict,
			"JOB_CONFLICT", "Job conflicts with an existing resource")
	case errors.Is(err, jobs.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE", "Job service is unavailable")
	default:
		// Do not log request bodies, credentials, or job payloads.
		slog.Error("job request failed", "error", err)
		writeError(w, http.StatusInternalServerError,
			"INTERNAL_ERROR", "Internal server error")
	}
}
