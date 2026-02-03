// Package agent
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/system"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
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

var ErrUnauthorized = errors.New("connection failed: unauthorized")

func NewAgent(cfg *config.Config, log logger.Logger) *Agent {
	return &Agent{
		send: make(chan []byte, 256),
		cfg:  cfg,
		log:  log,
	}
}

func (a *Agent) Start(ctx context.Context) error {
	a.send = make(chan []byte, 256)
	reconnectInterval := 5 * time.Second
	attempt := 0

	for {
		a.log.Info("starting agent...", "attempt", attempt+1)

		err := a.dialUp(ctx)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				return err
			}

			a.log.Warn("connection lost or failed, will retry", "error", err)
		}

		attempt++
		a.log.Debug("waiting before next reconnection attempt")

		select {
		case <-ctx.Done():
			a.log.Info("agent stopped")
			return nil
		case <-time.After(reconnectInterval):
		}
	}
}

func (a *Agent) dialUp(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+a.cfg.AgentServerID.String()+"."+a.cfg.AgentServerAPIToken)

	conn, res, err := dialer.DialContext(ctx, a.cfg.AgentTargetWsURL, header)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("dial failed: %w", err)
	}
	a.conn = conn
	a.log.Info("connected to server", "url", a.cfg.AgentTargetWsURL)

	a.sendServerOSInfo()

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return a.readPump(gctx) })
	g.Go(func() error { return a.writePump(gctx) })

	go func() {
		<-gctx.Done()
		if a.conn != nil {
			_ = a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			_ = a.conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutting down"),
			)
			a.conn.Close()
			a.conn = nil
		}
	}()

	if err := g.Wait(); err != nil {
		a.log.Warn("connection closed unexpectedly, pumps exited", "error", err)
	}

	return nil
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
			return nil
		default:
			_, message, err := a.conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}

				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return nil
				}

				return err
			}

			var serverMessage domain.WsServerMessage
			if err := json.Unmarshal(message, &serverMessage); err != nil {
				a.log.Error("invalid server message received", "error", err)
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			default:
				a.log.Debug("incoming server message", "payload", serverMessage.Payload)
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
			return nil
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

func (a *Agent) sendServerOSInfo() {
	system := system.NewReader(a.log)

	osInfo, err := json.Marshal(&domain.OSInfo{
		Hostname:      system.Hostname(),
		Name:          system.OsName(),
		Arch:          system.Arch(),
		KernelVersion: system.KernelVersion(),
	})
	if err != nil {
		a.log.Error("failed to marshal OS info payload", "error", err.Error())
		return
	}

	rawMessage := &domain.WsAgentMessage{
		ServerID: a.cfg.AgentServerID,
		Event:    "server_os_info",
		Payload:  osInfo,
	}

	message, err := json.Marshal(rawMessage)
	if err != nil {
		a.log.Error("failed to marshal agent message", "error", err.Error())
		return
	}

	select {
	case a.send <- message:
		a.log.Debug("server OS info sent successfully")
	default:
		a.log.Warn("send channel full, OS info dropped")
	}
}
