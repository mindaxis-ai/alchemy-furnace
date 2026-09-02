import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  PROMPT_DEBUG_CHANGE_EVENT,
  PROMPT_DEBUG_STORAGE_KEY,
  getPromptDebugEnabled,
  setPromptDebugEnabled,
} from '@/lib/prompt-debug-pref'

describe('prompt debug preference', () => {
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

  it('is disabled by default and persists explicit changes', () => {
    expect(getPromptDebugEnabled()).toBe(false)

    setPromptDebugEnabled(true)

    expect(getPromptDebugEnabled()).toBe(true)
    expect(window.localStorage.getItem(PROMPT_DEBUG_STORAGE_KEY)).toBe('true')
  })

  it('notifies same-tab subscribers', () => {
    const listener = vi.fn()
    window.addEventListener(PROMPT_DEBUG_CHANGE_EVENT, listener)

    setPromptDebugEnabled(true)

    expect(listener).toHaveBeenCalledTimes(1)
    window.removeEventListener(PROMPT_DEBUG_CHANGE_EVENT, listener)
  })
})
