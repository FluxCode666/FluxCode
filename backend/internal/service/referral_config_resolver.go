package service

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

const (
	// referralConfigCacheTTL 推广配置缓存 TTL（7 天）
	referralConfigCacheTTL = 7 * 24 * time.Hour
	// referralConfigErrorTTL 出错时的短缓存
	referralConfigErrorTTL = 30 * time.Second
)

// cachedGlobalConfig 全局推广配置缓存条目
type cachedGlobalConfig struct {
	config    *ReferralGlobalConfig
	expiresAt time.Time
}

// cachedUserConfig 用户推广配置缓存条目
type cachedUserConfig struct {
	config    *UserReferralConfig // nil 表示无自定义配置
	expiresAt time.Time
}

// ReferralConfigResolver 两级缓存的推广配置解析器
// - Level 1: 全局配置缓存（从 settings 表读取）
// - Level 2: 用户配置缓存（从 user_referral_configs 表读取）
// 最终配置 = 全局配置 + 用户覆盖（非 nil 字段覆盖全局值）
type ReferralConfigResolver struct {
	settingRepo    SettingRepository
	userConfigRepo UserReferralConfigRepository

	mu          sync.RWMutex
	globalCache *cachedGlobalConfig
	userCache   map[int64]*cachedUserConfig
}

// NewReferralConfigResolver 创建推广配置解析器
func NewReferralConfigResolver(settingRepo SettingRepository, userConfigRepo UserReferralConfigRepository) *ReferralConfigResolver {
	return &ReferralConfigResolver{
		settingRepo:    settingRepo,
		userConfigRepo: userConfigRepo,
		userCache:      make(map[int64]*cachedUserConfig),
	}
}

// Resolve 获取用户最终生效的推广配置
func (r *ReferralConfigResolver) Resolve(ctx context.Context, userID int64) *EffectiveReferralConfig {
	global := r.getGlobalConfig(ctx)
	if !global.Enabled {
		return &EffectiveReferralConfig{Enabled: false}
	}

	userCfg := r.getUserConfig(ctx, userID)
	return mergeConfig(global, userCfg)
}

// GetGlobalConfig 获取全局推广配置（带缓存）
func (r *ReferralConfigResolver) GetGlobalConfig(ctx context.Context) *ReferralGlobalConfig {
	return r.getGlobalConfig(ctx)
}

// InvalidateGlobalCache 失效全局缓存（配置更新时调用）
func (r *ReferralConfigResolver) InvalidateGlobalCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalCache = nil
}

// InvalidateUserCache 失效指定用户缓存
func (r *ReferralConfigResolver) InvalidateUserCache(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.userCache, userID)
}

// InvalidateAllUserCache 失效所有用户缓存
func (r *ReferralConfigResolver) InvalidateAllUserCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userCache = make(map[int64]*cachedUserConfig)
}

// getGlobalConfig 获取全局配置，命中缓存则直接返回
func (r *ReferralConfigResolver) getGlobalConfig(ctx context.Context) *ReferralGlobalConfig {
	r.mu.RLock()
	if c := r.globalCache; c != nil && time.Now().Before(c.expiresAt) {
		r.mu.RUnlock()
		return c.config
	}
	r.mu.RUnlock()

	// Cache miss — load from DB
	cfg := r.loadGlobalConfigFromDB(ctx)
	ttl := referralConfigCacheTTL

	r.mu.Lock()
	r.globalCache = &cachedGlobalConfig{config: cfg, expiresAt: time.Now().Add(ttl)}
	r.mu.Unlock()

	return cfg
}

// getUserConfig 获取用户配置，命中缓存则直接返回
func (r *ReferralConfigResolver) getUserConfig(ctx context.Context, userID int64) *UserReferralConfig {
	r.mu.RLock()
	if c, ok := r.userCache[userID]; ok && time.Now().Before(c.expiresAt) {
		r.mu.RUnlock()
		return c.config
	}
	r.mu.RUnlock()

	// Cache miss — load from DB
	cfg, err := r.userConfigRepo.GetByUserID(ctx, userID)
	if err != nil {
		slog.Error("load user referral config", "userID", userID, "error", err)
		// 错误时用短 TTL
		r.mu.Lock()
		r.userCache[userID] = &cachedUserConfig{config: nil, expiresAt: time.Now().Add(referralConfigErrorTTL)}
		r.mu.Unlock()
		return nil
	}

	r.mu.Lock()
	r.userCache[userID] = &cachedUserConfig{config: cfg, expiresAt: time.Now().Add(referralConfigCacheTTL)}
	r.mu.Unlock()

	return cfg
}

