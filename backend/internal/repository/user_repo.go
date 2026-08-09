package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbsalescommissiontier "github.com/Wei-Shaw/sub2api/ent/salescommissiontier"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type userRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewUserRepository(client *dbent.Client, sqlDB *sql.DB) service.UserRepository {
	return newUserRepositoryWithSQL(client, sqlDB)
}

func newUserRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *userRepository {
	return &userRepository{client: client, sql: sqlq}
}

func (r *userRepository) Create(ctx context.Context, userIn *service.User) error {
	if userIn == nil {
		return nil
	}

	// 统一使用 ent 的事务：保证用户与允许分组的更新原子化，
	// 并避免基于 *sql.Tx 手动构造 ent client 导致的 ExecQuerier 断言错误。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client 并由调用方负责提交/回滚。
		txClient = r.client
	}

	created, err := txClient.User.Create().
		SetEmail(userIn.Email).
		SetUsername(userIn.Username).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
		SetIsSales(userIn.IsSales).
		SetSalesCommissionRate(userIn.SalesCommissionRate).
		SetSalesCommissionMode(service.NormalizeSalesCommissionMode(userIn.SalesCommissionMode)).
		SetSalesCommissionMinMonthlySales(userIn.SalesCommissionMinMonthlySales).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrEmailExists)
	}

	if err := r.syncUserAllowedGroupsWithClient(ctx, txClient, created.ID, userIn.AllowedGroups); err != nil {
		return err
	}
	if err := r.syncUserSalesCommissionTiersWithClient(ctx, txClient, created.ID, userIn.SalesCommissionTiers); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	applyUserEntityToService(userIn, created)
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*service.User, error) {
	m, err := r.client.User.Query().Where(dbuser.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[id]; ok {
		out.AllowedGroups = v
	}
	tiersByUserID, err := r.loadSalesCommissionTiers(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if tiers, ok := tiersByUserID[id]; ok {
		out.SalesCommissionTiers = tiers
	}
	return out, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	m, err := r.client.User.Query().Where(dbuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
	}
	tiersByUserID, err := r.loadSalesCommissionTiers(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if tiers, ok := tiersByUserID[m.ID]; ok {
		out.SalesCommissionTiers = tiers
	}
	return out, nil
}

func (r *userRepository) Update(ctx context.Context, userIn *service.User) error {
	if userIn == nil {
		return nil
	}

	// 使用 ent 事务包裹用户更新与 allowed_groups 同步，避免跨层事务不一致。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client 并由调用方负责提交/回滚。
		txClient = r.client
	}

	updateOp := txClient.User.UpdateOneID(userIn.ID).
		SetEmail(userIn.Email).
		SetUsername(userIn.Username).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
		SetLegalTermsAccepted(userIn.LegalTermsAccepted).
		SetLegalTermsVersion(userIn.LegalTermsVersion).
		SetNillableLegalTermsAcceptedAt(userIn.LegalTermsAcceptedAt).
		SetBalanceNotifyEnabled(userIn.BalanceNotifyEnabled).
		SetBalanceNotifyThresholdType(userIn.BalanceNotifyThresholdType).
		SetNillableBalanceNotifyThreshold(userIn.BalanceNotifyThreshold).
		SetBalanceNotifyExtraEmails(marshalExtraEmails(userIn.BalanceNotifyExtraEmails)).
		SetTotalRecharged(userIn.TotalRecharged).
		SetIsSales(userIn.IsSales).
		SetSalesCommissionRate(userIn.SalesCommissionRate).
		SetSalesCommissionMode(service.NormalizeSalesCommissionMode(userIn.SalesCommissionMode)).
		SetSalesCommissionMinMonthlySales(userIn.SalesCommissionMinMonthlySales)
	if userIn.BalanceNotifyThreshold == nil {
		updateOp = updateOp.ClearBalanceNotifyThreshold()
	}
	updated, err := updateOp.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, service.ErrEmailExists)
	}

	if err := r.syncUserAllowedGroupsWithClient(ctx, txClient, updated.ID, userIn.AllowedGroups); err != nil {
		return err
	}
	if err := r.syncUserSalesCommissionTiersWithClient(ctx, txClient, updated.ID, userIn.SalesCommissionTiers); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	userIn.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	affected, err := r.client.User.Delete().Where(dbuser.IDEQ(id)).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, service.UserListFilters{})
}

