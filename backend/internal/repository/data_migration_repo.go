package repository

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type DataMigrationRepository struct{}

func NewDataMigrationRepository() *DataMigrationRepository {
	return &DataMigrationRepository{}
}

var dataMigrationPhaseTables = map[string][]string{
	"core": {
		"groups",
		"users",
		"settings",
	},
	"access_control": {
		"user_allowed_groups",
		"orphan_allowed_groups_audit",
		"user_attribute_definitions",
		"user_attribute_values",
	},
	"infrastructure": {
		"proxies",
		"accounts",
		"account_groups",
	},
	"api_keys": {
		"api_keys",
	},
	"subscriptions": {
		"user_subscriptions",
		"subscription_grants",
		"redeem_codes",
	},
	"pricing": {
		"pricing_plan_groups",
		"pricing_plans",
	},
	"pool_monitor": {
		"account_pool_alert_configs",
		"proxy_usage_metrics_hourly",
	},
	"usage": {
		"usage_logs",
	},
	"ops_metrics": {
		"ops_metrics_hourly",
		"ops_metrics_daily",
	},
	"billing": {
		"billing_usage_entries",
	},
}

var dataMigrationPhaseOrder = []string{
	"core",
	"access_control",
	"infrastructure",
	"api_keys",
	"subscriptions",
	"pricing",
	"pool_monitor",
	"usage",
	"ops_metrics",
	"billing",
}

func (r *DataMigrationRepository) BuildPlan(
	ctx context.Context,
	sourceDB *sql.DB,
	targetDB *sql.DB,
	sourceSchema string,
	targetSchema string,
	phase string,
) ([]service.DataMigrationPhaseReport, error) {
	return r.run(ctx, sourceDB, targetDB, sourceSchema, targetSchema, phase, false, false)
}

func (r *DataMigrationRepository) ApplyPlan(
	ctx context.Context,
	sourceDB *sql.DB,
	targetDB *sql.DB,
	sourceSchema string,
	targetSchema string,
	phase string,
	resetTarget bool,
) ([]service.DataMigrationPhaseReport, error) {
	return r.run(ctx, sourceDB, targetDB, sourceSchema, targetSchema, phase, true, resetTarget)
}

func (r *DataMigrationRepository) run(
	ctx context.Context,
	sourceDB *sql.DB,
	targetDB *sql.DB,
	sourceSchema string,
	targetSchema string,
	phase string,
	apply bool,
	resetTarget bool,
) ([]service.DataMigrationPhaseReport, error) {
	phases, err := resolveDataMigrationPhases(phase)
	if err != nil {
		return nil, err
	}

	// Apply 模式下临时禁用目标库所有外键/触发器约束，避免跨阶段 TRUNCATE CASCADE
	// 级联清空已迁移的表，以及源库存在孤立外键数据导致 INSERT 失败。
	// session_replication_role = 'replica' 会让当前连接跳过所有用户触发器（含 FK 检查）。
	if apply && targetDB != nil {
		if _, err := targetDB.ExecContext(ctx, "SET session_replication_role = 'replica'"); err != nil {
			return nil, fmt.Errorf("disable FK constraints: %w", err)
		}
		defer func() {
			_, _ = targetDB.ExecContext(ctx, "SET session_replication_role = 'origin'")
		}()
	}

	reports := make([]service.DataMigrationPhaseReport, 0, len(phases))
	for _, phaseName := range phases {
		tables := dataMigrationPhaseTables[phaseName]
		phaseReport := service.DataMigrationPhaseReport{
			Phase:  phaseName,
			Tables: make([]service.DataMigrationTableReport, 0, len(tables)),
		}
		for _, table := range tables {
			tableReport, err := r.processTable(ctx, sourceDB, targetDB, sourceSchema, targetSchema, table, apply, resetTarget)
			if err != nil {
				return nil, fmt.Errorf("phase %s table %s: %w", phaseName, table, err)
			}
			phaseReport.Tables = append(phaseReport.Tables, tableReport)
		}
		reports = append(reports, phaseReport)
	}

	return reports, nil
}

