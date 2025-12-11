// Package agent
package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
)

type Agent struct {
	conn *websocket.Conn
	log  logger.Logger

	serverID  int64
	serverURL string
	token     string

	initCh       chan int64
	metricsCh    chan domain.Metrics
	sessionCtxCh chan context.Context
}

var ErrUnauthorized = errors.New("connection failed: unauthorized (check token)")

func NewAgent(serverURL, token string, log logger.Logger) *Agent {
	return &Agent{
		serverURL: serverURL,
		token:     token,
		log:       log,

		serverID:     0,
		initCh:       make(chan int64, 1),
		metricsCh:    make(chan domain.Metrics, 10),
		sessionCtxCh: make(chan context.Context, 1),
	}
}

func (a *Agent) GetSessionContextChannel() chan context.Context {
	return a.sessionCtxCh
}

func (a *Agent) Run(ctx context.Context) error {
	reconnectInterval := 5 * time.Second
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			a.log.Info("agent run loop received shutdown signal")
			return ctx.Err()
		default:
		}

		a.log.Info("Attempting to start agent...", "attempt", attempt+1)

		err := a.start(ctx)

		if err != nil && errors.Is(err, ErrUnauthorized) {
			a.log.Error("FATAL: Unauthorized token. Exiting.", "error", err)
			return err
		}

		if err != nil {
			a.log.Error("agent session failed, retrying...", "error", err)
		} else {
			a.log.Info("agent session ended, attempting reconnect.")
		}

		attempt++
		a.log.Debug("waiting before next reconnection attempt")

		select {
		case <-time.After(reconnectInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *Agent) start(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+a.token)

	conn, resp, err := dialer.DialContext(ctx, a.serverURL, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	a.conn = conn
	a.log.Info("websocket connected to server", "url", a.serverURL)

	sessionCtx, cancel := context.WithCancel(ctx)

	pumpDone := make(chan error, 1)

	defer func() {
		cancel()
		a.conn.Close()
		a.serverID = 0
		a.log.Info("websocket connection closed and resources cleaned up")
	}()

	go func() { pumpDone <- a.readPump(sessionCtx) }()
	go func() { pumpDone <- a.writePump(sessionCtx) }()

	select {
	case a.sessionCtxCh <- sessionCtx:
	case <-ctx.Done():
		return ctx.Err()
	}

	a.log.Info("waiting for server initialization command...")
	if err := a.waitForInit(sessionCtx); err != nil {
		return err
	}

	var finalErr error

	select {
	case finalErr = <-pumpDone:
		a.log.Info("a pump has exited, shutting down agent session")
	case <-ctx.Done():
		finalErr = ctx.Err()
		a.log.Info("agent received external shutdown signal")

		select {
		case <-time.After(time.Millisecond * 100):
		case <-pumpDone:
		}
	}

	a.conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = a.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Shutting down"),
	)

	return finalErr
}

func (a *Agent) waitForInit(ctx context.Context) error {
	select {
	case id := <-a.initCh:
		a.serverID = id
		a.log.Info("agent initialized successfully", "server_id", a.serverID)
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("agent initialization timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) SendMetric(m domain.Metrics) {
	if a.serverID == 0 {
		a.log.Warn("agent not initialized, dropping metric")
		return
	}

	m.ServerID = a.serverID

	select {
	case a.metricsCh <- m:
	default:
		a.log.Warn("metrics channel full, dropping metric", "ts", time.Now().UTC())
	}
}
