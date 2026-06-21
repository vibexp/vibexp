-- Add legacy tracking fields to api_keys table
ALTER TABLE api_keys ADD COLUMN is_legacy BOOLEAN DEFAULT false;
ALTER TABLE api_keys ADD COLUMN migration_notes TEXT;
