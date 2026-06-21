package server

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// invalidProjectIDCases is the canonical set of caller-input failures all MCP
// tools that accept project_id must surface as IsError=true with a
// user-friendly message that does NOT leak the raw Postgres "pq:" prefix or
// the SQLSTATE 22P02 token.
type invalidProjectIDCase struct {
	name      string
	projectID string
	// substrMust is a list of substrings that must appear in the response text.
	substrMust []string
}

func invalidProjectIDCases() []invalidProjectIDCase {
	return []invalidProjectIDCase{
		{
			name:       "empty",
			projectID:  "",
			substrMust: []string{"required"},
		},
		{
			name:       "malformed_org_repo",
			projectID:  "shaharia-lab/vibexp.io",
			substrMust: []string{"not a valid UUID", "shaharia-lab/vibexp.io"},
		},
	}
}

// extractText returns the concatenated TextContent text in a CallToolResult.
func extractText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("expected non-nil CallToolResult")
		return ""
	}
	var out strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out.WriteString(tc.Text)
		}
	}
	return out.String()
}

// assertValidationFailure verifies the result represents a project_id validation
// rejection: IsError=true, contains all `substrMust`, and never leaks the raw
// driver substring "pq:" or the SQLSTATE token "22P02".
func assertValidationFailure(t *testing.T, res *mcp.CallToolResult, substrMust []string) {
	t.Helper()
	if res == nil {
		t.Fatal("expected non-nil CallToolResult")
		return
	}
	if !res.IsError {
		t.Error("expected IsError=true")
	}
	text := extractText(t, res)
	for _, s := range substrMust {
		if !strings.Contains(text, s) {
			t.Errorf("expected response text to contain %q; got %q", s, text)
		}
	}
	if strings.Contains(text, "pq:") {
		t.Errorf("response text leaked driver prefix \"pq:\": %q", text)
	}
	if strings.Contains(text, "22P02") {
		t.Errorf("response text leaked SQLSTATE \"22P02\": %q", text)
	}
}

func TestValidateProjectID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		projectID string
		wantErr   bool
		substr    []string
	}{
		{
			name:      "valid_uuid",
			projectID: "550e8400-e29b-41d4-a716-446655440000",
			wantErr:   false,
		},
		{
			name:      "empty_string",
			projectID: "",
			wantErr:   true,
			substr:    []string{"required"},
		},
		{
			name:      "malformed_org_repo",
			projectID: "shaharia-lab/vibexp.io",
			wantErr:   true,
			substr:    []string{"not a valid UUID", "shaharia-lab/vibexp.io"},
		},
		{
			name:      "almost_uuid_too_short",
			projectID: "550e8400-e29b-41d4-a716",
			wantErr:   true,
			substr:    []string{"not a valid UUID"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := validateProjectID(tc.projectID)
			if !tc.wantErr {
				if res != nil {
					t.Errorf("expected nil for valid input, got %+v", res)
				}
				return
			}
			assertValidationFailure(t, res, tc.substr)
		})
	}
}
