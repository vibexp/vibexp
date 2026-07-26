package server

import (
	"net/http"
	"strings"
)

// artifactTestBasePath is the real, team-scoped artifact collection route,
// trailing slash included — the router registers `POST /api/v1/{team_id}/artifacts/`.
// It must stay an existing pattern: these cases assert 401, and a path the router
// does not register returns 404 instead.
const artifactTestBasePath = "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/"

// artifactTestCases returns common test cases for artifact validation
func artifactBadRequestCases(auth string) []testCase {
	return []testCase{
		{Name: "Invalid JSON", Method: "POST", Path: artifactTestBasePath,
			Body: `{"invalid": json}`, Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Missing slug", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"title":"Test Artifact","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Empty slug", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"","title":"Test Artifact","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Missing title", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Empty title", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Missing content", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Empty content", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":""}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Project ID invalid UUID", Method: "POST", Path: artifactTestBasePath,
			Body: `{"slug":"test-slug","title":"Test Artifact","content":"Test content","project_id":"` +
				"invalid-uuid" + `"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Slug too long", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"` + strings.Repeat("a", 256) + `","title":"Test Artifact","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Title too long", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"` + strings.Repeat("a", 256) + `","content":"Test content"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Description too long", Method: "POST", Path: artifactTestBasePath,
			Body: `{"slug":"test-slug","title":"Test Artifact","content":"Test content","description":"` +
				strings.Repeat("a", 501) + `"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Invalid type", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","type":"invalid"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid type work-reports", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","type":"work-reports"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid type static-contexts", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","type":"static-contexts"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid type general", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","type":"general"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Invalid status", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","status":"invalid"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid status active", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","status":"active"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid status draft", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","status":"draft"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
		{Name: "Valid status archived", Method: "POST", Path: artifactTestBasePath,
			Body:          `{"slug":"test-slug","title":"Test Artifact","content":"Test content","status":"archived"}`,
			Authorization: auth, Expected: http.StatusUnauthorized},
	}
}
