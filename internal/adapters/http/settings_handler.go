package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/adapters/webhook"
	"horizonx/internal/domain"
)

type SettingsHandler struct {
	repo      domain.SettingsRepository
	notifier  *webhook.Notifier
	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewSettingsHandler(
	repo domain.SettingsRepository,
	notifier *webhook.Notifier,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *SettingsHandler {
	return &SettingsHandler{
		repo:      repo,
		notifier:  notifier,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

// webhookView is the wire shape for webhook settings. Secret is never
// echoed back on reads — a masked placeholder ("set") signals one exists.
type webhookView struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}

// GetWebhook returns current webhook settings. The secret is masked.
func (h *SettingsHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	ws, err := h.loadWebhook(r)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to load webhook settings",
		})
		return
	}

	view := webhookView{Enabled: ws.Enabled, URL: ws.URL}
	if ws.Secret != "" {
		view.Secret = "set"
	}
	h.writer.Write(w, http.StatusOK, &response.Response{Data: view})
}

// UpdateWebhook persists webhook settings. An empty secret field means
// "keep the existing secret" (the API never round-trips it); pass the
// explicit string "reset" to clear it.
func (h *SettingsHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool  `json:"enabled"`
		URL     string `json:"url"`
		Secret  string `json:"secret"`
	}
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid request body",
		})
		return
	}

	current, err := h.loadWebhook(r)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to load webhook settings",
		})
		return
	}

	next := current
	if req.Enabled != nil {
		next.Enabled = *req.Enabled
	}
	if req.URL != "" {
		if !isValidWebhookURL(req.URL) {
			h.writer.Write(w, http.StatusBadRequest, &response.Response{
				Message: "webhook URL must be a valid http(s) URL",
			})
			return
		}
		next.URL = req.URL
	}
	switch req.Secret {
	case "":
		// keep existing secret
	case "reset":
		next.Secret = ""
	default:
		next.Secret = req.Secret
	}

	raw, err := json.Marshal(next)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to encode webhook settings",
		})
		return
	}
	if err := h.repo.Set(r.Context(), domain.SettingWebhook, raw); err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to save webhook settings",
		})
		return
	}

	view := webhookView{Enabled: next.Enabled, URL: next.URL}
	if next.Secret != "" {
		view.Secret = "set"
	}
	h.writer.Write(w, http.StatusOK, &response.Response{Data: view})
}

// TestWebhook sends a sample payload through the notifier synchronously so
// the dashboard can prove end-to-end delivery.
func (h *SettingsHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	if h.notifier == nil {
		h.writer.Write(w, http.StatusServiceUnavailable, &response.Response{
			Message: "webhook notifier not available",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	code, err := h.notifier.TestPing(ctx)
	if err != nil {
		h.writer.Write(w, http.StatusBadGateway, &response.Response{
			Message: "webhook test ping failed: " + err.Error(),
		})
		return
	}
	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: map[string]any{"status": code, "message": "webhook test ping delivered"},
	})
}

// isValidWebhookURL accepts absolute http/https URLs. Hostnames and ports
// are free-form (self-hosted receivers on a tailnet are legitimate), so we
// only reject non-http schemes and malformed URLs.
func isValidWebhookURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (h *SettingsHandler) loadWebhook(r *http.Request) (domain.WebhookSettings, error) {
	raw, err := h.repo.Get(r.Context(), domain.SettingWebhook)
	if err != nil {
		if err == domain.ErrSettingNotFound {
			return domain.DefaultWebhookSettings(), nil
		}
		return domain.WebhookSettings{}, err
	}
	var ws domain.WebhookSettings
	if err := json.Unmarshal(raw, &ws); err != nil {
		return domain.WebhookSettings{}, err
	}
	return ws, nil
}
