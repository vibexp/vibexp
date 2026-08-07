/**
 * Unit tests for the enhanced analytics service (src/services/analytics.ts).
 *
 * The service reads its gate from `utils/gtm` (GTM_ENABLED) at module load, so
 * each variant is loaded through `vi.isolateModules` + `vi.doMock`:
 * - the default test environment has no VITE_GTM_ID, which must make every
 *   dispatch a no-op;
 * - the enabled variant mocks `@/utils/gtm` with GTM_ENABLED=true and a spy
 *   for its trackEvent to observe dispatches;
 * - the dev variant additionally mocks `../utils/environment` (the specifier
 *   jest maps for analytics.ts) so isDevMode() returns true, covering the
 *   import.meta.env.DEV debug/validation branch.
 */
import type { Mock, MockInstance } from 'vitest'

import type {
  AnalyticsEvent,
  TrackAuthParams,
  UserProperties,
} from '../../types/analytics'

type AnalyticsModule = typeof import('../analytics')
type Svc = AnalyticsModule['analyticsService']

interface LoadOptions {
  gtmEnabled: boolean
  dev?: boolean
  gtmTrack?: Mock
  useRealGtm?: boolean
}

async function loadService(opts: LoadOptions): Promise<Svc> {
  vi.resetModules()
  if (opts.useRealGtm) {
    // Restore the real gtm module — its own GTM_ENABLED gate is derived from
    // VITE_GTM_ID, which is empty in the default test env.
    vi.doMock('@/utils/gtm', () => vi.importActual('@/utils/gtm'))
  } else {
    vi.doMock('@/utils/gtm', () => ({
      GTM_ENABLED: opts.gtmEnabled,
      GTM_ID: opts.gtmEnabled ? 'GTM-TEST' : '',
      GA4_MEASUREMENT_ID: '',
      trackEvent: opts.gtmTrack ?? vi.fn(),
      initializeGTM: vi.fn(),
      getGA4ClientId: vi.fn(),
      pushDataLayerEvent: vi.fn(),
      setGA4UserId: vi.fn(),
    }))
  }
  vi.doMock('@/utils/environment', () => ({
    isDevMode: () => opts.dev ?? false,
    getApiBaseUrl: () => 'https://api.vibexp.io/api/v1',
  }))
  const mod: AnalyticsModule = await import('../analytics')
  return mod.analyticsService
}

function buildPageViewEvent(
  overrides: Partial<Record<string, unknown>> = {}
): AnalyticsEvent {
  return {
    event: 'page_view',
    timestamp: Date.now(),
    page_path: '/prompts',
    environment: 'development',
    ...overrides,
  } as AnalyticsEvent
}

const userProps: UserProperties = {
  user_id: 'user-1',
  email: 'user@example.com',
  name: 'Test User',
}

