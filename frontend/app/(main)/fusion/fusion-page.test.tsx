import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import FusionPage from '@/app/(main)/fusion/page'
import { ApiError } from '@/services/api'
import type { FusionPreview, PillItemListItem } from '@/services/types'
import type { SystemConfig } from '@/services/systemService'

const td = vi.hoisted(() => ({
  push: vi.fn(),
  listPillItems: vi.fn(),
  previewFusion: vi.fn(),
  confirmFusion: vi.fn(),
  getOperation: vi.fn(),
  listProviders: vi.fn(),
  listModels: vi.fn(),
  updateModel: vi.fn(),
  getSystemConfig: vi.fn(),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: td.push }),
}))

// 真实消息解析(命名空间点路径 + {value} 插值),与组件测试同模式;
// FusionPreviewModal 渲染真实组件,文案断言需要真实中文
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

vi.mock('next-intl', async () => {
  const zh = (await import('@/messages/zh-CN.json')).default
  return {
    useTranslations: (namespace: string) => (key: string, values?: Record<string, unknown>) =>
      resolveMsg(zh, namespace, key, values),
    useLocale: () => 'zh-CN',
  }
})

vi.mock('@/services/pillInventoryService', () => ({
  listPillItems: td.listPillItems,
  previewFusion: td.previewFusion,
  confirmFusion: td.confirmFusion,
  getOperation: td.getOperation,
}))

vi.mock('@/services/modelService', () => ({
  listProviders: td.listProviders,
  listModels: td.listModels,
  updateModel: td.updateModel,
}))

vi.mock('@/services/systemService', () => ({
  getSystemConfig: td.getSystemConfig,
}))

// 炉火动画内部是 canvas,jsdom 跑不了;替换为桩
vi.mock('@/components/alchemy/bagua-furnace', () => ({
  BaguaFurnace: () => <div data-testid="furnace" />,
}))

const ITEM_A = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const ITEM_B = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const PREVIEW_ID = 'pppppppp-pppp-4ppp-8ppp-pppppppppppp'

const poolItem = (id: string, name: string): PillItemListItem => ({
  id,
  name,
  state: 'available',
  recipe_id: 'r-1',
  revision_id: 'rv-1',
  revision: 1,
  created_at: '2026-08-20T00:00:00Z',
})

const preview: FusionPreview = {
  preview_id: PREVIEW_ID,
  expires_at: '2026-08-31T17:00:00Z',
  name: '新丹',
  description: '融合而生',
  skill_schema: { name: '新丹', description: '融合而生', system_prompt: 'x' },
  operator: { id: 'o-1', name: '金木水火土' },
  model: 'gpt-fusion',
  degraded: false,
}

const configuredConfig: SystemConfig = {
  version: '1.0.0',
  models: [],
  default_model: 'm',
  synthesis_model: 'm',
  fusion_model: 'm',
  fusion_model_info: {
    configured: true,
    model_name: 'gpt-fusion',
    model_display_name: 'FusionModel X',
    provider_name: 'openai',
    provider_display_name: 'OpenAI',
  },
}

/** 选择材料并开始融合（预览），等预览弹窗出现 */
async function selectTwoAndFuse(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: /文言文丹/ }))
  await user.click(screen.getByRole('button', { name: /俳句丹/ }))
  await user.click(screen.getByRole('button', { name: '开始融合' }))
  // 预览弹窗出现（真实 FusionPreviewModal）
  expect(await screen.findByText('不消耗材料；保存时消耗')).toBeInTheDocument()
}

/** 等待某文案消失（findBy* 没有 query 反向 API，用轮询辅助） */
async function waitForQueryByTextGone(text: string) {
  for (let i = 0; i < 50; i++) {
    if (!screen.queryByText(text)) return
    await new Promise((r) => setTimeout(r, 10))
  }
  throw new Error(`text still present: ${text}`)
}

