//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The audit log against real Postgres (#1511): every List filter alone and
// combined, keyset stability across rows sharing a timestamp, LIKE-wildcard
// escaping, the limit clamp, and the batched purge.

// auditBase is the fixed instant the fixtures are laid out around, so the
// date-range assertions do not depend on the clock.
var auditBase = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func resetAuditEvents(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), "TRUNCATE TABLE audit_events")
	require.NoError(t, err)
}

func strp(s string) *string { return &s }

// auditRow is a valid event at auditBase+offset with every optional field
// empty. Tests set what they filter on.
func auditRow(offset time.Duration) *models.AuditEvent {
	return &models.AuditEvent{
		CreatedAt: auditBase.Add(offset),
		ActorType: models.AuditActorUser,
		Operation: models.AuditOpPromptCreate,
		Source:    models.AuditSourceWeb,
		Outcome:   models.AuditOutcomeSuccess,
	}
}

func insertAudit(t *testing.T, repo *AuditEventRepository, e *models.AuditEvent) string {
	t.Helper()
	require.NoError(t, repo.Insert(context.Background(), e))
	return e.ID
}

// listIDs returns the ids of one page, in order.
func listIDs(t *testing.T, repo *AuditEventRepository, f models.AuditEventFilter) []string {
	t.Helper()
	events, _, err := repo.List(context.Background(), f)
	require.NoError(t, err)
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}

func TestIntegrationAuditEvent_InsertRoundTrip(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)
	ctx := context.Background()

	full := &models.AuditEvent{
		ActorUserID:   strp(uuid.NewString()),
		ActorEmail:    "ada@example.com",
		ActorName:     "Ada",
		ActorType:     models.AuditActorUser,
		TeamID:        strp(uuid.NewString()),
		TeamName:      strp("Platform"),
		ProjectID:     strp(uuid.NewString()),
		ProjectName:   strp("vibexp"),
		Operation:     models.AuditOpTeamMemberRoleChanged,
		ResourceType:  strp("team_member"),
		ResourceID:    strp("not-a-uuid-is-fine"),
		ResourceName:  strp("Grace"),
		Source:        models.AuditSourceMCP,
		AssistantName: strp("Claude Code"),
		APIKeyID:      strp(uuid.NewString()),
		APIKeyName:    strp("laptop"),
		Outcome:       models.AuditOutcomeSuccess,
		IP:            strp("203.0.113.7"),
		UserAgent:     strp("curl/8.0"),
		RequestID:     strp("req-1"),
		Metadata:      json.RawMessage(`{"role":{"old":"member","new":"admin"}}`),
	}
	before := time.Now().Add(-time.Second)
	require.NoError(t, repo.Insert(ctx, full))
	require.NotEmpty(t, full.ID, "the generated id is written back")
	assert.True(t, full.CreatedAt.After(before), "the generated time is written back")

	minimal := auditRow(0)
	minimal.ActorType = models.AuditActorSystem
	minimal.Source = models.AuditSourceSystem
	require.NoError(t, repo.Insert(ctx, minimal))

	events, next, err := repo.List(ctx, models.AuditEventFilter{})
	require.NoError(t, err)
	assert.Nil(t, next)
	require.Len(t, events, 2)

	got := events[0] // full is newer (now) than minimal (auditBase)
	assert.Equal(t, full.ID, got.ID)
	assert.True(t, full.CreatedAt.Equal(got.CreatedAt))
	assert.Equal(t, full.ActorUserID, got.ActorUserID)
	assert.Equal(t, full.ActorEmail, got.ActorEmail)
	assert.Equal(t, full.ActorName, got.ActorName)
	assert.Equal(t, full.ActorType, got.ActorType)
	assert.Equal(t, full.TeamID, got.TeamID)
	assert.Equal(t, full.TeamName, got.TeamName)
	assert.Equal(t, full.ProjectID, got.ProjectID)
	assert.Equal(t, full.ProjectName, got.ProjectName)
	assert.Equal(t, full.Operation, got.Operation)
	assert.Equal(t, full.ResourceType, got.ResourceType)
	assert.Equal(t, full.ResourceID, got.ResourceID)
	assert.Equal(t, full.ResourceName, got.ResourceName)
	assert.Equal(t, full.Source, got.Source)
	assert.Equal(t, full.AssistantName, got.AssistantName)
	assert.Equal(t, full.APIKeyID, got.APIKeyID)
	assert.Equal(t, full.APIKeyName, got.APIKeyName)
	assert.Equal(t, full.Outcome, got.Outcome)
	assert.Equal(t, full.IP, got.IP)
	assert.Equal(t, full.UserAgent, got.UserAgent)
	assert.Equal(t, full.RequestID, got.RequestID)
	assert.JSONEq(t, string(full.Metadata), string(got.Metadata))

	empty := events[1]
	assert.Equal(t, minimal.ID, empty.ID)
	assert.True(t, auditBase.Equal(empty.CreatedAt), "a caller-supplied time is stored")
	assert.Nil(t, empty.ActorUserID)
	assert.Empty(t, empty.ActorEmail)
	assert.Nil(t, empty.TeamID)
	assert.Nil(t, empty.IP)
	assert.JSONEq(t, `{}`, string(empty.Metadata), "absent metadata is stored as an empty object")
}

