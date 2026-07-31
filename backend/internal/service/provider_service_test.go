package service

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type providerSecretEncryptorStub struct{}

func (providerSecretEncryptorStub) Encrypt(plaintext string) (string, error) {
	return "cipher:" + plaintext, nil
}

func (providerSecretEncryptorStub) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "cipher:"), nil
}

type providerServiceAccountRepositoryStub struct {
	AccountRepository
	accounts map[int64]*Account
	nextID   int64
}

func newProviderServiceAccountRepositoryStub() *providerServiceAccountRepositoryStub {
	return &providerServiceAccountRepositoryStub{accounts: make(map[int64]*Account), nextID: 100}
}

func (r *providerServiceAccountRepositoryStub) Create(_ context.Context, account *Account) error {
	r.nextID++
	account.ID = r.nextID
	r.accounts[account.ID] = cloneProviderAccount(account)
	return nil
}

func (r *providerServiceAccountRepositoryStub) GetByID(_ context.Context, id int64) (*Account, error) {
	account := r.accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	return cloneProviderAccount(account), nil
}

func (r *providerServiceAccountRepositoryStub) Update(_ context.Context, account *Account) error {
	r.accounts[account.ID] = cloneProviderAccount(account)
	return nil
}

func (r *providerServiceAccountRepositoryStub) Delete(_ context.Context, id int64) error {
	delete(r.accounts, id)
	return nil
}

func (r *providerServiceAccountRepositoryStub) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	account := r.accounts[accountID]
	if account == nil {
		return errors.New("account not found")
	}
	account.GroupIDs = append([]int64(nil), groupIDs...)
	return nil
}

type providerServiceAdminRepositoryStub struct {
	ProviderAdminRepository
	accounts     *providerServiceAccountRepositoryStub
	profile      *ProviderProfile
	endpoints    map[ProtocolFamily]ProviderProtocolEndpoint
	capabilities map[string]ProviderModelCapability
	models       map[int64]LogicalModel
	modelIDs     map[string]int64
	nextID       int64
	updateCalls  int
}

func newProviderServiceAdminRepositoryStub(accounts *providerServiceAccountRepositoryStub) *providerServiceAdminRepositoryStub {
	return &providerServiceAdminRepositoryStub{
		accounts: accounts, endpoints: make(map[ProtocolFamily]ProviderProtocolEndpoint),
		capabilities: make(map[string]ProviderModelCapability), models: make(map[int64]LogicalModel),
		modelIDs: make(map[string]int64), nextID: 1000,
	}
}

func (r *providerServiceAdminRepositoryStub) SaveProfile(_ context.Context, profile *ProviderProfile) error {
	r.profile = cloneProviderServiceProfile(profile)
	return nil
}

func (r *providerServiceAdminRepositoryStub) SaveEndpoint(_ context.Context, endpoint *ProviderProtocolEndpoint) error {
	if endpoint.ID == 0 {
		r.nextID++
		endpoint.ID = r.nextID
	}
	r.endpoints[endpoint.Protocol] = *endpoint
	return nil
}

func (r *providerServiceAdminRepositoryStub) SaveCapability(_ context.Context, capability *ProviderModelCapability) error {
	if capability.ID == 0 {
		r.nextID++
		capability.ID = r.nextID
	}
	r.capabilities[providerCapabilityKeyByID(capability.LogicalModelID, capability.Protocol)] = *capability
	return nil
}

func (r *providerServiceAdminRepositoryStub) UpsertLogicalModel(_ context.Context, model *LogicalModel) error {
	if model.ID == 0 {
		if id := r.modelIDs[model.Name]; id != 0 {
			model.ID = id
		} else {
			r.nextID++
			model.ID = r.nextID
			r.modelIDs[model.Name] = model.ID
		}
	}
	r.models[model.ID] = *model
	return nil
}

func (r *providerServiceAdminRepositoryStub) GetByID(ctx context.Context, providerID int64) (*ProviderAggregate, error) {
	if r.profile == nil || r.profile.ID != providerID {
		return nil, errors.New("provider not found")
	}
	account, err := r.accounts.GetByID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	profile := cloneProviderServiceProfile(r.profile)
	profile.Endpoints = make([]ProviderProtocolEndpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		profile.Endpoints = append(profile.Endpoints, endpoint)
	}
	profile.Capabilities = make([]ProviderModelCapability, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		profile.Capabilities = append(profile.Capabilities, capability)
	}
	models := make(map[int64]LogicalModel, len(r.models))
	for id, model := range r.models {
		models[id] = model
	}
	return &ProviderAggregate{Profile: profile, Account: account, LogicalModels: models}, nil
}

func (r *providerServiceAdminRepositoryStub) List(ctx context.Context) ([]*ProviderAggregate, error) {
	if r.profile == nil {
		return nil, nil
	}
	aggregate, err := r.GetByID(ctx, r.profile.ID)
	if err != nil {
		return nil, err
	}
	return []*ProviderAggregate{aggregate}, nil
}

