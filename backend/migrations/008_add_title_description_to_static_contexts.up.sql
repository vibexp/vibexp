ALTER TABLE static_contexts
ADD COLUMN title VARCHAR(255) NOT NULL DEFAULT '',
ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- Update existing records to have meaningful titles based on their slugs
UPDATE static_contexts
SET title = INITCAP(REPLACE(slug, '-', ' '))
WHERE title = '';

-- Update existing records to have descriptions as truncated content
UPDATE static_contexts
SET description = LEFT(content, 200)
WHERE description = '';
