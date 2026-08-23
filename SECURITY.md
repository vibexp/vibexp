# Security Policy

## Supported versions

VibeXP is self-hosted, so the version you run is the version you patch. Security
fixes are published for the **latest minor release only** — once `0.11.0` is out,
`0.10.x` no longer receives patches.

| Version | Supported |
|---|---|
| Latest minor (`X.Y.*`) | ✅ Patches published as `X.Y.Z` |
| Any earlier minor | ❌ Upgrade to the latest minor |

There is one supported line at a time by design: the [documentation
site](https://docs.vibexp.io) tracks the latest published release, so an older
line could not be documented accurately even if it were patched.

## Reporting a vulnerability

**Please do not open a public issue, discussion, or pull request for a security
problem.** VibeXP ships as a single container image that operators run
themselves, so a public report tells every operator running an affected version
how to attack it before a patched image exists.

Report privately instead:

1. Go to the [Security tab](https://github.com/vibexp/vibexp/security) of this
   repository.
2. Click **Report a vulnerability**.
3. Describe what you found, the version or commit you tested, and how to
   reproduce it. A proof of concept helps, but a clear description is enough —
   do not delay a report to build one.

This opens a private advisory visible only to you and the maintainers. It is the
only reporting channel; there is no security mailing address to publish.

If the button is not visible to you, open a normal issue saying **only** that you
have a security report and would like a private channel — no details — and a
maintainer will open the advisory.

## What to expect

- **Acknowledgement.** A maintainer replies in the advisory thread confirming the
  report was received and whether it is reproducible.
- **A fix developed in private.** Work happens in the advisory's private fork,
  not on a public branch, so nothing is readable while the fix is being built
  and reviewed.
- **A patched release first, disclosure second.** The advisory is published only
  once the patched image is pullable from `ghcr.io/vibexp/vibexp`, so operators
  can upgrade the moment they learn of the problem. To be precise about what
  "private" means here: the released image is built by public CI from a tag in
  this repository, so the fix does become visible when the release is cut —
  shortly before the advisory. That window is kept as short as the build takes,
  and it is the reason the advisory is written and ready to publish beforehand
  rather than afterwards.
- **A CVE where warranted**, requested through GitHub.
- **Credit**, unless you would rather stay anonymous — say so in the advisory.

This project is maintained by volunteers, so please allow reasonable time for a
reply rather than a fixed SLA. If a report goes unanswered, a nudge in the
advisory thread is welcome.

## Scope

In scope: the VibeXP backend and frontend in this repository, the published
combined image, and the deployment defaults shipped in `config.docker.yaml` and
`docker-compose.yml`.

Out of scope: vulnerabilities in third-party dependencies with no VibeXP-specific
exploitation path (report those upstream), findings that require an already
compromised host or database, and self-inflicted misconfiguration of a
self-hosted instance — though a report that VibeXP's *defaults* are unsafe **is**
in scope, and welcome.

## Maintainers

The maintainer-side procedure for handling an embargoed fix — advisory, private
fork, patch release, publication, and forward-port — is documented in
`.claude/skills/release/SKILL.md` under "Security releases (embargoed)".