describe('FusionPage 融合炉（两阶段：预览不消耗 → 确认原子消耗）', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    vi.resetAllMocks()
    sessionStorage.clear()
    td.listPillItems.mockResolvedValue({
      total: 2,
      items: [poolItem(ITEM_A, '文言文丹'), poolItem(ITEM_B, '俳句丹')],
    })
    td.listProviders.mockResolvedValue({ list: [{ id: 'p1' }], total: 1 })
    td.getSystemConfig.mockResolvedValue(configuredConfig)
    td.previewFusion.mockResolvedValue(preview)
  })

  it('初始加载展示已配置融合模型与材料池', async () => {
    render(<FusionPage />)

    expect(await screen.findByText(/FusionModel X/)).toBeInTheDocument()
    expect(screen.getByText(/OpenAI/)).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /文言文丹/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /俳句丹/ })).toBeInTheDocument()
    // 库存计数：已加载 / 总数
    expect(screen.getByText(/2 \/ 2/)).toBeInTheDocument()
  })

  it('未配置融合模型时展示醒目警告', async () => {
    td.getSystemConfig.mockRejectedValue(new Error('config unavailable'))
    render(<FusionPage />)

    expect(await screen.findByText('尚未配置金丹融合专用模型')).toBeInTheDocument()
  })

  it('材料池支持分页：加载更多追加下一页，加载完按钮消失', async () => {
    const user = userEvent.setup()
    td.listPillItems
      .mockResolvedValueOnce({ total: 3, items: [poolItem(ITEM_A, '文言文丹'), poolItem(ITEM_B, '俳句丹')] })
      .mockResolvedValueOnce({ total: 3, items: [poolItem('cccccccc-cccc-4ccc-8ccc-cccccccccccc', '佛跳墙丹')] })
    render(<FusionPage />)
    await screen.findByRole('button', { name: /文言文丹/ })

    await user.click(screen.getByRole('button', { name: '加载更多' }))

    expect(await screen.findByRole('button', { name: /佛跳墙丹/ })).toBeInTheDocument()
    expect(td.listPillItems).toHaveBeenLastCalledWith({ page: 2, size: 48 })
    // 已加载完总数，加载更多按钮消失
    expect(screen.queryByRole('button', { name: '加载更多' })).toBeNull()
  })

  it('开始融合 = 预览：只调 previewFusion 不消耗材料，弹出预览', async () => {
    const user = userEvent.setup()
    render(<FusionPage />)
    await screen.findByRole('button', { name: /文言文丹/ })

    await selectTwoAndFuse(user)

    expect(td.previewFusion).toHaveBeenCalledTimes(1)
    expect(td.previewFusion).toHaveBeenCalledWith([ITEM_A, ITEM_B])
    // 预览不消耗：材料仍在融合槽中（移除按钮可点）
    expect(screen.getByRole('button', { name: 'remove 文言文丹' })).toBeInTheDocument()
  })

  it('预览后保存只调 confirm', async () => {
    const user = userEvent.setup()
    td.confirmFusion.mockResolvedValue({
      operation_id: 'op-1',
      recipe_id: 'rrrrrrrr-rrrr-4rrr-8rrr-rrrrrrrrrrrr',
      revision_id: 'rv-new',
      item_ids: ['new-item'],
      consumed_item_ids: [ITEM_A, ITEM_B],
    })
    render(<FusionPage />)
    await screen.findByRole('button', { name: /文言文丹/ })
    await selectTwoAndFuse(user)

    // 保存入库：只调 confirm（幂等 key），不调 createPill/deletePill
    await user.click(screen.getByRole('button', { name: '保存入库' }))
    await waitForQueryByTextGone('不消耗材料；保存时消耗')

    expect(td.confirmFusion).toHaveBeenCalledTimes(1)
    const [key, previewId, name, description] = td.confirmFusion.mock.calls[0]
    expect(previewId).toBe(PREVIEW_ID)
    expect(name).toBe('新丹')
    expect(description).toBe('融合而生')
    expect(key).toEqual(expect.stringMatching(/^[0-9a-f-]{36}$/))
    // 成功：预览关闭、材料清空、材料池重读（旧材料已消耗、新丹已入库）
    expect(screen.queryByText('不消耗材料；保存时消耗')).toBeNull()
    expect(td.listPillItems).toHaveBeenCalledTimes(2)
    expect(td.listPillItems).toHaveBeenLastCalledWith({ page: 1, size: 48 })
    // 未编辑时保存：只关闭不跳详情（goEdit=false 分支）
    expect(td.push).not.toHaveBeenCalled()
  })

  it('保存失败保留材料：错误弹窗展示，pending key 不换（重试同 key）', async () => {
    const user = userEvent.setup()
    td.confirmFusion
      .mockRejectedValueOnce(new ApiError('网络请求失败', 0))
      .mockResolvedValueOnce({
        operation_id: 'op-2',
        recipe_id: 'rrrrrrrr-rrrr-4rrr-8rrr-rrrrrrrrrrrr',
      })
    render(<FusionPage />)
    await screen.findByRole('button', { name: /文言文丹/ })
    await selectTwoAndFuse(user)

    await user.click(screen.getByRole('button', { name: '保存入库' }))
    // 失败：错误弹窗展示、预览未关闭、材料保留
    expect(await screen.findByRole('alertdialog')).toHaveTextContent('网络请求失败')
    expect(screen.getByText('不消耗材料；保存时消耗')).toBeInTheDocument()

    // 关闭错误弹窗后重试：pending 记录未清 → 复用同一 key（不自动换 key）
    // 错误弹窗的关闭按钮（modal 的关闭同文案，取最后一个）
    await user.click(screen.getAllByRole('button', { name: '关闭' }).at(-1)!)
    await user.click(screen.getByRole('button', { name: '保存入库' }))
    // 第二次保存成功：modal 关闭
    await waitForQueryByTextGone('不消耗材料；保存时消耗')

    expect(td.confirmFusion).toHaveBeenCalledTimes(2)
    const [key1] = td.confirmFusion.mock.calls[0]
    const [key2] = td.confirmFusion.mock.calls[1]
    expect(key2).toBe(key1)
  })

  it('断线恢复：网络失败后按原 key 查询已提交结果，命中按成功处理', async () => {
    const user = userEvent.setup()
    td.confirmFusion.mockRejectedValueOnce(new ApiError('网络请求失败', 0))
    td.getOperation.mockResolvedValue({
      operation_id: 'op-3',
      recipe_id: 'rrrrrrrr-rrrr-4rrr-8rrr-rrrrrrrrrrrr',
      item_ids: ['new-item'],
      consumed_item_ids: [ITEM_A, ITEM_B],
    })
    render(<FusionPage />)
    await screen.findByRole('button', { name: /文言文丹/ })
    await selectTwoAndFuse(user)

    await user.click(screen.getByRole('button', { name: '保存入库' }))

    // recover 命中：按成功处理（预览关闭、池重读），不再展示错误
    await waitForQueryByTextGone('不消耗材料；保存时消耗')
    expect(td.getOperation).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(td.listPillItems).toHaveBeenCalledTimes(2)
    // pending 记录已清除：之后同动作会创建新 key
    expect(sessionStorage.getItem('alchemy_pending_operations')).toBe('[]')
  })
})
