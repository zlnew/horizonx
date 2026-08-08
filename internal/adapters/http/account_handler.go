package http

import (
	"errors"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

// SessionDTO is the HTTP projection of a session. Deliberately NOT the
// domain type: domain.Session has no JSON tags because it's persisted to
// Redis with json.Marshal, and adding tags would corrupt every stored
// session on deploy (mass logout). DTO lives in the HTTP layer only.
type SessionDTO struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IsCurrent bool      `json:"is_current"`
}

func toSessionDTO(sess *domain.Session, currentID string) SessionDTO {
	return SessionDTO{
		ID:        sess.ID,
		IP:        sess.IP,
		UserAgent: sess.UserAgent,
		CreatedAt: sess.CreatedAt,
		ExpiresAt: sess.ExpiresAt,
		IsCurrent: sess.ID == currentID,
	}
}

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

func (h *AccountHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.svc.ListSessions(r.Context())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			h.writer.Write(w, http.StatusUnauthorized, &response.Response{
				Message: "unauthorized",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list sessions",
		})
		return
	}

	userCtx, _ := domain.GetUserContext(r.Context())
	dtos := make([]SessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		dtos = append(dtos, toSessionDTO(sess, userCtx.SessionID))
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: dtos,
	})
}

func (h *AccountHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid session id",
		})
		return
	}

	if err := h.svc.RevokeSession(r.Context(), sessionID); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "session not found",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to revoke session",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "session revoked successfully",
	})
}

func (h *AccountHandler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeOtherSessions(r.Context()); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to revoke other sessions",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "other sessions revoked successfully",
	})
}
