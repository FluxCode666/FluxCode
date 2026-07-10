package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsIncludeGPT56Variants(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
}

func TestDefaultModelsIncludeBareGPT56Alias(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.6")
	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
}
