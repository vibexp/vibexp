import {
  emitInstanceEmailChanged,
  onInstanceEmailChanged,
} from '../instanceEmailEvents'

describe('instanceEmailEvents', () => {
  it('delivers an emitted change to every subscriber until it unsubscribes', () => {
    const first = vi.fn()
    const second = vi.fn()
    const offFirst = onInstanceEmailChanged(first)
    const offSecond = onInstanceEmailChanged(second)

    emitInstanceEmailChanged()
    offFirst()
    emitInstanceEmailChanged()
    offSecond()

    expect(first).toHaveBeenCalledTimes(1)
    expect(second).toHaveBeenCalledTimes(2)
  })

  it('keeps notifying the others when one listener throws', () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    const offBad = onInstanceEmailChanged(() => {
      throw new Error('bad listener')
    })
    const good = vi.fn()
    const offGood = onInstanceEmailChanged(good)

    emitInstanceEmailChanged()
    offBad()
    offGood()

    expect(good).toHaveBeenCalledTimes(1)
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
