/**
 * 道人详情页面 - Agent 配置
 * 道人信息编辑（基础性格 + 模型选择）
 * 已服用金丹列表：权重(0-10)编辑、拖拽排序（服用顺序）、绑定/解绑
 * 语言模式合成状态展示（涌现规则 + 丹性相冲警告）
 */
import { useState, useEffect, useRef, type DragEvent } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
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
import { useChat } from '@/contexts/ChatContext'
import Layout from '@/components/Layout'
import { AVAILABLE_MODELS } from '@/services/models'
import * as agentService from '@/services/agentService'
import type { AgentPill, TensionSeverity } from '@/services/types'

/** 生成头像渐变颜色（根据名称确定性生成） */
function getAvatarColor(name: string): string {
  const colors = [
    'from-cinnabar-500 to-cinnabar-700',
    'from-jade-500 to-jade-700',
    'from-gold-500 to-gold-700',
    'from-blue-500 to-blue-700',
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length]
}

/** 丹性相冲严重程度徽标样式与文案 */
const SEVERITY_BADGE: Record<TensionSeverity, { label: string; className: string }> = {
  low: {
    label: '轻微',
    className: 'bg-jade-500/20 text-jade-300 border-jade-500/30',
  },
  medium: {
    label: '中等',
    className: 'bg-gold-500/20 text-gold-300 border-gold-500/30',
  },
  high: {
    label: '剧烈',
    className: 'bg-cinnabar-500/20 text-cinnabar-300 border-cinnabar-500/40',
  },
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
  onSave: (pillId: number, weight: number, sortOrder: number) => Promise<boolean>
  onUnbind: (pillId: number) => Promise<boolean>
  onDragStartRow: (index: number, e: DragEvent, rowEl: HTMLElement | null) => void
  onDragOverRow: (index: number, e: DragEvent) => void
  onDropRow: (index: number) => void
  onDragEndRow: () => void
}) {
  const [weight, setWeight] = useState(agentPill.weight)
  const [sortOrder, setSortOrder] = useState(agentPill.sort_order)
  const [saving, setSaving] = useState(false)
  const rowRef = useRef<HTMLDivElement>(null)

  // 外部数据刷新后同步本地状态
  useEffect(() => {
    setWeight(agentPill.weight)
    setSortOrder(agentPill.sort_order)
  }, [agentPill.weight, agentPill.sort_order])

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
        ${isDragOver ? 'border-gold-400/60 ring-1 ring-gold-400/40' : ''}
        ${reordering ? 'opacity-60 pointer-events-none' : ''}
      `}
    >
      <div className="flex items-center gap-3">
        {/* 拖拽手柄（调整服用顺序） */}
        <span
          draggable
          onDragStart={e => onDragStartRow(index, e, rowRef.current)}
          onDragEnd={onDragEndRow}
          className="cursor-grab active:cursor-grabbing p-1 -ml-1 rounded text-ink-500 hover:text-gold-300 hover:bg-gold-400/10 transition-colors flex-shrink-0"
          title="拖拽调整服用顺序"
        >
          <GripVertical className="w-4 h-4" />
        </span>
        <div className="w-10 h-10 rounded-xl bg-gold-500/15 flex items-center justify-center flex-shrink-0">
          <FlaskConical className="w-5 h-5 text-gold-400" />
        </div>
        <div className="flex-1 min-w-0">
          <Link
            to={`/pills/${agentPill.pill_id}`}
            className="text-sm font-medium text-rice-paper-100 hover:text-gold-300 transition-colors"
          >
            {pill?.name || `金丹 #${agentPill.pill_id}`}
          </Link>
          {pill?.description && (
            <p className="text-[10px] text-ink-400 truncate">{pill.description}</p>
          )}
        </div>
        <button
          onClick={() => onUnbind(agentPill.pill_id)}
          className="p-1.5 rounded hover:bg-cinnabar-500/20 text-ink-400 hover:text-cinnabar-400 transition-colors"
          title="解除绑定"
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>

      {/* 权重 / 顺序编辑 */}
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex-1 min-w-[120px]">
          <label className="dao-label flex items-center gap-1">
            <Scale className="w-3 h-3" />
            权重（0-10）
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
            服用顺序
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
          className="dao-btn-gold text-xs px-3 py-2 disabled:opacity-40"
        >
          {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
          保存
        </button>
      </div>
    </div>
  )
}

