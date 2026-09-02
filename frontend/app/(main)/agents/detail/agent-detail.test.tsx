import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/detail/agent-detail'
import { ApiError } from '@/services/api'
import type { AgentDetail, AgentEffect, AgentMemory, PillItemListItem } from '@/services/types'

// ---- 固定 UUID(静态详情路由只接受 RFC 4122;置于 hoisted 块内避免提升期 TDZ)----
const td = vi.hoisted(() => {
  const IDS = {
    AGENT_ID: '11111111-1111-4111-8111-111111111111',
    OTHER_AGENT_ID: '22222222-2222-4222-8222-222222222222',
    ITEM_A_ID: '33333333-3333-4333-8333-333333333333',
    ITEM_B_ID: '44444444-4444-4444-8444-444444444444',
    ITEM_C_ID: '55555555-5555-4555-8555-555555555555',
    MEMORY_A_ID: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    SESSION_A_ID: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
  }
  return {
    IDS,
    push: vi.fn(),
    fetchAgent: vi.fn(),
    getAgent: vi.fn(),
    updateAgent: vi.fn(),
    deleteAgent: vi.fn(),
    fetchMemories: vi.fn(),
    createMemory: vi.fn(),
    updateMemory: vi.fn(),
    deleteMemory: vi.fn(),
    clearMemories: vi.fn(),
    listEffects: vi.fn(),
    updateEffects: vi.fn(),
    removeEffect: vi.fn(),
    listModelOptions: vi.fn(),
    launchSingle: vi.fn(),
    launchRetry: vi.fn(),
    // 服用金丹(库存选择弹窗):固定当前道人,不请求 listAgents
    listAgents: vi.fn(),
    listPillItems: vi.fn(),
    consumePill: vi.fn(),
    getOperation: vi.fn(),
    dispatchCalls: [] as Array<{ type: string; payload?: unknown }>,
    propId: IDS.AGENT_ID as string | undefined,
    agentState: {
      agents: [] as AgentDetail[],
      total: 0,
      currentAgent: null as AgentDetail | null,
      loading: false,
      error: null as string | null,
      detailLoad: { id: null as string | null, status: 'idle' as DetailStatus, error: null as string | null },
    },
    launchState: { status: 'idle' } as
      | { status: 'idle' }
      | { status: 'submitting' }
      | { status: 'error'; message: string; errorCode?: string },
  }
})

const {
  AGENT_ID,
  OTHER_AGENT_ID,
  ITEM_A_ID,
  ITEM_B_ID,
  ITEM_C_ID,
  MEMORY_A_ID,
  SESSION_A_ID,
} = td.IDS

type DetailStatus = 'idle' | 'loading' | 'ready' | 'not-found' | 'error'

// RFC 4122(含 version/variant 位);幂等 pending key 由 crypto.randomUUID 生成
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

// 真实消息解析(命名空间点路径 + {value} 插值)
function resolveMsg(
  messages: unknown,
  namespace: string,
  key: string,
  values?: Record<string, unknown>,
): string {
  let node: unknown = messages
  for (const part of `${namespace}.${key}`.split('.')) {
    if (node == null || typeof node !== 'object') {
      node = undefined
      break
    }
    node = (node as Record<string, unknown>)[part]
  }
  let text = typeof node === 'string' ? node : `${namespace}.${key}`
  if (values) for (const [k, v] of Object.entries(values)) text = text.split(`{${k}}`).join(String(v))
  return text
}

const i18n = vi.hoisted(() => ({ locale: 'zh-CN' as 'zh-CN' | 'en' }))

vi.mock('next-intl', async () => {
  const en = (await import('@/messages/en.json')).default
  const zh = (await import('@/messages/zh-CN.json')).default
  return {
    useTranslations: (namespace: string) => (key: string, values?: Record<string, unknown>) =>
      resolveMsg(i18n.locale === 'en' ? en : zh, namespace, key, values),
    useLocale: () => i18n.locale,
  }
})

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: td.push }),
}))

vi.mock('@/contexts/AgentContext', () => ({
  useAgent: () => ({
    state: td.agentState,
    dispatch: (action: { type: string; payload?: unknown }) => {
      td.dispatchCalls.push(action)
      if (action.type === 'UPDATE_AGENT' || action.type === 'SET_CURRENT_AGENT') {
        td.agentState.currentAgent = action.payload as AgentDetail | null
      }
      if (action.type === 'REMOVE_AGENT' && td.agentState.currentAgent?.id === action.payload) {
        td.agentState.currentAgent = null
      }
    },
    fetchAgent: td.fetchAgent,
    fetchAgents: vi.fn(),
  }),
}))

vi.mock('@/services/agentService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentService')>()
  return {
    ...actual,
    getAgent: td.getAgent,
    updateAgent: td.updateAgent,
    deleteAgent: td.deleteAgent,
    listAgents: td.listAgents,
    fetchAgentMemories: td.fetchMemories,
    createAgentMemory: td.createMemory,
    updateAgentMemory: td.updateMemory,
    deleteAgentMemory: td.deleteMemory,
    clearAgentMemories: td.clearMemories,
  }
})

vi.mock('@/services/pillInventoryService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/pillInventoryService')>()
  return {
    ...actual,
    listEffects: td.listEffects,
    updateEffects: td.updateEffects,
    removeEffect: td.removeEffect,
    listPillItems: td.listPillItems,
    consumePill: td.consumePill,
    getOperation: td.getOperation,
  }
})

vi.mock('@/services/modelService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/modelService')>()
  return { ...actual, options: td.listModelOptions }
})

vi.mock('@/hooks/use-chat-launch-flow', () => ({
  useChatLaunchFlow: () => ({
    state: td.launchState,
    launchSingle: td.launchSingle,
    launchGroup: vi.fn(),
    retry: td.launchRetry,
    reset: vi.fn(),
  }),
}))

// ---- fixtures ----
// 已吸收能力(服用快照;item_id 指向消耗后原实例,仅供详情跳转)
const effA: AgentEffect = {
  id: 'eff-a',
  name: '丹心妙语',
  schema: {},
  weight: 2,
  sort_order: 1,
  item_id: ITEM_A_ID,
  revision_id: 'rev-1',
  created_at: '2026-08-20T00:00:00Z',
}
const effB: AgentEffect = { ...effA, id: 'eff-b', name: '浩然正气', weight: 1, sort_order: 2, item_id: ITEM_B_ID }
const effC: AgentEffect = { ...effA, id: 'eff-c', name: '清风徐来', weight: 3, sort_order: 3, item_id: ITEM_C_ID }

