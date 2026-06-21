// Package specconformance is the payload-drift detection layer (#1714,
// epic #1693): it validates recorded handler-test responses against the
// OpenAPI spec (backend-api/openapi.yaml) using pb33f/libopenapi-validator,
// and tracks which documented operations have at least one spec-validated
// response so the coverage ledger in internal/server can enforce shrink-only
// burn-down.
//
// It lives in its own package (not internal/testutils, which imports
// internal/server) so that internal/server tests can import it without an
// import cycle.
package specconformance

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
	verrors "github.com/pb33f/libopenapi-validator/errors"
	"github.com/pb33f/libopenapi-validator/paths"
	"github.com/pb33f/libopenapi/datamodel"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// loadedSpec holds the parsed spec model and the validator built from it,
// shared by every assertion in the test binary.
type loadedSpec struct {
	validator validator.Validator
	model     *v3.Document
	findOpts  *config.ValidationOptions
}

var (
	loadOnce sync.Once
	spec     *loadedSpec
	loadErr  error
)

// validateMu serializes access to the shared *loadedSpec returned by load().
// The pb33f/libopenapi validator mutates the shared *v3.Document while
// rendering schemas during validation (MarshalYAMLInlineWithContext writes
// into SchemaProxy/DynamicValue/Schema nodes), so concurrent FindPath /
// ValidateHttpResponse / model iteration on the singleton would race.
var validateMu sync.Mutex

// knownDivergence is a wire-level spec↔handler mismatch that is already
// tracked in a GitHub issue. Matching responses are skipped (logged, not
// failed) so the suite stays green while the divergence is being fixed.
// Entries are removed when the linked issue closes — code review owns this
// list, exactly like the drift-gate allowlist.
type knownDivergence struct {
	reason string
	match  func(status int, contentType string, errs []*verrors.ValidationError) bool
}

var knownDivergences = []knownDivergence{}

// coverage records, per test binary, the documented operations that received
// at least one spec-validated (conforming) response.
var coverage = struct {
	mu  sync.Mutex
	ops map[string]struct{}
}{ops: make(map[string]struct{})}

// AssertConformsToSpec validates the recorded response against the OpenAPI
// spec for the operation matching the request: the status code must be
// documented, the content type must match, and the body must satisfy the
// response schema. A conforming response marks the operation as covered for
// the payload-coverage ledger; a known divergence (see knownDivergences) is
// logged and skipped instead of failed.
func AssertConformsToSpec(t testing.TB, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	op, skipReason, errs, err := validateRecordedResponse(req, rec)
	if err != nil {
		t.Fatalf("specconformance: %v", err)
	}
	if skipReason != "" {
		t.Logf("specconformance: %s (status %d) skipped — known divergence: %s", op, rec.Code, skipReason)
		return
	}
	if len(errs) > 0 {
		t.Errorf("response does not conform to openapi.yaml for %s (status %d):\n%s",
			op, rec.Code, formatValidationErrors(errs))
		return
	}
	recordCovered(op)
}

// validateRecordedResponse is the testing-free core of AssertConformsToSpec,
// split out so the package can test its own failure detection without
// failing the calling test.
func validateRecordedResponse(
	req *http.Request, rec *httptest.ResponseRecorder,
) (op, skipReason string, errs []*verrors.ValidationError, err error) {
	s, err := load()
	if err != nil {
		return "", "", nil, err
	}

	resp := rec.Result()
	resp.Request = req

	// FindPath and ValidateHttpResponse both touch the shared *v3.Document,
	// which the validator mutates while rendering (see validateMu). Hold the
	// lock across both so concurrent assertions cannot race on the singleton.
	validateMu.Lock()
	pathItem, findErrs, pathValue := paths.FindPath(req, s.model, s.findOpts)
	if pathItem == nil {
		validateMu.Unlock()
		return req.Method + " " + req.URL.Path, "", findErrs, nil
	}
	ok, valErrs := s.validator.ValidateHttpResponse(req, resp)
	validateMu.Unlock()

	op = req.Method + " " + pathValue
	if ok {
		return op, "", nil, nil
	}

	contentType := rec.Header().Get("Content-Type")
	for _, d := range knownDivergences {
		if d.match(rec.Code, contentType, valErrs) {
			return op, d.reason, nil, nil
		}
	}
	return op, "", valErrs, nil
}

