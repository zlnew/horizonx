package http

import (
	"errors"
	"net/http"
	"strconv"

	"horizonx/internal/adapters/http/middleware"
	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

type JobHandler struct {
	svc domain.JobService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewJobHandler(
	svc domain.JobService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *JobHandler {
	return &JobHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
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
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list jobs",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *JobHandler) Pending(w http.ResponseWriter, r *http.Request) {
	serverID, ok := middleware.GetServerID(r.Context())
	if !ok {
		h.writer.Write(w, http.StatusUnauthorized, &response.Response{
			Message: "invalid credentials",
		})
		return
	}

	jobs, err := h.svc.GetPending(r.Context(), serverID)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get pending jobs",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: jobs,
	})
}

func (h *JobHandler) Show(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid job id",
		})
		return
	}

	job, err := h.svc.GetByID(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "job not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get job",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: job,
	})
}

func (h *JobHandler) Start(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	jobID, err := strconv.ParseInt(paramID, 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid job id",
		})
		return
	}

	job, err := h.svc.Start(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "job not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to start job",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: job,
	})
}

func (h *JobHandler) Finish(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	paramID := r.PathValue("id")

	var req domain.JobFinishRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	jobID, err := strconv.ParseInt(paramID, 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid job id",
		})
		return
	}

	job, err := h.svc.Finish(r.Context(), jobID, req.Status)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "job not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to finish job",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: job,
	})
}
