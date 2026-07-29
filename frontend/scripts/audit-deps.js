#!/usr/bin/env node
/**
 * Production dependency audit — `npm audit` with a narrow, documented exception.
 *
 * `npm audit` has no allowlist of its own: it is all-or-nothing per severity
 * level. This wrapper keeps the gate at `moderate` for **every** production
 * dependency except the packages named in ALLOWLIST below, so a real advisory in
 * anything else still fails the build.
 *
 * Run by the `frontend dependency audit` pre-commit hook (which fires only when
 * `package-lock.json` changes) and, since #547, by `make frontend-audit` in the
 * CI frontend job — so the gate applies to every contributor, not only those who
 * installed pre-commit. The allowlist below still has to stay honest about what
 * it is letting through.
 *
 * ---------------------------------------------------------------------------
 * Why `npm audit` still reports a `brace-expansion` advisory (and why that is
 * deliberate — do NOT "fix" it) — #547
 * ---------------------------------------------------------------------------
 * GHSA-mh99-v99m-4gvg declares `brace-expansion` vulnerable at `<= 5.0.7`, with
 * its only patched release at 5.0.8. Read as a semver range that also matches
 * the independent `1.x` and `2.x` lines vendored under `minimatch@3.1.5`
 * (eslint) and `minimatch@9.0.9` (jest) — neither of which has, or will get, a
 * release above 5.0.7.
 *
 * The 5.x line IS patched here, via the version-scoped override
 * `"brace-expansion@5.0.0 - 5.0.7": "^5.0.8"` in package.json. The residual
 * 1.x/2.x entries are left alone on purpose:
 *
 *   - they are dev-only (eslint/jest tooling), outside this gate's `--omit=dev`
 *     scope, and unreachable from the shipped SPA bundle;
 *   - bumping them to 1.1.16 / 2.1.2 fixes GHSA-3jxr-9vmj-r5cp but leaves
 *     GHSA-mh99 still matching. npm then loses its "fix available" path and
 *     reports the entire eslint + jest subtree instead — the advisory count goes
 *     from 3 to 26 with no route back;
 *   - a BLANKET `"brace-expansion": "^5.0.8"` override is worse still: v5 is a
 *     tshy dual build exporting `{ expand, ... }`, not a callable, while
 *     `minimatch@3.x` does `var expand = require('brace-expansion')` and calls
 *     it. Plain globs keep working, so it lands as a silent landmine that only
 *     fires on a brace pattern (`**\/*.{ts,tsx}`) in an eslint or jest config.
 *
 * The real resolution is upstream: eslint moving off `minimatch@3` and jest off
 * `minimatch@9`. Until then this is a knowingly-accepted dev-tree advisory.
 */

import { execFileSync } from 'child_process'

/**
 * Packages whose advisories are knowingly tolerated, each with the issue that
 * removes it. Keep this list as short as it can possibly be — an entry here is a
 * shipped vulnerability, not a resolved one.
 *
 * `react-router` / `react-router-dom` (5 advisories, 2 high): the vulnerable
 * range is `6.0.0 - 8.2.0` for `react-router` and `>=7.12.0-pre.0` for the
 * `react-router-dom` shim, patched only in `react-router@8.3.0`. We are on the
 * latest `react-router-dom` (7.18.x, bumped in #498 stage 1), which still
 * matches both entries, so **no version of the package we depend on satisfies
 * the gate** — v8 dropped the `react-router-dom` shim entirely. Fixing it is
 * a major migration across every routing import, tracked in #498; three of
 * the five advisories (RSC CSRF, RSCErrorHandler XSS, SSR `deserializeErrors`)
 * concern framework/SSR/RSC modes this client-only SPA does not use.
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
