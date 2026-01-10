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

type ServerHandler struct {
	svc domain.ServerService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewServerHandler(
	svc domain.ServerService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *ServerHandler {
	return &ServerHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
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
