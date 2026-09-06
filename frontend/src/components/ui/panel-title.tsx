/**
 * `PanelTitle` moved to `./panel` (#890), where it sits beside the rest of the
 * panel primitives and reads the surface's `PanelPresentation` so it sizes
 * itself (16px on a card, 14px flat in the reading page's details column).
 *
 * Re-exported here so the existing import path keeps working.
 */
export { PanelTitle } from './panel'
