package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/services/crm"
)

// MockHubSpotCRMService is a mock implementation of HubSpotCRMServiceInterface
type MockHubSpotCRMService struct {
	createContactCalled bool
	updateContactCalled bool
	lastContactData     crm.ContactData
	lastEmail           string
	createContactError  error
	updateContactError  error
	getContactError     error
	getContactResponse  *crm.Contact
}

func (m *MockHubSpotCRMService) CreateContact(ctx context.Context, contactData crm.ContactData) error {
	m.createContactCalled = true
	m.lastContactData = contactData
	return m.createContactError
}

func (m *MockHubSpotCRMService) UpdateContact(ctx context.Context, email string, contactData crm.ContactData) error {
	m.updateContactCalled = true
	m.lastEmail = email
	m.lastContactData = contactData
	return m.updateContactError
}

func (m *MockHubSpotCRMService) GetContactByEmail(ctx context.Context, email string) (*crm.Contact, error) {
	m.lastEmail = email
	return m.getContactResponse, m.getContactError
}

func TestNewHubSpotCRMListener(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()

	listener := NewHubSpotCRMListener(mockCRM, logger)

	assert.NotNil(t, listener)
	assert.NotNil(t, listener.crmService)
	assert.NotNil(t, listener.logger)
}

func TestNewHubSpotCRMListener_WithNilLogger(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}

	listener := NewHubSpotCRMListener(mockCRM, nil)

	assert.NotNil(t, listener)
	assert.NotNil(t, listener.logger) // Should create a default logger
}

func TestHubSpotCRMListener_HandleUserCreated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	listener := NewHubSpotCRMListener(mockCRM, logger)

	createdAt := time.Now()
	event := NewUserCreatedEvent("user-123", "test@example.com", "John Doe", createdAt)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.createContactCalled)
	assert.Equal(t, "test@example.com", mockCRM.lastContactData.Email)
	assert.Equal(t, "John", mockCRM.lastContactData.FirstName)
	assert.Equal(t, "Doe", mockCRM.lastContactData.LastName)
	assert.Equal(t, &createdAt, mockCRM.lastContactData.CreatedAt)
}

func TestHubSpotCRMListener_HandleUserCreated_WithCRMError(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{
		createContactError: errors.New("HubSpot API error"),
	}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewUserCreatedEvent("user-123", "test@example.com", "John Doe", time.Now())

	// Should not return error even if CRM fails (graceful degradation)
	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.createContactCalled)
}

func TestHubSpotCRMListener_HandleUserUpdated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	updatedAt := time.Now()
	event := NewUserUpdatedEvent("user-123", "test@example.com", "Jane Smith", updatedAt)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
	assert.Equal(t, "test@example.com", mockCRM.lastEmail)
	assert.Equal(t, "test@example.com", mockCRM.lastContactData.Email)
	assert.Equal(t, "Jane", mockCRM.lastContactData.FirstName)
	assert.Equal(t, "Smith", mockCRM.lastContactData.LastName)
	assert.Equal(t, &updatedAt, mockCRM.lastContactData.LastSeenAt)
}

func TestHubSpotCRMListener_HandlePromptCreated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	createdAt := time.Now()
	event := NewPromptCreatedEvent(
		"prompt-123",
		"user-123",
		"test@example.com",
		"default",
		"my-prompt",
		"Test Prompt",
		"This is the rendered body",
		createdAt,
	)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
	assert.Equal(t, "test@example.com", mockCRM.lastEmail)
	assert.Equal(t, "test@example.com", mockCRM.lastContactData.Email)
	assert.Equal(t, &createdAt, mockCRM.lastContactData.LastPromptCreatedAt)
}

func TestHubSpotCRMListener_HandlePromptCreated_WithCRMError(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{
		updateContactError: errors.New("HubSpot API error"),
	}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewPromptCreatedEvent(
		"prompt-123",
		"user-123",
		"test@example.com",
		"default",
		"my-prompt",
		"Test Prompt",
		"Body",
		time.Now(),
	)

	// Should not return error even if CRM fails (graceful degradation)
	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
}

func TestHubSpotCRMListener_HandleAIToolSessionCreated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewAIToolSessionCreatedEvent(
		"user-123",
		"test@example.com",
		"session-456",
		"claude_code_cli",
		true,
	)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
	assert.Equal(t, "test@example.com", mockCRM.lastEmail)
	assert.Equal(t, "test@example.com", mockCRM.lastContactData.Email)
	assert.Equal(t, []string{"claude_code_cli"}, mockCRM.lastContactData.AIToolsIntegrated)
}

