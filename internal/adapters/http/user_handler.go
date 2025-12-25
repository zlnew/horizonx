package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/domain"
)

type UserHandler struct {
	svc domain.UserService
}

func NewUserHandler(svc domain.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.UserListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		Roles: GetStringSlice(q, "roles"),
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    result.Data,
		Meta:    result.Meta,
	})
}

func (h *UserHandler) Store(w http.ResponseWriter, r *http.Request) {
	var req domain.UserSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.Create(r.Context(), req); err != nil {
		if errors.Is(err, domain.ErrRoleNotFound) {
			JSONError(w, http.StatusBadRequest, "role not found")
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			JSONError(w, http.StatusBadRequest, "email already registered")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User created successfully",
	})
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "invalid user id")
		return
	}

	var req domain.UserSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.Update(r.Context(), req, userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			JSONError(w, http.StatusNotFound, "user not found")
			return
		}

		if errors.Is(err, domain.ErrRoleNotFound) {
			JSONError(w, http.StatusBadRequest, "role not found")
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			JSONError(w, http.StatusBadRequest, "email already registered")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User updated successfully",
	})
}

func (h *UserHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "invalid user id")
		return
	}

	currentUserID, ok := middleware.GetUserID(r.Context())
	if !ok {
		JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if userID == currentUserID {
		JSONError(w, http.StatusBadRequest, "you cannot delete yourself")
		return
	}

	if err := h.svc.Delete(r.Context(), userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			JSONError(w, http.StatusNotFound, "user not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User deleted successfully",
	})
}