func (r *providerServiceAdminRepositoryStub) UpdateProfileIfVersion(_ context.Context, profile *ProviderProfile, expectedVersion int64) error {
	r.updateCalls++
	if r.profile == nil || r.profile.Version != expectedVersion {
		return ErrProviderVersionConflict
	}
	r.profile = cloneProviderServiceProfile(profile)
	return nil
}

func cloneProviderServiceProfile(profile *ProviderProfile) *ProviderProfile {
	if profile == nil {
		return nil
	}
	clone := *profile
	clone.Connection.Headers = cloneStringMap(profile.Connection.Headers)
	clone.Endpoints = append([]ProviderProtocolEndpoint(nil), profile.Endpoints...)
	clone.Capabilities = append([]ProviderModelCapability(nil), profile.Capabilities...)
	return &clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func providerCapabilityKeyByID(modelID int64, protocol ProtocolFamily) string {
	return strconv.FormatInt(modelID, 10) + "\x00" + string(protocol)
}

func TestProviderServiceCreateDefaultsToDraftWithConversionDisabled(t *testing.T) {
	accounts := newProviderServiceAccountRepositoryStub()
	repository := newProviderServiceAdminRepositoryStub(accounts)
	providerService := NewProviderService(repository, accounts, nil, providerSecretEncryptorStub{}, nil)
	apiKey := "upstream-secret"

	aggregate, err := providerService.Create(context.Background(), ProviderWriteInput{
		Name: "NewAPI", BaseURL: "https://newapi.example.com", APIKey: &apiKey,
	})

	require.NoError(t, err)
	require.Equal(t, ProviderStatusDraft, aggregate.Profile.Status)
	require.False(t, aggregate.Profile.AllowProtocolConversion)
	require.Equal(t, int64(1), aggregate.Profile.Version)
	require.Empty(t, aggregate.Account.GetCredential("api_key"))
	require.NotEqual(t, "upstream-secret", aggregate.Account.GetCredential(providerAPIKeyCiphertextKey))
}

func TestProviderServiceRejectsActivationWithoutNativeCapability(t *testing.T) {
	accounts := newProviderServiceAccountRepositoryStub()
	repository := newProviderServiceAdminRepositoryStub(accounts)
	providerService := NewProviderService(repository, accounts, nil, providerSecretEncryptorStub{}, nil)
	aggregate, err := providerService.Create(context.Background(), ProviderWriteInput{
		Name: "Empty provider", BaseURL: "https://provider.example.com",
	})
	require.NoError(t, err)

	_, err = providerService.Activate(context.Background(), aggregate.Profile.ID, aggregate.Profile.Version)

	require.ErrorIs(t, err, ErrProviderActivation)
	require.Equal(t, ProviderStatusDraft, repository.profile.Status)
	require.Zero(t, repository.updateCalls)
}

func TestProviderServiceRejectsStaleVersion(t *testing.T) {
	accounts := newProviderServiceAccountRepositoryStub()
	repository := newProviderServiceAdminRepositoryStub(accounts)
	providerService := NewProviderService(repository, accounts, nil, providerSecretEncryptorStub{}, nil)
	aggregate, err := providerService.Create(context.Background(), ProviderWriteInput{
		Name: "Versioned provider", BaseURL: "https://provider.example.com",
	})
	require.NoError(t, err)
	repository.profile.Version++

	_, err = providerService.Update(context.Background(), aggregate.Profile.ID, ProviderWriteInput{
		Name: "Stale edit", BaseURL: "https://provider.example.com", Version: aggregate.Profile.Version,
	})

	require.ErrorIs(t, err, ErrProviderVersionConflict)
	require.Equal(t, "Versioned provider", repository.profile.Name)
}

func TestProviderServiceFailedCapabilityTestDoesNotActivateDraft(t *testing.T) {
	accounts := newProviderServiceAccountRepositoryStub()
	repository := newProviderServiceAdminRepositoryStub(accounts)
	upstream := &providerForwardUpstreamStub{status: 500, result: `{"error":{"message":"failed"}}`}
	providerService := NewProviderService(repository, accounts, NewProviderForwarder(upstream, ProviderForwarderOptions{}), providerSecretEncryptorStub{}, nil)
	apiKey := "upstream-secret"
	aggregate, err := providerService.Create(context.Background(), ProviderWriteInput{
		Name: "Test provider", BaseURL: "https://provider.example.com", APIKey: &apiKey,
		Endpoints: []ProviderEndpointInput{{
			Protocol: ProtocolChatCompletions, Path: "/v1/chat/completions", Enabled: true,
		}},
		Capabilities: []ProviderCapabilityInput{{
			LogicalModelName: "model-a", Protocol: ProtocolChatCompletions,
			UpstreamModel: "vendor-model", FeatureProfile: FeatureProfileText, Enabled: true,
		}},
	})
	require.NoError(t, err)
	restoreDNS := stubProviderDNS(t, net.ParseIP("203.0.113.50"))
	defer restoreDNS()

	_, err = providerService.TestCapability(context.Background(), aggregate.Profile.ID, ProviderCapabilityTestInput{
		Protocol: ProtocolChatCompletions, LogicalModel: "model-a",
	})

	require.Error(t, err)
	require.Equal(t, ProviderStatusDraft, repository.profile.Status)
	require.Zero(t, repository.updateCalls)
}