func (r *userRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	q := r.client.User.Query()

	if filters.Status != "" {
		q = q.Where(dbuser.StatusEQ(filters.Status))
	}
	if filters.Role != "" {
		q = q.Where(dbuser.RoleEQ(filters.Role))
	}
	if filters.IsSales != nil {
		q = q.Where(dbuser.IsSalesEQ(*filters.IsSales))
	}
	if filters.Search != "" {
		q = q.Where(
			dbuser.Or(
				dbuser.EmailContainsFold(filters.Search),
				dbuser.UsernameContainsFold(filters.Search),
				dbuser.NotesContainsFold(filters.Search),
				dbuser.HasAPIKeysWith(apikey.KeyContainsFold(filters.Search)),
			),
		)
	}

	if filters.GroupName != "" {
		q = q.Where(dbuser.HasAllowedGroupsWith(
			dbgroup.NameContainsFold(filters.GroupName),
		))
	}

	// If attribute filters are specified, we need to filter by user IDs first
	var allowedUserIDs []int64
	if len(filters.Attributes) > 0 {
		var attrErr error
		allowedUserIDs, attrErr = r.filterUsersByAttributes(ctx, filters.Attributes)
		if attrErr != nil {
			return nil, nil, attrErr
		}
		if len(allowedUserIDs) == 0 {
			// No users match the attribute filters
			return []service.User{}, paginationResultFromTotal(0, params), nil
		}
		q = q.Where(dbuser.IDIn(allowedUserIDs...))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	usersQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range userListOrder(params) {
		usersQuery = usersQuery.Order(order)
	}

	users, err := usersQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outUsers := make([]service.User, 0, len(users))
	if len(users) == 0 {
		return outUsers, paginationResultFromTotal(int64(total), params), nil
	}

	userIDs := make([]int64, 0, len(users))
	userMap := make(map[int64]*service.User, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
		u := userEntityToService(users[i])
		outUsers = append(outUsers, *u)
		userMap[u.ID] = &outUsers[len(outUsers)-1]
	}

	shouldLoadSubscriptions := filters.IncludeSubscriptions == nil || *filters.IncludeSubscriptions
	if shouldLoadSubscriptions {
		// Batch load active subscriptions with groups to avoid N+1.
		subs, err := r.client.UserSubscription.Query().
			Where(
				usersubscription.UserIDIn(userIDs...),
				usersubscription.StatusEQ(service.SubscriptionStatusActive),
			).
			WithGroup().
			All(ctx)
		if err != nil {
			return nil, nil, err
		}

		for i := range subs {
			if u, ok := userMap[subs[i].UserID]; ok {
				u.Subscriptions = append(u.Subscriptions, *userSubscriptionEntityToService(subs[i]))
			}
		}
	}

	allowedGroupsByUser, err := r.loadAllowedGroups(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for id, u := range userMap {
		if groups, ok := allowedGroupsByUser[id]; ok {
			u.AllowedGroups = groups
		}
	}

	tiersByUserID, err := r.loadSalesCommissionTiers(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for id, u := range userMap {
		if tiers, ok := tiersByUserID[id]; ok {
			u.SalesCommissionTiers = tiers
		}
	}

	return outUsers, paginationResultFromTotal(int64(total), params), nil
}

func userListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	defaultField := true
	switch sortBy {
	case "email":
		field = dbuser.FieldEmail
		defaultField = false
	case "username":
		field = dbuser.FieldUsername
		defaultField = false
	case "role":
		field = dbuser.FieldRole
		defaultField = false
	case "balance":
		field = dbuser.FieldBalance
		defaultField = false
	case "concurrency":
		field = dbuser.FieldConcurrency
		defaultField = false
	case "status":
		field = dbuser.FieldStatus
		defaultField = false
	case "created_at":
		field = dbuser.FieldCreatedAt
		defaultField = false
	default:
		field = dbuser.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		if defaultField && field == dbuser.FieldID {
			return []func(*entsql.Selector){dbent.Asc(dbuser.FieldID)}
		}
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbuser.FieldID)}
	}
	if defaultField && field == dbuser.FieldID {
		return []func(*entsql.Selector){dbent.Desc(dbuser.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbuser.FieldID)}
}

// filterUsersByAttributes returns user IDs that match ALL the given attribute filters
func (r *userRepository) filterUsersByAttributes(ctx context.Context, attrs map[int64]string) ([]int64, error) {
	if len(attrs) == 0 {
		return nil, nil
	}

	if r.sql == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	clauses := make([]string, 0, len(attrs))
	args := make([]any, 0, len(attrs)*2+1)
	argIndex := 1
	for attrID, value := range attrs {
		clauses = append(clauses, fmt.Sprintf("(attribute_id = $%d AND value ILIKE $%d)", argIndex, argIndex+1))
		args = append(args, attrID, "%"+value+"%")
		argIndex += 2
	}

	query := fmt.Sprintf(
		`SELECT user_id
		 FROM user_attribute_values
		 WHERE %s
		 GROUP BY user_id
		 HAVING COUNT(DISTINCT attribute_id) = $%d`,
		strings.Join(clauses, " OR "),
		argIndex,
	)
	args = append(args, len(attrs))

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if scanErr := rows.Scan(&userID); scanErr != nil {
			return nil, scanErr
		}
		result = append(result, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	update := client.User.Update().Where(dbuser.IDEQ(id)).AddBalance(amount)
	// Track cumulative recharge amount for percentage-based notifications
	if amount > 0 {
		update = update.AddTotalRecharged(amount)
	}
	n, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// DeductBalance 扣除用户余额
// 透支策略：允许余额变为负数，确保当前请求能够完成
// 中间件会阻止余额 <= 0 的用户发起后续请求
func (r *userRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().
		Where(dbuser.IDEQ(id)).
		AddBalance(-amount).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().Where(dbuser.IDEQ(id)).AddConcurrency(amount).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.client.User.Query().Where(dbuser.EmailEQ(email)).Exist(ctx)
}

func (r *userRepository) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.UserAllowedGroup.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
		DoNothing().
		Exec(ctx)
}

func (r *userRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
	affected, err := r.client.UserAllowedGroup.Delete().
		Where(userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return int64(affected), nil
}

// RemoveGroupFromUserAllowedGroups 移除单个用户的指定分组权限
func (r *userRepository) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserAllowedGroup.Delete().
		Where(userallowedgroup.UserIDEQ(userID), userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	return err
}

func (r *userRepository) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	m, err := r.client.User.Query().
		Where(
			dbuser.RoleEQ(service.RoleAdmin),
			dbuser.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbuser.FieldID)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.ID})
	if err != nil {
		return nil, err
	}
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
	}
	return out, nil
}

