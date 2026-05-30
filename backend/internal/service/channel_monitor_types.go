package service

import (
	"context"
	"time"
)

type ChannelMonitorRuntimeSettings struct {
	Enabled                bool
	DefaultIntervalSeconds int
}

type ChannelMonitorRuntimeProvider interface {
	GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntimeSettings
}

type ChannelMonitor struct {
	ID                  int64
	Name                string
	Provider            string
	APIMode             string
	Endpoint            string
	APIKey              string
	PrimaryModel        string
	ExtraModels         []string
	GroupName           string
	Enabled             bool
	IntervalSeconds     int
	LastCheckedAt       *time.Time
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	TemplateID          *int64
	ExtraHeaders        map[string]string
	BodyOverrideMode    string
	BodyOverride        map[string]any
	APIKeyDecryptFailed bool
}

type ChannelMonitorListParams struct {
	Page     int
	PageSize int
	Provider string
	Enabled  *bool
	Search   string
}

type ChannelMonitorCreateParams struct {
	Name             string
	Provider         string
	APIMode          string
	Endpoint         string
	APIKey           string
	PrimaryModel     string
	ExtraModels      []string
	GroupName        string
	Enabled          bool
	IntervalSeconds  int
	CreatedBy        int64
	TemplateID       *int64
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}

type ChannelMonitorUpdateParams struct {
	Name             *string
	Provider         *string
	APIMode          *string
	Endpoint         *string
	APIKey           *string
	PrimaryModel     *string
	ExtraModels      *[]string
	GroupName        *string
	Enabled          *bool
	IntervalSeconds  *int
	TemplateID       *int64
	ClearTemplate    bool
	ExtraHeaders     *map[string]string
	BodyOverrideMode *string
	BodyOverride     *map[string]any
}

type CheckResult struct {
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

type UserMonitorView struct {
	ID                   int64
	Name                 string
	Provider             string
	GroupName            string
	PrimaryModel         string
	PrimaryStatus        string
	PrimaryLatencyMs     *int
	PrimaryPingLatencyMs *int
	Availability7d       float64
	ExtraModels          []ExtraModelStatus
	Timeline             []UserMonitorTimelinePoint
}

type UserMonitorTimelinePoint struct {
	Status        string    `json:"status"`
	LatencyMs     *int      `json:"latency_ms"`
	PingLatencyMs *int      `json:"ping_latency_ms"`
	CheckedAt     time.Time `json:"checked_at"`
}

type ExtraModelStatus struct {
	Model     string
	Status    string
	LatencyMs *int
}

type UserMonitorDetail struct {
	ID        int64
	Name      string
	Provider  string
	GroupName string
	Models    []ModelDetail
}

type ModelDetail struct {
	Model           string
	LatestStatus    string
	LatestLatencyMs *int
	Availability7d  float64
	Availability15d float64
	Availability30d float64
	AvgLatency7dMs  *int
}

type ChannelMonitorHistoryRow struct {
	MonitorID     int64
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

type ChannelMonitorHistoryEntry struct {
	ID            int64
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

type ChannelMonitorLatest struct {
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	CheckedAt     time.Time
}

type ChannelMonitorAvailability struct {
	Model             string
	WindowDays        int
	TotalChecks       int
	OperationalChecks int
	AvailabilityPct   float64
	AvgLatencyMs      *int
}

type MonitorStatusSummary struct {
	PrimaryStatus    string
	PrimaryLatencyMs *int
	Availability7d   float64
	ExtraModels      []ExtraModelStatus
}

type ChannelMonitorRepository interface {
	Create(ctx context.Context, m *ChannelMonitor) error
	GetByID(ctx context.Context, id int64) (*ChannelMonitor, error)
	Update(ctx context.Context, m *ChannelMonitor) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error)
	ListEnabled(ctx context.Context) ([]*ChannelMonitor, error)
	MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error
	InsertHistoryBatch(ctx context.Context, rows []*ChannelMonitorHistoryRow) error
	DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error)
	ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*ChannelMonitorHistoryEntry, error)
	ListLatestPerModel(ctx context.Context, monitorID int64) ([]*ChannelMonitorLatest, error)
	ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*ChannelMonitorAvailability, error)
	ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*ChannelMonitorLatest, error)
	ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*ChannelMonitorAvailability, error)
	ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primaryModels map[int64]string, perMonitorLimit int) (map[int64][]*ChannelMonitorHistoryEntry, error)
	UpsertDailyRollupsFor(ctx context.Context, targetDate time.Time) (int64, error)
	DeleteRollupsBefore(ctx context.Context, beforeDate time.Time) (int64, error)
	LoadAggregationWatermark(ctx context.Context) (*time.Time, error)
	UpdateAggregationWatermark(ctx context.Context, date time.Time) error
}
