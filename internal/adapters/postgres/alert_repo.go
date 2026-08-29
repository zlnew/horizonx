package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"horizonx/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AlertRuleRepository implements domain.AlertRuleRepository.
type AlertRuleRepository struct {
	db *pgxpool.Pool
}

func NewAlertRuleRepository(db *pgxpool.Pool) *AlertRuleRepository {
	return &AlertRuleRepository{db: db}
}

var _ domain.AlertRuleRepository = (*AlertRuleRepository)(nil)

// strPtr returns nil for an empty string so nullable columns store NULL
// instead of '' — offline/health rules legitimately have no operator,
// metric_path or target_status, and the API sends "" for them.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// strVal coalesces a NULL scan target back to the domain zero value "".
func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const alertRuleColumns = `
	id,
	name,
	scope,
	server_id,
	app_id,
	source,
	metric_path,
	operator,
	threshold,
	target_status,
	for_duration,
	cooldown,
	severity,
	enabled,
	silenced_until,
	created_at,
	updated_at
`

func (r *AlertRuleRepository) List(ctx context.Context, opts domain.AlertRuleListOptions) ([]*domain.AlertRule, int64, error) {
	baseQuery := "SELECT " + alertRuleColumns + " FROM alert_rules"
	args := []any{}
	conditions := []string{}
	argCounter := 1

	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", argCounter))
		args = append(args, "%"+opts.Search+"%")
		argCounter++
	}
	if opts.Scope != nil {
		conditions = append(conditions, fmt.Sprintf("scope = $%d", argCounter))
		args = append(args, string(*opts.Scope))
		argCounter++
	}
	if opts.ServerID != nil {
		conditions = append(conditions, fmt.Sprintf("server_id = $%d", argCounter))
		args = append(args, *opts.ServerID)
		argCounter++
	}
	if opts.AppID != nil {
		conditions = append(conditions, fmt.Sprintf("app_id = $%d", argCounter))
		args = append(args, *opts.AppID)
		argCounter++
	}
	if opts.Source != nil {
		conditions = append(conditions, fmt.Sprintf("source = $%d", argCounter))
		args = append(args, string(*opts.Source))
		argCounter++
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	baseQuery += " ORDER BY created_at DESC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM alert_rules"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count alert rules: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query alert rules: %w", err)
	}
	defer rows.Close()

	var rules []*domain.AlertRule
	for rows.Next() {
		var rule domain.AlertRule
		var metricPath, operator, targetStatus *string
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Scope,
			&rule.ServerID,
			&rule.AppID,
			&rule.Source,
			&metricPath,
			&operator,
			&rule.Threshold,
			&targetStatus,
			&rule.ForDuration,
			&rule.Cooldown,
			&rule.Severity,
			&rule.Enabled,
			&rule.SilencedUntil,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan alert rule: %w", err)
		}
		rule.MetricPath = strVal(metricPath)
		rule.Operator = domain.AlertOperator(strVal(operator))
		rule.TargetStatus = domain.ApplicationStatus(strVal(targetStatus))
		rules = append(rules, &rule)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *AlertRuleRepository) Get(ctx context.Context, ruleID int64) (*domain.AlertRule, error) {
	query := "SELECT " + alertRuleColumns + " FROM alert_rules WHERE id = $1 LIMIT 1"

	var rule domain.AlertRule
	var metricPath, operator, targetStatus *string
	err := r.db.QueryRow(ctx, query, ruleID).Scan(
		&rule.ID,
		&rule.Name,
		&rule.Scope,
		&rule.ServerID,
		&rule.AppID,
		&rule.Source,
		&metricPath,
		&operator,
		&rule.Threshold,
		&targetStatus,
		&rule.ForDuration,
		&rule.Cooldown,
		&rule.Severity,
		&rule.Enabled,
		&rule.SilencedUntil,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAlertRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alert rule: %w", err)
	}
	rule.MetricPath = strVal(metricPath)
	rule.Operator = domain.AlertOperator(strVal(operator))
	rule.TargetStatus = domain.ApplicationStatus(strVal(targetStatus))
	return &rule, nil
}

