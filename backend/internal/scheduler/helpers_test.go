package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// newTestScheduler returns a scheduler wired for unit tests (mocked repo +
// sqlmock connection) together with both doubles.
func newTestScheduler(t *testing.T) (*Scheduler, sqlmock.Sqlmock, *mocks.MockScheduleRepository) {
	t.Helper()
	// Unique DSN per test: sqlmock's NewWithDSN reuses one driver instance per
	// DSN string, so sharing a DSN across tests would leak expectations between
	// them.
	sqlDB, sqlMock, err := sqlmock.NewWithDSN(testSchedulerDSN + "?case=" + t.Name())
	require.NoError(t, err)
	// Ordered matching (the default) is what pins each schedule's lock/unlock
	// sequence. sqlmock errors on an UNEXPECTED Close, so ExpectClose is
	// registered in the cleanup — after the test's own expectations, which
	// ordered matching requires to come first anyway.
	t.Cleanup(func() {
		sqlMock.ExpectClose()
		if cerr := sqlDB.Close(); cerr != nil {
			t.Errorf("close mock db: %v", cerr)
		}
	})

	repo := mocks.NewMockScheduleRepository(t)
	reg := NewRegistry()
	cfg := Config{TickInterval: time.Hour, JobTimeout: time.Minute, DueLimit: 10}
	s := New(repo, &database.DB{DB: sqlDB}, reg, cfg, discardLogger())
	return s, sqlMock, repo
}

// dueSchedule returns a schedule that ListDue would return.
func dueSchedule(id, jobType string) *models.Schedule {
	return &models.Schedule{
		ID:              id,
		TeamID:          "team-1",
		JobType:         jobType,
		IntervalSeconds: 3600,
		NextRunAt:       time.Now().Add(-time.Minute),
	}
}

// expectLockClaim sets the sqlmock expectations for one successful
// claim-unlock cycle on the pinned connection.
func expectLockClaim(sqlMock sqlmock.Sqlmock) {
	sqlMock.ExpectQuery(`SELECT pg_try_advisory_lock`).WillReturnRows(
		sqlmock.NewRows([]string{"locked"}).AddRow(true))
	sqlMock.ExpectExec(`SELECT pg_advisory_unlock`).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewRegistry()
	assert.Nil(t, reg.Lookup("missing"))

	called := false
	reg.Register("job_a", func(ctx context.Context, teamID string) error {
		called = true
		return nil
	})
	require.NotNil(t, reg.Lookup("job_a"))
	require.NoError(t, reg.Lookup("job_a")(context.Background(), "team-1"))
	assert.True(t, called)
}

func TestValidateInterval(t *testing.T) {
	assert.Error(t, ValidateInterval(59*time.Minute))
	assert.Error(t, ValidateInterval(time.Second))
	assert.NoError(t, ValidateInterval(time.Hour))
	assert.NoError(t, ValidateInterval(24*time.Hour))
}

func TestLockKey_StableAndDistinct(t *testing.T) {
	// Deterministic across calls (advisory-lock correctness needs stability).
	assert.Equal(t, lockKey("sched-1"), lockKey("sched-1"))
	// Distinct schedules claim distinct locks.
	assert.NotEqual(t, lockKey("sched-1"), lockKey("sched-2"))
	// A UUID-shaped id (what the repo produces) works.
	assert.NotZero(t, lockKey("11111111-2222-3333-4444-555555555555"))
}
