'use client'

/**
 * 金丹详情页面 - 炼丹房编辑器
 * 只读/编辑双态,编辑直接绑定 usePillEditorFlow 的结构化草稿:
 * - 内置金丹只读,编辑前必须「制作副本」;自定义才有编辑/销毁
 * - 保存=PUT 成功后重新 GET 回源,失败保留全部字段并可重试(ActionFeedback)
 * - 自定义删除二次确认;未保存修改时拦截浏览器关闭与站内返回
 * - 动态数组使用稳定本地 key(非数组下标);未知 skill_schema 键原样保留
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  ArrowLeft,
  BrainCircuit,
  Clock,
  Copy,
  Dna,
  FlaskConical,
  Gift,
  Handshake,
  Heart,
  IdCard,
  Loader2,
  MessagesSquare,
  Pencil,
  Plus,
  Save,
  ShieldAlert,
  Split,
  Tag,
  Trash2,
  User,
  X,
} from 'lucide-react'
import { usePill } from '@/contexts/PillContext'
import { usePillEditorFlow } from '@/hooks/use-pill-editor-flow'
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes'
import { BindAgentModal } from '@/components/bind-agent-modal'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { formatDateTime } from '@/utils/format'
import type {
  DecisionHeuristic,
  ExampleDialogue,
  ExpressionDNA,
  MentalModel,
  SentenceLength,
  SkillSchema,
} from '@/services/types'

// ========== 稳定行 key(非数组下标) ==========

/**
 * 为动态数组维护稳定本地 key:编辑不换 key,删除精对齐,新增尾部追加。
 * key 只用于 React reconciliation,不写入保存数据。
 */
function useRowKeyStore() {
  const storeRef = useRef<Record<string, string[]>>({})
  const counterRef = useRef(0)
  const keysFor = useCallback((listId: string, count: number): string[] => {
    const store = storeRef.current
    if (!store[listId]) store[listId] = []
    const keys = store[listId]
    while (keys.length < count) keys.push(`${listId}-${++counterRef.current}`)
    if (keys.length > count) keys.length = count
    return keys
  }, [])
  const removeAt = useCallback((listId: string, index: number) => {
    storeRef.current[listId]?.splice(index, 1)
  }, [])
  return { keysFor, removeAt }
}

// ========== 通用展示/编辑子组件 ==========

/** 章节容器(标题可换行,不截断) */
function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: React.ElementType
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="dao-card p-5">
      <div className="mb-4 flex items-center gap-2">
        <Icon className="h-4 w-4 shrink-0 text-gold" />
        <h2 className="min-w-0 break-words font-serif text-base font-bold text-gold">{title}</h2>
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
        <li
          key={`${index}-${item}`}
          className="rounded-lg border border-border/70 bg-muted px-3 py-2 text-sm leading-relaxed"
        >
          {item}
        </li>
      ))}
    </ul>
  )
}

