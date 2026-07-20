package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// stubRelationService is a RelationServiceInterface double whose Create is
// scripted; any other method panics via the embedded nil interface (the seed
// service only calls Create).
type stubRelationService struct {
	RelationServiceInterface
	createFn func(req *models.CreateRelationRequest) (*models.Relation, bool, error)
}

func (s stubRelationService) Create(
	_ context.Context, _, _ string, req *models.CreateRelationRequest,
) (*models.Relation, bool, error) {
	return s.createFn(req)
}

var (
	seedNewer = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	seedOlder = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
)

func TestSeedCandidateToRequest(t *testing.T) {
	cases := []struct {
		name         string
		c            models.RelationSeedCandidate
		wantOK       bool
		wantRelation string
		wantFromID   string
		wantToID     string
	}{
		{
			name: "cross-type object blueprint -> governed-by",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "a1", ToType: "blueprint", ToID: "b1", Distance: 0.2,
			},
			wantOK: true, wantRelation: models.RelationTypeGovernedBy, wantFromID: "a1", wantToID: "b1",
		},
		{
			name: "cross-type object prompt -> built-from",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "a1", ToType: "prompt", ToID: "p1", Distance: 0.2,
			},
			wantOK: true, wantRelation: models.RelationTypeBuiltFrom, wantFromID: "a1", wantToID: "p1",
		},
		{
			name: "cross-type object memory -> explained-by",
			c: models.RelationSeedCandidate{
				FromType: "prompt", FromID: "p1", ToType: "memory", ToID: "m1", Distance: 0.2,
			},
			wantOK: true, wantRelation: models.RelationTypeExplainedBy, wantFromID: "p1", wantToID: "m1",
		},
		{
			name: "cross-type object artifact has no matrix rule -> skip",
			c: models.RelationSeedCandidate{
				FromType: "prompt", FromID: "p1", ToType: "artifact", ToID: "a1", Distance: 0.2,
			},
			wantOK: false,
		},
		{
			name: "same-type near-dup: newer (from) supersedes older (to)",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "new", ToType: "artifact", ToID: "old", Distance: 0.05,
				FromUpdatedAt: seedNewer, ToUpdatedAt: seedOlder,
			},
			wantOK: true, wantRelation: models.RelationTypeSupersedes, wantFromID: "new", wantToID: "old",
		},
		{
			name: "same-type near-dup: newer (to) supersedes older (from) — direction flips",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "old", ToType: "artifact", ToID: "new", Distance: 0.05,
				FromUpdatedAt: seedOlder, ToUpdatedAt: seedNewer,
			},
			wantOK: true, wantRelation: models.RelationTypeSupersedes, wantFromID: "new", wantToID: "old",
		},
		{
			name: "same-type over the tighter threshold -> skip",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "new", ToType: "artifact", ToID: "old", Distance: 0.25,
				FromUpdatedAt: seedNewer, ToUpdatedAt: seedOlder,
			},
			wantOK: false,
		},
		{
			name: "same-type equal timestamps -> skip (no direction)",
			c: models.RelationSeedCandidate{
				FromType: "artifact", FromID: "x", ToType: "artifact", ToID: "y", Distance: 0.05,
				FromUpdatedAt: seedNewer, ToUpdatedAt: seedNewer,
			},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, ok := seedCandidateToRequest(tc.c)
			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				assert.Nil(t, req)
				return
			}
			assert.Equal(t, models.RelationOriginAI, req.Origin)
			assert.Equal(t, tc.wantRelation, req.RelationType)
			assert.Equal(t, tc.wantFromID, req.FromID)
			assert.Equal(t, tc.wantToID, req.ToID)
		})
	}
}

func TestRelationSeedService_Backfill_Counts(t *testing.T) {
	logger, _ := logtest.New()
	repo := mocks.NewMockRelationRepository(t)

	// Four candidates: one seeds new, one already exists, one is rejected (cross
	// project), one is not applicable (object artifact — no matrix rule).
	repo.EXPECT().FindSeedCandidates(mock.Anything, "team-1", relationSeedCrossTypeMaxDistance, relationSeedCandidateLimit).
		Return([]models.RelationSeedCandidate{
			{FromType: "artifact", FromID: "a1", ToType: "blueprint", ToID: "b1", Distance: 0.1}, // -> created
			{FromType: "artifact", FromID: "a2", ToType: "prompt", ToID: "p1", Distance: 0.1},    // -> existing
			{FromType: "memory", FromID: "m1", ToType: "blueprint", ToID: "b2", Distance: 0.1},   // -> cross-project error
			{FromType: "prompt", FromID: "p2", ToType: "artifact", ToID: "a3", Distance: 0.1},    // -> not applicable
		}, nil)

	rel := stubRelationService{createFn: func(req *models.CreateRelationRequest) (*models.Relation, bool, error) {
		switch req.FromID {
		case "a1":
			return &models.Relation{ID: "r1"}, true, nil
		case "a2":
			return &models.Relation{ID: "r2"}, false, nil
		case "m1":
			return nil, false, ErrRelationCrossProject
		default:
			return nil, false, errors.New("unexpected candidate")
		}
	}}

	svc := NewRelationSeedService(repo, rel, logger)
	summary, err := svc.Backfill(context.Background(), "user-1", "team-1")

	require.NoError(t, err)
	assert.Equal(t, 4, summary.Candidates)
	assert.Equal(t, 1, summary.Seeded)
	assert.Equal(t, 1, summary.SkippedExisting)
	assert.Equal(t, 1, summary.SkippedInvalid)
}

func TestRelationSeedService_Backfill_RepoError(t *testing.T) {
	logger, _ := logtest.New()
	repo := mocks.NewMockRelationRepository(t)
	repo.EXPECT().FindSeedCandidates(mock.Anything, "team-1", mock.Anything, mock.Anything).
		Return(nil, errors.New("db down"))

	svc := NewRelationSeedService(repo, stubRelationService{}, logger)
	_, err := svc.Backfill(context.Background(), "user-1", "team-1")

	require.Error(t, err)
}
