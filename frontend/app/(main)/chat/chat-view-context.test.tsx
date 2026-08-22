import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatView } from '@/app/(main)/chat/chat-view'
import { ChatProvider } from '@/contexts/ChatContext'

const boundaries = vi.hoisted(() => ({
  push: vi.fn(),
  listSessions: vi.fn(),
  createSession: vi.fn(),
  fetchAgents: vi.fn(),
  getChatReadiness: vi.fn(),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: boundaries.push }),
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

vi.mock('@/services/chatService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/chatService')>()
  return {
    ...actual,
    listSessions: boundaries.listSessions,
    createSession: boundaries.createSession,
    getChatReadiness: boundaries.getChatReadiness,
  }
})

vi.mock('@/contexts/AgentContext', () => ({
  useAgent: () => ({
    state: {
      agents: [{
        id: 'agent-1',
        name: 'Agent One',
        model_name: 'model-one',
        status: 'active',
        proactivity: 50,
        created_at: '2026-08-20T00:00:00Z',
      }],
      total: 1,
      currentAgent: null,
      loading: false,
      error: null,
    },
    fetchAgents: boundaries.fetchAgents,
  }),
}))

describe('ChatView with ChatProvider state', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    boundaries.listSessions.mockResolvedValue({ list: [], total: 0 })
    boundaries.fetchAgents.mockResolvedValue(undefined)
    boundaries.getChatReadiness.mockResolvedValue({
      active_agent_count: 1,
      ready_agent_ids: ['agent-1'],
      can_create_single: true,
      can_create_group: false,
    })
  })

  it('does not leak a closed launch failure into session-list readiness', async () => {
    const user = userEvent.setup()
    boundaries.createSession.mockRejectedValueOnce(new Error('launch unavailable'))
    render(
      <ChatProvider>
        <ChatView />
      </ChatProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent('launch unavailable')

    await user.click(screen.getByRole('button', { name: '关闭弹窗' }))

    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'mode.selectAgent' })).not.toBeInTheDocument()
    })
    expect(screen.queryByText('launch unavailable')).not.toBeInTheDocument()
    expect(screen.queryByText('load.sessionsError')).not.toBeInTheDocument()
  })
})
