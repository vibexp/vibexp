import { execFileSync } from 'node:child_process'

/**
 * Runs commands inside the e2e stack's app container, for what neither the UI
 * nor the API can do: hand out a setup URL (the break-glass CLI), and read the
 * stack's Mailpit, whose API is not published on the host.
 *
 * A setup token is only ever logged (at boot) or printed by
 * `vibexp admin auth setup rearm`; the server stores its hash alone. So the
 * setup journey (#1239) re-arms setup the way an operator would and reads the
 * URL off the command's output.
 *
 * Like `e2eDatabase.ts`, the container is found by its compose labels, so this
 * works from any cwd. The project name defaults to the one `make e2e` and
 * `ci-e2e.yml` use; `E2E_COMPOSE_PROJECT` overrides it for a stack started
 * under another name (a second stack beside one already holding :8080).
 */

const COMPOSE_PROJECT = process.env.E2E_COMPOSE_PROJECT ?? 'vibexp-e2e'
const APP_SERVICE = 'app'

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

/** The id of the running app container, or null when the stack is not up. */
function appContainerId(): string | null {
  try {
    const id = run('docker', [
      'ps',
      '--quiet',
      '--filter',
      `label=com.docker.compose.project=${COMPOSE_PROJECT}`,
      '--filter',
      `label=com.docker.compose.service=${APP_SERVICE}`,
    ])
    return id.split('\n')[0] || null
  } catch {
    return null
  }
}

/** True when the docker e2e stack's app container is running. */
export function e2eAppAvailable(): boolean {
  return appContainerId() !== null
}

/**
 * Runs a command inside the app container and returns its stdout. The stack's
 * Mailpit shares that container's network namespace, so its HTTP API is
 * reachable from here at 127.0.0.1:8025 without being published on the host.
 */
export function execInApp(args: readonly string[]): string {
  const containerId = appContainerId()
  if (containerId === null) {
    throw new Error(
      `no running ${COMPOSE_PROJECT}/${APP_SERVICE} container: is the e2e stack up (make e2e)?`
    )
  }
  return run('docker', ['exec', containerId, ...args])
}

/**
 * Re-arms authentication setup and returns the new one-time setup token.
 * Every earlier setup URL and setup session stops working.
 *
 * The published image's binary is `./main` (what the boot log calls
 * `vibexp`), run from the image's working directory with the server's own
 * configuration.
 */
export function rearmSetup(): string {
  const output = execInApp(['./main', 'admin', 'auth', 'setup', 'rearm'])
  const token = /\/setup\?token=([\w-]+)/.exec(output)?.[1]
  if (!token) {
    throw new Error(`no setup URL in the rearm output:\n${output}`)
  }
  return token
}
