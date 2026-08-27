import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentDetailPage from '@/app/(main)/agents/[id]/agent-detail'
import { ApiError } from '@/services/api'
import type { AgentDetail, DistillationDraft, Pill } from '@/services/types'

// ---- 可变的测试替身 ----
const td = vi.hoisted(() => ({
  push: vi.fn(),
  fetchAgent: vi.fn(),
  fetchPills: vi.fn(),
  getAgent: vi.fn(),
  updateAgent: vi.fn(),
  replacePills: vi.fn(),
  deleteAgent: vi.fn(),
  listModelOptions: vi.fn(),
  launchSingle: vi.fn(),
  launchRetry: vi.fn(),
  dispatchCalls: [] as Array<{ type: string; payload?: unknown }>,
  agentState: {
    agents: [] as AgentDetail[],
    total: 0,
    currentAgent: null as AgentDetail | null,
    loading: false,
    error: null as string | null,
  },
  pillState: {
    pills: [] as Pill[],
    total: 0,
    currentPill: null as Pill | null,
    loading: false,
    error: null as string | null,
  },
  launchState: { status: 'idle' } as
    | { status: 'idle' }
    | { status: 'submitting' }
    | { status: 'error'; message: string; errorCode?: string },
  params: { id: 'agent-1' },
}))

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
  useParams: () => ({ id: td.params.id }),
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

vi.mock('@/contexts/PillContext', () => ({
  usePill: () => ({
    state: td.pillState,
    fetchPills: td.fetchPills,
  }),
}))

vi.mock('@/services/agentService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentService')>()
  return {
    ...actual,
    getAgent: td.getAgent,
    updateAgent: td.updateAgent,
    replacePills: td.replacePills,
    deleteAgent: td.deleteAgent,
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

const nuwaDraft: DistillationDraft = {
  name: '女娲造人',
  description: '蒸馏候选',
  persona_summary: '悲天悯人的造物主',
  tags: ['神话'],
  skill_schema: { identity_card: '造物主' },
  sources: [{ title: '淮南子', url: 'https://example.com/huainanzi', dimension: 'persona' }],
  model: 'gpt-5',
  research: {
    evidence_level: 'standard',
    document_count: 1,
    domain_count: 1,
    total_characters: 2500,
    warnings: [],
  },
}

// 隔离真实蒸馏面板(走网络),替换为显式 apply 触发器
vi.mock('@/components/nuwa-distill-panel', () => ({
  NuwaDistillPanel: ({ onApply }: { onApply: (draft: DistillationDraft) => void }) => (
    <button type="button" onClick={() => onApply(nuwaDraft)}>
      nuwa-apply
    </button>
  ),
}))

// ---- fixtures ----
const pillA: Pill = {
  id: 'pill-a',
  name: '丹心妙语',
  description: '温润如茶',
  skill_schema: {},
  tags: [],
  version: '1.0.0',
  is_builtin: false,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}
const pillB: Pill = { ...pillA, id: 'pill-b', name: '浩然正气' }
const pillC: Pill = { ...pillA, id: 'pill-c', name: '清风徐来' }

const baseAgent: AgentDetail = {
  id: 'agent-1',
  name: '太上老君',
  avatar: '',
  personality: '沉稳如山',
  model_name: 'gpt-4o',
  status: 'active',
  proactivity: 60,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
  agent_pills: [
    { id: 'ap-1', agent_id: 'agent-1', pill_id: 'pill-a', weight: 2, sort_order: 1, created_at: '2026-08-20T00:00:00Z', pill: pillA },
    { id: 'ap-2', agent_id: 'agent-1', pill_id: 'pill-b', weight: 1, sort_order: 2, created_at: '2026-08-20T00:00:00Z', pill: pillB },
  ],
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

function setAgent(agent: AgentDetail | null, opts: { loading?: boolean; error?: string | null } = {}) {
  td.agentState.currentAgent = agent
  td.agentState.loading = opts.loading ?? false
  td.agentState.error = opts.error ?? null
  td.params.id = agent?.id ?? 'agent-1'
}

async function enterEditing(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: '编辑道人' }))
}

