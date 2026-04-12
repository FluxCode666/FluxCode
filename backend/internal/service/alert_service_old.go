package service

import (
	"context"
	"fmt"
	"html"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	defaultAlertCooldownMinutes = 5
	alertTypeNoAccounts         = "no_available_accounts"
	alertTypePoolThreshold      = "pool_threshold"
	alertTypeProxyFailures      = "proxy_transport_failures"
)

type NoAvailableAccountsAlert struct {
	Message  string
	Path     string
	Method   string
	Platform string
	UserID   *int64
	APIKeyID *int64
	GroupID  *int64
}

type OpenAIPoolThresholdAlert struct {
	Platform              string
	AvailableCount        int
	BaseAccountCount      int
	AvailableRatioPercent float64
	CountThreshold        int
	RatioThreshold        int
	CountRuleTriggered    bool
	RatioRuleTriggered    bool
}

type OpenAIProxyTransportFailureAlert struct {
	Platform      string
	ProxyID       int64
	ProxyName     string
	WindowMinutes int
	FailureCount  int64
	Threshold     int
	LastError     string
}

type AlertCooldownStore interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type PoolMonitorConfigProvider interface {
	GetConfig(ctx context.Context, platform string) (*AccountPoolAlertConfig, error)
}

type AlertService struct {
	emailService  *EmailService
	poolConfig    PoolMonitorConfigProvider
	cooldownStore AlertCooldownStore
	siteName      string
	lastSent      map[string]time.Time
	mu            sync.Mutex

	cooldownMu      sync.Mutex
	cooldownCache   time.Duration
	cooldownCacheAt time.Time
}

func NewAlertService(emailService *EmailService, poolConfig PoolMonitorConfigProvider, cooldownStore AlertCooldownStore) *AlertService {
	siteName := strings.TrimSpace(os.Getenv("WEB_TITLE"))
	if siteName == "" {
		siteName = "FluxCode"
	}
	return &AlertService{
		emailService:  emailService,
		poolConfig:    poolConfig,
		cooldownStore: cooldownStore,
		siteName:      siteName,
		lastSent:      make(map[string]time.Time),
	}
}

func (s *AlertService) NotifyNoAvailableAccounts(ctx context.Context, detail NoAvailableAccountsAlert) {
	if s == nil || s.emailService == nil {
		return
	}
	platform := normalizePlatform(detail.Platform)
	if platform == "" {
		platform = PlatformOpenAI
	}
	if platform != PlatformOpenAI {
		return
	}
	if !s.isPoolThresholdEnabled(ctx, platform) {
		return
	}
	recipients := s.getRecipients(ctx)
	if len(recipients) == 0 {
		return
	}
	if !s.shouldSend(ctx, alertTypeNoAccounts) {
		return
	}

	subject := fmt.Sprintf("[%s] 号池异常告警", s.siteName)
	body := s.buildNoAvailableAccountsBody(detail)
	s.sendEmailAsync(recipients, subject, body)
}

func (s *AlertService) NotifyOpenAIPoolThreshold(ctx context.Context, detail OpenAIPoolThresholdAlert) {
	if s == nil || s.emailService == nil {
		return
	}
	recipients := s.getRecipients(ctx)
	if len(recipients) == 0 {
		return
	}
	platform := strings.TrimSpace(strings.ToLower(detail.Platform))
	if platform == "" {
		platform = PlatformOpenAI
	}
	if !s.shouldSend(ctx, alertTypePoolThreshold+":"+platform) {
		return
	}

	subject := fmt.Sprintf("[%s] OpenAI 号池阈值告警", s.siteName)
	body := s.buildOpenAIPoolThresholdBody(detail)
	s.sendEmailAsync(recipients, subject, body)
}

func (s *AlertService) NotifyOpenAIProxyTransportFailures(ctx context.Context, detail OpenAIProxyTransportFailureAlert) {
	if s == nil || s.emailService == nil || detail.ProxyID <= 0 {
		return
	}
	recipients := s.getRecipients(ctx)
	if len(recipients) == 0 {
		return
	}
	platform := strings.TrimSpace(strings.ToLower(detail.Platform))
	if platform == "" {
		platform = PlatformOpenAI
	}
	alertKey := fmt.Sprintf("%s:%s:%d", alertTypeProxyFailures, platform, detail.ProxyID)
	if !s.shouldSend(ctx, alertKey) {
		return
	}

	subject := fmt.Sprintf("[%s] OpenAI 代理连接异常告警", s.siteName)
	body := s.buildOpenAIProxyTransportFailureBody(detail)
	s.sendEmailAsync(recipients, subject, body)
}

