# 推广奖励功能设计文档

> **日期**：2026-04-21  
> **状态**：设计阶段  
> **作者**：Cascade AI + 用户协作

---

## 1. 需求摘要

| 项目 | 决定 |
|------|------|
| 奖励类型 | **赠送余额**（独立于普通余额，不可退款，优先消费） |
| 赠送余额过期 | 可配置过期天数（含永久选项），前端单独显示 |
| 推广码 | 系统自动生成随机短码，用户不可自定义 |
| 推广码传递 | URL 参数自动填充注册表单（`?ref=XXXX`），用户可手动修改 |
| 触发时机 | 被邀请人注册即得奖励；邀请人在被邀请人**首充**后得奖励，另支持**持续奖励**（被邀请人每次充值均可触发推广人奖励，可动态配置） |
| 配置方式 | 两级配置：全局默认参数 + 按用户覆盖（可给特定推广人设置特殊奖励金额/比例） |
| 管理级别 | 详细管理（全站统计、用户明细、关系链、手动调整赠送余额） |
| 营销增强 | 推广数据看板（转化漏斗 + 趋势图）、批量赠送余额、推广达人榜 Top N |
| 用户面板 | 独立「推广中心」页面：概览卡片 + 趋势图 + 邀请明细表 + 赠送余额明细 |
| 前端位置 | 用户侧边栏新增「推广中心」菜单项，路由 `/referral` |

---

## 2. 技术选型：方案 A — 独立赠送余额表

### 2.1 为什么选方案 A

| 方案 | 描述 | 结论 |
|------|------|------|
| **A：独立赠送余额表** | `gift_balance_records` 表记录每笔赠送，扣费时 FIFO 优先消费 | ✅ **采用** |
| B：User 表加字段 | User 加 `gift_balance` 字段 | ❌ 无法支持不同过期时间 |
| C：混合方案 | User 字段做缓存 + records 表做明细 | ❌ 缓存一致性复杂 |

方案 A 的优势：
- 每笔赠送独立追踪，过期管理精确
- 来源可审计（哪条推广带来的、管理员手动发放的）
- 扩展性好（未来可加活动奖励等来源）
- 虽然扣费 SQL 变复杂，但改动集中在两处（`deductUsageBillingBalance` + `DeductBalance`）

---

## 3. 数据模型

### 3.1 新增表：`gift_balance_records`

```sql
CREATE TABLE gift_balance_records (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id),
    amount          DOUBLE PRECISION NOT NULL,       -- 原始发放金额
    remaining       DOUBLE PRECISION NOT NULL,       -- 剩余可用金额
    source          VARCHAR(50) NOT NULL,            -- referral_invitee / referral_inviter / referral_ongoing / admin_grant
    source_ref_id   BIGINT,                          -- 关联 referrals.id 或管理操作 ID
    expires_at      TIMESTAMPTZ,                     -- NULL = 永不过期
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_gift_balance_user_active ON gift_balance_records (user_id)
    WHERE remaining > 0;
CREATE INDEX idx_gift_balance_expiry ON gift_balance_records (expires_at)
    WHERE remaining > 0 AND expires_at IS NOT NULL;
```

### 3.2 新增表：`referrals`

```sql
CREATE TABLE referrals (
    id                      BIGSERIAL PRIMARY KEY,
    referrer_id             BIGINT NOT NULL REFERENCES users(id),   -- 邀请人
    referee_id              BIGINT NOT NULL REFERENCES users(id),   -- 被邀请人
    referral_code           VARCHAR(20) NOT NULL,                    -- 使用的推广码
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending / completed
    invitee_reward_amount   DOUBLE PRECISION NOT NULL DEFAULT 0,     -- 被邀请人奖励金额
    inviter_reward_amount   DOUBLE PRECISION NOT NULL DEFAULT 0,     -- 邀请人奖励金额
    invitee_rewarded_at     TIMESTAMPTZ,                             -- 注册时发放
    inviter_rewarded_at     TIMESTAMPTZ,                             -- 首充后发放
    ongoing_reward_count    INT NOT NULL DEFAULT 0,                  -- 已触发持续奖励次数
    ongoing_reward_total    DOUBLE PRECISION NOT NULL DEFAULT 0,     -- 持续奖励累计金额
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_referrals_referee UNIQUE (referee_id)
);

CREATE INDEX idx_referrals_referrer ON referrals (referrer_id);
CREATE INDEX idx_referrals_code ON referrals (referral_code);
CREATE INDEX idx_referrals_status ON referrals (status) WHERE status = 'pending';
```

### 3.3 User 表扩展

```sql
ALTER TABLE users
    ADD COLUMN referral_code VARCHAR(20) UNIQUE,
    ADD COLUMN referred_by   BIGINT REFERENCES users(id);
```

- `referral_code`：用户唯一推广码，首次访问推广中心时惰性生成（6-8 位字母数字混合）
- `referred_by`：冗余字段，方便快速查询谁邀请了此用户

### 3.4 系统设置键（Settings 表 — 全局默认配置）

