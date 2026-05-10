package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Order Creation ---

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}
	if err := s.checkCancelRateLimit(ctx, req.UserID, cfg); err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}

	comp, err := s.computeOrderAmounts(ctx, req, plan, cfg)
	if err != nil {
		return nil, err
	}
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, comp)
	if err != nil {
		return nil, err
	}
	resp, err := s.invokeProvider(ctx, order, req, cfg, comp, plan)
	if err != nil {
		_, _ = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(OrderStatusFailed).
			Save(ctx)
		return nil, err
	}
	return resp, nil
}

// orderAmountComputation 是 CreateOrder 内部的金额聚合，避免参数过多。
type orderAmountComputation struct {
	OriginalAmount  float64               // 折前金额：plan.Price 或用户输入
	OrderAmount     float64               // 订单到账金额（充值=到账余额；订阅=plan 折后价）
	LimitAmount     float64               // 用于 daily limit 检查 / Subject 显示
	PaymentBase     float64               // 用户实际需要支付的金额（手续费前）
	PayAmount       float64               // PaymentBase 加上手续费后的实付（2 位小数）
	PayAmountStr    string                // PayAmount 的 2 位小数字符串
	FeeRate         float64               // 手续费率
	DiscountAmount  float64               // 节省金额
	BonusAmount     float64               // 加送金额
	PromotionID     *int64                // 命中活动
	PromotionRuleID *int64                // 订阅子规则
	RechargeHit     *RechargeDiscount     // 仅命中充值活动时非 nil
	SubscriptionHit *SubscriptionDiscount // 仅命中订阅活动时非 nil
}

// computeOrderAmounts 计算订单的所有金额字段，并尝试匹配最优促销活动。
func (s *PaymentService) computeOrderAmounts(ctx context.Context, req CreateOrderRequest, plan *dbent.SubscriptionPlan, cfg *PaymentConfig) (*orderAmountComputation, error) {
	comp := &orderAmountComputation{
		FeeRate: cfg.RechargeFeeRate,
	}

	switch {
	case plan != nil:
		comp.OriginalAmount = plan.Price
		comp.OrderAmount = plan.Price
		comp.LimitAmount = plan.Price
		comp.PaymentBase = plan.Price
		if s.promotionResolver != nil {
			hit, err := s.promotionResolver.ResolveSubscriptionDiscount(ctx, req.UserID, plan.ID, plan.Price, req.PromotionID)
			if err != nil {
				return nil, fmt.Errorf("resolve subscription promotion: %w", err)
			}
			if hit != nil {
				comp.OrderAmount = hit.FinalPrice
				comp.LimitAmount = hit.FinalPrice
				comp.PaymentBase = hit.FinalPrice
				comp.DiscountAmount = hit.DiscountAmount
				id := hit.Promotion.ID
				rid := hit.Rule.ID
				comp.PromotionID = &id
				comp.PromotionRuleID = &rid
				comp.SubscriptionHit = hit
			}
		}
	case req.OrderType == payment.OrderTypeBalance:
		comp.OriginalAmount = req.Amount
		comp.LimitAmount = req.Amount
		comp.PaymentBase = req.Amount
		comp.OrderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
		if s.promotionResolver != nil {
			hit, err := s.promotionResolver.ResolveRechargeDiscount(ctx, req.UserID, req.Amount, req.PromotionID)
			if err != nil {
				return nil, fmt.Errorf("resolve recharge promotion: %w", err)
			}
			if hit != nil {
				switch hit.Mode {
				case domain.PromotionDiscountModeReducePay:
					comp.PaymentBase = hit.PaymentAmount
					comp.DiscountAmount = hit.DiscountAmount
				case domain.PromotionDiscountModeBonusCredit:
					comp.OrderAmount = hit.CreditedAmount
					comp.BonusAmount = hit.BonusAmount
				}
				id := hit.Promotion.ID
				comp.PromotionID = &id
				comp.RechargeHit = hit
			}
		}
	default:
		comp.OriginalAmount = req.Amount
		comp.OrderAmount = req.Amount
		comp.LimitAmount = req.Amount
		comp.PaymentBase = req.Amount
	}

	comp.PayAmountStr = payment.CalculatePayAmount(comp.PaymentBase, comp.FeeRate)
	comp.PayAmount, _ = strconv.ParseFloat(comp.PayAmountStr, 64)
	return comp, nil
}

