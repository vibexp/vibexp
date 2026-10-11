package postgres

import "database/sql/driver"

// lockTestColumns names the four lock columns every read of prompts,
// blueprints and artifacts projects last (see resourceLockColumns), in scan
// order. sqlmock matches rows by position, so the names are labels only.
var lockTestColumns = []string{"locked_by", "locked_at", "lock_comment", "lock_u_name"}

// withLockColumns returns columns followed by lockTestColumns, without
// aliasing the caller's slice.
func withLockColumns(columns ...string) []string {
	return append(append(make([]string, 0, len(columns)+len(lockTestColumns)), columns...), lockTestColumns...)
}

// unlockedRow returns values followed by four NULLs: the lock columns of a
// row nobody holds a lock on. Spread it into AddRow.
func unlockedRow(values ...driver.Value) []driver.Value {
	return append(append(make([]driver.Value, 0, len(values)+len(lockTestColumns)), values...), nil, nil, nil, nil)
}

// WithLockColumns and UnlockedRow expose the helpers to the test files in
// package postgres_test, which cannot see unexported identifiers.
var (
	WithLockColumns = withLockColumns
	UnlockedRow     = unlockedRow
)
