package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// MockEmailProvider is a mock implementation of external.EmailProvider
type MockEmailProvider struct {
	mock.Mock
}

func (m *MockEmailProvider) SendEmail(ctx context.Context, message *external.OutgoingMessage) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

// stubSenderResolver stands in for the send-time resolver. It hands back a fixed
// sender so these tests exercise message construction and error handling rather
// than resolution, which has its own suite.
type stubSenderResolver struct {
	sender     *ResolvedEmailSender
	resolveErr error
	// recorded captures what RecordSendOutcome was told, so a test can assert that
	// delivery health is stamped (and that an instance send is not attributed to a
	// team).
	recorded      bool
	recordedTeam  string
	recordedError error
}

func (r *stubSenderResolver) Resolve(context.Context, string) (*ResolvedEmailSender, error) {
	if r.resolveErr != nil {
		return nil, r.resolveErr
	}
	return r.sender, nil
}

func (r *stubSenderResolver) RecordSendOutcome(
	_ context.Context, sender *ResolvedEmailSender, sendErr error,
) error {
	r.recorded = true
	if sender != nil {
		r.recordedTeam = sender.TeamID
	}
	r.recordedError = sendErr
	return nil
}

// instanceResolver mirrors the production instance branch: the given provider and
// the configured from-address, with no team attribution.
func instanceResolver(provider external.EmailProvider) *stubSenderResolver {
	return &stubSenderResolver{sender: &ResolvedEmailSender{
		Provider:    provider,
		FromAddress: "test@example.com",
		Source:      EmailSenderSourceInstance,
	}}
}

func createTestEmailService() *EmailService {
	cfg := &config.Config{
		Email: config.EmailConfig{
			PrivacyPolicyURL: "https://example.com/privacy-policy",
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "test@example.com",
				Password: "password123",
			},
		},
		Frontend: config.FrontendConfig{
			BaseURL: "https://app.example.com",
		},
	}
	mockProvider := new(MockEmailProvider)
	mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
	return NewEmailService(instanceResolver(mockProvider), cfg)
}

func TestNewEmailService(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			SMTP: config.SMTPConfig{
				Host:     "smtp.test.com",
				Port:     "587",
				Username: "user@test.com",
				Password: "pass123",
			},
		},
	}

	mockProvider := new(MockEmailProvider)
	service := NewEmailService(instanceResolver(mockProvider), cfg)

	assert.NotNil(t, service)
	assert.Equal(t, cfg, service.cfg)
	assert.NotNil(t, service.resolver)
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
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "587",
						Username: "test@example.com",
						Password: "password123",
					},
				},
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(instanceResolver(mockProvider), cfg)

			err := service.sendEmail(context.Background(), "", tt.to, tt.subject, tt.htmlBody, tt.textBody)

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
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "587",
						Username: "support@vibexp.io",
						Password: "password123",
					},
				},
				Frontend: config.FrontendConfig{
					BaseURL: "https://app.vibexp.io",
				},
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(instanceResolver(mockProvider), cfg)

			err := service.SendSupportRequest(context.Background(), tt.userName, tt.userEmail, tt.request)

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
				mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(out *external.OutgoingMessage) bool {
					msg := out.Message
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
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "587",
						Username: "noreply@vibexp.io",
						Password: "password123",
					},
				},
				Frontend: config.FrontendConfig{
					BaseURL: "https://app.vibexp.io",
				},
			}
			mockProvider := tt.setupMock()
			service := NewEmailService(instanceResolver(mockProvider), cfg)

			err := service.SendTeamInvitation(context.Background(), "team-1", tt.invitation, tt.teamName, tt.inviterName)

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
// cfg.Email.FromAddress over cfg.Email.SMTP.Username when FromAddress is set.
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
			mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(out *external.OutgoingMessage) bool {
				msg := out.Message
				capturedMessage = msg
				return true
			})).Return(nil)

			cfg := &config.Config{
				Email: config.EmailConfig{
					FromAddress: tt.emailFromAddress,
					SMTP: config.SMTPConfig{
						Username: tt.smtpUsername,
					},
				},
			}
			// A REAL resolver, not the stub: the from-address chain now lives in
			// the resolver's instance branch, so driving it from cfg is what proves
			// the configured address still reaches the outgoing message. An empty
			// team ID short-circuits to the instance branch without a repository
			// read, so the repo mock needs no expectations.
			resolver := NewEmailSenderResolver(
				repomocks.NewMockTeamEmailProviderRepository(t), nil,
				mockProvider, cfg, slog.New(slog.DiscardHandler))
			service := NewEmailService(resolver, cfg)

			err := service.sendEmail(context.Background(), "", "to@example.com", "Test Subject", "<p>body</p>", "body")
			assert.NoError(t, err)
			require.NotNil(t, capturedMessage)
			assert.Equal(t, tt.expectedFrom, capturedMessage.GetFrom())
			mockProvider.AssertExpectations(t)
		})
	}
}

