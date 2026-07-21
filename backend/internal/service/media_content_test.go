package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testInlineMediaDecodedBytes = 1 << 20

func inlineVideoDataURL(size int) string {
	return "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'v'}, size))
}

func inlineImageDataURL(size int) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'i'}, size))
}

type mediaContentTaskRepoStub struct {
	task      *MediaTask
	err       error
	returnNil bool
}

func (s *mediaContentTaskRepoStub) GetByPublicIDForUser(_ context.Context, publicID string, userID, apiKeyID int64) (*MediaTask, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.returnNil {
		return nil, nil
	}
	if s.task == nil || s.task.PublicID != publicID || s.task.UserID != userID ||
		(s.task.APIKeyID > 0 && s.task.APIKeyID != apiKeyID) {
		return nil, ErrMediaTaskNotFound
	}
	copy := *s.task
	// Most content tests predate billing visibility and model an already
	// publishable completed task. Individual settlement tests set an explicit
	// non-settled status.
	if copy.Status == MediaTaskStatusCompleted && copy.BillingStatus == "" {
		copy.BillingStatus = MediaBillingStatusSettled
	}
	return &copy, nil
}

type mediaContentArtifactRepoStub struct {
	items       []MediaArtifact
	createCalls int
	createErrAt int
	createErr   error
	deleteErr   error
}

func (s *mediaContentArtifactRepoStub) Create(_ context.Context, artifact *MediaArtifact) (*MediaArtifact, error) {
	s.createCalls++
	if s.createErr != nil && s.createCalls == s.createErrAt {
		return nil, s.createErr
	}
	copy := *artifact
	copy.ID = int64(len(s.items) + 1)
	s.items = append(s.items, copy)
	return &copy, nil
}

func (s *mediaContentArtifactRepoStub) DeleteExact(_ context.Context, artifact *MediaArtifact) (bool, error) {
	if s.deleteErr != nil {
		return false, s.deleteErr
	}
	for index := range s.items {
		stored := s.items[index]
		if artifact != nil && stored.ID == artifact.ID && stored.TaskID == artifact.TaskID &&
			stored.Direction == artifact.Direction && stored.Position == artifact.Position &&
			stored.StorageProvider == artifact.StorageProvider && stored.ObjectKey == artifact.ObjectKey &&
			stored.ChecksumSHA256 == artifact.ChecksumSHA256 {
			s.items = append(s.items[:index], s.items[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *mediaContentArtifactRepoStub) ListByTaskID(_ context.Context, taskID int64) ([]MediaArtifact, error) {
	var result []MediaArtifact
	for _, artifact := range s.items {
		if artifact.TaskID == taskID {
			result = append(result, artifact)
		}
	}
	return result, nil
}

type mediaContentSettingsStub struct {
	settings *SystemSettings
}

func (s mediaContentSettingsStub) GetAllSettings(context.Context) (*SystemSettings, error) {
	return s.settings, nil
}

type mediaContentAccountRepoStub struct {
	account *Account
	err     error
}

func (s mediaContentAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.account == nil {
		return nil, ErrMediaContentUnavailable
	}
	copy := *s.account
	return &copy, nil
}

type mediaContentFetcherAdapterStub struct {
	content *MediaContent
	err     error
	calls   atomic.Int64
}

func (*mediaContentFetcherAdapterStub) Name() string { return "content-fetcher" }
func (s *mediaContentFetcherAdapterStub) OpenContent(context.Context, *Account, *MediaArtifact, string) (*MediaContent, error) {
	s.calls.Add(1)
	return s.content, s.err
}

type mediaContentHTTPReaderStub struct{}

func (mediaContentHTTPReaderStub) ValidateURL(raw string) (string, error) { return raw, nil }
func (mediaContentHTTPReaderStub) Open(context.Context, MediaHTTPContentRequest) (*MediaContent, error) {
	return nil, ErrMediaContentUnavailable
}

type mediaContentHTTPReaderResultStub struct {
	content *MediaContent
	err     error
}

func (mediaContentHTTPReaderResultStub) ValidateURL(raw string) (string, error) { return raw, nil }
func (s mediaContentHTTPReaderResultStub) Open(context.Context, MediaHTTPContentRequest) (*MediaContent, error) {
	return s.content, s.err
}

type trackedMediaReadCloser struct {
	reader      io.Reader
	closeErr    error
	closeCalls  int
	largestRead int
}

func (r *trackedMediaReadCloser) Read(buffer []byte) (int, error) {
	if len(buffer) > r.largestRead {
		r.largestRead = len(buffer)
	}
	return r.reader.Read(buffer)
}

func (r *trackedMediaReadCloser) Close() error {
	r.closeCalls++
	return r.closeErr
}

type mediaContentObjectStoreStub struct {
	put       func(context.Context, MediaArtifactInput) (*MediaArtifact, error)
	putStream func(context.Context, MediaArtifactInput, io.Reader) (*MediaArtifact, error)
	open      func(context.Context, *MediaArtifact, string) (*MediaContent, error)
	discard   func(context.Context, MediaArtifactInput) error
}

type mediaContentConsistencyStub struct {
	repo  MediaArtifactRepository
	calls int
	errAt int
	err   error
}

func (*mediaContentConsistencyStub) CommitConfig(context.Context, string, string, []string) error {
	return errors.New("unexpected config commit")
}

func (s *mediaContentConsistencyStub) CommitArtifact(
	ctx context.Context,
	_ string,
	artifact *MediaArtifact,
) (*MediaArtifact, error) {
	s.calls++
	if s.err != nil && s.calls == s.errAt {
		return nil, s.err
	}
	return s.repo.Create(ctx, artifact)
}

func (s mediaContentObjectStoreStub) Discard(ctx context.Context, input MediaArtifactInput) error {
	if s.discard != nil {
		return s.discard(ctx, input)
	}
	return nil
}

func (s mediaContentObjectStoreStub) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	return s.put(ctx, input)
}

func (s mediaContentObjectStoreStub) PutStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	if s.putStream == nil {
		return nil, ErrMediaStorageProviderUnavailable
	}
	return s.putStream(ctx, input, body)
}

func (s mediaContentObjectStoreStub) Open(ctx context.Context, artifact *MediaArtifact, byteRange string) (*MediaContent, error) {
	if s.open != nil {
		return s.open(ctx, artifact, byteRange)
	}
	return nil, ErrMediaArtifactObjectStoreDisabled
}

func TestMediaContentServiceDecodesDataURLAndAppliesRange(t *testing.T) {
	accountID := int64(9)
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID,
		MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", Position: 0, MediaType: MediaTypeVideo,
		ContentType: "video/mp4", UpstreamReference: "data:video/mp4;base64,MDEyMzQ1Njc4OQ==",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "bytes=2-5")
	require.NoError(t, err)
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "bytes 2-5/10", content.ContentRange)
	require.Equal(t, []byte("2345"), body)
}

