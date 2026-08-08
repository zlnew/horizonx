package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/adapters/ws/agentws"
	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type ServerHandler struct {
	svc domain.ServerService

	agentRouter *agentws.Router

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewServerHandler(
	svc domain.ServerService,
	agentRouter *agentws.Router,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *ServerHandler {
	return &ServerHandler{
		svc:         svc,
		agentRouter: agentRouter,
		decoder:     d,
		writer:      w,
		validator:   v,
	}
}

func (h *ServerHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.ServerListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		IsOnline: GetBoolPtr(q, "is_online"),
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list servers",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *ServerHandler) Store(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.ServerSaveRequest
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

	srv, token, err := h.svc.Register(r.Context(), req)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to register server",
		})
		return
	}

	h.writer.Write(w, http.StatusCreated, &response.Response{
		Message: "server registered successfully",
		Data: &domain.ServerRegisteredResponse{
			Server: *srv,
			Token:  token,
		},
	})
}

func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	paramID := r.PathValue("id")

	var req domain.ServerSaveRequest
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

	serverID, err := uuid.Parse(paramID)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid server ID",
		})
		return
	}

	if err := h.svc.Update(r.Context(), req, serverID); err != nil {
		if errors.Is(err, domain.ErrServerNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "server not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update server",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "server updated successfully",
	})
}

func (h *ServerHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	serverID, err := uuid.Parse(paramID)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid server ID",
		})
		return
	}

	if err := h.svc.Delete(r.Context(), serverID); err != nil {
		if errors.Is(err, domain.ErrServerNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "server not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to delete server",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "server deleted successfully",
	})
}

// RotateSecret issues a new agent token. The raw token is returned ONCE —
// display it with a copy button and a "shown once" warning; the old token
// stops working immediately.
func (h *ServerHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	serverID, err := uuid.Parse(paramID)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid server ID",
		})
		return
	}

	token, err := h.svc.RotateSecret(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, domain.ErrServerNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "server not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to rotate server secret",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: map[string]any{
			"token":     token,
			"shownOnce": true,
		},
	})
}

// Ping is the A1 protocol proof: it sends a ping command through the agent
// WS router to the server's live agent. 202 means the command was handed to
// the agent's socket; 409 (ErrAgentOffline) means no live connection.
func (h *ServerHandler) Ping(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")
	serverID, err := uuid.Parse(paramID)
	if err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid server ID",
		})
		return
	}

	if h.agentRouter == nil {
		h.writer.Write(w, http.StatusServiceUnavailable, &response.Response{
			Message: "agent router unavailable",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err = h.agentRouter.SendCommand(ctx, serverID, domain.AgentCommand{
		Command: "ping",
	})
	if err != nil {
		if errors.Is(err, domain.ErrAgentOffline) {
			h.writer.Write(w, http.StatusConflict, &response.Response{
				Message: "agent offline",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to send ping",
		})
		return
	}

	h.writer.Write(w, http.StatusAccepted, &response.Response{
		Message: "ping sent",
	})
}
