'use client'

/**
 * 金丹库存实例详情页（金丹消耗品重构任务 6 Phase D）
 * - GET /pill-items/:itemId 读取任意状态实例：可用 / 已服用 / 已融合 / 已弃置，
 *   展示状态徽标、去向说明与消耗时间；不再弹「不存在或已删除」。
 * - 旧金丹 ID（任务 5 封堵后）在实例查询 404 时走显式 legacy 解析入口
 *   （GET /pills/:uuid）→ 命中展示「已升级为丹方」并跳转丹方详情；双 404 才判不存在。
 * - 服用/弃置动作属于 Phase E（服用对话框 + 幂等 pending），本页只读展示。
 */
import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  AlertCircle,
  ArrowLeft,
  CircleDashed,
  Clock,
  FlaskConical,
  Loader2,
  Sparkles,
  Tag,
} from 'lucide-react'
import { getPillItem, resolveLegacyPill } from '@/services/pillInventoryService'
import { ConsumePillModal } from '@/components/consume-pill-modal'
import { PillWorkspacePage } from '@/components/layout/pill-workspace-layout'
import { ApiError } from '@/services/api'
import { recipeDetailHref } from '@/lib/entity-detail-route'
import { formatDateTime } from '@/utils/format'
import type { PillItemState } from '@/lib/pill-inventory-state'
import type { PillItemDetail, PillLegacyPointer } from '@/services/types'

/** 状态 → 徽标文案键（pill.*）与展示样式 */
const STATE_BADGE: Record<PillItemState, { label: string; className: string }> = {
  available: { label: 'stateAvailable', className: 'border-sage/30 bg-sage/15 text-sage' },
  consumed_by_agent: { label: 'stateConsumedByAgent', className: 'border-gold/30 bg-gold/15 text-gold' },
  consumed_by_fusion: { label: 'stateConsumedByFusion', className: 'border-sky/30 bg-sky/15 text-sky' },
  discarded: { label: 'stateDiscarded', className: 'border-border/70 bg-muted text-muted-foreground' },
}

/** 状态 → 去向说明文案键（pill.*） */
const STATE_DESC: Record<PillItemState, string> = {
  available: 'stateDescAvailable',
  consumed_by_agent: 'stateDescConsumedByAgent',
  consumed_by_fusion: 'stateDescConsumedByFusion',
  discarded: 'stateDescDiscarded',
}

type DetailPhase =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; item: PillItemDetail }
  | { kind: 'legacy'; pointer: PillLegacyPointer }
  | { kind: 'not-found' }

interface PillItemDetailPageProps {
  /** 库存实例 UUID（与丹方 recipeId / 道人 effectId 严格区分） */
  itemId?: string
}

