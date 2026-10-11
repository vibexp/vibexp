package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidSiteInfo is returned when a submitted set of site info values is
// rejected. It wraps a *SettingsFieldError naming the field at fault.
var ErrInvalidSiteInfo = errors.New("invalid site info")

// InstanceSiteInfoResolver resolves the instance's company branding that
// applies right now.
//
// Resolve deliberately returns no error: it fails open to "nothing configured"
// (see InstanceSiteInfoService.Resolve).
type InstanceSiteInfoResolver interface {
	Resolve(ctx context.Context) models.InstanceSiteInfoValues
}

// InstanceSiteInfoServiceInterface is the instance site info surface an
// instance admin edits (#1353). Instance-admin authorization is the route
// middleware's job, so no method takes a permission check of its own.
type InstanceSiteInfoServiceInterface interface {
	InstanceSiteInfoResolver
	// Get returns the stored values and the row's metadata. Unlike Resolve it
	// returns a repository error, so a caller sees real state.
	Get(ctx context.Context) (*models.InstanceSiteInfoView, error)
	// Update validates and stores a complete replacement set of values,
	// auditing the change in the same transaction. Values are trimmed first
	// and a blank one is stored as "not configured". Invalid input returns an
	// ErrInvalidSiteInfo-wrapped error (carrying a *SettingsFieldError) and
	// writes nothing. A non-nil expectedVersion makes it a compare-and-set
	// (repositories.InstanceSettingsNoStoredVersion expects nothing stored): a
	// mismatch returns repositories.ErrInstanceSettingsVersionConflict and
	// writes nothing.
	Update(
		ctx context.Context, actorUserID string, values models.InstanceSiteInfoValues, expectedVersion *int64,
	) (*models.InstanceSiteInfoView, error)
	// Reset drops the stored values so nothing is configured again, auditing
	// the change in the same transaction. Resetting with no stored row is a
	// no-op and writes no audit entry.
	Reset(ctx context.Context, actorUserID string) error
}

// InstanceSiteInfoService implements InstanceSiteInfoServiceInterface.
//
// Like InstanceSearchSettingsService it reads the row on every call and caches
// nothing: the lookup is one primary-key read of a singleton row, and a change
// must show on the next request on every replica.
type InstanceSiteInfoService struct {
	repo   repositories.InstanceSiteInfoRepository
	logger *slog.Logger
}

var _ InstanceSiteInfoServiceInterface = (*InstanceSiteInfoService)(nil)

// NewInstanceSiteInfoService creates a new InstanceSiteInfoService.
func NewInstanceSiteInfoService(
	repo repositories.InstanceSiteInfoRepository,
	logger *slog.Logger,
) *InstanceSiteInfoService {
	return &InstanceSiteInfoService{repo: repo, logger: logger}
}

// Resolve implements InstanceSiteInfoResolver.
//
// It FAILS OPEN: a repository error logs at warn and yields the empty value
// set, so a blip reading a branding row never blocks the sign-in page or an
// email. The cost of a wrong guess is an unbranded page, not a broken one.
func (s *InstanceSiteInfoService) Resolve(ctx context.Context) models.InstanceSiteInfoValues {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceSiteInfoNotFound) {
		return models.InstanceSiteInfoValues{}
	}
	if err != nil {
		s.logger.With("error", err).
			Warn("failed to read instance site info; treating it as not configured")
		return models.InstanceSiteInfoValues{}
	}
	return instanceSiteInfoValuesFromStored(stored)
}

// Get implements InstanceSiteInfoServiceInterface.
func (s *InstanceSiteInfoService) Get(ctx context.Context) (*models.InstanceSiteInfoView, error) {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceSiteInfoNotFound) {
		return &models.InstanceSiteInfoView{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("InstanceSiteInfoService.Get: %w", err)
	}
	return instanceSiteInfoView(stored), nil
}

