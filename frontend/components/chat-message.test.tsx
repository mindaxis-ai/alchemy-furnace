import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatView } from '@/app/(main)/chat/chat-view'
import { ChatProvider } from '@/contexts/ChatContext'
import type { StreamHandlers } from '@/services/chatService'
import type { Agent, ChatMessage, ChatSession } from '@/services/types'

const doubles = vi.hoisted(() => ({
  push: vi.fn(),
  listSessions: vi.fn(),
  getSession: vi.fn(),
  getMessages: vi.fn(),
  streamChatMessage: vi.fn(),
  fetchAgents: vi.fn(),
  listProviders: vi.fn(),
  agents: [] as Agent[],
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
  stopStream: vi.fn(),
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
  useUser: () => ({ profile: null, loading: false, error: null }),
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
    doubles.fetchAgents.mockResolvedValue(undefined)
    doubles.listProviders.mockResolvedValue({ list: [{}], total: 1 })
    doubles.listSessions.mockResolvedValue({ list: [singleSession], total: 1 })
    doubles.getSession.mockResolvedValue(singleSession)
    doubles.getMessages.mockResolvedValue({ list: [], total: 0 })
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

  it('keeps streamed group chunks in distinct named and avatar-bearing speaker bubbles', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.getSession.mockResolvedValue(groupSession)
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: '/alpha.png' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: '/alpha.png', content: 'alpha one' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: '/alpha.png', content: ' alpha two' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: '/alpha.png', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: '/beta.png' })
      handlers.onChunk({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: '/beta.png', content: 'beta reply' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: '/beta.png', message_id: 'message-b' })
      handlers.onTurnDone?.({ spoke: 2 })
    })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, 'question for the group')
    await user.click(screen.getByRole('button', { name: 'input.send' }))

    expect(await screen.findByText('alpha one alpha two')).toBeInTheDocument()
    expect(screen.getByText('beta reply')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Alpha' }).querySelector('img')).toHaveAttribute('src', '/alpha.png')
    expect(screen.getByRole('button', { name: 'Beta' }).querySelector('img')).toHaveAttribute('src', '/beta.png')
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
      handlers.onError('beta upstream failed', { terminal: false, agent_id: 'agent-b', agent_name: 'Beta' })
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
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk({ agent_id: 'agent-a', agent_name: 'Alpha', content: 'complete alpha' })
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', agent_name: 'Alpha', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onChunk({ agent_id: 'agent-b', agent_name: 'Beta', content: 'partial beta' })
      handlers.onError('beta member failed', { terminal: false, agent_id: 'agent-b', agent_name: 'Beta' })
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
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      groupSession.id,
      'group question',
      expect.any(Object),
      { retry: true },
    )
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
