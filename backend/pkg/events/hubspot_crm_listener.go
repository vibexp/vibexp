package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/services/crm"
)

// HubSpotCRMServiceInterface defines the interface for HubSpot CRM operations
type HubSpotCRMServiceInterface interface {
	CreateContact(ctx context.Context, contactData crm.ContactData) error
	UpdateContact(ctx context.Context, email string, contactData crm.ContactData) error
	GetContactByEmail(ctx context.Context, email string) (*crm.Contact, error)
}

// HubSpotCRMListener handles user events and syncs them to HubSpot CRM
type HubSpotCRMListener struct {
	crmService HubSpotCRMServiceInterface
	logger     *logrus.Logger
}

// NewHubSpotCRMListener creates a new HubSpot CRM listener
func NewHubSpotCRMListener(crmService HubSpotCRMServiceInterface, logger *logrus.Logger) *HubSpotCRMListener {
	if logger == nil {
		logger = logrus.New()
	}
	return &HubSpotCRMListener{
		crmService: crmService,
		logger:     logger,
	}
}

// Handle processes user.created and user.updated events
func (l *HubSpotCRMListener) Handle(ctx context.Context, event Event) error {
	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "hubspot-crm-listener",
		"event_type": event.Type(),
	}).Debug("Received event for HubSpot CRM sync")

	// Backfill-origin events are replays of historical entities, not genuine user
	// actions. Syncing them would call the HubSpot API once per entity and
	// overwrite contacts' engagement timestamps (e.g. LastPromptCreatedAt) with
	// stale historical values, so skip them entirely.
	if IsBackfillOrigin(event) {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Debug("Skipping backfill-origin event for HubSpot CRM sync")
		return nil
	}

	switch event.Type() {
	case EventTypeUserCreated:
		return l.handleUserCreated(ctx, event)
	case EventTypeUserUpdated:
		return l.handleUserUpdated(ctx, event)
	case EventTypePromptCreated:
		return l.handlePromptCreated(ctx, event)
	case EventTypeAIToolSessionCreated:
		return l.handleAIToolSessionCreated(ctx, event)
	default:
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Warn("Unexpected event type received")
		return nil
	}
}

// handleUserCreated handles user.created events
func (l *HubSpotCRMListener) handleUserCreated(ctx context.Context, event Event) error {
	payload, ok := event.Payload().(*UserCreatedPayload)
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Error("Failed to cast payload to UserCreatedPayload")
		// Don't return error to avoid retry storms
		return nil
	}

	contactData := l.buildContactDataFromUserCreated(payload)

	if err := l.crmService.CreateContact(ctx, contactData); err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
			"user_id":    payload.UserID,
			"email":      payload.Email,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to create contact in HubSpot CRM")
		// Don't return error to avoid blocking user operations
		return nil
	}

	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "hubspot-crm-listener",
		"event_type": event.Type(),
		"user_id":    payload.UserID,
		"email":      payload.Email,
	}).Info("Successfully synced user.created event to HubSpot CRM")

	return nil
}

// handleUserUpdated handles user.updated events
func (l *HubSpotCRMListener) handleUserUpdated(ctx context.Context, event Event) error {
	payload, ok := event.Payload().(*UserUpdatedPayload)
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Error("Failed to cast payload to UserUpdatedPayload")
		// Don't return error to avoid retry storms
		return nil
	}

	contactData := l.buildContactDataFromUserUpdated(payload)

	// For user updates, we update the existing contact
	// The service will handle contact creation if it doesn't exist
	if err := l.crmService.UpdateContact(ctx, payload.Email, contactData); err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
			"user_id":    payload.UserID,
			"email":      payload.Email,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update contact in HubSpot CRM")
		// Don't return error to avoid blocking user operations
		return nil
	}

	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "hubspot-crm-listener",
		"event_type": event.Type(),
		"user_id":    payload.UserID,
		"email":      payload.Email,
	}).Info("Successfully synced user.updated event to HubSpot CRM")

	return nil
}

