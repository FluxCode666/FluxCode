package repository

import (
	"context"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/qiniu/go-sdk/v7/storagev2/uploader"
	"github.com/stretchr/testify/require"
)

func TestQiniuGeneratedImageObjectStoreUploadBuildsObjectAndCDNURL(t *testing.T) {
	fakeUploader := &qiniuUploadManagerStub{}
	store := newQiniuGeneratedImageObjectStore(qiniuGeneratedImageObjectStoreOptions{
		AccessKey:     "ak",
		SecretKey:     "sk",
		Bucket:        "generated-images",
		CDNDomain:     "cdn.example.com",
		Prefix:        "openai/generated",
		UseHTTPS:      true,
		UploadTimeout: 5 * time.Second,
		TokenTTL:      time.Hour,
		Uploader:      fakeUploader,
	})

	object, err := store.Upload(context.Background(), service.GeneratedImageObjectUpload{
		Data:        []byte("hello"),
		ContentType: "image/png",
	})

	require.NoError(t, err)
	require.NotNil(t, object)
	require.NotNil(t, fakeUploader.objectOptions)
	require.Equal(t, []byte("hello"), fakeUploader.data)
	require.Equal(t, "generated-images", fakeUploader.objectOptions.BucketName)
	require.NotNil(t, fakeUploader.objectOptions.ObjectName)
	require.True(t, strings.HasPrefix(*fakeUploader.objectOptions.ObjectName, "openai/generated/"))
	require.True(t, strings.HasSuffix(*fakeUploader.objectOptions.ObjectName, ".png"))
	require.Equal(t, path.Base(*fakeUploader.objectOptions.ObjectName), fakeUploader.objectOptions.FileName)
	require.Equal(t, "image/png", fakeUploader.objectOptions.ContentType)
	require.NotNil(t, fakeUploader.objectOptions.UpToken)
	require.Equal(t, *fakeUploader.objectOptions.ObjectName, object.Key)
	require.Equal(t, "https://cdn.example.com/"+*fakeUploader.objectOptions.ObjectName, object.URL)
}

func TestDynamicQiniuGeneratedImageObjectStoreSkipsUploadWhenSourceIsDB(t *testing.T) {
	fakeUploader := &qiniuUploadManagerStub{}
	store := newDynamicQiniuGeneratedImageObjectStore(&generatedImageStorageSettingsProviderStub{
		settings: &service.GeneratedImageStorageSettings{
			Source: service.GeneratedImageStorageSourceDB,
		},
	}, fakeUploader)

	object, err := store.Upload(context.Background(), service.GeneratedImageObjectUpload{
		Data:        []byte("hello"),
		ContentType: "image/png",
	})

	require.ErrorIs(t, err, service.ErrGeneratedImageObjectStoreDisabled)
	require.Nil(t, object)
	require.Nil(t, fakeUploader.objectOptions)
}

func TestDynamicQiniuGeneratedImageObjectStoreReadsQiniuSettings(t *testing.T) {
	fakeUploader := &qiniuUploadManagerStub{}
	store := newDynamicQiniuGeneratedImageObjectStore(&generatedImageStorageSettingsProviderStub{
		settings: &service.GeneratedImageStorageSettings{
			Source:                    service.GeneratedImageStorageSourceQiniu,
			QiniuAccessKey:            "ak",
			QiniuSecretKey:            "sk",
			QiniuBucket:               "generated-images",
			QiniuCDNDomain:            "cdn.example.com",
			QiniuPrefix:               "openai/generated",
			QiniuUseHTTPS:             true,
			QiniuUploadTimeoutSeconds: 5,
			QiniuTokenTTLSeconds:      3600,
		},
	}, fakeUploader)

	object, err := store.Upload(context.Background(), service.GeneratedImageObjectUpload{
		Data:        []byte("hello"),
		ContentType: "image/png",
	})

	require.NoError(t, err)
	require.NotNil(t, object)
	require.NotNil(t, fakeUploader.objectOptions)
	require.Equal(t, []byte("hello"), fakeUploader.data)
	require.Equal(t, "generated-images", fakeUploader.objectOptions.BucketName)
	require.NotNil(t, fakeUploader.objectOptions.ObjectName)
	require.True(t, strings.HasPrefix(*fakeUploader.objectOptions.ObjectName, "openai/generated/"))
	require.Equal(t, "https://cdn.example.com/"+*fakeUploader.objectOptions.ObjectName, object.URL)
}

type generatedImageStorageSettingsProviderStub struct {
	settings *service.GeneratedImageStorageSettings
	err      error
}

func (s *generatedImageStorageSettingsProviderStub) GetGeneratedImageStorageSettings(ctx context.Context) (*service.GeneratedImageStorageSettings, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.settings, nil
}

type qiniuUploadManagerStub struct {
	data          []byte
	objectOptions *uploader.ObjectOptions
}

func (s *qiniuUploadManagerStub) UploadReader(ctx context.Context, reader io.Reader, objectOptions *uploader.ObjectOptions, returnValue interface{}) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.data = data
	s.objectOptions = objectOptions
	return nil
}