func TestMediaContentServiceDecodesImageDataURLAndAppliesRange(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42,
		MediaType: MediaTypeImage, Status: MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", Position: 0, MediaType: MediaTypeImage,
		ContentType: "image/png", UpstreamReference: "data:image/png;base64,MDEyMzQ1Njc4OQ==",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenImage(context.Background(), "task_public", 42, 8, 0, "bytes=2-5")
	require.NoError(t, err)
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "image/png", content.ContentType)
	require.Equal(t, "bytes 2-5/10", content.ContentRange)
	require.Equal(t, []byte("2345"), body)
}

func TestMediaContentServiceOpensImageOutputByPosition(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42,
		MediaType: MediaTypeImage, Status: MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{
		{
			ID: 2, TaskID: 1, Direction: "output", Position: 0, MediaType: MediaTypeImage,
			ContentType: "image/png", UpstreamReference: "data:image/png;base64,Zmlyc3Q=",
		},
		{
			ID: 3, TaskID: 1, Direction: "output", Position: 1, MediaType: MediaTypeImage,
			ContentType: "image/png", UpstreamReference: "data:image/png;base64,c2Vjb25k",
		},
	}}
	svc := NewMediaContentService(
		tasks, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenImage(context.Background(), "task_public", 42, 8, 1, "")
	require.NoError(t, err)
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("second"), body)

	_, err = svc.OpenImage(context.Background(), "task_public", 42, 8, 2, "")
	require.ErrorIs(t, err, ErrMediaArtifactNotFound)
}

func TestMediaContentServiceHidesTaskOwnedByAnotherUser(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	svc := NewMediaContentService(
		tasks, &mediaContentArtifactRepoStub{},
		mediaContentSettingsStub{settings: &SystemSettings{}}, mediaContentAccountRepoStub{},
		NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)

	_, err := svc.OpenVideo(context.Background(), "task_public", 99, 8, "")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestMediaContentServiceHidesTaskOwnedByAnotherAPIKey(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, APIKeyID: 8,
		MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	svc := NewMediaContentService(
		tasks, &mediaContentArtifactRepoStub{},
		mediaContentSettingsStub{settings: &SystemSettings{}}, mediaContentAccountRepoStub{},
		NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)

	_, err := svc.OpenVideo(context.Background(), "task_public", 42, 9, "")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestMediaContentServiceRejectsUnsatisfiableRange(t *testing.T) {
	content, err := sliceMediaContent([]byte("0123"), "video/mp4", "bytes=9-10")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaRangeNotSatisfiable)
}

func TestValidateMediaRangeParsesSingleRangeCompletely(t *testing.T) {
	for _, valid := range []string{"bytes=0-9", "bytes=9-", "bytes=-9"} {
		t.Run("valid "+valid, func(t *testing.T) {
			require.NoError(t, ValidateMediaRange(valid))
		})
	}
	for _, invalid := range []string{
		"bytes=9-1", "bytes=-0", "bytes=-", "bytes=", "bytes=0-1,4-5",
		"bytes=+0-+1", "bytes=0-+1", "bytes=−1-2", "bytes=０-１", "",
		"bytes=9223372036854775808-", "bytes=0-9223372036854775808",
	} {
		t.Run("invalid "+invalid, func(t *testing.T) {
			require.ErrorIs(t, ValidateMediaRange(invalid), ErrInvalidMediaRange)
		})
	}
}

func TestMediaContentServicePropagatesObjectStoreRangeErrorsWithoutProxyFallback(t *testing.T) {
	for _, rangeErr := range []error{ErrInvalidMediaRange, ErrMediaRangeNotSatisfiable} {
		t.Run(rangeErr.Error(), func(t *testing.T) {
			tasks := &mediaContentTaskRepoStub{task: &MediaTask{
				ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
			}}
			artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
				ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
				UpstreamReference: "data:video/mp4;base64,MDEyMw==", ContentType: "video/mp4",
			}}}
			svc := NewMediaContentService(
				tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
				mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
				mediaContentObjectStoreStub{
					put:  func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
					open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) { return nil, rangeErr },
				},
			)
			content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "bytes=0-1")
			require.Nil(t, content)
			require.ErrorIs(t, err, rangeErr)
		})
	}
}