func TestIntegrationAuditEvent_InsertNormalizesNullMetadata(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)

	e := auditRow(0)
	e.Metadata = json.RawMessage(`null`)
	insertAudit(t, repo, e)

	events, _, err := repo.List(context.Background(), models.AuditEventFilter{})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.JSONEq(t, `{}`, string(events[0].Metadata))
}

func TestIntegrationAuditEvent_InsertRejectsValuesOutsideTheChecks(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)

	for name, mutate := range map[string]func(*models.AuditEvent){
		"actor_type": func(e *models.AuditEvent) { e.ActorType = "robot" },
		"source":     func(e *models.AuditEvent) { e.Source = "carrier-pigeon" },
		"outcome":    func(e *models.AuditEvent) { e.Outcome = "error" },
	} {
		t.Run(name, func(t *testing.T) {
			e := auditRow(0)
			mutate(e)
			err := repo.Insert(context.Background(), e)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "audit_events_"+name+"_check")
		})
	}
}

func TestIntegrationAuditEvent_InsertNil(t *testing.T) {
	require.Error(t, NewAuditEventRepository(integrationDB).Insert(context.Background(), nil))
}

// auditFilterFixture seeds one row per distinguishing value, so each filter
// below can name exactly the rows it must return.
type auditFilterFixture struct {
	repo         *AuditEventRepository
	userA, userB string
	teamA, teamB string
	projectA     string
	byName       map[string]string // fixture row name -> id
}

func newAuditFilterFixture(t *testing.T) *auditFilterFixture {
	t.Helper()
	resetAuditEvents(t)
	f := &auditFilterFixture{
		repo:     NewAuditEventRepository(integrationDB),
		userA:    uuid.NewString(),
		userB:    uuid.NewString(),
		teamA:    uuid.NewString(),
		teamB:    uuid.NewString(),
		projectA: uuid.NewString(),
		byName:   map[string]string{},
	}

	rows := []struct {
		name string
		at   time.Duration
		set  func(e *models.AuditEvent)
	}{
		{"a-web-prompt", 0, func(e *models.AuditEvent) {
			e.ActorUserID, e.ActorEmail = &f.userA, "ada@example.com"
			e.TeamID, e.ProjectID = &f.teamA, &f.projectA
			e.ResourceType, e.ResourceName = strp("prompt"), strp("Release checklist")
			e.IP = strp("203.0.113.7")
		}},
		{"a-mcp-memory", time.Hour, func(e *models.AuditEvent) {
			e.ActorUserID, e.ActorEmail = &f.userA, "ada@example.com"
			e.TeamID = &f.teamA
			e.Operation, e.Source = models.AuditOpMemoryUpdate, models.AuditSourceMCP
			e.ResourceType, e.ResourceName = strp("memory"), strp("Deploy notes")
		}},
		{"b-denied", 2 * time.Hour, func(e *models.AuditEvent) {
			e.ActorUserID, e.ActorEmail = &f.userB, "grace@example.com"
			e.TeamID = &f.teamB
			e.Operation, e.Outcome = models.AuditOpAuthzDenied, models.AuditOutcomeDenied
			e.Source = models.AuditSourceAPI
			e.IP = strp("198.51.100.20")
		}},
		{"anon-failed-login", 3 * time.Hour, func(e *models.AuditEvent) {
			e.ActorType, e.ActorEmail = models.AuditActorAnonymous, "mallory@example.com"
			e.Operation, e.Outcome = models.AuditOpAuthLoginFailed, models.AuditOutcomeFailure
			e.IP = strp("192.0.2.99")
		}},
		{"percent-name", 4 * time.Hour, func(e *models.AuditEvent) {
			e.ResourceName = strp("100% done_v2")
		}},
		{"lookalike-name", 5 * time.Hour, func(e *models.AuditEvent) {
			// Matches "100% done_v2" only if % and _ act as wildcards.
			e.ResourceName = strp("1000 doneXv2")
		}},
		{"backslash-name", 6 * time.Hour, func(e *models.AuditEvent) {
			e.ResourceName = strp(`C:\temp`)
		}},
	}
	for _, r := range rows {
		e := auditRow(r.at)
		r.set(e)
		f.byName[r.name] = insertAudit(t, f.repo, e)
	}
	return f
}

