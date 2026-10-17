#!/usr/bin/env bash
#
# Summarize a Playwright e2e run (#1563). Called by the `report` job of
# .github/workflows/ci-e2e.yml on the merged JSON report; runs the same locally
# on a downloaded `playwright-report` artifact:
#
#   gh run download <run-id> -n playwright-report -D /tmp/e2e-run
#   bash .github/scripts/e2e-run-stats.sh /tmp/e2e-run/test-results/results.json manifest.txt
#
# Prints a markdown summary (totals, per-project time, slowest files) to stdout,
# and writes the test-ID manifest to the second argument: one sorted line per
# test, `project › file › describe › title`. Diffing two runs' manifests is how
# a CI change proves it still runs exactly the same tests:
#
#   diff before.txt after.txt
#
# The report job writes the manifest into the artifact as
# test-results/test-manifest.txt, so an earlier run's needs no re-running.
#
# Times are test durations as Playwright reports them (hooks and fixtures
# included), so per-project numbers are worker-seconds, not wall clock.
set -euo pipefail

results="${1:?usage: e2e-run-stats.sh <results.json> [manifest-out]}"
manifest="${2:-}"

# Every test of every spec, flattened with its project, file and title path.
tests_jq='
  def walk_suites($path):
    ((.specs // [])[] as $spec
      | $spec.tests[]
      | {project: .projectName, file: $spec.file,
         id: ([.projectName, $spec.file] + $path + [$spec.title] | join(" › ")),
         status: .status, retries: ((.results | length) - 1),
         seconds: (([.results[].duration] | add // 0) / 1000)}),
    ((.suites // [])[] | .title as $t | walk_suites($path + [$t]));
  [.suites[] | walk_suites([])]
'

if [ -n "$manifest" ]; then
  jq -r "$tests_jq | .[].id" "$results" | sort > "$manifest"
fi

jq -r "$tests_jq"' as $tests
  | .stats as $s
  | "### E2E run stats",
    "",
    "| Passed | Failed | Flaky | Skipped | Tests | Test phase (wall) |",
    "|---:|---:|---:|---:|---:|---:|",
    "| \($s.expected) | \($s.unexpected) | \($s.flaky) | \($s.skipped) | \($tests | length) | \($s.duration / 1000 | floor)s |",
    "",
    "Retried tests: \([$tests[] | select(.retries > 0)] | length)",
    "",
    "| Project | Tests | Worker-seconds |",
    "|---|---:|---:|",
    ($tests | group_by(.project)[]
      | "| \(.[0].project) | \(length) | \([.[].seconds] | add | floor) |"),
    "",
    "<details><summary>Slowest files</summary>",
    "",
    "| File | Worker-seconds |",
    "|---|---:|",
    ($tests | group_by(.file) | map({file: .[0].file, s: ([.[].seconds] | add)})
      | sort_by(-.s)[:10][] | "| \(.file) | \(.s | floor) |"),
    "",
    "</details>"
' "$results"
