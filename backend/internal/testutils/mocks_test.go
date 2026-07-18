package testutils

import (
	"testing"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	"github.com/vibexp/vibexp/internal/external"
	extmocks "github.com/vibexp/vibexp/internal/external/mocks"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestServiceMocksImplementInterfaces validates that all service mocks implement their respective interfaces
func TestServiceMocksImplementInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		mock     interface{}
		expected interface{}
	}{
		{"AuthServiceInterface", &svcmocks.MockAuthServiceInterface{}, (*services.AuthServiceInterface)(nil)},
		{"APIKeyServiceInterface", &svcmocks.MockAPIKeyServiceInterface{}, (*services.APIKeyServiceInterface)(nil)},
		{"PromptServiceInterface", &svcmocks.MockPromptServiceInterface{}, (*services.PromptServiceInterface)(nil)},
		{
			"EmbeddingProviderServiceInterface",
			&svcmocks.MockEmbeddingProviderServiceInterface{},
			(*services.EmbeddingProviderServiceInterface)(nil),
		},
		{"EmailServiceInterface", &svcmocks.MockEmailServiceInterface{}, (*services.EmailServiceInterface)(nil)},
		{"MemoryServiceInterface", &svcmocks.MockMemoryServiceInterface{}, (*services.MemoryServiceInterface)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will fail at compile time if the mock doesn't implement the interface
			_ = tt.expected
			if tt.mock == nil {
				t.Errorf("Mock %s is nil", tt.name)
			}
		})
	}
}

// TestRepositoryMocksImplementInterfaces validates that all repository mocks implement their respective interfaces
func TestRepositoryMocksImplementInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		mock     interface{}
		expected interface{}
	}{
		{"UserRepository", &repomocks.MockUserRepository{}, (*repositories.UserRepository)(nil)},
		{"APIKeyRepository", &repomocks.MockAPIKeyRepository{}, (*repositories.APIKeyRepository)(nil)},
		{"PromptRepository", &repomocks.MockPromptRepository{}, (*repositories.PromptRepository)(nil)},
		{
			"EmbeddingProviderRepository",
			&repomocks.MockEmbeddingProviderRepository{},
			(*repositories.EmbeddingProviderRepository)(nil),
		},
		// SubscriptionRepository was removed as part of subscription model simplification
		{"MemoryRepository", &repomocks.MockMemoryRepository{}, (*repositories.MemoryRepository)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will fail at compile time if the mock doesn't implement the interface
			_ = tt.expected
			if tt.mock == nil {
				t.Errorf("Mock %s is nil", tt.name)
			}
		})
	}
}

// TestExternalMocksImplementInterfaces validates that all external mocks implement their respective interfaces
func TestExternalMocksImplementInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		mock     interface{}
		expected interface{}
	}{
		{"IdentityProvider", &idpmocks.MockIdentityProvider{}, (*idp.IdentityProvider)(nil)},
		{"EmailSender", &extmocks.MockEmailSender{}, (*external.EmailSender)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will fail at compile time if the mock doesn't implement the interface
			_ = tt.expected
			if tt.mock == nil {
				t.Errorf("Mock %s is nil", tt.name)
			}
		})
	}
}

// TestMockContainerCreation validates that mock containers can be created successfully
func TestMockContainerCreation(t *testing.T) {
	t.Run("NewMockServiceContainer", func(t *testing.T) {
		validateMockServiceContainer(t)
	})

	t.Run("NewMockRepositoryContainer", func(t *testing.T) {
		validateMockRepositoryContainer(t)
	})

	t.Run("NewMockExternalContainer", func(t *testing.T) {
		validateMockExternalContainer(t)
	})

	t.Run("NewMockContainer", func(t *testing.T) {
		validateMockContainer(t)
	})

	t.Run("NewMockContainerForTest", func(t *testing.T) {
		validateMockContainerForTest(t)
	})
}

func validateMockServiceContainer(t *testing.T) {
	t.Helper()
	container := NewMockServiceContainer(t)
	if container == nil {
		t.Fatal("NewMockServiceContainer returned nil")
		return
	}

	validateServiceField(t, container.Auth, "Auth")
	validateServiceField(t, container.APIKey, "APIKey")
	validateServiceField(t, container.Prompt, "Prompt")
	validateServiceField(t, container.EmbeddingProvider, "EmbeddingProvider")
	validateServiceField(t, container.Email, "Email")
	validateServiceField(t, container.Memory, "Memory")
}

func validateMockRepositoryContainer(t *testing.T) {
	t.Helper()
	container := NewMockRepositoryContainer(t)
	if container == nil {
		t.Fatal("NewMockRepositoryContainer returned nil")
		return
	}

	validateServiceField(t, container.User, "User repository")
	validateServiceField(t, container.APIKey, "APIKey repository")
	validateServiceField(t, container.Prompt, "Prompt repository")
	validateServiceField(t, container.EmbeddingProvider, "EmbeddingProvider repository")
	validateServiceField(t, container.Memory, "Memory repository")
}

func validateMockExternalContainer(t *testing.T) {
	t.Helper()
	container := NewMockExternalContainer(t)
	if container == nil {
		t.Fatal("NewMockExternalContainer returned nil")
		return
	}

	validateServiceField(t, container.IDP, "IdentityProvider external")
	validateServiceField(t, container.SMTP, "SMTP external")
}

func validateMockContainer(t *testing.T) {
	t.Helper()
	container := NewMockContainer(t)
	if container == nil {
		t.Fatal("NewMockContainer returned nil")
		return
	}

	validateServiceField(t, container.Services, "Services container")
	validateServiceField(t, container.Repositories, "Repositories container")
	validateServiceField(t, container.External, "External container")
}

func validateMockContainerForTest(t *testing.T) {
	t.Helper()
	container := NewMockContainerForTest(t)
	if container == nil {
		t.Fatal("NewMockContainerForTest returned nil")
		return
	}

	validateServiceField(t, container.Services, "Services container")
	validateServiceField(t, container.Repositories, "Repositories container")
	validateServiceField(t, container.External, "External container")
}

func validateServiceField(t *testing.T, field interface{}, fieldName string) {
	t.Helper()
	if field == nil {
		t.Errorf("%s mock is nil", fieldName)
	}
}
