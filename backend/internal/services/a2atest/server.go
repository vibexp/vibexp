package a2atest

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	a2av0 "github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/vibexp/vibexp/internal/models"
)

// Script configures the behavior of a toy A2A agent for a single test.
//
// Events, given the SDK ExecutorContext, returns the ordered events the agent
// yields for each incoming message. For a streaming card, emit a task/status/
// artifact lifecycle; for a sync card, a single message or a terminal task is
// enough. See the package example.
type Script struct {
	// Streaming controls the advertised card capability (Capabilities.Streaming).
	Streaming bool
	// ProtocolV03 advertises the card and serves JSON-RPC in the A2A v0.3 (compat)
	// shape instead of the current v1.0, so tests can exercise v0.3 transport
	// negotiation (mirrors backend/cmd/a2a-test-agent).
	ProtocolV03 bool
	// Events returns the ordered events to yield for an incoming message.
	Events func(execCtx *a2asrv.ExecutorContext) []a2a.Event
	// Err, when set, is yielded as the executor's error before any events
	// (simulates an agent-side failure surfaced as a JSON-RPC error).
	Err error
	// Delay is slept before yielding each event, simulating a slow agent. This
	// is the toy agent being slow on the wire, not a test-flow sleep.
	Delay time.Duration
	// RequireAuth, when non-empty, makes the agent reject any request whose
	// Authorization header does not match with HTTP 401.
	RequireAuth string
	// ForceStatus, when > 0, makes the agent reply with that HTTP status for
	// every invoke request (e.g. 500) instead of running the executor.
	ForceStatus int
	// DropAfter, when > 0, abruptly closes the connection after that many SSE
	// flushes, simulating a mid-stream connection drop.
	DropAfter int
	// OnMessage, when set, is invoked with each received message for
	// server-side assertions (e.g. context-id continuity).
	OnMessage func(msg *a2a.Message)
}

// Recorder captures what the toy agent received, for server-side assertions.
type Recorder struct {
	mu       sync.Mutex
	auth     []string
	messages []*a2a.Message
}

func (r *Recorder) recordAuth(h string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auth = append(r.auth, h)
}

func (r *Recorder) recordMessage(m *a2a.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, m)
}

// AuthHeaders returns every Authorization header the agent received, in order.
func (r *Recorder) AuthHeaders() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.auth...)
}

// LastAuthHeader returns the most recent Authorization header, or "".
func (r *Recorder) LastAuthHeader() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.auth) == 0 {
		return ""
	}
	return r.auth[len(r.auth)-1]
}

// Messages returns every message the agent received, in order.
func (r *Recorder) Messages() []*a2a.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*a2a.Message(nil), r.messages...)
}

// Server is an in-process toy A2A agent served over JSON-RPC on loopback.
type Server struct {
	*httptest.Server
	// Endpoint is the JSON-RPC invoke URL the agent card advertises.
	Endpoint string
	// Recorder captures received auth headers and messages.
	Recorder *Recorder

	streaming   bool
	protocolV03 bool
}

// NewServer starts an in-process JSON-RPC A2A agent backed by script and
// registers cleanup on t. The agent card is also served at the well-known path.
func NewServer(t testing.TB, script Script) *Server {
	t.Helper()

	rec := &Recorder{}
	exec := &scriptedExecutor{script: script, rec: rec}
	handler := a2asrv.NewHandler(exec)
	// v0.3 clients speak the classic JSON-RPC method names, so serve the compat
	// handler when the card advertises v0.3 (mirrors backend/cmd/a2a-test-agent).
	jsonrpc := a2asrv.NewJSONRPCHandler(handler)
	if script.ProtocolV03 {
		jsonrpc = a2av0.NewJSONRPCHandler(handler)
	}

	card := &cardHandler{}
	mux := http.NewServeMux()
	mux.Handle("/invoke", withGateways(rec, script, jsonrpc))
	mux.Handle(a2asrv.WellKnownAgentCardPath, card)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	endpoint := srv.URL + "/invoke"
	card.set(buildCard(endpoint, script.Streaming, script.ProtocolV03))

	return &Server{
		Server: srv, Endpoint: endpoint, Recorder: rec,
		streaming: script.Streaming, protocolV03: script.ProtocolV03,
	}
}

// Card returns the agent card advertised by this server.
func (s *Server) Card() *models.AgentCard {
	return buildCard(s.Endpoint, s.streaming, s.protocolV03)
}

