package service

import (
	"context"
	"fmt"
	"log/slog"
)

func (s *ChannelMonitorService) BatchMonitorStatusSummary(
	ctx context.Context,
	ids []int64,
	primaryByID map[int64]string,
	extrasByID map[int64][]string,
) map[int64]MonitorStatusSummary {
	out := make(map[int64]MonitorStatusSummary, len(ids))
	if len(ids) == 0 {
		return out
	}
	latestMap, err := s.repo.ListLatestForMonitorIDs(ctx, ids)
	if err != nil {
		slog.Warn("channel_monitor: batch load latest failed", "error", err)
		latestMap = map[int64][]*ChannelMonitorLatest{}
	}
	availMap, err := s.repo.ComputeAvailabilityForMonitors(ctx, ids, monitorAvailability7Days)
	if err != nil {
		slog.Warn("channel_monitor: batch compute availability failed", "error", err)
		availMap = map[int64][]*ChannelMonitorAvailability{}
	}

	for _, id := range ids {
		out[id] = buildStatusSummary(
			indexLatestByModel(latestMap[id]),
			indexAvailabilityByModel(availMap[id]),
			primaryByID[id],
			extrasByID[id],
		)
	}
	return out
}

func (s *ChannelMonitorService) ListUserView(ctx context.Context) ([]*UserMonitorView, error) {
	monitors, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors: %w", err)
	}
	if len(monitors) == 0 {
		return []*UserMonitorView{}, nil
	}

	ids, primaryByID, extrasByID := collectMonitorIndexes(monitors)
	summaries := s.BatchMonitorStatusSummary(ctx, ids, primaryByID, extrasByID)
	latestMap := s.batchLatest(ctx, ids)
	timelineMap := s.batchTimeline(ctx, ids, primaryByID)

	views := make([]*UserMonitorView, 0, len(monitors))
	for _, m := range monitors {
		primaryLatest := pickLatest(latestMap[m.ID], m.PrimaryModel)
		views = append(views, buildUserViewFromSummary(m, summaries[m.ID], primaryLatest, timelineMap[m.ID]))
	}
	return views, nil
}

func collectMonitorIndexes(monitors []*ChannelMonitor) ([]int64, map[int64]string, map[int64][]string) {
	ids := make([]int64, 0, len(monitors))
	primaryByID := make(map[int64]string, len(monitors))
	extrasByID := make(map[int64][]string, len(monitors))
	for _, m := range monitors {
		ids = append(ids, m.ID)
		primaryByID[m.ID] = m.PrimaryModel
		extrasByID[m.ID] = m.ExtraModels
	}
	return ids, primaryByID, extrasByID
}

func (s *ChannelMonitorService) batchLatest(ctx context.Context, ids []int64) map[int64][]*ChannelMonitorLatest {
	latestMap, err := s.repo.ListLatestForMonitorIDs(ctx, ids)
	if err != nil {
		slog.Warn("channel_monitor: user view batch latest failed", "error", err)
		return map[int64][]*ChannelMonitorLatest{}
	}
	return latestMap
}

func (s *ChannelMonitorService) batchTimeline(
	ctx context.Context,
	ids []int64,
	primaryByID map[int64]string,
) map[int64][]*ChannelMonitorHistoryEntry {
	timelineMap, err := s.repo.ListRecentHistoryForMonitors(ctx, ids, primaryByID, monitorTimelineMaxPoints)
	if err != nil {
		slog.Warn("channel_monitor: user view batch timeline failed", "error", err)
		return map[int64][]*ChannelMonitorHistoryEntry{}
	}
	return timelineMap
}

func pickLatest(rows []*ChannelMonitorLatest, model string) *ChannelMonitorLatest {
	if model == "" {
		return nil
	}
	for _, row := range rows {
		if row.Model == model {
			return row
		}
	}
	return nil
}

func (s *ChannelMonitorService) GetUserDetail(ctx context.Context, id int64) (*UserMonitorDetail, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !m.Enabled {
		return nil, ErrChannelMonitorNotFound
	}
	latest, err := s.repo.ListLatestPerModel(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list latest per model: %w", err)
	}
	availMap, err := s.collectAvailabilityWindows(ctx, id)
	if err != nil {
		return nil, err
	}

	return &UserMonitorDetail{
		ID:        m.ID,
		Name:      m.Name,
		Provider:  m.Provider,
		GroupName: m.GroupName,
		Models:    mergeModelDetails(m, latest, availMap),
	}, nil
}

