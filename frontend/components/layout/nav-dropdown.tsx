'use client'

import Link from 'next/link'
import { useTranslations } from 'next-intl'
import type { NavChild } from '@/components/layout/nav-config'

/**
 * Dify 式 mega-dropdown 面板（参考 reference/导航栏.png）
 * 全宽浅色面板，两列网格：圆形描边图标 + 标题 + 一句话描述
 *
 * Each child's `titleKey` / `descKey` is resolved here against the
 * active locale's message dictionary.
 */
export function NavDropdown({
  items,
  onNavigate,
}: {
  items: NavChild[]
  onNavigate: () => void
}) {
  const t = useTranslations('nav')
  return (
    <div
      role="menu"
      className="absolute inset-x-0 top-full border-b border-border/70 bg-card/95 shadow-[0_30px_60px_-20px_rgba(60,40,20,0.18)] backdrop-blur-md"
    >
      <div className="mx-auto grid max-w-5xl gap-1 px-6 py-6 sm:grid-cols-2">
        {items.map((item) => {
          const Icon = item.icon
          return (
            <Link
              key={item.titleKey}
              href={item.path}
              role="menuitem"
              onClick={onNavigate}
              className="group flex items-start gap-4 px-4 py-3 transition-colors duration-300 hover:bg-secondary/70"
            >
              <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full ring-1 ring-gold/30 bg-gold/10 text-gold transition-colors duration-300 group-hover:text-primary group-hover:ring-primary/30 group-hover:bg-primary/10">
                <Icon className="size-4" strokeWidth={1.75} aria-hidden />
              </span>
              <span className="min-w-0">
                <span className="block truncate font-serif text-sm font-bold text-foreground">
                  {t(item.titleKey)}
                </span>
                <span className="mt-0.5 block text-sm leading-snug text-muted-foreground">
                  {t(item.descKey)}
                </span>
              </span>
            </Link>
          )
        })}
      </div>
    </div>
  )
}
