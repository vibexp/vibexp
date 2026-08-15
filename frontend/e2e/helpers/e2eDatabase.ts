import { execFileSync } from 'node:child_process'

/**
 * Direct SQL access to the e2e stack's Postgres, for the one thing the API
 * cannot express: making a resource OLD.
 *
 * Freshness rules have a one-day floor on `threshold_days` (the server rejects
 * anything smaller), and the staleness test is
 * `GREATEST(updated_at, <last_accessed_* columns>) < now() - threshold`. A
 * resource a test just created is therefore never a stale candidate, no matter
 * how the rule is written — so a freshness journey has exactly two ways to make
 * an evaluation run mark something: wait a day, or backdate the row. This is the
 * second one, and it is what keeps the journey deterministic (issue #739).
 *
 * It talks to the container `docker-compose.e2e.yml` started, found by its
 * compose labels rather than by a container name or a path back to the compose
 * file — so it works from any cwd and survives compose's naming changing.
 * `make e2e` (local) and `ci-e2e.yml` both run that stack, so the two agree.
 *
 * When no such container is running — someone pointed Playwright at a plain
 * `npm run dev` — `e2eStackAvailable()` returns false and the caller is
 * expected to `test.skip` with a reason rather than assert nothing.
 */

const COMPOSE_PROJECT = 'vibexp-e2e'
const POSTGRES_SERVICE = 'postgres'
const POSTGRES_USER = 'vibexp'
const POSTGRES_DB = 'vibexp'

/** The four tables freshness can flag. A closed allowlist — never caller input. */
const BACKDATABLE_TABLES = [
  'artifacts',
  'prompts',
  'blueprints',
  'memories',
] as const

export type BackdatableTable = (typeof BACKDATABLE_TABLES)[number]

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * `execFileSync`'s thrown error says only "Command failed: docker exec …" and
 * keeps the process's stderr on a separate property, so a rejected statement
 * would surface as a bare exit code with the Postgres message nowhere in the
 * test output. Re-throwing with stderr attached is what makes a failure here
 * readable at all.
 */
function run(command: string, args: string[]): string {
  try {
    return execFileSync(command, args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    }).trim()
  } catch (error) {
    const stderr = (error as { stderr?: string }).stderr?.trim()
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(stderr ? `${message}\n${stderr}` : message)
  }
}

/**
 * The id of the running Postgres container, or null when the stack is not up.
 *
 * Any failure of `docker ps` (docker missing, daemon down) reads as "no stack"
 * rather than throwing: the caller's decision is the same either way.
 */
function postgresContainerId(): string | null {
  try {
    const id = run('docker', [
      'ps',
      '--quiet',
      '--filter',
      `label=com.docker.compose.project=${COMPOSE_PROJECT}`,
      '--filter',
      `label=com.docker.compose.service=${POSTGRES_SERVICE}`,
    ])
    // Several ids would mean several stacks; the first is as good a guess as
    // any and a wrong one fails loudly on the next query.
    return id.split('\n')[0] || null
  } catch {
    return null
  }
}

/** True when the docker e2e stack (`make e2e`) is running and reachable. */
export function e2eStackAvailable(): boolean {
  return postgresContainerId() !== null
}

/**
 * Runs one statement against the e2e database and returns its rows, unaligned
 * and untitled (`-A -t`), one per line.
 *
 * `ON_ERROR_STOP=1` is what makes a bad statement a non-zero exit instead of a
 * message on stdout that a caller would read as an empty result set. The SQL is
 * passed as an argv element, so there is no shell to quote for.
 *
 * `--quiet` is load-bearing rather than cosmetic: `--tuples-only` drops the
 * header and the row-count footer but NOT the command tag, so an
 * `UPDATE … RETURNING` otherwise emits a trailing `UPDATE 1` line that a caller
 * counting rows reads as a second row.
 */
export function runSql(sql: string): string[] {
  const containerId = postgresContainerId()
  if (containerId === null) {
    throw new Error(
      `no running ${COMPOSE_PROJECT}/${POSTGRES_SERVICE} container — is the e2e stack up (make e2e)?`
    )
  }

  const output = run('docker', [
    'exec',
    containerId,
    'psql',
    '--username',
    POSTGRES_USER,
    '--dbname',
    POSTGRES_DB,
    '--quiet',
    '--variable',
    'ON_ERROR_STOP=1',
    '--no-align',
    '--tuples-only',
    '--command',
    sql,
  ])

  return output === '' ? [] : output.split('\n')
}

/**
 * Ages one resource so a freshness rule considers it stale: pushes `updated_at`
 * back by `days` and drops every per-medium last-accessed stamp.
 *
 * Clearing the last-accessed columns matters as much as the backdate. They take
 * part in the same `GREATEST`, so a single recorded view — the detail read that
 * a UI-driven seed would perform — would hold the resource fresh however far
 * back `updated_at` went.
 *
 * `RETURNING id` is the assertion: an UPDATE that matched nothing is silent,
 * and the test that follows would then fail much later, looking like a broken
 * evaluator rather than a broken seed.
 */
export function backdateResource(
  table: BackdatableTable,
  resourceId: string,
  days: number
): void {
  if (!BACKDATABLE_TABLES.includes(table)) {
    throw new Error(`refusing to backdate unknown table ${table}`)
  }
  if (!UUID_PATTERN.test(resourceId)) {
    throw new Error(`refusing to backdate non-uuid resource id ${resourceId}`)
  }
  if (!Number.isInteger(days) || days <= 0) {
    throw new Error(`backdate days must be a positive integer, got ${days}`)
  }

  // `table` comes from the allowlist above and `resourceId`/`days` are pattern
  // checked, so this interpolation carries nothing a caller controls freely.
  const rows = runSql(
    `UPDATE ${table}
        SET updated_at = now() - make_interval(days => ${days}),
            last_accessed_web_at = NULL,
            last_accessed_cli_at = NULL,
            last_accessed_mcp_at = NULL,
            last_accessed_api_at = NULL
      WHERE id = '${resourceId}'
      RETURNING id`
  )

  if (rows.length !== 1) {
    throw new Error(
      `backdating ${table} ${resourceId} matched ${rows.length} rows, expected 1`
    )
  }
}
