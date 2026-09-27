-- Drops the instance search ranking and AI summary defaults (#1197). With the
-- rows gone the instance falls back to its built-in defaults.
DROP TABLE IF EXISTS instance_ai_summary_settings;
DROP TABLE IF EXISTS instance_search_settings;
