'use client'

/**
 * 丹方页面（金丹消耗品重构任务 6）
 * 丹方永久保留；列表显示配方、版本 vN、可用库存数量与归档标识。
 * - 搜索防抖 300ms；分页「加载更多」（后端分页，不只取前 100 条）
 * - 新建丹方：女娲只生成草稿，保存走 SaveRecipe（craft_one=false，不炼制）；
 *   幂等 key 提交，断线恢复先查 operation，成功后进入丹方详情
 * - 卡片炼制成功回调刷新列表计数
 */
import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  BookOpen,
  FlaskConical,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  X,
} from 'lucide-react'
import { RecipeCard } from '@/components/recipe-card'
import { NuwaDistillPanel } from '@/components/nuwa-distill-panel'
import { PillWorkspaceHeader, PillWorkspacePage } from '@/components/layout/pill-workspace-layout'
import { recipeDetailHref } from '@/lib/entity-detail-route'
import { listRecipes, saveRecipe } from '@/services/recipeService'
import {
  clearPendingOperation,
  recoverOperation,
  startPendingOperation,
} from '@/lib/pending-operations'
import type { DistillationDraft, RecipeListItem } from '@/services/types'

const PAGE_SIZE = 24

export default function RecipesPage() {
  const t = useTranslations('recipes')
  const router = useRouter()

  const [recipes, setRecipes] = useState<RecipeListItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')

  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createName, setCreateName] = useState('')
  const [createDescription, setCreateDescription] = useState('')
  const [distilledDraft, setDistilledDraft] = useState<DistillationDraft | null>(null)

  /** 按搜索条件加载丹方列表；append=true 追加下一页 */
  const loadRecipes = useCallback(async (keyword: string, page: number, append = false) => {
    setLoading(true)
    setError(null)
    try {
      const data = await listRecipes({
        page,
        size: PAGE_SIZE,
        keyword: keyword.trim() || undefined,
      })
      setRecipes((prev) => (append ? [...prev, ...data.items] : data.items))
      setTotal(data.total)
      setPage(page)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  // 首次立即加载；输入搜索词时防抖。保持单一请求入口，避免初始化双请求竞态。
  useEffect(() => {
    const timer = setTimeout(
      () => void loadRecipes(searchQuery, 1),
      searchQuery.trim() ? 300 : 0,
    )
    return () => clearTimeout(timer)
  }, [searchQuery, loadRecipes])

  /** 新建丹方：SaveRecipe 不炼制；成功后进入丹方详情完善（幂等提交 + 断线恢复） */
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!createName.trim() || creating) return
    setCreating(true)
    // 每个明确保存动作一个幂等 key；pending 期间同目标复用（重试不换 key）
    const key = startPendingOperation('save_recipe', createName.trim())
    const navigateTo = (recipeId: string) => {
      setShowCreate(false)
      setCreateName('')
      setCreateDescription('')
      setDistilledDraft(null)
      router.push(recipeDetailHref(recipeId))
    }
    try {
      const result = await saveRecipe(
        key,
        {
          name: createName.trim(),
          description: createDescription.trim() || undefined,
          skill_schema: distilledDraft?.skill_schema ?? {},
          tags: distilledDraft?.tags ?? [],
        },
        false,
      )
      clearPendingOperation(key)
      // 服务端必回 recipe_id；防御性判空（缺失时留在弹窗，不导航）
      if (result.recipe_id) navigateTo(result.recipe_id)
    } catch {
      // 断线恢复：先查已提交结果；未提交保留原 key 供重试（留在弹窗）
      try {
        const committed = await recoverOperation(key)
        if (committed?.recipe_id) {
          clearPendingOperation(key)
          navigateTo(committed.recipe_id)
        }
      } catch {
        // 恢复查询也失败：留在弹窗，同 key 重试
      }
    } finally {
      setCreating(false)
    }
  }

  const hasMore = recipes.length < total

  return (
    <PillWorkspacePage>
      <PillWorkspaceHeader
        icon={<BookOpen className="h-6 w-6 shrink-0 text-gold" />}
        title={t('title')}
        subtitle={t('subtitle')}
        actions={
          <button
            onClick={() => setShowCreate(true)}
            className="dao-btn-primary whitespace-nowrap"
          >
            <Plus className="h-4 w-4" />
            {t('create')}
          </button>
        }
      />

      {/* 搜索栏 */}
      <div className="mb-6">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-sage" />
          <input
            type="text"
            placeholder={t('searchPlaceholder')}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="dao-input pl-10"
          />
        </div>
      </div>

      {/* 新建丹方弹窗（女娲生成草稿；保存不炼制） */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="dao-card max-h-[90vh] w-full max-w-md overflow-y-auto p-6 animate-in fade-in duration-300">
            <div className="mb-5 flex items-center justify-between gap-2">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                <FlaskConical className="h-5 w-5 shrink-0 text-primary" />
                <h2 className="truncate font-serif text-lg font-bold text-gold">
                  {t('modal.title')}
                </h2>
              </div>
              <button
                aria-label={t('closeModal')}
                onClick={() => setShowCreate(false)}
                className="shrink-0 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <form onSubmit={handleCreate} className="space-y-4">
              <NuwaDistillPanel
                onApply={(draft) => {
                  setDistilledDraft(draft)
                  setCreateName(draft.name)
                  setCreateDescription(draft.description)
                }}
              />

              <div>
                <label className="dao-label">{t('modal.nameLabel')}</label>
                <input
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder={t('modal.namePlaceholder')}
                  className="dao-input"
                  autoFocus
                  required
                />
              </div>

              <div>
                <label className="dao-label">{t('modal.descLabel')}</label>
                <textarea
                  value={createDescription}
                  onChange={(e) => setCreateDescription(e.target.value)}
                  placeholder={t('modal.descPlaceholder')}
                  className="dao-textarea"
                  rows={3}
                />
                <p className="mt-1 text-[10px] text-sage">{t('modal.hint')}</p>
              </div>

              <div className="flex flex-wrap items-center gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreate(false)}
                  className="dao-btn-ghost flex-1 whitespace-nowrap"
                >
                  {t('modal.cancel')}
                </button>
                <button
                  type="submit"
                  disabled={!createName.trim() || creating}
                  className="dao-btn-primary flex-1 whitespace-nowrap disabled:opacity-50"
                >
                  {creating ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <FlaskConical className="h-4 w-4" />
                  )}
                  {t('modal.submit')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 加载状态 */}
      {loading && recipes.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      )}

      {/* 错误状态 */}
      {!loading && error && (
        <div className="dao-card flex flex-col items-center px-6 py-10 text-center">
          <AlertCircle className="mb-3 h-10 w-10 text-primary" />
          <h3 className="mb-1 font-medium text-foreground">{t('loadErrorTitle')}</h3>
          <p className="mb-4 max-w-xl break-words text-sm text-muted-foreground">{error}</p>
          <button
            type="button"
            onClick={() => void loadRecipes(searchQuery, 1)}
            className="dao-btn-ghost"
          >
            <RefreshCw className="h-4 w-4" />
            {t('retry')}
          </button>
        </div>
      )}

      {/* 空状态 */}
      {!loading && !error && recipes.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <BookOpen className="mb-3 h-12 w-12 text-sage/50" />
          <h3 className="mb-1 text-base font-medium text-muted-foreground">
            {searchQuery ? t('emptySearchTitle') : t('emptyTitle')}
          </h3>
          <p className="mb-4 text-sm text-sage">
            {searchQuery ? t('emptySearchDesc') : t('emptyDesc')}
          </p>
          {!searchQuery && (
            <button onClick={() => setShowCreate(true)} className="dao-btn-primary whitespace-nowrap">
              <Plus className="h-4 w-4" />
              {t('create')}
            </button>
          )}
        </div>
      )}

      {/* 丹方列表 - 桌面端网格，H5 单列 */}
      {recipes.length > 0 && (
        <>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {recipes.map((recipe) => (
              <RecipeCard
                key={recipe.id}
                recipe={recipe}
                onCrafted={() => void loadRecipes(searchQuery, 1)}
              />
            ))}
          </div>
          {hasMore && (
            <div className="mt-6 flex justify-center">
              <button
                type="button"
                onClick={() => void loadRecipes(searchQuery, page + 1, true)}
                disabled={loading}
                className="dao-btn-ghost whitespace-nowrap"
              >
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                {t('loadMore')}
              </button>
            </div>
          )}
        </>
      )}
    </PillWorkspacePage>
  )
}
