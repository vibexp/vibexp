package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	hubspotAPIBaseURL = "https://api.hubapi.com"
	maxRetries        = 3
	initialBackoff    = 1 * time.Second
)

// ContactData represents the data to be synced to HubSpot
type ContactData struct {
	Email              string
	FirstName          string
	LastName           string
	CreatedAt          *time.Time
	SubscriptionStatus string
	SubscriptionPlan   string
	LastSeenAt         *time.Time
	// Prompt tracking fields
	LastPromptCreatedAt *time.Time
	TotalPrompts        *int
	// AI tool integration fields
	AIToolsIntegrated      []string // Array of tool types: "claude_code_cli", "cursor_ide"
	TotalAIToolsIntegrated *int
}

// Contact represents a HubSpot contact
type Contact struct {
	ID         string
	Email      string
	FirstName  string
	LastName   string
	CreatedAt  time.Time
	Properties map[string]string
}

// HubSpotService handles interactions with HubSpot CRM API
type HubSpotService struct {
	accessToken string
	httpClient  *http.Client
	logger      *logrus.Logger
	baseURL     string // For testing, defaults to hubspotAPIBaseURL
}

// NewHubSpotService creates a new HubSpot service
func NewHubSpotService(accessToken string, logger *logrus.Logger) *HubSpotService {
	if logger == nil {
		logger = logrus.New()
	}

	return &HubSpotService{
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		baseURL: hubspotAPIBaseURL,
	}
}

// CreateContact creates a new contact in HubSpot
func (s *HubSpotService) CreateContact(ctx context.Context, contactData ContactData) error {
	properties := s.buildContactProperties(contactData)

	payload := map[string]interface{}{
		"properties": properties,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal contact data: %w", err)
	}

	url := fmt.Sprintf("%s/crm/v3/objects/contacts", s.baseURL)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.calculateBackoff(attempt)
			s.logger.WithFields(logrus.Fields{
				"service":   "hubspot-crm",
				"component": "create-contact",
				"attempt":   attempt + 1,
				"backoff":   backoff,
			}).Info("Retrying contact creation")
			time.Sleep(backoff)
		}

		statusCode, respBody, err := s.makeRequest(ctx, "POST", url, body)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Success
		if statusCode >= 200 && statusCode < 300 {
			s.logger.WithFields(logrus.Fields{
				"service":   "hubspot-crm",
				"component": "create-contact",
				"email":     contactData.Email,
			}).Info("Contact created successfully")
			return nil
		}

		// Contact already exists - try update instead
		if statusCode == 409 {
			s.logger.WithFields(logrus.Fields{
				"service":   "hubspot-crm",
				"component": "create-contact",
				"email":     contactData.Email,
			}).Info("Contact already exists, attempting update")
			return s.UpdateContact(ctx, contactData.Email, contactData)
		}

		// Rate limit - retry with backoff
		if statusCode == 429 {
			lastErr = fmt.Errorf("rate limited by HubSpot API")
			continue
		}

		// Other error - log and return
		lastErr = fmt.Errorf("API request failed with status %d: %s", statusCode, string(respBody))
		break
	}

	return lastErr
}

// UpdateContact updates an existing contact in HubSpot
func (s *HubSpotService) UpdateContact(ctx context.Context, email string, contactData ContactData) error {
	// First, get the contact by email to retrieve the contact ID
	contact, err := s.GetContactByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to get contact by email: %w", err)
	}

	properties := s.buildContactProperties(contactData)

	payload := map[string]interface{}{
		"properties": properties,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal contact data: %w", err)
	}

	url := fmt.Sprintf("%s/crm/v3/objects/contacts/%s", s.baseURL, contact.ID)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.calculateBackoff(attempt)
			s.logger.WithFields(logrus.Fields{
				"service":   "hubspot-crm",
				"component": "update-contact",
				"attempt":   attempt + 1,
				"backoff":   backoff,
			}).Info("Retrying contact update")
			time.Sleep(backoff)
		}

		statusCode, respBody, err := s.makeRequest(ctx, "PATCH", url, body)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Success
		if statusCode >= 200 && statusCode < 300 {
			s.logger.WithFields(logrus.Fields{
				"service":    "hubspot-crm",
				"component":  "update-contact",
				"contact_id": contact.ID,
				"email":      email,
			}).Info("Contact updated successfully")
			return nil
		}

		// Rate limit - retry with backoff
		if statusCode == 429 {
			lastErr = fmt.Errorf("rate limited by HubSpot API")
			continue
		}

		// Other error - log and return
		lastErr = fmt.Errorf("API request failed with status %d: %s", statusCode, string(respBody))
		break
	}

	return lastErr
}

