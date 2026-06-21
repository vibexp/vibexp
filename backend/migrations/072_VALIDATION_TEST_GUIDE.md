# Migration 071 Validation Test Guide

## Overview

Migration `071_add_team_id_to_unique_constraints.up.sql` includes validation logic to ensure data integrity before altering unique constraints. This guide explains how the validation works and how to test it.

## Validation Logic

Before modifying any unique constraints, the migration validates that no NULL `team_id` values exist in the affected tables:

- `prompts`
- `artifacts`
- `spec_libraries`
- `memories`
- `agents`

### How It Works

```sql
DO $$
BEGIN
    -- Check each table for NULL team_id values
    IF EXISTS (SELECT 1 FROM prompts WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in prompts.team_id';
    END IF;
    -- ... checks for other tables ...
END $$;
```

### Expected Behavior

1. **If NULL values exist**: Migration fails immediately with clear error message
   - Example: `Cannot add team_id to unique constraint: NULL values exist in prompts.team_id`
   - No constraint changes are made
   - Database remains in consistent state

2. **If no NULL values exist**: Migration proceeds successfully
   - Old constraints are dropped
   - New constraints with team_id are added
   - All resources are properly scoped by team

## Testing the Validation

### Prerequisites

- Running PostgreSQL database
- Connection to database with appropriate permissions
- Test data with and without NULL team_id values

### Test Scenarios

#### Scenario 1: Test Validation Failure (NULL values exist)

```sql
-- Setup: Create test data with NULL team_id
INSERT INTO prompts (id, slug, user_id, team_id, name, created_at, updated_at)
VALUES ('test-prompt-1', 'test-slug', 'user-123', NULL, 'Test Prompt', NOW(), NOW());

-- Run migration
-- Expected: Migration fails with error
-- Error message: "Cannot add team_id to unique constraint: NULL values exist in prompts.team_id"

-- Cleanup
DELETE FROM prompts WHERE id = 'test-prompt-1';
```

#### Scenario 2: Test Validation Success (No NULL values)

```sql
-- Setup: Ensure all team_id values are populated
-- Run data migration 070 first to populate team_id values

-- Run migration 071
-- Expected: Migration succeeds
-- Constraints are updated successfully
```

#### Scenario 3: Test Individual Table Validation

You can test each table individually:

```sql
-- Test prompts table
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM prompts WHERE team_id IS NULL LIMIT 1) THEN
        RAISE NOTICE 'Found NULL team_id in prompts';
    ELSE
        RAISE NOTICE 'No NULL team_id in prompts - validation will pass';
    END IF;
END $$;

-- Test artifacts table
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM artifacts WHERE team_id IS NULL LIMIT 1) THEN
        RAISE NOTICE 'Found NULL team_id in artifacts';
    ELSE
        RAISE NOTICE 'No NULL team_id in artifacts - validation will pass';
    END IF;
END $$;

-- Repeat for spec_libraries, memories, agents
```

### Using the Test Script

A comprehensive test script is provided at `migrations/071_test_validation.sql`:

```bash
# Run the test script (requires local database)
psql -h localhost -U your_user -d your_database -f migrations/071_test_validation.sql
```

The test script validates:
1. NULL detection logic works correctly
2. No false positives (passes when no NULLs present)
3. Error messages are clear and actionable
4. All table checks execute properly

## Integration Testing

### In CI/CD Pipeline

```bash
# 1. Setup test database
docker-compose up -d postgres

# 2. Run migrations up to 070 (data population)
migrate -path ./migrations -database $DATABASE_URL up 70

# 3. Verify no NULL values
psql $DATABASE_URL -c "SELECT 'prompts' as table_name, COUNT(*) as null_count FROM prompts WHERE team_id IS NULL
UNION ALL
SELECT 'artifacts', COUNT(*) FROM artifacts WHERE team_id IS NULL
UNION ALL
SELECT 'spec_libraries', COUNT(*) FROM spec_libraries WHERE team_id IS NULL
UNION ALL
SELECT 'memories', COUNT(*) FROM memories WHERE team_id IS NULL
UNION ALL
SELECT 'agents', COUNT(*) FROM agents WHERE team_id IS NULL;"

# Expected output: all counts should be 0

# 4. Run migration 071
migrate -path ./migrations -database $DATABASE_URL up 1

# 5. Verify constraints
psql $DATABASE_URL -c "\d prompts" | grep UNIQUE
# Should show: prompts_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id)
```

