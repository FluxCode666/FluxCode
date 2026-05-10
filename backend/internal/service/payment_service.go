package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// --- Order Status Constants ---

const (
	OrderStatusPending           = payment.OrderStatusPending
	OrderStatusPaid              = payment.OrderStatusPaid
	OrderStatusRecharging        = payment.OrderStatusRecharging
	OrderStatusCompleted         = payment.OrderStatusCompleted
	OrderStatusExpired           = payment.OrderStatusExpired
	OrderStatusCancelled         = payment.OrderStatusCancelled
	OrderStatusFailed            = payment.OrderStatusFailed
	OrderStatusRefundRequested   = payment.OrderStatusRefundRequested
	OrderStatusRefunding         = payment.OrderStatusRefunding
	OrderStatusPartiallyRefunded = payment.OrderStatusPartiallyRefunded
	OrderStatusRefunded          = payment.OrderStatusRefunded
	OrderStatusRefundFailed      = payment.OrderStatusRefundFailed
)

const (
	// defaultMaxPendingOrders and defaultOrderTimeoutMin are defined in
	// payment_config_service.go alongside other payment configuration defaults.
	paymentGraceMinutes = 5

	defaultPageSize    = 20
	maxPageSize        = 100
	topUsersLimit      = 10
	amountToleranceCNY = 0.01

	orderIDPrefix = "sub2_"

	paymentFulfillmentTimeout = 30 * time.Second
	paymentStateUpdateTimeout = 10 * time.Second
)

// --- Types ---

// generateOutTradeNo creates a unique external order ID for payment providers.
// Format: sub2_20250409aB3kX9mQ (prefix + date + 8-char random)
func generateOutTradeNo() string {
	date := time.Now().Format("20060102")
	rnd := generateRandomString(8)
	return orderIDPrefix + date + rnd
}

func generateRandomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

type CreateOrderRequest struct {
	UserID           int64
	Amount           float64
	PaymentType      string
	ClientIP         string
	IsMobile         bool
	SrcHost          string
	SrcURL           string
	OrderType        string
	PlanID           int64
	PromotionID      int64  // 用户选择的促销活动 ID（0 表示不使用）
	SubscriptionMode string // extend / stack（订阅续费模式，仅订阅订单有效）
}

type CreateOrderResponse struct {
	OrderID      int64     `json:"order_id"`
	Amount       float64   `json:"amount"`
	PayAmount    float64   `json:"pay_amount"`
	FeeRate      float64   `json:"fee_rate"`
	Status       string    `json:"status"`
	PaymentType  string    `json:"payment_type"`
	PayURL       string    `json:"pay_url,omitempty"`
	QRCode       string    `json:"qr_code,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	PaymentMode  string    `json:"payment_mode,omitempty"`

	// 促销活动信息（命中时填充）
	OriginalAmount float64 `json:"original_amount"`
	DiscountAmount float64 `json:"discount_amount"`
	BonusAmount    float64 `json:"bonus_amount"`
	PromotionID    *int64  `json:"promotion_id,omitempty"`
	PromotionName  string  `json:"promotion_name,omitempty"`
	PromotionMode  string  `json:"promotion_mode,omitempty"`
}

type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
	Keyword     string
}

type RefundPlan struct {
	OrderID         int64
	Order           *dbent.PaymentOrder
	RefundAmount    float64
	GatewayAmount   float64
	Reason          string
	Force           bool
	DeductBalance   bool
	DeductionType   string
	BalanceToDeduct float64
	SubDaysToDeduct int
	SubscriptionID  int64
}

type RefundResult struct {
	Success         bool    `json:"success"`
	Warning         string  `json:"warning,omitempty"`
	RequireForce    bool    `json:"require_force,omitempty"`
	BalanceDeducted float64 `json:"balance_deducted,omitempty"`
	SubDaysDeducted int     `json:"subscription_days_deducted,omitempty"`
}

type DashboardStats struct {
	TodayAmount   float64 `json:"today_amount"`
	TotalAmount   float64 `json:"total_amount"`
	TodayCount    int     `json:"today_count"`
	TotalCount    int     `json:"total_count"`
	AvgAmount     float64 `json:"avg_amount"`
	PendingOrders int     `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       []TopUserStat       `json:"top_users"`
}

type DailyStats struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

type PaymentMethodStat struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

type TopUserStat struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Amount float64 `json:"amount"`
}

// --- Service ---

type PaymentService struct {
	providerMu             sync.Mutex
	providersLoaded        bool
	entClient              *dbent.Client
	registry               *payment.Registry
	loadBalancer           payment.LoadBalancer
	redeemService          *RedeemService
	subscriptionSvc        *SubscriptionService
	configService          *PaymentConfigService
	userRepo               UserRepository
	groupRepo              GroupRepository
	referralService        *ReferralService
	salesCommissionService *SalesCommissionService
	promotionRepo          PromotionRepository
	promotionResolver      *PromotionResolver
}

// SetReferralService 注入推广奖励服务（避免循环依赖）
func (s *PaymentService) SetReferralService(svc *ReferralService) {
	s.referralService = svc
}

func (s *PaymentService) SetSalesCommissionService(svc *SalesCommissionService) {
	s.salesCommissionService = svc
}