const effectsData2 = { effects_revision: 2, effects: [effA, effB] }
// 服用产出:库存实例 → 服务端能力快照(weight 按服务端值落草稿)
const itemForConsume: PillItemListItem = {
  id: ITEM_C_ID,
  name: '清风徐来',
  state: 'available',
  recipe_id: 'recipe-x',
  revision_id: 'rev-9',
  revision: 1,
  created_at: '2026-08-20T00:00:00Z',
}
/** 契约准确的服用成功响应:operation_id 回显幂等 key */
function okConsumeResult(key: string): { operation_id: string; effect_id: string; consumed_item_ids: string[] } {
  return { operation_id: key, effect_id: 'eff-c', consumed_item_ids: [ITEM_C_ID] }
}

const baseAgent: AgentDetail = {
  id: AGENT_ID,
  name: '太上老君',
  avatar: '',
  personality: '沉稳如山',
  model_name: 'gpt-4o',
  status: 'active',
  proactivity: 60,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
  language_pattern: {
    is_valid: true,
    system_prompt: '你是太上老君',
    emergence_rules: ['开门见山'],
    inner_tensions: [{ dimension: '正式程度', description: '一枚偏正式一枚偏随意', severity: 'medium' }],
  },
}

const modelOptions = [
  { name: 'gpt-4o', display_name: 'GPT-4o', provider_name: 'openai', provider_display_name: 'OpenAI', is_default: true },
  { name: 'deepseek-chat', display_name: 'DeepSeek Chat', provider_name: 'deepseek', provider_display_name: 'DeepSeek', is_default: false },
]

// 本地记忆 fixture(来源会话为合法 UUID 供 chatSessionHref 校验)
const memoryA: AgentMemory = {
  uuid: MEMORY_A_ID,
  kind: 'user_fact',
  content: '用户喜欢围棋',
  keywords: ['围棋'],
  importance: 4,
  confidence: 0.9,
  pinned: true,
  status: 'active',
  source_session_id: SESSION_A_ID,
  source_message_id: '',
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

/**
 * 设置详情页状态机(组件按 agentId prop + agentState.detailLoad 决策)。
 * 缺省:agent 就绪 + 对应 UUID;status 单独传可覆盖(loading/error/not-found)。
 */
function setDetailState(opts: {
  agent?: AgentDetail | null
  status?: DetailStatus
  error?: string | null
  id?: string
}) {
  const id = opts.id ?? opts.agent?.id ?? AGENT_ID
  td.propId = id
  td.agentState.currentAgent = opts.agent ?? null
  td.agentState.detailLoad = {
    id,
    status: opts.status ?? (opts.agent ? 'ready' : 'idle'),
    error: opts.error ?? null,
  }
}

/** 渲染组件(统一入口:agentId 来自 td.propId) */
function renderPage() {
  return render(<AgentDetailPage agentId={td.propId} />)
}

async function enterEditing(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: '编辑道人' }))
}

