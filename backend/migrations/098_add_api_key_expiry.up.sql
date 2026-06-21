-- Add an optional expiry to API keys so a key can be time-bounded. NULL means the
-- key never expires (preserves existing behavior for all current keys). GetByKeyHash
-- treats a key as valid only while expires_at IS NULL OR expires_at > NOW().
ALTER TABLE api_keys ADD COLUMN expires_at TIMESTAMPTZ;
