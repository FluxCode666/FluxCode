package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvideHandlersWithMediaAssignsMediaTaskHandler(t *testing.T) {
	media := &MediaTaskHandler{}
	handlers := ProvideHandlersWithMedia(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, media, nil, nil,
	)
	require.NotNil(t, handlers)
	require.Same(t, media, handlers.MediaTask)
}