| Key | 类型 | 默认值 | 说明 |
|-----|------|--------|------|
| `referral_enabled` | bool | `false` | 推广功能总开关 |
| `referral_invitee_reward` | float | `1.0` | 被邀请人注册奖励金额 |
| `referral_inviter_reward` | float | `2.0` | 邀请人奖励金额（首充触发） |
| `referral_max_invites` | int | `0` | 每人最大邀请数（0=不限） |
| `referral_reward_expiry_days` | int | `0` | 赠送余额过期天数（0=永不过期） |
| `referral_ongoing_reward_enabled` | bool | `false` | 持续奖励开关（每次充值触发推广人奖励） |
| `referral_ongoing_reward_type` | string | `fixed` | 持续奖励类型：`fixed`=固定金额，`percentage`=充值金额百分比 |
| `referral_ongoing_reward_value` | float | `0.5` | 持续奖励值：fixed 时为固定金额，percentage 时为比例（如 0.05=5%） |
| `referral_ongoing_reward_max_count` | int | `0` | 每条推广关系最多触发持续奖励次数（0=不限） |
| `referral_ongoing_reward_duration_days` | int | `0` | 持续奖励有效期：注册后 N 天内的充值才触发（0=永久有效） |

### 3.5 新增表：`user_referral_configs` — 用户级推广配置覆盖

当管理员需要给特定推广人（如 KOL、大客户）设置不同于全局的奖励参数时，在此表插入一行。**未配置的用户使用全局默认值。**

```sql
CREATE TABLE user_referral_configs (
    id                          BIGSERIAL PRIMARY KEY,
    user_id                     BIGINT NOT NULL REFERENCES users(id) UNIQUE,
    invitee_reward              DOUBLE PRECISION,       -- NULL = 使用全局默认
    inviter_reward              DOUBLE PRECISION,       -- NULL = 使用全局默认
    max_invites                 INT,                    -- NULL = 使用全局默认
    reward_expiry_days          INT,                    -- NULL = 使用全局默认
    ongoing_reward_enabled      BOOLEAN,                -- NULL = 使用全局默认
    ongoing_reward_type         VARCHAR(20),            -- NULL = 使用全局默认 (fixed/percentage)
    ongoing_reward_value        DOUBLE PRECISION,       -- NULL = 使用全局默认
    ongoing_reward_max_count    INT,                    -- NULL = 使用全局默认
    ongoing_reward_duration_days INT,                   -- NULL = 使用全局默认
    notes                       TEXT,                   -- 管理员备注（如"KOL合作"）
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### 配置解析优先级

```
用户级 user_referral_configs 中的非 NULL 字段
  ↓ 若为 NULL，降级到
全局 Settings 表中的对应键
```

示例：全局 `referral_inviter_reward = 2.0`，用户 A 的 `user_referral_configs.inviter_reward = 5.0`，则用户 A 的被邀请人首充时，用户 A 获得 ¥5.0 奖励而非 ¥2.0。

#### 配置读取封装（含缓存）

```go
// ReferralConfigResolver 统一解析推广配置，先查用户覆盖，再降级到全局
// 内置两层缓存：全局配置缓存 + 用户级配置缓存
type ReferralConfigResolver struct {
    settingService *SettingService
    configRepo     UserReferralConfigRepository
    cache          *sync.Map  // key: "user:{userID}" → *UserReferralConfig (带 TTL)
    globalCache    atomic.Value // *ReferralGlobalConfig (带 TTL)
    cacheTTL       time.Duration // 默认 7 天
}
```

#### 缓存策略

| 层级 | 缓存 Key | 过期策略 | 主动失效时机 |
|------|----------|----------|-------------|
| **全局配置** | `referral:global_config` | TTL 7 天 | 管理员更新全局推广设置时 |
| **用户级配置** | `referral:user_config:{userID}` | TTL 7 天 | 管理员更新/删除该用户配置时 |

#### 缓存读取流程

```go
func (r *ReferralConfigResolver) GetEffectiveConfig(ctx context.Context, referrerID int64) *EffectiveReferralConfig {
    // 1. 尝试从缓存读取用户级配置
    userCfg := r.getUserConfigFromCache(referrerID)
    if userCfg == nil {
        // 缓存未命中，查 DB
        dbCfg, err := r.configRepo.GetByUserID(ctx, referrerID)
        if err == nil {
            userCfg = dbCfg
        }
        // 写入缓存（包括"无配置"也缓存，避免穿透）
        r.setUserConfigCache(referrerID, userCfg)
    }

    // 2. 获取全局配置（同样有缓存）
    globalCfg := r.getGlobalConfig(ctx)

    // 3. 合并：用户级非 NULL 字段覆盖全局
    return mergeConfig(globalCfg, userCfg)
}
```

#### 缓存失效

```go
// 管理员更新用户级配置后调用
func (r *ReferralConfigResolver) InvalidateUserConfig(userID int64) {
    r.cache.Delete(fmt.Sprintf("user:%d", userID))
}

