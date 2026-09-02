import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatView } from '@/app/(main)/chat/chat-view'
import { ChatProvider } from '@/contexts/ChatContext'
import type { StreamHandlers } from '@/services/chatService'
import type { Agent, ChatMessage, ChatSession } from '@/services/types'
import type { UserProfile } from '@/services/userService'

const doubles = vi.hoisted(() => ({
  push: vi.fn(),
  listSessions: vi.fn(),
  getSession: vi.fn(),
  getMessages: vi.fn(),
  streamChatMessage: vi.fn(),
  stopStream: vi.fn(),
  fetchAgents: vi.fn(),
  listProviders: vi.fn(),
  agents: [] as Agent[],
  userProfile: null as UserProfile | null,
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: doubles.push }),
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

vi.mock('@/services/chatService', () => ({
  listSessions: doubles.listSessions,
  getSession: doubles.getSession,
  getMessages: doubles.getMessages,
  streamChatMessage: doubles.streamChatMessage,
  stopStream: doubles.stopStream,
  createSession: vi.fn(),
  createGroupSession: vi.fn(),
  renameSession: vi.fn(),
  addMembers: vi.fn(),
  removeMember: vi.fn(),
}))

vi.mock('@/services/modelService', () => ({
  listProviders: doubles.listProviders,
}))

vi.mock('@/services/api', () => ({
  notifyDesktop: vi.fn(),
}))

vi.mock('@/contexts/AgentContext', () => ({
  useAgent: () => ({
    state: {
      agents: doubles.agents,
      total: doubles.agents.length,
      currentAgent: null,
      loading: false,
      error: null,
    },
    fetchAgents: doubles.fetchAgents,
  }),
}))

vi.mock('@/contexts/UserContext', () => ({
  useUser: () => ({ profile: doubles.userProfile, loading: false, error: null }),
}))

const activeAgent = (id: string, name: string, status: Agent['status'] = 'active'): Agent => ({
  id,
  name,
  model_name: `model-${id}`,
  status,
  proactivity: 50,
  created_at: '2026-08-20T00:00:00Z',
})

const singleSession: ChatSession = {
  id: '11111111-1111-4111-8111-111111111111',
  type: 'single',
  agent_id: 'agent-1',
  title: 'Old discourse',
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const groupSession: ChatSession = {
  id: '22222222-2222-4222-8222-222222222222',
  type: 'group',
  agent_id: '',
  title: 'Furnace circle',
  members: [
    { agent_id: 'agent-a', name: 'Alpha', proactivity: 60 },
    { agent_id: 'agent-b', name: 'Beta', proactivity: 70 },
  ],
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const secondSingleSession: ChatSession = {
  ...singleSession,
  id: '33333333-3333-4333-8333-333333333333',
  agent_id: 'agent-2',
  title: 'Current discourse',
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

function renderSession(sessionId: string) {
  return render(
    <ChatProvider>
      <ChatView sessionId={sessionId} />
    </ChatProvider>,
  )
}

describe('recoverable chat history and streaming', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    vi.resetAllMocks()
    doubles.agents = [activeAgent('agent-1', 'Agent One')]
    doubles.userProfile = null
    doubles.fetchAgents.mockResolvedValue(undefined)
    doubles.listProviders.mockResolvedValue({ list: [{}], total: 1 })
    doubles.listSessions.mockResolvedValue({ list: [singleSession], total: 1 })
    doubles.getSession.mockResolvedValue(singleSession)
    doubles.getMessages.mockResolvedValue({ list: [], total: 0 })
    const localValues = new Map<string, string>()
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => localValues.get(key) ?? null,
        setItem: (key: string, value: string) => localValues.set(key, value),
        removeItem: (key: string) => localValues.delete(key),
        clear: () => localValues.clear(),
      },
    })
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onDone()
    })
    Element.prototype.scrollIntoView = vi.fn()
  })

  it('shows loading and then a retryable back-to-lobby state for a missing session', async () => {
    let rejectSession!: (reason: unknown) => void
    doubles.listSessions.mockResolvedValue({ list: [], total: 0 })
    doubles.getSession.mockReturnValue(new Promise((_resolve, reject) => { rejectSession = reject }))

    renderSession('33333333-3333-4333-8333-333333333333')

    expect(screen.getByText('load.sessionLoading')).toBeInTheDocument()
    rejectSession(Object.assign(new Error('missing'), { status: 404 }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('load.sessionNotFoundTitle')
    expect(screen.getByRole('button', { name: 'load.retry' })).toBeEnabled()
    expect(screen.getByRole('link', { name: 'load.backToLobby' })).toHaveAttribute('href', '/chat')
    expect(doubles.getMessages).not.toHaveBeenCalled()
  })

  it('opens a deep-linked session outside the first 100 list rows by direct lookup', async () => {
    const deepSession = { ...singleSession, id: '44444444-4444-4444-8444-444444444444', title: 'Deep session' }
    doubles.listSessions.mockResolvedValue({
      list: Array.from({ length: 100 }, (_, index) => ({ ...singleSession, id: `listed-${index}` })),
      total: 101,
    })
    doubles.getSession.mockResolvedValue(deepSession)
    doubles.getMessages.mockResolvedValue({
      list: [{ id: 'deep-message', role: 'assistant', content: 'deep history', created_at: singleSession.created_at }],
      total: 1,
    })

    renderSession(deepSession.id)

    expect(await screen.findByText('deep history')).toBeInTheDocument()
    expect(doubles.getSession).toHaveBeenCalledWith(deepSession.id)
  })

  it('keeps deep-linked group metadata authoritative after a late empty list response', async () => {
    const pendingList = deferred<{ list: ChatSession[]; total: number }>()
    const deepGroup = { ...groupSession, id: '44444444-4444-4444-8444-444444444445', title: 'Deferred deep group' }
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockReturnValueOnce(pendingList.promise)
    doubles.getSession.mockResolvedValueOnce(deepGroup)
    doubles.getMessages.mockResolvedValueOnce({ list: [], total: 0, page: 1, page_size: 200 })
    doubles.streamChatMessage.mockImplementationOnce(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onAccepted?.()
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'partial group answer' })
      handlers.onError('group terminal failure', {
        terminal: true,
        recovery: 'persisted_retry',
        agent_id: 'agent-a',
        agent_name: 'Alpha',
      })
    })
    const user = userEvent.setup()

    renderSession(deepGroup.id)
    const input = await screen.findByRole('textbox')
    await act(async () => {
      pendingList.resolve({ list: [], total: 0 })
      await pendingList.promise
    })

    // 深链群会话主题在新目录里出现(目录群聊行 + 页头), 断言权威元数据至少呈现一处
    expect(screen.getAllByText('Deferred deep group').length).toBeGreaterThan(0)
    await user.type(input, 'deep group question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('partial group answer')).toBeInTheDocument()
    expect(screen.getByText('group terminal failure')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'stream.retry' })).toBeInTheDocument()
  })

  it('does not repopulate a cleared lobby from a late history response', async () => {
    let resolveMessages!: (value: { list: ChatMessage[]; total: number }) => void
    doubles.getMessages.mockReturnValue(new Promise(resolve => { resolveMessages = resolve }))
    const view = renderSession(singleSession.id)
    await waitFor(() => expect(doubles.getMessages).toHaveBeenCalledWith(singleSession.id))

    view.rerender(
      <ChatProvider>
        <ChatView />
      </ChatProvider>,
    )
    resolveMessages({
      list: [{ id: 'late', role: 'assistant', content: 'late stale history', created_at: singleSession.created_at }],
      total: 1,
    })

    await waitFor(() => expect(screen.queryByText('late stale history')).not.toBeInTheDocument())
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('cancels the old generation on session switch and ignores every stale callback', async () => {
    let oldHandlers!: StreamHandlers
    let newHandlers!: StreamHandlers
    let finishOldStream!: () => void
    let finishNewStream!: () => void
    const oldStream = new Promise<void>(resolve => { finishOldStream = resolve })
    const newStream = new Promise<void>(resolve => { finishNewStream = resolve })
    doubles.agents = [activeAgent('agent-1', 'Agent One'), activeAgent('agent-2', 'Agent Two')]
    doubles.listSessions.mockResolvedValue({ list: [singleSession, secondSingleSession], total: 2 })
    doubles.getMessages
      .mockResolvedValueOnce({ list: [], total: 0, page: 1, page_size: 200 })
      .mockResolvedValueOnce({
        list: [{ id: 'current-history', role: 'assistant', content: 'current session history', created_at: singleSession.created_at }],
        total: 1,
        page: 1,
        page_size: 200,
      })
    doubles.streamChatMessage.mockImplementationOnce(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      oldHandlers = handlers
      await oldStream
    })
    const user = userEvent.setup()
    const view = renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')
    await user.type(input, 'question in old session')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    const stopCallsBeforeSwitch = doubles.stopStream.mock.calls.length

    view.rerender(
      <ChatProvider>
        <ChatView sessionId={secondSingleSession.id} />
      </ChatProvider>,
    )

    expect(await screen.findByText('current session history')).toBeInTheDocument()
    expect(doubles.stopStream).toHaveBeenCalledTimes(stopCallsBeforeSwitch + 1)

    // B 会话发起自己的流式回合:流进行中发送按钮变为停止按钮
    doubles.streamChatMessage.mockImplementationOnce(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      newHandlers = handlers
      await newStream
    })
    const inputB = await screen.findByRole('textbox')
    await user.type(inputB, 'question in new session')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    expect(await screen.findByRole('button', { name: 'input.stop' })).toBeInTheDocument()

    // 旧会话全部回调类别(chunk/error/stopped/done/title)都不得影响 B
    await act(async () => {
      oldHandlers.onChunk({ content: 'stale old answer' })
      oldHandlers.onError('stale old failure', { terminal: true, recovery: 'persisted_retry' })
      oldHandlers.onStopped()
      oldHandlers.onDone()
      oldHandlers.onTitle!('stale injected title')
      finishOldStream()
      await oldStream
    })

    expect(screen.queryByText('stale old answer')).not.toBeInTheDocument()
    expect(screen.queryByText('stale old failure')).not.toBeInTheDocument()
    expect(screen.queryByText('stale injected title')).not.toBeInTheDocument()
    // 目录契约(Task 7): 仅自动展开当前会话所属分组, 旧会话 'Old discourse' 折叠后不可见
    // (其分组头仍在, 标题随折叠隐藏), 不污染当前会话视图
    expect(screen.queryByText('Old discourse')).not.toBeInTheDocument()
    expect(screen.getByText('current session history')).toBeInTheDocument()
    // 旧 done 不得终止 B 的进行中途合(停止按钮仍在)
    expect(screen.getByRole('button', { name: 'input.stop' })).toBeInTheDocument()

    // B 自己的回调在旧回调风暴后仍正常工作
    await act(async () => {
      newHandlers.onChunk({ content: 'new session answer' })
      newHandlers.onDone()
      finishNewStream()
      await newStream
    })
    expect(screen.getByText('new session answer')).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toBeEnabled()
  })

  it('loads the latest history first and prepends older pages on demand', async () => {
    doubles.getMessages
      .mockResolvedValueOnce({
        list: [
          { id: 'message-2', role: 'assistant', content: 'newer history', created_at: '2026-08-20T00:00:02Z' },
          { id: 'message-3', role: 'assistant', content: 'newest history', created_at: '2026-08-20T00:00:03Z' },
        ],
        total: 3,
        page: 1,
        page_size: 2,
      })
      .mockResolvedValueOnce({
        list: [
          { id: 'message-1', role: 'assistant', content: 'oldest history', created_at: '2026-08-20T00:00:01Z' },
        ],
        total: 3,
        page: 2,
        page_size: 2,
      })
    const user = userEvent.setup()

    renderSession(singleSession.id)

    expect(await screen.findByText('newest history')).toBeInTheDocument()
    expect(screen.queryByText('oldest history')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'history.loadOlder' }))

    const oldest = await screen.findByText('oldest history')
    const newer = screen.getByText('newer history')
    expect(doubles.getMessages).toHaveBeenNthCalledWith(2, singleSession.id, { page: 2, page_size: 2 })
    expect(oldest.compareDocumentPosition(newer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('shows a retryable session error instead of leaking it into generic operation state', async () => {
    doubles.getMessages.mockRejectedValueOnce(new Error('history offline'))

    renderSession(singleSession.id)

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('load.sessionErrorTitle')
    expect(alert).toHaveTextContent('history offline')
    expect(screen.getByRole('button', { name: 'load.retry' })).toBeEnabled()
  })

  it('keeps inactive single-agent history readable while disabling new messages', async () => {
    doubles.agents = [activeAgent('agent-1', 'Dormant Agent', 'inactive')]
    const history: ChatMessage[] = [{
      id: 'old-message',
      role: 'assistant',
      content: 'preserved history',
      created_at: '2026-08-19T00:00:00Z',
    }]
    doubles.getMessages.mockResolvedValueOnce({ list: history, total: 1 })

    renderSession(singleSession.id)

    expect(await screen.findByText('preserved history')).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toBeDisabled()
    expect(screen.getByText('readOnly.single')).toBeInTheDocument()
  })

  it('disables a group when any required current member is inactive', async () => {
    doubles.agents = [
      activeAgent('agent-a', 'Alpha'),
      activeAgent('agent-b', 'Beta', 'inactive'),
    ]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)

    renderSession(groupSession.id)

    expect(await screen.findByRole('textbox')).toBeDisabled()
    expect(screen.getByText('readOnly.group')).toBeInTheDocument()
  })

  it('retains interrupted content, marks it incomplete, and resends it on retry', async () => {
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onAccepted?.()
      handlers.onChunk({ content: 'partial answer' })
      handlers.onInterrupted()
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'original question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('partial answer')).toBeInTheDocument()
    expect(screen.getByText('incomplete')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'retry' }))
    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))
    expect(screen.getAllByText('original question')).toHaveLength(1)
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      singleSession.id,
      'original question',
      expect.any(Object),
      { retry: true },
    )
  })

  it('resends a single pre-persist failure normally without duplicating the optimistic user bubble', async () => {
    let attempt = 0
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onError('pattern unavailable', { terminal: true, recovery: 'resend' })
      } else {
        handlers.onDone()
      }
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'single optimistic question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    await user.click(await screen.findByRole('button', { name: 'stream.retry' }))

    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))
    expect(screen.getAllByText('single optimistic question')).toHaveLength(1)
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      singleSession.id,
      'single optimistic question',
      expect.any(Object),
      { retry: false },
    )
  })

  it('consumes a recovery after one successful click so it cannot be sent sequentially again', async () => {
    let attempt = 0
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onError('pattern unavailable', { terminal: true, recovery: 'resend' })
      } else {
        handlers.onDone()
      }
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'one-shot sequential question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    await user.click(await screen.findByRole('button', { name: 'stream.retry' }))
    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))

    const staleRecovery = screen.queryByRole('button', { name: 'stream.retry' })
    if (staleRecovery) await user.click(staleRecovery)

    expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('button', { name: 'stream.retry' })).not.toBeInTheDocument()
    expect(screen.getAllByText('one-shot sequential question')).toHaveLength(1)
  })

  it('guards rapid same-tick recovery clicks before streaming state can render', async () => {
    let attempt = 0
    let releaseRecovery!: () => void
    const recoveryGate = new Promise<void>(resolve => { releaseRecovery = resolve })
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onError('pattern unavailable', { terminal: true, recovery: 'resend' })
        return
      }
      await recoveryGate
      handlers.onDone()
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'one-shot rapid question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    const retry = await screen.findByRole('button', { name: 'stream.retry' })

    await act(async () => {
      retry.click()
      retry.click()
      await Promise.resolve()
    })
    releaseRecovery()
    await act(async () => { await recoveryGate })

    expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2)
    expect(screen.getAllByText('one-shot rapid question')).toHaveLength(1)
    expect(screen.queryByRole('button', { name: 'stream.retry' })).not.toBeInTheDocument()
  })

  it('consumes a persisted retry after its recovery succeeds', async () => {
    let attempt = 0
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onAccepted?.()
        handlers.onChunk({ content: 'persisted partial answer' })
        handlers.onInterrupted()
      } else {
        handlers.onDone()
      }
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'persisted one-shot question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    await user.click(await screen.findByRole('button', { name: 'retry' }))
    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))

    expect(screen.queryByRole('button', { name: 'retry' })).not.toBeInTheDocument()
    expect(screen.getAllByText('persisted one-shot question')).toHaveLength(1)
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      singleSession.id,
      'persisted one-shot question',
      expect.any(Object),
      { retry: true },
    )
  })

  it('allows a failed recovery attempt to declare one fresh recovery', async () => {
    let attempt = 0
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onError('first recoverable failure', { terminal: true, recovery: 'resend' })
      } else if (attempt === 2) {
        handlers.onError('fresh recoverable failure', { terminal: true, recovery: 'resend' })
      } else {
        handlers.onDone()
      }
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'fresh recovery question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    await user.click(await screen.findByRole('button', { name: 'stream.retry' }))
    expect(await screen.findByText('fresh recoverable failure')).toBeInTheDocument()

    expect(screen.getAllByRole('button', { name: 'stream.retry' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'stream.retry' }))
    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(3))

    expect(screen.queryByRole('button', { name: 'stream.retry' })).not.toBeInTheDocument()
    expect(screen.getAllByText('fresh recovery question')).toHaveLength(1)
  })

  it('does not guess a retry mode when transport ends before persistence is acknowledged', async () => {
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onInterrupted()
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'unacknowledged question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('stream.interrupted')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'stream.retry' })).not.toBeInTheDocument()
    expect(screen.getAllByText('unacknowledged question')).toHaveLength(1)
  })

  it('keeps streamed group chunks in distinct named and avatar-bearing speaker bubbles', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png', content: 'alpha one' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png', content: ' alpha two' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: 'https://example.com/beta.png' })
      handlers.onChunk({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: 'https://example.com/beta.png', content: 'beta reply' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: 'https://example.com/beta.png', message_id: 'message-b' })
      handlers.onTurnDone?.({ spoke: 2 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'question for the group')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('alpha one alpha two')).toBeInTheDocument()
    expect(screen.getByText('beta reply')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Alpha' }).querySelector('img')).toHaveAttribute('src', 'https://example.com/alpha.png')
    expect(screen.getByRole('button', { name: 'Beta' }).querySelector('img')).toHaveAttribute('src', 'https://example.com/beta.png')
  })

  it('attaches the actual prompt to its answer when prompt debugging is enabled', async () => {
    window.localStorage.setItem('alchemy.promptDebug', 'true')
    doubles.streamChatMessage.mockImplementation(async (
      _sessionId: string,
      _content: string,
      handlers: StreamHandlers,
      options: { debugPrompt?: boolean },
    ) => {
      expect(options.debugPrompt).toBe(true)
      handlers.onPromptDebug?.({
        agent_id: 'agent-1',
        agent_name: 'Agent One',
        model: 'model-agent-1',
        messages: [{ role: 'system', content: 'STRICT_DEBUG_SYSTEM_PROMPT' }],
        generation: { max_tokens: 128, max_sentences: 2 },
      })
      handlers.onChunk({ content: 'debuggable answer' })
      handlers.onDone()
    })
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'inspect this turn')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('debuggable answer')).toBeInTheDocument()
    const disclosure = screen.getByText('promptDebugShow')
    expect(screen.queryByText('STRICT_DEBUG_SYSTEM_PROMPT')).not.toBeInTheDocument()
    await user.click(disclosure)
    expect(screen.getByText('STRICT_DEBUG_SYSTEM_PROMPT')).toBeInTheDocument()
    expect(screen.getByText('model-agent-1')).toBeInTheDocument()
  })

  it('keeps each group prompt attached to the matching daoist answer', async () => {
    window.localStorage.setItem('alchemy.promptDebug', 'true')
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onPromptDebug?.({
        agent_id: 'agent-a', agent_name: 'Alpha', model: 'model-alpha',
        messages: [{ role: 'system', content: 'ALPHA_SYSTEM_PROMPT' }],
        generation: { max_tokens: 128, max_sentences: 2 },
      })
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'alpha answer' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', message_id: 'message-a' })
      handlers.onPromptDebug?.({
        agent_id: 'agent-b', agent_name: 'Beta', model: 'model-beta',
        messages: [{ role: 'system', content: 'BETA_SYSTEM_PROMPT' }],
        generation: { max_tokens: 128, max_sentences: 2 },
      })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onChunk({ agent_id: 'agent-b', agent_name: 'Beta', content: 'beta answer' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-b', agent_name: 'Beta', message_id: 'message-b' })
      handlers.onTurnDone?.({ spoke: 2 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'inspect group prompts')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    const disclosures = await screen.findAllByText('promptDebugShow')
    expect(disclosures).toHaveLength(2)
    await user.click(disclosures[0])
    expect(screen.getByText('ALPHA_SYSTEM_PROMPT')).toBeInTheDocument()
    expect(screen.queryByText('BETA_SYSTEM_PROMPT')).not.toBeInTheDocument()
    await user.click(disclosures[1])
    expect(screen.getByText('BETA_SYSTEM_PROMPT')).toBeInTheDocument()
  })

  it('prefers message.agent_avatar over agent profile and member avatars in group chat', async () => {
    doubles.agents = [
      { ...activeAgent('agent-a', 'Alpha'), avatar: 'https://example.com/profile-alpha.png' },
      { ...activeAgent('agent-b', 'Beta'), avatar: 'https://example.com/profile-beta.png' },
    ]
    const groupWithAvatars: ChatSession = {
      ...groupSession,
      members: [
        { agent_id: 'agent-a', name: 'Alpha', proactivity: 60, avatar: 'https://example.com/member-alpha.png' },
        { agent_id: 'agent-b', name: 'Beta', proactivity: 70, avatar: 'https://example.com/member-beta.png' },
      ],
    }
    doubles.listSessions.mockResolvedValue({ list: [groupWithAvatars], total: 1 })
    doubles.getSession.mockResolvedValue(groupWithAvatars)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/stream-alpha.png' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/stream-alpha.png', content: 'priority answer' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/stream-alpha.png', message_id: 'message-a' })
      handlers.onTurnDone?.({ spoke: 1 })
    })
    const user = userEvent.setup()
    renderSession(groupWithAvatars.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'priority question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('priority answer')).toBeInTheDocument()
    const alphaBtn = screen.getByRole('button', { name: 'Alpha' })
    expect(alphaBtn.querySelector('img')).toHaveAttribute('src', 'https://example.com/stream-alpha.png')
  })

  it('falls back to the agent initial when its avatar image errors', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png', content: 'broken avatar reply' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: 'https://example.com/alpha.png', message_id: 'message-a' })
      handlers.onTurnDone?.({ spoke: 1 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    const alphaBtn = await screen.findByRole('button', { name: 'Alpha' })
    const img = alphaBtn.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://example.com/alpha.png')
    fireEvent.error(img!)
    expect(alphaBtn.querySelector('img')).toBeNull()
    expect(within(alphaBtn).getByText('A')).toBeInTheDocument()
  })

  it('does not create an avatar img when the agent has no avatar', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'no avatar reply' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', message_id: 'message-a' })
      handlers.onTurnDone?.({ spoke: 1 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    const alphaBtn = await screen.findByRole('button', { name: 'Alpha' })
    expect(alphaBtn.querySelector('img')).toBeNull()
    expect(within(alphaBtn).getByText('A')).toBeInTheDocument()
  })

  it('shows the user avatar from userProfile and falls back to the initial on error', async () => {
    doubles.userProfile = {
      display_name: '炼丹者',
      bio: '',
      avatar: 'https://example.com/user.png',
      updated_at: '2026-08-20T00:00:00Z',
    }
    doubles.agents = [activeAgent('agent-1', 'Agent One')]
    doubles.listSessions.mockResolvedValue({ list: [singleSession], total: 1 })
    doubles.getSession.mockResolvedValue(singleSession)
    const user = userEvent.setup()
    renderSession(singleSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'my question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    const userBtn = await screen.findByRole('button', { name: 'userLabel' })
    const img = userBtn.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://example.com/user.png')
    fireEvent.error(img!)
    expect(userBtn.querySelector('img')).toBeNull()
    expect(within(userBtn).getByText('炼')).toBeInTheDocument()
  })

  it('uses the session-level daoist identity for single chat when the agents list is empty', async () => {
    doubles.agents = []
    const sessionWithIdentity: ChatSession = {
      ...singleSession,
      agent_name: '真实道号',
      agent_avatar: 'https://example.com/session-daoist.png',
    }
    doubles.listSessions.mockResolvedValue({ list: [sessionWithIdentity], total: 1 })
    doubles.getSession.mockResolvedValue(sessionWithIdentity)
    doubles.getMessages.mockResolvedValueOnce({
      list: [{ id: 'historical', role: 'assistant', content: 'past wisdom', created_at: '2026-08-19T00:00:00Z' }],
      total: 1,
    })

    renderSession(sessionWithIdentity.id)

    expect(await screen.findByText('past wisdom')).toBeInTheDocument()
    // 服务端权威道号同时呈现于目录分组与页头/消息区, 至少一处可见即断言成立
    expect(screen.getAllByText('真实道号').length).toBeGreaterThan(0)
    expect(screen.queryByText('assistantLabel')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '真实道号' }).querySelector('img'))
      .toHaveAttribute('src', 'https://example.com/session-daoist.png')
  })

  it('does not mark a completed speaker incomplete when the next group speaker fails before a chunk', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'complete alpha reply' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onError('beta upstream failed', { terminal: false, recovery: 'none', agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onTurnDone?.({ spoke: 1 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('complete alpha reply')).toBeInTheDocument()
    expect(screen.getByText('beta upstream failed')).toBeInTheDocument()
    expect(screen.queryByText('incomplete')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Beta' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'stream.retry' })).not.toBeInTheDocument()
  })

  it('ends a group turn on real transport interruption after a member error and offers one recovery', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onAccepted?.()
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'complete alpha' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onChunk({ agent_id: 'agent-b', agent_name: 'Beta', content: 'partial beta' })
      handlers.onError('beta member failed', { terminal: false, recovery: 'none', agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onInterrupted()
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'group question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('complete alpha')).toBeInTheDocument()
    expect(screen.getByText('partial beta')).toBeInTheDocument()
    expect(screen.getByText('beta member failed')).toBeInTheDocument()
    expect(input).toBeEnabled()
    expect(screen.getAllByRole('button', { name: 'stream.retry' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'stream.retry' }))
    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))
    expect(screen.getAllByText('group question')).toHaveLength(1)
    expect(screen.getByText('stream.retryBoundary')).toBeInTheDocument()
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      groupSession.id,
      'group question',
      expect.any(Object),
      { retry: true },
    )
  })

  it('resends a group preflight failure without a duplicate user or phantom attempt boundary', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    let attempt = 0
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      attempt += 1
      if (attempt === 1) {
        handlers.onError('group preflight unavailable', { terminal: true, recovery: 'resend' })
      } else {
        handlers.onTurnDone?.({ spoke: 0 })
      }
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'group optimistic question')
    await user.click(screen.getByRole('button', { name: 'input.send' }))
    await user.click(await screen.findByRole('button', { name: 'stream.retry' }))

    await waitFor(() => expect(doubles.streamChatMessage).toHaveBeenCalledTimes(2))
    expect(screen.getAllByText('group optimistic question')).toHaveLength(1)
    expect(screen.queryByText('stream.retryBoundary')).not.toBeInTheDocument()
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      groupSession.id,
      'group optimistic question',
      expect.any(Object),
      { retry: false },
    )
  })

  it('does not render a zero-speaker failed turn as a successful completion', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementationOnce(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onAccepted?.()
      handlers.onTurnDone?.({ spoke: 0, reason: 'failed', failed_speakers: 1 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')
    await user.type(input, 'question with no answer')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('本轮未收到有效回复，请重试')).toBeInTheDocument()
    expect(input).toBeEnabled()
  })

  it('offers mention candidates from current session members only', async () => {
    doubles.agents = [
      activeAgent('agent-a', 'Alpha'),
      activeAgent('agent-b', 'Beta'),
      activeAgent('agent-outsider', 'Outsider'),
    ]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, '@')

    expect(screen.getByRole('option', { name: /Alpha/ })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /Beta/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Outsider/ })).not.toBeInTheDocument()
  })
})
