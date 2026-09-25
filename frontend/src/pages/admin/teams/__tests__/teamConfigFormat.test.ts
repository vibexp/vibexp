import {
  displayValue,
  emailStatusMeta,
  formatSender,
  shortId,
  sourceLabel,
} from '../detail/teamConfigFormat'

it('labels the config source', () => {
  expect(sourceLabel('instance')).toBe('Inherited from instance')
  expect(sourceLabel('team')).toBe('Team')
})

it('maps every email status to a badge', () => {
  expect(emailStatusMeta('healthy')).toEqual({
    label: 'Healthy',
    variant: 'secondary',
  })
  expect(emailStatusMeta('failing')).toEqual({
    label: 'Failing',
    variant: 'destructive',
  })
  expect(emailStatusMeta('unknown')).toEqual({
    label: 'Unknown',
    variant: 'outline',
  })
})

it('shortens ids to 8 characters', () => {
  expect(shortId('3f2a9c1e-0000-4000-8000-000000000001')).toBe('3f2a9c1e')
})

it('renders missing values as a dash', () => {
  expect(displayValue(null)).toBe('—')
  expect(displayValue(undefined)).toBe('—')
  expect(displayValue('  ')).toBe('—')
  expect(displayValue(0)).toBe('0')
  expect(displayValue('x')).toBe('x')
})

it('formats a sender from whichever parts are set', () => {
  expect(formatSender('Team', 'a@example.com')).toBe('Team <a@example.com>')
  expect(formatSender(null, 'a@example.com')).toBe('a@example.com')
  expect(formatSender('Team', null)).toBe('Team')
  expect(formatSender(null, null)).toBe('—')
})
