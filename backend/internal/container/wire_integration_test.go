package container_test

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/pkg/events"
)

// setupTestConfig creates a minimal test configuration for container initialization
func setupTestConfig() *config.Config {
	return &config.Config{
		// SMTP configuration
		Email: config.EmailConfig{
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "test@example.com",
				Password: "password",
			},
		},

		// Application configuration
		Frontend: config.FrontendConfig{
			BaseURL: "http://localhost:3000",
		},

		// Encryption configuration (optional)
		Security: config.SecurityConfig{
			EncryptionKey: "", // Empty to skip encryption service
		},

		// GCP project id (observability only; empty for tests)
		GCP: config.GCPConfig{
			ProjectID: "",
		},
	}
}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *database.DB {
	t.Helper()

	// Use in-memory SQLite for a real but fast database
	// This ensures the database is functional without requiring external dependencies
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to create in-memory SQLite database")

	// Verify the database connection works
	err = sqlDB.Ping()
	require.NoError(t, err, "Failed to ping in-memory database")

	return &database.DB{DB: sqlDB}
}

// setupTestLogger creates a test logger with minimal output
func setupTestLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// TestInitializeContainer_Success tests successful container initialization with valid configuration
func TestInitializeContainer_Success(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)

	// Assert
	require.NoError(t, err, "Container initialization should succeed with valid configuration")
	require.NotNil(t, c, "Container should not be nil")

	// Verify container can be closed without errors
	err = c.Close()
	assert.NoError(t, err, "Container Close() should not return an error")
}

// TestInitializeContainer_AllRepositoriesNonNil verifies all repository methods return non-nil values
func TestInitializeContainer_AllRepositoriesNonNil(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify all repositories are non-nil
	assert.NotNil(t, c.UserRepository(), "UserRepository should not be nil")
	assert.NotNil(t, c.APIKeyRepository(), "APIKeyRepository should not be nil")
	assert.NotNil(t, c.PromptRepository(), "PromptRepository should not be nil")
	assert.NotNil(t, c.PromptGalleryRepository(), "PromptGalleryRepository should not be nil")
	assert.NotNil(t, c.PromptShareRepository(), "PromptShareRepository should not be nil")
	assert.NotNil(t, c.ArtifactRepository(), "ArtifactRepository should not be nil")
	assert.NotNil(t, c.BlueprintRepository(), "BlueprintRepository should not be nil")
	assert.NotNil(t, c.EmbeddingProviderRepository(), "EmbeddingProviderRepository should not be nil")
	assert.NotNil(t, c.ActivityRepository(), "ActivityRepository should not be nil")
	assert.NotNil(t, c.ClaudeCodeHooksRepository(), "ClaudeCodeHooksRepository should not be nil")
	assert.NotNil(t, c.CursorIDEHooksRepository(), "CursorIDEHooksRepository should not be nil")
	assert.NotNil(t, c.AgentRepository(), "AgentRepository should not be nil")
	assert.NotNil(t, c.AgentExecutionRepository(), "AgentExecutionRepository should not be nil")
	assert.NotNil(t, c.AgentExecutionEventRepository(), "AgentExecutionEventRepository should not be nil")
	assert.NotNil(t, c.MemoryRepository(), "MemoryRepository should not be nil")
	assert.NotNil(t, c.EmbeddingRepository(), "EmbeddingRepository should not be nil")
	assert.NotNil(t, c.BackofficeRepository(), "BackofficeRepository should not be nil")
	assert.NotNil(t, c.FeedRepository(), "FeedRepository should not be nil")
	assert.NotNil(t, c.FeedItemRepository(), "FeedItemRepository should not be nil")
}

