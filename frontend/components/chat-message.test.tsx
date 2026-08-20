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
    doubles.getMessages.mockResolvedValue({ list: [], total: 0 })
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onDone()
    })
    Element.prototype.scrollIntoView = vi.fn()
  })

  it('shows loading and then a retryable back-to-lobby state for a missing session', async () => {
    let resolveSessions!: (value: { list: ChatSession[]; total: number }) => void
    doubles.listSessions.mockReturnValue(new Promise(resolve => { resolveSessions = resolve }))

    renderSession('33333333-3333-4333-8333-333333333333')

    expect(screen.getByText('load.sessionLoading')).toBeInTheDocument()
    resolveSessions({ list: [], total: 0 })

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('load.sessionNotFoundTitle')
    expect(screen.getByRole('button', { name: 'load.retry' })).toBeEnabled()
    expect(screen.getByRole('link', { name: 'load.backToLobby' })).toHaveAttribute('href', '/chat')
    expect(doubles.getMessages).not.toHaveBeenCalled()
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

    renderSession(groupSession.id)

    expect(await screen.findByRole('textbox')).toBeDisabled()
    expect(screen.getByText('readOnly.group')).toBeInTheDocument()
  })

  it('retains interrupted content, marks it incomplete, and resends it on retry', async () => {
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onChunk('partial answer')
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
    expect(doubles.streamChatMessage).toHaveBeenLastCalledWith(
      singleSession.id,
      'original question',
      expect.any(Object),
    )
  })

  it('keeps streamed group chunks in distinct named and avatar-bearing speaker bubbles', async () => {
    doubles.agents = [activeAgent('agent-a', 'Alpha'), activeAgent('agent-b', 'Beta')]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha', agent_avatar: '/alpha.png' })
      handlers.onChunk('alpha one')
      handlers.onChunk(' alpha two')
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta', agent_avatar: '/beta.png' })
      handlers.onChunk('beta reply')
      handlers.onSpeakerDone?.({ agent_id: 'agent-b', message_id: 'message-b' })
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
    doubles.streamChatMessage.mockImplementation(async (_sessionId: string, _content: string, handlers: StreamHandlers) => {
      handlers.onSpeakerStart?.({ agent_id: 'agent-a', agent_name: 'Alpha' })
      handlers.onChunk('complete alpha reply')
      handlers.onSpeakerDone?.({ agent_id: 'agent-a', message_id: 'message-a' })
      handlers.onSpeakerStart?.({ agent_id: 'agent-b', agent_name: 'Beta' })
      handlers.onError('beta upstream failed')
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
  })

  it('offers mention candidates from current session members only', async () => {
    doubles.agents = [
      activeAgent('agent-a', 'Alpha'),
      activeAgent('agent-b', 'Beta'),
      activeAgent('agent-outsider', 'Outsider'),
    ]
    doubles.listSessions.mockResolvedValue({ list: [groupSession], total: 1 })
    const user = userEvent.setup()
    renderSession(groupSession.id)
    const input = await screen.findByRole('textbox')

    await user.type(input, '@')

    expect(screen.getByRole('option', { name: /Alpha/ })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /Beta/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Outsider/ })).not.toBeInTheDocument()
  })
})
