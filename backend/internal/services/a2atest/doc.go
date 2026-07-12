// Package a2atest provides an in-process toy A2A agent and in-memory
// repository fakes for exercising VibeXP's agent-invocation stack end to end,
// against the real a2a-go wire protocol.
//
// The toy agent is built from the SDK's own server primitives
// (github.com/a2aproject/a2a-go/v2/a2asrv), so the client and server speak
// canonical A2A protocol v1.0 — a test then verifies VibeXP's orchestration and
// persistence, not its protocol code.
//
// # Usage
//
//	srv := a2atest.NewServer(t, a2atest.Script{
//	    Streaming: true,
//	    Events: func(ec *a2asrv.ExecutorContext) []a2a.Event {
//	        art := a2a.NewArtifactEvent(ec, a2a.NewTextPart("Hello "))
//	        return []a2a.Event{
//	            a2a.NewSubmittedTask(ec, nil),
//	            a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil),
//	            art,
//	            a2a.NewArtifactUpdateEvent(ec, art.Artifact.ID, a2a.NewTextPart("world")),
//	            a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil),
//	        }
//	    },
//	})
//	agent := srv.Agent("agent-1")
//	execStore := a2atest.NewExecutionStore()
//	eventStore := a2atest.NewEventStore()
//	// ...construct the real AgentInvocationService with these fakes + a client
//	// whose SSRF guard permits loopback (services.newA2AHTTPClient(..., &ssrfGuard{allowPrivate:true}))...
//	a2atest.Eventually(t, func() bool {
//	    e, _ := execStore.GetByID(context.Background(), agent.UserID, execID)
//	    return e != nil && e.Status == "success"
//	})
//
// # SSRF
//
// The toy agent listens on a loopback httptest.Server (127.0.0.1). VibeXP's
// production SSRF guard blocks loopback, so the invocation client under test
// must be built with the package-internal loopback exemption
// (services.newA2AHTTPClient with &ssrfGuard{allowPrivate: true}); there is no
// env/config switch. See services.A2AHTTPClient.
//
// The fakes are safe for concurrent use so tests can poll them (with Eventually)
// while the invocation service's background streaming goroutines write to them
// under the race detector.
package a2atest
