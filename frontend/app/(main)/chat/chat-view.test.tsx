import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/[id]/agent-detail'
import { ChatView } from '@/app/(main)/chat/chat-view'
import { ApiError } from '@/services/api'
import type { ChatSession } from '@/services/types'

const testDoubles = vi.hoisted(() => ({
  push: vi.fn(),
  createSession: vi.fn(),
  createGroupSession: vi.fn(),
  fetchSessions: vi.fn(),
  loadMessages: vi.fn(),
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
  listProviders: vi.fn(),
  modelOptions: vi.fn(),
  chatState: {
    sessions: [] as ChatSession[],
    currentSession: null as ChatSession | null,
    messages: [],
    loading: false,
    streaming: false,
    error: null as string | null,
    sessionsError: null as string | null,
    currentSpeaker: null,
  },
  agentState: {
    agents: [
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
    ],
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
  listProviders: testDoubles.listProviders,
  options: testDoubles.modelOptions,
}))

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
    testDoubles.agentState.error = null
    testDoubles.fetchSessions.mockResolvedValue(undefined)
    testDoubles.fetchAgents.mockResolvedValue(undefined)
    testDoubles.fetchAgent.mockResolvedValue(undefined)
    testDoubles.fetchPills.mockResolvedValue(undefined)
    testDoubles.listProviders.mockResolvedValue({ list: [{}] })
    testDoubles.modelOptions.mockResolvedValue([])
  })

  it('renders the lobby skeleton immediately while readiness calls remain pending', () => {
    testDoubles.fetchSessions.mockReturnValue(new Promise(() => undefined))
    testDoubles.fetchAgents.mockReturnValue(new Promise(() => undefined))
    testDoubles.listProviders.mockReturnValue(new Promise(() => undefined))

    render(<ChatView />)

    expect(screen.getByRole('heading', { name: '论道' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'newSession' })).toBeEnabled()
  })

  it('keeps readiness failures independent and does not treat a provider failure as configured', async () => {
    testDoubles.chatState.sessionsError = 'session list unavailable'
    testDoubles.agentState.error = 'agent list unavailable'
    testDoubles.listProviders.mockRejectedValueOnce(new Error('provider API unavailable'))

    render(<ChatView />)

    expect(screen.getByRole('heading', { name: '论道' })).toBeInTheDocument()
    expect(await screen.findByText('provider API unavailable')).toBeInTheDocument()
    expect(screen.getByText('session list unavailable')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'newSession' }))
    expect(screen.getByText('agent list unavailable')).toBeInTheDocument()
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
