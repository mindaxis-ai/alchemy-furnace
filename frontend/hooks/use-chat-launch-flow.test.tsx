import type { ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatProvider, useChat } from '@/contexts/ChatContext'
import { useChatLaunchFlow } from '@/hooks/use-chat-launch-flow'
import { ApiError } from '@/services/api'
import type { ChatSession } from '@/services/types'

const createSession = vi.hoisted(() => vi.fn())
const createGroupSession = vi.hoisted(() => vi.fn())
const listSessions = vi.hoisted(() => vi.fn())
const push = vi.hoisted(() => vi.fn())

vi.mock('@/services/chatService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/chatService')>()
  return {
    ...actual,
    listSessions,
    createSession,
    createGroupSession,
  }
})

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}))

const wrapper = ({ children }: { children: ReactNode }) => (
  <ChatProvider>{children}</ChatProvider>
)

const singleSession: ChatSession = {
  id: '11111111-1111-4111-8111-111111111111',
  type: 'single',
  agent_id: 'agent-1',
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const groupSession: ChatSession = {
  id: '22222222-2222-4222-8222-222222222222',
  type: 'group',
  agent_id: '',
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('ChatProvider session creation', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listSessions.mockResolvedValue({ list: [], total: 0 })
  })

  it('owns list failures separately and clears them after a successful reload', async () => {
    listSessions
      .mockRejectedValueOnce(new Error('session list unavailable'))
      .mockResolvedValueOnce({ list: [], total: 0 })
    createSession.mockRejectedValueOnce(new Error('launch unavailable'))
    const { result } = renderHook(() => useChat(), { wrapper })

    await act(async () => {
      await result.current.fetchSessions()
    })
    expect(result.current.state.sessionsError).toBe('session list unavailable')
    expect(result.current.state.error).toBeNull()

    await act(async () => {
      await result.current.createSession('agent-1').catch(() => undefined)
    })
    expect(result.current.state.sessionsError).toBe('session list unavailable')
    expect(result.current.state.error).toBe('launch unavailable')

    await act(async () => {
      await result.current.fetchSessions()
    })
    expect(result.current.state.sessionsError).toBeNull()
    expect(result.current.state.error).toBe('launch unavailable')
  })

  it('surfaces the original single-session error and ends loading', async () => {
    const serviceError = new ApiError(
      '模型未配置',
      409,
      { error_code: 'service.chat.model_unavailable' },
    )
    createSession.mockRejectedValueOnce(serviceError)
    const { result } = renderHook(() => useChat(), { wrapper })

    let request!: Promise<unknown>
    act(() => {
      request = result.current.createSession('agent-1')
    })
    expect(result.current.state.loading).toBe(true)

    let caught: unknown
    await act(async () => {
      caught = await request.catch((error) => error)
    })

    expect(caught).toBe(serviceError)
    expect(result.current.state.error).toBe('模型未配置')
    expect(result.current.state.loading).toBe(false)
  })

  it('surfaces the original group-session error', async () => {
    const serviceError = new ApiError(
      '道人不可用',
      409,
      { error_code: 'service.chat.agent_inactive' },
    )
    createGroupSession.mockRejectedValueOnce(serviceError)
    const { result } = renderHook(() => useChat(), { wrapper })

    let caught: unknown
    await act(async () => {
      caught = await result.current
        .createGroupSession(['agent-1', 'agent-2'])
        .catch((error) => error)
    })

    expect(caught).toBe(serviceError)
    expect(result.current.state.error).toBe('道人不可用')
    expect(result.current.state.loading).toBe(false)
  })
})

describe('useChatLaunchFlow', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listSessions.mockResolvedValue({ list: [], total: 0 })
  })

  it('enters submitting immediately while session creation is pending', async () => {
    const pending = deferred<ChatSession>()
    createSession.mockReturnValueOnce(pending.promise)
    const { result } = renderHook(() => useChatLaunchFlow(), { wrapper })

    let launch!: Promise<boolean>
    act(() => {
      launch = result.current.launchSingle('agent-1')
    })

    expect(result.current.state).toEqual({ status: 'submitting' })
    expect(push).not.toHaveBeenCalled()

    await act(async () => {
      pending.resolve(singleSession)
      await launch
    })
  })

  it('retains a failed group selection and its stable error code for retry', async () => {
    const serviceError = new ApiError(
      '道人不可用',
      409,
      { error_code: 'service.chat.agent_inactive' },
    )
    createGroupSession
      .mockRejectedValueOnce(serviceError)
      .mockResolvedValueOnce(groupSession)
    const selectedAgentIds = ['agent-1', 'agent-2']
    const { result } = renderHook(() => useChatLaunchFlow(), { wrapper })

    await act(async () => {
      expect(await result.current.launchGroup(selectedAgentIds)).toBe(false)
    })
    selectedAgentIds.push('agent-3')

    expect(result.current.state).toEqual({
      status: 'error',
      message: '道人不可用',
      errorCode: 'service.chat.agent_inactive',
    })
    expect(push).not.toHaveBeenCalled()

    await act(async () => {
      expect(await result.current.retry()).toBe(true)
    })

    expect(createGroupSession).toHaveBeenNthCalledWith(1, ['agent-1', 'agent-2'])
    expect(createGroupSession).toHaveBeenNthCalledWith(2, ['agent-1', 'agent-2'])
    expect(push).toHaveBeenCalledOnce()
    expect(push).toHaveBeenCalledWith('/chat/22222222-2222-4222-8222-222222222222')
  })

  it('navigates exactly once after a successful single-session response', async () => {
    createSession.mockResolvedValueOnce(singleSession)
    const { result } = renderHook(() => useChatLaunchFlow(), { wrapper })

    await act(async () => {
      expect(await result.current.launchSingle('agent-1')).toBe(true)
    })

    expect(push).toHaveBeenCalledOnce()
    expect(push).toHaveBeenCalledWith('/chat/11111111-1111-4111-8111-111111111111')
    expect(result.current.state).toEqual({ status: 'idle' })
  })

  it('ignores duplicate launches while a request is in flight', async () => {
    const pending = deferred<ChatSession>()
    createSession.mockReturnValueOnce(pending.promise)
    const { result } = renderHook(() => useChatLaunchFlow(), { wrapper })

    let first!: Promise<boolean>
    let duplicate!: Promise<boolean>
    act(() => {
      first = result.current.launchSingle('agent-1')
      duplicate = result.current.launchSingle('agent-2')
    })

    expect(await duplicate).toBe(false)
    expect(createSession).toHaveBeenCalledOnce()
    expect(createSession).toHaveBeenCalledWith({ agent_id: 'agent-1', title: undefined })

    await act(async () => {
      pending.resolve(singleSession)
      expect(await first).toBe(true)
    })
    expect(push).toHaveBeenCalledOnce()
  })

  it('reset clears the failed request as well as its visible error', async () => {
    createSession.mockRejectedValueOnce(new Error('网络中断'))
    const { result } = renderHook(() => useChatLaunchFlow(), { wrapper })

    await act(async () => {
      await result.current.launchSingle('agent-1')
    })
    act(() => {
      result.current.reset()
    })

    expect(result.current.state).toEqual({ status: 'idle' })
    await act(async () => {
      expect(await result.current.retry()).toBe(false)
    })
    expect(createSession).toHaveBeenCalledOnce()
  })
})
