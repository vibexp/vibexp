package a2atest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// errNotFound is returned by the fakes when a lookup misses.
var errNotFound = errors.New("a2atest: not found")

// StatCall records one UpdateExecutionStats invocation.
type StatCall struct {
	AgentID    string
	Success    bool
	DurationMS int
}

// AgentStore is an in-memory repositories.AgentRepository serving a single
// preset agent and recording stats updates. Safe for concurrent use.
type AgentStore struct {
	mu    sync.Mutex
	agent *models.Agent
	stats []StatCall
}

var _ repositories.AgentRepository = (*AgentStore)(nil)

// NewAgentStore returns an AgentStore serving agent.
func NewAgentStore(agent *models.Agent) *AgentStore { return &AgentStore{agent: agent} }

// Stats returns the recorded UpdateExecutionStats calls.
func (s *AgentStore) Stats() []StatCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StatCall(nil), s.stats...)
}

func (s *AgentStore) Create(_ context.Context, agent *models.Agent) error {
	return s.put(agent)
}

func (s *AgentStore) GetByID(_ context.Context, _, _, agentID string) (*models.Agent, error) {
	return s.get(agentID)
}

func (s *AgentStore) GetByIDCrossTeam(_ context.Context, _, agentID string) (*models.Agent, error) {
	return s.get(agentID)
}

func (s *AgentStore) get(agentID string) (*models.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agent == nil || s.agent.ID != agentID {
		return nil, errNotFound
	}
	return s.agent, nil
}

func (s *AgentStore) List(
	_ context.Context, _ string, _ repositories.AgentFilters,
) ([]models.Agent, int, error) {
	return nil, 0, nil
}

func (s *AgentStore) Update(_ context.Context, agent *models.Agent) error {
	return s.put(agent)
}

// put replaces the single stored agent; the store serves one preset agent, so
// Create and Update share this behavior by design.
func (s *AgentStore) put(agent *models.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agent = agent
	return nil
}

func (s *AgentStore) Delete(_ context.Context, _, _, _ string) error { return nil }

func (s *AgentStore) GetStats(_ context.Context, _, _ string) (*models.AgentStatsResponse, error) {
	return &models.AgentStatsResponse{}, nil
}

func (s *AgentStore) UpdateExecutionStats(_ context.Context, agentID string, success bool, duration int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = append(s.stats, StatCall{AgentID: agentID, Success: success, DurationMS: duration})
	return nil
}

func (s *AgentStore) GetNamesByIDsCrossTeam(
	_ context.Context, _ string, _ []string,
) (map[string]string, error) {
	return map[string]string{}, nil
}

// ExecutionStore is an in-memory repositories.AgentExecutionRepository. It
// faithfully records the mutations the invocation service and stream processor
// perform (task info, artifacts, status, conversation id) so tests can assert
// persisted state. Safe for concurrent use.
type ExecutionStore struct {
	mu    sync.Mutex
	seq   int
	byID  map[string]*models.AgentExecution
	order []string
}

var _ repositories.AgentExecutionRepository = (*ExecutionStore)(nil)

// NewExecutionStore returns an empty ExecutionStore.
func NewExecutionStore() *ExecutionStore {
	return &ExecutionStore{byID: make(map[string]*models.AgentExecution)}
}

func (s *ExecutionStore) Create(_ context.Context, execution *models.AgentExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	if execution.ID == "" {
		execution.ID = fmt.Sprintf("exec-%d", s.seq)
	}
	stored := *execution
	s.byID[execution.ID] = &stored
	s.order = append(s.order, execution.ID)
	return nil
}

func (s *ExecutionStore) GetByID(_ context.Context, _, executionID string) (*models.AgentExecution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[executionID]
	if !ok {
		return nil, errNotFound
	}
	cp := *r
	return &cp, nil
}

func (s *ExecutionStore) Update(_ context.Context, execution *models.AgentExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[execution.ID]; !ok {
		return errNotFound
	}
	stored := *execution
	s.byID[execution.ID] = &stored
	return nil
}

func (s *ExecutionStore) UpdateTaskInfo(_ context.Context, executionID, taskID, contextID, currentState string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[executionID]
	if !ok {
		return errNotFound
	}
	if taskID != "" {
		v := taskID
		r.TaskID = &v
	}
	if contextID != "" {
		v := contextID
		r.ContextID = &v
	}
	if currentState != "" {
		v := currentState
		r.CurrentState = &v
	}
	return nil
}

