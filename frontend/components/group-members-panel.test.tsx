import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GroupMembersPanel } from '@/components/group-members-panel'
import type { Agent, ChatSession } from '@/services/types'

const doubles = vi.hoisted(() => ({
  fetchAgents: vi.fn(),
  inviteMembers: vi.fn(),
  kickMember: vi.fn(),
  updateGroupAvatar: vi.fn(),
  agents: [] as Agent[],
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
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

vi.mock('@/contexts/ChatContext', () => ({
  useChat: () => ({
    inviteMembers: doubles.inviteMembers,
    kickMember: doubles.kickMember,
    updateGroupAvatar: doubles.updateGroupAvatar,
  }),
}))

const groupSession: ChatSession = {
  id: '22222222-2222-4222-8222-222222222222',
  type: 'group',
  agent_id: '',
  title: 'Furnace circle',
  members: [
    { agent_id: 'agent-a', name: 'Alpha', proactivity: 60, avatar: 'https://example.com/alpha.png' },
    { agent_id: 'agent-b', name: 'Beta', proactivity: 70 },
  ],
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

describe('GroupMembersPanel avatar handling', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    doubles.fetchAgents.mockResolvedValue(undefined)
    doubles.updateGroupAvatar.mockResolvedValue({ ...groupSession, avatar: 'https://example.com/group.png' })
    doubles.agents = [
      {
        id: 'agent-c',
        name: 'Candidate',
        model_name: 'model-c',
        status: 'active',
        proactivity: 50,
        created_at: '2026-08-20T00:00:00Z',
        avatar: 'https://example.com/candidate.png',
      },
    ]
  })

  afterEach(() => cleanup())

  it('renders member avatar images for valid URLs', () => {
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    expect(screen.getByRole('img', { name: 'Alpha' })).toHaveAttribute('src', 'https://example.com/alpha.png')
  })

  it('falls back to the member initial when the avatar image errors', () => {
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    fireEvent.error(screen.getByRole('img', { name: 'Alpha' }))
    expect(screen.queryByRole('img')).toBeNull()
    const anchor = screen.getByRole('button', { name: '查看 Alpha 简介' })
    expect(within(anchor).getByText('A')).toBeInTheDocument()
  })

  it('does not create an img for members without avatar', () => {
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    const betaAnchor = screen.getByRole('button', { name: '查看 Beta 简介' })
    expect(within(betaAnchor).queryByRole('img')).toBeNull()
    expect(within(betaAnchor).getByText('B')).toBeInTheDocument()
  })

  it('renders candidate avatars in the invite list and falls back on error', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: 'invite' }))
    const img = screen.getByRole('img', { name: 'Candidate' })
    expect(img).toHaveAttribute('src', 'https://example.com/candidate.png')
    fireEvent.error(img)
    expect(screen.queryByRole('img', { name: 'Candidate' })).toBeNull()
    expect(screen.getByText('C')).toBeInTheDocument()
  })

  it('previews and saves the group avatar', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    const input = screen.getByLabelText('avatarLabel')
    await user.type(input, 'https://example.com/group.png')
    expect(screen.getByRole('img', { name: 'avatarPreviewAlt' })).toHaveAttribute('src', 'https://example.com/group.png')
    await user.click(screen.getByRole('button', { name: 'saveAvatar' }))
    expect(doubles.updateGroupAvatar).toHaveBeenCalledWith(groupSession.id, 'https://example.com/group.png')
  })

  it('blocks an invalid group avatar', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    await user.type(screen.getByLabelText('avatarLabel'), 'javascript:alert(1)')
    await user.click(screen.getByRole('button', { name: 'saveAvatar' }))
    expect(screen.getByText('avatarInvalid')).toBeInTheDocument()
    expect(doubles.updateGroupAvatar).not.toHaveBeenCalled()
  })
})

// 踢人流程:应用内确认对话框(Wails 桌面 WKWebView 不实现 window.confirm,
// 静默返回 false 导致"移除成员不生效";此处断言弹的是应用内 alertdialog)
describe('GroupMembersPanel kick flow', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    doubles.fetchAgents.mockResolvedValue(undefined)
    doubles.kickMember.mockResolvedValue(undefined)
  })

  afterEach(() => cleanup())

  const kickButtons = () => screen.getAllByRole('button', { name: 'kick' })

  it('shows an in-app confirm dialog before removing a member', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    await user.click(kickButtons()[0])
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    expect(screen.getByText('kickConfirm')).toBeInTheDocument()
  })

  it('removes the member when the dialog is confirmed', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    await user.click(kickButtons()[0])
    await user.click(screen.getByRole('button', { name: 'kickConfirmCta' }))
    expect(doubles.kickMember).toHaveBeenCalledWith('22222222-2222-4222-8222-222222222222', 'agent-a')
  })

  it('keeps the member when the dialog is cancelled', async () => {
    const user = userEvent.setup()
    render(<GroupMembersPanel session={groupSession} open onClose={() => {}} />)
    await user.click(kickButtons()[0])
    await user.click(screen.getByRole('button', { name: 'cancel' }))
    expect(doubles.kickMember).not.toHaveBeenCalled()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })
})