/** 字符串列表编辑器(稳定行 key) */
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
  const { keysFor, removeAt } = useRowKeyStore()
  const keys = keysFor('rows', items.length)
  return (
    <div className="space-y-2">
      {items.map((item, index) => (
        <div key={keys[index]} className="flex items-center gap-2">
          <input
            type="text"
            value={item}
            onChange={(e) => {
              const next = [...items]
              next[index] = e.target.value
              onChange(next)
            }}
            placeholder={placeholder}
            className="dao-input min-w-0 flex-1 py-1.5 text-sm"
          />
          <button
            type="button"
            aria-label={deleteAria}
            onClick={() => {
              removeAt('rows', index)
              onChange(items.filter((_, i) => i !== index))
            }}
            className="shrink-0 rounded p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={() => onChange([...items, ''])}
        className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/70 transition-colors hover:text-gold"
      >
        <Plus className="h-3.5 w-3.5" />
        {addLabel}
      </button>
    </div>
  )
}

// ========== 页面组件 ==========

export default function PillDetailPage() {
  const t = useTranslations('pill.editor')
  const tPill = useTranslations('pill')
  const tCommon = useTranslations('common')
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const pillId = id

  const { state, fetchPill, removePill } = usePill()
  const pill = state.currentPill

  const flow = usePillEditorFlow(pill, {
    onCopied: (newPillId) => router.push(`/pills/${newPillId}`),
  })
  useUnsavedChanges(flow.dirty, t('unsavedConfirm'))

  const [showBind, setShowBind] = useState(false)
  const [deleteArmed, setDeleteArmed] = useState(false)
  const [deleteStatus, setDeleteStatus] = useState<'idle' | 'submitting' | 'error'>('idle')

  // 加载数据
  useEffect(() => {
    if (pillId) fetchPill(pillId)
  }, [pillId, fetchPill])

  // ---- 草稿结构化编辑辅助(展开保留未知键) ----
  const patchSchema = (partial: Partial<SkillSchema>) => {
    flow.updateDraft({ skill_schema: { ...flow.draft.skill_schema, ...partial } })
  }
  const patchDna = (partial: Partial<ExpressionDNA>) => {
    patchSchema({ expression_dna: { ...(flow.draft.skill_schema.expression_dna ?? {}), ...partial } })
  }
  const { keysFor, removeAt } = useRowKeyStore()

  const mentalModels = flow.draft.skill_schema.mental_models ?? []
  const heuristics = flow.draft.skill_schema.decision_heuristics ?? []
  const dialogues = flow.draft.skill_schema.example_dialogues ?? []

  const updateModel = (index: number, next: MentalModel) => {
    const list = [...mentalModels]
    list[index] = next
    patchSchema({ mental_models: list })
  }
  const removeModel = (index: number) => {
    removeAt('mentalModels', index)
    patchSchema({ mental_models: mentalModels.filter((_, i) => i !== index) })
  }
  const updateHeuristic = (index: number, next: DecisionHeuristic) => {
    const list = [...heuristics]
    list[index] = next
    patchSchema({ decision_heuristics: list })
  }
  const removeHeuristic = (index: number) => {
    removeAt('heuristics', index)
    patchSchema({ decision_heuristics: heuristics.filter((_, i) => i !== index) })
  }
  const updateDialogue = (index: number, next: ExampleDialogue) => {
    const list = [...dialogues]
    list[index] = next
    patchSchema({ example_dialogues: list })
  }
  const removeDialogue = (index: number) => {
    removeAt('dialogues', index)
    patchSchema({ example_dialogues: dialogues.filter((_, i) => i !== index) })
  }

  /** 保存(写后重读由 flow 保证) */
  const handleSave = () => {
    void flow.save()
  }

  /** 制作副本(内置) */
  const handleMakeCopy = () => {
    void flow.makeCopy()
  }

  /** 真正执行删除(二次确认后或失败重试时调用) */
  const performDelete = async () => {
    if (!pill) return
    setDeleteStatus('submitting')
    try {
      await removePill(pill.id)
      router.push('/pills')
    } catch {
      // 失败保留页面,错误经 ActionFeedback 展示
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

  // ========== 加载 / 错误 / 空 三态 ==========
  if (!pill && state.loading) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{tPill('loading')}</p>
        </div>
      </div>
    )
  }

  if (!pill && state.error) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="mb-4 max-w-xl break-words text-center text-sm text-muted-foreground">
            {state.error}
          </p>
          <button type="button" onClick={() => fetchPill(pillId)} className="dao-btn-ghost">
            {tCommon('retry')}
          </button>
        </div>
      </div>
    )
  }

  if (!pill) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{tPill('notFound')}</p>
          <Link href="/pills" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {tPill('backToList')}
          </Link>
        </div>
      </div>
    )
  }

  // ========== 只读态 ==========
  if (flow.mode === 'readonly') {
    const schema = pill.skill_schema
    const dna = schema.expression_dna ?? {}
    const roModels = schema.mental_models ?? []
    const roDialogues = schema.example_dialogues ?? []
    const dnaHasContent = Boolean(
      dna.sentence_length ||
        typeof dna.formality === 'number' ||
        dna.rhythm ||
        dna.humor_type ||
        dna.certainty_style ||
        dna.citation_habit ||
        (dna.vocabulary && dna.vocabulary.length > 0),
    )
    return (
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
        <Link
          href="/pills"
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-gold"
        >
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
                <h1 className="break-words font-serif text-2xl font-bold text-foreground">
                  {pill.name}
                </h1>
                {pill.is_builtin && (
                  <span className="rounded-full border border-sage/30 bg-sage/15 px-2 py-0.5 text-[10px] text-sage">
                    {t('builtInBadge')}
                  </span>
                )}
              </div>
              <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">
                {pill.description || tPill('content.empty')}
              </p>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
                {pill.author && (
                  <span>
                    {t('authorLabel')}: {pill.author}
                  </span>
                )}
                <span>
                  {t('versionLabel')}: {pill.version}
                </span>
                <span className="flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5" />
                  {formatDateTime(pill.updated_at)}
                </span>
              </div>
              {pill.tags.length > 0 && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {pill.tags.map((tag) => (
                    <span
                      key={tag}
                      className="rounded-full border border-gold/20 px-2 py-0.5 text-[11px] text-gold"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              )}
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2 md:flex-col md:items-stretch">
              {pill.is_builtin ? (
                <button
                  type="button"
                  onClick={handleMakeCopy}
                  disabled={flow.saveStatus === 'submitting'}
                  className="dao-btn-primary text-sm"
                >
                  {flow.saveStatus === 'submitting' ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                  {flow.saveStatus === 'submitting' ? t('copying') : t('makeCopyCta')}
                </button>
              ) : (
                <button type="button" onClick={flow.beginEdit} className="dao-btn-primary text-sm">
                  <Pencil className="h-4 w-4" />
                  {tPill('editCta')}
                </button>
              )}
              <button type="button" onClick={() => setShowBind(true)} className="dao-btn-gold text-sm">
                <Gift className="h-4 w-4" />
                {tPill('bindCta')}
              </button>
              {!pill.is_builtin && (
                <button
                  type="button"
                  onClick={handleDelete}
                  disabled={deleteStatus === 'submitting'}
                  className="dao-btn-ghost text-sm text-primary hover:text-primary/80"
                >
                  {deleteStatus === 'submitting' ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Trash2 className="h-4 w-4" />
                  )}
                  {deleteArmed ? t('destroyArmCta') : t('destroyCta')}
                </button>
              )}
            </div>
          </div>

          {flow.saveStatus === 'error' && (
            <div className="mt-4">
              <ActionFeedback
                status="error"
                message={t('copyFailed')}
                onRetry={handleMakeCopy}
                retryLabel={tCommon('retry')}
              />
            </div>
          )}
          {deleteStatus === 'error' && (
            <div className="mt-4">
              <ActionFeedback
                status="error"
                message={t('deleteFailed')}
                onRetry={() => void performDelete()}
                retryLabel={tCommon('retry')}
              />
            </div>
          )}
        </div>

        <div className="space-y-5">
          <Section icon={IdCard} title={t('section.identity')}>
            <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">
              {schema.identity_card || tPill('schema.empty')}
            </p>
          </Section>

          <Section icon={Dna} title={t('section.dna')}>
            {dnaHasContent ? (
              <>
                <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                  {dna.sentence_length && (
                    <p>
                      <span className="text-muted-foreground">{t('dna.sentenceLength')} · </span>
                      {t(`dna.lengthOption.${dna.sentence_length}`)}
                    </p>
                  )}
                  {typeof dna.formality === 'number' && (
                    <p>
                      <span className="text-muted-foreground">
                        {t('dna.formality', { value: dna.formality.toFixed(2) })}
                      </span>
                    </p>
                  )}
                  {dna.rhythm && (
                    <p>
                      <span className="text-muted-foreground">{t('dna.rhythm')} · </span>
                      {dna.rhythm}
                    </p>
                  )}
                  {dna.humor_type && (
                    <p>
                      <span className="text-muted-foreground">{t('dna.humor')} · </span>
                      {dna.humor_type}
                    </p>
                  )}
                  {dna.certainty_style && (
                    <p>
                      <span className="text-muted-foreground">{t('dna.certainty')} · </span>
                      {dna.certainty_style}
                    </p>
                  )}
                  {dna.citation_habit && (
                    <p>
                      <span className="text-muted-foreground">{t('dna.citation')} · </span>
                      {dna.citation_habit}
                    </p>
                  )}
                </div>
                {dna.vocabulary && dna.vocabulary.length > 0 && (
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {dna.vocabulary.map((word) => (
                      <span key={word} className="rounded-full bg-gold/10 px-2 py-1 text-xs text-gold">
                        {word}
                      </span>
                    ))}
                  </div>
                )}
              </>
            ) : (
              <p className="text-sm text-muted-foreground">{tPill('schema.empty')}</p>
            )}
          </Section>

          <Section icon={BrainCircuit} title={t('section.mentalModels')}>
            {roModels.length === 0 ? (
              <p className="text-sm text-muted-foreground">{tPill('schema.empty')}</p>
            ) : (
              <div className="space-y-3">
                {roModels.map((model, index) => (
                  <article
                    key={`${index}-${model.name}`}
                    className="rounded-lg border border-border/70 bg-muted p-3"
                  >
                    <h3 className="font-medium">{model.name}</h3>
                    {model.one_liner && <p className="mt-1 text-sm text-gold">{model.one_liner}</p>}
                    {model.application && (
                      <p className="mt-1 whitespace-pre-wrap break-words text-sm text-muted-foreground">
                        {model.application}
                      </p>
                    )}
                  </article>
                ))}
              </div>
            )}
          </Section>

          <Section icon={Split} title={t('section.heuristics')}>
            <ReadOnlyList
              items={(schema.decision_heuristics ?? []).map((h) =>
                [h.condition, h.action, h.case].filter(Boolean).join(' → '),
              )}
              empty={tPill('schema.empty')}
            />
          </Section>

          <div className="grid grid-cols-1 gap-5 md:grid-cols-3">
            <Section icon={Heart} title={t('section.values')}>
              <ReadOnlyList items={schema.values ?? []} empty={tPill('schema.empty')} />
            </Section>
            <Section icon={ShieldAlert} title={t('section.antiPatterns')}>
              <ReadOnlyList items={schema.anti_patterns ?? []} empty={tPill('schema.empty')} />
            </Section>
            <Section icon={Handshake} title={t('section.honestLimits')}>
              <ReadOnlyList items={schema.honest_limits ?? []} empty={tPill('schema.empty')} />
            </Section>
          </div>

          <Section icon={MessagesSquare} title={t('section.dialogues')}>
            {roDialogues.length === 0 ? (
              <p className="text-sm text-muted-foreground">{tPill('schema.empty')}</p>
            ) : (
              <div className="space-y-3">
                {roDialogues.map((dialogue, index) => (
                  <article key={`${index}-${dialogue.user}`} className="rounded-lg border border-border/70 p-3 text-sm">
                    <p className="text-muted-foreground">{dialogue.user}</p>
                    <p className="mt-2 whitespace-pre-wrap break-words">{dialogue.assistant}</p>
                  </article>
                ))}
              </div>
            )}
          </Section>
        </div>

        {showBind && <BindAgentModal pill={pill} onClose={() => setShowBind(false)} />}
      </div>
    )
  }

  // ========== 编辑态 ==========
  const draft = flow.draft
  const dna = draft.skill_schema.expression_dna ?? {}
  const modelKeys = keysFor('mentalModels', mentalModels.length)
  const heuristicKeys = keysFor('heuristics', heuristics.length)
  const dialogueKeys = keysFor('dialogues', dialogues.length)

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      <Link
        href="/pills"
        className="mb-4 inline-flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-gold"
      >
        <ArrowLeft className="h-4 w-4" />
        {tPill('backToList')}
      </Link>

      {/* 金丹信息头部 */}
      <div className="dao-card mb-6 p-5 md:p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-start">
          <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
            <FlaskConical className="h-8 w-8" />
          </div>

          <div className="min-w-0 flex-1 space-y-3">
            <input
              value={draft.name}
              onChange={(e) => flow.updateDraft({ name: e.target.value })}
              className="dao-input min-w-[200px] flex-1 font-serif text-lg font-bold"
              placeholder={t('pillNamePlaceholder')}
            />

            <textarea
              value={draft.description}
              onChange={(e) => flow.updateDraft({ description: e.target.value })}
              className="dao-textarea"
              rows={2}
              placeholder={t('descPlaceholder')}
            />

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <User className="h-3 w-3" />
                  {t('authorLabel')}
                </label>
                <input
                  value={draft.author ?? ''}
                  onChange={(e) => flow.updateDraft({ author: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('authorPlaceholder')}
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label">{t('versionLabel')}</label>
                <input
                  value={draft.version ?? ''}
                  onChange={(e) => flow.updateDraft({ version: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="1.0.0"
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5 shrink-0" />
                  {formatDateTime(pill.created_at)}
                </label>
              </div>
            </div>

            <div className="min-w-0">
              <label className="dao-label flex items-center gap-1">
                <Tag className="h-3 w-3" />
                {t('tagsLabel')}
              </label>
              <StringListEditor
                items={draft.tags ?? []}
                onChange={(tags) => flow.updateDraft({ tags })}
                placeholder={t('tagsPlaceholder')}
                addLabel={t('tagsAdd')}
                deleteAria={t('deleteItemAria')}
              />
            </div>
          </div>

          {/* 操作按钮 */}
          <div className="flex shrink-0 items-center gap-2 md:flex-col">
            <button type="button" onClick={flow.discard} className="dao-btn-ghost whitespace-nowrap text-sm">
              <X className="h-4 w-4" />
              {tPill('cancelCta')}
            </button>
            <button type="button" onClick={() => setShowBind(true)} className="dao-btn-gold whitespace-nowrap text-sm">
              <Gift className="h-4 w-4" />
              {tPill('bindCta')}
            </button>
          </div>
        </div>
      </div>

      <div className="space-y-5">
        {/* 身份卡 */}
        <Section icon={IdCard} title={t('section.identity')}>
          <textarea
            value={draft.skill_schema.identity_card ?? ''}
            onChange={(e) => patchSchema({ identity_card: e.target.value })}
            className="dao-textarea"
            rows={3}
            placeholder={t('identityPlaceholder')}
          />
        </Section>

        {/* 表达 DNA */}
        <Section icon={Dna} title={t('section.dna')}>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="min-w-0">
              <label className="dao-label">{t('dna.sentenceLength')}</label>
              <select
                value={dna.sentence_length ?? 'mixed'}
                onChange={(e) => patchDna({ sentence_length: e.target.value as SentenceLength })}
                className="dao-input"
              >
                <option value="short">{t('dna.lengthOption.short')}</option>
                <option value="medium">{t('dna.lengthOption.medium')}</option>
                <option value="long">{t('dna.lengthOption.long')}</option>
                <option value="mixed">{t('dna.lengthOption.mixed')}</option>
              </select>
            </div>
            <div className="min-w-0">
              <label className="dao-label">
                {t('dna.formality', { value: (dna.formality ?? 0.5).toFixed(2) })}
              </label>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={dna.formality ?? 0.5}
                onChange={(e) =>
                  patchDna({ formality: Math.min(1, Math.max(0, Number(e.target.value))) })
                }
                className="mt-2.5 w-full accent-gold"
              />
              <div className="flex justify-between text-[10px] text-sage">
                <span>{t('dna.casual')}</span>
                <span>{t('dna.formal')}</span>
              </div>
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.vocabulary')}</label>
              <StringListEditor
                items={dna.vocabulary ?? []}
                onChange={(vocabulary) => patchDna({ vocabulary })}
                placeholder={t('dna.vocabularyPlaceholder')}
                addLabel={t('dna.vocabularyAdd')}
                deleteAria={t('deleteItemAria')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.tabooWords')}</label>
              <StringListEditor
                items={dna.taboo_words ?? []}
                onChange={(taboo_words) => patchDna({ taboo_words })}
                placeholder={t('dna.tabooWordsPlaceholder')}
                addLabel={t('dna.tabooWordsAdd')}
                deleteAria={t('deleteItemAria')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.rhythm')}</label>
              <input
                value={dna.rhythm ?? ''}
                onChange={(e) => patchDna({ rhythm: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.rhythmPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.humor')}</label>
              <input
                value={dna.humor_type ?? ''}
                onChange={(e) => patchDna({ humor_type: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.humorPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.certainty')}</label>
              <input
                value={dna.certainty_style ?? ''}
                onChange={(e) => patchDna({ certainty_style: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.certaintyPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('dna.citation')}</label>
              <input
                value={dna.citation_habit ?? ''}
                onChange={(e) => patchDna({ citation_habit: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={t('dna.citationPlaceholder')}
              />
            </div>
          </div>
        </Section>

        {/* 心智模型 */}
        <Section icon={BrainCircuit} title={t('section.mentalModels')}>
          <div className="space-y-4">
            {mentalModels.map((model, index) => (
              <div key={modelKeys[index]} className="space-y-2 rounded-lg border border-border/70 bg-muted p-3">
                <div className="flex items-center gap-2">
                  <input
                    value={model.name}
                    onChange={(e) => updateModel(index, { ...model, name: e.target.value })}
                    className="dao-input min-w-0 flex-1 py-1.5 text-sm"
                    placeholder={t('mentalModel.namePlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={t('mentalModel.deleteAria')}
                    onClick={() => removeModel(index)}
                    className="shrink-0 rounded p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
                <input
                  value={model.one_liner ?? ''}
                  onChange={(e) => updateModel(index, { ...model, one_liner: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('mentalModel.oneLinerPlaceholder')}
                />
                <textarea
                  value={model.application ?? ''}
                  onChange={(e) => updateModel(index, { ...model, application: e.target.value })}
                  className="dao-textarea text-sm"
                  rows={2}
                  placeholder={t('mentalModel.applicationPlaceholder')}
                />
                <div>
                  <label className="dao-label">{t('mentalModel.limitationsLabel')}</label>
                  <StringListEditor
                    items={model.limitations ?? []}
                    onChange={(limitations) => updateModel(index, { ...model, limitations })}
                    placeholder={t('mentalModel.limitationsPlaceholder')}
                    addLabel={t('mentalModel.limitationsAdd')}
                    deleteAria={t('deleteItemAria')}
                  />
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={() => patchSchema({ mental_models: [...mentalModels, { name: '' }] })}
              className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/70 transition-colors hover:text-gold"
            >
              <Plus className="h-3.5 w-3.5" />
              {t('mentalModel.addLabel')}
            </button>
          </div>
        </Section>

        {/* 决策启发式 */}
        <Section icon={Split} title={t('section.heuristics')}>
          <div className="space-y-4">
            {heuristics.map((heuristic, index) => (
              <div key={heuristicKeys[index]} className="space-y-2 rounded-lg border border-border/70 bg-muted p-3">
                <div className="flex items-center gap-2">
                  <input
                    value={heuristic.condition}
                    onChange={(e) => updateHeuristic(index, { ...heuristic, condition: e.target.value })}
                    className="dao-input min-w-0 flex-1 py-1.5 text-sm"
                    placeholder={t('heuristic.conditionPlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={t('heuristic.deleteAria')}
                    onClick={() => removeHeuristic(index)}
                    className="shrink-0 rounded p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
                <input
                  value={heuristic.action}
                  onChange={(e) => updateHeuristic(index, { ...heuristic, action: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('heuristic.actionPlaceholder')}
                />
                <input
                  value={heuristic.case ?? ''}
                  onChange={(e) => updateHeuristic(index, { ...heuristic, case: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={t('heuristic.casePlaceholder')}
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() =>
                patchSchema({ decision_heuristics: [...heuristics, { condition: '', action: '', case: '' }] })
              }
              className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/70 transition-colors hover:text-gold"
            >
              <Plus className="h-3.5 w-3.5" />
              {t('heuristic.addLabel')}
            </button>
          </div>
        </Section>

        {/* 价值观 */}
        <Section icon={Heart} title={t('section.values')}>
          <StringListEditor
            items={draft.skill_schema.values ?? []}
            onChange={(values) => patchSchema({ values })}
            placeholder={t('values.placeholder')}
            addLabel={t('values.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 反模式 */}
        <Section icon={ShieldAlert} title={t('section.antiPatterns')}>
          <StringListEditor
            items={draft.skill_schema.anti_patterns ?? []}
            onChange={(anti_patterns) => patchSchema({ anti_patterns })}
            placeholder={t('antiPatterns.placeholder')}
            addLabel={t('antiPatterns.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 诚实边界 */}
        <Section icon={Handshake} title={t('section.honestLimits')}>
          <StringListEditor
            items={draft.skill_schema.honest_limits ?? []}
            onChange={(honest_limits) => patchSchema({ honest_limits })}
            placeholder={t('honestLimits.placeholder')}
            addLabel={t('honestLimits.addLabel')}
            deleteAria={t('deleteItemAria')}
          />
        </Section>

        {/* 示例对话 */}
        <Section icon={MessagesSquare} title={t('section.dialogues')}>
          <div className="space-y-4">
            {dialogues.map((dialogue, index) => (
              <div key={dialogueKeys[index]} className="space-y-2 rounded-lg border border-border/70 bg-muted p-3">
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1 space-y-2">
                    <textarea
                      value={dialogue.user}
                      onChange={(e) => updateDialogue(index, { ...dialogue, user: e.target.value })}
                      className="dao-textarea text-sm"
                      rows={2}
                      placeholder={t('dialogue.userPlaceholder')}
                    />
                    <textarea
                      value={dialogue.assistant}
                      onChange={(e) => updateDialogue(index, { ...dialogue, assistant: e.target.value })}
                      className="dao-textarea text-sm"
                      rows={3}
                      placeholder={t('dialogue.assistantPlaceholder')}
                    />
                  </div>
                  <button
                    type="button"
                    aria-label={t('dialogue.deleteAria')}
                    onClick={() => removeDialogue(index)}
                    className="shrink-0 rounded p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={() => patchSchema({ example_dialogues: [...dialogues, { user: '', assistant: '' }] })}
              className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/70 transition-colors hover:text-gold"
            >
              <Plus className="h-3.5 w-3.5" />
              {t('dialogue.addLabel')}
            </button>
          </div>
        </Section>
      </div>

      {/* 底部保存栏 */}
      <div className="sticky bottom-4 mt-6 flex flex-wrap items-center justify-end gap-3">
        {flow.saveStatus === 'submitting' && <ActionFeedback status="submitting" message={t('saving')} />}
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
          onClick={handleSave}
          disabled={!draft.name.trim() || flow.saveStatus === 'submitting'}
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

      {/* 从金丹到道人 - 快捷绑定弹窗 */}
      {showBind && <BindAgentModal pill={pill} onClose={() => setShowBind(false)} />}
    </div>
  )
}
