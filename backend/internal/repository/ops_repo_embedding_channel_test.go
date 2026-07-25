package repository

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsErrorLogSQLPersistsAndLoadsChannelMetadata(t *testing.T) {
	t.Parallel()

	channelID := int64(42)
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{
		ChannelID: channelIDPtr(channelID),
		CreatedAt: time.Unix(1, 0),
	})
	require.Len(t, args, 44)
	require.Equal(t, sql.NullInt64{Int64: channelID, Valid: true}, args[6])
	require.Contains(t, insertOpsErrorLogSQL, "channel_id")

	sourceBytes, err := os.ReadFile("ops_repo.go")
	require.NoError(t, err)
	source := string(sourceBytes)
	require.GreaterOrEqual(t, strings.Count(source, "LEFT JOIN channels ch ON e.channel_id = ch.id"), 2)
}

func channelIDPtr(value int64) *int64 { return &value }
