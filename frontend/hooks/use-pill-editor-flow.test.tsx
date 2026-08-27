import type { ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { PillProvider, usePill } from '@/contexts/PillContext'
import { usePillEditorFlow } from '@/hooks/use-pill-editor-flow'
import { ApiError } from '@/services/api'
import type { DistillationDraft, Pill } from '@/services/types'

const listPills = vi.hoisted(() => vi.fn())
const getPill = vi.hoisted(() => vi.fn())
const createPill = vi.hoisted(() => vi.fn())
const updatePill = vi.hoisted(() => vi.fn())
const deletePill = vi.hoisted(() => vi.fn())
const clonePill = vi.hoisted(() => vi.fn())

vi.mock('@/services/pillService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/pillService')>()
  return { ...actual, listPills, getPill, createPill, updatePill, deletePill, clonePill }
})

const wrapper = ({ children }: { children: ReactNode }) => (
  <PillProvider>{children}</PillProvider>
)

const customPill: Pill = {
  id: 'pill-custom-1',
  name: '丹心妙语',
  description: '温润如茶',
  skill_schema: {
    identity_card: '一位温和的古代炼丹师',
    expression_dna: { formality: 0.3, vocabulary: ['茶', '炉'] },
    mental_models: [{ name: '阴阳转化', application: '把对立概念互相转化' }],
    future_unknown: { nested: ['甲', '乙'] },
  },
  tags: ['古风', '炼丹'],
  author: '太上老君',
  version: '2.1.0',
  is_builtin: false,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-20T00:00:00Z',
}

const builtinPill: Pill = {
  ...customPill,
  id: 'pill-builtin-1',
  is_builtin: true,
}

