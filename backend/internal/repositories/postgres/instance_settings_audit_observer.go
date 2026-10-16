package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Post-commit notification of instance settings changes (#1515).
//
// Every instance settings write appends its instance_settings_audit entry
// through appendInstanceSettingsAudit, most of them inside the transaction of
// the change (runAuditedSingletonTx and the auth settings writes built on it),
// from closures the repositories pass in. Rather than thread an observer
// through each of them, an entry appended on a transaction is held against
// that transaction and handed to the observer only once runAuditedSingletonTx
// has committed it; a rollback discards it. An entry appended outside a
// transaction (InstanceSettingsAuditRepository.Append) is durable at once and
// is handed over straight away.
//
// Invariant: an instance settings audit entry is appended on a transaction
// only inside runAuditedSingletonTx, which opens the transaction's slot. An
// append on any other transaction returns errAppendOutsideAuditedTx, so a
// future caller that breaks the invariant fails loudly instead of losing the
// event.
//
// Both maps are sync.Maps, and each pending slot has its own mutex, so
// concurrent audited transactions on one database never share state: each
// transaction's entries are keyed by that *sql.Tx.
//
// The observer is registered per database handle (the registry is keyed by the
// *database.DB) and the returned func removes it, so tests on their own handle
// never see one another's and nothing outlives a test that cleans up.

var (
	instanceSettingsObservers sync.Map // *database.DB -> *observerRegistration
	pendingSettingsAudits     sync.Map // *sql.Tx -> *pendingAudits
)

// observerRegistration wraps a registered observer so it can be removed by
// identity: an observer value need not be comparable (a slice of observers is
// not).
type observerRegistration struct {
	observer repositories.InstanceSettingsAuditObserver
}

// pendingAudits are the entries one transaction appended, awaiting its commit.
type pendingAudits struct {
	mu      sync.Mutex
	entries []models.InstanceSettingsAuditEntry
}

// RegisterInstanceSettingsAuditObserver makes db report each durable instance
// settings change to observer, replacing any earlier one, and returns a
// function that removes it again.
func RegisterInstanceSettingsAuditObserver(
	db *database.DB, observer repositories.InstanceSettingsAuditObserver,
) (unregister func()) {
	registration := &observerRegistration{observer: observer}
	instanceSettingsObservers.Store(db, registration)
	return func() { instanceSettingsObservers.CompareAndDelete(db, registration) }
}

// notifyInstanceSettingsAudited hands one durable entry to db's observer, if
// any.
func notifyInstanceSettingsAudited(ctx context.Context, db *database.DB, entry models.InstanceSettingsAuditEntry) {
	if registration, ok := instanceSettingsObservers.Load(db); ok {
		registration.(*observerRegistration).observer.OnInstanceSettingsAudited(ctx, entry)
	}
}

// errAppendOutsideAuditedTx is returned for an instance settings audit entry
// appended on a transaction runAuditedSingletonTx did not open: nothing would
// ever report it, so the write fails rather than drop the event silently.
var errAppendOutsideAuditedTx = errors.New(
	"instance settings audit entry appended on a transaction not run by runAuditedSingletonTx")

// openPendingAudits opens tx's slot for held entries. runAuditedSingletonTx
// calls it right after BeginTx and closes it with takeHeld.
func openPendingAudits(tx *sql.Tx) {
	pendingSettingsAudits.Store(tx, &pendingAudits{})
}

// holdUntilCommit records an entry appended on tx, to be reported when tx
// commits. A transaction without an open slot is a programming error: see
// errAppendOutsideAuditedTx.
func holdUntilCommit(tx *sql.Tx, entry models.InstanceSettingsAuditEntry) error {
	value, ok := pendingSettingsAudits.Load(tx)
	if !ok {
		return errAppendOutsideAuditedTx
	}
	pending := value.(*pendingAudits)
	pending.mu.Lock()
	defer pending.mu.Unlock()
	pending.entries = append(pending.entries, entry)
	return nil
}

// takeHeld removes and returns the entries held for tx. Called after a commit
// (to report them) and when the transaction ends either way (to drop them).
func takeHeld(tx *sql.Tx) []models.InstanceSettingsAuditEntry {
	value, ok := pendingSettingsAudits.LoadAndDelete(tx)
	if !ok {
		return nil
	}
	pending := value.(*pendingAudits)
	pending.mu.Lock()
	defer pending.mu.Unlock()
	return pending.entries
}
