package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"horizonx/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogRepository struct {
	db *pgxpool.Pool
}

func NewAuditLogRepository(db *pgxpool.Pool) domain.AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error) {
	details := log.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO audit_logs (actor_id, actor_email, action, resource_type, resource_id, details)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		log.ActorID, log.ActorEmail, log.Action, log.ResourceType, log.ResourceID, details)
	if err := row.Scan(&log.ID, &log.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert audit log: %w", err)
	}
	return log, nil
}

func (r *AuditLogRepository) List(ctx context.Context, opts domain.AuditLogListOptions) ([]*domain.AuditLog, int64, error) {
	baseQuery := `FROM audit_logs`
	conditions := []string{}
	args := []any{}
	argCounter := 1

	if opts.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argCounter))
		args = append(args, opts.Action)
		argCounter++
	}
	if opts.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argCounter))
		args = append(args, opts.ResourceType)
		argCounter++
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	baseQuery += " ORDER BY created_at DESC"

	if opts.IsPaginate {
		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, actor_id, actor_email, action, resource_type, resource_id, details, created_at
		`+baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*domain.AuditLog
	for rows.Next() {
		var l domain.AuditLog
		if err := rows.Scan(&l.ID, &l.ActorID, &l.ActorEmail, &l.Action, &l.ResourceType, &l.ResourceID, &l.Details, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit log: %w", err)
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}
