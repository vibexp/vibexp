import { formatBytes } from '@/pages/admin/dashboard/format'

it('reports sub-kibibyte sizes in bytes', () => {
  expect(formatBytes(0)).toBe('0 B')
  expect(formatBytes(1023)).toBe('1023 B')
})

it('steps up through the 1024-based units', () => {
  expect(formatBytes(1024)).toBe('1.0 KiB')
  expect(formatBytes(184549376)).toBe('176 MiB')
  expect(formatBytes(1024 ** 3)).toBe('1.0 GiB')
  expect(formatBytes(1024 ** 4)).toBe('1.0 TiB')
})

it('stops at the largest unit rather than inventing one', () => {
  // 5 PiB has no unit in the list; it must not fall off the end of the array.
  expect(formatBytes(1024 ** 5 * 5)).toBe('5120 TiB')
})

it('keeps a decimal below 10 and drops it above', () => {
  // "1.5 MiB" rounded to "2 MiB" would misreport by a third.
  expect(formatBytes(1024 * 1024 * 1.5)).toBe('1.5 MiB')
  expect(formatBytes(1024 * 1024 * 42)).toBe('42 MiB')
})