func NewPaymentService(
	entClient *dbent.Client,
	registry *payment.Registry,
	loadBalancer payment.LoadBalancer,
	redeemService *RedeemService,
	subscriptionSvc *SubscriptionService,
	configService *PaymentConfigService,
	userRepo UserRepository,
	groupRepo GroupRepository,
	promotionRepo PromotionRepository,
	promotionResolver *PromotionResolver,
) *PaymentService {
	return &PaymentService{
		entClient:         entClient,
		registry:          registry,
		loadBalancer:      loadBalancer,
		redeemService:     redeemService,
		subscriptionSvc:   subscriptionSvc,
		configService:     configService,
		userRepo:          userRepo,
		groupRepo:         groupRepo,
		promotionRepo:     promotionRepo,
		promotionResolver: promotionResolver,
	}
}

// ListAvailablePromotions 列出用户当前可用的促销活动
func (s *PaymentService) ListAvailablePromotions(ctx context.Context, userID int64, orderType string, planID int64) ([]AvailablePromotion, error) {
	if s.promotionResolver == nil {
		return nil, nil
	}
	switch orderType {
	case payment.OrderTypeSubscription:
		return s.promotionResolver.ListAvailableForSubscription(ctx, userID, planID)
	default:
		return s.promotionResolver.ListAvailableForRecharge(ctx, userID)
	}
}

// --- Provider Registry ---

// EnsureProviders lazily initializes the provider registry on first call.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if !s.providersLoaded {
		s.loadProviders(ctx)
		s.providersLoaded = true
	}
}

// RefreshProviders clears and re-registers all providers from the database.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.registry.Clear()
	s.loadProviders(ctx)
	s.providersLoaded = true
}

func (s *PaymentService) loadProviders(ctx context.Context) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		slog.Error("[PaymentService] failed to query provider instances", "error", err)
		return
	}
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			slog.Warn("[PaymentService] failed to decrypt config for instance", "instanceID", inst.ID, "error", err)
			continue
		}
		if inst.PaymentMode != "" {
			cfg["paymentMode"] = inst.PaymentMode
		}
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			slog.Warn("[PaymentService] failed to create provider for instance", "instanceID", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
		}
		s.registry.Register(p)
	}
}

// GetWebhookProvider returns the provider instance that should verify a webhook.
// It extracts out_trade_no from the raw body, looks up the order to find the
// original provider instance, and creates a provider with that instance's credentials.
// Falls back to the registry provider when the order cannot be found.
func (s *PaymentService) GetWebhookProvider(ctx context.Context, providerKey, outTradeNo string) (payment.Provider, error) {
	if outTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
		if err == nil {
			p, pErr := s.getOrderProvider(ctx, order)
			if pErr == nil {
				return p, nil
			}
			slog.Warn("[Webhook] order provider creation failed, falling back to registry", "outTradeNo", outTradeNo, "error", pErr)
		}
	}
	s.EnsureProviders(ctx)
	return s.registry.GetProviderByKey(providerKey)
}

// GetWebhookProviderCandidates returns all providers that may verify the webhook.
// The original order's provider instance is tried first when outTradeNo is known,
// then all enabled instances of the provider key are tried as a fallback.
func (s *PaymentService) GetWebhookProviderCandidates(ctx context.Context, providerKey, outTradeNo string) ([]payment.Provider, error) {
	candidates := make([]payment.Provider, 0, 4)

	if outTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
		if err == nil {
			if p, pErr := s.getOrderProvider(ctx, order); pErr == nil {
				candidates = append(candidates, p)
			} else {
				slog.Warn("[Webhook] failed to load order provider candidate", "outTradeNo", outTradeNo, "error", pErr)
			}
		}
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Order(dbent.Asc(paymentproviderinstance.FieldSortOrder), dbent.Asc(paymentproviderinstance.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
	}
	for _, inst := range instances {
		cfg, cfgErr := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if cfgErr != nil {
			slog.Warn("[Webhook] failed to load provider config", "provider", providerKey, "instanceID", inst.ID, "error", cfgErr)
			continue
		}
		p, createErr := createProviderForInstance(inst, cfg)
		if createErr != nil {
			slog.Warn("[Webhook] failed to create provider candidate", "provider", providerKey, "instanceID", inst.ID, "error", createErr)
			continue
		}
		candidates = append(candidates, p)
	}

	if len(candidates) > 0 {
		return candidates, nil
	}

	s.EnsureProviders(ctx)
	p, err := s.registry.GetProviderByKey(providerKey)
	if err != nil {
		return nil, err
	}
	return []payment.Provider{p}, nil
}

// --- Helpers ---

func psIsRefundStatus(s string) bool {
	switch s {
	case OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return true
	}
	return false
}

func psErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

func paymentDetachedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, timeout)
}

func createProviderForInstance(inst *dbent.PaymentProviderInstance, cfg map[string]string) (payment.Provider, error) {
	if inst == nil {
		return nil, fmt.Errorf("provider instance is nil")
	}
	copied := make(map[string]string, len(cfg)+1)
	for k, v := range cfg {
		copied[k] = v
	}
	if inst.PaymentMode != "" {
		copied["paymentMode"] = inst.PaymentMode
	}
	return provider.CreateProvider(inst.ProviderKey, strconv.FormatInt(inst.ID, 10), copied)
}

// Subscription validity period unit constants.
const (
	validityUnitWeek  = "week"
	validityUnitMonth = "month"
)

func psComputeValidityDays(days int, unit string) int {
	switch unit {
	case validityUnitWeek:
		return days * 7
	case validityUnitMonth:
		return days * 30
	default:
		return days
	}
}

func psStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func applyPagination(pageSize, page int) (size, pg int) {
	size = pageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	pg = page
	if pg < 1 {
		pg = 1
	}
	return size, pg
}
