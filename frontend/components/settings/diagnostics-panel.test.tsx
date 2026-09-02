import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DiagnosticsPanel } from '@/components/settings/diagnostics-panel'
import { PROMPT_DEBUG_STORAGE_KEY } from '@/lib/prompt-debug-pref'

vi.mock('next-intl', () => ({ useTranslations: () => (key: string) => key }))
vi.mock('@/services/api', () => ({ isDesktop: () => true }))
vi.mock('@/services/diagnosticsService', () => ({
  getDiagnostics: vi.fn().mockResolvedValue({
    timestamp: 1,
    log_dir: '/logs',
    app_log: '/logs/app.log',
    python_log: '/logs/python.log',
    python_engine: 'ok',
  }),
}))
vi.mock('@/lib/diagnostics/recent-api-failures', () => ({ listApiFailures: () => [] }))

describe('DiagnosticsPanel prompt debug control', () => {
  beforeEach(() => {
    const values = new Map<string, string>()
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
        removeItem: (key: string) => values.delete(key),
        clear: () => values.clear(),
      },
    })
  })
  afterEach(() => cleanup())

  it('starts off and persists the opt-in toggle', async () => {
    const user = userEvent.setup()
    render(<DiagnosticsPanel />)

    const toggle = await waitFor(() => screen.getByRole('switch', { name: 'promptDebugLabel' }))
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(toggle.querySelector('span')).toHaveClass('left-0.5', 'h-5', 'w-5')

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(window.localStorage.getItem(PROMPT_DEBUG_STORAGE_KEY)).toBe('true')
  })
})
