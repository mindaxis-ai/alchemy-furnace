import type { ReactNode } from 'react'
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AgentProvider, useAgent } from '@/contexts/AgentContext'
import { useAgentEditorFlow } from '@/hooks/use-agent-editor-flow'
import { ApiError } from '@/services/api'
import type { AgentDetail, DistillationDraft } from '@/services/types'

const listAgents = vi.hoisted(() => vi.fn())
const getAgent = vi.hoisted(() => vi.fn())
const updateAgent = vi.hoisted(() => vi.fn())
const replacePills = vi.hoisted(() => vi.fn())

vi.mock('@/services/agentService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentService')>()
  return { ...actual, listAgents, getAgent, updateAgent, replacePills }
})

const wrapper = ({ children }: { children: ReactNode }) => (
  <AgentProvider>{children}</AgentProvider>
)

const baseAgent: AgentDetail = {
  id: 'agent-1',
  name: '太上老君',
  avatar: 'https://example.com/laojun.png',
  personality: '沉稳如山',
  model_name: 'gpt-4o',
  status: 'active',
  proactivity: 60,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
  agent_pills: [
    { id: 'ap-1', agent_id: 'agent-1', pill_id: 'pill-a', weight: 2, sort_order: 1, created_at: '2026-08-20T00:00:00Z' },
    { id: 'ap-2', agent_id: 'agent-1', pill_id: 'pill-b', weight: 1, sort_order: 2, created_at: '2026-08-20T00:00:00Z' },
  ],
}

