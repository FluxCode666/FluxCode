package chatgptweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSolveTurnstileToken_EmptyDx(t *testing.T) {
	result := SolveTurnstileToken("", "test-p")
	require.Empty(t, result, "empty dx should produce empty token")
}

func TestSolveTurnstileToken_InvalidBase64(t *testing.T) {
	result := SolveTurnstileToken("not-valid-base64!!!", "test-p")
	require.Empty(t, result, "invalid base64 dx should produce empty token")
}

func TestXorString_RoundTrip(t *testing.T) {
	original := "hello world"
	key := "secret"
	encrypted := xorString(original, key)
	decrypted := xorString(encrypted, key)
	require.Equal(t, original, decrypted)
}

func TestTurnstileToStr_Nil(t *testing.T) {
	require.Equal(t, "undefined", turnstileToStr(nil))
}

func TestTurnstileToStr_SpecialStrings(t *testing.T) {
	require.Equal(t, "[object Math]", turnstileToStr("window.Math"))
	require.Equal(t, "function random() { [native code] }", turnstileToStr("window.Math.random"))
}

func TestTurnstileToStr_Float(t *testing.T) {
	require.Equal(t, "3.14", turnstileToStr(3.14))
}

func TestTurnstileToStr_StringSlice(t *testing.T) {
	require.Equal(t, "a,b,c", turnstileToStr([]string{"a", "b", "c"}))
}
