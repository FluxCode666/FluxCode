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
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Payment Notification & Fulfillment ---

func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	if n.Status != payment.NotificationStatusSuccess {
		return nil
	}
	// Look up order by out_trade_no (the external order ID we sent to the provider)
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		// Fallback: try legacy format (sub2_N where N is DB ID)
		trimmed := strings.TrimPrefix(n.OrderID, orderIDPrefix)
		if oid, parseErr := strconv.ParseInt(trimmed, 10, 64); parseErr == nil {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk)
		}
		return fmt.Errorf("order not found for out_trade_no: %s", n.OrderID)
	}
	return s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, pk)
}

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		slog.Error("order not found", "orderID", oid)
		return nil
	}
	// Skip amount check when paid=0 (e.g. QueryOrder doesn't return amount).
	// Also skip if paid is NaN/Inf (malformed provider data).
	if paid > 0 && !math.IsNaN(paid) && !math.IsInf(paid, 0) {
		if math.Abs(paid-o.PayAmount) > amountToleranceCNY {
			s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
			return fmt.Errorf("amount mismatch: expected %.2f, got %.2f", o.PayAmount, paid)
		}
	}
	// Use order's expected amount when provider didn't report one
	if paid <= 0 || math.IsNaN(paid) || math.IsInf(paid, 0) {
		paid = o.PayAmount
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, pk string) error {
	previousStatus := o.Status
	now := time.Now()
	grace := now.Add(-paymentGraceMinutes * time.Minute)
	c, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(o.ID),
		paymentorder.Or(
			paymentorder.StatusEQ(OrderStatusPending),
			paymentorder.StatusEQ(OrderStatusCancelled),
			paymentorder.And(
				paymentorder.StatusEQ(OrderStatusExpired),
				paymentorder.UpdatedAtGTE(grace),
			),
		),
	).SetStatus(OrderStatusPaid).SetPayAmount(paid).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o)
	}
	if previousStatus == OrderStatusCancelled || previousStatus == OrderStatusExpired {
		slog.Info("order recovered from webhook payment success",
			"orderID", o.ID,
			"previousStatus", previousStatus,
			"tradeNo", tradeNo,
			"provider", pk,
		)
		s.writeAuditLog(ctx, o.ID, "ORDER_RECOVERED", pk, map[string]any{
			"previous_status": previousStatus,
			"tradeNo":         tradeNo,
			"paidAmount":      paid,
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid})
	return s.executeFulfillment(ctx, o.ID)
}

func (s *PaymentService) alreadyProcessed(ctx context.Context, o *dbent.PaymentOrder) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded:
		return nil
	case OrderStatusFailed:
		return s.executeFulfillment(ctx, o.ID)
	case OrderStatusPaid, OrderStatusRecharging:
		return fmt.Errorf("order %d is being processed", o.ID)
	case OrderStatusExpired:
		slog.Warn("webhook payment success for expired order beyond grace period",
			"orderID", o.ID,
			"status", cur.Status,
			"updatedAt", cur.UpdatedAt,
		)
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AFTER_EXPIRY", "system", map[string]any{
			"status":    cur.Status,
			"updatedAt": cur.UpdatedAt,
			"reason":    "payment arrived after expiry grace period",
		})
		return nil
	default:
		return nil
	}
}

func (s *PaymentService) executeFulfillment(ctx context.Context, oid int64) error {
	execCtx, cancel := paymentDetachedContext(ctx, paymentFulfillmentTimeout)
	defer cancel()

	o, err := s.entClient.PaymentOrder.Get(execCtx, oid)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if o.OrderType == payment.OrderTypeSubscription {
		return s.ExecuteSubscriptionFulfillment(execCtx, oid)
	}
	return s.ExecuteBalanceFulfillment(execCtx, oid)
}

