'use client'

/**
 * 道人详情页面 - 只读/编辑双态
 * 只读: 完整资料、模型失效警告、语言模式缓存状态、已吸收能力(顺序+剂量,来自
 *       GET /agents/:id/effects;能力名点击跳转来源金丹实例详情)
 * 编辑: useAgentEditorFlow 草稿;编辑过程零 API,保存=基础资料→移除缺失能力→全量编排→
 *       GET 回读(flow 保证);409 冲突保留草稿并刷新合并;失败保留草稿且可重试
 * 删除: 有会话历史(409 delete_has_history)时引导停用;无历史才二次确认硬删除
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  ArrowLeft,
  Ban,
  Brain,
  CheckCircle2,
  Cpu,
  ExternalLink,
  FlaskConical,
  Loader2,
  MessageSquare,
  Pencil,
  Pill as PillIcon,
  Pin,
  PinOff,
  Plus,
  RefreshCw,
  Save,
  Scale,
  Sparkles,
  Trash2,
  TriangleAlert,
  Wand2,
  X,
} from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { avatarInputMaxLength } from '@/lib/avatar-validation'
import { chatSessionHref } from '@/lib/chat-route'
import { useAgentEditorFlow, type AgentEffectsData } from '@/hooks/use-agent-editor-flow'
import { isConsumedResult } from '@/hooks/use-consume-pill-operation'
import { useChatLaunchFlow } from '@/hooks/use-chat-launch-flow'
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes'
import { clearPendingOperation, listPendingOperations, recoverOperation } from '@/lib/pending-operations'
import { pillItemDetailHref } from '@/lib/entity-detail-route'
import { AgentPillComposer } from '@/components/agent-pill-composer'
import { ConsumeInventoryPillModal } from '@/components/agents/consume-inventory-pill-modal'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { EntityAvatar } from '@/components/avatar/entity-avatar'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { ApiError } from '@/services/api'
import * as agentService from '@/services/agentService'
import { listEffects } from '@/services/pillInventoryService'
import * as modelService from '@/services/modelService'
import type { ModelOption } from '@/services/modelService'
import type { AgentDetail, AgentMemory, CreateMemoryRequest, MemoryKind } from '@/services/types'
import type { AgentStatus, PillOperationResult, TensionSeverity } from '@/services/types'
import { formatDateTime } from '@/utils/format'

/** 从 409 响应体提取会话历史数(兼容 data.data.session_count 与 data.session_count 两种嵌套) */
function extractSessionCount(error: ApiError): number {
  const body = error.data as
    | { data?: { session_count?: unknown }; session_count?: unknown }
    | undefined
  const inner = body?.data?.session_count ?? body?.session_count
  return typeof inner === 'number' && Number.isFinite(inner) ? inner : 0
}

// 严重程度徽标样式（label 通过 tSev 渲染）
const SEVERITY_CLASS: Record<TensionSeverity, string> = {
  low: 'bg-sage/15 text-sage border-sage/30',
  medium: 'bg-gold/15 text-gold border-gold/30',
  high: 'bg-primary/10 text-primary border-primary/30',
}

// ========== 本地记忆管理 ==========

/** 记忆类型枚举(与后端 spec §10.1 一致) */
const MEMORY_KINDS: MemoryKind[] = ['user_fact', 'user_preference', 'relationship', 'open_loop', 'episode']

/** kind → i18n 键映射(键名见 agent.memory.kind_*) */
const MEMORY_KIND_LABEL_KEY: Record<MemoryKind, string> = {
  user_fact: 'kind_fact',
  user_preference: 'kind_preference',
  relationship: 'kind_relationship',
  open_loop: 'kind_open_loop',
  episode: 'kind_episode',
}

/** 来源会话跳转地址;非法 UUID 防御性返回 null(chatSessionHref 对畸形 id 抛错) */
function safeChatHref(sessionId: string): string | null {
  try {
    return chatSessionHref(sessionId)
  } catch {
    return null
  }
}

/**
 * 本地记忆管理区(只读态专属):
 * 开关(memory_enabled)→更新道人;开启后加载列表;支持 kind 筛选/新建/编辑/置顶/删除/清空/跳转来源。
 * 关闭或旧服务端未返回 memory_enabled 时,仅展示开关与提示文案,不发起列表请求。
 */