func TestMediaContentServiceFallsBackToInlineDataAfterObjectStoreError(t *testing.T) {
	storeErr := errors.New("object store unavailable")
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
		UpstreamReference: "data:video/mp4;base64,MDEyMw==", ContentType: "video/mp4",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			put:  func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
			open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) { return nil, storeErr },
		},
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.NoError(t, err)
	defer content.Body.Close()
	body, readErr := io.ReadAll(content.Body)
	require.NoError(t, readErr)
	require.Equal(t, []byte("0123"), body)
}

func TestMediaContentServiceFallsBackWhenObjectStoreReturnsNilBody(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
		UpstreamReference: "data:video/mp4;base64,MDEyMw==", ContentType: "video/mp4",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
			open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) {
				return &MediaContent{StatusCode: http.StatusOK, ContentType: "video/mp4"}, nil
			},
		},
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.NoError(t, err)
	require.NotNil(t, content.Body)
	defer content.Body.Close()
	body, readErr := io.ReadAll(content.Body)
	require.NoError(t, readErr)
	require.Equal(t, []byte("0123"), body)
}

func TestMediaContentServiceFallsBackToAdapterAfterObjectStoreError(t *testing.T) {
	storeErr := errors.New("object store unavailable")
	accountID := int64(9)
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID, Adapter: "content-fetcher",
		MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
		UpstreamReference: "upstream-video-reference", ContentType: "video/mp4",
	}}}
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.Register("content-fetcher", &mediaContentFetcherAdapterStub{content: &MediaContent{
		Body: io.NopCloser(strings.NewReader("proxy")), StatusCode: http.StatusOK, ContentLength: 5, ContentType: "video/mp4",
	}}))
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}}, registry, mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			put:  func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
			open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) { return nil, storeErr },
		},
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.NoError(t, err)
	defer content.Body.Close()
	body, readErr := io.ReadAll(content.Body)
	require.NoError(t, readErr)
	require.Equal(t, []byte("proxy"), body)
}

func TestMediaContentServiceUsesHistoricalAdapterAliasForContentFetch(t *testing.T) {
	accountID := int64(9)
	task := &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID,
		Adapter: "legacy-content", MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}
	tasks := &mediaContentTaskRepoStub{task: task}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: task.ID, Direction: "output", Position: 0, MediaType: MediaTypeVideo,
		ContentType: "video/mp4", UpstreamReference: "upstream-video-reference",
	}}}
	registry := NewMediaAdapterRegistry()
	adapter := &mediaContentFetcherAdapterStub{content: &MediaContent{
		Body: io.NopCloser(strings.NewReader("proxy")), StatusCode: http.StatusOK,
		ContentLength: 5, ContentType: "video/mp4",
	}}
	require.NoError(t, registry.Register("content-fetcher", adapter))
	require.NoError(t, registry.RegisterAlias("legacy-content", "content-fetcher"))
	svc := NewMediaContentService(
		tasks,
		artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}},
		registry,
		mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenVideo(context.Background(), task.PublicID, task.UserID, task.APIKeyID, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, content.Body.Close()) })
	require.Equal(t, int64(1), adapter.calls.Load())
	require.Equal(t, http.StatusOK, content.StatusCode)
	require.Equal(t, "legacy-content", task.Adapter)
}

func TestMediaContentServiceFinalFallbackErrorPreservesUnavailableAndInternalCauses(t *testing.T) {
	storeErr := errors.New("object store unavailable")
	proxyErr := errors.New("proxy network unavailable")
	accountID := int64(9)
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID, Adapter: "content-fetcher",
		MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
		UpstreamReference: "upstream-video-reference", ContentType: "video/mp4",
	}}}
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.Register("content-fetcher", &mediaContentFetcherAdapterStub{err: proxyErr}))
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}}, registry, mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			put:  func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
			open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) { return nil, storeErr },
		},
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.ErrorIs(t, err, storeErr)
	require.ErrorIs(t, err, proxyErr)
}

func TestMediaContentServiceSecureProxyFailuresRemainUnavailable(t *testing.T) {
	for _, proxyErr := range []error{ErrSecureHTTPUpstreamProxyUnsupported, ErrMediaSecureUpstreamRequired} {
		t.Run(proxyErr.Error(), func(t *testing.T) {
			accountID := int64(9)
			tasks := &mediaContentTaskRepoStub{task: &MediaTask{
				ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID, Adapter: "content-fetcher",
				MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
			}}
			artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
				ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo,
				UpstreamReference: "upstream-video-reference", ContentType: "video/mp4",
			}}}
			registry := NewMediaAdapterRegistry()
			require.NoError(t, registry.Register("content-fetcher", &mediaContentFetcherAdapterStub{err: proxyErr}))
			svc := NewMediaContentService(
				tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
				mediaContentAccountRepoStub{account: &Account{ID: accountID}}, registry,
				mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
			)

			content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
			require.Nil(t, content)
			require.ErrorIs(t, err, ErrMediaContentUnavailable)
			require.ErrorIs(t, err, proxyErr)
		})
	}
}

