package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// instanceSettingsAuditColumns is the canonical column list for
// instance_settings_audit projections; scanInstanceSettingsAuditDest reads them
// in this order.
const instanceSettingsAuditColumns = "id, setting, action, actor_user_id, before, after, created_at"

// Page-size bounds for List. A caller cannot request an unbounded read: a
// non-positive limit means the default, and anything above the maximum is
// clamped to it.
const (
	instanceSettingsAuditDefaultLimit = 50
	instanceSettingsAuditMaxLimit     = 100
)

// instanceSettingsAuditListQueryFmt lists one setting's entries newest first.
// %[1]s is the keyset predicate, present only when a cursor is given. The limit
// is always $2 so the two query shapes share their first placeholders.
const instanceSettingsAuditListQueryFmt = `
	SELECT ` + instanceSettingsAuditColumns + `
	FROM instance_settings_audit
	WHERE setting = $1%[1]s
	ORDER BY created_at DESC, id DESC
	LIMIT $2
`

// instanceSettingsAuditKeyset is the cursor predicate. Both placeholders are
// cast explicitly, so their types never depend on inference (lib/pq 42P08).
const instanceSettingsAuditKeyset = `
	AND (created_at, id) < ($3::timestamptz, $4::uuid)`

// instanceSettingsAuditSecretKeys are the snapshot keys that name a credential.
// Compared case-insensitively, so a differently-cased key cannot slip past.
var instanceSettingsAuditSecretKeys = []string{"secret", "secret_encrypted"}

// InstanceSettingsAuditRepository implements
// repositories.InstanceSettingsAuditRepository for PostgreSQL. The table is
// append-only: this repository deliberately exposes no update or delete.
type InstanceSettingsAuditRepository struct {
	db *database.DB
}

// NewInstanceSettingsAuditRepository creates a new InstanceSettingsAuditRepository.
func NewInstanceSettingsAuditRepository(db *database.DB) repositories.InstanceSettingsAuditRepository {
	return &InstanceSettingsAuditRepository{db: db}
}

