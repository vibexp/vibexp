/**
 * Hard browser navigation (a full page load to `url`), wrapped in a function so
 * it can be mocked in tests — jsdom's `window.location` is read-only and cannot
 * be spied on directly. Use this for leaving the SPA entirely (e.g. handing the
 * browser to an OAuth client's callback); use react-router for in-app navigation.
 */
export function hardRedirect(url: string): void {
  window.location.assign(url)
}
