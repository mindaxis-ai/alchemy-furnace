import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/[id]/agent-detail'
import { ChatView } from '@/app/(main)/chat/chat-view'
import { ApiError } from '@/services/api'
import type { Agent, ChatSession } from '@/services/types'

const defaultAgents: Agent[] = [
  {
    id: 'agent-1',
    name: 'Agent One',
    model_name: 'model-one',
    status: 'active',
    proactivity: 50,
    created_at: '2026-08-20T00:00:00Z',
  },
  {
    id: 'agent-2',
    name: 'Agent Two',
    model_name: 'model-two',
    status: 'active',
    proactivity: 50,
    created_at: '2026-08-20T00:00:00Z',
  },
]

const testDoubles = vi.hoisted(() => ({
  push: vi.fn(),
  createSession: vi.fn(),
  createGroupSession: vi.fn(),
  fetchSessions: vi.fn(),
  loadMessages: vi.fn(),
  clearCurrent: vi.fn(),
  streamMessage: vi.fn(),
  renameSession: vi.fn(),
  stopStream: vi.fn(),
  chatDispatch: vi.fn(),
  fetchAgents: vi.fn(),
  fetchAgent: vi.fn(),
  bindPill: vi.fn(),
  unbindPill: vi.fn(),
  updateAgentPill: vi.fn(),
  editAgent: vi.fn(),
  agentDispatch: vi.fn(),
  fetchPills: vi.fn(),
  pillDispatch: vi.fn(),
  modelOptions: vi.fn(),
  getChatReadiness: vi.fn(),
  chatState: {
    sessions: [] as ChatSession[],
    currentSession: null as ChatSession | null,
    messages: [],
    loading: false,
    streaming: false,
    error: null as string | null,
    sessionsError: null as string | null,
    currentSpeaker: null,
    sessionLoad: { status: 'idle' as const },
    history: {
      page: 1,
      pageSize: 200,
      total: 0,
      hasOlder: false,
      loadingOlder: false,
      olderError: null as string | null,
    },
  },
  agentState: {
    agents: [] as Agent[],
    total: 2,
    currentAgent: {
      id: 'agent-1',
      name: 'Agent One',
      personality: 'calm',
      model_name: 'model-one',
      status: 'active',
      proactivity: 50,
      created_at: '2026-08-20T00:00:00Z',
      agent_pills: [],
      language_pattern: null,
    },
    loading: false,
    error: null as string | null,
  },
}))

vi.mock('next/navigation', () => ({
  useParams: () => ({ id: 'agent-1' }),
  useRouter: () => ({ push: testDoubles.push }),
}))

vi.mock('next-intl', () => ({
  useTranslations: (namespace: string) => (key: string, values?: Record<string, unknown>) => {
    if (key === 'chatSessionTitle') return `Discourse with ${String(values?.name ?? '')}`
    if (key === 'mode.confirm') return 'mode.confirm'
    if (namespace === 'chatView.launch') return `launch.${key}`
    return key
  },
}))

vi.mock('@/contexts/ChatContext', () => ({
  useChat: () => ({
    state: testDoubles.chatState,
    dispatch: testDoubles.chatDispatch,
    fetchSessions: testDoubles.fetchSessions,
    loadMessages: testDoubles.loadMessages,
    clearCurrent: testDoubles.clearCurrent,
    streamMessage: testDoubles.streamMessage,
    createSession: testDoubles.createSession,
    createGroupSession: testDoubles.createGroupSession,
    renameSession: testDoubles.renameSession,
    stopStream: testDoubles.stopStream,
  }),
}))

vi.mock('@/contexts/AgentContext', () => ({
  useAgent: () => ({
    state: testDoubles.agentState,
    dispatch: testDoubles.agentDispatch,
    fetchAgents: testDoubles.fetchAgents,
    fetchAgent: testDoubles.fetchAgent,
    bindPill: testDoubles.bindPill,
    unbindPill: testDoubles.unbindPill,
    updateAgentPill: testDoubles.updateAgentPill,
    editAgent: testDoubles.editAgent,
  }),
}))

vi.mock('@/contexts/PillContext', () => ({
  usePill: () => ({
    state: {
      pills: [],
      total: 0,
      currentPill: null,
      loading: false,
      error: null,
    },
    dispatch: testDoubles.pillDispatch,
    fetchPills: testDoubles.fetchPills,
  }),
}))

vi.mock('@/services/modelService', () => ({
  options: testDoubles.modelOptions,
}))

vi.mock('@/services/chatService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/chatService')>()
  return {
    ...actual,
    getChatReadiness: testDoubles.getChatReadiness,
  }
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

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

