// Command llm-test-provider runs a minimal, deterministic OpenAI-compatible
// model provider. It performs no inference — it exists so the VibeXP e2e suite
// can drive AI Summary end to end (backend → provider → UI) without contacting
// a real LLM from CI (issue #1080).
//
// It serves exactly what the backend calls:
//
//   - GET  {base}/models           → one model, "e2e-stub-model"
//   - POST {base}/chat/completions → a fixed completion that cites the first
//     document it was given and echoes the style and document count it
//     received, so a spec can read the effect of a settings change in the UI
//   - GET  /healthz                → the compose healthcheck
//
// The bearer key "e2e-bad-key" is answered with 401 on both provider routes,
// so a spec drives the unauthorized path by editing the provider's key rather
// than by flipping shared state that parallel or retried tests could race.
//
// Run: go run ./cmd/llm-test-provider --port 9002
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	// stubModel is the only model the stub advertises.
	stubModel = "e2e-stub-model"
	// badKey is the API key the stub rejects with 401.
	badKey = "e2e-bad-key"
	// maxRequestBytes bounds a chat/completions body; the summary context is
	// capped far below this by the backend's own ai_summary budget.
	maxRequestBytes = 4 << 20
)

// styleMarkers maps a phrase of each style instruction the backend appends to
// its system prompt (services.summaryStyleInstructions) to the style's id.
var styleMarkers = []struct{ phrase, style string }{
	{"be concise", "concise"},
	{"be thorough", "detailed"},
	{"short paragraph", "balanced"},
}

// documentIndex matches the per-document delimiter the backend writes
// (services.buildSummaryMessages): <document index="N">.
var documentIndex = regexp.MustCompile(`<document index="(\d+)">`)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// newHandler builds the stub's routes. delay is slept before every completion
// so a spec can observe the loading skeleton deterministically.
func newHandler(delay time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if rejected(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   []map[string]string{{"id": stubModel, "object": "model"}},
		})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if rejected(w, r) {
			return
		}
		var req chatRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if len(req.Messages) == 0 {
			writeError(w, http.StatusBadRequest, "messages are required")
			return
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		model := req.Model
		if model == "" {
			model = stubModel
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "chatcmpl-e2e",
			"object": "chat.completion",
			"model":  model,
			"choices": []map[string]any{{
				"index":         0,
				"message":       chatMessage{Role: "assistant", Content: summarize(req.Messages)},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	})
	return mux
}

// rejected answers 401 for the bad key and reports whether it did.
func rejected(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer "+badKey {
		return false
	}
	writeError(w, http.StatusUnauthorized, "invalid api key")
	return true
}

// summarize builds the deterministic answer: it cites the first document it
// was given and reports the style and document count it received.
func summarize(messages []chatMessage) string {
	style := "unknown"
	var docs [][]string
	for _, m := range messages {
		switch m.Role {
		case "system":
			for _, marker := range styleMarkers {
				if strings.Contains(m.Content, marker.phrase) {
					style = marker.style
					break
				}
			}
		case "user":
			docs = append(docs, documentIndex.FindAllStringSubmatch(m.Content, -1)...)
		}
	}
	citation := ""
	if len(docs) > 0 {
		citation = fmt.Sprintf(" [%s]", docs[0][1])
	}
	return fmt.Sprintf("E2E stub summary of the matching documents%s.\n\nStyle: %s · Documents: %d",
		citation, style, len(docs))
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func main() {
	port := flag.Int("port", 9002, "Port to listen on")
	delay := flag.Duration("completion-delay", 0, "Delay before answering each chat completion")
	flag.Parse()

	// Bind on all interfaces so the stub is reachable by name across the e2e
	// docker network. It only ever serves canned responses.
	addr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", addr) // #nosec G102 -- throwaway e2e stub, binds all interfaces by design
	if err != nil {
		log.Fatalf("failed to bind to port %d: %v", *port, err)
	}

	srv := &http.Server{
		Handler:           newHandler(*delay),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	log.Printf("llm-test-provider listening on :%d", *port)
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
