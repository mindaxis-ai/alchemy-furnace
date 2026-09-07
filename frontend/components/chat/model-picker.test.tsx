import { useState } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { ModelPicker } from './model-picker'

vi.mock('next-intl', () => ({ useTranslations: () => (key: string) => key }))
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

function Picker({ disabled = false }: { disabled?: boolean }) {
  const [value, setValue] = useState('')
  return <ModelPicker value={value} onChange={setValue} disabled={disabled} />
}

it('selects an enabled model and can restore agent defaults', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 0, data: [
    { name: 'alternate', display_name: 'Alternate', provider_display_name: 'Local', provider_name: 'local', is_default: false },
  ] }))))
  render(<Picker />)
  await screen.findByRole('option', { name: 'Alternate · Local' })
  const user = userEvent.setup()
  await user.selectOptions(screen.getByRole('combobox'), 'alternate')
  expect(screen.getByRole('combobox')).toHaveValue('alternate')
  await user.selectOptions(screen.getByRole('combobox'), '')
  expect(screen.getByRole('combobox')).toHaveValue('')
})

it('keeps sending available when loading models fails and supports a retry', async () => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(new Response(JSON.stringify({ code: 0, data: [] }))))
  render(<Picker />)
  await userEvent.click(await screen.findByRole('button', { name: 'retry' }))
  await screen.findByRole('option', { name: 'agentDefault' })
  expect(screen.getByRole('combobox')).toBeEnabled()
})

it('prevents changing models during a running response', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 0, data: [] }))))
  render(<Picker disabled />)
  await screen.findByRole('option', { name: 'agentDefault' })
  expect(screen.getByRole('combobox')).toBeDisabled()
})