func (r *userRepository) loadAllowedGroups(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	rows, err := r.client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	for i := range rows {
		out[rows[i].UserID] = append(out[rows[i].UserID], rows[i].GroupID)
	}

	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool { return out[userID][i] < out[userID][j] })
	}

	return out, nil
}

// syncUserAllowedGroupsWithClient 在 ent client/事务内同步用户允许分组：
// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
func (r *userRepository) syncUserAllowedGroupsWithClient(ctx context.Context, client *dbent.Client, userID int64, groupIDs []int64) error {
	if client == nil {
		return nil
	}

	// Keep join table as the source of truth for reads.
	if _, err := client.UserAllowedGroup.Delete().Where(userallowedgroup.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}

	unique := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		unique[id] = struct{}{}
	}

	if len(unique) > 0 {
		creates := make([]*dbent.UserAllowedGroupCreate, 0, len(unique))
		for groupID := range unique {
			creates = append(creates, client.UserAllowedGroup.Create().SetUserID(userID).SetGroupID(groupID))
		}
		if err := client.UserAllowedGroup.
			CreateBulk(creates...).
			OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
			DoNothing().
			Exec(ctx); err != nil {
			return err
		}
	}

	return nil
}

func applyUserEntityToService(dst *service.User, src *dbent.User) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.SalesCommissionMode = src.SalesCommissionMode
	dst.SalesCommissionMinMonthlySales = src.SalesCommissionMinMonthlySales
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
	dst.LegalTermsAccepted = src.LegalTermsAccepted
	dst.LegalTermsVersion = src.LegalTermsVersion
	dst.LegalTermsAcceptedAt = src.LegalTermsAcceptedAt
}

