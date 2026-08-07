/**
 * 金丹详情页面 - 炼丹房编辑器
 * 金丹元信息 + nuwa-skill 结构化内容编辑：
 * 身份卡 / 表达 DNA / 心智模型 / 决策启发式 / 价值观 / 反模式 / 诚实边界 / 示例对话
 * 支持「赠予道人」快捷绑定
 */
import { useState, useEffect } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft,
  CircleDot,
  Loader2,
  FlaskConical,
  Clock,
  AlertCircle,
  Plus,
  Trash2,
  Save,
  Gift,
  IdCard,
  Dna,
  BrainCircuit,
  Split,
  Heart,
  ShieldAlert,
  Handshake,
  MessagesSquare,
  Tag,
  User,
} from 'lucide-react'
import { usePill } from '@/contexts/PillContext'
import BindAgentModal from '@/components/BindAgentModal'
import Layout from '@/components/Layout'
import { formatDateTime } from '@/utils/format'
import type {
  SkillSchema,
  ExpressionDNA,
  SentenceLength,
  MentalModel,
  DecisionHeuristic,
  ExampleDialogue,
} from '@/services/types'

// ========== 表单内部类型（数组字段以逗号文本编辑） ==========

interface MentalModelForm {
  name: string
  one_liner: string
  application: string
  limitationsText: string
}

interface FormState {
  name: string
  description: string
  author: string
  version: string
  tagsText: string
  identityCard: string
  dna: {
    sentence_length: SentenceLength
    formality: number
    vocabularyText: string
    tabooWordsText: string
    rhythm: string
    humor_type: string
    certainty_style: string
    citation_habit: string
  }
  mentalModels: MentalModelForm[]
  heuristics: DecisionHeuristic[]
  values: string[]
  antiPatterns: string[]
  honestLimits: string[]
  dialogues: ExampleDialogue[]
}

/** 逗号/顿号分隔文本 -> 字符串数组 */
function parseList(text: string): string[] {
  return text
    .split(/[,，、\n]/)
    .map(s => s.trim())
    .filter(Boolean)
}

/** 字符串数组 -> 逗号分隔文本 */
function joinList(list?: string[]): string {
  return (list || []).join('，')
}

/** 从后端 skill_schema 构建表单状态 */
function buildForm(schema: SkillSchema | undefined, meta: { name: string; description?: string; author?: string; version: string; tags?: string[] }): FormState {
  const dna: ExpressionDNA = schema?.expression_dna || {}
  return {
    name: meta.name,
    description: meta.description || '',
    author: meta.author || '',
    version: meta.version || '1.0.0',
    tagsText: joinList(meta.tags),
    identityCard: schema?.identity_card || '',
    dna: {
      sentence_length: dna.sentence_length || 'mixed',
      formality: typeof dna.formality === 'number' ? dna.formality : 0.5,
      vocabularyText: joinList(dna.vocabulary),
      tabooWordsText: joinList(dna.taboo_words),
      rhythm: dna.rhythm || '',
      humor_type: dna.humor_type || '',
      certainty_style: dna.certainty_style || '',
      citation_habit: dna.citation_habit || '',
    },
    mentalModels: (schema?.mental_models || []).map((m: MentalModel) => ({
      name: m.name || '',
      one_liner: m.one_liner || '',
      application: m.application || '',
      limitationsText: joinList(m.limitations),
    })),
    heuristics: (schema?.decision_heuristics || []).map(h => ({
      condition: h.condition || '',
      action: h.action || '',
      case: h.case || '',
    })),
    values: [...(schema?.values || [])],
    antiPatterns: [...(schema?.anti_patterns || [])],
    honestLimits: [...(schema?.honest_limits || [])],
    dialogues: (schema?.example_dialogues || []).map(d => ({
      user: d.user || '',
      assistant: d.assistant || '',
    })),
  }
}

