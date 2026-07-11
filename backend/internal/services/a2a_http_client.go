package services

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

// destroyClient releases the SDK client's resources, logging any error since it
// is not actionable at teardown.
func destroyClient(client *a2aclient.Client) {
	if err := client.Destroy(); err != nil {
		slog.With("service", "a2a-client", "error", err).Warn("failed to destroy A2A client")
	}
}

const (
	// DefaultA2ATimeout is the default timeout for A2A requests (5 minutes)
	DefaultA2ATimeout = 5 * time.Minute
)

// A2AHTTPClientInterface defines the interface for A2A communication. Outbound
// traffic goes through the official a2a-go SDK client (protocol v1.0 with v0.x
// negotiation); events are the SDK's typed a2a.Event union.
type A2AHTTPClientInterface interface {
	InvokeAgent(
		ctx context.Context, agent *models.Agent, input map[string]interface{}, contextID *string,
	) (*models.AgentExecution, error)
	InvokeAgentStreaming(
		ctx context.Context, agent *models.Agent, input map[string]interface{},
		contextID *string, eventChan chan<- a2a.Event,
	) error
	SupportsStreaming(agent *models.Agent) bool
}

// A2AHTTPClient talks to remote A2A agents through the official a2a-go SDK,
// layering VibeXP's SSRF guard and encrypted-credential authentication onto the
// SDK's transport.
type A2AHTTPClient struct {
	authenticator *AgentAuthenticator
	timeout       time.Duration
	guard         *ssrfGuard
	baseTransport http.RoundTripper
}

// NewA2AHTTPClient creates a new A2A client. The shared base transport uses an
// SSRF-safe dialer that rejects connections to reserved IP ranges at connect
// time, defeating DNS rebinding on top of endpoint validation.
func NewA2AHTTPClient(authenticator *AgentAuthenticator, cfg *config.Config) *A2AHTTPClient {
	return newA2AHTTPClient(authenticator, cfg, defaultSSRFGuard)
}

// newA2AHTTPClient builds a client around the supplied SSRF guard. Tests use a
// private-allowing guard to target loopback httptest servers.
func newA2AHTTPClient(authenticator *AgentAuthenticator, cfg *config.Config, guard *ssrfGuard) *A2AHTTPClient {
	timeout := DefaultA2ATimeout
	if cfg != nil && cfg.A2A.DefaultTimeout > 0 {
		timeout = cfg.A2A.DefaultTimeout
	}

	transport := guard.newSSRFSafeTransport(&http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	})

	return &A2AHTTPClient{
		authenticator: authenticator,
		timeout:       timeout,
		guard:         guard,
		baseTransport: transport,
	}
}

// agentAuthRoundTripper applies an agent's stored credentials to every outbound
// A2A request (apiKey header/query/cookie, http bearer/basic, prefix detection)
// on top of the SSRF-safe base transport. This reuses AgentAuthenticator so the
// SDK client authenticates exactly as before — including query/cookie schemes
// that the SDK's header-only ServiceParams/interceptors cannot express.
type agentAuthRoundTripper struct {
	base          http.RoundTripper
	authenticator *AgentAuthenticator
	agent         *models.Agent
}

func (rt *agentAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone so we never mutate a request the SDK may retain/reuse.
	cloned := req.Clone(req.Context())
	if err := rt.authenticator.ApplyAuthentication(cloned, rt.agent); err != nil {
		return nil, fmt.Errorf("failed to apply authentication: %w", err)
	}
	return rt.base.RoundTrip(cloned)
}