describe('AgentDetailPage', () => {
  beforeEach(() => {
    vi.resetAllMocks() // 清实现与 once 队列(clearAllMocks 不清 once,跨测试泄漏)
    // 真实 pending-operations 实现:写 '[]'(全部操作完成态)而非 clear()——
    // readAll 在 key 缺失时回落模块级 memoryStore(会话内幂等兜底),clear() 清不掉它,会跨测试泄漏
    sessionStorage.setItem('alchemy_pending_operations', '[]')
    td.dispatchCalls.length = 0
    td.agentState.agents = []
    td.agentState.currentAgent = null
    td.agentState.detailLoad = { id: null, status: 'idle', error: null }
    td.propId = AGENT_ID
    td.launchState = { status: 'idle' }
    i18n.locale = 'zh-CN'
    td.fetchAgent.mockResolvedValue(undefined)
    td.listEffects.mockResolvedValue(effectsData2)
    td.updateEffects.mockResolvedValue({ effects_revision: 3, effects: [effA, effB] })
    td.removeEffect.mockResolvedValue(undefined)
    td.listPillItems.mockResolvedValue({ total: 0, items: [] })
    td.consumePill.mockResolvedValue({ operation_id: 'op-1' })
    td.getOperation.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
    td.listModelOptions.mockResolvedValue(modelOptions)
    td.launchSingle.mockResolvedValue(true)
    td.fetchMemories.mockResolvedValue([])
    td.createMemory.mockResolvedValue(undefined)
    td.updateMemory.mockResolvedValue(undefined)
    td.deleteMemory.mockResolvedValue(undefined)
    td.clearMemories.mockResolvedValue(undefined)
  })

  afterEach(() => {
    cleanup()
    i18n.locale = 'zh-CN'
  })

  describe('链接/加载/错误/不存在四态', () => {
    it('无效链接(缺 id):不发 API 且不误报已删除', () => {
      td.propId = undefined
      renderPage()
      expect(td.fetchAgent).not.toHaveBeenCalled()
      expect(td.listEffects).not.toHaveBeenCalled()
      expect(screen.getByText('道人链接无效，请返回道人府重新选择')).toBeInTheDocument()
      expect(screen.queryByText('道人不存在或已被删除')).toBeNull()
    })

    it('有效 id 发起详情与能力列表拉取', () => {
      renderPage()
      expect(td.fetchAgent).toHaveBeenCalledWith(AGENT_ID)
      expect(td.listEffects).toHaveBeenCalledWith(AGENT_ID)
    })

    it('加载态:显示加载提示', () => {
      setDetailState({ status: 'loading' })
      renderPage()
      expect(screen.getByText('加载道人...')).toBeInTheDocument()
    })

    it('快速切换:旧实体残留时按加载处理,不闪现旧道人', () => {
      const oldAgent: AgentDetail = { ...baseAgent, id: OTHER_AGENT_ID, name: '老前任' }
      setDetailState({ agent: oldAgent, status: 'loading' })
      renderPage()
      expect(screen.getByText('加载道人...')).toBeInTheDocument()
      expect(screen.queryByText('老前任')).toBeNull()
    })

    it('错误态:显示错误并可重试,重试用同一 UUID', async () => {
      setDetailState({ status: 'error', error: '服务器内部错误' })
      const user = userEvent.setup()
      renderPage()
      expect(screen.getByText('服务器内部错误')).toBeInTheDocument()
      td.fetchAgent.mockClear()
      await user.click(screen.getByRole('button', { name: '重试' }))
      expect(td.fetchAgent).toHaveBeenCalledWith(AGENT_ID)
    })

    it('404 态:只有 API 明确 404 才显示不存在,且给出返回入口', () => {
      setDetailState({ status: 'not-found' })
      renderPage()
      expect(screen.getByText('道人不存在或已被删除')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '返回道人府' })).toHaveAttribute('href', '/agents')
    })
  })

  describe('只读态', () => {
    it('展示完整资料:名称/性格/模型/状态/主动性/已吸收能力(顺序+剂量+来源跳转)', async () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      // 能力列表加载完成后(异步挂载)再断言;顶部 meta 条与区块标题各显示一次计数
      expect(await screen.findAllByText('2 项能力')).toHaveLength(2)
      expect(screen.getByRole('heading', { name: '太上老君' })).toBeInTheDocument()
      expect(screen.getByText('沉稳如山')).toBeInTheDocument()
      expect(screen.getByText('gpt-4o')).toBeInTheDocument()
      expect(screen.getByText('活跃')).toBeInTheDocument()
      expect(screen.getByText(/60/)).toBeInTheDocument()
      // 已吸收能力按 sort_order 顺序展示名称与剂量(作用域限定在能力区块,排除语言模式卡片列表项)
      const section = screen.getByRole('heading', { name: '已吸收能力' }).closest('section')!
      expect(within(section).getByText('2 项能力')).toBeInTheDocument()
      const rows = within(section).getAllByRole('listitem')
      expect(rows[0]).toHaveTextContent('丹心妙语')
      expect(rows[0]).toHaveTextContent('2')
      // 能力名点击跳转来源金丹实例详情(库存实例地址)
      expect(within(rows[0]).getByRole('link')).toHaveAttribute('href', `/pills/detail?id=${ITEM_A_ID}`)
      expect(rows[1]).toHaveTextContent('浩然正气')
      expect(rows[1]).toHaveTextContent('1')
    })

    it('能力列表加载失败:区块内展示错误并可重试', async () => {
      td.listEffects.mockRejectedValueOnce(new ApiError('服务器内部错误', 500))
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      expect(await screen.findByText('能力列表加载失败')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: '重试' }))
      await waitFor(() => expect(td.listEffects).toHaveBeenCalledTimes(2))
      await screen.findByText('丹心妙语')
    })

    it('语言模式缓存状态:有效时展示涌现规则与丹性相冲', () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      expect(screen.getByText('已合成')).toBeInTheDocument()
      expect(screen.getByText('开门见山')).toBeInTheDocument()
      expect(screen.getByText(/丹性相冲/)).toBeInTheDocument()
      expect(screen.getByText('正式程度')).toBeInTheDocument()
    })

    it('语言模式缓存失效时展示待重新合成提示', () => {
      setDetailState({
        agent: {
          ...baseAgent,
          language_pattern: { is_valid: false, system_prompt: '', emergence_rules: [], inner_tensions: [] },
        },
      })
      renderPage()
      expect(screen.getByText('待重新合成')).toBeInTheDocument()
      expect(screen.getByText(/旧方所炼/)).toBeInTheDocument()
    })

    it('当前模型不在启用列表时展示失效警告', async () => {
      td.listModelOptions.mockResolvedValue([modelOptions[1]]) // 仅 deepseek-chat
      setDetailState({ agent: baseAgent })
      renderPage()
      expect(await screen.findByText('模型失效')).toBeInTheDocument()
    })

    it('当前模型在启用列表时不展示失效警告', async () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      await waitFor(() => expect(td.listModelOptions).toHaveBeenCalled())
      expect(screen.queryByText('模型失效')).toBeNull()
    })

    it('inactive 道人发起会话按钮禁用并带原因提示', () => {
      setDetailState({ agent: { ...baseAgent, status: 'inactive' } })
      renderPage()
      const chat = screen.getByRole('button', { name: '开始论道' })
      expect(chat).toBeDisabled()
      expect(chat).toHaveAttribute('title', expect.stringContaining('沉睡'))
    })

    it('详情头部:有效头像显示图片,加载失败回退首字', () => {
      setDetailState({ agent: { ...baseAgent, avatar: 'https://example.com/laojun.png' } })
      renderPage()
      const img = screen.getByRole('img', { name: '太上老君' })
      expect(img).toHaveAttribute('src', 'https://example.com/laojun.png')
      fireEvent.error(img)
      expect(screen.queryByRole('img')).toBeNull()
      expect(screen.getByText('太')).toBeInTheDocument()
    })

    it('详情头部:空头像不创建图片,显示首字', () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      expect(screen.queryByRole('img')).toBeNull()
      expect(screen.getByText('太')).toBeInTheDocument()
    })

    it('active 道人可发起会话', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await user.click(screen.getByRole('button', { name: '开始论道' }))
      expect(td.launchSingle).toHaveBeenCalledWith(AGENT_ID)
    })
  })

  describe('编辑态', () => {
    it('进入编辑:字段回显,保存前编辑不发起任何写 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      expect(screen.getByDisplayValue('太上老君')).toBeInTheDocument()
      expect(screen.getByDisplayValue('沉稳如山')).toBeInTheDocument()
      expect(screen.getByRole('combobox', { name: '选择模型' })).toHaveValue('gpt-4o')
      expect(screen.getByRole('slider', { name: /主动性/ })).toHaveValue('60')

      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '改名')
      // 上移/下移、增删也只改草稿
      await user.click(screen.getByRole('button', { name: /下移 丹心妙语/ }))

      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.removeEffect).not.toHaveBeenCalled()
      expect(td.updateEffects).not.toHaveBeenCalled()
      expect(td.getAgent).not.toHaveBeenCalled()
      // 能力列表仅在挂载时拉取一次,无保存前置重读
      expect(td.listEffects).toHaveBeenCalledTimes(1)
    })

    it('编辑态提供状态切换与头像字段', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      expect(screen.getByRole('button', { name: '活跃' })).toHaveAttribute('aria-pressed', 'true')
      await user.click(screen.getByRole('button', { name: '沉睡' }))
      expect(screen.getByRole('button', { name: '沉睡' })).toHaveAttribute('aria-pressed', 'true')

      const avatar = screen.getByLabelText('头像 URL')
      await user.type(avatar, 'https://example.com/laojun.png')
      expect(avatar).toHaveValue('https://example.com/laojun.png')
    })

    it('表达欲说明使用导演层新语义:不暗示越健谈越写长文', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      // 新文案:控制群聊参与/追问意愿,不默认写长文,用户当轮要求优先
      expect(
        screen.getByText(
          '控制道人在群聊中主动参与和自然追问的意愿，不会让单条回复默认变成长文；用户当轮要求始终优先。',
        ),
      ).toBeInTheDocument()
      // 旧「越高越健谈」文案不得残留
      expect(screen.queryByText(/越高越健谈/)).not.toBeInTheDocument()
    })

    it('保存成功:基础资料 → 重读能力 → 全量编排 → GET 回读,顺序与新编排正确', async () => {
      setDetailState({ agent: baseAgent })
      const fresh: AgentDetail = { ...baseAgent, name: '改名老君', updated_at: '2026-08-23T00:00:00Z' }
      td.updateAgent.mockResolvedValue(fresh)
      // 挂载拉取(rev2 三能力,池含清风徐来) → 保存前重读(rev3,编排后)
      td.listEffects
        .mockResolvedValueOnce({ effects_revision: 2, effects: [effA, effB, effC] })
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
      td.getAgent.mockResolvedValue(fresh)
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '改名老君')
      // 停服清风徐来 → 下移丹心妙语 → 从池(未编排能力)重新添加,编排变化进入同一保存事务
      await user.click(screen.getByRole('button', { name: /停服 清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /下移 丹心妙语/ }))
      await user.selectOptions(screen.getByRole('combobox', { name: '添加金丹' }), 'eff-c')
      await user.click(screen.getByRole('button', { name: '保存' }))

      await waitFor(() => expect(td.getAgent).toHaveBeenCalledWith(AGENT_ID))
      expect(td.updateAgent).toHaveBeenCalledWith(AGENT_ID, expect.objectContaining({ name: '改名老君' }))
      // 全量编排:乐观锁 = 保存前重读的 effects_revision;提交集 = 草稿顺序(effectId/weight/sortOrder)
      expect(td.updateEffects).toHaveBeenCalledWith(AGENT_ID, 3, [
        { effectId: 'eff-b', weight: 1, sortOrder: 0 },
        { effectId: 'eff-a', weight: 2, sortOrder: 1 },
        { effectId: 'eff-c', weight: 1, sortOrder: 2 },
      ])
      expect(td.updateAgent.mock.invocationCallOrder[0]).toBeLessThan(
        td.listEffects.mock.invocationCallOrder[1],
      )
      expect(td.listEffects.mock.invocationCallOrder[1]).toBeLessThan(
        td.updateEffects.mock.invocationCallOrder[0],
      )
      expect(td.updateEffects.mock.invocationCallOrder[0]).toBeLessThan(
        td.getAgent.mock.invocationCallOrder[0],
      )
      // GET 回读后回到只读并展示最新名称
      await screen.findByRole('heading', { name: '改名老君' })
    })

    it('移除能力:save 走幂等 removeEffect,重读后全量编排不含被移除项', async () => {
      setDetailState({ agent: baseAgent })
      const fresh: AgentDetail = { ...baseAgent, name: '收丹老君', updated_at: '2026-08-23T00:00:00Z' }
      td.updateAgent.mockResolvedValue(fresh)
      // 挂载(rev2 双能力) → 移除后重读(rev3 仅剩丹心妙语)
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA] })
      // 写后真相以 updateEffects 返回值为准(save 第四步):提交集=草稿=仅剩丹心妙语,
      // 否则只读态基线会回源 beforeEach 的默认双能力 mock
      td.updateEffects.mockResolvedValue({ effects_revision: 3, effects: [effA] })
      td.getAgent.mockResolvedValue(fresh)
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      // 移除浩然正气(能力移除不返还金丹)
      await user.click(screen.getByRole('button', { name: /停服 浩然正气/ }))
      await user.click(screen.getByRole('button', { name: '保存' }))

      await waitFor(() => expect(td.getAgent).toHaveBeenCalledWith(AGENT_ID))
      // 幂等 key = 稳定 UUID(重试沿用同一 key,由 pending 记录保证)
      expect(td.removeEffect).toHaveBeenCalledWith(expect.stringMatching(UUID_RE), AGENT_ID, 'eff-b')
      expect(td.updateEffects).toHaveBeenCalledWith(AGENT_ID, 3, [
        { effectId: 'eff-a', weight: 2, sortOrder: 0 },
      ])
      // 成功后 pending 记录清理完毕
      expect(sessionStorage.getItem('alchemy_pending_operations')).toBe('[]')
      // 回读只读态:能力区块仅剩一枚
      await screen.findByRole('heading', { name: '收丹老君' })
      const section = screen.getByRole('heading', { name: '已吸收能力' }).closest('section')!
      expect(within(section).getAllByRole('listitem')).toHaveLength(1)
    })

    it('409 能力冲突:保留草稿并合并新能力,提示确认后重试成功', async () => {
      setDetailState({ agent: baseAgent })
      const fresh: AgentDetail = { ...baseAgent, updated_at: '2026-08-23T00:00:00Z' }
      td.updateAgent.mockResolvedValue(fresh)
      // 挂载(rev2) → 保存前重读(rev2) → 冲突刷新(rev3 新增清风徐来) → 重试重读(rev3)
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
        .mockResolvedValue({ effects_revision: 3, effects: [effA, effB, effC] })
      td.updateEffects
        .mockRejectedValueOnce(
          new ApiError('能力编排冲突', 409, {
            error_code: 'service.agent.effects_conflict',
          }),
        )
        .mockResolvedValue({ effects_revision: 3, effects: [effA, effB, effC] })
      td.getAgent.mockResolvedValue(fresh)
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      // 用户把丹心妙语剂量改为 5(并发窗口另一窗口吸收了清风徐来)
      const weightInput = screen.getByRole('spinbutton', { name: /丹心妙语/ })
      fireEvent.change(weightInput, { target: { value: '5' } })
      await user.click(screen.getByRole('button', { name: '保存' }))

      // 冲突提示 + 合并后仍保留用户编辑(草稿不丢)
      expect(await screen.findByText(/已刷新合并最新能力/)).toBeInTheDocument()
      expect(screen.getByDisplayValue('太上老君')).toBeInTheDocument()

      // 重新确认:乐观锁 = 刷新后 rev3;提交集 = 合并结果(保留行维持用户权重,新能力追加末尾)
      await user.click(screen.getByRole('button', { name: '重试' }))
      await waitFor(() => expect(td.getAgent).toHaveBeenCalledWith(AGENT_ID))
      expect(td.updateEffects).toHaveBeenCalledWith(AGENT_ID, 3, [
        { effectId: 'eff-a', weight: 5, sortOrder: 0 },
        { effectId: 'eff-b', weight: 1, sortOrder: 1 },
        { effectId: 'eff-c', weight: 3, sortOrder: 2 },
      ])
      await screen.findByRole('heading', { name: '太上老君' })
    })

    it('保存失败:错误反馈可重试且草稿不丢', async () => {
      setDetailState({ agent: baseAgent })
      td.updateAgent.mockRejectedValueOnce(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '新名字')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
      expect(screen.getByDisplayValue('新名字')).toBeInTheDocument()
      expect(td.updateEffects).not.toHaveBeenCalled()

      const fresh: AgentDetail = { ...baseAgent, name: '新名字' }
      td.updateAgent.mockResolvedValue(fresh)
      td.getAgent.mockResolvedValue(fresh)
      await user.click(screen.getByRole('button', { name: '重试' }))
      await screen.findByRole('heading', { name: '新名字' })
    })

    it('道号为空白时拒绝保存:字段错误展示且零 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '   ')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(await screen.findByText('道号不能为空')).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.updateEffects).not.toHaveBeenCalled()
    })

    it('取消:放弃修改回到只读,零 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.type(nameInput, '改')
      await user.click(screen.getByRole('button', { name: '取消' }))

      expect(screen.getByRole('heading', { name: '太上老君' })).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
    })

    it('失效模型在编辑下拉中作为警告项保留,不静默丢失', async () => {
      td.listModelOptions.mockResolvedValue([modelOptions[1]])
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const select = await screen.findByRole('combobox', { name: '选择模型' })
      const texts = Array.from(select.querySelectorAll('option')).map(o => o.textContent)
      expect(texts.some(text => text?.includes('gpt-4o') && text.includes('已失效'))).toBe(true)
      expect(select).toHaveValue('gpt-4o')
    })

    it('道人编辑页不渲染女娲蒸馏入口:只读态与编辑态均无面板', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      // 只读态不出现蒸馏入口
      expect(screen.queryByText('nuwa-apply')).toBeNull()

      await enterEditing(user)
      // 编辑态同样不允许出现女娲入口:道人能力应来自已服用的金丹快照,而非直接蒸馏注入
      expect(screen.queryByText('nuwa-apply')).toBeNull()
      expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument()
    })

    it('头像为相对路径时拒绝保存:字段错误展示且零 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      // 输入框下方常驻提示:只支持完整 URL 或 data:image 数据 URI(相对路径不可用)
      expect(screen.getByText('只支持完整 URL 或 data:image 数据 URI（相对路径不可用）')).toBeInTheDocument()

      const avatar = screen.getByLabelText('头像 URL')
      await user.clear(avatar)
      await user.type(avatar, '/avatar.png')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(
        await screen.findByText('头像仅支持完整 http/https URL 或 data:image 数据 URI'),
      ).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.updateEffects).not.toHaveBeenCalled()
    })

    it('头像为非法协议时拒绝保存:零 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const avatar = screen.getByLabelText('头像 URL')
      await user.clear(avatar)
      await user.type(avatar, 'javascript:alert(1)')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(
        await screen.findByText('头像仅支持完整 http/https URL 或 data:image 数据 URI'),
      ).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.updateEffects).not.toHaveBeenCalled()
    })

    it('头像为合法 data URI 时保存通过并随 updateAgent 提交', async () => {
      setDetailState({ agent: baseAgent })
      const fresh: AgentDetail = { ...baseAgent, avatar: 'data:image/png;base64,AAAA' }
      td.updateAgent.mockResolvedValue(fresh)
      td.getAgent.mockResolvedValue(fresh)
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const avatar = screen.getByLabelText('头像 URL')
      await user.clear(avatar)
      await user.type(avatar, 'data:image/png;base64,AAAA')
      await user.click(screen.getByRole('button', { name: '保存' }))

      await waitFor(() => expect(td.getAgent).toHaveBeenCalledWith(AGENT_ID))
      expect(td.updateAgent).toHaveBeenCalledWith(
        AGENT_ID,
        expect.objectContaining({ avatar: 'data:image/png;base64,AAAA' }),
      )
      await screen.findByRole('heading', { name: '太上老君' })
    })

    it('头像超长时拒绝保存:字段错误展示且零 API', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const avatar = screen.getByLabelText('头像 URL')
      // fireEvent 直改 value 绕过 maxLength,验证提交前校验兜底
      fireEvent.change(avatar, { target: { value: `https://example.com/${'a'.repeat(2050)}` } })
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(
        await screen.findByText('头像过长（URL 上限 2048 字符，data URI 上限 1500000 字符）'),
      ).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.updateEffects).not.toHaveBeenCalled()
    })
  })

  describe('服用金丹入口', () => {
    it('编辑态:能力为空仍显示服用按钮,点击固定当前道人打开库存弹窗且不请求 listAgents', async () => {
      setDetailState({ agent: baseAgent })
      td.listEffects.mockResolvedValue({ effects_revision: 1, effects: [] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      const openBtn = screen.getByRole('button', { name: '服用金丹' })
      expect(openBtn).toBeEnabled()

      await user.click(openBtn)
      // 弹窗以固定 agentId 打开:标题用当前道人名,不重新拉道人列表
      expect(await screen.findByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('为「太上老君」服用金丹')).toBeInTheDocument()
      expect(td.listAgents).not.toHaveBeenCalled()
      expect(td.listPillItems).toHaveBeenCalledWith({ page: 1, size: 24 })
    })

    it('编辑态:能力读取失败时服用按钮禁用', async () => {
      setDetailState({ agent: baseAgent })
      td.listEffects.mockRejectedValue(new Error('boom'))
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      expect(screen.getByRole('button', { name: '服用金丹' })).toBeDisabled()
    })
  })

  describe('服用金丹行为', () => {
    it('弹窗打开期间保存与取消禁用(防止旧编排覆盖刚吸收的能力)', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await screen.findByRole('dialog')

      expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
      // 弹窗内也有「取消」,页面取消按钮(disabled)与弹窗取消(可点)同时存在
      const cancels = screen.getAllByRole('button', { name: '取消' })
      expect(cancels.some(b => b.hasAttribute('disabled'))).toBe(true)
      expect(cancels.some(b => !b.hasAttribute('disabled'))).toBe(true)
    })

    it('服用成功:仅刷新能力并入草稿(新能力+服务端权重),弹窗关闭,保存不误删新能力', async () => {
      setDetailState({ agent: baseAgent })
      const freshAgent: AgentDetail = { ...baseAgent, name: '收丹老君', updated_at: '2026-08-23T00:00:00Z' }
      td.updateAgent.mockResolvedValue(freshAgent)
      td.getAgent.mockResolvedValue(freshAgent)
      td.consumePill.mockImplementation(async (key: string) => okConsumeResult(key))
      td.listPillItems.mockResolvedValue({ total: 1, items: [itemForConsume] })
      // 挂载(rev2 双能力) → 服用同步(rev3 三能力) → 保存前重读(rev3)
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await user.click(await screen.findByRole('radio', { name: /清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /确认服用 1 枚/ }))

      // 弹窗关闭 + 成功提示 + 能力已按新基线同步(只刷新能力,不重新拉道人)
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(screen.getByText('已服用 1 枚金丹，能力已保留')).toBeInTheDocument()
      expect(td.listEffects).toHaveBeenCalledTimes(2)
      expect(td.listAgents).not.toHaveBeenCalled()
      expect(td.consumePill).toHaveBeenCalledTimes(1)
      // 新能力进草稿(composer 出现清风徐来行)
      expect(screen.getByRole('button', { name: /上移 清风徐来/ })).toBeInTheDocument()

      // 保存:全量编排提交含新能力(weight=服务端值),不被后续保存误删
      await user.click(screen.getByRole('button', { name: '保存' }))
      await waitFor(() => expect(td.updateEffects).toHaveBeenCalled())
      expect(td.updateEffects).toHaveBeenCalledWith(AGENT_ID, 3, [
        { effectId: 'eff-a', weight: 2, sortOrder: 0 },
        { effectId: 'eff-b', weight: 1, sortOrder: 1 },
        { effectId: 'eff-c', weight: 3, sortOrder: 2 },
      ])
      await screen.findByRole('heading', { name: '收丹老君' })
    })

    it('服用成功后取消编辑:新能力仍在(取消编辑不返还金丹、不还原)', async () => {
      setDetailState({ agent: baseAgent })
      td.consumePill.mockImplementation(async (key: string) => okConsumeResult(key))
      td.listPillItems.mockResolvedValue({ total: 1, items: [itemForConsume] })
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await user.click(await screen.findByRole('radio', { name: /清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /确认服用 1 枚/ }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

      await user.click(screen.getByRole('button', { name: '取消' }))
      const section = screen.getByRole('heading', { name: '已吸收能力' }).closest('section')!
      expect(within(section).getByText('清风徐来')).toBeInTheDocument()
      // 再进编辑:新能力仍在 composer(取消编辑只丢弃未保存的草稿改动,不撤销服用)
      await enterEditing(user)
      expect(screen.getByRole('button', { name: /上移 清风徐来/ })).toBeInTheDocument()
    })

    it('草稿有未保存移除意图:点服用金丹不开弹窗,提示先保存', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      await user.click(screen.getByRole('button', { name: /停服 浩然正气/ }))

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(screen.getByText('此能力尚未保存移除，请先保存后再服用')).toBeInTheDocument()
    })

    it('结果未知:稍后核对关窗后页内提示并锁定保存;核对命中已提交则解锁并同步能力', async () => {
      setDetailState({ agent: baseAgent })
      td.consumePill.mockRejectedValue(new Error('network down'))
      td.listPillItems.mockResolvedValue({ total: 1, items: [itemForConsume] })
      // 弹窗 recover 查询失败 → uncertain;页内核对命中已提交(operation_id 回显)
      td.getOperation
        .mockRejectedValueOnce(new Error('offline'))
        .mockImplementation(async (key: string) => okConsumeResult(key))
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await user.click(await screen.findByRole('radio', { name: /清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /确认服用 1 枚/ }))
      expect(await screen.findByText('服用结果尚未确认，请先核对，勿重复操作')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: '稍后核对' }))

      // 页内提示(弹窗已关)+ 保存/再次服用锁定
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(screen.getByRole('alert')).toHaveTextContent('服用结果尚未确认，请先核对，勿重复操作')
      expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
      expect(screen.getByRole('button', { name: '服用金丹' })).toBeDisabled()

      // 核对:recoverOperation 命中 → 同步能力 → 解锁
      await user.click(screen.getByRole('button', { name: '核对' }))
      await waitFor(() => expect(td.listEffects).toHaveBeenCalledTimes(2))
      expect(td.consumePill).toHaveBeenCalledTimes(1) // 核对绝不重新服用
      await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
      expect(screen.getByRole('button', { name: '保存' })).toBeEnabled()
      expect(screen.getByRole('button', { name: /上移 清风徐来/ })).toBeInTheDocument()
      expect(sessionStorage.getItem('alchemy_pending_operations')).toBe('[]')
    })

    it('能力同步失败:关闭弹窗转页内「重试同步」,重试仅再次 GET 能力绝不重新服用', async () => {
      setDetailState({ agent: baseAgent })
      td.consumePill.mockImplementation(async (key: string) => okConsumeResult(key))
      td.listPillItems.mockResolvedValue({ total: 1, items: [itemForConsume] })
      // 挂载成功 → 服用同步失败 → 重试同步成功
      td.listEffects
        .mockResolvedValueOnce(effectsData2)
        .mockRejectedValueOnce(new Error('boom'))
        .mockResolvedValueOnce({ effects_revision: 3, effects: [effA, effB, effC] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await user.click(await screen.findByRole('radio', { name: /清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /确认服用 1 枚/ }))
      expect(await screen.findByText('金丹已服用，能力列表同步失败')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: '关闭' })) // X 关闭弹窗

      // 页内提示 + 重试同步,保存锁定
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(screen.getByText('金丹已服用，能力列表同步失败')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()

      await user.click(screen.getByRole('button', { name: '重试同步' }))
      await waitFor(() => expect(td.listEffects).toHaveBeenCalledTimes(3))
      expect(td.consumePill).toHaveBeenCalledTimes(1) // 同步重试绝不 POST consume
      await waitFor(() => expect(screen.queryByText('金丹已服用，能力列表同步失败')).toBeNull())
      expect(screen.getByRole('button', { name: '保存' })).toBeEnabled()
      expect(screen.getByRole('button', { name: /上移 清风徐来/ })).toBeInTheDocument()
    })

    it('重挂载读 pending:结果未知关窗后离开再回,只读态仍提示核对', async () => {
      setDetailState({ agent: baseAgent })
      td.consumePill.mockRejectedValue(new Error('network down'))
      td.getOperation.mockRejectedValue(new Error('offline'))
      td.listPillItems.mockResolvedValue({ total: 1, items: [itemForConsume] })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)

      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await user.click(await screen.findByRole('radio', { name: /清风徐来/ }))
      await user.click(screen.getByRole('button', { name: /确认服用 1 枚/ }))
      await screen.findByText('服用结果尚未确认，请先核对，勿重复操作')
      await user.click(screen.getByRole('button', { name: '稍后核对' }))

      // 模拟离开重进:重挂载后从 storage 读回 pending → 只读分支提示核对
      cleanup()
      renderPage()
      expect(await screen.findByText('服用结果尚未确认，请先核对，勿重复操作')).toBeInTheDocument()
    })

    it('道人切换:旧弹窗响应丢弃,弹窗关闭不写入新草稿', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      const { rerender } = render(<AgentDetailPage agentId={AGENT_ID} />)
      await enterEditing(user)
      // 等能力列表加载完成(服用按钮就绪)再打开弹窗
      await screen.findByRole('button', { name: /上移 丹心妙语/ })
      await user.click(screen.getByRole('button', { name: '服用金丹' }))
      await screen.findByRole('dialog')

      setDetailState({ agent: { ...baseAgent, id: OTHER_AGENT_ID, name: '沉睡道人' } })
      rerender(<AgentDetailPage agentId={OTHER_AGENT_ID} />)
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    })
  })

  describe('删除与沉睡', () => {
    it('删除需二次确认:第一次点击不发起请求', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      expect(td.deleteAgent).not.toHaveBeenCalled()
      expect(screen.getByRole('button', { name: '再点一次确认删除' })).toBeInTheDocument()
    })

    it('无历史:二次确认后删除成功并返回列表', async () => {
      setDetailState({ agent: baseAgent })
      td.deleteAgent.mockResolvedValue(undefined)
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))
      await waitFor(() => expect(td.push).toHaveBeenCalledWith('/agents'))
      expect(td.deleteAgent).toHaveBeenCalledWith(AGENT_ID)
      expect(td.dispatchCalls.some(a => a.type === 'REMOVE_AGENT' && a.payload === AGENT_ID)).toBe(true)
    })

    it('有历史:409 冲突后显示会话数与沉睡动作,不再显示永久删除确认', async () => {
      setDetailState({ agent: baseAgent })
      td.deleteAgent.mockRejectedValue(
        new ApiError('道人有 3 段会话历史，只能沉睡不能删除', 409, {
          code: 409,
          error_code: 'service.agent.delete_has_history',
          message: '道人有 3 段会话历史，只能沉睡不能删除',
          data: { session_count: 3 },
        }),
      )
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))

      expect(await screen.findByText(/3 段会话历史/)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '沉睡道人' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: '再点一次确认删除' })).toBeNull()
      expect(td.push).not.toHaveBeenCalled()
    })

    it('沉睡动作:调用 updateAgent 置 inactive 并回读详情', async () => {
      setDetailState({ agent: baseAgent })
      td.deleteAgent.mockRejectedValue(
        new ApiError('有历史', 409, {
          error_code: 'service.agent.delete_has_history',
          data: { session_count: 2 },
        }),
      )
      const inactiveAgent: AgentDetail = { ...baseAgent, status: 'inactive' }
      td.updateAgent.mockResolvedValue(inactiveAgent)
      td.getAgent.mockResolvedValue(inactiveAgent)
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))
      await user.click(await screen.findByRole('button', { name: '沉睡道人' }))

      await waitFor(() =>
        expect(td.updateAgent).toHaveBeenCalledWith(AGENT_ID, { status: 'inactive' }),
      )
      await screen.findByText('沉睡')
    })

    it('删除其他失败:错误反馈可重试,停留在详情页', async () => {
      setDetailState({ agent: baseAgent })
      td.deleteAgent.mockRejectedValue(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      renderPage()

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('删除失败')
      expect(td.push).not.toHaveBeenCalled()
      expect(screen.getByRole('heading', { name: '太上老君' })).toBeInTheDocument()
    })
  })

  describe('未保存保护', () => {
    function dispatchBeforeUnload() {
      const event = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(event)
      return event
    }

    it('编辑且有改动时,浏览器关闭被阻止', async () => {
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.type(nameInput, '改')

      expect(dispatchBeforeUnload().defaultPrevented).toBe(true)
    })

    it('只读时不阻止离开', () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      expect(dispatchBeforeUnload().defaultPrevented).toBe(false)
    })
  })

  describe('英文布局', () => {
    it('en locale 下按钮与区块标题可达且不截断', async () => {
      i18n.locale = 'en'
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()

      // 只读态:长标题可换行
      const heading = screen.getByRole('heading', { name: 'Language pattern (pill nature)' })
      expect(heading).not.toHaveClass('truncate')

      // 编辑态:保存按钮可达
      await user.click(screen.getByRole('button', { name: 'Edit daoist' }))
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    })
  })

  describe('本地记忆管理区', () => {
    it('未返回 memory_enabled(视为关闭):不请求列表,仅展示开关与提示', () => {
      setDetailState({ agent: baseAgent })
      renderPage()
      expect(screen.getByRole('heading', { name: '本地记忆' })).toBeInTheDocument()
      expect(td.fetchMemories).not.toHaveBeenCalled()
      expect(
        screen.getByText('本地记忆已关闭，道人不会在论道中检索或沉淀记忆')
      ).toBeInTheDocument()
    })

    it('开启时加载列表:展示类型/内容/置顶徽标/重要性/置信度', async () => {
      td.fetchMemories.mockResolvedValue([memoryA])
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      renderPage()
      expect(await screen.findByText('用户喜欢围棋')).toBeInTheDocument()
      expect(td.fetchMemories).toHaveBeenCalledWith(AGENT_ID, undefined)
      // 类型标签同时出现在筛选 tab 与卡片徽标
      expect(screen.getAllByText('用户事实').length).toBeGreaterThan(0)
      expect(screen.getByText('置顶')).toBeInTheDocument()
      expect(screen.getByText(/重要性 4 · 置信度 90%/)).toBeInTheDocument()
      // 有来源会话时渲染跳转链接(规范地址 /chat?session=)
      const link = screen.getByRole('link', { name: '跳转来源' })
      expect(link).toHaveAttribute('href', `/chat?session=${SESSION_A_ID}`)
    })

    it('kind 筛选:点击类型标签后按类型重新请求', async () => {
      td.fetchMemories.mockResolvedValue([])
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('暂无记忆，点击「新建记忆」录入第一条')
      await user.click(screen.getByRole('button', { name: '用户偏好' }))
      expect(td.fetchMemories).toHaveBeenLastCalledWith(AGENT_ID, 'user_preference')
    })

    it('开关切换:调用 updateAgent 携带 memory_enabled 并加载列表', async () => {
      td.updateAgent.mockResolvedValue(baseAgent)
      td.fetchMemories.mockResolvedValue([memoryA])
      setDetailState({ agent: baseAgent })
      const user = userEvent.setup()
      renderPage()
      await user.click(screen.getByRole('switch', { name: '启用本地记忆' }))
      expect(td.updateAgent).toHaveBeenCalledWith(AGENT_ID, {}, true)
      await waitFor(() => expect(screen.getByText('用户喜欢围棋')).toBeInTheDocument())
    })

    it('新建记忆:提交表单后调用 createAgentMemory 并更新列表', async () => {
      const created: AgentMemory = {
        ...memoryA,
        uuid: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc',
        content: '用户喜欢喝茶',
        keywords: ['茶'],
        importance: 3,
        pinned: false,
      }
      td.fetchMemories.mockResolvedValue([])
      td.createMemory.mockResolvedValue(created)
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('暂无记忆，点击「新建记忆」录入第一条')
      await user.click(screen.getByRole('button', { name: '新建记忆' }))
      await user.type(screen.getByPlaceholderText('内容'), '用户喜欢喝茶')
      await user.type(screen.getByPlaceholderText('多个关键词用逗号分隔，最多 12 个'), '茶')
      await user.click(screen.getByRole('button', { name: '保存记忆' }))
      expect(td.createMemory).toHaveBeenCalledWith(AGENT_ID, {
        kind: 'user_fact',
        content: '用户喜欢喝茶',
        keywords: ['茶'],
        importance: 3,
        pinned: false,
      })
      expect(await screen.findByText('用户喜欢喝茶')).toBeInTheDocument()
    })

    it('置顶/取消置顶:调用 updateAgentMemory 仅携带 pinned', async () => {
      td.fetchMemories.mockResolvedValue([memoryA])
      td.updateMemory.mockResolvedValue({ ...memoryA, pinned: false })
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('用户喜欢围棋')
      await user.click(screen.getByRole('button', { name: '取消置顶' }))
      expect(td.updateMemory).toHaveBeenCalledWith(AGENT_ID, MEMORY_A_ID, { pinned: false })
    })

    // 删除/清空走应用内确认框(WKWebView 不实现 window.confirm,桌面端
    // confirm 恒 false 会让操作静默失效;对话框文案来自 memory namespace)
    it('删除:应用内确认后调用 deleteAgentMemory 并从列表移除', async () => {
      td.fetchMemories.mockResolvedValue([memoryA])
      td.deleteMemory.mockResolvedValue(undefined)
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('用户喜欢围棋')
      await user.click(screen.getByRole('button', { name: '删除' }))
      const dialog = screen.getByRole('alertdialog')
      expect(dialog).toHaveTextContent('确定删除这条记忆吗？删除后不可恢复。')
      await user.click(within(dialog).getByRole('button', { name: '确认删除' }))
      expect(td.deleteMemory).toHaveBeenCalledWith(AGENT_ID, MEMORY_A_ID)
      await waitFor(() => expect(screen.queryByText('用户喜欢围棋')).toBeNull())
    })

    it('删除:取消则保留记忆且零 API', async () => {
      td.fetchMemories.mockResolvedValue([memoryA])
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('用户喜欢围棋')
      await user.click(screen.getByRole('button', { name: '删除' }))
      await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '取消' }))
      expect(td.deleteMemory).not.toHaveBeenCalled()
      expect(screen.queryByRole('alertdialog')).toBeNull()
      expect(screen.getByText('用户喜欢围棋')).toBeInTheDocument()
    })

    it('清空:应用内确认后调用 clearAgentMemories 并清空列表', async () => {
      td.fetchMemories.mockResolvedValue([memoryA])
      td.clearMemories.mockResolvedValue(undefined)
      setDetailState({ agent: { ...baseAgent, memory_enabled: true } })
      const user = userEvent.setup()
      renderPage()
      await screen.findByText('用户喜欢围棋')
      await user.click(screen.getByRole('button', { name: '清空记忆' }))
      const dialog = screen.getByRole('alertdialog')
      expect(dialog).toHaveTextContent('确定清空该道人的全部记忆吗？此操作不可逆。')
      await user.click(within(dialog).getByRole('button', { name: '确认清空' }))
      expect(td.clearMemories).toHaveBeenCalledWith(AGENT_ID)
      await waitFor(() => expect(screen.queryByText('用户喜欢围棋')).toBeNull())
    })
  })
})
