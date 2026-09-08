package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// The `labels` list filter (issue #910). Two properties are pinned per resource,
// and only the second one is guarded by anything else:
//
//  1. the predicate is the OVERLAP operator `&&` (match ANY of the requested
//     labels), not containment;
//  2. it lands in the SHARED where-clause, so the COUNT query narrows with the
//     page query. Each of these repositories hard-codes its count and page
//     queries separately, so a predicate applied to the page alone yields a
//     short page describing an unfiltered total -- which no assertion about the
//     page itself would ever notice.

func TestApplyLabelsFilter(t *testing.T) {
	t.Run("no labels adds no predicate", func(t *testing.T) {
		assert.Empty(t, applyLabelsFilter(nil, "a.labels", nil))
		assert.Empty(t, applyLabelsFilter(nil, "a.labels", []string{}))
	})

	t.Run("labels add an overlap predicate", func(t *testing.T) {
		where := applyLabelsFilter(nil, "a.labels", []string{"onboarding", "api"})
		require.Len(t, where, 1)

		sql, args, err := where[0].ToSql()
		require.NoError(t, err)
		assert.Equal(t, "a.labels && ?", sql)
		assert.Contains(t, sql, "&&", "overlap, not containment: a multi-label filter must match ANY label")
		assert.NotContains(t, sql, "@>")
		require.Len(t, args, 1)
	})
}

func TestArtifactRepository_LabelsFilterNarrowsCountAndPage(t *testing.T) {
	repo, mock, mockDB := setupArtifactListTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	// The labels predicate is appended BEFORE the status-visibility one, so the
	// bound order is base args, labels, then the archived exclusion.
	labelArgs := append(artifactListBaseArgs(), pq.StringArray{"onboarding", "api"}, "archived")

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM artifacts a WHERE .*a\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM artifacts a WHERE .*a\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(artifactListOneRow(now))

	artifacts, total, err := repo.List(context.Background(), "user-123", repositories.ArtifactFilters{
		TeamID: "team-123", Page: 1, Limit: 10, Labels: []string{"onboarding", "api"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, []string{"onboarding"}, []string(artifacts[0].Labels))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBlueprintRepository_LabelsFilterNarrowsCountAndPage(t *testing.T) {
	repo, mock, mockDB := setupBlueprintListTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	labelArgs := append(blueprintListBaseArgs(), pq.StringArray{"onboarding"})

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s WHERE .*s\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM blueprints s WHERE .*s\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(blueprintListOneRow(now))

	blueprints, total, err := repo.List(context.Background(), "user-123", repositories.BlueprintFilters{
		TeamID: "team-123", Page: 1, Limit: 10, Labels: []string{"onboarding"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, blueprints, 1)
	assert.Equal(t, []string{"onboarding"}, []string(blueprints[0].Labels))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMemoryRepository_LabelsFilterNarrowsCountAndPage(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	labelArgs := append(memoryListBaseArgs(), pq.StringArray{"onboarding"}, "archived")

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories m WHERE .*m\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM memories m WHERE .*m\.labels && \$`).
		WithArgs(labelArgs...).
		WillReturnRows(sqlmock.NewRows(memoryListColumns).AddRow(
			"memory-1", "user-123", "team-123", "project-123", "remember this",
			"active", []byte(`{"env":"prod"}`), now, now, pq.StringArray{"onboarding"},
		))

	memories, total, err := repo.List(context.Background(), "user-123", repositories.MemoryFilters{
		TeamID: "team-123", Page: 1, Limit: 10, Labels: []string{"onboarding"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, memories, 1)
	assert.Equal(t, []string{"onboarding"}, []string(memories[0].Labels))
	assert.NoError(t, mock.ExpectationsWereMet())
}
