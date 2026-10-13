-- Reverts the account_tokens migration: drops every outstanding account link. Links that
-- were already sent stop working; passwords that were set through one are kept.
DROP TABLE IF EXISTS account_tokens;