func (r *userRepository) syncUserSalesCommissionTiersWithClient(ctx context.Context, client *dbent.Client, userID int64, tiers []service.SalesCommissionTier) error {
	if _, err := client.SalesCommissionTier.Delete().Where(dbsalescommissiontier.SalesUserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if len(tiers) == 0 {
		return nil
	}

	normalizedTiers, err := service.NormalizeSalesCommissionTiers(tiers)
	if err != nil {
		return err
	}
	builders := make([]*dbent.SalesCommissionTierCreate, 0, len(normalizedTiers))
	for _, tier := range normalizedTiers {
		builder := client.SalesCommissionTier.Create().
			SetSalesUserID(userID).
			SetMonthSalesFromCny(tier.MonthSalesFromCNY).
			SetCommissionRate(tier.CommissionRate).
			SetSortOrder(tier.SortOrder)
		if tier.MonthSalesToCNY != nil {
			builder.SetMonthSalesToCny(*tier.MonthSalesToCNY)
		}
		builders = append(builders, builder)
	}
	return client.SalesCommissionTier.CreateBulk(builders...).Exec(ctx)
}

func (r *userRepository) loadSalesCommissionTiers(ctx context.Context, userIDs []int64) (map[int64][]service.SalesCommissionTier, error) {
	result := make(map[int64][]service.SalesCommissionTier, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	tiers, err := r.client.SalesCommissionTier.Query().
		Where(dbsalescommissiontier.SalesUserIDIn(userIDs...)).
		Order(dbent.Asc(dbsalescommissiontier.FieldSalesUserID), dbent.Asc(dbsalescommissiontier.FieldSortOrder), dbent.Asc(dbsalescommissiontier.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, tier := range tiers {
		result[tier.SalesUserID] = append(result[tier.SalesUserID], service.SalesCommissionTier{
			MonthSalesFromCNY: tier.MonthSalesFromCny,
			MonthSalesToCNY:   tier.MonthSalesToCny,
			CommissionRate:    tier.CommissionRate,
			SortOrder:         tier.SortOrder,
		})
	}
	return result, nil
}

// marshalExtraEmails serializes notify email entries to JSON for storage.
func marshalExtraEmails(entries []service.NotifyEmailEntry) string {
	return service.MarshalNotifyEmails(entries)
}

// UpdateTotpSecret 更新用户的 TOTP 加密密钥
func (r *userRepository) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	client := clientFromContext(ctx, r.client)
	update := client.User.UpdateOneID(userID)
	if encryptedSecret == nil {
		update = update.ClearTotpSecretEncrypted()
	} else {
		update = update.SetTotpSecretEncrypted(*encryptedSecret)
	}
	_, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// EnableTotp 启用用户的 TOTP 双因素认证
func (r *userRepository) EnableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(true).
		SetTotpEnabledAt(time.Now()).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// DisableTotp 禁用用户的 TOTP 双因素认证
func (r *userRepository) DisableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(false).
		ClearTotpEnabledAt().
		ClearTotpSecretEncrypted().
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// --- 推广奖励相关 ---

// GetByReferralCode 根据推广码查找用户
func (r *userRepository) GetByReferralCode(ctx context.Context, code string) (*service.User, error) {
	if code == "" {
		return nil, nil
	}
	m, err := r.client.User.Query().Where(dbuser.ReferralCodeEQ(code), dbuser.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	out := userEntityToService(m)
	tiersByUserID, err := r.loadSalesCommissionTiers(ctx, []int64{out.ID})
	if err != nil {
		return nil, err
	}
	if tiers, ok := tiersByUserID[out.ID]; ok {
		out.SalesCommissionTiers = tiers
	}
	return out, nil
}

// UpdateReferralCode 更新用户推广码
func (r *userRepository) UpdateReferralCode(ctx context.Context, userID int64, code string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).SetReferralCode(code).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// UpdateReferredBy 设置用户的推荐人
func (r *userRepository) UpdateReferredBy(ctx context.Context, userID int64, referrerID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).SetReferredBy(referrerID).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}

// IsFirstRecharge 检查用户是否首次充值（total_recharged == 0 时为首次）
func (r *userRepository) IsFirstRecharge(ctx context.Context, userID int64) (bool, error) {
	u, err := r.client.User.Get(ctx, userID)
	if err != nil {
		return false, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return u.TotalRecharged == 0, nil
}

// ListActiveUserIDs 列出全部未删除用户 ID（用于批量发放赠送余额）
func (r *userRepository) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	users, err := r.client.User.Query().
		Where(dbuser.DeletedAtIsNil()).
		Select(dbuser.FieldID).
		Ints(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(users))
	for _, id := range users {
		ids = append(ids, int64(id))
	}
	return ids, nil
}
