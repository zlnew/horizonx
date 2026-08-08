package application

import (
	"context"
	"encoding/json"
	"fmt"

	"horizonx/internal/domain"

	"github.com/google/uuid"
)

// TailLogs starts a live `docker compose logs --follow` on the agent hosting
// the app. Returns immediately with the stream ID; chunks arrive over the
// userws channel app_logs:{appID}.
func (s *Service) TailLogs(ctx context.Context, appID int64, req domain.LogsTailRequest) (string, error) {
	if s.agentCmd == nil {
		return "", fmt.Errorf("agent command sender not configured")
	}

	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return "", err
	}

	streamID := fmt.Sprintf("tail-%d-%s", appID, uuid.NewString()[:8])
	payload, err := json.Marshal(domain.LogsTailStartPayload{
		StreamID:      streamID,
		ApplicationID: appID,
		DirName:       domain.GetAppKey(app),
		Service:       req.Service,
		Tail:          req.Tail,
	})
	if err != nil {
		return "", fmt.Errorf("marshal tail payload: %w", err)
	}

	cmd := domain.AgentCommand{ID: uuid.NewString(), Type: "logs_tail_start", Payload: payload}
	if err := s.agentCmd.SendCommand(ctx, app.ServerID, cmd); err != nil {
		return "", err
	}
	return streamID, nil
}

// StopTailLogs asks the agent to cancel a running tail stream.
func (s *Service) StopTailLogs(ctx context.Context, appID int64, streamID string) error {
	if s.agentCmd == nil {
		return fmt.Errorf("agent command sender not configured")
	}

	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(domain.LogsTailStopPayload{StreamID: streamID})
	if err != nil {
		return fmt.Errorf("marshal stop payload: %w", err)
	}

	cmd := domain.AgentCommand{ID: uuid.NewString(), Type: "logs_tail_stop", Payload: payload}
	return s.agentCmd.SendCommand(ctx, app.ServerID, cmd)
}

// QueryLogs runs a one-shot historical `docker compose logs` (no --follow)
// and streams the result as container_log_chunk events ending with EOF:true.
func (s *Service) QueryLogs(ctx context.Context, appID int64, req domain.LogsQueryRequest) (string, error) {
	if s.agentCmd == nil {
		return "", fmt.Errorf("agent command sender not configured")
	}

	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return "", err
	}

	queryID := fmt.Sprintf("query-%d-%s", appID, uuid.NewString()[:8])
	payload, err := json.Marshal(domain.LogsQueryPayload{
		QueryID:       queryID,
		ApplicationID: appID,
		DirName:       domain.GetAppKey(app),
		Service:       req.Service,
		Since:         req.Since,
		Until:         req.Until,
		Tail:          req.Tail,
	})
	if err != nil {
		return "", fmt.Errorf("marshal query payload: %w", err)
	}

	cmd := domain.AgentCommand{ID: uuid.NewString(), Type: "logs_query", Payload: payload}
	if err := s.agentCmd.SendCommand(ctx, app.ServerID, cmd); err != nil {
		return "", err
	}
	return queryID, nil
}