export default function AgentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const agentId = Number(id)

  const { state: agentState, fetchAgent, bindPill, unbindPill, updateAgentPill, editAgent } = useAgent()
  const { state: pillState, fetchPills } = usePill()
  const { createSession } = useChat()

  const [showBindPill, setShowBindPill] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editPersonality, setEditPersonality] = useState('')
  const [editModel, setEditModel] = useState('')
  const [isCreatingSession, setIsCreatingSession] = useState(false)
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
  }, [agentId, fetchAgent, fetchPills])

  // 初始化编辑表单
  useEffect(() => {
    if (agent) {
      setEditName(agent.name)
      setEditPersonality(agent.personality || '')
      setEditModel(agent.model_name)
    }
  }, [agent])

  /** 保存编辑 */
  const handleSaveEdit = async () => {
    if (!editName.trim()) return
    const updated = await editAgent(agentId, {
      name: editName.trim(),
      personality: editPersonality.trim(),
      model_name: editModel,
    })
    if (updated) setIsEditing(false)
  }

  /** 服用金丹（默认权重 1.0，顺序追加到末尾） */
  const handleBindPill = async (pillId: number) => {
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
    setIsCreatingSession(true)
    const session = await createSession(agentId, `与${agent?.name}的论道`)
    setIsCreatingSession(false)
    if (session) navigate(`/chat/${session.id}`)
  }

  /** 可服用金丹列表（未绑定的） */
  const availablePills = pillState.pills.filter(
    p => !agentPills.some(ap => ap.pill_id === p.id)
  )

  if (!agent && agentState.loading) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold-400 animate-spin mb-3" />
          <p className="text-sm text-ink-400">加载中...</p>
        </div>
      </Layout>
    )
  }

  if (!agent) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="w-12 h-12 text-cinnabar-400 mb-3" />
          <p className="text-sm text-ink-400">道人不存在</p>
          <Link to="/agents" className="dao-btn-primary mt-4">
            <ArrowLeft className="w-4 h-4" />
            返回道人府
          </Link>
        </div>
      </Layout>
    )
  }

  return (
    <Layout>
      {/* 返回按钮 */}
      <Link
        to="/agents"
        className="inline-flex items-center gap-1.5 text-sm text-ink-400 hover:text-gold-300 transition-colors mb-4"
      >
        <ArrowLeft className="w-4 h-4" />
        返回道人府
      </Link>

      {/* 道人信息头部 */}
      <div className="dao-card p-5 md:p-6 mb-6">
        <div className="flex flex-col md:flex-row gap-5">
          {/* 头像 */}
          <div className={`
            flex-shrink-0 w-20 h-20 md:w-24 md:h-24 rounded-2xl
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
                  <label className="dao-label">道号</label>
                  <input
                    value={editName}
                    onChange={e => setEditName(e.target.value)}
                    className="dao-input text-lg font-serif"
                  />
                </div>
                <div>
                  <label className="dao-label">基础性格 / 系统提示词</label>
                  <textarea
                    value={editPersonality}
                    onChange={e => setEditPersonality(e.target.value)}
                    className="dao-textarea"
                    rows={4}
                    placeholder="描述这位道人的性格特点和语言风格..."
                  />
                </div>
                <div>
                  <label className="dao-label flex items-center gap-1.5">
                    <Cpu className="w-3.5 h-3.5" />
                    选择模型
                  </label>
                  <select
                    value={editModel}
                    onChange={e => setEditModel(e.target.value)}
                    className="dao-input"
                  >
                    {AVAILABLE_MODELS.map(m => (
                      <option key={m.id} value={m.id}>{m.name} - {m.description}</option>
                    ))}
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={handleSaveEdit} className="dao-btn-primary text-sm">
                    <Check className="w-4 h-4" /> 保存
                  </button>
                  <button onClick={() => setIsEditing(false)} className="dao-btn-ghost text-sm">
                    <X className="w-4 h-4" /> 取消
                  </button>
                </div>
              </div>
            ) : (
              <>
                <div className="flex items-center gap-3 mb-2">
                  <h1 className="text-xl md:text-2xl font-serif font-bold text-rice-paper-100">
                    {agent.name}
                  </h1>
                  <span className={`
                    text-[10px] px-2 py-0.5 rounded-full border
                    ${agent.status === 'active'
                      ? 'bg-jade-500/20 text-jade-300 border-jade-500/30'
                      : 'bg-ink-500/30 text-ink-400 border-ink-400/20'
                    }
                  `}>
                    {agent.status === 'active' ? '活跃' : '沉睡'}
                  </span>
                </div>

                {agent.personality && (
                  <p className="text-sm text-ink-400 mb-3 leading-relaxed">
                    {agent.personality}
                  </p>
                )}

                <div className="flex flex-wrap items-center gap-3 text-xs text-ink-400">
                  <span className="flex items-center gap-1">
                    <Cpu className="w-3.5 h-3.5" />
                    {agent.model_name}
                  </span>
                  <span className="flex items-center gap-1">
                    <Pill className="w-3.5 h-3.5" />
                    {agentPills.length} 金丹
                  </span>
                </div>

                <div className="flex items-center gap-2 mt-4">
                  <button onClick={handleStartChat} className="dao-btn-primary text-sm">
                    {isCreatingSession ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <MessageSquare className="w-4 h-4" />
                    )}
                    开始论道
                  </button>
                  <button onClick={() => setIsEditing(true)} className="dao-btn-ghost text-sm">
                    <Edit3 className="w-4 h-4" />
                    编辑
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* 语言模式合成状态 */}
      {languagePattern && (
        <div className="dao-card p-4 mb-6">
          <div className="flex items-center gap-2 mb-3">
            <Wand2 className="w-4 h-4 text-jade-400" />
            <h2 className="text-sm font-serif font-bold text-gold-300">语言模式（丹性）</h2>
            <span className={`
              text-[10px] px-2 py-0.5 rounded-full border
              ${languagePattern.is_valid
                ? 'bg-jade-500/20 text-jade-300 border-jade-500/30'
                : 'bg-gold-500/20 text-gold-300 border-gold-500/30'
              }
            `}>
              {languagePattern.is_valid ? '已合成' : '待重新合成'}
            </span>
          </div>

          {!languagePattern.is_valid ? (
            /* 缓存已失效：以下内容为旧丹方合成结果，将在下次论道时重新合成 */
            <div className="flex items-start gap-2 text-xs text-gold-300/90 bg-gold-500/10 border border-gold-500/20 rounded-lg px-3 py-2">
              <RefreshCw className="w-3.5 h-3.5 mt-0.5 flex-shrink-0" />
              <span>
                丹方已变更，当前语言模式为旧方所炼，仅供参考；下次论道时将按新丹方重新合成（含涌现规则与丹性相冲检测）。
              </span>
            </div>
          ) : (
            <>
              {languagePattern.emergence_rules && languagePattern.emergence_rules.length > 0 && (
                <div className="mb-3">
                  <p className="text-xs text-ink-400 mb-1.5">涌现规则</p>
                  <ul className="space-y-1">
                    {languagePattern.emergence_rules.map((rule, i) => (
                      <li key={i} className="flex items-start gap-1.5 text-xs text-rice-paper-200">
                        <Sparkles className="w-3 h-3 text-gold-400 mt-0.5 flex-shrink-0" />
                        <span>{rule}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {languagePattern.inner_tensions && languagePattern.inner_tensions.length > 0 && (
                <div>
                  <p className="text-xs text-ink-400 mb-1.5 flex items-center gap-1">
                    <TriangleAlert className="w-3 h-3 text-cinnabar-400" />
                    丹性相冲（{languagePattern.inner_tensions.length}）
                  </p>
                  <ul className="space-y-2">
                    {languagePattern.inner_tensions.map((tension, i) => {
                      const badge = SEVERITY_BADGE[tension.severity] ?? SEVERITY_BADGE.medium
                      return (
                        <li
                          key={i}
                          className="flex items-start gap-2 text-xs bg-cinnabar-500/5 border border-cinnabar-500/20 rounded-lg px-3 py-2"
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2 mb-0.5">
                              <span className="font-medium text-rice-paper-200">{tension.dimension}</span>
                              <span className={`text-[10px] px-1.5 py-px rounded-full border ${badge.className}`}>
                                {badge.label}
                              </span>
                            </div>
                            <p className="text-ink-400 leading-relaxed">{tension.description}</p>
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
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-serif font-bold text-gold-300 flex items-center gap-2">
            <FlaskConical className="w-5 h-5" />
            已服用金丹
          </h2>
          <button
            onClick={() => setShowBindPill(!showBindPill)}
            className="dao-btn-gold text-sm"
          >
            <Plus className="w-4 h-4" />
            服用金丹
          </button>
        </div>

        {/* 服用金丹选择面板 */}
        {showBindPill && (
          <div className="dao-card p-4 mb-4 animate-fade-in">
            <h3 className="text-sm font-medium text-gold-300 mb-3">从金丹阁选择</h3>
            {availablePills.length === 0 ? (
              <p className="text-sm text-ink-400 text-center py-4">
                暂无可服用的金丹（所有金丹均已绑定）
              </p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {availablePills.map(pill => (
                  <button
                    key={pill.id}
                    onClick={() => handleBindPill(pill.id)}
                    className="flex items-center gap-3 p-3 rounded-lg bg-ink-800/50 border border-bronze-600/20 hover:border-gold-400/40 hover:bg-gold-400/5 transition-all text-left"
                  >
                    <div className="w-8 h-8 rounded-lg bg-gold-500/15 flex items-center justify-center flex-shrink-0">
                      <FlaskConical className="w-4 h-4 text-gold-400" />
                    </div>
                    <div className="min-w-0">
                      <p className="text-sm text-rice-paper-100 truncate">{pill.name}</p>
                      {pill.tags && pill.tags.length > 0 && (
                        <p className="text-[10px] text-ink-400 truncate">{pill.tags.join(' · ')}</p>
                      )}
                    </div>
                    <Plus className="w-4 h-4 text-gold-400 ml-auto flex-shrink-0" />
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {/* 已绑定金丹列表 */}
        {agentPills.length === 0 ? (
          <div className="dao-card flex flex-col items-center py-8 text-center">
            <Pill className="w-10 h-10 text-ink-600 mb-2" />
            <p className="text-sm text-ink-400">尚未服用任何金丹</p>
            <p className="text-xs text-ink-500 mt-1">服用金丹后，道人将习得对应的语言模式与人格特质</p>
          </div>
        ) : (
          <>
            <p className="text-[11px] text-ink-500 mb-2 flex items-center gap-1.5">
              {reordering ? (
                <>
                  <Loader2 className="w-3 h-3 animate-spin" />
                  正在保存新的服用顺序...
                </>
              ) : (
                <>
                  <GripVertical className="w-3 h-3" />
                  拖拽左侧手柄可调整服用顺序，权重与顺序变化后丹性将于下次论道时重新合成
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
        <div className="fixed bottom-20 md:bottom-6 right-4 dao-card p-3 flex items-center gap-2 text-sm text-cinnabar-400 animate-fade-in">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{agentState.error}</span>
        </div>
      )}
    </Layout>
  )
}
