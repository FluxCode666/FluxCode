package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	MediaBillingStatusPending    = "pending"
	MediaBillingStatusPrecharged = "precharged"
	MediaBillingStatusSettling   = "settling"
	MediaBillingStatusRetry      = "retry"
	MediaBillingStatusSettled    = "settled"
)

var (
	ErrMediaSettlementPlanConflict = errors.New("media settlement plan conflict")
	ErrMediaSettlementCASConflict  = errors.New("media settlement CAS conflict")
)

type MediaBillingSnapshot struct {
	RequestedModel  string          `json:"requested_model"`
	CandidateModels []string        `json:"candidate_models"`
	EstimatedAmount float64         `json:"estimated_amount"`
	GroupMultiplier float64         `json:"group_multiplier"`
	PricingSnapshot json.RawMessage `json:"pricing_snapshot"`
}

type MediaFailureKind string

const (
	MediaFailureKindUpstream    MediaFailureKind = "upstream"
	MediaFailureKindSystem      MediaFailureKind = "system"
	MediaFailureKindSyncTimeout MediaFailureKind = "sync_timeout"
)

type MediaFailureSettlement struct {
	Kind         MediaFailureKind
	RefundRatio  float64
	PenaltyRatio float64
	ErrorCode    string
}

type MediaBillingPort interface {
	Precharge(ctx context.Context, task *MediaTask, snapshot MediaBillingSnapshot) error
	SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error
	SettleFailure(ctx context.Context, task *MediaTask, settlement MediaFailureSettlement) error
}

type MediaSettlementType string

const (
	MediaSettlementTypeSuccess MediaSettlementType = "success"
	MediaSettlementTypeFailure MediaSettlementType = "failure"
)

type MediaSettlementPlan struct {
	Type    MediaSettlementType     `json:"type"`
	Usage   *MediaUsage             `json:"usage,omitempty"`
	Failure *MediaFailureSettlement `json:"failure,omitempty"`
}

type MediaSettlementCoordinator interface {
	SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error
	SettleFailure(ctx context.Context, task *MediaTask, settlement MediaFailureSettlement) error
	RetryPending(ctx context.Context, taskID int64) error
}

type MediaBillingCoordinator struct {
	repo MediaTaskRepository
	port MediaBillingPort
}

func NewMediaBillingCoordinator(repo MediaTaskRepository, port MediaBillingPort) *MediaBillingCoordinator {
	return &MediaBillingCoordinator{repo: repo, port: port}
}

func (c *MediaBillingCoordinator) SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error {
	plan := MediaSettlementPlan{Type: MediaSettlementTypeSuccess, Usage: &usage}
	return c.settle(ctx, task, plan)
}

func (c *MediaBillingCoordinator) SettleFailure(ctx context.Context, task *MediaTask, settlement MediaFailureSettlement) error {
	plan := MediaSettlementPlan{Type: MediaSettlementTypeFailure, Failure: &settlement}
	return c.settle(ctx, task, plan)
}

func (c *MediaBillingCoordinator) RetryPending(ctx context.Context, taskID int64) error {
	if err := c.validate(); err != nil {
		return err
	}
	task, err := c.repo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load media task %d for settlement retry: %w", taskID, err)
	}
	if task.BillingStatus == MediaBillingStatusSettled {
		return nil
	}
	plan, err := decodeMediaSettlementPlan(task.SettlementPlan)
	if err != nil {
		return fmt.Errorf("decode media task %d settlement plan: %w", taskID, err)
	}
	return c.executePersisted(ctx, task, plan)
}

func (c *MediaBillingCoordinator) settle(ctx context.Context, task *MediaTask, plan MediaSettlementPlan) error {
	if err := c.validate(); err != nil {
		return err
	}
	if task == nil || task.ID <= 0 || task.PublicID == "" {
		return errors.New("media settlement task is invalid")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encode media task %d settlement plan: %w", task.ID, err)
	}

	current, err := c.repo.GetByID(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("load media task %d before settlement: %w", task.ID, err)
	}
	if current.BillingStatus == MediaBillingStatusSettled {
		return nil
	}
	if len(current.SettlementPlan) > 0 && !bytes.Equal(current.SettlementPlan, encoded) {
		return fmt.Errorf("%w: task %d", ErrMediaSettlementPlanConflict, task.ID)
	}
	if len(current.SettlementPlan) == 0 {
		updated, updateErr := c.repo.UpdateBilling(ctx, current.ID, current.BillingStatus, map[string]any{
			"settlement_plan": json.RawMessage(encoded),
			"billing_status":  MediaBillingStatusSettling,
		})
		if updateErr != nil {
			return fmt.Errorf("persist media task %d settlement plan: %w", current.ID, updateErr)
		}
		if !updated {
			return fmt.Errorf("%w: persist task %d settlement plan", ErrMediaSettlementCASConflict, current.ID)
		}
		current.SettlementPlan = encoded
		current.BillingStatus = MediaBillingStatusSettling
	}
	return c.executePersisted(ctx, current, plan)
}