// TestInitializeContainer_AllServicesNonNil verifies all service methods return non-nil values
func TestInitializeContainer_AllServicesNonNil(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify all services are non-nil
	assert.NotNil(t, c.AuthService(), "AuthService should not be nil")
	assert.NotNil(t, c.APIKeyService(), "APIKeyService should not be nil")
	assert.NotNil(t, c.PromptService(), "PromptService should not be nil")
	assert.NotNil(t, c.PromptGalleryService(), "PromptGalleryService should not be nil")
	assert.NotNil(t, c.PromptShareService(), "PromptShareService should not be nil")
	assert.NotNil(t, c.ArtifactService(), "ArtifactService should not be nil")
	assert.NotNil(t, c.BlueprintService(), "BlueprintService should not be nil")
	assert.NotNil(t, c.EmbeddingProviderService(), "EmbeddingProviderService should not be nil")
	assert.NotNil(t, c.EmailService(), "EmailService should not be nil")
	assert.NotNil(t, c.ActivityService(), "ActivityService should not be nil")
	assert.NotNil(t, c.AgentService(), "AgentService should not be nil")
	assert.NotNil(t, c.AgentCardFetcher(), "AgentCardFetcher should not be nil")
	assert.NotNil(t, c.AgentInvocationService(), "AgentInvocationService should not be nil")
	assert.NotNil(t, c.MemoryService(), "MemoryService should not be nil")
	assert.NotNil(t, c.EmbeddingService(), "EmbeddingService should not be nil")
	assert.NotNil(t, c.EnvironmentService(), "EnvironmentService should not be nil")
	assert.NotNil(t, c.ResourceUsageService(), "ResourceUsageService should not be nil")
	assert.NotNil(t, c.BackofficeService(), "BackofficeService should not be nil")
	assert.NotNil(t, c.AdminService(), "AdminService should not be nil")
	assert.NotNil(t, c.FeedService(), "FeedService should not be nil")
	assert.NotNil(t, c.FeedItemService(), "FeedItemService should not be nil")
}

// TestInitializeContainer_AllExternalDependenciesNonNil verifies all external dependencies are properly initialized
func TestInitializeContainer_AllExternalDependenciesNonNil(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify all external dependencies are non-nil
	assert.NotNil(t, c.IdentityProviderRegistry(), "IdentityProviderRegistry should not be nil")
	assert.NotNil(t, c.EmailSender(), "EmailSender should not be nil")
}

// TestInitializeContainer_EventManagerNonNil verifies event manager is properly initialized
func TestInitializeContainer_EventManagerNonNil(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify event manager is non-nil and usable as EventPublisher
	eventPublisher := c.EventManager()
	assert.NotNil(t, eventPublisher, "EventManager should not be nil")
}

// TestInitializeContainer_DatabaseAccessible verifies database is accessible through container
func TestInitializeContainer_DatabaseAccessible(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify database is accessible (legacy method)
	containerDB := c.Database()
	assert.NotNil(t, containerDB, "Database should not be nil")
	assert.Equal(t, db, containerDB, "Container should return the same DB instance")
}

// TestInitializeContainer_CloseResourcesCleanup tests that Close() properly cleans up resources
func TestInitializeContainer_CloseResourcesCleanup(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)

	// Act
	err = c.Close()

	// Assert
	assert.NoError(t, err, "Close should not return an error")

	// Multiple Close() calls should be safe
	err = c.Close()
	assert.NoError(t, err, "Multiple Close() calls should be safe")
}

// TestInitializeContainer_WithEncryptionService tests container with encryption service enabled
func TestInitializeContainer_WithEncryptionService(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	cfg.Security.EncryptionKey = "testtesttesttesttesttesttesttest" // 32 bytes hex key
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)

	// Assert
	require.NoError(t, err, "Container initialization should succeed with encryption key")
	require.NotNil(t, c, "Container should not be nil")
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Verify agent service is still properly initialized with encryption
	assert.NotNil(t, c.AgentService(), "AgentService should be initialized with encryption service")
}

// TestInitializeContainer_EmbeddingWorker_BrokerFree verifies the container
// initializes the broker-free embedding path (in-process worker on the event bus)
// with no Pub/Sub or AI-service configuration.
func TestInitializeContainer_EmbeddingWorker_BrokerFree(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)

	// Assert
	require.NoError(t, err, "Container should initialize the in-process embedding worker")
	require.NotNil(t, c, "Container should not be nil")
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Verify event system is operational
	assert.NotNil(t, c.EventManager(), "EventManager should be initialized")
}

// runConditionalServiceTest is a helper to test conditional service initialization
func runConditionalServiceTest(
	t *testing.T,
	name string,
	setupCfg func(*config.Config),
	assertions func(*testing.T, container.Container),
) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		db := setupTestDB(t)
		cfg := setupTestConfig()
		setupCfg(cfg)
		logger := setupTestLogger()

		c, err := container.InitializeContainer(db, cfg, logger)
		require.NoError(t, err, "Container initialization should succeed")
		require.NotNil(t, c, "Container should not be nil")
		defer func() {
			assert.NoError(t, c.Close())
		}()

		assertions(t, c)
	})
}

