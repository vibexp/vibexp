// Package memauthsettings is an in-memory stand-in for the instance
// authentication settings storage (providers, access allowlist, shared version,
// admin grants and their audit log), for tests that drive the real services
// without a database. It keeps the contracts the services rely on: every
// provider and allowlist write is a compare-and-set that bumps the shared
// version by one and appends one redacted audit entry.
package memauthsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Store holds the settings. The zero value is not usable; call New.
type Store struct {
	mu        sync.Mutex
	version   int64
	providers []*models.InstanceAuthProvider
	allowlist *models.InstanceAuthAllowlist
	grants    []*models.InstanceAdminGrant
	audit     []*models.InstanceSettingsAuditEntry
	clock     time.Time
	nextID    int

	// Outside answers the allowlist's CountUsersOutside; nil counts nobody.
	Outside func(domains, emails, exempt []string) (count int, sample []string)
	// Err, when set, fails every repository call.
	Err error
}

// New creates an empty store at version 1.
func New() *Store {
	return &Store{version: 1, clock: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
}

// Providers is the store's InstanceAuthProviderRepository.
func (s *Store) Providers() repositories.InstanceAuthProviderRepository { return providerRepo{s: s} }

// Allowlist is the store's InstanceAuthAllowlistRepository.
func (s *Store) Allowlist() repositories.InstanceAuthAllowlistRepository { return allowlistRepo{s: s} }

// Versions is the store's InstanceAuthSettingsVersionRepository.
func (s *Store) Versions() repositories.InstanceAuthSettingsVersionRepository {
	return versionRepo{s: s}
}

// Grants is the store's InstanceAdminRepository.
func (s *Store) Grants() repositories.InstanceAdminRepository { return grantRepo{s: s} }

// AuditLog is the store's InstanceSettingsAuditRepository.
func (s *Store) AuditLog() repositories.InstanceSettingsAuditRepository { return auditRepo{s: s} }

// Version returns the shared auth settings version.
func (s *Store) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// Provider returns a copy of the stored provider with slug, or nil.
func (s *Store) Provider(slug string) *models.InstanceAuthProvider {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.providers {
		if p.Slug == slug {
			c := *p
			return &c
		}
	}
	return nil
}

// AuditEntries returns the entries recorded for setting, oldest first.
func (s *Store) AuditEntries(setting string) []*models.InstanceSettingsAuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*models.InstanceSettingsAuditEntry
	for _, e := range s.audit {
		if e.Setting == setting {
			out = append(out, e)
		}
	}
	return out
}

// AppendAudit records an entry as stored by another writer (the setup-mode
// repository, say), stamping its id and time.
func (s *Store) AppendAudit(entry *models.InstanceSettingsAuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendLocked(entry)
}

func (s *Store) tick() time.Time {
	s.clock = s.clock.Add(time.Minute)
	return s.clock
}

func (s *Store) newID() string {
	s.nextID++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.nextID)
}

func (s *Store) appendLocked(entry *models.InstanceSettingsAuditEntry) {
	entry.ID = s.newID()
	entry.CreatedAt = s.tick()
	s.audit = append(s.audit, entry)
}

func (s *Store) record(setting, action string, actor *string, before, after json.RawMessage) {
	s.appendLocked(&models.InstanceSettingsAuditEntry{
		Setting: setting, Action: action, ActorUserID: actor, Before: before, After: after,
	})
}

// cas checks expected against the shared version; nil is last-write-wins.
func (s *Store) cas(expected *int64) error {
	if expected != nil && *expected != s.version {
		return repositories.ErrInstanceSettingsVersionConflict
	}
	return nil
}

func snapshot(v any, extra map[string]any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	if len(extra) == 0 {
		return raw
	}
	doc := map[string]any{}
	if err = json.Unmarshal(raw, &doc); err != nil {
		panic(err)
	}
	for k, val := range extra {
		doc[k] = val
	}
	raw, err = json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return raw
}

// providerSnapshot mirrors the PostgreSQL repository's redacted audit shape.
func providerSnapshot(p *models.InstanceAuthProvider, marker string) json.RawMessage {
	if p == nil {
		return nil
	}
	extra := map[string]any{"has_client_secret": p.HasClientSecret()}
	if marker != "" {
		extra["client_secret"] = marker
	}
	return snapshot(p, extra)
}

func secretMarker(before, after *models.InstanceAuthProvider) string {
	var was, is string
	if before != nil && before.ClientSecretEncrypted != nil {
		was = *before.ClientSecretEncrypted
	}
	if after.ClientSecretEncrypted != nil {
		is = *after.ClientSecretEncrypted
	}
	if was == is {
		return models.InstanceSettingsAuditSecretUnchanged
	}
	return models.InstanceSettingsAuditSecretChanged
}

