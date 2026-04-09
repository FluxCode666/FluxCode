package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type proxyUsageMetricsRepoStub struct {
	calls []struct {
		platform string
		proxyID  int64
		success  bool
	}
	err error
}

func (s *proxyUsageMetricsRepoStub) Increment(ctx context.Context, platform string, proxyID int64, occurredAt time.Time, success bool) error {
	s.calls = append(s.calls, struct {
		platform string
		proxyID  int64
		success  bool
	}{
		platform: platform,
		proxyID:  proxyID,
		success:  success,
	})
	return s.err
}

func (s *proxyUsageMetricsRepoStub) ListSummary(ctx context.Context, platform string, startTime, endTime time.Time) ([]ProxyUsageSummaryItem, error) {
	return nil, errors.New("not implemented")
}

func (s *proxyUsageMetricsRepoStub) ListHourlyBuckets(ctx context.Context, platform string, startTime, endTime time.Time) ([]ProxyUsageHourlyBucket, error) {
	return nil, errors.New("not implemented")
}

func TestOpenAIGatewayService_RecordProxyUsageMetric(t *testing.T) {
	repo := &proxyUsageMetricsRepoStub{}
	svc := &OpenAIGatewayService{proxyMetricsRepo: repo}
	proxyID := int64(9)
	account := &Account{ID: 1, ProxyID: &proxyID}

	svc.recordProxyUsageMetric(context.Background(), account, "http://127.0.0.1:8080", true, time.Now())

	if len(repo.calls) != 1 {
		t.Fatalf("expected one increment call, got %d", len(repo.calls))
	}
	if repo.calls[0].platform != ProxyUsageMetricsPlatformOpenAI {
		t.Fatalf("unexpected platform: %s", repo.calls[0].platform)
	}
	if repo.calls[0].proxyID != proxyID {
		t.Fatalf("unexpected proxy id: %d", repo.calls[0].proxyID)
	}
	if !repo.calls[0].success {
		t.Fatalf("expected success=true")
	}
}

func TestOpenAIGatewayService_RecordProxyUsageMetric_SkipWithoutProxy(t *testing.T) {
	repo := &proxyUsageMetricsRepoStub{}
	svc := &OpenAIGatewayService{proxyMetricsRepo: repo}

	svc.recordProxyUsageMetric(context.Background(), &Account{ID: 1}, "", false, time.Now())

	if len(repo.calls) != 0 {
		t.Fatalf("expected no increment call, got %d", len(repo.calls))
	}
}
