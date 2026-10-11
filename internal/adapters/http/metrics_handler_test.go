package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"horizonx/internal/adapters/http/metrics"
	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeMetricsService struct {
	last domain.Metrics
	err  error
}

func (f *fakeMetricsService) Ingest(ctx context.Context, m domain.Metrics) error {
	if f.err != nil {
		return f.err
	}
	f.last = m
	return nil
}

func (f *fakeMetricsService) Latest(ctx context.Context, serverID uuid.UUID) (*domain.Metrics, error) {
	return nil, domain.ErrMetricsNotFound
}

func (f *fakeMetricsService) CPUUsageHistory(ctx context.Context, serverID uuid.UUID) ([]domain.CPUUsageSample, error) {
	return nil, nil
}

func (f *fakeMetricsService) NetSpeedHistory(ctx context.Context, serverID uuid.UUID) ([]domain.NetworkSpeedSample, error) {
	return nil, nil
}

func (f *fakeMetricsService) Cleanup(ctx context.Context, serverID uuid.UUID, cutoff time.Time) error {
	return nil
}

func newMetricsTestHandler(svc domain.MetricsService, reg *metrics.Registry) *MetricsHandler {
	return NewMetricsHandler(
		svc,
		request.NewJSONDecoder(),
		response.NewJSONWriter(stubLogger{}),
		nil,
		reg,
	)
}

func TestMetricsIngestFeedsDroppedGauge(t *testing.T) {
	svc := &fakeMetricsService{}
	reg := metrics.NewRegistry(nil, nil, nil)
	defer reg.Stop()
	h := newMetricsTestHandler(svc, reg)

	serverID := uuid.New()
	body := `{"server_id":"` + serverID.String() + `","dropped_messages":9}`
	req := httptest.NewRequest(http.MethodPost, "/agent/metrics", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Ingest(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, float64(9), reg.AgentDroppedMessagesValue(serverID.String()))
}

func TestMetricsIngestOldPayloadLeavesGaugeAtZero(t *testing.T) {
	svc := &fakeMetricsService{}
	reg := metrics.NewRegistry(nil, nil, nil)
	defer reg.Stop()
	h := newMetricsTestHandler(svc, reg)

	serverID := uuid.New()
	body := `{"server_id":"` + serverID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/agent/metrics", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Ingest(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, float64(0), reg.AgentDroppedMessagesValue(serverID.String()))
}

func TestMetricsIngestNilRegistryDoesNotPanic(t *testing.T) {
	svc := &fakeMetricsService{}
	h := newMetricsTestHandler(svc, nil)

	serverID := uuid.New()
	body := `{"server_id":"` + serverID.String() + `","dropped_messages":3}`
	req := httptest.NewRequest(http.MethodPost, "/agent/metrics", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { h.Ingest(rec, req) })
	require.Equal(t, http.StatusCreated, rec.Code)
}
