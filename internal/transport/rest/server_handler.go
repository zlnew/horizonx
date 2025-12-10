package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"horizonx-server/internal/domain"
)

type ServerHandler struct {
	svc domain.ServerService
}

func NewServerHandler(svc domain.ServerService) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) Index(w http.ResponseWriter, r *http.Request) {
	servers, err := h.svc.Get(r.Context())
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    servers,
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
	serverID := r.PathValue("id")

	var req domain.ServerSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	parsedServerID, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if err := h.svc.Update(r.Context(), req, parsedServerID); err != nil {
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
	serverID := r.PathValue("id")

	parsedServerID, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if err := h.svc.Delete(r.Context(), parsedServerID); err != nil {
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