// ids maps fixture row names to ids, newest first as List orders them (the
// fixture inserts in ascending time, so reversing the argument order is the
// caller's job: pass names newest first).
func (f *auditFilterFixture) ids(names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = f.byName[n]
	}
	return out
}

func TestIntegrationAuditEvent_ListFilters(t *testing.T) {
	f := newAuditFilterFixture(t)
	from, to := auditBase.Add(time.Hour), auditBase.Add(3*time.Hour)

	cases := []struct {
		name   string
		filter models.AuditEventFilter
		want   []string
	}{
		{"no filter returns everything newest first", models.AuditEventFilter{},
			[]string{"backslash-name", "lookalike-name", "percent-name", "anon-failed-login",
				"b-denied", "a-mcp-memory", "a-web-prompt"}},
		{"actor", models.AuditEventFilter{ActorUserID: &f.userA},
			[]string{"a-mcp-memory", "a-web-prompt"}},
		{"team", models.AuditEventFilter{TeamID: &f.teamB}, []string{"b-denied"}},
		{"project", models.AuditEventFilter{ProjectID: &f.projectA}, []string{"a-web-prompt"}},
		{"resource type", models.AuditEventFilter{ResourceType: strp("memory")}, []string{"a-mcp-memory"}},
		{"operations (any of)", models.AuditEventFilter{
			Operations: []models.AuditOperation{models.AuditOpAuthzDenied, models.AuditOpAuthLoginFailed},
		}, []string{"anon-failed-login", "b-denied"}},
		{"sources (any of)", models.AuditEventFilter{
			Sources: []models.AuditSource{models.AuditSourceMCP, models.AuditSourceAPI},
		}, []string{"b-denied", "a-mcp-memory"}},
		{"outcomes (any of)", models.AuditEventFilter{
			Outcomes: []models.AuditOutcome{models.AuditOutcomeDenied, models.AuditOutcomeFailure},
		}, []string{"anon-failed-login", "b-denied"}},
		{"from is inclusive, to is exclusive", models.AuditEventFilter{From: &from, To: &to},
			[]string{"b-denied", "a-mcp-memory"}},
		{"query matches the resource name, case-insensitively", models.AuditEventFilter{Query: strp("deploy")},
			[]string{"a-mcp-memory"}},
		{"query matches the actor email", models.AuditEventFilter{Query: strp("GRACE@")},
			[]string{"b-denied"}},
		{"query matches the ip", models.AuditEventFilter{Query: strp("192.0.2")},
			[]string{"anon-failed-login"}},
		{"an empty query does not filter", models.AuditEventFilter{Query: strp(""), ActorUserID: &f.userB},
			[]string{"b-denied"}},
		{"filters combine with AND", models.AuditEventFilter{
			ActorUserID: &f.userA,
			Sources:     []models.AuditSource{models.AuditSourceWeb},
			From:        &auditBase,
			To:          &to,
			Query:       strp("checklist"),
		}, []string{"a-web-prompt"}},
		{"a combination nothing matches", models.AuditEventFilter{
			TeamID:   &f.teamA,
			Outcomes: []models.AuditOutcome{models.AuditOutcomeDenied},
		}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, f.ids(tc.want...), listIDs(t, f.repo, tc.filter))
		})
	}
}