// 管理员更新全局推广设置后调用
func (r *ReferralConfigResolver) InvalidateGlobalConfig() {
    r.globalCache.Store((*ReferralGlobalConfig)(nil))
}
```

#### 防缓存穿透

对于没有自定义配置的用户，缓存一个空标记对象（`&UserReferralConfig{}`），避免每次都查 DB。TTL 到期后自然刷新。

#### 与现有缓存模式一致

项目已有 `billingCacheService.InvalidateUserBalance` 等缓存失效模式，`ReferralConfigResolver` 的缓存失效遵循相同模式：**写操作完成后立即失效对应缓存 key**。

---

## 4. 核心业务流程

### 4.1 推广码生成

```
用户首次访问「推广中心」页面
  → 检查 user.referral_code 是否为空
  → 若为空，生成 6-8 位随机短码（字母数字混合，排除易混淆字符 0/O/l/I）
  → 原子写入 user.referral_code（唯一约束，冲突重试）
  → 返回推广链接：{frontend_url}/register?ref={code}
```

### 4.2 被邀请人注册（奖励阶段一）

```
注册请求带 referral_code 参数（来自 URL ?ref=XXXX）
  → auth_service.RegisterWithVerification 中新增处理：
    1. 查找 referral_code 对应的 referrer 用户
    2. 验证：推广功能是否启用、referrer 是否存在且活跃、邀请上限是否达到
    3. 创建用户（现有逻辑）
    4. 创建 referral 记录：status=pending, invitee_reward_amount=配置值
    5. 设置 user.referred_by = referrer.id
    6. 发放被邀请人赠送余额：
       - 在 gift_balance_records 插入记录
       - source = 'referral_invitee'
       - expires_at = 根据 referral_reward_expiry_days 计算（0 则 NULL）
    7. 更新 referral.invitee_rewarded_at = NOW()
```

**触点**：`@backend/internal/service/auth_service.go` → `RegisterWithVerification`

### 4.3 邀请人奖励（奖励阶段二 — 首充触发）

```
被邀请人首次充值成功
  → payment_fulfillment.go → markCompleted 之后
  → 检查该用户是否有 pending 状态的 referral 记录
  → 若有：
    1. 查找 referral.referrer_id
    2. 在 gift_balance_records 插入记录：
       - user_id = referrer_id
       - source = 'referral_inviter'
       - amount = 配置的 referral_inviter_reward
    3. 更新 referral.status = 'completed'
    4. 更新 referral.inviter_rewarded_at = NOW()
    5. 失效 referrer 的余额缓存
```

**首充判断**：检查 `user.TotalRecharged` 在充值前是否为 0（即 TotalRecharged 从 0 变为 > 0 的第一笔）

**触点**：`@backend/internal/service/payment_fulfillment.go` → `markCompleted` 或 `doBalance` 末尾

### 4.4 赠送余额扣费（优先消费）

修改两处扣费逻辑，按 FIFO（先到期先消费，永不过期排最后）优先扣减赠送余额：

#### 4.4.1 生产路径：`deductUsageBillingBalance`

**文件**：`@backend/internal/repository/usage_billing_repo.go:341-357`

修改后逻辑（在同一事务内）：

```
1. 计算可用赠送余额总和（未过期、remaining > 0）
2. 若赠送余额 >= 扣减金额：
   - 按 COALESCE(expires_at, '2099-12-31') ASC, id ASC 顺序逐条扣减
   - 不动普通余额
3. 若赠送余额 < 扣减金额但 > 0：
   - 先全额扣完赠送余额
   - 剩余部分扣普通余额
4. 若赠送余额 = 0：
   - 直接扣普通余额（现有逻辑不变）
```

```sql
-- 在事务中执行：先扣赠送余额
WITH ordered_gifts AS (
    SELECT id, remaining,
        SUM(remaining) OVER (
            ORDER BY COALESCE(expires_at, '2099-12-31'::timestamptz) ASC, id ASC
            ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
        ) AS cumulative
    FROM gift_balance_records
    WHERE user_id = $2 AND remaining > 0
        AND (expires_at IS NULL OR expires_at > NOW())
),
to_deduct AS (
    SELECT id, remaining, cumulative,
        CASE
            WHEN cumulative <= $1 THEN remaining
            WHEN cumulative - remaining < $1 THEN $1 - (cumulative - remaining)
            ELSE 0
        END AS deduct_amount
    FROM ordered_gifts
    WHERE cumulative - remaining < $1
)
UPDATE gift_balance_records g
SET remaining = g.remaining - td.deduct_amount,
    updated_at = NOW()
FROM to_deduct td
WHERE g.id = td.id
RETURNING SUM(td.deduct_amount);

-- 计算剩余需从普通余额扣的部分
-- remaining_cost = $1 - gift_deducted
-- 若 remaining_cost > 0，执行现有的 UPDATE users SET balance = balance - remaining_cost
```

#### 4.4.2 降级路径：`DeductBalance`

**文件**：`@backend/internal/repository/user_repo.go:411-424`

同样改为先查赠送余额再扣普通余额，逻辑与上面一致，使用 Ent 事务包装。

### 4.5 赠送余额过期清理

```
定时任务（每小时或每天执行一次）：
UPDATE gift_balance_records
SET remaining = 0, updated_at = NOW()
WHERE expires_at IS NOT NULL
  AND expires_at <= NOW()
  AND remaining > 0;
