package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type successfulRequestRecordRepository struct {
	db *sql.DB
}

func NewSuccessfulRequestRecordRepository(db *sql.DB) service.SuccessfulRequestRecordRepository {
	return &successfulRequestRecordRepository{db: db}
}

func (r *successfulRequestRecordRepository) Create(ctx context.Context, record *service.SuccessfulRequestRecord) (bool, error) {
	if record == nil {
		return false, nil
	}
	if r == nil || r.db == nil {
		return false, fmt.Errorf("successful request record repository is not ready")
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO usage_log_payloads (
			event_id, usage_log_id, user_id, api_key_id, group_id,
			trace_id, request_id, client_request_id,
			method, endpoint, route_pattern, model, status_code,
			stream, client_disconnected, duration_ms,
			request_content_type, response_content_type,
			request_body, response_body,
			request_body_bytes, response_body_bytes,
			request_body_truncated, response_body_truncated,
			created_at
		) VALUES (
			$1,
			COALESCE(
				$2,
				(
					SELECT id
					FROM usage_logs
					WHERE request_id = 'client:' || $8
						AND api_key_id = $4
					ORDER BY created_at DESC, id DESC
					LIMIT 1
				)
			),
			$3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18,
			$19, $20,
			$21, $22,
			$23, $24,
			$25
		)
		ON CONFLICT (event_id) DO NOTHING
	`,
		strings.TrimSpace(record.EventID),
		record.UsageLogID,
		record.UserID,
		record.APIKeyID,
		record.GroupID,
		nullableTrimmedString(record.TraceID),
		strings.TrimSpace(record.RequestID),
		nullableTrimmedString(record.ClientRequestID),
		strings.TrimSpace(record.Method),
		strings.TrimSpace(record.Endpoint),
		nullableTrimmedString(record.RoutePattern),
		nullableTrimmedString(record.Model),
		record.StatusCode,
		record.Stream,
		record.ClientDisconnected,
		record.DurationMS,
		nullableTrimmedString(record.RequestContentType),
		nullableTrimmedString(record.ResponseContentType),
		record.RequestBody,
		record.ResponseBody,
		record.RequestBodyBytes,
		record.ResponseBodyBytes,
		record.RequestTruncated,
		record.ResponseTruncated,
		record.CreatedAt.UTC(),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected < 0 || affected > 1 {
		return false, fmt.Errorf("unexpected successful request record affected rows: %d", affected)
	}
	return affected == 1, nil
}

func (r *successfulRequestRecordRepository) ReconcileUnlinked(ctx context.Context, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("successful request record repository is not ready")
	}
	if limit <= 0 {
		limit = 500
	}
	result, err := r.db.ExecContext(ctx, `
		WITH matches AS (
			SELECT DISTINCT ON (payload.created_at, payload.id)
				payload.id AS payload_id,
				usage_log.id AS usage_log_id
			FROM usage_log_payloads AS payload
			JOIN usage_logs AS usage_log
				ON usage_log.request_id = 'client:' || payload.client_request_id
				AND usage_log.api_key_id = payload.api_key_id
			WHERE payload.usage_log_id IS NULL
				AND payload.client_request_id IS NOT NULL
				AND payload.client_request_id <> ''
				-- 正文与 usage_logs 正常只会存在秒级写入时序差。限定最近 24 小时，
				-- 避免使用记录清理后反复扫描由 ON DELETE SET NULL 保留的历史正文。
				AND payload.created_at >= NOW() - INTERVAL '24 hours'
			ORDER BY payload.created_at DESC, payload.id DESC,
				usage_log.created_at DESC, usage_log.id DESC
			LIMIT $1
		)
		UPDATE usage_log_payloads AS payload
		SET usage_log_id = matches.usage_log_id
		FROM matches
		WHERE payload.id = matches.payload_id
			AND payload.usage_log_id IS NULL
	`, limit)
	if err != nil {
		return 0, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if updated < 0 || updated > int64(limit) {
		return 0, fmt.Errorf("unexpected usage log payload reconciliation affected rows: %d", updated)
	}
	return updated, nil
}

func nullableTrimmedString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
