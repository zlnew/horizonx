package http

import (
	"errors"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/config"
	"horizonx/internal/domain"
)

type AuthHandler struct {
	svc domain.AuthService
	cfg *config.Config

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewAuthHandler(
	svc domain.AuthService,
	cfg *config.Config,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *AuthHandler {
	return &AuthHandler{
		svc:       svc,
		cfg:       cfg,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

func (h *AuthHandler) User(w http.ResponseWriter, r *http.Request) {
	user, err := h.svc.GetUser(r.Context())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			h.writer.Write(w, http.StatusUnauthorized, &response.Response{
				Message: "unauthorized",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get user",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: user,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req domain.LoginRequest
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

	res, err := h.svc.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			h.writer.Write(w, http.StatusUnauthorized, &response.Response{
				Message: "invalid credentials",
			})
			return
		}

		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to sign in",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    res.AccessToken,
		Path:     "/",
		Expires:  time.Now().Add(h.cfg.JWTExpiry),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: res.User,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	h.writer.Write(w, http.StatusOK, &response.Response{})
}
