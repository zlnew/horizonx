package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/domain"

	"github.com/google/uuid"
)

type MetricsHandler struct {
	svc domain.MetricsService
}

func NewMetricsHandler(svc domain.MetricsService) *MetricsHandler {
	return &MetricsHandler{
		svc: svc,
	}
}

func (h *MetricsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	serverID, ok := middleware.GetServerID(r.Context())
	if !ok {
		JSONError(w, http.StatusUnauthorized, "invalid credentials")
	}

	var metrics domain.Metrics

	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.Ingest(serverID, metrics); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to process metrics")
		return
	}

	JSONSuccess(w, http.StatusCreated, APIResponse{
		Message: "Metrics received",
		Data:    metrics,
	})
}

func (h *MetricsHandler) Latest(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		JSONError(w, http.StatusNotFound, "server not found")
		return
	}

	metrics, err := h.svc.Latest(serverID)
	if err != nil {
		if errors.Is(err, domain.ErrMetricsNotFound) {
			JSONError(w, http.StatusNotFound, "metrics not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to get latest metrics")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    metrics,
	})
}
