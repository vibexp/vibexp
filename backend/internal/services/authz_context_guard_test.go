package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The authorization calls of a service must receive the caller's context
// (#1541): AuthorizationService records a denial in the audit log from it, and
// a context.Background() there records the denial as source=system with no IP,
// user agent, request id or API key. A caller with no request (a job, an
// import) passes context.Background() at ITS call site, never inside the
// service.

// authzMethods are the AuthorizationService methods that take a context.
var authzMethods = map[string]bool{"Can": true, "Authorize": true, "CanActOnResource": true, "Allows": true}

type authzContextViolation struct {
	file string
	line int
	call string
}

func TestNoBackgroundContextIntoAuthorization(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name) // #nosec G304 -- this package's own source files
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		for _, v := range authzContextViolations(fset, file) {
			t.Errorf("%s:%d: %s receives a background context.\n"+
				"Pass the caller's ctx so the denial is recorded with its channel and client info (#1541); "+
				"a caller with no request passes context.Background() at its own call site.", v.file, v.line, v.call)
		}
		checked++
	}
	require.Positive(t, checked, "no service files were scanned")
}

// TestNoBackgroundContextIntoAuthorization_Fires pins the scan itself: each
// shape below must be reported, and a passed-in ctx must not.
func TestNoBackgroundContextIntoAuthorization_Fires(t *testing.T) {
	const src = `package services

import "context"

func (s *svc) direct(userID string) error {
	return s.authz.Can(context.Background(), userID, "t", 0)
}

func (s *svc) local(userID string) error {
	ctx := context.Background()
	return s.authz.Can(ctx, userID, "t", 0)
}

func (s *svc) derived(userID string) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 0)
	defer cancel()
	_, err := s.authz.Authorize(ctx, userID, "t", 0)
	return err
}

func (s *svc) fine(ctx context.Context, userID string) error {
	return s.authz.CanActOnResource(ctx, userID, "t", "o", 0, 0)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	require.NoError(t, err)

	got := authzContextViolations(fset, file)

	lines := make([]int, 0, len(got))
	for _, v := range got {
		lines = append(lines, v.line)
	}
	require.Equal(t, []int{6, 11, 17}, lines)
}

// authzContextViolations reports every authorization call in file whose
// context argument is a background context: context.Background()/TODO()
// itself, a context derived from one, or a local variable assigned either in
// the same function.
func authzContextViolations(fset *token.FileSet, file *ast.File) []authzContextViolation {
	var out []authzContextViolation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		background := backgroundLocals(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			method, ok := authzMethodCall(call)
			if !ok {
				return true
			}
			arg := call.Args[0]
			ident, isIdent := arg.(*ast.Ident)
			if isBackgroundContext(arg) || (isIdent && background[ident.Name]) {
				out = append(out, authzContextViolation{
					file: fset.Position(call.Pos()).Filename, line: fset.Position(call.Pos()).Line, call: method,
				})
			}
			return true
		})
	}
	return out
}

// authzMethodCall reports whether call is <x>.<authz field>.<method>(…), the
// shape every service uses for its AuthorizationService, and names it.
func authzMethodCall(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !authzMethods[sel.Sel.Name] {
		return "", false
	}
	field, ok := sel.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	name := strings.ToLower(field.Sel.Name)
	if !strings.Contains(name, "authz") && !strings.Contains(name, "authorization") {
		return "", false
	}
	return field.Sel.Name + "." + sel.Sel.Name, true
}

// backgroundLocals names the variables body assigns a background context to.
func backgroundLocals(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || !isBackgroundContext(assign.Rhs[0]) || len(assign.Lhs) == 0 {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			out[ident.Name] = true
		}
		return true
	})
	return out
}

// isBackgroundContext reports whether expr is context.Background() or
// context.TODO(), or a context.With* call derived from one.
func isBackgroundContext(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "context" {
		return false
	}
	switch {
	case sel.Sel.Name == "Background" || sel.Sel.Name == "TODO":
		return true
	case strings.HasPrefix(sel.Sel.Name, "With") && len(call.Args) > 0:
		return isBackgroundContext(call.Args[0])
	}
	return false
}
