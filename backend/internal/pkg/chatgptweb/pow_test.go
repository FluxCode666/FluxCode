package chatgptweb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPowConfig_ReturnsCorrectLength(t *testing.T) {
	cfg := BuildPowConfig("Mozilla/5.0 Test", []string{"https://chatgpt.com/backend-api/sentinel/sdk.js"}, "")
	require.Len(t, cfg, 18)
	require.Equal(t, "Mozilla/5.0 Test", cfg[4])
}

func TestBuildPowConfig_DefaultScript(t *testing.T) {
	cfg := BuildPowConfig("UA", nil, "")
	require.Len(t, cfg, 18)
	require.Equal(t, DefaultPowScript, cfg[5])
}

func TestPowGenerate_SolvesEasyDifficulty(t *testing.T) {
	cfg := BuildPowConfig("Mozilla/5.0 Test", nil, "")
	answer, solved := PowGenerate("test-seed", "0fffff", cfg, 100000)
	require.True(t, solved)
	require.NotEmpty(t, answer)
}

func TestPowGenerate_FailsImpossibleDifficulty(t *testing.T) {
	cfg := BuildPowConfig("Mozilla/5.0 Test", nil, "")
	_, solved := PowGenerate("test-seed", "000000", cfg, 10)
	require.False(t, solved)
}

func TestBuildProofToken_HasPrefix(t *testing.T) {
	token, err := BuildProofToken("test-seed", "0fffff", "UA", nil, "")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, "gAAAAAB"))
}

func TestBuildLegacyRequirementsToken_HasPrefix(t *testing.T) {
	token := BuildLegacyRequirementsToken("UA", nil, "")
	require.True(t, strings.HasPrefix(token, "gAAAAAC"))
}
