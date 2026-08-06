package http

import (
	"net/http"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

type AuditLogHandler struct {
	svc domain.AuditLogService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewAuditLogHandler(
	svc domain.AuditLogService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *AuditLogHandler {
	return &AuditLogHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

// Index lists audit log entries, newest first.
func (h *AuditLogHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.AuditLogListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 20),
			IsPaginate: GetBool(q, "paginate"),
		},
		Action:       GetString(q, "action", ""),
		ResourceType: GetString(q, "resource_type", ""),
	}

	logs, err := h.svc.List(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list audit logs",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: logs.Data,
		Meta: logs.Meta,
	})
}
