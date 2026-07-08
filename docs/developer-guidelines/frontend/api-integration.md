# Frontend API integration

How the SPA talks to the backend. **The OpenAPI spec is the single source of
truth.** The backend `openapi.yaml` is published as the generated
`@vibexp/api-client` npm package; the frontend consumes its types and a typed
`openapi-fetch` client. There is no hand-written HTTP client and no hand-written
wire types anymore (retired in #94, epic #87).

If you are adding or changing an API call, follow the pattern below. A
`no-restricted-imports` ESLint rule blocks the old plumbing, so the compiler and
linter will point you here.

## The pattern (a.k.a. the `notificationService` pattern)

Each domain gets one service module under `src/services/`. It does three things:

1. **Re-exports the wire types from the generated schema** so components import
   them from the service, not from a hand-written type file:

   ```ts
   import type { components, operations } from '@vibexp/api-client'

   export type Prompt = components['schemas']['Prompt']
   export type PromptListResponse = components['schemas']['PromptListResponse']
   // Query-param bags come off `operations`, wrapped in NonNullable:
   export type ListPromptsQuery = NonNullable<
     operations['listPrompts']['parameters']['query']
   >
   ```

2. **Calls `generatedClient` and resolves with `unwrap`** (both from
   `@/lib/apiClientGenerated`). `generatedClient` is the typed `openapi-fetch`
   client; `unwrap` returns the typed payload or throws an `ApiError` (the same
   error type the whole app already handles — RFC 9457 problem details, with
   `.status`/`.code`).

   ```ts
   import { generatedClient, unwrap } from '@/lib/apiClientGenerated'

   async getPrompt(teamId: string, slug: string): Promise<Prompt> {
     return unwrap(
       generatedClient.GET('/api/v1/{team_id}/prompts/{slug}', {
         params: { path: { team_id: teamId, slug } },
       })
     )
   }
   ```

   Paths are the full spec paths (they include `/api/v1/...`). Path params are
   percent-encoded by openapi-fetch — do **not** call `encodeURIComponent`
   yourself. The query serializer sends every non-`undefined` value, including
   empty strings.

3. **Deletes any hand-written `src/types/<domain>.ts`** for wire shapes. Import
   the domain's types from its service instead.

## The envelope rule (important)

The backend's `writeJSON` writes the value **raw** — it does not auto-wrap. So
whether a response is enveloped depends on the handler:

- **Raw payload** (`writeOK(w, obj)`) → the generated response type **is** the
  object. `unwrap(...)` returns it directly.
- **Enveloped list/paginated** (`writeOK(w, {status, message, data})`) → the
  generated type is `SuccessResponse & { data: X }` (or a named `*Envelope`
  schema). Do `(await unwrap(...)).data`.

Check the operation's `responses.200.content['application/json']` in the
generated `schema.d.ts` to know which. Don't assume.

## Recipes

- **Multipart upload:** pass the typed body (the spec types the binary part as
  `string`, so pass the `File` with a narrow cast) plus a `bodySerializer` that
  returns `FormData`. openapi-fetch then drops its JSON `Content-Type` so the
  browser sets the multipart boundary. See `attachmentService.upload`.
- **Blob download:** stays a thin documented `fetch` wrapper — `unwrap` resolves
  JSON payloads only. See `attachmentService.download`.
- **Local aliases to avoid churn:** when a generated schema rename would touch
  dozens of call sites, alias it locally in the service
  (`export type TeamMember = components['schemas']['TeamMemberDetail']`) rather
  than editing every importer.
- **Cancellation + timeout:** `generatedClient` combines the caller's
  `AbortSignal` with a 30s timeout via `AbortSignal.any`, so passing
  `{ signal }` still cancels in-flight requests.

## What is NOT generated (intentional exceptions)

- **UI-only types** stay hand-written under `src/types/`, imported by direct
  path: `alert.ts`, `analytics.ts`, `help.ts`, `a2a.ts`, and the `ApiError`
  class + `APIErrorResponse` in `errors.ts`. These have no wire surface.
- **`version.ts`** re-exports the generated `ContentVersion` and keeps a few
  resource-agnostic aliases; import it directly (`@/types/version`).
- **The OAuth consent surface** (`oauthService` + `types/oauth.ts`) is
  deliberately hand-written. Its endpoints (`/oauth/consent[/attach]`) are
  served by the embedded Authorization Server and are kept **out** of
  `openapi.yaml` on purpose (documenting them would break the spec drift +
  payload-coverage gates — see #89 / #34). `oauthService` therefore uses a
  tiny, self-contained local `fetch` wrapper that still throws `ApiError`. This
  is the one sanctioned exception; do not grow it into a second general client.

## Guardrail

`eslint.config.js` has a `no-restricted-imports` rule that blocks:

- `@/lib/apiClient` / `**/lib/apiClient` — the removed hand-written client
- `@/types` — the removed types barrel (import UI-only types by direct path)
- `@/types/api` / `**/types/api` — the removed `ApiResponse<T>` envelope

If you hit it, you're reaching for retired plumbing — use `generatedClient` +
`unwrap` and re-export wire types from your service instead.
