'use client'

/**
 * 金丹编辑器流程 hook
 * 只读/编辑双态 + 独立草稿 + 写后重读回源：
 * - 内置金丹只读，编辑前必须 makeCopy() 制作副本
 * - 草稿为深拷贝，与源金丹零引用共享
 * - 保存成功后必须重新 GET，只有 GET 成功才算保存完成
 * - 提交中去重：连续触发只产生一次写请求
 */
import { useCallback, useMemo, useRef, useState } from 'react'

import { usePill } from '@/contexts/PillContext'
import { clonePill, getPill } from '@/services/pillService'
import type { CreatePillRequest, DistillationDraft, Pill } from '@/services/types'

export type PillEditorMode = 'readonly' | 'editing'
export type PillSaveStatus = 'idle' | 'submitting' | 'error'

export interface PillEditorFlow {
  mode: PillEditorMode
  draft: CreatePillRequest
  dirty: boolean
  saveStatus: PillSaveStatus
  /** 进入编辑态（内置金丹无效，须先制作副本） */
  beginEdit(): void
  /** 制作副本：克隆当前金丹为自定义副本，成功后经 onCopied 回调新 UUID */
  makeCopy(): Promise<boolean>
  /** 保存草稿：PUT 成功后重新 GET 才回只读；任何一步失败保留草稿与编辑态 */
  save(): Promise<boolean>
  /** 放弃修改，回到只读 */
  discard(): void
  /** 女娲蒸馏草稿显式落表（仅调用此方法才写入表单） */
  applyNuwaDraft(draft: DistillationDraft): void
  /** 局部更新草稿 */
  updateDraft(patch: Partial<CreatePillRequest>): void
}

export interface PillEditorFlowOptions {
  /** 制作副本成功后的回调（携带新副本 id，供页面跳转到副本编辑态） */
  onCopied?: (newPillId: string) => void
}

/** 从金丹构造独立草稿（JSON 深拷贝，与源对象零引用共享） */
function buildDraft(pill: Pill): CreatePillRequest {
  return {
    name: pill.name,
    description: pill.description ?? '',
    author: pill.author ?? '',
    version: pill.version,
    tags: [...pill.tags],
    skill_schema: JSON.parse(JSON.stringify(pill.skill_schema)),
  }
}

function emptyDraft(): CreatePillRequest {
  return { name: '', description: '', author: '', version: '1.0.0', tags: [], skill_schema: {} }
}

/** 键序无关的稳定序列化（dirty 比较用，避免对象重建导致误报） */
function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'undefined'
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  const entries = Object.keys(value as Record<string, unknown>)
    .sort()
    .map(key => `${JSON.stringify(key)}:${stableStringify((value as Record<string, unknown>)[key])}`)
  return `{${entries.join(',')}}`
}

export function usePillEditorFlow(pill: Pill | null, options?: PillEditorFlowOptions): PillEditorFlow {
  const { dispatch, editPill } = usePill()
  const [mode, setMode] = useState<PillEditorMode>('readonly')
  const [saveStatus, setSaveStatus] = useState<PillSaveStatus>('idle')
  const [draft, setDraft] = useState<CreatePillRequest>(() => (pill ? buildDraft(pill) : emptyDraft()))
  /** 提交中去重（双击/重复触发只产生一次写请求） */
  const submittingRef = useRef(false)
  const onCopiedRef = useRef(options?.onCopied)
  onCopiedRef.current = options?.onCopied

  // 切换到另一颗金丹时整体复位（渲染期间派生调整，React 官方推荐模式）
  const [prevPillId, setPrevPillId] = useState(pill?.id)
  if (pill?.id !== prevPillId) {
    setPrevPillId(pill?.id)
    setMode('readonly')
    setSaveStatus('idle')
    setDraft(pill ? buildDraft(pill) : emptyDraft())
  }

  const baseline = useMemo(() => (pill ? buildDraft(pill) : emptyDraft()), [pill])
  const dirty = useMemo(
    () => mode === 'editing' && stableStringify(draft) !== stableStringify(baseline),
    [mode, draft, baseline],
  )

  const beginEdit = useCallback(() => {
    if (!pill || pill.is_builtin) return
    setDraft(buildDraft(pill))
    setSaveStatus('idle')
    setMode('editing')
  }, [pill])

  const updateDraft = useCallback((patch: Partial<CreatePillRequest>) => {
    setDraft(prev => ({ ...prev, ...patch }))
  }, [])

  const discard = useCallback(() => {
    setDraft(pill ? buildDraft(pill) : emptyDraft())
    setSaveStatus('idle')
    setMode('readonly')
  }, [pill])

  const makeCopy = useCallback(async (): Promise<boolean> => {
    if (!pill || submittingRef.current) return false
    submittingRef.current = true
    setSaveStatus('submitting')
    try {
      const copy = await clonePill(pill.id)
      dispatch({ type: 'ADD_PILL', payload: copy })
      setSaveStatus('idle')
      onCopiedRef.current?.(copy.id)
      return true
    } catch {
      setSaveStatus('error')
      return false
    } finally {
      submittingRef.current = false
    }
  }, [pill, dispatch])

  const save = useCallback(async (): Promise<boolean> => {
    if (!pill || pill.is_builtin || mode !== 'editing' || submittingRef.current) return false
    submittingRef.current = true
    setSaveStatus('submitting')
    try {
      // 写后重读：只有 GET 成功才算保存完成，最终状态以 GET 对象为准
      await editPill(pill.id, draft)
      const fresh = await getPill(pill.id)
      dispatch({ type: 'UPDATE_PILL', payload: fresh })
      setDraft(buildDraft(fresh))
      setSaveStatus('idle')
      setMode('readonly')
      return true
    } catch {
      // 失败保留草稿与编辑态，不丢用户输入
      setSaveStatus('error')
      return false
    } finally {
      submittingRef.current = false
    }
  }, [pill, mode, draft, editPill, dispatch])

  const applyNuwaDraft = useCallback((incoming: DistillationDraft) => {
    if (!pill || pill.is_builtin) return
    setDraft(prev => ({
      ...prev,
      name: incoming.name,
      description: incoming.description,
      tags: [...incoming.tags],
      skill_schema: JSON.parse(JSON.stringify(incoming.skill_schema)),
    }))
    setSaveStatus('idle')
    setMode('editing')
  }, [pill])

  return {
    mode,
    draft,
    dirty,
    saveStatus,
    beginEdit,
    makeCopy,
    save,
    discard,
    applyNuwaDraft,
    updateDraft,
  }
}
