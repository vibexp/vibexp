-- Reverts 028_user_credentials: drops every stored password hash and the
-- sign-in throttle state. Accounts are kept; a user who signed in only with a
-- password has no way in until they are given another sign-in method.
DROP TABLE IF EXISTS user_credentials;
