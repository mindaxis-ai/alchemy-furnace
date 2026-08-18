'use client'

/**
 * 金丹详情页面 - 炼丹房编辑器
 * 金丹元信息 + nuwa-skill 结构化内容编辑：
 * 身份卡 / 表达 DNA / 心智模型 / 决策启发式 / 价值观 / 反模式 / 诚实边界 / 示例对话
 * 支持「赠予道人」快捷绑定
 */
import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
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
  Pencil,
  X,
} from 'lucide-react'
import { usePill } from '@/contexts/PillContext'
import { BindAgentModal } from '@/components/bind-agent-modal'
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
      <div className="flex items-center gap-2 mb-4 min-w-0">
        <Icon className="w-4 h-4 text-gold shrink-0" />
        <h2 className="text-base font-serif font-bold text-gold truncate">{title}</h2>
      </div>
      {children}
    </section>
  )
}

function ReadOnlyList({ items, empty }: { items: string[]; empty: string }) {
  if (items.length === 0) return <p className="text-sm text-muted-foreground">{empty}</p>
  return (
    <ul className="space-y-2">
      {items.map((item, index) => (
        <li key={`${item}-${index}`} className="rounded-lg border border-border/70 bg-muted px-3 py-2 text-sm leading-relaxed">
          {item}
        </li>
      ))}
    </ul>
  )
}

/** 字符串列表编辑器 */
function StringListEditor({
  items,
  onChange,
  placeholder,
  addLabel,
  deleteAria,
}: {
  items: string[]
  onChange: (items: string[]) => void
  placeholder: string
  addLabel: string
  deleteAria: string
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
            className="dao-input flex-1 py-1.5 text-sm min-w-0"
          />
          <button
            type="button"
            aria-label={deleteAria}
            onClick={() => onChange(items.filter((_, i) => i !== index))}
            className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors shrink-0"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={() => onChange([...items, ''])}
        className="flex items-center gap-1 text-xs text-gold/70 hover:text-gold transition-colors whitespace-nowrap"
      >
        <Plus className="w-3.5 h-3.5" />
        {addLabel}
      </button>
    </div>
  )
}

// ========== 页面组件 ==========

