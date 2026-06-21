-- Reverse the WorkOS cutover migration: restore NOT NULL on google_id.
--
-- DATA LOSS WARNING: this migration deletes any user rows that have NULL
-- google_id (i.e. users who only ever signed in via WorkOS). Without this
-- delete, the ALTER fails. With it, those users lose access until they
-- re-sign-up via the legacy Google flow.
--
-- Run only as part of an emergency rollback to the pre-WorkOS code path,
-- AFTER confirming no WorkOS-only users have produced unrecoverable data
-- (e.g. paid subscriptions, owned teams). For most rollback scenarios it
-- is safer to leave this migration applied and just redeploy the old
-- backend code, which can read NULL google_id rows even if it cannot
-- write them.
DELETE FROM users
 WHERE google_id IS NULL
   AND idp_provider = 'workos';

ALTER TABLE users ALTER COLUMN google_id SET NOT NULL;
