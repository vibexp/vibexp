-- Remove config column as agent configuration comes from agent card
ALTER TABLE agents DROP COLUMN config;