/** 表单状态 -> skill_schema */
function buildSchema(form: FormState): SkillSchema {
  const mentalModels: MentalModel[] = form.mentalModels
    .filter(m => m.name.trim())
    .map(m => ({
      name: m.name.trim(),
      one_liner: m.one_liner.trim(),
      application: m.application.trim(),
      limitations: parseList(m.limitationsText),
    }))
  const heuristics: DecisionHeuristic[] = form.heuristics
    .filter(h => h.condition.trim() || h.action.trim())
    .map(h => ({ condition: h.condition.trim(), action: h.action.trim(), case: h.case?.trim() }))
  const dialogues: ExampleDialogue[] = form.dialogues
    .filter(d => d.user.trim() || d.assistant.trim())
    .map(d => ({ user: d.user.trim(), assistant: d.assistant.trim() }))

  return {
    identity_card: form.identityCard.trim(),
    expression_dna: {
      sentence_length: form.dna.sentence_length,
      formality: form.dna.formality,
      vocabulary: parseList(form.dna.vocabularyText),
      taboo_words: parseList(form.dna.tabooWordsText),
      rhythm: form.dna.rhythm.trim(),
      humor_type: form.dna.humor_type.trim(),
      certainty_style: form.dna.certainty_style.trim(),
      citation_habit: form.dna.citation_habit.trim(),
    },
    mental_models: mentalModels,
    decision_heuristics: heuristics,
    values: form.values.map(v => v.trim()).filter(Boolean),
    anti_patterns: form.antiPatterns.map(v => v.trim()).filter(Boolean),
    honest_limits: form.honestLimits.map(v => v.trim()).filter(Boolean),
    example_dialogues: dialogues,
  }
}

// ========== 通用编辑子组件 ==========

/** 章节容器 */
function Section({ icon: Icon, title, children }: { icon: React.ElementType; title: string; children: React.ReactNode }) {
  return (
    <section className="dao-card p-5">
      <div className="flex items-center gap-2 mb-4">
        <Icon className="w-4 h-4 text-gold-400" />
        <h2 className="text-base font-serif font-bold text-gold-300">{title}</h2>
      </div>
      {children}
    </section>
  )
}