func (s *ChannelMonitorService) collectAvailabilityWindows(ctx context.Context, monitorID int64) (map[int]map[string]*ChannelMonitorAvailability, error) {
	out := make(map[int]map[string]*ChannelMonitorAvailability, 3)
	windows := []int{monitorAvailability7Days, monitorAvailability15Days, monitorAvailability30Days}
	for _, window := range windows {
		rows, err := s.repo.ComputeAvailability(ctx, monitorID, window)
		if err != nil {
			return nil, fmt.Errorf("compute availability %dd: %w", window, err)
		}
		out[window] = indexAvailabilityByModel(rows)
	}
	return out, nil
}

func indexLatestByModel(rows []*ChannelMonitorLatest) map[string]*ChannelMonitorLatest {
	out := make(map[string]*ChannelMonitorLatest, len(rows))
	for _, row := range rows {
		out[row.Model] = row
	}
	return out
}

func indexAvailabilityByModel(rows []*ChannelMonitorAvailability) map[string]*ChannelMonitorAvailability {
	out := make(map[string]*ChannelMonitorAvailability, len(rows))
	for _, row := range rows {
		out[row.Model] = row
	}
	return out
}

func buildStatusSummary(
	latestByModel map[string]*ChannelMonitorLatest,
	availByModel map[string]*ChannelMonitorAvailability,
	primary string,
	extras []string,
) MonitorStatusSummary {
	summary := MonitorStatusSummary{ExtraModels: make([]ExtraModelStatus, 0, len(extras))}
	if primary != "" {
		if latest, ok := latestByModel[primary]; ok {
			summary.PrimaryStatus = latest.Status
			summary.PrimaryLatencyMs = latest.LatencyMs
		}
		if availability, ok := availByModel[primary]; ok {
			summary.Availability7d = availability.AvailabilityPct
		}
	}
	for _, model := range extras {
		entry := ExtraModelStatus{Model: model}
		if latest, ok := latestByModel[model]; ok {
			entry.Status = latest.Status
			entry.LatencyMs = latest.LatencyMs
		}
		summary.ExtraModels = append(summary.ExtraModels, entry)
	}
	return summary
}

func buildUserViewFromSummary(
	m *ChannelMonitor,
	summary MonitorStatusSummary,
	primaryLatest *ChannelMonitorLatest,
	timelineEntries []*ChannelMonitorHistoryEntry,
) *UserMonitorView {
	view := &UserMonitorView{
		ID:               m.ID,
		Name:             m.Name,
		Provider:         m.Provider,
		GroupName:        m.GroupName,
		PrimaryModel:     m.PrimaryModel,
		PrimaryStatus:    summary.PrimaryStatus,
		PrimaryLatencyMs: summary.PrimaryLatencyMs,
		Availability7d:   summary.Availability7d,
		ExtraModels:      summary.ExtraModels,
		Timeline:         buildTimelinePoints(timelineEntries),
	}
	if primaryLatest != nil {
		view.PrimaryPingLatencyMs = primaryLatest.PingLatencyMs
	}
	return view
}

func buildTimelinePoints(entries []*ChannelMonitorHistoryEntry) []UserMonitorTimelinePoint {
	out := make([]UserMonitorTimelinePoint, 0, len(entries))
	for _, entry := range entries {
		out = append(out, UserMonitorTimelinePoint{
			Status:        entry.Status,
			LatencyMs:     entry.LatencyMs,
			PingLatencyMs: entry.PingLatencyMs,
			CheckedAt:     entry.CheckedAt,
		})
	}
	return out
}

func mergeModelDetails(
	m *ChannelMonitor,
	latest []*ChannelMonitorLatest,
	availMap map[int]map[string]*ChannelMonitorAvailability,
) []ModelDetail {
	latestByModel := indexLatestByModel(latest)
	models := append([]string{m.PrimaryModel}, m.ExtraModels...)
	out := make([]ModelDetail, 0, len(models))
	for _, model := range models {
		detail := ModelDetail{Model: model}
		if latest, ok := latestByModel[model]; ok {
			detail.LatestStatus = latest.Status
			detail.LatestLatencyMs = latest.LatencyMs
		}
		if availability, ok := availMap[monitorAvailability7Days][model]; ok {
			detail.Availability7d = availability.AvailabilityPct
			detail.AvgLatency7dMs = availability.AvgLatencyMs
		}
		if availability, ok := availMap[monitorAvailability15Days][model]; ok {
			detail.Availability15d = availability.AvailabilityPct
		}
		if availability, ok := availMap[monitorAvailability30Days][model]; ok {
			detail.Availability30d = availability.AvailabilityPct
		}
		out = append(out, detail)
	}
	return out
}