```

可复用项目现有的定时任务框架（若有），或在应用启动时注册 goroutine ticker。

---

## 5. API 接口设计

### 5.1 用户侧 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/referral/info` | 获取推广信息（推广码、链接、概览统计） |
| POST | `/api/v1/referral/generate-code` | 生成/获取推广码 |
| GET | `/api/v1/referral/invitations` | 邀请明细列表（分页） |
| GET | `/api/v1/referral/stats` | 推广统计数据（趋势图用，按日/周/月） |
| GET | `/api/v1/referral/gift-balance` | 赠送余额明细（分页） |
| GET | `/api/v1/referral/gift-balance/summary` | 赠送余额汇总（当前可用、已消费、已过期） |

#### `/api/v1/referral/info` 响应示例

```json
{
  "referral_code": "A3F8K2",
  "referral_link": "https://site.com/register?ref=A3F8K2",
  "total_invitations": 15,
  "completed_invitations": 8,
  "pending_invitations": 7,
  "total_rewards_earned": 16.0,
  "gift_balance_available": 12.5,
  "referral_enabled": true,
  "inviter_reward": 2.0,
  "invitee_reward": 1.0,
  "max_invites": 0,
  "ongoing_reward_enabled": true,
  "ongoing_reward_type": "fixed",
  "ongoing_reward_value": 0.5,
  "ongoing_reward_max_count": 0,
  "ongoing_reward_duration_days": 0
}
```

#### `/api/v1/referral/stats` 响应示例

```json
{
  "period": "daily",
  "data": [
    { "date": "2026-04-15", "invitations": 3, "completions": 1, "rewards": 2.0 },
    { "date": "2026-04-16", "invitations": 1, "completions": 0, "rewards": 0 },
    ...
  ]
}
```

#### `/api/v1/referral/invitations` 响应示例

```json
{
  "items": [
    {
      "id": 42,
      "referee_email_masked": "u***@example.com",
      "status": "completed",
      "invitee_reward": 1.0,
      "inviter_reward": 2.0,
      "ongoing_reward_count": 3,
      "ongoing_reward_total": 1.5,
      "created_at": "2026-04-10T10:00:00Z",
      "inviter_rewarded_at": "2026-04-12T15:30:00Z"
    }
  ],
  "total": 15,
  "page": 1,
  "page_size": 20
}
```

### 5.2 管理员侧 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/referrals` | 全站推广列表（分页、可按用户/状态筛选） |
| GET | `/api/v1/admin/referrals/stats` | 全站推广统计概览 |
| GET | `/api/v1/admin/users/:id/referrals` | 指定用户的推广详情 |
| GET | `/api/v1/admin/users/:id/gift-balance` | 指定用户的赠送余额明细 |
| POST | `/api/v1/admin/users/:id/gift-balance` | 手动给用户发放/扣减赠送余额 |
| PUT | `/api/v1/admin/settings/referral` | 更新推广相关设置 |
| GET | `/api/v1/admin/settings/referral` | 获取推广相关设置 |
| GET | `/api/v1/admin/referrals/dashboard` | 推广数据看板（转化漏斗 + 趋势图表） |
| GET | `/api/v1/admin/referrals/leaderboard` | 推广达人榜 Top N |
| POST | `/api/v1/admin/gift-balance/batch` | 批量发放赠送余额 |
| GET | `/api/v1/admin/users/:id/referral-config` | 获取用户级推广配置（无则返回全局默认） |
| PUT | `/api/v1/admin/users/:id/referral-config` | 设置/更新用户级推广配置覆盖 |
| DELETE | `/api/v1/admin/users/:id/referral-config` | 删除用户级配置（恢复使用全局默认） |

#### 管理员手动发放赠送余额请求

```json
{
  "amount": 5.0,
  "expires_days": 30,
  "notes": "客服补偿"
}
```

#### 批量发放赠送余额请求

```json
{
  "target": "all",
  "user_ids": [],
  "amount": 1.0,
  "expires_days": 7,
  "notes": "周年庆活动赠送"
}
```

- `target`：`"all"` = 全站所有活跃用户，`"selected"` = 指定 user_ids
- 执行方式：后台异步执行（用户数多时避免超时），管理员可查看执行进度

#### `/api/v1/admin/referrals/dashboard` 响应示例

```json
{
  "funnel": {
    "total_referral_visits": 1200,
    "registrations": 350,
    "first_recharges": 120,
    "conversion_rate": "10.0%"
  },
  "trend": [
    { "date": "2026-04-15", "registrations": 15, "completions": 5, "rewards_total": 10.0 },
    { "date": "2026-04-16", "registrations": 8, "completions": 3, "rewards_total": 6.0 }
  ],
  "summary": {
    "total_referrals": 500,
    "total_completed": 180,
    "total_rewards_issued": 960.0,
    "total_gift_balance_outstanding": 420.5
  }
}
```

