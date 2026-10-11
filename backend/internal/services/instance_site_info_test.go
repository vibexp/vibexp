package services_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

func newInstanceSiteInfoService(t *testing.T, logs *bytes.Buffer) (
	*services.InstanceSiteInfoService, *repomocks.MockInstanceSiteInfoRepository,
) {
	t.Helper()
	repo := repomocks.NewMockInstanceSiteInfoRepository(t)
	return services.NewInstanceSiteInfoService(repo, testLogger(logs)), repo
}

func siteInfoString(s string) *string { return &s }

// storedSiteInfoRow has a name, a support URL and a logo; the rest is not
// configured.
func storedSiteInfoRow() *models.InstanceSiteInfo {
	updatedBy := testInstanceAdminID
	return &models.InstanceSiteInfo{
		CompanyName: siteInfoString("Old Co"),
		SupportURL:  siteInfoString("https://old.test/help"),
		LogoURL:     siteInfoString("https://old.test/logo.png"),
		UpdatedAt:   time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
		UpdatedBy:   &updatedBy,
		Version:     3,
	}
}

func validSiteInfoValues() models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues{
		CompanyName:  "Acme Inc.",
		CompanyEmail: "hello@acme.test",
		Website:      "https://acme.test",
		SupportURL:   "https://acme.test/help",
		TermsURL:     "https://acme.test/terms",
		PrivacyURL:   "https://acme.test/privacy",
		LogoURL:      "https://cdn.acme.test/logo.svg",
		LogoDarkURL:  "http://cdn.acme.test/logo-dark.svg",
	}
}

func TestInstanceSiteInfoService_Resolve_NoRowIsEmpty(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSiteInfoNotFound)

	assert.Equal(t, models.InstanceSiteInfoValues{}, svc.Resolve(context.Background()))
}

func TestInstanceSiteInfoService_Resolve_StoredRowIsUsed(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(storedSiteInfoRow(), nil)

	assert.Equal(t, models.InstanceSiteInfoValues{
		CompanyName: "Old Co",
		SupportURL:  "https://old.test/help",
		LogoURL:     "https://old.test/logo.png",
	}, svc.Resolve(context.Background()))
}

// A failed read must never break the page or email that asked: Resolve logs
// and answers "nothing configured".
func TestInstanceSiteInfoService_Resolve_FailsOpenOnRepositoryError(t *testing.T) {
	var logs bytes.Buffer
	svc, repo := newInstanceSiteInfoService(t, &logs)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("connection reset"))

	assert.Equal(t, models.InstanceSiteInfoValues{}, svc.Resolve(context.Background()))
	assert.Contains(t, logs.String(), "failed to read instance site info")
	assert.Contains(t, logs.String(), "connection reset")
}

func TestInstanceSiteInfoService_Get_NoRowIsAnEmptyView(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSiteInfoNotFound)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, &models.InstanceSiteInfoView{}, view)
}

func TestInstanceSiteInfoService_Get_StoredRowCarriesMetadata(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	row := storedSiteInfoRow()
	repo.EXPECT().Get(mock.Anything).Return(row, nil)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Old Co", view.Values.CompanyName)
	assert.Empty(t, view.Values.Website)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, row.UpdatedAt, *view.UpdatedAt)
	assert.Equal(t, row.UpdatedBy, view.UpdatedBy)
	require.NotNil(t, view.Version)
	assert.Equal(t, int64(3), *view.Version)
}

func TestInstanceSiteInfoService_Get_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("boom"))

	_, err := svc.Get(context.Background())

	assert.ErrorContains(t, err, "boom", "unlike Resolve, Get must not fail open: an admin must see real state")
}

func TestInstanceSiteInfoService_Update_StoresAndAudits(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	before := storedSiteInfoRow()
	expected := int64(3)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceSiteInfo, _ *int64, audit repositories.InstanceSiteInfoAuditFunc,
		) error {
			require.NotNil(t, s.CompanyName)
			assert.Equal(t, "Acme Inc.", *s.CompanyName)
			require.NotNil(t, s.UpdatedBy)
			assert.Equal(t, testInstanceAdminID, *s.UpdatedBy)
			s.UpdatedAt = time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
			s.Version = 4
			var err error
			entry, err = audit(before, s)
			return err
		})

	view, err := svc.Update(context.Background(), testInstanceAdminID, validSiteInfoValues(), &expected)

	require.NoError(t, err)
	assert.Equal(t, validSiteInfoValues(), view.Values)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC), *view.UpdatedAt)
	require.NotNil(t, view.Version)
	assert.Equal(t, int64(4), *view.Version)

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingSiteInfo, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testInstanceAdminID, *entry.ActorUserID)
	assert.JSONEq(t, `{"company_name":"Old Co","company_email":"","website":"",
		"support_url":"https://old.test/help","terms_url":"","privacy_url":"",
		"logo_url":"https://old.test/logo.png","logo_dark_url":""}`, string(entry.Before))
	assert.JSONEq(t, `{"company_name":"Acme Inc.","company_email":"hello@acme.test",
		"website":"https://acme.test","support_url":"https://acme.test/help",
		"terms_url":"https://acme.test/terms","privacy_url":"https://acme.test/privacy",
		"logo_url":"https://cdn.acme.test/logo.svg",
		"logo_dark_url":"http://cdn.acme.test/logo-dark.svg"}`, string(entry.After))
}

