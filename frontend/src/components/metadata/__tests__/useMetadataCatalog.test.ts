import { act, renderHook, waitFor } from '@testing-library/react'

import { useTeam } from '@/contexts/TeamContext'
import { metadataService } from '@/services/metadataService'

import { useMetadataCatalog } from '../useMetadataCatalog'

jest.mock('@/contexts/TeamContext')
jest.mock('@/services/metadataService')

const mockedUseTeam = useTeam as jest.MockedFunction<typeof useTeam>
const mockedListKeys = metadataService.listKeys as jest.MockedFunction<
  typeof metadataService.listKeys
>
const mockedListValues = metadataService.listValues as jest.MockedFunction<
  typeof metadataService.listValues
>

const setTeam = (): void => {
  mockedUseTeam.mockReturnValue({
    currentTeam: { id: 'team-1', name: 'Team One', slug: 'team-one' },
    teams: [],
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
    isLoading: false,
  } as unknown as ReturnType<typeof useTeam>)
}

const renderCatalog = () =>
  renderHook(() => useMetadataCatalog({ resourceType: 'blueprints' }))

describe('useMetadataCatalog', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    setTeam()
    mockedListKeys.mockResolvedValue({
      keys: ['env', 'team'],
      truncated: false,
    })
    mockedListValues.mockResolvedValue({ values: ['prod'], truncated: false })
  })

  afterEach(() => {
    jest.runOnlyPendingTimers()
    jest.useRealTimers()
  })

  test('fetches nothing until loadKeys is called', () => {
    renderCatalog()

    // Lazy by design: the popover has not been opened yet.
    expect(mockedListKeys).not.toHaveBeenCalled()
  })

  test('loadKeys requests the key catalog once', async () => {
    const { result } = renderCatalog()

    act(() => {
      result.current.loadKeys()
    })

    await waitFor(() => {
      expect(result.current.keys).toEqual(['env', 'team'])
    })
    expect(mockedListKeys).toHaveBeenCalledTimes(1)
    expect(mockedListKeys).toHaveBeenCalledWith('team-1', {
      resource_type: 'blueprints',
      limit: 100,
    })
  })

  test('selecting a key loads its values after the debounce', async () => {
    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    expect(mockedListValues).not.toHaveBeenCalled()

    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    expect(mockedListValues).toHaveBeenCalledTimes(1)
    expect(mockedListValues).toHaveBeenCalledWith('team-1', {
      resource_type: 'blueprints',
      key: 'env',
      limit: 100,
    })
  })

  test('rapid typing produces one request, not one per keystroke', async () => {
    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    act(() => {
      result.current.setValueQuery('p')
    })
    act(() => {
      result.current.setValueQuery('pr')
    })
    act(() => {
      result.current.setValueQuery('pro')
    })

    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    expect(mockedListValues).toHaveBeenCalledTimes(1)
    expect(mockedListValues).toHaveBeenCalledWith('team-1', {
      resource_type: 'blueprints',
      key: 'env',
      q: 'pro',
      limit: 100,
    })
  })

  test('a stale in-flight response is discarded', async () => {
    let resolveOlder: (value: {
      values: string[]
      truncated: boolean
    }) => void = () => {}
    mockedListValues.mockReturnValueOnce(
      new Promise(resolve => {
        resolveOlder = resolve
      })
    )
    mockedListValues.mockResolvedValueOnce({
      values: ['newer'],
      truncated: false,
    })

    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    // A second query supersedes the first while it is still in flight.
    act(() => {
      result.current.setValueQuery('newer')
    })
    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    await act(async () => {
      resolveOlder({ values: ['stale'], truncated: false })
      await Promise.resolve()
    })

    expect(result.current.values).toEqual(['newer'])
  })

  test('truncated is surfaced from the values response', async () => {
    mockedListValues.mockResolvedValue({ values: ['prod'], truncated: true })
    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(result.current.valuesTruncated).toBe(true)
    })
  })

  test('selecting a new key resets the previous search and values', async () => {
    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    act(() => {
      result.current.setValueQuery('pro')
    })
    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    act(() => {
      result.current.selectKey('team')
    })

    expect(result.current.valueQuery).toBe('')
    expect(result.current.values).toEqual([])
  })

  test('a failed key request surfaces an error instead of stale keys', async () => {
    mockedListKeys.mockRejectedValue(new Error('boom'))
    const { result } = renderCatalog()

    act(() => {
      result.current.loadKeys()
    })

    await waitFor(() => {
      expect(result.current.keysError).toBe('Failed to load metadata keys')
    })
    expect(result.current.keys).toEqual([])
  })

  test('a failed values request surfaces an error', async () => {
    mockedListValues.mockRejectedValue(new Error('boom'))
    const { result } = renderCatalog()

    act(() => {
      result.current.selectKey('env')
    })
    await act(async () => {
      jest.advanceTimersByTime(300)
      await Promise.resolve()
    })

    await waitFor(() => {
      expect(result.current.valuesError).toBe('Failed to load metadata values')
    })
  })

  test('no request is made without a current team', () => {
    mockedUseTeam.mockReturnValue({
      currentTeam: null,
      teams: [],
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn(),
      isLoading: false,
    })

    const { result } = renderCatalog()
    act(() => {
      result.current.loadKeys()
    })

    expect(mockedListKeys).not.toHaveBeenCalled()
  })
})
