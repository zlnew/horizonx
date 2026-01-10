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
)

type UserHandler struct {
	svc domain.UserService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewUserHandler(
	svc domain.UserService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *UserHandler {
	return &UserHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
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
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list users",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

func (h *UserHandler) Store(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.UserSaveRequest
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

	if err := h.svc.Create(r.Context(), req); err != nil {
		if errors.Is(err, domain.ErrRoleNotFound) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: "role not found",
			})
			return
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: "email already registered",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to create user",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "user created successfully",
	})
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "invalid user id",
		})
		return
	}

	var req domain.UserSaveRequest
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

	if err := h.svc.Update(r.Context(), req, userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "user not found",
			})
			return
		}

		if errors.Is(err, domain.ErrRoleNotFound) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: "role not found",
			})
			return
		}

		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: "email already registered",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to update user",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "user updated successfully",
	})
}

func (h *UserHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := middleware.GetUser(r.Context())
	if !ok {
		h.writer.Write(w, http.StatusUnauthorized, &response.Response{
			Message: "unauthorized",
		})
		return
	}

	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "invalid user id",
		})
		return
	}

	if userID == userCtx.ID {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "you cannot delete yourself",
		})
		return
	}

	if err := h.svc.Delete(r.Context(), userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "user not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to delete user",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "user deleted successfully",
	})
}