func (r *AlertRuleRepository) Create(ctx context.Context, rule *domain.AlertRule) (*domain.AlertRule, error) {
	query := `
		INSERT INTO alert_rules (
			name, scope, server_id, app_id, source,
			metric_path, operator, threshold, target_status,
			for_duration, cooldown, severity, enabled
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		rule.Name,
		string(rule.Scope),
		rule.ServerID,
		rule.AppID,
		string(rule.Source),
		strPtr(rule.MetricPath),
		strPtr(string(rule.Operator)),
		rule.Threshold,
		strPtr(string(rule.TargetStatus)),
		rule.ForDuration,
		rule.Cooldown,
		string(rule.Severity),
		rule.Enabled,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create alert rule: %w", err)
	}
	return rule, nil
}

func (r *AlertRuleRepository) Update(ctx context.Context, rule *domain.AlertRule, ruleID int64) error {
	query := `
		UPDATE alert_rules SET
			name = $1,
			scope = $2,
			server_id = $3,
			app_id = $4,
			source = $5,
			metric_path = $6,
			operator = $7,
			threshold = $8,
			target_status = $9,
			for_duration = $10,
			cooldown = $11,
			severity = $12,
			enabled = $13,
			updated_at = now()
		WHERE id = $14
	`

	ct, err := r.db.Exec(ctx, query,
		rule.Name,
		string(rule.Scope),
		rule.ServerID,
		rule.AppID,
		string(rule.Source),
		strPtr(rule.MetricPath),
		strPtr(string(rule.Operator)),
		rule.Threshold,
		strPtr(string(rule.TargetStatus)),
		rule.ForDuration,
		rule.Cooldown,
		string(rule.Severity),
		rule.Enabled,
		ruleID,
	)
	if err != nil {
		return fmt.Errorf("failed to update alert rule: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertRuleNotFound
	}
	return nil
}

func (r *AlertRuleRepository) Delete(ctx context.Context, ruleID int64) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, ruleID)
	if err != nil {
		return fmt.Errorf("failed to delete alert rule: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertRuleNotFound
	}
	return nil
}

// ListEnabled returns every enabled rule (history is not persisted for
// disabled rules; the evaluator re-reads live per event).
func (r *AlertRuleRepository) ListEnabled(ctx context.Context) ([]domain.AlertRule, error) {
	query := "SELECT " + alertRuleColumns + " FROM alert_rules WHERE enabled IS TRUE ORDER BY id"

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled alert rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.AlertRule
	for rows.Next() {
		var rule domain.AlertRule
		var metricPath, operator, targetStatus *string
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Scope,
			&rule.ServerID,
			&rule.AppID,
			&rule.Source,
			&metricPath,
			&operator,
			&rule.Threshold,
			&targetStatus,
			&rule.ForDuration,
			&rule.Cooldown,
			&rule.Severity,
			&rule.Enabled,
			&rule.SilencedUntil,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan enabled alert rule: %w", err)
		}
		rule.MetricPath = strVal(metricPath)
		rule.Operator = domain.AlertOperator(strVal(operator))
		rule.TargetStatus = domain.ApplicationStatus(strVal(targetStatus))
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *AlertRuleRepository) SetSilenced(ctx context.Context, ruleID int64, until *time.Time) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE alert_rules SET silenced_until = $2, updated_at = now() WHERE id = $1`,
		ruleID, until,
	)
	if err != nil {
		return fmt.Errorf("failed to set alert rule silenced: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertRuleNotFound
	}
	return nil
}

// AlertHistoryRepository implements domain.AlertHistoryRepository.
type AlertHistoryRepository struct {
	db *pgxpool.Pool
}

func NewAlertHistoryRepository(db *pgxpool.Pool) *AlertHistoryRepository {
	return &AlertHistoryRepository{db: db}
}

var _ domain.AlertHistoryRepository = (*AlertHistoryRepository)(nil)

const alertHistoryColumns = `
	h.id,
	h.rule_id,
	COALESCE(r.name, ''),
	h.server_id,
	COALESCE(s.name, ''),
	h.app_id,
	COALESCE(a.name, ''),
	h.severity,
	h.state,
	h.value,
	h.message,
	h.acked,
	h.silenced_until,
	h.first_fired_at,
	h.resolved_at,
	h.created_at
`

const alertHistoryFrom = `
	FROM alert_history h
	LEFT JOIN alert_rules r ON r.id = h.rule_id
	LEFT JOIN servers s ON s.id = h.server_id
	LEFT JOIN applications a ON a.id = h.app_id
`