func TestMediaContentServiceNoFallbackPreservesStoreErrorAsUnavailable(t *testing.T) {
	storeErr := errors.New("object store unavailable")
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo, ObjectKey: "objects/video",
		ContentType: "video/mp4",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			put:  func(context.Context, MediaArtifactInput) (*MediaArtifact, error) { return nil, nil },
			open: func(context.Context, *MediaArtifact, string) (*MediaContent, error) { return nil, storeErr },
		},
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.ErrorIs(t, err, storeErr)
}

func TestMediaContentServiceStageUploadClearsDataAndKeepsChecksum(t *testing.T) {
	data := []byte("fake-image")
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{put: func(_ context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
			require.Equal(t, data, input.Data)
			require.NotEmpty(t, input.ChecksumSHA256)
			return &MediaArtifact{MediaType: input.MediaType, ContentType: input.ContentType, ObjectKey: "objects/input"}, nil
		}},
	)

	staged, err := svc.Stage(context.Background(), 42, MediaArtifactInput{
		Direction: "input", MediaType: MediaTypeImage, ContentType: "image/png", Data: data,
	})
	require.NoError(t, err)
	require.Nil(t, staged.Data)
	require.Equal(t, "objects/input", staged.ObjectKey)
	sum := sha256.Sum256(data)
	require.Equal(t, hex.EncodeToString(sum[:]), staged.ChecksumSHA256)
}

func TestMediaContentServiceStageExternalURLDerivesStrictContentType(t *testing.T) {
	svc := NewMediaContentService(
		nil, nil, nil, nil, nil, mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)
	image, err := svc.Stage(context.Background(), 42, MediaArtifactInput{
		MediaType: MediaTypeImage, ExternalURL: "https://media.example/input.PNG?token=internal",
	})
	require.NoError(t, err)
	require.Equal(t, "image/png", image.ContentType)
	require.Equal(t, "https://media.example/input.PNG?token=internal", image.ExternalURL)

	video, err := svc.Stage(context.Background(), 42, MediaArtifactInput{
		MediaType: MediaTypeVideo, ExternalURL: "https://media.example/input.mp4?signature=internal",
	})
	require.NoError(t, err)
	require.Equal(t, "video/mp4", video.ContentType)

	quickTime, err := svc.Stage(context.Background(), 42, MediaArtifactInput{
		MediaType: MediaTypeVideo, ExternalURL: "https://media.example/input.MOV?signature=internal",
	})
	require.NoError(t, err)
	require.Equal(t, "video/quicktime", quickTime.ContentType)
}

func TestMediaContentServiceStageExternalURLRejectsUnknownOrWrongMediaExtension(t *testing.T) {
	svc := NewMediaContentService(
		nil, nil, nil, nil, nil, mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)
	for _, input := range []MediaArtifactInput{
		{MediaType: MediaTypeImage, ExternalURL: "https://media.example/no-extension"},
		{MediaType: MediaTypeImage, ExternalURL: "https://media.example/wrong.mp4"},
		{MediaType: MediaTypeVideo, ExternalURL: "https://media.example/wrong.png"},
	} {
		_, err := svc.Stage(context.Background(), 42, input)
		require.ErrorIs(t, err, ErrInvalidMediaInput)
	}
}

func TestMediaContentServiceDiscardExternalURLIsNoop(t *testing.T) {
	discardCalls := 0
	svc := NewMediaContentService(
		nil, nil, nil, nil, nil, mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			discard: func(context.Context, MediaArtifactInput) error {
				discardCalls++
				return nil
			},
		},
	)

	err := svc.Discard(context.Background(), 42, MediaArtifactInput{
		MediaType: MediaTypeImage, ExternalURL: "https://media.example/input.png",
	})
	require.NoError(t, err)
	require.Zero(t, discardCalls)
}

func TestMediaContentServiceDiscardObjectKeyUsesObjectStore(t *testing.T) {
	input := MediaArtifactInput{MediaType: MediaTypeVideo, ObjectKey: "staged/input-video"}
	var discarded MediaArtifactInput
	svc := NewMediaContentService(
		nil, nil, nil, nil, nil, mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			discard: func(_ context.Context, value MediaArtifactInput) error {
				discarded = value
				return nil
			},
		},
	)

	err := svc.Discard(context.Background(), 42, input)
	require.NoError(t, err)
	require.Equal(t, input, discarded)
}

func TestMediaContentServiceDiscardPreservesObjectStoreError(t *testing.T) {
	discardErr := errors.New("discard failed")
	svc := NewMediaContentService(
		nil, nil, nil, nil, nil, mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{
			discard: func(context.Context, MediaArtifactInput) error { return discardErr },
		},
	)

	err := svc.Discard(context.Background(), 42, MediaArtifactInput{ObjectKey: "staged/input"})
	require.ErrorIs(t, err, discardErr)
}

