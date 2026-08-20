import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/[id]/agent-detail'
import { ChatView } from '@/app/(main)/chat/chat-view'
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
    error: null,
  },
}))

vi.mock('next/navigation', () => ({
  useParams: () => ({ id: 'agent-1' }),
  useRouter: () => ({ push: testDoubles.push }),
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) => {
    if (key === 'chatSessionTitle') return `Discourse with ${String(values?.name ?? '')}`
    if (key === 'mode.confirm') return 'mode.confirm'
    return key
  },
}))

vi.mock('@/contexts/ChatContext', () => ({
  useChat: () => ({
    state: {
      sessions: [],
      currentSession: null,
      messages: [],
      loading: false,
      streaming: false,
      error: null,
      currentSpeaker: null,
    },
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

describe('legacy session launch callers', () => {
  afterEach(() => {
    cleanup()
  })

  beforeEach(() => {
    vi.clearAllMocks()
    testDoubles.fetchSessions.mockResolvedValue(undefined)
    testDoubles.fetchAgents.mockResolvedValue(undefined)
    testDoubles.fetchAgent.mockResolvedValue(undefined)
    testDoubles.fetchPills.mockResolvedValue(undefined)
    testDoubles.listProviders.mockResolvedValue({ list: [{}] })
    testDoubles.modelOptions.mockResolvedValue([])
  })

  it('keeps the single-chat picker open when session creation rejects', async () => {
    const creation = deferred<ChatSession>()
    testDoubles.createSession.mockReturnValueOnce(creation.promise)
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))

    expect(screen.getByRole('heading', { name: 'mode.selectAgent' })).toBeInTheDocument()
    await act(async () => {
      creation.reject(new Error('session failed'))
      await creation.promise.catch(() => undefined)
    })
    expect(screen.getByRole('heading', { name: 'mode.selectAgent' })).toBeInTheDocument()
    expect(testDoubles.push).not.toHaveBeenCalled()
  })

  it('keeps the group-chat selection when session creation rejects', async () => {
    const creation = deferred<ChatSession>()
    testDoubles.createGroupSession.mockReturnValueOnce(creation.promise)
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    expect(screen.getByRole('button', { name: 'mode.confirm (2)' })).toBeInTheDocument()
    await act(async () => {
      creation.reject(new Error('group failed'))
      await creation.promise.catch(() => undefined)
    })
    expect(screen.getByRole('button', { name: 'mode.confirm (2)' })).toBeInTheDocument()
    expect(testDoubles.push).not.toHaveBeenCalled()
  })

  it('restores the agent start-chat button after session creation rejects', async () => {
    const creation = deferred<ChatSession>()
    testDoubles.createSession.mockReturnValueOnce(creation.promise)
    const user = userEvent.setup()
    render(<AgentDetailPage />)

    const startButton = screen.getByRole('button', { name: 'startChatCta' })
    await user.click(startButton)
    expect(startButton.querySelector('svg')).toHaveClass('animate-spin')

    await act(async () => {
      creation.reject(new Error('session failed'))
      await creation.promise.catch(() => undefined)
    })

    await waitFor(() => {
      expect(startButton.querySelector('svg')).not.toHaveClass('animate-spin')
    })
    expect(testDoubles.push).not.toHaveBeenCalled()
  })
})
