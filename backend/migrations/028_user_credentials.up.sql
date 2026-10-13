-- Password credentials (#1431, epic #1426).
--
-- One optional row per user: the argon2id hash of their password and the state
-- of the per-account sign-in throttle. A user with no row has no password and
-- signs in through a provider (or not at all, until an invitation or setup link
-- gives them one). Nothing is backfilled: no account has a password before this
-- migration.
--
--   password_hash    The encoded argon2id hash, parameters and salt included
--                    ("$argon2id$v=19$m=...,t=...,p=...$salt$key"). Never the
--                    password, and never returned by the API.
--   failed_attempts  Consecutive failed sign-ins since the last success.
--   locked_until     While in the future, sign-in is refused without checking
--                    the password. Locks start short and grow to a cap; there is
--                    no permanent lock, so an attacker cannot lock a user out
--                    for good.
--   disabled_at      Set when an administrator disables the password without
--                    removing it. A disabled credential never signs in.
--
-- The row goes with the user (ON DELETE CASCADE): a deleted account leaves no
-- hash behind.
CREATE TABLE user_credentials (
    user_id          uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash    text NOT NULL,
    failed_attempts  integer NOT NULL DEFAULT 0
        CONSTRAINT user_credentials_failed_attempts_check CHECK (failed_attempts >= 0),
    locked_until     timestamptz,
    disabled_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_credentials IS
    'Password credential of a user: the argon2id hash and the per-account sign-in throttle. At most one row per user; no row = no password.';
COMMENT ON COLUMN user_credentials.password_hash IS
    'Encoded argon2id hash (PHC string format, parameters and salt included). Never plaintext.';
