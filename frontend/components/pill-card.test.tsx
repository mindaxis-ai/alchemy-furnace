import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PillCard } from '@/components/pill-card'
import type { Pill } from '@/services/types'

const push = vi.hoisted(() => vi.fn())

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}))

// key 透传：卡片文案断言只关心键/角色，不依赖具体语言
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

const customPill: Pill = {
  id: 'pill-custom-1',
  name: '丹心妙语',
  description: '温润如茶',
  skill_schema: {},
  tags: ['古风', '炼丹'],
  author: '太上老君',
  version: '2.1.0',
  is_builtin: false,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const builtinPill: Pill = { ...customPill, id: 'pill-builtin-1', is_builtin: true }

describe('PillCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('整卡是单一键盘可访问导航容器,点击进入详情', async () => {
    const user = userEvent.setup()
    render(<PillCard pill={customPill} />)

    // 单卡仅一个导航角色(名称与 chevron 不再是各自独立的链接)
    expect(screen.getAllByRole('link')).toHaveLength(1)

    await user.click(screen.getByRole('link', { name: '丹心妙语' }))
    expect(push).toHaveBeenCalledTimes(1)
    expect(push).toHaveBeenCalledWith('/pills/pill-custom-1')
  })

  it('键盘 Enter 与 Space 均可触发导航', () => {
    render(<PillCard pill={customPill} />)
    const nav = screen.getByRole('link', { name: '丹心妙语' })

    fireEvent.keyDown(nav, { key: 'Enter' })
    expect(push).toHaveBeenCalledWith('/pills/pill-custom-1')

    fireEvent.keyDown(nav, { key: ' ' })
    expect(push).toHaveBeenCalledTimes(2)
  })

  it('内部赠予按钮阻止冒泡,不触发卡片导航(无双导航)', async () => {
    const onBind = vi.fn()
    const user = userEvent.setup()
    render(<PillCard pill={customPill} onBind={onBind} />)

    await user.click(screen.getByRole('button', { name: 'bestowCta' }))
    expect(onBind).toHaveBeenCalledTimes(1)
    expect(onBind).toHaveBeenCalledWith(customPill)
    expect(push).not.toHaveBeenCalled()
  })

  it('内置金丹显示内置徽标', () => {
    render(<PillCard pill={builtinPill} />)
    expect(screen.getByText('builtInBadge')).toBeInTheDocument()
  })

  it('无描述时显示占位文案', () => {
    render(<PillCard pill={{ ...customPill, description: undefined }} />)
    expect(screen.getByText('noDescription')).toBeInTheDocument()
  })
})