// loadGlobalConfigFromDB 从 settings 表读取全局推广配置
func (r *ReferralConfigResolver) loadGlobalConfigFromDB(ctx context.Context) *ReferralGlobalConfig {
	keys := []string{
		// 普通推广
		SettingKeyReferralEnabled,
		SettingKeyReferralInviteeRewardEnabled,
		SettingKeyReferralInviteeReward,
		SettingKeyReferralInviterReward,
		SettingKeyReferralMaxInvites,
		SettingKeyReferralRewardExpiryDays,
		SettingKeyReferralOngoingRewardEnabled,
		SettingKeyReferralOngoingRewardType,
		SettingKeyReferralOngoingRewardValue,
		SettingKeyReferralOngoingRewardMaxCount,
		SettingKeyReferralOngoingRewardDurationDays,
		// 普通推广：首充奖励
		SettingKeyReferralInviterFirstChargeRewardEnabled,
		SettingKeyReferralInviterFirstChargeRewardType,
		SettingKeyReferralInviterFirstChargeRewardValue,
		SettingKeyReferralInviteeFirstChargeRewardEnabled,
		SettingKeyReferralInviteeFirstChargeRewardType,
		SettingKeyReferralInviteeFirstChargeRewardValue,
		// 普通推广：被邀请人持续奖励
		SettingKeyReferralInviteeOngoingRewardEnabled,
		SettingKeyReferralInviteeOngoingRewardType,
		SettingKeyReferralInviteeOngoingRewardValue,
		SettingKeyReferralInviteeOngoingRewardMaxCount,
		SettingKeyReferralInviteeOngoingRewardDurationDays,
		// 销售推广
		SettingKeyReferralSalesEnabled,
		SettingKeyReferralSalesInviteeRewardEnabled,
		SettingKeyReferralSalesInviteeReward,
		SettingKeyReferralSalesInviteeFirstChargeRewardEnabled,
		SettingKeyReferralSalesInviteeFirstChargeRewardType,
		SettingKeyReferralSalesInviteeFirstChargeRewardValue,
		SettingKeyReferralSalesInviteeOngoingRewardEnabled,
		SettingKeyReferralSalesInviteeOngoingRewardType,
		SettingKeyReferralSalesInviteeOngoingRewardValue,
		SettingKeyReferralSalesInviteeOngoingRewardMaxCount,
		SettingKeyReferralSalesInviteeOngoingRewardDurationDays,
	}

	values, err := r.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		slog.Error("load referral global config from DB", "error", err)
		return &ReferralGlobalConfig{Enabled: false}
	}

	cfg := &ReferralGlobalConfig{
		// 普通推广
		Enabled:              parseBool(values[SettingKeyReferralEnabled]),
		InviteeRewardEnabled: parseBool(values[SettingKeyReferralInviteeRewardEnabled]),
		InviteeRewardAmount:  parseFloat(values[SettingKeyReferralInviteeReward]),
		InviterRewardAmount:  parseFloat(values[SettingKeyReferralInviterReward]),
		MaxInvites:           parseInt(values[SettingKeyReferralMaxInvites]),
		RewardExpiryDays:     parseInt(values[SettingKeyReferralRewardExpiryDays]),
		// 普通推广：首充奖励
		InviterFirstChargeRewardEnabled: parseBool(values[SettingKeyReferralInviterFirstChargeRewardEnabled]),
		InviterFirstChargeRewardType:    values[SettingKeyReferralInviterFirstChargeRewardType],
		InviterFirstChargeRewardValue:   parseFloat(values[SettingKeyReferralInviterFirstChargeRewardValue]),
		InviteeFirstChargeRewardEnabled: parseBool(values[SettingKeyReferralInviteeFirstChargeRewardEnabled]),
		InviteeFirstChargeRewardType:    values[SettingKeyReferralInviteeFirstChargeRewardType],
		InviteeFirstChargeRewardValue:   parseFloat(values[SettingKeyReferralInviteeFirstChargeRewardValue]),
		OngoingRewardEnabled:            parseBool(values[SettingKeyReferralOngoingRewardEnabled]),
		OngoingRewardType:               values[SettingKeyReferralOngoingRewardType],
		OngoingRewardValue:              parseFloat(values[SettingKeyReferralOngoingRewardValue]),
		OngoingRewardMaxCount:           parseInt(values[SettingKeyReferralOngoingRewardMaxCount]),
		OngoingRewardDurationDays:       parseInt(values[SettingKeyReferralOngoingRewardDurationDays]),
		// 普通推广：被邀请人持续奖励
		InviteeOngoingRewardEnabled:      parseBool(values[SettingKeyReferralInviteeOngoingRewardEnabled]),
		InviteeOngoingRewardType:         values[SettingKeyReferralInviteeOngoingRewardType],
		InviteeOngoingRewardValue:        parseFloat(values[SettingKeyReferralInviteeOngoingRewardValue]),
		InviteeOngoingRewardMaxCount:     parseInt(values[SettingKeyReferralInviteeOngoingRewardMaxCount]),
		InviteeOngoingRewardDurationDays: parseInt(values[SettingKeyReferralInviteeOngoingRewardDurationDays]),
		// 销售推广
		SalesEnabled:                          parseBool(values[SettingKeyReferralSalesEnabled]),
		SalesInviteeRewardEnabled:             parseBool(values[SettingKeyReferralSalesInviteeRewardEnabled]),
		SalesInviteeRewardAmount:              parseFloat(values[SettingKeyReferralSalesInviteeReward]),
		SalesInviteeFirstChargeRewardEnabled:  parseBool(values[SettingKeyReferralSalesInviteeFirstChargeRewardEnabled]),
		SalesInviteeFirstChargeRewardType:     values[SettingKeyReferralSalesInviteeFirstChargeRewardType],
		SalesInviteeFirstChargeRewardValue:    parseFloat(values[SettingKeyReferralSalesInviteeFirstChargeRewardValue]),
		SalesInviteeOngoingRewardEnabled:      parseBool(values[SettingKeyReferralSalesInviteeOngoingRewardEnabled]),
		SalesInviteeOngoingRewardType:         values[SettingKeyReferralSalesInviteeOngoingRewardType],
		SalesInviteeOngoingRewardValue:        parseFloat(values[SettingKeyReferralSalesInviteeOngoingRewardValue]),
		SalesInviteeOngoingRewardMaxCount:     parseInt(values[SettingKeyReferralSalesInviteeOngoingRewardMaxCount]),
		SalesInviteeOngoingRewardDurationDays: parseInt(values[SettingKeyReferralSalesInviteeOngoingRewardDurationDays]),
	}
	// 默认 type 为 fixed（空字符串、未设置时）
	if cfg.InviterFirstChargeRewardType == "" {
		cfg.InviterFirstChargeRewardType = "fixed"
	}
	if cfg.InviteeFirstChargeRewardType == "" {
		cfg.InviteeFirstChargeRewardType = "fixed"
	}
	if cfg.OngoingRewardType == "" {
		cfg.OngoingRewardType = "fixed"
	}
	if cfg.InviteeOngoingRewardType == "" {
		cfg.InviteeOngoingRewardType = "fixed"
	}
	if cfg.SalesInviteeFirstChargeRewardType == "" {
		cfg.SalesInviteeFirstChargeRewardType = "fixed"
	}
	if cfg.SalesInviteeOngoingRewardType == "" {
		cfg.SalesInviteeOngoingRewardType = "fixed"
	}
	return cfg
}

