import { setRequestLocale } from 'next-intl/server'
import type { ReactNode } from 'react'
import { Navbar } from '@/components/layout/navbar'
import { Footer } from '@/components/layout/footer'
import { LocaleProvider } from '@/components/i18n/locale-provider'
import { defaultLocale } from '@/i18n'

/**
 * Layout for non-locale MVP routes (`/agents`, `/chat`, `/pills`,
 * `/models`, `/settings`).
 *
 * Wraps the subtree in the chrome (Navbar / Footer) so these pages
 * have the same global navigation as the home page under
 * `app/[locale]/`. The parenthesized folder name `(main)` is a Next.js
 * route group — it does NOT appear in the URL.
 *
 * Why `setRequestLocale(defaultLocale)` is called before the provider:
 *   The next-intl plugin (configured in `next.config.mjs`) hooks into
 *   every layout in the app. When the request locale is undefined
 *   (non-locale routes), `i18n.ts`'s `getRequestConfig` calls
 *   `notFound()`, and the entire page prerenders as a 404 — even
 *   though the page itself is fine. Setting the request locale to the
 *   default here mirrors what `app/[locale]/layout.tsx` does for
 *   locale routes, and is what makes the chrome render at all.
 *
 * `LocaleProvider` 在浏览器端会读取 localStorage 中保存的 locale,让
 * 语言切换在这些非 locale 路由下也能全局生效。
 */
export default function MainLayout({ children }: { children: ReactNode }) {
  setRequestLocale(defaultLocale)
  return (
    <LocaleProvider initialLocale={defaultLocale} preferStored>
      <Navbar />
      <div className="flex-1">{children}</div>
      <Footer />
    </LocaleProvider>
  )
}
