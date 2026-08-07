// Package webhook delivers deployment-event notifications to an external
// endpoint (e.g. a Discord webhook). P2-15.
//
// Deliberately fire-and-forget: webhook failures never affect the deploy
// flow. If webhook settings are disabled or have no URL the notifier is a
// no-op.
//
// The HTTP POST runs on a background worker, not on the event-bus publisher
// goroutine: a slow webhook must never block the agent request that
// triggered the event (the bus is fully synchronous).
//
// Settings are read live per event via a provider function (backed by the
// settings repo), so changing the webhook URL from the dashboard takes
// effect without a restart. When a secret is configured, every payload is
// signed with HMAC-SHA256 and delivered in X-HorizonX-Signature.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

const (
	notifyTimeout = 5 * time.Second

	// workerBuffer bounds the queue of pending notifications; if it fills we
	// drop the oldest (terminal deploy events are low-volume, a missed ping
	// is acceptable — never block the hot path).
	workerBuffer = 64
)

// SignatureHeader carries the HMAC-SHA256 hex digest of the POST body when a
// secret is configured. Receivers verify: X-HorizonX-Signature ==
// hex(hmac_sha256(secret, body)).
const SignatureHeader = "X-HorizonX-Signature"

// SettingsProvider returns the current webhook settings. Implemented by the
// server wiring as a read-through to the settings repo so changes apply
// without restart.
type SettingsProvider func() domain.WebhookSettings

// Notifier posts deployment status changes to a webhook URL.
type Notifier struct {
	getSettings SettingsProvider
	client      *http.Client
	appSvc      domain.ApplicationService
	log         logger.Logger

	events chan []byte
}

func New(getSettings SettingsProvider, appSvc domain.ApplicationService, log logger.Logger) *Notifier {
	n := &Notifier{
		getSettings: getSettings,
		client:      &http.Client{Timeout: notifyTimeout},
		appSvc:      appSvc,
		log:         log,
		events:      make(chan []byte, workerBuffer),
	}
	go n.run()
	return n
}

// run drains the notification queue on a dedicated goroutine so the event
// bus publisher never blocks on network I/O.
func (n *Notifier) run() {
	for payload := range n.events {
		ws := n.getSettings()
		if !ws.Enabled || ws.URL == "" {
			// Settings changed between enqueue and delivery — skip quietly.
			continue
		}
		if err := n.post(ws, payload); err != nil {
			// Fire-and-forget: log and move on, never break the deploy flow.
			n.log.Error("webhook: notify failed", "error", err)
		}
	}
}

// Handle implements the event bus subscriber signature.
// It only reacts to terminal deployment states (success/failed) to avoid
// spamming on every intermediate transition.
func (n *Notifier) Handle(event any) {
	ws := n.getSettings()
	if !ws.Enabled || ws.URL == "" {
		return
	}

	evt, ok := event.(domain.EventDeploymentStatusChanged)
	if !ok {
		return
	}

	switch evt.Status {
	case domain.DeploymentSuccess, domain.DeploymentFailed:
	default:
		return
	}

	appName := n.appName(context.Background(), evt.ApplicationID)
	emoji := "✅"
	statusText := "succeeded"
	if evt.Status == domain.DeploymentFailed {
		emoji = "❌"
		statusText = "failed"
	}

	msg := fmt.Sprintf(
		"%s Deployment **%s** (%s) %s.",
		emoji, appName, deploymentLink(evt.DeploymentID), statusText,
	)

	payload, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		n.log.Error("webhook: marshal failed", "error", err)
		return
	}

	// Enqueue without blocking; if the queue is full, drop the oldest and
	// keep the newest so the latest deploy state is what gets delivered.
	select {
	case n.events <- payload:
	default:
		select {
		case <-n.events:
			n.log.Warn("webhook: queue full, dropped oldest notification")
		default:
		}
		select {
		case n.events <- payload:
		default:
			n.log.Warn("webhook: queue full, dropped notification")
		}
	}
}

// TestPing sends a sample notification synchronously using the CURRENT
// settings, so the dashboard can prove end-to-end delivery. Returns the
// HTTP status code of the receiver.
func (n *Notifier) TestPing(ctx context.Context) (int, error) {
	ws := n.getSettings()
	if !ws.Enabled || ws.URL == "" {
		return 0, fmt.Errorf("webhook is not enabled or has no URL")
	}

	payload := []byte(`{"content":"✅ HorizonX webhook test ping."}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ws.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if ws.Secret != "" {
		req.Header.Set(SignatureHeader, sign(payload, ws.Secret))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func (n *Notifier) post(ws domain.WebhookSettings, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ws.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if ws.Secret != "" {
		req.Header.Set(SignatureHeader, sign(payload, ws.Secret))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Discord returns 204 on success; anything else is a miss.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}

// sign returns hex(hmac_sha256(secret, payload)).
func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (n *Notifier) appName(ctx context.Context, appID int64) string {
	if n.appSvc == nil {
		return fmt.Sprintf("app #%d", appID)
	}
	app, err := n.appSvc.GetByID(ctx, appID)
	if err != nil || app == nil {
		return fmt.Sprintf("app #%d", appID)
	}
	return app.Name
}

// deploymentLink renders a stable reference for the deployment in the
// notification text. No live URL is built here (dashboard origin varies per
// install); the deployment ID is the canonical handle.
func deploymentLink(deploymentID int64) string {
	return fmt.Sprintf("deployment #%d", deploymentID)
}
