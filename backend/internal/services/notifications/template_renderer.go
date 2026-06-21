package notifications

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

//go:embed templates/*.html
var notificationTemplateFS embed.FS

// RenderedContent holds the rendered title and body for a notification
type RenderedContent struct {
	InAppTitle    string
	InAppBody     string
	EmailSubject  string
	EmailBodyHTML string
	// EntityID is the source entity ID used for deduplication (e.g. feed item ID)
	EntityID string
}

// TemplateRendererInterface defines the contract for rendering notification content
type TemplateRendererInterface interface {
	Render(notifType NotificationType, data map[string]interface{}) (*RenderedContent, error)
}

// TemplateRenderer renders notification content using embedded templates
type TemplateRenderer struct {
	// appBaseURL is the deployment's frontend base URL (no trailing slash),
	// threaded into templates that link back into the app (e.g. the digest
	// email's "manage preferences" link). No tenant-specific domain is baked
	// into the templates.
	appBaseURL string
}

// NewTemplateRenderer creates a new TemplateRenderer. appBaseURL is the
// frontend base URL used to build app-facing links in rendered emails; any
// trailing slash is trimmed.
func NewTemplateRenderer(appBaseURL string) *TemplateRenderer {
	return &TemplateRenderer{
		appBaseURL: strings.TrimRight(appBaseURL, "/"),
	}
}

// Render generates notification content for the given type and data
func (t *TemplateRenderer) Render(
	notifType NotificationType,
	data map[string]interface{},
) (*RenderedContent, error) {
	switch notifType {
	case "feed.item.created":
		return t.renderFeedItemCreated(data)
	default:
		return &RenderedContent{
			InAppTitle:    fmt.Sprintf("New notification: %s", notifType),
			InAppBody:     "",
			EmailSubject:  fmt.Sprintf("New notification: %s", notifType),
			EmailBodyHTML: "",
		}, nil
	}
}

func (t *TemplateRenderer) renderFeedItemCreated(data map[string]interface{}) (*RenderedContent, error) {
	actorName, _ := data["actor_name"].(string)
	title, _ := data["title"].(string)

	inAppTitle := fmt.Sprintf("%s posted to the team feed", actorName)
	inAppBody := title

	emailHTML, err := t.renderHTMLTemplate("templates/feed_item_created_email.html", data)
	if err != nil {
		return nil, fmt.Errorf("render feed_item_created email template: %w", err)
	}

	itemID, _ := data["item_id"].(string)

	return &RenderedContent{
		InAppTitle:    inAppTitle,
		InAppBody:     inAppBody,
		EmailSubject:  "New post in your team feed",
		EmailBodyHTML: emailHTML,
		EntityID:      itemID,
	}, nil
}

func (t *TemplateRenderer) renderHTMLTemplate(path string, data interface{}) (string, error) {
	content, err := notificationTemplateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", path, err)
	}

	tmpl, err := template.New("notification").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", path, err)
	}

	return buf.String(), nil
}

// digestEmailData holds the rendering context for the daily digest email.
type digestEmailData struct {
	UserName   string
	Groups     []digestEmailGroup
	AppBaseURL string
}

// digestEmailGroup groups notifications by team for the digest email.
type digestEmailGroup struct {
	TeamName      string
	Notifications []digestEmailNotif
}

// digestEmailNotif is a single notification item in the digest email.
type digestEmailNotif struct {
	Title     string
	Body      string
	ActionURL string
}

// RenderDigestEmail renders the digest email template for the given user and notifications.
// Notifications are grouped by TeamID; the TeamID value is used as the group label.
// For proper team name resolution, prefer RenderDigestEmailWithTeamNames.
func (t *TemplateRenderer) RenderDigestEmail(user *models.User, notifs []*models.Notification) (string, error) {
	return t.RenderDigestEmailWithTeamNames(user, notifs, nil)
}

// RenderDigestEmailWithTeamNames renders the digest email template, resolving team IDs to
// display names using the provided teamNames map. When a team ID is absent from the map
// the team ID itself is used as a fallback so the email always renders.
func (t *TemplateRenderer) RenderDigestEmailWithTeamNames(
	user *models.User,
	notifs []*models.Notification,
	teamNames map[string]string,
) (string, error) {
	data := buildDigestEmailData(user, notifs, teamNames)
	data.AppBaseURL = t.appBaseURL
	return t.renderHTMLTemplate("templates/digest_email.html", data)
}

// buildDigestEmailData constructs the template data structure from a flat notification list.
// teamNames maps team IDs to display names; absent entries fall back to the team ID string.
func buildDigestEmailData(
	user *models.User, notifs []*models.Notification, teamNames map[string]string,
) digestEmailData {
	groupIndex := map[string]int{}
	var groups []digestEmailGroup

	for _, n := range notifs {
		teamID := n.TeamID
		displayName := teamID
		if teamNames != nil {
			if resolved, ok := teamNames[teamID]; ok {
				displayName = resolved
			}
		}

		idx, exists := groupIndex[teamID]
		if !exists {
			idx = len(groups)
			groupIndex[teamID] = idx
			groups = append(groups, digestEmailGroup{TeamName: displayName})
		}

		groups[idx].Notifications = append(groups[idx].Notifications, digestEmailNotif{
			Title:     n.Title,
			Body:      n.Body,
			ActionURL: n.ActionURL,
		})
	}

	return digestEmailData{
		UserName: user.Name,
		Groups:   groups,
	}
}
