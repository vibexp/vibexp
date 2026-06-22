package services_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

func newNullLogger() *slog.Logger {
	l, _ := logtest.New()
	return l
}

// TestParseEmbeddingChunks exercises ParseEmbeddingChunks directly.
//
//nolint:funlen // Test function naturally requires multiple test cases for comprehensive validation
func TestParseEmbeddingChunks(t *testing.T) {
	validChunk := map[string]interface{}{
		"embedding": []interface{}{float64(0.1), float64(0.2), float64(0.3)},
		"content":   "hello",
	}

	tests := []struct {
		name        string
		input       []interface{}
		wantLen     int
		wantErr     bool
		errContains string
	}{
		{
			name:    "nil input returns empty result",
			input:   nil,
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "empty array returns empty result",
			input:   []interface{}{},
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "single valid chunk",
			input:   []interface{}{validChunk},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "multiple valid chunks",
			input: []interface{}{
				validChunk,
				map[string]interface{}{
					"embedding": []interface{}{float64(1.0)},
					"content":   "world",
				},
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name:        "non-map entry at index 0",
			input:       []interface{}{"not a map"},
			wantErr:     true,
			errContains: "embedding at index 0 is not a map",
		},
		{
			name: "non-map entry at index 1",
			input: []interface{}{
				validChunk,
				"not a map",
			},
			wantErr:     true,
			errContains: "embedding at index 1 is not a map",
		},
		{
			name: "missing embedding field",
			input: []interface{}{
				map[string]interface{}{"content": "no embedding"},
			},
			wantErr:     true,
			errContains: "embedding at index 0 missing 'embedding' field",
		},
		{
			name: "embedding field is not an array",
			input: []interface{}{
				map[string]interface{}{"embedding": "not an array"},
			},
			wantErr:     true,
			errContains: "embedding at index 0 missing 'embedding' field",
		},
		{
			name: "non-float64 value inside embedding array",
			input: []interface{}{
				map[string]interface{}{
					"embedding": []interface{}{"not a float"},
				},
			},
			wantErr:     true,
			errContains: "embedding value at index 0,0 is not a number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks, err := services.ParseEmbeddingChunks(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, chunks)
			} else {
				assert.NoError(t, err)
				assert.Len(t, chunks, tt.wantLen)
			}
		})
	}
}

// TestEmbeddingHandlerAdapter_HandleEmbedding exercises HandleEmbedding.
//
//nolint:funlen // Test function naturally requires multiple test cases for comprehensive validation
func TestEmbeddingHandlerAdapter_HandleEmbedding(t *testing.T) {
	validEmbeddingPayload := func() map[string]interface{} {
		return map[string]interface{}{
			"userID":   "user-123",
			"model":    "text-embedding-ada-002",
			"promptID": "prompt-456",
			"embeddings": []interface{}{
				map[string]interface{}{
					"embedding": []interface{}{float64(0.1), float64(0.2)},
					"content":   "test content",
				},
			},
		}
	}

	tests := []struct {
		name        string
		entityType  string
		payload     map[string]interface{}
		setupMock   func(*mocks.MockEmbeddingServiceInterface)
		wantErr     bool
		errContains string
	}{
		{
			name:       "happy path: valid prompt embedding",
			entityType: "prompt",
			payload:    validEmbeddingPayload(),
			setupMock: func(m *mocks.MockEmbeddingServiceInterface) {
				m.EXPECT().SaveEmbeddingChunks(
					"user-123", "prompt", "prompt-456", "text-embedding-ada-002",
					mock.MatchedBy(func(chunks []services.EmbeddingChunk) bool {
						return len(chunks) == 1 &&
							len(chunks[0].Embedding) == 2 &&
							chunks[0].Content == "test content"
					}),
				).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name:        "unknown entity type returns error",
			entityType:  "unknown",
			payload:     validEmbeddingPayload(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "unsupported entity type: unknown",
		},
		{
			name:       "missing userID returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				delete(p, "userID")
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name:       "empty userID returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["userID"] = ""
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name:       "missing entity ID field (promptID) returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				delete(p, "promptID")
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name:       "missing model returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				delete(p, "model")
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name:       "missing embeddings key returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				delete(p, "embeddings")
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "embeddings field is missing or not an array",
		},
		{
			name:       "embeddings not an array returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["embeddings"] = "not an array"
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "embeddings field is missing or not an array",
		},
		{
			name:       "malformed chunk: non-map entry returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["embeddings"] = []interface{}{"not a map"}
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "failed to parse embeddings",
		},
		{
			name:       "malformed chunk: missing embedding field returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["embeddings"] = []interface{}{
					map[string]interface{}{"content": "no embedding field"},
				}
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "failed to parse embeddings",
		},
		{
			name:       "malformed chunk: non-float64 value returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["embeddings"] = []interface{}{
					map[string]interface{}{
						"embedding": []interface{}{"not a float"},
					},
				}
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "failed to parse embeddings",
		},
		{
			name:       "empty embeddings array returns error",
			entityType: "prompt",
			payload: func() map[string]interface{} {
				p := validEmbeddingPayload()
				p["embeddings"] = []interface{}{}
				return p
			}(),
			setupMock:   func(_ *mocks.MockEmbeddingServiceInterface) {},
			wantErr:     true,
			errContains: "no embeddings provided",
		},
		{
			name:       "save embeddings error is propagated",
			entityType: "prompt",
			payload:    validEmbeddingPayload(),
			setupMock: func(m *mocks.MockEmbeddingServiceInterface) {
				m.EXPECT().SaveEmbeddingChunks(
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				).Return(fmt.Errorf("db error")).Once()
			},
			wantErr:     true,
			errContains: "failed to save embeddings",
		},
		{
			name:       "artifact entity type uses artifactID field",
			entityType: "artifact",
			payload: map[string]interface{}{
				"userID":     "user-123",
				"model":      "text-embedding-ada-002",
				"artifactID": "artifact-789",
				"embeddings": []interface{}{
					map[string]interface{}{
						"embedding": []interface{}{float64(0.5)},
						"content":   "artifact content",
					},
				},
			},
			setupMock: func(m *mocks.MockEmbeddingServiceInterface) {
				m.EXPECT().SaveEmbeddingChunks(
					"user-123", "artifact", "artifact-789", "text-embedding-ada-002",
					mock.Anything,
				).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name:       "memory entity type uses memoryID field",
			entityType: "memory",
			payload: map[string]interface{}{
				"userID":   "user-123",
				"model":    "text-embedding-ada-002",
				"memoryID": "memory-999",
				"embeddings": []interface{}{
					map[string]interface{}{
						"embedding": []interface{}{float64(0.7)},
						"content":   "memory content",
					},
				},
			},
			setupMock: func(m *mocks.MockEmbeddingServiceInterface) {
				m.EXPECT().SaveEmbeddingChunks(
					"user-123", "memory", "memory-999", "text-embedding-ada-002",
					mock.Anything,
				).Return(nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := mocks.NewMockEmbeddingServiceInterface(t)
			tt.setupMock(mockSvc)

			adapter := services.NewEmbeddingHandlerAdapter(mockSvc, newNullLogger())
			err := adapter.HandleEmbedding(tt.entityType, tt.payload)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
