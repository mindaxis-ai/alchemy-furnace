'use client'

/**
 * @ 提及补全浮层（飞书式）
 * 渲染在 textarea 上方的备选道人列表。键盘上下选择 + Enter 选中,
 * 鼠标 hover 高亮,click 选中。
 *
 * 由 chat-view 的 ChatInput 控制位置和显示状态。
 */
import { useEffect, useRef } from 'react'
import { useTranslations } from 'next-intl'
import { AtSign } from 'lucide-react'
import type { GroupMember } from '@/services/types'

export interface MentionSuggestProps {
  candidates: GroupMember[]
  activeIndex: number
  onPick: (name: string) => void
  onHover: (i: number) => void
  /** 绝对定位:bottom 是相对视口底,left 是相对视口左 */
  position: { bottom: number; left: number }
}

export function MentionSuggest({
  candidates,
  activeIndex,
  onPick,
  onHover,
  position,
}: MentionSuggestProps) {
  const t = useTranslations('mention')
  const listRef = useRef<HTMLDivElement>(null)

  // 滚动保持 active 项可见
  useEffect(() => {
    if (!listRef.current) return
    const active = listRef.current.querySelector(`[data-idx="${activeIndex}"]`) as HTMLElement
    if (active) {
      active.scrollIntoView({ block: 'nearest' })
    }
  }, [activeIndex])

  if (candidates.length === 0) {
    return (
      <div
        style={{ position: 'fixed', bottom: position.bottom, left: position.left, zIndex: 30 }}
        className="px-3 py-2 rounded-lg bg-card border border-border text-xs text-muted-foreground shadow-lg"
      >
        {t('noMatches')}
      </div>
    )
  }

  return (
    <div
      ref={listRef}
      role="listbox"
      style={{ position: 'fixed', bottom: position.bottom, left: position.left, zIndex: 30 }}
      className="
        w-72 max-h-64 overflow-y-auto rounded-xl
        bg-card border border-gold/25
        shadow-2xl shadow-black/25
        animate-in fade-in slide-in-from-bottom-1 duration-100
      "
    >
      <div className="sticky top-0 bg-card/95 backdrop-blur px-3 py-2 text-[10px] tracking-[0.12em] text-muted-foreground border-b border-border/50 flex items-center gap-1.5">
        <AtSign className="w-3 h-3" />
        {t('allMembers')}
      </div>
      {candidates.map((c, i) => {
        const active = i === activeIndex
        return (
          <button
            key={c.agent_id}
            type="button"
            role="option"
            aria-selected={active}
            data-idx={i}
            onMouseDown={e => e.preventDefault() /* 防止 blur 关闭 */}
            onClick={() => onPick(c.name)}
            onMouseEnter={() => onHover(i)}
            className={`
              w-full flex items-center gap-2.5 px-3 py-2 text-left
              transition-colors duration-75
              ${active ? 'bg-gold/15 text-foreground' : 'text-foreground/80 hover:bg-secondary/60'}
            `}
          >
            <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold text-xs shrink-0 overflow-hidden">
              {c.avatar ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={c.avatar} alt={c.name} className="h-full w-full object-cover" />
              ) : c.name.charAt(0)}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-medium truncate">{c.name}</p>
              <p className="text-[10px] text-muted-foreground">表达欲 {c.proactivity}</p>
            </div>
            {active && <span className="text-[10px] text-gold">↵</span>}
          </button>
        )
      })}
    </div>
  )
}