func (s *AlertService) shouldSend(ctx context.Context, key string) bool {
	cooldown := s.getCooldown(ctx)
	if cooldown <= 0 {
		return true
	}

	if s != nil && s.cooldownStore != nil {
		ok, err := s.cooldownStore.Acquire(ctx, key, cooldown)
		if err == nil {
			return ok
		}
		logger.LegacyPrintf("service.alert", "[Alert] acquire distributed cooldown failed (fallback local): key=%s err=%v", key, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if last, ok := s.lastSent[key]; ok && now.Sub(last) < cooldown {
		return false
	}
	s.lastSent[key] = now
	return true
}

func (s *AlertService) sendEmailAsync(recipients []string, subject, body string) {
	recipients = append([]string(nil), recipients...)
	go func() {
		alertCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, to := range recipients {
			if err := s.emailService.SendEmail(alertCtx, to, subject, body); err != nil {
				logger.LegacyPrintf("service.alert", "[Alert] Failed to send alert email to %s: %v", to, err)
			}
		}
	}()
}

func (s *AlertService) isPoolThresholdEnabled(ctx context.Context, platform string) bool {
	if s == nil || s.poolConfig == nil {
		return true // fail-open
	}
	cfg, err := s.poolConfig.GetConfig(ctx, platform)
	if err != nil || cfg == nil {
		return true // fail-open
	}
	return cfg.PoolThresholdEnabled
}

func (s *AlertService) getRecipients(ctx context.Context) []string {
	if s == nil || s.poolConfig == nil {
		return nil
	}
	cfg, err := s.poolConfig.GetConfig(ctx, PlatformOpenAI)
	if err != nil || cfg == nil {
		return nil
	}
	recipients := normalizeAlertRecipients(cfg.AlertEmails)
	if len(recipients) == 0 {
		return nil
	}
	return recipients
}

func (s *AlertService) getCooldown(ctx context.Context) time.Duration {
	const cacheTTL = 30 * time.Second

	s.cooldownMu.Lock()
	if !s.cooldownCacheAt.IsZero() && time.Since(s.cooldownCacheAt) < cacheTTL {
		v := s.cooldownCache
		s.cooldownMu.Unlock()
		return v
	}
	s.cooldownMu.Unlock()

	minutes := defaultAlertCooldownMinutes
	if s.poolConfig != nil {
		if cfg, err := s.poolConfig.GetConfig(ctx, PlatformOpenAI); err == nil && cfg != nil {
			minutes = cfg.AlertCooldownMinutes
		}
	}
	if minutes <= 0 {
		minutes = 0
	}
	d := time.Duration(minutes) * time.Minute

	s.cooldownMu.Lock()
	s.cooldownCache = d
	s.cooldownCacheAt = time.Now()
	s.cooldownMu.Unlock()
	return d
}

func (s *AlertService) buildNoAvailableAccountsBody(detail NoAvailableAccountsAlert) string {
	var builder strings.Builder
	_, _ = builder.WriteString("<div>")
	_, _ = builder.WriteString("<p>号池异常：没有可用账号。</p>")
	_, _ = builder.WriteString(fmt.Sprintf("<p>时间：%s</p>", time.Now().Format("2006-01-02 15:04:05")))
	if detail.Method != "" || detail.Path != "" {
		_, _ = builder.WriteString(fmt.Sprintf("<p>请求：%s %s</p>", detail.Method, detail.Path))
	}
	if detail.Platform != "" {
		_, _ = builder.WriteString(fmt.Sprintf("<p>平台：%s</p>", html.EscapeString(detail.Platform)))
	}
	if detail.UserID != nil {
		_, _ = builder.WriteString(fmt.Sprintf("<p>UserID：%d</p>", *detail.UserID))
	}
	if detail.GroupID != nil {
		_, _ = builder.WriteString(fmt.Sprintf("<p>GroupID：%d</p>", *detail.GroupID))
	}
	if detail.APIKeyID != nil {
		_, _ = builder.WriteString(fmt.Sprintf("<p>APIKeyID：%d</p>", *detail.APIKeyID))
	}
	if detail.Message != "" {
		_, _ = builder.WriteString(fmt.Sprintf("<p>详情：%s</p>", html.EscapeString(detail.Message)))
	}
	_, _ = builder.WriteString("</div>")
	return builder.String()
}

func (s *AlertService) buildOpenAIPoolThresholdBody(detail OpenAIPoolThresholdAlert) string {
	platform := strings.TrimSpace(detail.Platform)
	if platform == "" {
		platform = PlatformOpenAI
	}

	var builder strings.Builder
	_, _ = builder.WriteString("<div>")
	_, _ = builder.WriteString("<p>号池异常：OpenAI 账号池可用量低于阈值。</p>")
	_, _ = builder.WriteString(fmt.Sprintf("<p>时间：%s</p>", time.Now().Format("2006-01-02 15:04:05")))
	_, _ = builder.WriteString(fmt.Sprintf("<p>平台：%s</p>", html.EscapeString(platform)))
	_, _ = builder.WriteString(fmt.Sprintf("<p>当前可用账号数：%d</p>", detail.AvailableCount))
	_, _ = builder.WriteString(fmt.Sprintf("<p>分母账号数（status=active && schedulable=true）：%d</p>", detail.BaseAccountCount))
	if detail.BaseAccountCount > 0 {
		_, _ = builder.WriteString(fmt.Sprintf("<p>当前可用比例：%.2f%%</p>", detail.AvailableRatioPercent))
	}
	if detail.CountThreshold > 0 {
		_, _ = builder.WriteString(fmt.Sprintf("<p>账号数阈值：%d</p>", detail.CountThreshold))
	}
	if detail.RatioThreshold > 0 {
		_, _ = builder.WriteString(fmt.Sprintf("<p>比例阈值：%d%%</p>", detail.RatioThreshold))
	}
	_, _ = builder.WriteString(fmt.Sprintf("<p>命中规则：账号数=%t，比例=%t</p>", detail.CountRuleTriggered, detail.RatioRuleTriggered))
	_, _ = builder.WriteString("</div>")
	return builder.String()
}

func (s *AlertService) buildOpenAIProxyTransportFailureBody(detail OpenAIProxyTransportFailureAlert) string {
	platform := strings.TrimSpace(detail.Platform)
	if platform == "" {
		platform = PlatformOpenAI
	}
	window := detail.WindowMinutes
	if window <= 0 {
		window = defaultProxyFailureWindowMinutes
	}

	var builder strings.Builder
	_, _ = builder.WriteString("<div>")
	_, _ = builder.WriteString("<p>号池异常：代理连接上游失败次数达到阈值。</p>")
	_, _ = builder.WriteString(fmt.Sprintf("<p>时间：%s</p>", time.Now().Format("2006-01-02 15:04:05")))
	_, _ = builder.WriteString(fmt.Sprintf("<p>平台：%s</p>", html.EscapeString(platform)))
	_, _ = builder.WriteString(fmt.Sprintf("<p>代理ID：%d</p>", detail.ProxyID))
	if strings.TrimSpace(detail.ProxyName) != "" {
		_, _ = builder.WriteString(fmt.Sprintf("<p>代理名称：%s</p>", html.EscapeString(detail.ProxyName)))
	}
	_, _ = builder.WriteString(fmt.Sprintf("<p>窗口：%d 分钟</p>", window))
	_, _ = builder.WriteString(fmt.Sprintf("<p>失败次数：%d</p>", detail.FailureCount))
	_, _ = builder.WriteString(fmt.Sprintf("<p>阈值：%d</p>", detail.Threshold))
	if strings.TrimSpace(detail.LastError) != "" {
		_, _ = builder.WriteString(fmt.Sprintf("<p>最近错误：%s</p>", html.EscapeString(detail.LastError)))
	}
	_, _ = builder.WriteString("</div>")
	return builder.String()
}

func normalizeAlertRecipients(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		addr := strings.TrimSpace(item)
		if addr == "" {
			continue
		}
		key := strings.ToLower(addr)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}
	return out
}
