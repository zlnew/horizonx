package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/webhook"
	"horizonx/internal/domain"
)

// fakeSettingsRepo is an in-memory SettingsRepository for handler tests.
type fakeSettingsRepo struct {
	values map[string]json.RawMessage
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{values: map[string]json.RawMessage{}}
}

func (f *fakeSettingsRepo) Get(ctx context.Context, key string) (json.RawMessage, error) {
	raw, ok := f.values[key]
	if !ok {
		return nil, domain.ErrSettingNotFound
	}
	return raw, nil
}

func (f *fakeSettingsRepo) Set(ctx context.Context, key string, value json.RawMessage) error {
	f.values[key] = value
	return nil
}

func (f *fakeSettingsRepo) Delete(ctx context.Context, key string) error {
	delete(f.values, key)
	return nil
}

func newSettingsTestHandler(repo domain.SettingsRepository, notifier *webhook.Notifier) *SettingsHandler {
	log := stubLogger{}
	return NewSettingsHandler(
		repo,
		notifier,
		request.NewJSONDecoder(),
		response.NewJSONWriter(log),
		nil,
	)
}

func TestSettingsGetWebhookDefaults(t *testing.T) {
	h := newSettingsTestHandler(newFakeSettingsRepo(), nil)
	req := httptest.NewRequest(http.MethodGet, "/settings/webhook", nil)
	rec := httptest.NewRecorder()

	h.GetWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Data struct {
			Enabled bool   `json:"enabled"`
			URL     string `json:"url"`
			Secret  string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Enabled {
		t.Error("expected default disabled")
	}
	if body.Data.URL != "" || body.Data.Secret != "" {
		t.Errorf("expected empty defaults, got %+v", body.Data)
	}
}

func TestSettingsUpdateWebhookMasksSecret(t *testing.T) {
	repo := newFakeSettingsRepo()
	h := newSettingsTestHandler(repo, nil)

	body := `{"enabled":true,"url":"https://discord.com/api/webhooks/abc","secret":"s3cr3t"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UpdateWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Enabled bool   `json:"enabled"`
			URL     string `json:"url"`
			Secret  string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Secret != "set" {
		t.Errorf("secret should be masked as 'set', got %q", resp.Data.Secret)
	}
	if resp.Data.URL != "https://discord.com/api/webhooks/abc" {
		t.Errorf("unexpected URL in response: %q", resp.Data.URL)
	}

	// Persisted value must keep the real secret.
	raw, err := repo.Get(context.Background(), domain.SettingWebhook)
	if err != nil {
		t.Fatalf("repo get: %v", err)
	}
	var stored domain.WebhookSettings
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode stored: %v", err)
	}
	if stored.Secret != "s3cr3t" {
		t.Errorf("stored secret = %q, want s3cr3t", stored.Secret)
	}
}

func TestSettingsUpdateWebhookRejectsBadURL(t *testing.T) {
	h := newSettingsTestHandler(newFakeSettingsRepo(), nil)

	body := `{"enabled":true,"url":"not a url"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UpdateWebhook(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad URL, got %d", rec.Code)
	}
}

func TestSettingsUpdateWebhookEmptySecretKeepsExisting(t *testing.T) {
	repo := newFakeSettingsRepo()
	existing, _ := json.Marshal(domain.WebhookSettings{Enabled: true, URL: "https://example.com/hook", Secret: "keepme"})
	repo.values[domain.SettingWebhook] = existing
	h := newSettingsTestHandler(repo, nil)

	body := `{"url":"https://example.com/hook2"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UpdateWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	raw, _ := repo.Get(context.Background(), domain.SettingWebhook)
	var stored domain.WebhookSettings
	_ = json.Unmarshal(raw, &stored)
	if stored.Secret != "keepme" {
		t.Errorf("empty secret field must keep existing secret, got %q", stored.Secret)
	}
	if stored.URL != "https://example.com/hook2" {
		t.Errorf("URL should update, got %q", stored.URL)
	}
}

func TestSettingsUpdateWebhookResetClearsSecret(t *testing.T) {
	repo := newFakeSettingsRepo()
	existing, _ := json.Marshal(domain.WebhookSettings{Enabled: true, URL: "https://example.com/hook", Secret: "dropme"})
	repo.values[domain.SettingWebhook] = existing
	h := newSettingsTestHandler(repo, nil)

	body := `{"secret":"reset"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UpdateWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	raw, _ := repo.Get(context.Background(), domain.SettingWebhook)
	var stored domain.WebhookSettings
	_ = json.Unmarshal(raw, &stored)
	if stored.Secret != "" {
		t.Errorf("secret should be cleared on reset, got %q", stored.Secret)
	}
}