// Agent returns an active agent whose card points at this server. Tests may
// further customize the returned agent (e.g. add security schemes + credentials)
// before handing it to a fake AgentRepository.
func (s *Server) Agent(id string) *models.Agent {
	return &models.Agent{
		ID:        id,
		UserID:    "user-" + id,
		Status:    "active",
		AgentCard: s.Card(),
	}
}

// scriptedExecutor is a configurable a2asrv.AgentExecutor.
type scriptedExecutor struct {
	script Script
	rec    *Recorder
}

func (e *scriptedExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	e.observe(execCtx)
	return func(yield func(a2a.Event, error) bool) {
		if e.script.Err != nil {
			yield(nil, e.script.Err)
			return
		}
		if e.script.Events == nil {
			return
		}
		for _, ev := range e.script.Events(execCtx) {
			if e.script.Delay > 0 {
				time.Sleep(e.script.Delay) // toy agent latency, not a test-flow sleep
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// observe records the incoming message and invokes the OnMessage hook, for
// server-side assertions (auth, context-id continuity).
func (e *scriptedExecutor) observe(execCtx *a2asrv.ExecutorContext) {
	if execCtx.Message == nil {
		return
	}
	e.rec.recordMessage(execCtx.Message)
	if e.script.OnMessage != nil {
		e.script.OnMessage(execCtx.Message)
	}
}

func (e *scriptedExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

// withGateways records the Authorization header, optionally enforces auth /
// forces an HTTP status, and optionally drops the connection mid-stream.
func withGateways(rec *Recorder, script Script, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.recordAuth(r.Header.Get("Authorization"))

		if script.ForceStatus > 0 {
			http.Error(w, http.StatusText(script.ForceStatus), script.ForceStatus)
			return
		}
		if script.RequireAuth != "" && r.Header.Get("Authorization") != script.RequireAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if script.DropAfter > 0 {
			w = &dropWriter{ResponseWriter: w, dropAfter: script.DropAfter}
		}
		next.ServeHTTP(w, r)
	})
}

// buildCard builds a minimal valid agent card advertising endpoint, in either
// the current v1.0 shape (default) or the v0.3-compat shape when protocolV03.
func buildCard(endpoint string, streaming, protocolV03 bool) *models.AgentCard {
	iface := a2a.NewAgentInterface(endpoint, a2a.TransportProtocolJSONRPC)
	if protocolV03 {
		iface = &a2a.AgentInterface{
			URL:             endpoint,
			ProtocolBinding: a2a.TransportProtocolJSONRPC,
			ProtocolVersion: a2av0.Version,
		}
	}
	return &models.AgentCard{
		Name:                "Toy Agent",
		Description:         "in-process a2atest agent",
		Version:             "1.0.0",
		Capabilities:        a2a.AgentCapabilities{Streaming: streaming},
		DefaultInputModes:   []string{"text/plain"},
		DefaultOutputModes:  []string{"text/plain"},
		SupportedInterfaces: []*a2a.AgentInterface{iface},
		Skills:              []a2a.AgentSkill{{ID: "s", Name: "s", Description: "s", Tags: []string{"t"}}},
	}
}

// cardHandler serves an agent card that is only known after the server starts
// (its URL embeds the listen address).
type cardHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (c *cardHandler) set(card *a2a.AgentCard) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.h = a2asrv.NewStaticAgentCardHandler(card)
}

func (c *cardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	h := c.h
	c.mu.RUnlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

var errConnDropped = errors.New("a2atest: connection dropped mid-stream")

// dropWriter closes the underlying TCP connection after dropAfter SSE flushes,
// simulating a mid-stream connection drop. The SSE writer type-asserts the
// ResponseWriter to http.Flusher, so counting flushes here is reliable.
type dropWriter struct {
	http.ResponseWriter
	dropAfter int
	flushes   int
	dropped   bool
}

func (w *dropWriter) Flush() {
	if w.dropped {
		return
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	w.flushes++
	if w.flushes >= w.dropAfter {
		if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				if cerr := conn.Close(); cerr != nil {
					_ = cerr // best-effort: the connection is being torn down deliberately
				}
			}
		}
		w.dropped = true
	}
}

func (w *dropWriter) Write(b []byte) (int, error) {
	if w.dropped {
		return 0, errConnDropped
	}
	return w.ResponseWriter.Write(b)
}
