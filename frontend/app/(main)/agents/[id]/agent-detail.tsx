'use client'

/**
 * 道人详情页面 - 只读/编辑双态
 * 只读: 完整资料、模型失效警告、语言模式缓存状态、服丹编排(顺序+剂量)
 * 编辑: useAgentEditorFlow 草稿;编辑过程零 API,保存=基础资料→完整编排→GET 回读(flow 保证);
 *       失败保留草稿且 ActionFeedback 可重试;「恢复服务端版本」回到基线但保持编辑态
 * 删除: 有会话历史(409 delete_has_history)时引导停用;无历史才二次确认硬删除
 */
import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  ArrowLeft,
  Ban,
  Cpu,
  FlaskConical,
  Loader2,
  MessageSquare,
  Pencil,
  Pill as PillIcon,
  RefreshCw,
  RotateCcw,
  Save,
  Scale,
  Sparkles,
  Trash2,
  TriangleAlert,
  Wand2,
  X,
} from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { usePill } from '@/contexts/PillContext'
import { useAgentEditorFlow } from '@/hooks/use-agent-editor-flow'
import { useChatLaunchFlow } from '@/hooks/use-chat-launch-flow'
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes'
import { AgentPillComposer } from '@/components/agent-pill-composer'
import { NuwaDistillPanel } from '@/components/nuwa-distill-panel'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { ApiError } from '@/services/api'
import * as agentService from '@/services/agentService'
import * as modelService from '@/services/modelService'
import type { ModelOption } from '@/services/modelService'
import type { AgentStatus, TensionSeverity } from '@/services/types'
import { formatDateTime } from '@/utils/format'

/** 生成头像渐变颜色（根据名称确定性生成） */
function getAvatarColor(name: string): string {
  const colors = [
    'from-primary to-primary/70',
    'from-sage to-sage/70',
    'from-gold to-gold/70',
    'from-foreground/60 to-foreground/80',
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length]
}

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

export default function AgentDetailPage() {
  const t = useTranslations('agent')
  const tStatus = useTranslations('agentCard.status')
  const tEditor = useTranslations('agentDetail.editor')
  const tSev = useTranslations('agent.severity')
  const tLaunch = useTranslations('chatView.launch')
  const tCommon = useTranslations('common')
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  // Next output:export prerenders [id] as "_"; ignore that static shell param
  // so we never GET /agents/_ (400 "道人ID格式不正确"). Real UUIDs aren't "_".
  const agentId = id === '_' ? undefined : id

  const { state: agentState, fetchAgent, dispatch } = useAgent()
  const { state: pillState, fetchPills } = usePill()
  const launchFlow = useChatLaunchFlow()

  const agent = agentState.currentAgent
  const flow = useAgentEditorFlow(agent)
  useUnsavedChanges(flow.dirty, t('unsavedConfirm'))

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
      fetchPills()
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
  }, [agentId, fetchAgent, fetchPills])

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

  /** 保存(写后重读由 flow 保证) */
  const handleSave = () => {
    void flow.save()
  }

  // ========== 加载 / 错误 / 空 三态 ==========
  if (!agent && agentState.loading) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    )
  }

  if (!agent && agentState.error) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="mb-4 max-w-xl break-words text-center text-sm text-muted-foreground">
            {agentState.error}
          </p>
          <button type="button" onClick={() => agentId && fetchAgent(agentId)} className="dao-btn-ghost">
            {tCommon('retry')}
          </button>
        </div>
      </div>
    )
  }

  if (!agent) {
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

  const languagePattern = agent.language_pattern

  // ========== 只读态 ==========
  if (flow.mode === 'readonly') {
    const agentPills = [...(agent.agent_pills ?? [])].sort((a, b) => a.sort_order - b.sort_order)
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
            {agent.avatar ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={agent.avatar}
                alt={agent.name}
                className="h-20 w-20 shrink-0 rounded-2xl object-cover shadow-lg md:h-24 md:w-24"
              />
            ) : (
              <div
                className={`flex h-20 w-20 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br font-serif text-3xl font-bold text-white shadow-lg md:h-24 md:w-24 md:text-4xl ${getAvatarColor(agent.name)}`}
              >
                {agent.name.charAt(0)}
              </div>
            )}

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
                  {t('pillsCount', { count: agentPills.length })}
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

        {/* 服丹编排(只读:按服用顺序展示金丹名与剂量) */}
        <section className="dao-card p-5">
          <div className="mb-4 flex items-center gap-2">
            <FlaskConical className="h-4 w-4 shrink-0 text-gold" />
            <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">
              {t('pills.title')}
            </h2>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t('pillsCount', { count: agentPills.length })}
            </span>
          </div>
          {agentPills.length === 0 ? (
            <div>
              <p className="text-sm text-muted-foreground">{t('pills.empty')}</p>
              <p className="mt-1 text-xs text-sage">{t('pills.emptyHint')}</p>
            </div>
          ) : (
            <ol className="space-y-2">
              {agentPills.map((agentPill, index) => (
                <li
                  key={agentPill.id}
                  className="flex items-center gap-3 rounded-lg border border-border/70 bg-muted px-3 py-2"
                >
                  <span className="shrink-0 text-xs text-muted-foreground">{index + 1}.</span>
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gold/15 text-gold">
                    <FlaskConical className="h-4 w-4" />
                  </span>
                  <Link
                    href={`/pills/${agentPill.pill_id}`}
                    className="min-w-0 flex-1 truncate text-sm font-medium text-foreground transition-colors hover:text-gold"
                  >
                    {agentPill.pill?.name ?? t('composer.unknownPill')}
                  </Link>
                  <span className="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-muted-foreground">
                    <Scale className="h-3 w-3" />
                    {t('pills.weight')} {agentPill.weight}
                  </span>
                </li>
              ))}
            </ol>
          )}
        </section>
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
        <NuwaDistillPanel onApply={flow.applyNuwaDraft} />

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
              className="dao-input py-1.5 text-sm"
            />
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
              className="dao-btn-ghost whitespace-nowrap text-sm"
            >
              <X className="h-4 w-4" />
              {t('cancelCta')}
            </button>
          </div>
        </div>
      </div>

      {/* 服丹编排编辑(受控组件:仅改草稿,保存时一次性提交) */}
      <section className="dao-card p-5">
        <div className="mb-4 flex items-center gap-2">
          <FlaskConical className="h-4 w-4 shrink-0 text-gold" />
          <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">
            {t('pills.title')}
          </h2>
        </div>
        <AgentPillComposer
          value={draft.pills}
          onChange={pills => flow.updateDraft({ pills })}
          pills={pillState.pills}
          fieldErrors={flow.fieldErrors}
        />
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
        <button
          type="button"
          onClick={flow.restoreServerVersion}
          disabled={flow.saveStatus === 'submitting'}
          className="dao-btn-ghost whitespace-nowrap text-sm"
        >
          <RotateCcw className="h-4 w-4" />
          {t('restoreCta')}
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={flow.saveStatus === 'submitting'}
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
    </div>
  )
}
