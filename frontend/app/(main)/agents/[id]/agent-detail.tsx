'use client'

/**
 * 道人详情页面 - Agent 配置
 * 道人信息编辑（基础性格 + 模型选择）
 * 已服用金丹列表：权重(0-10)编辑、拖拽排序（服用顺序）、绑定/解绑
 * 语言模式合成状态展示（涌现规则 + 丹性相冲警告）
 */
import { useState, useEffect, useRef, type DragEvent } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  ArrowLeft,
  Cpu,
  Pill,
  Sparkles,
  Plus,
  Trash2,
  Loader2,
  AlertCircle,
  MessageSquare,
  Edit3,
  X,
  Check,
  FlaskConical,
  Scale,
  ListOrdered,
  Wand2,
  TriangleAlert,
  GripVertical,
  RefreshCw,
} from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { usePill } from '@/contexts/PillContext'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { useChatLaunchFlow } from '@/hooks/use-chat-launch-flow'
import * as modelService from '@/services/modelService'
import type { ModelOption } from '@/services/modelService'
import * as agentService from '@/services/agentService'
import type { AgentPill, TensionSeverity } from '@/services/types'

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

/** 已服用金丹条目（权重可编辑，可拖拽调整服用顺序） */
function AgentPillRow({
  agentPill,
  index,
  isDragOver,
  reordering,
  onSave,
  onUnbind,
  onDragStartRow,
  onDragOverRow,
  onDropRow,
  onDragEndRow,
}: {
  agentPill: AgentPill
  index: number
  isDragOver: boolean
  reordering: boolean
  onSave: (pillId: string, weight: number, sortOrder: number) => Promise<boolean>
  onUnbind: (pillId: string) => Promise<boolean>
  onDragStartRow: (index: number, e: DragEvent, rowEl: HTMLElement | null) => void
  onDragOverRow: (index: number, e: DragEvent) => void
  onDropRow: (index: number) => void
  onDragEndRow: () => void
}) {
  const t = useTranslations('agent')
  const [weight, setWeight] = useState(agentPill.weight)
  const [sortOrder, setSortOrder] = useState(agentPill.sort_order)
  const [saving, setSaving] = useState(false)
  const rowRef = useRef<HTMLDivElement>(null)

  // 外部数据刷新后同步本地状态（渲染期调整，避免级联渲染）
  const [synced, setSynced] = useState({ w: agentPill.weight, s: agentPill.sort_order })
  if (synced.w !== agentPill.weight || synced.s !== agentPill.sort_order) {
    setSynced({ w: agentPill.weight, s: agentPill.sort_order })
    setWeight(agentPill.weight)
    setSortOrder(agentPill.sort_order)
  }

  const dirty = weight !== agentPill.weight || sortOrder !== agentPill.sort_order
  const pill = agentPill.pill

  const handleSave = async () => {
    setSaving(true)
    await onSave(agentPill.pill_id, weight, sortOrder)
    setSaving(false)
  }

  return (
    <div
      ref={rowRef}
      onDragOver={e => onDragOverRow(index, e)}
      onDrop={e => {
        e.preventDefault()
        onDropRow(index)
      }}
      className={`
        dao-card p-4 flex flex-col gap-3 transition-all
        ${isDragOver ? 'border-gold/60 ring-1 ring-gold/40' : ''}
        ${reordering ? 'opacity-60 pointer-events-none' : ''}
      `}
    >
      <div className="flex items-center gap-3 min-w-0">
        {/* 拖拽手柄（调整服用顺序） */}
        <span
          draggable
          onDragStart={e => onDragStartRow(index, e, rowRef.current)}
          onDragEnd={onDragEndRow}
          className="cursor-grab active:cursor-grabbing p-1 -ml-1 rounded text-muted-foreground/60 hover:text-gold hover:bg-gold/10 transition-colors shrink-0"
          title={t('pills.reorderTitle')}
        >
          <GripVertical className="w-4 h-4" />
        </span>
        <div className="w-10 h-10 rounded-xl bg-gold/15 flex items-center justify-center shrink-0">
          <FlaskConical className="w-5 h-5 text-gold" />
        </div>
        <div className="flex-1 min-w-0">
          <Link
            href={`/pills/${agentPill.pill_id}`}
            className="text-sm font-medium text-foreground hover:text-gold transition-colors truncate block"
          >
            {pill?.name || `金丹 #${agentPill.pill_id}`}
          </Link>
          {pill?.description && (
            <p className="text-[10px] text-muted-foreground truncate">{pill.description}</p>
          )}
        </div>
        <button
          onClick={() => onUnbind(agentPill.pill_id)}
          className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors shrink-0"
          title={t('pills.unbindTitle')}
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>

      {/* 权重 / 顺序编辑 */}
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex-1 min-w-[120px]">
          <label className="dao-label flex items-center gap-1">
            <Scale className="w-3 h-3" />
            {t('pills.weight')}（0-10）
          </label>
          <input
            type="number"
            min={0}
            max={10}
            step={0.5}
            value={weight}
            onChange={e => setWeight(Math.min(10, Math.max(0, Number(e.target.value))))}
            className="dao-input py-1.5 text-sm"
          />
        </div>
        <div className="flex-1 min-w-[120px]">
          <label className="dao-label flex items-center gap-1">
            <ListOrdered className="w-3 h-3" />
            {t('pills.sortOrder')}
          </label>
          <input
            type="number"
            min={0}
            step={1}
            value={sortOrder}
            onChange={e => setSortOrder(Math.max(0, Math.floor(Number(e.target.value))))}
            className="dao-input py-1.5 text-sm"
          />
        </div>
        <button
          onClick={handleSave}
          disabled={!dirty || saving}
          className="dao-btn-gold text-xs px-3 py-2 disabled:opacity-40 whitespace-nowrap"
        >
          {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
          {t('saveCta')}
        </button>
      </div>
    </div>
  )
}

export default function AgentDetailPage() {
  const t = useTranslations('agent')
  const tStatus = useTranslations('agentCard.status')
  const tEditor = useTranslations('agentDetail.editor')
  const tSev = useTranslations('agent.severity')
  const tLaunch = useTranslations('chatView.launch')
  const { id } = useParams<{ id: string }>()
  const agentId = id

  const { state: agentState, fetchAgent, bindPill, unbindPill, updateAgentPill, editAgent } = useAgent()
  const { state: pillState, fetchPills } = usePill()
  const launchFlow = useChatLaunchFlow()

  const [showBindPill, setShowBindPill] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editPersonality, setEditPersonality] = useState('')
  const [editModel, setEditModel] = useState('')
  const [editProactivity, setEditProactivity] = useState(50)
  const [modelOptions, setModelOptions] = useState<ModelOption[]>([])
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)
  const [reordering, setReordering] = useState(false)

  const agent = agentState.currentAgent
  const agentPills = [...(agent?.agent_pills || [])].sort((a, b) => a.sort_order - b.sort_order)
  const languagePattern = agent?.language_pattern

  // 加载数据
  useEffect(() => {
    if (agentId) {
      fetchAgent(agentId)
      fetchPills()
    }
    // 加载已启用模型选项（默认模型排在首位）
    modelService.options()
      .then(opts => {
        setModelOptions([...opts].sort((a, b) => Number(b.is_default) - Number(a.is_default)))
      })
      .catch(() => {
        // 模型选项加载失败时保留下拉为空态提示
      })
  }, [agentId, fetchAgent, fetchPills])

  // 初始化编辑表单（渲染期调整，避免级联渲染）
  const [syncedAgent, setSyncedAgent] = useState(agent)
  if (agent && agent !== syncedAgent) {
    setSyncedAgent(agent)
    setEditName(agent.name)
    setEditPersonality(agent.personality || '')
    setEditModel(agent.model_name)
    setEditProactivity(agent.proactivity ?? 50)
  }

  /** 保存编辑 */
  const handleSaveEdit = async () => {
    if (!editName.trim()) return
    const updated = await editAgent(agentId, {
      name: editName.trim(),
      personality: editPersonality.trim(),
      model_name: editModel,
      proactivity: editProactivity,
    })
    if (updated) setIsEditing(false)
  }

  /** 服用金丹（默认权重 1.0，顺序追加到末尾） */
  const handleBindPill = async (pillId: string) => {
    const maxOrder = agentPills.reduce((max, ap) => Math.max(max, ap.sort_order), -1)
    await bindPill(agentId, pillId, 1, maxOrder + 1)
  }

  /** 拖拽开始：记录源索引并设置拖拽图像为整行 */
  const handleDragStartRow = (index: number, e: DragEvent, rowEl: HTMLElement | null) => {
    setDragIndex(index)
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(index))
    if (rowEl) e.dataTransfer.setDragImage(rowEl, 20, 20)
  }

  /** 拖拽悬停：允许放置并高亮目标行 */
  const handleDragOverRow = (index: number, e: DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragIndex !== null && index !== dragOverIndex) setDragOverIndex(index)
  }

  /** 拖拽结束（未放置）：清理状态 */
  const handleDragEndRow = () => {
    setDragIndex(null)
    setDragOverIndex(null)
  }

  /** 放置：重排列表并持久化受影响的服用顺序（PUT /agents/:id/pills/:pill_id） */
  const handleDropRow = async (targetIndex: number) => {
    const sourceIndex = dragIndex
    handleDragEndRow()
    if (sourceIndex === null || sourceIndex === targetIndex) return

    const reordered = [...agentPills]
    const [moved] = reordered.splice(sourceIndex, 1)
    reordered.splice(targetIndex, 0, moved)

    // 新位置即新 sort_order，仅持久化顺序发生变化的行
    const changed = reordered
      .map((ap, newOrder) => ({ ap, newOrder }))
      .filter(({ ap, newOrder }) => ap.sort_order !== newOrder)
    if (changed.length === 0) return

    setReordering(true)
    try {
      // 串行提交，避免后端写入竞争
      for (const { ap, newOrder } of changed) {
        await agentService.updateAgentPill(agentId, ap.pill_id, ap.weight, newOrder)
      }
    } catch {
      // 失败时下方刷新会回显服务端真实顺序
    }
    await fetchAgent(agentId)
    setReordering(false)
  }

  /** 开始对话 */
  const handleStartChat = async () => {
    await launchFlow.launchSingle(agentId)
  }

  /** 可服用金丹列表（未绑定的） */
  const availablePills = pillState.pills.filter(
    p => !agentPills.some(ap => ap.pill_id === p.id)
  )

  // 严重程度徽标样式（label 通过 tSev 渲染）
  const SEVERITY_CLASS: Record<TensionSeverity, string> = {
    low: 'bg-sage/15 text-sage border-sage/30',
    medium: 'bg-gold/15 text-gold border-gold/30',
    high: 'bg-primary/10 text-primary border-primary/30',
  }

  if (!agent && agentState.loading) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold animate-spin mb-3" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    )
  }

  if (!agent) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="w-12 h-12 text-primary mb-3" />
          <p className="text-sm text-muted-foreground">{t('notFound')}</p>
          <Link href="/agents" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="w-4 h-4" />
            {t('backToList')}
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      {/* 返回按钮 */}
      <Link
        href="/agents"
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-gold transition-colors mb-4 whitespace-nowrap"
      >
        <ArrowLeft className="w-4 h-4" />
        {t('backToList')}
      </Link>

      {/* 道人信息头部 */}
      <div className="dao-card p-5 md:p-6 mb-6">
        <div className="flex flex-col md:flex-row gap-5">
          {/* 头像 */}
          <div className={`
            shrink-0 w-20 h-20 md:w-24 md:h-24 rounded-2xl
            bg-gradient-to-br ${getAvatarColor(agent.name)}
            flex items-center justify-center text-white font-serif font-bold text-3xl md:text-4xl
            shadow-lg
          `}>
            {agent.name.charAt(0)}
          </div>

          {/* 信息 */}
          <div className="flex-1 min-w-0">
            {isEditing ? (
              <div className="space-y-3">
                <div>
                  <label className="dao-label">{t('editor.nameLabel')}</label>
                  <input
                    value={editName}
                    onChange={e => setEditName(e.target.value)}
                    className="dao-input text-lg font-serif"
                  />
                </div>
                <div>
                  <label className="dao-label">{t('editor.personaLabel')}</label>
                  <textarea
                    value={editPersonality}
                    onChange={e => setEditPersonality(e.target.value)}
                    className="dao-textarea"
                    rows={4}
                    placeholder={t('editor.personaPlaceholder')}
                  />
                </div>
                <div>
                  <label className="dao-label flex items-center gap-1.5">
                    <Cpu className="w-3.5 h-3.5" />
                    {t('editor.modelLabel')}
                  </label>
                  {modelOptions.length === 0 ? (
                    <p className="text-xs text-muted-foreground bg-muted border border-border/70 rounded-lg px-3 py-2.5">
                      {t('editor.modelEmpty')}
                      <Link href="/settings" className="text-gold hover:text-gold/80 mx-1 whitespace-nowrap">
                        {t('editor.modelLink')}
                      </Link>
                      {t('editor.modelEmptySuffix')}
                    </p>
                  ) : (
                    <select
                      value={editModel}
                      onChange={e => setEditModel(e.target.value)}
                      className="dao-input"
                    >
                      {/* 当前使用的模型可能已停用/删除，保留为可选项以便回显 */}
                      {editModel && !modelOptions.some(o => o.name === editModel) && (
                        <option value={editModel}>{editModel}{t('editor.modelCurrent')}</option>
                      )}
                      {modelOptions.map(m => (
                        <option key={`${m.provider_name}/${m.name}`} value={m.name}>
                          {m.display_name || m.name}（{m.provider_display_name || m.provider_name}）{m.is_default ? ` · ${t('editor.defaultBadge')}` : ''}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
                <div>
                  <label className="dao-label flex items-center gap-1.5">
                    <Sparkles className="w-3.5 h-3.5" />
                    {tEditor('proactivity')}
                    <span className="ml-auto font-mono text-xs text-sage">
                      {editProactivity}
                    </span>
                  </label>
                  <input
                    type="range"
                    min={0}
                    max={100}
                    step={1}
                    value={editProactivity}
                    onChange={e => setEditProactivity(Number(e.target.value))}
                    aria-label={tEditor('proactivity')}
                    className="w-full accent-gold mt-1.5"
                  />
                  <p className="mt-1.5 text-[11px] text-sage">
                    {tEditor('proactivityHint')}
                  </p>
                </div>
                <div className="flex items-center gap-2 flex-wrap">
                  <button onClick={handleSaveEdit} className="dao-btn-primary text-sm whitespace-nowrap">
                    <Check className="w-4 h-4" /> {t('saveCta')}
                  </button>
                  <button onClick={() => setIsEditing(false)} className="dao-btn-ghost text-sm whitespace-nowrap">
                    <X className="w-4 h-4" /> {t('cancelCta')}
                  </button>
                </div>
              </div>
            ) : (
              <>
                <div className="flex items-center gap-3 mb-2 flex-wrap">
                  <h1 className="text-xl md:text-2xl font-serif font-bold text-foreground truncate min-w-0">
                    {agent.name}
                  </h1>
                  <span className={`
                    text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap shrink-0
                    ${agent.status === 'active'
                      ? 'bg-sage/15 text-sage border-sage/30'
                      : 'bg-muted text-muted-foreground border-border'
                    }
                  `}>
                    {agent.status === 'active' ? tStatus('active') : tStatus('inactive')}
                  </span>
                </div>

                {agent.personality && (
                  <p className="text-sm text-muted-foreground mb-3 leading-relaxed">
                    {agent.personality}
                  </p>
                )}

                <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1 min-w-0">
                    <Cpu className="w-3.5 h-3.5 shrink-0" />
                    <span className="truncate">{agent.model_name}</span>
                  </span>
                  <span className="flex items-center gap-1">
                    <Pill className="w-3.5 h-3.5" />
                    {t('pillsCount', { count: agentPills.length })}
                  </span>
                  <span
                    className="flex items-center gap-1"
                    title={tEditor('proactivityHint')}
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    {tEditor('proactivity')}: {agent.proactivity}
                  </span>
                </div>

                <div className="mt-4 flex flex-wrap items-center gap-2">
                  <button
                    onClick={handleStartChat}
                    disabled={launchFlow.state.status === 'submitting'}
                    className="dao-btn-primary whitespace-nowrap text-sm disabled:cursor-wait disabled:opacity-60"
                  >
                    {launchFlow.state.status === 'submitting' ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <MessageSquare className="w-4 h-4" />
                    )}
                    {t('startChatCta')}
                  </button>
                  <button onClick={() => setIsEditing(true)} className="dao-btn-ghost text-sm whitespace-nowrap">
                    <Edit3 className="w-4 h-4" />
                    {t('editCta')}
                  </button>
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
                          onRetry={() => { void launchFlow.retry() }}
                          retryLabel={tLaunch('retry')}
                        />
                        {launchFlow.state.errorCode === 'service.chat.model_unavailable' && (
                          <Link
                            href="/settings"
                            className="mt-2 inline-flex max-w-full break-words text-left text-xs font-medium whitespace-normal text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                          >
                            {tLaunch('modelSettings')}
                          </Link>
                        )}
                      </>
                    )}
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </div>

      {/* 语言模式合成状态 */}
      {languagePattern && (
        <div className="dao-card p-4 mb-6">
          <div className="flex items-center gap-2 mb-3 flex-wrap min-w-0">
            <Wand2 className="w-4 h-4 text-sage shrink-0" />
            <h2 className="text-sm font-serif font-bold text-gold truncate">
              {t('languagePattern.title')}
            </h2>
            <span className={`
              text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap shrink-0
              ${languagePattern.is_valid
                ? 'bg-sage/15 text-sage border-sage/30'
                : 'bg-gold/15 text-gold border-gold/30'
              }
            `}>
              {languagePattern.is_valid ? t('languagePattern.synthesized') : t('languagePattern.stale')}
            </span>
          </div>

          {!languagePattern.is_valid ? (
            /* 缓存已失效：以下内容为旧丹方合成结果，将在下次论道时重新合成 */
            <div className="flex items-start gap-2 text-xs text-gold bg-gold/10 border border-gold/20 rounded-lg px-3 py-2">
              <RefreshCw className="w-3.5 h-3.5 mt-0.5 shrink-0" />
              <span>{t('languagePattern.staleDesc')}</span>
            </div>
          ) : (
            <>
              {languagePattern.emergence_rules && languagePattern.emergence_rules.length > 0 && (
                <div className="mb-3">
                  <p className="text-xs text-muted-foreground mb-1.5">{t('languagePattern.emergenceRules')}</p>
                  <ul className="space-y-1">
                    {languagePattern.emergence_rules.map((rule, i) => (
                      <li key={i} className="flex items-start gap-1.5 text-xs text-foreground/90">
                        <Sparkles className="w-3 h-3 text-gold mt-0.5 shrink-0" />
                        <span>{rule}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {languagePattern.inner_tensions && languagePattern.inner_tensions.length > 0 && (
                <div>
                  <p className="text-xs text-muted-foreground mb-1.5 flex items-center gap-1">
                    <TriangleAlert className="w-3 h-3 text-primary" />
                    {t('languagePattern.tensions', { count: languagePattern.inner_tensions.length })}
                  </p>
                  <ul className="space-y-2">
                    {languagePattern.inner_tensions.map((tension, i) => {
                      const sevClass = SEVERITY_CLASS[tension.severity] ?? SEVERITY_CLASS.medium
                      const sevLabel = tSev(tension.severity)
                      return (
                        <li
                          key={i}
                          className="flex items-start gap-2 text-xs bg-primary/5 border border-primary/20 rounded-lg px-3 py-2"
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2 mb-0.5 flex-wrap">
                              <span className="font-medium text-foreground/90">{tension.dimension}</span>
                              <span className={`text-[10px] px-1.5 py-px rounded-full border whitespace-nowrap shrink-0 ${sevClass}`}>
                                {sevLabel}
                              </span>
                            </div>
                            <p className="text-muted-foreground leading-relaxed">{tension.description}</p>
                          </div>
                        </li>
                      )
                    })}
                  </ul>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* 已服用金丹 */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4 gap-2 flex-wrap">
          <h2 className="text-lg font-serif font-bold text-gold flex items-center gap-2 min-w-0">
            <FlaskConical className="w-5 h-5 shrink-0" />
            <span className="truncate">{t('pills.title')}</span>
          </h2>
          <button
            onClick={() => setShowBindPill(!showBindPill)}
            className="dao-btn-gold text-sm whitespace-nowrap"
          >
            <Plus className="w-4 h-4" />
            {t('pills.addCta')}
          </button>
        </div>

        {/* 服用金丹选择面板 */}
        {showBindPill && (
          <div className="dao-card p-4 mb-4 animate-in fade-in duration-300">
            <h3 className="text-sm font-medium text-gold mb-3">{t('pills.selectFromVault')}</h3>
            {availablePills.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                {t('pills.allBound')}
              </p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {availablePills.map(pill => (
                  <button
                    key={pill.id}
                    onClick={() => handleBindPill(pill.id)}
                    className="flex items-center gap-3 p-3 rounded-lg bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left min-w-0"
                  >
                    <div className="w-8 h-8 rounded-lg bg-gold/15 flex items-center justify-center shrink-0">
                      <FlaskConical className="w-4 h-4 text-gold" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm text-foreground truncate">{pill.name}</p>
                      {pill.tags && pill.tags.length > 0 && (
                        <p className="text-[10px] text-muted-foreground truncate">{pill.tags.join(' · ')}</p>
                      )}
                    </div>
                    <Plus className="w-4 h-4 text-gold ml-auto shrink-0" />
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {/* 已绑定金丹列表 */}
        {agentPills.length === 0 ? (
          <div className="dao-card flex flex-col items-center py-8 text-center">
            <Pill className="w-10 h-10 text-muted-foreground/50 mb-2" />
            <p className="text-sm text-muted-foreground">{t('pills.empty')}</p>
            <p className="text-xs text-sage mt-1">{t('pills.emptyHint')}</p>
          </div>
        ) : (
          <>
            <p className="text-[11px] text-sage mb-2 flex items-center gap-1.5">
              {reordering ? (
                <>
                  <Loader2 className="w-3 h-3 animate-spin shrink-0" />
                  <span>{t('pills.reordering')}</span>
                </>
              ) : (
                <>
                  <GripVertical className="w-3 h-3 shrink-0" />
                  <span>{t('pills.dragHint')}</span>
                </>
              )}
            </p>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {agentPills.map((agentPill, index) => (
                <AgentPillRow
                  key={agentPill.id}
                  agentPill={agentPill}
                  index={index}
                  isDragOver={dragOverIndex === index && dragIndex !== null && dragIndex !== index}
                  reordering={reordering}
                  onSave={(pillId, weight, sortOrder) => updateAgentPill(agentId, pillId, weight, sortOrder)}
                  onUnbind={(pillId) => unbindPill(agentId, pillId)}
                  onDragStartRow={handleDragStartRow}
                  onDragOverRow={handleDragOverRow}
                  onDropRow={handleDropRow}
                  onDragEndRow={handleDragEndRow}
                />
              ))}
            </div>
          </>
        )}
      </div>

      {/* 错误提示 */}
      {agentState.error && (
        <div className="fixed bottom-20 md:bottom-6 right-4 dao-card p-3 flex items-center gap-2 text-sm text-primary animate-in fade-in duration-300">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{agentState.error}</span>
        </div>
      )}
    </div>
  )
}
