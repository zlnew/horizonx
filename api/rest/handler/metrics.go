// Package handler
package handler

import (
	"encoding/json"
	"net/http"

	"horizonx-server/internal/storage/snapshot"
)

type MetricsHandler struct {
	ms *snapshot.MetricsStore
}

func NewMetricsHandler(ms *snapshot.MetricsStore) *MetricsHandler {
	return &MetricsHandler{ms: ms}
}

func (h *MetricsHandler) Get(w http.ResponseWriter, r *http.Request) {
	data := h.ms.Get()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
