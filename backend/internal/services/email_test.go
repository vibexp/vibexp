package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

// MockEmailProvider is a mock implementation of external.EmailProvider
type MockEmailProvider struct {
	mock.Mock
}

func (m *MockEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func createTestEmailService() *EmailService {
	cfg := &config.Config{
		SMTPHost:         "smtp.example.com",
		SMTPPort:         "587",
		SMTPUsername:     "test@example.com",
		SMTPPassword:     "password123",
		FrontendBaseURL:  "https://app.example.com",
		PrivacyPolicyURL: "https://example.com/privacy-policy",
	}
	mockProvider := new(MockEmailProvider)
	mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
	return NewEmailService(mockProvider, cfg)
}

func TestNewEmailService(t *testing.T) {
	cfg := &config.Config{
		SMTPHost:     "smtp.test.com",
		SMTPPort:     "587",
		SMTPUsername: "user@test.com",
		SMTPPassword: "pass123",
	}

	mockProvider := new(MockEmailProvider)
	service := NewEmailService(mockProvider, cfg)

	assert.NotNil(t, service)
	assert.Equal(t, cfg, service.cfg)
	assert.NotNil(t, service.provider)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_sendEmail(t *testing.T) {
	tests := []struct {
		name        string
		to          string
		subject     string
		htmlBody    string
		textBody    string
		setupMock   func() *MockEmailProvider
		expectError bool
		errorMsg    string
	}{
		{
			name:     "Successful email with HTML and text",
			to:       "recipient@example.com",
			subject:  "Test Subject",
			htmlBody: "<p>Test HTML Body</p>",
			textBody: "Test text body",
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
				return mockProvider
			},
			expectError: false,
		},
		{
			name:     "SMTP provider error",
			to:       "recipient@example.com",
			subject:  "Test Subject",
			htmlBody: "<p>Test HTML Body</p>",
			textBody: "Test text body",
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(fmt.Errorf("SMTP connection failed"))
				return mockProvider
			},
			expectError: true,
			errorMsg:    "failed to send email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				SMTPHost:     "smtp.example.com",
				SMTPPort:     "587",
				SMTPUsername: "test@example.com",
				SMTPPassword: "password123",
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(mockProvider, cfg)

			err := service.sendEmail(tt.to, tt.subject, tt.htmlBody, tt.textBody)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

// Test helper to verify interface compliance
func TestEmailService_ImplementsInterface(t *testing.T) {
	service := createTestEmailService()

	// Verify that EmailService implements EmailServiceInterface
	var _ EmailServiceInterface = service
}

// Test email template footer links and content
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_EmailTemplateFooter(t *testing.T) {
	service := createTestEmailService()

	testData := struct {
		TeamName         string
		InviterName      string
		Role             string
		AcceptURL        string
		ExpiryDate       string
		Year             int
		AppBaseURL       string
		PrivacyPolicyURL string
	}{
		TeamName:         "Test Team",
		InviterName:      "John Doe",
		Role:             "member",
		AcceptURL:        "https://app.example.com/invitations/accept/token123",
		ExpiryDate:       "January 1, 2026",
		Year:             2025,
		AppBaseURL:       "https://app.example.com",
		PrivacyPolicyURL: "https://example.com/privacy-policy",
	}

	// Render the template
	htmlBody, err := service.renderTemplateFromFS(
		"templates/email/base.html",
		"templates/email/team-invitation.html",
		testData,
	)
	assert.NoError(t, err)

	// Verify footer links are updated correctly
	t.Run("Footer has correct unsubscribe link with UTM parameters", func(t *testing.T) {
		// Note: HTML templates escape & to &amp; but text templates use plain &
		// Check for both patterns to be flexible
		unsubscribeLinkEscaped := "https://app.example.com/settings/notifications?" +
			"utm_source=email&amp;utm_medium=footer&amp;utm_campaign=email-preferences"
		unsubscribeLinkUnescaped := "https://app.example.com/settings/notifications?" +
			"utm_source=email&utm_medium=footer&utm_campaign=email-preferences"
		hasEscaped := strings.Contains(htmlBody, unsubscribeLinkEscaped)
		hasUnescaped := strings.Contains(htmlBody, unsubscribeLinkUnescaped)
		assert.True(t, hasEscaped || hasUnescaped, "Footer should contain unsubscribe link with UTM parameters")
	})

	t.Run("Footer has correct privacy policy link with UTM parameters", func(t *testing.T) {
		// Note: HTML templates escape & to &amp; but text templates use plain &
		privacyLinkEscaped := "https://example.com/privacy-policy?" +
			"utm_source=email&amp;utm_medium=footer&amp;utm_campaign=privacy"
		privacyLinkUnescaped := "https://example.com/privacy-policy?" +
			"utm_source=email&utm_medium=footer&utm_campaign=privacy"
		hasEscaped := strings.Contains(htmlBody, privacyLinkEscaped)
		hasUnescaped := strings.Contains(htmlBody, privacyLinkUnescaped)
		assert.True(t, hasEscaped || hasUnescaped, "Footer should contain privacy policy link with UTM parameters")
	})

	t.Run("Footer does not contain 'View in Browser' link", func(t *testing.T) {
		assert.NotContains(t, htmlBody, "View in Browser")
	})

	t.Run("Footer has updated copy about email preferences", func(t *testing.T) {
		assert.Contains(t, htmlBody, "You're receiving this email because you have a VibeXP account")
		assert.Contains(t, htmlBody, "notification settings")
	})

	t.Run("Footer does not contain old support system message", func(t *testing.T) {
		assert.NotContains(t, htmlBody, "This message was sent via the VibeXP support system")
	})

	t.Run("Footer link text changed from 'Unsubscribe' to 'Manage Email Preferences'", func(t *testing.T) {
		assert.Contains(t, htmlBody, "Manage Email Preferences")
		// Verify the old "Unsubscribe" text is not present as a standalone link
		// (but may appear in the body copy)
		assert.NotContains(t, htmlBody, `<a href="https://app.example.com">Unsubscribe</a>`)
	})
}

// Test SendSupportRequest method
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_SendSupportRequest(t *testing.T) {
	tests := []struct {
		name              string
		userName          string
		userEmail         string
		request           *models.SupportRequest
		setupMock         func() *MockEmailProvider
		expectError       bool
		expectedSendCalls int
	}{
		{
			name:      "successful support request without acknowledgement",
			userName:  "John Doe",
			userEmail: "john@example.com",
			request: &models.SupportRequest{
				Text:            "I need help with my account",
				Acknowledgement: false,
			},
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				// Only one email - admin notification
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil).Once()
				return mockProvider
			},
			expectError:       false,
			expectedSendCalls: 1,
		},
		{
			name:      "successful support request with acknowledgement",
			userName:  "Jane Smith",
			userEmail: "jane@example.com",
			request: &models.SupportRequest{
				Text:            "Feature request",
				Acknowledgement: true,
			},
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				// Two emails - admin notification + user acknowledgement
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil).Twice()
				return mockProvider
			},
			expectError:       false,
			expectedSendCalls: 2,
		},
		{
			name:      "support request with additional info",
			userName:  "Test User",
			userEmail: "test@example.com",
			request: &models.SupportRequest{
				Text:            "I have a question",
				Acknowledgement: false,
				AdditionalInfo: map[string]string{
					"browser": "Chrome",
					"version": "120.0",
				},
			},
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil).Once()
				return mockProvider
			},
			expectError:       false,
			expectedSendCalls: 1,
		},
		{
			name:      "admin notification fails",
			userName:  "Error User",
			userEmail: "error@example.com",
			request: &models.SupportRequest{
				Text:            "This will fail",
				Acknowledgement: false,
			},
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(fmt.Errorf("SMTP error")).Once()
				return mockProvider
			},
			expectError:       true,
			expectedSendCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				SMTPHost:        "smtp.example.com",
				SMTPPort:        "587",
				SMTPUsername:    "support@vibexp.io",
				SMTPPassword:    "password123",
				FrontendBaseURL: "https://app.vibexp.io",
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(mockProvider, cfg)

			err := service.SendSupportRequest(tt.userName, tt.userEmail, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to send admin notification")
			} else {
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

// Test SendTeamInvitation method
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_SendTeamInvitation(t *testing.T) {
	tests := []struct {
		name        string
		invitation  *models.TeamInvitation
		teamName    string
		inviterName string
		setupMock   func() *MockEmailProvider
		expectError bool
	}{
		{
			name: "successful team invitation",
			invitation: &models.TeamInvitation{
				ID:           "invite-123",
				TeamID:       "team-456",
				InviteeEmail: "newmember@example.com",
				Token:        "token123abc",
				Role:         "member",
				ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
			},
			teamName:    "Awesome Team",
			inviterName: "Team Lead",
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(msg *gomail.EmailMessage) bool {
					// Verify the email is sent to the invitee
					return msg.GetTo()[0] == "newmember@example.com"
				})).Return(nil).Once()
				return mockProvider
			},
			expectError: false,
		},
		{
			name: "team invitation with admin role",
			invitation: &models.TeamInvitation{
				ID:           "invite-456",
				TeamID:       "team-789",
				InviteeEmail: "admin@example.com",
				Token:        "admintoken",
				Role:         "admin",
				ExpiresAt:    time.Now().Add(24 * time.Hour),
			},
			teamName:    "Enterprise Team",
			inviterName: "CEO",
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil).Once()
				return mockProvider
			},
			expectError: false,
		},
		{
			name: "team invitation email send fails",
			invitation: &models.TeamInvitation{
				ID:           "invite-789",
				TeamID:       "team-000",
				InviteeEmail: "fail@example.com",
				Token:        "failtoken",
				Role:         "member",
				ExpiresAt:    time.Now().Add(48 * time.Hour),
			},
			teamName:    "Failing Team",
			inviterName: "Unlucky Manager",
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(fmt.Errorf("SMTP connection refused")).Once()
				return mockProvider
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				SMTPHost:        "smtp.example.com",
				SMTPPort:        "587",
				SMTPUsername:    "noreply@vibexp.io",
				SMTPPassword:    "password123",
				FrontendBaseURL: "https://app.vibexp.io",
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(mockProvider, cfg)

			err := service.SendTeamInvitation(tt.invitation, tt.teamName, tt.inviterName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

// Test buildAdditionalInfo helper
func TestEmailService_BuildAdditionalInfo(t *testing.T) {
	service := createTestEmailService()

	tests := []struct {
		name           string
		additionalInfo map[string]string
		expectHTMLNil  bool
		expectTextNil  bool
	}{
		{
			name:           "empty additional info",
			additionalInfo: map[string]string{},
			expectHTMLNil:  true,
			expectTextNil:  true,
		},
		{
			name:           "nil additional info",
			additionalInfo: nil,
			expectHTMLNil:  true,
			expectTextNil:  true,
		},
		{
			name: "with additional info",
			additionalInfo: map[string]string{
				"browser": "Chrome",
				"os":      "Windows",
			},
			expectHTMLNil: false,
			expectTextNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, text := service.buildAdditionalInfo(tt.additionalInfo)

			if tt.expectHTMLNil {
				assert.Empty(t, html)
			} else {
				assert.NotEmpty(t, html)
				assert.Contains(t, html, "Additional Information")
			}

			if tt.expectTextNil {
				assert.Empty(t, text)
			} else {
				assert.NotEmpty(t, text)
				assert.Contains(t, text, "ADDITIONAL INFORMATION")
			}
		})
	}
}

// Test extractFirstName helper
func TestEmailService_ExtractFirstName(t *testing.T) {
	service := createTestEmailService()

	tests := []struct {
		name     string
		fullName string
		expected string
	}{
		{
			name:     "full name with first and last",
			fullName: "John Doe",
			expected: "John",
		},
		{
			name:     "single name",
			fullName: "John",
			expected: "John",
		},
		{
			name:     "empty name",
			fullName: "",
			expected: "there",
		},
		{
			name:     "name with multiple parts",
			fullName: "John Middle Doe",
			expected: "John",
		},
		{
			name:     "name with extra spaces",
			fullName: "  John   Doe  ",
			expected: "John",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.extractFirstName(tt.fullName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEmailService_SendEmail_UsesEmailFromAddress verifies that sendEmail prefers
// cfg.EmailFromAddress over cfg.SMTPUsername when EmailFromAddress is set.
func TestEmailService_SendEmail_UsesEmailFromAddress(t *testing.T) {
	tests := []struct {
		name             string
		emailFromAddress string
		smtpUsername     string
		expectedFrom     string
	}{
		{
			name:             "uses EmailFromAddress when set",
			emailFromAddress: "noreply@vibexp.io",
			smtpUsername:     "smtp-user@gmail.com",
			expectedFrom:     "noreply@vibexp.io",
		},
		{
			name:             "falls back to SMTPUsername when EmailFromAddress empty",
			emailFromAddress: "",
			smtpUsername:     "smtp-user@gmail.com",
			expectedFrom:     "smtp-user@gmail.com",
		},
		{
			name:             "both empty results in empty from",
			emailFromAddress: "",
			smtpUsername:     "",
			expectedFrom:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedMessage *gomail.EmailMessage
			mockProvider := new(MockEmailProvider)
			mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(msg *gomail.EmailMessage) bool {
				capturedMessage = msg
				return true
			})).Return(nil)

			cfg := &config.Config{
				EmailFromAddress: tt.emailFromAddress,
				SMTPUsername:     tt.smtpUsername,
			}
			service := NewEmailService(mockProvider, cfg)

			err := service.sendEmail("to@example.com", "Test Subject", "<p>body</p>", "body")
			assert.NoError(t, err)
			require.NotNil(t, capturedMessage)
			assert.Equal(t, tt.expectedFrom, capturedMessage.GetFrom())
			mockProvider.AssertExpectations(t)
		})
	}
}
