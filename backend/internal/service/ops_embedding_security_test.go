package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsGetErrorLogByID_RedactsEmbeddingContent(t *testing.T) {
	t.Parallel()

	repo := &opsRepoMock{GetErrorLogByIDFn: func(context.Context, int64) (*OpsErrorLogDetail, error) {
		return &OpsErrorLogDetail{
			OpsErrorLog: OpsErrorLog{Platform: PlatformEmbedding, RequestType: ptrInt16(int16(RequestTypeEmbedding))},
			ErrorBody:   "body-canary", RequestBody: "input-canary", RequestHeaders: "header-canary",
			UpstreamErrorMessage: "message-canary", UpstreamErrorDetail: "detail-canary", UpstreamErrors: "vector-canary",
		}, nil
	}}
	svc := &OpsService{opsRepo: repo}

	detail, err := svc.GetErrorLogByID(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, detail.ErrorBody)
	require.Empty(t, detail.RequestBody)
	require.Empty(t, detail.RequestHeaders)
	require.Empty(t, detail.UpstreamErrorMessage)
	require.Empty(t, detail.UpstreamErrorDetail)
	require.Empty(t, detail.UpstreamErrors)

	b, err := json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(b), "canary")
}

func TestOpsGetErrorLogs_RedactsEmbeddingSummary(t *testing.T) {
	t.Parallel()

	repo := &opsRepoMock{ListErrorLogsFn: func(context.Context, *OpsErrorLogFilter) (*OpsErrorLogList, error) {
		return &OpsErrorLogList{Errors: []*OpsErrorLog{{
			Platform: PlatformEmbedding, Type: "upstream_auth", Message: "summary-canary", IsRetryable: true,
		}}}, nil
	}}
	svc := &OpsService{opsRepo: repo}

	result, err := svc.GetErrorLogs(context.Background(), &OpsErrorLogFilter{})
	require.NoError(t, err)
	require.Len(t, result.Errors, 1)
	require.Equal(t, "upstream_auth", result.Errors[0].Type)
	require.Equal(t, "upstream_auth", result.Errors[0].Message)
	require.False(t, result.Errors[0].IsRetryable)
}

func TestIsEmbeddingOpsMetadata_AcceptsTrailingSlash(t *testing.T) {
	t.Parallel()
	require.True(t, isEmbeddingOpsMetadata("", nil, "/v1/embeddings/"))
}

func TestOpsPrepareErrorLogInput_DropsEmbeddingContentBeforePersistence(t *testing.T) {
	t.Parallel()

	message := "upstream-message-canary"
	detail := "upstream-detail-canary"
	requestBody := "stored-input-canary"
	headers := "header-canary"
	entry := &OpsInsertErrorLogInput{
		Platform: PlatformEmbedding, RequestPath: "/v1/embeddings", RequestType: ptrInt16(int16(RequestTypeEmbedding)),
		ErrorType: "invalid_response", ErrorMessage: "message-canary", ErrorBody: "vector-canary", UserAgent: "agent-canary", IsRetryable: true,
		UpstreamErrorMessage: &message, UpstreamErrorDetail: &detail,
		UpstreamErrors:  []*OpsUpstreamErrorEvent{{Message: "event-canary", Detail: "event-detail-canary", UpstreamRequestBody: "event-input-canary"}},
		RequestBodyJSON: &requestBody, RequestHeadersJSON: &headers,
	}
	svc := &OpsService{opsRepo: &opsRepoMock{}}

	prepared, ok, err := svc.prepareErrorLogInput(context.Background(), entry, []byte(`{"input":"raw-input-canary"}`))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "invalid_response", prepared.ErrorType)
	require.Equal(t, "invalid_response", prepared.ErrorMessage)
	require.Empty(t, prepared.ErrorBody)
	require.Empty(t, prepared.UserAgent)
	require.Nil(t, prepared.UpstreamErrorMessage)
	require.Nil(t, prepared.UpstreamErrorDetail)
	require.Nil(t, prepared.UpstreamErrors)
	require.Nil(t, prepared.UpstreamErrorsJSON)
	require.Nil(t, prepared.RequestBodyJSON)
	require.Nil(t, prepared.RequestHeadersJSON)
	require.False(t, prepared.IsRetryable)
}

func TestOpsListRetryAttempts_RedactsEmbeddingPreviews(t *testing.T) {
	t.Parallel()

	preview := "vector-preview-canary"
	truncated := true
	errorMessage := "retry-error-canary"
	repo := &opsRepoMock{
		GetErrorLogByIDFn: func(context.Context, int64) (*OpsErrorLogDetail, error) {
			return &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{Platform: PlatformEmbedding}}, nil
		},
		ListRetryAttemptsByErrorIDFn: func(context.Context, int64, int) ([]*OpsRetryAttempt, error) {
			return []*OpsRetryAttempt{{ResponsePreview: &preview, ResponseTruncated: &truncated, ErrorMessage: &errorMessage}}, nil
		},
	}
	svc := &OpsService{opsRepo: repo}

	items, err := svc.ListRetryAttemptsByErrorID(context.Background(), 1, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].ResponsePreview)
	require.Nil(t, items[0].ResponseTruncated)
	require.Nil(t, items[0].ErrorMessage)
}

func TestOpsEmbeddingRetryIsFailClosed(t *testing.T) {
	t.Parallel()

	retryCalls := 0
	repo := &opsRepoMock{
		GetErrorLogByIDFn: func(context.Context, int64) (*OpsErrorLogDetail, error) {
			return &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{Platform: PlatformEmbedding, RequestPath: "/v1/embeddings"}, RequestBody: `{"model":"embed","input":"secret"}`}, nil
		},
		InsertRetryAttemptFn: func(context.Context, *OpsInsertRetryAttemptInput) (int64, error) {
			retryCalls++
			return 1, nil
		},
	}
	svc := &OpsService{opsRepo: repo}

	_, err := svc.RetryError(context.Background(), 1, 2, OpsRetryModeClient, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "embedding")
	require.Zero(t, retryCalls)

	_, err = svc.RetryUpstreamEvent(context.Background(), 1, 2, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "embedding")
	require.Zero(t, retryCalls)
}

func ptrInt16(v int16) *int16 { return &v }
