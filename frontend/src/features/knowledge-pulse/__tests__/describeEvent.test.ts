import { describe, expect, it } from 'vitest'

import { describeEvent, shortAge } from '../describeEvent'
import { pulseEvent } from './fixtures'

describe('describeEvent', () => {
  it('reads name the source', () => {
    expect(describeEvent(pulseEvent({ id: 'e' }))).toEqual({
      verb: 'read',
      object: 'Deploy runbook',
      suffix: 'via MCP',
    })
  })

  it('writes say created or updated', () => {
    const create = pulseEvent({
      id: 'e',
      kind: 'write',
      detail: { action: 'create' },
      actor: { type: 'user', id: 'u1', name: 'Ada', source: 'web' },
    })
    expect(describeEvent(create)).toMatchObject({
      verb: 'created',
      suffix: 'via the web',
    })
    expect(
      describeEvent({ ...create, detail: { action: 'update' } }).verb
    ).toBe('updated')
  })

  it('links name both ends and the relation', () => {
    const link = pulseEvent({
      id: 'e',
      kind: 'link',
      detail: {
        relation_type: 'built-from',
        to: { type: 'artifact', id: 'a1', title: 'Spec' },
      },
    })
    expect(describeEvent(link)).toEqual({
      verb: 'linked',
      object: 'Deploy runbook',
      target: 'Spec',
      suffix: 'built-from',
    })
    expect(describeEvent({ ...link, detail: {} }).target).toBe('another item')
  })

  it('feed posts go to the feed; an untitled resource falls back to its type', () => {
    const post = pulseEvent({
      id: 'e',
      kind: 'feed_post',
      resource: { type: 'feed_item', id: 'f1', title: '' },
    })
    expect(describeEvent(post)).toEqual({
      verb: 'posted',
      object: 'a feed post',
      suffix: 'to the feed',
    })
  })
})

describe('shortAge', () => {
  const now = Date.parse('2026-10-19T12:00:00Z')
  it.each([
    ['2026-10-19T11:59:55Z', '5s'],
    ['2026-10-19T11:57:00Z', '3m'],
    ['2026-10-19T09:00:00Z', '3h'],
    ['2026-10-19T12:00:09Z', '0s'],
  ])('%s → %s', (at, want) => {
    expect(shortAge(at, now)).toBe(want)
  })
})
