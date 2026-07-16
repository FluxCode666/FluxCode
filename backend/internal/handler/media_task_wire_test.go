package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
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

func TestMediaTaskWireProvidersJoinProductionGraph(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "wire.go", nil, 0)
	require.NoError(t, err)

	globalProviders, globalBinds := wireProviderSetContents(t, file, "ProviderSet")
	require.NotContains(t, globalProviders, "ProvideHandlers")
	require.Contains(t, globalProviders, "ProvideHandlersWithMedia")
	require.Contains(t, globalProviders, "MediaTaskProviderSet")
	require.Zero(t, globalBinds)

	mediaProviders, mediaBinds := wireProviderSetContents(t, file, "MediaTaskProviderSet")
	require.Contains(t, mediaProviders, "NewMediaTaskHandler")
	require.NotContains(t, mediaProviders, "ProvideHandlersWithMedia")
	require.Equal(t, 3, mediaBinds)
}

func wireProviderSetContents(t *testing.T, file *ast.File, variable string) (map[string]struct{}, int) {
	t.Helper()
	providers := make(map[string]struct{})
	binds := 0
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.VAR {
			continue
		}
		for _, spec := range generic.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != variable || len(value.Values) != 1 {
				continue
			}
			call, ok := value.Values[0].(*ast.CallExpr)
			require.True(t, ok, "%s must be initialized by wire.NewSet", variable)
			for _, argument := range call.Args {
				switch typed := argument.(type) {
				case *ast.Ident:
					providers[typed.Name] = struct{}{}
				case *ast.CallExpr:
					if selector, ok := typed.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Bind" {
						binds++
					}
				}
			}
			return providers, binds
		}
	}
	t.Fatalf("wire provider set %s not found", variable)
	return nil, 0
}