// Append records one instance settings change and populates the model from
// the persisted row.
//
// The redaction guard runs first, so a snapshot carrying a credential never
// reaches the database. Building the redacted snapshot is the service's job
// (#1188); this is a defensive backstop, not the redaction itself.
func (r *InstanceSettingsAuditRepository) Append(
	ctx context.Context, entry *models.InstanceSettingsAuditEntry,
) error {
	if err := assertInstanceSettingsAuditRedacted(entry.Before, entry.After); err != nil {
		return err
	}

	query := `
		INSERT INTO instance_settings_audit (setting, action, actor_user_id, before, after)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + instanceSettingsAuditColumns

	if err := r.db.QueryRowContext(
		ctx, query,
		entry.Setting, entry.Action, entry.ActorUserID,
		nullableJSON(entry.Before), nullableJSON(entry.After),
	).Scan(scanInstanceSettingsAuditDest(entry)...); err != nil {
		return fmt.Errorf("failed to append instance settings audit entry: %w", err)
	}
	return nil
}

// List returns up to limit entries for one setting, newest first, strictly
// after cursor when one is given, plus the cursor for the next page (nil when
// the log is exhausted).
//
// It fetches one row more than the page to learn whether a next page exists,
// so the last page never hands back a cursor that leads to an empty page.
// Ordering breaks created_at ties on id: `now()` is transaction-start time, so
// entries written in one transaction share a created_at.
func (r *InstanceSettingsAuditRepository) List(
	ctx context.Context, setting string, limit int, cursor *models.InstanceSettingsAuditCursor,
) ([]*models.InstanceSettingsAuditEntry, *models.InstanceSettingsAuditCursor, error) {
	limit = clampInstanceSettingsAuditLimit(limit)

	predicate := ""
	args := []interface{}{setting, limit + 1}
	if cursor != nil {
		predicate = instanceSettingsAuditKeyset
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(instanceSettingsAuditListQueryFmt, predicate), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list instance settings audit entries: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close instance settings audit rows", "error", closeErr)
		}
	}()

	entries := make([]*models.InstanceSettingsAuditEntry, 0, limit)
	for rows.Next() {
		var entry models.InstanceSettingsAuditEntry
		if err := rows.Scan(scanInstanceSettingsAuditDest(&entry)...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan instance settings audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to iterate instance settings audit entries: %w", err)
	}

	if len(entries) <= limit {
		return entries, nil, nil
	}
	entries = entries[:limit]
	last := entries[limit-1]
	return entries, &models.InstanceSettingsAuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}

// clampInstanceSettingsAuditLimit maps a requested page size into
// [1, instanceSettingsAuditMaxLimit], with non-positive meaning the default.
func clampInstanceSettingsAuditLimit(limit int) int {
	if limit <= 0 {
		return instanceSettingsAuditDefaultLimit
	}
	return min(limit, instanceSettingsAuditMaxLimit)
}

// nullableJSON binds an absent snapshot as SQL NULL. A nil json.RawMessage
// already does, but an empty non-nil one would bind as an empty string, which
// jsonb rejects.
func nullableJSON(doc json.RawMessage) interface{} {
	if len(doc) == 0 {
		return nil
	}
	return []byte(doc)
}

// assertInstanceSettingsAuditRedacted rejects a snapshot that carries a
// credential: any object key naming one, at any depth, whose value is not
// exactly the "changed"/"unchanged" marker string. An absent snapshot passes.
func assertInstanceSettingsAuditRedacted(snapshots ...json.RawMessage) error {
	for _, doc := range snapshots {
		if len(doc) == 0 {
			continue
		}
		var value interface{}
		decoder := json.NewDecoder(bytes.NewReader(doc))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("invalid instance settings audit snapshot: %w", err)
		}
		if key, ok := findUnredactedSecret(value); ok {
			return fmt.Errorf("%w: key %q", repositories.ErrInstanceSettingsAuditUnredacted, key)
		}
	}
	return nil
}

// findUnredactedSecret walks a decoded JSON value and reports the first
// credential key whose value is not a redaction marker.
func findUnredactedSecret(value interface{}) (string, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		return findUnredactedSecretInObject(v)
	case []interface{}:
		for _, child := range v {
			if found, ok := findUnredactedSecret(child); ok {
				return found, true
			}
		}
	}
	return "", false
}

// findUnredactedSecretInObject checks one object's own keys, then recurses
// into its values.
func findUnredactedSecretInObject(object map[string]interface{}) (string, bool) {
	for key, child := range object {
		if isInstanceSettingsAuditSecretKey(key) && !isInstanceSettingsAuditMarker(child) {
			return key, true
		}
		if found, ok := findUnredactedSecret(child); ok {
			return found, true
		}
	}
	return "", false
}

func isInstanceSettingsAuditSecretKey(key string) bool {
	for _, secretKey := range instanceSettingsAuditSecretKeys {
		if strings.EqualFold(key, secretKey) {
			return true
		}
	}
	return false
}

func isInstanceSettingsAuditMarker(value interface{}) bool {
	s, ok := value.(string)
	return ok && (s == models.InstanceSettingsAuditSecretChanged || s == models.InstanceSettingsAuditSecretUnchanged)
}

// scanInstanceSettingsAuditDest returns the scan targets for
// instanceSettingsAuditColumns, in order.
//
// The snapshots are scanned through *[]byte rather than *json.RawMessage:
// database/sql maps a NULL onto a nil *[]byte but refuses to store one into a
// named byte-slice type, and both columns are legitimately NULL (before on a
// create, after on a delete).
func scanInstanceSettingsAuditDest(entry *models.InstanceSettingsAuditEntry) []interface{} {
	return []interface{}{
		&entry.ID, &entry.Setting, &entry.Action, &entry.ActorUserID,
		(*[]byte)(&entry.Before), (*[]byte)(&entry.After), &entry.CreatedAt,
	}
}
