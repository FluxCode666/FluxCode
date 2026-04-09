package repository

import (
	"fmt"
	"strings"
	"time"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

const defaultSchedulingStrategyPlatform = "default"
const legacyInactiveStatus = "inactive"

var bannedAccountErrorMessageKeywords = []string{
	"terms of service",
	"violation",
}

type accountSchedulingStrategy interface {
	Platform() string
	StatePredicates(state service.AccountSchedulingState, now time.Time) []dbpredicate.Account
	BucketCaseWhenClauses(platformConditionSQL string) []string
}

type baseAccountSchedulingStrategy struct {
	platform string
}

func (s baseAccountSchedulingStrategy) Platform() string {
	return s.platform
}

func (s baseAccountSchedulingStrategy) StatePredicates(state service.AccountSchedulingState, now time.Time) []dbpredicate.Account {
	switch state {
	case service.AccountSchedulingStateAvailable:
		return []dbpredicate.Account{
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			notExpiredPredicate(now),
			tempUnschedulablePredicate(),
			rateLimitInactivePredicate(),
			overloadInactivePredicate(),
		}
	case service.AccountSchedulingStateManualUnschedulable:
		return []dbpredicate.Account{
			dbaccount.StatusEQ(service.StatusActive),
			notExpiredPredicate(now),
			dbaccount.SchedulableEQ(false),
		}
	case service.AccountSchedulingStateTempUnschedulable:
		return []dbpredicate.Account{
			dbaccount.StatusEQ(service.StatusActive),
			notExpiredPredicate(now),
			dbaccount.SchedulableEQ(true),
			tempUnschedulableActivePredicate(),
		}
	case service.AccountSchedulingStateRateLimited:
		return []dbpredicate.Account{
			dbaccount.StatusEQ(service.StatusActive),
			notExpiredPredicate(now),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			rateLimitActivePredicate(),
		}
	case service.AccountSchedulingStateOverloaded:
		return []dbpredicate.Account{
			dbaccount.StatusEQ(service.StatusActive),
			notExpiredPredicate(now),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			rateLimitInactivePredicate(),
			overloadActivePredicate(),
		}
	case service.AccountSchedulingStateExpired:
		return []dbpredicate.Account{expiredSchedulingPredicate()}
	case service.AccountSchedulingStateInactive:
		return []dbpredicate.Account{inactiveAccountPredicate()}
	case service.AccountSchedulingStateError:
		return []dbpredicate.Account{dbaccount.StatusEQ(service.StatusError)}
	case service.AccountSchedulingStateBanned:
		return []dbpredicate.Account{bannedAccountPredicate()}
	default:
		return nil
	}
}

func (s baseAccountSchedulingStrategy) BucketCaseWhenClauses(platformConditionSQL string) []string {
	conditions := []struct {
		state     service.AccountSchedulingState
		condition string
	}{
		{service.AccountSchedulingStateBanned, bannedAccountConditionSQL("status", "error_message")},
		{service.AccountSchedulingStateError, "status = 'error'"},
		{service.AccountSchedulingStateInactive, inactiveAccountConditionSQL("status")},
		{service.AccountSchedulingStateExpired, "expires_at IS NOT NULL AND expires_at <= NOW()"},
		{service.AccountSchedulingStateManualUnschedulable, "status = 'active' AND schedulable = FALSE AND (expires_at IS NULL OR expires_at > NOW())"},
		{service.AccountSchedulingStateTempUnschedulable, "status = 'active' AND schedulable = TRUE AND (expires_at IS NULL OR expires_at > NOW()) AND temp_unschedulable_until IS NOT NULL AND temp_unschedulable_until > NOW()"},
		{service.AccountSchedulingStateRateLimited, "status = 'active' AND schedulable = TRUE AND (expires_at IS NULL OR expires_at > NOW()) AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW()) AND rate_limit_reset_at IS NOT NULL AND rate_limit_reset_at > NOW()"},
		{service.AccountSchedulingStateOverloaded, "status = 'active' AND schedulable = TRUE AND (expires_at IS NULL OR expires_at > NOW()) AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW()) AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW()) AND overload_until IS NOT NULL AND overload_until > NOW()"},
		{service.AccountSchedulingStateAvailable, "status = 'active' AND schedulable = TRUE AND (expires_at IS NULL OR expires_at > NOW()) AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW()) AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW()) AND (overload_until IS NULL OR overload_until <= NOW())"},
	}

	clauses := make([]string, 0, len(conditions))
	for _, item := range conditions {
		clauses = append(clauses, fmt.Sprintf("WHEN %s AND %s THEN '%s'", platformConditionSQL, item.condition, item.state))
	}
	return clauses
}

type defaultSchedulingStrategy struct{ baseAccountSchedulingStrategy }
type antigravitySchedulingStrategy struct{ baseAccountSchedulingStrategy }

var defaultAccountSchedulingStrategy accountSchedulingStrategy = defaultSchedulingStrategy{baseAccountSchedulingStrategy{platform: defaultSchedulingStrategyPlatform}}

var registeredAccountSchedulingStrategies = []accountSchedulingStrategy{
	antigravitySchedulingStrategy{baseAccountSchedulingStrategy{platform: service.PlatformAntigravity}},
}

var accountSchedulingStrategiesByPlatform = func() map[string]accountSchedulingStrategy {
	strategies := make(map[string]accountSchedulingStrategy, len(registeredAccountSchedulingStrategies))
	for _, strategy := range registeredAccountSchedulingStrategies {
		strategies[strategy.Platform()] = strategy
	}
	return strategies
}()

func resolveAccountSchedulingStrategy(platform string) accountSchedulingStrategy {
	normalizedPlatforms := normalizeSchedulingPlatforms([]string{platform})
	if len(normalizedPlatforms) == 0 {
		return defaultAccountSchedulingStrategy
	}
	if strategy, ok := accountSchedulingStrategiesByPlatform[normalizedPlatforms[0]]; ok {
		return strategy
	}
	return defaultAccountSchedulingStrategy
}

func buildAccountSchedulingBucketCaseSQL() string {
	clauses := make([]string, 0, 16)
	for _, strategy := range registeredAccountSchedulingStrategies {
		clauses = append(clauses, strategy.BucketCaseWhenClauses(fmt.Sprintf("platform = '%s'", strategy.Platform()))...)
	}
	clauses = append(clauses, defaultAccountSchedulingStrategy.BucketCaseWhenClauses(defaultSchedulingPlatformConditionSQL())...)
	return "\n\tCASE\n\t\t" + strings.Join(clauses, "\n\t\t") + "\n\t\tELSE NULL\n\tEND\n"
}

func registeredSchedulingPlatforms() []string {
	platforms := make([]string, 0, len(registeredAccountSchedulingStrategies))
	for _, strategy := range registeredAccountSchedulingStrategies {
		platforms = append(platforms, strategy.Platform())
	}
	return platforms
}

func defaultSchedulingPlatformConditionSQL() string {
	platforms := registeredSchedulingPlatforms()
	if len(platforms) == 0 {
		return "TRUE"
	}
	quoted := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		quoted = append(quoted, fmt.Sprintf("'%s'", platform))
	}
	return fmt.Sprintf("platform NOT IN (%s)", strings.Join(quoted, ", "))
}