// buildClient constructs a per-agent SDK client whose transport carries the SSRF
// guard and the agent's credentials. The dial-time SSRF Control hook is the
// authoritative guard; the pre-flight host check just yields a clearer error.
func (c *A2AHTTPClient) buildClient(ctx context.Context, agent *models.Agent) (*a2aclient.Client, error) {
	if agent.AgentCard == nil {
		return nil, fmt.Errorf("agent card is missing")
	}
	if len(agent.AgentCard.SupportedInterfaces) == 0 {
		return nil, fmt.Errorf("agent card has no supported interfaces")
	}
	for _, iface := range agent.AgentCard.SupportedInterfaces {
		if iface == nil || iface.URL == "" {
			continue
		}
		if err := c.guard.validateOutboundHost(ctx, iface.URL); err != nil {
			return nil, fmt.Errorf("agent endpoint is not allowed: %w", err)
		}
	}

	httpClient := &http.Client{
		Timeout: c.timeout,
		Transport: &agentAuthRoundTripper{
			base:          c.baseTransport,
			authenticator: c.authenticator,
			agent:         agent,
		},
	}

	client, err := a2aclient.NewFromCard(ctx, agent.AgentCard,
		a2aclient.WithJSONRPCTransport(httpClient),
		a2aclient.WithRESTTransport(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build A2A client: %w", err)
	}
	return client, nil
}

// buildA2AMessage builds a v1.0 user message from the invocation input,
// threading the conversation contextId when continuing a conversation.
func buildA2AMessage(input map[string]interface{}, contextID *string) *a2a.Message {
	text, ok := input["text"].(string)
	if !ok {
		text = fmt.Sprintf("%v", input)
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	if contextID != nil && *contextID != "" {
		msg.ContextID = *contextID
	}
	return msg
}

func newSendMessageRequest(input map[string]interface{}, contextID *string) *a2a.SendMessageRequest {
	return &a2a.SendMessageRequest{
		Message: buildA2AMessage(input, contextID),
		Config: &a2a.SendMessageConfig{
			AcceptedOutputModes: []string{"text/plain"},
		},
	}
}

// mapTaskStateToStatus maps an A2A task state to a VibeXP execution status.
// Non-terminal states are reported as "working"; #163 adds task polling and
// #164 finalizes the cancelled flow.
func mapTaskStateToStatus(state a2a.TaskState) string {
	switch state {
	case a2a.TaskStateCompleted:
		return "completed"
	case a2a.TaskStateFailed, a2a.TaskStateRejected:
		return "failed"
	case a2a.TaskStateCanceled:
		return "cancelled"
	default:
		// submitted / working / input-required / auth-required — accepted, still running
		return "working"
	}
}

// mapSendResultToExecution turns the SDK's SendMessage result (a *a2a.Message or
// *a2a.Task) into an execution snapshot. Persisting the reply body itself is #163.
func mapSendResultToExecution(result a2a.SendMessageResult, duration time.Duration) *models.AgentExecution {
	execution := &models.AgentExecution{
		Status:   "completed",
		Duration: intPtr(int(duration.Milliseconds())),
	}

	task, ok := result.(*a2a.Task)
	if !ok {
		// A direct *a2a.Message reply — the agent answered synchronously.
		return execution
	}

	if id := string(task.ID); id != "" {
		execution.TaskID = &id
	}
	if task.ContextID != "" {
		execution.ContextID = &task.ContextID
	}
	state := string(task.Status.State)
	execution.CurrentState = &state
	execution.Status = mapTaskStateToStatus(task.Status.State)
	return execution
}

// InvokeAgent sends a message to an A2A agent synchronously and returns the result.
func (c *A2AHTTPClient) InvokeAgent(
	ctx context.Context,
	agent *models.Agent,
	input map[string]interface{},
	contextID *string,
) (*models.AgentExecution, error) {
	client, err := c.buildClient(ctx, agent)
	if err != nil {
		return nil, err
	}
	defer destroyClient(client)

	start := time.Now()
	result, err := client.SendMessage(ctx, newSendMessageRequest(input, contextID))
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("agent message send failed: %w", err)
	}

	return mapSendResultToExecution(result, duration), nil
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}

// SupportsStreaming checks if the agent supports streaming based on card capabilities.
func (c *A2AHTTPClient) SupportsStreaming(agent *models.Agent) bool {
	if agent.AgentCard == nil {
		return false
	}
	return agent.AgentCard.Capabilities.Streaming
}

// InvokeAgentStreaming sends a message with streaming and forwards each typed SDK
// event to eventChan. The SDK auto-falls back to a unary send for non-streaming
// cards. The caller owns closing eventChan.
func (c *A2AHTTPClient) InvokeAgentStreaming(
	ctx context.Context,
	agent *models.Agent,
	input map[string]interface{},
	contextID *string,
	eventChan chan<- a2a.Event,
) error {
	client, err := c.buildClient(ctx, agent)
	if err != nil {
		return err
	}
	defer destroyClient(client)

	for event, streamErr := range client.SendStreamingMessage(ctx, newSendMessageRequest(input, contextID)) {
		if streamErr != nil {
			return fmt.Errorf("agent streaming failed: %w", streamErr)
		}
		select {
		case eventChan <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