// --- Per-team sending (#504) ---------------------------------------------------

// teamResolver mirrors the production team branch: the team's provider and its own
// sender identity, attributed to the team so delivery health is recorded.
func teamResolver(provider external.EmailProvider) *stubSenderResolver {
	return &stubSenderResolver{sender: &ResolvedEmailSender{
		Provider:    provider,
		FromAddress: "hello@acme.test",
		FromName:    "Acme Team",
		ReplyTo:     "support@acme.test",
		Source:      EmailSenderSourceTeam,
		TeamID:      "team-1",
	}}
}

// captureSentOutgoing captures the whole OutgoingMessage, for the assertions
// about what travels beside the gomail message.
func captureSentOutgoing(mockProvider *MockEmailProvider, sendErr error) **external.OutgoingMessage {
	captured := new(*external.OutgoingMessage)
	mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(out *external.OutgoingMessage) bool {
		*captured = out
		return true
	})).Return(sendErr)
	return captured
}

func captureSentMessage(mockProvider *MockEmailProvider, sendErr error) **gomail.EmailMessage {
	captured := new(*gomail.EmailMessage)
	mockProvider.On("SendEmail", mock.Anything, mock.MatchedBy(func(out *external.OutgoingMessage) bool {
		msg := out.Message
		*captured = msg
		return true
	})).Return(sendErr)
	return captured
}

// A team with its own provider sends through it, as itself — including the
// display name and Reply-To, neither of which the old send path could express.
func TestEmailService_sendEmail_UsesTheTeamSenderIdentity(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentMessage(mockProvider, nil)
	resolver := teamResolver(mockProvider)

	service := NewEmailService(resolver, &config.Config{})

	err := service.sendEmail(
		context.Background(), "team-1", "to@example.com", "Subject", "<p>body</p>", "body")

	require.NoError(t, err)
	require.NotNil(t, *captured)
	// A BARE address: gomail validates this field with an email regex and
	// silently yields "" for an RFC-5322 `"Name" <addr>` form, which would send
	// with no From header at all. The configured from-name is deliberately not
	// applied here — see the FromName test below.
	assert.Equal(t, "hello@acme.test", (*captured).GetFrom())
	// Reply-To is mapped by all four providers but was never populated before.
	assert.Equal(t, "support@acme.test", (*captured).GetReplyTo())
}

func TestEmailService_sendEmail_BareAddressWhenNoFromName(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentMessage(mockProvider, nil)
	resolver := instanceResolver(mockProvider)

	service := NewEmailService(resolver, &config.Config{})

	require.NoError(t, service.sendEmail(
		context.Background(), "", "to@example.com", "Subject", "<p>b</p>", "b"))
	assert.Equal(t, "test@example.com", (*captured).GetFrom())
	assert.Empty(t, (*captured).GetReplyTo())
}

// A configured from-name must NOT be folded into gomail's From field. gomail
// validates that field with a plain email regex and returns "" for anything
// else, so a display name there would send mail with no From header at all —
// worse than not showing the name. Since #549 the name IS delivered, but on the
// OutgoingMessage beside this field rather than inside it; this test pins the
// field itself so nobody "simplifies" the two back into one.
func TestEmailService_sendEmail_FromNameIsNotFoldedIntoFrom(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentMessage(mockProvider, nil)

	service := NewEmailService(teamResolver(mockProvider), &config.Config{})

	require.NoError(t, service.sendEmail(
		context.Background(), "team-1", "to@example.com", "S", "<p>b</p>", "b"))

	from := (*captured).GetFrom()
	assert.Equal(t, "hello@acme.test", from)
	assert.NotContains(t, from, "Acme Team")
	assert.NotEmpty(t, from, "gomail drops a non-bare address, leaving no From at all")
}

