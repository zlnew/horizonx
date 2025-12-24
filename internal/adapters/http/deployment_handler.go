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

func (h *DeploymentHandler) Index(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	q := r.URL.Query()

	opts := domain.DeploymentListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		ApplicationID: &appID,
		DeployedBy:    GetInt64(q, "deployed_by"),
		Statuses:      GetStringSlice(q, "statuses"),
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to list deployments")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *DeploymentHandler) Show(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	deploymentID, err := strconv.ParseInt(r.PathValue("deployment_id"), 10, 64)
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

	if deployment.ApplicationID != appID {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    deployment,
	})
}

func (h *DeploymentHandler) UpdateCommitInfo(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid deployment id")
		return
	}

	var req domain.DeploymentCommitInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.UpdateCommitInfo(r.Context(), deploymentID, req.CommitHash, req.CommitMessage); err != nil {
		if errors.Is(err, domain.ErrDeploymentNotFound) {
			JSONError(w, http.StatusNotFound, "deployment not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to update commit info")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Commit Info updated",
	})
}

func (h *DeploymentHandler) UpdateLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