func (r *AlertHistoryRepository) list(ctx context.Context, opts domain.AlertHistoryListOptions, forceState string) ([]*domain.AlertHistory, int64, error) {
	baseQuery := "SELECT " + alertHistoryColumns + alertHistoryFrom
	args := []any{}
	conditions := []string{}
	argCounter := 1

	if forceState != "" {
		conditions = append(conditions, fmt.Sprintf("h.state = $%d", argCounter))
		args = append(args, forceState)
		argCounter++
	} else if opts.State != nil {
		conditions = append(conditions, fmt.Sprintf("h.state = $%d", argCounter))
		args = append(args, string(*opts.State))
		argCounter++
	}
	if opts.Severity != nil {
		conditions = append(conditions, fmt.Sprintf("h.severity = $%d", argCounter))
		args = append(args, string(*opts.Severity))
		argCounter++
	}
	if opts.Acked != nil {
		if *opts.Acked {
			conditions = append(conditions, "h.acked IS TRUE")
		} else {
			conditions = append(conditions, "h.acked IS FALSE")
		}
	}
	if opts.ServerID != nil {
		conditions = append(conditions, fmt.Sprintf("h.server_id = $%d", argCounter))
		args = append(args, *opts.ServerID)
		argCounter++
	}
	if opts.AppID != nil {
		conditions = append(conditions, fmt.Sprintf("h.app_id = $%d", argCounter))
		args = append(args, *opts.AppID)
		argCounter++
	}
	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(h.message ILIKE $%d OR r.name ILIKE $%d)", argCounter, argCounter+1))
		searchParam := "%" + opts.Search + "%"
		args = append(args, searchParam, searchParam)
		argCounter += 2
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	baseQuery += " ORDER BY h.created_at DESC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) " + alertHistoryFrom[strings.Index(alertHistoryFrom, "FROM"):]
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count alert history: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query alert history: %w", err)
	}
	defer rows.Close()

	var alerts []*domain.AlertHistory
	for rows.Next() {
		var alert domain.AlertHistory
		if err := rows.Scan(
			&alert.ID,
			&alert.RuleID,
			&alert.RuleName,
			&alert.ServerID,
			&alert.ServerName,
			&alert.AppID,
			&alert.AppName,
			&alert.Severity,
			&alert.State,
			&alert.Value,
			&alert.Message,
			&alert.Acked,
			&alert.SilencedUntil,
			&alert.FirstFiredAt,
			&alert.ResolvedAt,
			&alert.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan alert history: %w", err)
		}
		alerts = append(alerts, &alert)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return alerts, total, nil
}

func (r *AlertHistoryRepository) ListActive(ctx context.Context, opts domain.AlertHistoryListOptions) ([]*domain.AlertHistory, int64, error) {
	return r.list(ctx, opts, string(domain.AlertStateFiring))
}

func (r *AlertHistoryRepository) List(ctx context.Context, opts domain.AlertHistoryListOptions) ([]*domain.AlertHistory, int64, error) {
	return r.list(ctx, opts, "")
}

func (r *AlertHistoryRepository) Get(ctx context.Context, historyID int64) (*domain.AlertHistory, error) {
	query := "SELECT " + alertHistoryColumns + alertHistoryFrom + " WHERE h.id = $1 LIMIT 1"

	var alert domain.AlertHistory
	err := r.db.QueryRow(ctx, query, historyID).Scan(
		&alert.ID,
		&alert.RuleID,
		&alert.RuleName,
		&alert.ServerID,
		&alert.ServerName,
		&alert.AppID,
		&alert.AppName,
		&alert.Severity,
		&alert.State,
		&alert.Value,
		&alert.Message,
		&alert.Acked,
		&alert.SilencedUntil,
		&alert.FirstFiredAt,
		&alert.ResolvedAt,
		&alert.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAlertHistoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alert history: %w", err)
	}
	return &alert, nil
}

func (r *AlertHistoryRepository) Create(ctx context.Context, alert *domain.AlertHistory) (*domain.AlertHistory, error) {
	query := `
		INSERT INTO alert_history (
			rule_id, server_id, app_id, severity, state,
			value, message, acked, silenced_until, first_fired_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		alert.RuleID,
		alert.ServerID,
		alert.AppID,
		string(alert.Severity),
		string(alert.State),
		alert.Value,
		alert.Message,
		alert.Acked,
		alert.SilencedUntil,
		alert.FirstFiredAt,
	).Scan(&alert.ID, &alert.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create alert history: %w", err)
	}
	return alert, nil
}

func (r *AlertHistoryRepository) Resolve(ctx context.Context, historyID int64) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE alert_history SET state = 'resolved', resolved_at = now() WHERE id = $1 AND state = 'firing'`,
		historyID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve alert history: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertHistoryNotFound
	}
	return nil
}

func (r *AlertHistoryRepository) Ack(ctx context.Context, historyID int64) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE alert_history SET acked = TRUE WHERE id = $1`,
		historyID,
	)
	if err != nil {
		return fmt.Errorf("failed to ack alert history: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertHistoryNotFound
	}
	return nil
}

func (r *AlertHistoryRepository) SetSilenced(ctx context.Context, historyID int64, until *time.Time) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE alert_history SET silenced_until = $2 WHERE id = $1`,
		historyID, until,
	)
	if err != nil {
		return fmt.Errorf("failed to set alert history silenced: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrAlertHistoryNotFound
	}
	return nil
}
