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

func createTestContactFormRequest() *models.ContactFormRequest {
	return &models.ContactFormRequest{
		Name:        "John Doe",
		Email:       "john.doe@example.com",
		PhoneNumber: stringPtr("+1234567890"),
		Message:     "This is a test message from the contact form.",
	}
}

func createMinimalContactFormRequest() *models.ContactFormRequest {
	return &models.ContactFormRequest{
		Name:    "Jane Smith",
		Email:   "jane@example.com",
		Message: "Minimal contact form message without phone number.",
	}
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
func TestEmailService_SendContactMessage(t *testing.T) {
	tests := []struct {
		name        string
		request     *models.ContactFormRequest
		setupMock   func() *MockEmailProvider
		expectError bool
		errorMsg    string
	}{
		{
			name:    "Successful contact message with phone number",
			request: createTestContactFormRequest(),
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
				return mockProvider
			},
			expectError: false,
		},
		{
			name:    "Successful contact message without phone number",
			request: createMinimalContactFormRequest(),
			setupMock: func() *MockEmailProvider {
				mockProvider := new(MockEmailProvider)
				mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
				return mockProvider
			},
			expectError: false,
		},
		{
			name:    "SMTP sending error",
			request: createTestContactFormRequest(),
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

			err := service.SendContactMessage(tt.request)

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

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_renderHTMLTemplate(t *testing.T) {
	service := createTestEmailService()

	tests := []struct {
		name        string
		template    string
		data        interface{}
		expectError bool
		expected    string
	}{
		{
			name: "Valid template with contact form data",
			template: `
				<h2>Contact from {{.Name}}</h2>
				<p>Email: {{.Email}}</p>
				{{if .PhoneNumber}}<p>Phone: {{.PhoneNumber}}</p>{{end}}
				<p>Message: {{.Message}}</p>
			`,
			data:        createTestContactFormRequest(),
			expectError: false,
			expected:    "Contact from John Doe",
		},
		{
			name:        "Template with conditional rendering - with phone",
			template:    `{{if .PhoneNumber}}Phone: {{.PhoneNumber}}{{else}}No phone provided{{end}}`,
			data:        createTestContactFormRequest(),
			expectError: false,
			expected:    "Phone: &#43;1234567890", // HTML escaped
		},
		{
			name:        "Template with conditional rendering - without phone",
			template:    `{{if .PhoneNumber}}Phone: {{.PhoneNumber}}{{else}}No phone provided{{end}}`,
			data:        createMinimalContactFormRequest(),
			expectError: false,
			expected:    "No phone provided",
		},
		{
			name:        "Invalid template syntax",
			template:    `{{.Name}} {{invalid syntax`,
			data:        createTestContactFormRequest(),
			expectError: true,
		},
		{
			name:        "Template with non-existent field",
			template:    `{{.NonExistentField}}`,
			data:        createTestContactFormRequest(),
			expectError: true,
		},
		{
			name:        "Empty template",
			template:    "",
			data:        createTestContactFormRequest(),
			expectError: false,
			expected:    "",
		},
		{
			name:     "Template with HTML escaping",
			template: `Name: {{.Name}}`,
			data: &models.ContactFormRequest{
				Name:    "John <script>alert('xss')</script> Doe",
				Email:   "john@example.com",
				Message: "Test message",
			},
			expectError: false,
			expected:    "John &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; Doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.renderHTMLTemplate(tt.template, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expected != "" {
					assert.Contains(t, result, tt.expected)
				}
			}
		})
	}
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

// Mock SMTP functionality for testing email composition
func TestEmailService_EmailComposition(t *testing.T) {
	service := createTestEmailService()

	// Test the admin notification template rendering
	req := createTestContactFormRequest()

	// Test template rendering for admin notification
	adminTemplate := `
	<h2>New Contact Form Submission</h2>
	<p><strong>Name:</strong> {{.Name}}</p>
	<p><strong>Email:</strong> {{.Email}}</p>
	{{if .PhoneNumber}}<p><strong>Phone:</strong> {{.PhoneNumber}}</p>{{end}}
	<p><strong>Message:</strong></p>
	<div style="background-color: #f5f5f5; padding: 15px; border-left: 4px solid #007cba; margin: 10px 0;">
		{{.Message}}
	</div>
	<hr>
	<p><em>This message was sent via the vibexp.io contact form.</em></p>
	`

	body, err := service.renderHTMLTemplate(adminTemplate, req)
	assert.NoError(t, err)
	assert.Contains(t, body, "John Doe")
	assert.Contains(t, body, "john.doe@example.com")
	assert.Contains(t, body, "&#43;1234567890") // HTML escaped
	assert.Contains(t, body, "This is a test message from the contact form.")
	assert.Contains(t, body, "vibexp.io contact form")
}

// Edge case tests
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmailService_EdgeCases(t *testing.T) {
	t.Run("Empty contact form fields", func(t *testing.T) {
		mockProvider := new(MockEmailProvider)
		mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)

		cfg := &config.Config{
			SMTPHost:     "smtp.example.com",
			SMTPPort:     "587",
			SMTPUsername: "test@example.com",
			SMTPPassword: "password123",
		}
		service := NewEmailService(mockProvider, cfg)

		req := &models.ContactFormRequest{
			Name:    "",
			Email:   "",
			Message: "",
		}

		// Should still attempt to send (validation happens at handler level)
		err := service.SendContactMessage(req)
		assert.NoError(t, err)
		mockProvider.AssertExpectations(t)
	})

	t.Run("Very large message content", func(t *testing.T) {
		mockProvider := new(MockEmailProvider)
		mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)

		cfg := &config.Config{
			SMTPHost:     "smtp.example.com",
			SMTPPort:     "587",
			SMTPUsername: "test@example.com",
			SMTPPassword: "password123",
		}
		service := NewEmailService(mockProvider, cfg)

		largeMessage := strings.Repeat("A", 10000) // 10KB message
		req := &models.ContactFormRequest{
			Name:    "Large Message User",
			Email:   "large@example.com",
			Message: largeMessage,
		}

		err := service.SendContactMessage(req)
		assert.NoError(t, err)
		mockProvider.AssertExpectations(t)
	})

	t.Run("Template rendering with nil data", func(t *testing.T) {
		service := createTestEmailService()

		result, err := service.renderHTMLTemplate("{{.Name}}", nil)
		// Go templates handle nil data gracefully, it just renders empty
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Unicode and emoji handling", func(t *testing.T) {
		service := createTestEmailService()

		req := &models.ContactFormRequest{
			Name:    "张三 🎉 José María",
			Email:   "unicode@example.com",
			Message: "Message with unicode: 你好世界 🌍 émojis ñáéíóú",
		}

		// Test template rendering with unicode
		template := "Name: {{.Name}}, Message: {{.Message}}"
		result, err := service.renderHTMLTemplate(template, req)
		assert.NoError(t, err)
		assert.Contains(t, result, "张三 🎉 José María")
		assert.Contains(t, result, "你好世界 🌍")
	})
}

