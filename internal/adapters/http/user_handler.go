package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	search := q.Get("search")
	isPaginate := q.Get("paginate") == "true"

	opts := domain.ListOptions{
		Page:       page,
		Limit:      limit,
		Search:     search,
		IsPaginate: isPaginate,
	}

	result, err := h.svc.List(r.Context(), opts)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
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
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.Create(r.Context(), req); err != nil {
		if errors.Is(err, domain.ErrRoleNotFound) {
			JSONError(w, http.StatusBadRequest, "Role not found")
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			JSONError(w, http.StatusBadRequest, "Email already registered")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User created successfully",
	})
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	var req domain.UserSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	parsedUserID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if err := h.svc.Update(r.Context(), req, parsedUserID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			JSONError(w, http.StatusNotFound, "User not found")
			return
		}

		if errors.Is(err, domain.ErrRoleNotFound) {
			JSONError(w, http.StatusBadRequest, "Role not found")
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			JSONError(w, http.StatusBadRequest, "Email already registered")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User updated successfully",
	})
}

func (h *UserHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")

	parsedUserID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if err := h.svc.Delete(r.Context(), parsedUserID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			JSONError(w, http.StatusNotFound, "User not found")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "User deleted successfully",
	})
}
