package chatgptweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePowResources_ExtractsScripts(t *testing.T) {
	h := `<html data-build="c/abc123/_"><head>
<script src="https://cdn.chatgpt.com/a.js"></script>
<script src="https://cdn.chatgpt.com/b.js"></script>
</head></html>`
	sources, dataBuild := ParsePowResources(h)
	require.Equal(t, []string{"https://cdn.chatgpt.com/a.js", "https://cdn.chatgpt.com/b.js"}, sources)
	require.Equal(t, "c/abc123/_", dataBuild)
}

func TestParsePowResources_EmptyHTML(t *testing.T) {
	sources, dataBuild := ParsePowResources("")
	require.Equal(t, []string{DefaultPowScript}, sources)
	require.Empty(t, dataBuild)
}

func TestParsePowResources_DataBuildFromScript(t *testing.T) {
	h := `<html><head><script src="https://cdn.chatgpt.com/c/xyz789/_/chunk.js"></script></head></html>`
	sources, dataBuild := ParsePowResources(h)
	require.Len(t, sources, 1)
	require.Equal(t, "c/xyz789/_", dataBuild)
}

func TestParsePowResources_NoScripts(t *testing.T) {
	h := `<html><head><style>body{}</style></head><body>Hello</body></html>`
	sources, dataBuild := ParsePowResources(h)
	require.Equal(t, []string{DefaultPowScript}, sources)
	require.Empty(t, dataBuild)
}
