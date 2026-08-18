'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useTranslations } from 'next-intl'
import { ChevronDown, Flame, Menu, X } from 'lucide-react'
import { navItems } from '@/components/layout/nav-config'
import { NavDropdown } from '@/components/layout/nav-dropdown'
import { cn } from '@/lib/utils'

/**
 * Dify 式顶部导航栏 v2（参考 reference/导航栏v2.png）
 * - 通栏浅色底 + 底部细边框；导航项居左，圆角药丸 hover/激活态
 * - 右侧主行动按钮「开炉论道」（对应 Dify 的「开始使用」）
 * - 含子项的导航项 hover/focus/点击展开 mega-dropdown
 * - Esc / 点击外部 / 鼠标移出 / 路由变化关闭
 * - 移动端（<md）降级为汉堡抽屉，不渲染下拉面板
 */
export function Navbar() {
  const pathname = usePathname()
  const t = useTranslations('nav')
  const [open, setOpen] = useState<string | null>(null)
  const [mobileOpen, setMobileOpen] = useState(false)
  const [scrolled, setScrolled] = useState(false)
  const rootRef = useRef<HTMLElement>(null)
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Nav links are absolute paths. When we are under `/[locale]/...`,
  // strip the leading `/<locale>` so `isActive` matches the segment
  // portion users actually see.
  const stripLocale = (p: string) => {
    const m = p.match(/^\/(zh-CN|en)(\/|$)/)
    return m ? (m[2] ? `/${p.slice(m[0].length)}` : '/') : p
  }
  const localPath = stripLocale(pathname)

  const isActive = (path: string) =>
    path === '/' ? localPath === '/' : localPath.startsWith(path)

  /* 滚动加深 */
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 10)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  /* 路由变化关闭（渲染期调整，避免级联渲染） */
  const [prevPath, setPrevPath] = useState(localPath)
  if (prevPath !== localPath) {
    setPrevPath(localPath)
    setOpen(null)
    setMobileOpen(false)
  }

  /* Esc 与点击外部关闭 */
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(null)
    const onClick = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(null)
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('mousedown', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('mousedown', onClick)
    }
  }, [open])

  const scheduleClose = () => {
    closeTimer.current = setTimeout(() => setOpen(null), 120)
  }
  const cancelClose = () => {
    if (closeTimer.current) clearTimeout(closeTimer.current)
  }

  return (
    <header
      ref={rootRef}
      className={cn(
        'app-drag sticky top-0 z-50 border-b transition-all duration-300',
        scrolled || open
          ? 'border-border bg-card shadow-[0_10px_30px_-15px_rgba(60,40,20,0.15)]'
          : 'border-border/60 bg-card',
      )}
    >
      <div className="flex h-16 items-center justify-between px-5 sm:px-8 lg:px-14">
        {/* 桌面端导航（居左，无品牌区） */}
        <nav aria-label={t('ariaLabel')} className="hidden items-center gap-1 lg:flex">
          {navItems.map((item) => {
            const active = isActive(item.path)
            const hasChildren = !!item.children?.length
            const expanded = open === item.labelKey
            return (
              <div
                key={item.path}
                className="relative"
                onMouseEnter={() => {
                  if (!hasChildren) return
                  cancelClose()
                  setOpen(item.labelKey)
                }}
                onMouseLeave={scheduleClose}
              >
                <Link
                  href={item.path}
                  aria-current={active ? 'page' : undefined}
                  aria-haspopup={hasChildren ? 'true' : undefined}
                  aria-expanded={hasChildren ? expanded : undefined}
                  onClick={(e) => {
                    if (hasChildren) {
                      e.preventDefault()
                      setOpen(expanded ? null : item.labelKey)
                    }
                  }}
                  onFocus={() => hasChildren && setOpen(item.labelKey)}
                  className={cn(
                    'group relative flex items-center gap-1 px-4 py-2 text-[15px] font-medium transition-colors duration-300',
                    active || expanded
                      ? 'text-foreground'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  {t(item.labelKey)}
                  {hasChildren && (
                    <ChevronDown
                      className={cn(
                        'size-3.5 transition-transform duration-300',
                        expanded && 'rotate-180',
                      )}
                      aria-hidden
                    />
                  )}
                  {/* 朱砂下划线：激活常显，hover 淡入 */}
                  <span
                    aria-hidden
                    className={cn(
                      'absolute inset-x-4 bottom-0 h-[2px] origin-left transition-transform duration-300',
                      active || expanded
                        ? 'scale-x-100 bg-primary'
                        : 'scale-x-0 bg-primary/50 group-hover:scale-x-100',
                    )}
                  />
                </Link>
              </div>
            )
          })}
        </nav>

        {/* 右侧：主行动按钮 */}
        <div className="hidden items-center gap-3 lg:flex">
          <Link
            href="/chat"
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-[15px] font-medium text-primary-foreground shadow-[0_10px_20px_-8px_rgba(181,74,63,0.5)] transition-all duration-300 hover:bg-cinnabar/90 hover:shadow-[0_14px_24px_-8px_rgba(181,74,63,0.55)]"
          >
            <Flame className="size-4" strokeWidth={2} aria-hidden />
            {t('startCta')}
          </Link>
        </div>

        {/* 移动端：汉堡按钮 */}
        <div className="flex items-center gap-2 lg:hidden">
          <button
            type="button"
            className="grid size-10 place-items-center text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            aria-label={mobileOpen ? t('closeMenu') : t('openMenu')}
            aria-expanded={mobileOpen}
            onClick={() => setMobileOpen(!mobileOpen)}
          >
            {mobileOpen ? <X className="size-5" /> : <Menu className="size-5" />}
          </button>
        </div>
      </div>

      {/* mega-dropdown 面板（桌面端） */}
      {open && (
        <div
          className="absolute inset-x-0 top-full hidden lg:block"
          onMouseEnter={cancelClose}
          onMouseLeave={scheduleClose}
        >
          <NavDropdown
            items={navItems.find((i) => i.labelKey === open)?.children ?? []}
            onNavigate={() => setOpen(null)}
          />
        </div>
      )}

      {/* 移动端抽屉菜单 */}
      {mobileOpen && (
        <nav
          aria-label={t('mobileAriaLabel')}
          className="border-t border-border/70 bg-card/95 backdrop-blur-md lg:hidden"
        >
          <div className="space-y-1 px-4 py-3">
            {navItems.map((item) => {
              const active = isActive(item.path)
              const Icon = item.icon
              return (
                <div key={item.path}>
                  <Link
                    href={item.path}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'relative flex items-center gap-3 px-4 py-3 text-sm font-medium transition-colors',
                      active
                        ? 'text-primary'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {active && (
                      <span aria-hidden className="absolute left-0 top-1/2 h-5 w-[2px] -translate-y-1/2 bg-primary" />
                    )}
                    <Icon className="size-4" strokeWidth={1.75} aria-hidden />
                    {t(item.labelKey)}
                  </Link>
                  {item.children && (
                    <div className="ml-11 space-y-0.5 pb-1">
                      {item.children.map((c) => (
                        <Link
                          key={c.titleKey}
                          href={c.path}
                          className="block px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-secondary/70 hover:text-foreground"
                        >
                          {t(c.titleKey)}
                        </Link>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </nav>
      )}
    </header>
  )
}