// DocumentedOperations returns the set of "METHOD /path" operations the spec
// documents, derived from the same resolved model the validator uses.
func DocumentedOperations() (map[string]struct{}, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}
	// Iterating the shared model (and GetOperations' lazy access) touches the
	// same *v3.Document the validator mutates, so guard it with validateMu in
	// case this is called concurrently with validateRecordedResponse.
	validateMu.Lock()
	defer validateMu.Unlock()
	ops := make(map[string]struct{})
	for pair := s.model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		for opPair := pair.Value().GetOperations().First(); opPair != nil; opPair = opPair.Next() {
			ops[strings.ToUpper(opPair.Key())+" "+pair.Key()] = struct{}{}
		}
	}
	return ops, nil
}

// ValidatedOperations returns a snapshot of the operations that received at
// least one spec-validated response in this test binary.
func ValidatedOperations() map[string]struct{} {
	coverage.mu.Lock()
	defer coverage.mu.Unlock()
	out := make(map[string]struct{}, len(coverage.ops))
	for op := range coverage.ops {
		out[op] = struct{}{}
	}
	return out
}

// LedgerViolations diffs documented operations against spec-validated ones
// and returns one message per ledger inconsistency: an uncovered operation
// missing from the ledger, an entry for an operation the spec no longer
// documents, or an entry whose operation has since gained coverage
// (shrink-only). An empty result means the ledger is exact.
func LedgerViolations(ledger map[string]string) ([]string, error) {
	documented, err := DocumentedOperations()
	if err != nil {
		return nil, err
	}
	return ledgerViolations(ledger, documented, ValidatedOperations()), nil
}

func ledgerViolations(ledger map[string]string, documented, covered map[string]struct{}) []string {
	out := make([]string, 0, len(documented)+len(ledger))
	for op := range documented {
		if _, ok := covered[op]; ok {
			continue
		}
		if _, ok := ledger[op]; ok {
			continue
		}
		out = append(out, fmt.Sprintf(
			"documented operation has no spec-validated response test: %s\n"+
				"\t→ add one via specconformance.AssertConformsToSpec (preferred); only if\n"+
				"\t  intentionally deferring coverage, add the payloadCoverageLedger entry:\n"+
				"\t  %q: \"TODO(#1714): uncovered\",", op, op))
	}
	for op := range ledger {
		if _, ok := documented[op]; !ok {
			out = append(out, fmt.Sprintf(
				"stale ledger entry (operation no longer documented in openapi.yaml): %q — remove it", op))
			continue
		}
		if _, ok := covered[op]; ok {
			out = append(out, fmt.Sprintf(
				"stale ledger entry (operation now has a spec-validated response): %q — remove it, the ledger is shrink-only", op))
		}
	}
	sort.Strings(out)
	return out
}

func recordCovered(op string) {
	coverage.mu.Lock()
	defer coverage.mu.Unlock()
	coverage.ops[op] = struct{}{}
}

func load() (*loadedSpec, error) {
	loadOnce.Do(func() {
		spec, loadErr = loadSpec()
	})
	return spec, loadErr
}

// loadSpec parses openapi.yaml once per test binary. The rolodex resolves the
// multi-file layout (paths/*.yaml, schemas/*.yaml) natively via BasePath, so
// no bundling step is needed.
func loadSpec() (*loadedSpec, error) {
	specPath, err := findSpecFile()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(specPath) // #nosec G304 -- path located by walking up from the test's own directory
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", specPath, err)
	}
	doc, err := libopenapi.NewDocumentWithConfiguration(raw, &datamodel.DocumentConfiguration{
		BasePath:            filepath.Dir(specPath),
		SpecFilePath:        specPath,
		AllowFileReferences: true,
		// The rolodex logs spurious lookup-miss errors while probing
		// candidate base paths for ./paths/*.yaml refs; resolution still
		// succeeds, so keep test output clean.
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", specPath, err)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build v3 model for %s: %w", specPath, err)
	}
	v := validator.NewValidatorFromV3Model(&model.Model)
	return &loadedSpec{
		validator: v,
		model:     &model.Model,
		findOpts:  config.NewValidationOptions(),
	}, nil
}

// findSpecFile walks up from the test working directory until it finds
// openapi.yaml (backend-api/openapi.yaml from any package depth).
func findSpecFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for range 6 {
		candidate := filepath.Join(dir, "openapi.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("openapi.yaml not found walking up from the test working directory")
}

func formatValidationErrors(errs []*verrors.ValidationError) string {
	var b strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&b, "  - %s: %s\n", e.Message, e.Reason)
		for _, sv := range e.SchemaValidationErrors {
			fmt.Fprintf(&b, "      %s (at %s)\n", sv.Reason, sv.FieldPath)
		}
	}
	return b.String()
}
