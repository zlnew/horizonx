package http

import (
	"errors"
	"net/http"
	"strconv"

	"horizonx/internal/adapters/http/middleware"
	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type ApplicationHandler struct {
	svc domain.ApplicationService

	// logs is the container-log entry point (A2). Kept separate so the
	// handler depends on the narrow interface, not a fat service.
	logs domain.ContainerLogService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewApplicationHandler(
	svc domain.ApplicationService,
	logs domain.ContainerLogService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *ApplicationHandler {
	return &ApplicationHandler{
		svc:       svc,
		logs:      logs,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *ApplicationHandler) Index(w http.ResponseWriter, r *http.Request) {
	serverIDStr := r.URL.Query().Get("server_id")
	if serverIDStr == "" {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "server_id query parameter is required",
		})
		return
	}

	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid server_id",
		})
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
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list applications",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *ApplicationHandler) Show(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	app, err := h.svc.GetByID(r.Context(), appID)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get application",
		})
		return
	}

	envVars, err := h.svc.ListEnvVars(r.Context(), appID)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get applications environment variables",
		})
		return
	}

	app.EnvVars = &envVars

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: app,
	})
}

func (h *ApplicationHandler) Store(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.ApplicationCreateRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	app, err := h.svc.Create(r.Context(), req)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to create application",
		})
		return
	}

	h.writer.Write(w, http.StatusCreated, &response.Response{
		Message: "application created successfully",
		Data:    app,
	})
}

func (h *ApplicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	var req domain.ApplicationUpdateRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	if err := h.svc.Update(r.Context(), req, appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update application",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "application updated successfully",
	})
}

func (h *ApplicationHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	if err := h.svc.Delete(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "application deleted successfully",
	})
}

func (h *ApplicationHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := domain.GetUserContext(r.Context())
	if !ok {
		h.writer.Write(w, http.StatusUnauthorized, &response.Response{
			Message: "unauthorized",
		})
		return
	}

	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	deployment, err := h.svc.Deploy(r.Context(), appID, userCtx.ID)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "deployment started",
		Data:    deployment,
	})
}

// Rollback re-deploys the last successfully built image for an app (P0-4).
func (h *ApplicationHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := domain.GetUserContext(r.Context())
	if !ok {
		h.writer.Write(w, http.StatusUnauthorized, &response.Response{
			Message: "unauthorized",
		})
		return
	}

	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	deployment, err := h.svc.Rollback(r.Context(), appID, userCtx.ID)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "rollback started",
		Data:    deployment,
	})
}

func (h *ApplicationHandler) Start(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	if err := h.svc.Start(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "starting application",
	})
}

// TailLogs starts a live container-log tail. Returns 202 immediately with
// the stream ID; log chunks arrive over the userws channel app_logs:{id}.
func (h *ApplicationHandler) TailLogs(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid application id"})
		return
	}

	var req domain.LogsTailRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid request body"})
		return
	}
	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.Write(w, http.StatusUnprocessableEntity, &response.Response{Message: errs["error"]})
		return
	}

	streamID, err := h.logs.TailLogs(r.Context(), appID, req)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{Message: "application not found"})
			return
		}
		if errors.Is(err, domain.ErrAgentOffline) {
			h.writer.Write(w, http.StatusConflict, &response.Response{Message: "agent offline — cannot start log tail"})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: err.Error()})
		return
	}

	h.writer.Write(w, http.StatusAccepted, &response.Response{
		Message: "log tail started",
		Data:    map[string]any{"stream_id": streamID},
	})
}

// StopTailLogs cancels a running tail stream.
func (h *ApplicationHandler) StopTailLogs(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid application id"})
		return
	}

	var req domain.LogsTailStopRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid request body"})
		return
	}
	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.Write(w, http.StatusUnprocessableEntity, &response.Response{Message: errs["error"]})
		return
	}

	if err := h.logs.StopTailLogs(r.Context(), appID, req.StreamID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{Message: "application not found"})
			return
		}
		if errors.Is(err, domain.ErrAgentOffline) {
			// Nothing to stop if the agent is gone — the process died with it.
			h.writer.Write(w, http.StatusAccepted, &response.Response{Message: "agent offline — nothing to stop"})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: err.Error()})
		return
	}

	h.writer.Write(w, http.StatusAccepted, &response.Response{
		Message: "log tail stopped",
	})
}

// QueryLogs runs a one-shot historical query. Returns 202 immediately with
// the query ID; chunks (ending EOF:true) arrive over app_logs:{id}.
func (h *ApplicationHandler) QueryLogs(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid application id"})
		return
	}

	var req domain.LogsQueryRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: "invalid request body"})
		return
	}
	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.Write(w, http.StatusUnprocessableEntity, &response.Response{Message: errs["error"]})
		return
	}

	queryID, err := h.logs.QueryLogs(r.Context(), appID, req)
	if err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{Message: "application not found"})
			return
		}
		if errors.Is(err, domain.ErrAgentOffline) {
			h.writer.Write(w, http.StatusConflict, &response.Response{Message: "agent offline — cannot query logs"})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{Message: err.Error()})
		return
	}

	h.writer.Write(w, http.StatusAccepted, &response.Response{
		Message: "log query started",
		Data:    map[string]any{"query_id": queryID},
	})
}

func (h *ApplicationHandler) Stop(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	if err := h.svc.Stop(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "stopping application",
	})
}

func (h *ApplicationHandler) Restart(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	if err := h.svc.Restart(r.Context(), appID); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "restarting application",
	})
}

func (h *ApplicationHandler) AddEnvVar(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	var req domain.EnvironmentVariableRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	if err := h.svc.AddEnvVar(r.Context(), appID, req); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to add environment variable",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "environment variable added",
	})
}

func (h *ApplicationHandler) UpdateEnvVar(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	key := r.PathValue("key")
	if key == "" {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "key is required",
		})
		return
	}

	var req domain.EnvironmentVariableRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	if err := h.svc.UpdateEnvVar(r.Context(), appID, key, req); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update environment variable",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "environment variable updated",
	})
}

func (h *ApplicationHandler) DeleteEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid application id",
		})
		return
	}

	key := r.PathValue("key")
	if key == "" {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "key is required",
		})
		return
	}

	if err := h.svc.DeleteEnvVar(r.Context(), appID, key); err != nil {
		if errors.Is(err, domain.ErrApplicationNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "application not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to delete environment variable",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "environment variable deleted",
	})
}

func (h *ApplicationHandler) ReportHealth(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	serverID, valid := middleware.GetServerID(r.Context())
	if !valid {
		h.writer.Write(w, http.StatusUnauthorized, &response.Response{
			Message: "invalid credentials",
		})
		return
	}

	var req []domain.ApplicationHealth
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: err.Error(),
		})
		return
	}

	if err := h.svc.UpdateHealth(r.Context(), serverID, req); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update application health",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "applications health reported",
	})
}