describe('AgentDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    td.dispatchCalls.length = 0
    td.agentState.agents = []
    td.pillState.pills = [pillA, pillB, pillC]
    td.launchState = { status: 'idle' }
    i18n.locale = 'zh-CN'
    td.fetchAgent.mockResolvedValue(undefined)
    td.fetchPills.mockResolvedValue(undefined)
    td.listModelOptions.mockResolvedValue(modelOptions)
    td.launchSingle.mockResolvedValue(true)
  })

  afterEach(() => {
    cleanup()
    i18n.locale = 'zh-CN'
  })

  describe('加载/错误/空三态', () => {
    it('静态导出 "_" 占位不触发任何拉取', () => {
      // Next output:export 将 [id] 预渲染为 "_";硬加载/深链时 useParams()
      // 短暂返回 "_"。它不是真 ID,绝不能 GET /agents/_(400 且弹错)。
      setAgent(null)
      td.params.id = '_'
      render(<AgentDetailPage />)
      expect(td.fetchAgent).not.toHaveBeenCalled()
      expect(td.fetchPills).not.toHaveBeenCalled()
    })

    it('加载态:显示加载提示', () => {
      setAgent(null, { loading: true })
      render(<AgentDetailPage />)
      expect(screen.getByText('加载道人...')).toBeInTheDocument()
    })

    it('错误态:显示错误并可重试拉取', async () => {
      setAgent(null, { error: '服务器内部错误' })
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      expect(screen.getByText('服务器内部错误')).toBeInTheDocument()
      td.fetchAgent.mockClear()
      await user.click(screen.getByRole('button', { name: '重试' }))
      expect(td.fetchAgent).toHaveBeenCalledWith('agent-1')
    })

    it('空态:道人不存在时给出返回入口', () => {
      setAgent(null)
      render(<AgentDetailPage />)
      expect(screen.getByText('道人不存在或已被删除')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '返回道人府' })).toHaveAttribute('href', '/agents')
    })
  })

  describe('只读态', () => {
    it('展示完整资料:名称/性格/模型/状态/主动性/服丹编排(顺序+剂量)', () => {
      setAgent(baseAgent)
      render(<AgentDetailPage />)
      expect(screen.getByRole('heading', { name: '太上老君' })).toBeInTheDocument()
      expect(screen.getByText('沉稳如山')).toBeInTheDocument()
      expect(screen.getByText('gpt-4o')).toBeInTheDocument()
      expect(screen.getByText('活跃')).toBeInTheDocument()
      expect(screen.getByText(/60/)).toBeInTheDocument()
      // 服丹编排按 sort_order 顺序展示金丹名与剂量(作用域限定在编排区块,排除语言模式卡片列表项)
      const pillsSection = screen.getByRole('heading', { name: '已服用金丹' }).closest('section')!
      const rows = within(pillsSection).getAllByRole('listitem')
      expect(rows[0]).toHaveTextContent('丹心妙语')
      expect(rows[0]).toHaveTextContent('2')
      expect(rows[1]).toHaveTextContent('浩然正气')
      expect(rows[1]).toHaveTextContent('1')
    })

    it('语言模式缓存状态:有效时展示涌现规则与丹性相冲', () => {
      setAgent(baseAgent)
      render(<AgentDetailPage />)
      expect(screen.getByText('已合成')).toBeInTheDocument()
      expect(screen.getByText('开门见山')).toBeInTheDocument()
      expect(screen.getByText(/丹性相冲/)).toBeInTheDocument()
      expect(screen.getByText('正式程度')).toBeInTheDocument()
    })

    it('语言模式缓存失效时展示待重新合成提示', () => {
      setAgent({
        ...baseAgent,
        language_pattern: { is_valid: false, system_prompt: '', emergence_rules: [], inner_tensions: [] },
      })
      render(<AgentDetailPage />)
      expect(screen.getByText('待重新合成')).toBeInTheDocument()
      expect(screen.getByText(/旧方所炼/)).toBeInTheDocument()
    })

    it('当前模型不在启用列表时展示失效警告', async () => {
      td.listModelOptions.mockResolvedValue([modelOptions[1]]) // 仅 deepseek-chat
      setAgent(baseAgent)
      render(<AgentDetailPage />)
      expect(await screen.findByText('模型失效')).toBeInTheDocument()
    })

    it('当前模型在启用列表时不展示失效警告', async () => {
      setAgent(baseAgent)
      render(<AgentDetailPage />)
      await waitFor(() => expect(td.listModelOptions).toHaveBeenCalled())
      expect(screen.queryByText('模型失效')).toBeNull()
    })

    it('inactive 道人发起会话按钮禁用并带原因提示', () => {
      setAgent({ ...baseAgent, status: 'inactive' })
      render(<AgentDetailPage />)
      const chat = screen.getByRole('button', { name: '开始论道' })
      expect(chat).toBeDisabled()
      expect(chat).toHaveAttribute('title', expect.stringContaining('停用'))
    })

    it('active 道人可发起会话', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await user.click(screen.getByRole('button', { name: '开始论道' }))
      expect(td.launchSingle).toHaveBeenCalledWith('agent-1')
    })
  })

  describe('编辑态', () => {
    it('进入编辑:字段回显,保存前编辑不发起任何 API', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
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
      expect(td.replacePills).not.toHaveBeenCalled()
      expect(td.getAgent).not.toHaveBeenCalled()
    })

    it('编辑态提供状态切换与头像字段', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)

      expect(screen.getByRole('button', { name: '活跃' })).toHaveAttribute('aria-pressed', 'true')
      await user.click(screen.getByRole('button', { name: '沉睡' }))
      expect(screen.getByRole('button', { name: '沉睡' })).toHaveAttribute('aria-pressed', 'true')

      const avatar = screen.getByLabelText('头像 URL')
      await user.type(avatar, 'https://example.com/laojun.png')
      expect(avatar).toHaveValue('https://example.com/laojun.png')
    })

    it('保存成功:基础资料 → 完整编排 → GET 回读,顺序与新编排正确', async () => {
      setAgent(baseAgent)
      const fresh: AgentDetail = { ...baseAgent, name: '改名老君', updated_at: '2026-08-23T00:00:00Z' }
      td.updateAgent.mockResolvedValue(fresh)
      td.replacePills.mockResolvedValue(fresh)
      td.getAgent.mockResolvedValue(fresh)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)

      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '改名老君')
      // 下移第一枚 + 新增一枚,编排变化进入同一保存事务
      await user.click(screen.getByRole('button', { name: /下移 丹心妙语/ }))
      await user.selectOptions(screen.getByRole('combobox', { name: '添加金丹' }), 'pill-c')
      await user.click(screen.getByRole('button', { name: '保存' }))

      await waitFor(() => expect(td.getAgent).toHaveBeenCalledWith('agent-1'))
      expect(td.updateAgent).toHaveBeenCalledWith('agent-1', expect.objectContaining({ name: '改名老君' }))
      expect(td.replacePills).toHaveBeenCalledWith('agent-1', [
        { pill_id: 'pill-b', weight: 1 },
        { pill_id: 'pill-a', weight: 2 },
        { pill_id: 'pill-c', weight: 1 },
      ])
      expect(td.updateAgent.mock.invocationCallOrder[0]).toBeLessThan(td.replacePills.mock.invocationCallOrder[0])
      expect(td.replacePills.mock.invocationCallOrder[0]).toBeLessThan(td.getAgent.mock.invocationCallOrder[0])
      // GET 回读后回到只读并展示最新名称
      await screen.findByRole('heading', { name: '改名老君' })
    })

    it('保存失败:错误反馈可重试且草稿不丢', async () => {
      setAgent(baseAgent)
      td.updateAgent.mockRejectedValueOnce(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '新名字')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
      expect(screen.getByDisplayValue('新名字')).toBeInTheDocument()
      expect(td.replacePills).not.toHaveBeenCalled()

      const fresh: AgentDetail = { ...baseAgent, name: '新名字' }
      td.updateAgent.mockResolvedValue(fresh)
      td.replacePills.mockResolvedValue(fresh)
      td.getAgent.mockResolvedValue(fresh)
      await user.click(screen.getByRole('button', { name: '重试' }))
      await screen.findByRole('heading', { name: '新名字' })
    })

    it('道号为空白时拒绝保存:字段错误展示且零 API', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '   ')
      await user.click(screen.getByRole('button', { name: '保存' }))

      expect(await screen.findByText('道号不能为空')).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
      expect(td.replacePills).not.toHaveBeenCalled()
    })

    it('恢复服务端版本:草稿回到基线但保持编辑态', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.clear(nameInput)
      await user.type(nameInput, '改名')

      await user.click(screen.getByRole('button', { name: '恢复服务端版本' }))
      expect(screen.getByDisplayValue('太上老君')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument()
    })

    it('取消:放弃修改回到只读,零 API', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.type(nameInput, '改')
      await user.click(screen.getByRole('button', { name: '取消' }))

      expect(screen.getByRole('heading', { name: '太上老君' })).toBeInTheDocument()
      expect(td.updateAgent).not.toHaveBeenCalled()
    })

    it('失效模型在编辑下拉中作为警告项保留,不静默丢失', async () => {
      td.listModelOptions.mockResolvedValue([modelOptions[1]])
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)

      const select = await screen.findByRole('combobox', { name: '选择模型' })
      const texts = Array.from(select.querySelectorAll('option')).map(o => o.textContent)
      expect(texts.some(text => text?.includes('gpt-4o') && text.includes('已失效'))).toBe(true)
      expect(select).toHaveValue('gpt-4o')
    })

    it('女娲蒸馏草稿显式应用后才落入表单,且不触碰服丹编排', async () => {
      setAgent(baseAgent)
      td.updateAgent.mockImplementation(async (_id, data) => ({ ...baseAgent, ...data }))
      td.replacePills.mockResolvedValue(baseAgent)
      td.getAgent.mockResolvedValue({ ...baseAgent, name: '女娲造人', personality: '悲天悯人的造物主' })
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      // 只读态不出现蒸馏入口
      expect(screen.queryByText('nuwa-apply')).toBeNull()

      await enterEditing(user)
      await user.click(screen.getByText('nuwa-apply'))
      expect(screen.getByDisplayValue('女娲造人')).toBeInTheDocument()
      expect(screen.getByDisplayValue('悲天悯人的造物主')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: '保存' }))
      await waitFor(() => expect(td.updateAgent).toHaveBeenCalled())
      expect(td.updateAgent).toHaveBeenCalledWith('agent-1', expect.objectContaining({
        name: '女娲造人',
        personality: '悲天悯人的造物主',
      }))
      // 编排原样提交,蒸馏不触碰
      expect(td.replacePills).toHaveBeenCalledWith('agent-1', [
        { pill_id: 'pill-a', weight: 2 },
        { pill_id: 'pill-b', weight: 1 },
      ])
    })
  })

  describe('删除与停用', () => {
    it('删除需二次确认:第一次点击不发起请求', async () => {
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      expect(td.deleteAgent).not.toHaveBeenCalled()
      expect(screen.getByRole('button', { name: '再点一次确认删除' })).toBeInTheDocument()
    })

    it('无历史:二次确认后删除成功并返回列表', async () => {
      setAgent(baseAgent)
      td.deleteAgent.mockResolvedValue(undefined)
      const user = userEvent.setup()
      render(<AgentDetailPage />)

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))
      await waitFor(() => expect(td.push).toHaveBeenCalledWith('/agents'))
      expect(td.deleteAgent).toHaveBeenCalledWith('agent-1')
      expect(td.dispatchCalls.some(a => a.type === 'REMOVE_AGENT' && a.payload === 'agent-1')).toBe(true)
    })

    it('有历史:409 冲突后显示会话数与停用动作,不再显示永久删除确认', async () => {
      setAgent(baseAgent)
      td.deleteAgent.mockRejectedValue(
        new ApiError('道人有 3 段会话历史，只能停用不能删除', 409, {
          code: 409,
          error_code: 'service.agent.delete_has_history',
          message: '道人有 3 段会话历史，只能停用不能删除',
          data: { session_count: 3 },
        }),
      )
      const user = userEvent.setup()
      render(<AgentDetailPage />)

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))

      expect(await screen.findByText(/3 段会话历史/)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '停用道人' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: '再点一次确认删除' })).toBeNull()
      expect(td.push).not.toHaveBeenCalled()
    })

    it('停用动作:调用 updateAgent 置 inactive 并回读详情', async () => {
      setAgent(baseAgent)
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
      render(<AgentDetailPage />)

      await user.click(screen.getByRole('button', { name: '删除道人' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认删除' }))
      await user.click(await screen.findByRole('button', { name: '停用道人' }))

      await waitFor(() =>
        expect(td.updateAgent).toHaveBeenCalledWith('agent-1', { status: 'inactive' }),
      )
      await screen.findByText('沉睡')
    })

    it('删除其他失败:错误反馈可重试,停留在详情页', async () => {
      setAgent(baseAgent)
      td.deleteAgent.mockRejectedValue(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<AgentDetailPage />)

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
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)
      await enterEditing(user)
      const nameInput = screen.getByDisplayValue('太上老君')
      await user.type(nameInput, '改')

      expect(dispatchBeforeUnload().defaultPrevented).toBe(true)
    })

    it('只读时不阻止离开', () => {
      setAgent(baseAgent)
      render(<AgentDetailPage />)
      expect(dispatchBeforeUnload().defaultPrevented).toBe(false)
    })
  })

  describe('英文布局', () => {
    it('en locale 下按钮与区块标题可达且不截断', async () => {
      i18n.locale = 'en'
      setAgent(baseAgent)
      const user = userEvent.setup()
      render(<AgentDetailPage />)

      // 只读态:长标题可换行
      const heading = screen.getByRole('heading', { name: 'Language pattern (pill nature)' })
      expect(heading).not.toHaveClass('truncate')

      // 编辑态:保存与恢复按钮可达
      await user.click(screen.getByRole('button', { name: 'Edit daoist' }))
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
      expect(screen.getByRole('button', { name: 'Restore server version' })).toBeInTheDocument()
    })
  })
})
