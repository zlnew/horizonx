// Package alert implements the alert domain service: rule CRUD, history
// queries, ack and silence. It mirrors the other application-layer services
// (application, server, job) — thin orchestration over the alert repos.
package alert

import (
	"context"
	"fmt"
	"time"

	"horizonx/internal/domain"
)

type Service struct {
	ruleRepo    domain.AlertRuleRepository
	historyRepo domain.AlertHistoryRepository
}

func NewService(ruleRepo domain.AlertRuleRepository, historyRepo domain.AlertHistoryRepository) domain.AlertService {
	return &Service{
		ruleRepo:    ruleRepo,
		historyRepo: historyRepo,
	}
}

// ---- Rules ----

func (s *Service) ListRules(ctx context.Context, opts domain.AlertRuleListOptions) (*domain.ListResult[*domain.AlertRule], error) {
	rules, total, err := s.ruleRepo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &domain.ListResult[*domain.AlertRule]{
		Data: rules,
		Meta: domain.CalculateMeta(total, opts.Page, opts.Limit),
	}, nil
}

func (s *Service) GetRule(ctx context.Context, ruleID int64) (*domain.AlertRule, error) {
	return s.ruleRepo.Get(ctx, ruleID)
}

func (s *Service) CreateRule(ctx context.Context, req domain.AlertRuleSaveRequest) (*domain.AlertRule, error) {
	rule, err := buildRule(req, &domain.AlertRule{})
	if err != nil {
		return nil, err
	}

	return s.ruleRepo.Create(ctx, rule)
}

func (s *Service) UpdateRule(ctx context.Context, req domain.AlertRuleSaveRequest, ruleID int64) (*domain.AlertRule, error) {
	current, err := s.ruleRepo.Get(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	rule, err := buildRule(req, current)
	if err != nil {
		return nil, err
	}

	if err := s.ruleRepo.Update(ctx, rule, ruleID); err != nil {
		return nil, err
	}

	return s.ruleRepo.Get(ctx, ruleID)
}

func (s *Service) DeleteRule(ctx context.Context, ruleID int64) error {
	return s.ruleRepo.Delete(ctx, ruleID)
}

// buildRule converts a save request into a persisted AlertRule. `current` is
// the existing row on update (preserves silenced_until, which is only ever
// changed via the silence endpoint).
func buildRule(req domain.AlertRuleSaveRequest, current *domain.AlertRule) (*domain.AlertRule, error) {
	rule := &domain.AlertRule{
		Name:          req.Name,
		Scope:         req.Scope,
		ServerID:      req.ServerID,
		AppID:         req.AppID,
		Source:        req.Source,
		MetricPath:    req.MetricPath,
		Operator:      req.Operator,
		Threshold:     req.Threshold,
		TargetStatus:  req.TargetStatus,
		ForDuration:   req.ForDuration,
		Cooldown:      req.Cooldown,
		Severity:      req.Severity,
		SilencedUntil: current.SilencedUntil,
	}

	if rule.Severity == "" {
		rule.Severity = domain.AlertSeverityWarning
	}
	if req.Enabled == nil {
		rule.Enabled = true
	} else {
		rule.Enabled = *req.Enabled
	}
	if rule.ForDuration < 0 {
		rule.ForDuration = 0
	}
	if rule.Cooldown < 0 {
		rule.Cooldown = 0
	}

	if err := validateRule(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func validateRule(rule *domain.AlertRule) error {
	switch rule.Scope {
	case domain.AlertScopeServer:
		if rule.ServerID == nil {
			return fmt.Errorf("server_id is required when scope is server")
		}
		rule.AppID = nil
	case domain.AlertScopeApp:
		if rule.AppID == nil {
			return fmt.Errorf("app_id is required when scope is app")
		}
		rule.ServerID = nil
	case domain.AlertScopeGlobal:
		rule.ServerID = nil
		rule.AppID = nil
	default:
		return fmt.Errorf("scope must be one of: server, app, global")
	}

	switch rule.Source {
	case domain.ConditionMetric:
		if !domain.ValidMetricPath(rule.MetricPath) {
			return fmt.Errorf("metric_path is not a supported metric: %q", rule.MetricPath)
		}
		if rule.Operator == "" {
			return fmt.Errorf("operator is required for metric rules")
		}
		if rule.Threshold == nil {
			return fmt.Errorf("threshold is required for metric rules")
		}
	case domain.ConditionHealth:
		switch rule.TargetStatus {
		case domain.AppStatusFailed, domain.AppStatusStopped, domain.AppStatusUnknown:
		default:
			return fmt.Errorf("target_status must be one of: failed, stopped, unknown")
		}
	case domain.ConditionOffline:
		// no extra parameters needed
	default:
		return fmt.Errorf("source must be one of: metric, health, offline")
	}

	return nil
}

// ---- History ----

func (s *Service) ListActive(ctx context.Context, opts domain.AlertHistoryListOptions) (*domain.ListResult[*domain.AlertHistory], error) {
	alerts, total, err := s.historyRepo.ListActive(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &domain.ListResult[*domain.AlertHistory]{
		Data: alerts,
		Meta: domain.CalculateMeta(total, opts.Page, opts.Limit),
	}, nil
}

func (s *Service) ListHistory(ctx context.Context, opts domain.AlertHistoryListOptions) (*domain.ListResult[*domain.AlertHistory], error) {
	alerts, total, err := s.historyRepo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &domain.ListResult[*domain.AlertHistory]{
		Data: alerts,
		Meta: domain.CalculateMeta(total, opts.Page, opts.Limit),
	}, nil
}

func (s *Service) GetHistory(ctx context.Context, historyID int64) (*domain.AlertHistory, error) {
	return s.historyRepo.Get(ctx, historyID)
}

func (s *Service) Ack(ctx context.Context, historyID int64) error {
	return s.historyRepo.Ack(ctx, historyID)
}

// Silence mutes the rule behind an active alert until `until`. Both the rule
// (so the evaluator skips re-firing) and the alert history row (so the UI can
// show the mute) are stamped. Clearing is done by passing a zero time, which
// translates to NULL.
func (s *Service) Silence(ctx context.Context, historyID int64, until time.Time) error {
	alert, err := s.historyRepo.Get(ctx, historyID)
	if err != nil {
		return err
	}

	var untilPtr *time.Time
	if !until.IsZero() {
		u := until.UTC()
		untilPtr = &u
	}

	if err := s.ruleRepo.SetSilenced(ctx, alert.RuleID, untilPtr); err != nil {
		return err
	}
	return s.historyRepo.SetSilenced(ctx, historyID, untilPtr)
}
