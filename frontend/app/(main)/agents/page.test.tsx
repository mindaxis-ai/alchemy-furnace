import type { ReactNode } from 'react'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AgentsPage from '@/app/(main)/agents/page'
import { AgentProvider } from '@/contexts/AgentContext'
import type { Agent } from '@/services/types'

// ---- 可变的测试替身 ----
const td = vi.hoisted(() => ({
  listAgents: vi.fn(),
  createAgent: vi.fn(),
  listModelOptions: vi.fn(),
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

vi.mock('next-intl', async () => {
  const zh = (await import('@/messages/zh-CN.json')).default
  return {
    useTranslations: (namespace: string) => (key: string, values?: Record<string, unknown>) =>
      resolveMsg(zh, namespace, key, values),
    useLocale: () => 'zh-CN',
  }
})

// 卡片行为在 agent-card.test.tsx 覆盖,此处隔离为名字占位
vi.mock('@/components/agent-card', () => ({
  AgentCard: ({ agent }: { agent: Agent }) => (
    <div data-testid="agent-card">
      {agent.name}·{agent.status}
    </div>
  ),
}))

vi.mock('@/services/agentService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentService')>()
  return { ...actual, listAgents: td.listAgents, createAgent: td.createAgent }
})

vi.mock('@/services/modelService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/modelService')>()
  return { ...actual, options: td.listModelOptions }
})

const activeAgent: Agent = {
  id: 'agent-1',
  name: '太上老君',
  personality: '沉稳如山',
  model_name: 'gpt-4o',
  status: 'active',
  proactivity: 60,
  created_at: '2026-08-20T00:00:00Z',
}

const inactiveAgent: Agent = { ...activeAgent, id: 'agent-2', name: '沉睡道人', status: 'inactive' }

function renderPage() {
  return render(
    <AgentProvider>
      <AgentsPage />
    </AgentProvider>,
  )
}

describe('AgentsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    td.listAgents.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 100 })
    td.listModelOptions.mockResolvedValue([])
  })
  afterEach(() => cleanup())

  it('进入页面默认以 status=active 请求(API 参数筛选)', async () => {
    renderPage()
    await waitFor(() => expect(td.listAgents).toHaveBeenCalled())
    expect(td.listAgents).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'active' }))
  })

  it('切换筛选即带参数重新请求:已停用 → inactive,全部 → 不带 status', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(td.listAgents).toHaveBeenCalled())

    await user.click(screen.getByRole('button', { name: '已停用' }))
    await waitFor(() =>
      expect(td.listAgents).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'inactive' })),
    )

    await user.click(screen.getByRole('button', { name: '全部' }))
    await waitFor(() => expect(td.listAgents).toHaveBeenLastCalledWith(undefined))

    await user.click(screen.getByRole('button', { name: '活跃' }))
    await waitFor(() =>
      expect(td.listAgents).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'active' })),
    )
  })

  it('筛选按钮是三态切换组,当前项 aria-pressed', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(td.listAgents).toHaveBeenCalled())

    const group = screen.getByRole('group', { name: '状态筛选' })
    const active = screen.getByRole('button', { name: '活跃' })
    expect(group).toContainElement(active)
    expect(active).toHaveAttribute('aria-pressed', 'true')

    await user.click(screen.getByRole('button', { name: '全部' }))
    expect(screen.getByRole('button', { name: '全部' })).toHaveAttribute('aria-pressed', 'true')
    expect(active).toHaveAttribute('aria-pressed', 'false')
  })

  it('渲染 API 返回的道人卡片', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    renderPage()
    expect(await screen.findByText('太上老君·active')).toBeInTheDocument()
  })

  it('非默认筛选下空列表显示筛选空态,不出现招募引导文案', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(td.listAgents).toHaveBeenCalled())

    await user.click(screen.getByRole('button', { name: '已停用' }))
    expect(await screen.findByText('该状态下暂无道人')).toBeInTheDocument()
    expect(screen.queryByText('点击上方按钮招募你的第一位道人')).toBeNull()
  })

  it('加载失败后可重试,重试携带当前筛选参数', async () => {
    td.listAgents.mockRejectedValue(new Error('网络错误'))
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('道人列表加载失败')).toBeInTheDocument()

    // 切到已停用后再重试
    td.listAgents.mockClear()
    td.listAgents.mockResolvedValue({ list: [inactiveAgent], total: 1, page: 1, page_size: 100 })
    await user.click(screen.getByRole('button', { name: '已停用' }))
    await screen.findByText('沉睡道人·inactive')

    td.listAgents.mockClear()
    td.listAgents.mockRejectedValue(new Error('网络错误'))
    await user.click(screen.getByRole('button', { name: '全部' }))
    expect(await screen.findByText('道人列表加载失败')).toBeInTheDocument()

    td.listAgents.mockClear()
    td.listAgents.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 100 })
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    await waitFor(() => expect(td.listAgents).toHaveBeenLastCalledWith(undefined))
  })

  it('创建入口保留:提交创建请求并关闭弹窗', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    td.createAgent.mockResolvedValue({ ...activeAgent, id: 'agent-9', name: '新道人' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')

    await user.click(screen.getByRole('button', { name: '招募道人' }))
    // 弹窗 label 无 htmlFor 关联,按占位文案定位输入框
    const nameInput = screen.getByPlaceholderText('如：太虚真人')
    await user.type(nameInput, '新道人')
    await user.click(screen.getByRole('button', { name: '招募' }))

    await waitFor(() =>
      expect(td.createAgent).toHaveBeenCalledWith(expect.objectContaining({ name: '新道人' })),
    )
    await waitFor(() => expect(screen.queryByText('招募道人', { selector: 'h2' })).toBeNull())
  })
})