function LocalMemorySection({ agent }: { agent: AgentDetail }) {
  const tMem = useTranslations('agent.memory')
  const tCommon = useTranslations('common')

  const [enabled, setEnabled] = useState(agent.memory_enabled ?? false)
  const [memories, setMemories] = useState<AgentMemory[]>([])
  const [kindFilter, setKindFilter] = useState<MemoryKind | ''>('')
  const [loadState, setLoadState] = useState<'idle' | 'loading' | 'error'>('idle')
  const [formOpen, setFormOpen] = useState<AgentMemory | 'new' | null>(null)
  const [saving, setSaving] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [notice, setNotice] = useState('')
  const [formError, setFormError] = useState('')
  // 应用内确认框(WKWebView 不实现 window.confirm,桌面端 confirm 恒 false)
  const [confirmDelete, setConfirmDelete] = useState<AgentMemory | null>(null)
  const [confirmClear, setConfirmClear] = useState(false)

  /** kind 标签文案(键名映射见 MEMORY_KIND_LABEL_KEY) */
  const kindLabel = (kind: MemoryKind): string => tMem(MEMORY_KIND_LABEL_KEY[kind])

  const loadMemories = useCallback(
    (kind: MemoryKind | '' = kindFilter) => {
      setLoadState('loading')
      agentService
        .fetchAgentMemories(agent.id, kind || undefined)
        .then(list => {
          setMemories(list)
          setLoadState('idle')
        })
        .catch(() => setLoadState('error'))
    },
    [agent.id, kindFilter]
  )

  useEffect(() => {
    if (!enabled) return
    loadMemories()
  }, [enabled, loadMemories])

  /** 开关:更新道人 memory_enabled;开启后立即加载列表,关闭时清空本地列表 */
  const handleToggle = async (next: boolean) => {
    setToggling(true)
    setNotice('')
    try {
      await agentService.updateAgent(agent.id, {}, next)
      setEnabled(next)
      if (next) loadMemories()
    } catch {
      setNotice(tMem('op_failed'))
    } finally {
      setToggling(false)
    }
  }

  /** 置顶/取消置顶 */
  const handlePin = async (memory: AgentMemory, pinned: boolean) => {
    setNotice('')
    try {
      const updated = await agentService.updateAgentMemory(agent.id, memory.id, { pinned })
      setMemories(prev => prev.map(m => (m.id === memory.id ? updated : m)))
      setNotice(tMem('saved'))
    } catch {
      setNotice(tMem('op_failed'))
    }
  }

  /** 删除单条(物理删除,应用内二次确认) */
  const handleDelete = (memory: AgentMemory) => {
    setConfirmDelete(memory)
  }

  const doDeleteMemory = async (memory: AgentMemory) => {
    setNotice('')
    try {
      await agentService.deleteAgentMemory(agent.id, memory.id)
      setMemories(prev => prev.filter(m => m.id !== memory.id))
      setNotice(tMem('saved'))
    } catch {
      setNotice(tMem('op_failed'))
    }
  }

  /** 清空全部(物理删除,应用内二次确认) */
  const handleClear = () => {
    setConfirmClear(true)
  }

  const doClearMemories = async () => {
    setClearing(true)
    setNotice('')
    try {
      await agentService.clearAgentMemories(agent.id)
      setMemories([])
      setNotice(tMem('saved'))
    } catch {
      setNotice(tMem('op_failed'))
    } finally {
      setClearing(false)
    }
  }

  /** 表单提交:新建或编辑后落库并更新列表 */
  const handleFormSave = async (input: CreateMemoryRequest, editingMemory: AgentMemory | null) => {
    setSaving(true)
    setNotice('')
    setFormError('')
    try {
      if (editingMemory) {
        const updated = await agentService.updateAgentMemory(agent.id, editingMemory.id, input)
        setMemories(prev => prev.map(m => (m.id === editingMemory.id ? updated : m)))
      } else {
        const created = await agentService.createAgentMemory(agent.id, input)
        setMemories(prev => [created, ...prev])
      }
      setFormOpen(null)
      setNotice(tMem('saved'))
    } catch {
      setFormError(tMem('op_failed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="dao-card mb-6 p-5">
      <div className="mb-3 flex min-w-0 flex-wrap items-center gap-2">
        <Brain className="h-4 w-4 shrink-0 text-sage" />
        <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">
          {tMem('title')}
        </h2>
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          aria-label={tMem('enabled')}
          onClick={() => void handleToggle(!enabled)}
          disabled={toggling}
          className="ml-auto inline-flex shrink-0 items-center gap-2"
        >
          <span
            className={`flex h-5 w-9 shrink-0 items-center rounded-full border px-0.5 transition-colors ${
              enabled ? 'border-sage/50 bg-sage/30' : 'border-border/70 bg-muted'
            }`}
          >
            <span
              className={`h-3.5 w-3.5 rounded-full transition-transform ${
                enabled ? 'translate-x-4 bg-sage' : 'translate-x-0 bg-muted-foreground/60'
              }`}
            />
          </span>
          <span className="text-sm font-medium text-foreground">{tMem('enabled')}</span>
        </button>
      </div>

      {!enabled ? (
        <p className="text-sm text-muted-foreground">{tMem('disabled_hint')}</p>
      ) : (
        <>
          <p className="mb-3 text-xs text-sage">{tMem('enabled_hint')}</p>

          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            {/* kind 筛选 */}
            <div className="flex flex-wrap gap-1.5">
              {(['', ...MEMORY_KINDS] as Array<MemoryKind | ''>).map(filter => (
                <button
                  key={filter || 'all'}
                  type="button"
                  aria-pressed={kindFilter === filter}
                  onClick={() => setKindFilter(filter)}
                  className={`rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
                    kindFilter === filter
                      ? 'border-gold/50 bg-gold/15 text-gold'
                      : 'border-border/70 bg-muted text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {filter === '' ? tMem('all') : kindLabel(filter)}
                </button>
              ))}
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => {
                  setFormError('')
                  setFormOpen('new')
                }}
                className="dao-btn-primary whitespace-nowrap text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                {tMem('create')}
              </button>
              {memories.length > 0 && (
                <button
                  type="button"
                  onClick={() => void handleClear()}
                  disabled={clearing}
                  className="dao-btn-ghost whitespace-nowrap text-xs text-primary hover:text-primary/80"
                >
                  {clearing ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="h-3.5 w-3.5" />
                  )}
                  {tMem('clear')}
                </button>
              )}
            </div>
          </div>

          {loadState === 'loading' && (
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin text-gold" />
              {tMem('loading')}
            </p>
          )}

          {loadState === 'error' && (
            <div className="mb-3">
              <ActionFeedback
                status="error"
                message={tMem('error')}
                onRetry={() => loadMemories()}
                retryLabel={tCommon('retry')}
              />
            </div>
          )}

          {loadState === 'idle' &&
            (memories.length === 0 ? (
              <p className="text-sm text-muted-foreground">{tMem('empty')}</p>
            ) : (
              <ul className="space-y-2">
                {memories.map(memory => {
                  const sourceHref = memory.source_session_id
                    ? safeChatHref(memory.source_session_id)
                    : null
                  return (
                    <li
                      key={memory.id}
                      className="rounded-lg border border-border/70 bg-muted px-3 py-2.5"
                    >
                      <div className="mb-1 flex flex-wrap items-center gap-2">
                        <span className="shrink-0 whitespace-nowrap rounded-full border border-sage/30 bg-sage/15 px-2 py-0.5 text-[10px] text-sage">
                          {kindLabel(memory.kind)}
                        </span>
                        {memory.pinned && (
                          <span className="flex shrink-0 items-center gap-0.5 whitespace-nowrap rounded-full border border-gold/40 bg-gold/10 px-2 py-0.5 text-[10px] text-gold">
                            <Pin className="h-2.5 w-2.5" />
                            {tMem('pinned')}
                          </span>
                        )}
                        <span className="ml-auto shrink-0 whitespace-nowrap text-[10px] text-muted-foreground">
                          {tMem('importance')} {memory.importance} · {tMem('confidence')}{' '}
                          {Math.round(memory.confidence * 100)}%
                        </span>
                      </div>
                      <p className="whitespace-pre-wrap break-words text-sm text-foreground/90">
                        {memory.content}
                      </p>
                      {memory.keywords.length > 0 && (
                        <p className="mt-1 truncate text-xs text-muted-foreground">
                          {memory.keywords.join(' · ')}
                        </p>
                      )}
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          onClick={() => void handlePin(memory, !memory.pinned)}
                          className="inline-flex items-center gap-1 whitespace-nowrap text-xs text-muted-foreground transition-colors hover:text-foreground"
                        >
                          {memory.pinned ? (
                            <PinOff className="h-3 w-3" />
                          ) : (
                            <Pin className="h-3 w-3" />
                          )}
                          {memory.pinned ? tMem('unpin') : tMem('pin')}
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setFormError('')
                            setFormOpen(memory)
                          }}
                          className="inline-flex items-center gap-1 whitespace-nowrap text-xs text-muted-foreground transition-colors hover:text-foreground"
                        >
                          <Pencil className="h-3 w-3" />
                          {tMem('edit')}
                        </button>
                        <button
                          type="button"
                          onClick={() => void handleDelete(memory)}
                          className="inline-flex items-center gap-1 whitespace-nowrap text-xs text-primary transition-colors hover:text-primary/80"
                        >
                          <Trash2 className="h-3 w-3" />
                          {tMem('delete')}
                        </button>
                        {sourceHref && (
                          <Link
                            href={sourceHref}
                            className="inline-flex items-center gap-1 whitespace-nowrap text-xs text-gold transition-colors hover:text-gold/80"
                          >
                            <ExternalLink className="h-3 w-3" />
                            {tMem('source_jump')}
                          </Link>
                        )}
                      </div>
                    </li>
                  )
                })}
              </ul>
            ))}

          {formOpen !== null && (
            <MemoryForm
              key={formOpen === 'new' ? 'new' : formOpen.id}
              initial={formOpen === 'new' ? null : formOpen}
              saving={saving}
              error={formError}
              onSave={input => void handleFormSave(input, formOpen === 'new' ? null : formOpen)}
              onCancel={() => {
                setFormOpen(null)
                setFormError('')
              }}
            />
          )}

          {notice !== '' && (
            <p role="status" className="mt-3 text-xs text-sage">
              {notice}
            </p>
          )}

          {/* 删除/清空确认(应用内对话框,web/桌面行为一致) */}
          {confirmDelete && (
            <ConfirmDialog
              description={tMem('delete_confirm')}
              confirmLabel={tMem('delete_confirm_btn')}
              cancelLabel={tMem('cancel_edit')}
              destructive
              onConfirm={() => {
                const memory = confirmDelete
                setConfirmDelete(null)
                void doDeleteMemory(memory)
              }}
              onCancel={() => setConfirmDelete(null)}
            />
          )}
          {confirmClear && (
            <ConfirmDialog
              description={tMem('clear_confirm')}
              confirmLabel={tMem('clear_confirm_btn')}
              cancelLabel={tMem('cancel_edit')}
              destructive
              onConfirm={() => {
                setConfirmClear(false)
                void doClearMemories()
              }}
              onCancel={() => setConfirmClear(false)}
            />
          )}
        </>
      )}
    </section>
  )
}

/** 记忆新建/编辑表单(kind 下拉、内容、关键词、重要性 1-5、置顶) */
function MemoryForm({
  initial,
  saving,
  error,
  onSave,
  onCancel,
}: {
  initial: AgentMemory | null
  saving: boolean
  error: string
  onSave: (input: CreateMemoryRequest) => void
  onCancel: () => void
}) {
  const tMem = useTranslations('agent.memory')
  const [kind, setKind] = useState<MemoryKind>(initial?.kind ?? 'user_fact')
  const [content, setContent] = useState(initial?.content ?? '')
  const [keywords, setKeywords] = useState(initial?.keywords?.join(', ') ?? '')
  const [importance, setImportance] = useState(initial?.importance ?? 3)
  const [pinned, setPinned] = useState(initial?.pinned ?? false)
  const [contentError, setContentError] = useState('')

  const kindLabel = (k: MemoryKind): string => tMem(MEMORY_KIND_LABEL_KEY[k])

  const submit = () => {
    if (!content.trim()) {
      setContentError(tMem('content_required'))
      return
    }
    onSave({
      kind,
      content: content.trim(),
      keywords: keywords
        .split(/[,，]/)
        .map(k => k.trim())
        .filter(Boolean)
        .slice(0, 12),
      importance,
      pinned,
    })
  }

  return (
    <div className="mt-3 rounded-lg border border-border/70 bg-muted p-3">
      <div className="space-y-3">
        <div>
          <label className="dao-label">{tMem('kind_label')}</label>
          <select
            aria-label={tMem('kind_label')}
            value={kind}
            onChange={e => setKind(e.target.value as MemoryKind)}
            className="dao-input"
          >
            {MEMORY_KINDS.map(k => (
              <option key={k} value={k}>
                {kindLabel(k)}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="dao-label">{tMem('content_label')}</label>
          <textarea
            value={content}
            onChange={e => setContent(e.target.value)}
            rows={3}
            maxLength={500}
            className="dao-textarea"
            placeholder={tMem('content_label')}
          />
          {contentError !== '' && <p className="mt-1 text-xs text-primary">{contentError}</p>}
        </div>

        <div>
          <label className="dao-label">{tMem('keywords')}</label>
          <input
            value={keywords}
            onChange={e => setKeywords(e.target.value)}
            className="dao-input py-1.5 text-sm"
            placeholder={tMem('keywords_hint')}
          />
          <p className="mt-1 text-[10px] text-sage">{tMem('keywords_hint')}</p>
        </div>

        <div>
          <label className="dao-label flex items-center gap-1.5">
            {tMem('importance')}
            <span className="ml-auto font-mono text-xs text-sage">{importance}/5</span>
          </label>
          <input
            type="range"
            min={1}
            max={5}
            step={1}
            value={importance}
            aria-label={tMem('importance')}
            onChange={e => setImportance(Number(e.target.value))}
            className="mt-1.5 w-full accent-gold"
          />
        </div>

        <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
          <input
            type="checkbox"
            checked={pinned}
            onChange={e => setPinned(e.target.checked)}
            className="h-4 w-4 accent-gold"
          />
          {tMem('pinned')}
        </label>

        {error !== '' && <p className="text-xs text-primary">{error}</p>}

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={submit}
            disabled={saving}
            className="dao-btn-primary whitespace-nowrap text-xs"
          >
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            {tMem('create_btn')}
          </button>
          <button
            type="button"
            onClick={onCancel}
            disabled={saving}
            className="dao-btn-ghost whitespace-nowrap text-xs"
          >
            <X className="h-3.5 w-3.5" />
            {tMem('cancel_edit')}
          </button>
        </div>
      </div>
    </div>
  )
}

interface AgentDetailPageProps {
  agentId?: string
}

export default function AgentDetailPage({ agentId }: AgentDetailPageProps) {
  const t = useTranslations('agent')
  const tStatus = useTranslations('agentCard.status')
  const tEditor = useTranslations('agentDetail.editor')
  const tSev = useTranslations('agent.severity')
  const tLaunch = useTranslations('chatView.launch')
  const tCommon = useTranslations('common')
  const router = useRouter()

  const { state: agentState, fetchAgent, dispatch } = useAgent()
  const launchFlow = useChatLaunchFlow()

  const agent = agentState.currentAgent
  // 已吸收能力列表（服用快照 + effects_revision 乐观锁）；null=未加载/失败
  const [effectsData, setEffectsData] = useState<AgentEffectsData | null>(null)
  const [effectsLoadFailed, setEffectsLoadFailed] = useState(false)
  // —— 服用金丹（库存选择弹窗 + 页内提示与保存锁定）——
  const [consumeOpen, setConsumeOpen] = useState(false)
  const [consumeSuccess, setConsumeSuccess] = useState(false)
  const [consumeSyncFailed, setConsumeSyncFailed] = useState(false)
  const [consumePending, setConsumePending] = useState(false)
  const [consumePendingRemoval, setConsumePendingRemoval] = useState(false)
  /** 最近一次能力同步是否成功（弹窗关闭时据此决定是否转页内「重试同步」提示） */
  const syncOkRef = useRef(true)
  /** 弹窗打开时固定的道人 ID（服用/同步都以它为准，不随渲染切换） */
  const capturedAgentIdRef = useRef<string | null>(null)
  const loadEffects = useCallback(async () => {
    if (!agentId) return
    setEffectsLoadFailed(false)
    try {
      setEffectsData(await listEffects(agentId))
    } catch {
      setEffectsLoadFailed(true)
    }
  }, [agentId])
  const flow = useAgentEditorFlow(agent, effectsData, setEffectsData, { mutationBlocked: consumeOpen })
  // 离开提示同时覆盖草稿 dirty 与未确认操作（弹窗打开/结果未知/同步失败）
  useUnsavedChanges(flow.dirty || consumeOpen || consumePending || consumeSyncFailed, t('unsavedConfirm'))

  // —— 服用金丹：能力同步与页内提示 ——

  /** 同步服用后的能力（listEffects → reconcile 并入草稿）；失败返回 false（绝不再次 consume） */
  const syncConsumedEffects = useCallback(
    async (targetAgentId: string): Promise<boolean> => {
      try {
        const fresh = await listEffects(targetAgentId)
        flow.reconcileConsumedEffects(targetAgentId, fresh)
        syncOkRef.current = true
        return true
      } catch {
        syncOkRef.current = false
        return false
      }
    },
    [flow],
  )

  /** 读取当前道人的 pending 服用记录（重挂载恢复：先读能力与 pending，再决定提示） */
  const refreshPending = useCallback(() => {
    setConsumePending(
      listPendingOperations().some(r => r.action === 'consume' && r.consumeInput?.agentId === agentId),
    )
  }, [agentId])

  // 挂载恢复:读会话级 pending 记录到状态。不能改用 state 初始化——SSR/水合期无
  // sessionStorage,初始值必须与服务器一致(结果未知提示只能水合后浮现)
  /* eslint-disable react-hooks/set-state-in-effect -- 外部存储一次性读取,非派生可算 */
  useEffect(() => {
    refreshPending()
  }, [refreshPending])
  /* eslint-enable react-hooks/set-state-in-effect */

  // 道人 ID 变化：关闭弹窗、丢弃旧响应写入（operation 恢复记录保留，回到该道人时提示仍在）
  const lastPageAgentIdRef = useRef(agentId)
  useEffect(() => {
    if (lastPageAgentIdRef.current === agentId) return
    lastPageAgentIdRef.current = agentId
    setConsumeOpen(false)
    setConsumeSyncFailed(false)
    setConsumeSuccess(false)
    setConsumePendingRemoval(false)
    refreshPending()
  }, [agentId, refreshPending])

  // 成功提示短暂展示后自动消失
  useEffect(() => {
    if (!consumeSuccess) return
    const timer = setTimeout(() => setConsumeSuccess(false), 4000)
    return () => clearTimeout(timer)
  }, [consumeSuccess])

  /** 打开库存选择弹窗；草稿有未保存的移除意图时先提示保存（禁止对同版本重复服用） */
  const handleConsumeOpen = useCallback(() => {
    const pendingRemovals = (effectsData?.effects ?? []).filter(
      e => !flow.draft.effects.some(d => d.effect_id === e.id),
    )
    if (pendingRemovals.length > 0) {
      setConsumePendingRemoval(true)
      return
    }
    setConsumePendingRemoval(false)
    capturedAgentIdRef.current = agentId ?? null
    syncOkRef.current = true
    setConsumeSuccess(false)
    setConsumeSyncFailed(false)
    setConsumeOpen(true)
  }, [agentId, effectsData, flow.draft.effects])

  /** 弹窗关闭：最后一次同步失败时转为页内「重试同步」提示 */
  const handleConsumeClose = useCallback(() => {
    setConsumeOpen(false)
    if (!syncOkRef.current) setConsumeSyncFailed(true)
    refreshPending()
  }, [refreshPending])

  /** 服用成功后的能力同步（弹窗 onCommitted；失败抛出让弹窗只允许「重试同步」） */
  const handleConsumed = useCallback(
    async (_result: PillOperationResult) => {
      const target = capturedAgentIdRef.current ?? agentId
      if (!target) throw new Error('agent not loaded')
      const ok = await syncConsumedEffects(target)
      if (!ok) throw new Error('effects sync failed')
      setConsumeSuccess(true)
    },
    [agentId, syncConsumedEffects],
  )

  /** 页内「重试同步」：仅再次 GET effects 并合并，绝不再次 POST consume */
  const handleRetrySync = useCallback(() => {
    const target = capturedAgentIdRef.current ?? agentId
    if (!target) return
    void syncConsumedEffects(target).then(ok => {
      if (ok) {
        setConsumeSyncFailed(false)
        setConsumeSuccess(true)
      }
    })
  }, [agentId, syncConsumedEffects])

  /** 「核对」：恢复查询 pending 记录；已提交→同步能力，未提交(404)→直接结束未知状态 */
  const handleVerifyPending = useCallback(async () => {
    if (!agentId) return
    const records = listPendingOperations().filter(
      r => r.action === 'consume' && r.consumeInput?.agentId === agentId,
    )
    for (const record of records) {
      try {
        const result = await recoverOperation(record.key)
        clearPendingOperation(record.key)
        if (result && record.consumeInput && isConsumedResult(result, record.key, record.consumeInput)) {
          const ok = await syncConsumedEffects(agentId)
          if (!ok) setConsumeSyncFailed(true)
        }
      } catch {
        // 查询失败：仍未知，保留 pending 与提示
      }
    }
    refreshPending()
  }, [agentId, refreshPending, syncConsumedEffects])

  /** 草稿未保存的移除意图数（该版本仍活跃，禁止重复服用） */
  const pendingRemovalCount = (effectsData?.effects ?? []).filter(
    e => !flow.draft.effects.some(d => d.effect_id === e.id),
  ).length

  // null = 尚未加载成功(不做失效判定);数组 = 已加载(可能为空)
  const [modelOptions, setModelOptions] = useState<ModelOption[] | null>(null)
  const [deleteArmed, setDeleteArmed] = useState(false)
  const [deleteStatus, setDeleteStatus] = useState<'idle' | 'submitting' | 'error'>('idle')
  const [historyCount, setHistoryCount] = useState<number | null>(null)
  const [deactivateStatus, setDeactivateStatus] = useState<'idle' | 'submitting' | 'error'>('idle')

  // 加载数据
  useEffect(() => {
    if (agentId) {
      fetchAgent(agentId)
      loadEffects()
    }
    let cancelled = false
    modelService
      .options()
      .then(opts => {
        if (cancelled) return
        setModelOptions([...opts].sort((a, b) => Number(b.is_default) - Number(a.is_default)))
      })
      .catch(() => {
        // 模型选项加载失败:保留下拉空态提示,不做失效判定
      })
    return () => {
      cancelled = true
    }
  }, [agentId, fetchAgent, loadEffects])

  /** 模型失效判定:选项已成功加载且当前模型不在启用列表中 */
  const isModelInvalid = (modelName: string) =>
    modelOptions !== null && modelName !== '' && !modelOptions.some(o => o.name === modelName)

  /** 真正执行删除(二次确认后或失败重试时调用) */
  const performDelete = async () => {
    if (!agent) return
    setDeleteStatus('submitting')
    try {
      await agentService.deleteAgent(agent.id)
      dispatch({ type: 'REMOVE_AGENT', payload: agent.id })
      router.push('/agents')
    } catch (error) {
      if (error instanceof ApiError && error.errorCode === 'service.agent.delete_has_history') {
        // 有会话历史:引导停用而非硬删除
        setHistoryCount(extractSessionCount(error))
        setDeleteStatus('idle')
        setDeleteArmed(false)
        return
      }
      setDeleteStatus('error')
      setDeleteArmed(false)
    }
  }

  /** 自定义删除:二次确认(第一次点击仅进入待确认态) */
  const handleDelete = () => {
    if (!deleteArmed) {
      setDeleteArmed(true)
      return
    }
    void performDelete()
  }

  /** 停用:更新状态后回读详情(经 dispatch 写回,无整页闪烁) */
  const performDeactivate = async () => {
    if (!agent) return
    setDeactivateStatus('submitting')
    try {
      await agentService.updateAgent(agent.id, { status: 'inactive' })
      const fresh = await agentService.getAgent(agent.id)
      dispatch({ type: 'UPDATE_AGENT', payload: fresh })
      dispatch({ type: 'SET_CURRENT_AGENT', payload: fresh })
      setHistoryCount(null)
      setDeactivateStatus('idle')
    } catch {
      setDeactivateStatus('error')
    }
  }

  /** 保存(写后重读由 flow 保证);服用进行中/结果未知/同步未完成时禁止保存草稿 */
  const handleSave = () => {
    if (consumeOpen || consumePending || consumeSyncFailed) return
    void flow.save()
  }

  // ========== 链接无效 / 加载 / 错误 / 不存在 四态 ==========
  // 状态顺序必须是:无 id 判定为无效链接;id 与加载归属不符按加载;
  // 只有详情 API 明确 404 才判定"不存在或已删除";其余一律不闪现删除。
  if (!agentId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{t('invalidLink')}</p>
          <Link href="/agents" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {t('backToList')}
          </Link>
        </div>
      </div>
    )
  }

  const detailLoad = agentState.detailLoad

  if (detailLoad.id !== agentId || detailLoad.status === 'loading') {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    )
  }

  if (detailLoad.status === 'error') {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="mb-4 max-w-xl break-words text-center text-sm text-muted-foreground">
            {detailLoad.error}
          </p>
          <button type="button" onClick={() => fetchAgent(agentId)} className="dao-btn-ghost">
            {tCommon('retry')}
          </button>
        </div>
      </div>
    )
  }

  if (detailLoad.status === 'not-found') {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{t('notFound')}</p>
          <Link href="/agents" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {t('backToList')}
          </Link>
        </div>
      </div>
    )
  }

  // 兜底:就绪状态与当前实体未同时到位(如快速切换)时按加载处理,绝不误报"已删除"
  if (!(detailLoad.status === 'ready' && agent?.id === agentId)) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    )
  }

  const languagePattern = agent.language_pattern

  // ========== 只读态 ==========
  if (flow.mode === 'readonly') {
    const effects = effectsData?.effects ?? []
    const modelInvalid = isModelInvalid(agent.model_name)
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <Link
          href="/agents"
          className="mb-4 inline-flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-gold"
        >
          <ArrowLeft className="h-4 w-4" />
          {t('backToList')}
        </Link>

        {/* 道人信息头部 */}
        <div className="dao-card mb-6 p-5 md:p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start">
            <EntityAvatar name={agent.name} src={agent.avatar} size="lg" shape="square" alt={agent.name} />

            <div className="min-w-0 flex-1">
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <h1 className="min-w-0 break-words font-serif text-xl font-bold text-foreground md:text-2xl">
                  {agent.name}
                </h1>
                <span
                  className={`shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-[10px] ${
                    agent.status === 'active'
                      ? 'border-sage/30 bg-sage/15 text-sage'
                      : 'border-border bg-muted text-muted-foreground'
                  }`}
                >
                  {agent.status === 'active' ? tStatus('active') : tStatus('inactive')}
                </span>
                {modelInvalid && (
                  <span
                    className="flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full border border-gold/40 bg-gold/10 px-2 py-0.5 text-[10px] text-gold"
                    title={t('modelInvalidWarning')}
                  >
                    <TriangleAlert className="h-3 w-3" />
                    {t('modelInvalidBadge')}
                  </span>
                )}
              </div>

              <p className="mb-3 whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">
                {agent.personality || t('persona.empty')}
              </p>

              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span className="flex min-w-0 items-center gap-1">
                  <Cpu className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{agent.model_name}</span>
                </span>
                <span className="flex items-center gap-1">
                  <PillIcon className="h-3.5 w-3.5" />
                  {t('pillsCount', { count: effects.length })}
                </span>
                <span className="flex items-center gap-1" title={tEditor('proactivityHint')}>
                  <Sparkles className="h-3.5 w-3.5" />
                  {tEditor('proactivity')}: {agent.proactivity}
                </span>
                <span className="flex items-center gap-1">
                  {t('meta.createdAt')} {formatDateTime(agent.created_at)}
                </span>
              </div>

              <div className="mt-4 flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  onClick={() => void launchFlow.launchSingle(agent.id)}
                  disabled={launchFlow.state.status === 'submitting' || agent.status !== 'active'}
                  title={agent.status !== 'active' ? t('inactiveChatHint') : undefined}
                  className="dao-btn-primary whitespace-nowrap text-sm disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {launchFlow.state.status === 'submitting' ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <MessageSquare className="h-4 w-4" />
                  )}
                  {t('startChatCta')}
                </button>
                <button
                  type="button"
                  onClick={flow.beginEdit}
                  className="dao-btn-ghost whitespace-nowrap text-sm"
                >
                  <Pencil className="h-4 w-4" />
                  {t('editCta')}
                </button>
                {historyCount === null && (
                  <button
                    type="button"
                    onClick={handleDelete}
                    disabled={deleteStatus === 'submitting'}
                    className="dao-btn-ghost whitespace-nowrap text-sm text-primary hover:text-primary/80"
                  >
                    {deleteStatus === 'submitting' ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="h-4 w-4" />
                    )}
                    {deleteArmed ? t('deleteConfirmCta') : t('deleteCta')}
                  </button>
                )}
              </div>

              {launchFlow.state.status !== 'idle' && (
                <div className="mt-3 rounded-lg border border-gold/40 bg-gold/5 px-3 py-2.5 shadow-sm">
                  {launchFlow.state.status === 'submitting' ? (
                    <ActionFeedback status="submitting" message={tLaunch('submitting')} />
                  ) : (
                    <>
                      <ActionFeedback
                        status="error"
                        message={launchFlow.state.message}
                        onRetry={() => {
                          void launchFlow.retry()
                        }}
                        retryLabel={tLaunch('retry')}
                      />
                      {launchFlow.state.errorCode === 'service.chat.model_unavailable' && (
                        <Link
                          href="/settings"
                          className="mt-2 inline-flex max-w-full whitespace-normal break-words text-left text-xs font-medium text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                        >
                          {tLaunch('modelSettings')}
                        </Link>
                      )}
                    </>
                  )}
                </div>
              )}

              {historyCount !== null && (
                <div className="mt-3 rounded-lg border border-gold/40 bg-gold/5 px-3 py-2.5 shadow-sm">
                  <p className="flex items-start gap-2 break-words text-sm text-foreground/90">
                    <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-gold" />
                    {t('deleteHistoryHint', { count: historyCount })}
                  </p>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <button
                      type="button"
                      onClick={() => void performDeactivate()}
                      disabled={deactivateStatus === 'submitting'}
                      className="dao-btn-gold whitespace-nowrap text-xs"
                    >
                      {deactivateStatus === 'submitting' ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <Ban className="h-3.5 w-3.5" />
                      )}
                      {t('deactivateCta')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setHistoryCount(null)}
                      className="dao-btn-ghost whitespace-nowrap text-xs"
                    >
                      {t('cancelCta')}
                    </button>
                  </div>
                  {deactivateStatus === 'error' && (
                    <div className="mt-2">
                      <ActionFeedback
                        status="error"
                        message={t('deactivateFailed')}
                        onRetry={() => void performDeactivate()}
                        retryLabel={tCommon('retry')}
                      />
                    </div>
                  )}
                </div>
              )}

              {deleteStatus === 'error' && (
                <div className="mt-3">
                  <ActionFeedback
                    status="error"
                    message={t('deleteFailed')}
                    onRetry={() => void performDelete()}
                    retryLabel={tCommon('retry')}
                  />
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 语言模式缓存状态 */}
        {languagePattern && (
          <div className="dao-card mb-6 p-4">
            <div className="mb-3 flex min-w-0 flex-wrap items-center gap-2">
              <Wand2 className="h-4 w-4 shrink-0 text-sage" />
              <h2 className="min-w-0 break-words font-serif text-sm font-bold text-gold">
                {t('languagePattern.title')}
              </h2>
              <span
                className={`shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-[10px] ${
                  languagePattern.is_valid
                    ? 'border-sage/30 bg-sage/15 text-sage'
                    : 'border-gold/30 bg-gold/15 text-gold'
                }`}
              >
                {languagePattern.is_valid
                  ? t('languagePattern.synthesized')
                  : t('languagePattern.stale')}
              </span>
            </div>

            {!languagePattern.is_valid ? (
              <div className="flex items-start gap-2 rounded-lg border border-gold/20 bg-gold/10 px-3 py-2 text-xs text-gold">
                <RefreshCw className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>{t('languagePattern.staleDesc')}</span>
              </div>
            ) : (
              <>
                {languagePattern.emergence_rules.length > 0 && (
                  <div className="mb-3">
                    <p className="mb-1.5 text-xs text-muted-foreground">
                      {t('languagePattern.emergenceRules')}
                    </p>
                    <ul className="space-y-1">
                      {languagePattern.emergence_rules.map((rule, i) => (
                        <li key={i} className="flex items-start gap-1.5 text-xs text-foreground/90">
                          <Sparkles className="mt-0.5 h-3 w-3 shrink-0 text-gold" />
                          <span>{rule}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {languagePattern.inner_tensions.length > 0 && (
                  <div>
                    <p className="mb-1.5 flex items-center gap-1 text-xs text-muted-foreground">
                      <TriangleAlert className="h-3 w-3 text-primary" />
                      {t('languagePattern.tensions', {
                        count: languagePattern.inner_tensions.length,
                      })}
                    </p>
                    <ul className="space-y-2">
                      {languagePattern.inner_tensions.map((tension, i) => (
                        <li
                          key={i}
                          className="flex items-start gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-xs"
                        >
                          <div className="min-w-0 flex-1">
                            <div className="mb-0.5 flex flex-wrap items-center gap-2">
                              <span className="font-medium text-foreground/90">
                                {tension.dimension}
                              </span>
                              <span
                                className={`shrink-0 whitespace-nowrap rounded-full border px-1.5 py-px text-[10px] ${SEVERITY_CLASS[tension.severity] ?? SEVERITY_CLASS.medium}`}
                              >
                                {tSev(tension.severity)}
                              </span>
                            </div>
                            <p className="leading-relaxed text-muted-foreground">
                              {tension.description}
                            </p>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* 本地记忆管理(开关/筛选/增删改/置顶/清空/跳转来源) */}
        <LocalMemorySection agent={agent} />

        {/* 已吸收能力(只读:按编排顺序展示能力名与剂量;名称点击跳转来源金丹实例) */}
        <section className="dao-card p-5">
          <div className="mb-4 flex items-center gap-2">
            <FlaskConical className="h-4 w-4 shrink-0 text-gold" />
            <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">
              {t('pills.title')}
            </h2>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t('pillsCount', { count: effects.length })}
            </span>
          </div>
          {effectsLoadFailed ? (
            <div>
              <ActionFeedback
                status="error"
                message={t('effectsLoadFailed')}
                onRetry={() => void loadEffects()}
                retryLabel={tCommon('retry')}
              />
            </div>
          ) : effectsData === null ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin text-gold" />
              {t('loading')}
            </p>
          ) : effects.length === 0 ? (
            <div>
              <p className="text-sm text-muted-foreground">{t('pills.empty')}</p>
              <p className="mt-1 text-xs text-sage">{t('pills.emptyHint')}</p>
            </div>
          ) : (
            <ol className="space-y-2">
              {effects.map((effect, index) => (
                <li
                  key={effect.id}
                  className="flex items-center gap-3 rounded-lg border border-border/70 bg-muted px-3 py-2"
                >
                  <span className="shrink-0 text-xs text-muted-foreground">{index + 1}.</span>
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gold/15 text-gold">
                    <FlaskConical className="h-4 w-4" />
                  </span>
                  <Link
                    href={pillItemDetailHref(effect.item_id)}
                    className="min-w-0 flex-1 truncate text-sm font-medium text-foreground transition-colors hover:text-gold"
                  >
                    {effect.name}
                  </Link>
                  <span className="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-muted-foreground">
                    <Scale className="h-3 w-3" />
                    {t('pills.weight')} {effect.weight}
                  </span>
                </li>
              ))}
            </ol>
          )}
        </section>

        {/* 服用金丹结果未核对:仅提示+核对(结果未知时保存/再次服用被锁定) */}
        {consumePending && (
          <div
            role="alert"
            className="mb-4 mt-4 flex flex-wrap items-center gap-2 rounded-lg border border-gold/40 bg-gold/10 px-3 py-2.5 text-xs text-gold"
          >
            <AlertCircle className="h-3.5 w-3.5 shrink-0" />
            <span>{tEditor('consume.uncertain')}</span>
            <button
              type="button"
              onClick={() => void handleVerifyPending()}
              className="ml-auto shrink-0 rounded-md border border-gold/40 px-2 py-0.5 font-medium hover:bg-gold/20"
            >
              {tEditor('consume.verify')}
            </button>
          </div>
        )}
      </div>
    )
  }

  // ========== 编辑态 ==========
  const draft = flow.draft
  const draftModelInvalid = isModelInvalid(draft.model_name)
  const selectOptions = modelOptions ?? []
  const modelMissing = draft.model_name !== '' && !selectOptions.some(o => o.name === draft.model_name)

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      <Link
        href="/agents"
        className="mb-4 inline-flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-gold"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('backToList')}
      </Link>

      {/* 基础资料编辑 */}
      <div className="dao-card mb-6 p-5 md:p-6">
        <div className="mt-4 space-y-3">
          <div>
            <label htmlFor="agent-name" className="dao-label">
              {t('editor.nameLabel')}
            </label>
            <input
              id="agent-name"
              value={draft.name}
              onChange={e => flow.updateDraft({ name: e.target.value })}
              className="dao-input font-serif text-lg"
            />
            {flow.fieldErrors.name === 'required' && (
              <p className="mt-1 text-xs text-primary">{t('editor.nameRequired')}</p>
            )}
          </div>

          <div>
            <label htmlFor="agent-avatar" className="dao-label">
              {t('editor.avatarLabel')}
            </label>
            <input
              id="agent-avatar"
              value={draft.avatar}
              onChange={e => flow.updateDraft({ avatar: e.target.value })}
              placeholder={t('editor.avatarPlaceholder')}
              maxLength={avatarInputMaxLength(draft.avatar)}
              className="dao-input py-1.5 text-sm"
            />
            <p className="text-[10px] text-sage mt-1">
              {t('editor.avatarHint')}
            </p>
            {flow.fieldErrors.avatar && (
              <p className="mt-1 text-xs text-primary">
                {flow.fieldErrors.avatar === 'tooLong'
                  ? t('editor.avatarTooLong')
                  : t('editor.avatarInvalid')}
              </p>
            )}
          </div>

          <div>
            <label htmlFor="agent-personality" className="dao-label">
              {t('editor.personaLabel')}
            </label>
            <textarea
              id="agent-personality"
              value={draft.personality}
              onChange={e => flow.updateDraft({ personality: e.target.value })}
              className="dao-textarea"
              rows={4}
              placeholder={t('editor.personaPlaceholder')}
            />
          </div>

          <div>
            <label className="dao-label flex items-center gap-1.5">
              <Cpu className="h-3.5 w-3.5" />
              {t('editor.modelLabel')}
            </label>
            {modelOptions !== null && modelOptions.length === 0 ? (
              <p className="rounded-lg border border-border/70 bg-muted px-3 py-2.5 text-xs text-muted-foreground">
                {t('editor.modelEmpty')}
                <Link href="/settings" className="mx-1 whitespace-nowrap text-gold hover:text-gold/80">
                  {t('editor.modelLink')}
                </Link>
                {t('editor.modelEmptySuffix')}
              </p>
            ) : (
              <>
                <select
                  aria-label={t('editor.modelLabel')}
                  value={draft.model_name}
                  onChange={e => flow.updateDraft({ model_name: e.target.value })}
                  className="dao-input"
                >
                  {/* 当前失效模型作为警告项保留展示,不静默丢弃 */}
                  {modelMissing && (
                    <option value={draft.model_name}>
                      {draft.model_name}
                      {t('editor.modelInvalidSuffix')}
                    </option>
                  )}
                  {selectOptions.map(m => (
                    <option key={`${m.provider_name}/${m.name}`} value={m.name}>
                      {m.display_name || m.name}（{m.provider_display_name || m.provider_name}）
                      {m.is_default ? ` · ${t('editor.defaultBadge')}` : ''}
                    </option>
                  ))}
                </select>
                {draftModelInvalid && (
                  <p className="mt-1 flex items-start gap-1 break-words text-xs text-gold">
                    <TriangleAlert className="mt-0.5 h-3 w-3 shrink-0" />
                    {t('modelInvalidWarning')}
                  </p>
                )}
              </>
            )}
          </div>

          <div>
            <label className="dao-label flex items-center gap-1.5">
              <Sparkles className="h-3.5 w-3.5" />
              {tEditor('proactivity')}
              <span className="ml-auto font-mono text-xs text-sage">{draft.proactivity}</span>
            </label>
            <input
              type="range"
              min={0}
              max={100}
              step={1}
              value={draft.proactivity}
              aria-label={tEditor('proactivity')}
              onChange={e => flow.updateDraft({ proactivity: Number(e.target.value) })}
              className="mt-1.5 w-full accent-gold"
            />
            <p className="mt-1.5 text-[11px] text-sage">{tEditor('proactivityHint')}</p>
          </div>

          <div>
            <span className="dao-label">{t('editor.statusLabel')}</span>
            <div role="group" aria-label={t('editor.statusLabel')} className="flex gap-2">
              {(['active', 'inactive'] as AgentStatus[]).map(status => (
                <button
                  key={status}
                  type="button"
                  aria-pressed={draft.status === status}
                  onClick={() => flow.updateDraft({ status })}
                  className={`rounded-full border px-3 py-1 text-xs transition-colors ${
                    draft.status === status
                      ? 'border-gold/50 bg-gold/15 text-gold'
                      : 'border-border/70 bg-muted text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {tStatus(status)}
                </button>
              ))}
            </div>
          </div>

          <div>
            <button
              type="button"
              onClick={flow.discard}
              disabled={consumeOpen}
              className="dao-btn-ghost whitespace-nowrap text-sm disabled:opacity-50"
            >
              <X className="h-4 w-4" />
              {t('cancelCta')}
            </button>
          </div>
        </div>
      </div>

      {/* 能力编排编辑(受控组件:仅改草稿,保存时一次性提交;池=已吸收能力,不能凭空绑定金丹) */}
      <section className="dao-card p-5">
        <div className="mb-4 flex items-center gap-2">
          <FlaskConical className="h-4 w-4 shrink-0 text-gold" />
          <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">
            {t('pills.title')}
          </h2>
          <button
            type="button"
            onClick={handleConsumeOpen}
            disabled={effectsLoadFailed || effectsData === null || consumePending || consumeSyncFailed}
            className="ml-auto inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-lg border border-gold/40 bg-gold/10 px-2.5 py-1 text-xs font-medium text-gold transition-colors hover:bg-gold/20 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <FlaskConical className="h-3.5 w-3.5" />
            {tEditor('consume.open')}
          </button>
        </div>

        {/* 服用金丹提示(未保存移除意图/结果未知/同步失败/成功) */}
        {/* 移除意图已保存/撤销后自动收起(派生自草稿,无需同步 effect) */}
        {consumePendingRemoval && pendingRemovalCount > 0 && (
          <div
            role="alert"
            className="mb-4 flex items-start gap-2 rounded-lg border border-gold/40 bg-gold/10 px-3 py-2.5 text-xs text-gold"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{tEditor('consume.pendingRemoval')}</span>
          </div>
        )}
        {consumePending && (
          <div
            role="alert"
            className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-gold/40 bg-gold/10 px-3 py-2.5 text-xs text-gold"
          >
            <AlertCircle className="h-3.5 w-3.5 shrink-0" />
            <span>{tEditor('consume.uncertain')}</span>
            <button
              type="button"
              onClick={() => void handleVerifyPending()}
              className="ml-auto shrink-0 rounded-md border border-gold/40 px-2 py-0.5 font-medium hover:bg-gold/20"
            >
              {tEditor('consume.verify')}
            </button>
          </div>
        )}
        {consumeSyncFailed && (
          <div
            role="alert"
            className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2.5 text-xs text-primary"
          >
            <AlertCircle className="h-3.5 w-3.5 shrink-0" />
            <span>{tEditor('consume.syncFailed')}</span>
            <button
              type="button"
              onClick={handleRetrySync}
              className="ml-auto shrink-0 rounded-md border border-primary/40 px-2 py-0.5 font-medium hover:bg-primary/20"
            >
              {tEditor('consume.retrySync')}
            </button>
          </div>
        )}
        {consumeSuccess && (
          <div
            role="status"
            className="mb-4 flex items-center gap-2 rounded-lg border border-sage/40 bg-sage/10 px-3 py-2.5 text-xs text-sage"
          >
            <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
            <span>{tEditor('consume.success')}</span>
          </div>
        )}
        {effectsLoadFailed ? (
          <div>
            <ActionFeedback
              status="error"
              message={t('effectsLoadFailed')}
              onRetry={() => void loadEffects()}
              retryLabel={tCommon('retry')}
            />
          </div>
        ) : effectsData === null ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin text-gold" />
            {t('loading')}
          </p>
        ) : (
          <AgentPillComposer
            value={draft.effects}
            onChange={effects => flow.updateDraft({ effects })}
            effects={effectsData.effects}
            fieldErrors={flow.fieldErrors}
          />
        )}
      </section>

      {/* 底部保存栏 */}
      <div className="sticky bottom-4 mt-6 flex flex-wrap items-center justify-end gap-3">
        {flow.saveStatus === 'submitting' && (
          <ActionFeedback status="submitting" message={t('saving')} />
        )}
        {flow.saveStatus === 'error' && (
          <ActionFeedback
            status="error"
            message={t('saveFailed')}
            onRetry={handleSave}
            retryLabel={tCommon('retry')}
          />
        )}
        {flow.saveStatus === 'conflict' && (
          <ActionFeedback
            status="error"
            message={t('effectsConflict')}
            onRetry={handleSave}
            retryLabel={tCommon('retry')}
          />
        )}
        <button
          type="button"
          onClick={handleSave}
          disabled={flow.saveStatus === 'submitting' || consumeOpen || consumePending || consumeSyncFailed}
          className="dao-btn-primary whitespace-nowrap shadow-lg disabled:opacity-50"
        >
          {flow.saveStatus === 'submitting' ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Save className="h-4 w-4" />
          )}
          {t('saveCta')}
        </button>
      </div>

      {/* 服用金丹:库存选择弹窗(固定当前道人;提交成功→同步能力→关闭) */}
      {consumeOpen && agent && (
        <ConsumeInventoryPillModal
          agentId={agent.id}
          agentName={agent.name}
          activeEffects={effectsData?.effects ?? []}
          onClose={handleConsumeClose}
          onCommitted={handleConsumed}
        />
      )}
    </div>
  )
}
