package domain

import (
	"context"
	"encoding/json"
	"time"
)

// AuditLog is an append-only record of a user or system action.
type AuditLog struct {
	ID           int64           `json:"id"`
	ActorID      *int64          `json:"actor_id,omitempty"`
	ActorEmail   string          `json:"actor_email,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type AuditLogListOptions struct {
	ListOptions
	Action       string `json:"action,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
}

type AuditLogRepository interface {
	Create(ctx context.Context, log *AuditLog) (*AuditLog, error)
	List(ctx context.Context, opts AuditLogListOptions) ([]*AuditLog, int64, error)
}

type AuditLogService interface {
	Create(ctx context.Context, actorID *int64, action, resourceType, resourceID string, details any) (*AuditLog, error)
	List(ctx context.Context, opts AuditLogListOptions) (*ListResult[*AuditLog], error)
}