// buildContactDataFromUserCreated builds ContactData from UserCreatedPayload
func (l *HubSpotCRMListener) buildContactDataFromUserCreated(payload *UserCreatedPayload) crm.ContactData {
	firstName, lastName := l.parseName(payload.Name)

	return crm.ContactData{
		Email:     payload.Email,
		FirstName: firstName,
		LastName:  lastName,
		CreatedAt: &payload.CreatedAt,
	}
}

// buildContactDataFromUserUpdated builds ContactData from UserUpdatedPayload
func (l *HubSpotCRMListener) buildContactDataFromUserUpdated(payload *UserUpdatedPayload) crm.ContactData {
	firstName, lastName := l.parseName(payload.Name)

	return crm.ContactData{
		Email:      payload.Email,
		FirstName:  firstName,
		LastName:   lastName,
		LastSeenAt: &payload.UpdatedAt,
	}
}

// handlePromptCreated handles prompt.created events
func (l *HubSpotCRMListener) handlePromptCreated(ctx context.Context, event Event) error {
	payload, ok := event.Payload().(*PromptCreatedPayload)
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Error("Failed to cast payload to PromptCreatedPayload")
		return nil
	}

	contactData := crm.ContactData{
		Email:               payload.Email,
		LastPromptCreatedAt: &payload.CreatedAt,
	}

	if err := l.crmService.UpdateContact(ctx, payload.Email, contactData); err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
			"user_id":    payload.UserID,
			"email":      payload.Email,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update contact with prompt data in HubSpot CRM")
		return nil
	}

	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "hubspot-crm-listener",
		"event_type": event.Type(),
		"user_id":    payload.UserID,
		"email":      payload.Email,
	}).Info("Successfully synced prompt.created event to HubSpot CRM")

	return nil
}

// handleAIToolSessionCreated handles ai_tool_session.created events
func (l *HubSpotCRMListener) handleAIToolSessionCreated(ctx context.Context, event Event) error {
	payload, ok := event.Payload().(*AIToolSessionCreatedPayload)
	if !ok {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
		}).Error("Failed to cast payload to AIToolSessionCreatedPayload")
		return nil
	}

	contactData := crm.ContactData{
		Email:             payload.Email,
		AIToolsIntegrated: []string{payload.ToolType},
	}

	if err := l.crmService.UpdateContact(ctx, payload.Email, contactData); err != nil {
		l.logger.WithFields(logrus.Fields{
			"service":    "vibexp-api",
			"component":  "hubspot-crm-listener",
			"event_type": event.Type(),
			"user_id":    payload.UserID,
			"email":      payload.Email,
			"tool_type":  payload.ToolType,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update contact with AI tool data in HubSpot CRM")
		return nil
	}

	l.logger.WithFields(logrus.Fields{
		"service":    "vibexp-api",
		"component":  "hubspot-crm-listener",
		"event_type": event.Type(),
		"user_id":    payload.UserID,
		"email":      payload.Email,
		"tool_type":  payload.ToolType,
	}).Info("Successfully synced ai_tool_session.created event to HubSpot CRM")

	return nil
}

// parseName parses a full name into first name and last name
// For example: "John Doe" -> firstName: "John", lastName: "Doe"
// For single names: "John" -> firstName: "John", lastName: ""
func (l *HubSpotCRMListener) parseName(fullName string) (firstName, lastName string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "", ""
	}

	parts := strings.SplitN(fullName, " ", 2)
	firstName = parts[0]

	if len(parts) > 1 {
		lastName = strings.TrimSpace(parts[1])
	}

	return firstName, lastName
}

// EventTypes returns the event types this listener handles
func (l *HubSpotCRMListener) EventTypes() []string {
	return []string{
		EventTypeUserCreated,
		EventTypeUserUpdated,
		EventTypePromptCreated,
		EventTypeAIToolSessionCreated,
	}
}
