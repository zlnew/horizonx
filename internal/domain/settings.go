package domain

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrSettingNotFound is returned when a setting key has no row.
var ErrSettingNotFound = errors.New("setting not found")

// SettingsRepository persists runtime-configurable settings (webhook config,
// future knobs). Values are stored as JSONB so typed settings can evolve
// without schema churn. Unlike env vars, these are changeable at runtime and
// survive restarts.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (json.RawMessage, error)
	Set(ctx context.Context, key string, value json.RawMessage) error
	Delete(ctx context.Context, key string) error
}

// SettingsKeys are the canonical setting keys.
const (
	SettingWebhook = "webhook"
)

// WebhookSettings controls deployment-event webhook delivery.
// Secret is the HMAC-SHA256 signing secret: when set, the notifier signs
// every POST body with X-HorizonX-Signature so receivers can verify
// authenticity. The API never returns the raw secret on reads.
type WebhookSettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}

// DefaultWebhookSettings is the zero-config state: disabled, no URL, no secret.
func DefaultWebhookSettings() WebhookSettings {
	return WebhookSettings{Enabled: false}
}