func (c *MediaBillingCoordinator) executePersisted(ctx context.Context, task *MediaTask, plan MediaSettlementPlan) error {
	if task.BillingStatus == MediaBillingStatusSettled {
		return nil
	}
	if task.BillingStatus != MediaBillingStatusSettling {
		updated, err := c.repo.UpdateBilling(ctx, task.ID, task.BillingStatus, map[string]any{
			"billing_status": MediaBillingStatusSettling,
		})
		if err != nil {
			return fmt.Errorf("claim media task %d settlement: %w", task.ID, err)
		}
		if !updated {
			fresh, loadErr := c.repo.GetByID(ctx, task.ID)
			if loadErr != nil {
				return fmt.Errorf("reload media task %d after settlement CAS: %w", task.ID, loadErr)
			}
			if fresh.BillingStatus == MediaBillingStatusSettled {
				return nil
			}
			return fmt.Errorf("%w: claim task %d settlement", ErrMediaSettlementCASConflict, task.ID)
		}
		task.BillingStatus = MediaBillingStatusSettling
	}

	var portErr error
	switch plan.Type {
	case MediaSettlementTypeSuccess:
		if plan.Usage == nil || plan.Failure != nil {
			return errors.New("invalid successful media settlement plan")
		}
		portErr = c.port.SettleSuccess(ctx, task, *plan.Usage)
	case MediaSettlementTypeFailure:
		if plan.Failure == nil || plan.Usage != nil {
			return errors.New("invalid failed media settlement plan")
		}
		portErr = c.port.SettleFailure(ctx, task, *plan.Failure)
	default:
		return fmt.Errorf("unknown media settlement type %q", plan.Type)
	}
	if portErr != nil {
		updated, updateErr := c.repo.UpdateBilling(ctx, task.ID, MediaBillingStatusSettling, map[string]any{
			"billing_status": MediaBillingStatusRetry,
		})
		if updateErr != nil {
			return errors.Join(fmt.Errorf("settle media task %d: %w", task.ID, portErr), fmt.Errorf("mark settlement retry: %w", updateErr))
		}
		if !updated {
			return errors.Join(fmt.Errorf("settle media task %d: %w", task.ID, portErr), fmt.Errorf("%w: mark task %d retry", ErrMediaSettlementCASConflict, task.ID))
		}
		return fmt.Errorf("settle media task %d: %w", task.ID, portErr)
	}

	updates := map[string]any{"billing_status": MediaBillingStatusSettled}
	switch plan.Type {
	case MediaSettlementTypeSuccess:
		updates["final_amount"] = task.PrechargedAmount
		updates["refunded_amount"] = float64(0)
	case MediaSettlementTypeFailure:
		refundRatio := clampMediaRatio(plan.Failure.RefundRatio)
		updates["refunded_amount"] = task.PrechargedAmount * refundRatio
		updates["final_amount"] = task.PrechargedAmount * (1 - refundRatio)
	}
	updated, err := c.repo.UpdateBilling(ctx, task.ID, MediaBillingStatusSettling, updates)
	if err != nil {
		return fmt.Errorf("mark media task %d settlement complete: %w", task.ID, err)
	}
	if !updated {
		fresh, loadErr := c.repo.GetByID(ctx, task.ID)
		if loadErr == nil && fresh.BillingStatus == MediaBillingStatusSettled {
			return nil
		}
		if loadErr != nil {
			return fmt.Errorf("reload media task %d after settlement completion CAS: %w", task.ID, loadErr)
		}
		return fmt.Errorf("%w: complete task %d settlement", ErrMediaSettlementCASConflict, task.ID)
	}
	return nil
}

func (c *MediaBillingCoordinator) validate() error {
	if c == nil || c.repo == nil || c.port == nil {
		return errors.New("media billing coordinator dependencies are incomplete")
	}
	return nil
}

func decodeMediaSettlementPlan(raw json.RawMessage) (MediaSettlementPlan, error) {
	if len(raw) == 0 {
		return MediaSettlementPlan{}, errors.New("media settlement plan is empty")
	}
	var plan MediaSettlementPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return MediaSettlementPlan{}, fmt.Errorf("decode media settlement plan: %w", err)
	}
	return plan, nil
}

func clampMediaRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

var _ MediaSettlementCoordinator = (*MediaBillingCoordinator)(nil)
