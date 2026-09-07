'use client'

/**
 * 丹方详情页（金丹消耗品重构任务 6）— 只读/编辑新版本双态
 * - 丹方永久保留；「编辑新版本」= updateRecipe 生成 revision+1，旧金丹/能力不受影响
 * - 「炼制 1 枚」= craftPill(幂等 key)，归档丹方拒绝；断线恢复先查 operation
 * - 导出走 recipe_id(+revision_id) 只读模式，不消耗库存
 * - 编辑保存 = 写后重读回源（useRecipeEditorFlow）；409 冲突提示刷新
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  ArrowLeft,
  BookOpen,
  BrainCircuit,
  Clock,
  Dna,
  Download,
  FlaskConical,
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
import { ApiError } from '@/services/api'
import { useRecipeEditorFlow } from '@/hooks/use-recipe-editor-flow'
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes'
import { craftPill, getRecipe } from '@/services/recipeService'
import {
  clearPendingOperation,
  recoverOperation,
  startPendingOperation,
} from '@/lib/pending-operations'
import { SkillExportDialog } from '@/components/skill-export-dialog'
import { PillWorkspacePage } from '@/components/layout/pill-workspace-layout'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { formatDateTime } from '@/utils/format'
import type {
  DecisionHeuristic,
  ExampleDialogue,
  ExpressionDNA,
  MentalModel,
  RecipeDetail,
  SentenceLength,
  SkillSchema,
} from '@/services/types'

// ========== 稳定行 key(非数组下标) ==========

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

interface RecipeDetailPageProps {
  recipeId?: string
  /** 卡片「编辑新版本」带入：详情就绪后直接进入编辑态（归档丹方忽略） */
  initialEdit?: boolean
}

type LoadStatus = 'idle' | 'loading' | 'error' | 'not-found' | 'ready'
type CraftStatus = 'idle' | 'submitting' | 'error'