// The caller's context reaches the provider, so a cancelled request no longer
// leaves a send running detached on context.Background().
func TestEmailService_sendEmail_PropagatesCallerContext(t *testing.T) {
	type ctxKey string
	const marker ctxKey = "marker"

	mockProvider := new(MockEmailProvider)
	var seen context.Context
	mockProvider.On("SendEmail", mock.MatchedBy(func(ctx context.Context) bool {
		seen = ctx
		return true
	}), mock.Anything).Return(nil)

	service := NewEmailService(instanceResolver(mockProvider), &config.Config{})
	ctx := context.WithValue(context.Background(), marker, "yes")

	require.NoError(t, service.sendEmail(ctx, "", "to@example.com", "S", "<p>b</p>", "b"))
	require.NotNil(t, seen)
	assert.Equal(t, "yes", seen.Value(marker), "the caller's ctx must reach the provider")
}

// A team send is recorded against the team so the health banner can show it.
func TestEmailService_sendEmail_RecordsTeamDeliveryOutcome(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockProvider := new(MockEmailProvider)
		captureSentMessage(mockProvider, nil)
		resolver := teamResolver(mockProvider)

		service := NewEmailService(resolver, &config.Config{})
		require.NoError(t, service.sendEmail(
			context.Background(), "team-1", "to@example.com", "S", "<p>b</p>", "b"))

		assert.True(t, resolver.recorded, "a success must be stamped too, or health can never recover")
		assert.Equal(t, "team-1", resolver.recordedTeam)
		assert.NoError(t, resolver.recordedError)
	})

	t.Run("failure records the error and does not fall back to the instance provider", func(t *testing.T) {
		mockProvider := new(MockEmailProvider)
		captureSentMessage(mockProvider, fmt.Errorf("relay denied"))
		resolver := teamResolver(mockProvider)

		service := NewEmailService(resolver, &config.Config{})
		err := service.sendEmail(context.Background(), "team-1", "to@example.com", "S", "<p>b</p>", "b")

		require.Error(t, err, "a failing team provider must surface, never silently re-send as the operator")
		assert.Contains(t, err.Error(), "relay denied")
		assert.True(t, resolver.recorded)
		require.Error(t, resolver.recordedError)
		// Exactly one send attempt: no instance retry (epic #499 decision 7).
		mockProvider.AssertNumberOfCalls(t, "SendEmail", 1)
	})
}

// An instance send has no team row to stamp, so it must not be attributed to
// whichever team happened to be in scope.
func TestEmailService_sendEmail_InstanceSendIsNotAttributedToATeam(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captureSentMessage(mockProvider, nil)
	resolver := instanceResolver(mockProvider)

	service := NewEmailService(resolver, &config.Config{})
	require.NoError(t, service.sendEmail(
		context.Background(), "", "to@example.com", "S", "<p>b</p>", "b"))

	assert.Empty(t, resolver.recordedTeam)
}

// A resolver failure aborts the send rather than falling back.
func TestEmailService_sendEmail_ResolverFailureAborts(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	resolver := &stubSenderResolver{resolveErr: fmt.Errorf("cannot decrypt secret")}

	service := NewEmailService(resolver, &config.Config{})
	err := service.sendEmail(context.Background(), "team-1", "to@example.com", "S", "<p>b</p>", "b")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve the email sender")
	mockProvider.AssertNotCalled(t, "SendEmail")
}

// Support mail is the operator's own correspondence: it must resolve with an empty
// team ID no matter who sent the request, so it can never go out on a tenant's
// credentials.
func TestEmailService_SendSupportRequest_AlwaysResolvesInstance(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)

	resolver := &recordingResolver{sender: &ResolvedEmailSender{
		Provider:    mockProvider,
		FromAddress: "ops@instance.test",
		Source:      EmailSenderSourceInstance,
	}}

	service := NewEmailService(resolver, &config.Config{
		Email:    config.EmailConfig{ContactRecipientAddress: "ops@instance.test"},
		Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"},
	})

	err := service.SendSupportRequest(context.Background(), "Jane", "jane@acme.test",
		&models.SupportRequest{Text: "Please help me with this problem."})

	require.NoError(t, err)
	require.NotEmpty(t, resolver.teamIDs)
	for _, teamID := range resolver.teamIDs {
		assert.Empty(t, teamID, "support mail must never be attributed to a team")
	}
}

