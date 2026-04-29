package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// GiftBalanceExpiryService 定期清理过期赠送余额
type GiftBalanceExpiryService struct {
	referralService *ReferralService
	interval        time.Duration
	stopCh          chan struct{}
	stopOnce        sync.Once
	wg              sync.WaitGroup
}

func NewGiftBalanceExpiryService(referralService *ReferralService, interval time.Duration) *GiftBalanceExpiryService {
	return &GiftBalanceExpiryService{
		referralService: referralService,
		interval:        interval,
		stopCh:          make(chan struct{}),
	}
}

func (s *GiftBalanceExpiryService) Start() {
	if s == nil || s.referralService == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *GiftBalanceExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *GiftBalanceExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expired, err := s.referralService.ExpireGiftBalanceRecords(ctx)
	if err != nil {
		slog.Error("[GiftBalanceExpiry] expire records failed", "error", err)
		return
	}
	if expired > 0 {
		slog.Info("[GiftBalanceExpiry] expired gift balance records", "count", expired)
	}
}
