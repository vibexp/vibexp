# Journey Tests (Phase 2)

Journey tests validate complete end-to-end user flows that span multiple features and pages. These tests ensure critical user experiences work correctly from start to finish.

## Test Structure

Journey tests are organized by user scenario:

```
e2e/journeys/
├── prompt-gallery.journey.spec.ts           # Gallery → custom prompt
├── upgrade-subscription.journey.spec.ts     # Full Stripe lifecycle (gated)
├── team-collaboration.journey.spec.ts       # Team collaboration workflow
└── README.md                                # This file
```

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
   - Copy rendered output
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

**Test Coverage**: 19 test cases

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
- [Developer Guidelines](../../../docs/developer-guidelines/frontend/)

## Support

For issues or questions:

1. Check test output and screenshots in `test-results/`
2. Run tests with `--debug` flag for step-by-step execution
3. Use `--ui` mode to visually debug test failures
4. Review [Playwright documentation](https://playwright.dev/)
