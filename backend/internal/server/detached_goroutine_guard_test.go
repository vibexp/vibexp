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
	parsed := map[string]*ast.File{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name) // #nosec G304 -- this package's own source files
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		parsed[name] = file
	}

	counts := map[string]int{}
	for _, key := range backgroundGoroutines(parsed) {
		counts[key]++
	}
	for key, n := range counts {
		if _, ok := detachedGoroutineAllowlist[key]; !ok {
			t.Errorf("%s starts a goroutine on context.Background(): detach with "+
				"context.WithoutCancel(r.Context()) so its audit events keep the request's attribution (#1548), "+
				"or add it to detachedGoroutineAllowlist with the reason it has none to keep", key)
			continue
		}
		// An entry covers ONE goroutine: a second one in the same function
		// must be reviewed, not inherit the exemption.
		require.Equal(t, 1, n, "%s starts %d detached goroutines; the allowlist covers one", key, n)
	}
	// A stale allowlist entry would quietly exempt whatever lands there next.
	for key := range detachedGoroutineAllowlist {
		require.Contains(t, counts, key, "allowlisted goroutine no longer exists: remove %s", key)
	}
}

// TestNoDetachedBackgroundGoroutines_Fires pins the scan: a go statement on
// an empty root context is reported whether the context appears in its
// function literal or arguments, in a local holding one (:= or var), or in
// the body of the package function or same-receiver method it starts; one
// on a context derived from a passed one is not.
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

func (s *Server) varLocal() {
	var detached = context.Background()
	go s.run(detached)
}

func (s *Server) viaMethod(teamID string) {
	go s.runDetached(teamID)
}

func (s *Server) runDetached(teamID string) {
	s.run(context.Background())
}

func viaFunc() {
	go runAlone()
}

func runAlone() {
	_ = context.Background()
}

func (s *Server) fine(ctx context.Context) {
	detached := context.WithoutCancel(ctx)
	go func() { s.run(detached) }()
	go s.runWith(detached)
}

func (s *Server) runWith(ctx context.Context) {
	s.run(ctx)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	require.NoError(t, err)

	got := backgroundGoroutines(map[string]*ast.File{"fixture.go": file})
	sort.Strings(got)
	require.Equal(t, []string{
		"fixture.go:argument", "fixture.go:literal", "fixture.go:local",
		"fixture.go:varLocal", "fixture.go:viaFunc", "fixture.go:viaMethod",
	}, got)
}

// backgroundGoroutines lists "<file>:<function>" once per go statement in
// files that starts a goroutine on an empty root context (context.Background
// or its placeholder counterpart): directly, through a local of the function
// assigned one, or inside the body of the package function or same-receiver
// method the statement starts (one call level deep, and through the
// receiver identifier itself: go s.helper(), not go rs.s.helper()).
// Goroutines started some other way (errgroup, time.AfterFunc, a runner's Go
// method) are not go statements and are not seen; none is used in this
// package.
func backgroundGoroutines(files map[string]*ast.File) []string {
	bodies := funcBodies(files)
	var out []string
	for name, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				for range detachedGoStmts(fn, bodies) {
					out = append(out, name+":"+fn.Name.Name)
				}
			}
		}
	}
	return out
}

// detachedGoStmts returns fn's go statements that start on an empty root
// context.
func detachedGoStmts(fn *ast.FuncDecl, bodies map[string]*ast.BlockStmt) []*ast.GoStmt {
	recv := receiverOf(fn)
	locals := rootContextLocals(fn.Body)
	var out []*ast.GoStmt
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		if usesRootContext(stmt, locals) || startsRootContextFunc(stmt.Call, recv, bodies) {
			out = append(out, stmt)
		}
		return false
	})
	return out
}

// receiverOf is a method's receiver: its variable name and type name.
func receiverOf(fn *ast.FuncDecl) (r struct{ name, typ string }) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return r
	}
	field := fn.Recv.List[0]
	if len(field.Names) > 0 {
		r.name = field.Names[0].Name
	}
	t := field.Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		r.typ = ident.Name
	}
	return r
}

// funcBodies maps "func:Name" and "Type.Name" to the declared bodies.
func funcBodies(files map[string]*ast.File) map[string]*ast.BlockStmt {
	out := map[string]*ast.BlockStmt{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := "func:" + fn.Name.Name
			if recv := receiverOf(fn); recv.typ != "" {
				key = recv.typ + "." + fn.Name.Name
			}
			out[key] = fn.Body
		}
	}
	return out
}

// startsRootContextFunc reports whether call starts a package function, or a
// method on the caller's own receiver, whose body uses an empty root context.
func startsRootContextFunc(
	call *ast.CallExpr, recv struct{ name, typ string }, bodies map[string]*ast.BlockStmt,
) bool {
	var body *ast.BlockStmt
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		body = bodies["func:"+fun.Name]
	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok && recv.name != "" && x.Name == recv.name {
			body = bodies[recv.typ+"."+fun.Sel.Name]
		}
	}
	return body != nil && usesRootContext(body, rootContextLocals(body))
}

// rootContextLocals names the variables body assigns an expression using an
// empty root context to, with := / = or a var declaration.
func rootContextLocals(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			if len(v.Rhs) == 1 && usesRootContext(v.Rhs[0], nil) {
				if ident, ok := v.Lhs[0].(*ast.Ident); ok {
					out[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			if len(v.Values) == 1 && usesRootContext(v.Values[0], nil) {
				out[v.Names[0].Name] = true
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
