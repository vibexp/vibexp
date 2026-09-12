//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Behaviour-level proof, against real Postgres, that the prompt `labels` filter
// is OVERLAP since #938 and no longer containment: the squirrel test asserts the
// SQL that gets generated, this asserts what the database actually MATCHES.
//
// Both halves matter. `?labels=a,b` returning the rows carrying a OR b is the
// wire-visible behaviour change; the total narrowing with the page is the
// property the shared where-clause builder exists for, since PromptRepository
// hard-codes its COUNT query and its page query separately.

type promptLabelsFixture struct {
	repo      *PromptRepository
	userID    string
	teamID    string
	projectID string
}

func newPromptLabelsFixture(t *testing.T) promptLabelsFixture {
	t.Helper()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	return promptLabelsFixture{
		repo:      NewPromptRepository(integrationDB).(*PromptRepository),
		userID:    userID,
		teamID:    teamID,
		projectID: insertTestProject(t, userID, teamID),
	}
}

func (f promptLabelsFixture) seed(t *testing.T, labels ...string) string {
	t.Helper()

	id := uuid.New().String()
	_, err := integrationDB.Exec(
		`INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, status, labels)
		 VALUES ($1, $2, $3, $4, $5, $6, 'body', 'published', $7)`,
		id, f.userID, f.teamID, f.projectID,
		"Prompt "+id[:8], "prompt-"+id[:8], pq.StringArray(labels))
	require.NoError(t, err)
	return id
}

func (f promptLabelsFixture) list(t *testing.T, labels []string) ([]string, int) {
	t.Helper()

	prompts, total, err := f.repo.List(context.Background(), f.userID, repositories.PromptFilters{
		TeamID: f.teamID, Page: 1, Limit: 100, Labels: labels,
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(prompts))
	for _, p := range prompts {
		ids = append(ids, p.ID)
	}
	return ids, total
}

func TestIntegrationPromptLabelsFilter_MatchesAnyLabelNotEveryLabel(t *testing.T) {
	f := newPromptLabelsFixture(t)
	onlyA := f.seed(t, "alpha")
	onlyB := f.seed(t, "beta")
	both := f.seed(t, "alpha", "beta")
	neither := f.seed(t, "gamma")

	t.Run("unfiltered returns every seeded prompt", func(t *testing.T) {
		ids, total := f.list(t, nil)
		assert.Len(t, ids, total, "page and total must agree")
		assert.ElementsMatch(t, []string{onlyA, onlyB, both, neither}, ids)
	})

	// The #938 change itself: under the old containment predicate this returned
	// the single prompt carrying BOTH labels.
	t.Run("two labels match any of them, and the total narrows with the page", func(t *testing.T) {
		ids, total := f.list(t, []string{"alpha", "beta"})
		assert.ElementsMatch(t, []string{onlyA, onlyB, both}, ids)
		assert.Equal(t, 3, total, "the COUNT query must carry the predicate too, not just the page")
	})

	t.Run("a prompt carrying only one of the requested labels is returned", func(t *testing.T) {
		ids, _ := f.list(t, []string{"alpha", "gamma"})
		assert.ElementsMatch(t, []string{onlyA, both, neither}, ids)
	})

	t.Run("a label nothing carries matches nothing", func(t *testing.T) {
		ids, total := f.list(t, []string{"delta"})
		assert.Empty(t, ids)
		assert.Equal(t, 0, total)
	})
}