describe('analyticsService', () => {
  beforeEach(() => {
    window.dataLayer = []
  })

  describe('gating: GTM disabled (default jest env)', () => {
    it('never dispatches through the real gtm module with the default env', async () => {
      const svc = await loadService({ gtmEnabled: false, useRealGtm: true })

      svc.trackEvent({ event: 'prompts_page_view' })
      svc.trackPage({ path: '/prompts', title: 'Prompts' })
      svc.trackAuth({ eventType: 'signed_in' })
      svc.identify(userProps)

      expect(window.dataLayer).toHaveLength(0)
    })

    it('reports isEnabled() false even when a dataLayer exists', async () => {
      const svc = await loadService({ gtmEnabled: false })

      expect(svc.isEnabled()).toBe(false)
      expect(svc.getConfig()).toMatchObject({
        enabled: false,
        debug: false,
        // NODE_ENV=test keeps the constructor on the development branch.
        environment: 'development',
      })
    })

    it('makes every tracking method a no-op', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: false, gtmTrack })

      svc.track(buildPageViewEvent())
      svc.trackEvent({ event: 'prompts_page_view' })
      svc.trackPage({ path: '/prompts', title: 'Prompts' })
      svc.trackAuth({ eventType: 'logged_out' })
      svc.trackError({ error: new Error('boom') })
      svc.identify(userProps)
      svc.setUserProperties({ name: 'Renamed' })
      svc.clearUser()

      expect(gtmTrack).not.toHaveBeenCalled()
      expect(window.dataLayer).toHaveLength(0)
    })
  })

  describe('gating: GTM enabled (mocked gate)', () => {
    let gtmTrack: Mock
    let svc: Svc

    beforeEach(async () => {
      gtmTrack = vi.fn()
      svc = await loadService({ gtmEnabled: true, gtmTrack })
    })

    it('is enabled with a dataLayer, disabled without one', () => {
      expect(svc.isEnabled()).toBe(true)
      ;(window as { dataLayer?: unknown }).dataLayer = undefined
      expect(svc.isEnabled()).toBe(false)
    })

    it('track() enriches the event and dispatches it to GTM', () => {
      document.title = 'VibeXP Test'

      svc.track(buildPageViewEvent())

      expect(gtmTrack).toHaveBeenCalledTimes(1)
      expect(gtmTrack).toHaveBeenCalledWith(
        'page_view',
        expect.objectContaining({
          event: 'page_view',
          page_path: '/prompts',
          page_title: 'VibeXP Test',
          environment: 'development',
          session_id: expect.stringMatching(/^session_/) as string,
        })
      )
    })

    it('trackEvent() forwards the event name and custom properties', () => {
      svc.trackEvent({
        event: 'prompt_created',
        properties: { prompt_id: 'p-1' },
      })

      expect(gtmTrack).toHaveBeenCalledWith(
        'prompt_created',
        expect.objectContaining({ prompt_id: 'p-1' })
      )
    })

    it('trackPage() dispatches a page_view with the given path and title', () => {
      svc.trackPage({ path: '/memories', title: 'Memories' })

      expect(gtmTrack).toHaveBeenCalledWith(
        'page_view',
        expect.objectContaining({
          page_path: '/memories',
          page_title: 'Memories',
        })
      )
    })

    it.each([
      ['signin_page_view', 'user_signin_page_view'],
      ['signed_in', 'user_signed_in'],
      ['signed_in_first_time', 'user_signed_in_first_time'],
      ['logged_out', 'user_logged_out'],
    ] as const)(
      'trackAuth(%s) maps to the %s event',
      (eventType, expectedEvent) => {
        svc.trackAuth({ eventType })

        expect(gtmTrack).toHaveBeenCalledWith(
          expectedEvent,
          expect.objectContaining({ event: expectedEvent })
        )
      }
    )

    it('an unknown auth event type is reported as a javascript_error', () => {
      svc.trackAuth({ eventType: 'bogus' } as unknown as TrackAuthParams)

      expect(gtmTrack).toHaveBeenCalledWith(
        'javascript_error',
        expect.objectContaining({
          error_message: expect.stringContaining(
            'Unknown auth event type'
          ) as string,
        })
      )
    })

    it('trackError() dispatches a javascript_error with the error details', () => {
      svc.trackError({ error: new Error('boom'), component: 'unit-test' })

      expect(gtmTrack).toHaveBeenCalledWith(
        'javascript_error',
        expect.objectContaining({
          error_message: 'boom',
          error_component: 'unit-test',
        })
      )
    })

    it('identify() pushes the user to the data layer and later events carry it', () => {
      svc.identify(userProps)

      expect(window.dataLayer).toContainEqual(
        expect.objectContaining({
          event: 'user_identified',
          user_id: 'user-1',
        })
      )

      svc.track(buildPageViewEvent())
      expect(gtmTrack).toHaveBeenCalledWith(
        'page_view',
        expect.objectContaining({
          user_properties: expect.objectContaining({
            user_id: 'user-1',
          }) as UserProperties,
        })
      )
    })

    it('setUserProperties() merges into the identified user', () => {
      svc.identify(userProps)
      svc.setUserProperties({ name: 'Renamed User' })

      expect(window.dataLayer).toContainEqual(
        expect.objectContaining({
          event: 'user_properties_updated',
          user_properties: expect.objectContaining({
            user_id: 'user-1',
            name: 'Renamed User',
          }) as UserProperties,
        })
      )
    })

    it('setUserProperties() without an identified user pushes nothing', () => {
      svc.setUserProperties({ name: 'Nobody' })

      expect(window.dataLayer).toHaveLength(0)
    })

    it('clearUser() tracks the logout and clears the data layer user', () => {
      svc.identify(userProps)
      svc.clearUser()

      expect(gtmTrack).toHaveBeenCalledWith(
        'user_logged_out',
        expect.objectContaining({ event: 'user_logged_out' })
      )
      expect(window.dataLayer).toContainEqual(
        expect.objectContaining({ event: 'user_cleared' })
      )
    })

    it('swallows internal errors silently when error tracking is disabled', () => {
      svc.configure({ enableErrorTracking: false })

      svc.trackAuth({ eventType: 'bogus' } as unknown as TrackAuthParams)

      // No javascript_error dispatch: handleError returns before trackError.
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('the global error listener reports window error events', () => {
      window.dispatchEvent(
        new ErrorEvent('error', {
          error: new Error('kaboom'),
          message: 'kaboom',
        })
      )

      expect(gtmTrack).toHaveBeenCalledWith(
        'javascript_error',
        expect.objectContaining({
          error_message: 'kaboom',
          error_component: 'global_error_handler',
        })
      )
    })

    it('the global listener reports unhandled promise rejections', () => {
      const event = new Event('unhandledrejection')
      Object.assign(event, { reason: new Error('rejected promise') })

      window.dispatchEvent(event)

      expect(gtmTrack).toHaveBeenCalledWith(
        'javascript_error',
        expect.objectContaining({
          error_message: 'rejected promise',
          error_component: 'unhandled_promise_rejection',
        })
      )
    })
  })

  describe('dev mode (import.meta.env.DEV branch)', () => {
    let consoleLog: MockInstance
    let consoleWarn: MockInstance
    let consoleError: MockInstance
    let consoleGroup: MockInstance
    let consoleGroupEnd: MockInstance

    beforeEach(() => {
      consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined)
      consoleWarn = vi
        .spyOn(console, 'warn')
        .mockImplementation(() => undefined)
      consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined)
      consoleGroup = vi
        .spyOn(console, 'group')
        .mockImplementation(() => undefined)
      consoleGroupEnd = vi
        .spyOn(console, 'groupEnd')
        .mockImplementation(() => undefined)
    })

    afterEach(() => {
      consoleLog.mockRestore()
      consoleWarn.mockRestore()
      consoleError.mockRestore()
      consoleGroup.mockRestore()
      consoleGroupEnd.mockRestore()
    })

    it('logs service initialization with debug enabled', async () => {
      const svc = await loadService({ gtmEnabled: true, dev: true })

      expect(svc.getConfig()).toMatchObject({
        debug: true,
        enableConsoleLogging: true,
      })
      expect(consoleLog).toHaveBeenCalledWith(
        '[Analytics] Service initialized',
        expect.any(Object)
      )
    })

    it('validates, console-groups and dispatches a valid event', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      svc.track(buildPageViewEvent())

      expect(consoleGroup).toHaveBeenCalledWith('[Analytics] Event: page_view')
      expect(gtmTrack).toHaveBeenCalledWith(
        'page_view',
        expect.objectContaining({ event: 'page_view' })
      )
    })

    it('drops an invalid event instead of dispatching it', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      // Missing timestamp / page_path / environment.
      svc.track({ event: 'page_view' } as AnalyticsEvent)

      expect(consoleError).toHaveBeenCalledWith(
        '[Analytics] Invalid event will not be sent:',
        expect.any(Array)
      )
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('drops trackEvent calls with invalid parameters', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      svc.trackEvent({ event: '' })

      expect(consoleError).toHaveBeenCalledWith(
        '[Analytics] Invalid trackEvent parameters:',
        expect.any(Array)
      )
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('drops trackPage calls with invalid parameters', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      svc.trackPage({ path: '', title: '' })

      expect(consoleError).toHaveBeenCalledWith(
        '[Analytics] Invalid trackPage parameters:',
        expect.any(Array)
      )
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('dispatches signed_in_first_time in dev (validator allowlist matches the type)', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      svc.trackAuth({ eventType: 'signed_in_first_time' })

      expect(consoleError).not.toHaveBeenCalledWith(
        '[Analytics] Invalid trackAuth parameters:',
        expect.any(Array)
      )
      expect(gtmTrack).toHaveBeenCalledWith(
        'user_signed_in_first_time',
        expect.objectContaining({ event: 'user_signed_in_first_time' })
      )
    })

    it('drops trackAuth calls the dev validator rejects', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: true, dev: true, gtmTrack })

      svc.trackAuth({ eventType: 'bogus' } as unknown as TrackAuthParams)

      expect(consoleError).toHaveBeenCalledWith(
        '[Analytics] Invalid trackAuth parameters:',
        expect.any(Array)
      )
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('logs identify and clearUser with PII redacted to user_id only', async () => {
      const svc = await loadService({ gtmEnabled: true, dev: true })

      svc.identify(userProps)
      expect(consoleLog).toHaveBeenCalledWith('[Analytics] User identified:', {
        user_id: 'user-1',
      })

      svc.clearUser()
      expect(consoleLog).toHaveBeenCalledWith('[Analytics] User cleared')
    })

    it('configure() updates the config and logs the change', async () => {
      const svc = await loadService({ gtmEnabled: true, dev: true })

      svc.configure({ enableErrorTracking: false })

      expect(svc.getConfig().enableErrorTracking).toBe(false)
      expect(consoleLog).toHaveBeenCalledWith(
        '[Analytics] Configuration updated',
        expect.any(Object)
      )
    })

    it('warns instead of dispatching when tracking is disabled', async () => {
      const gtmTrack = vi.fn()
      const svc = await loadService({ gtmEnabled: false, dev: true, gtmTrack })

      svc.track(buildPageViewEvent())

      expect(consoleWarn).toHaveBeenCalledWith(
        '[Analytics] Tracking disabled or GTM not available'
      )
      expect(gtmTrack).not.toHaveBeenCalled()
    })

    it('warns when user properties are set before identify()', async () => {
      const svc = await loadService({ gtmEnabled: true, dev: true })

      svc.setUserProperties({ name: 'Nobody' })

      expect(consoleWarn).toHaveBeenCalledWith(
        '[Analytics] Cannot update user properties - user not identified'
      )
    })
  })
})
