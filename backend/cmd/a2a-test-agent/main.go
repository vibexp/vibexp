// Command a2a-test-agent runs a minimal, local A2A (Agent-to-Agent) server that
// returns canned/dummy responses. It performs no model inference — it exists so
// the VibeXP e2e suite has a real, spec-compliant agent to register and chat
// with end to end (locally and in CI), instead of route-mocking the flow.
//
// It advertises a v0.3 agent card (the widely deployed A2A protocol version) and
// serves the JSON-RPC endpoint under both the classic v0.3 method names and the
// current-spec names, so it exercises the same v0.3 transport negotiation real
// agents require. It reuses the a2a-go SDK already vendored by the backend
// module, so it needs no extra dependency.
//
// Run: go run ./cmd/a2a-test-agent --port 9001 --host 127.0.0.1
package main

import (
	"context"
	"flag"
	"fmt"
	"iter"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	a2av0 "github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// dummyExecutor implements a2asrv.AgentExecutor. Instead of calling a model it
// echoes the caller's text in a deterministic message, so round-trips are easy
// to assert against from an e2e test.
type dummyExecutor struct{}

var _ a2asrv.AgentExecutor = (*dummyExecutor)(nil)

// Execute yields a single agent Message echoing the caller's text. Returning a
// direct *a2a.Message (rather than a task) is exactly the reply shape that
// exercises the streaming message-reply path in the VibeXP client.
func (*dummyExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		reply := "dummy response"
		if in := incomingText(execCtx); in != "" {
			reply = fmt.Sprintf("dummy response: you said %q", in)
		}
		yield(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(reply)), nil)
	}
}

// Cancel is a no-op: there is no long-running work to stop.
func (*dummyExecutor) Cancel(_ context.Context, _ *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(_ func(a2a.Event, error) bool) {}
}

// incomingText concatenates the text parts of the inbound message, if any.
func incomingText(execCtx *a2asrv.ExecutorContext) string {
	if execCtx == nil || execCtx.Message == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range execCtx.Message.Parts {
		if p != nil && p.Text() != "" {
			b.WriteString(p.Text())
		}
	}
	return strings.TrimSpace(b.String())
}

func main() {
	port := flag.Int("port", 9001, "Port to listen on")
	host := flag.String("host", "127.0.0.1", "Host/IP advertised in the agent card URL")
	flag.Parse()

	// The card advertises the v0.3-compat endpoint, since clients reading a v0.3
	// card speak the classic method names (message/send, message/stream). The
	// host must be one the A2A client (the VibeXP backend) can reach: 127.0.0.1
	// for a local run, or the compose service name inside the e2e docker network.
	cardURL := fmt.Sprintf("http://%s:%d/invoke/v0", *host, *port)

	agentCard := &a2a.AgentCard{
		Name:        "A2A Dummy Test Agent",
		Description: "A local, no-inference A2A agent that returns canned responses for testing A2A clients.",
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			{
				URL:             cardURL,
				ProtocolBinding: a2a.TransportProtocolJSONRPC,
				ProtocolVersion: a2av0.Version,
			},
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Skills: []a2a.AgentSkill{
			{
				ID:          "echo",
				Name:        "Echo dummy response",
				Description: "Replies with a deterministic dummy message that echoes the input text.",
				Tags:        []string{"test", "dummy", "echo"},
				Examples:    []string{"hi", "ping", "hello"},
			},
		},
	}

	requestHandler := a2asrv.NewHandler(&dummyExecutor{})

	mux := http.NewServeMux()
	// Current-spec JSON-RPC method names (SendMessage / SendStreamingMessage).
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(requestHandler))
	// Classic v0.3 method names (message/send, message/stream) — advertised.
	mux.Handle("/invoke/v0", a2av0.NewJSONRPCHandler(requestHandler))
	// Serve the card via the v0.3-compat producer so it carries the top-level
	// url / preferredTransport / protocolVersion fields v0.3 clients validate.
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewAgentCardHandler(a2av0.NewStaticAgentCardProducer(agentCard)))

	// Bind on all interfaces so the agent is reachable both on loopback (local
	// run) and by name across the e2e docker network. This is a throwaway test
	// agent that only ever serves canned responses — no sensitive surface.
	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr) // #nosec G102 -- throwaway e2e agent, binds all interfaces by design
	if err != nil {
		log.Fatalf("failed to bind to port %d: %v", *port, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("a2a-test-agent listening on :%d (card advertises %s)", *port, cardURL)
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
