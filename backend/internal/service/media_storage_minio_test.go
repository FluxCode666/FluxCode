package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

type mediaS3ClientStub struct {
	objects   map[string][]byte
	put       *s3.PutObjectInput
	get       *s3.GetObjectInput
	getResult *s3.GetObjectOutput
	deleted   string
	putErr    error
	deleteErr error
}

func (s *mediaS3ClientStub) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	s.put = input
	if s.putErr != nil {
		return nil, s.putErr
	}
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[aws.ToString(input.Key)] = data
	return &s3.PutObjectOutput{}, nil
}

func (s *mediaS3ClientStub) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	s.get = input
	if s.getResult != nil {
		return s.getResult, nil
	}
	data := s.objects[aws.ToString(input.Key)]
	length := int64(len(data))
	result := &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)), ContentLength: &length,
		ContentType: aws.String("image/png"), AcceptRanges: aws.String("bytes"),
	}
	if input.Range != nil {
		result.ContentRange = aws.String("bytes 0-0/" + string(rune('0'+len(data))))
	}
	return result, nil
}

func (s *mediaS3ClientStub) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	s.deleted = aws.ToString(input.Key)
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	delete(s.objects, s.deleted)
	return &s3.DeleteObjectOutput{}, nil
}

func (s *mediaS3ClientStub) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func testMediaPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	return data
}

func TestMinIOMediaArtifactStorePutOpenAndDiscard(t *testing.T) {
	client := &mediaS3ClientStub{objects: map[string][]byte{}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20)
	require.NoError(t, err)
	png := testMediaPNG(t)

	artifact, err := store.Put(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeImage, ContentType: "image/png", Data: png,
	})
	require.NoError(t, err)
	require.Equal(t, MediaStorageProviderMinIO, artifact.StorageProvider)
	require.Equal(t, int64(len(png)), artifact.SizeBytes)
	require.NotEmpty(t, artifact.ChecksumSHA256)
	require.Contains(t, artifact.ObjectKey, "/output/")
	require.Equal(t, "image/png", aws.ToString(client.put.ContentType))

	content, err := store.Open(context.Background(), artifact, "bytes=0-0")
	require.NoError(t, err)
	require.Equal(t, 206, content.StatusCode)
	require.Equal(t, "bytes=0-0", aws.ToString(client.get.Range))
	require.NoError(t, content.Body.Close())

	input := mediaArtifactInputFromStored(artifact)
	require.NoError(t, store.Discard(context.Background(), input))
	require.Equal(t, artifact.ObjectKey, client.deleted)
}

func TestMinIOMediaArtifactStoreRejectsMismatchedContent(t *testing.T) {
	store, err := newMinIOMediaArtifactObjectStoreWithClient(&mediaS3ClientStub{}, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20)
	require.NoError(t, err)

	_, err = store.Put(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeImage, ContentType: "image/jpeg", Data: testMediaPNG(t),
	})
	require.ErrorIs(t, err, ErrInvalidMediaInput)
}

func TestMinIOMediaArtifactStoreHealthCheckWritesReadsAndDeletes(t *testing.T) {
	client := &mediaS3ClientStub{objects: map[string][]byte{}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20)
	require.NoError(t, err)

	require.NoError(t, store.Check(context.Background()))
	require.Contains(t, client.deleted, "/health/")
	require.Empty(t, client.objects)
}

func TestMinIOMediaArtifactStoreRejectsStoredSizeMismatch(t *testing.T) {
	png := testMediaPNG(t)
	wrongLength := int64(len(png) - 1)
	client := &mediaS3ClientStub{getResult: &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(png)), ContentLength: &wrongLength,
		ContentType: aws.String("image/png"),
	}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20)
	require.NoError(t, err)

	_, err = store.Open(context.Background(), &MediaArtifact{
		StorageProvider: MediaStorageProviderMinIO, ObjectKey: "media/output/test.png",
		MediaType: MediaTypeImage, ContentType: "image/png", SizeBytes: int64(len(png)),
	}, "")
	require.ErrorIs(t, err, ErrMediaStorageIntegrity)
}

func TestMinIOMediaArtifactStoreBoundsUnknownLengthAndVerifiesChecksum(t *testing.T) {
	png := testMediaPNG(t)
	unknownLength := int64(0)
	client := &mediaS3ClientStub{getResult: &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(png)), ContentLength: &unknownLength,
		ContentType: aws.String("image/png"),
	}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20)
	require.NoError(t, err)

	content, err := store.Open(context.Background(), &MediaArtifact{
		StorageProvider: MediaStorageProviderMinIO, ObjectKey: "media/output/test.png",
		MediaType: MediaTypeImage, ContentType: "image/png", SizeBytes: int64(len(png)),
		ChecksumSHA256: "not-the-real-checksum",
	}, "")
	require.NoError(t, err)
	_, err = io.ReadAll(content.Body)
	require.ErrorIs(t, err, ErrMediaStorageIntegrity)
	require.NoError(t, content.Body.Close())
}

func TestVerifiedMediaReadCloserRejectsUnknownOversizedBody(t *testing.T) {
	reader := newVerifiedMediaReadCloser(io.NopCloser(bytes.NewReader([]byte("12345"))), 4, -1, "")
	_, err := io.ReadAll(reader)
	require.ErrorIs(t, err, ErrMediaContentTooLarge)
}