func normalizeSchedulingPlatforms(platforms []string) []string {
	seen := make(map[string]struct{}, len(platforms))
	out := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		normalized := strings.ToLower(strings.TrimSpace(platform))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func accountSchedulingStatePredicate(platforms []string, state service.AccountSchedulingState, now time.Time) dbpredicate.Account {
	if !service.IsValidAccountSchedulingState(state) {
		return nil
	}

	groups := buildAccountSchedulingStrategyGroups(platforms)
	branches := make([]dbpredicate.Account, 0, len(groups))
	for _, group := range groups {
		predicates := make([]dbpredicate.Account, 0, 8)
		if group.platformPredicate != nil {
			predicates = append(predicates, group.platformPredicate)
		}
		predicates = append(predicates, group.strategy.StatePredicates(state, now)...)
		branches = append(branches, andAccountPredicates(predicates...))
	}
	return orAccountPredicates(branches...)
}

type accountSchedulingStrategyGroup struct {
	strategy          accountSchedulingStrategy
	platformPredicate dbpredicate.Account
}

func buildAccountSchedulingStrategyGroups(platforms []string) []accountSchedulingStrategyGroup {
	normalizedPlatforms := normalizeSchedulingPlatforms(platforms)
	if len(normalizedPlatforms) == 0 {
		groups := make([]accountSchedulingStrategyGroup, 0, len(registeredAccountSchedulingStrategies)+1)
		for _, strategy := range registeredAccountSchedulingStrategies {
			groups = append(groups, accountSchedulingStrategyGroup{
				strategy:          strategy,
				platformPredicate: dbaccount.PlatformEQ(strategy.Platform()),
			})
		}
		groups = append(groups, accountSchedulingStrategyGroup{
			strategy:          defaultAccountSchedulingStrategy,
			platformPredicate: dbaccount.PlatformNotIn(registeredSchedulingPlatforms()...),
		})
		return groups
	}

	groupedPlatforms := make(map[string][]string)
	order := make([]string, 0, len(normalizedPlatforms))
	for _, platform := range normalizedPlatforms {
		strategy := resolveAccountSchedulingStrategy(platform)
		key := strategy.Platform()
		if _, ok := groupedPlatforms[key]; !ok {
			order = append(order, key)
		}
		groupedPlatforms[key] = append(groupedPlatforms[key], platform)
	}

	groups := make([]accountSchedulingStrategyGroup, 0, len(order))
	for _, key := range order {
		platformGroup := groupedPlatforms[key]
		strategy := resolveAccountSchedulingStrategy(platformGroup[0])
		var platformPredicate dbpredicate.Account
		if len(platformGroup) == 1 {
			platformPredicate = dbaccount.PlatformEQ(platformGroup[0])
		} else {
			platformPredicate = dbaccount.PlatformIn(platformGroup...)
		}
		groups = append(groups, accountSchedulingStrategyGroup{
			strategy:          strategy,
			platformPredicate: platformPredicate,
		})
	}
	return groups
}

func inactiveAccountPredicate() dbpredicate.Account {
	return dbaccount.StatusIn(legacyInactiveStatus, service.StatusDisabled)
}

func bannedAccountPredicate() dbpredicate.Account {
	messagePredicates := make([]dbpredicate.Account, 0, len(bannedAccountErrorMessageKeywords))
	for _, keyword := range bannedAccountErrorMessageKeywords {
		messagePredicates = append(messagePredicates, dbaccount.ErrorMessageContainsFold(keyword))
	}
	return andAccountPredicates(
		dbaccount.StatusEQ(service.StatusError),
		orAccountPredicates(messagePredicates...),
	)
}

func inactiveAccountConditionSQL(statusColumn string) string {
	return fmt.Sprintf("%s IN ('%s', '%s')", statusColumn, legacyInactiveStatus, service.StatusDisabled)
}

func bannedAccountConditionSQL(statusColumn, errorMessageColumn string) string {
	normalizedErrorMessage := fmt.Sprintf("LOWER(COALESCE(%s, ''))", errorMessageColumn)
	clauses := make([]string, 0, len(bannedAccountErrorMessageKeywords))
	for _, keyword := range bannedAccountErrorMessageKeywords {
		clauses = append(clauses, fmt.Sprintf("%s LIKE '%%%s%%'", normalizedErrorMessage, keyword))
	}
	return fmt.Sprintf("%s = '%s' AND (%s)", statusColumn, service.StatusError, strings.Join(clauses, " OR "))
}

func tempUnschedulableActivePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("temp_unschedulable_until")
		s.Where(entsql.And(
			entsql.NotNull(col),
			entsql.GT(col, entsql.Expr("NOW()")),
		))
	})
}

func expiredSchedulingPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		expiresAtCol := s.C("expires_at")
		statusCol := s.C("status")
		s.Where(entsql.And(
			entsql.NotNull(expiresAtCol),
			entsql.LTE(expiresAtCol, entsql.Expr("NOW()")),
			entsql.NotIn(statusCol, service.StatusError, legacyInactiveStatus, service.StatusDisabled),
		))
	})
}

func rateLimitInactivePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("rate_limit_reset_at")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
	})
}

func rateLimitActivePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("rate_limit_reset_at")
		s.Where(entsql.And(
			entsql.NotNull(col),
			entsql.GT(col, entsql.Expr("NOW()")),
		))
	})
}

func overloadInactivePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("overload_until")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
	})
}

func overloadActivePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("overload_until")
		s.Where(entsql.And(
			entsql.NotNull(col),
			entsql.GT(col, entsql.Expr("NOW()")),
		))
	})
}

func andAccountPredicates(predicates ...dbpredicate.Account) dbpredicate.Account {
	filtered := make([]dbpredicate.Account, 0, len(predicates))
	for _, predicate := range predicates {
		if predicate != nil {
			filtered = append(filtered, predicate)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return dbaccount.And(filtered...)
}

func orAccountPredicates(predicates ...dbpredicate.Account) dbpredicate.Account {
	filtered := make([]dbpredicate.Account, 0, len(predicates))
	for _, predicate := range predicates {
		if predicate != nil {
			filtered = append(filtered, predicate)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return dbaccount.Or(filtered...)
}