const nuwaDraft: DistillationDraft = {
  name: '女娲造人',
  description: '蒸馏候选',
  persona_summary: '悲天悯人的造物主',
  tags: ['神话'],
  skill_schema: { identity_card: '造物主' },
  sources: [{ title: '淮南子', url: 'https://example.com/huainanzi', dimension: 'persona' }],
  model: 'gpt-5',
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('useAgentEditorFlow', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listAgents.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 100 })
  })
  afterEach(() => cleanup())

  it('beginEdit 生成独立草稿，编辑只改草稿不动源对象', () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    expect(result.current.mode).toBe('readonly')
    expect(result.current.dirty).toBe(false)

    act(() => result.current.beginEdit())
    expect(result.current.mode).toBe('editing')
    expect(result.current.dirty).toBe(false)
    expect(result.current.draft.name).toBe('太上老君')
    expect(result.current.draft.personality).toBe('沉稳如山')
    expect(result.current.draft.model_name).toBe('gpt-4o')
    expect(result.current.draft.status).toBe('active')
    // 服丹编排映射为草稿行(带本地稳定 key)
    expect(result.current.draft.pills).toEqual([
      { key: 'pill-a', pill_id: 'pill-a', weight: 2 },
      { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
    ])
    // 引用隔离:草稿行不得与源 agent_pills 共享引用
    expect(result.current.draft.pills[0]).not.toBe(baseAgent.agent_pills![0])

    act(() => result.current.updateDraft({ name: '改名' }))
    expect(result.current.dirty).toBe(true)
    expect(baseAgent.name).toBe('太上老君')
  })

  it('discard 放弃修改并回到只读', () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '改名' }))
    expect(result.current.dirty).toBe(true)

    act(() => result.current.discard())
    expect(result.current.mode).toBe('readonly')
    expect(result.current.dirty).toBe(false)
    expect(result.current.draft.name).toBe('太上老君')
  })

  it('保存基础资料后 PUT 完整编排，全部成功后 GET 回读才退出编辑', async () => {
    updateAgent.mockResolvedValue({ ...baseAgent, name: '改名' })
    replacePills.mockResolvedValue({ ...baseAgent, name: '改名' })
    const fresh: AgentDetail = { ...baseAgent, name: '改名', updated_at: '2026-08-23T00:00:00Z' }
    getAgent.mockResolvedValue(fresh)
    const { result } = renderHook(
      () => ({ flow: useAgentEditorFlow(baseAgent), agent: useAgent() }),
      { wrapper },
    )

    act(() => result.current.flow.beginEdit())
    act(() => result.current.flow.updateDraft({ name: '改名', proactivity: 80 }))

    let ok = false
    await act(async () => {
      ok = await result.current.flow.save()
    })
    expect(ok).toBe(true)
    // 第一步:基础资料
    expect(updateAgent).toHaveBeenCalledTimes(1)
    expect(updateAgent).toHaveBeenCalledWith('agent-1', {
      name: '改名',
      avatar: 'https://example.com/laojun.png',
      personality: '沉稳如山',
      model_name: 'gpt-4o',
      proactivity: 80,
      status: 'active',
    })
    // 第二步:完整编排(剥离本地 key)
    expect(replacePills).toHaveBeenCalledTimes(1)
    expect(replacePills).toHaveBeenCalledWith('agent-1', [
      { pill_id: 'pill-a', weight: 2 },
      { pill_id: 'pill-b', weight: 1 },
    ])
    // 第三步:写后重读
    expect(getAgent).toHaveBeenCalledTimes(1)
    expect(getAgent).toHaveBeenCalledWith('agent-1')
    // 顺序:基础资料 → 编排 → GET
    expect(updateAgent.mock.invocationCallOrder[0]).toBeLessThan(replacePills.mock.invocationCallOrder[0])
    expect(replacePills.mock.invocationCallOrder[0]).toBeLessThan(getAgent.mock.invocationCallOrder[0])
    // Context 中的最终对象是 GET 回读的 fresh,不是 PUT 回显
    expect(result.current.agent.state.currentAgent?.updated_at).toBe('2026-08-23T00:00:00Z')
    expect(result.current.flow.mode).toBe('readonly')
    expect(result.current.flow.dirty).toBe(false)
    expect(result.current.flow.saveStatus).toBe('idle')
  })

  it('基础资料保存失败：不进入编排步骤，保留草稿与编辑态', async () => {
    updateAgent.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '新名字' }))

    let ok = true
    await act(async () => {
      ok = await result.current.save()
    })
    expect(ok).toBe(false)
    expect(result.current.saveStatus).toBe('error')
    expect(result.current.mode).toBe('editing')
    expect(result.current.draft.name).toBe('新名字')
    expect(result.current.dirty).toBe(true)
    expect(replacePills).not.toHaveBeenCalled()
    expect(getAgent).not.toHaveBeenCalled()
  })

  it('编排 PUT 失败：保留草稿与编辑态，不发起 GET', async () => {
    updateAgent.mockResolvedValue({ ...baseAgent })
    replacePills.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() =>
      result.current.updateDraft({ pills: [{ key: 'pill-a', pill_id: 'pill-a', weight: 5 }] }),
    )

    let ok = true
    await act(async () => {
      ok = await result.current.save()
    })
    expect(ok).toBe(false)
    expect(result.current.saveStatus).toBe('error')
    expect(result.current.mode).toBe('editing')
    expect(result.current.draft.pills).toEqual([{ key: 'pill-a', pill_id: 'pill-a', weight: 5 }])
    expect(getAgent).not.toHaveBeenCalled()
  })

  it('保存后 GET 失败不得宣称完成：保持编辑态与草稿', async () => {
    updateAgent.mockResolvedValue({ ...baseAgent })
    replacePills.mockResolvedValue({ ...baseAgent })
    getAgent.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '新名字' }))

    let ok = true
    await act(async () => {
      ok = await result.current.save()
    })
    expect(ok).toBe(false)
    expect(result.current.saveStatus).toBe('error')
    expect(result.current.mode).toBe('editing')
    expect(result.current.draft.name).toBe('新名字')
    expect(result.current.dirty).toBe(true)
  })

  it('连续点击保存只产生一次写请求', async () => {
    const gate = deferred<AgentDetail>()
    updateAgent.mockImplementation(() => gate.promise)
    replacePills.mockResolvedValue({ ...baseAgent })
    getAgent.mockResolvedValue({ ...baseAgent })
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '改名' }))

    let first!: Promise<boolean>
    let second!: Promise<boolean>
    act(() => {
      first = result.current.save()
      second = result.current.save()
    })
    expect(updateAgent).toHaveBeenCalledTimes(1)

    gate.resolve(baseAgent)
    await act(async () => {
      await Promise.all([first, second])
    })
    await expect(first).resolves.toBe(true)
    await expect(second).resolves.toBe(false)
  })

  it('restoreServerVersion 重置草稿到服务器基线但保持编辑态', () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '改名', personality: '暴躁' }))
    expect(result.current.dirty).toBe(true)

    act(() => result.current.restoreServerVersion())
    expect(result.current.mode).toBe('editing')
    expect(result.current.dirty).toBe(false)
    expect(result.current.draft.name).toBe('太上老君')
    expect(result.current.draft.personality).toBe('沉稳如山')
  })

  it('name 为空拒绝保存并写入 fieldErrors，不发起任何 API', async () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '   ' }))

    let ok = true
    await act(async () => {
      ok = await result.current.save()
    })
    expect(ok).toBe(false)
    expect(result.current.fieldErrors.name).toBeTruthy()
    expect(result.current.mode).toBe('editing')
    expect(updateAgent).not.toHaveBeenCalled()
    expect(replacePills).not.toHaveBeenCalled()
  })

  it('金丹剂量越界写入 fieldErrors，不发起任何 API', async () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    act(() => result.current.beginEdit())
    act(() =>
      result.current.updateDraft({ pills: [{ key: 'pill-a', pill_id: 'pill-a', weight: 11 }] }),
    )

    let ok = true
    await act(async () => {
      ok = await result.current.save()
    })
    expect(ok).toBe(false)
    expect(result.current.fieldErrors['pills.pill-a.weight']).toBeTruthy()
    expect(updateAgent).not.toHaveBeenCalled()
    expect(replacePills).not.toHaveBeenCalled()
  })

  it('Nuwa 草稿只在显式 applyNuwaDraft 后写入 name/personality', () => {
    const { result } = renderHook(() => useAgentEditorFlow(baseAgent), { wrapper })
    expect(result.current.draft.name).toBe('太上老君')
    expect(result.current.draft.personality).toBe('沉稳如山')

    act(() => result.current.applyNuwaDraft(nuwaDraft))
    expect(result.current.mode).toBe('editing')
    expect(result.current.draft.name).toBe('女娲造人')
    expect(result.current.draft.personality).toBe('悲天悯人的造物主')
    expect(result.current.dirty).toBe(true)
    // 不得触碰服丹编排
    expect(result.current.draft.pills).toEqual([
      { key: 'pill-a', pill_id: 'pill-a', weight: 2 },
      { key: 'pill-b', pill_id: 'pill-b', weight: 1 },
    ])
  })
})
