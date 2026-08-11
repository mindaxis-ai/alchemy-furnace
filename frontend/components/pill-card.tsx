'use client'

/**
 * 金丹卡片组件 - 浅色宣纸风
 * 显示金丹名称、标签、作者/版本、内置标识
 * 支持「赠予道人」快捷绑定操作
 */
import Link from 'next/link'
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
  return (
    <div className="dao-card flex flex-col p-5 group h-full relative">
      {/* 顶部：图标 + 内置标识 */}
      <div className="flex items-start justify-between gap-2 mb-3">
        <div className={`
          w-14 h-14 rounded-2xl flex items-center justify-center shrink-0
          bg-gold/15 text-gold shadow-[0_15px_30px_-12px_rgba(201,169,110,0.4)]
          transition-all duration-300 group-hover:scale-110
        `}>
          <FlaskConical className="w-7 h-7" />
        </div>
        <div className="flex items-center gap-2 min-w-0">
          {pill.is_builtin && (
            <span className="text-[10px] px-2 py-0.5 rounded-full border bg-sage/20 text-sage border-sage/30 whitespace-nowrap shrink-0">
              {t('builtInBadge')}
            </span>
          )}
          <span className="text-[10px] px-2 py-0.5 rounded-full border bg-muted text-muted-foreground border-border/70 whitespace-nowrap shrink-0">
            v{pill.version || '1.0.0'}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <Link href={`/pills/${pill.id}`} className="min-w-0">
        <h3 className="font-serif font-bold text-lg text-foreground group-hover:text-gold transition-colors mb-1.5 truncate">
          {pill.name}
        </h3>
      </Link>

      {/* 描述 */}
      {pill.description ? (
        <p className="text-sm text-muted-foreground line-clamp-2 mb-3 flex-1">
          {pill.description}
        </p>
      ) : (
        <p className="text-sm text-muted-foreground/60 italic mb-3 flex-1">
          {t('noDescription')}
        </p>
      )}

      {/* 标签 */}
      {pill.tags && pill.tags.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 mb-3">
          <Tag className="w-3 h-3 text-sage" />
          {pill.tags.slice(0, 4).map(tag => (
            <span
              key={tag}
              className="text-[10px] px-1.5 py-0.5 rounded bg-accent text-sage border border-border/70"
            >
              {tag}
            </span>
          ))}
        </div>
      )}

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <CircleDot className="w-3 h-3" />
      </div>

      {/* 底部信息 */}
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="flex items-center gap-1 min-w-0 truncate">
          <Clock className="w-3.5 h-3.5 shrink-0" />
          <span className="truncate">{formatDateTime(pill.created_at)}</span>
        </span>
        <div className="flex items-center gap-2 shrink-0">
          {onBind && (
            <button
              onClick={() => onBind(pill)}
              className="flex items-center gap-1 text-xs text-gold/80 hover:text-gold transition-colors whitespace-nowrap"
              title={t('bestowTitle')}
            >
              <UserPlus className="w-3.5 h-3.5" />
              {t('bestowCta')}
            </button>
          )}
          <Link
            href={`/pills/${pill.id}`}
            className="flex items-center text-sage group-hover:text-gold transition-colors"
          >
            <ChevronRight className="w-5 h-5" />
          </Link>
        </div>
      </div>
    </div>
  )
}
