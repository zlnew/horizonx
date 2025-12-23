package http

import (
	"encoding/json"
	"net/http"

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
	var payload domain.MetricsPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(payload.Metrics) == 0 {
		JSONError(w, http.StatusBadRequest, "No metrics provided")
		return
	}

	for i := range payload.Metrics {
		if err := h.svc.Ingest(payload.Metrics[i]); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to process metrics")
			return
		}
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Metrics received",
		Data: map[string]any{
			"count": len(payload.Metrics),
		},
	})
}

func (h *MetricsHandler) Latest(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		JSONError(w, http.StatusNotFound, "server not found")
		return
	}

	metrics, ok := h.svc.Latest(serverID)
	if !ok {
		JSONError(w, http.StatusNotFound, "no metrics yet")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    metrics,
	})
}