func TestIntegrationAuditEvent_ListQueryEscapesWildcards(t *testing.T) {
	f := newAuditFilterFixture(t)

	cases := map[string][]string{
		"%":      {"percent-name"},
		"0% d":   {"percent-name"},
		"done_v": {"percent-name"},
		"_":      {"percent-name"},
		`\`:      {"backslash-name"},
		`:\t`:    {"backslash-name"},
	}
	for query, want := range cases {
		t.Run(query, func(t *testing.T) {
			assert.Equal(t, f.ids(want...), listIDs(t, f.repo, models.AuditEventFilter{Query: strp(query)}),
				"the term must match literally, never the lookalike row")
		})
	}
}

// TestIntegrationAuditEvent_ListCursorIsStableAcrossEqualTimestamps pages
// through rows that share one created_at in pages that split the tie, and
// requires every row exactly once in (created_at DESC, id DESC) order.
func TestIntegrationAuditEvent_ListCursorIsStableAcrossEqualTimestamps(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)
	ctx := context.Background()

	// Five rows at one instant between an older and a newer row.
	var want []string
	newer := insertAudit(t, repo, auditRow(time.Minute))
	tied := make([]string, 0, 5)
	for range 5 {
		tied = append(tied, insertAudit(t, repo, auditRow(0)))
	}
	older := insertAudit(t, repo, auditRow(-time.Minute))

	// Ties are ordered by id descending.
	sortedTied := slices.Clone(tied)
	slices.Sort(sortedTied)
	slices.Reverse(sortedTied)
	want = append(want, newer)
	want = append(want, sortedTied...)
	want = append(want, older)

	var got []string
	filter := models.AuditEventFilter{Limit: 2}
	for page := 0; ; page++ {
		require.Less(t, page, 10, "pagination must terminate")
		events, next, err := repo.List(ctx, filter)
		require.NoError(t, err)
		for _, e := range events {
			got = append(got, e.ID)
		}
		if next == nil {
			break
		}
		require.NotEmpty(t, events, "a cursor never leads to an empty page")
		filter.After = next
	}

	assert.Equal(t, want, got, "no duplicates, no gaps, stable order")
}

func TestIntegrationAuditEvent_ListLastPageHasNoCursor(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)
	for i := range 3 {
		insertAudit(t, repo, auditRow(time.Duration(i)*time.Second))
	}

	events, next, err := repo.List(context.Background(), models.AuditEventFilter{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, events, 3)
	assert.Nil(t, next, "an exactly full last page has no next page")
}

func TestIntegrationAuditEvent_ListClampsTheLimit(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)

	total := repositories.AuditEventListMaxLimit + 5
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO audit_events (created_at, actor_type, operation, source, outcome)
		SELECT $1::timestamptz + make_interval(secs => g), 'system', 'audit.export', 'system', 'success'
		FROM generate_series(1, $2) AS g`, auditBase, total)
	require.NoError(t, err)

	cases := []struct {
		limit, want int
		more        bool
	}{
		{0, repositories.AuditEventListDefaultLimit, true},
		{-7, repositories.AuditEventListDefaultLimit, true},
		{1, 1, true},
		{repositories.AuditEventListMaxLimit + 1000, repositories.AuditEventListMaxLimit, true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("limit %d", tc.limit), func(t *testing.T) {
			events, next, err := repo.List(context.Background(), models.AuditEventFilter{Limit: tc.limit})
			require.NoError(t, err)
			assert.Len(t, events, tc.want)
			assert.Equal(t, tc.more, next != nil)
		})
	}
}

