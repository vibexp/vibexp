//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The instance settings observer (#1515) against real Postgres: every
// committed change is reported exactly once, after its transaction commits,
// and a rolled-back change is never reported.

// observedSettings is an InstanceSettingsAuditObserver that keeps every entry.
type observedSettings struct {
	mu      sync.Mutex
	entries []models.InstanceSettingsAuditEntry
}

func (o *observedSettings) OnInstanceSettingsAudited(_ context.Context, e models.InstanceSettingsAuditEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.entries = append(o.entries, e)
}

func (o *observedSettings) settings() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.entries))
	for i, e := range o.entries {
		out[i] = e.Setting
	}
	return out
}

// observeSettings registers a fresh observer on the integration database for
// the length of the test.
func observeSettings(t *testing.T) *observedSettings {
	t.Helper()
	o := &observedSettings{}
	t.Cleanup(RegisterInstanceSettingsAuditObserver(integrationDB, o))
	return o
}

func settingsAuditEntry(setting string) *models.InstanceSettingsAuditEntry {
	return &models.InstanceSettingsAuditEntry{
		Setting: setting, Action: models.InstanceSettingsAuditActionUpsert, After: []byte(`{"changed":true}`),
	}
}

func searchAudit(setting string) repositories.InstanceSearchSettingsAuditFunc {
	return func(_, _ *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
		return settingsAuditEntry(setting), nil
	}
}

func searchSettingsRow() *models.InstanceSearchSettings {
	return &models.InstanceSearchSettings{RankWeightRelevance: 1, RankHalfLifeDays: 30, RankCandidateCap: 100}
}

func TestIntegrationInstanceSettingsObserver_OneReportPerCommittedChange(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		setting string
		prepare func(t *testing.T)
		write   func(t *testing.T) error
	}{
		{
			name: "search", setting: models.InstanceSettingSearch,
			prepare: func(t *testing.T) { resetInstanceSettingsTable(t, "instance_search_settings") },
			write: func(*testing.T) error {
				return NewInstanceSearchSettingsRepository(integrationDB).
					UpsertAudited(ctx, searchSettingsRow(), nil, searchAudit(models.InstanceSettingSearch))
			},
		},
		{
			name: "site info", setting: models.InstanceSettingSiteInfo,
			prepare: func(t *testing.T) { resetInstanceSettingsTable(t, "instance_site_info") },
			write: func(*testing.T) error {
				return NewInstanceSiteInfoRepository(integrationDB).UpsertAudited(ctx,
					&models.InstanceSiteInfo{SupportURL: siteInfoPtr("https://acme.test/help")}, nil,
					func(_, _ *models.InstanceSiteInfo) (*models.InstanceSettingsAuditEntry, error) {
						return settingsAuditEntry(models.InstanceSettingSiteInfo), nil
					})
			},
		},
		{
			name: "auth policy", setting: models.InstanceSettingAuthPolicy,
			prepare: func(t *testing.T) { resetInstanceSettingsTable(t, "instance_auth_policy") },
			write: func(t *testing.T) error {
				actor := insertTestUser(t)
				return NewInstanceAuthPolicyRepository(integrationDB).UpsertAudited(ctx,
					&models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeOpen},
					&actor, nil)
			},
		},
		{
			name: "instance admin grant", setting: models.InstanceSettingInstanceAdmins,
			prepare: func(t *testing.T) { resetInstanceSettingsTable(t, "instance_admins") },
			write: func(t *testing.T) error {
				grantor, grantee := insertTestUser(t), insertTestUser(t)
				_, err := NewInstanceAdminRepository(integrationDB).Grant(ctx, grantee, &grantor)
				return err
			},
		},
		{
			name: "audit-log settings", setting: models.InstanceSettingAuditLog,
			prepare: func(t *testing.T) { resetInstanceSettingsTable(t, instanceAuditSettingsTable) },
			write: func(*testing.T) error {
				return NewInstanceAuditSettingsRepository(integrationDB).UpsertAudited(ctx,
					&models.InstanceAuditSettings{RetentionWeeks: 4, CaptureIPUA: true}, nil, upsertAuditSettingsAudit())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetInstanceSettingsAuditTables(t)
			tc.prepare(t)
			observed := observeSettings(t)

			require.NoError(t, tc.write(t))

			assert.Equal(t, []string{tc.setting}, observed.settings(), "exactly one report, for this setting")
		})
	}
}

func TestIntegrationInstanceSettingsObserver_RolledBackChangeIsNeverReported(t *testing.T) {
	ctx := context.Background()
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	require.NoError(t, repo.UpsertAudited(ctx, searchSettingsRow(), nil, searchAudit(models.InstanceSettingSearch)))
	observed := observeSettings(t)

	t.Run("a version conflict", func(t *testing.T) {
		stale := int64(99)
		err := repo.UpsertAudited(ctx, searchSettingsRow(), &stale, searchAudit(models.InstanceSettingSearch))
		require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
	})

	t.Run("a builder that fails after the write", func(t *testing.T) {
		fail := func(_, _ *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
			return nil, errors.New("boom")
		}
		require.Error(t, repo.UpsertAudited(ctx, searchSettingsRow(), nil, fail))
	})

	assert.Empty(t, observed.settings(), "nothing that rolled back is reported")
	held := 0
	pendingSettingsAudits.Range(func(any, any) bool { held++; return true })
	assert.Zero(t, held, "a rolled-back transaction's entries are dropped, not kept")
}

func TestIntegrationInstanceSettingsObserver_StandaloneAppendIsReported(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	observed := observeSettings(t)

	require.NoError(t, NewInstanceSettingsAuditRepository(integrationDB).
		Append(context.Background(), settingsAuditEntry(models.InstanceSettingEmailProvider)))

	assert.Equal(t, []string{models.InstanceSettingEmailProvider}, observed.settings())
}

func TestIntegrationInstanceSettingsObserver_UnregisteredReportsNothing(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	o := &observedSettings{}
	RegisterInstanceSettingsAuditObserver(integrationDB, o)()

	require.NoError(t, NewInstanceSettingsAuditRepository(integrationDB).
		Append(context.Background(), settingsAuditEntry(models.InstanceSettingEmailProvider)))

	assert.Empty(t, o.settings())
}