// Update implements InstanceSiteInfoServiceInterface.
func (s *InstanceSiteInfoService) Update(
	ctx context.Context, actorUserID string, values models.InstanceSiteInfoValues, expectedVersion *int64,
) (*models.InstanceSiteInfoView, error) {
	values = NormalizeInstanceSiteInfo(values)
	if err := ValidateInstanceSiteInfo(values); err != nil {
		return nil, err
	}

	stored := &models.InstanceSiteInfo{
		CompanyName:  optionalSiteInfoValue(values.CompanyName),
		CompanyEmail: optionalSiteInfoValue(values.CompanyEmail),
		Website:      optionalSiteInfoValue(values.Website),
		SupportURL:   optionalSiteInfoValue(values.SupportURL),
		TermsURL:     optionalSiteInfoValue(values.TermsURL),
		PrivacyURL:   optionalSiteInfoValue(values.PrivacyURL),
		LogoURL:      optionalSiteInfoValue(values.LogoURL),
		LogoDarkURL:  optionalSiteInfoValue(values.LogoDarkURL),
		UpdatedBy:    optionalActor(actorUserID),
	}
	err := s.repo.UpsertAudited(ctx, stored, expectedVersion,
		instanceSiteInfoAuditFunc(models.InstanceSettingsAuditActionUpsert, actorUserID))
	if err != nil {
		return nil, fmt.Errorf("InstanceSiteInfoService.Update: %w", err)
	}

	return instanceSiteInfoView(stored), nil
}

// Reset implements InstanceSiteInfoServiceInterface.
func (s *InstanceSiteInfoService) Reset(ctx context.Context, actorUserID string) error {
	_, err := s.repo.DeleteAudited(ctx,
		instanceSiteInfoAuditFunc(models.InstanceSettingsAuditActionDelete, actorUserID))
	if err != nil {
		return fmt.Errorf("InstanceSiteInfoService.Reset: %w", err)
	}
	return nil
}

// instanceSiteInfoAuditFunc builds the audit entry for one change. The
// snapshots hold the eight values as-is: the table stores no credential, so
// there is nothing to redact.
func instanceSiteInfoAuditFunc(action, actorUserID string) repositories.InstanceSiteInfoAuditFunc {
	return func(before, after *models.InstanceSiteInfo) (*models.InstanceSettingsAuditEntry, error) {
		beforeDoc, err := instanceSiteInfoAuditSnapshot(before)
		if err != nil {
			return nil, err
		}
		afterDoc, err := instanceSiteInfoAuditSnapshot(after)
		if err != nil {
			return nil, err
		}
		return &models.InstanceSettingsAuditEntry{
			Setting:     models.InstanceSettingSiteInfo,
			Action:      action,
			ActorUserID: optionalActor(actorUserID),
			Before:      beforeDoc,
			After:       afterDoc,
		}, nil
	}
}

// instanceSiteInfoAuditSnapshot renders a row as its values, or nil for no row.
func instanceSiteInfoAuditSnapshot(row *models.InstanceSiteInfo) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}
	doc, err := json.Marshal(instanceSiteInfoValuesFromStored(row))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal instance site info snapshot: %w", err)
	}
	return doc, nil
}

// instanceSiteInfoView renders a stored row as the read model.
func instanceSiteInfoView(stored *models.InstanceSiteInfo) *models.InstanceSiteInfoView {
	updatedAt := stored.UpdatedAt
	version := stored.Version
	return &models.InstanceSiteInfoView{
		Values:    instanceSiteInfoValuesFromStored(stored),
		UpdatedAt: &updatedAt,
		UpdatedBy: stored.UpdatedBy,
		Version:   &version,
	}
}

// instanceSiteInfoValuesFromStored projects a persisted row onto its values; a
// NULL column is the empty string.
func instanceSiteInfoValuesFromStored(stored *models.InstanceSiteInfo) models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues{
		CompanyName:  optionalValue(stored.CompanyName),
		CompanyEmail: optionalValue(stored.CompanyEmail),
		Website:      optionalValue(stored.Website),
		SupportURL:   optionalValue(stored.SupportURL),
		TermsURL:     optionalValue(stored.TermsURL),
		PrivacyURL:   optionalValue(stored.PrivacyURL),
		LogoURL:      optionalValue(stored.LogoURL),
		LogoDarkURL:  optionalValue(stored.LogoDarkURL),
	}
}

