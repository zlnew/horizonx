package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"horizonx-server/internal/domain"

	"github.com/google/uuid"
)

type ServerHandler struct {
	svc domain.ServerService
}

func NewServerHandler(svc domain.ServerService) *ServerHandler {
	return &ServerHandler{svc: svc}
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
		JSONError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *ServerHandler) Store(w http.ResponseWriter, r *http.Request) {
	var req domain.ServerSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	srv, token, err := h.svc.Register(r.Context(), req)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusCreated, APIResponse{
		Message: "Server registered successfully",
		Data: map[string]any{
			"server": srv,
			"token":  token,
		},
	})
}

func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	var req domain.ServerSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	serverID, err := uuid.Parse(paramID)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid server ID")
	}

	if err := h.svc.Update(r.Context(), req, serverID); err != nil {
		if errors.Is(err, domain.ErrServerNotFound) {
			JSONError(w, http.StatusNotFound, "Server not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Server updated successfully",
	})
}

func (h *ServerHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	paramID := r.PathValue("id")

	serverID, err := uuid.Parse(paramID)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid server ID")
		return
	}

	if err := h.svc.Delete(r.Context(), serverID); err != nil {
		if errors.Is(err, domain.ErrServerNotFound) {
			JSONError(w, http.StatusNotFound, "Server not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Server deleted successfully",
	})
}