func TestHubSpotCRMListener_HandleAIToolSessionCreated_CursorIDE(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewAIToolSessionCreatedEvent(
		"user-123",
		"test@example.com",
		"session-789",
		"cursor_ide",
		false,
	)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
	assert.Equal(t, []string{"cursor_ide"}, mockCRM.lastContactData.AIToolsIntegrated)
}

func TestHubSpotCRMListener_HandleAIToolSessionCreated_WithCRMError(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{
		updateContactError: errors.New("HubSpot API error"),
	}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewAIToolSessionCreatedEvent(
		"user-123",
		"test@example.com",
		"session-456",
		"claude_code_cli",
		true,
	)

	// Should not return error even if CRM fails (graceful degradation)
	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled)
}

func TestHubSpotCRMListener_HandleInvalidPayload(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	// Create an event with wrong payload type
	event := NewBaseEvent(EventTypeUserCreated, "invalid-payload", "user-123")

	// Should not return error even with invalid payload (graceful degradation)
	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.False(t, mockCRM.createContactCalled)
}

func TestHubSpotCRMListener_HandleUnexpectedEvent(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	// Create an event type that listener doesn't handle
	event := NewBaseEvent("unknown.event", "payload", "user-123")

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.False(t, mockCRM.createContactCalled)
	assert.False(t, mockCRM.updateContactCalled)
}

func TestHubSpotCRMListener_EventTypes(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	listener := NewHubSpotCRMListener(mockCRM, logger)

	eventTypes := listener.EventTypes()

	assert.Len(t, eventTypes, 4)
	assert.Contains(t, eventTypes, EventTypeUserCreated)
	assert.Contains(t, eventTypes, EventTypeUserUpdated)
	assert.Contains(t, eventTypes, EventTypePromptCreated)
	assert.Contains(t, eventTypes, EventTypeAIToolSessionCreated)
}

func TestHubSpotCRMListener_ParseName(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	listener := NewHubSpotCRMListener(mockCRM, logger)

	tests := []struct {
		name          string
		fullName      string
		expectedFirst string
		expectedLast  string
	}{
		{
			name:          "Full name with first and last",
			fullName:      "John Doe",
			expectedFirst: "John",
			expectedLast:  "Doe",
		},
		{
			name:          "Full name with multiple spaces",
			fullName:      "John  Doe",
			expectedFirst: "John",
			expectedLast:  "Doe",
		},
		{
			name:          "Single name",
			fullName:      "John",
			expectedFirst: "John",
			expectedLast:  "",
		},
		{
			name:          "Name with multiple parts",
			fullName:      "John Michael Doe",
			expectedFirst: "John",
			expectedLast:  "Michael Doe",
		},
		{
			name:          "Empty name",
			fullName:      "",
			expectedFirst: "",
			expectedLast:  "",
		},
		{
			name:          "Name with leading/trailing spaces",
			fullName:      "  John Doe  ",
			expectedFirst: "John",
			expectedLast:  "Doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstName, lastName := listener.parseName(tt.fullName)
			assert.Equal(t, tt.expectedFirst, firstName)
			assert.Equal(t, tt.expectedLast, lastName)
		})
	}
}

func TestHubSpotCRMListener_SkipsBackfillOriginPromptCreated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	event := NewPromptCreatedEvent(
		"prompt-1", "user-1", "test@example.com", "proj", "slug", "title", "body", time.Now(),
	)
	MarkBackfillOrigin(event)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.False(t, mockCRM.updateContactCalled,
		"a backfill-origin prompt.created must not call UpdateContact (no API load, no LastPromptCreatedAt overwrite)")
}

func TestHubSpotCRMListener_HandlesNormalPromptCreated(t *testing.T) {
	mockCRM := &MockHubSpotCRMService{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	listener := NewHubSpotCRMListener(mockCRM, logger)

	createdAt := time.Now()
	event := NewPromptCreatedEvent(
		"prompt-1", "user-1", "test@example.com", "proj", "slug", "title", "body", createdAt,
	)

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, mockCRM.updateContactCalled,
		"a genuine user-driven prompt.created must still sync to the CRM")
	assert.Equal(t, "test@example.com", mockCRM.lastEmail)
	assert.Equal(t, &createdAt, mockCRM.lastContactData.LastPromptCreatedAt)
}
