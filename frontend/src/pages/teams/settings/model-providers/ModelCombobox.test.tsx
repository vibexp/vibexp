import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ModelCombobox } from './ModelCombobox'

// cmdk needs browser APIs jsdom does not provide.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

// A 300-entry list, the size OpenRouter returns (#1076).
const manyModels = Array.from({ length: 300 }, (_, i) => ({
  id: `vendor/model-${String(i).padStart(3, '0')}`,
}))

const renderCombobox = (
  props: Partial<Parameters<typeof ModelCombobox>[0]> = {}
) => {
  const onChange = vi.fn()
  render(
    <ModelCombobox
      value=""
      onChange={onChange}
      models={manyModels}
      {...props}
    />
  )
  return { onChange }
}

it('renders a real trigger button showing the current value', () => {
  renderCombobox({ value: 'gpt-4o-mini' })

  const trigger = screen.getByTestId('model-combobox')
  expect(trigger.tagName).toBe('BUTTON')
  expect(trigger).toHaveAttribute('role', 'combobox')
  expect(trigger).toHaveTextContent('gpt-4o-mini')
})

it('offers every listed model without truncating the list', async () => {
  const user = userEvent.setup()
  renderCombobox()

  await user.click(screen.getByTestId('model-combobox'))

  expect(await screen.findAllByTestId('model-option')).toHaveLength(300)
})

it('filters client-side as the user types and picks a match', async () => {
  const user = userEvent.setup()
  const { onChange } = renderCombobox()

  await user.click(screen.getByTestId('model-combobox'))
  await user.type(
    await screen.findByPlaceholderText('Search models…'),
    'model-299'
  )

  const options = screen.getAllByTestId('model-option')
  expect(options).toHaveLength(1)
  await user.click(options[0])

  expect(onChange).toHaveBeenCalledWith('vendor/model-299')
})

it('shows owned_by as secondary text', async () => {
  const user = userEvent.setup()
  renderCombobox({ models: [{ id: 'gpt-4o', owned_by: 'openai' }] })

  await user.click(screen.getByTestId('model-combobox'))

  expect(await screen.findByText('openai')).toBeInTheDocument()
})

it('lets a typed id that is not in the list be used', async () => {
  const user = userEvent.setup()
  const { onChange } = renderCombobox({ models: [{ id: 'gpt-4o' }] })

  await user.click(screen.getByTestId('model-combobox'))
  await user.type(
    await screen.findByPlaceholderText('Search models…'),
    'my-finetune'
  )
  await user.click(screen.getByTestId('model-option-custom'))

  expect(onChange).toHaveBeenCalledWith('my-finetune')
})

it('does not offer the custom entry when the typed id is listed', async () => {
  const user = userEvent.setup()
  renderCombobox({ models: [{ id: 'gpt-4o' }] })

  await user.click(screen.getByTestId('model-combobox'))
  await user.type(
    await screen.findByPlaceholderText('Search models…'),
    'gpt-4o'
  )

  expect(screen.queryByTestId('model-option-custom')).not.toBeInTheDocument()
})
