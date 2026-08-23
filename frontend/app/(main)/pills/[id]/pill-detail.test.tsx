import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PillDetailPage from '@/app/(main)/pills/[id]/pill-detail'
import { ApiError } from '@/services/api'
import type { Pill } from '@/services/types'

// ---- 可变的测试替身 ----
const td = vi.hoisted(() => ({
  push: vi.fn(),
  getPill: vi.fn(),
  clonePill: vi.fn(),
  fetchPill: vi.fn(),
  fetchPills: vi.fn(),
  addPill: vi.fn(),
  editPill: vi.fn(),
  removePill: vi.fn(),
  dispatchCalls: [] as Array<{ type: string; payload?: unknown }>,
  pillState: {
    pills: [] as Pill[],
    total: 0,
    currentPill: null as Pill | null,
    loading: false,
    error: null as string | null,
  },
  params: { id: 'pill-custom-1' },
}))

// 真实消息解析(命名空间与键都为点路径 + {value} 插值),用于英文长标签等真实文案断言
// 注意:namespace 可能是嵌套路径(如 'pill.editor'),需整段点路径逐层解析
function resolveMsg(messages: unknown, namespace: string, key: string, values?: Record<string, unknown>): string {
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

vi.mock('@/contexts/PillContext', () => ({
  usePill: () => ({
    state: td.pillState,
    dispatch: (action: { type: string; payload?: unknown }) => {
      td.dispatchCalls.push(action)
      if (action.type === 'UPDATE_PILL' || action.type === 'SET_CURRENT_PILL') {
        td.pillState.currentPill = action.payload as Pill | null
      }
      if (action.type === 'ADD_PILL') td.pillState.pills = [action.payload as Pill, ...td.pillState.pills]
      if (action.type === 'REMOVE_PILL' && td.pillState.currentPill?.id === action.payload) {
        td.pillState.currentPill = null
      }
      if (action.type === 'SET_ERROR') td.pillState.error = action.payload as string | null
    },
    fetchPill: td.fetchPill,
    fetchPills: td.fetchPills,
    addPill: td.addPill,
    editPill: td.editPill,
    removePill: td.removePill,
  }),
}))

// flow hook 直接 import getPill/clonePill;其余保留真实实现
vi.mock('@/services/pillService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/pillService')>()
  return { ...actual, getPill: td.getPill, clonePill: td.clonePill }
})

// 隔离道人绑定弹窗(其依赖 AgentContext 链路,与本测试无关)
vi.mock('@/components/bind-agent-modal', () => ({
  BindAgentModal: () => null,
}))

