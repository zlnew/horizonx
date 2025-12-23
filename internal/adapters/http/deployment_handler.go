package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"horizonx-server/internal/domain"
)

type DeploymentHandler struct {
	svc domain.DeploymentService
}

func NewDeploymentHandler(svc domain.DeploymentService) *DeploymentHandler {
	return &DeploymentHandler{
		svc: svc,
	}
}

func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.PathValue("id")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
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

func (h *DeploymentHandler) UpdateCommitInfo(w http.ResponseWriter, r *http.Request) {
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := strconv.ParseInt(deploymentIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid deployment id")
		return
	}

	var req domain.DeploymentCommitInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.svc.GetByID(r.Context(), deploymentID); err != nil {
		JSONError(w, http.StatusNotFound, "deployment not found")
		return
	}

	if err := h.svc.UpdateCommitInfo(r.Context(), deploymentID, req.CommitHash, req.CommitMessage); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to update commit info")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Commit Info updated",
	})
}

func (h *DeploymentHandler) UpdateLogs(w http.ResponseWriter, r *http.Request) {
	deploymentIDStr := r.PathValue("id")
	deploymentID, err := strconv.ParseInt(deploymentIDStr, 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid deployment id")
		return
	}

	var req domain.DeploymentLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.svc.GetByID(r.Context(), deploymentID); err != nil {
		JSONError(w, http.StatusNotFound, "deployment not found")
		return
	}

	if err := h.svc.UpdateLogs(r.Context(), deploymentID, req.Logs, req.IsPartial); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to update logs")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Logs updated",
	})
}
