package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A goroutine the server starts on an empty root context loses the request's
// values, so any audit event inside it is recorded as source=system with no
// client info (#1548). Work that outlives a request detaches with
// context.WithoutCancel(r.Context()) instead. internal/services' guard
// (TestNoBackgroundContextIntoAuthorization) cannot see this: the context is
// created here, in the server, before any service is called.
//
// detachedGoroutineAllowlist holds the goroutines that legitimately start on
// an empty root context, keyed by "<file>:<enclosing function>", with the
// reason. Add an entry only for work that has no request behind it, or that
// records no audit event and authorizes nothing.
var detachedGoroutineAllowlist = map[string]string{
	"server.go:Start": "graceful shutdown: runs after the server context is done; " +
		"there is no request to keep",
	"embedding_provider_handlers.go:enqueueTeamReembed": "the team re-embed backfill authorizes nothing and " +
		"records no audit event, so there is no attribution to lose",
}

func TestNoDetachedBackgroundGoroutines(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var found []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name) // #nosec G304 -- this package's own source files
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		found = append(found, backgroundGoroutines(name, file)...)
	}

	for _, key := range found {
		if _, ok := detachedGoroutineAllowlist[key]; !ok {
			t.Errorf("%s starts a goroutine on context.Background(): detach with "+
				"context.WithoutCancel(r.Context()) so its audit events keep the request's attribution (#1548), "+
				"or add it to detachedGoroutineAllowlist with the reason it has none to keep", key)
		}
	}
	// A stale allowlist entry would quietly exempt whatever lands there next.
	for key := range detachedGoroutineAllowlist {
		require.Contains(t, found, key, "allowlisted goroutine no longer exists: remove %s", key)
	}
}

// TestNoDetachedBackgroundGoroutines_Fires pins the scan: a go statement with
// an empty root context inside its function literal or its arguments, or a
// local holding one, is reported, and one using a passed context is not.
func TestNoDetachedBackgroundGoroutines_Fires(t *testing.T) {
	const src = `package server

import "context"

func (s *Server) literal() {
	go func() { s.run(context.Background()) }()
}

func (s *Server) argument() {
	go s.run(context.TODO())
}

func (s *Server) local() {
	detached := context.Background()
	go func() { s.run(detached) }()
}

func (s *Server) fine(ctx context.Context) {
	detached := context.WithoutCancel(ctx)
	go func() { s.run(detached) }()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	require.NoError(t, err)

	got := backgroundGoroutines("fixture.go", file)
	sort.Strings(got)
	require.Equal(t, []string{"fixture.go:argument", "fixture.go:literal", "fixture.go:local"}, got)
}

// backgroundGoroutines lists "<file>:<function>" for every function in file
// that starts a goroutine on an empty root context (context.Background or its
// placeholder counterpart), directly or through a local of the function
// assigned one.
func backgroundGoroutines(name string, file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := rootContextLocals(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			stmt, ok := n.(*ast.GoStmt)
			if ok && usesRootContext(stmt, locals) {
				out = append(out, name+":"+fn.Name.Name)
				return false
			}
			return true
		})
	}
	return out
}

// rootContextLocals names the variables body assigns an expression using an
// empty root context to.
func rootContextLocals(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 && usesRootContext(assign.Rhs[0], nil) {
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
				out[ident.Name] = true
			}
		}
		return true
	})
	return out
}

// usesRootContext reports whether node mentions an empty root context
// (context.Background or its placeholder counterpart), or one of locals.
func usesRootContext(node ast.Node, locals map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			found = found || locals[v.Name]
		case *ast.SelectorExpr:
			if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "context" &&
				(v.Sel.Name == "Background" || v.Sel.Name == "TODO") {
				found = true
			}
		}
		return !found
	})
	return found
}