func (r *DataMigrationRepository) processTable(
	ctx context.Context,
	sourceDB *sql.DB,
	targetDB *sql.DB,
	sourceSchema string,
	targetSchema string,
	table string,
	apply bool,
	resetTarget bool,
) (service.DataMigrationTableReport, error) {
	sourceCount, err := countMigrationRows(ctx, sourceDB, sourceSchema, table)
	if err != nil {
		return service.DataMigrationTableReport{}, err
	}
	targetCount, err := countMigrationRows(ctx, targetDB, targetSchema, table)
	if err != nil {
		return service.DataMigrationTableReport{}, err
	}

	report := service.DataMigrationTableReport{
		Table:      table,
		SourceRows: sourceCount,
		TargetRows: targetCount,
		Difference: sourceCount - targetCount,
	}
	if !apply {
		return report, nil
	}

	columns, err := listMigrationColumns(ctx, targetDB, targetSchema, table)
	if err != nil {
		return service.DataMigrationTableReport{}, err
	}
	sourceColumns, err := listMigrationColumns(ctx, sourceDB, sourceSchema, table)
	if err != nil {
		return service.DataMigrationTableReport{}, err
	}
	columns = intersectMigrationColumns(columns, sourceColumns)
	if len(columns) == 0 {
		return service.DataMigrationTableReport{}, fmt.Errorf("no columns found")
	}

	if resetTarget {
		if _, err := targetDB.ExecContext(ctx, fmt.Sprintf(
			"TRUNCATE TABLE %s RESTART IDENTITY CASCADE",
			qualifyMigrationTable(targetSchema, table),
		)); err != nil {
			return service.DataMigrationTableReport{}, fmt.Errorf("truncate target: %w", err)
		}
	}

	rows, err := sourceDB.QueryContext(ctx, fmt.Sprintf(
		"SELECT to_jsonb(t) FROM %s AS t",
		qualifyMigrationTable(sourceSchema, table),
	))
	if err != nil {
		return service.DataMigrationTableReport{}, fmt.Errorf("query source rows: %w", err)
	}
	defer rows.Close()

	columnList := joinMigrationColumns(columns)
	conflictClause := ""
	if !resetTarget {
		// 非 reset 模式下尽量做到幂等：遇到唯一约束冲突时跳过，避免重复写入。
		conflictClause = " ON CONFLICT DO NOTHING"
	}
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM jsonb_populate_recordset(NULL::%s, $1::jsonb)%s",
		qualifyMigrationTable(targetSchema, table),
		columnList,
		columnList,
		qualifyMigrationTable(targetSchema, table),
		conflictClause,
	)

	// Batch inserts dramatically reduce round trips (critical for large tables like usage_logs).
	const batchSize = 200

	var copiedRows int64
	batch := make([][]byte, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		var buf bytes.Buffer
		buf.Grow(2 + len(batch)*256) // rough prealloc, avoids a lot of small growth on large tables
		buf.WriteByte('[')
		for i, item := range batch {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(item)
		}
		buf.WriteByte(']')

		res, err := targetDB.ExecContext(ctx, insertSQL, buf.String())
		if err != nil {
			return err
		}
		if rowsAffected, err := res.RowsAffected(); err == nil {
			copiedRows += rowsAffected
		} else {
			// Fallback: assume all rows inserted when RowsAffected is unavailable (should be rare with pq).
			copiedRows += int64(len(batch))
		}

		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return service.DataMigrationTableReport{}, fmt.Errorf("scan source row: %w", err)
		}
		batch = append(batch, payload)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return service.DataMigrationTableReport{}, fmt.Errorf("insert target rows: %w", err)
			}
		}
	}
	if err := flush(); err != nil {
		return service.DataMigrationTableReport{}, fmt.Errorf("insert target rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return service.DataMigrationTableReport{}, fmt.Errorf("iterate source rows: %w", err)
	}

	if hasMigrationColumn(columns, "id") {
		if err := ensureMigrationSerialSequence(ctx, targetDB, targetSchema, table, "id"); err != nil {
			return service.DataMigrationTableReport{}, fmt.Errorf("reset target sequence: %w", err)
		}
	}

	targetAfter, err := countMigrationRows(ctx, targetDB, targetSchema, table)
	if err != nil {
		return service.DataMigrationTableReport{}, err
	}

	report.TargetRows = targetAfter
	report.CopiedRows = copiedRows
	report.Difference = sourceCount - targetAfter
	return report, nil
}

func resolveDataMigrationPhases(phase string) ([]string, error) {
	trimmed := strings.TrimSpace(phase)
	if trimmed == "" || trimmed == "all" {
		return append([]string(nil), dataMigrationPhaseOrder...), nil
	}
	if _, ok := dataMigrationPhaseTables[trimmed]; !ok {
		return nil, fmt.Errorf("unsupported migration phase: %s", trimmed)
	}
	return []string{trimmed}, nil
}

func countMigrationRows(ctx context.Context, db *sql.DB, schema string, table string) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s",
		qualifyMigrationTable(schema, table),
	)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return count, nil
}

func listMigrationColumns(ctx context.Context, db *sql.DB, schema string, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, schema, table)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns: %w", err)
	}
	return columns, nil
}

func intersectMigrationColumns(targetColumns []string, sourceColumns []string) []string {
	if len(targetColumns) == 0 || len(sourceColumns) == 0 {
		return nil
	}
	sourceSet := make(map[string]struct{}, len(sourceColumns))
	for _, c := range sourceColumns {
		sourceSet[c] = struct{}{}
	}
	out := make([]string, 0, len(targetColumns))
	for _, c := range targetColumns {
		if _, ok := sourceSet[c]; ok {
			out = append(out, c)
		}
	}
	return out
}

func hasMigrationColumn(columns []string, value string) bool {
	for _, c := range columns {
		if c == value {
			return true
		}
	}
	return false
}

func ensureMigrationSerialSequence(
	ctx context.Context,
	db *sql.DB,
	schema string,
	table string,
	column string,
) error {
	if db == nil {
		return nil
	}

	var seq sql.NullString
	if err := db.QueryRowContext(
		ctx,
		"SELECT pg_get_serial_sequence(format('%I.%I', $1::text, $2::text), $3::text)",
		schema,
		table,
		column,
	).Scan(&seq); err != nil {
		return fmt.Errorf("get serial sequence: %w", err)
	}
	if !seq.Valid || strings.TrimSpace(seq.String) == "" {
		return nil
	}

	// Ensure sequence is at least MAX(id) so future inserts won't conflict after we copy explicit IDs.
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"SELECT setval($1::regclass, GREATEST(COALESCE((SELECT MAX(%s) FROM %s), 0), 1), false)",
		quoteMigrationIdentifier(column),
		qualifyMigrationTable(schema, table),
	), seq.String)
	if err != nil {
		return fmt.Errorf("setval: %w", err)
	}
	return nil
}

func joinMigrationColumns(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteMigrationIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func qualifyMigrationTable(schema string, table string) string {
	return fmt.Sprintf("%s.%s", quoteMigrationIdentifier(schema), quoteMigrationIdentifier(table))
}

func quoteMigrationIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
