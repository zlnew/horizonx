// Package webhook delivers deployment-event notifications to an external
// endpoint (e.g. a Discord webhook). P2-15.
//
// Deliberately fire-and-forget: webhook failures never affect the deploy
// flow. If no webhook URL is configured the notifier is a no-op.
//
// The HTTP POST runs on a background worker, not on the event-bus publisher
// goroutine: a slow webhook must never block the agent request that
// triggered the event (the bus is fully synchronous).
package webhook

import (
	"bytes"
	"context"
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

// Notifier posts deployment status changes to a webhook URL.
type Notifier struct {
	url    string
	client *http.Client
	appSvc domain.ApplicationService
	log    logger.Logger

	events chan []byte
}

func New(url string, appSvc domain.ApplicationService, log logger.Logger) *Notifier {
	n := &Notifier{
		url:    url,
		client: &http.Client{Timeout: notifyTimeout},
		appSvc: appSvc,
		log:    log,
		events: make(chan []byte, workerBuffer),
	}
	go n.run()
	return n
}

// run drains the notification queue on a dedicated goroutine so the event
// bus publisher never blocks on network I/O.
func (n *Notifier) run() {
	for payload := range n.events {
		if err := n.post(payload); err != nil {
			// Fire-and-forget: log and move on, never break the deploy flow.
			n.log.Error("webhook: notify failed", "error", err)
		}
	}
}

// Handle implements the event bus subscriber signature.
// It only reacts to terminal deployment states (success/failed) to avoid
// spamming on every intermediate transition.
func (n *Notifier) Handle(event any) {
	if n.url == "" {
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

func (n *Notifier) post(payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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

func deploymentLink(deploymentID int64) string {
	return fmt.Sprintf("deployment #%d", deploymentID)
}
