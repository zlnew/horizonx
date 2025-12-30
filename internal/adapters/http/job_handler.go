package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/domain"
)

type JobHandler struct {
	svc domain.JobService
}

func NewJobHandler(svc domain.JobService) *JobHandler {
	return &JobHandler{svc: svc}
}

func (h *JobHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.JobListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		TraceID:       GetUUID(q, "trace_id"),
		ServerID:      GetUUID(q, "server_id"),
		ApplicationID: GetInt64(q, "application_id"),
		DeploymentID:  GetInt64(q, "deployment_id"),
		Type:          GetString(q, "job_type", ""),
		Statuses:      GetStringSlice(q, "statuses"),
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *JobHandler) Pending(w http.ResponseWriter, r *http.Request) {
	serverID, ok := middleware.GetServerID(r.Context())
	if !ok {
		JSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	jobs, err := h.svc.GetPending(r.Context(), serverID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to get pending jobs")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    jobs,
	})
}

func (h *JobHandler) Show(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.svc.GetByID(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			JSONError(w, http.StatusNotFound, "job not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    job,
	})
}

func (h *JobHandler) Start(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	jobID, err := strconv.ParseInt(paramID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.svc.Start(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			JSONError(w, http.StatusNotFound, "job not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to start job")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    job,
	})
}

func (h *JobHandler) Finish(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	var req domain.JobFinishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	jobID, err := strconv.ParseInt(paramID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.svc.Finish(r.Context(), jobID, req.Status)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			JSONError(w, http.StatusNotFound, "job not found")
			return
		}

		log.Println("asd", err.Error())

		JSONError(w, http.StatusInternalServerError, "failed to finish job")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    job,
	})
}