## Manual Testing in Production

**IMPORTANT**: Before running in production:

1. **Verify data migration 070 completed successfully**
   ```sql
   SELECT COUNT(*) FROM prompts WHERE team_id IS NULL;
   SELECT COUNT(*) FROM artifacts WHERE team_id IS NULL;
   SELECT COUNT(*) FROM spec_libraries WHERE team_id IS NULL;
   SELECT COUNT(*) FROM memories WHERE team_id IS NULL;
   SELECT COUNT(*) FROM agents WHERE team_id IS NULL;
   ```
   All counts should be 0.

2. **Run migration in transaction** (if your migration tool supports it)
   ```sql
   BEGIN;
   \i migrations/071_add_team_id_to_unique_constraints.up.sql
   -- If successful:
   COMMIT;
   -- If error:
   ROLLBACK;
   ```

3. **Verify constraints after migration**
   ```sql
   -- Check prompts constraints
   SELECT conname, contype, pg_get_constraintdef(oid)
   FROM pg_constraint
   WHERE conrelid = 'prompts'::regclass AND contype = 'u';

   -- Repeat for other tables
   ```

## Troubleshooting

### Error: NULL values exist in {table}.team_id

**Cause**: Data migration 070 did not complete or new records were created without team_id

**Resolution**:
1. Identify records with NULL team_id:
   ```sql
   SELECT id, slug, user_id, team_id FROM prompts WHERE team_id IS NULL;
   ```

2. Populate team_id values:
   ```sql
   -- Option A: Use user's selected team
   UPDATE prompts p
   SET team_id = u.selected_team_id
   FROM users u
   WHERE p.user_id = u.id AND p.team_id IS NULL;

   -- Option B: If selected_team_id is also NULL, use first team
   UPDATE prompts p
   SET team_id = (
       SELECT tm.team_id
       FROM team_members tm
       WHERE tm.user_id = p.user_id
       ORDER BY tm.joined_at ASC
       LIMIT 1
   )
   WHERE p.team_id IS NULL;
   ```

3. Re-run migration 071

### Error: Constraint already exists

**Cause**: Migration was partially applied or run multiple times

**Resolution**:
1. Check existing constraints:
   ```sql
   \d prompts
   ```

2. Drop and re-create if needed:
   ```sql
   ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_slug_user_id_team_id_key;
   -- Then re-run migration
   ```

## Validation Success Criteria

Migration 071 validation is successful when:

- [ ] All NULL team_id checks pass (no NULLs found)
- [ ] Old constraints are dropped successfully
- [ ] New constraints with team_id are created
- [ ] No constraint violation errors occur
- [ ] All tables maintain data integrity

## Related Migrations

- **Migration 070**: Populates team_id values (prerequisite)
- **Migration 071**: This migration (constraint updates)
- **Migration 072**: Future migrations depending on these constraints

## Security Considerations

The validation prevents:
1. **Inconsistent constraint behavior**: Without validation, NULL team_id values would bypass the unique constraint
2. **Data integrity issues**: Ensures all resources are properly scoped to teams
3. **Cross-team data leaks**: Prevents resources from being accessible across team boundaries

## Performance Impact

- **Validation checks**: Fast (uses LIMIT 1, stops at first NULL)
- **Constraint operations**: Requires table locks, but brief for most databases
- **Recommended**: Run during low-traffic periods

## Rollback

If migration needs to be rolled back:

```bash
migrate -path ./migrations -database $DATABASE_URL down 1
```

The down migration (`071_add_team_id_to_unique_constraints.down.sql`) will:
1. Drop the new constraints with team_id
2. Re-create the old constraints without team_id
