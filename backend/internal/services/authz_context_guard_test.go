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
	var parsed []*ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name) // #nosec G304 -- this package's own source files
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		parsed = append(parsed, file)
	}
	require.NotEmpty(t, parsed, "no service files were scanned")

	for _, v := range authzContextViolations(fset, parsed) {
		t.Errorf("%s:%d: %s receives a background context.\n"+
			"Pass the caller's ctx so the denial is recorded with its channel and client info (#1541); "+
			"a caller with no request passes context.Background() at its own call site.", v.file, v.line, v.call)
	}
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

func (s *svc) viaHelper(userID string) error {
	return s.check(context.Background(), userID)
}

func (s *svc) viaTwoHelpers(userID string) error {
	var ctx = context.Background()
	return s.outer(ctx, userID)
}

func (s *svc) outer(ctx context.Context, userID string) error {
	return s.check(ctx, userID)
}

func (s *svc) check(ctx context.Context, userID string) error {
	return s.authz.CanActOnResource(ctx, userID, "t", "o", 0, 0)
}

func bare(authorizer AuthorizationServiceInterface, userID string) error {
	return authorizer.Can(context.Background(), userID, "t", 0)
}

func (s *svc) fine(ctx context.Context, userID string) error {
	bgCtx := context.Background()
	_ = s.repo.Get(bgCtx, userID)
	return s.check(ctx, userID)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	require.NoError(t, err)

	got := authzContextViolations(fset, []*ast.File{file})

	lines := make([]int, 0, len(got))
	for _, v := range got {
		lines = append(lines, v.line)
	}
	require.Equal(t, []int{6, 11, 17, 22, 27, 39}, lines)
}

// authzContextViolations reports every call that hands a background context
// to an authorization check: an AuthorizationService method called on an
// authz field or an AuthorizationService value, or a function of the package
// that forwards its context parameter to one (directly or through another
// such function). A background context is context.Background()/TODO(), a
// context derived from one, or a local assigned either in the same function.
func authzContextViolations(fset *token.FileSet, files []*ast.File) []authzContextViolation {
	forwarders := authzForwarders(files)
	var out []authzContextViolation
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			background := backgroundLocals(fn.Body)
			scope := callScope{recv: receiverOf(fn), authorizers: authorizerNames(fn), forwarders: forwarders}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				name, ok := scope.authzCall(call)
				if !ok {
					return true
				}
				arg := call.Args[0]
				ident, isIdent := arg.(*ast.Ident)
				if isBackgroundContext(arg) || (isIdent && background[ident.Name]) {
					pos := fset.Position(call.Pos())
					out = append(out, authzContextViolation{file: pos.Filename, line: pos.Line, call: name})
				}
				return true
			})
		}
	}
	return out
}

// receiver is a method's receiver: its variable name and type name.
type receiver struct{ name, typ string }

func receiverOf(fn *ast.FuncDecl) receiver {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return receiver{}
	}
	field := fn.Recv.List[0]
	r := receiver{typ: typeName(field.Type)}
	if len(field.Names) > 0 {
		r.name = field.Names[0].Name
	}
	return r
}

// forwarderKey identifies a function ("func:Name") or a method
// ("Type.Name"), so methods of different types never share an entry.
func forwarderKey(recvType, name string) string {
	if recvType == "" {
		return "func:" + name
	}
	return recvType + "." + name
}

// authzForwarders names the functions and methods that pass their first
// (context) parameter on to an authorization check, directly or through
// another forwarder: a background context handed to one reaches the check.
// A method's calls are followed through its own receiver only (s.helper),
// which is how a service reaches its private helpers.
func authzForwarders(files []*ast.File) map[string]bool {
	forwarders := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				recv := receiverOf(fn)
				key := forwarderKey(recv.typ, fn.Name.Name)
				param := firstContextParam(fn)
				if forwarders[key] || param == "" {
					continue
				}
				scope := callScope{recv: recv, authorizers: authorizerNames(fn), forwarders: forwarders}
				if scope.forwardsContext(fn.Body, param) {
					forwarders[key] = true
					changed = true
				}
			}
		}
	}
	return forwarders
}

// forwardsContext reports whether body passes param as the context of an
// authorization call.
func (c callScope) forwardsContext(body *ast.BlockStmt, param string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || found {
			return !found
		}
		if _, ok := c.authzCall(call); !ok {
			return true
		}
		if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == param {
			found = true
		}
		return !found
	})
	return found
}

// firstContextParam is the name of fn's first parameter when it is a
// context.Context, "" otherwise.
func firstContextParam(fn *ast.FuncDecl) string {
	params := fn.Type.Params
	if params == nil || len(params.List) == 0 || len(params.List[0].Names) == 0 {
		return ""
	}
	sel, ok := params.List[0].Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Context" {
		return ""
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "context" {
		return ""
	}
	return params.List[0].Names[0].Name
}

// authorizerNames names fn's parameters typed as an AuthorizationService.
func authorizerNames(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		if strings.Contains(typeName(field.Type), "AuthorizationService") {
			for _, name := range field.Names {
				out[name.Name] = true
			}
		}
	}
	return out
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

// callScope is what decides, inside one function, whether a call reaches an
// authorization check.
type callScope struct {
	recv        receiver
	authorizers map[string]bool
	forwarders  map[string]bool
}

// authzCall reports whether call is an authorization check and names it:
// <x>.<authz field>.<method>(…), <authorizer>.<method>(…) on an
// AuthorizationService parameter, or a call to a forwarder (a package
// function, or a method on the caller's own receiver).
func (c callScope) authzCall(call *ast.CallExpr) (string, bool) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name, c.forwarders[forwarderKey("", fun.Name)]
	case *ast.SelectorExpr:
		if recv, ok := fun.X.(*ast.Ident); ok && c.recv.name != "" && recv.Name == c.recv.name &&
			c.forwarders[forwarderKey(c.recv.typ, fun.Sel.Name)] {
			return recv.Name + "." + fun.Sel.Name, true
		}
		if !authzMethods[fun.Sel.Name] {
			return "", false
		}
		switch recv := fun.X.(type) {
		case *ast.Ident:
			return recv.Name + "." + fun.Sel.Name, c.authorizers[recv.Name]
		case *ast.SelectorExpr:
			name := strings.ToLower(recv.Sel.Name)
			if strings.Contains(name, "authz") || strings.Contains(name, "authorization") {
				return recv.Sel.Name + "." + fun.Sel.Name, true
			}
		}
	}
	return "", false
}

// backgroundLocals names the variables body assigns a background context to,
// with := / = or a var declaration.
func backgroundLocals(body *ast.BlockStmt) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Rhs) == 1 && isBackgroundContext(node.Rhs[0]) && len(node.Lhs) > 0 {
				if ident, ok := node.Lhs[0].(*ast.Ident); ok {
					out[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			if len(node.Values) == 1 && isBackgroundContext(node.Values[0]) && len(node.Names) > 0 {
				out[node.Names[0].Name] = true
			}
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