#### `/api/v1/admin/referrals/leaderboard` 响应示例

```json
{
  "period": "all_time",
  "top": [
    { "rank": 1, "user_id": 1001, "email": "top@example.com", "total_invites": 85, "completed": 42, "rewards_earned": 84.0 },
    { "rank": 2, "user_id": 2003, "email": "pro@example.com", "total_invites": 62, "completed": 30, "rewards_earned": 60.0 }
  ]
}
```

- 支持按时间筛选：`period` = `all_time` / `this_month` / `this_week`
- 支持自定义 Top N：查询参数 `limit`（默认 10）

#### `PUT /api/v1/admin/users/:id/referral-config` 请求示例

```json
{
  "inviter_reward": 5.0,
  "invitee_reward": 2.0,
  "ongoing_reward_value": 0.1,
  "ongoing_reward_type": "percentage",
  "ongoing_reward_max_count": 100,
  "max_invites": 500,
  "notes": "KOL合作 - 特殊佣金比例"
}
```

- **字段为 `null` 或不传**：该项使用全局默认值
- **字段有值**：覆盖全局默认值
- **DELETE 接口**：删除整行，完全恢复使用全局配置

#### `GET /api/v1/admin/users/:id/referral-config` 响应示例

```json
{
  "has_custom_config": true,
  "config": {
    "inviter_reward": 5.0,
    "invitee_reward": null,
    "max_invites": 500,
    "reward_expiry_days": null,
    "ongoing_reward_enabled": null,
    "ongoing_reward_type": "percentage",
    "ongoing_reward_value": 0.1,
    "ongoing_reward_max_count": 100,
    "ongoing_reward_duration_days": null,
    "notes": "KOL合作 - 特殊佣金比例"
  },
  "effective": {
    "inviter_reward": 5.0,
    "invitee_reward": 1.0,
    "max_invites": 500,
    "reward_expiry_days": 0,
    "ongoing_reward_enabled": true,
    "ongoing_reward_type": "percentage",
    "ongoing_reward_value": 0.1,
    "ongoing_reward_max_count": 100,
    "ongoing_reward_duration_days": 0
  }
}
```

- `config`：用户级自定义值（null = 使用全局）
- `effective`：最终生效值（合并全局和用户级后的结果）

---

## 6. 前端设计

### 6.1 路由与导航

**新增路由**：

```typescript
{
  path: '/referral',
  name: 'Referral',
  component: () => import('@/views/user/ReferralView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: false,
    title: 'Referral Center',
    titleKey: 'referral.title',
    descriptionKey: 'referral.description',
    requiresReferral: true  // 新增 meta，用于功能开关控制
  }
}
```

**侧边栏**：在 `AppSidebar.vue` 的 `userNavItems` 中，在 `redeem` 之前插入：

```typescript
...(appStore.cachedPublicSettings?.referral_enabled
  ? [{ path: '/referral', label: t('nav.referral'), icon: MegaphoneIcon }]
  : []),
```

**管理员侧**：在 `adminNavItems` 中新增推广管理入口。

> **注意**：管理员 Dashboard 页面**不放置**推广营销相关图表或卡片，所有推广数据看板、转化漏斗、达人榜等统一集中在「推广管理」页（`/admin/referrals`）下展示。

### 6.2 用户推广中心页面布局

