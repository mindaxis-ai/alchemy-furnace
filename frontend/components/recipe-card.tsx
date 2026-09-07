'use client'

/**
 * 丹方卡片组件（金丹消耗品重构任务 6）
 * 整卡为单一键盘可访问的导航容器(role="link")，点击/Enter/Space 进入丹方详情；
 * 内部「炼制 1 枚」「导出 Skill」「编辑新版本」按钮阻止冒泡，避免双重导航。
 * - 炼制 1 枚 = craftPill(幂等 key)，成功后回调父级刷新库存计数
 * - 导出走丹方只读模式（当前不可变版本），不消耗库存
 * - 编辑新版本直达详情页编辑态（?edit=1）
 */
import { useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import {
  BookOpen,
  ChevronRight,
  Clock,
  Download,
  FlaskConical,
  Loader2,
  Pencil,
} from 'lucide-react'
import { recipeDetailHref, recipeEditHref } from '@/lib/entity-detail-route'
import { craftPill } from '@/services/recipeService'
import {
  clearPendingOperation,
  recoverOperation,
  startPendingOperation,
} from '@/lib/pending-operations'
import { SkillExportDialog } from '@/components/skill-export-dialog'
import type { RecipeListItem } from '@/services/types'
import { formatDateTime } from '@/utils/format'

interface RecipeCardProps {
  recipe: RecipeListItem
  /** 炼制 1 枚成功后的回调（父级刷新列表计数） */
  onCrafted?: () => void
}

export function RecipeCard({ recipe, onCrafted }: RecipeCardProps) {
  const t = useTranslations('recipes')
  const router = useRouter()
  const href = recipeDetailHref(recipe.id)

  const [showExport, setShowExport] = useState(false)
  const [craftStatus, setCraftStatus] = useState<'idle' | 'submitting' | 'success' | 'error'>('idle')
  const [craftError, setCraftError] = useState<string | null>(null)
  // 成功反馈 2s 后复位（卸载后 setState 为 no-op，无需清理）
  const successTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => () => clearTimeout(successTimerRef.current), [])

  const navigate = () => router.push(href)

  /** 炼制 1 枚：每个明确动作一个幂等 key；断线恢复先查 operation 再同 key 重试 */
  const handleCraft = async () => {
    if (craftStatus === 'submitting') return
    setCraftStatus('submitting')
    setCraftError(null)
    const key = startPendingOperation('craft', recipe.name)
    let committed = false
    try {
      await craftPill(key, recipe.id, recipe.current_revision_id)
      committed = true
    } catch {
      try {
        committed = (await recoverOperation(key)) !== null
      } catch {
        committed = false
      }
    }
    if (committed) {
      clearPendingOperation(key)
      setCraftStatus('success')
      onCrafted?.()
      successTimerRef.current = setTimeout(() => setCraftStatus('idle'), 2000)
    } else {
      setCraftStatus('error')
    }
  }

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={recipe.name}
      onClick={navigate}
      onKeyDown={(event) => {
        // 仅当按键源自卡片本身才导航;焦点在内部按钮上时放行,
        // 交由按钮自身的键盘激活(Enter/Space→click,其 onClick 已 stopPropagation)
        if (event.target !== event.currentTarget) return
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          navigate()
        }
      }}
      className="dao-card group relative flex h-full cursor-pointer flex-col p-5 transition-shadow focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
    >
      {/* 顶部：图标 + 版本 + 归档 */}
      <div className="mb-3 flex items-start justify-between gap-2">
        <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_15px_30px_-12px_rgba(201,169,110,0.4)] transition-all duration-300 group-hover:scale-110">
          <BookOpen className="h-7 w-7" />
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {recipe.archived_at && (
            <span className="shrink-0 whitespace-nowrap rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-[10px] text-primary">
              {t('archivedBadge')}
            </span>
          )}
          <span className="shrink-0 whitespace-nowrap rounded-full border border-border/70 bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
            {t('revisionLabel', { revision: recipe.revision })}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <h3 className="mb-1.5 min-w-0 truncate font-serif text-lg font-bold text-foreground transition-colors group-hover:text-gold">
        {recipe.name}
      </h3>

      {/* 库存数量 */}
      <p className="mb-3 flex-1 text-sm text-muted-foreground">
        {recipe.available_count > 0 ? (
          <span className="text-gold">{t('availableCount', { count: recipe.available_count })}</span>
        ) : (
          <span className="italic text-muted-foreground/60">{t('noInventory')}</span>
        )}
      </p>

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <FlaskConical className="h-3 w-3" />
      </div>

      {/* 底部信息 + 操作 */}
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="flex min-w-0 items-center gap-1 truncate">
          <Clock className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{formatDateTime(recipe.created_at)}</span>
        </span>
        <ChevronRight className="h-5 w-5 shrink-0 text-sage transition-colors group-hover:text-gold" />
      </div>

      {/* 操作列：炼制 1 枚 / 导出 Skill / 编辑新版本 */}
      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 border-t border-border/60 pt-3">
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            void handleCraft()
          }}
          disabled={craftStatus === 'submitting' || Boolean(recipe.archived_at)}
          className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/80 transition-colors hover:text-gold disabled:opacity-50"
          title={t('craftCta')}
        >
          {craftStatus === 'submitting' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : craftStatus === 'success' ? (
            <span className="text-sage">✓</span>
          ) : (
            <FlaskConical className="h-3.5 w-3.5" />
          )}
          {craftStatus === 'submitting'
            ? t('crafting')
            : craftStatus === 'success'
              ? t('crafted')
              : t('craftCta')}
        </button>
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            setShowExport(true)
          }}
          className="flex items-center gap-1 whitespace-nowrap text-xs text-muted-foreground transition-colors hover:text-gold"
          title={t('exportSkillCta')}
        >
          <Download className="h-3.5 w-3.5" />
          {t('exportSkillCta')}
        </button>
        {!recipe.archived_at && (
          <button
            type="button"
            onClick={(event) => {
              event.stopPropagation()
              router.push(recipeEditHref(recipe.id))
            }}
            className="flex items-center gap-1 whitespace-nowrap text-xs text-muted-foreground transition-colors hover:text-gold"
            title={t('editCta')}
          >
            <Pencil className="h-3.5 w-3.5" />
            {t('editCta')}
          </button>
        )}
      </div>

      {/* 炼制失败：保留错误信息 */}
      {craftStatus === 'error' && (
        <p role="alert" className="mt-2 text-xs text-primary">
          {craftError ?? t('craftFailed')}
        </p>
      )}

      {showExport && (
        <SkillExportDialog
          recipe={{
            id: recipe.id,
            name: recipe.name,
            revisionId: recipe.current_revision_id,
          }}
          onClose={() => setShowExport(false)}
        />
      )}
    </div>
  )
}
