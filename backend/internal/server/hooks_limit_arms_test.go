package server

// Coverage for the resource-limit gate and JSONB converters of the IDE-hooks
// families (coverage epic #358 / issue #393). The Claude and Cursor
// session/tool limit checks are structural mirrors, so the three outcomes
// (check error → 500, limit reached → 403, allowed → true) are table-driven
// across all four. The convert*ToJSONBData helpers and the cursor field
// converter are pure and driven directly, including the marshal/unmarshal
// error arms.

import (
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// hooksLimitContainer swaps only ResourceUsageService into the base mock.
type hooksLimitContainer struct {
	BaseMockContainer
	ruSvc services.ResourceUsageServiceInterface
}

func (c *hooksLimitContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return c.ruSvc
}

func newHooksLimitServer(ru services.ResourceUsageServiceInterface) *Server {
	return &Server{
		container: &hooksLimitContainer{ruSvc: ru},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
	}
}

func TestHooksResourceLimitChecks(t *testing.T) {
	const userID = "hooks-limit-user"

	// Each check function paired with the resource type it gates on.
	checks := []struct {
		name         string
		resourceType string
		call         func(s *Server, w http.ResponseWriter, r *http.Request) bool
	}{
		{"claude session", "ai_session", func(s *Server, w http.ResponseWriter, r *http.Request) bool {
			return s.checkClaudeSessionLimit(w, r, userID)
		}},
		{"claude tool", "ai_tool", func(s *Server, w http.ResponseWriter, r *http.Request) bool {
			return s.checkClaudeToolLimit(w, r, userID)
		}},
		{"cursor session", "ai_session", func(s *Server, w http.ResponseWriter, r *http.Request) bool {
			return s.checkSessionLimitAndRespond(w, r, userID)
		}},
		{"cursor tool", "ai_tool", func(s *Server, w http.ResponseWriter, r *http.Request) bool {
			return s.checkToolLimitAndRespond(w, r, userID)
		}},
	}

	for _, c := range checks {
		c := c
		t.Run(c.name+"/check error → 500", func(t *testing.T) {
			ru := servicesmocks.NewMockResourceUsageServiceInterface(t)
			ru.On("CheckResourceLimit", mock.Anything, userID, c.resourceType).
				Return(false, stderrors.New("db down")).Once()
			srv := newHooksLimitServer(ru)

			rr := httptest.NewRecorder()
			ok := c.call(srv, rr, httptest.NewRequest(http.MethodPost, "/hooks", nil))
			assert.False(t, ok)
			assert.Equal(t, http.StatusInternalServerError, rr.Code)
		})

		t.Run(c.name+"/limit reached → 403 with usage details", func(t *testing.T) {
			ru := servicesmocks.NewMockResourceUsageServiceInterface(t)
			ru.On("CheckResourceLimit", mock.Anything, userID, c.resourceType).
				Return(false, nil).Once()
			ru.On("GetResourceUsage", mock.Anything, userID).
				Return(&models.ResourceUsageResponse{Resources: []models.ResourceUsageItem{
					{ResourceType: c.resourceType, Count: 10, Limit: 10},
				}}, nil).Once()
			srv := newHooksLimitServer(ru)

			rr := httptest.NewRecorder()
			ok := c.call(srv, rr, httptest.NewRequest(http.MethodPost, "/hooks", nil))
			assert.False(t, ok)
			assert.Equal(t, http.StatusForbidden, rr.Code)
			assert.Contains(t, rr.Body.String(), c.resourceType)
		})

		t.Run(c.name+"/allowed → true", func(t *testing.T) {
			ru := servicesmocks.NewMockResourceUsageServiceInterface(t)
			ru.On("CheckResourceLimit", mock.Anything, userID, c.resourceType).
				Return(true, nil).Once()
			srv := newHooksLimitServer(ru)

			rr := httptest.NewRecorder()
			ok := c.call(srv, rr, httptest.NewRequest(http.MethodPost, "/hooks", nil))
			assert.True(t, ok)
		})
	}
}

func TestConvertToJSONBData_Arms(t *testing.T) {
	converters := map[string]func(interface{}) (*models.JSONBData, error){
		"claude": convertToJSONBData,
		"cursor": convertCursorFieldToJSONBData,
	}
	for name, convert := range converters {
		t.Run(name+"/nil is a no-op", func(t *testing.T) {
			got, err := convert(nil)
			require.NoError(t, err)
			assert.Nil(t, got)
		})
		t.Run(name+"/object decodes into a map", func(t *testing.T) {
			got, err := convert(map[string]any{"file": "main.go"})
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "main.go", (*got)["file"])
		})
		t.Run(name+"/marshal error is returned", func(t *testing.T) {
			_, err := convert(make(chan int))
			require.Error(t, err)
		})
		t.Run(name+"/non-object cannot unmarshal into a map", func(t *testing.T) {
			_, err := convert(5)
			require.Error(t, err)
		})
	}
}

func TestPrepareClaudeHookPayload_ConverterErrorArm(t *testing.T) {
	toolName := "Bash"
	// ToolInput=5 marshals fine at the top level but fails to decode into a
	// JSONBData map, exercising the logged-and-continue error arm.
	payload := &models.IncomingHookPayload{
		SessionID:     "sess-1",
		HookEventName: "PreToolUse",
		ToolName:      &toolName,
		ToolInput:     5,
	}

	got, err := prepareClaudeHookPayload("user-1", "team-1", payload)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.SessionID)
	assert.Nil(t, got.ToolInput, "unconvertible tool input is dropped, not fatal")
}

func TestConvertCursorPayloadFields_HandlesMixedInputs(t *testing.T) {
	// A valid object in one slot and an unconvertible number in another
	// exercises both the success and logged-error arms in one pass.
	payload := &models.IncomingCursorHookPayload{
		Configuration: map[string]any{"model": "auto"},
		Input:         5,
	}
	cfg, _, _, input, _, _ := convertCursorPayloadFields(payload)
	require.NotNil(t, cfg)
	assert.Equal(t, "auto", (*cfg)["model"])
	assert.Nil(t, input, "unconvertible input is dropped")
}