```
┌─────────────────────────────────────────────────┐
│  推广中心                                        │
├─────────────────────────────────────────────────┤
│  推广链接卡片                                    │
│  ┌───────────────────────────────────────────┐  │
│  │ 你的推广链接: https://site.com/reg?ref=XX │  │
│  │ [复制链接]  [复制推广码]                    │  │
│  └───────────────────────────────────────────┘  │
├─────────────────────────────────────────────────┤
│  概览卡片（4 列）                                │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐          │
│  │总邀请│ │已完成│ │待首充│ │总奖励│           │
│  │  15  │ │  8   │ │  7   │ │ ¥16  │           │
│  └──────┘ └──────┘ └──────┘ └──────┘          │
├─────────────────────────────────────────────────┤
│  邀请趋势图（折线图，按日/周/月切换）            │
│  ┌───────────────────────────────────────────┐  │
│  │  📈 邀请数 + 完成数 双线图                 │  │
│  └───────────────────────────────────────────┘  │
├─────────────────────────────────────────────────┤
│  邀请明细表                                      │
│  ┌───────────────────────────────────────────┐  │
│  │ 被邀请人 | 状态 | 被邀请人奖励 | 邀请人奖 │  │
│  │ 励 | 注册时间 | 完成时间                   │  │
│  └───────────────────────────────────────────┘  │
├─────────────────────────────────────────────────┤
│  赠送余额明细                                    │
│  ┌───────────────────────────────────────────┐  │
│  │ 可用: ¥12.5 | 已消费: ¥3.5 | 已过期: ¥0  │  │
│  ├───────────────────────────────────────────┤  │
│  │ 来源 | 金额 | 剩余 | 过期时间 | 创建时间  │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### 6.3 注册页面改动

在 `RegisterView.vue` 中：
- 解析 URL 参数 `ref`，自动填入推广码字段
- 在注册表单中新增「推广码」输入框（可选字段，预填但可修改）
- 注册 API 请求中传入 `referral_code` 参数

### 6.4 Dashboard 余额显示改动

在用户 Dashboard 的余额显示区域：
- 普通余额和赠送余额分行显示
- 赠送余额标注「优先消费」和最近到期时间

### 6.5 管理员推广管理页面

新增 `/admin/referrals` 路由，包含以下模块：

```
┌─────────────────────────────────────────────────────┐
│  推广管理                                            │
├─────────────────────────────────────────────────────┤
│  ❶ 数据看板                                         │
│  ┌──────────────────────────────────────────────┐   │
│  │  转化漏斗                                     │   │
│  │  推广访问 1200 → 注册 350 → 首充 120         │   │
│  │  转化率: 10.0%                                │   │
│  └──────────────────────────────────────────────┘   │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐              │
│  │总推广│ │已完成│ │总奖励│ │待发放│               │
│  │ 500  │ │ 180  │ │¥960  │ │¥420  │               │
│  └──────┘ └──────┘ └──────┘ └──────┘              │
│  ┌──────────────────────────────────────────────┐   │
│  │  📈 注册+首充趋势图（按日/周/月切换）          │   │
│  └──────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│  ❷ 推广达人榜 Top N                                 │
│  ┌──────────────────────────────────────────────┐   │
│  │ 排名 | 用户 | 总邀请 | 已完成 | 获得奖励     │   │
│  │  1   | top@ |  85    |  42    | ¥84          │   │
│  │  2   | pro@ |  62    |  30    | ¥60          │   │
│  │  时间筛选: [全部] [本月] [本周]               │   │
│  └──────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│  ❸ 推广关系列表                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │ 搜索/筛选（用户、状态）                       │   │
│  │ 邀请人 | 被邀请人 | 状态 | 奖励 | 时间       │   │
│  │ 点击行 → 查看详细推广链和赠送余额明细         │   │
│  └──────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│  ❹ 操作区                                           │
│  ┌──────────────────────────────────────────────┐   │
│  │ [批量发放赠送余额]  [推广设置]                │   │
│  │                                               │   │
│  │ 批量发放弹窗：                                │   │
│  │   目标: ○全站活跃用户  ○指定用户              │   │
│  │   金额: [___]  过期天数: [___]                │   │
│  │   备注: [________________]                    │   │
│  │   [确认发放]                                  │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

#### 功能细节

- **数据看板**：转化漏斗展示从推广链接点击 → 注册 → 首充的转化率；概览卡片展示核心指标。推广访问数通过注册页 `?ref=` 参数记录（后端在注册页加载时轻量级计数，或前端上报），不做独立的链接跳转中间页
- **推广达人榜**：展示 Top 10/20/50 邀请人排名，支持按时间段筛选，激励头部推广者
- **批量赠送余额**：支持全站或指定用户批量发放，后台异步执行，显示执行进度
- **手动发放/扣减**：在用户详情中可单独发放或扣减赠送余额

---

## 7. 注册流程改动详情

**现有注册参数**：`email, password, verifyCode, promoCode, invitationCode`

**新增参数**：`referralCode`（可选）

**处理逻辑**（在 `RegisterWithVerification` 中）：

```go
// 在用户创建成功后、promoCode 处理之前：
if referralCode != "" && s.settingService.IsReferralEnabled(ctx) {
    referrer, err := s.userRepo.GetByReferralCode(ctx, referralCode)
    if err == nil && referrer.IsActive() {
        // 检查邀请上限
        if !s.referralService.IsMaxInvitesReached(ctx, referrer.ID) {
            // 创建推广关系
            referral := &Referral{
                ReferrerID:          referrer.ID,
                RefereeID:           user.ID,
                ReferralCode:        referralCode,
                Status:              "pending",
                InviteeRewardAmount: s.settingService.GetReferralInviteeReward(ctx),
                InviterRewardAmount: s.settingService.GetReferralInviterReward(ctx),
            }
            s.referralService.CreateAndRewardInvitee(ctx, referral)
            // 设置 user.referred_by
            s.userRepo.SetReferredBy(ctx, user.ID, referrer.ID)
        }
    }
    // 推广码无效时静默忽略，不阻断注册
}
```

---

## 8. 充值奖励触发逻辑详情

**触点**：`payment_fulfillment.go` → `doBalance` 或 `markCompleted`

### 8.1 首充奖励（一次性）

