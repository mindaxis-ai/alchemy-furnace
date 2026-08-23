/* eslint-disable @next/next/no-html-link-for-pages --
 * 该 harness 故意使用原生 <a href> 触发真实锚点点击,以验证 useUnsavedChanges 对站内导航的拦截;
 * 换成 next/link 的 <Link> 需额外路由上下文,且违背"测原生拦截"的本意。 */
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useUnsavedChanges } from '@/hooks/use-unsaved-changes'

const MESSAGE = '当前有未保存的修改，确定要离开吗？'

function Harness({ active }: { active: boolean }) {
  useUnsavedChanges(active, MESSAGE)
  return (
    <div>
      <a href="/pills">站内返回</a>
      <a href="https://example.com">外部链接</a>
      <button type="button">普通按钮</button>
    </div>
  )
}

describe('useUnsavedChanges', () => {
  let confirmSpy: ReturnType<typeof vi.fn>
  // 文档冒泡探针：hook 的 capture 处理器若 stopPropagation(取消导航)，探针就不会触发；
  // 若放行，探针触发并顺手 preventDefault，压住 jsdom 的 "navigation not implemented" 噪音。
  let probe: ReturnType<typeof vi.fn>

  beforeEach(() => {
    confirmSpy = vi.fn()
    vi.stubGlobal('confirm', confirmSpy)
    probe = vi.fn((event: Event) => (event as MouseEvent).preventDefault())
    // vi.fn 的构造签名使类型不直接匹配 EventListener,但运行期签名 (event)=>void 兼容
    document.addEventListener('click', probe as unknown as EventListener)
  })

  afterEach(() => {
    cleanup()
    document.removeEventListener('click', probe as unknown as EventListener)
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('激活时 beforeunload 被阻止(触发浏览器原生确认)', () => {
    render(<Harness active />)
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
  })

  it('未激活时 beforeunload 不被阻止', () => {
    render(<Harness active={false} />)
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
  })

  it('激活时点击站内链接,确认后允许离开(不阻止冒泡)', () => {
    confirmSpy.mockReturnValue(true)
    render(<Harness active />)
    fireEvent.click(screen.getByRole('link', { name: '站内返回' }))
    expect(confirmSpy).toHaveBeenCalledWith(MESSAGE)
    expect(probe).toHaveBeenCalled()
  })

  it('激活时点击站内链接,取消则阻止离开(stopPropagation)', () => {
    confirmSpy.mockReturnValue(false)
    render(<Harness active />)
    fireEvent.click(screen.getByRole('link', { name: '站内返回' }))
    expect(confirmSpy).toHaveBeenCalledWith(MESSAGE)
    expect(probe).not.toHaveBeenCalled()
  })

  it('站内拦截不波及外部链接与普通元素', () => {
    confirmSpy.mockReturnValue(false)
    render(<Harness active />)

    fireEvent.click(screen.getByRole('link', { name: '外部链接' }))
    expect(confirmSpy).not.toHaveBeenCalled()
    expect(probe).toHaveBeenCalled()

    probe.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '普通按钮' }))
    expect(confirmSpy).not.toHaveBeenCalled()
  })

  it('带修饰键的点击不拦截(允许新标签打开)', () => {
    confirmSpy.mockReturnValue(false)
    render(<Harness active />)
    fireEvent.click(screen.getByRole('link', { name: '站内返回' }), { ctrlKey: true })
    expect(confirmSpy).not.toHaveBeenCalled()
    expect(probe).toHaveBeenCalled()
  })

  it('取消激活后移除监听,不再拦截', () => {
    confirmSpy.mockReturnValue(false)
    const { rerender } = render(<Harness active />)
    rerender(<Harness active={false} />)

    const unload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unload)
    expect(unload.defaultPrevented).toBe(false)

    fireEvent.click(screen.getByRole('link', { name: '站内返回' }))
    expect(confirmSpy).not.toHaveBeenCalled()
  })
})
