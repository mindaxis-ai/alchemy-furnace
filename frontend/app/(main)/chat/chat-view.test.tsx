import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/detail/agent-detail'
import { ChatView } from '@/app/(main)/chat/chat-view'
import { ApiError } from '@/services/api'
import type { Agent, ChatSession } from '@/services/types'

// jsdom 未实现 scrollIntoView：ChatView 消息区滚底效果依赖它（chat-view-context.test.tsx 同款桩）
Element.prototype.scrollIntoView = vi.fn()

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
  editAgent: vi.fn(),
  agentDispatch: vi.fn(),
  getChatReadiness: vi.fn(),
  listEffects: vi.fn(),
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
    detailLoad: { id: 'agent-1', status: 'ready' as const, error: null as string | null },
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
    editAgent: testDoubles.editAgent,
  }),
}))

vi.mock('@/services/chatService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/chatService')>()
  return {
    ...actual,
    getChatReadiness: testDoubles.getChatReadiness,
  }
})

// agent-detail 挂载会拉取能力列表:默认空集合,避免 jsdom 真实 fetch 失败触发 effectsLoadFailed alert
vi.mock('@/services/pillInventoryService', () => ({
  listEffects: testDoubles.listEffects,
  updateEffects: vi.fn(),
  removeEffect: vi.fn(),
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
    testDoubles.agentState.agents = [...defaultAgents]
    testDoubles.agentState.error = null
    testDoubles.fetchSessions.mockResolvedValue(undefined)
    testDoubles.fetchAgents.mockResolvedValue(undefined)
    testDoubles.fetchAgent.mockResolvedValue(undefined)
    testDoubles.getChatReadiness.mockResolvedValue({
      active_agent_count: 2,
      ready_agent_ids: ['agent-1', 'agent-2'],
      can_create_single: true,
      can_create_group: true,
    })
    testDoubles.listEffects.mockResolvedValue({ effects_revision: 0, effects: [] })
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

  it('switching to an existing session uses the canonical URL and never creates one', async () => {
    const user = userEvent.setup()
    testDoubles.chatState.sessions = [{ ...singleSession, title: '历史会话' }]
    render(<ChatView />)

    // 列表项按钮的可访问名含会话标题 + 道人名,用标题正则定位
    await user.click(screen.getByRole('button', { name: /历史会话/ }))

    expect(testDoubles.push).toHaveBeenCalledOnce()
    expect(testDoubles.push).toHaveBeenCalledWith('/chat?session=11111111-1111-4111-8111-111111111111')
    expect(testDoubles.createSession).not.toHaveBeenCalled()
    expect(testDoubles.createGroupSession).not.toHaveBeenCalled()
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

  it('renders an agent avatar image in the launch picker', async () => {
    testDoubles.agentState.agents = [
      { ...defaultAgents[0], avatar: 'https://example.com/agent-one.png' },
      defaultAgents[1],
    ]
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))

    expect(await screen.findByRole('img', { name: 'Agent One' })).toHaveAttribute(
      'src',
      'https://example.com/agent-one.png',
    )
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
    expect(testDoubles.push).toHaveBeenCalledWith('/chat?session=11111111-1111-4111-8111-111111111111')
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
    expect(testDoubles.push).toHaveBeenCalledWith('/chat?session=11111111-1111-4111-8111-111111111111')
  })

  it('sends the group topic along with the members on confirm', async () => {
    const user = userEvent.setup()
    testDoubles.createGroupSession.mockResolvedValueOnce(groupSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.type(screen.getByPlaceholderText('mode.topicPlaceholder'), '丹道夜话')
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    await waitFor(() => {
      expect(testDoubles.createGroupSession).toHaveBeenCalledWith(['agent-1', 'agent-2'], '丹道夜话')
    })
    expect(testDoubles.push).toHaveBeenCalledWith('/chat?session=22222222-2222-4222-8222-222222222222')
  })

  it('creates a group with its avatar', async () => {
    const user = userEvent.setup()
    testDoubles.createGroupSession.mockResolvedValueOnce(groupSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.type(screen.getByLabelText('mode.avatarLabel'), 'https://example.com/group.png')
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    await waitFor(() => expect(testDoubles.createGroupSession).toHaveBeenCalledWith(
      ['agent-1', 'agent-2'], undefined, 'https://example.com/group.png',
    ))
  })

  it('blocks an invalid group avatar before creating', async () => {
    const user = userEvent.setup()
    render(<ChatView />)
    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.type(screen.getByLabelText('mode.avatarLabel'), 'javascript:alert(1)')
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    expect(screen.getByRole('alert')).toHaveTextContent('mode.avatarInvalid')
    expect(testDoubles.createGroupSession).not.toHaveBeenCalled()
  })

  it('preserves the typed topic across a failed group launch and its retry', async () => {
    const user = userEvent.setup()
    testDoubles.createGroupSession
      .mockRejectedValueOnce(new Error('group launch failed'))
      .mockResolvedValueOnce(groupSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.type(screen.getByPlaceholderText('mode.topicPlaceholder'), '丹道夜话')
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('group launch failed')
    expect(screen.getByPlaceholderText('mode.topicPlaceholder')).toHaveValue('丹道夜话')

    await user.click(screen.getByRole('button', { name: 'launch.retry' }))

    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })
    expect(testDoubles.createGroupSession).toHaveBeenNthCalledWith(1, ['agent-1', 'agent-2'], '丹道夜话')
    expect(testDoubles.createGroupSession).toHaveBeenNthCalledWith(2, ['agent-1', 'agent-2'], '丹道夜话')
  })

  it('blocks a group topic longer than 200 characters with zero API calls', async () => {
    const user = userEvent.setup()
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.type(screen.getByPlaceholderText('mode.topicPlaceholder'), '丹'.repeat(201))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    expect(screen.getByRole('alert')).toHaveTextContent('mode.topicTooLong')
    expect(testDoubles.createGroupSession).not.toHaveBeenCalled()
  })

  it('creates a group with an empty topic as undefined', async () => {
    const user = userEvent.setup()
    testDoubles.createGroupSession.mockResolvedValueOnce(groupSession)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'newSession' }))
    await user.click(screen.getByRole('button', { name: 'mode.group' }))
    await user.click(screen.getByRole('button', { name: /Agent One/ }))
    await user.click(screen.getByRole('button', { name: /Agent Two/ }))
    await user.click(screen.getByRole('button', { name: 'mode.confirm (2)' }))

    await waitFor(() => {
      expect(testDoubles.createGroupSession).toHaveBeenCalledWith(['agent-1', 'agent-2'], undefined)
    })
  })

  it('updates the group topic in the header and the directory after a successful rename', async () => {
    const user = userEvent.setup()
    const groupWithTitle = {
      ...groupSession,
      title: '群聊会话乙',
      members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ],
    }
    testDoubles.chatState.sessions = [groupWithTitle]
    testDoubles.chatState.currentSession = groupWithTitle
    testDoubles.renameSession.mockResolvedValue({ ...groupWithTitle, title: '新主题' })
    const view = render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'rename' }))
    await user.clear(screen.getByLabelText('renameLabel'))
    await user.type(screen.getByLabelText('renameLabel'), '新主题')
    await user.click(screen.getByRole('button', { name: 'saveRename' }))

    // 真实运行里 ChatContext 的 SET_SESSION_TITLE 同步 sessions + currentSession;
    // mock 下模拟同样更新,并用 rerender 触发一次消费方重渲染
    testDoubles.chatState.currentSession = { ...groupWithTitle, title: '新主题' }
    testDoubles.chatState.sessions = [{ ...groupWithTitle, title: '新主题' }]
    view.rerender(<ChatView />)

    expect(screen.queryByLabelText('renameLabel')).not.toBeInTheDocument()
    // 页头主题与目录条目同时更新
    expect(screen.getAllByText('新主题').length).toBeGreaterThan(1)
    expect(testDoubles.renameSession).toHaveBeenCalledWith(
      '22222222-2222-4222-8222-222222222222',
      '新主题',
    )
  })

  it('keeps the old topic in header and directory when a rename fails', async () => {
    const user = userEvent.setup()
    const groupWithTitle = {
      ...groupSession,
      title: '群聊会话乙',
      members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ],
    }
    testDoubles.chatState.sessions = [groupWithTitle]
    testDoubles.chatState.currentSession = groupWithTitle
    testDoubles.renameSession.mockResolvedValue(null)
    render(<ChatView />)

    await user.click(screen.getByRole('button', { name: 'rename' }))
    await user.clear(screen.getByLabelText('renameLabel'))
    await user.type(screen.getByLabelText('renameLabel'), '仍要保存的主题')
    await user.click(screen.getByRole('button', { name: 'saveRename' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('renameError')
    expect(screen.getByLabelText('renameLabel')).toHaveValue('仍要保存的主题')
    // 页头与目录仍是旧标题
    expect(screen.getAllByText('群聊会话乙').length).toBeGreaterThan(0)
  })

  it('shows the shared launch failure and retry on agent detail', async () => {
    const user = userEvent.setup()
    testDoubles.createSession.mockRejectedValueOnce(new ApiError(
      'Agent model needs configuration',
      409,
      { error_code: 'service.chat.model_unavailable' },
    ))
    render(<AgentDetailPage agentId="agent-1" />)

    await user.click(screen.getByRole('button', { name: 'startChatCta' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Agent model needs configuration')
    expect(screen.getByRole('button', { name: 'launch.retry' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'launch.modelSettings' })).toHaveAttribute('href', '/settings')
    expect(testDoubles.push).not.toHaveBeenCalled()
  })

  it('shows the authoritative single-session identity even when the agent list is empty', () => {
    testDoubles.agentState.agents = []
    testDoubles.chatState.sessions = [{
      ...singleSession,
      title: '与太上老君论道',
      agent_name: 'Agent One',
      agent_avatar: 'https://example.com/one.png',
    }]
    testDoubles.chatState.currentSession = testDoubles.chatState.sessions[0]
    render(<ChatView />)

    // 桌面目录父级展示服务端返回的道人名(不依赖道人列表)
    expect(screen.getAllByRole('button', { name: /Agent One/ }).length).toBeGreaterThan(0)
    // 单聊页头展示服务端名称与头像
    expect(screen.getAllByText('Agent One').length).toBeGreaterThan(0)
    expect(screen.getAllByRole('img', { name: 'Agent One' }).length).toBeGreaterThan(0)
    // 页头与目录绝不渲染会话 UUID
    expect(screen.queryByText(/11111111/)).not.toBeInTheDocument()
  })

  it('selects the group tab by default when the current session is a group', () => {
    testDoubles.chatState.sessions = [
      { ...singleSession, title: '单聊会话甲', agent_name: 'Agent One' },
      { ...groupSession, title: '群聊会话乙', members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ] },
    ]
    testDoubles.chatState.currentSession = testDoubles.chatState.sessions[1]
    render(<ChatView />)

    expect(screen.getByRole('tab', { name: 'tabs.group' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByText('单聊会话甲')).not.toBeInTheDocument()
    expect(screen.getAllByText('群聊会话乙').length).toBeGreaterThan(0)
  })

  it('filters the directory between the single and group tabs', async () => {
    const user = userEvent.setup()
    testDoubles.chatState.sessions = [
      { ...singleSession, agent_name: 'Agent One' },
      { ...groupSession, title: '群聊会话乙', members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ] },
    ]
    testDoubles.chatState.currentSession = testDoubles.chatState.sessions[0]
    render(<ChatView />)

    // 默认对谈:单聊子项可见,群聊被过滤
    expect(screen.getByText('untitledSingle')).toBeInTheDocument()
    expect(screen.queryByText('群聊会话乙')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'tabs.group' }))

    // 切换到围炉论道:只见群聊,单聊分组与道人父级消失
    expect(screen.getByText('群聊会话乙')).toBeInTheDocument()
    expect(screen.queryByText('untitledSingle')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Agent One/ })).not.toBeInTheDocument()
  })

  it('shows the same directory in the mobile sheet and closes it on selection', async () => {
    const user = userEvent.setup()
    testDoubles.chatState.sessions = [{
      ...singleSession,
      title: '移动端会话',
      agent_name: 'Agent One',
    }]
    testDoubles.chatState.currentSession = testDoubles.chatState.sessions[0]
    render(<ChatView />)

    // 打开移动端 Sheet(论道/旧录 切换)
    await user.click(screen.getByRole('tab', { name: '旧录' }))
    // 桌面侧栏与 Sheet 各渲染一份相同目录结构(道人父级分组按钮)
    expect(screen.getAllByRole('button', { name: /Agent One/ })).toHaveLength(2)

    // DOM 顺序: 桌面侧栏目录 → Sheet 目录 → 聊天主区;[1] 即 Sheet 内的目录子项
    await user.click(screen.getAllByText('移动端会话')[1])

    expect(testDoubles.push).toHaveBeenCalledWith('/chat?session=11111111-1111-4111-8111-111111111111')
    // 选择后 Sheet 关闭,只剩桌面目录
    expect(screen.getAllByRole('button', { name: /Agent One/ })).toHaveLength(1)
  })

  it('offers an @全体成员 entry at the top of the mention popup in a group chat', async () => {
    const user = userEvent.setup()
    testDoubles.chatState.currentSession = {
      ...groupSession,
      members: [
        { agent_id: 'agent-1', name: 'Agent One', proactivity: 50 },
        { agent_id: 'agent-2', name: 'Agent Two', proactivity: 50 },
      ],
    }
    render(<ChatView />)

    const input = await screen.findByLabelText('input.messageLabel')
    await user.type(input, '@')

    // 群聊 @ 补全浮层:全体成员置顶候选,点击后插入 @全体成员
    expect(screen.getByRole('option', { name: /everyone/ })).toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: /everyone/ }))
    expect(input).toHaveValue('@everyone ')
  })
})