func TestDisabledMediaArtifactObjectStoreRejectsPutAndOpen(t *testing.T) {
	store := NewDisabledMediaArtifactObjectStore()
	_, err := store.Put(context.Background(), MediaArtifactInput{Data: []byte("video")})
	require.ErrorIs(t, err, ErrMediaArtifactObjectStoreDisabled)
	_, err = store.Open(context.Background(), &MediaArtifact{}, "")
	require.ErrorIs(t, err, ErrMediaArtifactObjectStoreDisabled)
}

func TestMediaContentServiceVideoUploadRequiresObjectStorage(t *testing.T) {
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.Stage(context.Background(), 42, MediaArtifactInput{MediaType: MediaTypeVideo, Data: []byte("video")})
	require.ErrorIs(t, err, ErrMediaVideoObjectStorageRequired)
}

func TestMediaContentServiceRejectsEmptyOrMismatchedOutputsBeforeSideEffects(t *testing.T) {
	for _, tt := range []struct {
		name   string
		task   *MediaTask
		inputs []MediaArtifactInput
	}{
		{name: "empty", task: &MediaTask{ID: 10, MediaType: MediaTypeImage}},
		{
			name: "mismatched media type in batch", task: &MediaTask{ID: 10, MediaType: MediaTypeImage},
			inputs: []MediaArtifactInput{
				{MediaType: MediaTypeImage, ContentType: "image/png", ExternalURL: "https://cdn.example/first.png"},
				{MediaType: MediaTypeVideo, ContentType: "video/mp4", ObjectKey: "private/wrong-type"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			artifacts := &mediaContentArtifactRepoStub{}
			putCalls := 0
			svc := NewMediaContentService(
				nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
				mediaContentHTTPReaderStub{}, mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
					putCalls++
					return &MediaArtifact{ObjectKey: "objects/output"}, nil
				}},
			)

			stored, err := svc.PersistOutputs(context.Background(), tt.task, tt.inputs)
			require.Nil(t, stored)
			require.ErrorIs(t, err, ErrInvalidMediaInput)
			require.Zero(t, putCalls)
			require.Empty(t, artifacts.items)
		})
	}
}

func TestMediaContentServiceStreamsExternalOutputAndClosesBody(t *testing.T) {
	accountID := int64(9)
	video := testMediaMP4(2 << 20)
	sum := sha256.Sum256(video)
	body := &trackedMediaReadCloser{reader: bytes.NewReader(video)}
	artifacts := &mediaContentArtifactRepoStub{}
	bufferedPutCalled := false
	streamPutCalled := false
	store := mediaContentObjectStoreStub{
		put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			bufferedPutCalled = true
			return nil, errors.New("unexpected buffered put")
		},
		putStream: func(_ context.Context, input MediaArtifactInput, reader io.Reader) (*MediaArtifact, error) {
			streamPutCalled = true
			require.Empty(t, input.Data)
			require.Empty(t, input.ExternalURL)
			require.Equal(t, int64(len(video)), input.SizeBytes)
			written, err := io.CopyBuffer(io.Discard, reader, make([]byte, 32<<10))
			require.NoError(t, err)
			require.Equal(t, int64(len(video)), written)
			return &MediaArtifact{
				MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: written,
				ChecksumSHA256: hex.EncodeToString(sum[:]), StorageProvider: MediaStorageProviderLocal,
				ObjectKey: "tasks/output/video.mp4",
			}, nil
		},
	}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}}, nil,
		mediaContentHTTPReaderResultStub{content: &MediaContent{
			Body: body, StatusCode: http.StatusOK, ContentType: "video/mp4", ContentLength: int64(len(video)),
		}},
		store,
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{
		ID: 10, AccountID: &accountID, MediaType: MediaTypeVideo,
	}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", ExternalURL: "https://cdn.example/video.mp4",
	}})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.True(t, streamPutCalled)
	require.False(t, bufferedPutCalled)
	require.Equal(t, 1, body.closeCalls)
	require.LessOrEqual(t, body.largestRead, 32<<10)
}