func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*dbent.SubscriptionPlan, error) {
	if req.OrderType == payment.OrderTypeBalance && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
	}
	if req.OrderType == payment.OrderTypeSubscription {
		return s.validateSubOrder(ctx, req)
	}
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	if (cfg.MinAmount > 0 && req.Amount < cfg.MinAmount) || (cfg.MaxAmount > 0 && req.Amount > cfg.MaxAmount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{"min": fmt.Sprintf("%.2f", cfg.MinAmount), "max": fmt.Sprintf("%.2f", cfg.MaxAmount)})
	}
	return nil, nil
}

func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*dbent.SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
	}
	plan, err := s.configService.GetPlan(ctx, req.PlanID)
	if err != nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
	}
	group, err := s.groupRepo.GetByID(ctx, plan.GroupID)
	if err != nil || group.Status != payment.EntityStatusActive {
		return nil, infraerrors.NotFound("GROUP_NOT_FOUND", "subscription group is no longer available")
	}
	if !group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_TYPE_MISMATCH", "group is not a subscription type")
	}
	return plan, nil
}

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, comp *orderAmountComputation) (*dbent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
	}
	if err := s.checkDailyLimit(ctx, tx, req.UserID, comp.LimitAmount, cfg.DailyLimit); err != nil {
		return nil, err
	}
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = defaultOrderTimeoutMin
	}
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(comp.OrderAmount).
		SetPayAmount(comp.PayAmount).
		SetFeeRate(comp.FeeRate).
		SetRechargeCode("").
		SetOutTradeNo(generateOutTradeNo(cfg.OrderIDPrefix)).
		SetPaymentType(req.PaymentType).
		SetPaymentTradeNo("").
		SetOrderType(req.OrderType).
		SetStatus(OrderStatusPending).
		SetExpiresAt(exp).
		SetClientIP(req.ClientIP).
		SetSrcHost(req.SrcHost).
		SetOriginalAmount(comp.OriginalAmount).
		SetDiscountAmount(comp.DiscountAmount).
		SetBonusAmount(comp.BonusAmount).
		SetNillablePromotionID(comp.PromotionID).
		SetNillablePromotionRuleID(comp.PromotionRuleID)
	if req.SrcURL != "" {
		b.SetSrcURL(req.SrcURL)
	}
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
	}
	if req.SubscriptionMode != "" {
		b.SetSubscriptionMode(req.SubscriptionMode)
	}
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	code := fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)
	order, err = tx.PaymentOrder.UpdateOneID(order.ID).SetRechargeCode(code).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set recharge code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}
	return order, nil
}

func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *dbent.Tx, userID int64, max int) error {
	if max <= 0 {
		max = defaultMaxPendingOrders
	}
	c, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
	}
	if c >= max {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", fmt.Sprintf("too many pending orders (max %d)", max)).
			WithMetadata(map[string]string{"max": strconv.Itoa(max)})
	}
	return nil
}

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
	}
	ts := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted), paymentorder.PaidAtGTE(ts)).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
	}
	var used float64
	for _, o := range orders {
		if o.OrderType == payment.OrderTypeBalance {
			used += o.PayAmount
			continue
		}
		used += o.Amount
	}
	if used+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", fmt.Sprintf("daily recharge limit reached, remaining: %.2f", math.Max(0, limit-used)))
	}
	return nil
}

func (s *PaymentService) invokeProvider(ctx context.Context, order *dbent.PaymentOrder, req CreateOrderRequest, cfg *PaymentConfig, comp *orderAmountComputation, plan *dbent.SubscriptionPlan) (*CreateOrderResponse, error) {
	// Select an instance across all providers that support the requested payment type.
	// This enables cross-provider load balancing (e.g. EasyPay + Alipay direct for "alipay").
	sel, err := s.loadBalancer.SelectInstance(ctx, "", req.PaymentType, payment.Strategy(cfg.LoadBalanceStrategy), comp.PayAmount)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment method (%s) is not configured", req.PaymentType))
	}
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no available payment instance")
	}
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment method is temporarily unavailable")
	}
	subject := s.buildPaymentSubject(plan, comp.LimitAmount, cfg)
	outTradeNo := order.OutTradeNo
	pr, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: outTradeNo, Amount: comp.PayAmountStr, PaymentType: req.PaymentType, Subject: subject, ClientIP: req.ClientIP, IsMobile: req.IsMobile, InstanceSubMethods: sel.SupportedTypes})
	if err != nil {
		slog.Error("[PaymentService] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
	}
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).SetNillablePayURL(psNilIfEmpty(pr.PayURL)).SetNillableQrCode(psNilIfEmpty(pr.QRCode)).SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update order with payment details: %w", err)
	}
	auditDetail := map[string]any{
		"paymentAmount":  req.Amount,
		"creditedAmount": order.Amount,
		"payAmount":      order.PayAmount,
		"paymentType":    req.PaymentType,
		"orderType":      req.OrderType,
		"originalAmount": comp.OriginalAmount,
		"discountAmount": comp.DiscountAmount,
		"bonusAmount":    comp.BonusAmount,
	}
	if comp.PromotionID != nil {
		auditDetail["promotionId"] = *comp.PromotionID
	}
	if comp.PromotionRuleID != nil {
		auditDetail["promotionRuleId"] = *comp.PromotionRuleID
	}
	s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), auditDetail)
	resp := &CreateOrderResponse{OrderID: order.ID, Amount: order.Amount, PayAmount: comp.PayAmount, FeeRate: order.FeeRate, Status: OrderStatusPending, PaymentType: req.PaymentType, PayURL: pr.PayURL, QRCode: pr.QRCode, ClientSecret: pr.ClientSecret, ExpiresAt: order.ExpiresAt, PaymentMode: sel.PaymentMode}
	resp.OriginalAmount = comp.OriginalAmount
	resp.DiscountAmount = comp.DiscountAmount
	resp.BonusAmount = comp.BonusAmount
	if comp.PromotionID != nil {
		resp.PromotionID = comp.PromotionID
	}
	if comp.SubscriptionHit != nil {
		resp.PromotionName = comp.SubscriptionHit.Promotion.Name
		resp.PromotionMode = comp.SubscriptionHit.Rule.DiscountMode
	} else if comp.RechargeHit != nil {
		resp.PromotionName = comp.RechargeHit.Promotion.Name
		resp.PromotionMode = comp.RechargeHit.Mode
	}
	return resp, nil
}

