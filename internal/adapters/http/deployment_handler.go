package http

import (
	"errors"
	"net/http"
	"strconv"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

type DeploymentHandler struct {
	svc domain.DeploymentService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewDeploymentHandler(
	svc domain.DeploymentService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *DeploymentHandler {
	return &DeploymentHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *DeploymentHandler) Index(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
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
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list deployments",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *DeploymentHandler) Show(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	deploymentID, err := strconv.ParseInt(r.PathValue("deployment_id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid deployment id",
		})
		return
	}

	deployment, err := h.svc.GetByID(r.Context(), deploymentID)
	if err != nil {
		if errors.Is(err, domain.ErrDeploymentNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "deployment not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get deployment",
		})
		return
	}

	if deployment.ApplicationID != appID {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: deployment,
	})
}

func (h *DeploymentHandler) UpdateCommitInfo(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	deploymentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid deployment id",
		})
		return
	}

	var req domain.DeploymentCommitInfoRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if err := h.svc.UpdateCommitInfo(r.Context(), deploymentID, req.CommitHash, req.CommitMessage); err != nil {
		if errors.Is(err, domain.ErrDeploymentNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "deployment not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update commit info",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "commit info updated",
	})
}
