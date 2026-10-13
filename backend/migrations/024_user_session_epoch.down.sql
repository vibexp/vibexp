-- Reverts 024_user_session_epoch: drops the per-user session revocation
-- counter. Cookies minted under a non-zero epoch keep decoding (the field is
-- ignored again), so sessions that had been revoked become valid until their
-- own expiry.
ALTER TABLE users DROP COLUMN IF EXISTS session_epoch;