```go
// 在 markCompleted 成功后：
func (s *PaymentService) checkAndRewardReferrer(ctx context.Context, userID int64, rechargeAmount float64) {
    // 检查是否为首充：TotalRecharged 在本次充值前为 0
    user, err := s.userRepo.GetByID(ctx, userID)
    if err != nil || user.TotalRecharged > rechargeAmount {
        goto ongoingCheck // 非首充，跳到持续奖励检查
    }

    {
        // 查找 pending 的推广关系
        referral, err := s.referralRepo.GetPendingByRefereeID(ctx, userID)
        if err != nil {
            return // 无推广关系
        }
        // 发放邀请人首充奖励
        s.referralService.RewardReferrer(ctx, referral)
    }

ongoingCheck:
    // 持续奖励检查
    s.checkOngoingReward(ctx, userID, rechargeAmount)
}
```

### 8.2 持续奖励（每次充值）

```go
func (s *PaymentService) checkOngoingReward(ctx context.Context, userID int64, rechargeAmount float64) {
    // 检查持续奖励是否启用
    if !s.settingService.IsOngoingRewardEnabled(ctx) {
        return
    }

    // 查找该用户的推广关系（无论 pending/completed）
    referral, err := s.referralRepo.GetByRefereeID(ctx, userID)
    if err != nil {
        return // 无推广关系
    }

    // 检查持续奖励次数上限
    maxCount := s.settingService.GetOngoingRewardMaxCount(ctx)
    if maxCount > 0 && referral.OngoingRewardCount >= maxCount {
        return // 已达上限
    }

    // 检查持续奖励有效期
    durationDays := s.settingService.GetOngoingRewardDurationDays(ctx)
    if durationDays > 0 {
        deadline := referral.CreatedAt.AddDate(0, 0, durationDays)
        if time.Now().After(deadline) {
            return // 已过有效期
        }
    }

    // 计算奖励金额
    rewardType := s.settingService.GetOngoingRewardType(ctx)    // "fixed" or "percentage"
    rewardValue := s.settingService.GetOngoingRewardValue(ctx)
    var rewardAmount float64
    if rewardType == "percentage" {
        rewardAmount = rechargeAmount * rewardValue // e.g. 100 * 0.05 = 5.0
    } else {
        rewardAmount = rewardValue // 固定金额
    }
    if rewardAmount <= 0 {
        return
    }

    // 发放持续奖励给推广人
    s.referralService.RewardOngoing(ctx, referral, rewardAmount)
    // → 创建 gift_balance_record (source='referral_ongoing')
    // → 更新 referral.ongoing_reward_count += 1
    // → 更新 referral.ongoing_reward_total += rewardAmount
}
```

### 8.3 奖励模式总结

| 模式 | 触发时机 | 配置开关 | 次数 |
|------|----------|----------|------|
| **首充奖励** | 被邀请人首次充值 | `referral_enabled` | 1 次 |
| **持续奖励** | 被邀请人每次充值 | `referral_ongoing_reward_enabled` | 可配置上限 |

两种模式**独立控制**：
- 仅开启首充奖励 → 传统推广模式
- 同时开启持续奖励 → 首充奖励 + 后续每笔充值额外奖励
- 仅开启持续奖励（关闭首充） → 纯佣金模式

**注意**：需要在充值金额入账（`TotalRecharged` 更新）之前获取旧的 `TotalRecharged` 值来判断首充。

---

## 9. 安全与防刷措施

| 措施 | 说明 |
|------|------|
| 自我邀请防护 | 注册时校验 referrer_id ≠ user.id |
| 推广码唯一约束 | DB UNIQUE 约束 + 冲突重试 |
| 邀请上限 | `referral_max_invites` 配置，0 = 不限 |
| 推广关系唯一 | `referee_id` UNIQUE 约束，一个用户只能被邀请一次 |
| 推广码格式 | 6-8 位字母数字混合，排除 0/O/l/I 避免混淆 |
| 奖励幂等性 | `invitee_rewarded_at` / `inviter_rewarded_at` 非空则跳过 |
| 首充判断 | 基于 `TotalRecharged` 历史值，避免重复触发 |
| 推广码无效静默 | 无效推广码不阻断注册流程 |
| 赠送余额不可退款 | 退款逻辑仅操作普通余额 |

---

## 10. 后端代码改动范围

| 文件/模块 | 改动类型 | 说明 |
|-----------|----------|------|
| `ent/schema/` | 新增 | `GiftBalanceRecord`, `Referral` schema |
| `migrations/` | 新增 | DDL 迁移文件 |
| `internal/service/referral_service.go` | 新增 | 推广业务逻辑 |
| `internal/service/gift_balance_service.go` | 新增 | 赠送余额业务逻辑 |
| `internal/service/user.go` | 修改 | User struct 增加 `ReferralCode`, `ReferredBy`, `GiftBalance` |
| `internal/service/auth_service.go` | 修改 | `RegisterWithVerification` 增加 referralCode 处理 |
| `internal/service/payment_fulfillment.go` | 修改 | `markCompleted` 后检查首充奖励 |
| `internal/service/setting_service.go` | 修改 | 新增推广相关设置读取方法 |
| `internal/repository/usage_billing_repo.go` | 修改 | `deductUsageBillingBalance` 增加赠送余额优先扣减 |
| `internal/repository/user_repo.go` | 修改 | `DeductBalance` 增加赠送余额优先扣减 |
| `internal/repository/gift_balance_repo.go` | 新增 | 赠送余额 CRUD |
| `internal/repository/referral_repo.go` | 新增 | 推广关系 CRUD |
| `internal/service/referral_config_resolver.go` | 新增 | 两级配置解析器（用户级 → 全局降级） |
| `internal/repository/user_referral_config_repo.go` | 新增 | 用户级推广配置 CRUD |
| `internal/handler/referral_handler.go` | 新增 | 用户侧推广 API |
| `internal/handler/admin/referral_handler.go` | 新增 | 管理员推广 API |
| `internal/handler/dto/` | 修改 | 新增 DTO 映射 |

