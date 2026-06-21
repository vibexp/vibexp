-- Drop tables in reverse order to respect foreign key constraints
DROP TABLE IF EXISTS claude_plugin_ratings;
DROP TABLE IF EXISTS claude_plugin_installations;
DROP TABLE IF EXISTS claude_plugin_marketplace_assignments;
DROP TABLE IF EXISTS claude_plugin_marketplaces;
