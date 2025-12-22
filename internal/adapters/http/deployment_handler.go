package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

type DeploymentHandler struct {
	svc  domain.DeploymentService
	repo domain.DeploymentRepository
	bus  *event.Bus
	log  logger.Logger
}

func NewDeploymentHandler(svc domain.DeploymentService, repo domain.DeploymentRepository, bus *event.Bus, log logger.Logger) *DeploymentHandler {
	return &DeploymentHandler{
		svc:  svc,
		repo: repo,
		bus:  bus,
		log:  log,
	}
}

// List returns deployment history for an application
func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("id")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	deployments, err := h.svc.List(r.Context(), appID, limit)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to list deployments")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    deployments,
	})
}

// Show returns a single deployment with full logs
func (h *DeploymentHandler) Show(w http.ResponseWriter, r *http.Request) {
	deploymentIDStr := r.PathValue("deployment_id")
	deploymentID, err := strconv.ParseInt(deploymentIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid deployment id")
		return
	}

	deployment, err := h.svc.GetByID(r.Context(), deploymentID)
	if err != nil {
		if errors.Is(err, domain.ErrDeploymentNotFound) {
			JSONError(w, http.StatusNotFound, "deployment not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to get deployment")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    deployment,
	})
}

// GetLatest returns the latest deployment for an application
func (h *DeploymentHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("id")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	deployment, err := h.svc.GetLatest(r.Context(), appID)
	if err != nil {
		if errors.Is(err, domain.ErrDeploymentNotFound) {
			JSONError(w, http.StatusNotFound, "no deployments found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to get latest deployment")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    deployment,
	})
}

// UpdateLogs handles real-time log updates from agent (agent endpoint)
func (h *DeploymentHandler) UpdateLogs(w http.ResponseWriter, r *http.Request) {
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := strconv.ParseInt(deploymentIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid deployment id")
		return
	}

	var payload struct {
		Logs      string `json:"logs"`
		IsPartial bool   `json:"is_partial"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get deployment to get application_id
	deployment, err := h.repo.GetByID(r.Context(), deploymentID)
	if err != nil {
		JSONError(w, http.StatusNotFound, "deployment not found")
		return
	}

	// Update logs in database
	if err := h.repo.UpdateLogs(r.Context(), deploymentID, payload.Logs); err != nil {
		h.log.Error("failed to update deployment logs", "error", err)
		JSONError(w, http.StatusInternalServerError, "failed to update logs")
		return
	}

	// Broadcast logs via WebSocket
	if h.bus != nil {
		h.bus.Publish("deployment_logs_updated", domain.EventDeploymentLogsUpdated{
			DeploymentID:  deploymentID,
			ApplicationID: deployment.ApplicationID,
			Logs:          payload.Logs,
			IsPartial:     payload.IsPartial,
		})
	}

	h.log.Debug("deployment logs updated",
		"deployment_id", deploymentID,
		"app_id", deployment.ApplicationID,
		"is_partial", payload.IsPartial,
	)

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Logs updated",
	})
}