func TestIntegrationAuditEvent_PurgeOlderThanBatches(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)
	ctx := context.Background()
	cutoff := auditBase

	// 7 old rows (more than two batches of 3), one exactly at the cutoff, and
	// two newer rows: only the 7 strictly older go.
	for i := 1; i <= 7; i++ {
		insertAudit(t, repo, auditRow(-time.Duration(i)*time.Hour))
	}
	atCutoff := insertAudit(t, repo, auditRow(0))
	newer1 := insertAudit(t, repo, auditRow(time.Hour))
	newer2 := insertAudit(t, repo, auditRow(2*time.Hour))

	deleted, err := repo.PurgeOlderThan(ctx, cutoff, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(7), deleted)

	assert.ElementsMatch(t, []string{atCutoff, newer1, newer2}, listIDs(t, repo, models.AuditEventFilter{}))

	again, err := repo.PurgeOlderThan(ctx, cutoff, 3)
	require.NoError(t, err)
	assert.Zero(t, again, "a second purge finds nothing")
}

func TestIntegrationAuditEvent_PurgeExactMultipleOfBatch(t *testing.T) {
	resetAuditEvents(t)
	repo := NewAuditEventRepository(integrationDB)
	for i := 1; i <= 4; i++ {
		insertAudit(t, repo, auditRow(-time.Duration(i)*time.Minute))
	}

	deleted, err := repo.PurgeOlderThan(context.Background(), auditBase, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(4), deleted)
	assert.Empty(t, listIDs(t, repo, models.AuditEventFilter{}))
}

func TestIntegrationAuditEvent_PurgeRejectsANonPositiveBatch(t *testing.T) {
	repo := NewAuditEventRepository(integrationDB)
	for _, size := range []int{0, -1} {
		_, err := repo.PurgeOlderThan(context.Background(), auditBase, size)
		assert.Error(t, err)
	}
}

func TestIntegrationProject_GetNameByIDForAudit(t *testing.T) {
	resetIntegrationTables(t)
	owner := insertTestUser(t)
	teamID := insertTestTeam(t, owner)
	projectID := insertTestProject(t, owner, teamID)
	repo := NewProjectRepository(integrationDB)
	ctx := context.Background()

	name, err := repo.GetNameByIDForAudit(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, "Project "+projectID[:8], name, "no membership is needed")

	missing, err := repo.GetNameByIDForAudit(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Empty(t, missing)

	_, err = repo.GetNameByIDForAudit(ctx, "not-a-uuid")
	assert.Error(t, err)
}

// Count applies exactly List's filters (the export's total must agree with what
// it streams) and ignores the page size and cursor.
func TestIntegrationAuditEvent_CountMatchesListFilters(t *testing.T) {
	f := newAuditFilterFixture(t)
	from, to := auditBase.Add(time.Hour), auditBase.Add(3*time.Hour)
	filters := map[string]models.AuditEventFilter{
		"no filter":   {},
		"actor":       {ActorUserID: &f.userA},
		"wildcard q":  {Query: strp("100% done_v2")},
		"date range":  {From: &from, To: &to},
		"enums":       {Outcomes: []models.AuditOutcome{models.AuditOutcomeDenied, models.AuditOutcomeFailure}},
		"no match":    {TeamID: &f.teamA, Outcomes: []models.AuditOutcome{models.AuditOutcomeDenied}},
		"combination": {ActorUserID: &f.userA, Sources: []models.AuditSource{models.AuditSourceWeb}},
	}
	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			listed := listIDs(t, f.repo, models.AuditEventFilter{
				ActorUserID: filter.ActorUserID, TeamID: filter.TeamID, Query: filter.Query,
				From: filter.From, To: filter.To, Outcomes: filter.Outcomes, Sources: filter.Sources,
				Limit: repositories.AuditEventListMaxLimit,
			})

			n, err := f.repo.Count(context.Background(), filter)

			require.NoError(t, err)
			assert.Equal(t, len(listed), n)
		})
	}

	t.Run("ignores the limit and the cursor", func(t *testing.T) {
		n, err := f.repo.Count(context.Background(), models.AuditEventFilter{
			Limit: 1, After: &models.AuditCursor{CreatedAt: auditBase.Add(time.Hour), ID: f.byName["a-mcp-memory"]},
		})
		require.NoError(t, err)
		assert.Equal(t, 7, n)
	})
}
