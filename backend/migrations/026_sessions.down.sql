-- Reverts 026_sessions.up.sql. Every session minted after the upgrade carries
-- only a session id, so after this every such session signs in again.
DROP TABLE IF EXISTS sessions;