const nuwaDraft: DistillationDraft = {
  name: '女娲草稿',
  description: '由女娲蒸馏出的候选',
  persona_summary: '冷静克制的史官',
  tags: ['神话', '蒸馏'],
  skill_schema: {
    identity_card: '史官',
    expression_dna: { formality: 0.8 },
  },
  sources: [{ title: '史记', url: 'https://example.com/shiji', dimension: 'tone' }],
  model: 'gpt-5',
  research: {
    evidence_level: 'standard',
    document_count: 1,
    domain_count: 1,
    total_characters: 2500,
    warnings: [],
  },
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

describe('usePillEditorFlow', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listPills.mockResolvedValue({ list: [], total: 0, page: 1, page_size: 100 })
  })

  it('自定义金丹 beginEdit 进入编辑态并生成独立草稿', () => {
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
    expect(result.current.mode).toBe('readonly')
    expect(result.current.dirty).toBe(false)

    act(() => result.current.beginEdit())
    expect(result.current.mode).toBe('editing')
    expect(result.current.dirty).toBe(false)
    expect(result.current.draft.name).toBe('丹心妙语')
    expect(result.current.draft.version).toBe('2.1.0')

    // 草稿必须与源对象引用隔离(顶层与嵌套都不得共享)
    expect(result.current.draft.skill_schema).not.toBe(customPill.skill_schema)
    expect(result.current.draft.skill_schema.mental_models).not.toBe(
      customPill.skill_schema.mental_models,
    )
    expect(result.current.draft.tags).not.toBe(customPill.tags)

    act(() => result.current.updateDraft({ name: '改名' }))
    expect(result.current.dirty).toBe(true)
    expect(customPill.name).toBe('丹心妙语')
  })

  it('discard 放弃修改并回到只读', () => {
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '改名' }))
    expect(result.current.dirty).toBe(true)

    act(() => result.current.discard())
    expect(result.current.mode).toBe('readonly')
    expect(result.current.dirty).toBe(false)
    expect(result.current.draft.name).toBe('丹心妙语')
  })

  it('内置金丹不能 beginEdit，只能 makeCopy 制作副本', async () => {
    const onCopied = vi.fn()
    const copy: Pill = {
      ...customPill,
      id: 'pill-copy-9',
      name: '丹心妙语 副本',
      is_builtin: false,
    }
    clonePill.mockResolvedValue(copy)
    const { result } = renderHook(
      () => ({ flow: usePillEditorFlow(builtinPill, { onCopied }), pill: usePill() }),
      { wrapper },
    )

    act(() => result.current.flow.beginEdit())
    expect(result.current.flow.mode).toBe('readonly')

    let ok = false
    await act(async () => {
      ok = await result.current.flow.makeCopy()
    })
    expect(ok).toBe(true)
    expect(clonePill).toHaveBeenCalledTimes(1)
    expect(clonePill).toHaveBeenCalledWith('pill-builtin-1')
    expect(onCopied).toHaveBeenCalledWith('pill-copy-9')
    // 副本进入 Context 列表
    expect(
      result.current.pill.state.pills.some(p => p.id === 'pill-copy-9'),
    ).toBe(true)
  })

  it('makeCopy 失败进入 error 态且不跳转', async () => {
    const onCopied = vi.fn()
    clonePill.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(
      () => usePillEditorFlow(builtinPill, { onCopied }),
      { wrapper },
    )

    let ok = true
    await act(async () => {
      ok = await result.current.makeCopy()
    })
    expect(ok).toBe(false)
    expect(result.current.saveStatus).toBe('error')
    expect(onCopied).not.toHaveBeenCalled()
  })

  it('保存成功后必须重新 GET，最终状态以 GET 对象为准', async () => {
    listPills.mockResolvedValue({ list: [customPill], total: 1, page: 1, page_size: 100 })
    const putEcho: Pill = { ...customPill, name: 'PUT回显' }
    const fresh: Pill = {
      ...customPill,
      name: '丹心妙语·改',
      updated_at: '2026-08-22T00:00:00Z',
    }
    updatePill.mockResolvedValue(putEcho)
    getPill.mockResolvedValue(fresh)
    const { result } = renderHook(
      () => ({ flow: usePillEditorFlow(customPill), pill: usePill() }),
      { wrapper },
    )
    await act(async () => {
      await result.current.pill.fetchPills()
    })

    act(() => result.current.flow.beginEdit())
    act(() => result.current.flow.updateDraft({ name: '丹心妙语·改' }))

    let ok = false
    await act(async () => {
      ok = await result.current.flow.save()
    })
    expect(ok).toBe(true)
    expect(updatePill).toHaveBeenCalledTimes(1)
    expect(updatePill).toHaveBeenCalledWith(
      'pill-custom-1',
      expect.objectContaining({ name: '丹心妙语·改' }),
    )
    expect(getPill).toHaveBeenCalledTimes(1)
    expect(getPill).toHaveBeenCalledWith('pill-custom-1')
    // PUT 必须先于 GET(写后重读)
    expect(updatePill.mock.invocationCallOrder[0]).toBeLessThan(
      getPill.mock.invocationCallOrder[0],
    )
    // Context 中的最终对象是 GET 返回的 fresh,不是 PUT 回显
    expect(
      result.current.pill.state.pills.find(p => p.id === 'pill-custom-1')?.name,
    ).toBe('丹心妙语·改')
    expect(result.current.flow.mode).toBe('readonly')
    expect(result.current.flow.dirty).toBe(false)
    expect(result.current.flow.saveStatus).toBe('idle')
  })

  it('保存失败保留草稿与 dirty，不丢编辑态', async () => {
    updatePill.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
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
    expect(getPill).not.toHaveBeenCalled()
  })

  it('保存后 GET 失败不得宣称完成：保持编辑态与草稿', async () => {
    updatePill.mockResolvedValue(customPill)
    getPill.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
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
    const gate = deferred<Pill>()
    updatePill.mockImplementation(() => gate.promise)
    getPill.mockResolvedValue({ ...customPill, name: '丹心妙语·改' })
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
    act(() => result.current.beginEdit())
    act(() => result.current.updateDraft({ name: '丹心妙语·改' }))

    let first!: Promise<boolean>
    let second!: Promise<boolean>
    act(() => {
      first = result.current.save()
      second = result.current.save()
    })
    expect(updatePill).toHaveBeenCalledTimes(1)

    gate.resolve(customPill)
    await act(async () => {
      await Promise.all([first, second])
    })
    await expect(first).resolves.toBe(true)
    await expect(second).resolves.toBe(false)
  })

  it('Nuwa 草稿只在显式 applyNuwaDraft 后写入表单', () => {
    const { result } = renderHook(() => usePillEditorFlow(customPill), { wrapper })
    // 未 apply 前表单保持原金丹内容
    expect(result.current.draft.name).toBe('丹心妙语')
    expect(result.current.draft.tags).toEqual(['古风', '炼丹'])

    act(() => result.current.applyNuwaDraft(nuwaDraft))
    expect(result.current.mode).toBe('editing')
    expect(result.current.draft.name).toBe('女娲草稿')
    expect(result.current.draft.description).toBe('由女娲蒸馏出的候选')
    expect(result.current.draft.tags).toEqual(['神话', '蒸馏'])
    expect(result.current.draft.skill_schema.identity_card).toBe('史官')
    expect(result.current.dirty).toBe(true)
    // 草稿引用隔离:之后修改表单不得污染面板持有的 Nuwa 草稿
    expect(result.current.draft.skill_schema).not.toBe(nuwaDraft.skill_schema)
  })

  it('内置金丹上 applyNuwaDraft 无效(须先制作副本)', () => {
    const { result } = renderHook(() => usePillEditorFlow(builtinPill), { wrapper })
    act(() => result.current.applyNuwaDraft(nuwaDraft))
    expect(result.current.mode).toBe('readonly')
    expect(result.current.draft.name).toBe('丹心妙语')
    expect(result.current.dirty).toBe(false)
  })

  it('Context 写失败抛出 ApiError 而非静默返回 null', async () => {
    createPill.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const { result } = renderHook(() => usePill(), { wrapper })

    await act(async () => {
      await expect(
        result.current.addPill({ name: 'x', skill_schema: { expression_dna: {} } }),
      ).rejects.toBeInstanceOf(ApiError)
    })
    expect(result.current.state.error).toBe('服务器内部错误')
  })
})
