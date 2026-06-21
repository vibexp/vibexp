-- Remove default_team_id references
UPDATE users SET default_team_id = NULL;

-- Delete auto-created teams (those with slug 'my-team')
DELETE FROM teams WHERE slug = 'my-team';
