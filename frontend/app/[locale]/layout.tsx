import { notFound } from 'next/navigation'
import { getMessages, setRequestLocale } from 'next-intl/server'
import type { ReactNode } from 'react'
import { Navbar } from '@/components/layout/navbar'
import { Footer } from '@/components/layout/footer'
import { LocaleProvider } from '@/components/i18n/locale-provider'
import { locales, type Locale } from '@/i18n'

/**
 * Locale-scoped layout.
 *
 * Owns the i18n provider AND the global chrome (Navbar / Footer). The
 * chrome must live here — not in the root layout — because the root
 * layout cannot read the active locale without `headers()`, which
 * breaks `output: 'export'`'s static prerender of `/_not-found`.
 *
 * `setRequestLocale` is required for static rendering under
 * `output: 'export'`; it tells next-intl which locale the page tree
 * should be rendered for.
 *
 * 这里使用 `LocaleProvider` 但不启用 `preferStored`,因为 locale 前缀路由
 * (`/zh-CN`、`/en`)应以 URL 为准,尊重直接访问的链接。
 */
export function generateStaticParams() {
  return locales.map((locale) => ({ locale }))
}

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode
  params: Promise<{ locale: string }>
}) {
  const { locale } = await params

  if (!(locales as readonly string[]).includes(locale)) {
    notFound()
  }

  setRequestLocale(locale)
  // getMessages 仍为 next-intl 插件所需;LocaleProvider 内部会按 locale 选消息。
  await getMessages()

  return (
    <LocaleProvider initialLocale={locale as Locale}>
      <Navbar />
      <div className="flex-1">{children}</div>
      <Footer />
    </LocaleProvider>
  )
}
