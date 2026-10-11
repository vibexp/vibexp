# Journey Tests (Phase 2)

Journey tests validate complete end-to-end user flows that span multiple features and pages. These tests ensure critical user experiences work correctly from start to finish.

## Test Structure

Journey tests are organized by user scenario:

```
e2e/journeys/
├── prompt-gallery.journey.spec.ts           # Gallery → custom prompt
├── upgrade-subscription.journey.spec.ts     # Full Stripe lifecycle (gated)
├── team-collaboration.journey.spec.ts       # Team collaboration workflow
├── resource-freshness.journey.spec.ts       # Freshness loop (gated on the docker stack)
├── cross-team-settings-copy.journey.spec.ts # Copying settings between two teams
├── ai-summary.journey.spec.ts               # AI Summary on search (gated on the docker stack)
├── admin-panel-v3.journey.spec.ts           # Admin filters, presets, CSV export, detail pages
├── admin-email-settings.journey.spec.ts     # Admin → Settings → Email against the stack's Mailpit
├── instance-settings.journey.spec.ts        # Admin → Settings → Search defaults reach a team; reset
├── admin-auth-settings.journey.spec.ts      # Admin → Settings → Authentication + first-run /setup
├── custom-statuses.journey.spec.ts          # Custom status: create, apply, search, filter, delete
├── resource-locking.journey.spec.ts         # Lock, enforcement and unlock across three users
├── site-info.journey.spec.ts                # Company branding: sign-in, app shell, VibeXP footer mark
└── README.md                                # This file
```

## Site info branding (`site-info.journey.spec.ts`)

The ship gate for epic #1352. As the instance admin it stores company branding
(name, contact address, links, logo), then checks it where people meet it: the
sign-in page in a signed-out browser (logo, "Sign in to …", the legal line,
copyright, support link, "Powered by VibeXP"), and the app shell in the admin's
own session (sidebar brand, the user menu's Support / Terms / Privacy links,
the tab title). It then clears the terms and privacy URLs and sees the legal
line leave the sign-in page.

Three of its assertions are the reason this is an e2e at all:

- **The whole chain.** The values travel database → `/config.js`
  (`window.__VIBEXP_SITE_INFO__`) → `SiteInfoProvider` → page. `useSiteInfo`
  answers "nothing configured" outside a provider on purpose, so the unit suite
  stays green if the provider is ever unmounted from `App.tsx`; this journey is
  what turns red.
- **The logo really loads.** A logo that fails to load falls back to the VibeXP
  mark, so the journey checks `naturalWidth`, not just that an `<img>` exists.
