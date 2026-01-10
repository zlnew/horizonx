package http

import (
	"net/http"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

type LogHandler struct {
	svc domain.LogService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewLogHandler(
	svc domain.LogService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *LogHandler {
	return &LogHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *LogHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.LogListOptions{
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
		Levels:        GetStringSlice(q, "levels"),
		Sources:       GetStringSlice(q, "sources"),
		Actions:       GetStringSlice(q, "actions"),
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list logs",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *LogHandler) Store(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.LogEmitRequest
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

	l := &domain.Log{
		Timestamp:     req.Timestamp,
		Level:         req.Level,
		Source:        req.Source,
		Action:        req.Action,
		TraceID:       req.TraceID,
		JobID:         req.JobID,
		ServerID:      req.ServerID,
		ApplicationID: req.ApplicationID,
		DeploymentID:  req.DeploymentID,
		Message:       req.Message,
		Context:       req.Context,
	}
	if _, err := h.svc.Create(r.Context(), l); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to create log",
		})
		return
	}

	h.writer.Write(w, http.StatusCreated, &response.Response{
		Message: "log created successfully",
	})
}
