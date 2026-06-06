package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type announcementReadStatusAnnouncementRepoStub struct {
	AnnouncementRepository
}

func (s *announcementReadStatusAnnouncementRepoStub) GetByID(_ context.Context, id int64) (*Announcement, error) {
	return &Announcement{
		ID:        id,
		Title:     "announcement",
		Content:   "content",
		Status:    AnnouncementStatusActive,
		Targeting: AnnouncementTargeting{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

type announcementReadStatusUserRepoStub struct {
	UserRepository
	users []User
}

func (s *announcementReadStatusUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	start := params.Offset()
	if start > len(s.users) {
		start = len(s.users)
	}
	end := start + params.Limit()
	if end > len(s.users) {
		end = len(s.users)
	}

	total := int64(len(s.users))
	pages := 0
	if params.Limit() > 0 {
		pages = int((total + int64(params.Limit()) - 1) / int64(params.Limit()))
	}
	return s.users[start:end], &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    pages,
	}, nil
}

type announcementReadStatusReadRepoStub struct {
	AnnouncementReadRepository
	readByUserID map[int64]time.Time
}

func (s *announcementReadStatusReadRepoStub) GetReadMapByUsers(_ context.Context, _ int64, userIDs []int64) (map[int64]time.Time, error) {
	out := make(map[int64]time.Time)
	for _, userID := range userIDs {
		if readAt, ok := s.readByUserID[userID]; ok {
			out[userID] = readAt
		}
	}
	return out, nil
}

type announcementReadStatusSubRepoStub struct {
	UserSubscriptionRepository
}

func (s *announcementReadStatusSubRepoStub) ListActiveByUserID(_ context.Context, _ int64) ([]UserSubscription, error) {
	return []UserSubscription{}, nil
}

func TestAnnouncementServiceListUserReadStatusFiltersReadBeforePagination(t *testing.T) {
	readAt := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	svc := NewAnnouncementService(
		&announcementReadStatusAnnouncementRepoStub{},
		&announcementReadStatusReadRepoStub{
			readByUserID: map[int64]time.Time{
				2: readAt,
				3: readAt,
			},
		},
		&announcementReadStatusUserRepoStub{
			users: []User{
				{ID: 1, Email: "unread@example.com", Username: "unread"},
				{ID: 2, Email: "read-one@example.com", Username: "read-one"},
				{ID: 3, Email: "read-two@example.com", Username: "read-two"},
			},
		},
		&announcementReadStatusSubRepoStub{},
	)

	items, page, err := svc.ListUserReadStatus(
		context.Background(),
		10,
		pagination.PaginationParams{Page: 1, PageSize: 1, SortBy: "email", SortOrder: "asc"},
		"",
		"read",
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	require.Equal(t, 2, page.Pages)
	require.Len(t, items, 1)
	require.Equal(t, int64(2), items[0].UserID)
	require.NotNil(t, items[0].ReadAt)
}