func (s *PaymentService) buildPaymentSubject(plan *dbent.SubscriptionPlan, limitAmount float64, cfg *PaymentConfig) string {
	if plan != nil {
		if plan.ProductName != "" {
			return plan.ProductName
		}
		return "Sub2API Subscription " + plan.Name
	}
	amountStr := strconv.FormatFloat(limitAmount, 'f', 2, 64)
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	if pf != "" || sf != "" {
		return strings.TrimSpace(pf + " " + amountStr + " " + sf)
	}
	return "Sub2API " + amountStr + " CNY"
}

// PromotionPreview 下单前折扣预览结果（不写入任何状态）。
type PromotionPreview struct {
	Hit            bool
	PromotionID    *int64
	PromotionRule  *int64
	PromotionName  string
	PromotionMode  string // 充值：reduce_pay / bonus_credit；订阅：rate / amount
	OriginalAmount float64
	PaymentAmount  float64 // 用户实际需要支付（不含手续费）
	CreditedAmount float64 // 到账金额
	DiscountAmount float64
	BonusAmount    float64
}

// PreviewPromotion 在用户下单前按当前参数计算折扣预览。不做 daily-limit 等事务性校验。
//
// 对充值订单：req.Amount 必须 > 0；对订阅订单：req.PlanID 必须有效。
func (s *PaymentService) PreviewPromotion(ctx context.Context, req CreateOrderRequest) (*PromotionPreview, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}

	var plan *dbent.SubscriptionPlan
	switch req.OrderType {
	case payment.OrderTypeSubscription:
		if req.PlanID == 0 {
			return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription preview requires plan_id")
		}
		p, err := s.configService.GetPlan(ctx, req.PlanID)
		if err != nil || !p.ForSale {
			return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
		}
		plan = p
	case payment.OrderTypeBalance:
		if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 {
			return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
		}
	default:
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "unsupported order_type")
	}

	comp, err := s.computeOrderAmounts(ctx, req, plan, cfg)
	if err != nil {
		return nil, err
	}

	preview := &PromotionPreview{
		OriginalAmount: comp.OriginalAmount,
		PaymentAmount:  comp.PaymentBase,
		CreditedAmount: comp.OrderAmount,
		DiscountAmount: comp.DiscountAmount,
		BonusAmount:    comp.BonusAmount,
	}
	if comp.PromotionID != nil {
		preview.Hit = true
		preview.PromotionID = comp.PromotionID
	}
	if comp.PromotionRuleID != nil {
		preview.PromotionRule = comp.PromotionRuleID
	}
	if comp.SubscriptionHit != nil {
		preview.PromotionName = comp.SubscriptionHit.Promotion.Name
		preview.PromotionMode = comp.SubscriptionHit.Rule.DiscountMode
	} else if comp.RechargeHit != nil {
		preview.PromotionName = comp.RechargeHit.Promotion.Name
		preview.PromotionMode = comp.RechargeHit.Mode
	}
	return preview, nil
}

// --- Order Queries ---

func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
	}
	return orders, total, nil
}

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
	}
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
		))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	return orders, total, nil
}
