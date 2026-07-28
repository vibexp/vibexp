-- Reverse of 018_remove_web_push_preferences_keys.up.sql.
--
-- ⚠️ NO-OP BY DESIGN. The up migration deletes `web_push` keys from a JSONB
-- document without recording their prior values, so they are NOT recoverable
-- here — the same structure-only caveat as migrations 015–017, one step
-- further: there is not even structure to restore. Restore from a backup if
-- the values mattered (they did not: nothing has read web_push since #688).
--
-- Rolling back does NOT bring web push back: the delivery channel, the device
-- token endpoints, the Firebase dependencies and the API contract field were
-- all removed in the application. A down-then-up cycle therefore stays at the
-- post-018 state, which is correct — there is no pre-018 shape worth
-- reconstructing.

SELECT 1;
