package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateInvitationToken_IsUnpaddedAndURLSafe pins the token shape that #251
// depends on.
//
// The bug: padded base64.URLEncoding always ends in '=', which clients
// percent-encode to %3D in the URL path, and chi hands the handler back the raw
// encoded segment — so the exact-match token lookup missed on every invitation.
// Unpadded RawURLEncoding contains only [A-Za-z0-9_-], none of which are reserved
// in a path segment, so the token survives the round trip untouched.
//
// The handlers still decode the path parameter (see invitationTokenParam), which is
// what keeps the padded tokens already in the database working; this test only
// guards the shape of NEWLY minted ones.
func TestGenerateInvitationToken_IsUnpaddedAndURLSafe(t *testing.T) {
	s := &TeamInvitationService{}

	// Generate several: padding depends on input length, and the reserved-character
	// classes below should hold for every token, not just a lucky one.
	for i := 0; i < 20; i++ {
		token, err := s.generateInvitationToken()
		require.NoError(t, err)

		assert.NotContains(t, token, "=", "token must not carry base64 padding (#251)")
		assert.NotContains(t, token, "+", "token must use the URL-safe base64 alphabet")
		assert.NotContains(t, token, "/", "token must use the URL-safe base64 alphabet")

		// 32 random bytes, unpadded → ceil(32*8/6) = 43 chars. The token column is
		// varchar(64), so this must stay well inside it.
		assert.Len(t, token, 43)
		assert.LessOrEqual(t, len(token), 64, "token must fit team_invitations.token varchar(64)")

		// Nothing needing percent-encoding in a path segment.
		assert.Equal(t, token, strings.TrimSpace(token))
		for _, r := range token {
			isURLSafeBase64 := (r >= 'A' && r <= 'Z') ||
				(r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') ||
				r == '-' || r == '_'
			assert.True(t, isURLSafeBase64, "unexpected character %q in token %q", r, token)
		}
	}
}

// TestGenerateInvitationToken_IsUnique guards against a degenerate generator — the
// token is the invitation's bearer credential and the column is UNIQUE.
func TestGenerateInvitationToken_IsUnique(t *testing.T) {
	s := &TeamInvitationService{}
	seen := make(map[string]bool, 50)

	for i := 0; i < 50; i++ {
		token, err := s.generateInvitationToken()
		require.NoError(t, err)
		assert.False(t, seen[token], "generated a duplicate token: %q", token)
		seen[token] = true
	}
}
