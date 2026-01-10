package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"horizonx/internal/domain"
)

type AccountHandler struct {
	svc domain.AccountService
}

func NewAccountHandler(svc domain.AccountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

func (h *AccountHandler) Profile(w http.ResponseWriter, r *http.Request) {
	var req domain.AccountProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.UpdateProfile(r.Context(), req); err != nil {
		JSONError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Profile updated successfully",
	})
}

func (h *AccountHandler) Password(w http.ResponseWriter, r *http.Request) {
	var req domain.AccountPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	if err := h.svc.ChangePassword(r.Context(), req); err != nil {
		if errors.Is(err, domain.ErrInvalidCurrentPassword) {
			JSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		JSONError(w, http.StatusInternalServerError, "failed to change password")
		return
	}

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "Password changed successfully",
	})
}
