import { formatDate, formatDateTime, formatRelativeTime } from '../time'

describe('formatDate', () => {
  it('returns "Never" for null', () => {
    expect(formatDate(null)).toBe('Never')
  })

  it('returns "Never" for undefined', () => {
    expect(formatDate(undefined)).toBe('Never')
  })

  it('formats a date string as short date', () => {
    const result = formatDate('2024-03-15T10:00:00Z')
    expect(result).toMatch(/Mar/)
    expect(result).toMatch(/15/)
    expect(result).toMatch(/2024/)
  })

  it('accepts a Date object', () => {
    const date = new Date('2024-06-01T00:00:00Z')
    const result = formatDate(date)
    expect(result).toMatch(/2024/)
  })
})

describe('formatDateTime', () => {
  it('returns "Never" for null', () => {
    expect(formatDateTime(null)).toBe('Never')
  })

  it('returns "Never" for undefined', () => {
    expect(formatDateTime(undefined)).toBe('Never')
  })

  it('formats a date string with long month, day, year, hour, and minute', () => {
    const result = formatDateTime('2024-01-01T12:00:00Z')
    expect(result).toMatch(/January/)
    expect(result).toMatch(/2024/)
    // Should include time (12:00 or similar)
    expect(result).toMatch(/\d{1,2}:\d{2}/)
  })

  it('accepts a Date object', () => {
    const date = new Date('2024-06-15T00:00:00Z')
    const result = formatDateTime(date)
    expect(result).toMatch(/2024/)
    expect(result).toMatch(/June|Jul/) // UTC vs local timezone may shift
  })

  it('includes AM/PM indicator', () => {
    const result = formatDateTime('2024-03-15T10:00:00Z')
    expect(result).toMatch(/AM|PM/)
  })
})

describe('formatRelativeTime', () => {
  beforeEach(() => {
    jest.useFakeTimers()
    jest.setSystemTime(new Date('2024-06-01T12:00:00Z'))
  })

  afterEach(() => {
    jest.useRealTimers()
  })

  it('returns "Never" for null', () => {
    expect(formatRelativeTime(null)).toBe('Never')
  })

  it('returns "Never" for undefined', () => {
    expect(formatRelativeTime(undefined)).toBe('Never')
  })

  it('returns "just now" for dates within the last minute', () => {
    const recent = new Date('2024-06-01T11:59:30Z').toISOString()
    expect(formatRelativeTime(recent)).toBe('just now')
  })

  it('returns minutes ago for dates within the last hour', () => {
    const fifteenMinsAgo = new Date('2024-06-01T11:45:00Z').toISOString()
    expect(formatRelativeTime(fifteenMinsAgo)).toBe('15m ago')
  })

  it('returns hours ago for dates within the last day', () => {
    const threeHoursAgo = new Date('2024-06-01T09:00:00Z').toISOString()
    expect(formatRelativeTime(threeHoursAgo)).toBe('3h ago')
  })

  it('returns days ago for dates within the last week', () => {
    const twoDaysAgo = new Date('2024-05-30T12:00:00Z').toISOString()
    expect(formatRelativeTime(twoDaysAgo)).toBe('2d ago')
  })

  it('returns formatted date for dates older than a week', () => {
    const oldDate = new Date('2024-01-01T00:00:00Z').toISOString()
    const result = formatRelativeTime(oldDate)
    expect(result).toMatch(/2024/)
    expect(result).toMatch(/Jan/)
  })

  it('accepts a Date object', () => {
    const recent = new Date('2024-06-01T11:59:50Z')
    expect(formatRelativeTime(recent)).toBe('just now')
  })

  it('handles future dates gracefully (returns "just now" or formatted)', () => {
    const future = new Date('2024-06-01T13:00:00Z').toISOString()
    // diffMs is negative → seconds < 60 → "just now"
    expect(formatRelativeTime(future)).toBe('just now')
  })
})