// recordingResolver captures every team ID it is asked to resolve.
type recordingResolver struct {
	sender  *ResolvedEmailSender
	teamIDs []string
}

func (r *recordingResolver) Resolve(_ context.Context, teamID string) (*ResolvedEmailSender, error) {
	r.teamIDs = append(r.teamIDs, teamID)
	return r.sender, nil
}

func (r *recordingResolver) RecordSendOutcome(context.Context, *ResolvedEmailSender, error) error {
	return nil
}

// The invitation email is attributed to the inviting team.
func TestEmailService_SendTeamInvitation_ResolvesWithTheTeam(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	mockProvider.On("SendEmail", mock.Anything, mock.Anything).Return(nil)
	resolver := &recordingResolver{sender: &ResolvedEmailSender{
		Provider:    mockProvider,
		FromAddress: "hello@acme.test",
		Source:      EmailSenderSourceTeam,
		TeamID:      "team-42",
	}}

	service := NewEmailService(resolver, &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"},
	})

	err := service.SendTeamInvitation(context.Background(), "team-42", &models.TeamInvitation{
		InviteeEmail: "invitee@example.com",
		Token:        "tok",
		Role:         models.TeamMemberRoleMember,
		ExpiresAt:    time.Now().Add(time.Hour),
	}, "Acme", "Boss")

	require.NoError(t, err)
	assert.Equal(t, []string{"team-42"}, resolver.teamIDs)
}

// A team that has NOT configured a provider still gets its invitation sent — via
// the instance provider, from the instance address. This is the fallback half of
// the invitation path; the team half is covered above.
func TestEmailService_SendTeamInvitation_UnconfiguredTeamUsesInstance(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentMessage(mockProvider, nil)
	resolver := instanceResolver(mockProvider)

	service := NewEmailService(resolver, &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"},
	})

	err := service.SendTeamInvitation(context.Background(), "team-without-provider",
		&models.TeamInvitation{
			InviteeEmail: "invitee@example.com",
			Token:        "tok",
			Role:         models.TeamMemberRoleMember,
			ExpiresAt:    time.Now().Add(time.Hour),
		}, "Acme", "Boss")

	require.NoError(t, err)
	assert.Equal(t, "test@example.com", (*captured).GetFrom(),
		"an unconfigured team sends from the instance address")
	// Nothing to stamp: there is no team row behind an instance send.
	assert.Empty(t, resolver.recordedTeam)
}

// The resolved display name reaches the provider, which is the whole point of
// #549 — the From address stays bare, and the name rides alongside it.
func TestEmailService_sendEmail_ForwardsTheResolvedFromName(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentOutgoing(mockProvider, nil)

	service := NewEmailService(teamResolver(mockProvider), &config.Config{})

	require.NoError(t, service.sendEmail(
		context.Background(), "team-1", "to@example.com", "S", "<p>b</p>", "b"))

	out := *captured
	require.NotNil(t, out)
	assert.Equal(t, "Acme Team", out.FromName)
	assert.Equal(t, "hello@acme.test", out.Message.GetFrom())
	assert.Equal(t, `"Acme Team" <hello@acme.test>`, out.FromHeader())
}

// A sender with no display name — the instance branch — must still produce a
// bare From, not net/mail's "<addr>" form.
func TestEmailService_sendEmail_NoFromNameStaysBare(t *testing.T) {
	mockProvider := new(MockEmailProvider)
	captured := captureSentOutgoing(mockProvider, nil)

	service := NewEmailService(instanceResolver(mockProvider), &config.Config{})

	require.NoError(t, service.sendEmail(
		context.Background(), "", "to@example.com", "S", "<p>b</p>", "b"))

	out := *captured
	require.NotNil(t, out)
	assert.Empty(t, out.FromName)
	assert.Equal(t, "test@example.com", out.FromHeader())
}
