package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandlerEmbeddingsRejectsSimpleModeBeforeDependencies(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed","input":"secret"}`))
	h := &OpenAIGatewayHandler{cfg: &config.Config{RunMode: config.RunModeSimple}}

	require.NotPanics(t, func() { h.Embeddings(c) })
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestOpenAIGatewayHandlerEmbeddingModelsRejectsSimpleModeBeforeDependencies(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h := &OpenAIGatewayHandler{cfg: &config.Config{RunMode: config.RunModeSimple}}

	require.NotPanics(t, func() { h.EmbeddingModels(c) })
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

type unreadEmbeddingBody struct {
	readCalls int
}

func (b *unreadEmbeddingBody) Read([]byte) (int, error) {
	b.readCalls++
	return 0, errors.New("request body must not be read")
}

func (*unreadEmbeddingBody) Close() error { return nil }

func TestOpenAIGatewayHandlerEmbeddingsAcquiresUserSlotBeforeReadingBody(t *testing.T) {
	groupID := int64(9)
	user := &service.User{ID: 11}
	group := &service.Group{ID: groupID, Platform: service.PlatformEmbedding}
	apiKey := &service.APIKey{ID: 12, UserID: user.ID, User: user, GroupID: &groupID, Group: group}
	body := &unreadEmbeddingBody{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	c.Request.Body = body
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: user.ID, Concurrency: 1})

	cache := &helperConcurrencyCacheStub{userSeq: []bool{false}}
	h := NewOpenAIGatewayHandler(
		&service.OpenAIGatewayService{},
		service.NewConcurrencyService(cache),
		&service.BillingCacheService{},
		nil, nil, nil,
		&config.Config{},
	)
	h.Embeddings(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Zero(t, body.readCalls)
	require.Equal(t, 1, cache.userAcquireCalls)
}

func TestOpenAIGatewayHandlerEmbeddingFailureSetsSafeOpsMetadata(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	h := &OpenAIGatewayHandler{}

	h.handleEmbeddingError(c, &service.EmbeddingForwardError{
		Category: "invalid_response", AccountID: 41, ChannelID: 42, UpstreamModel: "upstream-embed",
	})

	category, ok := c.Get(service.OpsEmbeddingErrorCategoryKey)
	require.True(t, ok)
	require.Equal(t, "invalid_response", category)
	accountID, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.Equal(t, int64(41), accountID)
	channelID, ok := c.Get(opsChannelIDKey)
	require.True(t, ok)
	require.Equal(t, int64(42), channelID)
	upstreamModel, ok := c.Get(opsUpstreamModelKey)
	require.True(t, ok)
	require.Equal(t, "upstream-embed", upstreamModel)
	require.NotContains(t, recorder.Body.String(), "upstream-embed")
}

type handlerEmbeddingBillingRepo struct {
	command *service.UsageBillingCommand
	err     error
	events  *[]string
}

func (r *handlerEmbeddingBillingRepo) Apply(_ context.Context, command *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	*r.events = append(*r.events, "transaction")
	r.command = command
	if r.err != nil {
		return nil, r.err
	}
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

type handlerEmbeddingGateway struct {
	billing      *service.OpenAIGatewayService
	result       *service.EmbeddingForwardResult
	events       *[]string
	forwardCalls int
	billCalls    int
}

func (g *handlerEmbeddingGateway) ListAvailableEmbeddingModels(context.Context, *int64) ([]string, error) {
	return []string{"embed-public"}, nil
}

func (g *handlerEmbeddingGateway) ForwardEmbeddings(context.Context, service.EmbeddingForwardInput) (*service.EmbeddingForwardResult, error) {
	g.forwardCalls++
	*g.events = append(*g.events, "forward")
	return g.result, nil
}

func (g *handlerEmbeddingGateway) BillEmbedding(ctx context.Context, input *service.EmbeddingBillingInput) error {
	g.billCalls++
	*g.events = append(*g.events, "bill")
	return g.billing.BillEmbedding(ctx, input)
}

type handlerEmbeddingBillingChecker struct {
	events *[]string
}

func (c *handlerEmbeddingBillingChecker) CheckBillingEligibility(context.Context, *service.User, *service.APIKey, *service.Group, *service.UserSubscription) error {
	*c.events = append(*c.events, "eligibility")
	return nil
}

func newEmbeddingHTTPBillingHarness(t *testing.T, billingErr error) (*gin.Engine, *handlerEmbeddingGateway, *handlerEmbeddingBillingRepo) {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	groupID := int64(51)
	user := &service.User{ID: 52, Balance: 10}
	group := &service.Group{ID: groupID, Platform: service.PlatformEmbedding, RateMultiplier: 1}
	apiKey := &service.APIKey{ID: 53, UserID: user.ID, User: user, GroupID: &groupID, Group: group}
	events := make([]string, 0, 4)
	repo := &handlerEmbeddingBillingRepo{err: billingErr, events: &events}
	billingGateway := service.NewOpenAIGatewayService(
		nil, nil, nil, repo, nil, nil, nil, nil, cfg, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	price := 2e-6
	forward := &handlerEmbeddingGateway{
		billing: billingGateway,
		events:  &events,
		result: &service.EmbeddingForwardResult{
			Body:         []byte(`{"object":"list","data":[{"embedding":[0.123456789],"index":0}],"model":"embed-public","usage":{"prompt_tokens":10}}`),
			PromptTokens: 10,
			Eligibility: service.EmbeddingModelEligibility{
				Account:     service.Account{ID: 54, Platform: service.PlatformEmbedding, Type: service.AccountTypeAPIKey},
				PublicModel: "embed-public", UpstreamModel: "embed-upstream", BillingModel: "embed-public",
				Pricing: &service.ResolvedPricing{Mode: service.BillingModeToken, BasePricing: &service.ModelPricing{InputPricePerToken: price}},
			},
		},
	}
	h := NewOpenAIGatewayHandler(nil, service.NewConcurrencyService(nil), nil, nil, nil, nil, cfg)
	h.embeddingGateway = forward
	h.embeddingBillingChecker = &handlerEmbeddingBillingChecker{events: &events}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: user.ID, Concurrency: 0})
		c.Next()
	})
	router.POST("/v1/embeddings", h.Embeddings)
	return router, forward, repo
}

func TestOpenAIGatewayHandlerEmbeddingHTTPCommitsType4BillingBeforeVector(t *testing.T) {
	router, gateway, repo := newEmbeddingHTTPBillingHarness(t, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed-public","input":"safe"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"eligibility", "forward", "bill", "transaction"}, *gateway.events)
	require.Equal(t, 1, gateway.forwardCalls)
	require.Equal(t, 1, gateway.billCalls)
	require.Contains(t, recorder.Body.String(), "0.123456789")
	require.NotNil(t, repo.command)
	require.NotNil(t, repo.command.UsageLog)
	require.Equal(t, service.RequestTypeEmbedding, repo.command.UsageLog.RequestType)
	require.InDelta(t, 0.00002, repo.command.BalanceCost, 1e-12)
}

func TestOpenAIGatewayHandlerEmbeddingHTTPNeverWritesVectorOrRefowardsAfterBillingFailure(t *testing.T) {
	router, gateway, repo := newEmbeddingHTTPBillingHarness(t, errors.New("usage insert failed"))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed-public","input":"safe"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, 1, gateway.forwardCalls)
	require.Equal(t, 1, gateway.billCalls)
	require.NotNil(t, repo.command)
	require.NotContains(t, recorder.Body.String(), "0.123456789")
	require.Contains(t, recorder.Body.String(), "Embedding billing failed")
}