// --- providers ----------------------------------------------------------------

type providerRepo struct{ s *Store }

func (r providerRepo) List(context.Context) ([]*models.InstanceAuthProvider, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return nil, r.s.Err
	}
	out := make([]*models.InstanceAuthProvider, 0, len(r.s.providers))
	for _, p := range r.s.providers {
		c := *p
		out = append(out, &c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

func (r providerRepo) Get(_ context.Context, id string) (*models.InstanceAuthProvider, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return nil, r.s.Err
	}
	for _, p := range r.s.providers {
		if p.ID == id {
			c := *p
			return &c, nil
		}
	}
	return nil, repositories.ErrInstanceAuthProviderNotFound
}

func (r providerRepo) GetBySlug(_ context.Context, slug string) (*models.InstanceAuthProvider, error) {
	if p := r.s.Provider(slug); p != nil {
		return p, nil
	}
	return nil, repositories.ErrInstanceAuthProviderNotFound
}

func (r providerRepo) Create(
	_ context.Context, provider *models.InstanceAuthProvider, actor *string, expected *int64,
) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return r.s.Err
	}
	if err := r.s.cas(expected); err != nil {
		return err
	}
	for _, p := range r.s.providers {
		singleton := provider.Type != models.InstanceAuthProviderOIDC && p.Type == provider.Type
		if p.Slug == provider.Slug || singleton {
			return repositories.ErrInstanceAuthProviderConflict
		}
	}
	provider.ID = r.s.newID()
	provider.CreatedAt = r.s.tick()
	provider.UpdatedAt = provider.CreatedAt
	provider.UpdatedBy = actor
	stored := *provider
	r.s.providers = append(r.s.providers, &stored)
	r.s.version++
	r.s.record(models.InstanceSettingAuthProviders, models.InstanceSettingsAuditActionUpsert, actor,
		nil, providerSnapshot(&stored, secretMarker(nil, &stored)))
	return nil
}

func (r providerRepo) Update(
	_ context.Context, provider *models.InstanceAuthProvider, actor *string, expected *int64,
) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return r.s.Err
	}
	if err := r.s.cas(expected); err != nil {
		return err
	}
	for i, p := range r.s.providers {
		if p.ID != provider.ID {
			continue
		}
		before := *p
		provider.Slug, provider.Type, provider.CreatedAt = before.Slug, before.Type, before.CreatedAt
		provider.UpdatedAt = r.s.tick()
		provider.UpdatedBy = actor
		stored := *provider
		r.s.providers[i] = &stored
		r.s.version++
		r.s.record(models.InstanceSettingAuthProviders, models.InstanceSettingsAuditActionUpsert, actor,
			providerSnapshot(&before, ""), providerSnapshot(&stored, secretMarker(&before, &stored)))
		return nil
	}
	return repositories.ErrInstanceAuthProviderNotFound
}

func (r providerRepo) Delete(_ context.Context, id string, actor *string, expected *int64) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return r.s.Err
	}
	if err := r.s.cas(expected); err != nil {
		return err
	}
	for i, p := range r.s.providers {
		if p.ID != id {
			continue
		}
		r.s.providers = append(r.s.providers[:i], r.s.providers[i+1:]...)
		r.s.version++
		r.s.record(models.InstanceSettingAuthProviders, models.InstanceSettingsAuditActionDelete, actor,
			providerSnapshot(p, ""), nil)
		return nil
	}
	return repositories.ErrInstanceAuthProviderNotFound
}

func (r providerRepo) InsertIfEmpty(context.Context, []*models.InstanceAuthProvider) (bool, error) {
	return false, fmt.Errorf("memauthsettings: InsertIfEmpty is not supported")
}

// --- allowlist ----------------------------------------------------------------

type allowlistRepo struct{ s *Store }

func (r allowlistRepo) Get(context.Context) (*models.InstanceAuthAllowlist, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return nil, r.s.Err
	}
	if r.s.allowlist == nil {
		return nil, repositories.ErrInstanceAuthAllowlistNotFound
	}
	c := *r.s.allowlist
	return &c, nil
}

