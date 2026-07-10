package postgres

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

// coverageRow is the (total, embedded) pair every per-type coverage query returns.
func coverageRow(total, embedded int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"total", "embedded"}).AddRow(total, embedded)
}

func TestEmbeddingBackfillRepository_CountCoverage_PerTypeAndModelScoped(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})

	// One query per entity type, in the canonical coverage order, each team-scoped
	// ($1) and model-scoped ($2). The embedded count is a COUNT(*) FILTER over an
	// EXISTS against embeddings keyed by the active model — so a stale-model embedding
	// row is not counted as embedded.
	perType := []struct {
		table    string
		total    int64
		embedded int64
	}{
		{"prompts", 10, 7},
		{"artifacts", 4, 4},
		{"memories", 5, 0},
		{"blueprints", 0, 0},
		{"feed_items", 8, 3},
	}
	for _, pt := range perType {
		mock.ExpectQuery("FROM "+pt.table).
			WithArgs("team-1", testModelID).
			WillReturnRows(coverageRow(pt.total, pt.embedded))
	}

	got, err := repo.CountCoverage(context.Background(), testModelID, "team-1")
	require.NoError(t, err)
	require.Len(t, got, 5)

	assert.Equal(t, "prompt", got[0].EntityType)
	assert.Equal(t, int64(10), got[0].Total)
	assert.Equal(t, int64(7), got[0].Embedded)

	assert.Equal(t, "artifact", got[1].EntityType)
	assert.Equal(t, int64(4), got[1].Embedded)

	assert.Equal(t, "memory", got[2].EntityType)
	assert.Equal(t, int64(0), got[2].Embedded)

	assert.Equal(t, "blueprint", got[3].EntityType)
	assert.Equal(t, int64(0), got[3].Total)

	assert.Equal(t, "feed_item", got[4].EntityType)
	assert.Equal(t, int64(8), got[4].Total)
	assert.Equal(t, int64(3), got[4].Embedded)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEmbeddingBackfillRepository_CountCoverage_BindsModelInExistsFilter proves the
// embedded count filters embeddings by the model id (bound as $2), which is what keeps
// a stale-model row from being counted as embedded.
func TestEmbeddingBackfillRepository_CountCoverage_BindsModelInExistsFilter(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})

	// The prompt query must carry the EXISTS-on-embeddings filter keyed by model id.
	mock.ExpectQuery(regexp.QuoteMeta("emb.model_id = $2")).
		WithArgs("team-9", "active-model").
		WillReturnRows(coverageRow(2, 1))
	for _, table := range []string{"artifacts", "memories", "blueprints", "feed_items"} {
		mock.ExpectQuery("FROM "+table).
			WithArgs("team-9", "active-model").
			WillReturnRows(coverageRow(0, 0))
	}

	got, err := repo.CountCoverage(context.Background(), "active-model", "team-9")
	require.NoError(t, err)
	require.Len(t, got, 5)
	assert.Equal(t, int64(2), got[0].Total)
	assert.Equal(t, int64(1), got[0].Embedded)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEmbeddingBackfillRepository_CountCoverage_EmptyModel confirms an empty model id
// (no active provider) is still bound and the query runs — the caller relies on an
// empty model matching no embedding row so everything reads as pending.
func TestEmbeddingBackfillRepository_CountCoverage_EmptyModel(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewEmbeddingBackfillRepository(&database.DB{DB: mockDB})

	for _, table := range []string{"prompts", "artifacts", "memories", "blueprints", "feed_items"} {
		mock.ExpectQuery("FROM "+table).
			WithArgs("team-1", "").
			WillReturnRows(coverageRow(3, 0))
	}

	got, err := repo.CountCoverage(context.Background(), "", "team-1")
	require.NoError(t, err)
	require.Len(t, got, 5)
	for _, c := range got {
		assert.Equal(t, int64(3), c.Total)
		assert.Equal(t, int64(0), c.Embedded)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