func TestMediaContentServiceDiscardsStreamedObjectWhenResponseCloseFails(t *testing.T) {
	accountID := int64(9)
	closeErr := errors.New("upstream response close failed")
	body := &trackedMediaReadCloser{reader: bytes.NewReader(testMediaMP4(2048)), closeErr: closeErr}
	artifacts := &mediaContentArtifactRepoStub{}
	discarded := false
	store := mediaContentObjectStoreStub{
		put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return nil, errors.New("unexpected buffered put")
		},
		putStream: func(_ context.Context, input MediaArtifactInput, reader io.Reader) (*MediaArtifact, error) {
			_, err := io.Copy(io.Discard, reader)
			if err != nil {
				return nil, err
			}
			return &MediaArtifact{
				MediaType: input.MediaType, ContentType: input.ContentType,
				StorageProvider: MediaStorageProviderLocal, ObjectKey: "tasks/output/video.mp4",
			}, nil
		},
		discard: func(_ context.Context, input MediaArtifactInput) error {
			discarded = true
			require.Equal(t, MediaStorageProviderLocal, input.StorageProvider)
			require.Equal(t, "tasks/output/video.mp4", input.ObjectKey)
			return nil
		},
	}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}}, nil,
		mediaContentHTTPReaderResultStub{content: &MediaContent{
			Body: body, StatusCode: http.StatusOK, ContentType: "video/mp4", ContentLength: 2048,
		}},
		store,
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{
		ID: 10, AccountID: &accountID, MediaType: MediaTypeVideo,
	}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", ExternalURL: "https://cdn.example/video.mp4",
	}})
	require.Nil(t, stored)
	require.ErrorIs(t, err, closeErr)
	require.True(t, discarded)
	require.Equal(t, 1, body.closeCalls)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServiceRejectsInvalidExternalStreamsBeforeDatabaseWrite(t *testing.T) {
	accountID := int64(9)
	video := testMediaMP4(2048)
	for _, tt := range []struct {
		name          string
		maxStoreBytes int64
		input         MediaArtifactInput
		contentType   string
		contentLength int64
		wantErr       error
	}{
		{
			name: "adapter and response length differ", maxStoreBytes: 4096,
			input: MediaArtifactInput{
				MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: int64(len(video) - 1),
				ExternalURL: "https://cdn.example/video.mp4",
			},
			contentType: "video/mp4", contentLength: int64(len(video)), wantErr: ErrMediaStorageIntegrity,
		},
		{
			name: "adapter and response content type differ", maxStoreBytes: 4096,
			input: MediaArtifactInput{
				MediaType: MediaTypeVideo, ContentType: "video/webm", ExternalURL: "https://cdn.example/video.webm",
			},
			contentType: "video/mp4", contentLength: int64(len(video)), wantErr: ErrMediaStorageIntegrity,
		},
		{
			name: "declared mime differs from body", maxStoreBytes: 4096,
			input: MediaArtifactInput{
				MediaType: MediaTypeVideo, ContentType: "video/webm", ExternalURL: "https://cdn.example/video.webm",
			},
			contentType: "video/webm", contentLength: int64(len(video)), wantErr: ErrInvalidMediaInput,
		},
		{
			name: "stream exceeds storage limit", maxStoreBytes: 1024,
			input: MediaArtifactInput{
				MediaType: MediaTypeVideo, ContentType: "video/mp4", ExternalURL: "https://cdn.example/video.mp4",
			},
			contentType: "video/mp4", contentLength: -1, wantErr: ErrMediaContentTooLarge,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := NewLocalMediaArtifactObjectStore(root, tt.maxStoreBytes)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })
			body := &trackedMediaReadCloser{reader: bytes.NewReader(video)}
			artifacts := &mediaContentArtifactRepoStub{}
			svc := NewMediaContentService(
				nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}},
				mediaContentAccountRepoStub{account: &Account{ID: accountID}}, nil,
				mediaContentHTTPReaderResultStub{content: &MediaContent{
					Body: body, StatusCode: http.StatusOK, ContentType: tt.contentType, ContentLength: tt.contentLength,
				}},
				store,
			)

			stored, err := svc.PersistOutputs(context.Background(), &MediaTask{
				ID: 10, AccountID: &accountID, MediaType: MediaTypeVideo,
			}, []MediaArtifactInput{tt.input})
			require.Nil(t, stored)
			require.ErrorIs(t, err, tt.wantErr)
			require.Empty(t, artifacts.items)
			require.Zero(t, countRegularFiles(t, root))
			require.Equal(t, 1, body.closeCalls)
		})
	}
}

func TestMediaContentServiceRollsBackEarlierOutputsWhenLaterArtifactCreateFails(t *testing.T) {
	persistErr := errors.New("persist second output failed")
	artifacts := &mediaContentArtifactRepoStub{createErr: persistErr, createErrAt: 2}
	discarded := make([]string, 0, 2)
	store := mediaContentObjectStoreStub{
		put: func(_ context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{
				MediaType: input.MediaType, ContentType: input.ContentType,
				StorageProvider: MediaStorageProviderLocal,
				ObjectKey:       "output/object-" + strconv.Itoa(input.Position),
			}, nil
		},
		discard: func(_ context.Context, input MediaArtifactInput) error {
			discarded = append(discarded, input.ObjectKey)
			return nil
		},
	}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, store,
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("first")},
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("second")},
	})

	require.Nil(t, stored)
	require.ErrorIs(t, err, persistErr)
	require.Empty(t, artifacts.items)
	// The current unindexed object is discarded first, then the earlier
	// indexed object is removed by the batch rollback.
	require.Equal(t, []string{"output/object-1", "output/object-0"}, discarded)
}

func TestMediaContentServiceRollsBackEarlierOutputsWhenLaterPutFails(t *testing.T) {
	putErr := errors.New("store second output failed")
	artifacts := &mediaContentArtifactRepoStub{}
	putCalls := 0
	discarded := make([]string, 0, 1)
	store := mediaContentObjectStoreStub{
		put: func(_ context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
			putCalls++
			if putCalls == 2 {
				return nil, putErr
			}
			return &MediaArtifact{
				MediaType: input.MediaType, ContentType: input.ContentType,
				StorageProvider: MediaStorageProviderLocal, ObjectKey: "output/first",
			}, nil
		},
		discard: func(_ context.Context, input MediaArtifactInput) error {
			discarded = append(discarded, input.ObjectKey)
			return nil
		},
	}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, store,
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("first")},
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("second")},
	})

	require.Nil(t, stored)
	require.ErrorIs(t, err, putErr)
	require.Empty(t, artifacts.items)
	require.Equal(t, []string{"output/first"}, discarded)
}

