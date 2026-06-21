package server

import (
	"fmt"
	"html"
	"strings"
)

// getHTMLHead returns the HTML head section with meta tags
func getHTMLHead() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <meta name="description" content="%s">

    <!-- OpenGraph Meta Tags -->
    <meta property="og:type" content="website">
    <meta property="og:title" content="%s">
    <meta property="og:description" content="%s">
    <meta property="og:url" content="%s">
    <meta property="og:site_name" content="VibeXP.io">
    <meta property="og:image" content="%s">

    <!-- Twitter Card Meta Tags -->
    <meta name="twitter:card" content="summary_large_image">
    <meta name="twitter:title" content="%s">
    <meta name="twitter:description" content="%s">
    <meta name="twitter:image" content="%s">

    <!-- Redirect to frontend SPA for regular users -->
    <script>
        const userAgent = navigator.userAgent.toLowerCase();
        const isCrawler = /bot|crawler|spider|crawling|facebookexternalhit|twitterbot|linkedinbot/i.test(userAgent);
        if (!isCrawler) {
            window.location.href = '%s';
        }
    </script>`
}

// getHTMLStyles returns the CSS styles for the shared prompt page
func getHTMLStyles() string {
	return `
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto',
                'Oxygen', 'Ubuntu', 'Cantarell', sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
            line-height: 1.6;
            color: #333;
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo {
            max-width: 80px;
            height: auto;
        }
        h1 {
            color: #1a1a1a;
            margin-bottom: 10px;
        }
        .description {
            color: #666;
            margin-bottom: 20px;
        }
        .redirect-notice {
            background: #f0f0f0;
            padding: 15px;
            border-radius: 8px;
            text-align: center;
        }
        .redirect-link {
            color: #0066cc;
            text-decoration: none;
            font-weight: bold;
        }
        .redirect-link:hover {
            text-decoration: underline;
        }
    </style>
</head>`
}

// getHTMLBody returns the HTML body section
func getHTMLBody() string {
	return `
<body>
    <noscript>
        <div class="header">
            <img src="%s" alt="VibeXP" class="logo">
            <h1>%s</h1>
            <p class="description">%s</p>
        </div>
        <div class="redirect-notice">
            <p>Please <a href="%s" class="redirect-link">click here</a> to view this shared prompt.</p>
        </div>
    </noscript>
</body>
</html>`
}

// generateSharedPromptHTML generates the complete HTML for a shared prompt
func generateSharedPromptHTML(name, description, shareURL, imageURL string) string {
	safeName := html.EscapeString(name)
	safeDescription := html.EscapeString(description)

	fullDescription := safeDescription
	if fullDescription == "" {
		fullDescription = "Shared prompt"
	}

	pageTitle := fmt.Sprintf("%s | Shared Prompt | VibeXP.io", safeName)

	head := fmt.Sprintf(
		getHTMLHead(),
		pageTitle,       // <title>
		fullDescription, // meta description
		safeName,        // og:title
		fullDescription, // og:description
		shareURL,        // og:url
		imageURL,        // og:image
		safeName,        // twitter:title
		fullDescription, // twitter:description
		imageURL,        // twitter:image
		shareURL,        // JavaScript redirect
	)

	body := fmt.Sprintf(
		getHTMLBody(),
		imageURL,        // noscript logo
		safeName,        // noscript h1
		fullDescription, // noscript description
		shareURL,        // noscript link
	)

	return head + getHTMLStyles() + body
}

// isCrawlerUserAgent checks if the User-Agent indicates a crawler/bot
func isCrawlerUserAgent(userAgent string) bool {
	userAgent = strings.ToLower(userAgent)

	// Common crawler user agents
	crawlers := []string{
		"bot",
		"crawler",
		"spider",
		"crawling",
		"facebookexternalhit",
		"facebookcatalog",
		"twitterbot",
		"linkedinbot",
		"slackbot",
		"telegrambot",
		"whatsapp",
		"pinterest",
		"discordbot",
		"googlebot",
		"bingbot",
		"yandexbot",
		"baiduspider",
		"duckduckbot",
	}

	for _, crawler := range crawlers {
		if strings.Contains(userAgent, crawler) {
			return true
		}
	}

	return false
}
