//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type publishedEvent struct {
	eventType string
	accountID *int64
	groupID   *int64
	payload   map[string]any
}

type testOutboxPublisher struct {
	events *[]publishedEvent
}

func (p *testOutboxPublisher) Publish(_ context.Context, eventType string, accountID *int64, groupID *int64, payload map[string]any) error {
	*p.events = append(*p.events, publishedEvent{eventType, accountID, groupID, payload})
	return nil
}

func (p *testOutboxPublisher) Read(_ context.Context, _ int64, _ time.Duration) ([]service.SchedulerOutboxEvent, error) {
	return nil, nil
}
func (p *testOutboxPublisher) Ack(_ context.Context, _ ...string) error { return nil }
func (p *testOutboxPublisher) Pending(_ context.Context) (int64, error) { return 0, nil }

// setupTestOutboxPublisher installs a test publisher and returns a pointer to
// the captured events slice. The original publisher is restored on cleanup.
func setupTestOutboxPublisher(t *testing.T) *[]publishedEvent {
	t.Helper()
	old := schedulerOutboxPublisher
	events := make([]publishedEvent, 0, 8)
	pub := &testOutboxPublisher{events: &events}
	SetSchedulerOutboxPublisher(pub)
	t.Cleanup(func() { schedulerOutboxPublisher = old })
	return &events
}