/** 字符串列表编辑器 */
function StringListEditor({
  items,
  onChange,
  placeholder,
  addLabel,
}: {
  items: string[]
  onChange: (items: string[]) => void
  placeholder: string
  addLabel: string
}) {
  return (
    <div className="space-y-2">
      {items.map((item, index) => (
        <div key={index} className="flex items-center gap-2">
          <input
            type="text"
            value={item}
            onChange={e => {
              const next = [...items]
              next[index] = e.target.value
              onChange(next)
            }}
            placeholder={placeholder}
            className="dao-input flex-1 py-1.5 text-sm"
          />
          <button
            type="button"
            onClick={() => onChange(items.filter((_, i) => i !== index))}
            className="p-1.5 rounded hover:bg-cinnabar-500/20 text-ink-400 hover:text-cinnabar-400 transition-colors flex-shrink-0"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={() => onChange([...items, ''])}
        className="flex items-center gap-1 text-xs text-gold-400/70 hover:text-gold-300 transition-colors"
      >
        <Plus className="w-3.5 h-3.5" />
        {addLabel}
      </button>
    </div>
  )
}

// ========== 页面组件 ==========

export default function PillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const pillId = Number(id)

  const { state, fetchPill, editPill, removePill } = usePill()
  const [form, setForm] = useState<FormState | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [showBind, setShowBind] = useState(false)

  const pill = state.currentPill

  // 加载数据
  useEffect(() => {
    if (pillId) fetchPill(pillId)
  }, [pillId, fetchPill])

  // 金丹加载完成后初始化表单
  useEffect(() => {
    if (pill && pill.id === pillId) {
      setForm(buildForm(pill.skill_schema, pill))
    }
  }, [pill, pillId])

  /** 更新表单字段 */
  const patch = (partial: Partial<FormState>) => {
    setForm(prev => (prev ? { ...prev, ...partial } : prev))
  }

  /** 更新表达 DNA 字段 */
  const patchDna = (partial: Partial<FormState['dna']>) => {
    setForm(prev => (prev ? { ...prev, dna: { ...prev.dna, ...partial } } : prev))
  }

  /** 保存金丹 */
  const handleSave = async () => {
    if (!form || !form.name.trim()) return
    setSaving(true)
    const updated = await editPill(pillId, {
      name: form.name.trim(),
      description: form.description.trim(),
      author: form.author.trim(),
      version: form.version.trim() || '1.0.0',
      tags: parseList(form.tagsText),
      skill_schema: buildSchema(form),
    })
    setSaving(false)
    if (updated) {
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    }
  }

  /** 删除金丹 */
  const handleDelete = async () => {
    if (!window.confirm('确定要销毁这颗金丹吗？此操作不可恢复。')) return
    const ok = await removePill(pillId)
    if (ok) navigate('/pills')
  }

  if (!pill && state.loading) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold-400 animate-spin mb-3" />
          <p className="text-sm text-ink-400">加载中...</p>
        </div>
      </Layout>
    )
  }

  if (!pill || !form) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="w-12 h-12 text-cinnabar-400 mb-3" />
          <p className="text-sm text-ink-400">金丹不存在</p>
          <Link to="/pills" className="dao-btn-primary mt-4">
            <ArrowLeft className="w-4 h-4" />
            返回金丹阁
          </Link>
        </div>
      </Layout>
    )
  }

  return (
    <Layout>
      {/* 返回按钮 */}
      <Link
        to="/pills"
        className="inline-flex items-center gap-1.5 text-sm text-ink-400 hover:text-gold-300 transition-colors mb-4"
      >
        <ArrowLeft className="w-4 h-4" />
        返回金丹阁
      </Link>

      {/* 金丹信息头部 */}
      <div className="dao-card p-5 md:p-6 mb-6">
        <div className="flex flex-col md:flex-row md:items-start gap-4">
          {/* 图标 */}
          <div className="flex-shrink-0 w-16 h-16 rounded-2xl flex items-center justify-center bg-gold-500/15 text-gold-400 glow-gold">
            <FlaskConical className="w-8 h-8" />
          </div>

          {/* 元信息编辑 */}
          <div className="flex-1 min-w-0 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <input
                value={form.name}
                onChange={e => patch({ name: e.target.value })}
                className="dao-input text-lg font-serif font-bold flex-1 min-w-[200px]"
                placeholder="金丹名称"
              />
              {pill.is_builtin && (
                <span className="text-[10px] px-2 py-0.5 rounded-full border bg-jade-500/20 text-jade-300 border-jade-500/30">
                  内置
                </span>
              )}
            </div>

            <textarea
              value={form.description}
              onChange={e => patch({ description: e.target.value })}
              className="dao-textarea"
              rows={2}
              placeholder="金丹简介（含触发语、反触发语）..."
            />

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="dao-label flex items-center gap-1">
                  <User className="w-3 h-3" />
                  作者
                </label>
                <input
                  value={form.author}
                  onChange={e => patch({ author: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="作者"
                />
              </div>
              <div>
                <label className="dao-label">版本</label>
                <input
                  value={form.version}
                  onChange={e => patch({ version: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="1.0.0"
                />
              </div>
              <div>
                <label className="dao-label flex items-center gap-1">
                  <Tag className="w-3 h-3" />
                  标签（逗号分隔）
                </label>
                <input
                  value={form.tagsText}
                  onChange={e => patch({ tagsText: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="文言文，古雅"
                />
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-4 text-xs text-ink-400">
              <span className="flex items-center gap-1">
                <Clock className="w-3.5 h-3.5" />
                {formatDateTime(pill.created_at)}
              </span>
            </div>
          </div>

          {/* 操作按钮 */}
          <div className="flex md:flex-col items-center gap-2">
            <button onClick={() => setShowBind(true)} className="dao-btn-gold text-sm">
              <Gift className="w-4 h-4" />
              赠予道人
            </button>
            <button
              onClick={handleDelete}
              className="dao-btn-ghost text-sm text-cinnabar-400 hover:text-cinnabar-300"
            >
              <Trash2 className="w-4 h-4" />
              销毁
            </button>
          </div>
        </div>
      </div>

      <div className="space-y-5">
        {/* 身份卡 */}
        <Section icon={IdCard} title="身份卡（第一人称）">
          <textarea
            value={form.identityCard}
            onChange={e => patch({ identityCard: e.target.value })}
            className="dao-textarea"
            rows={3}
            placeholder="如：我是一名从 2077 年穿越而来的流浪黑客..."
          />
        </Section>

        {/* 表达 DNA */}
        <Section icon={Dna} title="表达 DNA">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="dao-label">句式长度</label>
              <select
                value={form.dna.sentence_length}
                onChange={e => patchDna({ sentence_length: e.target.value as SentenceLength })}
                className="dao-input"
              >
                <option value="short">短句（short）</option>
                <option value="medium">中等（medium）</option>
                <option value="long">长句（long）</option>
                <option value="mixed">混合（mixed）</option>
              </select>
            </div>
            <div>
              <label className="dao-label">正式程度（{form.dna.formality.toFixed(2)}）</label>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={form.dna.formality}
                onChange={e => patchDna({ formality: Number(e.target.value) })}
                className="w-full accent-gold-400 mt-2.5"
              />
              <div className="flex justify-between text-[10px] text-ink-500">
                <span>随意</span>
                <span>正式</span>
              </div>
            </div>
            <div>
              <label className="dao-label">高频词（逗号分隔）</label>
              <input
                value={form.dna.vocabularyText}
                onChange={e => patchDna({ vocabularyText: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="芯片，霓虹，义体"
              />
            </div>
            <div>
              <label className="dao-label">禁用词（逗号分隔）</label>
              <input
                value={form.dna.tabooWordsText}
                onChange={e => patchDna({ tabooWordsText: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="绝不说出口的词汇"
              />
            </div>
            <div>
              <label className="dao-label">节奏</label>
              <input
                value={form.dna.rhythm}
                onChange={e => patchDna({ rhythm: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="如：短促有力，善用停顿"
              />
            </div>
            <div>
              <label className="dao-label">幽默类型</label>
              <input
                value={form.dna.humor_type}
                onChange={e => patchDna({ humor_type: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="如：冷幽默、自嘲"
              />
            </div>
            <div>
              <label className="dao-label">确定性表达风格</label>
              <input
                value={form.dna.certainty_style}
                onChange={e => patchDna({ certainty_style: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="如：直言不讳 / 留有余地"
              />
            </div>
            <div>
              <label className="dao-label">引用习惯</label>
              <input
                value={form.dna.citation_habit}
                onChange={e => patchDna({ citation_habit: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder="如：喜引《道德经》"
              />
            </div>
          </div>
        </Section>

        {/* 心智模型 */}
        <Section icon={BrainCircuit} title="心智模型">
          <div className="space-y-4">
            {form.mentalModels.map((model, index) => (
              <div key={index} className="p-3 rounded-lg bg-ink-800/50 border border-bronze-600/20 space-y-2">
                <div className="flex items-center gap-2">
                  <input
                    value={model.name}
                    onChange={e => {
                      const next = [...form.mentalModels]
                      next[index] = { ...model, name: e.target.value }
                      patch({ mentalModels: next })
                    }}
                    className="dao-input flex-1 py-1.5 text-sm"
                    placeholder="模型名称，如：第一性原理"
                  />
                  <button
                    type="button"
                    onClick={() => patch({ mentalModels: form.mentalModels.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-cinnabar-500/20 text-ink-400 hover:text-cinnabar-400 transition-colors flex-shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
                <input
                  value={model.one_liner}
                  onChange={e => {
                    const next = [...form.mentalModels]
                    next[index] = { ...model, one_liner: e.target.value }
                    patch({ mentalModels: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder="一句话概括"
                />
                <textarea
                  value={model.application}
                  onChange={e => {
                    const next = [...form.mentalModels]
                    next[index] = { ...model, application: e.target.value }
                    patch({ mentalModels: next })
                  }}
                  className="dao-textarea text-sm"
                  rows={2}
                  placeholder="如何应用此模型分析问题"
                />
                <input
                  value={model.limitationsText}
                  onChange={e => {
                    const next = [...form.mentalModels]
                    next[index] = { ...model, limitationsText: e.target.value }
                    patch({ mentalModels: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder="局限性（逗号分隔）"
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ mentalModels: [...form.mentalModels, { name: '', one_liner: '', application: '', limitationsText: '' }] })}
              className="flex items-center gap-1 text-xs text-gold-400/70 hover:text-gold-300 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              添加心智模型
            </button>
          </div>
        </Section>

        {/* 决策启发式 */}
        <Section icon={Split} title="决策启发式">
          <div className="space-y-4">
            {form.heuristics.map((heuristic, index) => (
              <div key={index} className="p-3 rounded-lg bg-ink-800/50 border border-bronze-600/20 space-y-2">
                <div className="flex items-center gap-2">
                  <input
                    value={heuristic.condition}
                    onChange={e => {
                      const next = [...form.heuristics]
                      next[index] = { ...heuristic, condition: e.target.value }
                      patch({ heuristics: next })
                    }}
                    className="dao-input flex-1 py-1.5 text-sm"
                    placeholder="当...（条件）"
                  />
                  <button
                    type="button"
                    onClick={() => patch({ heuristics: form.heuristics.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-cinnabar-500/20 text-ink-400 hover:text-cinnabar-400 transition-colors flex-shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
                <input
                  value={heuristic.action}
                  onChange={e => {
                    const next = [...form.heuristics]
                    next[index] = { ...heuristic, action: e.target.value }
                    patch({ heuristics: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder="就...（行动）"
                />
                <input
                  value={heuristic.case || ''}
                  onChange={e => {
                    const next = [...form.heuristics]
                    next[index] = { ...heuristic, case: e.target.value }
                    patch({ heuristics: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder="案例（可选）"
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ heuristics: [...form.heuristics, { condition: '', action: '', case: '' }] })}
              className="flex items-center gap-1 text-xs text-gold-400/70 hover:text-gold-300 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              添加决策启发式
            </button>
          </div>
        </Section>

        {/* 价值观 */}
        <Section icon={Heart} title="价值观">
          <StringListEditor
            items={form.values}
            onChange={values => patch({ values })}
            placeholder="如：诚实优于圆滑"
            addLabel="添加价值观"
          />
        </Section>

        {/* 反模式 */}
        <Section icon={ShieldAlert} title="反模式（绝不做的事）">
          <StringListEditor
            items={form.antiPatterns}
            onChange={antiPatterns => patch({ antiPatterns })}
            placeholder="如：不堆砌空洞的形容词"
            addLabel="添加反模式"
          />
        </Section>

        {/* 诚实边界 */}
        <Section icon={Handshake} title="诚实边界">
          <StringListEditor
            items={form.honestLimits}
            onChange={honestLimits => patch({ honestLimits })}
            placeholder="如：对不熟悉的领域坦言不知"
            addLabel="添加诚实边界"
          />
        </Section>

        {/* 示例对话 */}
        <Section icon={MessagesSquare} title="示例对话">
          <div className="space-y-4">
            {form.dialogues.map((dialogue, index) => (
              <div key={index} className="p-3 rounded-lg bg-ink-800/50 border border-bronze-600/20 space-y-2">
                <div className="flex items-start gap-2">
                  <div className="flex-1 space-y-2">
                    <textarea
                      value={dialogue.user}
                      onChange={e => {
                        const next = [...form.dialogues]
                        next[index] = { ...dialogue, user: e.target.value }
                        patch({ dialogues: next })
                      }}
                      className="dao-textarea text-sm"
                      rows={2}
                      placeholder="道友问：..."
                    />
                    <textarea
                      value={dialogue.assistant}
                      onChange={e => {
                        const next = [...form.dialogues]
                        next[index] = { ...dialogue, assistant: e.target.value }
                        patch({ dialogues: next })
                      }}
                      className="dao-textarea text-sm"
                      rows={3}
                      placeholder="道人答：..."
                    />
                  </div>
                  <button
                    type="button"
                    onClick={() => patch({ dialogues: form.dialogues.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-cinnabar-500/20 text-ink-400 hover:text-cinnabar-400 transition-colors flex-shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ dialogues: [...form.dialogues, { user: '', assistant: '' }] })}
              className="flex items-center gap-1 text-xs text-gold-400/70 hover:text-gold-300 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              添加示例对话
            </button>
          </div>
        </Section>
      </div>

      {/* 底部保存栏 */}
      <div className="sticky bottom-16 md:bottom-4 mt-6 flex items-center justify-end gap-3">
        {saved && (
          <span className="flex items-center gap-1 text-sm text-jade-400 animate-fade-in">
            <CircleDot className="w-4 h-4" />
            已存入金丹阁
          </span>
        )}
        <button
          onClick={handleSave}
          disabled={!form.name.trim() || saving}
          className="dao-btn-primary shadow-lg disabled:opacity-50"
        >
          {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
          保存金丹
        </button>
      </div>

      {/* 从金丹到道人 - 快捷绑定弹窗 */}
      {showBind && (
        <BindAgentModal pill={pill} onClose={() => setShowBind(false)} />
      )}
    </Layout>
  )
}
