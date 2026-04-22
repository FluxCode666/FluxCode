package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// RPM 计数器缓存常量定义
//
// 设计说明：
// 使用 Redis 简单计数器跟踪每个账号每分钟的请求数：
// - Key: rpm:{accountID}:{minuteTimestamp}
// - Value: 当前分钟内的请求计数
// - TTL: 120 秒（覆盖当前分钟 + 一定冗余）
//
// 使用 Lua 脚本在单次 EVAL 内完成 TIME + INCR + EXPIRE，
// 避免独立 TIME 调用产生的额外 RTT。
// 每次调用只操作单个 key，无 CROSSSLOT 风险。
const (
	// RPM 计数器键前缀
	// 格式: rpm:{accountID}:{minuteTimestamp}
	rpmKeyPrefix = "rpm:"

	// RPM 计数器 TTL（120 秒，覆盖当前分钟窗口 + 冗余）
	rpmKeyTTL = 120 * time.Second
)

// RPMCacheImpl RPM 计数器缓存 Redis 实现
type RPMCacheImpl struct {
	rdb *redis.Client
}

// NewRPMCache 创建 RPM 计数器缓存
func NewRPMCache(rdb *redis.Client) service.RPMCache {
	return &RPMCacheImpl{rdb: rdb}
}

// rpmIncrementScript 在单次 EVAL 内完成 TIME + INCR + EXPIRE，避免独立 TIME 调用的额外 RTT。
// KEYS[1] = rpm:{accountID} 前缀（不包含分钟后缀，脚本内动态拼接）
// ARGV[1] = TTL（秒）
// 返回：当前分钟计数
var rpmIncrementScript = redis.NewScript(`
	local t = redis.call('TIME')
	local minuteTS = math.floor(tonumber(t[1]) / 60)
	local key = KEYS[1] .. ":" .. minuteTS
	local count = redis.call('INCR', key)
	redis.call('EXPIRE', key, tonumber(ARGV[1]))
	return count
`)

// rpmGetScript 在单次 EVAL 内完成 TIME + GET，避免独立 TIME 调用的额外 RTT。
// KEYS[1] = rpm:{accountID} 前缀
// 返回：当前分钟计数（0 表示无记录）
var rpmGetScript = redis.NewScript(`
	local t = redis.call('TIME')
	local minuteTS = math.floor(tonumber(t[1]) / 60)
	local key = KEYS[1] .. ":" .. minuteTS
	local val = redis.call('GET', key)
	if val == false then return 0 end
	return tonumber(val)
`)

// IncrementRPM 原子递增并返回当前分钟的计数
// 使用 Lua 脚本在单次 EVAL 内完成 TIME + INCR + EXPIRE（1 RTT）
// 每次只操作单个 key，无 CROSSSLOT 风险
func (c *RPMCacheImpl) IncrementRPM(ctx context.Context, accountID int64) (int, error) {
	keyPrefix := fmt.Sprintf("%s%d", rpmKeyPrefix, accountID)
	ttlSeconds := int(rpmKeyTTL.Seconds())

	result, err := rpmIncrementScript.Run(ctx, c.rdb, []string{keyPrefix}, ttlSeconds).Int()
	if err != nil {
		return 0, fmt.Errorf("rpm increment: %w", err)
	}
	return result, nil
}

// GetRPM 获取当前分钟的 RPM 计数
// 使用 Lua 脚本在单次 EVAL 内完成 TIME + GET（1 RTT）
func (c *RPMCacheImpl) GetRPM(ctx context.Context, accountID int64) (int, error) {
	keyPrefix := fmt.Sprintf("%s%d", rpmKeyPrefix, accountID)

	result, err := rpmGetScript.Run(ctx, c.rdb, []string{keyPrefix}).Int()
	if err != nil {
		return 0, fmt.Errorf("rpm get: %w", err)
	}
	return result, nil
}

// GetRPMBatch 批量获取多个账号的 RPM 计数（使用 Pipeline）
// TIME 合并到 Pipeline 中，避免独立调用的额外 RTT（1 RTT 总计）
func (c *RPMCacheImpl) GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return map[int64]int{}, nil
	}

	// 将 TIME 合并到 Pipeline 中，避免独立 RTT
	pipe := c.rdb.Pipeline()
	timeCmd := pipe.Time(ctx)
	cmds := make(map[int64]*redis.StringCmd, len(accountIDs))
	// 先添加所有 GET 命令（使用占位 key，实际 key 在 Exec 后根据 TIME 结果重建）
	// 不可行：Pipeline 命令必须在 Exec 前全部构造完成。
	// 因此改为两步：先 Exec TIME，再用结果构建 GET Pipeline。
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("rpm batch get time: %w", err)
	}
	serverTime := timeCmd.Val()
	minuteTS := serverTime.Unix() / 60
	minuteSuffix := strconv.FormatInt(minuteTS, 10)

	// 使用 Pipeline 批量 GET
	pipe = c.rdb.Pipeline()
	for _, id := range accountIDs {
		key := fmt.Sprintf("%s%d:%s", rpmKeyPrefix, id, minuteSuffix)
		cmds[id] = pipe.Get(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("rpm batch get: %w", err)
	}

	result := make(map[int64]int, len(accountIDs))
	for id, cmd := range cmds {
		if val, err := cmd.Int(); err == nil {
			result[id] = val
		} else {
			result[id] = 0
		}
	}
	return result, nil
}
