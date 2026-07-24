package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This guardrail protects the one invariant that makes user suspension (#454)
// actually enforceable:
//
//	authenticatedContext — the sole constructor of an authenticated request
//	context — must be reachable ONLY through authenticateUser, which performs the
//	suspension check first.
//
// Suspension is only as strong as its weakest authentication path. A new auth
// path that calls authenticatedContext directly would silently authenticate
// suspended accounts, and nothing about that diff would look wrong. This test
// makes that mistake a build failure instead of a security hole.
//
// NON-GOALS — this is a lexical call-graph gate, not a semantic one:
//   - It cannot prove authenticateUser's check is CORRECT, only that no path
//     bypasses it.
//   - A path that never builds an authenticated context at all (and instead
//     writes contextkeys.UserID by hand) is invisible here; TestAuthContextKeys
//     AreNotSetOutsideAuthenticatedContext below covers that.
//   - Test files are exempt: they legitimately construct contexts directly.
const (
	authContextConstructor = "authenticatedContext"
	authContextChokepoint  = "authenticateUser"
)

// parseServerPackage parses every non-test .go file of this package.
func parseServerPackage(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()

	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoErrorf(t, parseErr, "parsing %s", path)
		files = append(files, file)
	}
	require.NotEmpty(t, files, "no package sources found — the glob is wrong")
	return fset, files
}

// enclosingFunc returns the name of the function declaration containing pos.
func enclosingFunc(file *ast.File, pos token.Pos) string {
	name := "<file scope>"
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			name = fn.Name.Name
		}
	}
	return name
}

// TestAuthenticatedContextIsOnlyReachedThroughAuthenticateUser fails when any
// production code outside authenticateUser calls authenticatedContext — i.e.
// when an authentication path can mint an authenticated context without first
// clearing the suspension check.
func TestAuthenticatedContextIsOnlyReachedThroughAuthenticateUser(t *testing.T) {
	fset, files := parseServerPackage(t)

	callers := make(map[string]int)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != authContextConstructor {
				return true
			}
			caller := enclosingFunc(file, call.Pos())
			callers[caller]++
			if caller != authContextChokepoint {
				t.Errorf(
					"%s: %s() calls %s() directly, bypassing the suspension check.\n"+
						"Every authentication path must go through s.%s(ctx, userID, authType, extra), "+
						"which rejects suspended accounts before building the context.",
					fset.Position(call.Pos()), caller, authContextConstructor, authContextChokepoint,
				)
			}
			return true
		})
	}

	// A zero count would mean the constructor was renamed and this guardrail
	// silently stopped guarding anything.
	require.NotEmpty(t, callers,
		"no calls to %s found — was it renamed? This guardrail must be updated with it",
		authContextConstructor)
	assert.Equal(t, 1, callers[authContextChokepoint],
		"%s should call %s exactly once", authContextChokepoint, authContextConstructor)
}

// TestAuthContextKeysAreNotSetOutsideAuthenticatedContext closes the other half
// of the hole: a path could skip authenticatedContext entirely and write the
// user-id context key itself. Only authenticatedContext may do that.
func TestAuthContextKeysAreNotSetOutsideAuthenticatedContext(t *testing.T) {
	fset, files := parseServerPackage(t)

	found := 0
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WithValue" {
				return true
			}
			// Match context.WithValue(ctx, contextkeys.UserID, ...)
			key, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok || key.Sel.Name != "UserID" {
				return true
			}
			pkg, ok := key.X.(*ast.Ident)
			if !ok || pkg.Name != "contextkeys" {
				return true
			}

			found++
			if caller := enclosingFunc(file, call.Pos()); caller != authContextConstructor {
				t.Errorf(
					"%s: %s() sets contextkeys.UserID directly, bypassing the suspension check.\n"+
						"Build the authenticated context via s.%s() instead.",
					fset.Position(call.Pos()), caller, authContextChokepoint,
				)
			}
			return true
		})
	}

	require.Equal(t, 1, found,
		"expected exactly one contextkeys.UserID assignment (inside %s)", authContextConstructor)
}
