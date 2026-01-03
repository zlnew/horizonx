package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/domain"

	"github.com/google/uuid"
)

type ApplicationHandler struct {
	svc domain.ApplicationService
}

func NewApplicationHandler(svc domain.ApplicationService) *ApplicationHandler {
	return &ApplicationHandler{svc: svc}
}

func (h *ApplicationHandler) Index(w http.ResponseWriter, r *http.Request) {
	serverIDStr := r.URL.Query().Get("server_id")
	if serverIDStr == "" {
		JSONError(w, http.StatusBadRequest, "server_id query parameter is required")
		return
	}

	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid server_id")
		return
	}

	q := r.URL.Query()

	opts := domain.ApplicationListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "lmit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		ServerID: &serverID,
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to list applications")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *ApplicationHandler) Show(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	app, err := h.svc.GetByID(r.Context(), appID)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to get application")
		return
	}

	envVars, err := h.svc.ListEnvVars(r.Context(), appID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to get applications environment variables")
		return
	}

	app.EnvVars = &envVars

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    app,
	})
}

func (h *ApplicationHandler) Store(w http.ResponseWriter, r *http.Request) {
	var req domain.ApplicationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	app, err := h.svc.Create(r.Context(), req)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to create application")
		return
	}

	JSONSuccess(w, http.StatusCreated, APIResponse{
		Message: "Application created successfully",
		Data:    app,
	})
}

func (h *ApplicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	var req domain.ApplicationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.Update(r.Context(), req, appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to update application")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Application updated successfully",
	})
}

func (h *ApplicationHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	if err := h.svc.Delete(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Application deleted successfully",
	})
}

func (h *ApplicationHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deployment, err := h.svc.Deploy(r.Context(), appID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Deployment started",
		Data:    deployment,
	})
}

func (h *ApplicationHandler) Start(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	if err := h.svc.Start(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Starting application",
	})
}

func (h *ApplicationHandler) Stop(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	if err := h.svc.Stop(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Stopping application",
	})
}

func (h *ApplicationHandler) Restart(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	if err := h.svc.Restart(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Restarting application",
	})
}

func (h *ApplicationHandler) AddEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	var req domain.EnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.AddEnvVar(r.Context(), appID, req); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to add environment variable")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Environment variable added",
	})
}

func (h *ApplicationHandler) UpdateEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		JSONError(w, http.StatusBadRequest, "key is required")
		return
	}

	var req domain.EnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.UpdateEnvVar(r.Context(), appID, key, req); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to update environment variable")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Environment variable updated",
	})
}

func (h *ApplicationHandler) DeleteEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		JSONError(w, http.StatusBadRequest, "key is required")
		return
	}

	if err := h.svc.DeleteEnvVar(r.Context(), appID, key); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			JSONError(w, http.StatusNotFound, "application not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "failed to delete environment variable")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Environment variable deleted",
	})
}

func (h *ApplicationHandler) ReportHealth(w http.ResponseWriter, r *http.Request) {
	serverID, valid := middleware.GetServerID(r.Context())
	if !valid {
		JSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	var req []domain.ApplicationHealth
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.UpdateHealth(r.Context(), serverID, req); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to update application health")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Applications Health Reported",
	})
}
