import { expect, type BrowserContext, type Page } from '@playwright/test'

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
   * Record a context's API traffic. Bind it before the first navigation so
   * nothing is missed. A body that cannot be read (a redirect, an aborted
   * request) is skipped; callers pair their assertion with a check that a
   * substantial amount of traffic was seen.
   */
  attach(context: BrowserContext): void {
    context.on('response', response => {
      const url = response.url()
      if (!url.includes('/api/v1/')) return
      this.pending.push(
        response
          .text()
          .then(body => {
            this.recorded.push({ url, body })
          })
          .catch(() => {
            /* body unavailable — nothing to inspect */
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