func (s *ExecutionStore) UpdateArtifacts(
	_ context.Context, executionID string, artifacts []map[string]interface{},
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[executionID]
	if !ok {
		return errNotFound
	}
	r.Artifacts = artifacts
	return nil
}

func (s *ExecutionStore) UpdateStatus(_ context.Context, executionID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[executionID]
	if !ok {
		return errNotFound
	}
	r.Status = status
	return nil
}

func (s *ExecutionStore) GetFirstExecutionInConversation(
	_ context.Context, _, conversationID string,
) (*models.AgentExecution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.order {
		r := s.byID[id]
		if r.ConversationID != nil && *r.ConversationID == conversationID {
			cp := *r
			return &cp, nil
		}
	}
	return nil, errNotFound
}

func (s *ExecutionStore) UpdateConversationID(_ context.Context, executionID, conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[executionID]
	if !ok {
		return errNotFound
	}
	v := conversationID
	r.ConversationID = &v
	return nil
}

// Unused by the invocation path; present to satisfy the interface.

func (s *ExecutionStore) List(
	_ context.Context, _ string, _ repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	return nil, 0, nil
}

func (s *ExecutionStore) GetByAgentID(
	_ context.Context, _, _ string, _ repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	return nil, 0, nil
}

func (s *ExecutionStore) GetByTaskID(_ context.Context, _, _ string) (*models.AgentExecution, error) {
	return nil, errNotFound
}

func (s *ExecutionStore) GetByConversationID(
	_ context.Context, _, _ string, _ int, _ *time.Time,
) ([]models.AgentExecution, bool, int, error) {
	return nil, false, 0, nil
}

func (s *ExecutionStore) ListConversations(
	_ context.Context, _, _ string, _, _ int,
) ([]models.ConversationSummary, int, error) {
	return nil, 0, nil
}

// EventStore is an in-memory repositories.AgentExecutionEventRepository
// recording streaming events with their sequence numbers. Safe for concurrent
// use so tests can poll it while the stream processor writes.
type EventStore struct {
	mu     sync.Mutex
	seq    int
	events []models.AgentExecutionEvent
}

var _ repositories.AgentExecutionEventRepository = (*EventStore)(nil)

// NewEventStore returns an empty EventStore.
func NewEventStore() *EventStore { return &EventStore{} }

func (s *EventStore) Create(_ context.Context, event *models.AgentExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	if event.ID == "" {
		event.ID = fmt.Sprintf("event-%d", s.seq)
	}
	s.events = append(s.events, *event)
	return nil
}

func (s *EventStore) GetByID(_ context.Context, eventID string) (*models.AgentExecutionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.events {
		if s.events[i].ID == eventID {
			cp := s.events[i]
			return &cp, nil
		}
	}
	return nil, errNotFound
}

func (s *EventStore) ListByExecutionID(
	_ context.Context, executionID string, limit, offset int,
) ([]models.AgentExecutionEvent, int, error) {
	all := s.forExecution(executionID)
	total := len(all)
	if offset > total {
		offset = total
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (s *EventStore) ListAfterSequence(
	_ context.Context, executionID string, afterSequence int,
) ([]models.AgentExecutionEvent, error) {
	out := make([]models.AgentExecutionEvent, 0)
	for _, e := range s.forExecution(executionID) {
		if e.SequenceNumber > afterSequence {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *EventStore) GetLatestByExecutionID(
	_ context.Context, executionID string,
) (*models.AgentExecutionEvent, error) {
	all := s.forExecution(executionID)
	if len(all) == 0 {
		return nil, errNotFound
	}
	latest := all[len(all)-1]
	return &latest, nil
}

func (s *EventStore) CountByExecutionID(_ context.Context, executionID string) (int, error) {
	return len(s.forExecution(executionID)), nil
}

// forExecution returns a sorted-by-sequence copy of the events for executionID.
func (s *EventStore) forExecution(executionID string) []models.AgentExecutionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.AgentExecutionEvent, 0, len(s.events))
	for i := range s.events {
		if s.events[i].ExecutionID == executionID {
			out = append(out, s.events[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SequenceNumber < out[j].SequenceNumber
	})
	return out
}
