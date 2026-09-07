import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
  distillNuwa: vi.fn(),
}))

// 女娲面板探针:道人创建页必须不再渲染女娲(唯一入口在金丹创建弹窗)
const NuwaDistillPanelSpy = vi.hoisted(() => vi.fn())

vi.mock('@/components/nuwa-distill-panel', () => ({
  NuwaDistillPanel: () => {
    NuwaDistillPanelSpy()
    return <div data-testid="nuwa-panel" />
  },
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

vi.mock('@/services/distillationService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/distillationService')>()
  return { ...actual, distillNuwa: td.distillNuwa }
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

  it('切换筛选即带参数重新请求:沉睡 → inactive,全部 → 不带 status', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => expect(td.listAgents).toHaveBeenCalled())

    await user.click(screen.getByRole('button', { name: '沉睡' }))
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

    await user.click(screen.getByRole('button', { name: '沉睡' }))
    expect(await screen.findByText('该状态下暂无道人')).toBeInTheDocument()
    expect(screen.queryByText('点击上方按钮招募你的第一位道人')).toBeNull()
  })

  it('加载失败后可重试,重试携带当前筛选参数', async () => {
    td.listAgents.mockRejectedValue(new Error('网络错误'))
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('道人列表加载失败')).toBeInTheDocument()

    // 切到沉睡后再重试
    td.listAgents.mockClear()
    td.listAgents.mockResolvedValue({ list: [inactiveAgent], total: 1, page: 1, page_size: 100 })
    await user.click(screen.getByRole('button', { name: '沉睡' }))
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

  it('创建道人弹窗不渲染女娲面板(唯一入口在金丹创建弹窗)', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')

    await user.click(screen.getByRole('button', { name: '招募道人' }))
    expect(screen.getByRole('heading', { name: '招募道人' })).toBeInTheDocument()
    expect(NuwaDistillPanelSpy).not.toHaveBeenCalled()
    expect(screen.queryByTestId('nuwa-panel')).toBeNull()
  })

  it('输入道号后网络填充头像与基础性格', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    td.distillNuwa.mockResolvedValue({
      avatar: 'https://upload.wikimedia.org/laozi.jpg',
      persona_summary: '沉静克制，善用朴素比喻，从事物本源推演判断。',
    })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')
    await user.click(screen.getByRole('button', { name: '招募道人' }))

    const fillButton = screen.getByRole('button', { name: '网络填充' })
    expect(fillButton).toBeDisabled()
    await user.type(screen.getByPlaceholderText('如：太虚真人'), '太上老君')
    await user.click(fillButton)

    await waitFor(() => expect(td.distillNuwa).toHaveBeenCalledWith({
      subject: '太上老君',
      brief: expect.stringContaining('基础性格'),
      locale: 'zh-CN',
    }))
    expect(screen.getByLabelText('头像 URL')).toHaveValue('https://upload.wikimedia.org/laozi.jpg')
    expect(screen.getByPlaceholderText('描述这位道人的性格特点和语言风格...')).toHaveValue(
      '沉静克制，善用朴素比喻，从事物本源推演判断。',
    )
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

  it('创建弹窗:非法协议头像被拒并显示错误,零 API', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')
    await user.click(screen.getByRole('button', { name: '招募道人' }))

    // 名称必填走原生 required 校验,先填名称再触发提交,验证头像校验兜底
    const nameInput = screen.getByPlaceholderText('如：太虚真人')
    await user.type(nameInput, '新道人')
    const avatar = screen.getByLabelText('头像 URL')
    await user.type(avatar, 'javascript:alert(1)')
    await user.click(screen.getByRole('button', { name: '招募' }))

    expect(
      await screen.findByText('头像仅支持完整 http/https URL 或 data:image 数据 URI'),
    ).toBeInTheDocument()
    expect(td.createAgent).not.toHaveBeenCalled()
  })

  it('创建弹窗:相对路径头像被拒并显示提示,零 API', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')
    await user.click(screen.getByRole('button', { name: '招募道人' }))

    // 输入框下方常驻提示:只支持完整 URL 或 data:image 数据 URI(相对路径不可用)
    expect(screen.getByText('只支持完整 URL 或 data:image 数据 URI（相对路径不可用）')).toBeInTheDocument()

    // 名称必填走原生 required 校验,先填名称再触发提交,验证头像校验兜底
    const nameInput = screen.getByPlaceholderText('如：太虚真人')
    await user.type(nameInput, '新道人')
    const avatar = screen.getByLabelText('头像 URL')
    await user.type(avatar, '/avatar.png')
    await user.click(screen.getByRole('button', { name: '招募' }))

    expect(
      await screen.findByText('头像仅支持完整 http/https URL 或 data:image 数据 URI'),
    ).toBeInTheDocument()
    expect(td.createAgent).not.toHaveBeenCalled()
  })

  it('创建弹窗:合法 data URI 头像随创建请求提交', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    td.createAgent.mockResolvedValue({ ...activeAgent, id: 'agent-9', name: '新道人' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')
    await user.click(screen.getByRole('button', { name: '招募道人' }))

    const nameInput = screen.getByPlaceholderText('如：太虚真人')
    await user.type(nameInput, '新道人')
    await user.type(screen.getByLabelText('头像 URL'), 'data:image/png;base64,AAAA')
    await user.click(screen.getByRole('button', { name: '招募' }))

    await waitFor(() =>
      expect(td.createAgent).toHaveBeenCalledWith(
        expect.objectContaining({ name: '新道人', avatar: 'data:image/png;base64,AAAA' }),
      ),
    )
  })

  it('创建弹窗:超长 URL 头像被拒,零 API', async () => {
    td.listAgents.mockResolvedValue({ list: [activeAgent], total: 1, page: 1, page_size: 100 })
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('太上老君·active')
    await user.click(screen.getByRole('button', { name: '招募道人' }))

    const nameInput = screen.getByPlaceholderText('如：太虚真人')
    await user.type(nameInput, '新道人')
    const avatar = screen.getByLabelText('头像 URL')
    // fireEvent 直改 value 绕过 maxLength,验证提交前校验兜底
    fireEvent.change(avatar, { target: { value: `https://example.com/${'a'.repeat(2050)}` } })
    await user.click(screen.getByRole('button', { name: '招募' }))

    expect(
      await screen.findByText('头像过长（URL 上限 2048 字符，data URI 上限 1500000 字符）'),
    ).toBeInTheDocument()
    expect(td.createAgent).not.toHaveBeenCalled()
  })
})