// Values are trimmed, and a blank one is stored as NULL rather than as an
// empty or whitespace string.
func TestInstanceSiteInfoService_Update_TrimsAndStoresBlankAsNil(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceSiteInfo, _ *int64, _ repositories.InstanceSiteInfoAuditFunc,
		) error {
			require.NotNil(t, s.CompanyName)
			assert.Equal(t, "Acme Inc.", *s.CompanyName)
			require.NotNil(t, s.Website)
			assert.Equal(t, "https://acme.test", *s.Website)
			assert.Nil(t, s.CompanyEmail)
			assert.Nil(t, s.SupportURL, "whitespace only is not configured")
			assert.Nil(t, s.TermsURL)
			assert.Nil(t, s.PrivacyURL)
			assert.Nil(t, s.LogoURL)
			assert.Nil(t, s.LogoDarkURL)
			return nil
		})

	view, err := svc.Update(context.Background(), testInstanceAdminID, models.InstanceSiteInfoValues{
		CompanyName: "  Acme Inc.\n",
		Website:     "\thttps://acme.test ",
		SupportURL:  "   ",
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, models.InstanceSiteInfoValues{CompanyName: "Acme Inc.", Website: "https://acme.test"}, view.Values)
}

// Saving with everything blank is still a save: it stores an all-NULL row
// (only Reset deletes).
func TestInstanceSiteInfoService_Update_AllBlankStillWrites(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	view, err := svc.Update(context.Background(), testInstanceAdminID, models.InstanceSiteInfoValues{}, nil)

	require.NoError(t, err)
	assert.Equal(t, models.InstanceSiteInfoValues{}, view.Values)
}

// A first save has no previous row: the entry's before is empty.
func TestInstanceSiteInfoService_Update_FirstSaveHasNoBefore(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceSiteInfo, _ *int64, audit repositories.InstanceSiteInfoAuditFunc,
		) error {
			var err error
			entry, err = audit(nil, s)
			return err
		})

	_, err := svc.Update(context.Background(), "  ", validSiteInfoValues(), nil)

	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Nil(t, entry.Before)
	assert.NotNil(t, entry.After)
	assert.Nil(t, entry.ActorUserID, "a blank actor is recorded as no actor")
}

func TestInstanceSiteInfoService_Update_VersionConflictPropagates(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(repositories.ErrInstanceSettingsVersionConflict)

	stale := int64(2)
	_, err := svc.Update(context.Background(), testInstanceAdminID, validSiteInfoValues(), &stale)

	assert.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
}

func TestInstanceSiteInfoService_Update_RejectsInvalidValuesWithoutWriting(t *testing.T) {
	// No repo expectations: an invalid set must write no row and no audit entry.
	svc, _ := newInstanceSiteInfoService(t, nil)
	invalid := validSiteInfoValues()
	invalid.LogoURL = "javascript:alert(1)"

	_, err := svc.Update(context.Background(), testInstanceAdminID, invalid, nil)

	assert.ErrorIs(t, err, services.ErrInvalidSiteInfo)
}

func TestInstanceSiteInfoService_Reset_DeletesAndAudits(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, audit repositories.InstanceSiteInfoAuditFunc) (bool, error) {
			var err error
			entry, err = audit(storedSiteInfoRow(), nil)
			return true, err
		})

	require.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingSiteInfo, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entry.Action)
	assert.NotNil(t, entry.Before)
	assert.Nil(t, entry.After)
}

func TestInstanceSiteInfoService_Reset_NoRowIsNotAnError(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil)

	assert.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))
}

func TestInstanceSiteInfoService_Reset_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceSiteInfoService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, errors.New("boom"))

	assert.ErrorContains(t, svc.Reset(context.Background(), testInstanceAdminID), "boom")
}

func TestNormalizeInstanceSiteInfo_TrimsEveryField(t *testing.T) {
	padded := models.InstanceSiteInfoValues{
		CompanyName:  " Acme Inc. ",
		CompanyEmail: " hello@acme.test ",
		Website:      " https://acme.test ",
		SupportURL:   " https://acme.test/help ",
		TermsURL:     " https://acme.test/terms ",
		PrivacyURL:   " https://acme.test/privacy ",
		LogoURL:      " https://cdn.acme.test/logo.svg ",
		LogoDarkURL:  " http://cdn.acme.test/logo-dark.svg ",
	}

	assert.Equal(t, validSiteInfoValues(), services.NormalizeInstanceSiteInfo(padded))
}

