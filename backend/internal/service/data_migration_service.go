package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type DataMigrationMode string

const (
	DataMigrationModeDryRun DataMigrationMode = "dry-run"
	DataMigrationModeApply  DataMigrationMode = "apply"
)

type DataMigrationRunRequest struct {
	SourceDSN   string
	TargetDSN   string
	Mode        DataMigrationMode
	Phase       string
	ResetTarget bool
	ReportFile  string
}

type DataMigrationTableReport struct {
	Table      string `json:"table"`
	SourceRows int64  `json:"source_rows"`
	TargetRows int64  `json:"target_rows"`
	CopiedRows int64  `json:"copied_rows,omitempty"`
	Difference int64  `json:"difference"`
}

type DataMigrationPhaseReport struct {
	Phase  string                     `json:"phase"`
	Tables []DataMigrationTableReport `json:"tables"`
}

type DataMigrationReport struct {
	Mode        DataMigrationMode          `json:"mode"`
	Phase       string                     `json:"phase"`
	ResetTarget bool                       `json:"reset_target"`
	Applied     bool                       `json:"applied"`
	StartedAt   time.Time                  `json:"started_at"`
	FinishedAt  time.Time                  `json:"finished_at"`
	Phases      []DataMigrationPhaseReport `json:"phases"`
}

type DataMigrationDBOpener interface {
	Open(ctx context.Context, dsn string) (*sql.DB, error)
}

type DataMigrationRepository interface {
	BuildPlan(ctx context.Context, sourceDB *sql.DB, targetDB *sql.DB, sourceSchema string, targetSchema string, phase string) ([]DataMigrationPhaseReport, error)
	ApplyPlan(ctx context.Context, sourceDB *sql.DB, targetDB *sql.DB, sourceSchema string, targetSchema string, phase string, resetTarget bool) ([]DataMigrationPhaseReport, error)
}

type dataMigrationSQLOpener struct{}

func NewDataMigrationSQLOpener() DataMigrationDBOpener {
	return dataMigrationSQLOpener{}
}

func (dataMigrationSQLOpener) Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

type DataMigrationService struct {
	opener DataMigrationDBOpener
	repo   DataMigrationRepository
}

func NewDataMigrationService(opener DataMigrationDBOpener, repo DataMigrationRepository) *DataMigrationService {
	return &DataMigrationService{
		opener: opener,
		repo:   repo,
	}
}

func (s *DataMigrationService) Run(ctx context.Context, req DataMigrationRunRequest) (DataMigrationReport, error) {
	if s == nil || s.opener == nil || s.repo == nil {
		return DataMigrationReport{}, fmt.Errorf("data migration service dependencies are not configured")
	}
	if strings.TrimSpace(req.SourceDSN) == "" {
		return DataMigrationReport{}, fmt.Errorf("source_dsn is required")
	}
	if strings.TrimSpace(req.TargetDSN) == "" {
		return DataMigrationReport{}, fmt.Errorf("target_dsn is required")
	}

	mode := req.Mode
	if mode == "" {
		mode = DataMigrationModeDryRun
	}
	if mode != DataMigrationModeDryRun && mode != DataMigrationModeApply {
		return DataMigrationReport{}, fmt.Errorf("unsupported migration mode: %s", mode)
	}

	sourceDB, err := s.opener.Open(ctx, req.SourceDSN)
	if err != nil {
		return DataMigrationReport{}, fmt.Errorf("open source db: %w", err)
	}
	defer func() {
		if sourceDB != nil {
			_ = sourceDB.Close()
		}
	}()

	targetDB, err := s.opener.Open(ctx, req.TargetDSN)
	if err != nil {
		return DataMigrationReport{}, fmt.Errorf("open target db: %w", err)
	}
	defer func() {
		if targetDB != nil {
			_ = targetDB.Close()
		}
	}()

	report := DataMigrationReport{
		Mode:        mode,
		Phase:       normalizeDataMigrationPhase(req.Phase),
		ResetTarget: req.ResetTarget,
		Applied:     mode == DataMigrationModeApply,
		StartedAt:   time.Now().UTC(),
	}

	if mode == DataMigrationModeDryRun {
		report.Phases, err = s.repo.BuildPlan(ctx, sourceDB, targetDB, "public", "public", report.Phase)
	} else {
		report.Phases, err = s.repo.ApplyPlan(ctx, sourceDB, targetDB, "public", "public", report.Phase, req.ResetTarget)
	}
	if err != nil {
		return DataMigrationReport{}, err
	}

	report.FinishedAt = time.Now().UTC()
	if strings.TrimSpace(req.ReportFile) != "" {
		if err := writeDataMigrationReport(req.ReportFile, report); err != nil {
			return DataMigrationReport{}, err
		}
	}

	return report, nil
}

func normalizeDataMigrationPhase(phase string) string {
	trimmed := strings.TrimSpace(phase)
	if trimmed == "" {
		return "all"
	}
	return trimmed
}

func writeDataMigrationReport(path string, report DataMigrationReport) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal migration report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write migration report: %w", err)
	}
	return nil
}