func (s *PaymentService) ExecuteBalanceFulfillment(ctx context.Context, oid int64) error {
	fulfillCtx, cancel := paymentDetachedContext(ctx, paymentFulfillmentTimeout)
	defer cancel()

	o, err := s.entClient.PaymentOrder.Get(fulfillCtx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(fulfillCtx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doBalance(fulfillCtx, o); err != nil {
		s.markFailed(fulfillCtx, oid, err)
		return err
	}
	return nil
}

// redeemAction represents the idempotency decision for balance fulfillment.
type redeemAction int

const (
	// redeemActionCreate: code does not exist — create it, then redeem.
	redeemActionCreate redeemAction = iota
	// redeemActionRedeem: code exists but is unused — skip creation, redeem only.
	redeemActionRedeem
	// redeemActionSkipCompleted: code exists and is already used — skip to mark completed.
	redeemActionSkipCompleted
)

// resolveRedeemAction decides the idempotency action based on an existing redeem code lookup.
// existing is the result of GetByCode; lookupErr is the error from that call.
func resolveRedeemAction(existing *RedeemCode, lookupErr error) redeemAction {
	if existing == nil || lookupErr != nil {
		return redeemActionCreate
	}
	if existing.IsUsed() {
		return redeemActionSkipCompleted
	}
	return redeemActionRedeem
}

func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder) error {
	// Idempotency: check if redeem code already exists (from a previous partial run)
	existing, lookupErr := s.redeemService.GetByCode(ctx, o.RechargeCode)
	action := resolveRedeemAction(existing, lookupErr)

	switch action {
	case redeemActionSkipCompleted:
		// Code already created and redeemed — just mark completed
		return s.markCompleted(ctx, o, "RECHARGE_SUCCESS")
	case redeemActionCreate:
		rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(ctx, rc); err != nil {
			return fmt.Errorf("create redeem code: %w", err)
		}
	case redeemActionRedeem:
		// Code exists but unused — skip creation, proceed to redeem
	}
	// 在 Redeem（会增加 TotalRecharged）之前判断是否首充
	var isFirstRecharge bool
	if s.referralService != nil {
		isFirstRecharge, _ = s.userRepo.IsFirstRecharge(ctx, o.UserID)
	}

	if _, err := s.redeemService.Redeem(ctx, o.UserID, o.RechargeCode, ""); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	if err := s.markCompleted(ctx, o, "RECHARGE_SUCCESS"); err != nil {
		return err
	}

	// 充值完成后触发推广奖励
	if s.referralService != nil {
		if isFirstRecharge {
			s.referralService.HandleInviterRewardOnFirstRecharge(ctx, o.UserID, o.Amount)
		}
		s.referralService.HandleOngoingRewardOnRecharge(ctx, o.UserID, o.Amount)
	}

	return nil
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, auditAction string) error {
	now := time.Now()
	_, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, auditAction, "system", map[string]any{
		"rechargeCode":   o.RechargeCode,
		"creditedAmount": o.Amount,
		"payAmount":      o.PayAmount,
	})
	return nil
}

func (s *PaymentService) ExecuteSubscriptionFulfillment(ctx context.Context, oid int64) error {
	fulfillCtx, cancel := paymentDetachedContext(ctx, paymentFulfillmentTimeout)
	defer cancel()

	o, err := s.entClient.PaymentOrder.Get(fulfillCtx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(fulfillCtx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doSub(fulfillCtx, o); err != nil {
		s.markFailed(fulfillCtx, oid, err)
		return err
	}
	return nil
}

func (s *PaymentService) doSub(ctx context.Context, o *dbent.PaymentOrder) error {
	gid := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	g, err := s.groupRepo.GetByID(ctx, gid)
	if err != nil || g.Status != payment.EntityStatusActive {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	// Idempotency: check audit log to see if subscription was already assigned.
	// Prevents double-extension on retry after markCompleted fails.
	if s.hasAuditLog(ctx, o.ID, "SUBSCRIPTION_SUCCESS") {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", gid)
		return s.markCompleted(ctx, o, "SUBSCRIPTION_SUCCESS")
	}
	orderNote := fmt.Sprintf("payment order %d", o.ID)
	_, _, err = s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: o.UserID, GroupID: gid, ValidityDays: days, AssignedBy: 0, Notes: orderNote})
	if err != nil {
		return fmt.Errorf("assign subscription: %w", err)
	}
	return s.markCompleted(ctx, o, "SUBSCRIPTION_SUCCESS")
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	oid := strconv.FormatInt(orderID, 10)
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0
}

func (s *PaymentService) markFailed(ctx context.Context, oid int64, cause error) {
	failCtx, cancel := paymentDetachedContext(ctx, paymentStateUpdateTimeout)
	defer cancel()

	now := time.Now()
	r := psErrMsg(cause)
	// Only mark FAILED if still in RECHARGING state — prevents overwriting
	// a COMPLETED order when markCompleted failed but fulfillment succeeded.
	c, e := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(oid), paymentorder.StatusEQ(OrderStatusRecharging)).
		SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason(r).Save(failCtx)
	if e != nil {
		slog.Error("mark FAILED", "orderID", oid, "error", e)
	}
	if c > 0 {
		s.writeAuditLog(failCtx, oid, "FULFILLMENT_FAILED", "system", map[string]any{"reason": r})
	}
}

func (s *PaymentService) RetryFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.PaidAt == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "order is not paid")
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot retry")
	}
	if o.Status == OrderStatusRecharging {
		return infraerrors.Conflict("CONFLICT", "order is being processed")
	}
	if o.Status == OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "order already completed")
	}
	if o.Status != OrderStatusFailed && o.Status != OrderStatusPaid {
		return infraerrors.BadRequest("INVALID_STATUS", "only paid and failed orders can retry")
	}
	_, err = s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusFailed, OrderStatusPaid)).SetStatus(OrderStatusPaid).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("reset for retry: %w", err)
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}
