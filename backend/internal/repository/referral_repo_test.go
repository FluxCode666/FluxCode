package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestReferralRepositoryListAllIncludesInviteeFirstChargeRewardAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewReferralRepository(db)
	createdAt := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM referrals r`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id",
		"referrer_id",
		"referee_id",
		"referral_code",
		"status",
		"invitee_reward_amount",
		"inviter_reward_amount",
		"invitee_rewarded_at",
		"inviter_rewarded_at",
		"ongoing_reward_count",
		"ongoing_reward_total",
		"invitee_ongoing_reward_count",
		"invitee_ongoing_reward_total",
		"invitee_first_charge_reward_amount",
		"created_at",
		"updated_at",
		"referrer_email",
		"referrer_is_sales",
		"referee_email",
		"referee_username",
	}).AddRow(
		int64(1),
		int64(12),
		int64(34),
		"ABC123",
		"completed",
		10.0,
		20.0,
		nil,
		nil,
		1,
		5.0,
		2,
		7.0,
		3.5,
		createdAt,
		updatedAt,
		"referrer@example.com",
		false,
		"buyer@example.com",
		"buyer",
	)
	mock.ExpectQuery(`SELECT r\.id, r\.referrer_id, r\.referee_id`).
		WithArgs(50, 0).
		WillReturnRows(rows)

	referrals, total, err := repo.ListAll(context.Background(), "", 0, 0, 0, 50)

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, referrals, 1)
	require.Equal(t, 3.5, referrals[0].InviteeFirstChargeRewardAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReferralInviteeFirstChargeRewardSelectDoesNotRequireSourceRefID(t *testing.T) {
	require.NotContains(t, referralInviteeFirstChargeRewardSelect, "source_ref_id")
	require.NotContains(t, referralInviteeFirstChargeRewardSelect, "payment_orders")
	require.Contains(t, referralInviteeFirstChargeRewardSelect, "gbr.user_id = r.referee_id")
}
