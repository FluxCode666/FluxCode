package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

func newAgentIdentityTestKey(t *testing.T) (agentIdentityKey, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return agentIdentityKey{runtimeID: "runtime-test", privateKey: privateKey, taskID: "task-test"}, base64.StdEncoding.EncodeToString(der)
}

func TestBuildAgentAssertionMatchesCodexEnvelopeAndSignature(t *testing.T) {
	key, _ := newAgentIdentityTestKey(t)
	now := time.Date(2026, 7, 14, 8, 9, 10, 0, time.FixedZone("UTC+8", 8*60*60))
	assertion, err := buildAgentAssertion(key, now)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(assertion, "AgentAssertion "))

	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(assertion, "AgentAssertion "))
	require.NoError(t, err)
	var envelope struct {
		AgentRuntimeID string `json:"agent_runtime_id"`
		TaskID         string `json:"task_id"`
		Timestamp      string `json:"timestamp"`
		Signature      string `json:"signature"`
	}
	require.NoError(t, json.Unmarshal(decoded, &envelope))
	require.Equal(t, "runtime-test", envelope.AgentRuntimeID)
	require.Equal(t, "task-test", envelope.TaskID)
	require.Equal(t, "2026-07-14T00:09:10Z", envelope.Timestamp)
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(key.privateKey.Public().(ed25519.PublicKey), []byte("runtime-test:task-test:2026-07-14T00:09:10Z"), signature))
}

func TestDecryptAgentTaskIDSupportsSealedBoxResponse(t *testing.T) {
	key, _ := newAgentIdentityTestKey(t)
	digest := sha512.Sum512(key.privateKey.Seed())
	var curvePrivate [32]byte
	copy(curvePrivate[:], digest[:32])
	curvePrivate[0] &= 248
	curvePrivate[31] &= 127
	curvePrivate[31] |= 64
	curvePublicBytes, err := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	require.NoError(t, err)
	var curvePublic [32]byte
	copy(curvePublic[:], curvePublicBytes)
	ciphertext, err := box.SealAnonymous(nil, []byte("task-sealed"), &curvePublic, rand.Reader)
	require.NoError(t, err)

	taskID, err := decryptAgentTaskID(key, base64.StdEncoding.EncodeToString(ciphertext))
	require.NoError(t, err)
	require.Equal(t, "task-sealed", taskID)
}

func TestEnsureAgentIdentityTaskSharesAccountLock(t *testing.T) {
	key, privateKey := newAgentIdentityTestKey(t)
	account := &Account{ID: 91001, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity, "agent_runtime_id": key.runtimeID, "agent_private_key": privateKey,
	}}
	repo := &agentIdentityTestCredentialsRepo{account: account}
	registerCalls := 0
	var registerMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		registerMu.Lock()
		registerCalls++
		registerMu.Unlock()
		_, _ = w.Write([]byte(`{"task_id":"task-shared"}`))
	}))
	defer server.Close()
	oldBaseURL := openAIAgentIdentityAuthAPIBaseURL
	openAIAgentIdentityAuthAPIBaseURL = server.URL
	t.Cleanup(func() {
		openAIAgentIdentityAuthAPIBaseURL = oldBaseURL
		agentIdentityTaskLocks.Delete(account.ID)
	})

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		requestAccount := *account
		requestAccount.Credentials = cloneCredentials(account.Credentials)
		go func() {
			<-start
			errs <- ensureAgentIdentityTaskForAccount(context.Background(), repo, nil, &sync.Mutex{}, &requestAccount, "")
		}()
	}
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	registerMu.Lock()
	require.Equal(t, 1, registerCalls)
	registerMu.Unlock()
	require.Equal(t, "task-shared", account.GetCredential("task_id"))
}

func TestAgentIdentityErrorRedaction(t *testing.T) {
	key, privateKey := newAgentIdentityTestKey(t)
	account := &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity, "agent_runtime_id": key.runtimeID, "agent_private_key": privateKey, "task_id": key.taskID,
	}}
	body := redactAgentIdentitySensitiveBodyForAccount(context.Background(), nil, account, []byte(`{"message":"runtime-test task-test AgentAssertion abc123"}`))
	require.NotContains(t, string(body), key.runtimeID)
	require.NotContains(t, string(body), key.taskID)
	require.NotContains(t, string(body), "AgentAssertion abc123")
}

func TestNormalizeOpenAIAgentIdentityInputNamespaces(t *testing.T) {
	agentIdentity := &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
	}}
	body := []byte(`{
		"model":"gpt-5.5",
		"tools":[{"type":"namespace","name":"collaboration"}],
		"input":[
			{"type":"message","namespace":"keep-message","content":"hello"},
			{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{}"},
			{"type":"custom_tool_call","namespace":"image_gen","name":"generate","input":"{}"}
		]
	}`)

	normalized, changed, err := normalizeOpenAIAgentIdentityInputNamespaces(agentIdentity, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "collaboration", gjson.GetBytes(normalized, "tools.0.name").String())
	require.Equal(t, "keep-message", gjson.GetBytes(normalized, "input.0.namespace").String())
	require.False(t, gjson.GetBytes(normalized, "input.1.namespace").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.2.namespace").Exists())

	standardOAuth := &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{}}
	unchanged, changed, err := normalizeOpenAIAgentIdentityInputNamespaces(standardOAuth, body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, unchanged)
}

func TestOpenAIWSPassthroughRequestNormalizerStripsAgentIdentityInputNamespaces(t *testing.T) {
	account := &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity,
	}}
	normalizer := &openAIWSPassthroughRequestNormalizer{account: account}

	normalized, err := normalizer.Normalize(coderws.MessageText, []byte(`{
		"type":"response.create",
		"model":"gpt-5.5",
		"input":[{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{}"}]
	}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(normalized, "input.0.namespace").Exists())
}

func TestSetOpenAIChatGPTAccountHeadersIncludesFedRAMP(t *testing.T) {
	account := &Account{Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity, "chatgpt_account_id": "team-fedramp", "chatgpt_account_is_fedramp": true,
	}}
	headers := make(http.Header)
	setOpenAIChatGPTAccountHeaders(headers, account)
	require.Equal(t, "team-fedramp", headers.Get("chatgpt-account-id"))
	require.Equal(t, "true", headers.Get("x-openai-fedramp"))
}

type agentIdentityTestCredentialsRepo struct {
	AccountRepository
	account *Account
	mu      sync.Mutex
}

func (r *agentIdentityTestCredentialsRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.account, nil
}

func (r *agentIdentityTestCredentialsRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.Credentials = cloneCredentials(credentials)
	return nil
}