func (r allowlistRepo) UpsertAudited(
	_ context.Context, allowlist *models.InstanceAuthAllowlist, actor *string, expected *int64,
) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return r.s.Err
	}
	before := r.s.allowlist
	var storedVersion *int64
	if before != nil {
		storedVersion = &before.Version
	}
	if repositories.InstanceSettingsVersionConflicts(expected, storedVersion) {
		return repositories.ErrInstanceSettingsVersionConflict
	}
	allowlist.UpdatedAt = r.s.tick()
	allowlist.CreatedAt = allowlist.UpdatedAt
	allowlist.Version = 1
	var beforeDoc json.RawMessage
	if before != nil {
		allowlist.CreatedAt = before.CreatedAt
		allowlist.Version = before.Version + 1
		beforeDoc = snapshot(before, nil)
	}
	allowlist.UpdatedBy = actor
	stored := *allowlist
	r.s.allowlist = &stored
	r.s.version++
	r.s.record(models.InstanceSettingAuthAllowlist, models.InstanceSettingsAuditActionUpsert, actor,
		beforeDoc, snapshot(&stored, nil))
	return nil
}

func (r allowlistRepo) DeleteAudited(_ context.Context, actor *string) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return false, r.s.Err
	}
	if r.s.allowlist == nil {
		return false, nil
	}
	before := snapshot(r.s.allowlist, nil)
	r.s.allowlist = nil
	r.s.version++
	r.s.record(models.InstanceSettingAuthAllowlist, models.InstanceSettingsAuditActionDelete, actor, before, nil)
	return true, nil
}

func (r allowlistRepo) InsertIfAbsent(context.Context, *models.InstanceAuthAllowlist) (bool, error) {
	return false, fmt.Errorf("memauthsettings: InsertIfAbsent is not supported")
}

func (r allowlistRepo) CountUsersOutside(
	_ context.Context, domains, emails, exempt []string, sampleLimit int,
) (int, []string, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return 0, nil, r.s.Err
	}
	if r.s.Outside == nil {
		return 0, nil, nil
	}
	count, sample := r.s.Outside(domains, emails, exempt)
	if len(sample) > sampleLimit {
		sample = sample[:sampleLimit]
	}
	return count, sample, nil
}

// --- version ------------------------------------------------------------------

type versionRepo struct{ s *Store }

func (r versionRepo) Get(context.Context) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return r.s.version, r.s.Err
}

// --- admin grants -------------------------------------------------------------

type grantRepo struct{ s *Store }

func (r grantRepo) List(context.Context) ([]*models.InstanceAdminGrant, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return nil, r.s.Err
	}
	out := make([]*models.InstanceAdminGrant, 0, len(r.s.grants))
	for _, g := range r.s.grants {
		c := *g
		out = append(out, &c)
	}
	return out, nil
}

func (r grantRepo) IsGranted(_ context.Context, userID string) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return false, r.s.Err
	}
	for _, g := range r.s.grants {
		if g.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (r grantRepo) Grant(_ context.Context, userID string, grantedBy *string) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return false, r.s.Err
	}
	for _, g := range r.s.grants {
		if g.UserID == userID {
			return false, nil
		}
	}
	grant := &models.InstanceAdminGrant{UserID: userID, GrantedBy: grantedBy, CreatedAt: r.s.tick()}
	r.s.grants = append(r.s.grants, grant)
	r.s.record(models.InstanceSettingInstanceAdmins, models.InstanceSettingsAuditActionUpsert, grantedBy,
		nil, snapshot(grant, nil))
	return true, nil
}

func (r grantRepo) Revoke(_ context.Context, userID string, actor *string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return r.s.Err
	}
	for i, g := range r.s.grants {
		if g.UserID != userID {
			continue
		}
		r.s.grants = append(r.s.grants[:i], r.s.grants[i+1:]...)
		r.s.record(models.InstanceSettingInstanceAdmins, models.InstanceSettingsAuditActionDelete, actor,
			snapshot(g, nil), nil)
		return nil
	}
	return repositories.ErrInstanceAdminNotFound
}

// --- audit log ----------------------------------------------------------------

type auditRepo struct{ s *Store }

func (r auditRepo) Append(_ context.Context, entry *models.InstanceSettingsAuditEntry) error {
	r.s.AppendAudit(entry)
	return nil
}

func (r auditRepo) List(
	_ context.Context, setting string, limit int, cursor *models.InstanceSettingsAuditCursor,
) ([]*models.InstanceSettingsAuditEntry, *models.InstanceSettingsAuditCursor, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.Err != nil {
		return nil, nil, r.s.Err
	}
	var matching []*models.InstanceSettingsAuditEntry
	for i := len(r.s.audit) - 1; i >= 0; i-- {
		e := r.s.audit[i]
		if e.Setting == setting && (cursor == nil || e.CreatedAt.Before(cursor.CreatedAt)) {
			matching = append(matching, e)
		}
	}
	if len(matching) <= limit {
		return matching, nil, nil
	}
	last := matching[limit-1]
	return matching[:limit], &models.InstanceSettingsAuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}