func TestMediaContentServiceRetainsObjectsWhenArtifactCommitOutcomeIsUnknown(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	consistency := &mediaContentConsistencyStub{
		repo: artifacts, errAt: 2, err: ErrMediaStorageCommitOutcomeUnknown,
	}
	discarded := make([]string, 0, 2)
	store := mediaContentObjectStoreStub{
		put: func(_ context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{
				MediaType: input.MediaType, ContentType: input.ContentType,
				StorageProvider: MediaStorageProviderLocal, StorageRevision: "revision-1",
				ObjectKey: "output/object-" + strconv.Itoa(input.Position),
			}, nil
		},
		discard: func(_ context.Context, input MediaArtifactInput) error {
			discarded = append(discarded, input.ObjectKey)
			return nil
		},
	}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, store, consistency,
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("first")},
		{MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("second")},
	})

	require.Nil(t, stored)
	require.ErrorIs(t, err, ErrMediaStorageCommitOutcomeUnknown)
	require.Len(t, artifacts.items, 1, "a prior committed row must not be compensated while the current outcome is unknown")
	require.Empty(t, discarded, "an unknown commit may already reference every written object")
}

func TestMediaContentServiceRejectsPrivateStoredImageWithoutDeliveryPath(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{ObjectKey: "private/images/output.png"}, nil
		}},
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
		MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("image"),
	}})
	require.Nil(t, stored)
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServiceRejectsObjectStoreArtifactMediaTypeMismatch(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{MediaType: MediaTypeVideo, ObjectKey: "objects/wrong-type"}, nil
		}},
	)

	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
		MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("image"),
	}})
	require.Nil(t, stored)
	require.ErrorIs(t, err, ErrInvalidMediaInput)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServiceRejectsImageInternalReferenceWithoutDeliveryPath(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
		MediaType: MediaTypeImage, ContentType: "image/png", UpstreamReference: "internal-image-reference",
	}})
	require.Nil(t, stored)
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServicePersistsOnlyPublicOrBoundedInlineImageFallbacks(t *testing.T) {
	t.Run("safe public url", func(t *testing.T) {
		artifacts := &mediaContentArtifactRepoStub{}
		svc := NewMediaContentService(
			nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
			mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
		)
		stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
			MediaType: MediaTypeImage, ContentType: "image/png", ExternalURL: "https://cdn.example/image.png",
		}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Equal(t, "https://cdn.example/image.png", stored[0].PublicURL)
		require.Empty(t, stored[0].UpstreamReference)
	})

	t.Run("bounded image data", func(t *testing.T) {
		artifacts := &mediaContentArtifactRepoStub{}
		svc := NewMediaContentService(
			nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
			mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
		)
		stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
			MediaType: MediaTypeImage, ContentType: "image/png", UpstreamReference: "data:image/png;base64,aW1hZ2U=",
		}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Equal(t, "data:image/png;base64,aW1hZ2U=", stored[0].UpstreamReference)
	})

	t.Run("over-limit image data", func(t *testing.T) {
		artifacts := &mediaContentArtifactRepoStub{}
		svc := NewMediaContentService(
			nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
			mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
		)
		stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{{
			MediaType: MediaTypeImage, ContentType: "image/png",
			UpstreamReference: inlineImageDataURL(testInlineMediaDecodedBytes + 1),
		}})
		require.Nil(t, stored)
		require.ErrorIs(t, err, ErrMediaContentTooLarge)
		require.Empty(t, artifacts.items)
	})

	for _, invalid := range []string{
		"data:text/html;base64,PHNjcmlwdD4=",
		"https://cdn.example/image.png?X-Amz-Signature=secret",
	} {
		t.Run("unsafe "+invalid, func(t *testing.T) {
			artifacts := &mediaContentArtifactRepoStub{}
			svc := NewMediaContentService(
				nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
				mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
			)
			input := MediaArtifactInput{MediaType: MediaTypeImage, ContentType: "image/png", UpstreamReference: invalid}
			if strings.HasPrefix(invalid, "https:") {
				input.UpstreamReference = ""
				input.ExternalURL = invalid
			}
			stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeImage}, []MediaArtifactInput{input})
			require.Nil(t, stored)
			require.ErrorIs(t, err, ErrMediaContentUnavailable)
			require.Empty(t, artifacts.items)
		})
	}
}

