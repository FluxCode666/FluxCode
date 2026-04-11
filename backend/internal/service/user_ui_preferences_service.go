package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	userPrefKeyDashboardAttractPopupDisabledFmt = "user:%d:dashboard_attract_popup_disabled"
)

type UserUIPreferencesService struct {
	settingRepo SettingRepository
}

func NewUserUIPreferencesService(settingRepo SettingRepository) *UserUIPreferencesService {
	return &UserUIPreferencesService{settingRepo: settingRepo}
}

func (s *UserUIPreferencesService) GetDashboardAttractPopupDisabled(ctx context.Context, userID int64) (bool, error) {
	raw, err := s.settingRepo.GetValue(ctx, fmt.Sprintf(userPrefKeyDashboardAttractPopupDisabledFmt, userID))
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false, nil
		}
		return false, err
	}

	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true, nil
	default:
		return false, nil
	}
}

func (s *UserUIPreferencesService) SetDashboardAttractPopupDisabled(ctx context.Context, userID int64, disabled bool) error {
	return s.settingRepo.Set(ctx, fmt.Sprintf(userPrefKeyDashboardAttractPopupDisabledFmt, userID), strconv.FormatBool(disabled))
}