describe('chat launch surfaces', () => {
  afterEach(() => {
    cleanup()
  })

  beforeEach(() => {
    vi.resetAllMocks()
    testDoubles.chatState.sessions = []
    testDoubles.chatState.currentSession = null
    testDoubles.chatState.error = null
    testDoubles.chatState.sessionsError = null
    testDoubles.agentState.agents = [...defaultAgents]
    testDoubles.agentState.error = null
    testDoubles.fetchSessions.mockResolvedValue(undefined)
    testDoubles.fetchAgents.mockResolvedValue(undefined)
    testDoubles.fetchAgent.mockResolvedValue(undefined)
    testDoubles.fetchPills.mockResolvedValue(undefined)
    testDoubles.modelOptions.mockResolvedValue([])
    testDoubles.getChatReadiness.mockResolvedValue({
      active_agent_count: 2,
      ready_agent_ids: ['agent-1', 'agent-2'],
      can_create_single: true,
      can_create_group: true,
    })
  })

  it('renders the lobby skeleton immediately while readiness calls remain pending', () => {
    testDoubles.fetchSessions.mockReturnValue(new Promise(() => undefined))
    testDoubles.fetchAgents.mockReturnValue(new Promise(() => undefined))
    testDoubles.getChatReadiness.mockReturnValue(new Promise(() => undefined))

    render(<ChatView />)

    expect(screen.getByRole('heading', { name: '论道' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'newSession' })).toBeEnabled()
  })

  it('links to agent management when no active agents exist', async () => {
    testDoubles.agentState.agents = []
    testDoubles.agentState.total = 0
    testDoubles.getChatReadiness.mockResolvedValueOnce({
      active_agent_count: 0,
      ready_agent_ids: [],
      can_create_single: false,
      can_create_group: false,
    })

    render(<ChatView />)

    expect(await screen.findByRole('link', { name: 'gate.createAgent' })).toHaveAttribute('href', '/agents')
    expect(screen.getByRole('button', { name: 'newSession' })).toBeDisabled()
  })

  it('links to model management and blocks creation when no agent has formal credentials', async () => {
    testDoubles.getChatReadiness.mockResolvedValueOnce({
      active_agent_count: 2,
      ready_agent_ids: [],
      can_create_single: false,
      can_create_group: false,
    })

    render(<ChatView />)

    expect(await screen.findByRole('link', { name: 'gate.configureModel' })).toHaveAttribute('href', '/models')
    expect(screen.getByRole('button', { name: 'newSession' })).toBeDisabled()
  })

  it('allows single but disables group when exactly one agent is ready', async () => {
    testDoubles.getChatReadiness.mockResolvedValueOnce({
      active_agent_count: 2,
      ready_agent_ids: ['agent-1'],
      can_create_single: true,
      can_create_group: false,
    })
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))

    expect(await screen.findByRole('dialog', { name: 'mode.selectAgent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Agent One/ })).toBeEnabled()
    expect(screen.getByRole('button', { name: /Agent Two/ })).toBeDisabled()
    expect(screen.getByText('mode.agentNotReady')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'mode.group' })).toBeDisabled()
    expect(screen.getByText('mode.groupUnavailable')).toBeInTheDocument()
  })

  it('filters or disables ineligible agents using ready_agent_ids', async () => {
    testDoubles.getChatReadiness.mockResolvedValueOnce({
      active_agent_count: 2,
      ready_agent_ids: ['agent-2'],
      can_create_single: true,
      can_create_group: false,
    })
    testDoubles.createSession.mockResolvedValueOnce(singleSession)
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await screen.findByRole('dialog', { name: 'mode.selectAgent' })

    const blocked = screen.getByRole('button', { name: /Agent One/ })
    expect(blocked).toBeDisabled()
    await user.click(blocked)
    expect(testDoubles.createSession).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    expect(testDoubles.createSession).toHaveBeenCalledWith('agent-2')
  })

  it('keeps sessions and agents visible when readiness alone fails', async () => {
    testDoubles.chatState.sessions = [{ ...singleSession, title: 'Existing Chat' }]
    testDoubles.agentState.error = 'agent list unavailable'
    testDoubles.getChatReadiness.mockRejectedValueOnce(new Error('readiness API unavailable'))
    const user = userEvent.setup()
    render(<ChatView />)

    // 会话列表不被 readiness 失败遮蔽
    expect(screen.getByRole('heading', { name: '论道' })).toBeInTheDocument()
    expect(screen.getByText('Existing Chat')).toBeInTheDocument()
    expect(await screen.findByText('readiness API unavailable')).toBeInTheDocument()

    // readiness 失败时仍可打开选择器查看道人,但所有道人都不可发起
    await user.click(screen.getByRole('button', { name: 'newSession' }))
    const dialog = await screen.findByRole('dialog', { name: 'mode.selectAgent' })
    expect(screen.getByText('agent list unavailable')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: /Agent One/ })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: /Agent Two/ })).toBeDisabled()
  })

  it('does not open the picker through the new-session shortcut while not ready', async () => {
    testDoubles.getChatReadiness.mockResolvedValueOnce({
      active_agent_count: 2,
      ready_agent_ids: [],
      can_create_single: false,
      can_create_group: false,
    })
    render(<ChatView />)

    // 等 readiness 落地(出现模型管理入口)再发快捷键
    await screen.findByRole('link', { name: 'gate.configureModel' })
    act(() => {
      window.dispatchEvent(new Event('alchemy:new-session'))
    })

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('exposes the picker as a modal dialog named by its ritual heading', async () => {
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))

    expect(screen.getByRole('dialog', { name: 'mode.selectAgent' })).toHaveAttribute('aria-modal', 'true')
  })

  it('does not dismiss a pending launch through the close control', async () => {
    const creation = deferred<ChatSession>()
    const user = userEvent.setup()
    testDoubles.createSession.mockReturnValueOnce(creation.promise)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    const close = screen.getByRole('button', { name: '关闭弹窗' })

    expect(close).toBeDisabled()
    await user.click(close)
    expect(screen.getByRole('heading', { name: 'mode.selectAgent' })).toBeInTheDocument()

    await act(async () => {
      creation.resolve(singleSession)
      await creation.promise
    })
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'mode.selectAgent' })).not.toBeInTheDocument()
    })
    expect(testDoubles.push).toHaveBeenCalledOnce()
  })

  it('retains a failed single selection, exposes model recovery, and closes after retry succeeds', async () => {
    const user = userEvent.setup()
    testDoubles.createSession
      .mockRejectedValueOnce(new ApiError(
        'Selected model is unavailable',
        409,
        { error_code: 'service.chat.model_unavailable' },
      ))
      .mockResolvedValueOnce(singleSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    const selectedAgent = screen.getByRole('button', { name: /Agent One/ })
    await user.click(selectedAgent)

    expect(await screen.findByRole('alert')).toHaveTextContent('Selected model is unavailable')
    expect(selectedAgent).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('link', { name: 'launch.modelSettings' })).toHaveAttribute('href', '/settings')

    await user.click(screen.getByRole('button', { name: 'launch.retry' }))

    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'mode.selectAgent' })).not.toBeInTheDocument()
    })
    expect(testDoubles.push).toHaveBeenCalledWith('/chat/11111111-1111-4111-8111-111111111111')
  })

  it('retains a failed group selection and disables duplicate launch while submitting', async () => {
    const creation = deferred<ChatSession>()
    const user = userEvent.setup()
    testDoubles.createGroupSession.mockReturnValueOnce(creation.promise)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    const confirm = screen.getByRole('button', { name: 'mode.confirm (2)' })
    await user.click(confirm)

    expect(confirm).toBeDisabled()
    expect(screen.getByRole('status')).toHaveTextContent('launch.submitting')

    await act(async () => {
      creation.reject(new Error('group launch failed'))
      await creation.promise.catch(() => undefined)
    })

    expect(screen.getByRole('alert')).toHaveTextContent('group launch failed')
    expect(screen.getByRole('button', { name: 'mode.confirm (2)' })).toBeEnabled()
    expect(screen.getByRole('button', { name: /Agent One/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: /Agent Two/ })).toHaveAttribute('aria-pressed', 'true')
  })

  it('clears a failed group request before the user changes members or mode', async () => {
    const user = userEvent.setup()
    testDoubles.createGroupSession.mockRejectedValueOnce(new Error('old group failed'))
    testDoubles.createSession.mockResolvedValueOnce(singleSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('old group failed')

    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'launch.retry' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'mode.single' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))

    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'mode.selectAgent' })).not.toBeInTheDocument()
    })
    expect(testDoubles.createGroupSession).toHaveBeenCalledOnce()
    expect(testDoubles.push).toHaveBeenCalledWith('/chat/11111111-1111-4111-8111-111111111111')
  })

  it('shows the shared launch failure and retry on agent detail', async () => {
    const user = userEvent.setup()
    testDoubles.createSession.mockRejectedValueOnce(new ApiError(
      'Agent model needs configuration',
      409,
      { error_code: 'service.chat.model_unavailable' },
    ))
    render(<AgentDetailPage />)

    await user.click(screen.getByRole('button', { name: 'startChatCta' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Agent model needs configuration')
    expect(screen.getByRole('button', { name: 'launch.retry' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'launch.modelSettings' })).toHaveAttribute('href', '/settings')
    expect(testDoubles.push).not.toHaveBeenCalled()
  })
})
