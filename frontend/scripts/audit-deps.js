#!/usr/bin/env node
/**
 * Production dependency audit — `npm audit` with a narrow, documented exception.
 *
 * `npm audit` has no allowlist of its own: it is all-or-nothing per severity
 * level. This wrapper keeps the gate at `moderate` for **every** production
 * dependency except the packages named in ALLOWLIST below, so a real advisory in
 * anything else still fails the build.
 *
 * Run by the `frontend dependency audit` pre-commit hook (on `package-lock.json`
 * changes) and by CI, via `npm run audit:deps`.
 */

import { execFileSync } from 'child_process'

/**
 * Packages whose advisories are knowingly tolerated, each with the issue that
 * removes it. Keep this list as short as it can possibly be — an entry here is a
 * shipped vulnerability, not a resolved one.
 *
 * `react-router` / `react-router-dom` (5 advisories, 2 high): the vulnerable
 * range is `6.0.0 - 8.2.0`, patched only in `react-router@8.3.0`. The latest
 * `react-router-dom` (7.18.1) pulls `react-router@7.18.1`, still inside that
 * range, so **no version of the package we depend on satisfies the gate** —
 * v8 dropped the `react-router-dom` shim entirely. Fixing it is a major
 * migration across every routing import, tracked in #498; three of the five
 * advisories (RSC CSRF, RSCErrorHandler XSS, SSR `deserializeErrors`) concern
 * framework/SSR/RSC modes this client-only SPA does not use.
 */
const ALLOWLIST = new Set(['react-router', 'react-router-dom'])

const FAIL_AT = new Set(['moderate', 'high', 'critical'])

/** Lowest severity in FAIL_AT, so the messages below cannot contradict it. */
const THRESHOLD = ['low', 'moderate', 'high', 'critical'].find(level =>
  FAIL_AT.has(level)
)

function audit() {
  try {
    // Exits non-zero whenever anything is found, so the output is the payload
    // and the status tells us nothing on its own.
    return execFileSync('npm', ['audit', '--json', '--omit=dev'], {
      encoding: 'utf8',
      maxBuffer: 32 * 1024 * 1024,
    })
  } catch (error) {
    if (typeof error.stdout === 'string' && error.stdout.length > 0) {
      return error.stdout
    }
    throw error
  }
}

const report = JSON.parse(audit())
const vulnerabilities = Object.values(report.vulnerabilities ?? {})

const blocking = vulnerabilities.filter(
  vulnerability =>
    FAIL_AT.has(vulnerability.severity) && !ALLOWLIST.has(vulnerability.name)
)

const tolerated = vulnerabilities.filter(
  vulnerability =>
    FAIL_AT.has(vulnerability.severity) && ALLOWLIST.has(vulnerability.name)
)

for (const vulnerability of tolerated) {
  console.log(
    `allowlisted: ${vulnerability.name} (${vulnerability.severity}) — see #498`
  )
}

if (blocking.length === 0) {
  console.log(
    `npm audit: no blocking advisories at ${THRESHOLD} or above (${String(tolerated.length)} allowlisted).`
  )
  process.exit(0)
}

console.error(`\nBlocking advisories (${THRESHOLD} or above):\n`)
for (const vulnerability of blocking) {
  console.error(`  ${vulnerability.name} — ${vulnerability.severity}`)
  for (const via of vulnerability.via) {
    if (typeof via === 'object') {
      console.error(`    - ${via.title} (${via.url})`)
    }
  }
  const fix = vulnerability.fixAvailable
  if (fix && typeof fix === 'object') {
    console.error(
      `    fix: ${fix.name}@${fix.version}${fix.isSemVerMajor ? ' (semver-major)' : ''}`
    )
  }
}
console.error(
  '\nFix the advisory, or add the package to ALLOWLIST in scripts/audit-deps.js with a tracking issue.\n'
)
process.exit(1)
