package models

// The types in this file are INTERNAL DTOs and carry no JSON tags on purpose. The
// wire shape a provider sees is openAIChatCompletionsRequest/-Response in
// internal/services, and an HTTP response shape is the generated strict-server
// type. Tags here would invite marshaling CompletionRequest straight at a
// provider, which would silently send a body with no "model" field.

// CompletionMessage is one turn of a completion request. Role is the
// OpenAI-compatible role name ("system", "user" or "assistant"); the wire format
// of every provider type this backend supports uses those names verbatim.
type CompletionMessage struct {
	Role    string
	Content string
}

// CompletionRequest is the provider-agnostic shape of ONE non-streaming
// completion. Streaming, tool-calling and structured output are deliberately
// absent (#1069): a consumer that needs them extends this type together with
// every ModelProvider implementation, rather than reaching around the seam.
//
// Temperature is a pointer because 0 is a meaningful value that must be
// distinguishable from "leave it to the provider's default".
type CompletionRequest struct {
	Messages    []CompletionMessage
	MaxTokens   int
	Temperature *float64
}

// TokenUsage reports what the completion cost. Both fields are zero when the
// provider does not report usage — an OpenAI-compatible server is not required
// to, so a zero here means "unreported", never "free".
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// CompletionResponse is the first choice of a completion, flattened. FinishReason
// is passed through verbatim ("stop", "length", …) so a consumer can tell a
// truncated answer from a complete one.
//
// ProviderID and Model identify the provider row that answered. LLMService sets
// them, because only it knows which row "the team default" resolved to; a
// ModelProvider leaves them empty.
type CompletionResponse struct {
	Content      string
	FinishReason string
	Usage        TokenUsage
	ProviderID   string
	Model        string
}
