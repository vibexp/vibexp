package server

// Coverage for the server-side share-page renderer (shared_prompt_html.go,
// coverage epic #358 / issue #393). generateSharedPromptHTML assembles the
// crawler-facing SSR page from the three template helpers; the assertions pin
// the meta-tag markers a social crawler actually reads (OpenGraph / Twitter
// card / <title>) plus the HTML-escaping and empty-description fallback the
// renderer promises, rather than snapshotting the whole document.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSharedPromptHTML_EmbedsMetadata(t *testing.T) {
	const (
		name        = "Release Notes Writer"
		description = "Drafts polished release notes from a changelog"
		shareURL    = "https://vibexp.io/shared/prompts/tok-123"
		imageURL    = "https://vibexp.io/assets/og-image.png"
	)

	got := generateSharedPromptHTML(name, description, shareURL, imageURL)

	require.NotEmpty(t, got)
	// Structural skeleton from the three template helpers.
	assert.True(t, strings.HasPrefix(got, "<!DOCTYPE html>"), "must start with the doctype")
	assert.Contains(t, got, "</html>", "must close the document")
	assert.Contains(t, got, "<style>", "styles must be spliced between head and body")

	// <title> and description carry the prompt name.
	assert.Contains(t, got, "<title>"+name+" | Shared Prompt | VibeXP.io</title>")
	assert.Contains(t, got, `content="`+description+`"`)

	// OpenGraph markers a crawler reads.
	assert.Contains(t, got, `property="og:title" content="`+name+`"`)
	assert.Contains(t, got, `property="og:description" content="`+description+`"`)
	assert.Contains(t, got, `property="og:url" content="`+shareURL+`"`)
	assert.Contains(t, got, `property="og:image" content="`+imageURL+`"`)

	// Twitter card markers.
	assert.Contains(t, got, `name="twitter:title" content="`+name+`"`)
	assert.Contains(t, got, `name="twitter:image" content="`+imageURL+`"`)

	// The JS + noscript redirect both point at the share URL.
	assert.Contains(t, got, "window.location.href = '"+shareURL+"'")
	assert.Contains(t, got, `<a href="`+shareURL+`" class="redirect-link">`)
}

func TestGenerateSharedPromptHTML_EscapesUserContent(t *testing.T) {
	got := generateSharedPromptHTML(
		`<script>alert(1)</script>`,
		`"quoted" & <b>bold</b>`,
		"https://vibexp.io/shared/prompts/tok-xss",
		"https://vibexp.io/assets/og-image.png",
	)

	// The raw injection must never survive into the rendered page.
	assert.NotContains(t, got, "<script>alert(1)</script>")
	assert.Contains(t, got, "&lt;script&gt;alert(1)&lt;/script&gt;")
	// Ampersand + angle brackets in the description are escaped too.
	assert.Contains(t, got, "&amp; &lt;b&gt;bold&lt;/b&gt;")
}

func TestGenerateSharedPromptHTML_EmptyDescriptionFallsBack(t *testing.T) {
	got := generateSharedPromptHTML(
		"Nameless",
		"",
		"https://vibexp.io/shared/prompts/tok-empty",
		"https://vibexp.io/assets/og-image.png",
	)

	// Empty description collapses to the "Shared prompt" default in every slot.
	assert.Contains(t, got, `name="description" content="Shared prompt"`)
	assert.Contains(t, got, `property="og:description" content="Shared prompt"`)
}

func TestIsCrawlerUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "googlebot", userAgent: "Mozilla/5.0 (compatible; Googlebot/2.1)", want: true},
		{name: "facebook scraper", userAgent: "facebookexternalhit/1.1", want: true},
		{name: "twitterbot", userAgent: "Twitterbot/1.0", want: true},
		{name: "slackbot", userAgent: "Slackbot-LinkExpanding 1.0", want: true},
		{name: "whatsapp", userAgent: "WhatsApp/2.23", want: true},
		{name: "generic bot substring", userAgent: "SomeRandomBot/9", want: true},
		{name: "case insensitive", userAgent: "GOOGLEBOT", want: true},
		{name: "regular chrome browser", userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120", want: false},
		{name: "empty", userAgent: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCrawlerUserAgent(tt.userAgent))
		})
	}
}