func TestValidateInstanceSiteInfo_Accepts(t *testing.T) {
	longURL := "https://acme.test/" + strings.Repeat("a", models.MaxSiteInfoURLLength-len("https://acme.test/"))
	cases := map[string]models.InstanceSiteInfoValues{
		"nothing configured": {},
		"every field set":    validSiteInfoValues(),
		"a name at the cap, counted in characters": {
			CompanyName: strings.Repeat("é", models.MaxSiteInfoCompanyNameLength),
		},
		"a URL at the cap":             {Website: longURL},
		"a URL with a port and query":  {SupportURL: "http://localhost:8080/help?lang=en#top"},
		"an upper-case scheme":         {TermsURL: "HTTPS://acme.test/terms"},
		"only the dark logo":           {LogoDarkURL: "https://cdn.acme.test/dark.png"},
		"an email with a plus address": {CompanyEmail: "hello+support@acme.test"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, services.ValidateInstanceSiteInfo(values))
		})
	}
}

func TestValidateInstanceSiteInfo_Rejects(t *testing.T) {
	longEmail := strings.Repeat("a", models.MaxSiteInfoEmailLength) + "@acme.test"
	longURL := "https://acme.test/" + strings.Repeat("a", models.MaxSiteInfoURLLength)
	cases := []struct {
		name   string
		values models.InstanceSiteInfoValues
		field  string
		code   string
	}{
		{"a name over the cap", models.InstanceSiteInfoValues{
			CompanyName: strings.Repeat("a", models.MaxSiteInfoCompanyNameLength+1),
		}, "company_name", services.SettingsFieldOutOfRange},
		{"an email over the cap", models.InstanceSiteInfoValues{CompanyEmail: longEmail},
			"company_email", services.SettingsFieldOutOfRange},
		{"an email without a domain", models.InstanceSiteInfoValues{CompanyEmail: "hello"},
			"company_email", services.SettingsFieldInvalidValue},
		{"an email with a display name", models.InstanceSiteInfoValues{CompanyEmail: "Acme <hello@acme.test>"},
			"company_email", services.SettingsFieldInvalidValue},
		{"two addresses", models.InstanceSiteInfoValues{CompanyEmail: "a@acme.test, b@acme.test"},
			"company_email", services.SettingsFieldInvalidValue},
		{"a URL over the cap", models.InstanceSiteInfoValues{Website: longURL},
			"website", services.SettingsFieldOutOfRange},
		{"a javascript URL", models.InstanceSiteInfoValues{Website: "javascript:alert(1)"},
			"website", services.SettingsFieldInvalidValue},
		{"a data URL", models.InstanceSiteInfoValues{LogoURL: "data:image/svg+xml;base64,PHN2Zz4="},
			"logo_url", services.SettingsFieldInvalidValue},
		{"a mailto URL", models.InstanceSiteInfoValues{SupportURL: "mailto:help@acme.test"},
			"support_url", services.SettingsFieldInvalidValue},
		{"a relative URL", models.InstanceSiteInfoValues{TermsURL: "/terms"},
			"terms_url", services.SettingsFieldInvalidValue},
		{"a scheme-relative URL", models.InstanceSiteInfoValues{PrivacyURL: "//acme.test/privacy"},
			"privacy_url", services.SettingsFieldInvalidValue},
		{"a URL without a host", models.InstanceSiteInfoValues{LogoDarkURL: "https:///logo.png"},
			"logo_dark_url", services.SettingsFieldInvalidValue},
		{"an unparseable URL", models.InstanceSiteInfoValues{Website: "https://acme.test/%zz"},
			"website", services.SettingsFieldInvalidValue},
		{"an ftp URL", models.InstanceSiteInfoValues{LogoURL: "ftp://acme.test/logo.png"},
			"logo_url", services.SettingsFieldInvalidValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := services.ValidateInstanceSiteInfo(tc.values)

			require.ErrorIs(t, err, services.ErrInvalidSiteInfo)
			var fieldErr *services.SettingsFieldError
			require.ErrorAs(t, err, &fieldErr)
			assert.Equal(t, []string{tc.field}, fieldErr.Fields)
			assert.Equal(t, tc.code, fieldErr.Code)
		})
	}
}

// The first field at fault, in declaration order, is the one reported.
func TestValidateInstanceSiteInfo_ReportsTheFirstInvalidField(t *testing.T) {
	err := services.ValidateInstanceSiteInfo(models.InstanceSiteInfoValues{
		CompanyEmail: "nope",
		Website:      "javascript:alert(1)",
	})

	var fieldErr *services.SettingsFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, []string{"company_email"}, fieldErr.Fields)
}
