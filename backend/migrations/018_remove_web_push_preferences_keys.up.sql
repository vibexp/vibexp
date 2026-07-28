-- Remove the stale `web_push` keys from user_preferences.preferences (issue #690).
--
-- Migration 017 (#688) dropped the Firebase device-token data layer but
-- deliberately left these JSONB keys in place: the `web_push` field was still
-- `required` in the published preferences API contract, so stored documents
-- had to keep matching it until the contract itself changed. This migration is
-- the data half of that contract change — the field is removed from
-- schemas/preferences.yaml and models.Preferences* in the same release, so the
-- keys have no reader left.
--
-- The keys live at two shapes inside the JSONB blob:
--   preferences.notifications.channels.web_push          (global channel switch)
--   preferences.notifications.types.<type>.web_push      (per-type switch)
-- #- '{a,b}' deletes a key at a nested path; the second statement rewrites each
-- per-type object without its web_push key via a lateral jsonb_each expansion.
-- Both are no-ops for rows that never carried the keys.
--
-- DEPLOY ORDERING: same single-image argument as 017 — the binary that stopped
-- reading/writing web_push and this migration ship together, so there is no
-- window in which an older binary expects the keys. Go's encoding/json already
-- ignored unknown keys, so even a rolled-back binary is safe against a
-- stripped document.
--
-- Locking: two UPDATEs take row locks on user_preferences only; the table is
-- small (one row per user) and no index or constraint references the JSONB
-- contents.

UPDATE public.user_preferences
SET preferences = preferences #- '{notifications,channels,web_push}'
WHERE preferences #> '{notifications,channels}' ? 'web_push';

UPDATE public.user_preferences up
SET preferences = jsonb_set(
    preferences,
    '{notifications,types}',
    COALESCE((
        SELECT jsonb_object_agg(t.key, t.value - 'web_push')
        FROM jsonb_each(preferences #> '{notifications,types}') AS t(key, value)
    ), '{}'::jsonb)
)
WHERE EXISTS (
    SELECT 1
    FROM jsonb_each(preferences #> '{notifications,types}') AS t(key, value)
    WHERE t.value ? 'web_push'
);