export default function PillDetailPage() {
  const t = useTranslations('pill.editor')
  const tPill = useTranslations('pill')
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const pillId = id

  const { state, fetchPill, editPill, removePill } = usePill()
  const [form, setForm] = useState<FormState | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [showBind, setShowBind] = useState(false)
  const [isEditing, setIsEditing] = useState(false)

  const pill = state.currentPill

  // 加载数据
  useEffect(() => {
    if (pillId) fetchPill(pillId)
  }, [pillId, fetchPill])

  // 金丹加载完成后初始化表单（渲染期调整，避免级联渲染）
  const [syncedPill, setSyncedPill] = useState(pill)
  if (pill && pill.id === pillId && pill !== syncedPill) {
    setSyncedPill(pill)
    setForm(buildForm(pill.skill_schema, pill))
  }

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
      setIsEditing(false)
      setTimeout(() => setSaved(false), 2000)
    }
  }

  /** 删除金丹 */
  const handleDelete = async () => {
    if (!window.confirm(t('deleteConfirm'))) return
    const ok = await removePill(pillId)
    if (ok) router.push('/pills')
  }

  if (!pill && state.loading) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold animate-spin mb-3" />
          <p className="text-sm text-muted-foreground">{tPill('loading')}</p>
        </div>
      </div>
    )
  }

  if (!pill || !form) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="w-12 h-12 text-primary mb-3" />
          <p className="text-sm text-muted-foreground">{tPill('notFound')}</p>
          <Link href="/pills" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="w-4 h-4" />
            {tPill('backToList')}
          </Link>
        </div>
      </div>
    )
  }

  if (!isEditing) {
    const vocabulary = parseList(form.dna.vocabularyText)
    const tags = parseList(form.tagsText)
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <Link href="/pills" className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-gold">
          <ArrowLeft className="h-4 w-4" />
          {tPill('backToList')}
        </Link>

        <div className="dao-card mb-6 p-5 md:p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start">
            <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
              <FlaskConical className="h-8 w-8" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="font-serif text-2xl font-bold text-foreground">{pill.name}</h1>
                {pill.is_builtin && <span className="rounded-full border border-sage/30 bg-sage/15 px-2 py-0.5 text-[10px] text-sage">{t('builtInBadge')}</span>}
              </div>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">{pill.description || tPill('content.empty')}</p>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
                {pill.author && <span>{t('authorLabel')}: {pill.author}</span>}
                <span>{t('versionLabel')}: {pill.version}</span>
                <span className="flex items-center gap-1"><Clock className="h-3.5 w-3.5" />{formatDateTime(pill.updated_at)}</span>
              </div>
              {tags.length > 0 && <div className="mt-3 flex flex-wrap gap-1.5">{tags.map(tag => <span key={tag} className="rounded-full border border-gold/20 px-2 py-0.5 text-[11px] text-gold">{tag}</span>)}</div>}
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2 md:flex-col md:items-stretch">
              <button onClick={() => setIsEditing(true)} className="dao-btn-primary text-sm"><Pencil className="h-4 w-4" />{tPill('editCta')}</button>
              <button onClick={() => setShowBind(true)} className="dao-btn-gold text-sm"><Gift className="h-4 w-4" />{tPill('bindCta')}</button>
            </div>
          </div>
        </div>

        <div className="space-y-5">
          <Section icon={IdCard} title={t('section.identity')}>
            <p className="whitespace-pre-wrap text-sm leading-relaxed">{form.identityCard || tPill('schema.empty')}</p>
          </Section>
          <Section icon={Dna} title={t('section.dna')}>
            <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
              <p><span className="text-muted-foreground">{t('dna.sentenceLength')} · </span>{t(`dna.lengthOption.${form.dna.sentence_length}`)}</p>
              <p><span className="text-muted-foreground">{t('dna.formality', { value: form.dna.formality.toFixed(2) })}</span></p>
              {form.dna.rhythm && <p><span className="text-muted-foreground">{t('dna.rhythm')} · </span>{form.dna.rhythm}</p>}
              {form.dna.humor_type && <p><span className="text-muted-foreground">{t('dna.humor')} · </span>{form.dna.humor_type}</p>}
              {form.dna.certainty_style && <p><span className="text-muted-foreground">{t('dna.certainty')} · </span>{form.dna.certainty_style}</p>}
              {form.dna.citation_habit && <p><span className="text-muted-foreground">{t('dna.citation')} · </span>{form.dna.citation_habit}</p>}
            </div>
            {vocabulary.length > 0 && <div className="mt-3 flex flex-wrap gap-1.5">{vocabulary.map(word => <span key={word} className="rounded-full bg-gold/10 px-2 py-1 text-xs text-gold">{word}</span>)}</div>}
          </Section>
          <Section icon={BrainCircuit} title={t('section.mentalModels')}>
            {form.mentalModels.length === 0 ? <p className="text-sm text-muted-foreground">{tPill('schema.empty')}</p> : <div className="space-y-3">{form.mentalModels.map((model, index) => <article key={`${model.name}-${index}`} className="rounded-lg border border-border/70 bg-muted p-3"><h3 className="font-medium">{model.name}</h3>{model.one_liner && <p className="mt-1 text-sm text-gold">{model.one_liner}</p>}<p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">{model.application}</p></article>)}</div>}
          </Section>
          <Section icon={Split} title={t('section.heuristics')}>
            <ReadOnlyList items={form.heuristics.map(item => [item.condition, item.action, item.case].filter(Boolean).join(' → '))} empty={tPill('schema.empty')} />
          </Section>
          <div className="grid grid-cols-1 gap-5 md:grid-cols-3">
            <Section icon={Heart} title={t('section.values')}><ReadOnlyList items={form.values} empty={tPill('schema.empty')} /></Section>
            <Section icon={ShieldAlert} title={t('section.antiPatterns')}><ReadOnlyList items={form.antiPatterns} empty={tPill('schema.empty')} /></Section>
            <Section icon={Handshake} title={t('section.honestLimits')}><ReadOnlyList items={form.honestLimits} empty={tPill('schema.empty')} /></Section>
          </div>
          <Section icon={MessagesSquare} title={t('section.dialogues')}>
            {form.dialogues.length === 0 ? <p className="text-sm text-muted-foreground">{tPill('schema.empty')}</p> : <div className="space-y-3">{form.dialogues.map((dialogue, index) => <article key={index} className="rounded-lg border border-border/70 p-3 text-sm"><p className="text-muted-foreground">{dialogue.user}</p><p className="mt-2 whitespace-pre-wrap">{dialogue.assistant}</p></article>)}</div>}
          </Section>
        </div>
        {showBind && <BindAgentModal pill={pill} onClose={() => setShowBind(false)} />}
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      {/* 返回按钮 */}
      <Link
        href="/pills"
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-gold transition-colors mb-4 whitespace-nowrap"
      >
        <ArrowLeft className="w-4 h-4" />
        {tPill('backToList')}
      </Link>

      {/* 金丹信息头部 */}
      <div className="dao-card p-5 md:p-6 mb-6">
        <div className="flex flex-col md:flex-row md:items-start gap-4">
          {/* 图标 */}
          <div className="shrink-0 w-16 h-16 rounded-2xl flex items-center justify-center bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
            <FlaskConical className="w-8 h-8" />
          </div>

          {/* 元信息编辑 */}
          <div className="flex-1 min-w-0 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <input
                value={form.name}
                onChange={e => patch({ name: e.target.value })}
                className="dao-input text-lg font-serif font-bold flex-1 min-w-[200px]"
                placeholder={t('pillNamePlaceholder')}
              />
              {pill.is_builtin && (
                <span className="text-[10px] px-2 py-0.5 rounded-full border bg-sage/15 text-sage border-sage/30 whitespace-nowrap shrink-0">
                  {t('builtInBadge')}
                </span>
              )}
            </div>

            <textarea
              value={form.description}
              onChange={e => patch({ description: e.target.value })}
              className="dao-textarea"
              rows={2}
              placeholder={t('descPlaceholder')}
            />

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <User className="w-3 h-3" />
                  {t('authorLabel')}
                </label>
                <input
                  value={form.author}
                  onChange={e => patch({ author: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('authorPlaceholder')}
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label">{t('versionLabel')}</label>
                <input
                  value={form.version}
                  onChange={e => patch({ version: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="1.0.0"
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <Tag className="w-3 h-3" />
                  {t('tagsLabel')}
                </label>
                <input
                  value={form.tagsText}
                  onChange={e => patch({ tagsText: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('tagsPlaceholder')}
                />
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
              <span className="flex items-center gap-1 min-w-0">
                <Clock className="w-3.5 h-3.5 shrink-0" />
                <span className="truncate">{formatDateTime(pill.created_at)}</span>
              </span>
            </div>
          </div>

          {/* 操作按钮 */}
          <div className="flex md:flex-col items-center gap-2 shrink-0">
            <button
              onClick={() => {
                setForm(buildForm(pill.skill_schema, pill))
                setIsEditing(false)
              }}
              className="dao-btn-ghost text-sm whitespace-nowrap"
            >
              <X className="w-4 h-4" />
              {tPill('cancelCta')}
            </button>
            <button onClick={() => setShowBind(true)} className="dao-btn-gold text-sm whitespace-nowrap">
              <Gift className="w-4 h-4" />
              {tPill('bindCta')}
            </button>
            <button
              onClick={handleDelete}
              className="dao-btn-ghost text-sm text-primary hover:text-primary/80 whitespace-nowrap"
            >
              <Trash2 className="w-4 h-4" />
              {t('destroyCta')}
            </button>
          </div>
        </div>
      </div>

      <div className="space-y-5">
        {/* 身份卡 */}
        <Section icon={IdCard} title={t('section.identity')}>
          <textarea
            value={form.identityCard}
            onChange={e => patch({ identityCard: e.target.value })}
            className="dao-textarea"
            rows={3}
            placeholder={t('identityPlaceholder')}
          />
        </Section>

        {/* 表达 DNA */}
        <Section icon={Dna} title={t('section.dna')}>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="min-w-0">
              <label className="dao-label">{t('dna.sentenceLength')}</label>
              <select
                value={form.dna.sentence_length}
                onChange={e => patchDna({ sentence_length: e.target.value as SentenceLength })}
                className="dao-input"
              >
                <option value="short">{t('dna.lengthOption.short')}</option>
                <option value="medium">{t('dna.lengthOption.medium')}</option>
                <option value="long">{t('dna.lengthOption.long')}</option>
                <option value="mixed">{t('dna.lengthOption.mixed')}</option>
              </select>
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.formality', { value: form.dna.formality.toFixed(2) })}</label>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={form.dna.formality}
                onChange={e => patchDna({ formality: Number(e.target.value) })}
                className="w-full accent-gold mt-2.5"
              />
              <div className="flex justify-between text-[10px] text-sage">
                <span>{t('dna.casual')}</span>
                <span>{t('dna.formal')}</span>
              </div>
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.vocabulary')}</label>
              <input
                value={form.dna.vocabularyText}
                onChange={e => patchDna({ vocabularyText: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.vocabularyPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.tabooWords')}</label>
              <input
                value={form.dna.tabooWordsText}
                onChange={e => patchDna({ tabooWordsText: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.tabooWordsPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.rhythm')}</label>
              <input
                value={form.dna.rhythm}
                onChange={e => patchDna({ rhythm: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.rhythmPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.humor')}</label>
              <input
                value={form.dna.humor_type}
                onChange={e => patchDna({ humor_type: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.humorPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.certainty')}</label>
              <input
                value={form.dna.certainty_style}
                onChange={e => patchDna({ certainty_style: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.certaintyPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.citation')}</label>
              <input
                value={form.dna.citation_habit}
                onChange={e => patchDna({ citation_habit: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.citationPlaceholder')}
              />
            </div>
          </div>
        </Section>

        {/* 心智模型 */}
        <Section icon={BrainCircuit} title={t('section.mentalModels')}>
          <div className="space-y-4">
            {form.mentalModels.map((model, index) => (
              <div key={index} className="p-3 rounded-lg bg-muted border border-border/70 space-y-2">
                <div className="flex items-center gap-2">
                  <input
                    value={model.name}
                    onChange={e => {
                      const next = [...form.mentalModels]
                      next[index] = { ...model, name: e.target.value }
                      patch({ mentalModels: next })
                    }}
                    className="dao-input flex-1 py-1.5 text-sm min-w-0"
                    placeholder={t('mentalModel.namePlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={t('mentalModel.deleteAria')}
                    onClick={() => patch({ mentalModels: form.mentalModels.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors shrink-0"
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
                  placeholder={t('mentalModel.oneLinerPlaceholder')}
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
                  placeholder={t('mentalModel.applicationPlaceholder')}
                />
                <input
                  value={model.limitationsText}
                  onChange={e => {
                    const next = [...form.mentalModels]
                    next[index] = { ...model, limitationsText: e.target.value }
                    patch({ mentalModels: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('mentalModel.limitationsPlaceholder')}
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ mentalModels: [...form.mentalModels, { name: '', one_liner: '', application: '', limitationsText: '' }] })}
              className="flex items-center gap-1 text-xs text-gold/70 hover:text-gold transition-colors whitespace-nowrap"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('mentalModel.addLabel')}
            </button>
          </div>
        </Section>

        {/* 决策启发式 */}
        <Section icon={Split} title={t('section.heuristics')}>
          <div className="space-y-4">
            {form.heuristics.map((heuristic, index) => (
              <div key={index} className="p-3 rounded-lg bg-muted border border-border/70 space-y-2">
                <div className="flex items-center gap-2">
                  <input
                    value={heuristic.condition}
                    onChange={e => {
                      const next = [...form.heuristics]
                      next[index] = { ...heuristic, condition: e.target.value }
                      patch({ heuristics: next })
                    }}
                    className="dao-input flex-1 py-1.5 text-sm min-w-0"
                    placeholder={t('heuristic.conditionPlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={t('heuristic.deleteAria')}
                    onClick={() => patch({ heuristics: form.heuristics.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors shrink-0"
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
                  placeholder={t('heuristic.actionPlaceholder')}
                />
                <input
                  value={heuristic.case || ''}
                  onChange={e => {
                    const next = [...form.heuristics]
                    next[index] = { ...heuristic, case: e.target.value }
                    patch({ heuristics: next })
                  }}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('heuristic.casePlaceholder')}
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ heuristics: [...form.heuristics, { condition: '', action: '', case: '' }] })}
              className="flex items-center gap-1 text-xs text-gold/70 hover:text-gold transition-colors whitespace-nowrap"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('heuristic.addLabel')}
            </button>
          </div>
        </Section>

        {/* 价值观 */}
        <Section icon={Heart} title={t('section.values')}>
          <StringListEditor
            items={form.values}
            onChange={values => patch({ values })}
            placeholder={t('values.placeholder')}
            addLabel={t('values.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 反模式 */}
        <Section icon={ShieldAlert} title={t('section.antiPatterns')}>
          <StringListEditor
            items={form.antiPatterns}
            onChange={antiPatterns => patch({ antiPatterns })}
            placeholder={t('antiPatterns.placeholder')}
            addLabel={t('antiPatterns.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 诚实边界 */}
        <Section icon={Handshake} title={t('section.honestLimits')}>
          <StringListEditor
            items={form.honestLimits}
            onChange={honestLimits => patch({ honestLimits })}
            placeholder={t('honestLimits.placeholder')}
            addLabel={t('honestLimits.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 示例对话 */}
        <Section icon={MessagesSquare} title={t('section.dialogues')}>
          <div className="space-y-4">
            {form.dialogues.map((dialogue, index) => (
              <div key={index} className="p-3 rounded-lg bg-muted border border-border/70 space-y-2">
                <div className="flex items-start gap-2">
                  <div className="flex-1 space-y-2 min-w-0">
                    <textarea
                      value={dialogue.user}
                      onChange={e => {
                        const next = [...form.dialogues]
                        next[index] = { ...dialogue, user: e.target.value }
                        patch({ dialogues: next })
                      }}
                      className="dao-textarea text-sm"
                      rows={2}
                      placeholder={t('dialogue.userPlaceholder')}
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
                      placeholder={t('dialogue.assistantPlaceholder')}
                    />
                  </div>
                  <button
                    type="button"
                    aria-label={t('dialogue.deleteAria')}
                    onClick={() => patch({ dialogues: form.dialogues.filter((_, i) => i !== index) })}
                    className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ dialogues: [...form.dialogues, { user: '', assistant: '' }] })}
              className="flex items-center gap-1 text-xs text-gold/70 hover:text-gold transition-colors whitespace-nowrap"
            >
              <Plus className="w-3.5 h-3.5" />
              {t('dialogue.addLabel')}
            </button>
          </div>
        </Section>
      </div>

      {/* 底部保存栏 */}
      <div className="sticky bottom-4 mt-6 flex items-center justify-end gap-3 flex-wrap">
        {saved && (
          <span className="flex items-center gap-1 text-sm text-sage animate-in fade-in duration-300 whitespace-nowrap">
            <CircleDot className="w-4 h-4" />
            {t('savedFeedback')}
          </span>
        )}
        <button
          onClick={handleSave}
          disabled={!form.name.trim() || saving}
          className="dao-btn-primary shadow-lg disabled:opacity-50 whitespace-nowrap"
        >
          {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
          {t('saveCta')}
        </button>
      </div>

      {/* 从金丹到道人 - 快捷绑定弹窗 */}
      {showBind && (
        <BindAgentModal pill={pill} onClose={() => setShowBind(false)} />
      )}
    </div>
  )
}
