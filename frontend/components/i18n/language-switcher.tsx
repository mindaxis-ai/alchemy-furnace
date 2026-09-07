'use client'

import { useEffect, useRef, useState, useTransition } from 'react'
import { useLocale, useTranslations } from 'next-intl'
import { Check, ChevronDown, Globe } from 'lucide-react'
import { locales, type Locale } from '@/i18n'
import { cn } from '@/lib/utils'
import { persistLocale } from '@/components/i18n/locale-provider'

/**
 * Language switcher — a dropdown.
 *
 * Lives in the navbar on every page. Selecting a locale navigates to
 * the home page of the target locale (`/<locale>`). For the MVP we only
 * ship the home page under `[locale]/`; the other top-level routes
 * (`/pills`, `/chat`, …) don't have locale-prefixed versions yet, so
 * switching from one of those pages always lands on the new locale's
 * home. Once those routes are migrated, this component can be extended
 * to preserve the current sub-path.
 *
 * UX:
 *   - Trigger is a button that shows the *active* locale's name plus a
 *     rotating chevron. `aria-haspopup="listbox"` + `aria-expanded` wire
 *     it up for screen readers.
 *   - The panel is a real `role="listbox"` of `role="option"` items with
 *     `aria-selected` for the active entry.
 *   - Closes on: outside click (`mousedown` so it beats blur races), Esc
 *     key, and after a selection. Selecting the current locale is a
 *     no-op.
 *
 * Implementation note: `mousedown` (not `click`) is used for the
 * outside-click handler so a click that begins outside but ends inside
 * the panel still closes the menu — `click` would only fire on the
 * bubbling phase after focus has already shifted.
 */
export function LanguageSwitcher({ className }: { className?: string }) {
  const currentLocale = useLocale() as Locale
  const t = useTranslations('languageSwitcher')
  const [open, setOpen] = useState(false)
  const [pending, startTransition] = useTransition()
  const rootRef = useRef<HTMLDivElement>(null)

  // Outside click + Esc to close.
  useEffect(() => {
    if (!open) return

    const onMouseDown = (e: MouseEvent) => {
      if (!rootRef.current) return
      if (rootRef.current.contains(e.target as Node)) return
      setOpen(false)
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const handleSelect = (next: Locale) => {
    if (next === currentLocale) {
      setOpen(false)
      return
    }
    setOpen(false)
    // 持久化到 localStorage,确保跨页面、刷新后仍然生效
    persistLocale(next)
    // Hard navigation so the root layout re-runs and the new locale's
    // metadata / fonts apply cleanly. Using `next/navigation`'s router
    // would only re-render the [locale] subtree, which is also fine —
    // but a hard nav mirrors what a fresh page load looks like, which
    // is what users expect from a language switcher.
    startTransition(() => {
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- 有意硬导航:让 root layout 重跑,新 locale 的字体/metadata 干净应用(见上方注释)
      window.location.href = `/${next}`
    })
  }

  return (
    <div
      ref={rootRef}
      className={cn('relative inline-block', className)}
    >
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={pending}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={t('label')}
        className={cn(
          'inline-flex items-center gap-1.5 rounded-full border border-border/70 bg-card/80 px-3 py-1.5 text-xs font-medium text-foreground shadow-[0_4px_12px_-6px_rgba(60,40,20,0.12)] transition-colors duration-200 hover:border-primary/40 hover:text-primary',
          pending && 'opacity-60',
        )}
      >
        <Globe className="size-3.5 text-muted-foreground" aria-hidden />
        <span>{t(currentLocale)}</span>
        <ChevronDown
          className={cn(
            'size-3.5 text-muted-foreground transition-transform duration-200',
            open && 'rotate-180',
          )}
          aria-hidden
        />
      </button>

      {open && (
        <ul
          role="listbox"
          aria-label={t('label')}
          className="absolute right-0 z-50 mt-2 min-w-[10rem] overflow-hidden rounded-xl border border-border/70 bg-card/95 p-1 shadow-[0_20px_40px_-12px_rgba(60,40,20,0.18)] backdrop-blur-md"
        >
          {locales.map((loc) => {
            const active = loc === currentLocale
            return (
              <li key={loc} role="option" aria-selected={active}>
                <button
                  type="button"
                  onClick={() => handleSelect(loc)}
                  disabled={pending}
                  className={cn(
                    'flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors duration-200',
                    active
                      ? 'bg-primary/10 text-primary'
                      : 'text-foreground hover:bg-secondary/70',
                  )}
                >
                  <span className="font-medium">{t(loc)}</span>
                  {active && (
                    <Check className="size-3.5 text-primary" aria-hidden />
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
