import { execFileSync } from 'node:child_process'

/**
 * Runs the break-glass CLI inside the e2e stack's app container, for the one
 * thing neither the UI nor the API can do: hand out a setup URL.
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
 * Re-arms authentication setup and returns the new one-time setup token.
 * Every earlier setup URL and setup session stops working.
 *
 * The published image's binary is `./main` (what the boot log calls
 * `vibexp`), run from the image's working directory with the server's own
 * configuration.
 */
export function rearmSetup(): string {
  const containerId = appContainerId()
  if (containerId === null) {
    throw new Error(
      `no running ${COMPOSE_PROJECT}/${APP_SERVICE} container: is the e2e stack up (make e2e)?`
    )
  }
  const output = run('docker', [
    'exec',
    containerId,
    './main',
    'admin',
    'auth',
    'setup',
    'rearm',
  ])
  const token = /\/setup\?token=([\w-]+)/.exec(output)?.[1]
  if (!token) {
    throw new Error(`no setup URL in the rearm output:\n${output}`)
  }
  return token
}
