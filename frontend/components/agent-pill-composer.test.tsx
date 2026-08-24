import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AgentPillComposer } from '@/components/agent-pill-composer'
import type { AgentPillDraftItem, Pill } from '@/services/types'

// 真实消息解析(命名空间点路径 + {value} 插值),与 pill-detail.test.tsx 同模式
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

const twoRows: AgentPillDraftItem[] = [
  { key: 'pill-a', pill_id: 'pill-a', weight: 2 },
  { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
]

function renderComposer({
  value = twoRows,
  onChange = vi.fn(),
  pills = [pillA, pillB, pillC],
  fieldErrors,
}: {
  value?: AgentPillDraftItem[]
  onChange?: (next: AgentPillDraftItem[]) => void
  pills?: Pill[]
  fieldErrors?: Record<string, string>
} = {}) {
  return { onChange, ...render(
    <AgentPillComposer value={value} onChange={onChange} pills={pills} fieldErrors={fieldErrors} />,
  ) }
}

describe('AgentPillComposer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })
  afterEach(() => cleanup())

  it('按草稿顺序渲染金丹名与剂量输入', () => {
    renderComposer()
    const weights = screen.getAllByRole('spinbutton')
    expect(weights).toHaveLength(2)
    expect(weights[0]).toHaveValue(2)
    expect(weights[1]).toHaveValue(1)
    expect(screen.getByText('丹心妙语')).toBeInTheDocument()
    expect(screen.getByText('浩然正气')).toBeInTheDocument()
  })

  it('剂量输入是键盘可访问的 number input,带金丹名 aria-label', () => {
    renderComposer()
    expect(screen.getByRole('spinbutton', { name: /丹心妙语/ })).toBeInTheDocument()
  })

  it('修改剂量:onChange 输出整行更新的最终数组,其余行不变', () => {
    const onChange = vi.fn()
    renderComposer({ onChange })

    // 受控组件:测试不更新 value,用 fireEvent.change 直接给出目标值
    const weightInput = screen.getByRole('spinbutton', { name: /丹心妙语/ })
    fireEvent.change(weightInput, { target: { value: '5' } })
    expect(onChange).toHaveBeenCalledWith([
      { key: 'pill-a', pill_id: 'pill-a', weight: 5 },
      { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
    ])
  })

  it('上移/下移按钮交换顺序;首行不可上移,末行不可下移(键盘可达)', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    renderComposer({ onChange })

    const upFirst = screen.getByRole('button', { name: /上移 丹心妙语/ })
    const downLast = screen.getByRole('button', { name: /下移 浩然正气/ })
    expect(upFirst).toBeDisabled()
    expect(downLast).toBeDisabled()

    await user.click(screen.getByRole('button', { name: /下移 丹心妙语/ }))
    expect(onChange).toHaveBeenCalledWith([
      { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
      { key: 'pill-a', pill_id: 'pill-a', weight: 2 },
    ])
  })

  it('上移第二行后其位于首位', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    renderComposer({ onChange })

    await user.click(screen.getByRole('button', { name: /上移 浩然正气/ }))
    expect(onChange).toHaveBeenCalledWith([
      { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
      { key: 'pill-a', pill_id: 'pill-a', weight: 2 },
    ])
  })

  it('停服按钮移除对应行', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    renderComposer({ onChange })

    await user.click(screen.getByRole('button', { name: /停服 丹心妙语/ }))
    expect(onChange).toHaveBeenCalledWith([{ key: 'pill-b', pill_id: 'pill-b', weight: 1 }])
  })

  it('添加下拉只列未服用金丹,选中后追加到末尾(默认剂量 1,带稳定 key)', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    renderComposer({ onChange })

    const select = screen.getByRole('combobox', { name: '添加金丹' })
    const options = Array.from(select.querySelectorAll('option')).map(o => o.textContent)
    expect(options).toContain('清风徐来')
    expect(options).not.toContain('丹心妙语')
    expect(options).not.toContain('浩然正气')

    await user.selectOptions(select, 'pill-c')
    expect(onChange).toHaveBeenCalledWith([
      ...twoRows,
      { key: 'pill-c', pill_id: 'pill-c', weight: 1 },
    ])
  })

  it('全部金丹已服用时不显示添加下拉,显示提示', () => {
    renderComposer({ pills: [pillA, pillB] })
    expect(screen.queryByRole('combobox', { name: '添加金丹' })).toBeNull()
    expect(screen.getByText(/都已服用|all/i)).toBeInTheDocument()
  })

  it('金丹列表中找不到 pill_id 时显示未知金丹占位', () => {
    renderComposer({ value: [{ key: 'gone', pill_id: 'gone', weight: 1 }] })
    expect(screen.getByText(/未知金丹/)).toBeInTheDocument()
  })

  it('fieldErrors 中 pills.<key>.weight 的机器码被翻译成错误文案展示', () => {
    renderComposer({ fieldErrors: { 'pills.pill-a.weight': 'range' } })
    expect(screen.getByRole('alert')).toHaveTextContent(/0-10/)
  })

  it('空编排时显示空提示', () => {
    renderComposer({ value: [] })
    expect(screen.getByText(/尚未服用|no pills/i)).toBeInTheDocument()
  })
})
