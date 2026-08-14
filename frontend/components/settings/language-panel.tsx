'use client'

/**
 * 设置页「语言」面板
 * 复用了原 navbar 上的 LanguageSwitcher（下拉风格一致），
 * 把它从常驻展示位挪到这里：存在感更弱，符合"设置项"的语义。
 *
 * 切换行为沿用 LanguageSwitcher 的实现：写 localStorage + cookie
 * （让 middleware 在访问 `/` 时也能识别偏好），
 * 然后 `window.location.href = '/<locale>'` 做硬跳转到目标语言首页。
 * 当前页（如 /pills/[id]）没有 locale 镜像，硬跳转回到首页是合理折中。
 */
import { Globe } from 'lucide-react'
import { useTranslations } from 'next-intl'
import { LanguageSwitcher } from '@/components/i18n/language-switcher'

export function LanguagePanel() {
  const t = useTranslations('settings.language')

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部（与 profile / fire-effect / about 同款版式） */}
      <div className="mb-6 flex items-center gap-3 min-w-0">
        <Globe className="h-6 w-6 shrink-0 text-gold" />
        <div className="min-w-0">
          <h1 className="page-title truncate">{t('pageTitle')}</h1>
          <p className="page-subtitle truncate">{t('pageSubtitle')}</p>
        </div>
      </div>

      <div className="space-y-6">
        <section className="dao-card p-5">
          <div className="mb-4 flex items-center gap-2 min-w-0">
            <Globe className="h-5 w-5 shrink-0 text-gold" />
            <h2 className="truncate text-base font-serif font-bold text-gold">
              {t('sectionTitle')}
            </h2>
          </div>

          <p className="mb-5 text-sm leading-relaxed text-muted-foreground">
            {t('description')}
          </p>

          <div className="flex items-center gap-3">
            <LanguageSwitcher className="!text-sm" />
            <span className="text-xs text-muted-foreground">
              {t('hint')}
            </span>
          </div>
        </section>
      </div>
    </div>
  )
}