func TestMinIOMediaArtifactStoreStreamsObjects(t *testing.T) {
	client := &mediaS3ClientStub{objects: map[string][]byte{}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 4<<20,
	)
	require.NoError(t, err)
	video := testMediaMP4(2 << 20)

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: int64(len(video)),
	}, bytes.NewReader(video))
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, int64(len(video)), artifact.SizeBytes)
	require.NotEmpty(t, artifact.ChecksumSHA256)
	require.Equal(t, int64(len(video)), aws.ToInt64(client.put.ContentLength))
	require.Equal(t, video, client.objects[artifact.ObjectKey])
}

func TestMinIOMediaArtifactStoreCleansUpFailedStreamWrites(t *testing.T) {
	putErr := errors.New("MinIO stream upload failed")
	client := &mediaS3ClientStub{objects: map[string][]byte{}, putErr: putErr}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20,
	)
	require.NoError(t, err)

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
	}, bytes.NewReader(testMediaMP4(2048)))
	require.Nil(t, artifact)
	require.ErrorIs(t, err, putErr)
	require.NotEmpty(t, client.deleted)
}

func TestMinIOMediaArtifactStoreCleansUpChecksumMismatch(t *testing.T) {
	client := &mediaS3ClientStub{objects: map[string][]byte{}}
	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		client, MediaMinIOConfig{Bucket: "generated", Prefix: "media"}, 1<<20,
	)
	require.NoError(t, err)

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4", ChecksumSHA256: "incorrect",
	}, bytes.NewReader(testMediaMP4(2048)))
	require.Nil(t, artifact)
	require.ErrorIs(t, err, ErrMediaStorageIntegrity)
	require.NotEmpty(t, client.deleted)
	require.Empty(t, client.objects)
}

type mediaS3EOFTrackingReader struct {
	reader     *bytes.Reader
	read       int64
	reachedEOF bool
}

func (r *mediaS3EOFTrackingReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.read += int64(n)
	if errors.Is(err, io.EOF) {
		r.reachedEOF = true
	}
	return n, err
}

func newMediaS3HTTPTestClient(t *testing.T, endpoint string) *s3.Client {
	t.Helper()
	config, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test-access", "test-secret", "")),
	)
	require.NoError(t, err)
	return s3.NewFromConfig(config, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
		options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.Retryer = aws.NopRetryer{}
	})
}

func TestMinIOMediaArtifactStoreUnknownLengthStreamUsesRealS3Client(t *testing.T) {
	type observation struct {
		method           string
		contentLength    int64
		transferEncoding []string
		body             []byte
		readErr          error
	}
	observed := make(chan observation, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		observed <- observation{
			method: request.Method, contentLength: request.ContentLength,
			transferEncoding: request.TransferEncoding, body: body, readErr: readErr,
		}
		writer.Header().Set("ETag", `"test-etag"`)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		newMediaS3HTTPTestClient(t, server.URL),
		MediaMinIOConfig{Bucket: "generated", Prefix: "media"},
		4<<20,
	)
	require.NoError(t, err)
	video := testMediaMP4(2 << 20)
	source := &mediaS3EOFTrackingReader{reader: bytes.NewReader(video)}

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
	}, source)
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, int64(len(video)), artifact.SizeBytes)
	require.NotEmpty(t, artifact.ChecksumSHA256)
	require.Equal(t, int64(len(video)), source.read)
	require.True(t, source.reachedEOF)

	require.Len(t, observed, 1)
	request := <-observed
	require.NoError(t, request.readErr)
	require.Equal(t, http.MethodPut, request.method)
	require.Equal(t, int64(-1), request.contentLength)
	require.Contains(t, request.transferEncoding, "chunked")
	require.Equal(t, video, request.body)
}

func TestMinIOMediaArtifactStoreRealS3ClientCleansUpServerError(t *testing.T) {
	putBody := make(chan []byte, 1)
	deletePath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(request.Body)
			putBody <- body
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(writer, `<Error><Code>InternalError</Code><Message>test failure</Message></Error>`)
		case http.MethodDelete:
			deletePath <- request.URL.Path
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		newMediaS3HTTPTestClient(t, server.URL),
		MediaMinIOConfig{Bucket: "generated", Prefix: "media"},
		1<<20,
	)
	require.NoError(t, err)
	video := testMediaMP4(2048)

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
	}, bytes.NewReader(video))
	require.Nil(t, artifact)
	require.Error(t, err)
	require.Len(t, putBody, 1)
	require.Equal(t, video, <-putBody)
	require.Len(t, deletePath, 1)
	require.Contains(t, <-deletePath, "/generated/media/")
}

func TestMinIOMediaArtifactStoreRealS3ClientCleansUpCanceledUpload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	deletePath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPut:
			cancel()
			panic(http.ErrAbortHandler)
		case http.MethodDelete:
			deletePath <- request.URL.Path
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	store, err := newMinIOMediaArtifactObjectStoreWithClient(
		newMediaS3HTTPTestClient(t, server.URL),
		MediaMinIOConfig{Bucket: "generated", Prefix: "media"},
		1<<20,
	)
	require.NoError(t, err)

	artifact, err := store.PutStream(ctx, MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
	}, bytes.NewReader(testMediaMP4(2048)))
	require.Nil(t, artifact)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, deletePath, 1)
	require.Contains(t, <-deletePath, "/generated/media/")
}
