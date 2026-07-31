package repository

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// schedulerOutboxPublisher 全局 Redis Streams 发布器。
// 在应用启动时通过 SetSchedulerOutboxPublisher 注入。
var schedulerOutboxPublisher service.SchedulerOutboxQueue

// SetSchedulerOutboxPublisher 设置全局 outbox 发布器（应用启动时调用）。
func SetSchedulerOutboxPublisher(q service.SchedulerOutboxQueue) {
	schedulerOutboxPublisher = q
}

// enqueueSchedulerOutbox 发布调度事件到 Redis Streams。
// 去重逻辑已移至 SchedulerOutboxQueue.Publish 内部（基于 Redis SET NX EX）。
func enqueueSchedulerOutbox(ctx context.Context, eventType string, accountID *int64, groupID *int64, payload any) error {
	if schedulerOutboxPublisher == nil {
		return nil
	}
	var payloadMap map[string]any
	if payload != nil {
		switch v := payload.(type) {
		case map[string]any:
			payloadMap = v
		default:
			encoded, err := json.Marshal(v)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(encoded, &payloadMap); err != nil {
				return err
			}
		}
	}
	return schedulerOutboxPublisher.Publish(ctx, eventType, accountID, groupID, payloadMap)
}

func schedulerOutboxEventSupportsDedup(eventType string) bool {
	switch eventType {
	case service.SchedulerOutboxEventAccountChanged,
		service.SchedulerOutboxEventAccountGroupsChanged,
		service.SchedulerOutboxEventGroupChanged,
		service.SchedulerOutboxEventProviderChanged,
		service.SchedulerOutboxEventCapabilityChanged,
		service.SchedulerOutboxEventLogicalModelChanged,
		service.SchedulerOutboxEventAdapterChanged,
		service.SchedulerOutboxEventFullRebuild:
		return true
	default:
		return false
	}
}
