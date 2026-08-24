'use client'

/**
 * 道人编辑器流程 hook
 * 只读/编辑双态 + 独立草稿 + 写后重读回源：
 * - 草稿为深拷贝，与源道人零引用共享
 * - 保存为两步：先 PUT 基础资料，再 PUT 完整服丹编排（原子）；任一步失败保留草稿与编辑态
 * - 全部成功后必须重新 GET，只有 GET 成功才算保存完成，最终状态以 GET 对象为准
 * - 提交中去重：连续触发只产生一次写请求
 */
import { useCallback, useMemo, useRef, useState } from 'react'

import { useAgent } from '@/contexts/AgentContext'
import { getAgent, replacePills, updateAgent } from '@/services/agentService'
import type {
  AgentDetail,
  AgentEditorDraft,
  DistillationDraft,
} from '@/services/types'

export type AgentEditorMode = 'readonly' | 'editing'
export type AgentSaveStatus = 'idle' | 'submitting' | 'error'

export interface AgentEditorFlow {
  mode: AgentEditorMode
  draft: AgentEditorDraft
  dirty: boolean
  saveStatus: AgentSaveStatus
  /** 字段级校验错误（如 name / pills.<key>.weight），保存前客户端校验填充 */
  fieldErrors: Record<string, string>
  /** 进入编辑态 */
  beginEdit(): void
  /** 局部更新草稿 */
  updateDraft(patch: Partial<AgentEditorDraft>): void
  /** 保存草稿：基础资料 → 完整编排 → GET 回读；任何一步失败保留草稿与编辑态 */
  save(): Promise<boolean>
  /** 放弃修改并退出到只读 */
  discard(): void
  /** 放弃未保存的本地修改、回到服务器基线，但保持编辑态（与 discard 退出编辑不同） */
  restoreServerVersion(): void
  /** 女娲蒸馏草稿显式落表（仅调用此方法才写入 name/personality；不触碰服丹编排） */
  applyNuwaDraft(draft: DistillationDraft): void
}

/** 从道人详情构造独立草稿（服丹行重建，与源对象零引用共享） */
function buildDraft(agent: AgentDetail): AgentEditorDraft {
  return {
    name: agent.name,
    avatar: agent.avatar ?? '',
    personality: agent.personality ?? '',
    model_name: agent.model_name,
    proactivity: agent.proactivity,
    status: agent.status,
    pills: [...(agent.agent_pills ?? [])]
      .sort((a, b) => a.sort_order - b.sort_order)
      .map(ap => ({ key: ap.pill_id, pill_id: ap.pill_id, weight: ap.weight })),
  }
}

function emptyDraft(): AgentEditorDraft {
  return {
    name: '',
    avatar: '',
    personality: '',
    model_name: '',
    proactivity: 50,
    status: 'active',
    pills: [],
  }
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

/** 客户端字段级校验：返回 fieldErrors（空对象 = 通过） */
function validateDraft(draft: AgentEditorDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = 'required'
  for (const pill of draft.pills) {
    if (pill.weight <= 0 || pill.weight > 10) errors[`pills.${pill.key}.weight`] = 'range'
  }
  return errors
}

export function useAgentEditorFlow(agent: AgentDetail | null): AgentEditorFlow {
  const { dispatch } = useAgent()
  const [mode, setMode] = useState<AgentEditorMode>('readonly')
  const [saveStatus, setSaveStatus] = useState<AgentSaveStatus>('idle')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [draft, setDraft] = useState<AgentEditorDraft>(() => (agent ? buildDraft(agent) : emptyDraft()))
  /** 提交中去重（双击/重复触发只产生一次写请求） */
  const submittingRef = useRef(false)

  // 切换到另一位道人时整体复位（渲染期间派生调整，React 官方推荐模式）
  const [prevAgentId, setPrevAgentId] = useState(agent?.id)
  if (agent?.id !== prevAgentId) {
    setPrevAgentId(agent?.id)
    setMode('readonly')
    setSaveStatus('idle')
    setFieldErrors({})
    setDraft(agent ? buildDraft(agent) : emptyDraft())
  }

  const baseline = useMemo(() => (agent ? buildDraft(agent) : emptyDraft()), [agent])
  const dirty = useMemo(
    () => mode === 'editing' && stableStringify(draft) !== stableStringify(baseline),
    [mode, draft, baseline],
  )

  const beginEdit = useCallback(() => {
    if (!agent) return
    setDraft(buildDraft(agent))
    setFieldErrors({})
    setSaveStatus('idle')
    setMode('editing')
  }, [agent])

  const updateDraft = useCallback((patch: Partial<AgentEditorDraft>) => {
    setDraft(prev => ({ ...prev, ...patch }))
    setFieldErrors({})
  }, [])

  const discard = useCallback(() => {
    setDraft(agent ? buildDraft(agent) : emptyDraft())
    setFieldErrors({})
    setSaveStatus('idle')
    setMode('readonly')
  }, [agent])

  const restoreServerVersion = useCallback(() => {
    setDraft(agent ? buildDraft(agent) : emptyDraft())
    setFieldErrors({})
    setSaveStatus('idle')
    // 保持编辑态：仅丢弃本地未保存修改
  }, [agent])

  const save = useCallback(async (): Promise<boolean> => {
    if (!agent || mode !== 'editing' || submittingRef.current) return false
    // 字段级校验：失败不发起任何 API
    const errors = validateDraft(draft)
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return false
    }
    submittingRef.current = true
    setSaveStatus('submitting')
    setFieldErrors({})
    try {
      // 第一步：基础资料；第二步：完整服丹编排（原子）。任一失败保留草稿与编辑态
      await updateAgent(agent.id, {
        name: draft.name,
        avatar: draft.avatar,
        personality: draft.personality,
        model_name: draft.model_name,
        proactivity: draft.proactivity,
        status: draft.status,
      })
      await replacePills(
        agent.id,
        draft.pills.map(p => ({ pill_id: p.pill_id, weight: p.weight })),
      )
      // 写后重读：只有 GET 成功才算保存完成，最终状态以 GET 对象为准
      const fresh = await getAgent(agent.id)
      dispatch({ type: 'UPDATE_AGENT', payload: fresh })
      dispatch({ type: 'SET_CURRENT_AGENT', payload: fresh })
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
  }, [agent, mode, draft, dispatch])

  const applyNuwaDraft = useCallback((incoming: DistillationDraft) => {
    if (!agent) return
    setDraft(prev => ({ ...prev, name: incoming.name, personality: incoming.persona_summary }))
    setFieldErrors({})
    setSaveStatus('idle')
    setMode('editing')
  }, [agent])

  return {
    mode,
    draft,
    dirty,
    saveStatus,
    fieldErrors,
    beginEdit,
    updateDraft,
    save,
    discard,
    restoreServerVersion,
    applyNuwaDraft,
  }
}