export default function PillItemDetailPage({ itemId }: PillItemDetailPageProps) {
  const t = useTranslations('pill')
  const [phase, setPhase] = useState<DetailPhase>({ kind: 'loading' })
  const [showConsume, setShowConsume] = useState(false)

  const load = useCallback(async () => {
    if (!itemId) return
    setPhase({ kind: 'loading' })
    try {
      const item = await getPillItem(itemId)
      setPhase({ kind: 'ready', item })
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        // 旧金丹 ID：实例不存在时查询显式 legacy 解析入口（任务 5 封堵）
        try {
          const pointer = await resolveLegacyPill(itemId)
          setPhase({ kind: 'legacy', pointer })
        } catch (legacyErr) {
          // 双 404 才判「不存在」；legacy 网络错误按普通错误态展示
          if (legacyErr instanceof ApiError && legacyErr.status === 404) {
            setPhase({ kind: 'not-found' })
          } else {
            setPhase({
              kind: 'error',
              message: legacyErr instanceof Error ? legacyErr.message : String(legacyErr),
            })
          }
        }
      } else {
        setPhase({
          kind: 'error',
          message: err instanceof Error ? err.message : String(err),
        })
      }
    }
  }, [itemId])

  useEffect(() => {
    void load()
  }, [load])

  // ========== 链接无效 / 加载 / 错误 / 不存在 四态 ==========
  if (!itemId) {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{t('invalidLink')}</p>
          <Link href="/pills" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {t('backToList')}
          </Link>
        </div>
    </PillWorkspacePage>
    )
  }

  if (phase.kind === 'loading') {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="mb-3 h-8 w-8 animate-spin text-gold" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
    </PillWorkspacePage>
    )
  }

  if (phase.kind === 'error') {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <div role="alert" className="mb-4 max-w-xl break-words text-center text-sm text-muted-foreground">
            {phase.message}
          </div>
          <button type="button" onClick={() => void load()} className="dao-btn-ghost">
            {t('retry')}
          </button>
        </div>
    </PillWorkspacePage>
    )
  }

  if (phase.kind === 'not-found') {
    return (
      <PillWorkspacePage>
        <div className="flex flex-col items-center justify-center py-16">
          <AlertCircle className="mb-3 h-12 w-12 text-primary" />
          <p className="text-sm text-muted-foreground">{t('notFound')}</p>
          <Link href="/pills" className="dao-btn-primary mt-4 whitespace-nowrap">
            <ArrowLeft className="h-4 w-4" />
            {t('backToList')}
          </Link>
        </div>
    </PillWorkspacePage>
    )
  }

  // ========== 旧金丹 → 已升级为丹方（legacy 解析命中） ==========
  if (phase.kind === 'legacy') {
    return (
      <PillWorkspacePage>
        <Link
          href="/pills"
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-gold"
        >
          <ArrowLeft className="h-4 w-4" />
          {t('backToList')}
        </Link>
        <div className="dao-card flex flex-col items-center px-6 py-14 text-center">
          <Sparkles className="mb-3 h-12 w-12 text-gold" />
          <h1 className="mb-2 font-serif text-xl font-bold">{t('legacyTitle')}</h1>
          <p className="mb-6 max-w-md text-sm leading-relaxed text-muted-foreground">
            {t('legacyDesc')}
          </p>
          <Link
            href={recipeDetailHref(phase.pointer.recipe_id)}
            className="dao-btn-primary whitespace-nowrap"
          >
            {t('viewRecipeCta')}
          </Link>
        </div>
    </PillWorkspacePage>
    )
  }

  // ========== 实例详情（任意状态只读） ==========
  const { item } = phase
  const badge = STATE_BADGE[item.state]
  const consumed = item.state !== 'available' && item.consumed_at

  return (
    <PillWorkspacePage>
      <Link
        href="/pills"
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-gold"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('backToList')}
      </Link>

      <div className="dao-card mb-6 p-5 md:p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-start">
          <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_0_18px_rgba(201,169,110,0.35)]">
            <FlaskConical className="h-8 w-8" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="min-w-0 font-serif text-2xl font-bold text-foreground [overflow-wrap:anywhere]">{item.name}</h1>
              <span className={`shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-xs ${badge.className}`}>
                {t(badge.label)}
              </span>
              {item.archived_at && (
                <span className="shrink-0 whitespace-nowrap rounded-full border border-border/70 bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                  {t('archivedBadge')}
                </span>
              )}
            </div>
            {item.description && (
              <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">
                {item.description}
              </p>
            )}
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
              <span>{t('revisionLabel', { revision: item.revision })}</span>
              <span className="min-w-0 [overflow-wrap:anywhere]">
                {t('versionLabel')}: {item.version_label}
              </span>
              <span className="flex items-center gap-1">
                <Clock className="h-3.5 w-3.5" />
                {formatDateTime(item.created_at)}
              </span>
              {consumed && (
                <span className="flex items-center gap-1">
                  <Tag className="h-3.5 w-3.5" />
                  {t('consumedAtLabel')}: {formatDateTime(consumed)}
                </span>
              )}
            </div>
            <div className="mt-5 flex flex-col gap-3 border-t border-border/60 pt-4 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
              <Link
                href={recipeDetailHref(item.recipe_id)}
                className="inline-flex min-h-10 items-center gap-1.5 self-start whitespace-nowrap text-sm text-gold transition-colors hover:text-gold/80 focus-visible:outline-2 focus-visible:outline-primary"
              >
                {t('recipeOrigin')}
                <span aria-hidden="true"> →</span>
              </Link>
              {/* 可用实例：服用入口（消耗库存、能力保留；成功后重读状态） */}
              {item.state === 'available' && (
                <button
                  type="button"
                  onClick={() => setShowConsume(true)}
                  className="dao-btn-primary min-h-10 w-full shrink-0 whitespace-nowrap text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary sm:w-auto"
                >
                  <FlaskConical className="h-4 w-4 shrink-0" />
                  {t('consumeCta')}
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* 状态去向说明 */}
      <div className="dao-card flex items-start gap-3 p-5">
        <CircleDashed className="mt-0.5 h-4 w-4 shrink-0 text-gold" />
        <p className="text-sm leading-relaxed text-muted-foreground">{t(STATE_DESC[item.state])}</p>
      </div>

      {/* 服用对话框：提交成功才移除库存并更新能力；失败保持原状 */}
      {showConsume && (
        <ConsumePillModal
          itemId={item.id}
          itemName={item.name}
          onClose={() => setShowConsume(false)}
          onConsumed={() => {
            setShowConsume(false)
            void load()
          }}
        />
      )}
    </PillWorkspacePage>
  )
}
