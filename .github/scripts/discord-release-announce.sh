#!/usr/bin/env bash
#
# Announce a published release in the Shaharia Lab Discord release channel
# (#1298). Called by the `announce-discord` job of .github/workflows/release.yml
# once the combined image is pushed; kept as a script so the payload can be
# exercised locally:
#
#   DRY_RUN=1 TAG=v0.9.0 RELEASE_URL=https://github.com/vibexp/vibexp/releases/tag/v0.9.0 \
#     RELEASE_BODY="$(gh release view v0.9.0 --json body -q .body)" \
#     bash .github/scripts/discord-release-announce.sh | jq .
#
# Everything arrives through the environment — nothing is interpolated into
# this file, because the release notes are attacker-influenced text:
#   TAG               release tag, e.g. v0.9.0 (required)
#   RELEASE_URL       html_url of the GitHub release (required)
#   RELEASE_NAME      release title; shown only when it says more than the tag
#   RELEASE_BODY      release notes (markdown); may be empty
#   IMAGE_NAME        image repository, default ghcr.io/vibexp/vibexp
#   DISCORD_WEBHOOK   webhook URL. Unset/empty -> skip cleanly (forks).
#   DRY_RUN           "1" prints the payload JSON and exits; needs neither the
#                     webhook nor the variables below.
#   GH_TOKEN, GITHUB_REPOSITORY, GITHUB_JOB, RUN_ID, RUN_ATTEMPT
#                     for the duplicate guard (see below).
#
# The webhook URL is a credential: never add `set -x`, and never let curl echo
# it. curl runs silent with stdout AND stderr discarded, so a failure is
# reported by HTTP status / curl exit code only.
set -euo pipefail

# Discord caps an embed description at 4096 characters. The notes are cut well
# short of that so the "read the full notes" pointer always fits.
readonly NOTES_LIMIT=3500

tag="${TAG:?TAG is required}"
release_url="${RELEASE_URL:?RELEASE_URL is required}"
release_name="${RELEASE_NAME:-}"
release_body="${RELEASE_BODY:-}"
image="${IMAGE_NAME:-ghcr.io/vibexp/vibexp}:${tag#v}"
summary="${GITHUB_STEP_SUMMARY:-/dev/null}"

if [ "${DRY_RUN:-}" != "1" ]; then
    if [ -z "${DISCORD_WEBHOOK:-}" ]; then
        echo "::notice::DISCORD_WEBHOOK is not configured; skipping the Discord release announcement."
        echo "- Discord announcement: **skipped** — \`DISCORD_WEBHOOK\` secret not configured." >> "$summary"
        exit 0
    fi

    # Duplicate guard. A re-run of this workflow run must not announce the same
    # release twice, but `RUN_ATTEMPT == 1` is the wrong test: a release whose
    # first attempt failed at the image build never reached this job, and its
    # re-run is the FIRST announcement. So ask what actually happened — did any
    # earlier attempt finish this job successfully?
    #
    # The Actions API names a job by its display name; the workflow gives this
    # job no `name:`, so that display name is the job id, i.e. $GITHUB_JOB.
    #
    # Fetch and test SEPARATELY (same reason as dispatch-cli-e2e): a failed
    # lookup must be an error, not something that reads as "not announced yet"
    # or "already announced".
    attempt=1
    while [ "$attempt" -lt "${RUN_ATTEMPT:?RUN_ATTEMPT is required}" ]; do
        if ! conclusions="$(gh api \
              "repos/${GITHUB_REPOSITORY:?}/actions/runs/${RUN_ID:?}/attempts/$attempt/jobs?per_page=100" \
              --jq ".jobs[] | select(.name == \"${GITHUB_JOB:?}\") | .conclusion")"; then
            echo "::error::Could not read the jobs of attempt $attempt of run $RUN_ID; not announcing blind."
            echo "- Discord announcement: **failed** — could not check attempt $attempt for an earlier announcement." >> "$summary"
            exit 1
        fi
        if grep -qx success <<<"$conclusions"; then
            echo "::notice::$tag was already announced in attempt $attempt of this run; skipping."
            echo "- Discord announcement: **skipped** — already announced in attempt $attempt." >> "$summary"
            exit 0
        fi
        attempt=$((attempt + 1))
    done
fi

# jq does all the escaping (quotes, backticks, newlines in the notes) and slices
# by code point, so a multi-byte character is never cut in half.
# `allowed_mentions.parse: []` makes every @everyone / @here / role or user
# mention in the notes render as plain text instead of pinging the channel.
payload="$(jq -n \
    --arg tag "$tag" \
    --arg url "$release_url" \
    --arg name "$release_name" \
    --arg body "$release_body" \
    --arg image "$image" \
    --argjson limit "$NOTES_LIMIT" '
    ($body | gsub("\r"; "") | sub("^\\s+"; "") | sub("\\s+$"; "")) as $notes
    | {
        allowed_mentions: {parse: []},
        embeds: [
          {
            title: ("VibeXP \($tag) released" | .[0:256]),
            url: $url,
            description: (
              if $notes == "" then
                "VibeXP \($tag) is out. [Read the release notes](\($url))."
              elif ($notes | length) > $limit then
                ($notes[0:$limit] | sub("\\s+$"; ""))
                  + "\n\n… [read the full release notes](\($url))"
              else
                $notes
              end
            ),
            fields: (
              (if $name != "" and $name != $tag
               then [{name: "Release", value: ($name | .[0:1024])}]
               else [] end)
              + [{name: "Docker image", value: "`\($image)`"}]
            )
          }
        ]
      }')"

if [ "${DRY_RUN:-}" = "1" ]; then
    printf '%s\n' "$payload"
    exit 0
fi

# `wait=true` makes Discord answer once the message is stored, so a 2xx means
# "posted" rather than "accepted".
case "$DISCORD_WEBHOOK" in
    *\?*) endpoint="${DISCORD_WEBHOOK}&wait=true" ;;
    *)    endpoint="${DISCORD_WEBHOOK}?wait=true" ;;
esac

curl_exit=0
status="$(curl -sS --max-time 20 --retry 3 --retry-all-errors \
    -o /dev/null -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    --data-binary @- "$endpoint" <<<"$payload" 2>/dev/null)" || curl_exit=$?

if [ "$curl_exit" -ne 0 ]; then
    echo "::error::Discord announcement for $tag failed: curl exit code $curl_exit (no HTTP response)."
    echo "- Discord announcement: **failed** — curl exit code $curl_exit, no HTTP response." >> "$summary"
    exit 1
fi

case "$status" in
    2??)
        echo "Announced $tag on Discord (HTTP $status)."
        echo "- Discord announcement: **announced** $tag (HTTP $status)." >> "$summary"
        ;;
    *)
        echo "::error::Discord announcement for $tag was rejected: HTTP $status."
        echo "- Discord announcement: **failed** — Discord answered HTTP $status." >> "$summary"
        exit 1
        ;;
esac