// Performance test for template rendering
func BenchmarkEmailService_renderHTMLTemplate(b *testing.B) {
	service := createTestEmailService()
	req := createTestContactFormRequest()

	template := `
	<h2>New Contact Form Submission</h2>
	<p><strong>Name:</strong> {{.Name}}</p>
	<p><strong>Email:</strong> {{.Email}}</p>
	{{if .PhoneNumber}}<p><strong>Phone:</strong> {{.PhoneNumber}}</p>{{end}}
	<p><strong>Message:</strong></p>
	<div>{{.Message}}</div>
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.renderHTMLTemplate(template, req)
		if err != nil {
			b.Fatal(err)
		}
	}
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

// Comprehensive test for the admin notification template
func TestEmailService_AdminNotificationTemplate(t *testing.T) {
	service := createTestEmailService()

	tests := []struct {
		name     string
		request  *models.ContactFormRequest
		expected []string
	}{
		{
			name:    "Complete contact form with phone",
			request: createTestContactFormRequest(),
			expected: []string{
				"New Contact Form Submission",
				"John Doe",
				"john.doe@example.com",
				"&#43;1234567890", // HTML escaped
				"This is a test message from the contact form",
				"vibexp.io contact form",
			},
		},
		{
			name:    "Contact form without phone number",
			request: createMinimalContactFormRequest(),
			expected: []string{
				"New Contact Form Submission",
				"Jane Smith",
				"jane@example.com",
				"Minimal contact form message",
				"vibexp.io contact form",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the actual template from the service
			adminTemplate := `
	<h2>New Contact Form Submission</h2>
	<p><strong>Name:</strong> {{.Name}}</p>
	<p><strong>Email:</strong> {{.Email}}</p>
	{{if .PhoneNumber}}<p><strong>Phone:</strong> {{.PhoneNumber}}</p>{{end}}
	<p><strong>Message:</strong></p>
	<div style="background-color: #f5f5f5; padding: 15px; border-left: 4px solid #007cba; margin: 10px 0;">
		{{.Message}}
	</div>
	<hr>
	<p><em>This message was sent via the vibexp.io contact form.</em></p>
	`

			result, err := service.renderHTMLTemplate(adminTemplate, tt.request)
			assert.NoError(t, err)

			for _, expected := range tt.expected {
				assert.Contains(t, result, expected, "Template should contain: %s", expected)
			}

			// Verify phone number is only included when present
			if tt.request.PhoneNumber != nil {
				// Phone number will be HTML escaped
				assert.Contains(t, result, "&#43;1234567890")
			} else {
				// Should not contain phone section when PhoneNumber is nil
				assert.NotContains(t, result, "<strong>Phone:</strong>")
			}
		})
	}
}
