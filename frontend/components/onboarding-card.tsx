'use client'

/**
 * 首启引导卡 - 当无任何模型供应商时,引导用户进入设置页
 *
 * 触发: chat-view 加载时 listProviders 拉取为空
 * 行为: 渲染居中卡片 + 跳转按钮 → /settings
 * 兼容: serve 与 desktop 模式同行为(无模式判断)
 */
import { useTranslations } from 'next-intl'
import { useRouter } from 'next/navigation'
import { Settings, Sparkles } from 'lucide-react'

export function OnboardingCard() {
  const t = useTranslations('onboarding')
  const router = useRouter()

  return (
    <div className="mx-auto max-w-md rounded-xl border border-gold/30 bg-card p-8 text-center shadow-sm">
      <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-gold/10">
        <Sparkles className="h-7 w-7 text-gold" />
      </div>
      <h2 className="text-lg font-serif font-bold text-foreground">
        {t('title')}
      </h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {t('description')}
      </p>
      <button
        onClick={() => router.push('/settings')}
        className="dao-btn-primary mt-6"
      >
        <Settings className="h-4 w-4" />
        {t('action')}
      </button>
    </div>
  )
}
