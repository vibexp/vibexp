package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestEscapeLikePattern(t *testing.T) {
	cases := map[string]string{
		"plain":   "plain",
		"100%":    `100\%`,
		"a_b":     `a\_b`,
		`C:\temp`: `C:\\temp`,
		`\%_`:     `\\\%\_`,
		"":        "",
	}
	for in, want := range cases {
		assert.Equal(t, want, escapeLikePattern(in), in)
	}
}

func TestClampAuditEventLimit(t *testing.T) {
	assert.Equal(t, repositories.AuditEventListDefaultLimit, clampAuditEventLimit(0))
	assert.Equal(t, repositories.AuditEventListDefaultLimit, clampAuditEventLimit(-1))
	assert.Equal(t, 1, clampAuditEventLimit(1))
	assert.Equal(t, repositories.AuditEventListMaxLimit, clampAuditEventLimit(repositories.AuditEventListMaxLimit))
	assert.Equal(t, repositories.AuditEventListMaxLimit, clampAuditEventLimit(repositories.AuditEventListMaxLimit+1))
}

func TestAuditMetadataParam(t *testing.T) {
	assert.Equal(t, []byte("{}"), auditMetadataParam(nil))
	assert.Equal(t, []byte("{}"), auditMetadataParam(json.RawMessage("")))
	assert.Equal(t, []byte("{}"), auditMetadataParam(json.RawMessage(" null ")))
	assert.Equal(t, []byte(`{"a":1}`), auditMetadataParam(json.RawMessage(`{"a":1}`)))
}

func TestBuildAuditEventWhere(t *testing.T) {
	t.Run("an empty filter matches every row", func(t *testing.T) {
		sql, args, err := buildAuditEventWhere(models.AuditEventFilter{}).ToSql()
		require.NoError(t, err)
		assert.Equal(t, "(1=1)", sql, "squirrel's empty AND")
		assert.Empty(t, args)
	})

	t.Run("every field becomes one ANDed predicate", func(t *testing.T) {
		id, q := "x", "50%"
		at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		sql, args, err := buildAuditEventWhere(models.AuditEventFilter{
			ActorUserID:  &id,
			TeamID:       &id,
			ProjectID:    &id,
			ResourceType: &id,
			Operations:   []models.AuditOperation{models.AuditOpPromptCreate},
			Sources:      []models.AuditSource{models.AuditSourceWeb, models.AuditSourceMCP},
			Outcomes:     []models.AuditOutcome{models.AuditOutcomeDenied},
			From:         &at,
			To:           &at,
			Query:        &q,
			After:        &models.AuditCursor{CreatedAt: at, ID: "id-1"},
		}).ToSql()
		require.NoError(t, err)

		for _, fragment := range []string{
			"actor_user_id = ?", "team_id = ?", "project_id = ?", "resource_type = ?",
			"operation IN (?)", "source IN (?,?)", "outcome IN (?)",
			"created_at >= ?", "created_at < ?",
			`resource_name ILIKE ? ESCAPE '\'`, `actor_email ILIKE ? ESCAPE '\'`, `ip ILIKE ? ESCAPE '\'`,
			"(created_at, id) < (?::timestamptz, ?::uuid)",
		} {
			assert.Contains(t, sql, fragment)
		}
		assert.Contains(t, args, `%50\%%`, "the free-text term is escaped and wrapped")
		assert.Contains(t, args, "id-1")
	})
}
