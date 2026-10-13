-- One-time account links (#1432, epic #1426).
--
-- A password account is never self-registered: a person gets one through an
-- invitation or an administrator and sets the password from a link, and a
-- forgotten password is recovered the same way. Each such link is one row here.
--
--   token_hash   SHA-256 of the token. The token itself is 32 random bytes and
--                exists only in the link; it is never stored, so a copy of this
--                table opens no account. A fast hash is right: there is nothing
--                to brute-force in 256 bits of randomness.
--   purpose      'setup' creates the account's first password; 'reset' replaces
--                a forgotten one.
--   delivery     'email' when the link was sent to the account's mailbox, 'link'
--                when an administrator copied it by hand. Completing an emailed
--                link proves the mailbox and may mark the email verified; a
--                copied link proves nothing about who holds it.
--   consumed_at  Set once, by the request that completes the link. A link is
--                single use.
--   created_by   The administrator who issued the link; NULL for a link the
--                system issued (forgot password) or once that user is deleted.
--
-- A user has at most one live link per purpose AND delivery (the partial unique
-- index): issuing a new emailed link removes the previous emailed one, and the
-- same for copied links, but an emailed and a copied link of one purpose may be
-- live together, so an administrator can both invite someone by email and hand
-- them a link. Completing any link removes every other live link of the user,
-- of either purpose: once a password is set, no older link may set another.
-- Dead rows (expired or consumed) are deleted by an hourly sweep in the server
-- process.
CREATE TABLE account_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The values mirror models.AccountTokenPurpose*; change both together.
    purpose      text NOT NULL
        CONSTRAINT account_tokens_purpose_check CHECK (purpose IN ('setup', 'reset')),
    -- The values mirror models.AccountTokenDelivery*; change both together.
    delivery     text NOT NULL
        CONSTRAINT account_tokens_delivery_check CHECK (delivery IN ('email', 'link')),
    token_hash   bytea NOT NULL,
    expires_at   timestamptz NOT NULL,
    consumed_at  timestamptz,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_tokens_token_hash_key UNIQUE (token_hash)
);

-- At most one live link per user, purpose and delivery.
CREATE UNIQUE INDEX account_tokens_one_live_per_delivery
    ON account_tokens (user_id, purpose, delivery)
    WHERE consumed_at IS NULL;

-- The sweep deletes by expiry.
CREATE INDEX account_tokens_expires_at ON account_tokens (expires_at);

COMMENT ON TABLE account_tokens IS
    'One-time account links for setting a first password (setup) or replacing a forgotten one (reset). Only the SHA-256 of each token is stored.';