// GetContactByEmail retrieves a contact from HubSpot by email
func (s *HubSpotService) GetContactByEmail(ctx context.Context, email string) (*Contact, error) {
	url := fmt.Sprintf("%s/crm/v3/objects/contacts/%s?idProperty=email", s.baseURL, email)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.calculateBackoff(attempt)
			time.Sleep(backoff)
		}

		statusCode, respBody, err := s.makeRequest(ctx, "GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Success
		if statusCode >= 200 && statusCode < 300 {
			var response struct {
				ID         string            `json:"id"`
				Properties map[string]string `json:"properties"`
			}

			if err := json.Unmarshal(respBody, &response); err != nil {
				return nil, fmt.Errorf("failed to unmarshal response: %w", err)
			}

			contact := &Contact{
				ID:         response.ID,
				Email:      response.Properties["email"],
				FirstName:  response.Properties["firstname"],
				LastName:   response.Properties["lastname"],
				Properties: response.Properties,
			}

			return contact, nil
		}

		// Not found
		if statusCode == 404 {
			return nil, fmt.Errorf("contact not found: %s", email)
		}

		// Rate limit - retry with backoff
		if statusCode == 429 {
			lastErr = fmt.Errorf("rate limited by HubSpot API")
			continue
		}

		// Other error
		lastErr = fmt.Errorf("API request failed with status %d: %s", statusCode, string(respBody))
		break
	}

	return nil, lastErr
}

// buildContactProperties builds the HubSpot properties from ContactData
func (s *HubSpotService) buildContactProperties(data ContactData) map[string]string {
	properties := make(map[string]string)

	if data.Email != "" {
		properties["email"] = data.Email
	}

	if data.FirstName != "" {
		properties["firstname"] = data.FirstName
	}

	if data.LastName != "" {
		properties["lastname"] = data.LastName
	}

	if data.SubscriptionStatus != "" {
		properties["lifecyclestage"] = s.mapSubscriptionStatusToLifecycleStage(data.SubscriptionStatus)
	}

	if data.SubscriptionPlan != "" {
		// Store subscription plan as a custom property
		properties["subscription_plan"] = data.SubscriptionPlan
	}

	if data.LastSeenAt != nil {
		// Update "Time Last Seen" and "Time of Last Session"
		timestamp := fmt.Sprintf("%d", data.LastSeenAt.UnixMilli())
		properties["hs_analytics_last_timestamp"] = timestamp
		properties["hs_analytics_last_visit_timestamp"] = timestamp
	}

	// Prompt tracking fields
	if data.LastPromptCreatedAt != nil {
		properties["last_vibexp_prompt_created_at"] = fmt.Sprintf("%d", data.LastPromptCreatedAt.UnixMilli())
	}

	if data.TotalPrompts != nil {
		properties["total_vibexp_prompts"] = fmt.Sprintf("%d", *data.TotalPrompts)
	}

	// AI tool integration fields
	if len(data.AIToolsIntegrated) > 0 {
		// HubSpot multi-select field expects semicolon-separated values
		properties["vibexp_ai_tools_integrated"] = strings.Join(data.AIToolsIntegrated, ";")
	}

	if data.TotalAIToolsIntegrated != nil {
		properties["total_vibexp_ai_tools_integrated"] = fmt.Sprintf("%d", *data.TotalAIToolsIntegrated)
	}

	return properties
}

// mapSubscriptionStatusToLifecycleStage maps subscription status to HubSpot lifecycle stage
func (s *HubSpotService) mapSubscriptionStatusToLifecycleStage(status string) string {
	switch strings.ToLower(status) {
	case "active":
		return "customer"
	case "trialing":
		return "opportunity"
	case "canceled", "cancelled":
		return "former customer"
	default:
		return "lead"
	}
}

// makeRequest makes an HTTP request to HubSpot API
func (s *HubSpotService) makeRequest(ctx context.Context, method, url string, body []byte) (int, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.accessToken))
	req.Header.Set("Content-Type", "application/json")

	// #nosec G704 - URL is HubSpot API endpoint from service configuration, not user input
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.WithError(closeErr).Warn("Failed to close response body")
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

// calculateBackoff calculates exponential backoff with jitter
func (s *HubSpotService) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: 1s, 2s, 4s, etc.
	backoff := initialBackoff * time.Duration(math.Pow(2, float64(attempt)))

	// Add jitter (random 0-50% of backoff)
	jitter := time.Duration(float64(backoff) * 0.5 * float64(time.Now().UnixNano()%100) / 100.0)

	return backoff + jitter
}