// mergeConfig 将用户覆盖配置合并到全局配置
func mergeConfig(global *ReferralGlobalConfig, user *UserReferralConfig) *EffectiveReferralConfig {
	eff := &EffectiveReferralConfig{
		Enabled:                          global.Enabled,
		InviteeRewardAmount:              global.InviteeRewardAmount,
		InviterRewardAmount:              global.InviterRewardAmount,
		MaxInvites:                       global.MaxInvites,
		RewardExpiryDays:                 global.RewardExpiryDays,
		InviterFirstChargeRewardEnabled:  global.InviterFirstChargeRewardEnabled,
		InviterFirstChargeRewardType:     global.InviterFirstChargeRewardType,
		InviterFirstChargeRewardValue:    global.InviterFirstChargeRewardValue,
		InviteeFirstChargeRewardEnabled:  global.InviteeFirstChargeRewardEnabled,
		InviteeFirstChargeRewardType:     global.InviteeFirstChargeRewardType,
		InviteeFirstChargeRewardValue:    global.InviteeFirstChargeRewardValue,
		OngoingRewardEnabled:             global.OngoingRewardEnabled,
		OngoingRewardType:                global.OngoingRewardType,
		OngoingRewardValue:               global.OngoingRewardValue,
		OngoingRewardMaxCount:            global.OngoingRewardMaxCount,
		OngoingRewardDurationDays:        global.OngoingRewardDurationDays,
		InviteeOngoingRewardEnabled:      global.InviteeOngoingRewardEnabled,
		InviteeOngoingRewardType:         global.InviteeOngoingRewardType,
		InviteeOngoingRewardValue:        global.InviteeOngoingRewardValue,
		InviteeOngoingRewardMaxCount:     global.InviteeOngoingRewardMaxCount,
		InviteeOngoingRewardDurationDays: global.InviteeOngoingRewardDurationDays,
	}
	if user == nil {
		return eff
	}
	if user.InviteeRewardAmount != nil {
		eff.InviteeRewardAmount = *user.InviteeRewardAmount
	}
	if user.InviterRewardAmount != nil {
		eff.InviterRewardAmount = *user.InviterRewardAmount
	}
	if user.MaxInvites != nil {
		eff.MaxInvites = *user.MaxInvites
	}
	if user.RewardExpiryDays != nil {
		eff.RewardExpiryDays = *user.RewardExpiryDays
	}
	if user.InviterFirstChargeRewardEnabled != nil {
		eff.InviterFirstChargeRewardEnabled = *user.InviterFirstChargeRewardEnabled
	}
	if user.InviterFirstChargeRewardType != nil && *user.InviterFirstChargeRewardType != "" {
		eff.InviterFirstChargeRewardType = *user.InviterFirstChargeRewardType
	}
	if user.InviterFirstChargeRewardValue != nil {
		eff.InviterFirstChargeRewardValue = *user.InviterFirstChargeRewardValue
	}
	if user.InviteeFirstChargeRewardEnabled != nil {
		eff.InviteeFirstChargeRewardEnabled = *user.InviteeFirstChargeRewardEnabled
	}
	if user.InviteeFirstChargeRewardType != nil && *user.InviteeFirstChargeRewardType != "" {
		eff.InviteeFirstChargeRewardType = *user.InviteeFirstChargeRewardType
	}
	if user.InviteeFirstChargeRewardValue != nil {
		eff.InviteeFirstChargeRewardValue = *user.InviteeFirstChargeRewardValue
	}
	if user.OngoingRewardEnabled != nil {
		eff.OngoingRewardEnabled = *user.OngoingRewardEnabled
	}
	if user.OngoingRewardType != nil && *user.OngoingRewardType != "" {
		eff.OngoingRewardType = *user.OngoingRewardType
	}
	if user.OngoingRewardValue != nil {
		eff.OngoingRewardValue = *user.OngoingRewardValue
	}
	if user.OngoingRewardMaxCount != nil {
		eff.OngoingRewardMaxCount = *user.OngoingRewardMaxCount
	}
	if user.OngoingRewardDurationDays != nil {
		eff.OngoingRewardDurationDays = *user.OngoingRewardDurationDays
	}
	if user.InviteeOngoingRewardEnabled != nil {
		eff.InviteeOngoingRewardEnabled = *user.InviteeOngoingRewardEnabled
	}
	if user.InviteeOngoingRewardType != nil && *user.InviteeOngoingRewardType != "" {
		eff.InviteeOngoingRewardType = *user.InviteeOngoingRewardType
	}
	if user.InviteeOngoingRewardValue != nil {
		eff.InviteeOngoingRewardValue = *user.InviteeOngoingRewardValue
	}
	if user.InviteeOngoingRewardMaxCount != nil {
		eff.InviteeOngoingRewardMaxCount = *user.InviteeOngoingRewardMaxCount
	}
	if user.InviteeOngoingRewardDurationDays != nil {
		eff.InviteeOngoingRewardDurationDays = *user.InviteeOngoingRewardDurationDays
	}
	return eff
}

// --- parse helpers ---

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
