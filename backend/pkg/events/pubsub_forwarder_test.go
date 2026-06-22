package events

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"cloud.google.com/go/pubsub" //nolint:staticcheck // v2 has breaking changes, will upgrade when stable
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type pubsubTestHelper struct {
	srv    *pstest.Server
	conn   *grpc.ClientConn
	client *pubsub.Client
	t      *testing.T
}

func setupPubSubTestHelper(t *testing.T) *pubsubTestHelper {
	t.Helper()
	srv := pstest.NewServer()

	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		if closeErr := srv.Close(); closeErr != nil {
			t.Logf("Failed to close server: %v", closeErr)
		}
		t.Fatal(err)
	}

	client, err := pubsub.NewClient(context.Background(), "test-project", option.WithGRPCConn(conn))
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("Failed to close connection: %v", closeErr)
		}
		if closeErr := srv.Close(); closeErr != nil {
			t.Logf("Failed to close server: %v", closeErr)
		}
		t.Fatal(err)
	}

	return &pubsubTestHelper{
		srv:    srv,
		conn:   conn,
		client: client,
		t:      t,
	}
}

func (h *pubsubTestHelper) cleanup() {
	if h.client != nil {
		if err := h.client.Close(); err != nil {
			h.t.Logf("Failed to close client: %v", err)
		}
	}
	if h.conn != nil {
		if err := h.conn.Close(); err != nil {
			h.t.Logf("Failed to close connection: %v", err)
		}
	}
	if h.srv != nil {
		if err := h.srv.Close(); err != nil {
			h.t.Logf("Failed to close server: %v", err)
		}
	}
}

func TestPubSubForwarderListener_RequiresClient(t *testing.T) {
	_, err := NewPubSubForwarderListener(PubSubForwarderConfig{
		Client:     nil,
		TopicName:  "test-topic",
		EventTypes: []string{"test.event"},
		Logger:     slog.New(slog.DiscardHandler),
	})

	if err == nil {
		t.Error("Expected error when client is nil")
	}
	if err.Error() != "pubsub client is required" {
		t.Errorf("Expected 'pubsub client is required' error, got: %v", err)
	}
}

func TestPubSubForwarderListener_RequiresTopicName(t *testing.T) {
	helper := setupPubSubTestHelper(t)
	defer helper.cleanup()

	_, err := NewPubSubForwarderListener(PubSubForwarderConfig{
		Client:     helper.client,
		TopicName:  "",
		EventTypes: []string{"test.event"},
		Logger:     slog.New(slog.DiscardHandler),
	})

	if err == nil {
		t.Error("Expected error when topic name is empty")
	}
	if err.Error() != "topic name is required" {
		t.Errorf("Expected 'topic name is required' error, got: %v", err)
	}
}

func TestPubSubForwarderListener_CreatesSuccessfully(t *testing.T) {
	helper := setupPubSubTestHelper(t)
	defer helper.cleanup()

	topic, err := helper.client.CreateTopic(context.Background(), "test-topic")
	if err != nil {
		t.Fatal(err)
	}

	forwarder, err := NewPubSubForwarderListener(PubSubForwarderConfig{
		Client:       helper.client,
		TopicName:    topic.ID(),
		EventTypes:   []string{"test.event", "another.event"},
		Logger:       slog.New(slog.DiscardHandler),
		PublishAsync: false,
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if forwarder == nil {
		t.Fatal("Expected forwarder to be created")
	}

	eventTypes := forwarder.EventTypes()
	if len(eventTypes) != 2 {
		t.Errorf("Expected 2 event types, got %d", len(eventTypes))
	}
}

func TestPubSubForwarder_ForwardsEventsSuccessfully(t *testing.T) {
	helper := setupPubSubTestHelper(t)
	defer helper.cleanup()

	ctx := context.Background()
	topic, err := helper.client.CreateTopic(ctx, "test-topic")
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.DiscardHandler)

	forwarder, err := NewPubSubForwarderListener(PubSubForwarderConfig{
		Client:       helper.client,
		TopicName:    topic.ID(),
		EventTypes:   []string{"user.created"},
		Logger:       logger,
		PublishAsync: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	event := NewUserCreatedEvent("user123", "test@example.com", "Test User", time.Now())
	err = forwarder.Handle(ctx, event)
	if err != nil {
		t.Errorf("Expected no error handling event, got: %v", err)
	}

	messages := helper.srv.Messages()
	if len(messages) == 0 {
		t.Error("Expected at least one message to be published")
	}

	if len(messages) > 0 {
		msg := messages[0]
		if msg.Attributes["eventType"] != "user.created" {
			t.Errorf("Expected eventType=user.created, got: %s", msg.Attributes["eventType"])
		}
		if msg.Attributes["event_type"] != "user.created" {
			t.Errorf("Expected event_type=user.created, got: %s", msg.Attributes["event_type"])
		}
		if msg.Attributes["user_id"] != "user123" {
			t.Errorf("Expected user_id=user123, got: %s", msg.Attributes["user_id"])
		}
	}
}
