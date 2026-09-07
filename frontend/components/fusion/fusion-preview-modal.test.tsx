import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { FusionPreviewModal } from '@/components/fusion/fusion-preview-modal'
import type { FusionPreview } from '@/services/types'

// 真实消息解析(命名空间点路径 + {value} 插值),与既有组件测试同模式
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

const ITEM_A = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const ITEM_B = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'

const preview: FusionPreview = {
  preview_id: 'pppppppp-pppp-4ppp-8ppp-pppppppppppp',
  expires_at: '2026-08-31T17:00:00Z',
  name: '新丹',
  description: '融合而生',
  skill_schema: { name: '新丹', description: '融合而生', system_prompt: 'x' },
  operator: { id: 'o-1', name: '金木水火土' },
  model: 'gpt-fusion',
  degraded: false,
}

const parents = [
  { id: ITEM_A, name: '文言文丹', state: 'available' as const, recipe_id: 'r-1', revision_id: 'rv-1', revision: 3, created_at: '2026-08-20T00:00:00Z' },
  { id: ITEM_B, name: '俳句丹', state: 'available' as const, recipe_id: 'r-2', revision_id: 'rv-2', revision: 1, created_at: '2026-08-20T00:00:00Z' },
]

function renderModal(overrides: Partial<Parameters<typeof FusionPreviewModal>[0]> = {}) {
  const props = {
    result: preview,
    parents,
    saving: false,
    onSave: vi.fn(),
    onEdit: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  }
  const utils = render(<FusionPreviewModal {...props} />)
  return { ...utils, props }
}

describe('FusionPreviewModal 融合预览弹窗', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => cleanup())

  it('不再提供换一炉入口', async () => {
    renderModal()
    await screen.findByText('不消耗材料；保存时消耗')
    expect(screen.queryByRole('button', { name: /换一炉/ })).not.toBeInTheDocument()
  })

  it('标明「不消耗材料；保存时消耗」，展示算子与材料血统', async () => {
    renderModal()
    expect(await screen.findByText('不消耗材料；保存时消耗')).toBeInTheDocument()
    expect(screen.getByText(/金木水火土/)).toBeInTheDocument()
    // 血统：文言文丹 × 俳句丹
    expect(screen.getByText(/文言文丹 × 俳句丹/)).toBeInTheDocument()
  })

  it('保存仅调用 onSave（含编辑后的名称/描述），重复点击只产生一项操作', async () => {
    const user = userEvent.setup()
    let resolveSave!: (v: void) => void
    const onSave = vi.fn(() => new Promise<void>((r) => { resolveSave = r }))
    const { props } = renderModal({ onSave })
    await screen.findByText('不消耗材料；保存时消耗')

    const saveBtn = screen.getByRole('button', { name: /保存入库/ })
    await user.click(saveBtn)
    await user.click(saveBtn)

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith({ name: '新丹', description: '融合而生' })
    resolveSave(undefined)
    await waitFor(() => expect(props.onSave).toHaveBeenCalledTimes(1))
  })

  it('saving 期间所有提交按钮禁用', async () => {
    const user = userEvent.setup()
    const { props } = renderModal({ saving: true })
    await screen.findByText('不消耗材料；保存时消耗')

    expect(screen.getByRole('button', { name: '编辑' })).toBeDisabled()
    // saving 时保存按钮文案切换为「入库中…」
    expect(screen.getByRole('button', { name: /入库中/ })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: '编辑' }))
    expect(props.onEdit).not.toHaveBeenCalled()
    expect(props.onSave).not.toHaveBeenCalled()
  })

  it('编辑结果走 onEdit；关闭走 onClose', async () => {
    const user = userEvent.setup()
    const { props } = renderModal()
    await screen.findByText('不消耗材料；保存时消耗')

    await user.click(screen.getByRole('button', { name: '编辑' }))
    expect(props.onEdit).toHaveBeenCalledWith({ name: '新丹', description: '融合而生' })

    // 底部「关闭」按钮（右上角 X 也是同名 aria-label，取最后一个）
    await user.click(screen.getAllByRole('button', { name: '关闭' }).at(-1)!)
    expect(props.onClose).toHaveBeenCalledTimes(1)
  })
})
