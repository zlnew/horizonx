package http

import (
	"errors"
	"net/http"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

type AccountHandler struct {
	svc domain.AccountService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewAccountHandler(
	svc domain.AccountService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *AccountHandler {
	return &AccountHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *AccountHandler) Profile(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.AccountProfileRequest
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

	if err := h.svc.UpdateProfile(r.Context(), req); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update profile",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "profile updated successfully",
	})
}

func (h *AccountHandler) Password(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.AccountPasswordRequest
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

	if err := h.svc.ChangePassword(r.Context(), req); err != nil {
		if errors.Is(err, domain.ErrInvalidCurrentPassword) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: err.Error(),
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to change password",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "password changed successfully",
	})
}
