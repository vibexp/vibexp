package postgres

import "database/sql/driver"

// lockTestColumns names the four lock columns every read of prompts,
// blueprints and artifacts projects last (see resourceLockColumns), in scan
// order. sqlmock matches rows by position, so the names are labels only.
var lockTestColumns = []string{"locked_by", "locked_at", "lock_comment", "lock_u_name"}

// lastWriteTestColumns names the two last-write columns every read of the
// four resource tables projects after everything else (see
// resourceLastWriteProjection), in scan order.
var lastWriteTestColumns = []string{"lw_source", "lw_assistant_name"}

// withLockColumns returns columns followed by lockTestColumns and then
// lastWriteTestColumns, the tail every prompt, blueprint and artifact read
// ends with, without aliasing the caller's slice.
func withLockColumns(columns ...string) []string {
	out := make([]string, 0, len(columns)+len(lockTestColumns)+len(lastWriteTestColumns))
	out = append(out, columns...)
	out = append(out, lockTestColumns...)
	return append(out, lastWriteTestColumns...)
}

// unlockedRow returns values followed by six NULLs: the lock columns of a row
// nobody holds a lock on, then the last-write columns of a row with no
// recorded write. Spread it into AddRow.
func unlockedRow(values ...driver.Value) []driver.Value {
	out := make([]driver.Value, 0, len(values)+len(lockTestColumns)+len(lastWriteTestColumns))
	out = append(out, values...)
	return append(out, nil, nil, nil, nil, nil, nil)
}

// WithLockColumns and UnlockedRow expose the helpers to the test files in
// package postgres_test, which cannot see unexported identifiers.
var (
	WithLockColumns = withLockColumns
	UnlockedRow     = unlockedRow
)
