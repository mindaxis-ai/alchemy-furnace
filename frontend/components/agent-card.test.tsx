import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AgentCard } from '@/components/agent-card'
import type { Agent } from '@/services/types'

// ---- 可变的测试替身 ----
const td = vi.hoisted(() => ({
  push: vi.fn(),
  launchSingle: vi.fn(),
  launchState: { status: 'idle' } as
    | { status: 'idle' }
    | { status: 'submitting' }
    | { status: 'error'; message: string; errorCode?: string },
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

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: td.push }),
}))

vi.mock('@/hooks/use-chat-launch-flow', () => ({
  useChatLaunchFlow: () => ({
    state: td.launchState,
    launchSingle: td.launchSingle,
    launchGroup: vi.fn(),
    retry: vi.fn(),
    reset: vi.fn(),
  }),
}))

const activeAgent: Agent = {
  id: 'agent-1',
  name: '太上老君',
  personality: '沉稳如山',
  model_name: 'gpt-4o',
  status: 'active',
  proactivity: 60,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const inactiveAgent: Agent = { ...activeAgent, id: 'agent-2', name: '沉睡道人', status: 'inactive' }

describe('AgentCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    td.launchState = { status: 'idle' }
    td.launchSingle.mockResolvedValue(true)
  })
  afterEach(() => cleanup())

  it('整卡是单一键盘可访问导航容器,点击进入详情', async () => {
    const user = userEvent.setup()
    render(<AgentCard agent={activeAgent} />)

    expect(screen.getAllByRole('link')).toHaveLength(1)
    await user.click(screen.getByRole('link', { name: '太上老君' }))
    expect(td.push).toHaveBeenCalledTimes(1)
    expect(td.push).toHaveBeenCalledWith('/agents/agent-1')
  })

  it('键盘 Enter 与 Space 均可触发导航', () => {
    render(<AgentCard agent={activeAgent} />)
    const nav = screen.getByRole('link', { name: '太上老君' })

    fireEvent.keyDown(nav, { key: 'Enter' })
    expect(td.push).toHaveBeenCalledWith('/agents/agent-1')
    fireEvent.keyDown(nav, { key: ' ' })
    expect(td.push).toHaveBeenCalledTimes(2)
  })

  it('内部论道按钮阻止冒泡:发起会话且不触发卡片导航(无双导航)', async () => {
    const user = userEvent.setup()
    render(<AgentCard agent={activeAgent} />)

    await user.click(screen.getByRole('button', { name: /论道/ }))
    expect(td.launchSingle).toHaveBeenCalledTimes(1)
    expect(td.launchSingle).toHaveBeenCalledWith('agent-1')
    expect(td.push).not.toHaveBeenCalled()
  })

  it('键盘焦点落在内部论道按钮上时,Enter/Space 不触发卡片导航', () => {
    render(<AgentCard agent={activeAgent} />)
    const chatBtn = screen.getByRole('button', { name: /论道/ })

    fireEvent.keyDown(chatBtn, { key: 'Enter' })
    fireEvent.keyDown(chatBtn, { key: ' ' })
    expect(td.push).not.toHaveBeenCalled()
  })

  it('inactive 卡片:显示已停用徽记,论道按钮禁用并带原因提示,整卡仍可进详情', async () => {
    const user = userEvent.setup()
    render(<AgentCard agent={inactiveAgent} />)

    expect(screen.getByText('沉睡')).toBeInTheDocument()
    const chatBtn = screen.getByRole('button', { name: /论道/ })
    expect(chatBtn).toBeDisabled()
    expect(chatBtn).toHaveAttribute('title', expect.stringContaining('停用'))

    await user.click(chatBtn)
    expect(td.launchSingle).not.toHaveBeenCalled()

    // 整卡仍可进入详情(详情页可恢复 active)
    await user.click(screen.getByRole('link', { name: '沉睡道人' }))
    expect(td.push).toHaveBeenCalledWith('/agents/agent-2')
  })

  it('active 卡片不显示停用提示,论道按钮可用', () => {
    render(<AgentCard agent={activeAgent} />)
    const chatBtn = screen.getByRole('button', { name: /论道/ })
    expect(chatBtn).toBeEnabled()
    expect(chatBtn).not.toHaveAttribute('title', expect.stringContaining('停用'))
  })

  it('发起会话失败时在卡片上展示错误(不静默)', () => {
    td.launchState = { status: 'error', message: '创建会话失败' }
    render(<AgentCard agent={activeAgent} />)
    expect(screen.getByRole('alert')).toHaveTextContent('创建会话失败')
  })

  it('紧凑模式保持链接行为', () => {
    render(<AgentCard agent={activeAgent} compact />)
    expect(screen.getByRole('link', { name: /太上老君/ })).toHaveAttribute('href', '/agents/agent-1')
  })
})
