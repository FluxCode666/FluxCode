package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
)

const mediaObjectSniffBytes int64 = 512

type validatedMediaObjectStream struct {
	ctx          context.Context
	source       io.Reader
	maximum      int64
	expected     int64
	read         int64
	hasher       hash.Hash
	wantChecksum string
	complete     bool
	terminalErr  error
}

func newValidatedMediaObjectStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
	maxBytes int64,
) (*validatedMediaObjectStream, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if body == nil || (input.MediaType != MediaTypeImage && input.MediaType != MediaTypeVideo) {
		return nil, "", ErrInvalidMediaInput
	}
	if maxBytes <= 0 || input.SizeBytes < 0 {
		return nil, "", ErrInvalidMediaInput
	}
	if input.SizeBytes > maxBytes {
		return nil, "", ErrMediaContentTooLarge
	}
	contentType := normalizeStoredMediaContentType(input.ContentType, input.MediaType)
	if contentType == "application/octet-stream" {
		return nil, "", ErrInvalidMediaInput
	}

	probeBytes := mediaObjectSniffBytes
	if maxBytes < probeBytes {
		probeBytes = maxBytes + 1
	}
	prefix := make([]byte, int(probeBytes))
	n, err := io.ReadFull(&mediaContextReader{ctx: ctx, reader: body}, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, "", err
	}
	prefix = prefix[:n]
	if len(prefix) == 0 {
		return nil, "", ErrInvalidMediaInput
	}
	if int64(len(prefix)) > maxBytes {
		return nil, "", ErrMediaContentTooLarge
	}
	if input.SizeBytes > 0 && int64(len(prefix)) > input.SizeBytes {
		return nil, "", fmt.Errorf("%w: media object exceeds declared size", ErrMediaStorageIntegrity)
	}
	detected := detectMediaObjectContentType(prefix, input.MediaType, contentType)
	if detected == "application/octet-stream" || detected != contentType {
		return nil, "", ErrInvalidMediaInput
	}

	expected := int64(-1)
	if input.SizeBytes > 0 {
		expected = input.SizeBytes
	}
	stream := &validatedMediaObjectStream{
		ctx: ctx, source: io.MultiReader(bytes.NewReader(prefix), body),
		maximum: maxBytes, expected: expected, hasher: sha256.New(),
		wantChecksum: strings.ToLower(strings.TrimSpace(input.ChecksumSHA256)),
	}
	return stream, detected, nil
}

func (r *validatedMediaObjectStream) Read(buffer []byte) (int, error) {
	if r == nil || r.source == nil {
		return 0, ErrMediaContentUnavailable
	}
	if r.terminalErr != nil {
		return 0, r.terminalErr
	}
	if r.complete {
		return 0, io.EOF
	}
	if err := r.ctx.Err(); err != nil {
		r.terminalErr = err
		return 0, err
	}
	if len(buffer) == 0 {
		return 0, nil
	}

	limit := r.maximum
	if r.expected >= 0 && r.expected < limit {
		limit = r.expected
	}
	if r.read >= limit {
		return r.probeEOF()
	}
	remaining := limit - r.read
	if int64(len(buffer)) > remaining {
		buffer = buffer[:remaining]
	}
	n, err := r.source.Read(buffer)
	if n > 0 {
		r.read += int64(n)
		_, _ = r.hasher.Write(buffer[:n])
	}
	if errors.Is(err, io.EOF) {
		verificationErr := r.verifyEOF()
		if verificationErr != nil {
			return n, verificationErr
		}
		return n, io.EOF
	}
	if err != nil {
		r.terminalErr = err
	}
	return n, err
}

func (r *validatedMediaObjectStream) probeEOF() (int, error) {
	var probe [1]byte
	n, err := r.source.Read(probe[:])
	if n > 0 {
		if r.expected >= 0 {
			r.terminalErr = fmt.Errorf("%w: media object exceeds declared size", ErrMediaStorageIntegrity)
		} else {
			r.terminalErr = ErrMediaContentTooLarge
		}
		return 0, r.terminalErr
	}
	if errors.Is(err, io.EOF) {
		if verificationErr := r.verifyEOF(); verificationErr != nil {
			return 0, verificationErr
		}
		return 0, io.EOF
	}
	if err != nil {
		r.terminalErr = err
	}
	return 0, err
}

func (r *validatedMediaObjectStream) verifyEOF() error {
	if r.read <= 0 {
		r.terminalErr = ErrInvalidMediaInput
		return r.terminalErr
	}
	if r.expected >= 0 && r.read != r.expected {
		r.terminalErr = fmt.Errorf("%w: media object size differs from declared size", ErrMediaStorageIntegrity)
		return r.terminalErr
	}
	checksum := hex.EncodeToString(r.hasher.Sum(nil))
	if r.wantChecksum != "" && !strings.EqualFold(checksum, r.wantChecksum) {
		r.terminalErr = fmt.Errorf("%w: media object checksum differs from declared checksum", ErrMediaStorageIntegrity)
		return r.terminalErr
	}
	r.complete = true
	return nil
}

func (r *validatedMediaObjectStream) Finalize() (int64, string, error) {
	if r == nil {
		return 0, "", ErrMediaContentUnavailable
	}
	if r.terminalErr != nil {
		return 0, "", r.terminalErr
	}
	if !r.complete {
		if r.expected >= 0 && r.read < r.expected {
			return 0, "", fmt.Errorf("%w: media storage stopped before consuming declared content", ErrMediaStorageIntegrity)
		}
		var probe [1]byte
		n, err := r.probeEOFForFinalize(probe[:])
		if n > 0 {
			return 0, "", fmt.Errorf("%w: media storage stopped before consuming the complete body", ErrMediaStorageIntegrity)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, "", err
		}
	}
	if r.terminalErr != nil {
		return 0, "", r.terminalErr
	}
	if !r.complete {
		return 0, "", ErrMediaStorageIntegrity
	}
	return r.read, hex.EncodeToString(r.hasher.Sum(nil)), nil
}

func (r *validatedMediaObjectStream) probeEOFForFinalize(buffer []byte) (int, error) {
	if r.read >= r.maximum || (r.expected >= 0 && r.read >= r.expected) {
		return r.probeEOF()
	}
	if err := r.ctx.Err(); err != nil {
		r.terminalErr = err
		return 0, err
	}
	n, err := r.source.Read(buffer)
	if n > 0 {
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		if verificationErr := r.verifyEOF(); verificationErr != nil {
			return 0, verificationErr
		}
		return 0, io.EOF
	}
	if err != nil {
		r.terminalErr = err
	}
	return 0, err
}

type mediaContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *mediaContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
