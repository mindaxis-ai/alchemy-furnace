import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LocaleProvider, LOCALE_STORAGE_KEY } from '@/components/i18n/locale-provider'

const captured = vi.hoisted(() => ({ locales: [] as string[] }))

vi.mock('next-intl', () => ({
  NextIntlClientProvider: (props: { locale: string; children: React.ReactNode }) => {
    captured.locales.push(props.locale)
    return <>{props.children}</>
  },
}))

describe('LocaleProvider', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    captured.locales.length = 0
    window.localStorage.clear()
  })

  it('applies the stored locale when preferStored is set', () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'en')
    render(
      <LocaleProvider initialLocale="zh-CN" preferStored>
        <div>child</div>
      </LocaleProvider>,
    )

    expect(screen.getByText('child')).toBeInTheDocument()
    expect(captured.locales.at(-1)).toBe('en')
  })

  it('keeps the initial locale when nothing is stored', () => {
    render(
      <LocaleProvider initialLocale="zh-CN" preferStored>
        <div>child</div>
      </LocaleProvider>,
    )

    expect(captured.locales.at(-1)).toBe('zh-CN')
  })

  it('ignores the stored locale when preferStored is false (locale-prefixed routes)', () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'en')
    render(
      <LocaleProvider initialLocale="zh-CN" preferStored={false}>
        <div>child</div>
      </LocaleProvider>,
    )

    expect(captured.locales.at(-1)).toBe('zh-CN')
  })

  it('ignores invalid stored values', () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'fr-FR')
    render(
      <LocaleProvider initialLocale="zh-CN" preferStored>
        <div>child</div>
      </LocaleProvider>,
    )

    expect(captured.locales.at(-1)).toBe('zh-CN')
  })
})