- **The footer mark's tooltip is anchored on the mark.** jsdom cannot see where
  a bubble lands (#891). The expected position comes from the mark's own box,
  never from a constant. Wrapping the mark's `<a>` in a `display: contents`
  span turns this test red.

The legal-line check has a positive beside it: the heading still names the
company on the same page, so the missing line is about the two URLs and not
about a page that failed to brand.

**The singleton.** Site info is one row shared by every spec in the run, and
there is no admin page for it yet (#1358), so the journey reads and writes it
through the admin API. It records what it found and restores exactly that in
`afterAll` (the stored values, or a reset when nothing was stored). The
company name is derived per attempt, so a retry cannot match a previous
attempt's state. No other spec asserts on the brand text.

The logo is the instance's own bundled `/logo_rounded.png`, addressed
absolutely from the base URL, so the journey depends on no external host.

## Resource locking (`resource-locking.journey.spec.ts`)

The ship gate for epic #1285, with three users in three browser contexts: the
team **owner**, member **A** who places the lock, and member **B** who created
the artifact. A locks the artifact from its detail page with a comment; B is
offered neither Unlock nor Delete and is refused both by the server, then edits
the content while status, type, project, labels and metadata are disabled; the
list marks the row and "Locked only" finds it; deleting the containing project
answers 423 `HAS_LOCKED_RESOURCES`; the owner unlocks, the Lock history lists
both events, and Delete works again. A prompt and a blueprint get a shorter
pass: lock, refused delete, unlock by the locker, delete.

Two of its assertions are the reason this is an e2e at all:

- **One lock, three sets of rights.** Each sub-issue is tested with a mock at
  its boundary; none can show a lock placed in one session being respected in a
  second and released from a third. The owner is the journey's "admin": A is
  the locker and B has to be a plain member, so the owner is the one actor who
  can prove `resource.unlock.any` — the permission both members are asserted
  not to hold.
- **The comment tooltip is anchored on its trigger.** jsdom has no layout
  engine, so a "Locked by" trigger that lost its box (#891: a `display:
contents` wrapper, bubble at the viewport origin) is invisible to the unit
  suite. The expected position is derived from the trigger's own box, never
  from a constant. Wrapping the trigger in a `display: contents` span turns
  this test red (the gap went from under 12px to over 1500px when tried).

Every "is not offered" has a positive beside it that fails if the page or the
rule were simply missing. B created both artifacts and is offered Lock and
Delete on the unlocked control, so their absence on the locked one — and the
423 `RESOURCE_WRITE_LOCKED` on B's own `DELETE` — can only be about the lock;
B's unlock attempt is a 403. The control row is what the list filter has to
drop. And the project delete that was refused is repeated as the last step,
when nothing in the project is locked any more, and goes through.

State is confirmed twice where it matters: what the page shows, and what the
API stores (`lock` on the resource, `GET …/lock/history`).

**What it cannot cover.** The SPA has no page message for
`HAS_LOCKED_RESOURCES`, and a locked resource has no Delete button, so the
refused project delete and the refused prompt/blueprint deletes are asserted on
the API response. The frozen-field matrix per type, a lock that arrives after
the edit page loaded, the project-migration and type/status-delete guards and
the MCP lock tool are covered by the unit and integration tests of #1289–#1296.
Memories cannot be locked.

## Custom statuses (`custom-statuses.journey.spec.ts`)

The ship gate for epic #1263: an owner creates a custom artifact status on
Settings › Statuses, sets it on an artifact through the edit form, finds that
artifact through global search and the list's status filter, then deletes the
status and sees the artifact back on the system default. A plain member reads
the same Statuses page with no create, rename or delete control, and a direct
`POST /api/v1/{team_id}/statuses` from that session answers 403.

Two of its assertions are the reason this is an e2e at all:

- **An artifact in a custom status stays visible.** Search and the lists hide
  resources by a denylist of non-live statuses. If that regressed into an
  allowlist of the system values, every unit suite would still pass and the
  artifact would vanish here — from the search results and from the filtered
  list, whose URL (`?status=<slug>`) must also survive a reload, when the page
  has to wait for the status catalog before it can tell the slug from junk.
- **Deleting a status reassigns its items, and the UI says so.** The confirm
  dialog names the one artifact and its destination before the request is sent,
  the response reports `reassigned_count: 1` / `reassigned_to: active`, and the
  detail badge, the form select and the list filter all agree afterwards.

The badges are asserted on the status's **display name** while the API is
asserted to store its **slug** — the pair is what shows the catalog, rather than
the raw value, is what the pages render.

The team is new, so it holds exactly two artifacts: the one that moves through
the custom status and a control that stays `active`. The control is what lets
the filter assertion fail — a filter that narrowed nothing would still show it.
The stack configures no embedding provider, so search takes the keyword path,
which reads the same live-status rule and has no indexing latency.

It uses artifacts only. The other three resource types share the same catalog
code and are covered by the unit and integration tests of #1265–#1270.

## Cross-team settings copy (`cross-team-settings-copy.journey.spec.ts`)

The ship gate for epic #827: one owner, two teams, and each of the three copy
surfaces driven through its settings-page dialog — a model provider, an
embedding provider, and the custom artifact types.

Two of its assertions are the reason this is an e2e at all:

- **The credential never leaves the server.** Every `/api/v1/` response body the
  run observes is recorded — the browser's via a context `response` listener,
  and the spec's own `page.request` calls explicitly, since those bypass the
  page's network stack and would otherwise sit outside the search — and the
  seeded key is searched for across all of them. That is a claim about a whole
  session, not about one handler, so no unit test can make it. It is paired with
  a check that the recorder saw a substantial amount of traffic, including a
  payload carrying `has_api_key`; without that a run recording nothing would
  pass just as happily.
- **The audit trail records the copy**, with the actor and the source team, which
  spans three services, a migration and a page.

It is deterministic without controlling time: the destination team starts empty,
so the embedding copy's server-side activation verdict is necessarily
`becomes_active`, and its warning dialog is asserted rather than probed for. The
type-slug collision is seeded before the copy rather than discovered.

Names that a locator matches on are scoped per **attempt**, not per file:
`describe.serial` re-runs `beforeAll` on a retry, and a file-scoped team name
would leave the source-team picker offering two identical options on the second
attempt — a red ship gate caused by the harness rather than the feature.

## Authentication settings and first-run setup (`admin-auth-settings.journey.spec.ts`)

The ship gate for issue #1239 (epic #1230): the sign-in providers, the access
allowlist and the setup page, driven from the browser against the real admin
API. Three tests, run serially as the root admin (`ADMIN_EMAIL`):

- **Providers, with no restart.** Add a provider in the dialog, see it on the
  **public** `GET /api/v1/auth/providers` list and on the sign-in page of a
  visitor with no session, edit it (read-only redirect URI, client secret left
  blank and kept), add a second one whose issuer nothing listens on (stored,
  `Unhealthy`, **not** offered), test that one from its row, reorder (and
  reload), delete, then disable the last enabled provider through the
  lockout-risk confirmation. The change history shows it, by this admin.
- **An allowlist tightening signs a second user out.** A second browser context
  is dev-logged in under a domain made up for the attempt; the admin saves an
  allowlist that leaves it out, confirms the dialog naming that user, and the
  second session's `/api/v1/auth/me` stops answering 200 while the root admin's
  keeps working. Then "Reset to open access".
- **First-run setup.** Setup is re-armed the way an operator would
  (`vibexp admin auth setup rearm` in the app container, through
  `e2e/helpers/e2eApp.ts`), and a context with **no session at all** sees the
  sign-in page's setup hint, a wrong token's terminal state, then the setup
  shell for the real token: token gone from the address bar, providers and
  allowlist only, the admins API a 404 and `/auth/me` a 401 on that session. A
  provider configured there brings up the "Sign in with … as a root admin to
  finish setup" prompt.

**What it cannot cover.** The stack has no identity provider, so nobody signs in
_through_ a provider: the journey stops at the prompt, and the root-admin
provider login that ends setup is covered by the backend's tests (#1236). Dev
login sessions carry no provider either, so the `own_provider` lockout reason
cannot be produced here; Vitest covers its copy. The stored GitHub provider is
never "tested" from the UI, because that check would send its made-up
credentials to github.com.

**It changes instance-wide settings beside other specs**, so it is written to be
unnoticeable:

- every provider it creates carries the `e2e-auth-` slug prefix and is deleted
  before and after each test, including one left by a failed attempt;
- the allowlist it saves admits **every email domain the suite signs in with**
  (`SUITE_DOMAINS` in the spec) and shuts out only the made-up one. **A spec
  that dev-logs in under a new domain must add it there**, or its users are
  signed out while this journey's allowlist is stored;
- the allowlist found at the start is restored afterwards.

The setup test **requires the docker e2e stack** (the setup URL exists nowhere
else) and skips with a reason without it. A stack started under another compose
project name is found through `E2E_COMPOSE_PROJECT`.

## Admin panel v3 (`admin-panel-v3.journey.spec.ts`)

The ship gate for epic #1131. It seeds three users, two shared teams and three
projects through the public API (never SQL), gives one team real configuration
(a model provider and an email provider holding a known secret, search / AI
summary / freshness settings, a custom artifact type), then drives the admin
surface as the instance admin (`ADMIN_EMAIL`):

- **users / teams / projects lists**: a count sort (desc, then flipped), an
  advanced filter asserted in the URL **and** in the rows, surviving a reload
  (and, on users, a detail round trip with `goBack`);
- **presets** on the teams list: save, clear, apply (the URL is restored),
  delete through the manage dialog;
- **CSV export** of each filtered list: filename, header row, and exactly the
  filtered rows;
- **detail pages**: the four user tabs, every team configuration tab (secrets
  shown as `Configured ✓` only), and the project Overview charts plus its
  Configuration tab.

The last test makes two claims about the whole session, recording every
`/api/v1/` body (the browsers' and the spec's own `page.request` calls, through
`e2e/helpers/apiRecorder.ts`, the recorder it shares with the cross-team copy
journey) plus the HTML of every admin page it asserted on:

- **Redaction**: the seeded secret is in no body and on no page, and no admin
  payload carries a secret-bearing key (`api_key`, `secret`, `webhook_url`,
  `webhook_secret`, `last_error`). Paired with a check that it saw a
  `has_api_key` admin payload, so a run that recorded nothing cannot pass.
- **Counts only, never titles**: no seeded resource title is in any
  `/api/v1/admin/` body or on any admin page.

Other specs share the database, so no assertion reads a global total: each list
is narrowed to the attempt's rows by searching for a per-attempt tag first.
Presets this spec saved on an earlier failed attempt are pruned in `beforeAll`,
so the per-admin cap of 20 is never reached. The non-admin half (redirects from
the v3 pages, 404 from the preset, export and team-config APIs) lives in
`e2e/features/admin/admin-negative.spec.ts`.

## AI Summary (`ai-summary.journey.spec.ts`)

The ship gate for epic #1068: search → the collapsed AI Summary section →
expand → the cited summary and its Sources footer, the header search dialog's
compact summary, a settings change reaching the next summary, the classified
unauthorized error with a working Retry, and the provider-less member/owner
split.

**No real LLM is ever contacted.** `docker-compose.e2e.yml` runs
`backend/cmd/llm-test-provider`, a deterministic OpenAI-compatible stub, and the
spec registers it as the team's model provider — so a summary really travels
backend → provider and back, exercising the LLM service and its error
classification rather than a route mock. The stub:

- cites the first document it is sent and echoes the style and document count
  it received (`Style: detailed · Documents: 3`) — that is how the settings
  assertion reads its effect from the page;
- answers the key **`e2e-bad-key`** with 401, so the unauthorized path is driven
  by editing the provider's key, never by toggling shared stub state that a
  retried or parallel test could race;
- delays each completion by one second, so the loading skeleton is observable.

**The request count is asserted in the browser.** "One summary per search" is a
client contract — the summary is cached per search identity, and the page is
deliberately not part of it — so every `POST …/search/summary` the browser issues
is counted, and collapse → re-expand → results page 2 must leave it at exactly
one. That cache lives for one page load, so a step that needs a fresh summary for
the same query reloads the page.

Like the freshness journey it **requires the docker e2e stack**; pointed at a
bare `npm run dev` it skips with a reason unless `E2E_LLM_PROVIDER_URL` names a
stub the backend can reach.

## Resource freshness (`resource-freshness.journey.spec.ts`)

The ship gate for epic #726: a rule created in the UI → the scheduler's
evaluation run marking an aged artifact stale → the badge, the "stale only"
filter, the analytics and audit tabs → opening the artifact clearing the flag.

It needs two things the other journeys do not, both about **controlling time
rather than waiting for it**:

- **`SCHEDULER_TICK_INTERVAL: 5s`** in `docker-compose.e2e.yml`. Writing a
  freshness rule leaves the team's `schedules` row due immediately, so the tick
  is the entire delay before the evaluation runs; at the 1-minute default the
  spec would idle for a minute per run. It only changes how often due schedules
  are polled — each schedule keeps its own interval.
- **Direct SQL** (`e2e/helpers/e2eDatabase.ts`) to age the seeded artifact.
  `threshold_days` has a one-day floor and staleness compares
  `GREATEST(updated_at, <last_accessed_*>)` against it, so a resource the spec
  just created can never be a candidate — there is no API that makes a row old.
  The helper reaches the compose stack's Postgres container by its compose
  labels.

Because of the second point the journey **requires the docker e2e stack**
(`make e2e`, or `ci-e2e.yml`); pointed at a bare `npm run dev` it skips with a
reason. Ordering inside it is load-bearing: the artifact is aged _before_ the
rule exists, because the run that the rule's creation triggers is the only
evaluation the journey gets.

> **2026-06 consolidation (issue #762).** After the shadcn v2 redesign, the
> journeys that merely re-tested a single feature (`api-key-workflow`,
> `prompt-workflow`, `artifact-management`) were removed because they duplicate
> the `e2e/features/**` specs. `onboarding.journey`/`onboarding.spec` were
> removed because the mandatory onboarding wizard was deleted with v1 (#1129).
>
> Notes on the survivors:
>
> - `upgrade-subscription.journey` is a **real** Stripe checkout lifecycle and is
>   skipped unless `STRIPE_E2E=true` (needs live Stripe test keys + `stripe
listen`; see the `/test-stripe-payment-flow` skill).
> - `team-collaboration.journey` has one `test.fixme` (team-scoped resource
>   creation) tracking a real bug: newly created teams have no default project,
>   so prompts created in a fresh team are mis-scoped.

The sections below describe the original Phase-2 journey plan; some journeys
referenced here have since been consolidated as noted above.

## Journey 1: New User Onboarding (`onboarding.journey.spec.ts`)

**Purpose**: Validates the critical first-time user experience from landing on the homepage through completing onboarding steps.

**User Flow**:

1. Visit homepage (unauthenticated)
2. See sign-in options
3. Complete dev login
4. Land on dashboard with onboarding checklist (0/6)
5. Verify "Private Workspace" team is auto-created
6. Complete onboarding steps:
   - Connect AI Tools
   - Create First Prompt
   - Save First Artifact
   - Add to Memory
   - Connect MCP Server
   - Generate API Keys
7. Verify progress tracking

**Test Coverage**: 13 test cases

**Key Validations**:

- Unauthenticated homepage displays correctly
- Login flow completes successfully
- Dashboard loads with onboarding checklist
- Private Workspace team is auto-created
- Each onboarding step navigates to correct page
- Progress persists across page reloads

## Journey 2: Prompt Creation & Usage Workflow (`prompt-workflow.journey.spec.ts`)

**Purpose**: Tests the complete prompt lifecycle including advanced features like placeholders and @mentions.

**User Flow**:

1. Navigate to Prompts → My Prompts
2. Create a new prompt with:
   - Name, description, tags
   - Content with `{{placeholder}}` variables
3. Save and view the prompt
4. Test placeholder rendering:
   - Switch to "Rendered" view
   - Fill in placeholder values
   - Copy the prompt body (the Copy action copies the SOURCE, not the
     placeholder-rendered output, since #901)
5. Create another prompt that includes the first via @mention
6. Edit the original prompt
7. View both prompts in the list
8. Filter/search prompts
9. Delete a prompt

**Test Coverage**: 18 test cases

**Key Validations**:

- Empty state display for new users
- Prompt creation with basic content
- Auto-generation of slug from name
- Placeholder variable detection (`{{var}}`)
- Raw vs Rendered view switching
- Placeholder filling and rendering
- @mention functionality for prompt inclusion
- Combining @mentions and placeholders
- Prompt editing preserving content
- List view displaying multiple prompts
- Filtering by status (active/draft)
- Searching by name and content
- Deletion with confirmation

## Journey 3: API Key Workflow (`api-key-workflow.journey.spec.ts`)

**Purpose**: Validates the security-critical API key management workflow.

**User Flow**:

1. Navigate to Settings → API Keys
2. Create new API key with name
3. View full key (shown only once)
4. Copy key to clipboard
5. Close modal
6. View key in list (masked format)
7. Delete API key
8. Confirm deletion

**Test Coverage**: 19 test cases

**Key Validations**:

- Navigation from dashboard and settings menu
- Empty state for users with no keys
- Create modal opens with name input
- Validation of required name field
- API key creation with unique value
- Full key displayed immediately after creation
- Security warning about one-time display
- Copy to clipboard functionality
- Clipboard contains correct key value
- Modal closes after key is saved
- Key displayed in list with masked format
- Key metadata visible (name, created date)
- Full key NOT revealed after modal closed
- Delete confirmation dialog
- Canceling deletion keeps key
- Confirming deletion removes key
- Multiple keys in list handled correctly

## Journey 4: Prompt Gallery to Custom Prompt (`prompt-gallery.journey.spec.ts`)

**Purpose**: Tests the discovery-to-customization workflow where users browse the public prompt gallery, find useful templates, and adapt them for their own needs.

**User Flow**:

1. Navigate to Prompt Gallery from main navigation
2. Browse available categories (Writing, Coding, Business, etc.)
3. Navigate to category detail page
4. View prompt details from gallery
5. Clone/Use a gallery prompt
6. Customize the cloned prompt
7. Save as custom prompt
8. Verify prompt appears in My Prompts

**Test Coverage**: 14 test cases

**Key Validations**:

- Gallery navigation from main nav
- Empty state display if no prompts exist
- Category grid display
- Category detail page navigation
- Prompt detail viewing
- Content preview display
- "Use This Prompt" / "Clone" button availability
- Prompt cloning to personal workspace
- Editing cloned prompt
- Customizing cloned content
- Verification in My Prompts list
- Using customized prompt

## Journey 5: Subscription Upgrade (`subscription-upgrade.journey.spec.ts`)

**Purpose**: Validates the complete subscription upgrade flow from viewing plans through Stripe checkout completion and quota verification. This tests the critical payment integration.

**User Flow**:

1. Navigate to Subscription page from user menu
2. View Individual Plans vs Team Plans tabs
3. See current plan status
4. Create a team (required for team subscriptions)
5. Switch to team context
6. View all three team tiers (Starter, Professional, Enterprise)
7. Select Professional plan with seat count (e.g., 2 seats)
8. Initiate Stripe checkout
9. Complete test payment (test card: 4242 4242 4242 4242)
10. Verify subscription status upgraded
11. Verify quota limits calculated correctly
12. Access billing portal

**Test Coverage**: 12 test cases (3 skipped - require Stripe test mode)

**Key Validations**:

- Subscription page navigation
- Individual vs Team Plans tabs display
- Current plan status display
- All three team tiers visible
- Seat count selector functionality
- Pricing per seat display
- Error when subscribing without team
- Team creation for subscription
- Team context switching
- Stripe checkout initiation
- Subscription status after payment ⏭️ (skipped - manual test)
- Quota limits verification ⏭️ (skipped - manual test)
- Billing portal access ⏭️ (skipped - manual test)

**Manual Testing Required**: Tests marked with ⏭️ require Stripe test mode setup and are skipped in CI.

## Journey 6: Artifact Management Workflow (`artifact-management.journey.spec.ts`)

**Purpose**: Tests the complete artifact lifecycle from creation through editing, viewing, and deletion. Validates structured content management features.

**User Flow**:

1. Navigate to Artifacts from main navigation
2. View empty state or existing artifacts
3. Create new artifact with:
   - Title
   - Description
   - Type (general, work_reports, static_contexts)
   - Content
4. View artifact details
5. Edit artifact content
6. Search/filter artifacts by title, type, status
7. Delete artifact with confirmation

**Test Coverage**: 16 test cases

**Key Validations**:

- Artifacts navigation from main nav
- Empty state display for new users
- Create artifact button visibility
- Artifact creation form
- Title, description, content fields
- Type selector functionality
- Auto-generated slug from title
- Required field validation
- Artifact list display
- Artifact detail viewing
- Metadata display (created date, type, status)
- Edit page navigation
- Content updating
- Metadata preservation after edit
- Search by title
- Filter by type and status
- Deletion with confirmation

## Journey 7: Team Collaboration Workflow (`team-collaboration.journey.spec.ts`)

**Purpose**: Tests the complete team collaboration experience from team creation through member management, resource sharing, and team context switching. Validates multi-tenant features.

**User Flow**:

1. Navigate to Teams from Settings
2. View default Personal Workspace
3. Create new team with name and description
4. View team details
5. Invite team members (enter email)
6. Switch between teams using team switcher
7. Create resources in team context (prompts, artifacts)
8. Verify team resources are isolated
9. Manage team members and roles
10. Leave or delete team

**Test Coverage**: 21 test cases

**Key Validations**:

- Teams navigation from settings
- Default Personal Workspace display
- Create Team button visibility
- Team creation with name and description
- Auto-generated team slug
- Required name validation
- Team details viewing
- Team member count display
- Team owner badge display
- Invite Members button
- Invite dialog with email input
- Email validation for invitations
- Team switcher in header
- Current team display in switcher
- Team context switching
- Team context persistence across navigation
- Resource creation in team context
- Resource isolation between teams
- Team members list display
- Member roles display (Owner, Admin, Member)
- Delete Team option for owner
- Deletion confirmation dialog

## Running Journey Tests

### Run All Journey Tests

```bash
npm run test:e2e -- journeys/
```

### Run Specific Journey

```bash
# Onboarding journey
npm run test:e2e -- journeys/onboarding.journey.spec.ts

# Prompt workflow journey
npm run test:e2e -- journeys/prompt-workflow.journey.spec.ts

# API key workflow journey
npm run test:e2e -- journeys/api-key-workflow.journey.spec.ts

# Prompt gallery journey (Journey 4)
npm run test:e2e -- journeys/prompt-gallery.journey.spec.ts

# Subscription upgrade journey (Journey 5)
npm run test:e2e -- journeys/subscription-upgrade.journey.spec.ts

# Artifact management journey (Journey 6)
npm run test:e2e -- journeys/artifact-management.journey.spec.ts

# Team collaboration journey (Journey 7)
npm run test:e2e -- journeys/team-collaboration.journey.spec.ts
```

### Run with UI Mode (for debugging)

```bash
npm run test:e2e -- journeys/ --ui
```

### Run Specific Test Case

```bash
npm run test:e2e -- journeys/onboarding.journey.spec.ts:22
```

## Test Patterns Used

### Fixtures

All journey tests use fixtures from `e2e/fixtures/`:

- `authenticatedPage`: Pre-authenticated user page
- `freshUserPage`: New user with clean state (for onboarding tests)
- `generateUniqueEmail()`: Generates unique test emails
- `generatePromptData()`: Generates test prompt data

### Example Usage

```typescript
import { test, expect } from '../fixtures/auth'
import { generatePromptData } from '../fixtures/test-data'

test('should create prompt', async ({ authenticatedPage }) => {
  const promptData = generatePromptData({
    name: 'My Test Prompt',
    content: 'You are a helpful assistant. {{task}}',
  })

  // Test implementation...
})
```

### Best Practices

1. **Use Descriptive Test Names**: Test names should read like user stories
2. **Test Complete Flows**: Don't just test isolated actions
3. **Verify State Changes**: Check that actions have expected effects
4. **Use Real Navigation**: Don't jump directly to URLs unnecessarily
5. **Handle Async Operations**: Wait for API responses and UI updates
6. **Clean Test Data**: Each test should be independent
7. **Take Screenshots on Failure**: Automatic via Playwright config

## Implementation Notes

### Known Issues

Some tests may be flaky or fail if:

1. **Onboarding Progress Tracking**: The backend onboarding tracking feature may not be fully implemented yet. Tests checking for progress updates (e.g., "1/6", "2/6") may fail.

2. **Dynamic UI Elements**: Some UI elements (buttons, modals) may have varying selectors. Tests use flexible locators (`:has-text()`, multiple selector options) to handle this.

3. **Timing Issues**: Tests include appropriate `waitForTimeout()` calls, but some async operations may need adjustment.

### Future Improvements

- [ ] Add visual regression testing for journey flows
- [ ] Add performance metrics collection
- [ ] Test error recovery paths
- [ ] Add accessibility validation in journeys
- [ ] Test offline/network failure scenarios

## Success Metrics

Target metrics for Phase 3:

- ✅ 7 journey test files created (3 existing + 4 new from issue #772)
- ✅ 110+ total test cases implemented
  - Journey 1 (Onboarding): 13 tests
  - Journey 2 (Prompt Workflow): 18 tests
  - Journey 3 (API Key Workflow): 19 tests
  - Journey 4 (Prompt Gallery): 14 tests
  - Journey 5 (Subscription Upgrade): 12 tests
  - Journey 6 (Artifact Management): 16 tests
  - Journey 7 (Team Collaboration): 19 tests
- ⏳ Tests complete in < 5 minutes (parallel execution)
- ⏳ All journey tests pass consistently (requires backend and Stripe test mode setup)
- ✅ Tests use enhanced fixtures from Phase 1
- ⏳ No flaky tests (3 consecutive passes needed)
- ✅ Comprehensive coverage of core user journeys

## Related Documentation

- [E2E Testing README](../README.md) - Main E2E testing documentation
- [Phase 1: Fixtures & Infrastructure](../fixtures/README.md)
- [Frontend Developer Guide](https://docs.vibexp.io/developer-guide/frontend/overview/) - Published contributor documentation

## Support

For issues or questions:

1. Check test output and screenshots in `test-results/`
2. Run tests with `--debug` flag for step-by-step execution
3. Use `--ui` mode to visually debug test failures
4. Review [Playwright documentation](https://playwright.dev/)
