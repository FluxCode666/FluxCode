package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/qiniu/go-sdk/v7/storagev2/credentials"
	"github.com/qiniu/go-sdk/v7/storagev2/uploader"
	"github.com/qiniu/go-sdk/v7/storagev2/uptoken"
)

type qiniuUploadManager interface {
	UploadReader(ctx context.Context, reader io.Reader, objectOptions *uploader.ObjectOptions, returnValue interface{}) error
}

type qiniuGeneratedImageObjectStore struct {
	accessKey     string
	secretKey     string
	bucket        string
	cdnDomain     string
	prefix        string
	useHTTPS      bool
	uploadTimeout time.Duration
	tokenTTL      time.Duration
	uploader      qiniuUploadManager
}

type qiniuGeneratedImageObjectStoreOptions struct {
	AccessKey     string
	SecretKey     string
	Bucket        string
	CDNDomain     string
	Prefix        string
	UseHTTPS      bool
	UploadTimeout time.Duration
	TokenTTL      time.Duration
	Uploader      qiniuUploadManager
}

type generatedImageStorageSettingsProvider interface {
	GetGeneratedImageStorageSettings(ctx context.Context) (*service.GeneratedImageStorageSettings, error)
}

type dynamicQiniuGeneratedImageObjectStore struct {
	settingsProvider generatedImageStorageSettingsProvider
	uploader         qiniuUploadManager
}

func NewQiniuGeneratedImageObjectStore(settingService *service.SettingService) service.GeneratedImageObjectStore {
	if settingService == nil {
		return nil
	}
	return newDynamicQiniuGeneratedImageObjectStore(settingService, nil)
}

func newDynamicQiniuGeneratedImageObjectStore(provider generatedImageStorageSettingsProvider, uploadManager qiniuUploadManager) service.GeneratedImageObjectStore {
	if provider == nil {
		return nil
	}
	if uploadManager == nil {
		uploadManager = uploader.NewUploadManager(&uploader.UploadManagerOptions{})
	}
	return &dynamicQiniuGeneratedImageObjectStore{
		settingsProvider: provider,
		uploader:         uploadManager,
	}
}

func (s *dynamicQiniuGeneratedImageObjectStore) Upload(ctx context.Context, upload service.GeneratedImageObjectUpload) (*service.GeneratedImageObject, error) {
	if s == nil || s.settingsProvider == nil {
		return nil, service.ErrGeneratedImageObjectStoreDisabled
	}
	settings, err := s.settingsProvider.GetGeneratedImageStorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || settings.Source != service.GeneratedImageStorageSourceQiniu {
		return nil, service.ErrGeneratedImageObjectStoreDisabled
	}
	return newQiniuGeneratedImageObjectStore(qiniuGeneratedImageObjectStoreOptions{
		AccessKey:     settings.QiniuAccessKey,
		SecretKey:     settings.QiniuSecretKey,
		Bucket:        settings.QiniuBucket,
		CDNDomain:     settings.QiniuCDNDomain,
		Prefix:        settings.QiniuPrefix,
		UseHTTPS:      settings.QiniuUseHTTPS,
		UploadTimeout: time.Duration(settings.QiniuUploadTimeoutSeconds) * time.Second,
		TokenTTL:      time.Duration(settings.QiniuTokenTTLSeconds) * time.Second,
		Uploader:      s.uploader,
	}).Upload(ctx, upload)
}

func newQiniuGeneratedImageObjectStore(options qiniuGeneratedImageObjectStoreOptions) *qiniuGeneratedImageObjectStore {
	store := &qiniuGeneratedImageObjectStore{
		accessKey:     strings.TrimSpace(options.AccessKey),
		secretKey:     strings.TrimSpace(options.SecretKey),
		bucket:        strings.TrimSpace(options.Bucket),
		cdnDomain:     strings.TrimRight(strings.TrimSpace(options.CDNDomain), "/"),
		prefix:        strings.Trim(strings.TrimSpace(options.Prefix), "/"),
		useHTTPS:      options.UseHTTPS,
		uploadTimeout: options.UploadTimeout,
		tokenTTL:      options.TokenTTL,
		uploader:      options.Uploader,
	}
	if store.uploadTimeout <= 0 {
		store.uploadTimeout = 30 * time.Second
	}
	if store.tokenTTL <= 0 {
		store.tokenTTL = time.Hour
	}
	if store.uploader == nil {
		store.uploader = uploader.NewUploadManager(&uploader.UploadManagerOptions{})
	}
	return store
}

func (s *qiniuGeneratedImageObjectStore) Upload(ctx context.Context, upload service.GeneratedImageObjectUpload) (*service.GeneratedImageObject, error) {
	if s == nil {
		return nil, fmt.Errorf("qiniu generated image object store is not configured")
	}
	data := append([]byte(nil), upload.Data...)
	if len(data) == 0 {
		return nil, fmt.Errorf("generated image data is empty")
	}
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	key := s.newObjectKey(contentType)
	policy, err := uptoken.NewPutPolicyWithKey(s.bucket, key, time.Now().Add(s.tokenTTL))
	if err != nil {
		return nil, fmt.Errorf("create qiniu upload policy: %w", err)
	}
	upToken := uptoken.NewSigner(policy, credentials.NewCredentials(s.accessKey, s.secretKey))
	objectOptions := &uploader.ObjectOptions{
		UpToken:     upToken,
		BucketName:  s.bucket,
		ObjectName:  &key,
		ContentType: contentType,
	}

	uploadCtx := ctx
	var cancel context.CancelFunc
	if s.uploadTimeout > 0 {
		uploadCtx, cancel = context.WithTimeout(ctx, s.uploadTimeout)
		defer cancel()
	}
	if err := s.uploader.UploadReader(uploadCtx, bytes.NewReader(data), objectOptions, nil); err != nil {
		return nil, fmt.Errorf("upload generated image to qiniu: %w", err)
	}
	return &service.GeneratedImageObject{
		Key: key,
		URL: s.cdnURL(key),
	}, nil
}

func (s *qiniuGeneratedImageObjectStore) newObjectKey(contentType string) string {
	name := uuid.NewString() + imageObjectExtension(contentType)
	datePath := time.Now().UTC().Format("2006/01/02")
	if s.prefix == "" {
		return datePath + "/" + name
	}
	return s.prefix + "/" + datePath + "/" + name
}

func (s *qiniuGeneratedImageObjectStore) cdnURL(key string) string {
	base := s.cdnDomain
	if !strings.Contains(base, "://") {
		scheme := "http"
		if s.useHTTPS {
			scheme = "https"
		}
		base = scheme + "://" + base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(key, "/")
}

func imageObjectExtension(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	extensions, err := mime.ExtensionsByType(contentType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}
