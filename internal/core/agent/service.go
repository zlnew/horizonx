// Package agent
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"horizonx-server/internal/config"
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
	send chan []byte
	cfg  *config.Config
	log  logger.Logger
}

var ErrUnauthorized = errors.New("connection failed: unauthorized (check token)")

func IsFatalError(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

func NewAgent(cfg *config.Config, log logger.Logger) *Agent {
	return &Agent{
		send: make(chan []byte, 256),
		cfg:  cfg,
		log:  log,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	a.send = make(chan []byte, 256)
	reconnectInterval := 5 * time.Second
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			a.log.Info("agent run loop received shutdown signal")
			return ctx.Err()
		default:
		}

		a.log.Info("starting agent...", "attempt", attempt+1)

		err := a.start(ctx)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				a.log.Error("FATAL: unauthorized token. exiting...", "error", err)
				return err
			}

			a.log.Error("failed to start agent", "error", err)
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
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+a.cfg.AgentServerID.String()+"."+a.cfg.AgentServerAPIToken)

	conn, res, err := dialer.DialContext(ctx, a.cfg.AgentTargetWsURL, header)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("ws dial failed: %w", err)
	}

	a.conn = conn
	a.log.Info("ws connected to server", "url", a.cfg.AgentTargetWsURL)

	sessionCtx, cancel := context.WithCancel(ctx)
	pumpDone := make(chan error, 2)

	defer func() {
		cancel()
		a.conn.Close()
	}()

	go func() { pumpDone <- a.readPump(sessionCtx) }()
	go func() { pumpDone <- a.writePump(sessionCtx) }()

	var finalErr error
	select {
	case finalErr = <-pumpDone:
		a.log.Info("a pump has exited, shutting down agent session", "error", finalErr)
	case <-ctx.Done():
		finalErr = ctx.Err()
		a.log.Info("agent received external shutdown signal, closing session")
	}

	_ = a.conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = a.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutting down"),
	)

	select {
	case <-time.After(time.Millisecond * 100):
	case pumpErr := <-pumpDone:
		if finalErr == nil {
			finalErr = pumpErr
		}
	}

	return finalErr
}

func (a *Agent) readPump(ctx context.Context) error {
	a.conn.SetReadLimit(maxMessageSize)
	a.conn.SetReadDeadline(time.Now().Add(pongWait))
	a.conn.SetPongHandler(func(string) error {
		a.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := a.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					a.log.Error("ws read error (unexpected close)", "error", err)
				}
				return err
			}

			var event domain.WsServerEvent
			if err := json.Unmarshal(message, &event); err != nil {
				a.log.Error("invalid event payload received", "error", err)
				continue
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	}
}

func (a *Agent) writePump(ctx context.Context) error {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case message, ok := <-a.send:
			a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				a.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return nil
			}

			w, err := a.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return err
			}

			_, err = w.Write(message)
			if err != nil {
				w.Close()
				return err
			}

			if err := w.Close(); err != nil {
				return err
			}

		case <-ticker.C:
			a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := a.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
}
