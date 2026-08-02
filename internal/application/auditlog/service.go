// Package auditlog — append-only record of user + system actions.
package auditlog

import (
	"context"
	"encoding/json"

	"horizonx/internal/domain"
)

type AuditLogService struct {
	repo domain.AuditLogRepository
}

func NewService(repo domain.AuditLogRepository) domain.AuditLogService {
	return &AuditLogService{repo: repo}
}

// Create records an action. details is marshaled to JSON; nil → {}.
func (s *AuditLogService) Create(ctx context.Context, actorID *int64, action, resourceType, resourceID string, details any) (*domain.AuditLog, error) {
	raw := json.RawMessage(`{}`)
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return s.repo.Create(ctx, &domain.AuditLog{
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      raw,
	})
}

func (s *AuditLogService) List(ctx context.Context, opts domain.AuditLogListOptions) (*domain.ListResult[*domain.AuditLog], error) {
	if opts.IsPaginate {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		if opts.Limit <= 0 {
			opts.Limit = 10
		}
	} else {
		if opts.Limit <= 0 {
			opts.Limit = 100
		}
	}
	logs, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &domain.ListResult[*domain.AuditLog]{Data: logs, Meta: domain.CalculateMeta(total, opts.Page, opts.Limit)}, nil
}
