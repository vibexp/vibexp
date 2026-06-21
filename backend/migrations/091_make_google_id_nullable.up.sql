-- WorkOS AuthKit cutover: make google_id nullable so WorkOS users can be
-- created without a google_id. Existing Google users are unaffected because
-- their rows already carry a non-NULL google_id value.
ALTER TABLE users ALTER COLUMN google_id DROP NOT NULL;