func TestMediaContentServiceRejectsVideoProxyWhenFallbackDisabled(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeVideo}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", UpstreamReference: "internal-reference",
	}})
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServiceBoundsInlineDataBeforePersistingArtifact(t *testing.T) {
	t.Run("boundary accepted", func(t *testing.T) {
		artifacts := &mediaContentArtifactRepoStub{}
		svc := NewMediaContentService(
			nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
			nil, nil, mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
		)
		stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeVideo}, []MediaArtifactInput{{
			MediaType: MediaTypeVideo, ContentType: "video/mp4", UpstreamReference: inlineVideoDataURL(testInlineMediaDecodedBytes),
		}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
	})

	t.Run("over limit rejected without DB write", func(t *testing.T) {
		artifacts := &mediaContentArtifactRepoStub{}
		svc := NewMediaContentService(
			nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
			nil, nil, mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
		)
		_, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeVideo}, []MediaArtifactInput{{
			MediaType: MediaTypeVideo, ContentType: "video/mp4", UpstreamReference: inlineVideoDataURL(testInlineMediaDecodedBytes + 1),
		}})
		require.ErrorIs(t, err, ErrMediaContentTooLarge)
		require.Empty(t, artifacts.items)
	})
}

func TestMediaContentServiceBoundsInlineDataWhileOpening(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", MediaType: MediaTypeVideo,
		ContentType: "video/mp4", UpstreamReference: inlineVideoDataURL(testInlineMediaDecodedBytes + 1),
	}}}
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)
	content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaContentTooLarge)
}

func TestMediaContentServiceStoredOutputKeepsInputMetadata(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{ObjectKey: "objects/output"}, nil
		}},
	)
	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10, MediaType: MediaTypeVideo}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: 123, ChecksumSHA256: "abc123",
	}})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, MediaTypeVideo, stored[0].MediaType)
	require.Equal(t, "video/mp4", stored[0].ContentType)
	require.Equal(t, int64(123), stored[0].SizeBytes)
	require.Equal(t, "abc123", stored[0].ChecksumSHA256)
}

func TestMediaContentServiceOpenVideoRequiresCompletedVideoOwnedByUser(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusInProgress,
	}}
	svc := NewMediaContentService(
		tasks, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestMediaContentServiceHidesCompletedOutputUntilSettlementSucceeds(t *testing.T) {
	task := &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, APIKeyID: 8,
		MediaType: MediaTypeImage, Status: MediaTaskStatusCompleted, BillingStatus: MediaBillingStatusRetry,
	}
	tasks := &mediaContentTaskRepoStub{task: task}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: task.ID, Direction: "output", Position: 0, MediaType: MediaTypeImage,
		ContentType: "image/png", UpstreamReference: "data:image/png;base64,c2VjcmV0",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenImage(context.Background(), task.PublicID, task.UserID, task.APIKeyID, 0, "")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaTaskNotFound)

	task.BillingStatus = MediaBillingStatusSettled
	content, err = svc.OpenImage(context.Background(), task.PublicID, task.UserID, task.APIKeyID, 0, "")
	require.NoError(t, err)
	require.NotNil(t, content)
	require.NoError(t, content.Body.Close())
}

func TestMediaContentServiceOpenVideoClassifiesTaskRepositoryErrors(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	for _, tt := range []struct {
		name          string
		repositoryErr error
		wantNotFound  bool
	}{
		{name: "not found", repositoryErr: ErrMediaTaskNotFound, wantNotFound: true},
		{name: "database failure", repositoryErr: databaseErr},
		{name: "deadline exceeded", repositoryErr: context.DeadlineExceeded},
		{name: "request canceled", repositoryErr: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewMediaContentService(
				&mediaContentTaskRepoStub{err: tt.repositoryErr}, &mediaContentArtifactRepoStub{},
				mediaContentSettingsStub{settings: &SystemSettings{}}, mediaContentAccountRepoStub{},
				NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
			)

			content, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
			require.Nil(t, content)
			if tt.wantNotFound {
				require.ErrorIs(t, err, ErrMediaTaskNotFound)
				require.NotErrorIs(t, err, ErrMediaContentUnavailable)
				return
			}
			require.ErrorIs(t, err, tt.repositoryErr)
			require.ErrorIs(t, err, ErrMediaContentUnavailable)
			require.NotErrorIs(t, err, ErrMediaTaskNotFound)
		})
	}
}

func TestMediaContentServiceOpenVideoHidesIneligibleTaskStates(t *testing.T) {
	for _, tt := range []struct {
		name      string
		task      *MediaTask
		returnNil bool
	}{
		{name: "nil task", returnNil: true},
		{name: "non video", task: &MediaTask{ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeImage, Status: MediaTaskStatusCompleted}},
		{name: "not completed", task: &MediaTask{ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusInProgress}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewMediaContentService(
				&mediaContentTaskRepoStub{task: tt.task, returnNil: tt.returnNil}, &mediaContentArtifactRepoStub{},
				mediaContentSettingsStub{settings: &SystemSettings{}}, mediaContentAccountRepoStub{},
				NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
			)
			_, err := svc.OpenVideo(context.Background(), "task_public", 42, 8, "")
			require.ErrorIs(t, err, ErrMediaTaskNotFound)
		})
	}
}

func TestMediaContentServiceRejectsInvalidBase64DataURL(t *testing.T) {
	data, contentType, ok := decodeMediaDataReference("data:video/mp4;base64,%%%not-base64%%%", "video/mp4")
	require.False(t, ok)
	require.Nil(t, data)
	require.Empty(t, contentType)
}

var (
	_ MediaArtifactWriter = (*MediaContentService)(nil)
	_ MediaInputLifecycle = (*MediaContentService)(nil)
)