export default function RecipeDetailPage({ recipeId, initialEdit }: RecipeDetailPageProps) {
  const t = useTranslations('recipes')
  const tEditor = useTranslations('pill.editor')
  const tCommon = useTranslations('common')

  const [recipe, setRecipe] = useState<RecipeDetail | null>(null)
  const [loadStatus, setLoadStatus] = useState<LoadStatus>('idle')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [showExport, setShowExport] = useState(false)
  const [craftStatus, setCraftStatus] = useState<CraftStatus>('idle')
  const [craftMessage, setCraftMessage] = useState<string | null>(null)

  const flow = useRecipeEditorFlow(recipe, { onSaved: (r) => setRecipe(r) })
  const { mode: editorMode, beginEdit } = flow
  useUnsavedChanges(flow.dirty, t('unsavedConfirm'))
  // 编辑态行 key 仓库：必须在任何条件返回之前调用（hooks 顺序约束）
  const { keysFor, removeAt } = useRowKeyStore()

  const load = useCallback(async (id: string) => {
    setLoadStatus('loading')
    setLoadError(null)
    try {
      const data = await getRecipe(id)
      setRecipe(data)
      setLoadStatus('ready')
    } catch (err) {
      const status = err instanceof ApiError ? err.status : 0
      if (status === 404) {
        setLoadStatus('not-found')
      } else {
        setLoadStatus('error')
        setLoadError(err instanceof Error ? err.message : String(err))
      }
    }
  }, [])

  useEffect(() => {
    if (recipeId) void load(recipeId)
  }, [recipeId, load])

  // 「编辑新版本」直达：加载就绪且为只读态时进入编辑（归档丹方忽略；只触发一次）
  const initialEditFiredRef = useRef(false)
  useEffect(() => {
    if (!initialEdit || initialEditFiredRef.current) return
    if (loadStatus !== 'ready' || !recipe || editorMode !== 'readonly') return
    if (recipe.archived_at) return
    initialEditFiredRef.current = true
    beginEdit()
  }, [initialEdit, loadStatus, recipe, editorMode, beginEdit])

  /** 炼制 1 枚：每个明确动作一个幂等 key；断线恢复先查 operation 再同 key 重试 */
  const handleCraft = async () => {
    if (!recipe || craftStatus === 'submitting') return
    setCraftStatus('submitting')
    setCraftMessage(null)
    const key = startPendingOperation('craft', recipe.name)
    try {
      await craftPill(key, recipe.id, recipe.current_revision_id)
      clearPendingOperation(key)
      setCraftMessage(t('crafted'))
    } catch {
      try {
        const committed = await recoverOperation(key)
        if (committed) {
          clearPendingOperation(key)
          setCraftMessage(t('crafted'))
        } else {
          setCraftMessage(t('craftFailed'))
        }
      } catch {
        setCraftMessage(t('craftFailed'))
      }
    } finally {
      setCraftStatus('idle')
    }
  }

  // ========== 链接无效 / 加载 / 错误 / 不存在 四态 ==========
  if (!recipeId) {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{t('invalidLink')}</p>
          <Link href="/recipes" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {t('backToList')}
          </Link>
        </div>
      </PillWorkspacePage>
    )
  }

  if (loadStatus !== 'ready') {
    if (loadStatus === 'not-found') {
      return (
        <PillWorkspacePage>
          <div className="flex flex-col items-center justify-center py-16">
            <AlertCircle className="mb-3 h-12 w-12 text-primary" />
            <p className="text-sm text-muted-foreground">{t('notFound')}</p>
            <Link href="/recipes" className="dao-btn-primary mt-4 whitespace-nowrap">
              <ArrowLeft className="h-4 w-4" />
              {t('backToList')}
            </Link>
          </div>
        </PillWorkspacePage>
      )
    }
    if (loadStatus === 'error') {
      return (
        <PillWorkspacePage>
          <div className="flex flex-col items-center justify-center py-16">
            <AlertCircle className="mb-3 h-12 w-12 text-primary" />
            <p className="mb-4 max-w-xl break-words text-center text-sm text-muted-foreground">
              {loadError}
            </p>
            <button type="button" onClick={() => recipeId && void load(recipeId)} className="dao-btn-ghost">
              {tCommon('retry')}
            </button>
          </div>
        </PillWorkspacePage>
      )
    }
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loadingDetail')}</p>
        </div>
      </PillWorkspacePage>
    )
  }

  // 兜底:缺 id 外的任何状态未就绪都按加载处理,不闪现旧数据
  if (!recipe || recipe.id !== recipeId) {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loadingDetail')}</p>
        </div>
      </PillWorkspacePage>
    )
  }

  // ========== 只读态 ==========
  if (flow.mode === 'readonly') {
    const schema = recipe.skill_schema ?? {}
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
      <PillWorkspacePage>
        <Link
          href="/recipes"
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-gold"
        >
          <ArrowLeft className="h-4 w-4" />
          {t('backToList')}
        </Link>

        <div className="dao-card mb-6 p-5 md:p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start">
            <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
              <BookOpen className="h-8 w-8" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="break-words font-serif text-2xl font-bold text-foreground">
                  {recipe.name}
                </h1>
                {recipe.archived_at && (
                  <span className="rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-[10px] text-primary">
                    {t('archivedBadge')}
                  </span>
                )}
                <span className="rounded-full border border-border/70 bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
                  {t('revisionLabel', { revision: recipe.revision })}
                </span>
              </div>
              <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">
                {recipe.description || t('content.empty')}
              </p>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
                {recipe.author && (
                  <span>
                    {t('authorLabel')}: {recipe.author}
                  </span>
                )}
                {recipe.version_label && (
                  <span>
                    {t('versionLabel')}: {recipe.version_label}
                  </span>
                )}
                <span className="flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5" />
                  {t('createdLabel')} {formatDateTime(recipe.created_at)}
                </span>
              </div>
              {recipe.tags && recipe.tags.length > 0 && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {recipe.tags.map((tag) => (
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
              <button
                type="button"
                onClick={() => void handleCraft()}
                disabled={craftStatus === 'submitting' || Boolean(recipe.archived_at)}
                className="dao-btn-primary text-sm disabled:opacity-50"
              >
                {craftStatus === 'submitting' ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <FlaskConical className="h-4 w-4" />
                )}
                {craftStatus === 'submitting' ? t('crafting') : t('craftCta')}
              </button>
              <button
                type="button"
                onClick={() => setShowExport(true)}
                className="dao-btn-gold text-sm"
              >
                <Download className="h-4 w-4" />
                {t('exportSkillCta')}
              </button>
              {!recipe.archived_at && (
                <button type="button" onClick={flow.beginEdit} className="dao-btn-ghost text-sm">
                  <Pencil className="h-4 w-4" />
                  {t('editCta')}
                </button>
              )}
            </div>
          </div>

          {craftMessage && (
            <div className="mt-4">
              <ActionFeedback
                status={craftMessage === t('crafted') ? 'success' : 'error'}
                message={craftMessage}
              />
            </div>
          )}
          {recipe.archived_at && (
            <p className="mt-4 rounded-lg border border-sage/30 bg-sage/10 p-3 text-xs leading-relaxed text-muted-foreground">
              {t('archivedNotice')}
            </p>
          )}
        </div>

        <div className="space-y-5">
          <Section icon={IdCard} title={tEditor('section.identity')}>
            <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">
              {schema.identity_card || t('schema.empty')}
            </p>
          </Section>

          <Section icon={Dna} title={tEditor('section.dna')}>
            {dnaHasContent ? (
              <>
                <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                  {dna.sentence_length && (
                    <p>
                      <span className="text-muted-foreground">{tEditor('dna.sentenceLength')} · </span>
                      {tEditor(`dna.lengthOption.${dna.sentence_length}`)}
                    </p>
                  )}
                  {typeof dna.formality === 'number' && (
                    <p>
                      <span className="text-muted-foreground">
                        {tEditor('dna.formality', { value: dna.formality.toFixed(2) })}
                      </span>
                    </p>
                  )}
                  {dna.rhythm && (
                    <p>
                      <span className="text-muted-foreground">{tEditor('dna.rhythm')} · </span>
                      {dna.rhythm}
                    </p>
                  )}
                  {dna.humor_type && (
                    <p>
                      <span className="text-muted-foreground">{tEditor('dna.humor')} · </span>
                      {dna.humor_type}
                    </p>
                  )}
                  {dna.certainty_style && (
                    <p>
                      <span className="text-muted-foreground">{tEditor('dna.certainty')} · </span>
                      {dna.certainty_style}
                    </p>
                  )}
                  {dna.citation_habit && (
                    <p>
                      <span className="text-muted-foreground">{tEditor('dna.citation')} · </span>
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
              <p className="text-sm text-muted-foreground">{t('schema.empty')}</p>
            )}
          </Section>

          <Section icon={BrainCircuit} title={tEditor('section.mentalModels')}>
            {roModels.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('schema.empty')}</p>
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

          <Section icon={Split} title={tEditor('section.heuristics')}>
            <ReadOnlyList
              items={(schema.decision_heuristics ?? []).map((h) =>
                [h.condition, h.action, h.case].filter(Boolean).join(' → '),
              )}
              empty={t('schema.empty')}
            />
          </Section>

          <div className="grid grid-cols-1 gap-5 md:grid-cols-3">
            <Section icon={Heart} title={tEditor('section.values')}>
              <ReadOnlyList items={schema.values ?? []} empty={t('schema.empty')} />
            </Section>
            <Section icon={ShieldAlert} title={tEditor('section.antiPatterns')}>
              <ReadOnlyList items={schema.anti_patterns ?? []} empty={t('schema.empty')} />
            </Section>
            <Section icon={Handshake} title={tEditor('section.honestLimits')}>
              <ReadOnlyList items={schema.honest_limits ?? []} empty={t('schema.empty')} />
            </Section>
          </div>

          <Section icon={MessagesSquare} title={tEditor('section.dialogues')}>
            {roDialogues.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('schema.empty')}</p>
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

        {showExport && (
          <SkillExportDialog
            recipe={{ id: recipe.id, name: recipe.name }}
            onClose={() => setShowExport(false)}
          />
        )}
      </PillWorkspacePage>
    )
  }

  // ========== 编辑态（生成新版本） ==========
  const draft = flow.draft
  const dna = draft.skill_schema.expression_dna ?? {}
  const mentalModels = draft.skill_schema.mental_models ?? []
  const heuristics = draft.skill_schema.decision_heuristics ?? []
  const dialogues = draft.skill_schema.example_dialogues ?? []
  const modelKeys = keysFor('mentalModels', mentalModels.length)
  const heuristicKeys = keysFor('heuristics', heuristics.length)
  const dialogueKeys = keysFor('dialogues', dialogues.length)

  const patchSchema = (partial: Partial<SkillSchema>) => {
    flow.updateDraft({ skill_schema: { ...draft.skill_schema, ...partial } })
  }
  const patchDna = (partial: Partial<ExpressionDNA>) => {
    patchSchema({ expression_dna: { ...(draft.skill_schema.expression_dna ?? {}), ...partial } })
  }
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

  const handleSave = () => {
    void flow.save()
  }

  return (
    <PillWorkspacePage>
      <Link
        href="/recipes"
        className="mb-4 inline-flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-gold"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('backToList')}
      </Link>

      <div className="dao-card mb-6 p-5 md:p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-start">
          <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
            <BookOpen className="h-8 w-8" />
          </div>

          <div className="min-w-0 flex-1 space-y-3">
            <input
              value={draft.name}
              onChange={(e) => flow.updateDraft({ name: e.target.value })}
              className="dao-input min-w-[200px] flex-1 font-serif text-lg font-bold"
              placeholder={tEditor('pillNamePlaceholder')}
            />

            <textarea
              value={draft.description}
              onChange={(e) => flow.updateDraft({ description: e.target.value })}
              className="dao-textarea"
              rows={2}
              placeholder={tEditor('descPlaceholder')}
            />

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <User className="h-3 w-3" />
                  {t('authorLabel')}
                </label>
                <input
                  value={draft.author}
                  onChange={(e) => flow.updateDraft({ author: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={tEditor('authorPlaceholder')}
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label">{t('versionLabel')}</label>
                <input
                  value={draft.version_label}
                  onChange={(e) => flow.updateDraft({ version_label: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder="2.0.0"
                />
              </div>
              <div className="min-w-0">
                <label className="dao-label flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5 shrink-0" />
                  {t('revisionLabel', { revision: recipe.revision })} → v{recipe.revision + 1}
                </label>
              </div>
            </div>

            <div className="min-w-0">
              <label className="dao-label flex items-center gap-1">
                <Tag className="h-3 w-3" />
                {tEditor('tagsLabel')}
              </label>
              <StringListEditor
                items={draft.tags}
                onChange={(tags) => flow.updateDraft({ tags })}
                placeholder={tEditor('tagsPlaceholder')}
                addLabel={tEditor('tagsAdd')}
                deleteAria={tEditor('deleteItemAria')}
              />
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-2 md:flex-col">
            <button type="button" onClick={flow.discard} className="dao-btn-ghost whitespace-nowrap text-sm">
              <X className="h-4 w-4" />
              {t('editCancel')}
            </button>
          </div>
        </div>

        <p className="mt-4 rounded-lg border border-sage/30 bg-sage/10 p-3 text-xs leading-relaxed text-muted-foreground">
          {t('editHint')}
        </p>
      </div>

      <div className="space-y-5">
        <Section icon={IdCard} title={tEditor('section.identity')}>
          <textarea
            value={draft.skill_schema.identity_card ?? ''}
            onChange={(e) => patchSchema({ identity_card: e.target.value })}
            className="dao-textarea"
            rows={3}
            placeholder={tEditor('identityPlaceholder')}
          />
        </Section>

        <Section icon={Dna} title={tEditor('section.dna')}>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.sentenceLength')}</label>
              <select
                value={dna.sentence_length ?? 'mixed'}
                onChange={(e) => patchDna({ sentence_length: e.target.value as SentenceLength })}
                className="dao-input"
              >
                <option value="short">{tEditor('dna.lengthOption.short')}</option>
                <option value="medium">{tEditor('dna.lengthOption.medium')}</option>
                <option value="long">{tEditor('dna.lengthOption.long')}</option>
                <option value="mixed">{tEditor('dna.lengthOption.mixed')}</option>
              </select>
            </div>
            <div className="min-w-0">
              <label className="dao-label">
                {tEditor('dna.formality', { value: (dna.formality ?? 0.5).toFixed(2) })}
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
                <span>{tEditor('dna.casual')}</span>
                <span>{tEditor('dna.formal')}</span>
              </div>
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.vocabulary')}</label>
              <StringListEditor
                items={dna.vocabulary ?? []}
                onChange={(vocabulary) => patchDna({ vocabulary })}
                placeholder={tEditor('dna.vocabularyPlaceholder')}
                addLabel={tEditor('dna.vocabularyAdd')}
                deleteAria={tEditor('deleteItemAria')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.tabooWords')}</label>
              <StringListEditor
                items={dna.taboo_words ?? []}
                onChange={(taboo_words) => patchDna({ taboo_words })}
                placeholder={tEditor('dna.tabooWordsPlaceholder')}
                addLabel={tEditor('dna.tabooWordsAdd')}
                deleteAria={tEditor('deleteItemAria')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.rhythm')}</label>
              <input
                value={dna.rhythm ?? ''}
                onChange={(e) => patchDna({ rhythm: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={tEditor('dna.rhythmPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.humor')}</label>
              <input
                value={dna.humor_type ?? ''}
                onChange={(e) => patchDna({ humor_type: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={tEditor('dna.humorPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.certainty')}</label>
              <input
                value={dna.certainty_style ?? ''}
                onChange={(e) => patchDna({ certainty_style: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={tEditor('dna.certaintyPlaceholder')}
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{tEditor('dna.citation')}</label>
              <input
                value={dna.citation_habit ?? ''}
                onChange={(e) => patchDna({ citation_habit: e.target.value })}
                className="dao-input py-1.5 text-sm"
                placeholder={tEditor('dna.citationPlaceholder')}
              />
            </div>
          </div>
        </Section>

        <Section icon={BrainCircuit} title={tEditor('section.mentalModels')}>
          <div className="space-y-4">
            {mentalModels.map((model, index) => (
              <div key={modelKeys[index]} className="space-y-2 rounded-lg border border-border/70 bg-muted p-3">
                <div className="flex items-center gap-2">
                  <input
                    value={model.name}
                    onChange={(e) => updateModel(index, { ...model, name: e.target.value })}
                    className="dao-input min-w-0 flex-1 py-1.5 text-sm"
                    placeholder={tEditor('mentalModel.namePlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={tEditor('mentalModel.deleteAria')}
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
                  placeholder={tEditor('mentalModel.oneLinerPlaceholder')}
                />
                <textarea
                  value={model.application ?? ''}
                  onChange={(e) => updateModel(index, { ...model, application: e.target.value })}
                  className="dao-textarea text-sm"
                  rows={2}
                  placeholder={tEditor('mentalModel.applicationPlaceholder')}
                />
                <div>
                  <label className="dao-label">{tEditor('mentalModel.limitationsLabel')}</label>
                  <StringListEditor
                    items={model.limitations ?? []}
                    onChange={(limitations) => updateModel(index, { ...model, limitations })}
                    placeholder={tEditor('mentalModel.limitationsPlaceholder')}
                    addLabel={tEditor('mentalModel.limitationsAdd')}
                    deleteAria={tEditor('deleteItemAria')}
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
              {tEditor('mentalModel.addLabel')}
            </button>
          </div>
        </Section>

        <Section icon={Split} title={tEditor('section.heuristics')}>
          <div className="space-y-4">
            {heuristics.map((heuristic, index) => (
              <div key={heuristicKeys[index]} className="space-y-2 rounded-lg border border-border/70 bg-muted p-3">
                <div className="flex items-center gap-2">
                  <input
                    value={heuristic.condition}
                    onChange={(e) => updateHeuristic(index, { ...heuristic, condition: e.target.value })}
                    className="dao-input min-w-0 flex-1 py-1.5 text-sm"
                    placeholder={tEditor('heuristic.conditionPlaceholder')}
                  />
                  <button
                    type="button"
                    aria-label={tEditor('heuristic.deleteAria')}
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
                  placeholder={tEditor('heuristic.actionPlaceholder')}
                />
                <input
                  value={heuristic.case ?? ''}
                  onChange={(e) => updateHeuristic(index, { ...heuristic, case: e.target.value })}
                  className="dao-input py-1.5 text-sm"
                  placeholder={tEditor('heuristic.casePlaceholder')}
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
              {tEditor('heuristic.addLabel')}
            </button>
          </div>
        </Section>

        <Section icon={Heart} title={tEditor('section.values')}>
          <StringListEditor
            items={draft.skill_schema.values ?? []}
            onChange={(values) => patchSchema({ values })}
            placeholder={tEditor('values.placeholder')}
            addLabel={tEditor('values.addLabel')}
            deleteAria={tEditor('deleteItemAria')}
          />
        </Section>

        <Section icon={ShieldAlert} title={tEditor('section.antiPatterns')}>
          <StringListEditor
            items={draft.skill_schema.anti_patterns ?? []}
            onChange={(anti_patterns) => patchSchema({ anti_patterns })}
            placeholder={tEditor('antiPatterns.placeholder')}
            addLabel={tEditor('antiPatterns.addLabel')}
            deleteAria={tEditor('deleteItemAria')}
          />
        </Section>

        <Section icon={Handshake} title={tEditor('section.honestLimits')}>
          <StringListEditor
            items={draft.skill_schema.honest_limits ?? []}
            onChange={(honest_limits) => patchSchema({ honest_limits })}
            placeholder={tEditor('honestLimits.placeholder')}
            addLabel={tEditor('honestLimits.addLabel')}
            deleteAria={tEditor('deleteItemAria')}
          />
        </Section>

        <Section icon={MessagesSquare} title={tEditor('section.dialogues')}>
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
                      placeholder={tEditor('dialogue.userPlaceholder')}
                    />
                    <textarea
                      value={dialogue.assistant}
                      onChange={(e) => updateDialogue(index, { ...dialogue, assistant: e.target.value })}
                      className="dao-textarea text-sm"
                      rows={3}
                      placeholder={tEditor('dialogue.assistantPlaceholder')}
                    />
                  </div>
                  <button
                    type="button"
                    aria-label={tEditor('dialogue.deleteAria')}
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
              {tEditor('dialogue.addLabel')}
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
            message={flow.conflict ? t('saveConflict') : t('saveFailed')}
            onRetry={flow.conflict ? undefined : handleSave}
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
          {t('editSave')}
        </button>
      </div>
    </PillWorkspacePage>
  )
}
