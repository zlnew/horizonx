package http

import (
	"encoding/json"
	"net/http"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type MetricsHandler struct {
	svc domain.MetricsService
	log logger.Logger
}

func NewMetricsHandler(svc domain.MetricsService, log logger.Logger) *MetricsHandler {
	return &MetricsHandler{
		svc: svc,
		log: log,
	}
}

func (h *MetricsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var payload domain.MetricsPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.log.Warn("invalid metrics payload", "error", err)
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(payload.Metrics) == 0 {
		h.log.Warn("empty metrics batch received")
		JSONError(w, http.StatusBadRequest, "No metrics provided")
		return
	}

	h.log.Info("received metrics batch", "count", len(payload.Metrics))

	for i := range payload.Metrics {
		if err := h.svc.Ingest(payload.Metrics[i]); err != nil {
			h.log.Error("failed to ingest metric", "error", err, "index", i)
			JSONError(w, http.StatusInternalServerError, "Failed to process metrics")
			return
		}
	}

	h.log.Debug("metrics ingested successfully", "count", len(payload.Metrics))

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Metrics received",
		Data: map[string]any{
			"count": len(payload.Metrics),
		},
	})
}
