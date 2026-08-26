import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// ChatView 是重组件,这里只关心适配层是否把校验后的 sessionId 透传给它。
// 不得 mock @/lib/chat-route —— 适配层的契约就是调用真实解析函数。
const chatViewSpy = vi.fn()

vi.mock('./chat-view', () => ({
  ChatView: (props: { sessionId?: string }) => {
    chatViewSpy(props)
    return null
  },
}))

vi.mock('next/navigation', () => ({
  useSearchParams: () => searchParams,
}))

let searchParams: URLSearchParams

afterEach(() => {
  chatViewSpy.mockClear()
})

describe('ChatPageClient', () => {
  it('passes the canonical query session id into ChatView', () => {
    searchParams = new URLSearchParams('session=11111111-1111-4111-8111-111111111111')
    return import('./chat-page-client').then(({ ChatPageClient }) => {
      render(<ChatPageClient />)
      expect(chatViewSpy).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: '11111111-1111-4111-8111-111111111111' }),
      )
    })
  })

  it.each([
    ['empty', ''],
    ['placeholder', 'session=_'],
    ['malformed', 'session=bad-id'],
  ])('opens the lobby for %s query', (_label, query) => {
    searchParams = new URLSearchParams(query)
    return import('./chat-page-client').then(({ ChatPageClient }) => {
      render(<ChatPageClient />)
      expect(chatViewSpy).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: undefined }),
      )
    })
  })
})
