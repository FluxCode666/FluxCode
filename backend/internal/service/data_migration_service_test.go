package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeDataMigrationOpener struct {
	openCalls []string
}

func (f *fakeDataMigrationOpener) Open(_ context.Context, dsn string) (*sql.DB, error) {
	f.openCalls = append(f.openCalls, dsn)
	return sql.Open("postgres", "postgres://localhost/test?sslmode=disable")
}

type fakeDataMigrationRepository struct {
	buildCalls      int
	applyCalls      int
	lastPhase       string
	lastResetTarget bool
	report          []DataMigrationPhaseReport
}

func (f *fakeDataMigrationRepository) BuildPlan(
	_ context.Context,
	_ *sql.DB,
	_ *sql.DB,
	_ string,
	_ string,
	phase string,
) ([]DataMigrationPhaseReport, error) {
	f.buildCalls++
	f.lastPhase = phase
	return f.report, nil
}

func (f *fakeDataMigrationRepository) ApplyPlan(
	_ context.Context,
	_ *sql.DB,
	_ *sql.DB,
	_ string,
	_ string,
	phase string,
	resetTarget bool,
) ([]DataMigrationPhaseReport, error) {
	f.applyCalls++
	f.lastPhase = phase
	f.lastResetTarget = resetTarget
	return f.report, nil
}

func TestDataMigrationService_Run_DryRunWritesPlanWithoutApplying(t *testing.T) {
	t.Parallel()

	reportPath := filepath.Join(t.TempDir(), "dry-run-report.json")
	opener := &fakeDataMigrationOpener{}
	repo := &fakeDataMigrationRepository{
		report: []DataMigrationPhaseReport{
			{
				Phase: "subscriptions",
				Tables: []DataMigrationTableReport{
					{Table: "user_subscriptions", SourceRows: 2, TargetRows: 0, Difference: 2},
				},
			},
		},
	}

	svc := NewDataMigrationService(opener, repo)
	report, err := svc.Run(context.Background(), DataMigrationRunRequest{
		SourceDSN:  "postgres://source",
		TargetDSN:  "postgres://target",
		Mode:       DataMigrationModeDryRun,
		Phase:      "subscriptions",
		ReportFile: reportPath,
	})
	require.NoError(t, err)
	require.False(t, report.Applied)
	require.Equal(t, 1, repo.buildCalls)
	require.Zero(t, repo.applyCalls)
	require.Equal(t, []string{"postgres://source", "postgres://target"}, opener.openCalls)

	var saved DataMigrationReport
	raw, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &saved))
	require.Equal(t, DataMigrationModeDryRun, saved.Mode)
	require.Equal(t, "subscriptions", saved.Phase)
	require.False(t, saved.Applied)
}

func TestDataMigrationService_Run_ApplyWritesReportAndPassesResetTarget(t *testing.T) {
	t.Parallel()

	reportPath := filepath.Join(t.TempDir(), "apply-report.json")
	opener := &fakeDataMigrationOpener{}
	repo := &fakeDataMigrationRepository{
		report: []DataMigrationPhaseReport{
			{
				Phase: "pricing",
				Tables: []DataMigrationTableReport{
					{Table: "pricing_plan_groups", SourceRows: 1, TargetRows: 1, CopiedRows: 1, Difference: 0},
				},
			},
		},
	}

	svc := NewDataMigrationService(opener, repo)
	report, err := svc.Run(context.Background(), DataMigrationRunRequest{
		SourceDSN:   "postgres://source",
		TargetDSN:   "postgres://target",
		Mode:        DataMigrationModeApply,
		Phase:       "pricing",
		ResetTarget: true,
		ReportFile:  reportPath,
	})
	require.NoError(t, err)
	require.True(t, report.Applied)
	require.Equal(t, 1, repo.applyCalls)
	require.Zero(t, repo.buildCalls)
	require.True(t, repo.lastResetTarget)
	require.Equal(t, "pricing", repo.lastPhase)

	var saved DataMigrationReport
	raw, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &saved))
	require.Equal(t, DataMigrationModeApply, saved.Mode)
	require.True(t, saved.Applied)
	require.Len(t, saved.Phases, 1)
}