### 前端改动范围

| 文件/模块 | 改动类型 | 说明 |
|-----------|----------|------|
| `views/user/ReferralView.vue` | 新增 | 推广中心页面 |
| `views/admin/ReferralManageView.vue` | 新增 | 管理员推广管理页面 |
| `views/auth/RegisterView.vue` | 修改 | 增加推广码字段 |
| `views/user/DashboardView.vue` | 修改 | 增加赠送余额显示 |
| `api/referral.ts` | 新增 | 推广 API 调用 |
| `router/index.ts` | 修改 | 新增路由 |
| `components/layout/AppSidebar.vue` | 修改 | 新增菜单项 |
| `stores/` | 修改 | public settings 增加 `referral_enabled` |
| `i18n/` | 修改 | 新增推广相关翻译键 |

---

## 11. 过期清理机制

### 方案

在应用启动时注册一个后台 goroutine，定期（默认每小时）清理过期的赠送余额：

```go
func (s *GiftBalanceService) StartExpiryCleanup(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.cleanExpiredGiftBalances(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (s *GiftBalanceService) cleanExpiredGiftBalances(ctx context.Context) {
    // UPDATE gift_balance_records
    // SET remaining = 0, updated_at = NOW()
    // WHERE expires_at IS NOT NULL AND expires_at <= NOW() AND remaining > 0
}
```

---

## 12. i18n 键规划

```
referral.title = 推广中心
referral.description = 邀请好友，获取赠送余额奖励
referral.yourLink = 你的推广链接
referral.copyLink = 复制链接
referral.copyCode = 复制推广码
referral.totalInvitations = 总邀请
referral.completedInvitations = 已完成
referral.pendingInvitations = 待首充
referral.totalRewards = 总奖励
referral.invitationTrend = 邀请趋势
referral.invitationList = 邀请明细
referral.giftBalance = 赠送余额
referral.giftBalanceAvailable = 可用
referral.giftBalanceConsumed = 已消费
referral.giftBalanceExpired = 已过期
referral.status.pending = 待首充
referral.status.completed = 已完成
nav.referral = 推广中心
admin.referral.title = 推广管理
admin.referral.description = 管理推广关系和赠送余额
admin.referral.dashboard = 数据看板
admin.referral.funnel = 转化漏斗
admin.referral.funnel.visits = 推广访问
admin.referral.funnel.registrations = 注册
admin.referral.funnel.firstRecharges = 首充
admin.referral.funnel.conversionRate = 转化率
admin.referral.leaderboard = 推广达人榜
admin.referral.leaderboard.rank = 排名
admin.referral.leaderboard.allTime = 全部
admin.referral.leaderboard.thisMonth = 本月
admin.referral.leaderboard.thisWeek = 本周
admin.referral.batchGrant = 批量发放赠送余额
admin.referral.batchGrant.targetAll = 全站活跃用户
admin.referral.batchGrant.targetSelected = 指定用户
admin.referral.batchGrant.confirm = 确认发放
admin.referral.batchGrant.progress = 执行进度
admin.referral.totalRewardsIssued = 总奖励发放
admin.referral.outstandingGiftBalance = 待消费赠送余额
```

---

## 附录：决策日志

| 日期 | 决策 | 理由 |
|------|------|------|
| 2026-04-21 | 采用方案 A（独立赠送余额表） | 支持不同过期时间、可审计、扩展性好 |
| 2026-04-21 | 推广码惰性生成 | 避免给所有用户预生成码，按需生成更经济 |
| 2026-04-21 | 首充判断用 TotalRecharged | 已有字段，无需额外记录 |
| 2026-04-21 | 推广码无效静默忽略 | 不阻断注册体验 |
| 2026-04-21 | 赠送余额扣费 FIFO | 先到期先消费，永不过期排最后，最合理 |
| 2026-04-21 | 用户面板独立页面 | 内容丰富，需要独立空间展示 |
| 2026-04-21 | 管理端基础增强（看板+达人榜+批量赠送） | 性价比最高的营销工具，不增加过多复杂度 |
| 2026-04-21 | 支持持续奖励（每次充值触发） | 用户要求，与首充奖励独立控制，支持固定金额/百分比两种模式 |
| 2026-04-21 | 两级配置（全局 + 用户级覆盖） | 支持 KOL/大客户特殊奖励，NULL 字段自动降级到全局默认，简洁高效 |
