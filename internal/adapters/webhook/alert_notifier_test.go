package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"horizonx/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureServer returns an httptest server that records every POST body and
// its X-HorizonX-Signature header.
type captureServer struct {
	mu        sync.Mutex
	body      []string
	signature []string
	ts        *httptest.Server
}

func newCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	c := &captureServer{}
	c.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rec map[string]string
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.body = append(c.body, rec["content"])
		c.signature = append(c.signature, r.Header.Get(SignatureHeader))
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.ts.Close)
	return c
}

func (c *captureServer) messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.body))
	copy(out, c.body)
	return out
}

func (c *captureServer) signatures() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.signature))
	copy(out, c.signature)
	return out
}

func (c *captureServer) waitFor(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(c.messages()) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Failf(t, "timed out", "expected %d webhook POSTs, got %d", n, len(c.messages()))
}

func newTestNotifier(t *testing.T, cap *captureServer) *Notifier {
	t.Helper()
	n := New(func() domain.WebhookSettings {
		return domain.WebhookSettings{Enabled: true, URL: cap.ts.URL, Secret: "test-secret"}
	}, &fakeAppSvc{}, nil)
	t.Cleanup(func() { close(n.events) })
	return n
}

func TestNotifierDeliversAlertFiredAndResolved(t *testing.T) {
	cap := newCaptureServer(t)
	n := newTestNotifier(t, cap)

	sid := uuid.New()
	appID := int64(5)

	n.Handle(domain.EventAlertFired{
		RuleID:   1,
		RuleName: "high cpu",
		ServerID: sid,
		AppID:    &appID,
		Severity: domain.AlertSeverityCritical,
		Message:  "CPU usage breached: 95.00 > 90.00",
	})
	cap.waitFor(t, 1, 2*time.Second)

	n.Handle(domain.EventAlertResolved{
		RuleID:   1,
		RuleName: "high cpu",
		ServerID: sid,
		AppID:    &appID,
	})
	cap.waitFor(t, 2, 2*time.Second)

	msgs := cap.messages()
	require.Len(t, msgs, 2, "one POST per alert transition")
	assert.Contains(t, msgs[0], "high cpu", "fire message names the rule")
	assert.Contains(t, msgs[0], "95.00", "fire message carries the observed value")
	assert.Contains(t, msgs[1], "high cpu", "resolve message names the rule")
}

func TestNotifierStillDeliversDeployEvents(t *testing.T) {
	cap := newCaptureServer(t)
	n := newTestNotifier(t, cap)

	// Regression: the existing deploy path must keep firing alongside alerts.
	n.Handle(domain.EventDeploymentStatusChanged{
		DeploymentID:  7,
		ApplicationID: 3,
		Status:        domain.DeploymentSuccess,
	})
	cap.waitFor(t, 1, 2*time.Second)

	n.Handle(domain.EventAlertFired{
		RuleID:   2,
		RuleName: "server offline",
		ServerID: uuid.New(),
		Severity: domain.AlertSeverityWarning,
		Message:  "Server is offline",
	})
	cap.waitFor(t, 2, 2*time.Second)

	msgs := cap.messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[0], "Deployment", "deploy event unaffected")
	assert.Contains(t, msgs[1], "server offline", "alert event delivered alongside")
}

func TestNotifierSignsAlertPayloads(t *testing.T) {
	cap := newCaptureServer(t)
	n := newTestNotifier(t, cap)

	n.Handle(domain.EventAlertFired{
		RuleID:   1,
		RuleName: "high cpu",
		ServerID: uuid.New(),
		Severity: domain.AlertSeverityWarning,
		Message:  "CPU usage breached",
	})
	cap.waitFor(t, 1, 2*time.Second)

	sigs := cap.signatures()
	require.Len(t, sigs, 1, "alert POST must carry X-HorizonX-Signature")
	assert.NotEmpty(t, sigs[0])
}

func TestRuleRefFallback(t *testing.T) {
	assert.Equal(t, "rule #42", ruleRef(42, ""))
	assert.Equal(t, "my rule", ruleRef(42, "my rule"))
}