// optionalSiteInfoValue maps an already-normalized value onto its column: nil
// (NULL) for the empty string.
func optionalSiteInfoValue(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// NormalizeInstanceSiteInfo trims surrounding whitespace from every value, so
// a blank submission becomes the empty string that means "not configured".
func NormalizeInstanceSiteInfo(v models.InstanceSiteInfoValues) models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues{
		CompanyName:  strings.TrimSpace(v.CompanyName),
		CompanyEmail: strings.TrimSpace(v.CompanyEmail),
		Website:      strings.TrimSpace(v.Website),
		SupportURL:   strings.TrimSpace(v.SupportURL),
		TermsURL:     strings.TrimSpace(v.TermsURL),
		PrivacyURL:   strings.TrimSpace(v.PrivacyURL),
		LogoURL:      strings.TrimSpace(v.LogoURL),
		LogoDarkURL:  strings.TrimSpace(v.LogoDarkURL),
	}
}

// ValidateInstanceSiteInfo checks an already-normalized value set (see
// NormalizeInstanceSiteInfo) and reports the first field at fault, in
// declaration order. Every field may be empty. A set one:
//
//   - company_name: at most models.MaxSiteInfoCompanyNameLength characters.
//   - company_email: at most models.MaxSiteInfoEmailLength characters, and one
//     bare address with no display name.
//   - each URL: at most models.MaxSiteInfoURLLength characters, and an absolute
//     http or https URL with a host. These are rendered as links and image
//     sources on pages every visitor sees, so the scheme allowlist is what
//     keeps a `javascript:` or `data:` URL out.
//
// The instance_site_info CHECK constraints enforce the same length caps in the
// database.
func ValidateInstanceSiteInfo(v models.InstanceSiteInfoValues) error {
	if err := validateSiteInfoLength("company_name", v.CompanyName, models.MaxSiteInfoCompanyNameLength); err != nil {
		return err
	}
	if err := validateSiteInfoEmail("company_email", v.CompanyEmail); err != nil {
		return err
	}
	urls := []struct{ field, value string }{
		{"website", v.Website},
		{"support_url", v.SupportURL},
		{"terms_url", v.TermsURL},
		{"privacy_url", v.PrivacyURL},
		{"logo_url", v.LogoURL},
		{"logo_dark_url", v.LogoDarkURL},
	}
	for _, u := range urls {
		if err := validateSiteInfoURL(u.field, u.value); err != nil {
			return err
		}
	}
	return nil
}

// validateSiteInfoLength rejects a value longer than limit characters. It
// counts characters, as the CHECK constraints' char_length does.
func validateSiteInfoLength(field, value string, limit int) error {
	if n := utf8.RuneCountInString(value); n > limit {
		return fmt.Errorf("%w: %w", ErrInvalidSiteInfo, settingsFieldError(
			[]string{field}, "%s must be at most %d characters, got %d", field, limit, n))
	}
	return nil
}

func validateSiteInfoEmail(field, value string) error {
	if value == "" {
		return nil
	}
	if err := validateSiteInfoLength(field, value, models.MaxSiteInfoEmailLength); err != nil {
		return err
	}
	if !isBareEmailAddress(value) {
		return fmt.Errorf("%w: %w", ErrInvalidSiteInfo, settingsInvalidValueError(
			[]string{field}, "%s must be a valid email address", field))
	}
	return nil
}

func validateSiteInfoURL(field, value string) error {
	if value == "" {
		return nil
	}
	if err := validateSiteInfoLength(field, value, models.MaxSiteInfoURLLength); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: %w", ErrInvalidSiteInfo, settingsInvalidValueError(
			[]string{field}, "%s must be an absolute http or https URL", field))
	}
	return nil
}
