//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// memories.created_at / updated_at are `timestamp WITHOUT time zone`, so
// Postgres keeps the wall-clock reading it is handed and drops the offset.
// These tests hand the repository times in a zone that is NOT UTC — which is
// what time.Now() yields on any server not running on UTC — and require the
// same INSTANT back. Without the UTC conversion on write they fail by exactly
// the zone's offset, whatever zone the test process itself runs in.
//
// Needs a real database: sqlmock never evaluates the column type, which is
// the whole of the bug.

var memoryTestZone = time.FixedZone("UTC+5", 5*60*60)

func TestMemoryTimestamps_CreateStoresTheInstantNotTheLocalWallClock(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewMemoryRepository(integrationDB)

	// Postgres keeps microseconds; truncate so the comparison is exact.
	written := time.Now().In(memoryTestZone).Truncate(time.Microsecond)
	memory := &models.Memory{
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Text: "written from a non-UTC zone", Status: models.MemoryStatusActive,
		CreatedAt: written, UpdatedAt: written,
	}
	require.NoError(t, repo.Create(ctx, memory))

	// The values the insert hands back are already the stored ones.
	assert.True(t, written.Equal(memory.CreatedAt),
		"created_at: wrote %s, stored %s", written.UTC(), memory.CreatedAt.UTC())

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	assert.True(t, written.Equal(got.CreatedAt),
		"created_at: wrote %s, read back %s", written.UTC(), got.CreatedAt.UTC())
	assert.True(t, written.Equal(got.UpdatedAt),
		"updated_at: wrote %s, read back %s", written.UTC(), got.UpdatedAt.UTC())
}

func TestMemoryTimestamps_UpdateStoresTheInstantNotTheLocalWallClock(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewMemoryRepository(integrationDB)

	created := time.Now().In(memoryTestZone).Add(-time.Hour).Truncate(time.Microsecond)
	memory := &models.Memory{
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Text: "before", Status: models.MemoryStatusActive,
		CreatedAt: created, UpdatedAt: created,
	}
	require.NoError(t, repo.Create(ctx, memory))

	// Reload for the row's version: the update is optimistically locked on it.
	memory, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)

	// The update path is also the one a version restore goes through.
	edited := time.Now().In(memoryTestZone).Truncate(time.Microsecond)
	memory.Text = "after"
	memory.UpdatedAt = edited
	require.NoError(t, repo.Update(ctx, memory))

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	// Not exact: the table's BEFORE UPDATE trigger re-stamps updated_at with the
	// database clock. What matters is that it is the same moment, not hours off.
	assert.WithinDuration(t, edited, got.UpdatedAt, 5*time.Second,
		"updated_at: wrote %s, read back %s", edited.UTC(), got.UpdatedAt.UTC())
	assert.True(t, created.Equal(got.CreatedAt), "an update must not move created_at")
	assert.True(t, got.UpdatedAt.After(got.CreatedAt), "an edit is later than the creation")
}
