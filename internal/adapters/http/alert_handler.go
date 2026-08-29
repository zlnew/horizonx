package http

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"
)

// AlertHandler exposes alert rule CRUD, the active/history views and the
// ack/silence actions. All methods require the JWT user stack; the router
// gates them behind PermAlertRead / PermAlertWrite.
type AlertHandler struct {
	svc domain.AlertService

	decoder   request.RequestDecoder
	writer    response.ResponseWriter
	validator validator.Validator
}

func NewAlertHandler(
	svc domain.AlertService,
	d request.RequestDecoder,
	w response.ResponseWriter,
	v validator.Validator,
) *AlertHandler {
	return &AlertHandler{
		svc:       svc,
		decoder:   d,
		writer:    w,
		validator: v,
	}
}

// ---- rules ----

// RulesIndex lists alert rules with optional scope/server/app/source filters.
func (h *AlertHandler) RulesIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := domain.AlertRuleListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		ServerID: GetUUID(q, "server_id"),
		AppID:    GetInt64(q, "app_id"),
	}
	if v := q.Get("scope"); v != "" {
		s := domain.AlertScope(v)
		opts.Scope = &s
	}
	if v := q.Get("source"); v != "" {
		s := domain.AlertCondition(v)
		opts.Source = &s
	}

	result, err := h.svc.ListRules(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list alert rules",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

// RulesStore creates an alert rule.
func (h *AlertHandler) RulesStore(w http.ResponseWriter, r *http.Request) {
	var req domain.AlertRuleSaveRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid request body",
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	rule, err := h.svc.CreateRule(r.Context(), req)
	if err != nil {
		h.writer.Write(w, http.StatusUnprocessableEntity, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusCreated, &response.Response{
		Message: "alert rule created",
		Data:    rule,
	})
}

// RulesShow returns a single alert rule.
func (h *AlertHandler) RulesShow(w http.ResponseWriter, r *http.Request) {
	ruleID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	rule, err := h.svc.GetRule(r.Context(), ruleID)
	if err != nil {
		if errors.Is(err, domain.ErrAlertRuleNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert rule not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get alert rule",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{Data: rule})
}

// RulesUpdate replaces an alert rule.
func (h *AlertHandler) RulesUpdate(w http.ResponseWriter, r *http.Request) {
	ruleID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	var req domain.AlertRuleSaveRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid request body",
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	rule, err := h.svc.UpdateRule(r.Context(), req, ruleID)
	if err != nil {
		if errors.Is(err, domain.ErrAlertRuleNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert rule not found",
			})
			return
		}
		h.writer.Write(w, http.StatusUnprocessableEntity, &response.Response{
			Message: err.Error(),
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "alert rule updated",
		Data:    rule,
	})
}

// RulesDestroy deletes an alert rule (its history cascades).
func (h *AlertHandler) RulesDestroy(w http.ResponseWriter, r *http.Request) {
	ruleID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteRule(r.Context(), ruleID); err != nil {
		if errors.Is(err, domain.ErrAlertRuleNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert rule not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to delete alert rule",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "alert rule deleted",
	})
}

// ---- alerts ----

// ActiveIndex lists currently firing alerts.
func (h *AlertHandler) ActiveIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := alertHistoryListOptions(q)
	result, err := h.svc.ListActive(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list active alerts",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

// HistoryIndex lists paginated alert history with state/severity/ack filters.
func (h *AlertHandler) HistoryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := alertHistoryListOptions(q)
	result, err := h.svc.ListHistory(r.Context(), opts)
	if err != nil {
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to list alert history",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Data: result.Data,
		Meta: result.Meta,
	})
}

// HistoryShow returns a single alert history row.
func (h *AlertHandler) HistoryShow(w http.ResponseWriter, r *http.Request) {
	historyID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	alert, err := h.svc.GetHistory(r.Context(), historyID)
	if err != nil {
		if errors.Is(err, domain.ErrAlertHistoryNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to get alert",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{Data: alert})
}

// HistoryAck marks an alert acknowledged.
func (h *AlertHandler) HistoryAck(w http.ResponseWriter, r *http.Request) {
	historyID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	if err := h.svc.Ack(r.Context(), historyID); err != nil {
		if errors.Is(err, domain.ErrAlertHistoryNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to acknowledge alert",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "alert acknowledged",
	})
}

// SilenceRule silences the rule behind an alert until silenced_until. While
// silenced the evaluator skips re-firing that rule.
func (h *AlertHandler) SilenceRule(w http.ResponseWriter, r *http.Request) {
	historyID, ok := parseID(w, h, r)
	if !ok {
		return
	}

	var req struct {
		SilencedUntil time.Time `json:"silenced_until" validate:"required"`
	}
	if err := h.decoder.Decode(r, &req); err != nil {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid request body",
		})
		return
	}

	if errs := h.validator.Validate(&req); len(errs) > 0 {
		h.writer.WriteValidationError(w, errs)
		return
	}

	if err := h.svc.Silence(r.Context(), historyID, req.SilencedUntil); err != nil {
		if errors.Is(err, domain.ErrAlertHistoryNotFound) {
			h.writer.Write(w, http.StatusNotFound, &response.Response{
				Message: "alert not found",
			})
			return
		}
		h.writer.Write(w, http.StatusInternalServerError, &response.Response{
			Message: "failed to silence alert",
		})
		return
	}

	h.writer.Write(w, http.StatusOK, &response.Response{
		Message: "alert silenced",
		Data: map[string]any{
			"silenced_until": req.SilencedUntil,
		},
	})
}

// ---- helpers ----

func parseID(w http.ResponseWriter, h *AlertHandler, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		h.writer.Write(w, http.StatusBadRequest, &response.Response{
			Message: "invalid id",
		})
		return 0, false
	}
	return id, true
}

func alertHistoryListOptions(q url.Values) domain.AlertHistoryListOptions {
	opts := domain.AlertHistoryListOptions{
		ListOptions: domain.ListOptions{
			Page:       GetInt(q, "page", 1),
			Limit:      GetInt(q, "limit", 10),
			Search:     GetString(q, "search", ""),
			IsPaginate: GetBool(q, "paginate"),
		},
		ServerID: GetUUID(q, "server_id"),
		AppID:    GetInt64(q, "app_id"),
		Acked:    GetBoolPtr(q, "acked"),
	}
	if v := q.Get("state"); v != "" {
		s := domain.AlertState(v)
		opts.State = &s
	}
	if v := q.Get("severity"); v != "" {
		s := domain.AlertSeverity(v)
		opts.Severity = &s
	}
	return opts
}
