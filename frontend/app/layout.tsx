import type { ReactNode } from 'react'
import type { Metadata } from 'next'
import { Noto_Serif_SC, Noto_Sans_SC } from 'next/font/google'
import { Providers } from '@/components/providers'
import DesktopGuards from '@/components/desktop-guards'
import './globals.css'

const notoSerifSC = Noto_Serif_SC({
  subsets: ['latin'],
  weight: ['400', '700', '900'],
  variable: '--font-noto-serif-sc',
})

const notoSansSC = Noto_Sans_SC({
  subsets: ['latin'],
  weight: ['400', '500', '700'],
  variable: '--font-noto-sans-sc',
})

/**
 * Root layout — intentionally minimal and static.
 *
 * Constraints that shaped this layout:
 *   1. `output: 'export'` means the framework's auto-generated
 *      `/_not-found` page must be renderable WITHOUT touching
 *      `headers()`. So we cannot call `getLocale()` / `getMessages()`
 *      here — they would crash the build.
 *   2. The Navbar / Footer are translated via `useTranslations`,
 *      which requires a `NextIntlClientProvider`. That provider lives
 *      in `app/[locale]/layout.tsx`. The chrome therefore also lives
 *      there, not here.
 *   3. Non-locale MVP routes (`/agents`, `/chat`, `/pills`, `/models`,
 *      `/settings`) still need the client-side `Providers` (Pill /
 *      Agent / Chat contexts) to work, so we keep those here.
 *
 * Net effect: chrome is rendered only for pages under `[locale]/`.
 * Migrating the other routes under `[locale]/` in a follow-up will
 * restore the chrome for them automatically.
 */
export const metadata: Metadata = {
  title: {
    default: '炼丹炉 · Skill-Persona Alchemy',
    template: '%s · 炼丹炉',
  },
  description: '以火为引，以药为基 —— 语言模式金丹与道人 Agent 的炼制之所',
  icons: { icon: '/favicon.svg' },
}

// Hard-fail to static rendering so the not-found page can be exported
// without needing the request object.
export const dynamic = 'force-static'

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="zh-CN" className={`${notoSerifSC.variable} ${notoSansSC.variable}`}>
      <body className="flex min-h-screen flex-col">
        <Providers>
          <DesktopGuards />
          {children}
        </Providers>
      </body>
    </html>
  )
}
