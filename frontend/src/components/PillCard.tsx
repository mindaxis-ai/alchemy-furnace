/**
 * 金丹卡片组件 - 道教丹药瓶视觉风格
 * 显示金丹名称、标签、作者/版本、内置标识
 * 支持「赠予道人」快捷绑定操作
 */
import { Link } from 'react-router-dom'
import { CircleDot, Clock, ChevronRight, FlaskConical, Tag, UserPlus } from 'lucide-react'
import type { Pill } from '@/services/types'
import { formatDateTime } from '@/utils/format'

interface PillCardProps {
  pill: Pill
  /** 点击「赠予道人」快捷绑定 */
  onBind?: (pill: Pill) => void
}

export default function PillCard({ pill, onBind }: PillCardProps) {
  return (
    <div className="dao-card flex flex-col p-5 group h-full relative">
      {/* 顶部：图标 + 内置标识 */}
      <div className="flex items-start justify-between mb-3">
        <div className={`
          w-14 h-14 rounded-2xl flex items-center justify-center
          bg-gold-500/15 text-gold-400 glow-gold
          transition-all duration-300 group-hover:scale-110
        `}>
          <FlaskConical className="w-7 h-7" />
        </div>
        <div className="flex items-center gap-2">
          {pill.is_builtin && (
            <span className="text-[10px] px-2 py-0.5 rounded-full border bg-jade-500/20 text-jade-300 border-jade-500/30">
              内置
            </span>
          )}
          <span className="text-[10px] px-2 py-0.5 rounded-full border bg-ink-500/30 text-ink-400 border-ink-400/20">
            v{pill.version || '1.0.0'}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <Link to={`/pills/${pill.id}`}>
        <h3 className="font-serif font-bold text-lg text-rice-paper-100 group-hover:text-gold-300 transition-colors mb-1.5">
          {pill.name}
        </h3>
      </Link>

      {/* 描述 */}
      {pill.description && (
        <p className="text-sm text-ink-400 line-clamp-2 mb-3 flex-1">
          {pill.description}
        </p>
      )}

      {/* 标签 */}
      {pill.tags && pill.tags.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 mb-3">
          <Tag className="w-3 h-3 text-ink-500" />
          {pill.tags.slice(0, 4).map(tag => (
            <span
              key={tag}
              className="text-[10px] px-1.5 py-0.5 rounded bg-bronze-600/15 text-bronze-400/90 border border-bronze-600/20"
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
      <div className="flex items-center justify-between text-xs text-ink-400">
        <span className="flex items-center gap-1">
          <Clock className="w-3.5 h-3.5" />
          {formatDateTime(pill.created_at)}
        </span>
        <div className="flex items-center gap-2">
          {onBind && (
            <button
              onClick={() => onBind(pill)}
              className="flex items-center gap-1 text-xs text-gold-400/70 hover:text-gold-300 transition-colors"
              title="将此金丹赠予一位道人服用"
            >
              <UserPlus className="w-3.5 h-3.5" />
              赠予道人
            </button>
          )}
          <Link
            to={`/pills/${pill.id}`}
            className="flex items-center text-ink-500 group-hover:text-gold-400 transition-colors"
          >
            <ChevronRight className="w-5 h-5" />
          </Link>
        </div>
      </div>
    </div>
  )
}
