'use client'

/**
 * 金丹卡片组件 - 浅色宣纸风
 * 整卡为单一键盘可访问的导航容器(role="link"),点击/Enter/Space 进入详情;
 * 内部「赠予道人」按钮阻止冒泡,避免双重导航。
 */
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import { CircleDot, Clock, ChevronRight, FlaskConical, Tag, UserPlus } from 'lucide-react'
import type { Pill } from '@/services/types'
import { formatDateTime } from '@/utils/format'

interface PillCardProps {
  pill: Pill
  /** 点击「赠予道人」快捷绑定 */
  onBind?: (pill: Pill) => void
}

export function PillCard({ pill, onBind }: PillCardProps) {
  const t = useTranslations('pillCard')
  const router = useRouter()
  const href = `/pills/${pill.id}`

  const navigate = () => router.push(href)

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={pill.name}
      onClick={navigate}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          navigate()
        }
      }}
      className="dao-card group relative flex h-full cursor-pointer flex-col p-5 transition-shadow focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
    >
      {/* 顶部：图标 + 内置标识 */}
      <div className="mb-3 flex items-start justify-between gap-2">
        <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gold/15 text-gold shadow-[0_15px_30px_-12px_rgba(201,169,110,0.4)] transition-all duration-300 group-hover:scale-110">
          <FlaskConical className="h-7 w-7" />
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {pill.is_builtin && (
            <span className="shrink-0 whitespace-nowrap rounded-full border border-sage/30 bg-sage/20 px-2 py-0.5 text-[10px] text-sage">
              {t('builtInBadge')}
            </span>
          )}
          <span className="shrink-0 whitespace-nowrap rounded-full border border-border/70 bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
            v{pill.version || '1.0.0'}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <h3 className="mb-1.5 min-w-0 truncate font-serif text-lg font-bold text-foreground transition-colors group-hover:text-gold">
        {pill.name}
      </h3>

      {/* 描述 */}
      {pill.description ? (
        <p className="mb-3 line-clamp-2 flex-1 text-sm text-muted-foreground">{pill.description}</p>
      ) : (
        <p className="mb-3 flex-1 text-sm italic text-muted-foreground/60">{t('noDescription')}</p>
      )}

      {/* 标签 */}
      {pill.tags && pill.tags.length > 0 && (
        <div className="mb-3 flex flex-wrap items-center gap-1.5">
          <Tag className="h-3 w-3 text-sage" />
          {pill.tags.slice(0, 4).map((tag) => (
            <span
              key={tag}
              className="rounded border border-border/70 bg-accent px-1.5 py-0.5 text-[10px] text-sage"
            >
              {tag}
            </span>
          ))}
        </div>
      )}

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <CircleDot className="h-3 w-3" />
      </div>

      {/* 底部信息 */}
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="flex min-w-0 items-center gap-1 truncate">
          <Clock className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{formatDateTime(pill.created_at)}</span>
        </span>
        <div className="flex shrink-0 items-center gap-2">
          {onBind && (
            <button
              type="button"
              onClick={(event) => {
                // 阻止冒泡到整卡导航容器,避免双重导航
                event.stopPropagation()
                onBind(pill)
              }}
              className="flex items-center gap-1 whitespace-nowrap text-xs text-gold/80 transition-colors hover:text-gold"
              title={t('bestowTitle')}
            >
              <UserPlus className="h-3.5 w-3.5" />
              {t('bestowCta')}
            </button>
          )}
          <ChevronRight className="h-5 w-5 text-sage transition-colors group-hover:text-gold" />
        </div>
      </div>
    </div>
  )
}
