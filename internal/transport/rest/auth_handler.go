package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
)

type AuthHandler struct {
	svc domain.AuthService
	cfg *config.Config
}

func NewAuthHandler(svc domain.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		svc: svc,
		cfg: cfg,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if validationErrors := ValidateStruct(req); len(validationErrors) > 0 {
		JSONValidationError(w, validationErrors)
		return
	}

	res, err := h.svc.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			JSONError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		JSONError(w, http.StatusInternalServerError, "Something went wrong")
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

	JSONSuccess(w, http.StatusOK, APIResponse{
		Message: "OK",
		Data:    res.User,
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

	JSONSuccess(w, http.StatusOK, APIResponse{})
}
