import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ConversationDirectory } from '@/components/chat/conversation-directory'
import type { ChatSession } from '@/services/types'

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

const sessions: ChatSession[] = [
  {
    id: '11111111-1111-4111-8111-111111111111',
    type: 'single',
    agent_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    agent_name: 'Alpha',
    title: 'Alpha first',
    created_at: '2026-08-20T00:00:00Z',
    updated_at: '2026-08-21T00:00:00Z',
  },
  {
    id: '22222222-2222-4222-8222-222222222222',
    type: 'single',
    agent_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    agent_name: 'Alpha',
    title: 'Alpha second',
    created_at: '2026-08-20T00:00:00Z',
    updated_at: '2026-08-22T00:00:00Z',
  },
  {
    id: '33333333-3333-4333-8333-333333333333',
    type: 'single',
    agent_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
    agent_name: 'Beta',
    title: 'Beta talk',
    created_at: '2026-08-20T00:00:00Z',
    updated_at: '2026-08-19T00:00:00Z',
  },
  {
    id: '44444444-4444-4444-8444-444444444444',
    type: 'group',
    agent_id: '',
    title: 'Furnace circle',
    members: [
      { agent_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', name: 'Alpha', proactivity: 60 },
      { agent_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', name: 'Beta', proactivity: 70 },
    ],
    created_at: '2026-08-20T00:00:00Z',
    updated_at: '2026-08-23T00:00:00Z',
  },
]

describe('conversation directory', () => {
  afterEach(() => cleanup())

  it('defaults to single, groups by daoist, and opens the active group', () => {
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="22222222-2222-4222-8222-222222222222"
        onSelect={vi.fn()}
      />,
    )

    expect(screen.getByRole('tab', { name: 'tabs.single' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('button', { name: /Alpha/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Alpha second')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Beta/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Beta talk')).not.toBeInTheDocument()
  })

  it('switches to the group tab and lists group topics', async () => {
    const user = userEvent.setup()
    render(<ConversationDirectory sessions={sessions} onSelect={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'tabs.group' }))

    expect(screen.getByRole('tab', { name: 'tabs.group' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('Furnace circle')).toBeInTheDocument()
    expect(screen.queryByText('Alpha first')).not.toBeInTheDocument()
  })

  it('shows a configured group avatar instead of the member stack', async () => {
    const user = userEvent.setup()
    const withAvatar = sessions.map(session => session.type === 'group'
      ? { ...session, avatar: 'https://example.com/group.png' }
      : session)
    render(<ConversationDirectory sessions={withAvatar} onSelect={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'tabs.group' }))
    expect(screen.getByRole('img', { name: 'Furnace circle' })).toHaveAttribute('src', 'https://example.com/group.png')
    expect(screen.queryByLabelText('Alpha')).not.toBeInTheDocument()
  })

  it('selects the group tab for a deep-linked group session', () => {
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="44444444-4444-4444-8444-444444444444"
        onSelect={vi.fn()}
      />,
    )

    expect(screen.getByRole('tab', { name: 'tabs.group' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('Furnace circle')).toBeInTheDocument()
  })

  it('shows independent empty states for both tabs', async () => {
    const user = userEvent.setup()
    render(<ConversationDirectory sessions={[]} onSelect={vi.fn()} />)

    expect(screen.getByText('emptySingle')).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'tabs.group' }))
    expect(screen.getByText('emptyGroup')).toBeInTheDocument()
    expect(screen.queryByText('emptySingle')).not.toBeInTheDocument()
  })

  it('calls onSelect with the session id when a session is clicked', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="22222222-2222-4222-8222-222222222222"
        onSelect={onSelect}
      />,
    )

    await user.click(screen.getByText('Alpha first'))
    expect(onSelect).toHaveBeenCalledWith('11111111-1111-4111-8111-111111111111')

    await user.click(screen.getByRole('tab', { name: 'tabs.group' }))
    await user.click(screen.getByText('Furnace circle'))
    expect(onSelect).toHaveBeenCalledWith('44444444-4444-4444-8444-444444444444')
  })

  it('collapses a parent group and restores it on demand', async () => {
    const user = userEvent.setup()
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="22222222-2222-4222-8222-222222222222"
        onSelect={vi.fn()}
      />,
    )

    const alpha = screen.getByRole('button', { name: /Alpha/ })
    await user.click(alpha)
    expect(alpha).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Alpha second')).not.toBeInTheDocument()

    await user.click(alpha)
    expect(alpha).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Alpha second')).toBeInTheDocument()
  })

  it('never renders agent UUIDs in visible text', () => {
    render(
      <ConversationDirectory
        sessions={[{ ...sessions[0], agent_name: undefined, agent_id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' }]}
        onSelect={vi.fn()}
      />,
    )

    expect(screen.getByText('unknownAgent')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('cccccccc-cccc-4ccc-8ccc-cccccccccccc')
  })

  it('confirms deletion without selecting the session row', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    const onDelete = vi.fn().mockResolvedValue(true)
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="22222222-2222-4222-8222-222222222222"
        onSelect={onSelect}
        onDelete={onDelete}
      />,
    )

    const row = screen.getByText('Alpha first').closest('li')
    expect(row).not.toBeNull()
    await user.click(within(row!).getByRole('button', { name: 'deleteAction' }))

    expect(onSelect).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    expect(screen.getByText('deleteDescription')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'deleteConfirm' }))
    expect(onDelete).toHaveBeenCalledWith('11111111-1111-4111-8111-111111111111')
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('disables deleting the current session while it is streaming', () => {
    render(
      <ConversationDirectory
        sessions={sessions}
        currentSessionId="22222222-2222-4222-8222-222222222222"
        deleteDisabledSessionId="22222222-2222-4222-8222-222222222222"
        onSelect={vi.fn()}
        onDelete={vi.fn().mockResolvedValue(true)}
      />,
    )

    const row = screen.getByText('Alpha second').closest('li')
    expect(within(row!).getByRole('button', { name: 'deleteAction' })).toBeDisabled()
  })
})
