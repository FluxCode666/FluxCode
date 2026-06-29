package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const generatedImageCleanupHour = 4

// GeneratedImageCleanupService removes archived generated images after their HTTP URL retention window.
type GeneratedImageCleanupService struct {
	store          GeneratedImageStore
	settingService *SettingService
	now            func() time.Time

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func NewGeneratedImageCleanupService(store GeneratedImageStore, settingService *SettingService) *GeneratedImageCleanupService {
	return &GeneratedImageCleanupService{
		store:          store,
		settingService: settingService,
		now:            timezone.Now,
		stopCh:         make(chan struct{}),
	}
}

func (s *GeneratedImageCleanupService) Start() {
	if s == nil || s.store == nil {
		return
	}
	s.startOnce.Do(func() {
		next := nextGeneratedImageCleanupRunAfter(s.currentTime(), timezone.Location())
		logger.LegacyPrintf("service.generated_image_cleanup", "[GeneratedImageCleanup] started next_run=%s", next.Format(time.RFC3339))
		go s.runLoop()
	})
}

func (s *GeneratedImageCleanupService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		logger.LegacyPrintf("service.generated_image_cleanup", "[GeneratedImageCleanup] stopped")
	})
}

func (s *GeneratedImageCleanupService) runLoop() {
	for {
		now := s.currentTime()
		next := nextGeneratedImageCleanupRunAfter(now, timezone.Location())
		delay := next.Sub(now)
		if delay < 0 {
			delay = 0
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			deleted, err := s.cleanupOnce(ctx)
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.generated_image_cleanup", "[GeneratedImageCleanup] cleanup failed err=%v", err)
				continue
			}
			if deleted > 0 {
				logger.LegacyPrintf("service.generated_image_cleanup", "[GeneratedImageCleanup] cleaned expired generated images count=%d", deleted)
			}
		case <-s.stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (s *GeneratedImageCleanupService) cleanupOnce(ctx context.Context) (int64, error) {
	if s == nil || s.store == nil || s.settingService == nil {
		return 0, nil
	}
	if !s.settingService.IsGeneratedImageCleanupEnabled(ctx) {
		return 0, nil
	}
	ttl := s.settingService.GetOpenAIImageURLCacheTTL(ctx)
	cutoff := s.currentTime().Add(-ttl)
	return s.store.DeleteBefore(ctx, cutoff)
}

func (s *GeneratedImageCleanupService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return timezone.Now()
}

func nextGeneratedImageCleanupRunAfter(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	localNow := now.In(loc)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), generatedImageCleanupHour, 0, 0, 0, loc)
	if !localNow.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
