/**
 * The first-run onboarding's copy, verbatim from the approved design
 * (Claude Design `templates/onboarding/Onboarding.dc.html`). Keep it in sync
 * with the design rather than editing it here.
 */

/** The flow's steps, in order. The last one is the celebration screen. */
export const ONBOARDING_STEPS = [
  { title: 'Welcome', description: 'Meet the shared brain' },
  {
    title: 'Building blocks',
    description: 'Memory, blueprints, artifacts, feeds',
  },
  { title: 'Claude Code plugin', description: 'Add the VibeXP commands' },
  { title: 'Team rules', description: '~/.claude/CLAUDE.md' },
  { title: 'VibeXP CLI', description: 'Token-efficient setup' },
  { title: 'All set', description: 'Start a smarter session' },
] as const

export const STEP_COUNT = ONBOARDING_STEPS.length
export const LAST_STEP = STEP_COUNT - 1
/** Steps that can be done or skipped: every one but the celebration. */
export const ACTIONABLE_STEP_COUNT = STEP_COUNT - 1

/** The primary button's label on each step. */
export const NEXT_LABELS = [
  'Get started',
  'Continue',
  "I've installed it",
  "I've added the rules",
  "I've run it",
  'Go to VibeXP',
] as const

export const WELCOME_VIDEO_ID = '-9NFGBjoc5g'

export const RULES = `# Team shared knowledge (VibeXP) — mandatory

VibeXP is our team's shared knowledge platform. For every task, follow this loop:

1. Read — Before starting, run /vibexp:prime to load the team's shared
   knowledge for this team and project (if they exist in VibeXP).
2. Learn — While working, collect learnings, potential improvements,
   durable facts and guidelines.
3. Write — After finishing, run /vibexp:wrap to update durable memory,
   save guidelines and rules as blueprints, and publish reports or
   general info as artifacts.
4. Consolidate — Keep the shared knowledge current, deduplicated and
   usable for the next session.

Read → learn → write: every session should leave the team's knowledge
better than it found it.

Only share knowledge that is relevant and useful to the team. Never write
personal information, secrets, credentials or customer data to VibeXP.`

/** The rules as one shell command that appends them to the file. */
export const RULES_APPEND = `cat >> ~/.claude/CLAUDE.md <<'EOF'\n\n${RULES}\nEOF`

export const PLUGIN_COMMANDS = {
  claude: {
    add: '/plugin marketplace add vibexp/plugin-marketplace',
    install: '/plugin install vibexp@vibexp',
  },
  term: {
    add: 'claude plugin marketplace add vibexp/plugin-marketplace',
    install: 'claude plugin install vibexp@vibexp',
  },
} as const

export type PluginMode = keyof typeof PLUGIN_COMMANDS

/**
 * The CLI step's command.
 *
 * A deliberate deviation from the design's `claude -p "/vibexp:install"`: the
 * install skill asks questions, waits for a go-ahead and signs in through the
 * browser, none of which a non-interactive `-p` run can do. It is started
 * interactively instead, with this instance's URL as its argument so the user
 * does not have to type it.
 */
export function cliInstallCommand(origin: string): string {
  return `claude "/vibexp:install ${origin}"`
}
