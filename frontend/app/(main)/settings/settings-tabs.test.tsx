import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SettingsTabs } from './settings-tabs'

const REPO_URL = 'https://github.com/mindaxis-ai/alchemy-furnace'

// 桌面模式点击 GitHub 链接 → 拦截默认行为,经 /desktop/open-url 交系统浏览器;
// web 模式保持原生 target=_blank 行为。isDesktop 读 document 的 is-desktop class
// (api.ts),jsdom 直接加 class 即可模拟桌面环境。
const td = vi.hoisted(() => ({
  replace: vi.fn(),
  params: new URLSearchParams('tab=about'),
  getVersion: vi.fn(),
  openExternalUrl: vi.fn(),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: td.replace }),
  useSearchParams: () => td.params,
}))

vi.mock('next-intl', () => ({
  useTranslations: () => {
    const t = (key: string) => key
    // AboutPanel 的 techList 走 t.raw(),mock 补上
    return Object.assign(t, { raw: (key: string) => key === 'techList'
      ? ['Wails 2', 'Go 1.25 · Gin · GORM', 'Python · FastAPI · LangGraph', 'Next.js 16 · React 19', 'TypeScript 5.7 · Tailwind CSS 4', 'SQLite']
      : [key] })
  },
}))

vi.mock('@/services/systemService', () => ({
  getVersion: td.getVersion,
  openExternalUrl: td.openExternalUrl,
}))

describe('SettingsTabs about panel external link', () => {
  afterEach(() => {
    cleanup()
    document.documentElement.classList.remove('is-desktop')
  })

  beforeEach(() => {
    vi.resetAllMocks()
    td.params = new URLSearchParams('tab=about')
    td.getVersion.mockResolvedValue({ version: '1.0.0', commit: 'abc', buildDate: '2026-08-31' })
    td.openExternalUrl.mockResolvedValue(undefined)
  })

  it('keeps the GitHub link as a plain anchor with target=_blank in web mode', async () => {
    render(<SettingsTabs />)
    const link = screen.getByText('githubRepo').closest('a')
    expect(link).toHaveAttribute('href', REPO_URL)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('does not call the external bridge when clicked in web mode', async () => {
    render(<SettingsTabs />)
    await screen.findByText('githubRepo')
    fireEvent.click(screen.getByText('githubRepo'))
    expect(td.openExternalUrl).not.toHaveBeenCalled()
  })

  it('routes the GitHub link through the system browser when clicked in desktop mode', async () => {
    document.documentElement.classList.add('is-desktop')
    render(<SettingsTabs />)
    await screen.findByText('githubRepo')
    fireEvent.click(screen.getByText('githubRepo'))
    expect(td.openExternalUrl).toHaveBeenCalledTimes(1)
    expect(td.openExternalUrl).toHaveBeenCalledWith(REPO_URL)
  })

  it('shows the current desktop stack', () => {
    render(<SettingsTabs />)
    expect(screen.getByText('Wails 2')).toBeInTheDocument()
    expect(screen.getByText('Python · FastAPI · LangGraph')).toBeInTheDocument()
    expect(screen.getByText('SQLite')).toBeInTheDocument()
    expect(screen.queryByText('PostgreSQL')).toBeNull()
  })
})
