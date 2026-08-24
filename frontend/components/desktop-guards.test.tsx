import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DesktopGuards from './desktop-guards'

const push = vi.hoisted(() => vi.fn())
const isDesktopMock = vi.hoisted(() => vi.fn())

vi.mock('next/navigation', () => ({ useRouter: () => ({ push }) }))
vi.mock('@/services/api', () => ({ isDesktop: () => isDesktopMock() }))

describe('DesktopGuards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    isDesktopMock.mockReturnValue(true)
  })
  afterEach(() => cleanup())

  it('桌面端按 ⌘, 用 router.push 进设置页（非 window.location 硬导航）', () => {
    render(<DesktopGuards />)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: ',', metaKey: true }))
    expect(push).toHaveBeenCalledWith('/settings')
  })

  it('桌面端按 ⌘N 派发 alchemy:new-session 自定义事件', () => {
    const onNew = vi.fn()
    window.addEventListener('alchemy:new-session', onNew)
    render(<DesktopGuards />)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true }))
    expect(onNew).toHaveBeenCalledTimes(1)
    window.removeEventListener('alchemy:new-session', onNew)
  })

  it('非桌面端不挂监听（按键不触发任何导航）', () => {
    isDesktopMock.mockReturnValue(false)
    render(<DesktopGuards />)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: ',', metaKey: true }))
    expect(push).not.toHaveBeenCalled()
  })
})
