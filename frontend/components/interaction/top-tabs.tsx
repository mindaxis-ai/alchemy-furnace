'use client'

import { useLayoutEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export interface Tab {
  key: string
  label: string
}

/**
 * 黑神话式顶部标签条（参考 reference/交互.png）
 * 下划线指示器以 transform 滑动到激活项，仅 transform/opacity 动画
 */
export function TopTabs({
  tabs,
  activeKey,
  onChange,
  className,
}: {
  tabs: Tab[]
  activeKey: string
  onChange: (key: string) => void
  className?: string
}) {
  const tabRefs = useRef<Map<string, HTMLButtonElement>>(new Map())
  const [indicator, setIndicator] = useState({ x: 0, w: 0, ready: false })

  useLayoutEffect(() => {
    const el = tabRefs.current.get(activeKey)
    if (!el) return
    setIndicator({ x: el.offsetLeft, w: el.offsetWidth, ready: true })
  }, [activeKey, tabs])

  return (
    <div
      role="tablist"
      className={cn(
        'relative flex max-w-full items-center gap-1 overflow-x-auto border-b border-border/70',
        '[scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
        className,
      )}
    >
      {tabs.map((tab) => {
        const active = tab.key === activeKey
        return (
          <button
            key={tab.key}
            ref={(el) => {
              if (el) tabRefs.current.set(tab.key, el)
            }}
            role="tab"
            aria-selected={active}
            onClick={() => onChange(tab.key)}
            className={cn(
              'relative shrink-0 whitespace-nowrap px-4 py-3 font-serif text-sm font-bold transition-colors duration-300',
              active ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {tab.label}
          </button>
        )
      })}
      {/* 滑动下划线指示器 */}
      <span
        aria-hidden
        className={cn(
          'absolute bottom-0 left-0 h-0.5 rounded-full bg-primary transition-all duration-300 ease-out',
          !indicator.ready && 'opacity-0',
        )}
        style={{ transform: `translateX(${indicator.x}px)`, width: indicator.w }}
      />
    </div>
  )
}
