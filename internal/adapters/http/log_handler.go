package http

import (
	"encoding/json"
	"net/http"

	"horizonx-server/internal/domain"
)

type LogHandler struct {
	svc domain.LogService
}

func NewLogHandler(svc domain.LogService) *LogHandler {
	return &LogHandler{svc: svc}
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
		JSONError(w, http.StatusInternalServerError, "failed to list logs")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *LogHandler) Store(w http.ResponseWriter, r *http.Request) {
	var req domain.LogEmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
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
		JSONError(w, http.StatusInternalServerError, "failed to create log")
		return
	}

	JSONSuccess(w, http.StatusCreated, APIResponse{
		Message: "log created successfully",
	})
}