// ---- fixtures ----
const customPill: Pill = {
  id: 'pill-custom-1',
  name: '丹心妙语',
  description: '温润如茶',
  skill_schema: {
    identity_card: '一位温和的古代炼丹师',
    expression_dna: {
      sentence_length: 'medium',
      formality: 0.3,
      vocabulary: ['茶'],
      taboo_words: ['妄言'],
      rhythm: '舒缓',
      humor_type: '冷幽默',
      certainty_style: '留有余地',
      citation_habit: '喜引《道德经》',
    },
    mental_models: [
      { name: '阴阳转化', one_liner: '对立互化', application: '把对立概念互相转化', limitations: ['不适于线性问题'] },
    ],
    decision_heuristics: [{ condition: '遇到两难', action: '寻找第三选择', case: '两难案例' }],
    values: ['诚实优于圆滑'],
    anti_patterns: ['不堆砌空洞形容词'],
    honest_limits: ['对不熟悉领域坦言不知'],
    example_dialogues: [{ user: '何为道？', assistant: '道可道，非常道。' }],
    // 非编辑区块 + 真正未知键:保存时必须原样保留
    fusion_lineage: {
      parents: [{ uuid: 'p1', name: '母丹' }],
      operator: { id: 'op', name: '融合' },
      fused_at: '2026-01-01T00:00:00Z',
    },
    future_unknown: { nested: ['甲', '乙'] },
  },
  tags: ['古风'],
  author: '太上老君',
  version: '2.1.0',
  is_builtin: false,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const builtinPill: Pill = { ...customPill, id: 'pill-builtin-1', is_builtin: true }

const emptyPill: Pill = {
  ...customPill,
  id: 'pill-empty-1',
  name: '空丹',
  skill_schema: {},
  tags: [],
}

function setPill(pill: Pill | null, opts: { loading?: boolean; error?: string | null } = {}) {
  td.pillState.currentPill = pill
  td.pillState.loading = opts.loading ?? false
  td.pillState.error = opts.error ?? null
  td.params.id = pill?.id ?? 'pill-custom-1'
}

describe('PillDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    td.dispatchCalls.length = 0
    td.pillState.pills = []
    i18n.locale = 'zh-CN'
    td.fetchPill.mockResolvedValue(undefined)
  })

  afterEach(() => {
    cleanup()
    i18n.locale = 'zh-CN'
  })

  describe('五种状态', () => {
    it('加载态:显示加载提示', () => {
      setPill(null, { loading: true })
      render(<PillDetailPage />)
      expect(screen.getByText('加载金丹...')).toBeInTheDocument()
    })

    it('错误态:显示错误并可重试拉取', async () => {
      setPill(null, { error: '服务器内部错误' })
      const user = userEvent.setup()
      render(<PillDetailPage />)
      expect(screen.getByText('服务器内部错误')).toBeInTheDocument()
      const retry = screen.getByRole('button', { name: '重试' })
      td.fetchPill.mockClear()
      await user.click(retry)
      expect(td.fetchPill).toHaveBeenCalledWith('pill-custom-1')
    })

    it('空态:金丹不存在时给出返回入口', () => {
      setPill(null)
      render(<PillDetailPage />)
      expect(screen.getByText('金丹不存在或已被删除')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '返回金丹阁' })).toHaveAttribute('href', '/pills')
    })

    it('只读态:展示名称、全部 8 个 schema 区块与操作入口', () => {
      setPill(customPill)
      render(<PillDetailPage />)
      expect(screen.getByRole('heading', { name: '丹心妙语' })).toBeInTheDocument()
      for (const title of [
        '身份卡（第一人称）', '表达 DNA', '心智模型', '决策启发式',
        '价值观', '反模式（绝不做的事）', '诚实边界', '示例对话',
      ]) {
        expect(screen.getByText(title)).toBeInTheDocument()
      }
      expect(screen.getByRole('button', { name: '编辑丹方' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '销毁' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: '制作副本' })).toBeNull()
    })

    it('空字段:空 schema 的金丹在只读态展示各区块空状态', () => {
      setPill(emptyPill)
      render(<PillDetailPage />)
      expect(screen.getByRole('heading', { name: '空丹' })).toBeInTheDocument()
      // 各区块空兜底文案出现多次(每个空区块一处)
      expect(screen.getAllByText('此丹无结构化丹方。').length).toBeGreaterThan(0)
    })

    it('编辑态:点击编辑后出现表单与保存栏', async () => {
      setPill(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      expect(screen.getByDisplayValue('丹心妙语')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '保存金丹' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '取消' })).toBeInTheDocument()
    })
  })

  describe('内置金丹', () => {
    it('只读态显示「制作副本」,隐藏编辑/销毁/保存', () => {
      setPill(builtinPill)
      render(<PillDetailPage />)
      expect(screen.getByRole('button', { name: '制作副本' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: '编辑丹方' })).toBeNull()
      expect(screen.queryByRole('button', { name: '销毁' })).toBeNull()
      expect(screen.queryByRole('button', { name: '保存金丹' })).toBeNull()
      expect(screen.getByText('内置')).toBeInTheDocument()
    })

    it('点击制作副本:克隆成功后跳转到副本详情', async () => {
      setPill(builtinPill)
      const copy: Pill = { ...customPill, id: 'pill-copy-9', name: '丹心妙语 副本', is_builtin: false }
      td.clonePill.mockResolvedValue(copy)
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '制作副本' }))
      await waitFor(() => expect(td.push).toHaveBeenCalledWith('/pills/pill-copy-9'))
      expect(td.clonePill).toHaveBeenCalledWith('pill-builtin-1')
    })

    it('制作副本失败:给出错误反馈且不跳转', async () => {
      setPill(builtinPill)
      td.clonePill.mockRejectedValue(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '制作副本' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('制作副本失败，请重试')
      expect(td.push).not.toHaveBeenCalled()
    })
  })

  describe('自定义编辑与保存', () => {
    it('编辑覆盖基础信息与全部 schema 区块,保存时未知键原样保留', async () => {
      setPill(customPill)
      td.editPill.mockResolvedValue(customPill)
      td.getPill.mockResolvedValue(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))

      // 基础信息
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.clear(nameInput)
      await user.type(nameInput, '丹心妙语·改')
      // 身份卡
      const identity = screen.getByDisplayValue('一位温和的古代炼丹师')
      await user.clear(identity)
      await user.type(identity, '冷静的史官')
      // 表达 DNA
      await user.selectOptions(screen.getByRole('combobox'), 'long')
      // 高频词
      const vocab = screen.getByDisplayValue('茶')
      await user.clear(vocab)
      await user.type(vocab, '茗')
      // 心智模型名称
      const modelName = screen.getByDisplayValue('阴阳转化')
      await user.clear(modelName)
      await user.type(modelName, '系统论')
      // 决策启发式条件
      const condition = screen.getByDisplayValue('遇到两难')
      await user.clear(condition)
      await user.type(condition, '遇到不确定')
      // 价值观
      const value = screen.getByDisplayValue('诚实优于圆滑')
      await user.clear(value)
      await user.type(value, '诚实且清晰')
      // 反模式
      const anti = screen.getByDisplayValue('不堆砌空洞形容词')
      await user.clear(anti)
      await user.type(anti, '不夸大')
      // 诚实边界
      const limit = screen.getByDisplayValue('对不熟悉领域坦言不知')
      await user.clear(limit)
      await user.type(limit, '不知即言不知')
      // 示例对话
      const dialogueUser = screen.getByDisplayValue('何为道？')
      await user.clear(dialogueUser)
      await user.type(dialogueUser, '何为史？')

      await user.click(screen.getByRole('button', { name: '保存金丹' }))

      await waitFor(() => expect(td.editPill).toHaveBeenCalled())
      const [calledId, payload] = td.editPill.mock.calls[0]
      expect(calledId).toBe('pill-custom-1')
      expect(payload.name).toBe('丹心妙语·改')
      expect(payload.skill_schema.identity_card).toBe('冷静的史官')
      expect(payload.skill_schema.expression_dna.sentence_length).toBe('long')
      expect(payload.skill_schema.expression_dna.vocabulary).toEqual(['茗'])
      expect(payload.skill_schema.mental_models[0].name).toBe('系统论')
      expect(payload.skill_schema.decision_heuristics[0].condition).toBe('遇到不确定')
      expect(payload.skill_schema.values).toEqual(['诚实且清晰'])
      expect(payload.skill_schema.anti_patterns).toEqual(['不夸大'])
      expect(payload.skill_schema.honest_limits).toEqual(['不知即言不知'])
      expect(payload.skill_schema.example_dialogues[0].user).toBe('何为史？')
      // 开闭:非编辑区块与未知键原样保留
      expect(payload.skill_schema.fusion_lineage).toEqual(customPill.skill_schema.fusion_lineage)
      expect(payload.skill_schema.future_unknown).toEqual({ nested: ['甲', '乙'] })
    })

    it('保存成功:PUT 之后必须重新 GET,只有 GET 成功才退出编辑', async () => {
      setPill(customPill)
      const fresh: Pill = { ...customPill, name: '丹心妙语·改', updated_at: '2026-08-22T00:00:00Z' }
      td.editPill.mockResolvedValue({ ...customPill, name: 'PUT回显' })
      td.getPill.mockResolvedValue(fresh)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.clear(nameInput)
      await user.type(nameInput, '丹心妙语·改')
      await user.click(screen.getByRole('button', { name: '保存金丹' }))

      await waitFor(() => expect(td.getPill).toHaveBeenCalledWith('pill-custom-1'))
      // PUT 先于 GET
      expect(td.editPill.mock.invocationCallOrder[0]).toBeLessThan(td.getPill.mock.invocationCallOrder[0])
      // 退出编辑,回到只读,展示 GET 回源的最新名称
      await screen.findByRole('heading', { name: '丹心妙语·改' })
      expect(screen.queryByRole('button', { name: '保存金丹' })).toBeNull()
      expect(screen.getByRole('button', { name: '编辑丹方' })).toBeInTheDocument()
    })

    it('保存失败:保留全部字段与编辑态,错误可重试', async () => {
      setPill(customPill)
      td.editPill.mockRejectedValueOnce(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.clear(nameInput)
      await user.type(nameInput, '新名字')
      await user.click(screen.getByRole('button', { name: '保存金丹' }))

      // 错误反馈挂在操作区,可重试;字段与编辑态保留
      expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
      expect(screen.getByDisplayValue('新名字')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '保存金丹' })).toBeInTheDocument()
      expect(td.getPill).not.toHaveBeenCalled()

      // 重试成功后才退出编辑
      td.editPill.mockResolvedValue(customPill)
      td.getPill.mockResolvedValue({ ...customPill, name: '新名字' })
      await user.click(screen.getByRole('button', { name: '重试' }))
      await screen.findByRole('heading', { name: '新名字' })
    })

    it('保存后 GET 失败:不算完成,保留编辑态与草稿', async () => {
      setPill(customPill)
      td.editPill.mockResolvedValue(customPill)
      td.getPill.mockRejectedValue(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.clear(nameInput)
      await user.type(nameInput, '新名字')
      await user.click(screen.getByRole('button', { name: '保存金丹' }))

      expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
      expect(screen.getByDisplayValue('新名字')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '保存金丹' })).toBeInTheDocument()
    })
  })

  describe('删除', () => {
    it('自定义删除需二次确认:第一次点击仅进入待确认态', async () => {
      setPill(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '销毁' }))
      expect(td.removePill).not.toHaveBeenCalled()
      expect(screen.getByRole('button', { name: '再点一次确认销毁' })).toBeInTheDocument()
    })

    it('二次确认后删除成功并返回列表', async () => {
      setPill(customPill)
      td.removePill.mockResolvedValue(undefined)
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '销毁' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认销毁' }))
      await waitFor(() => expect(td.push).toHaveBeenCalledWith('/pills'))
      expect(td.removePill).toHaveBeenCalledWith('pill-custom-1')
    })

    it('删除失败:保留页面并给出错误反馈', async () => {
      setPill(customPill)
      td.removePill.mockRejectedValue(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '销毁' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认销毁' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('删除失败，请重试')
      expect(td.push).not.toHaveBeenCalled()
      // 仍在详情页(标题仍在)
      expect(screen.getByRole('heading', { name: '丹心妙语' })).toBeInTheDocument()
    })

    it('删除失败后点「重试」直接重发删除请求(不再停在待确认态)', async () => {
      setPill(customPill)
      td.removePill.mockRejectedValueOnce(new ApiError('服务器内部错误', 500))
      const user = userEvent.setup()
      render(<PillDetailPage />)

      await user.click(screen.getByRole('button', { name: '销毁' }))
      await user.click(screen.getByRole('button', { name: '再点一次确认销毁' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('删除失败，请重试')
      expect(td.removePill).toHaveBeenCalledTimes(1)

      // 用户已二次确认过,「重试」应直接重发删除,成功后返回列表
      td.removePill.mockResolvedValue(undefined)
      await user.click(screen.getByRole('button', { name: '重试' }))
      await waitFor(() => expect(td.removePill).toHaveBeenCalledTimes(2))
      await waitFor(() => expect(td.push).toHaveBeenCalledWith('/pills'))
    })
  })

  describe('未保存保护', () => {
    function dispatchBeforeUnload() {
      const event = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(event)
      return event
    }

    it('编辑且有改动时,浏览器关闭被阻止', async () => {
      setPill(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.type(nameInput, '改')

      expect(dispatchBeforeUnload().defaultPrevented).toBe(true)
    })

    it('只读/未改动时,浏览器关闭不被阻止', () => {
      setPill(customPill)
      render(<PillDetailPage />)
      expect(dispatchBeforeUnload().defaultPrevented).toBe(false)
    })

    it('保存成功后退出编辑,不再阻止离开', async () => {
      setPill(customPill)
      td.editPill.mockResolvedValue(customPill)
      td.getPill.mockResolvedValue(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.type(nameInput, '改')
      expect(dispatchBeforeUnload().defaultPrevented).toBe(true)

      await user.click(screen.getByRole('button', { name: '保存金丹' }))
      await waitFor(() => expect(screen.queryByRole('button', { name: '保存金丹' })).toBeNull())
      expect(dispatchBeforeUnload().defaultPrevented).toBe(false)
    })

    it('放弃修改后回到只读,不再阻止离开', async () => {
      setPill(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)
      await user.click(screen.getByRole('button', { name: '编辑丹方' }))
      const nameInput = screen.getByDisplayValue('丹心妙语')
      await user.type(nameInput, '改')
      expect(dispatchBeforeUnload().defaultPrevented).toBe(true)

      await user.click(screen.getByRole('button', { name: '取消' }))
      await waitFor(() => expect(screen.queryByRole('button', { name: '保存金丹' })).toBeNull())
      expect(dispatchBeforeUnload().defaultPrevented).toBe(false)
    })
  })

  describe('英文长标签', () => {
    it('en locale 下区块标题可换行(无 truncate),按钮不被覆盖', async () => {
      i18n.locale = 'en'
      setPill(customPill)
      const user = userEvent.setup()
      render(<PillDetailPage />)

      // 只读态长标题
      for (const title of [
        'Identity card (first person)', 'Expression DNA', 'Mental models',
        'Decision heuristics', 'Anti-patterns (never do)',
      ]) {
        const heading = screen.getByRole('heading', { name: title })
        expect(heading).not.toHaveClass('truncate')
      }
      // 进入编辑,保存按钮仍可达且可用
      await user.click(screen.getByRole('button', { name: 'Edit recipe' }))
      const save = screen.getByRole('button', { name: 'Save pill' })
      expect(save).toBeEnabled()
      // 编辑态长标签同样可换行
      const editHeading = screen.getByRole('heading', { name: 'Anti-patterns (never do)' })
      expect(editHeading).not.toHaveClass('truncate')
    })
  })
})
