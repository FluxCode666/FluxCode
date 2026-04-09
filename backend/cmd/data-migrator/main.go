package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
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

	// Build a DSN string that works with lib/pq. We intentionally keep it readable
	// (postgres://...) so it's easy to copy/paste into migration commands.
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
	var req service.DataMigrationRunRequest
	var sourceConfigPath string
	var targetConfigPath string

	flag.StringVar(&req.SourceDSN, "source_dsn", "", "source postgres dsn")
	flag.StringVar(&req.TargetDSN, "target_dsn", "", "target postgres dsn")
	flag.StringVar(&sourceConfigPath, "source_config", "", "path to source backend config.yaml (optional; used when source_dsn is empty)")
	flag.StringVar(&targetConfigPath, "target_config", "", "path to target backend config.yaml (optional; used when target_dsn is empty)")
	flag.StringVar((*string)(&req.Mode), "mode", string(service.DataMigrationModeDryRun), "migration mode: dry-run or apply")
	flag.StringVar(&req.Phase, "phase", "all", "migration phase: all|core|access_control|infrastructure|api_keys|subscriptions|pricing|pool_monitor|usage|ops_metrics|billing")
	flag.BoolVar(&req.ResetTarget, "reset_target", false, "reset target tables before apply")
	flag.StringVar(&req.ReportFile, "report_file", "", "path to write json report")
	flag.Parse()

	if strings.TrimSpace(req.SourceDSN) == "" && strings.TrimSpace(sourceConfigPath) != "" {
		dsn, err := loadDSNFromConfig(sourceConfigPath)
		if err != nil {
			log.Fatalf("load source_config: %v", err)
		}
		req.SourceDSN = dsn
	}
	if strings.TrimSpace(req.TargetDSN) == "" && strings.TrimSpace(targetConfigPath) != "" {
		dsn, err := loadDSNFromConfig(targetConfigPath)
		if err != nil {
			log.Fatalf("load target_config: %v", err)
		}
		req.TargetDSN = dsn
	}

	svc := service.NewDataMigrationService(
		service.NewDataMigrationSQLOpener(),
		repository.NewDataMigrationRepository(),
	)

	report, err := svc.Run(context.Background(), req)
	if err != nil {
		log.Fatalf("run data migration: %v", err)
	}

	fmt.Printf("mode=%s phase=%s applied=%t phases=%d\n", report.Mode, report.Phase, report.Applied, len(report.Phases))
	for _, phase := range report.Phases {
		fmt.Printf("[%s]\n", phase.Phase)
		for _, table := range phase.Tables {
			fmt.Printf("  %s source=%d target=%d copied=%d diff=%d\n", table.Table, table.SourceRows, table.TargetRows, table.CopiedRows, table.Difference)
		}
	}

	if req.ReportFile != "" {
		fmt.Fprintf(os.Stdout, "report_file=%s\n", req.ReportFile)
	}
}
