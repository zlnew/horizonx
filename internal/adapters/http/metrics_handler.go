package http

import (
	"errors"
	"net/http"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type MetricsHandler struct {
	svc domain.MetricsService

	registry MetricsRegistry

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

// MetricsRegistry is the subset of the Prometheus registry the metrics
// handler needs. Satisfied by *metrics.Registry; nil-safe in use.
type MetricsRegistry interface {
	SetAgentDroppedMessages(serverID string, v float64)
}

func NewMetricsHandler(
	svc domain.MetricsService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
	reg MetricsRegistry,
) *MetricsHandler {
	return &MetricsHandler{
		svc:       svc,
		registry:  reg,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *MetricsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var metrics domain.Metrics

	if err := h.decoder.Decode(r, &metrics); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if err := h.svc.Ingest(r.Context(), metrics); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to process metrics",
		})
		return
	}

	if h.registry != nil {
		h.registry.SetAgentDroppedMessages(metrics.ServerID.String(), float64(metrics.DroppedMessages))
	}

	h.writer.Write(w, http.StatusCreated, &response.Response{
		Message: "metrics received",
		Data:    metrics,
	})
}

func (h *MetricsHandler) Latest(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.writer.Write(w, http.StatusNotFound, &response.Response{
			Message: "server not found",
		})
		return
	}

	metrics, err := h.svc.Latest(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, domain.ErrMetricsNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "metrics not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get latest metrics",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: metrics,
	})
}

func (h *MetricsHandler) CPUUsageHistory(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.writer.Write(w, http.StatusNotFound, &response.Response{
			Message: "server not found",
		})
		return
	}

	data, err := h.svc.CPUUsageHistory(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, domain.ErrMetricsNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "cpu usage history not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get cpu usage history",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: data,
	})
}

func (h *MetricsHandler) NetSpeedHistory(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.writer.Write(w, http.StatusNotFound, &response.Response{
			Message: "server not found",
		})
		return
	}

	data, err := h.svc.NetSpeedHistory(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, domain.ErrMetricsNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "net speed history not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get net speed history",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: data,
	})
}
