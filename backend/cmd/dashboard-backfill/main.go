package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

type databaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type appConfig struct {
	Database databaseConfig `yaml:"database"`
}

func loadDSNFromConfig(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	var cfg appConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}
	db := cfg.Database
	if strings.TrimSpace(db.Host) == "" {
		return "", fmt.Errorf("database.host is required")
	}
	if db.Port == 0 {
		db.Port = 5432
	}
	if strings.TrimSpace(db.User) == "" {
		return "", fmt.Errorf("database.user is required")
	}
	if strings.TrimSpace(db.DBName) == "" {
		return "", fmt.Errorf("database.dbname is required")
	}
	sslmode := strings.TrimSpace(db.SSLMode)
	if sslmode == "" {
		sslmode = "disable"
	}
	host := strings.TrimSpace(db.Host)
	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", host, db.Port),
		Path:   "/" + db.DBName,
	}
	if strings.TrimSpace(db.Password) == "" {
		u.User = url.User(db.User)
	} else {
		u.User = url.UserPassword(db.User, db.Password)
	}
	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func main() {
	var dsn string
	var configPath string
	var startStr string
	var endStr string
	var days int

	var tz string
	flag.StringVar(&tz, "tz", "Asia/Shanghai", "timezone for aggregation (e.g. Asia/Shanghai, UTC)")
	flag.StringVar(&dsn, "dsn", "", "postgres DSN (e.g. postgres://user:pass@host:5432/db?sslmode=disable)")
	flag.StringVar(&configPath, "config", "", "path to backend config.yaml (used when dsn is empty)")
	flag.StringVar(&startStr, "start", "", "backfill start time (RFC3339, e.g. 2026-01-01T00:00:00Z)")
	flag.StringVar(&endStr, "end", "", "backfill end time (RFC3339, default: now)")
	flag.IntVar(&days, "days", 0, "backfill last N days (shorthand, overrides --start)")
	flag.Parse()

	// Initialize timezone (required by aggregation repo)
	if err := timezone.Init(tz); err != nil {
		log.Fatalf("init timezone: %v", err)
	}

	// Resolve DSN
	if strings.TrimSpace(dsn) == "" && strings.TrimSpace(configPath) != "" {
		resolved, err := loadDSNFromConfig(configPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		dsn = resolved
	}
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("must provide --dsn or --config")
	}

	log.Printf("connecting to database...")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	log.Printf("database connected")

	// Resolve time range
	now := time.Now().UTC()
	var start, end time.Time

	if days > 0 {
		start = now.AddDate(0, 0, -days)
		end = now
	} else if strings.TrimSpace(startStr) != "" {
		var err error
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			log.Fatalf("invalid --start: %v", err)
		}
		if strings.TrimSpace(endStr) != "" {
			end, err = time.Parse(time.RFC3339, endStr)
			if err != nil {
				log.Fatalf("invalid --end: %v", err)
			}
		} else {
			end = now
		}
	} else {
		// 默认全量：从 usage_logs 最早记录开始，到当前时间
		var earliest sql.NullTime
		if err := db.QueryRowContext(context.Background(), "SELECT MIN(created_at) FROM usage_logs").Scan(&earliest); err != nil || !earliest.Valid {
			log.Fatal("no usage_logs data found and no --start/--days specified; nothing to backfill")
		}
		start = earliest.Time.UTC()
		end = now
		log.Printf("auto-detected usage_logs range: %s -> %s", start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	if !end.After(start) {
		log.Fatal("end must be after start")
	}

	repo := repository.NewDashboardAggregationRepository(db)
	if repo == nil {
		log.Fatal("failed to create aggregation repository (non-PostgreSQL driver?)")
	}

	log.Printf("backfill range: %s -> %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	ctx := context.Background()
	jobStart := time.Now()

	// Day-by-day backfill to avoid single huge transaction and show progress
	cursor := truncateToDayUTC(start)
	dayCount := 0
	for cursor.Before(end) {
		windowEnd := cursor.Add(24 * time.Hour)
		if windowEnd.After(end) {
			windowEnd = end
		}
		if err := repo.AggregateRange(ctx, cursor, windowEnd); err != nil {
			log.Fatalf("aggregate range [%s, %s): %v",
				cursor.Format(time.RFC3339), windowEnd.Format(time.RFC3339), err)
		}
		dayCount++
		log.Printf("  [%d] aggregated %s -> %s", dayCount,
			cursor.Format("2006-01-02"), windowEnd.Format("2006-01-02T15:04:05Z"))
		cursor = windowEnd
	}

	// Update watermark
	if err := repo.UpdateAggregationWatermark(ctx, end); err != nil {
		log.Printf("WARNING: failed to update watermark: %v", err)
	} else {
		log.Printf("watermark updated to %s", end.Format(time.RFC3339))
	}

	log.Printf("backfill completed: %d day(s), duration=%s", dayCount, time.Since(jobStart).String())
}

func truncateToDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
