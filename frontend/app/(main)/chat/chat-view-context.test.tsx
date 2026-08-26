import { Suspense } from 'react'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// jsdom 不实现 scrollIntoView;ChatView 会话加载后的滚动 effect 会调用它。
Element.prototype.scrollIntoView = vi.fn()

import { ChatPageClient } from '@/app/(main)/chat/chat-page-client'
import { ChatView } from '@/app/(main)/chat/chat-view'
import { ChatProvider } from '@/contexts/ChatContext'

const SESSION_ID = '11111111-1111-4111-8111-111111111111'
const GROUP_ID = '22222222-2222-4222-8222-222222222222'

const boundaries = vi.hoisted(() => ({
  push: vi.fn(),
  searchParams: new URLSearchParams(),
  listSessions: vi.fn(),
  createSession: vi.fn(),
  createGroupSession: vi.fn(),
  getSession: vi.fn(),
  getMessages: vi.fn(),
  stopStream: vi.fn(),
  fetchAgents: vi.fn(),
  getChatReadiness: vi.fn(),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: boundaries.push }),
  useSearchParams: () => boundaries.searchParams,
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
    createGroupSession: boundaries.createGroupSession,
    getSession: boundaries.getSession,
    getMessages: boundaries.getMessages,
    stopStream: boundaries.stopStream,
    getChatReadiness: boundaries.getChatReadiness,
  }
})

vi.mock('@/contexts/AgentContext', () => ({
  useAgent: () => ({
    state: {
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
      currentAgent: null,
      loading: false,
      error: null,
    },
    fetchAgents: boundaries.fetchAgents,
  }),
}))

// 该文件不开启 vitest globals,RTL 不会自动 cleanup;显式清理避免前一个用例的
// 渲染残留到下一个用例(否则大厅会出现两个 "newSession" 按钮)。
afterEach(() => {
  cleanup()
})

describe('ChatView with ChatProvider state', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    boundaries.searchParams = new URLSearchParams()
    boundaries.listSessions.mockResolvedValue({ list: [], total: 0 })
    boundaries.fetchAgents.mockResolvedValue(undefined)
    boundaries.getMessages.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 200 })
    boundaries.getChatReadiness.mockResolvedValue({
      active_agent_count: 2,
      ready_agent_ids: ['agent-1', 'agent-2'],
      can_create_single: true,
      can_create_group: true,
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

// 真实"打包后导航契约":创建返回 UUID → push 规范 URL → 用该 URL 的 query 重新挂载
// ChatPageClient → 必须用真实 UUID 拉取会话与消息并进入输入区。这正是桌面 webview
// 在 Go 307 到 /chat?session=<UUID> 后加载的页面,不允许 router mock 掩盖。
describe('packaged session navigation contract', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    boundaries.searchParams = new URLSearchParams()
    boundaries.listSessions.mockResolvedValue({ list: [], total: 0 })
    boundaries.fetchAgents.mockResolvedValue(undefined)
    boundaries.getMessages.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 200 })
    boundaries.getChatReadiness.mockResolvedValue({
      active_agent_count: 2,
      ready_agent_ids: ['agent-1', 'agent-2'],
      can_create_single: true,
      can_create_group: true,
    })
  })

  function renderChatPage() {
    return render(
      <ChatProvider>
        <Suspense fallback={null}>
          <ChatPageClient />
        </Suspense>
      </ChatProvider>,
    )
  }

  it('loads a newly created single chat by its real UUID after canonical navigation', async () => {
    const user = userEvent.setup()
    boundaries.createSession.mockResolvedValueOnce({
      id: SESSION_ID,
      type: 'single',
      agent_id: 'agent-1',
      title: '新对话',
      created_at: '2026-08-20T00:00:00Z',
      updated_at: '2026-08-20T00:00:00Z',
    })
    boundaries.getSession.mockResolvedValueOnce({
      id: SESSION_ID,
      type: 'single',
      agent_id: 'agent-1',
      title: '新对话',
      created_at: '2026-08-20T00:00:00Z',
      updated_at: '2026-08-20T00:00:00Z',
    })

    // 第一阶段:大厅创建单聊
    const lobby = render(
      <ChatProvider>
        <ChatView />
      </ChatProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))

    expect(boundaries.createSession).toHaveBeenCalledOnce()
    expect(boundaries.push).toHaveBeenCalledOnce()
    expect(boundaries.push).toHaveBeenCalledWith(`/chat?session=${SESSION_ID}`)
    expect(boundaries.getSession).not.toHaveBeenCalled()
    lobby.unmount()

    // 第二阶段:用规范 URL 的 query 重新挂载(桌面 webview 307 后的真实加载)
    boundaries.searchParams = new URLSearchParams(`session=${SESSION_ID}`)
    renderChatPage()

    expect(await screen.findByRole('textbox')).toBeInTheDocument()
    expect(boundaries.getSession).toHaveBeenCalledWith(SESSION_ID)
    // loadMessages 透传真实 UUID;分页默认值由 chatService 内部补全
    expect(boundaries.getMessages).toHaveBeenCalledWith(SESSION_ID)
    // 大厅引导文案不应再出现
    expect(screen.queryByText('选择一位道人，开始你的论道之旅。')).not.toBeInTheDocument()
  })

  it('loads a newly created group chat by its real UUID after canonical navigation', async () => {
    const user = userEvent.setup()
    boundaries.createGroupSession.mockResolvedValueOnce({
      id: GROUP_ID,
      type: 'group',
      agent_id: '',
      title: '围炉论道',
      created_at: '2026-08-20T00:00:00Z',
      updated_at: '2026-08-20T00:00:00Z',
      members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ],
    })
    boundaries.getSession.mockResolvedValueOnce({
      id: GROUP_ID,
      type: 'group',
      agent_id: '',
      title: '围炉论道',
      created_at: '2026-08-20T00:00:00Z',
      updated_at: '2026-08-20T00:00:00Z',
      members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ],
    })

    // 第一阶段:大厅建群(切到群模式,选两位道人,确认)
    const lobby = render(
      <ChatProvider>
        <ChatView />
      </ChatProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: /mode.confirm/ }))

    expect(boundaries.createGroupSession).toHaveBeenCalledOnce()
    expect(boundaries.createGroupSession).toHaveBeenCalledWith(['agent-1', 'agent-2'])
    expect(boundaries.push).toHaveBeenCalledOnce()
    expect(boundaries.push).toHaveBeenCalledWith(`/chat?session=${GROUP_ID}`)
    expect(boundaries.getSession).not.toHaveBeenCalled()
    lobby.unmount()

    // 第二阶段:规范 URL 重新挂载
    boundaries.searchParams = new URLSearchParams(`session=${GROUP_ID}`)
    renderChatPage()

    expect(await screen.findByRole('textbox')).toBeInTheDocument()
    expect(boundaries.getSession).toHaveBeenCalledWith(GROUP_ID)
    expect(boundaries.getMessages).toHaveBeenCalledWith(GROUP_ID)
    expect(screen.queryByText('选择一位道人，开始你的论道之旅。')).not.toBeInTheDocument()
  })
})
