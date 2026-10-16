import { expect, type BrowserContext, type Page } from '@playwright/test'

/** How long one response body may take to read before it counts as unreadable. */
const BODY_READ_TIMEOUT_MS = 5_000

/** Rejects if `promise` has not settled within `ms`. */
async function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error(`timed out after ${String(ms)}ms`))
    }, ms)
  })
  try {
    return await Promise.race([promise, timeout])
  } finally {
    clearTimeout(timer)
  }
}

/** One recorded `/api/v1/` response. */
export interface RecordedResponse {
  url: string
  body: string
}

/**
 * Records every `/api/v1/` response body a journey sees, so a session-wide
 * claim ("this secret was never serialized") can be asserted over all of them.
 *
 * Shared by the journeys that make such claims (#838, #1151): two copies of a
 * redaction recorder would drift, and a fix landing in only one would silently
 * weaken the other's leak check.
 *
 * Both halves of the traffic are covered:
 * - the browser's, via {@link attach} on each context;
 * - the spec's own `page.request` calls, via {@link send}. Those bypass the
 *   page's network stack entirely and never surface as context `response`
 *   events, so without this the seeding traffic — exactly where a leaked
 *   credential would show up — would sit outside the search.
 */
export class ApiRecorder {
  private readonly recorded: RecordedResponse[] = []

  /**
   * Reading a body is asynchronous, so one can still be in flight when the
   * search runs. Every read is tracked and awaited by {@link settled}, so "we
   * found nothing" can never mean "we had not finished looking".
   */
  private readonly pending: Promise<void>[] = []

  /**
   * URLs whose body could not be read, or did not finish reading within
   * {@link BODY_READ_TIMEOUT_MS}. A read can stay pending forever (Playwright
   * does not always settle it when the request is torn down), and an
   * unbounded wait in {@link settled} hung the cross-team copy journey until
   * its test timeout (#1568). Kept, not dropped silently, so a leak check can
   * name what it could not inspect.
   */
  readonly unreadable: string[] = []

  /**
   * Record a context's API traffic. Bind it before the first navigation so
   * nothing is missed. A body that cannot be read in time (a redirect, an
   * aborted request) is listed in {@link unreadable}; callers pair their
   * assertion with a check that a substantial amount of traffic was seen.
   */
  attach(context: BrowserContext): void {
    context.on('response', response => {
      const url = response.url()
      if (!url.includes('/api/v1/')) return
      this.pending.push(
        withTimeout(response.text(), BODY_READ_TIMEOUT_MS)
          .then(body => {
            this.recorded.push({ url, body })
          })
          .catch(() => {
            this.unreadable.push(url)
          })
      )
    })
  }

  /** A recorded API call that must succeed; returns the parsed JSON body. */
  async send(
    page: Page,
    method: 'GET' | 'POST' | 'PUT',
    url: string,
    data?: unknown
  ): Promise<Record<string, unknown>> {
    const res = await page.request.fetch(url, { method, data })
    const body = await res.text()
    this.recorded.push({ url: res.url(), body })
    expect(res.ok(), `${method} ${url} failed: ${res.status()} ${body}`).toBe(
      true
    )
    return JSON.parse(body) as Record<string, unknown>
  }

  /** Everything recorded, once every in-flight body read has settled. */
  async settled(): Promise<readonly RecordedResponse[]> {
    await Promise.allSettled(this.pending)
    return this.recorded
  }
}