// TestInitializeContainer_ConditionalServices tests services with conditional initialization
func TestInitializeContainer_ConditionalServices(t *testing.T) {
	runConditionalServiceTest(t, "Without encryption key",
		func(cfg *config.Config) {
			cfg.Security.EncryptionKey = ""
		},
		func(t *testing.T, c container.Container) {
			assert.NotNil(t, c.AgentService(), "AgentService should be initialized without encryption")
		})

	runConditionalServiceTest(t, "With encryption key",
		func(cfg *config.Config) {
			cfg.Security.EncryptionKey = "testtesttesttesttesttesttesttest"
		},
		func(t *testing.T, c container.Container) {
			assert.NotNil(t, c.AgentService(), "AgentService should be initialized with encryption")
		})
}

// TestInitializeContainer_EventSystemIntegration tests event system integration and event types
func TestInitializeContainer_EventSystemIntegration(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify event manager can be used as EventPublisher interface
	eventPublisher := c.EventManager()
	assert.NotNil(t, eventPublisher, "Event manager should implement EventPublisher interface")

	// Verify event manager has started (it should be running)
	// We can't directly test if it's started, but we verified it's not nil
	// The actual event publishing is tested in the event system's own tests
}

// TestInitializeContainer_ServiceDependencies tests that services receive all required dependencies
func TestInitializeContainer_ServiceDependencies(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Test services that have complex dependencies

	// AuthService depends on UserRepository, Config, IdentityProvider, EventPublisher, Logger
	authService := c.AuthService()
	assert.NotNil(t, authService, "AuthService should be properly wired")

	// PromptService depends on PromptRepository, PromptReferenceRepository, UserRepository, EventPublisher, Logger
	promptService := c.PromptService()
	assert.NotNil(t, promptService, "PromptService should be properly wired")

	// ArtifactService depends on ArtifactRepository, EventPublisher, ResourceUsageService, Logger
	artifactService := c.ArtifactService()
	assert.NotNil(t, artifactService, "ArtifactService should be properly wired")

	// AgentInvocationService has complex dependencies
	agentInvocationService := c.AgentInvocationService()
	assert.NotNil(t, agentInvocationService, "AgentInvocationService should be properly wired")
}

// TestInitializeContainer_NoCircularDependencies verifies Wire prevents circular dependencies
func TestInitializeContainer_NoCircularDependencies(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)

	// Assert
	// If there were circular dependencies, Wire would fail at compile time
	// This test ensures the container can be created, proving no circular dependencies exist
	require.NoError(t, err, "Container should initialize without circular dependency errors")
	require.NotNil(t, c, "Container should not be nil")
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Verify all major service groups are initialized
	assert.NotNil(t, c.UserRepository(), "Repository layer initialized")
	assert.NotNil(t, c.AuthService(), "Service layer initialized")
	assert.NotNil(t, c.EventManager(), "Event system initialized")
	assert.NotNil(t, c.IdentityProviderRegistry(), "External dependencies initialized")
}

// TestInitializeContainer_ResourceUsageService tests resource usage service with nil handling
func TestInitializeContainer_ResourceUsageService(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert
	// ResourceUsageService should handle scenarios where DB might be unavailable
	// But in our case, it should be properly initialized
	resourceUsageService := c.ResourceUsageService()
	assert.NotNil(t, resourceUsageService, "ResourceUsageService should be initialized")
}

// TestInitializeContainer_EventPublishing_SmokeTest verifies event manager can publish events
func TestInitializeContainer_EventPublishing_SmokeTest(t *testing.T) {
	// Arrange
	db := setupTestDB(t)
	cfg := setupTestConfig()
	logger := setupTestLogger()

	// Act
	c, err := container.InitializeContainer(db, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer func() {
		assert.NoError(t, c.Close())
	}()

	// Assert - Verify event manager can accept and process events (smoke test)
	eventPublisher := c.EventManager()
	require.NotNil(t, eventPublisher, "EventManager should not be nil")

	// Create a test event using the proper event factory
	testEvent := events.NewUserCreatedEvent(
		"test-user-123",
		"test@example.com",
		"Test User",
		time.Now(),
	)

	// Publish the event - should not return error even if no listeners are interested
	err = eventPublisher.Publish(context.Background(), testEvent)
	assert.NoError(t, err, "Event manager should accept events without error")
}

// Note: Tests for nil database, config, or logger are intentionally omitted
// as the container is designed to require these dependencies and will panic
// if they are missing. This is acceptable fail-fast behavior for dependency injection.
